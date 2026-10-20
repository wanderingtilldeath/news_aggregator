package images

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/fetch"
	"github.com/wanderingtilldeath/news-aggregator/internal/parse"
	"github.com/wanderingtilldeath/news-aggregator/internal/robots"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// plainFetching is a Resolver that may talk to the loopback test server.
func plainFetching(rb robots.Checker) *Fetching {
	return &Fetching{Fetcher: fetch.New(fetch.Options{Timeout: 2 * time.Second}), Robots: rb, Parser: parse.HTMLParser{}}
}

type blockAll struct{}

func (blockAll) Check(context.Context, string) (robots.Decision, error) {
	return robots.Decision{Allowed: false}, nil
}

type robotsDown struct{}

func (robotsDown) Check(context.Context, string) (robots.Decision, error) {
	return robots.Decision{}, errors.New("robots.txt unreachable")
}

func site(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	html := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, "<html><head>"+body+"</head><body>x</body></html>")
		}
	}
	mux.HandleFunc("/og", html(`<meta property="og:image" content="/img/lead.jpg">`))
	mux.HandleFunc("/twitter", html(`<meta name="twitter:image" content="https://cdn.example/t.jpg">`))
	mux.HandleFunc("/none", html(`<title>no image</title>`))
	mux.HandleFunc("/moved", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/og", http.StatusMovedPermanently) })
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/loop", http.StatusFound) })
	mux.HandleFunc("/pdf", func(w http.ResponseWriter, _ *http.Request) { w.Header().Set("Content-Type", "application/pdf") })
	mux.HandleFunc("/forbidden", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) })
	mux.HandleFunc("/slowdown", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) })
	mux.HandleFunc("/boom", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func TestFetchingImage(t *testing.T) {
	ts := site(t)
	f := plainFetching(nil)

	for _, tt := range []struct{ path, want string }{
		{"/og", ts.URL + "/img/lead.jpg"}, // relative og:image resolved against the source page
		{"/twitter", "https://cdn.example/t.jpg"},
		{"/none", ""},
		{"/moved", ts.URL + "/img/lead.jpg"}, // one redirect hop is followed
		{"/loop", ""},                        // a redirect loop gives up, with no error
		{"/pdf", ""},                         // not HTML: nothing to read
		{"/boom", ""},                        // an HTTP error is "no image", not a failure
		{"/missing", ""},
	} {
		got, err := f.Image(context.Background(), ts.URL+tt.path)
		if err != nil || got != tt.want {
			t.Errorf("Image(%s) = %q, %v; want %q", tt.path, got, err, tt.want)
		}
	}

	if got, err := f.Image(context.Background(), "ftp://example.com/x"); err != nil || got != "" {
		t.Errorf("a non-http URL should be ignored, got %q, %v", got, err)
	}
}

func TestFetchingHonoursRobots(t *testing.T) {
	ts := site(t)

	if got, err := plainFetching(blockAll{}).Image(context.Background(), ts.URL+"/og"); err != nil || got != "" {
		t.Errorf("robots-blocked source gave %q, %v; want no image and no error", got, err)
	}
	if _, err := plainFetching(robotsDown{}).Image(context.Background(), ts.URL+"/og"); err == nil {
		t.Error("an unreachable robots.txt is transient and must surface as an error")
	}
}

func TestProductionResolverRefusesNonPublicAddresses(t *testing.T) {
	ts := site(t) // listens on 127.0.0.1

	f := NewFetching("test-agent", 2*time.Second, 1<<20, nil, false)
	if _, err := f.Image(context.Background(), ts.URL+"/og"); err == nil || !strings.Contains(err.Error(), "non-public") {
		t.Fatalf("a loopback source must be refused at connect time, got %v", err)
	}
}

type fakeResolver func(source string) (string, error)

func (f fakeResolver) Image(_ context.Context, source string) (string, error) { return f(source) }

func TestPassMarksDefinitiveAnswersAndRetriesFailures(t *testing.T) {
	st := store.NewMemory()
	for _, n := range []string{"has-image", "no-image", "flaky"} {
		err := st.Save(context.Background(), store.Page{
			URLHash: n, URL: "http://a.test/" + n, Title: n, IsArticle: true, Image: "http://a.test/logo.jpg",
			SourceURL: "http://news.test/" + n,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// An article that cites nothing is never looked up.
	_ = st.Save(context.Background(), store.Page{URLHash: "plain", URL: "http://a.test/plain", IsArticle: true})

	r := fakeResolver(func(src string) (string, error) {
		switch src {
		case "http://news.test/has-image":
			return "http://news.test/lead.jpg", nil
		case "http://news.test/flaky":
			return "", errors.New("timeout")
		}
		return "", nil
	})

	since := time.Now().Add(-time.Hour)
	n, err := Pass(context.Background(), st, r, since, 10, 0, nil, quiet)
	if err != nil || n != 1 {
		t.Fatalf("Pass = %d, %v; want 1 image found", n, err)
	}

	pages := st.Pages()
	if pages["http://a.test/has-image"].Image != "http://news.test/lead.jpg" {
		t.Errorf("image not stored: %+v", pages["http://a.test/has-image"])
	}
	if pages["http://a.test/no-image"].Image != "http://a.test/logo.jpg" {
		t.Errorf("no result must keep the page's own image: %+v", pages["http://a.test/no-image"])
	}

	pending, _ := st.PendingImages(context.Background(), since, 10)
	if len(pending) != 1 || pending[0].URL != "http://a.test/flaky" {
		t.Errorf("pending = %+v; want only the flaky one to be retried next run", pending)
	}
}

func TestPassStopsWhenContextIsCancelled(t *testing.T) {
	st := store.NewMemory()
	for _, n := range []string{"a", "b", "c"} {
		_ = st.Save(context.Background(), store.Page{URLHash: n, URL: "http://a.test/" + n, IsArticle: true, SourceURL: "http://news.test/" + n})
	}

	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	r := fakeResolver(func(string) (string, error) { calls++; cancel(); return "", nil })

	if _, err := Pass(ctx, st, r, time.Now().Add(-time.Hour), 10, time.Millisecond, nil, quiet); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Errorf("%d lookups after cancel, want 1", calls)
	}
}

func TestAllowPrivateLiftsTheRestrictionForLocalTesting(t *testing.T) {
	ts := site(t)

	f := NewFetching("test-agent", 2*time.Second, 1<<20, nil, true)
	if got, err := f.Image(context.Background(), ts.URL+"/og"); err != nil || got != ts.URL+"/img/lead.jpg" {
		t.Fatalf("Image = %q, %v", got, err)
	}
}

func TestFetchingReportsBotBlocks(t *testing.T) {
	ts := site(t)
	f := plainFetching(nil)

	for path, status := range map[string]int{"/forbidden": 403, "/slowdown": 429} {
		_, err := f.Image(context.Background(), ts.URL+path)
		var be *BlockedError
		if !errors.As(err, &be) || be.Status != status {
			t.Errorf("%s: err = %v, want a BlockedError with status %d", path, err, status)
		}
	}

	// A plain server error is not a block: it may pass, so it is an ordinary "no image".
	if got, err := f.Image(context.Background(), ts.URL+"/boom"); err != nil || got != "" {
		t.Errorf("a 500 should be a quiet no-image, got %q, %v", got, err)
	}
}

func TestPassStopsAskingAHostThatBlocksUs(t *testing.T) {
	st := store.NewMemory()
	save := func(name, source string) {
		if err := st.Save(context.Background(), store.Page{
			URLHash: name, URL: "http://a.test/" + name, Title: name, IsArticle: true,
			Image: "http://a.test/logo.jpg", SourceURL: source,
		}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 6; i++ {
		save(fmt.Sprintf("pib%d", i), fmt.Sprintf("https://pib.test/release/%d", i))
	}
	save("hindu1", "https://thehindu.test/story-1")
	save("hindu2", "https://thehindu.test/story-2")

	asked := map[string]int{}
	r := fakeResolver(func(src string) (string, error) {
		host := strings.Split(strings.TrimPrefix(src, "https://"), "/")[0]
		asked[host]++
		if host == "pib.test" {
			return "", &BlockedError{Host: host, Status: 403}
		}
		return "https://cdn.test/lead.jpg", nil
	})

	since := time.Now().Add(-time.Hour)
	n, err := Pass(context.Background(), st, r, since, 50, 0, nil, quiet)
	if err != nil || n != 2 {
		t.Fatalf("Pass = %d, %v; want the 2 other-host images", n, err)
	}
	if asked["pib.test"] != blockedLimit {
		t.Errorf("the blocking host was asked %d times, want %d (then skipped)", asked["pib.test"], blockedLimit)
	}
	if asked["thehindu.test"] != 2 {
		t.Errorf("another host must be unaffected, asked %d times", asked["thehindu.test"])
	}
	if pending, _ := st.PendingImages(context.Background(), since, 50); len(pending) != 0 {
		t.Errorf("%d articles still pending: blocked ones count as looked up, so they are not retried every run", len(pending))
	}
}

func TestAnAnswerResetsTheBlockCount(t *testing.T) {
	st := store.NewMemory()
	for i := 0; i < 5; i++ {
		_ = st.Save(context.Background(), store.Page{URLHash: fmt.Sprint(i), URL: fmt.Sprintf("http://a.test/%d", i), IsArticle: true,
			SourceURL: fmt.Sprintf("https://flaky.test/%d", i)})
	}

	calls := 0
	r := fakeResolver(func(string) (string, error) {
		calls++
		if calls%2 == 1 { // refused, answered, refused, answered, refused: never two refusals in a row
			return "", &BlockedError{Host: "flaky.test", Status: 429}
		}
		return "https://cdn.test/x.jpg", nil
	})

	if _, err := Pass(context.Background(), st, r, time.Now().Add(-time.Hour), 50, 0, nil, quiet); err != nil {
		t.Fatal(err)
	}
	if calls != 5 {
		t.Errorf("asked %d times, want all 5: a host that answers in between is not blocking us", calls)
	}
}

func TestSharedBlocklistSpansPasses(t *testing.T) {
	st := store.NewMemory()
	save := func(name string) {
		_ = st.Save(context.Background(), store.Page{URLHash: name, URL: "http://a.test/" + name, IsArticle: true,
			SourceURL: "https://pib.test/release/" + name})
	}

	calls := 0
	r := fakeResolver(func(string) (string, error) { calls++; return "", &BlockedError{Host: "pib.test", Status: 403} })
	bl := NewBlocklist()
	since := time.Now().Add(-time.Hour)

	for i := 0; i < 3; i++ {
		save(fmt.Sprintf("first%d", i))
	}
	if _, err := Pass(context.Background(), st, r, since, 50, 0, bl, quiet); err != nil {
		t.Fatal(err)
	}
	if calls != blockedLimit {
		t.Fatalf("first pass asked %d times, want %d", calls, blockedLimit)
	}

	// A later pass in the same run finds new articles citing the same host.
	for i := 0; i < 3; i++ {
		save(fmt.Sprintf("second%d", i))
	}
	if _, err := Pass(context.Background(), st, r, since, 50, 0, bl, quiet); err != nil {
		t.Fatal(err)
	}
	if calls != blockedLimit {
		t.Errorf("after the host was blocked, a later pass asked again (%d calls in total)", calls)
	}
	if pending, _ := st.PendingImages(context.Background(), since, 50); len(pending) != 0 {
		t.Errorf("%d still pending", len(pending))
	}

	// Without a shared blocklist each pass starts fresh.
	save("third")
	if _, err := Pass(context.Background(), st, r, since, 50, 0, nil, quiet); err != nil {
		t.Fatal(err)
	}
	if calls != blockedLimit+1 {
		t.Errorf("a pass with its own blocklist should ask once more, got %d calls", calls)
	}
}
