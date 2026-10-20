package images

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

var png = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01}

type lookup map[string]string

func (l lookup) ImageURL(_ context.Context, id string) (string, error) { return l[id], nil }

func get(t *testing.T, h http.Handler, id string, hdr ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/img/"+id, nil)
	req.SetPathValue("id", id)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// upstream serves a few kinds of "image" and counts requests per path.
func upstream(t *testing.T) (*httptest.Server, *sync.Map) {
	t.Helper()
	var counts sync.Map
	bump := func(p string) {
		v, _ := counts.LoadOrStore(p, new(atomic.Int32))
		v.(*atomic.Int32).Add(1)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ok.png", func(w http.ResponseWriter, _ *http.Request) {
		bump("/ok.png")
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	})
	mux.HandleFunc("/mislabelled.png", func(w http.ResponseWriter, _ *http.Request) {
		bump("/mislabelled.png")
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("<html><body>Access denied</body></html>"))
	})
	mux.HandleFunc("/vector.svg", func(w http.ResponseWriter, _ *http.Request) {
		bump("/vector.svg")
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	})
	mux.HandleFunc("/huge.png", func(w http.ResponseWriter, _ *http.Request) {
		bump("/huge.png")
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(append(append([]byte{}, png...), make([]byte, maxImageBytes+10)...))
	})
	mux.HandleFunc("/gone.png", func(w http.ResponseWriter, _ *http.Request) {
		bump("/gone.png")
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &counts
}

func hits(c *sync.Map, path string) int {
	if v, ok := c.Load(path); ok {
		return int(v.(*atomic.Int32).Load())
	}
	return 0
}

func TestProxyServesCachesAndRevalidates(t *testing.T) {
	srv, counts := upstream(t)
	p := NewProxy(lookup{"a": srv.URL + "/ok.png"}, "test", true)

	first := get(t, p, "a")
	if first.Code != 200 || first.Header().Get("Content-Type") != "image/png" || first.Body.Len() != len(png) {
		t.Fatalf("first = %d %q %d bytes", first.Code, first.Header().Get("Content-Type"), first.Body.Len())
	}
	for h, want := range map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "default-src 'none'; sandbox",
	} {
		if got := first.Header().Get(h); got != want {
			t.Errorf("%s = %q, want %q", h, got, want)
		}
	}
	if !strings.Contains(first.Header().Get("Cache-Control"), "max-age=") || first.Header().Get("ETag") == "" {
		t.Errorf("headers = %v; want browser caching and an ETag", first.Header())
	}

	second := get(t, p, "a")
	if second.Code != 200 || hits(counts, "/ok.png") != 1 {
		t.Errorf("second view hit upstream %d times, want 1 in total", hits(counts, "/ok.png"))
	}

	notModified := get(t, p, "a", "If-None-Match", first.Header().Get("ETag"))
	if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 {
		t.Errorf("revalidation = %d with %d bytes, want 304 and no body", notModified.Code, notModified.Body.Len())
	}
}

func TestProxyRejectsWhatIsNotARasterImage(t *testing.T) {
	srv, counts := upstream(t)
	p := NewProxy(lookup{
		"html": srv.URL + "/mislabelled.png", "svg": srv.URL + "/vector.svg", "huge": srv.URL + "/huge.png", "gone": srv.URL + "/gone.png",
	}, "test", true)

	for _, id := range []string{"html", "svg", "huge", "gone"} {
		if rec := get(t, p, id); rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404 (mislabelled HTML, SVG, oversized and missing images are never served)", id, rec.Code)
		}
	}

	// A failure is remembered, so a broken image is not refetched on every page view.
	for i := 0; i < 5; i++ {
		get(t, p, "gone")
	}
	if n := hits(counts, "/gone.png"); n != 1 {
		t.Errorf("broken image fetched %d times, want 1", n)
	}
}

func TestProxyIgnoresUnknownAndHostileIDs(t *testing.T) {
	p := NewProxy(lookup{}, "test", true)

	for _, id := range []string{"unknown", strings.Repeat("a", 200), ""} {
		if rec := get(t, p, id); rec.Code != http.StatusNotFound {
			t.Errorf("id %.10q = %d, want 404", id, rec.Code)
		}
	}
	// The URL always comes from the store, never from the request.
	req := httptest.NewRequest("GET", "/img/x?url=http://169.254.169.254/latest/meta-data/", nil)
	req.SetPathValue("id", "x")
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("a url= parameter must be ignored, got %d", rec.Code)
	}
}

func TestProxyRefusesNonPublicUpstreams(t *testing.T) {
	srv, counts := upstream(t) // on loopback
	p := NewProxy(lookup{"a": srv.URL + "/ok.png"}, "test", false)

	if rec := get(t, p, "a"); rec.Code != http.StatusNotFound {
		t.Errorf("a loopback image URL gave %d, want 404", rec.Code)
	}
	if n := hits(counts, "/ok.png"); n != 0 {
		t.Errorf("the proxy connected to a loopback address %d times", n)
	}
}

func TestProxyFetchesOncePerImageUnderConcurrentViews(t *testing.T) {
	srv, counts := upstream(t)
	p := NewProxy(lookup{"a": srv.URL + "/ok.png"}, "test", true)

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rec := get(t, p, "a"); rec.Code != 200 {
				t.Errorf("status %d", rec.Code)
			}
		}()
	}
	wg.Wait()

	if n := hits(counts, "/ok.png"); n != 1 {
		t.Errorf("upstream hit %d times for 30 concurrent views, want 1", n)
	}
}

func TestProxyCacheIsBounded(t *testing.T) {
	srv, counts := upstream(t)
	p := NewProxy(lookup{"a": srv.URL + "/ok.png", "b": srv.URL + "/ok.png?b"}, "test", true)
	p.MaxCacheBytes = int64(len(png)) + 1 // room for one image

	get(t, p, "a")
	get(t, p, "b")
	get(t, p, "a") // evicted by b, so it must be fetched again

	if n := hits(counts, "/ok.png"); n != 3 {
		t.Errorf("%d upstream fetches, want 3 (a, b, a again after eviction)", n)
	}
	if p.bytes > p.MaxCacheBytes+int64(len(png)) {
		t.Errorf("cache holds %d bytes, over its %d budget", p.bytes, p.MaxCacheBytes)
	}
}
