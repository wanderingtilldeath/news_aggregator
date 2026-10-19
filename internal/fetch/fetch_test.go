package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func htmlHandler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(body))
	}
}

func TestHTTPFetcher_Success(t *testing.T) {
	var gotUA, gotAccept string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA, gotAccept = r.Header.Get("User-Agent"), r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("ETag", `"abc"`)
		w.Write([]byte("hello"))
	}))
	defer ts.Close()

	f := New(Options{UserAgent: "TestBot/1.0 (+https://example.test)"})

	res, err := f.Fetch(context.Background(), Request{URL: ts.URL})
	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}

	if res.Status != 200 || string(res.Body) != "hello" || res.ETag != `"abc"` || res.Truncated {
		t.Fatalf("unexpected result: %+v", res)
	}
	if gotUA != "TestBot/1.0 (+https://example.test)" {
		t.Errorf("User-Agent = %q", gotUA)
	}
	if !strings.Contains(gotAccept, "text/html") {
		t.Errorf("Accept = %q", gotAccept)
	}
}

func TestHTTPFetcher_DefaultUserAgentHasContactURL(t *testing.T) {
	if !strings.Contains(DefaultUserAgent, "+http") {
		t.Fatalf("DefaultUserAgent %q should carry a contact URL", DefaultUserAgent)
	}
}

func TestHTTPFetcher_ContextCancelled(t *testing.T) {
	ts := httptest.NewServer(htmlHandler("hello"))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := New(Options{}).Fetch(ctx, Request{URL: ts.URL}); err == nil {
		t.Fatal("Fetch() with a cancelled context should return an error")
	}
}

func TestHTTPFetcher_BodyIsCapped(t *testing.T) {
	ts := httptest.NewServer(htmlHandler(strings.Repeat("x", 10_000)))
	defer ts.Close()

	res, err := New(Options{MaxBytes: 1000}).Fetch(context.Background(), Request{URL: ts.URL})
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Body) != 1000 || !res.Truncated {
		t.Fatalf("len(body)=%d truncated=%v, want 1000 true", len(res.Body), res.Truncated)
	}
}

func TestHTTPFetcher_ExactlyAtCapIsNotTruncated(t *testing.T) {
	ts := httptest.NewServer(htmlHandler(strings.Repeat("x", 1000)))
	defer ts.Close()

	res, err := New(Options{MaxBytes: 1000}).Fetch(context.Background(), Request{URL: ts.URL})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Body) != 1000 || res.Truncated {
		t.Fatalf("len(body)=%d truncated=%v, want 1000 false", len(res.Body), res.Truncated)
	}
}

func TestHTTPFetcher_RejectsNonHTML(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write([]byte("%PDF-1.4"))
	}))
	defer ts.Close()

	_, err := New(Options{}).Fetch(context.Background(), Request{URL: ts.URL})
	if !errors.Is(err, ErrUnsupportedContentType) {
		t.Fatalf("err = %v, want ErrUnsupportedContentType", err)
	}
}

func TestHTTPFetcher_RedirectIsReturnedNotFollowed(t *testing.T) {
	var hits int
	mux := http.NewServeMux()
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Redirect(w, r, "/new", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/new", func(w http.ResponseWriter, r *http.Request) { hits++ })
	ts := httptest.NewServer(mux)
	defer ts.Close()

	res, err := New(Options{}).Fetch(context.Background(), Request{URL: ts.URL + "/old"})
	if err != nil {
		t.Fatal(err)
	}

	if res.Status != 301 || res.Location != ts.URL+"/new" || hits != 1 {
		t.Fatalf("status=%d location=%q hits=%d", res.Status, res.Location, hits)
	}
}

func TestHTTPFetcher_RetryAfterAndStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()

	res, err := New(Options{}).Fetch(context.Background(), Request{URL: ts.URL})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != 429 || res.RetryAfter != 7*time.Second {
		t.Fatalf("status=%d retryAfter=%v", res.Status, res.RetryAfter)
	}
}

func TestHTTPFetcher_ConditionalHeadersAnd304(t *testing.T) {
	var inm, ims string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inm, ims = r.Header.Get("If-None-Match"), r.Header.Get("If-Modified-Since")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer ts.Close()

	res, err := New(Options{}).Fetch(context.Background(), Request{
		URL: ts.URL, ETag: `"v1"`, LastModified: "Mon, 02 Jan 2006 15:04:05 GMT",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != 304 || inm != `"v1"` || ims != "Mon, 02 Jan 2006 15:04:05 GMT" {
		t.Fatalf("status=%d inm=%q ims=%q", res.Status, inm, ims)
	}
}
