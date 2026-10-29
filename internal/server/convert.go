package server

// Converted plays (roadmap 6): for a TV that has no sound with a film's
// audio format, Movies converts the audio to AAC and copies the picture; its
// HLS segments are joined here into one MPEG-TS stream for the TV. Chosen per
// TV and audio format on the phone ("No sound?"), remembered in convert.json.
// The owner reported sound on the LG with TrueHD and E-AC3 (2026-10-28), so
// nothing is converted unless asked for.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"movies-cast/internal/dlna"
	"movies-cast/internal/movies"
)

func convertKey(tv dlna.Renderer, audio string) string {
	return tv.ID + "|audio|" + strings.ToLower(audio)
}

// wantConvert decides and, with remember, stores the choice.
func (s *Server) wantConvert(tv dlna.Renderer, m movies.Movie, convert, original, remember bool) bool {
	key := convertKey(tv, m.Audio)
	s.mu.Lock()
	defer s.mu.Unlock()
	want := !original && (convert || s.convert[key])
	if remember && s.convert[key] != want {
		if want {
			s.convert[key] = true
		} else {
			delete(s.convert, key)
		}
		s.saveProvenLocked()
	}
	return want
}

// tsMime picks the MIME type the TV lists for MPEG-TS: the LG lists
// video/mp2t, the Samsung UE50MU6179 video/mpeg and video/vnd.dlna.mpeg-tts
// (measured 2026-10-28).
func tsMime(sink string) string {
	for _, m := range []string{"video/mp2t", "video/mpeg"} {
		if strings.Contains(sink, ":"+m+":") {
			return m
		}
	}
	return "video/mp2t"
}

func (s *Server) startConverted(w http.ResponseWriter, r *http.Request, sess *session, tv dlna.Renderer, m movies.Movie, start time.Duration) {
	ctx := r.Context()
	cv, err := s.movies.Convert(ctx, sess.ms, m.ID)
	if err != nil {
		log.Printf("convert %s: %v", m.ID, err)
		fail(w, http.StatusBadGateway, "Movies konnte den Film nicht umwandeln")
		return
	}
	sink, err := s.tv.Sink(ctx, tv)
	if err != nil {
		log.Printf("protocol info %s: %v", tv.Name, err)
	}
	base, err := s.streamBase(ctx, tv)
	if err != nil {
		fail(w, http.StatusBadGateway, "keine Netzverbindung zum Fernseher")
		return
	}
	idx, off := cv.At(start)
	p := &play{id: randomID(), sess: sess, movie: m, tv: tv, ext: "ts", psID: cv.PlaySessionID, created: s.now(),
		conv: &cv, segIdx: idx, offset: off, mime: tsMime(sink)}
	p.ctx, p.cancel = context.WithCancel(context.Background())
	s.mu.Lock()
	if old := s.byTV[tv.ID]; old != nil {
		s.endPlayLocked(old.id)
	}
	s.plays[p.id] = p
	s.byTV[tv.ID] = p
	s.mu.Unlock()
	if err := s.sendConverted(ctx, p, base); err != nil {
		s.abortPlay(p.id)
		log.Printf("converted %s: %v", tv.Name, err)
		writeJSON(w, http.StatusBadGateway, errorBody("Fernseher startet nicht"+faultCode(err)))
		return
	}
	log.Printf("Play %s: converted audio %s -> aac, %d segments, from %v", tv.Name, m.Audio, len(cv.Segments), off.Round(time.Second))
	go s.watch(p.ctx, p, 0)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "title": m.Name, "converted": true, "resumeSeconds": int64(off / time.Second)})
}

// sendConverted hands the TV the stream from p.segIdx. Every send gets its
// own name, so the TV does not take a seek for the stream it already has.
func (s *Server) sendConverted(ctx context.Context, p *play, base string) error {
	s.mu.Lock()
	off := p.offset
	total := p.conv.Total()
	name := "movie-" + strconv.FormatInt(int64(off/time.Second), 10) + ".ts"
	mime := p.mime
	s.mu.Unlock()
	media := dlna.Media{URL: base + "/s/" + p.id + "/" + url.PathEscape(name), Title: p.movie.Name, MimeType: mime,
		Duration: total - off, Converted: true}
	if err := s.tv.SetURI(ctx, p.tv, media); err != nil {
		return err
	}
	return s.tv.Play(ctx, p.tv)
}

// seekConverted re-sends the film from the segment that holds to.
func (s *Server) seekConverted(ctx context.Context, p *play, to time.Duration) error {
	base, err := s.streamBase(ctx, p.tv)
	if err != nil {
		return err
	}
	s.mu.Lock()
	p.segIdx, p.offset = p.conv.At(to)
	p.restartUntil = s.now().Add(20 * time.Second)
	s.mu.Unlock()
	return s.sendConverted(ctx, p, base)
}

// parseNPT reads "npt=12.5-" or "npt=0:01:02.5-" from TimeSeekRange.dlna.org.
func parseNPT(h string) (time.Duration, bool) {
	v, ok := strings.CutPrefix(strings.TrimSpace(h), "npt=")
	if !ok {
		return 0, false
	}
	v, _, _ = strings.Cut(v, "-")
	var secs float64
	for _, part := range strings.Split(v, ":") {
		f, err := strconv.ParseFloat(part, 64)
		if err != nil || f < 0 {
			return 0, false
		}
		secs = secs*60 + f
	}
	return time.Duration(secs * float64(time.Second)), true
}

// serveConverted joins the segments from the play's start (or from the time
// the TV asks for) into one response.
func (s *Server) serveConverted(w http.ResponseWriter, r *http.Request, p *play) {
	s.mu.Lock()
	idx, off, cv, mime, pctx := p.segIdx, p.offset, p.conv, p.mime, p.ctx
	s.mu.Unlock()
	if t, ok := parseNPT(r.Header.Get("TimeSeekRange.dlna.org")); ok {
		var start time.Duration
		idx, start = cv.At(off + t)
		w.Header().Set("TimeSeekRange.dlna.org", fmt.Sprintf("npt=%.3f-", (start-off).Seconds()))
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("transferMode.dlna.org", "Streaming")
	w.Header().Set("contentFeatures.dlna.org", dlna.Media{Converted: true}.Flags())
	w.WriteHeader(http.StatusOK)
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if r.Method == http.MethodHead {
		log.Printf("stream %s HEAD converted -> 200", host)
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	defer context.AfterFunc(pctx, cancel)()
	t0 := time.Now()
	var n int64
	first := idx
	for ; idx < len(cv.Segments) && ctx.Err() == nil; idx++ {
		k, err := s.copySegment(ctx, w, p, cv.Segments[idx])
		n += k
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("convert segment %d: %v", idx, err)
			}
			break
		}
	}
	log.Printf("stream %s GET converted segments %d..%d -> %d B in %v", host, first, idx, n, time.Since(t0).Round(time.Millisecond))
}

func (s *Server) copySegment(ctx context.Context, w io.Writer, p *play, seg movies.Segment) (int64, error) {
	req, err := s.movies.SegmentRequest(ctx, p.sess.ms, seg)
	if err != nil {
		return 0, err
	}
	for hops := 0; ; hops++ {
		resp, err := s.stream.Do(req)
		if err != nil {
			// The URL carries the token: only the cause goes into the log.
			var ue *url.Error
			if errors.As(err, &ue) {
				err = ue.Err
			}
			return 0, err
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			defer resp.Body.Close()
			return io.Copy(w, resp.Body)
		case resp.StatusCode/100 == 3 && resp.Header.Get("Location") != "" && hops < 3:
			loc := resp.Header.Get("Location")
			resp.Body.Close()
			if req, err = s.movies.FollowSegment(ctx, p.sess.ms, req.URL, loc); err != nil {
				return 0, err
			}
		default:
			resp.Body.Close()
			return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
	}
}
