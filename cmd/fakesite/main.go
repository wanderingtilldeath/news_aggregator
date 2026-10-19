// Command fakesite serves a synthetic, deterministic website for testing and
// benchmarking crawlerx. It can listen on several consecutive ports; each
// port counts as a separate host to the crawler, so per-host politeness does
// not cap the benchmark's throughput.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/fakesite"
)

func main() {
	var (
		addr       = flag.String("addr", "127.0.0.1", "listen address (use 0.0.0.0 in containers)")
		port       = flag.Int("port", 9000, "first port to listen on")
		hosts      = flag.Int("hosts", 1, "number of consecutive ports to serve; each is a separate host")
		pages      = flag.Int("pages", 1000, "pages per host")
		fanout     = flag.Int("fanout", 5, "tree children per page")
		seed       = flag.Uint64("seed", 1, "content seed")
		latency    = flag.Duration("latency", 0, "artificial latency per page")
		crawlDelay = flag.Int("crawl-delay", 0, "Crawl-delay seconds advertised in robots.txt")
		chaos      = flag.Bool("chaos", false, "serve misbehaving endpoints (flaky, 429, redirect loop, huge, pdf...) linked from /")
		logPath    = flag.String("log", "", "append one JSON line per request to this file (- for stdout)")
	)
	flag.Parse()

	var logOut io.Writer
	switch *logPath {
	case "":
	case "-":
		logOut = os.Stdout
	default:
		f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		logOut = f
	}

	site := fakesite.New(fakesite.Options{
		Pages: *pages, Fanout: *fanout, Seed: *seed, Latency: *latency,
		CrawlDelay: *crawlDelay, Chaos: *chaos, Log: logOut,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var servers []*http.Server
	for i := 0; i < *hosts; i++ {
		address := net.JoinHostPort(*addr, fmt.Sprint(*port+i))

		ln, err := net.Listen("tcp", address)
		if err != nil {
			log.Fatalf("listen %s: %v", address, err)
		}

		srv := &http.Server{Handler: site, ReadHeaderTimeout: 10 * time.Second}
		servers = append(servers, srv)

		go func() {
			if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("serve %s: %v", address, err)
			}
		}()

		log.Printf("fakesite: http://%s/ (%d pages)", address, *pages)
	}

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, srv := range servers {
		_ = srv.Shutdown(shutdownCtx)
	}
}
