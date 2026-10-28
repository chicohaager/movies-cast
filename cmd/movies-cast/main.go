// movies-cast plays IceWhale Movies on DLNA TVs, controlled from a phone.
//
// Environment:
//
//	LISTEN      address to listen on (default :8097)
//	MOVIES_URL  Movies' Jellyfin-compatible API (default http://127.0.0.1/movies)
//	PUBLIC_URL  URL the TV uses to reach this server (default: derived per TV)
//	RENDERERS   comma-separated device description URLs, in addition to SSDP
//	SSDP        "false" turns discovery off
//	STATE_FILE  file that keeps sessions across restarts (holds Movies tokens, 0600)
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // scratch image has no zoneinfo; without it TZ is ignored and logs are UTC

	"movies-cast/internal/dlna"
	"movies-cast/internal/movies"
	"movies-cast/internal/server"
)

var version = "dev"

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func list(k string) []string {
	var out []string
	for _, s := range strings.Split(os.Getenv(k), ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func probe(listen string) int {
	_, port, err := net.SplitHostPort(listen)
	if err != nil {
		return 1
	}
	c := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1:"+port+"/healthz", nil)
	if err != nil {
		return 1
	}
	resp, err := c.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthz:", resp.Status)
		return 1
	}
	return 0
}

func main() {
	showVersion := flag.Bool("version", false, "print version")
	healthcheck := flag.Bool("healthcheck", false, "probe /healthz of the running server and exit 0/1 (for scratch images without curl)")
	flag.Parse()
	if *showVersion {
		fmt.Println("movies-cast", version)
		return
	}
	listen := env("LISTEN", ":8097")
	if *healthcheck {
		os.Exit(probe(listen))
	}
	_, portStr, err := net.SplitHostPort(listen)
	if err != nil {
		log.Fatalf("LISTEN %q: %v", listen, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		log.Fatalf("LISTEN %q: port: %v", listen, err)
	}
	static := list("RENDERERS")
	mc := movies.New(env("MOVIES_URL", "http://127.0.0.1/movies"), version)
	tv := dlna.NewClient()
	finder := dlna.NewFinder(static, env("SSDP", "true") != "false", tv)
	srv := server.New(server.Config{PublicURL: os.Getenv("PUBLIC_URL"), Port: port, StateFile: os.Getenv("STATE_FILE")}, mc, tv, finder)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	hs := &http.Server{Addr: listen, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", listen)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("movies-cast %s listening on %s, movies=%s, ssdp=%v, static renderers=%v", version, ln.Addr(), mc.Base, finder.SSDP, static)

	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hs.Shutdown(sctx)
	}()
	if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
