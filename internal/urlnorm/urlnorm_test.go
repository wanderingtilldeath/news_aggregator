package urlnorm

import "testing"

func TestResolve(t *testing.T) {
	tests := []struct {
		name    string
		link    string
		baseURL string
		want    string
		wantOK  bool
	}{
		{"relative path resolved against base", "/path", "https://example.com/base/", "https://example.com/path", true},
		{"fragment removed", "/path#top", "https://example.com/", "https://example.com/path", true},
		{"non-http scheme rejected", "mailto:someone@example.com", "https://example.com/", "", false},
		{"invalid base URL rejected", "/path", "://not-a-url", "", false},
		{"relative to directory", "next", "https://example.com/a/b", "https://example.com/a/next", true},
		{"dot segments in relative link", "../x", "https://example.com/a/b/c", "https://example.com/a/x", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Resolve(tt.link, tt.baseURL)
			if ok != tt.wantOK {
				t.Fatalf("Resolve() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Fatalf("Resolve() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCanonicalize(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"HTTP://Example.COM/", "http://example.com/"},
		{"http://example.com", "http://example.com/"},
		{"http://example.com:80/a", "http://example.com/a"},
		{"https://example.com:443/a", "https://example.com/a"},
		{"https://example.com:8443/a", "https://example.com:8443/a"},
		{"http://example.com./a", "http://example.com/a"},
		{"http://user:pw@example.com/a", "http://example.com/a"},
		{"http://example.com/a#frag", "http://example.com/a"},
		{"http://example.com/a/./b/../c", "http://example.com/a/c"},
		{"http://example.com/a/b/", "http://example.com/a/b/"},
		{"http://example.com/a/b/..", "http://example.com/a/"},
		{"http://example.com/../a", "http://example.com/a"},
		{"http://example.com/a%2Fb", "http://example.com/a%2Fb"},
		{"http://example.com/?b=2&a=1", "http://example.com/?a=1&b=2"},
		{"http://example.com/?utm_source=x&a=1&gclid=z", "http://example.com/?a=1"},
		{"http://example.com/?utm_source=x", "http://example.com/"},
		{"http://example.com/?", "http://example.com/"},
		{"http://[::1]:80/", "http://[::1]/"},
	}

	for _, tt := range tests {
		got, err := Canonicalize(tt.in)
		if err != nil {
			t.Errorf("Canonicalize(%q) error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Canonicalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCanonicalizeRejects(t *testing.T) {
	for _, in := range []string{"ftp://example.com/", "mailto:a@b.c", "/relative", "http:///nohost", ""} {
		if got, err := Canonicalize(in); err == nil {
			t.Errorf("Canonicalize(%q) = %q, want error", in, got)
		}
	}
}

func TestHost(t *testing.T) {
	if got := Host("http://example.com:8080/a"); got != "example.com:8080" {
		t.Fatalf("Host() = %q", got)
	}
}

func FuzzCanonicalize(f *testing.F) {
	for _, seed := range []string{
		"http://example.com/", "HTTPS://Example.com:443/a/../b?z=1&a=2#f",
		"http://example.com/a%2Fb/./c/", "http://[::1]:80/x", "http://u:p@h/?utm_a=1;b",
		"http://example.com/%zz", "http://example.com/a/b/..", "http://example.com//a//b",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		first, err := Canonicalize(raw)
		if err != nil {
			return
		}

		second, err := Canonicalize(first)
		if err != nil {
			t.Fatalf("canonical form %q of %q does not re-canonicalize: %v", first, raw, err)
		}
		if first != second {
			t.Fatalf("not idempotent: %q -> %q -> %q", raw, first, second)
		}
	})
}
