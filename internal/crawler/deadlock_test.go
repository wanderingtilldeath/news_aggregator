package crawler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const wideFanout = 150

// TestCrawler_WideFanoutDoesNotDeadlock is the regression test for the
// original deadlock: workers both consumed from and pushed into a bounded
// jobs channel (100 slots), so once every worker was blocked sending its
// discovered links into the full queue nobody was left to drain it.
//
// Each page here links to 150 pages (more than the old queue held), and the
// second level links back to all of them, which kept every worker producing
// at once. On the channel-based crawler this hung forever.
func TestCrawler_WideFanoutDoesNotDeadlock(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fanoutLinks()))
	})
	mux.HandleFunc("/p/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fanoutLinks()))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	cfg := testConfig()
	cfg.Workers = 4

	h := newHarness(t, cfg, nil)

	done := make(chan struct{})
	go func() {
		h.crawl(t, server.URL+"/", 2)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("crawler hung: workers blocked on each other")
	}
}

func fanoutLinks() string {
	var b strings.Builder
	for i := 0; i < wideFanout; i++ {
		fmt.Fprintf(&b, `<a href="/p/%d">p%d</a>`, i, i)
	}
	return b.String()
}
