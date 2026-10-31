package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"movies-cast/internal/dlna"
)

// Kodi on Android TV (Nvidia Shield) plays the sound the WebView of the TV
// app cannot: on a tester's Shield (2026-10-30) the WebView had no sound
// for AAC and no audio track choice, while Kodi, as a DLNA renderer, played
// a film sent from the phone. Other apps (Jellyfin, Plex, Emby for Android
// TV) hand films to Kodi as their external player the same way.
//
// POST /api/tv/kodi {movie, resume} from the TV app:
//   - a DLNA renderer answers on the app's own address (Kodi with "Allow
//     remote control via UPnP"): the film goes there by DLNA, like a phone's
//     send to a TV — resume, progress, autoplay and the phone's controls work;
//     answer {"mode":"dlna"}.
//   - else: a stream URL for an Android intent to Kodi (the app opens it with
//     ACTION_VIEW); Kodi then plays from the start and reports nothing back;
//     answer {"mode":"intent","url","mime","title"}.

const kodiSearchWait = 6 * time.Second

func (s *Server) tvKodi(w http.ResponseWriter, r *http.Request, sid string, sess *session) {
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
	if !m.Playable() {
		fail(w, http.StatusBadRequest, "Eine Folge oder einen Titel wählen")
		return
	}
	ip := clientIP(r)
	if kodi, ok := s.rendererOn(r.Context(), ip); ok {
		body, _ := json.Marshal(map[string]any{"renderer": kodi.ID, "movie": m.ID, "resume": in.Resume, "force": true})
		req := httptest.NewRequestWithContext(r.Context(), http.MethodPost, "/api/play", strings.NewReader(string(body)))
		rec := httptest.NewRecorder()
		s.startPlay(rec, req, sid, sess)
		log.Printf("kodi: %s on %s (%s) by DLNA: HTTP %d", m.Name, kodi.Name, ip, rec.Code)
		if rec.Code != http.StatusOK {
			// The refusal as the phone would get it (format, unreachable, ...).
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(rec.Code)
			_, _ = w.Write(rec.Body.Bytes())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"mode": "dlna", "renderer": kodi.Name, "title": m.Name})
		return
	}
	ext := m.Container
	if ext == "" {
		ext = "mkv"
	}
	tv := dlna.Renderer{ID: "kodi:" + sid, Name: "Kodi"}
	p := &play{id: randomID(), sess: sess, movie: m, tv: tv, ext: ext, psID: randomID(), created: s.now(), dlnaSub: -1}
	p.ctx, p.cancel = context.WithCancel(context.Background())
	s.mu.Lock()
	if old := s.byTV[tv.ID]; old != nil {
		s.endPlayLocked(old.id)
	}
	s.plays[p.id] = p
	s.byTV[tv.ID] = p
	s.mu.Unlock()
	// Kodi runs on the device the app runs on and reaches this server at the
	// address the app used.
	url := "http://" + r.Host + "/s/" + p.id + "/movie." + ext
	if r.TLS != nil {
		url = s.plainURL(r) + "/s/" + p.id + "/movie." + ext
	}
	log.Printf("kodi: %s to %s by intent (no DLNA renderer on that address)", m.Name, ip)
	writeJSON(w, http.StatusOK, map[string]any{"mode": "intent", "url": url, "mime": mimeFor(ext), "title": m.Name})
}

// plainURL is the HTTP address of this server for a client that came over
// HTTPS: Kodi gets the film over HTTP like a TV (the self-signed certificate
// would stop it).
func (s *Server) plainURL(r *http.Request) string {
	host := r.Host
	if i := strings.LastIndex(host, ":"); i > 0 && !strings.HasSuffix(host, "]") {
		host = host[:i]
	}
	port := s.cfg.Port
	if port == 0 {
		port = 8097
	}
	return "http://" + host + ":" + strconv.Itoa(port)
}

// rendererOn finds a DLNA renderer on ip (not a Cast device), searching
// again when the list does not have one: Kodi answers only while it runs.
func (s *Server) rendererOn(ctx context.Context, ip string) (dlna.Renderer, bool) {
	pick := func(rs []dlna.Renderer) (dlna.Renderer, bool) {
		var best dlna.Renderer
		ok := false
		for _, x := range rs {
			if isCast(x) || x.Host() != ip {
				continue
			}
			if !ok || strings.Contains(strings.ToLower(x.Name), "kodi") {
				best, ok = x, true
			}
		}
		return best, ok
	}
	rs, _ := s.renderers.Find(ctx, false)
	if x, ok := pick(rs); ok {
		return x, true
	}
	sctx, cancel := context.WithTimeout(ctx, kodiSearchWait)
	defer cancel()
	rs, _ = s.renderers.Find(sctx, true)
	return pick(rs)
}
