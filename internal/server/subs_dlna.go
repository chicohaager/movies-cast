package server

import (
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"movies-cast/internal/movies"
)

// Subtitles for DLNA TVs (roadmap 16): the phone lists a film's text
// subtitles, the chosen one goes to the TV as SRT next to the film.

// subtitles answers the text subtitles of a film. "listed": false means the
// item answer may be incomplete (see movies.Subtitles); the phone says so.
func (s *Server) subtitles(w http.ResponseWriter, r *http.Request, sid string, sess *session) {
	subs, _, listed, err := s.movies.Subtitles(r.Context(), sess.ms, r.PathValue("id"))
	if err != nil {
		s.moviesErr(w, sid, err)
		return
	}
	if subs == nil {
		subs = []movies.Subtitle{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"subtitles": subs, "listed": listed})
}

// serveSRT hands the TV the chosen subtitle track as SRT. The TV fetches it
// like the film (no cookie), so it is bound to the play and its first peer
// the same way (serveStream checked that already).
func (s *Server) serveSRT(w http.ResponseWriter, r *http.Request, p *play) {
	s.mu.Lock()
	idx, msID, movieID, ms := p.dlnaSub, p.msID, p.movie.ID, p.sess.ms
	s.mu.Unlock()
	if idx < 0 {
		http.NotFound(w, r)
		return
	}
	req, err := s.movies.SubtitleRequest(r.Context(), ms, movieID, msID, idx)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	resp, err := s.stream.Do(req)
	if err != nil {
		http.Error(w, "upstream", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("subtitle %s #%d: upstream HTTP %d", movieID, idx, resp.StatusCode)
		http.Error(w, "upstream", http.StatusBadGateway)
		return
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxSubtitleBytes))
	if err != nil {
		http.Error(w, "upstream", http.StatusBadGateway)
		return
	}
	out := vttToSRT(string(b))
	w.Header().Set("Content-Type", "text/srt; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(out)))
	if r.Method != http.MethodHead {
		_, _ = io.WriteString(w, out)
	}
	log.Printf("subtitle %s #%d to %s: %d cues", movieID, idx, clientIP(r), strings.Count(out, " --> "))
}
