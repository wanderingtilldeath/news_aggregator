// Package urlnorm turns raw hrefs into the canonical absolute URLs the
// crawler dedupes and fetches. Two spellings of the same page must map to
// one string, or the frontier will crawl it twice.
package urlnorm

import (
	"errors"
	"net/url"
	"path"
	"strings"
)

// trackingParams are query parameters that identify a visitor or campaign
// and never change the page content.
var trackingParams = map[string]bool{
	"gclid": true, "fbclid": true, "msclkid": true, "yclid": true,
	"mc_cid": true, "mc_eid": true, "_ga": true, "igshid": true,
}

// Canonicalize returns the canonical form of an absolute http(s) URL:
// lowercase scheme and host, default port removed, no userinfo or fragment,
// dot-segments resolved, tracking parameters dropped and the query sorted.
// It is idempotent: Canonicalize(Canonicalize(x)) == Canonicalize(x).
func Canonicalize(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return canonical(u)
}

// Resolve resolves link against baseURL and canonicalizes the result. It
// reports ok=false for unparsable URLs or schemes other than http/https
// (mailto:, javascript:, tel:, ...).
func Resolve(link string, baseURL string) (string, bool) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", false
	}

	ref, err := url.Parse(strings.TrimSpace(link))
	if err != nil {
		return "", false
	}

	out, err := canonical(base.ResolveReference(ref))
	if err != nil {
		return "", false
	}

	return out, true
}

// Host returns the lowercase host[:port] of a canonical URL, which is the
// unit of politeness.
func Host(canonicalURL string) string {
	u, err := url.Parse(canonicalURL)
	if err != nil {
		return ""
	}
	return u.Host
}

func canonical(u *url.URL) (string, error) {
	u = cloneURL(u)

	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("urlnorm: unsupported scheme " + u.Scheme)
	}
	if u.Opaque != "" {
		return "", errors.New("urlnorm: opaque URL")
	}

	hostname := strings.TrimRight(strings.ToLower(u.Hostname()), ".")
	if hostname == "" {
		return "", errors.New("urlnorm: missing host")
	}

	port := u.Port()
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}

	u.Host = hostname
	if strings.Contains(hostname, ":") { // bare IPv6 literal
		u.Host = "[" + hostname + "]"
	}
	if port != "" {
		u.Host += ":" + port
	}

	u.User = nil
	u.Fragment = ""
	u.RawFragment = ""
	u.ForceQuery = false

	cleanPath(u)
	u.RawQuery = canonicalQuery(u.RawQuery)

	return u.String(), nil
}

// cleanPath removes dot-segments and doubles an empty path to "/". It works
// on the escaped path so that an encoded slash (%2F) is never turned into a
// real separator.
func cleanPath(u *url.URL) {
	p := u.EscapedPath()
	if p == "" {
		p = "/"
	}

	cleaned := path.Clean(p)
	last := p[strings.LastIndex(p, "/")+1:]
	if strings.HasSuffix(p, "/") || last == "." || last == ".." {
		if cleaned != "/" {
			cleaned += "/"
		}
	}

	decoded, err := url.PathUnescape(cleaned)
	if err != nil {
		decoded = cleaned
	}
	u.Path = decoded
	u.RawPath = cleaned
}

func canonicalQuery(raw string) string {
	if raw == "" {
		return ""
	}

	values, err := url.ParseQuery(raw)
	if err != nil {
		// Unparsable (e.g. a bare ';'): keep as-is rather than guess.
		return raw
	}

	for key := range values {
		if trackingParams[strings.ToLower(key)] || strings.HasPrefix(strings.ToLower(key), "utm_") {
			delete(values, key)
		}
	}

	return values.Encode()
}

func cloneURL(u *url.URL) *url.URL {
	c := *u
	if u.User != nil {
		user := *u.User
		c.User = &user
	}
	return &c
}
