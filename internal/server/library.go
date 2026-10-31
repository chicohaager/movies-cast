package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"movies-cast/internal/movies"
)

// Series, music and photos (roadmap 12, 19) and the queue (15).
//
// Movies 0.3.0 lists films only (measured 2026-10-31 on the owner's box: one
// view "movies", 0 Series/Episode/Audio/Photo items); a Jellyfin server as the
// source (roadmap 20) has all of them. So the phone shows a tab only for what
// the server has (/api/views).

// children answers the episodes of a series or the tracks of an album.
func (s *Server) children(w http.ResponseWriter, r *http.Request, sid string, sess *session) {
	parent, err := s.movies.Movie(r.Context(), sess.ms, r.PathValue("id"))
	if err != nil {
		s.moviesErr(w, sid, err)
		return
	}
	list, err := s.movies.Children(r.Context(), sess.ms, parent)
	if err != nil {
		s.moviesErr(w, sid, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// libraryKinds maps the phone's tab to Jellyfin item types.
var libraryKinds = map[string]string{"music": "MusicAlbum", "photos": "Photo"}

func (s *Server) library(w http.ResponseWriter, r *http.Request, sid string, sess *session) {
	types, ok := libraryKinds[r.URL.Query().Get("kind")]
	if !ok {
		fail(w, http.StatusBadRequest, "ungültige Anfrage")
		return
	}
	list, err := s.movies.Library(r.Context(), sess.ms, types)
	if err != nil {
		s.moviesErr(w, sid, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// views answers which tabs the phone shows: {"series":bool,"music":bool,"photos":bool}.
func (s *Server) views(w http.ResponseWriter, r *http.Request, sid string, sess *session) {
	vs, err := s.movies.Views(r.Context(), sess.ms)
	if err != nil {
		s.moviesErr(w, sid, err)
		return
	}
	out := map[string]bool{"series": false, "music": false, "photos": false}
	for _, v := range vs {
		switch v {
		case "tvshows":
			out["series"] = true
		case "music":
			out["music"] = true
		case "homevideos", "photos":
			out["photos"] = true
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// queued is one entry of a TV's queue.
type queued struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// tvQueue is what plays next on one TV: the entries the phone added, in
// order; after them, for an episode or a track, the next one of its series
// or album, unless the phone said "no autoplay" for this play.
type tvQueue struct {
	items  []queued
	noAuto string // play ID for which the automatic next item is off
}

const maxQueue = 100

func (s *Server) queueGet(w http.ResponseWriter, r *http.Request, _ string, sess *session) {
	id := r.URL.Query().Get("renderer")
	s.mu.Lock()
	q := s.queue[id]
	items := []queued{}
	auto := true
	if q != nil {
		items = append(items, q.items...)
		if p := s.byTV[id]; p != nil && q.noAuto == p.id {
			auto = false
		}
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "autoplay": auto})
}

func (s *Server) queueSet(w http.ResponseWriter, r *http.Request, sid string, sess *session) {
	var in struct {
		Renderer string `json:"renderer"`
		Movie    string `json:"movie"`
		Action   string `json:"action"` // add, remove, clear, noauto, auto
	}
	if err := readJSON(r, &in); err != nil || in.Renderer == "" {
		fail(w, http.StatusBadRequest, "ungültige Anfrage")
		return
	}
	var name string
	if in.Action == "add" {
		m, err := s.movies.Movie(r.Context(), sess.ms, in.Movie)
		if err != nil {
			s.moviesErr(w, sid, err)
			return
		}
		if !m.Playable() {
			fail(w, http.StatusBadRequest, "Eine Folge oder einen Titel wählen")
			return
		}
		name = m.Name
		if m.SeriesName != "" && m.Episode > 0 {
			name = m.SeriesName + " · " + episodeLabel(m) + " · " + m.Name
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	q := s.queue[in.Renderer]
	if q == nil {
		q = &tvQueue{}
		s.queue[in.Renderer] = q
	}
	switch in.Action {
	case "add":
		if len(q.items) >= maxQueue {
			fail(w, http.StatusBadRequest, "höchstens 100 Einträge in der Warteschlange")
			return
		}
		q.items = append(q.items, queued{ID: in.Movie, Name: name})
	case "remove":
		for i, it := range q.items {
			if it.ID == in.Movie {
				q.items = append(q.items[:i], q.items[i+1:]...)
				break
			}
		}
	case "clear":
		q.items = nil
	case "noauto", "auto":
		q.noAuto = ""
		if p := s.byTV[in.Renderer]; p != nil && in.Action == "noauto" {
			q.noAuto = p.id
		}
	default:
		fail(w, http.StatusBadRequest, "unbekannte Aktion")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": append([]queued{}, q.items...)})
}

func episodeLabel(m movies.Movie) string {
	return fmt.Sprintf("S%02dE%02d", m.Season, m.Episode)
}

func jsonString(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// finished says whether a play that stopped ran to its end: a TV reports its
// last position up to a poll before the end, so the last 45 s (or 5 %) count.
func finished(m movies.Movie, last time.Duration) bool {
	if m.Runtime <= 0 || last <= 0 {
		return false
	}
	rt := time.Duration(m.Runtime) * time.Second
	return rt-last <= 45*time.Second || last >= rt*95/100
}

// nextFor picks what follows p on its TV and takes it off the queue.
func (s *Server) nextFor(ctx context.Context, p *play) (string, bool) {
	s.mu.Lock()
	q := s.queue[p.tv.ID]
	if q != nil && len(q.items) > 0 {
		next := q.items[0]
		q.items = q.items[1:]
		s.mu.Unlock()
		return next.ID, true
	}
	auto := q == nil || q.noAuto != p.id
	s.mu.Unlock()
	if !auto {
		return "", false
	}
	n, ok, err := s.movies.Next(ctx, p.sess.ms, p.movie)
	if err != nil {
		log.Printf("next after %s: %v", p.movie.Name, err)
		return "", false
	}
	return n.ID, ok
}

// playNext sends the next item to the TV after p ran to its end. It goes
// through startPlay like a tap on the phone, so format hints, remembered
// conversions and Wake-on-LAN rules stay the same; a refusal is logged (the
// phone shows the TV as idle).
func (s *Server) playNext(p *play) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	id, ok := s.nextFor(ctx, p)
	if !ok {
		return
	}
	body := `{"renderer":` + jsonString(p.tv.ID) + `,"movie":` + jsonString(id) + `,"force":true}`
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/play", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.startPlay(rec, req, "", p.sess)
	log.Printf("autoplay %s after %s on %s: HTTP %d", id, p.movie.Name, p.tv.Name, rec.Code)
}

// source tells the login page whose account it asks for: the ZimaOS account
// for Movies, else the account on the Jellyfin or Emby server (roadmap 20).
// No login needed; it says only what /System/Info/Public says anyway.
func (s *Server) source(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"zimaos": s.movies.ZimaOS()}
	if !s.movies.ZimaOS() {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if in, err := s.movies.PublicInfo(ctx); err == nil {
			out["product"] = in.Product
		}
	}
	writeJSON(w, http.StatusOK, out)
}
