package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/track"
)

const (
	maxSubscribeBody = 4 << 10
	maxEmailLen      = 254
	maxTokenLen      = 128
	// signupsPerMinute caps new subscribe calls across all callers, so a script
	// cannot fill the table or make us send mail to arbitrary addresses.
	signupsPerMinute = 30
)

// limiter is a fixed-window counter shared by all callers.
// shortcut: global, not per client (the web frontend calls us from one server, so
// the client address is not visible here) and per process; when real abuse shows
// up, rate limit per client at the frontend or the Worker.
type limiter struct {
	mu     sync.Mutex
	window time.Time
	n      int
}

func (l *limiter) allow(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.window) >= time.Minute {
		l.window, l.n = now, 0
	}
	l.n++

	return l.n <= signupsPerMinute
}

// parseEmail returns the lowercased address, or false when s is not a plain
// address ("Name <a@b.c>" and anything longer than 254 bytes are refused).
func parseEmail(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > maxEmailLen {
		return "", false
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s || !strings.Contains(a.Address[strings.LastIndex(a.Address, "@"):], ".") {
		return "", false
	}

	return strings.ToLower(a.Address), true
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

type subscribeRequest struct {
	Email string `json:"email"`
	// Tracks are the editions to receive (see package track) and Mode the reading
	// length; empty means every track and the one-minute read.
	Tracks []string `json:"tracks"`
	Mode   string   `json:"mode"`
}

// subscribe records an unconfirmed subscriber. The answer is the same whether
// the address is new, pending or already confirmed, so the endpoint cannot be
// used to find out who is subscribed.
func (s *server) subscribe(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSubscribeBody)

	var req subscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	email, ok := parseEmail(req.Email)
	if !ok {
		writeError(w, http.StatusBadRequest, "enter a valid email address")
		return
	}
	for _, t := range req.Tracks {
		if !track.Valid(t) {
			writeError(w, http.StatusBadRequest, "unknown edition")
			return
		}
	}
	if req.Mode != "" && !track.ValidMode(req.Mode) {
		writeError(w, http.StatusBadRequest, "reading length must be 1min or 7min")
		return
	}
	if !s.signups.allow(time.Now()) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "too many sign-ups right now, try again in a minute")
		return
	}

	fresh, err := newToken()
	if err != nil {
		s.Logger.Error("subscribe: token", "err", err)
		writeError(w, http.StatusInternalServerError, "could not subscribe")
		return
	}
	token, confirmed, err := s.Store.Subscribe(r.Context(), email, fresh)
	if err != nil {
		s.Logger.Error("subscribe", "err", err)
		writeError(w, http.StatusInternalServerError, "could not subscribe")
		return
	}
	// Choices are taken only while the address is unconfirmed: a signup form
	// cannot change what an existing, confirmed reader receives.
	if !confirmed {
		if _, err := s.Store.SetPreferences(r.Context(), token, store.Preferences{Tracks: req.Tracks, Mode: req.Mode}); err != nil {
			s.Logger.Error("subscribe: preferences", "err", err)
			writeError(w, http.StatusInternalServerError, "could not subscribe")
			return
		}
	}
	if !confirmed && s.Notify != nil {
		s.Notify(context.WithoutCancel(r.Context()), email, token)
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "check your inbox to confirm"})
}

type confirmRequest struct {
	Token string `json:"token"`
}

// confirm activates the subscriber who owns the token. It is a POST, not a link
// that confirms on GET, so mail scanners that open every link cannot confirm
// for the reader; the web page asks the reader to press a button.
func (s *server) confirm(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSubscribeBody)

	var req confirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" || len(req.Token) > maxTokenLen {
		writeError(w, http.StatusBadRequest, "invalid token")
		return
	}
	found, err := s.Store.Confirm(r.Context(), req.Token)
	if err != nil {
		s.Logger.Error("confirm", "err", err)
		writeError(w, http.StatusInternalServerError, "could not confirm")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "this confirmation link is not valid")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "confirmed"})
}

// unsubscribe ends the subscription of the token's owner. The token comes from
// the query string or a JSON body: the mail providers' one-click unsubscribe
// (RFC 8058) POSTs form data to the URL in the List-Unsubscribe header, with the
// token in that URL, and the web page posts JSON.
func (s *server) unsubscribe(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSubscribeBody)

	token := r.URL.Query().Get("token")
	if token == "" {
		var req confirmRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			token = req.Token
		}
	}
	if token == "" || len(token) > maxTokenLen {
		writeError(w, http.StatusBadRequest, "invalid token")
		return
	}

	found, err := s.Store.Unsubscribe(r.Context(), token)
	if err != nil {
		s.Logger.Error("unsubscribe", "err", err)
		writeError(w, http.StatusInternalServerError, "could not unsubscribe")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "this unsubscribe link is not valid")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "unsubscribed"})
}
