package movies

// Converted streams for a TV that cannot play a film's audio. Measured on
// Movies 0.3.0 (2026-10-28): /Videos/{id}/stream?static=false&AudioCodec=aac
// returns the original file unchanged; PlaybackInfo with a device profile
// that cannot play the audio offers only HLS — master.m3u8, 5 s MPEG-TS
// segments, video copied (HEVC 3840x2160 stayed HEVC), audio AAC, the first
// segment 0.1 s after the request. The segments of one encoding join into
// one MPEG-TS stream, which is what the server hands the TV.
//
// The URLs Movies returns carry the token as api_key: they are used, never
// logged or shown.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Segment is one piece of a converted stream.
type Segment struct {
	URL      string
	Duration time.Duration
}

// Conversion is a running conversion of one film.
type Conversion struct {
	PlaySessionID string
	Segments      []Segment
}

// Total is the length of all segments.
func (cv Conversion) Total() time.Duration {
	var d time.Duration
	for _, s := range cv.Segments {
		d += s.Duration
	}
	return d
}

// At returns the index of the segment that holds t and where it starts.
func (cv Conversion) At(t time.Duration) (int, time.Duration) {
	var start time.Duration
	for i, s := range cv.Segments {
		if t < start+s.Duration || i == len(cv.Segments)-1 {
			return i, start
		}
		start += s.Duration
	}
	return 0, 0
}

// convertProfile can play the film's container and picture but only AAC/MP3
// audio, so Movies copies the video and converts the audio.
func convertProfile() map[string]any {
	return map[string]any{
		"MaxStreamingBitrate": 200_000_000,
		"DirectPlayProfiles":  []map[string]any{{"Container": "ts", "Type": "Video", "VideoCodec": "hevc,h264", "AudioCodec": "aac,mp3"}},
		"TranscodingProfiles": []map[string]any{{
			"Container": "ts", "Type": "Video", "VideoCodec": "hevc,h264", "AudioCodec": "aac",
			"Protocol": "hls", "Context": "Streaming", "BreakOnNonKeyFrames": true,
		}},
		"CodecProfiles":    []any{},
		"SubtitleProfiles": []any{},
	}
}

// Convert asks Movies for a converted stream of item id and reads its
// segment list.
func (c *Client) Convert(ctx context.Context, s *Session, id string) (Conversion, error) {
	var pi struct {
		PlaySessionId string
		MediaSources  []struct{ TranscodingUrl string }
	}
	body := map[string]any{
		"DeviceProfile": convertProfile(), "EnableDirectPlay": false, "EnableDirectStream": false,
		"EnableTranscoding": true, "AllowVideoStreamCopy": true, "AllowAudioStreamCopy": false,
	}
	path := "/Items/" + url.PathEscape(id) + "/PlaybackInfo?UserId=" + url.QueryEscape(s.UserID)
	if err := c.do(ctx, s, http.MethodPost, path, body, &pi); err != nil {
		return Conversion{}, err
	}
	if len(pi.MediaSources) == 0 || pi.MediaSources[0].TranscodingUrl == "" {
		return Conversion{}, errors.New("movies: no conversion offered")
	}
	master, err := c.absolute(pi.MediaSources[0].TranscodingUrl)
	if err != nil {
		return Conversion{}, err
	}
	media, err := c.firstVariant(ctx, s, master)
	if err != nil {
		return Conversion{}, err
	}
	segs, err := c.segments(ctx, s, media)
	if err != nil {
		return Conversion{}, err
	}
	return Conversion{PlaySessionID: pi.PlaySessionId, Segments: segs}, nil
}

// absolute resolves a URL Movies gave (it starts with the gateway path,
// "/movies/Videos/…") against the server of c.Base. It must stay on that
// server: the token rides along.
func (c *Client) absolute(ref string) (string, error) {
	base, err := url.Parse(c.Base)
	if err != nil {
		return "", err
	}
	u, err := base.Parse(ref)
	if err != nil {
		return "", err
	}
	if u.Scheme != base.Scheme || u.Host != base.Host {
		return "", errors.New("movies: conversion URL on another server")
	}
	return u.String(), nil
}

func (c *Client) getText(ctx context.Context, s *Session, u string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Emby-Authorization", c.authHeader(s.DeviceID, s.Token))
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", errors.New("movies: playlist: " + scrub(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("movies: playlist: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	return string(b), err
}

// scrub drops the URL (with its api_key) from a transport error.
func scrub(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Op + ": " + ue.Err.Error()
	}
	return err.Error()
}

func (c *Client) firstVariant(ctx context.Context, s *Session, master string) (string, error) {
	txt, err := c.getText(ctx, s, master)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(txt, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return c.resolve(master, line)
		}
	}
	return "", errors.New("movies: empty master playlist")
}

func (c *Client) resolve(from, ref string) (string, error) {
	f, err := url.Parse(from)
	if err != nil {
		return "", err
	}
	u, err := f.Parse(ref)
	if err != nil {
		return "", err
	}
	return c.absolute(u.String())
}

func (c *Client) segments(ctx context.Context, s *Session, media string) ([]Segment, error) {
	txt, err := c.getText(ctx, s, media)
	if err != nil {
		return nil, err
	}
	return parseMediaPlaylist(txt, func(ref string) (string, error) { return c.resolve(media, ref) })
}

// parseMediaPlaylist reads #EXTINF durations and segment URIs.
func parseMediaPlaylist(txt string, resolve func(string) (string, error)) ([]Segment, error) {
	var out []Segment
	var dur time.Duration
	sc := bufio.NewScanner(strings.NewReader(txt))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "#EXTINF:"):
			v, _, _ := strings.Cut(strings.TrimPrefix(line, "#EXTINF:"), ",")
			f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil || f <= 0 {
				return nil, fmt.Errorf("movies: bad #EXTINF %q", line)
			}
			dur = time.Duration(f * float64(time.Second))
		case line == "" || strings.HasPrefix(line, "#"):
		default:
			if dur == 0 {
				return nil, errors.New("movies: segment without #EXTINF")
			}
			u, err := resolve(line)
			if err != nil {
				return nil, err
			}
			out = append(out, Segment{URL: u, Duration: dur})
			dur = 0
		}
	}
	if len(out) == 0 {
		return nil, errors.New("movies: no segments")
	}
	return out, sc.Err()
}

// SegmentRequest builds the request for one segment.
func (c *Client) SegmentRequest(ctx context.Context, s *Session, seg Segment) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, seg.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Emby-Authorization", c.authHeader(s.DeviceID, s.Token))
	return req, nil
}

// FollowSegment builds the request for a redirect Movies answered a segment
// with: from segment 121 of Avatar on, every segment came as 307 to the same
// path with more parameters (gen, runtimeTicks; measured 2026-10-28). Only a
// target on the Movies server is followed — the token rides along.
func (c *Client) FollowSegment(ctx context.Context, s *Session, from *url.URL, location string) (*http.Request, error) {
	u, err := from.Parse(location)
	if err != nil {
		return nil, err
	}
	abs, err := c.absolute(u.String())
	if err != nil {
		return nil, err
	}
	return c.SegmentRequest(ctx, s, Segment{URL: abs})
}
