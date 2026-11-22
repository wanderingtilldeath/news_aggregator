// Command takedown honours a publisher's request: it deletes every stored page of the given hosts and drops their
// stories from the monthly compilations (which keep a copy after the pages are pruned).
//
//	DATABASE_URL=postgres://... go run ./cmd/takedown example.com www.other.org
//
// Add the same hosts to BLOCKED_HOSTS (GitHub variable, and Render if it runs the daily job) so they are not
// crawled again, then run the crawl workflow so the published snapshot is rebuilt without them. With no hosts
// given it uses BLOCKED_HOSTS, a comma separated list, which is how the crawl workflow calls it.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/wanderingtilldeath/news-aggregator/internal/db"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "takedown:", err)
		os.Exit(1)
	}
}

func run() error {
	hosts := os.Args[1:]
	if len(hosts) == 0 {
		hosts = strings.Split(os.Getenv("BLOCKED_HOSTS"), ",")
	}
	var clean []string
	for _, h := range hosts {
		if h = strings.TrimSpace(h); h != "" {
			clean = append(clean, h)
		}
	}
	if len(clean) == 0 {
		fmt.Println("takedown: no hosts given and BLOCKED_HOSTS is empty; nothing to do")
		return nil
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, url, 2)
	if err != nil {
		return err
	}
	defer pool.Close()
	pages := store.NewPostgres(pool)
	defer pages.Close()

	got, err := pages.PurgeHosts(ctx, clean)
	if err != nil {
		return err
	}
	fmt.Printf("takedown: %d hosts (%s): deleted %d pages and dropped %d stories from the monthly compilations\n",
		len(clean), strings.Join(clean, ", "), got.Pages, got.Compiled)

	return nil
}
