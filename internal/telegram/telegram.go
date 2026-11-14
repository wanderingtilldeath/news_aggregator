// Package telegram posts messages to a Telegram channel with the Bot API. It is
// free and needs a bot token from @BotFather plus a channel the bot administers.
package telegram

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

// MaxMessage is Telegram's limit for one message's text.
const MaxMessage = 4096

// ErrRateLimited means Telegram asked us to slow down (HTTP 429).
var ErrRateLimited = errors.New("telegram: rate limited")

// Poster sends an HTML-formatted message to a chat (a channel such as
// "@postpaper_upsc", or a numeric chat id).
type Poster interface {
	Post(ctx context.Context, chat, html string) error
}

// Bot posts through the Bot API.
type Bot struct {
	Token string
	// BaseURL defaults to https://api.telegram.org; tests point it at a fake.
	BaseURL string
	Client  *http.Client
}

func (b Bot) Post(ctx context.Context, chat, html string) error {
	if len(html) > MaxMessage {
		return fmt.Errorf("telegram: message is %d bytes, the limit is %d", len(html), MaxMessage)
	}
	body, err := json.Marshal(map[string]any{
		"chat_id":                  chat,
		"text":                     html,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	})
	if err != nil {
		return err
	}

	base := b.BaseURL
	if base == "" {
		base = "https://api.telegram.org"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/bot"+b.Token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		// The URL holds the token, so the underlying error is not passed on.
		return errors.New("telegram: could not build the request")
	}
	req.Header.Set("Content-Type", "application/json")

	client := b.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		// net/http errors quote the URL, which contains the token: say only that it failed.
		return errors.New("telegram: request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}
	detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	var e struct {
		Description string `json:"description"`
	}
	_ = json.Unmarshal(detail, &e)
	msg := strings.TrimSpace(e.Description)
	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("%w: %s", ErrRateLimited, msg)
	}

	return fmt.Errorf("telegram: status %d: %s", resp.StatusCode, msg)
}

// Log prints messages instead of sending them (dry runs).
type Log struct{ Logger *slog.Logger }

func (l Log) Post(_ context.Context, chat, html string) error {
	logger := l.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("telegram post (not sent)", "chat", chat, "text", html)

	return nil
}
