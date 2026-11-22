package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestKindAcceptsOnlyRealWebhookAddresses(t *testing.T) {
	for raw, want := range map[string]string{
		"https://hooks.slack.com/services/T000/B000/XXXX":     Slack,
		"https://discord.com/api/webhooks/123456/abcDEF-_":    Discord,
		"https://discordapp.com/api/webhooks/123456/abcDEF":   Discord,
		" https://hooks.slack.com/services/T000/B000/XXXX \n": Slack,
	} {
		if got, ok := Kind(raw); !ok || got != want {
			t.Errorf("Kind(%q) = %q, %v; want %q", raw, got, ok, want)
		}
	}
	for _, raw := range []string{
		"", "not a url", "http://hooks.slack.com/services/T/B/x", // not https
		"https://evil.test/services/T/B/x", "https://hooks.slack.com.evil.test/services/T/B/x", "https://evil.test@hooks.slack.com/services/T/B/x",
		"https://hooks.slack.com:8443/services/T/B/x", "https://hooks.slack.com/services/T/B/x?redirect=https://evil.test",
		"https://hooks.slack.com/other/T/B/x", "https://hooks.slack.com/services/T/B", "https://hooks.slack.com/services/T/B/x/extra",
		"https://discord.com/api/webhooks/123", "https://discord.com/channels/1/2/3", "https://127.0.0.1/services/T/B/x",
		"https://hooks.slack.com/services/T/B/" + strings.Repeat("x", 400),
	} {
		if got, ok := Kind(raw); ok {
			t.Errorf("Kind(%q) = %q, true; want it refused", raw, got)
		}
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func answer(status int, seen *[]byte, host *string) *http.Client {
	return &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if seen != nil {
			*seen, _ = io.ReadAll(r.Body)
		}
		if host != nil {
			*host = r.URL.Host
		}

		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("ok")), Header: http.Header{}}, nil
	})}
}

func TestWebhookPostsTheRightPayloadForEachService(t *testing.T) {
	var body []byte
	var host string
	if err := (Webhook{Client: answer(200, &body, &host)}).Post(context.Background(), Slack, "https://hooks.slack.com/services/T/B/x", "hello <b>"); err != nil {
		t.Fatal(err)
	}
	var slack map[string]any
	_ = json.Unmarshal(body, &slack)
	if slack["text"] != "hello <b>" || slack["unfurl_links"] != false || host != "hooks.slack.com" {
		t.Fatalf("slack payload = %v to %s", slack, host)
	}

	if err := (Webhook{Client: answer(204, &body, &host)}).Post(context.Background(), Discord, "https://discord.com/api/webhooks/1/y", "hello"); err != nil {
		t.Fatalf("Discord answers 204 on success: %v", err)
	}
	var discord map[string]any
	_ = json.Unmarshal(body, &discord)
	if am, _ := discord["allowed_mentions"].(map[string]any); discord["content"] != "hello" || discord["flags"] != float64(4) || am == nil || len(am["parse"].([]any)) != 0 {
		t.Fatalf("discord payload = %v, want content and the suppress-embeds flag", discord)
	}
}

func TestWebhookRefusesBadAddressesAndMapsFailures(t *testing.T) {
	w := Webhook{Client: answer(200, nil, nil)}
	ctx := context.Background()
	if err := w.Post(ctx, Slack, "https://evil.test/services/T/B/x", "x"); err == nil {
		t.Fatal("posted to a host that is not Slack")
	}
	if err := w.Post(ctx, Discord, "https://hooks.slack.com/services/T/B/x", "x"); err == nil {
		t.Fatal("posted a Discord message to a Slack address")
	}
	if err := w.Post(ctx, Slack, "https://hooks.slack.com/services/T/B/x", strings.Repeat("x", MaxMessage+1)); err == nil {
		t.Fatal("posted a message over the limit")
	}

	for status, want := range map[int]error{429: ErrRateLimited, 404: ErrGone, 410: ErrGone, 400: ErrRejected, 401: ErrRejected, 403: ErrRejected} {
		if err := (Webhook{Client: answer(status, nil, nil)}).Post(ctx, Slack, "https://hooks.slack.com/services/T/B/x", "x"); !errors.Is(err, want) {
			t.Errorf("status %d gave %v, want %v", status, err, want)
		}
	}
	err := (Webhook{Client: answer(500, nil, nil)}).Post(ctx, Slack, "https://hooks.slack.com/services/T/B/SECRET", "x")
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("a failure must not repeat the secret address: %v", err)
	}
}

func TestEscapes(t *testing.T) {
	if got := EscapeSlack("A & B <c>"); got != "A &amp; B &lt;c&gt;" {
		t.Fatalf("EscapeSlack = %q", got)
	}
	if got := EscapeDiscord("[Breaking] *bold* _x_"); strings.ContainsAny(got, "[]*_") {
		t.Fatalf("EscapeDiscord left markup in %q", got)
	}
}
