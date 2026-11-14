// Package mail sends the product's email: the signup confirmation and, later,
// the daily digest. A Sender is either the Brevo HTTP API or a logger.
//
// Brevo is called over HTTPS, not SMTP, because free Render web services
// block outbound SMTP ports (25, 465, 587).
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Message is one email to one recipient.
type Message struct {
	To      string
	Subject string
	HTML    string
	Text    string
	// Headers are extra headers such as List-Unsubscribe.
	Headers map[string]string
}

// Sender delivers a Message.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// ErrRateLimited means the provider refused the call because a quota is used up;
// the caller should stop sending and try again later.
var ErrRateLimited = errors.New("mail: provider rate limit reached")

// Log writes each message to the logger instead of sending it: the default
// provider, and what -dry-run uses. The text part holds links with secret tokens,
// so use it for development only.
type Log struct{ Logger *slog.Logger }

func (l Log) Send(_ context.Context, m Message) error {
	logger := l.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("mail (log provider, not sent)", "to", m.To, "subject", m.Subject, "text", m.Text)

	return nil
}

// Brevo sends through the Brevo transactional API. The free plan allows 300
// emails a day.
type Brevo struct {
	APIKey    string
	FromEmail string
	FromName  string
	// BaseURL defaults to https://api.brevo.com; tests point it at a fake.
	BaseURL string
	Client  *http.Client
}

const brevoBase = "https://api.brevo.com"

type brevoAddress struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

type brevoRequest struct {
	Sender      brevoAddress      `json:"sender"`
	To          []brevoAddress    `json:"to"`
	Subject     string            `json:"subject"`
	HTMLContent string            `json:"htmlContent,omitempty"`
	TextContent string            `json:"textContent,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

func (b Brevo) Send(ctx context.Context, m Message) error {
	body, err := json.Marshal(brevoRequest{
		Sender:      brevoAddress{Name: b.FromName, Email: b.FromEmail},
		To:          []brevoAddress{{Email: m.To}},
		Subject:     m.Subject,
		HTMLContent: m.HTML,
		TextContent: m.Text,
		Headers:     m.Headers,
	})
	if err != nil {
		return fmt.Errorf("mail: encode: %w", err)
	}

	base := b.BaseURL
	if base == "" {
		base = brevoBase
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v3/smtp/email", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("mail: request: %w", err)
	}
	req.Header.Set("api-key", b.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := b.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		// The error names the URL and the cause, never the headers, so the key stays out of logs.
		return fmt.Errorf("mail: brevo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 == 2 {
		return nil
	}
	detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("%w: %s", ErrRateLimited, strings.TrimSpace(string(detail)))
	}

	return fmt.Errorf("mail: brevo returned %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
}

// New picks the sender by provider name: "brevo" calls the Brevo API, anything
// else logs and sends nothing.
func New(provider, brevoKey, fromEmail, fromName string, logger *slog.Logger) Sender {
	if provider == "brevo" {
		return Brevo{APIKey: brevoKey, FromEmail: fromEmail, FromName: fromName}
	}

	return Log{Logger: logger}
}
