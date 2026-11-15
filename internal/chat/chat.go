// Package chat posts a message to a Slack or Discord channel through its incoming webhook. A webhook
// address is a secret that the owner of the channel makes and pastes in, and it is the only place
// this server ever posts to on a stranger's behalf, so the address is checked hard: https only, and
// only the hosts and paths Slack and Discord use for webhooks. Redirects are never followed.
package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	Slack   = "slack"
	Discord = "discord"
	// MaxMessage keeps a post under Discord's limit of 2000 characters, with room to spare.
	MaxMessage = 1900
)

var (
	// ErrRateLimited means the service asked us to slow down (HTTP 429).
	ErrRateLimited = errors.New("chat: rate limited")
	// ErrGone means the webhook no longer exists (HTTP 404 or 410): the channel owner deleted it.
	ErrGone = errors.New("chat: webhook is gone")
)

// Kind reports whether raw is an incoming-webhook address of Slack or Discord, and which.
func Kind(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" || len(raw) > 300 {
		return "", false
	}
	switch strings.ToLower(u.Hostname()) {
	case "hooks.slack.com":
		if strings.HasPrefix(u.Path, "/services/") && strings.Count(u.Path, "/") == 4 {
			return Slack, true
		}
	case "discord.com", "discordapp.com":
		if strings.HasPrefix(u.Path, "/api/webhooks/") && strings.Count(u.Path, "/") == 4 {
			return Discord, true
		}
	}

	return "", false
}

// Poster sends text to a webhook of the given kind.
type Poster interface {
	Post(ctx context.Context, kind, webhook, text string) error
}

// Webhook posts over HTTPS. The zero value works; tests give it a Client that routes elsewhere.
type Webhook struct {
	Client *http.Client
}

func (w Webhook) client() *http.Client {
	if w.Client != nil {
		return w.Client
	}

	return &http.Client{
		Timeout: 10 * time.Second,
		// A redirect could send the post to an address that was never checked.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func (w Webhook) Post(ctx context.Context, kind, webhook, text string) error {
	if got, ok := Kind(webhook); !ok || got != kind {
		return errors.New("chat: not a webhook address")
	}
	if len(text) > MaxMessage {
		return fmt.Errorf("chat: message is %d characters, the limit is %d", len(text), MaxMessage)
	}

	payload := map[string]any{"text": text, "unfurl_links": false, "unfurl_media": false}
	if kind == Discord {
		// Flag 4 stops Discord from turning every link into a big preview.
		payload = map[string]any{"content": text, "flags": 4}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook, bytes.NewReader(body))
	if err != nil {
		return errors.New("chat: could not build the request") // the address is a secret: leave it out of errors
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client().Do(req)
	if err != nil {
		return errors.New("chat: the post did not reach the service")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return ErrRateLimited
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return ErrGone
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return fmt.Errorf("chat: the service answered %d", resp.StatusCode)
	}

	return nil
}

// Log prints what would be posted and sends nothing, for a dry run.
type Log struct{ Logger *slog.Logger }

func (l Log) Post(_ context.Context, kind, _, text string) error {
	l.Logger.Info("chat post (dry run)", "kind", kind, "text", text)

	return nil
}

// EscapeSlack makes text safe inside a Slack message: Slack reads & < > as markup.
func EscapeSlack(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// EscapeDiscord takes the characters that would break a Discord markdown link or format the text.
func EscapeDiscord(s string) string {
	return strings.NewReplacer("[", "(", "]", ")", "*", "", "_", " ", "`", "'", "~", "").Replace(s)
}
