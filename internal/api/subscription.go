package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/track"
)

const maxSourceLen = 40

// manageGap is the least time between two "manage your subscription" mails to one address.
const manageGap = 2 * time.Minute

// cleanSource turns a campaign tag from a signup link into a short lower-case label: letters, digits, dot,
// dash and underscore only. Anything else is dropped, so a link cannot put markup or a long text in the
// analytics page.
func cleanSource(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if b.Len() >= maxSourceLen {
			break
		}
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}

	return b.String()
}

// maskEmail shows enough of an address for its owner to recognise it ("a***@example.com").
func maskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 1 {
		return "***"
	}

	return email[:1] + "***" + email[at:]
}

type subscriptionView struct {
	Email  string   `json:"email"`
	Status string   `json:"status"`
	Mode   string   `json:"mode"`
	Topics []string `json:"topics"`
}

func viewOf(sub store.Subscription) subscriptionView {
	topics := sub.Topics
	if topics == nil {
		topics = []string{}
	}

	return subscriptionView{Email: maskEmail(sub.Email), Status: sub.Status, Mode: sub.Mode, Topics: topics}
}

// subscription returns what the token's owner receives, for the manage page. The token is the credential,
// the same one the unsubscribe link carries; the address is masked in the answer.
func (s *server) subscription(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" || len(token) > maxTokenLen {
		writeError(w, http.StatusBadRequest, "invalid token")
		return
	}
	sub, ok, err := s.Store.Subscription(r.Context(), token)
	if err != nil {
		s.Logger.Error("subscription", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load the subscription")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "this link is not valid")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, viewOf(sub))
}

type subscriptionUpdate struct {
	Token  string   `json:"token"`
	Mode   string   `json:"mode"`
	Topics []string `json:"topics"`
}

// updateSubscription changes the reading length and followed topics of the token's owner. An unsubscribed
// reader gets 409: they sign up again from the site instead.
func (s *server) updateSubscription(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSubscribeBody)

	var req subscriptionUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" || len(req.Token) > maxTokenLen {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if !track.ValidMode(req.Mode) {
		writeError(w, http.StatusBadRequest, "reading length must be 1min or 7min")
		return
	}
	sub, ok, err := s.Store.Subscription(r.Context(), req.Token)
	if err != nil {
		s.Logger.Error("update subscription", "err", err)
		writeError(w, http.StatusInternalServerError, "could not save")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "this link is not valid")
		return
	}
	if sub.Status == "unsubscribed" {
		writeError(w, http.StatusConflict, "you have unsubscribed; sign up again to get the briefing")
		return
	}

	// The editions a reader once chose stay as they are: the email is the same for everyone.
	prefs := store.Preferences{Tracks: sub.Tracks, Mode: req.Mode, Topics: req.Topics}
	if _, err := s.Store.SetPreferences(r.Context(), req.Token, prefs); err != nil {
		s.Logger.Error("update subscription", "err", err)
		writeError(w, http.StatusInternalServerError, "could not save")
		return
	}
	sub.Mode, sub.Topics = req.Mode, track.NormalizeTopics(req.Topics)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, viewOf(sub))
}

type analyticsView struct {
	Subscribers struct {
		Pending      int     `json:"pending"`
		Active       int     `json:"active"`
		Unsubscribed int     `json:"unsubscribed"`
		ConfirmRate  float64 `json:"confirm_rate"`
	} `json:"subscribers"`
	Modes   map[string]int `json:"modes"`
	Topics  []topicView    `json:"topics"`
	Daily   []dayView      `json:"daily"`
	Sources []sourceView   `json:"sources"`
}

type topicView struct {
	Topic string `json:"topic"`
	Count int    `json:"count"`
}

type dayView struct {
	Day          string `json:"day"`
	Signups      int    `json:"signups"`
	Confirmed    int    `json:"confirmed"`
	Unsubscribed int    `json:"unsubscribed"`
	Sent         int    `json:"sent"`
}

type sourceView struct {
	Source      string  `json:"source"`
	Signups     int     `json:"signups"`
	Confirmed   int     `json:"confirmed"`
	ConfirmRate float64 `json:"confirm_rate"`
}

func rate(part, whole int) float64 {
	if whole == 0 {
		return 0
	}

	return float64(part) / float64(whole)
}

// analytics returns the audience numbers: readers now, signups and sends per day, signups per source.
// Only counts, never an address. It needs an admin session (see admin.go) whose role may read analytics.
func (s *server) analytics(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, permAnalytics); !ok {
		return
	}
	days := 30
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 365 {
			writeError(w, http.StatusBadRequest, "days must be 1 to 365")
			return
		}
		days = n
	}
	a, err := s.Store.AudienceStats(r.Context(), days, editionZone.String())
	if err != nil {
		s.Logger.Error("analytics", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load the numbers")
		return
	}

	var out analyticsView
	out.Subscribers.Pending, out.Subscribers.Active, out.Subscribers.Unsubscribed = a.Pending, a.Active, a.Unsubscribed
	out.Subscribers.ConfirmRate = rate(a.Active+a.Unsubscribed, a.Pending+a.Active+a.Unsubscribed)
	out.Modes = a.Modes
	out.Topics = make([]topicView, 0, len(a.Topics))
	for _, t := range a.Topics {
		out.Topics = append(out.Topics, topicView{Topic: t.Topic, Count: t.Count})
	}
	out.Daily = make([]dayView, 0, len(a.Daily))
	for _, d := range a.Daily {
		out.Daily = append(out.Daily, dayView(d))
	}
	out.Sources = make([]sourceView, 0, len(a.Sources))
	for _, src := range a.Sources {
		name := src.Source
		if name == "" {
			name = "direct"
		}
		out.Sources = append(out.Sources, sourceView{Source: name, Signups: src.Signups, Confirmed: src.Confirmed, ConfirmRate: rate(src.Confirmed, src.Signups)})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

type manageLinkRequest struct {
	Email string `json:"email"`
}

// manageLink mails an active subscriber the link to manage their subscription: the reader's way back in without a
// password, for one who no longer has an old email. The answer is the same whether or not the address is
// subscribed, so it cannot be used to find out who is. Only an active subscriber is mailed, each address at most
// once every two minutes, and all callers together are held to the sign-up limit, so it cannot be used to flood
// anyone or burn the mail quota.
func (s *server) manageLink(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSubscribeBody)

	var req manageLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	email, ok := parseEmail(req.Email)
	if !ok {
		writeError(w, http.StatusBadRequest, "enter a valid email address")
		return
	}

	token, active, err := s.Store.ManageToken(r.Context(), email)
	if err != nil {
		s.Logger.Error("manage link", "err", err)
		writeError(w, http.StatusInternalServerError, "could not send the link")
		return
	}
	now := time.Now()
	if active && s.ManageNotify != nil && s.signups.allow(now) && s.manageMails.ready(email, now) {
		s.ManageNotify(context.WithoutCancel(r.Context()), email, token)
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "if this address is subscribed, a link is on its way"})
}
