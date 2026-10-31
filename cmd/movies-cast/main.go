// movies-cast plays IceWhale Movies on DLNA TVs, controlled from a phone.
//
// Environment:
//
//	LISTEN      address to listen on (default :8097)
//	MOVIES_URL  Movies' Jellyfin-compatible API (default http://127.0.0.1/movies)
//	PUBLIC_URL  URL the TV uses to reach this server (default: derived per TV)
//	RENDERERS   comma-separated description URLs or hosts (URL found by SSDP), in addition to SSDP
//	SSDP_PORT   local UDP port of the SSDP search (default 1901; 0 = any), so a firewall can allow the answers
//	SSDP        "false" turns discovery off
//	STATE_FILE  file that keeps sessions across restarts (holds Movies tokens, 0600)
//	UPDATE_CHECK "true": the status card names the newest release (asks GitHub once a day)
//	CAST        "false" turns the search for Google Cast devices off
//	TLS_LISTEN  also serve HTTPS here (e.g. :8443); the TVs keep fetching films over HTTP
//	TLS_CERT, TLS_KEY  certificate and key for HTTPS (e.g. from `tailscale cert`);
//	            without them a self-signed one is created once in TLS_DIR
//	TLS_DIR     where the self-signed certificate lives (default: the STATE_FILE's directory, else .)
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // scratch image has no zoneinfo; for a TZ=<zone> set by hand (the compose binds /etc/localtime instead)

	"movies-cast/internal/dlna"
	"movies-cast/internal/movies"
	"movies-cast/internal/server"
	"movies-cast/internal/tlsconf"
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
	ssdpPort, err := strconv.Atoi(env("SSDP_PORT", "1901"))
	if err != nil || ssdpPort < 0 || ssdpPort > 65535 {
		log.Fatalf("SSDP_PORT %q: not a port", os.Getenv("SSDP_PORT"))
	}
	finder := dlna.NewFinder(static, env("SSDP", "true") != "false", ssdpPort, tv)
	if err := finder.Open(context.Background()); err != nil {
		log.Printf("ssdp: port %d: %v (searching from a random port instead)", ssdpPort, err)
		finder.Port = 0
	}
	// Google Cast devices next to the DLNA TVs (CAST=false turns it off).
	var transport server.Transport = tv
	var renderers server.Renderers = finder
	if env("CAST", "true") != "false" {
		transport, renderers = server.CastAware(tv, finder)
	}
	srv := server.New(server.Config{PublicURL: os.Getenv("PUBLIC_URL"), Port: port, StateFile: os.Getenv("STATE_FILE")}, mc, transport, renderers)

	if env("UPDATE_CHECK", "false") == "true" {
		srv.UpdateCheck()
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// No WriteTimeout: a stream runs for hours. IdleTimeout closes kept-alive
	// connections nobody uses; without it a client could hold sockets forever.
	hs := &http.Server{Addr: listen, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", listen)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("movies-cast %s listening on %s, movies=%s, ssdp=%v, static renderers=%v", version, ln.Addr(), mc.Base, finder.SSDP, static)

	// Serve returns as soon as Shutdown starts; main waits for Shutdown, or
	// the process ended before running requests had their 5 s.
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := hs.Shutdown(sctx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()
	if tl := os.Getenv("TLS_LISTEN"); tl != "" {
		dir := env("TLS_DIR", ".")
		if sf := os.Getenv("STATE_FILE"); sf != "" && os.Getenv("TLS_DIR") == "" {
			dir = filepath.Dir(sf)
		}
		cert, err := tlsconf.Load(os.Getenv("TLS_CERT"), os.Getenv("TLS_KEY"), dir, dlna.OwnIPs())
		if err != nil {
			log.Fatalf("TLS: %v", err)
		}
		tln, err := lc.Listen(ctx, "tcp", tl)
		if err != nil {
			log.Fatal(err)
		}
		ts := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute,
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}}
		log.Printf("HTTPS on %s (%s)", tln.Addr(), map[bool]string{true: "own certificate", false: "self-signed, in " + dir}[os.Getenv("TLS_CERT") != ""])
		go func() {
			<-ctx.Done()
			sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = ts.Shutdown(sctx)
		}()
		go func() {
			if err := ts.ServeTLS(tln, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatal(err)
			}
		}()
	}
	if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-done
}
