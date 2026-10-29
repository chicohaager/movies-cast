package server

// TV management in the app: a name per TV, hiding a TV (Sonos speakers,
// a TV nobody uses), and TVs added by address. Before 0.17.0 all of this
// meant editing RENDERERS in the installed compose file by hand (done
// three times on 2026-10-28). Kept in tvs.json next to STATE_FILE.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"movies-cast/internal/dlna"
)

const maxLabel = 40

type tvPrefs struct {
	Labels map[string]string `json:"labels"` // renderer ID → name shown
	Hidden map[string]bool   `json:"hidden"`
	Hosts  []string          `json:"hosts"` // added by address in the app
	// Known holds every TV seen while it was on, with the MAC address the
	// ARP table had for it: an off TV answers no search, so this is how the
	// list can still offer it and wake it (Wake-on-LAN).
	Known map[string]knownTV `json:"known,omitempty"`
}

type knownTV struct {
	Name     string `json:"name"`
	Model    string `json:"model,omitempty"`
	Host     string `json:"host"`
	MAC      string `json:"mac,omitempty"`
	Location string `json:"location,omitempty"` // description URL, asked when a search misses the TV
}

// finderInfo is what the real dlna.Finder offers beyond Find.
type finderInfo interface {
	LastSearch() dlna.SearchStats
	SetExtraHosts([]string)
	SetKnown([]string)
}

// knownLocationsLocked lists the description URLs of the known TVs. Caller holds s.mu.
func (s *Server) knownLocationsLocked() []string {
	var out []string
	for _, k := range s.prefs.Known {
		if k.Location != "" {
			out = append(out, k.Location)
		}
	}
	return out
}

func (s *Server) tvsFile() string {
	if s.cfg.StateFile == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(s.cfg.StateFile), "tvs.json")
}

func (s *Server) loadTVPrefs() error {
	f := s.tvsFile()
	if f == "" {
		return nil
	}
	b, err := os.ReadFile(f)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var p tvPrefs
	if err := json.Unmarshal(b, &p); err != nil {
		return fmt.Errorf("%s: %w", f, err)
	}
	s.mu.Lock()
	if p.Labels != nil {
		s.prefs.Labels = p.Labels
	}
	if p.Hidden != nil {
		s.prefs.Hidden = p.Hidden
	}
	s.prefs.Hosts = p.Hosts
	if p.Known != nil {
		s.prefs.Known = p.Known
	}
	s.mu.Unlock()
	if fi, ok := s.renderers.(finderInfo); ok {
		fi.SetExtraHosts(p.Hosts)
		s.mu.Lock()
		fi.SetKnown(s.knownLocationsLocked())
		s.mu.Unlock()
	}
	return nil
}

// saveTVPrefsLocked writes tvs.json atomically (0600). Caller holds s.mu.
func (s *Server) saveTVPrefsLocked() error {
	f := s.tvsFile()
	if f == "" {
		return nil
	}
	b, err := json.Marshal(s.prefs)
	if err != nil {
		return err
	}
	return writeFileAtomic(f, b)
}

type tvOut struct {
	ID     string `json:"id"`
	Name   string `json:"name"`  // as the TV calls itself
	Label  string `json:"label"` // set in the app; empty = Name
	Host   string `json:"host"`
	Hidden bool   `json:"hidden"`
}

func (s *Server) searchStats() (dlna.SearchStats, bool) {
	fi, ok := s.renderers.(finderInfo)
	if !ok {
		return dlna.SearchStats{}, false
	}
	return fi.LastSearch(), true
}

// emptyTVList explains why no TV can be offered, from what the search saw.
func (s *Server) emptyTVList(w http.ResponseWriter, found int) {
	de, en, key, args := "Kein Fernseher gefunden", "No TV found", "tv.none", []string{}
	st, ok := s.searchStats()
	switch {
	case found > 0:
		de = "Alle gefundenen Fernseher sind ausgeblendet (Fernseher verwalten)"
		en = "Every TV found is hidden (Manage TVs)"
		key = "tvs.allHidden"
	case ok && st.Answers == 0 && st.Port != 0:
		port := strconv.Itoa(st.Port)
		de = "Keine Antwort auf die Fernseher-Suche. Lässt die Firewall UDP " + port + " aus dem LAN durch? Sonst Fernseher per IP hinzufügen."
		en = "No answer to the TV search. Does the firewall let UDP " + port + " in from the LAN? Otherwise add the TV by IP."
		key, args = "tvs.noAnswer", []string{port}
	case ok && st.Answers == 0:
		de = "Keine Antwort auf die Fernseher-Suche. Lässt die Firewall die Suche durch? Sonst Fernseher per IP hinzufügen."
		en = "No answer to the TV search. Does the firewall let the search through? Otherwise add the TV by IP."
		key = "tvs.noAnswerRandom"
	case ok:
		n := strconv.Itoa(st.Answers)
		de = n + " Geräte haben auf die Suche geantwortet, aber keins spielt Video"
		en = n + " devices answered the search, none of them plays video"
		key, args = "tvs.noVideo", []string{n}
	}
	writeJSON(w, http.StatusBadGateway, map[string]any{"error": de, "errorEn": en, "key": key, "args": args})
}

// Bounds for what a device on the LAN or a user can make the server keep.
const (
	maxKnownTVs = 64 // TVs remembered in tvs.json
	maxTVHosts  = 32 // added addresses, each asked on every search
)

// remember records the TVs found (name, address, MAC) in tvs.json, so a TV
// that is off later can still be listed and woken. A TV whose MAC the ARP
// table lacks this time keeps the one it had.
func (s *Server) remember(rs []dlna.Renderer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for _, x := range rs {
		k := knownTV{Name: x.Name, Model: x.Model, Host: x.Host(), MAC: s.macOf(x.Host()), Location: x.Location}
		old, had := s.prefs.Known[x.ID]
		if k.MAC == "" {
			k.MAC = old.MAC
		}
		if had && old == k {
			continue
		}
		if s.prefs.Known == nil {
			s.prefs.Known = map[string]knownTV{}
		}
		// Full: a new device is not kept (the ones known stay, so a device
		// answering with ever new IDs cannot push a real TV out).
		if !had && len(s.prefs.Known) >= maxKnownTVs {
			continue
		}
		s.prefs.Known[x.ID] = k
		changed = true
	}
	if changed {
		if err := s.saveTVPrefsLocked(); err != nil {
			log.Printf("tvs: %v", err)
		}
		if fi, ok := s.renderers.(finderInfo); ok {
			fi.SetKnown(s.knownLocationsLocked())
		}
	}
}

func (s *Server) listTVs(w http.ResponseWriter, r *http.Request, _ string, _ *session) {
	rs, err := s.renderers.Find(r.Context(), r.URL.Query().Get("refresh") == "1")
	if err != nil {
		log.Printf("renderers: %v", err)
	}
	s.remember(rs)
	s.mu.Lock()
	out := make([]tvOut, 0, len(rs))
	for _, x := range rs {
		out = append(out, tvOut{ID: x.ID, Name: x.Name, Label: s.prefs.Labels[x.ID], Host: x.Host(), Hidden: s.prefs.Hidden[x.ID]})
	}
	hosts := append([]string{}, s.prefs.Hosts...)
	s.mu.Unlock()
	st, _ := s.searchStats()
	writeJSON(w, http.StatusOK, map[string]any{"tvs": out, "hosts": hosts, "search": st})
}

func (s *Server) setTV(w http.ResponseWriter, r *http.Request, _ string, _ *session) {
	var in struct {
		ID     string `json:"id"`
		Label  string `json:"label"`
		Hidden bool   `json:"hidden"`
	}
	if err := readJSON(r, &in); err != nil || in.ID == "" {
		fail(w, http.StatusBadRequest, "ungültige Anfrage")
		return
	}
	label := strings.TrimSpace(in.Label)
	if utf8.RuneCountInString(label) > maxLabel {
		fail(w, http.StatusBadRequest, "Name zu lang (höchstens 40 Zeichen)")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if label == "" {
		delete(s.prefs.Labels, in.ID)
	} else {
		s.prefs.Labels[in.ID] = label
	}
	if in.Hidden {
		s.prefs.Hidden[in.ID] = true
	} else {
		delete(s.prefs.Hidden, in.ID)
	}
	if err := s.saveTVPrefsLocked(); err != nil {
		log.Printf("tvs: %v", err)
		fail(w, http.StatusInternalServerError, "Speichern fehlgeschlagen")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// validTVHost accepts an IPv4 address on a private LAN that is not this
// machine — the same rule SSDP answers must pass (dlna.LANLocation).
func validTVHost(h string, own []net.IP) bool {
	ip := net.ParseIP(h)
	if ip == nil || ip.To4() == nil || !ip.IsPrivate() || ip.IsLoopback() {
		return false
	}
	return !slices.ContainsFunc(own, ip.Equal)
}

func (s *Server) setTVHost(w http.ResponseWriter, r *http.Request, _ string, _ *session) {
	var in struct {
		Host   string `json:"host"`
		Remove bool   `json:"remove"`
	}
	if err := readJSON(r, &in); err != nil {
		fail(w, http.StatusBadRequest, "ungültige Anfrage")
		return
	}
	h := strings.TrimSpace(in.Host)
	if !in.Remove && !validTVHost(h, dlna.OwnIPs()) {
		fail(w, http.StatusBadRequest, "Adresse ungültig (IPv4-Adresse im eigenen Netz)")
		return
	}
	s.mu.Lock()
	hosts := slices.DeleteFunc(append([]string{}, s.prefs.Hosts...), func(x string) bool { return x == h })
	if !in.Remove && len(hosts) >= maxTVHosts {
		s.mu.Unlock()
		fail(w, http.StatusBadRequest, fmt.Sprintf("höchstens %d Adressen", maxTVHosts))
		return
	}
	if !in.Remove {
		hosts = append(hosts, h)
	}
	s.prefs.Hosts = hosts
	err := s.saveTVPrefsLocked()
	s.mu.Unlock()
	if err != nil {
		log.Printf("tvs: %v", err)
		fail(w, http.StatusInternalServerError, "Speichern fehlgeschlagen")
		return
	}
	if fi, ok := s.renderers.(finderInfo); ok {
		fi.SetExtraHosts(hosts)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "hosts": hosts})
}

// system is the status page: every line is something that was checked by
// hand over ssh on 2026-10-28 before it was here.
func (s *Server) system(w http.ResponseWriter, r *http.Request, _ string, _ *session) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	moviesErr := ""
	if err := s.movies.Ping(ctx); err != nil {
		moviesErr = err.Error()
	}
	st, _ := s.searchStats()
	s.mu.Lock()
	out := map[string]any{
		"version":       s.movies.Version,
		"moviesOK":      moviesErr == "",
		"moviesError":   moviesErr,
		"search":        st,
		"hiddenTVs":     len(s.prefs.Hidden),
		"addedHosts":    len(s.prefs.Hosts),
		"provenFormats": len(s.proven),
		"activePlays":   len(s.plays),
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, out)
}

// wakeTV sends a Wake-on-LAN packet to a known TV and searches until it
// answers again (its description URL, not just the cached list: a TV that
// went off stays in the Finder's cache for its TTL). The packet is sent
// again every round; a TV on Wi-Fi can miss one.
func (s *Server) wakeTV(w http.ResponseWriter, r *http.Request, _ string, _ *session) {
	var in struct {
		ID string `json:"id"`
	}
	if err := readJSON(r, &in); err != nil || in.ID == "" {
		fail(w, http.StatusBadRequest, "ungültige Anfrage")
		return
	}
	s.mu.Lock()
	k, ok := s.prefs.Known[in.ID]
	s.mu.Unlock()
	if !ok || k.MAC == "" {
		fail(w, http.StatusNotFound, "Für diesen Fernseher ist keine MAC-Adresse bekannt")
		return
	}
	ctx := r.Context()
	t0 := s.now()
	for round := 0; ; round++ {
		if err := s.wake(ctx, k.MAC, k.Host); err != nil {
			log.Printf("wake %s: %v", k.Name, err)
			fail(w, http.StatusBadGateway, "Weckpaket konnte nicht gesendet werden")
			return
		}
		if round == 0 {
			log.Printf("wake %s (%s): magic packet sent", k.Name, k.Host)
		}
		rs, _ := s.renderers.Find(ctx, true)
		for _, x := range rs {
			if x.ID == in.ID && s.alive(ctx, x) {
				d := s.now().Sub(t0).Round(time.Second)
				log.Printf("wake %s: answers after %v", k.Name, d)
				s.remember(rs)
				writeJSON(w, http.StatusOK, map[string]any{"ok": true, "seconds": int(d / time.Second)})
				return
			}
		}
		if s.now().Sub(t0) >= s.wakeWait {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.wakeEvery):
		}
	}
	secs := strconv.Itoa(int(s.wakeWait / time.Second))
	log.Printf("wake %s: no answer within %ss", k.Name, secs)
	writeJSON(w, http.StatusGatewayTimeout, map[string]any{
		"error":   "Der Fernseher antwortet " + secs + " s nach dem Weckpaket nicht. Ist am Fernseher das Einschalten über das Netzwerk an (LG: „Mobile TV On“)?",
		"errorEn": "The TV did not answer within " + secs + " s of the wake-up packet. Is switching on via the network enabled on the TV (LG: \"Mobile TV On\")?",
		"key":     "wake.noAnswer",
		"args":    []string{secs},
	})
}

// descAlive asks the TV's description URL: a TV in standby does not answer it.
func descAlive(c *http.Client) func(context.Context, dlna.Renderer) bool {
	return func(ctx context.Context, x dlna.Renderer) bool {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, x.Location, nil)
		if err != nil {
			return false
		}
		resp, err := c.Do(req)
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}
}
