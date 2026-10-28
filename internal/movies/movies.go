// Package movies talks to IceWhale Movies (sysext zimaos-movies) through its
// Jellyfin-compatible API, which the ZimaOS gateway serves under /movies.
// Measured against Movies 0.3.0 on 2026-10-28: routes are case-sensitive
// (/movies/system/info/public is 404), auth is the ZimaOS account.
package movies

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrUnauthorized means the token is missing, expired or logged out.
var ErrUnauthorized = errors.New("movies: unauthorized")

const ticksPerSecond = 10_000_000

type Client struct {
	Base    string // e.g. http://127.0.0.1/movies
	HTTP    *http.Client
	Version string
}

func New(base, version string) *Client {
	return &Client{Base: strings.TrimRight(base, "/"), HTTP: &http.Client{
		Timeout: 20 * time.Second,
		// The token rides in X-Emby-Authorization, which Go does not strip on a
		// cross-host redirect (only Authorization/Cookie); so follow none.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, Version: version}
}

// Session is one login at Movies.
type Session struct {
	Token    string
	UserID   string
	UserName string
	DeviceID string
}

func (c *Client) authHeader(deviceID, token string) string {
	h := fmt.Sprintf(`MediaBrowser Client="movies-cast", Device="movies-cast", DeviceId="%s", Version="%s"`, deviceID, c.Version)
	if token != "" {
		h += fmt.Sprintf(`, Token="%s"`, token)
	}
	return h
}

func (c *Client) do(ctx context.Context, s *Session, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	dev, tok := "", ""
	if s != nil {
		dev, tok = s.DeviceID, s.Token
	}
	req.Header.Set("X-Emby-Authorization", c.authHeader(dev, tok))
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode/100 != 2 {
		// The body is not echoed: on a login it could quote the request.
		return fmt.Errorf("movies %s %s: HTTP %d", method, path, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(out)
}

// Login authenticates with a ZimaOS account.
func (c *Client) Login(ctx context.Context, user, password, deviceID string) (*Session, error) {
	var res struct {
		AccessToken string
		User        struct{ Id, Name string }
	}
	s := &Session{DeviceID: deviceID}
	if err := c.do(ctx, s, http.MethodPost, "/Users/AuthenticateByName", map[string]string{"Username": user, "Pw": password}, &res); err != nil {
		return nil, err
	}
	if res.AccessToken == "" || res.User.Id == "" {
		return nil, errors.New("movies: login answer without token")
	}
	s.Token, s.UserID, s.UserName = res.AccessToken, res.User.Id, res.User.Name
	return s, nil
}

func (c *Client) Logout(ctx context.Context, s *Session) error {
	return c.do(ctx, s, http.MethodPost, "/Sessions/Logout", nil, nil)
}

// Movie is what the UI lists.
type Movie struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Year       int    `json:"year,omitempty"`
	Overview   string `json:"overview,omitempty"`
	Runtime    int64  `json:"runtimeSeconds"`
	Resume     int64  `json:"resumeSeconds"`
	Played     bool   `json:"played"`
	Container  string `json:"container,omitempty"`
	VideoCodec string `json:"videoCodec,omitempty"`
	Width      int    `json:"width,omitempty"`
	Audio      string `json:"audio,omitempty"`
	Size       int64  `json:"size,omitempty"`
	HasPoster  bool   `json:"hasPoster"`
}

type item struct {
	Id             string
	Name           string
	ProductionYear int
	Overview       string
	RunTimeTicks   int64
	Container      string
	ImageTags      map[string]string
	UserData       struct {
		PlaybackPositionTicks int64
		Played                bool
	}
	MediaSources []struct {
		Container    string
		Size         int64
		MediaStreams []struct {
			Type, Codec, Language string
			Width                 int
		}
	}
}

func (it item) movie() Movie {
	m := Movie{
		ID: it.Id, Name: it.Name, Year: it.ProductionYear, Overview: it.Overview,
		Runtime: it.RunTimeTicks / ticksPerSecond, Resume: it.UserData.PlaybackPositionTicks / ticksPerSecond,
		Played: it.UserData.Played, Container: it.Container, HasPoster: it.ImageTags["Primary"] != "",
	}
	if len(it.MediaSources) > 0 {
		ms := it.MediaSources[0]
		if ms.Container != "" {
			m.Container = ms.Container
		}
		m.Size = ms.Size
		for _, st := range ms.MediaStreams {
			switch {
			case st.Type == "Video" && m.VideoCodec == "":
				m.VideoCodec, m.Width = st.Codec, st.Width
			case st.Type == "Audio" && m.Audio == "":
				m.Audio = st.Codec
			}
		}
	}
	return m
}

// Movies lists every movie of the user, sorted by name.
func (c *Client) Movies(ctx context.Context, s *Session) ([]Movie, error) {
	q := url.Values{
		"Recursive":        {"true"},
		"IncludeItemTypes": {"Movie"},
		"SortBy":           {"SortName"},
		"Fields":           {"Overview,MediaSources"},
	}
	var res struct {
		Items            []item
		TotalRecordCount int
	}
	if err := c.do(ctx, s, http.MethodGet, "/Users/"+url.PathEscape(s.UserID)+"/Items?"+q.Encode(), nil, &res); err != nil {
		return nil, err
	}
	out := make([]Movie, 0, len(res.Items))
	for _, it := range res.Items {
		out = append(out, it.movie())
	}
	if res.TotalRecordCount > len(out) {
		return out, fmt.Errorf("movies: got %d of %d items", len(out), res.TotalRecordCount)
	}
	return out, nil
}

// Movie returns one item.
func (c *Client) Movie(ctx context.Context, s *Session, id string) (Movie, error) {
	var it item
	if err := c.do(ctx, s, http.MethodGet, "/Users/"+url.PathEscape(s.UserID)+"/Items/"+url.PathEscape(id), nil, &it); err != nil {
		return Movie{}, err
	}
	if it.Id == "" {
		return Movie{}, fmt.Errorf("movies: item %s not found", id)
	}
	return it.movie(), nil
}

// StreamRequest builds the request for the original file; the token goes in
// a header, never in the URL.
func (c *Client) StreamRequest(ctx context.Context, s *Session, method, id string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.Base+"/Videos/"+url.PathEscape(id)+"/stream?static=true", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Emby-Authorization", c.authHeader(s.DeviceID, s.Token))
	return req, nil
}

// ImageRequest builds the request for the Primary image of an item.
func (c *Client) ImageRequest(ctx context.Context, s *Session, id string, maxWidth int) (*http.Request, error) {
	u := fmt.Sprintf("%s/Items/%s/Images/Primary?maxWidth=%d", c.Base, url.PathEscape(id), maxWidth)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Emby-Authorization", c.authHeader(s.DeviceID, s.Token))
	return req, nil
}

// PlayState is reported to Movies so its "continue watching" stays right.
type PlayState struct {
	ItemID        string
	PlaySessionID string
	Position      time.Duration
	Paused        bool
}

func (p PlayState) body() map[string]any {
	return map[string]any{
		"ItemId":        p.ItemID,
		"MediaSourceId": p.ItemID,
		"PlaySessionId": p.PlaySessionID,
		"PositionTicks": int64(p.Position / time.Second * ticksPerSecond),
		"IsPaused":      p.Paused,
		"PlayMethod":    "DirectPlay",
		"CanSeek":       true,
	}
}

func (c *Client) ReportStart(ctx context.Context, s *Session, p PlayState) error {
	return c.do(ctx, s, http.MethodPost, "/Sessions/Playing", p.body(), nil)
}

func (c *Client) ReportProgress(ctx context.Context, s *Session, p PlayState) error {
	return c.do(ctx, s, http.MethodPost, "/Sessions/Playing/Progress", p.body(), nil)
}

func (c *Client) ReportStopped(ctx context.Context, s *Session, p PlayState) error {
	return c.do(ctx, s, http.MethodPost, "/Sessions/Playing/Stopped", p.body(), nil)
}

// Subtitle is one text subtitle track Movies serves as WebVTT.
type Subtitle struct {
	Index    int    `json:"index"`
	Language string `json:"lang,omitempty"`
	Label    string `json:"label"`
}

// mediaSources is the part of an item or PlaybackInfo answer that lists streams.
type mediaSources struct {
	MediaSources []struct {
		Id           string
		MediaStreams []struct {
			Type, Language, DisplayTitle, DeliveryMethod string
			Index, Width                                 int
		}
	}
}

// subtitles returns the text subtitles, the media source ID the subtitle URLs
// need, and whether the streams look fully read (a video stream with a width).
func (res mediaSources) subtitles() ([]Subtitle, string, bool) {
	if len(res.MediaSources) == 0 {
		return nil, "", false
	}
	ms := res.MediaSources[0]
	var out []Subtitle
	probed := false
	for _, st := range ms.MediaStreams {
		if st.Type == "Video" && st.Width > 0 {
			probed = true
		}
		// "External" = Movies converts it and serves Stream.vtt; image
		// subtitles (PGS) would need burning in and are left out.
		if st.Type != "Subtitle" || st.DeliveryMethod != "External" {
			continue
		}
		label := st.DisplayTitle
		if label == "" {
			label = st.Language
		}
		out = append(out, Subtitle{Index: st.Index, Language: st.Language, Label: label})
	}
	return out, ms.Id, probed
}

// Subtitles lists the text subtitles from the item answer (Fields=MediaStreams;
// without that field it shows Video and Audio only). listed is false when the
// answer cannot be trusted to be complete: no subtitle and a video stream
// without width. Measured 2026-10-28, Movies 0.3.0: Avatar's item lists its
// five subtitles, Shelter's lists none and no width, while PlaybackInfo lists
// Shelter's two. ProbedSubtitles asks PlaybackInfo, which has a cost.
func (c *Client) Subtitles(ctx context.Context, s *Session, id string) (subs []Subtitle, msID string, listed bool, err error) {
	var res mediaSources
	q := url.Values{"Fields": {"MediaSources,MediaStreams"}}
	p := "/Users/" + url.PathEscape(s.UserID) + "/Items/" + url.PathEscape(id) + "?" + q.Encode()
	if err := c.do(ctx, s, http.MethodGet, p, nil, &res); err != nil {
		return nil, "", false, err
	}
	subs, msID, probed := res.subtitles()
	return subs, msID, probed || len(subs) > 0, nil
}

// ProbedSubtitles asks PlaybackInfo, which lists the subtitles of every film,
// but Movies answers it by starting a transcode on the box (measured
// 2026-10-28: log "[streamMedia] … transcode codec=h264" and ffmpeg for the
// first segment), although the TV then plays the original file. So it is only
// asked when someone wants subtitles and the item did not list them.
func (c *Client) ProbedSubtitles(ctx context.Context, s *Session, id string) ([]Subtitle, string, error) {
	var res mediaSources
	p := "/Items/" + url.PathEscape(id) + "/PlaybackInfo?UserId=" + url.QueryEscape(s.UserID)
	if err := c.do(ctx, s, http.MethodPost, p, map[string]any{}, &res); err != nil {
		return nil, "", err
	}
	subs, msID, _ := res.subtitles()
	return subs, msID, nil
}

// SubtitleRequest builds the request for one subtitle track as WebVTT.
func (c *Client) SubtitleRequest(ctx context.Context, s *Session, id, mediaSourceID string, index int) (*http.Request, error) {
	u := fmt.Sprintf("%s/Videos/%s/%s/Subtitles/%d/Stream.vtt", c.Base, url.PathEscape(id), url.PathEscape(mediaSourceID), index)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Emby-Authorization", c.authHeader(s.DeviceID, s.Token))
	return req, nil
}

// Resume lists the films with a resume point, most recently watched first
// (the order Movies answers with).
func (c *Client) Resume(ctx context.Context, s *Session) ([]Movie, error) {
	q := url.Values{"MediaTypes": {"Video"}, "Fields": {"Overview,MediaSources"}}
	var res struct{ Items []item }
	if err := c.do(ctx, s, http.MethodGet, "/Users/"+url.PathEscape(s.UserID)+"/Items/Resume?"+q.Encode(), nil, &res); err != nil {
		return nil, err
	}
	out := make([]Movie, 0, len(res.Items))
	for _, it := range res.Items {
		out = append(out, it.movie())
	}
	return out, nil
}
