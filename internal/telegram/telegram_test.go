package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fake(t *testing.T, status int, reply string) (Bot, *http.Request, *map[string]any) {
	t.Helper()
	var got http.Request
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = *r.Clone(context.Background())
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)

	return Bot{Token: "123:SECRET", BaseURL: srv.URL}, &got, &body
}

func TestPostSendsTheDocumentedRequest(t *testing.T) {
	b, got, body := fake(t, 200, `{"ok":true}`)
	if err := b.Post(context.Background(), "@postpaper_upsc", "<b>Hi</b>"); err != nil {
		t.Fatal(err)
	}
	if got.Method != "POST" || got.URL.Path != "/bot123:SECRET/sendMessage" {
		t.Fatalf("request = %s %s", got.Method, got.URL.Path)
	}
	m := *body
	if m["chat_id"] != "@postpaper_upsc" || m["text"] != "<b>Hi</b>" || m["parse_mode"] != "HTML" || m["disable_web_page_preview"] != true {
		t.Fatalf("body = %v", m)
	}
}

func TestPostErrorsNeverContainTheToken(t *testing.T) {
	b, _, _ := fake(t, 400, `{"ok":false,"description":"Bad Request: chat not found"}`)
	err := b.Post(context.Background(), "@nope", "x")
	if err == nil || !strings.Contains(err.Error(), "chat not found") || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("err = %v", err)
	}

	b, _, _ = fake(t, 429, `{"ok":false,"description":"Too Many Requests: retry after 5"}`)
	if err := b.Post(context.Background(), "@x", "x"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("429 = %v, want ErrRateLimited", err)
	}

	// A dead server: the transport error quotes the URL, so it must be replaced.
	b.BaseURL = "http://127.0.0.1:1"
	if err := b.Post(context.Background(), "@x", "x"); err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("connect error = %v", err)
	}

	if err := b.Post(context.Background(), "@x", strings.Repeat("a", MaxMessage+1)); err == nil {
		t.Fatal("an over-long message was accepted")
	}
}
