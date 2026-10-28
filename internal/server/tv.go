package server

// The TV app: a webOS app that opens /tv/ on this server and plays films in
// its own <video> element. Measured on the LG OLED55B97LA (webOS 4.x,
// Chrome 53 engine, 2026-10-28): the element plays the original MKV with
// HEVC 4K and AC3/E-AC3, exposes audioTracks, and seeks in under a second;
// canPlayType("video/x-matroska") still answers "", so nothing here asks it.
//
// Login on the TV is a pairing: the TV shows a six-digit code, a phone that
// is already logged in confirms it, and the TV gets its own session that
// shares the phone's Movies login.

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"rsc.io/qr"

	"movies-cast/internal/dlna"
	"movies-cast/internal/movies"
)

const (
	pairTTL = 10 * time.Minute
	// The two tracks of Shelter are 67 KB and 68 KB (measured); 10 MB leaves
	// room and stops a runaway answer.
	maxSubtitleBytes = 10 << 20
	maxPendingPair   = 20
	// A TV app that stops sending progress (switched off, app closed) ends
	// its play after this long.
	browserSilence = 2 * time.Minute
)

type pairing struct {
	code     string
	created  time.Time
	approved *session // set by the phone
}

func pairCode() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%06d", binary.BigEndian.Uint32(b[:])%1_000_000)
}

func (s *Server) sweepPairingsLocked() {
	for id, p := range s.pairings {
		if s.now().Sub(p.created) > pairTTL {
			delete(s.pairings, id)
		}
	}
}

// tvPairStart is called by the TV app without a session.
func (s *Server) tvPairStart(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepPairingsLocked()
	if len(s.pairings) >= maxPendingPair {
		fail(w, http.StatusTooManyRequests, "zu viele offene Kopplungen, bitte in einigen Minuten erneut")
		return
	}
	code := pairCode()
	for _, p := range s.pairings { // codes must be unique among the open ones
		if p.code == code {
			code = pairCode()
		}
	}
	id := randomID()
	s.pairings[id] = &pairing{code: code, created: s.now()}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "code": code, "expiresSeconds": int(pairTTL / time.Second)})
}

// tvPairQR draws the phone link with the code as a QR code for the pairing
// screen: the phone camera opens the page with the code filled in, nothing to
// type. Keyed by the pairing's secret ID (only the TV has it), so the server
// draws no QR code for text of anyone's choosing. The link uses the host the
// TV reached this server by — the phone in the same Wi-Fi reaches it too.
func (s *Server) tvPairQR(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	s.mu.Lock()
	s.sweepPairingsLocked()
	p := s.pairings[id]
	code := ""
	if p != nil && p.approved == nil {
		code = p.code
	}
	s.mu.Unlock()
	host := r.Host
	if code == "" || host == "" || strings.ContainsAny(host, "/\\?#@ ") {
		fail(w, http.StatusNotFound, "Kopplung abgelaufen")
		return
	}
	c, err := qr.Encode("http://"+host+"/?pair="+code, qr.M)
	if err != nil {
		fail(w, http.StatusInternalServerError, "ungültige Anfrage")
		return
	}
	c.Scale = 8
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(c.PNG())
}

// tvPairPoll is polled by the TV; once the phone approved, it turns the
// pairing into a session cookie for the TV.
func (s *Server) tvPairPoll(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	s.mu.Lock()
	s.sweepPairingsLocked()
	p := s.pairings[id]
	var sid string
	var name string
	if p != nil && p.approved != nil {
		delete(s.pairings, id)
		sid = randomID()
		s.sweepSessionsLocked()
		s.sessions[sid] = &session{ms: p.approved.ms, lastSeen: s.now()}
		name = p.approved.ms.UserName
		s.saveSessionsLocked()
	}
	s.mu.Unlock()
	switch {
	case p == nil:
		fail(w, http.StatusNotFound, "Kopplung abgelaufen")
	case sid == "":
		writeJSON(w, http.StatusOK, map[string]bool{"pending": true})
	default:
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: sid, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: int(sessionMaxAge / time.Second)})
		writeJSON(w, http.StatusOK, map[string]string{"name": name})
	}
}

// tvPairApprove is called by a logged-in phone with the code the TV shows.
func (s *Server) tvPairApprove(w http.ResponseWriter, r *http.Request, _ string, sess *session) {
	var in struct {
		Code string `json:"code"`
	}
	if err := readJSON(r, &in); err != nil || len(in.Code) != 6 {
		fail(w, http.StatusBadRequest, "6-stelligen Code angeben")
		return
	}
	ip := clientIP(r)
	if s.loginBlocked(ip) {
		fail(w, http.StatusTooManyRequests, "Zu viele Fehlversuche, bitte in einigen Minuten erneut")
		return
	}
	s.mu.Lock()
	s.sweepPairingsLocked()
	var found *pairing
	for _, p := range s.pairings {
		if p.code == in.Code && p.approved == nil {
			found = p
		}
	}
	if found != nil {
		found.approved = sess
	}
	s.mu.Unlock()
	if found == nil {
		s.loginFailed(ip)
		fail(w, http.StatusNotFound, "Code unbekannt oder abgelaufen")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// tvPlay creates a play whose stream the TV app opens itself.
func (s *Server) tvPlay(w http.ResponseWriter, r *http.Request, sid string, sess *session) {
	var in struct {
		Movie  string `json:"movie"`
		Resume bool   `json:"resume"`
	}
	if err := readJSON(r, &in); err != nil || in.Movie == "" {
		fail(w, http.StatusBadRequest, "Film angeben")
		return
	}
	m, err := s.movies.Movie(r.Context(), sess.ms, in.Movie)
	if err != nil {
		s.moviesErr(w, sid, err)
		return
	}
	// Subtitles are a bonus: without them the film still plays.
	subs, msID, listed, err := s.movies.Subtitles(r.Context(), sess.ms, m.ID)
	if err != nil {
		log.Printf("subtitles %s: %v", m.ID, err)
		subs, listed = nil, true
	}
	s.mu.Lock()
	if ps, ok := s.probedSubs[m.ID]; ok && !listed {
		subs, msID, listed = ps.subs, ps.msID, true
	}
	s.mu.Unlock()
	// One play per TV-app session: a new film replaces the running one.
	tv := dlna.Renderer{ID: "app:" + sid, Name: "TV-App"}
	p := &play{id: randomID(), sess: sess, movie: m, tv: tv, psID: randomID(), created: s.now(), browser: true, lastBeat: s.now(), subs: subs, msID: msID, subsLater: !listed}
	p.ctx, p.cancel = context.WithCancel(context.Background())
	s.mu.Lock()
	if old := s.byTV[tv.ID]; old != nil {
		s.endPlayLocked(old.id)
	}
	s.plays[p.id] = p
	s.byTV[tv.ID] = p
	s.mu.Unlock()
	var resume time.Duration
	if in.Resume && m.Resume > 0 {
		resume = time.Duration(m.Resume) * time.Second
		p.lastPos = resume
	}
	go s.watchBrowser(p, resume)
	ext := m.Container
	if ext == "" {
		ext = "mkv"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"play": p.id, "url": "/s/" + p.id + "/movie." + ext, "title": m.Name,
		"resumeSeconds": int64(resume / time.Second), "durationSeconds": m.Runtime,
		"subtitles": subsOut(p.id, subs), "subtitlesOnRequest": !listed,
	})
}

type probedSubs struct {
	subs []movies.Subtitle
	msID string
}

type subOut struct {
	Label string `json:"label"`
	Lang  string `json:"lang,omitempty"`
	URL   string `json:"url"`
}

func subsOut(playID string, subs []movies.Subtitle) []subOut {
	out := make([]subOut, 0, len(subs))
	for _, st := range subs {
		out = append(out, subOut{Label: st.Label, Lang: st.Language, URL: fmt.Sprintf("/api/tv/subs/%s/%d.vtt", playID, st.Index)})
	}
	return out
}

// tvSubsProbe lists the subtitles of a film whose item answer listed none, by
// asking PlaybackInfo. That starts a transcode on the box, so the TV app asks
// only when someone wants subtitles (down key, or a remembered language).
func (s *Server) tvSubsProbe(w http.ResponseWriter, r *http.Request, sid string, sess *session) {
	var in struct{ Play string }
	if err := readJSON(r, &in); err != nil {
		fail(w, http.StatusBadRequest, "ungültige Anfrage")
		return
	}
	s.mu.Lock()
	p := s.plays[in.Play]
	if p == nil || p.sess != sess || !p.browser {
		s.mu.Unlock()
		fail(w, http.StatusNotFound, "Wiedergabe unbekannt")
		return
	}
	later, movieID := p.subsLater, p.movie.ID
	ps, cached := s.probedSubs[movieID]
	s.mu.Unlock()
	if later && !cached {
		subs, msID, err := s.movies.ProbedSubtitles(r.Context(), sess.ms, movieID)
		if err != nil {
			s.moviesErr(w, sid, err)
			return
		}
		ps = probedSubs{subs: subs, msID: msID}
	}
	s.mu.Lock()
	if later {
		s.probedSubs[movieID] = ps
		p.subs, p.msID, p.subsLater = ps.subs, ps.msID, false
	}
	subs := p.subs
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"subtitles": subsOut(p.id, subs)})
}

// tvProgress is sent by the TV app every few seconds and on pause/stop/end.
func (s *Server) tvProgress(w http.ResponseWriter, r *http.Request, _ string, sess *session) {
	var in struct {
		Play     string  `json:"play"`
		Position float64 `json:"position"`
		Paused   bool    `json:"paused"`
		Stopped  bool    `json:"stopped"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, http.StatusBadRequest, "ungültige Anfrage")
		return
	}
	s.mu.Lock()
	p := s.plays[in.Play]
	if p == nil || p.sess != sess || !p.browser {
		s.mu.Unlock()
		fail(w, http.StatusNotFound, "Wiedergabe unbekannt")
		return
	}
	if in.Position > 0 {
		p.lastPos = time.Duration(in.Position * float64(time.Second))
	}
	p.paused = in.Paused
	p.lastBeat = s.now()
	if in.Stopped {
		s.endPlayLocked(p.id) // the watcher reports the stop
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// watchBrowser reports start, progress and stop of a TV-app play to Movies.
func (s *Server) watchBrowser(p *play, resume time.Duration) {
	ctx := p.ctx
	bg := context.WithoutCancel(ctx)
	state := func(pos time.Duration, paused bool) movies.PlayState {
		return movies.PlayState{ItemID: p.movie.ID, PlaySessionID: p.psID, Position: pos, Paused: paused}
	}
	if err := s.movies.ReportStart(bg, p.sess.ms, state(resume, false)); err != nil {
		log.Printf("report start: %v", err)
	}
	defer func() {
		s.mu.Lock()
		pos := p.lastPos
		s.mu.Unlock()
		rctx, cancel := context.WithTimeout(bg, 10*time.Second)
		defer cancel()
		if err := s.movies.ReportStopped(rctx, p.sess.ms, state(pos, false)); err != nil {
			log.Printf("report stopped: %v", err)
		}
	}()
	tick := time.NewTicker(s.pollEvery)
	defer tick.Stop()
	lastReport := s.now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		s.mu.Lock()
		pos, paused, beat := p.lastPos, p.paused, p.lastBeat
		s.mu.Unlock()
		if s.now().Sub(beat) > s.browserSilence || s.now().Sub(p.created) > playMaxAge {
			s.abortPlay(p.id)
			return
		}
		if s.now().Sub(lastReport) >= s.reportEvery {
			lastReport = s.now()
			if err := s.movies.ReportProgress(ctx, p.sess.ms, state(pos, paused)); err != nil && ctx.Err() == nil {
				log.Printf("report progress: %v", err)
			}
		}
	}
}

// tvSubtitle forwards one subtitle track of the session's own play as
// WebVTT; the TV's <track> fetches it same-origin with the session cookie.
func (s *Server) tvSubtitle(w http.ResponseWriter, r *http.Request, sid string, sess *session) {
	idx, err := strconv.Atoi(strings.TrimSuffix(r.PathValue("file"), ".vtt"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	p := s.plays[r.PathValue("play")]
	ok := p != nil && p.sess == sess && p.browser
	var movieID, msID string
	if ok {
		ok = false
		for _, st := range p.subs {
			if st.Index == idx {
				ok = true
			}
		}
		movieID, msID = p.movie.ID, p.msID
	}
	s.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	req, err := s.movies.SubtitleRequest(r.Context(), sess.ms, movieID, msID, idx)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	resp, err := s.stream.Do(req)
	if err != nil {
		fail(w, http.StatusBadGateway, "Movies nicht erreichbar")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		s.moviesErr(w, sid, movies.ErrUnauthorized)
		return
	}
	if resp.StatusCode != http.StatusOK {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = io.Copy(w, io.LimitReader(resp.Body, maxSubtitleBytes))
}

// listResume is the "continue watching" row, in Movies' order.
func (s *Server) listResume(w http.ResponseWriter, r *http.Request, sid string, sess *session) {
	ms, err := s.movies.Resume(r.Context(), sess.ms)
	if err != nil {
		s.moviesErr(w, sid, err)
		return
	}
	writeJSON(w, http.StatusOK, ms)
}

// A film can go to the TV's movies-cast app instead of DLNA, when the phone
// asks for it ("app": true) after DLNA failed and the answer said the app is
// open — DLNA stays the way (owner, 2026-10-28: "DLNA is the top feature"). Measured 2026-10-28: the LG did not answer the
// DLNA Play for an HEVC MKV twice ("file not recognised" on screen) and the
// owner then started the same film in the app, where it plays.

type tvCommand struct {
	Movie  string    `json:"movie"`
	Resume bool      `json:"resume"`
	at     time.Time // dropped when no poll took it within appCmdTTL
}

const (
	appPollFresh = 10 * time.Second // the app polls every 3 s while not playing
	appCmdTTL    = 30 * time.Second
)

// appSessionLocked is the TV app session on the renderer's address that
// polled recently, or nil. Caller holds s.mu.
func (s *Server) appSessionLocked(tv dlna.Renderer) *session {
	host := tv.Host()
	var best *session
	for _, ss := range s.sessions {
		if ss.appIP == host && s.now().Sub(ss.appPolled) < appPollFresh && (best == nil || ss.appPolled.After(best.appPolled)) {
			best = ss
		}
	}
	return best
}

func (s *Server) appOpen(tv dlna.Renderer) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appSessionLocked(tv) != nil
}

// sendToApp queues the film for the TV app on the renderer's address, if one
// polled recently. Caller does not hold s.mu.
func (s *Server) sendToApp(tv dlna.Renderer, m movies.Movie, resume bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	best := s.appSessionLocked(tv)
	if best == nil {
		return false
	}
	best.cmd = &tvCommand{Movie: m.ID, Resume: resume, at: s.now()}
	return true
}

// tvCmd is polled by the TV app while it is not playing; it marks the session
// as an app on this address and hands over a film the phone chose.
func (s *Server) tvCmd(w http.ResponseWriter, r *http.Request, _ string, sess *session) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	s.mu.Lock()
	sess.appIP, sess.appPolled = host, s.now()
	cmd := sess.cmd
	sess.cmd = nil
	s.mu.Unlock()
	if cmd == nil || s.now().Sub(cmd.at) > appCmdTTL {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	writeJSON(w, http.StatusOK, cmd)
}
