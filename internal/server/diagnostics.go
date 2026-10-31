package server

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"
)

// diagnostics is the file a tester attaches to a report (CONTRIBUTING.md):
// version, source, the TVs by name and model, and per TV model the formats
// it played (proven), refused (failed) and the sound formats converted for
// it. No addresses, no MACs, no film titles, no account names.
func (s *Server) diagnostics(w http.ResponseWriter, r *http.Request, _ string, _ *session) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	src := map[string]any{"zimaos": s.movies.ZimaOS()}
	if in, err := s.movies.PublicInfo(ctx); err == nil {
		src["product"], src["version"] = in.Product, in.Version
	} else {
		src["error"] = "not reachable"
	}
	rs, _ := s.renderers.Find(ctx, false)
	type tvOut struct {
		Name, Model, Kind string
		Played, Refused   []string
		SoundConverted    []string
	}
	byID := map[string]*tvOut{}
	var order []string
	add := func(id, name, model, kind string) *tvOut {
		if t := byID[id]; t != nil {
			return t
		}
		t := &tvOut{Name: name, Model: model, Kind: kind, Played: []string{}, Refused: []string{}, SoundConverted: []string{}}
		byID[id] = t
		order = append(order, id)
		return t
	}
	for _, x := range rs {
		kind := "dlna"
		if isCast(x) {
			kind = "cast"
		}
		add(x.ID, x.Name, x.Model, kind)
	}
	s.mu.Lock()
	for id, k := range s.prefs.Known {
		add(id, k.Name, k.Model, "dlna")
	}
	split := func(key string) (string, string) {
		id, rest, _ := strings.Cut(key, "|")
		return id, strings.ReplaceAll(rest, "|", " ")
	}
	for key := range s.proven {
		id, f := split(key)
		t := add(id, "", "", "")
		t.Played = append(t.Played, f)
	}
	for key := range s.failed {
		id, f := split(key)
		t := add(id, "", "", "")
		t.Refused = append(t.Refused, f)
	}
	for key, on := range s.convert {
		if on {
			id, f := split(key)
			t := add(id, "", "", "")
			t.SoundConverted = append(t.SoundConverted, f)
		}
	}
	plays := len(s.plays)
	s.mu.Unlock()
	tvs := make([]*tvOut, 0, len(order))
	for _, id := range order {
		t := byID[id]
		sort.Strings(t.Played)
		sort.Strings(t.Refused)
		sort.Strings(t.SoundConverted)
		tvs = append(tvs, t)
	}
	st, _ := s.searchStats()
	w.Header().Set("Content-Disposition", `attachment; filename="movies-cast-diagnostics.json"`)
	writeJSON(w, http.StatusOK, map[string]any{
		"movies-cast": s.movies.Version,
		"at":          s.now().UTC().Format(time.RFC3339),
		"source":      src,
		"search":      map[string]any{"answers": st.Answers, "renderers": st.Renderers, "port": st.Port, "portHeld": st.PortHeld},
		"tvs":         tvs,
		"activePlays": plays,
	})
}
