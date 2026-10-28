// Package server is the phone-facing web app: pick a movie, pick a TV, play.
//
// The TV never sees a Movies token. It gets /s/<playID>/<name>.mkv on this
// server, and this server forwards the bytes with the token in a header. The
// play ID dies on Stop, on a new play on the same TV, on logout and after
// playMaxAge. Reason: the LG keeps the last URL after Stop and hands it to
// anyone on the LAN who asks GetPositionInfo (measured 2026-10-28).
package server

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"movies-cast/internal/dlna"
	"movies-cast/internal/movies"
)

//go:embed web
var webFS embed.FS

const (
	cookieName      = "mc_s"
	sessionMaxAge   = 30 * 24 * time.Hour
	playMaxAge      = 24 * time.Hour
	rendererTTL     = 5 * time.Minute
	reportInterval  = 10 * time.Second
	startupDeadline = 60 * time.Second
	// A TV switched off mid-film stops answering; after this long the play ends.
	unreachableLimit = 2 * time.Minute
	maxSessions      = 200
	loginFailLimit   = 5
	loginFailWindow  = 10 * time.Minute
)

// Renderers abstracts discovery so tests can inject fakes.
type Renderers interface {
	Find(ctx context.Context, refresh bool) ([]dlna.Renderer, error)
}

// Transport is the subset of dlna.Client the server uses.
type Transport interface {
	SetURI(ctx context.Context, r dlna.Renderer, m dlna.Media) error
	Play(ctx context.Context, r dlna.Renderer) error
	Pause(ctx context.Context, r dlna.Renderer) error
	Stop(ctx context.Context, r dlna.Renderer) error
	Seek(ctx context.Context, r dlna.Renderer, pos time.Duration) error
	Status(ctx context.Context, r dlna.Renderer) (dlna.Status, error)
	Volume(ctx context.Context, r dlna.Renderer) (int, error)
	SetVolume(ctx context.Context, r dlna.Renderer, v int) error
	Sink(ctx context.Context, r dlna.Renderer) (string, error)
}

type Config struct {
	PublicURL string // optional, e.g. http://10.0.0.143:8097; else derived per TV
	Port      int    // port this server listens on, for the derived URL
	StateFile string // optional; sessions survive restarts there (0600)
}

type Server struct {
	cfg         Config
	movies      *movies.Client
	tv          Transport
	renderers   Renderers
	stream      *http.Client
	now         func() time.Time
	pollEvery   time.Duration
	reportEvery time.Duration
	unreachable time.Duration

	mu       sync.Mutex
	sessions map[string]*session
	plays    map[string]*play // by play ID
	byTV     map[string]*play // active play per renderer ID
	// probedSubs keeps what PlaybackInfo listed per film, so it is asked once
	// per film and server start (each answer starts a transcode on the box).
	probedSubs map[string]probedSubs
	// proven holds TV|container|codec combinations a TV has played for
	// provenAfter; their format hint is skipped until the server restarts.
	proven map[string]bool

	loginFails map[string][]time.Time // by client IP
	pairings   map[string]*pairing    // TV-app pairings by secret ID

	browserSilence time.Duration
}

type session struct {
	ms        *movies.Session
	lastSeen  time.Time
	savedSeen time.Time // lastSeen as last written to the state file
}

type play struct {
	id        string
	sess      *session
	movie     movies.Movie
	ext       string // container as sent to the TV
	tv        dlna.Renderer
	psID      string
	created   time.Time
	ctx       context.Context // ends when the play is revoked: stops watcher and running streams
	cancel    context.CancelFunc
	sawPlay   bool
	lastPos   time.Duration
	lastState string
	browser   bool // played by the TV app's <video>, not via DLNA
	subs      []movies.Subtitle
	msID      string    // media source ID the subtitle URLs need
	subsLater bool      // the item listed none; ask PlaybackInfo when subtitles are wanted
	lastBeat  time.Time // last progress from the TV app
	paused    bool
}

func New(cfg Config, mc *movies.Client, tv Transport, r Renderers) *Server {
	srv := &Server{
		cfg: cfg, movies: mc, tv: tv, renderers: r,
		// No overall timeout: a movie stream runs for hours. Header timeout only.
		stream: &http.Client{
			Transport: &http.Transport{ResponseHeaderTimeout: 30 * time.Second, Proxy: nil},
			// Go strips Authorization on a cross-host redirect, not X-Emby-Authorization.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		loginFails:     map[string][]time.Time{},
		pairings:       map[string]*pairing{},
		now:            time.Now,
		pollEvery:      time.Second,
		reportEvery:    reportInterval,
		unreachable:    unreachableLimit,
		browserSilence: browserSilence,
		sessions:       map[string]*session{},
		plays:          map[string]*play{},
		byTV:           map[string]*play{},
		probedSubs:     map[string]probedSubs{},
		proven:         map[string]bool{},
	}
	if err := srv.loadSessions(); err != nil {
		log.Printf("state: %v (starting without stored sessions)", err)
	}
	if err := srv.loadProven(); err != nil {
		log.Printf("formats: %v (starting without proven formats)", err)
	}
	return srv
}

func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /", http.FileServerFS(static))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		// The webOS app (origin file://) probes this before it opens /tv/; it
		// says nothing but "ok", so any origin may read it.
		w.Header().Set("Access-Control-Allow-Origin", "*")
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.withSession(s.logout))
	mux.HandleFunc("GET /api/me", s.withSession(s.me))
	mux.HandleFunc("GET /api/movies", s.withSession(s.listMovies))
	mux.HandleFunc("GET /api/poster/{id}", s.withSession(s.poster))
	mux.HandleFunc("GET /api/renderers", s.withSession(s.listRenderers))
	mux.HandleFunc("POST /api/play", s.withSession(s.startPlay))
	mux.HandleFunc("POST /api/control", s.withSession(s.control))
	mux.HandleFunc("GET /api/status", s.withSession(s.status))
	mux.HandleFunc("POST /api/tv/pair/start", s.tvPairStart)
	mux.HandleFunc("GET /api/tv/pair/poll", s.tvPairPoll)
	mux.HandleFunc("POST /api/tv/pair/approve", s.withSession(s.tvPairApprove))
	mux.HandleFunc("POST /api/tv/play", s.withSession(s.tvPlay))
	mux.HandleFunc("POST /api/tv/progress", s.withSession(s.tvProgress))
	mux.HandleFunc("GET /api/tv/subs/{play}/{file}", s.withSession(s.tvSubtitle))
	mux.HandleFunc("POST /api/tv/subs-probe", s.withSession(s.tvSubsProbe))
	mux.HandleFunc("GET /api/resume", s.withSession(s.listResume))
	mux.HandleFunc("GET /s/{play}/{name}", s.serveStream)
	mux.HandleFunc("HEAD /s/{play}/{name}", s.serveStream)
	return securityHeaders(sameOrigin(mux))
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		if !strings.HasPrefix(r.URL.Path, "/s/") {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'")
		}
		h.ServeHTTP(w, r)
	})
}

// sameOrigin rejects state-changing requests from other sites. A browser
// sends Origin on every cross-origin POST; a same-origin fetch sends our host.
func sameOrigin(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if o := r.Header.Get("Origin"); o != "" {
				u, err := url.Parse(o)
				if err != nil || u.Host != r.Host {
					http.Error(w, "fremde Herkunft", http.StatusForbidden)
					return
				}
			}
			if sf := r.Header.Get("Sec-Fetch-Site"); sf != "" && sf != "same-origin" && sf != "none" {
				http.Error(w, "fremde Herkunft", http.StatusForbidden)
				return
			}
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				http.Error(w, "JSON erwartet", http.StatusUnsupportedMediaType)
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// fail answers with the German message and its English text (errorEn); the
// page shows the one of its language. TestEveryFailMessageHasEnglish keeps
// errorsEn complete.
func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg, "errorEn": english(msg)})
}

var errorsEn = map[string]string{
	"Movies nicht erreichbar":                                     "Movies not reachable",
	"Fernseher nicht gefunden":                                    "TV not found",
	"Zu viele Fehlversuche, bitte in einigen Minuten erneut":      "Too many failed attempts, please try again in a few minutes",
	"ungültige Anfrage":                                           "invalid request",
	"nicht angemeldet":                                            "not logged in",
	"zu viele offene Kopplungen, bitte in einigen Minuten erneut": "too many open pairings, please try again in a few minutes",
	"Wiedergabe wurde inzwischen ersetzt oder gestoppt":           "the playback was replaced or stopped in the meantime",
	"Wiedergabe unbekannt":                                        "unknown playback",
	"ungültige ID":                                                "invalid ID",
	"unbekannte Aktion":                                           "unknown action",
	"Suche nach Fernsehern fehlgeschlagen":                        "search for TVs failed",
	"Movies antwortet nicht wie erwartet":                         "Movies does not answer as expected",
	"Kopplung abgelaufen":                                         "pairing expired",
	"keine Netzverbindung zum Fernseher":                          "no network connection to the TV",
	"Film und Fernseher angeben":                                  "choose a film and a TV",
	"Film angeben":                                                "choose a film",
	"Fernseher startet nicht":                                     "the TV does not start",
	"Fernseher hat nicht reagiert":                                "the TV did not react",
	"Fernseher hat den Film abgelehnt":                            "the TV refused the film",
	"Fernseher antwortet nicht":                                   "the TV does not answer",
	"Code unbekannt oder abgelaufen":                              "unknown or expired code",
	"Benutzer und Passwort angeben":                               "enter user and password",
	"Benutzer oder Passwort falsch":                               "wrong user or password",
	"Anmeldung bei Movies abgelaufen":                             "Movies login expired",
	"6-stelligen Code angeben":                                    "enter the 6-digit code",
}

// english translates msg; a renderer suffix " (UPnP-Fehler N)" from faultCode
// is kept with its number. Unknown text stays German rather than empty.
func english(msg string) string {
	base, suffix, _ := strings.Cut(msg, " (UPnP-Fehler ")
	en, ok := errorsEn[base]
	if !ok {
		return msg
	}
	if suffix != "" {
		en += " (UPnP error " + suffix
	}
	return en
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(v)
}

type handlerWithSession func(http.ResponseWriter, *http.Request, string, *session)

func (s *Server) withSession(h handlerWithSession) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil {
			fail(w, http.StatusUnauthorized, "nicht angemeldet")
			return
		}
		s.mu.Lock()
		sess := s.sessions[c.Value]
		if sess != nil && s.now().Sub(sess.lastSeen) > sessionMaxAge {
			s.dropSessionLocked(c.Value)
			sess = nil
		}
		if sess != nil {
			sess.lastSeen = s.now()
			if sess.lastSeen.Sub(sess.savedSeen) > seenSaveEvery {
				s.saveSessionsLocked()
			}
		}
		s.mu.Unlock()
		if sess == nil {
			fail(w, http.StatusUnauthorized, "nicht angemeldet")
			return
		}
		h(w, r, c.Value, sess)
	}
}

// moviesErr maps a Movies error; an expired Movies token ends our session too.
func (s *Server) moviesErr(w http.ResponseWriter, sid string, err error) {
	if errors.Is(err, movies.ErrUnauthorized) {
		s.mu.Lock()
		s.dropSessionLocked(sid)
		s.mu.Unlock()
		fail(w, http.StatusUnauthorized, "Anmeldung bei Movies abgelaufen")
		return
	}
	log.Printf("movies: %v", err)
	fail(w, http.StatusBadGateway, "Movies antwortet nicht wie erwartet")
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ User, Password string }
	if err := readJSON(r, &in); err != nil || in.User == "" || in.Password == "" {
		fail(w, http.StatusBadRequest, "Benutzer und Passwort angeben")
		return
	}
	ip := clientIP(r)
	if s.loginBlocked(ip) {
		fail(w, http.StatusTooManyRequests, "Zu viele Fehlversuche, bitte in einigen Minuten erneut")
		return
	}
	sid := randomID()
	ms, err := s.movies.Login(r.Context(), in.User, in.Password, "movies-cast-"+sid[:12])
	if errors.Is(err, movies.ErrUnauthorized) {
		s.loginFailed(ip)
		fail(w, http.StatusUnauthorized, "Benutzer oder Passwort falsch")
		return
	}
	if err != nil {
		log.Printf("login: %v", err)
		fail(w, http.StatusBadGateway, "Movies nicht erreichbar")
		return
	}
	s.mu.Lock()
	s.sweepSessionsLocked()
	s.sessions[sid] = &session{ms: ms, lastSeen: s.now()}
	delete(s.loginFails, ip)
	s.saveSessionsLocked()
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: sid, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: int(sessionMaxAge / time.Second)})
	writeJSON(w, http.StatusOK, map[string]string{"name": ms.UserName})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, sid string, sess *session) {
	s.mu.Lock()
	s.dropSessionLocked(sid)
	// A paired TV app shares this Movies login; logging it out at Movies
	// would log the TV out too. Only the last session ends the Movies login.
	shared := false
	for _, other := range s.sessions {
		if other.ms == sess.ms {
			shared = true
		}
	}
	s.mu.Unlock()
	if !shared {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Second)
		defer cancel()
		if err := s.movies.Logout(ctx, sess.ms); err != nil {
			log.Printf("logout at movies: %v", err)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// sweepSessionsLocked drops expired sessions and, above maxSessions, the
// least recently used ones. Without it every login (new phone, cleared
// cookies) left a Movies token in memory for good. Caller holds s.mu.
func (s *Server) sweepSessionsLocked() {
	for id, ss := range s.sessions {
		if s.now().Sub(ss.lastSeen) > sessionMaxAge {
			s.dropSessionLocked(id)
		}
	}
	for len(s.sessions) >= maxSessions {
		var oldest string
		for id, ss := range s.sessions {
			if oldest == "" || ss.lastSeen.Before(s.sessions[oldest].lastSeen) {
				oldest = id
			}
		}
		s.dropSessionLocked(oldest)
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) recentFailsLocked(ip string) []time.Time {
	var keep []time.Time
	for _, t := range s.loginFails[ip] {
		if s.now().Sub(t) < loginFailWindow {
			keep = append(keep, t)
		}
	}
	if len(keep) == 0 {
		delete(s.loginFails, ip)
	} else {
		s.loginFails[ip] = keep
	}
	return keep
}

func (s *Server) loginBlocked(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.recentFailsLocked(ip)) >= loginFailLimit
}

func (s *Server) loginFailed(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loginFails[ip] = append(s.recentFailsLocked(ip), s.now())
}

// faultCode turns a renderer error into a short suffix for the user. The
// renderer's own text stays in the log: it comes from whatever device
// answered SSDP and is not shown to the browser.
func faultCode(err error) string {
	var f *dlna.Fault
	if errors.As(err, &f) && f.Code != 0 {
		return " (UPnP-Fehler " + strconv.Itoa(f.Code) + ")"
	}
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "Timeout") {
		return " (keine Antwort)"
	}
	return ""
}

// dropSessionLocked removes a session and every play it owns. Caller holds s.mu.
func (s *Server) dropSessionLocked(sid string) {
	sess := s.sessions[sid]
	delete(s.sessions, sid)
	defer s.saveSessionsLocked()
	for id, p := range s.plays {
		if p.sess == sess {
			s.endPlayLocked(id)
		}
	}
}

func (s *Server) me(w http.ResponseWriter, _ *http.Request, _ string, sess *session) {
	writeJSON(w, http.StatusOK, map[string]string{"name": sess.ms.UserName})
}

func (s *Server) listMovies(w http.ResponseWriter, r *http.Request, sid string, sess *session) {
	ms, err := s.movies.Movies(r.Context(), sess.ms)
	if err != nil {
		s.moviesErr(w, sid, err)
		return
	}
	writeJSON(w, http.StatusOK, ms)
}

func (s *Server) poster(w http.ResponseWriter, r *http.Request, sid string, sess *session) {
	req, err := s.movies.ImageRequest(r.Context(), sess.ms, r.PathValue("id"), 400)
	if err != nil {
		fail(w, http.StatusBadRequest, "ungültige ID")
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
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = io.Copy(w, resp.Body)
}

type rendererOut struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Model string `json:"model"`
	Host  string `json:"host"`
}

func (s *Server) listRenderers(w http.ResponseWriter, r *http.Request, _ string, _ *session) {
	rs, err := s.renderers.Find(r.Context(), r.URL.Query().Get("refresh") == "1")
	if err != nil && len(rs) == 0 {
		log.Printf("renderers: %v", err)
		fail(w, http.StatusBadGateway, "Suche nach Fernsehern fehlgeschlagen")
		return
	}
	out := make([]rendererOut, 0, len(rs))
	for _, x := range rs {
		out = append(out, rendererOut{ID: x.ID, Name: x.Name, Model: x.Model, Host: x.Host()})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) renderer(ctx context.Context, id string) (dlna.Renderer, bool) {
	rs, _ := s.renderers.Find(ctx, false)
	for _, x := range rs {
		if x.ID == id {
			return x, true
		}
	}
	return dlna.Renderer{}, false
}

// streamBase is the URL prefix the TV uses to reach this server.
func (s *Server) streamBase(ctx context.Context, tv dlna.Renderer) (string, error) {
	if s.cfg.PublicURL != "" {
		return strings.TrimRight(s.cfg.PublicURL, "/"), nil
	}
	// The local address the kernel picks to reach the TV is the one the TV can reach back.
	var d net.Dialer
	c, err := d.DialContext(ctx, "udp4", net.JoinHostPort(tv.Host(), "1900"))
	if err != nil {
		return "", err
	}
	defer c.Close()
	la, ok := c.LocalAddr().(*net.UDPAddr)
	if !ok {
		return "", fmt.Errorf("unexpected local address %v", c.LocalAddr())
	}
	return "http://" + net.JoinHostPort(la.IP.String(), strconv.Itoa(s.cfg.Port)), nil
}

func (s *Server) startPlay(w http.ResponseWriter, r *http.Request, sid string, sess *session) {
	var in struct {
		Renderer string `json:"renderer"`
		Movie    string `json:"movie"`
		Resume   bool   `json:"resume"`
		Force    bool   `json:"force"` // send although the TV does not list the format
	}
	if err := readJSON(r, &in); err != nil || in.Renderer == "" || in.Movie == "" {
		fail(w, http.StatusBadRequest, "Film und Fernseher angeben")
		return
	}
	tv, ok := s.renderer(r.Context(), in.Renderer)
	if !ok {
		fail(w, http.StatusNotFound, "Fernseher nicht gefunden")
		return
	}
	m, err := s.movies.Movie(r.Context(), sess.ms, in.Movie)
	if err != nil {
		s.moviesErr(w, sid, err)
		return
	}
	ext := m.Container
	if ext == "" {
		ext = "mkv"
	}
	s.mu.Lock()
	proven := s.proven[formatKey(tv, ext, m.VideoCodec)]
	s.mu.Unlock()
	if !in.Force && !proven {
		// A hint, not a block: the LG lists no HEVC and plays it (dlna.Unlisted).
		sink, err := s.tv.Sink(r.Context(), tv)
		if err != nil {
			log.Printf("protocol info %s: %v", tv.Name, err)
		} else if miss := dlna.Unlisted(sink, ext, m.VideoCodec); len(miss) > 0 {
			list := strings.Join(miss, ", ")
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":    "Dieser Fernseher nennt " + list + " nicht in seiner Formatliste, das Bild bleibt dann womöglich schwarz.",
				"errorEn":  "This TV does not list " + list + " among its formats; the screen may stay black.",
				"unlisted": miss,
			})
			return
		}
	}
	base, err := s.streamBase(r.Context(), tv)
	if err != nil {
		fail(w, http.StatusBadGateway, "keine Netzverbindung zum Fernseher")
		return
	}

	p := &play{id: randomID(), sess: sess, movie: m, tv: tv, ext: ext, psID: randomID(), created: s.now()}
	p.ctx, p.cancel = context.WithCancel(context.Background())
	s.mu.Lock()
	if old := s.byTV[tv.ID]; old != nil {
		s.endPlayLocked(old.id)
	}
	s.plays[p.id] = p
	s.byTV[tv.ID] = p
	s.mu.Unlock()

	media := dlna.Media{URL: base + "/s/" + p.id + "/movie." + url.PathEscape(ext), Title: m.Name, MimeType: mimeFor(ext),
		Size: m.Size, Duration: time.Duration(m.Runtime) * time.Second}
	ctx := r.Context()
	if err := s.tv.SetURI(ctx, tv, media); err != nil {
		s.abortPlay(p.id)
		log.Printf("SetURI %s: %v", tv.Name, err)
		fail(w, http.StatusBadGateway, "Fernseher hat den Film abgelehnt"+faultCode(err))
		return
	}
	if err := s.tv.Play(ctx, tv); err != nil {
		s.abortPlay(p.id)
		log.Printf("Play %s: %v", tv.Name, err)
		fail(w, http.StatusBadGateway, "Fernseher startet nicht"+faultCode(err))
		return
	}
	var resume time.Duration
	if in.Resume && m.Resume > 0 {
		resume = time.Duration(m.Resume) * time.Second
	}
	// Stop, logout or another play on this TV may have revoked p while
	// SetURI and Play were on the wire; then no watcher for a dead play.
	s.mu.Lock()
	current := s.plays[p.id] == p
	s.mu.Unlock()
	if !current {
		fail(w, http.StatusConflict, "Wiedergabe wurde inzwischen ersetzt oder gestoppt")
		return
	}
	go s.watch(p.ctx, p, resume)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "title": m.Name, "resumeSeconds": int64(resume / time.Second)})
}

// provenAfter is how long a TV must report PLAYING before its format counts
// as playable on it.
const provenAfter = 10 * time.Second

func formatKey(tv dlna.Renderer, container, codec string) string {
	return tv.ID + "|" + strings.ToLower(container) + "|" + strings.ToLower(codec)
}

func mimeFor(ext string) string {
	switch strings.ToLower(ext) {
	case "mp4", "m4v", "mov":
		return "video/mp4"
	case "ts", "m2ts":
		return "video/mp2t"
	case "avi":
		return "video/x-msvideo"
	case "webm":
		return "video/webm"
	default:
		return "video/x-matroska"
	}
}

func (s *Server) abortPlay(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.endPlayLocked(id)
}

// endPlayLocked revokes the stream URL and stops the watcher. Caller holds s.mu.
func (s *Server) endPlayLocked(id string) {
	p := s.plays[id]
	if p == nil {
		return
	}
	delete(s.plays, id)
	if s.byTV[p.tv.ID] == p {
		delete(s.byTV, p.tv.ID)
	}
	if p.cancel != nil {
		p.cancel()
	}
}

// watch seeks to the resume point once the TV plays, reports progress to
// Movies, and ends the play when the TV stops or plays something else.
func (s *Server) watch(ctx context.Context, p *play, resume time.Duration) {
	state := func(pos time.Duration, paused bool) movies.PlayState {
		return movies.PlayState{ItemID: p.movie.ID, PlaySessionID: p.psID, Position: pos, Paused: paused}
	}
	bg := context.WithoutCancel(ctx)
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
	start := s.now()
	lastOK := start
	lastReport := time.Time{}
	seeked := resume == 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		s.mu.Lock()
		sawPlay := p.sawPlay
		s.mu.Unlock()
		if s.now().Sub(p.created) > playMaxAge || (!sawPlay && s.now().Sub(start) > startupDeadline) {
			s.abortPlay(p.id)
			return
		}
		st, err := s.tv.Status(ctx, p.tv)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if s.now().Sub(lastOK) > s.unreachable {
				log.Printf("%s unreachable for %v, ending play", p.tv.Name, s.unreachable)
				s.abortPlay(p.id)
				return
			}
			continue
		}
		lastOK = s.now()
		mine := strings.Contains(st.TrackURI, "/s/"+p.id+"/")
		s.mu.Lock()
		if mine {
			p.lastState = st.State
			if st.Position > 0 {
				p.lastPos = st.Position
			}
			if st.State == "PLAYING" {
				p.sawPlay = true
				if k := formatKey(p.tv, p.ext, p.movie.VideoCodec); st.Position >= provenAfter && !s.proven[k] {
					s.proven[k] = true
					s.saveProvenLocked()
				}
			}
		}
		sawPlay = p.sawPlay
		s.mu.Unlock()

		switch {
		case !mine && sawPlay, // the TV plays something else now
			mine && sawPlay && (st.State == "STOPPED" || st.State == "NO_MEDIA_PRESENT"):
			s.abortPlay(p.id)
			return
		}
		// The LG ignores Seek until it plays; seek once, right after PLAYING.
		if !seeked && mine && st.State == "PLAYING" {
			seeked = true
			if err := s.tv.Seek(ctx, p.tv, resume); err != nil {
				log.Printf("resume seek: %v", err)
			}
		}
		if mine && sawPlay && s.now().Sub(lastReport) >= s.reportEvery {
			lastReport = s.now()
			if err := s.movies.ReportProgress(ctx, p.sess.ms, state(st.Position, st.State == "PAUSED_PLAYBACK")); err != nil && ctx.Err() == nil {
				log.Printf("report progress: %v", err)
			}
		}
	}
}

func (s *Server) control(w http.ResponseWriter, r *http.Request, _ string, sess *session) {
	var in struct {
		Renderer string  `json:"renderer"`
		Action   string  `json:"action"`
		Seconds  float64 `json:"seconds"`
		Volume   int     `json:"volume"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, http.StatusBadRequest, "ungültige Anfrage")
		return
	}
	tv, ok := s.renderer(r.Context(), in.Renderer)
	if !ok {
		fail(w, http.StatusNotFound, "Fernseher nicht gefunden")
		return
	}
	ctx := r.Context()
	var err error
	switch in.Action {
	case "pause":
		err = s.tv.Pause(ctx, tv)
	case "play":
		err = s.tv.Play(ctx, tv)
	case "seek":
		err = s.tv.Seek(ctx, tv, time.Duration(in.Seconds*float64(time.Second)))
	case "volume":
		err = s.tv.SetVolume(ctx, tv, in.Volume)
	case "stop":
		err = s.tv.Stop(ctx, tv)
		s.mu.Lock()
		if p := s.byTV[tv.ID]; p != nil && p.sess == sess {
			s.endPlayLocked(p.id)
		}
		s.mu.Unlock()
	default:
		fail(w, http.StatusBadRequest, "unbekannte Aktion")
		return
	}
	if err != nil {
		log.Printf("%s on %s: %v", in.Action, tv.Name, err)
		fail(w, http.StatusBadGateway, "Fernseher hat nicht reagiert"+faultCode(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request, _ string, _ *session) {
	tv, ok := s.renderer(r.Context(), r.URL.Query().Get("renderer"))
	if !ok {
		fail(w, http.StatusNotFound, "Fernseher nicht gefunden")
		return
	}
	st, err := s.tv.Status(r.Context(), tv)
	if err != nil {
		fail(w, http.StatusBadGateway, "Fernseher antwortet nicht")
		return
	}
	out := map[string]any{
		"state":           st.State,
		"positionSeconds": int64(st.Position / time.Second),
		"durationSeconds": int64(st.Duration / time.Second),
		"ours":            false,
	}
	s.mu.Lock()
	if p := s.byTV[tv.ID]; p != nil && strings.Contains(st.TrackURI, "/s/"+p.id+"/") {
		out["ours"] = true
		out["title"] = p.movie.Name
		out["movie"] = p.movie.ID
	}
	s.mu.Unlock()
	if v, err := s.tv.Volume(r.Context(), tv); err == nil {
		out["volume"] = v
	}
	writeJSON(w, http.StatusOK, out)
}

// serveStream forwards the original file to the TV, Range requests included.
func (s *Server) serveStream(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	p := s.plays[r.PathValue("play")]
	if p != nil && s.now().Sub(p.created) > playMaxAge {
		s.endPlayLocked(p.id)
		p = nil
	}
	var ms *movies.Session
	var id string
	var pctx context.Context
	if p != nil {
		ms, id, pctx = p.sess.ms, p.movie.ID, p.ctx
	}
	s.mu.Unlock()
	if p == nil {
		http.NotFound(w, r)
		return
	}
	// A revoked play also cuts a transfer that is already running.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	defer context.AfterFunc(pctx, cancel)()
	req, err := s.movies.StreamRequest(ctx, ms, r.Method, id)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	for _, h := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
		if v := r.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	resp, err := s.stream.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("stream %s: %v", id, err)
		}
		http.Error(w, "upstream", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		http.Error(w, "upstream auth", http.StatusBadGateway)
		return
	}
	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	// DLNA renderers look for these before seeking.
	w.Header().Set("transferMode.dlna.org", "Streaming")
	w.Header().Set("contentFeatures.dlna.org", "DLNA.ORG_OP=01;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=01700000000000000000000000000000")
	w.WriteHeader(resp.StatusCode)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.Copy(w, resp.Body)
}
