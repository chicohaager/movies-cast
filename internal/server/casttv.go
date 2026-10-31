package server

import (
	"context"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"movies-cast/internal/cast"
	"movies-cast/internal/dlna"
)

// Google Cast devices next to DLNA TVs (roadmap 17). A Cast device is a
// dlna.Renderer with Location cast://host:port; castTransport sends those to
// the Cast client and everything else to DLNA, so the play, watch and
// status code stays the same for both.

const castScheme = "cast://"

func isCast(r dlna.Renderer) bool { return strings.HasPrefix(r.Location, castScheme) }

// CastAware wraps the DLNA transport and finder so Cast devices are found
// and played too.
func CastAware(t Transport, r Renderers) (Transport, Renderers) {
	return &castTransport{dlna: t, devices: map[string]*cast.Device{}, pending: map[string]dlna.Media{}},
		&castRenderers{inner: r, discover: cast.Discover, ttl: rendererTTL}
}

type castTransport struct {
	dlna Transport
	dial func(ctx context.Context, addr string) (net.Conn, error) // tests

	mu      sync.Mutex
	devices map[string]*cast.Device // by host:port
	pending map[string]dlna.Media   // SetURI waits for Play, like on a DLNA TV
}

func (c *castTransport) device(r dlna.Renderer) *cast.Device {
	addr := strings.TrimPrefix(r.Location, castScheme)
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.devices[addr]
	if d == nil {
		d = &cast.Device{Addr: addr}
		if c.dial != nil {
			d.Dial = c.dial
		}
		c.devices[addr] = d
	}
	return d
}

func (c *castTransport) SetURI(ctx context.Context, r dlna.Renderer, m dlna.Media) error {
	if !isCast(r) {
		return c.dlna.SetURI(ctx, r, m)
	}
	c.mu.Lock()
	c.pending[r.ID] = m
	c.mu.Unlock()
	return nil
}

func (c *castTransport) Play(ctx context.Context, r dlna.Renderer) error {
	if !isCast(r) {
		return c.dlna.Play(ctx, r)
	}
	c.mu.Lock()
	m, ok := c.pending[r.ID]
	delete(c.pending, r.ID)
	c.mu.Unlock()
	if ok {
		return c.device(r).Load(ctx, m.URL, m.MimeType, m.Title, 0)
	}
	return c.device(r).Play(ctx)
}

func (c *castTransport) Pause(ctx context.Context, r dlna.Renderer) error {
	if !isCast(r) {
		return c.dlna.Pause(ctx, r)
	}
	return c.device(r).Pause(ctx)
}

func (c *castTransport) Stop(ctx context.Context, r dlna.Renderer) error {
	if !isCast(r) {
		return c.dlna.Stop(ctx, r)
	}
	return c.device(r).Stop(ctx)
}

func (c *castTransport) Seek(ctx context.Context, r dlna.Renderer, pos time.Duration) error {
	if !isCast(r) {
		return c.dlna.Seek(ctx, r, pos)
	}
	return c.device(r).Seek(ctx, pos)
}

// castState maps the Cast player state onto the AVTransport states the
// server watches (PLAYING, PAUSED_PLAYBACK, TRANSITIONING, STOPPED).
func castState(ms cast.MediaStatus) string {
	switch ms.PlayerState {
	case "PLAYING":
		return "PLAYING"
	case "PAUSED":
		return "PAUSED_PLAYBACK"
	case "BUFFERING", "LOADING":
		return "TRANSITIONING"
	case "":
		return "NO_MEDIA_PRESENT"
	}
	return "STOPPED" // IDLE: finished, cancelled, interrupted or failed
}

func (c *castTransport) Status(ctx context.Context, r dlna.Renderer) (dlna.Status, error) {
	if !isCast(r) {
		return c.dlna.Status(ctx, r)
	}
	c.mu.Lock()
	m, pending := c.pending[r.ID]
	c.mu.Unlock()
	if pending {
		return dlna.Status{State: "STOPPED", TrackURI: m.URL, Duration: m.Duration}, nil
	}
	ms, err := c.device(r).Status(ctx)
	if err != nil {
		return dlna.Status{}, err
	}
	return dlna.Status{
		State:    castState(ms),
		Position: time.Duration(ms.CurrentTime * float64(time.Second)),
		Duration: time.Duration(ms.Duration * float64(time.Second)),
		TrackURI: ms.ContentID,
	}, nil
}

func (c *castTransport) Volume(ctx context.Context, r dlna.Renderer) (int, error) {
	if !isCast(r) {
		return c.dlna.Volume(ctx, r)
	}
	return c.device(r).Volume(ctx)
}

func (c *castTransport) SetVolume(ctx context.Context, r dlna.Renderer, v int) error {
	if !isCast(r) {
		return c.dlna.SetVolume(ctx, r, v)
	}
	return c.device(r).SetVolume(ctx, v)
}

// Sink: Cast has no protocol list. A wildcard keeps the format hint quiet;
// a file the device cannot play fails at LOAD, and the phone then offers
// converted sound like after a refused DLNA send.
func (c *castTransport) Sink(ctx context.Context, r dlna.Renderer) (string, error) {
	if !isCast(r) {
		return c.dlna.Sink(ctx, r)
	}
	return "http-get:*:*:*", nil
}

// castRenderers adds the Cast devices that answered mDNS to the DLNA list.
type castRenderers struct {
	inner    Renderers
	discover func(context.Context, time.Duration) ([]cast.Found, error)
	ttl      time.Duration

	mu    sync.Mutex
	found []dlna.Renderer
	at    time.Time
}

func (c *castRenderers) Find(ctx context.Context, refresh bool) ([]dlna.Renderer, error) {
	rs, err := c.inner.Find(ctx, refresh)
	c.mu.Lock()
	stale := refresh || time.Since(c.at) > c.ttl
	found := c.found
	c.mu.Unlock()
	if stale {
		fs, derr := c.discover(ctx, 2*time.Second)
		if derr != nil {
			log.Printf("cast: %v", derr)
		} else {
			found = found[:0:0]
			for _, f := range fs {
				found = append(found, dlna.Renderer{ID: "cast:" + f.ID, Name: f.Name, Model: f.Model, Location: castScheme + f.Addr})
			}
			c.mu.Lock()
			c.found, c.at = found, time.Now()
			c.mu.Unlock()
		}
	}
	out := append(append([]dlna.Renderer(nil), rs...), found...)
	if err != nil && len(found) > 0 {
		return out, nil // the DLNA search failed, the Cast devices are still there
	}
	return out, err
}

// The DLNA finder's extras pass through; Cast devices are never handed to
// it (their cast:// location is no description URL).
func (c *castRenderers) LastSearch() dlna.SearchStats {
	if fi, ok := c.inner.(finderInfo); ok {
		return fi.LastSearch()
	}
	return dlna.SearchStats{}
}

func (c *castRenderers) SetExtraHosts(h []string) {
	if fi, ok := c.inner.(finderInfo); ok {
		fi.SetExtraHosts(h)
	}
}

func (c *castRenderers) SetKnown(locs []string) {
	fi, ok := c.inner.(finderInfo)
	if !ok {
		return
	}
	var keep []string
	for _, l := range locs {
		if !strings.HasPrefix(l, castScheme) {
			keep = append(keep, l)
		}
	}
	fi.SetKnown(keep)
}
