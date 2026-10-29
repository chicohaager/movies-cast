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
	"sort"
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
	// failed holds combinations a TV refused or did not start via DLNA (the
	// LG: no answer to Play, "file not recognised" on screen, twice on
	// 2026-10-28); a later proven play clears the entry.
	failed map[string]bool
	// convert holds TV|audio|codec keys whose sound is converted (convert.go).
	convert map[string]bool
	// account: settings of the phone page per Movies user ID, by name
	// ("theme", "lang"; accountPrefs in store.go).
	account map[string]map[string]string
	prefs   tvPrefs // names, hidden TVs, TVs added by address (tvs.go)
	// strings is /strings.js, built once from web/i18n/*.json (strings.go).
	strings []byte

	loginFails   map[string][]time.Time // by client IP
	loginPending map[string]int         // logins waiting for Movies, by client IP
	newPairCode  func() string          // pairCode; a fixed sequence in tests
	pairings     map[string]*pairing    // TV-app pairings by secret ID

	browserSilence time.Duration

	// Wake-on-LAN (tvs.go); replaced in tests.
	macOf     func(host string) string
	wake      func(ctx context.Context, mac, host string) error
	alive     func(ctx context.Context, x dlna.Renderer) bool
	wakeWait  time.Duration
	wakeEvery time.Duration
}

type session struct {
	ms        *movies.Session
	lastSeen  time.Time
	savedSeen time.Time // lastSeen as last written to the state file
	cookieAt  time.Time // when the cookie was last issued with a fresh Max-Age
	// Set by the TV app's command poll (tvCmd): where it runs and when it last
	// asked. A phone's play for a renderer on that address goes to the app.
	appIP     string
	appPolled time.Time
	cmd       *tvCommand // waiting for the app's next poll
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
	// A converted play (convert.go): the stream starts at segment segIdx,
	// offset is the film time there; the TV counts from 0 at that point.
	conv         *movies.Conversion
	segIdx       int
	offset       time.Duration
	mime         string
	restartUntil time.Time // a converted seek re-sends the film; STOPPED until then is expected
	// peer is the address that fetched /s/ first: the TV gives the URL to
	// anyone who asks it (GetPositionInfo), so the URL alone is not enough.
	peer    string
	refused string // the last other address turned away; the phone shows it
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
		loginPending:   map[string]int{},
		pairings:       map[string]*pairing{},
		newPairCode:    pairCode,
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
		failed:         map[string]bool{},
		convert:        map[string]bool{},
		account:        map[string]map[string]string{},
		prefs:          tvPrefs{Labels: map[string]string{}, Hidden: map[string]bool{}},
		macOf:          dlna.MACOf,
		wake: func(ctx context.Context, mac, host string) error {
			return dlna.Wake(ctx, mac, dlna.WakeTargets(host))
		},
		wakeWait:  60 * time.Second,
		wakeEvery: 3 * time.Second,
	}
	srv.alive = descAlive(&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
	if err := srv.loadSessions(); err != nil {
		log.Printf("state: %v (starting without stored sessions)", err)
	}
	if err := srv.loadProven(); err != nil {
		log.Printf("formats: %v (starting without proven formats)", err)
	}
	b, err := buildStrings()
	if err != nil {
		panic(err) // embedded at build time; a broken file is caught by TestStringsComplete
	}
	srv.strings = b
	if err := srv.loadTVPrefs(); err != nil {
		log.Printf("tvs: %v (starting without TV settings)", err)
	}
	if err := srv.loadAccountPrefs(); err != nil {
		log.Printf("account settings: %v (starting without some of them)", err)
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
	mux.HandleFunc("GET /strings.js", s.stringsJS)
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.withSession(s.logout))
	mux.HandleFunc("GET /api/me", s.withSession(s.me))
	mux.HandleFunc("POST /api/theme", s.withSession(s.setAccountPref("theme")))
	mux.HandleFunc("POST /api/lang", s.withSession(s.setAccountPref("lang")))
	mux.HandleFunc("GET /api/movies", s.withSession(s.listMovies))
	mux.HandleFunc("GET /api/poster/{id}", s.withSession(s.poster))
	mux.HandleFunc("GET /api/renderers", s.withSession(s.listRenderers))
	mux.HandleFunc("GET /api/tvs", s.withSession(s.listTVs))
	mux.HandleFunc("POST /api/tvs", s.withSession(s.setTV))
	mux.HandleFunc("POST /api/tvs/hosts", s.withSession(s.setTVHost))
	mux.HandleFunc("POST /api/tvs/wake", s.withSession(s.wakeTV))
	mux.HandleFunc("GET /api/system", s.withSession(s.system))
	mux.HandleFunc("POST /api/play", s.withSession(s.startPlay))
	mux.HandleFunc("POST /api/control", s.withSession(s.control))
	mux.HandleFunc("GET /api/status", s.withSession(s.status))
	mux.HandleFunc("POST /api/tv/pair/start", s.tvPairStart)
	mux.HandleFunc("GET /api/tv/pair/poll", s.tvPairPoll)
	mux.HandleFunc("GET /api/tv/pair/qr", s.tvPairQR)
	mux.HandleFunc("POST /api/tv/pair/approve", s.withSession(s.tvPairApprove))
	mux.HandleFunc("POST /api/tv/play", s.withSession(s.tvPlay))
	mux.HandleFunc("GET /api/tv/cmd", s.withSession(s.tvCmd))
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

// fail answers with the German message, its English text (errorEn) and a key
// for every other language: "srv:<English>" in web/i18n/<lang>.json, plus the
// suffix a TV error carries ("sfx.upnp" with the code, "sfx.noAnswer").
// TestEveryFailMessageHasEnglish keeps errorsEn complete.
func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, errorBody(msg))
}

func errorBody(msg string) map[string]any {
	out := map[string]any{"error": msg, "errorEn": english(msg)}
	base, suffix := splitSuffix(msg)
	if en, ok := errorsEn[base]; ok {
		out["key"] = "srv:" + en
	}
	switch {
	case suffix == noAnswerSuffix:
		out["suffix"] = map[string]any{"key": "sfx.noAnswer"}
	case strings.HasPrefix(suffix, upnpSuffix):
		out["suffix"] = map[string]any{"key": "sfx.upnp", "args": []string{strings.TrimSuffix(strings.TrimPrefix(suffix, upnpSuffix), ")")}}
	}
	return out
}

const (
	upnpSuffix     = " (UPnP-Fehler "
	noAnswerSuffix = " (keine Antwort)"
)

// splitSuffix separates a TV error's suffix from faultCode.
func splitSuffix(msg string) (base, suffix string) {
	if b, ok := strings.CutSuffix(msg, noAnswerSuffix); ok {
		return b, noAnswerSuffix
	}
	if i := strings.Index(msg, upnpSuffix); i >= 0 {
		return msg[:i], msg[i:]
	}
	return msg, ""
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
	"Die Movies-Cast-App auf diesem Fernseher ist nicht geöffnet": "the Movies-Cast app is not open on this TV",
	"Name zu lang (höchstens 40 Zeichen)":                         "name too long (40 characters at most)",
	"Speichern fehlgeschlagen":                                    "saving failed",
	"Adresse ungültig (IPv4-Adresse im eigenen Netz)":             "invalid address (an IPv4 address on your own network)",
	"höchstens 32 Adressen":                                       "32 addresses at most",
	"Für diesen Fernseher ist keine MAC-Adresse bekannt":          "no MAC address is known for this TV",
	"Weckpaket konnte nicht gesendet werden":                      "the wake-up packet could not be sent",
	"Der Fernseher ist aus oder nicht erreichbar":                 "the TV is off or not reachable",
	"Movies konnte den Film nicht umwandeln":                      "Movies could not convert the film",
}

// english translates msg; a renderer suffix " (UPnP-Fehler N)" from faultCode
// is kept with its number. Unknown text stays German rather than empty.
func english(msg string) string {
	base, suffix := splitSuffix(msg)
	en, ok := errorsEn[base]
	if !ok {
		return msg
	}
	switch {
	case suffix == noAnswerSuffix: // was left German until 0.17 ("… (keine Antwort)")
		en += " (no answer)"
	case suffix != "":
		en += " (UPnP error " + strings.TrimPrefix(suffix, upnpSuffix)
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
		reissue := false
		if sess != nil {
			sess.lastSeen = s.now()
			if sess.lastSeen.Sub(sess.savedSeen) > seenSaveEvery {
				s.saveSessionsLocked()
			}
			// The session slides with use; the cookie must too, or the
			// browser drops it sessionMaxAge after login however often used.
			if sess.lastSeen.Sub(sess.cookieAt) > seenSaveEvery {
				sess.cookieAt, reissue = sess.lastSeen, true
			}
		}
		s.mu.Unlock()
		if sess == nil {
			fail(w, http.StatusUnauthorized, "nicht angemeldet")
			return
		}
		if reissue {
			setSessionCookie(w, c.Value)
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
	if !s.loginBegin(ip) {
		fail(w, http.StatusTooManyRequests, "Zu viele Fehlversuche, bitte in einigen Minuten erneut")
		return
	}
	sid := randomID()
	ms, err := s.movies.Login(r.Context(), in.User, in.Password, "movies-cast-"+sid[:12])
	s.loginDone(ip, errors.Is(err, movies.ErrUnauthorized))
	if errors.Is(err, movies.ErrUnauthorized) {
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
	s.sessions[sid] = &session{ms: ms, lastSeen: s.now(), cookieAt: s.now()}
	delete(s.loginFails, ip)
	s.saveSessionsLocked()
	s.mu.Unlock()
	setSessionCookie(w, sid)
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
	for ip := range s.loginFails {
		s.recentFailsLocked(ip) // drops an IP whose failures are all old
	}
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

// loginBegin takes one of the attempts left to ip. Attempts still waiting
// for Movies count: checked against failures alone, parallel guesses all
// passed before the first failure was recorded. A true needs a loginDone.
func (s *Server) loginBegin(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.recentFailsLocked(ip))+s.loginPending[ip] >= loginFailLimit {
		return false
	}
	s.loginPending[ip]++
	return true
}

func (s *Server) loginDone(ip string, failed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loginPending[ip]--; s.loginPending[ip] <= 0 {
		delete(s.loginPending, ip)
	}
	if failed {
		s.loginFails[ip] = append(s.recentFailsLocked(ip), s.now())
	}
}

// setSessionCookie issues the session cookie for sessionMaxAge from now.
func setSessionCookie(w http.ResponseWriter, sid string) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: sid, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: int(sessionMaxAge / time.Second)})
}

// unreachable is a connection that never reached the TV (off, standby, gone),
// as opposed to a TV that answered with an error or did not answer in time.
func unreachable(err error) bool {
	var op *net.OpError
	return errors.As(err, &op) && op.Op == "dial"
}

func (s *Server) canWake(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prefs.Known[id].MAC != ""
}

// faultCode turns a renderer error into a short suffix for the user. The
// renderer's own text stays in the log: it comes from whatever device
// answered SSDP and is not shown to the browser.
func faultCode(err error) string {
	var f *dlna.Fault
	if errors.As(err, &f) && f.Code != 0 {
		return upnpSuffix + strconv.Itoa(f.Code) + ")"
	}
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "Timeout") {
		return noAnswerSuffix
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

// me names the account and the settings chosen for it so far ("theme": ""
// means the system's mode; a setting never chosen is left out).
func (s *Server) me(w http.ResponseWriter, _ *http.Request, _ string, sess *session) {
	out := map[string]string{"name": sess.ms.UserName}
	s.mu.Lock()
	for name, byUser := range s.account {
		if v, ok := byUser[sess.ms.UserID]; ok {
			out[name] = v
		}
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, out)
}

// setAccountPref stores one setting for the logged-in account: {"<name>": value}.
func (s *Server) setAccountPref(name string) func(http.ResponseWriter, *http.Request, string, *session) {
	return func(w http.ResponseWriter, r *http.Request, _ string, sess *session) {
		var in map[string]string
		v, ok := "", false
		if readJSON(r, &in) == nil {
			v, ok = in[name]
		}
		if !ok || !accountPrefs[name].valid(v) {
			fail(w, http.StatusBadRequest, "ungültige Anfrage")
			return
		}
		s.mu.Lock()
		s.account[name][sess.ms.UserID] = v
		s.saveAccountPrefLocked(name)
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]string{name: v})
	}
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
	Off   bool   `json:"off,omitempty"` // did not answer; known, can be woken
}

func (s *Server) listRenderers(w http.ResponseWriter, r *http.Request, _ string, _ *session) {
	rs, err := s.renderers.Find(r.Context(), r.URL.Query().Get("refresh") == "1")
	if err != nil {
		log.Printf("renderers: %v", err)
	}
	s.remember(rs)
	s.mu.Lock()
	out := make([]rendererOut, 0, len(rs))
	found := map[string]bool{}
	for _, x := range rs {
		found[x.ID] = true
		if s.prefs.Hidden[x.ID] {
			continue
		}
		name := x.Name
		if l := s.prefs.Labels[x.ID]; l != "" {
			name = l
		}
		out = append(out, rendererOut{ID: x.ID, Name: name, Model: x.Model, Host: x.Host()})
	}
	// TVs seen before that did not answer now: offered when they can be woken.
	for id, k := range s.prefs.Known {
		if found[id] || s.prefs.Hidden[id] || k.MAC == "" {
			continue
		}
		name := k.Name
		if l := s.prefs.Labels[id]; l != "" {
			name = l
		}
		out = append(out, rendererOut{ID: id, Name: name, Model: k.Model, Host: k.Host, Off: true})
	}
	s.mu.Unlock()
	// Found TVs keep the finder's order; off ones come from a map, by name.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Off != out[j].Off {
			return !out[i].Off
		}
		return out[i].Off && out[i].Name < out[j].Name
	})
	if len(out) == 0 {
		s.emptyTVList(w, len(rs))
		return
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
		App      bool   `json:"app"`   // open the film in the TV's movies-cast app instead of DLNA
		// Convert sends the sound converted (convert.go), Original the file as
		// it is; Remember stores the choice for this TV and audio format.
		Convert  bool    `json:"convert"`
		Original bool    `json:"original"`
		Remember bool    `json:"remember"`
		At       float64 `json:"at"` // start here (seconds), for "No sound?" during a play
	}
	if err := readJSON(r, &in); err != nil || in.Renderer == "" || in.Movie == "" {
		fail(w, http.StatusBadRequest, "Film und Fernseher angeben")
		return
	}
	tv, ok := s.renderer(r.Context(), in.Renderer)
	if !ok {
		b := errorBody("Fernseher nicht gefunden")
		b["wake"] = s.canWake(in.Renderer)
		writeJSON(w, http.StatusNotFound, b)
		return
	}
	m, err := s.movies.Movie(r.Context(), sess.ms, in.Movie)
	if err != nil {
		s.moviesErr(w, sid, err)
		return
	}
	// DLNA is the way; the app only when the phone asks for it, after DLNA
	// failed and the answer said the app is open (appAvailable).
	if in.App {
		if s.sendToApp(tv, m, in.Resume) {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "viaApp": true, "title": m.Name})
			return
		}
		fail(w, http.StatusConflict, "Die Movies-Cast-App auf diesem Fernseher ist nicht geöffnet")
		return
	}
	if s.wantConvert(tv, m, in.Convert, in.Original, in.Remember) {
		start := time.Duration(in.At * float64(time.Second))
		if start == 0 && in.Resume {
			start = time.Duration(m.Resume) * time.Second
		}
		s.startConverted(w, r, sess, tv, m, start)
		return
	}
	appOpen := s.appOpen(tv)
	ext := m.Container
	if ext == "" {
		ext = "mkv"
	}
	key := formatKey(tv, ext, m.VideoCodec)
	s.mu.Lock()
	proven, failed := s.proven[key], s.failed[key]
	s.mu.Unlock()
	if !in.Force && failed {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":        "Dieser Fernseher hat diese Art Datei per DLNA schon einmal nicht abgespielt. Hat er die Movies-Cast-App, spiel den Film dort.",
			"args":         []string{},
			"errorEn":      "This TV failed to play this kind of file via DLNA before. If it has the Movies-Cast app, play the film there.",
			"key":          "format.failedBefore",
			"unlisted":     []string{},
			"appAvailable": appOpen,
		})
		return
	}
	if !in.Force && !proven {
		// A hint, not a block: the LG lists no HEVC and plays it (dlna.Unlisted).
		sink, err := s.tv.Sink(r.Context(), tv)
		if err != nil {
			log.Printf("protocol info %s: %v", tv.Name, err)
		} else if miss := dlna.Unlisted(sink, ext, m.VideoCodec); len(miss) > 0 {
			list := strings.Join(miss, ", ")
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":        "Dieser Fernseher nennt " + list + " nicht in seiner Formatliste, das Bild bleibt dann womöglich schwarz.",
				"errorEn":      "This TV does not list " + list + " among its formats; the screen may stay black.",
				"key":          "format.unlisted",
				"args":         []string{list},
				"unlisted":     miss,
				"appAvailable": appOpen,
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
		if unreachable(err) {
			// Off or in standby: says nothing about the format.
			b := errorBody("Der Fernseher ist aus oder nicht erreichbar")
			b["wake"] = s.canWake(tv.ID)
			writeJSON(w, http.StatusBadGateway, b)
			return
		}
		s.markFailed(key)
		b := errorBody("Fernseher hat den Film abgelehnt" + faultCode(err))
		b["appAvailable"] = appOpen
		writeJSON(w, http.StatusBadGateway, b)
		return
	}
	t0 := s.now()
	if err := s.tv.Play(ctx, tv); err != nil {
		s.abortPlay(p.id)
		log.Printf("Play %s: %v after %v", tv.Name, err, s.now().Sub(t0).Round(time.Millisecond))
		s.markFailed(key)
		b := errorBody("Fernseher startet nicht" + faultCode(err))
		b["appAvailable"] = appOpen
		writeJSON(w, http.StatusBadGateway, b)
		return
	}
	var resume time.Duration
	if in.Resume && m.Resume > 0 {
		resume = time.Duration(m.Resume) * time.Second
	}
	if in.At > 0 { // back from converted sound, at the same place
		resume = time.Duration(in.At * float64(time.Second))
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
	log.Printf("Play %s: answered after %v (%s %s)", tv.Name, s.now().Sub(t0).Round(time.Millisecond), ext, m.VideoCodec)
	go s.watch(p.ctx, p, resume)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "title": m.Name, "resumeSeconds": int64(resume / time.Second)})
}

// provenAfter is how long a TV must report PLAYING before its format counts
// as playable on it.
const provenAfter = 10 * time.Second

func (s *Server) markFailed(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.failed[key] {
		s.failed[key] = true
		s.saveProvenLocked()
	}
}

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
	if p.conv != nil {
		resume = p.offset // the stream starts there; no seek
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
		restarting := s.now().Before(p.restartUntil)
		raw := st.Position // the TV counts from the start of the stream it got
		st.Position += p.offset
		if mine {
			p.lastState = st.State
			if st.Position > p.offset {
				p.lastPos = st.Position
			}
			if st.State == "PLAYING" {
				p.sawPlay = true
				if k := formatKey(p.tv, p.ext, p.movie.VideoCodec); raw >= provenAfter && !s.proven[k] {
					s.proven[k] = true
					delete(s.failed, k)
					s.saveProvenLocked()
				}
			}
		}
		sawPlay = p.sawPlay
		s.mu.Unlock()

		switch {
		case !mine && sawPlay, // the TV plays something else now
			mine && sawPlay && !restarting && (st.State == "STOPPED" || st.State == "NO_MEDIA_PRESENT"):
			s.abortPlay(p.id)
			return
		}
		// The LG ignores Seek until it plays; seek once, right after PLAYING.
		if !seeked && p.conv != nil {
			seeked = true
		}
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
		to := time.Duration(in.Seconds * float64(time.Second))
		s.mu.Lock()
		p := s.byTV[tv.ID]
		s.mu.Unlock()
		if p != nil && p.conv != nil {
			err = s.seekConverted(ctx, p, to)
		} else {
			err = s.tv.Seek(ctx, tv, to)
		}
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
		out["converted"] = p.conv != nil
		if p.refused != "" {
			out["refusedFrom"], out["streamTo"] = p.refused, p.peer
		}
		if p.conv != nil {
			out["positionSeconds"] = int64((st.Position + p.offset) / time.Second)
			out["durationSeconds"] = int64(p.conv.Total() / time.Second)
		}
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
	other, first := false, ""
	if p != nil {
		ms, id, pctx = p.sess.ms, p.movie.ID, p.ctx
		switch ip := clientIP(r); p.peer {
		case "":
			p.peer = ip
		case ip:
		default:
			other, first = true, p.peer
			p.refused = ip
		}
	}
	s.mu.Unlock()
	if p == nil {
		http.NotFound(w, r)
		return
	}
	if other {
		log.Printf("stream %s refused: fetched by %s first", clientIP(r), first)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if p.conv != nil {
		s.serveConverted(w, r, p)
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
	t0 := time.Now()
	var n int64
	if r.Method != http.MethodHead {
		n, _ = io.Copy(w, resp.Body)
	}
	// What the TV asked for and got: the DLNA failures of 2026-10-28 left no
	// trace of whether the TV fetched the file at all. No play ID in the line.
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	log.Printf("stream %s %s range=%q -> %d, %d B in %v", host, r.Method, r.Header.Get("Range"), resp.StatusCode, n, time.Since(t0).Round(time.Millisecond))
}
