package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testKey = "xkeysib-SECRET-KEY"

func fakeBrevo(t *testing.T, status int, reply string) (Brevo, *http.Request, *map[string]any) {
	t.Helper()

	var got http.Request
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = *r.Clone(context.Background())
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)

	return Brevo{APIKey: testKey, FromEmail: "digest@example.com", FromName: "Postpaper", BaseURL: srv.URL}, &got, &body
}

func TestBrevoSendsTheDocumentedRequest(t *testing.T) {
	b, got, body := fakeBrevo(t, 201, `{"messageId":"<1@x>"}`)

	err := b.Send(context.Background(), Message{
		To: "reader@example.com", Subject: "Hi", HTML: "<p>hi</p>", Text: "hi",
		Headers: map[string]string{"List-Unsubscribe": "<https://x.test/u>"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got.Method != "POST" || got.URL.Path != "/v3/smtp/email" {
		t.Fatalf("request = %s %s", got.Method, got.URL.Path)
	}
	if got.Header.Get("api-key") != testKey || got.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("headers = %v", got.Header)
	}
	b2 := *body
	if b2["subject"] != "Hi" || b2["htmlContent"] != "<p>hi</p>" || b2["textContent"] != "hi" {
		t.Fatalf("body = %v", b2)
	}
	if s := b2["sender"].(map[string]any); s["email"] != "digest@example.com" || s["name"] != "Postpaper" {
		t.Fatalf("sender = %v", s)
	}
	if to := b2["to"].([]any)[0].(map[string]any); to["email"] != "reader@example.com" {
		t.Fatalf("to = %v", to)
	}
	if h := b2["headers"].(map[string]any); h["List-Unsubscribe"] != "<https://x.test/u>" {
		t.Fatalf("headers = %v", h)
	}
}

func TestBrevoErrorsNameTheStatusAndNeverTheKey(t *testing.T) {
	b, _, _ := fakeBrevo(t, 401, `{"code":"unauthorized","message":"Key not found"}`)
	err := b.Send(context.Background(), Message{To: "a@x.test", Subject: "s", Text: "t"})
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "Key not found") {
		t.Fatalf("err = %v, want status and provider message", err)
	}
	if errors.Is(err, ErrRateLimited) || strings.Contains(err.Error(), testKey) {
		t.Fatalf("err = %v: wrong class or leaks the key", err)
	}

	b, _, _ = fakeBrevo(t, 429, `{"message":"daily limit"}`)
	if err := b.Send(context.Background(), Message{To: "a@x.test"}); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("429 = %v, want ErrRateLimited", err)
	}

	// A dead server: the transport error must not carry the key either.
	b.BaseURL = "http://127.0.0.1:1"
	if err := b.Send(context.Background(), Message{To: "a@x.test"}); err == nil || strings.Contains(err.Error(), testKey) {
		t.Fatalf("connect error = %v", err)
	}
}

func TestConfirmationMessageHasTheLink(t *testing.T) {
	m := ConfirmationMessage("reader@example.com", "https://digest.example.com/", "tok-123_abc")

	want := "https://digest.example.com/confirm?token=tok-123_abc"
	if m.To != "reader@example.com" || !strings.Contains(m.Text, want) || !strings.Contains(m.HTML, want) {
		t.Fatalf("message = %+v, want link %s in both parts", m, want)
	}

	// A token is never trusted into markup or a query.
	m = ConfirmationMessage("a@x.test", "https://d.test", `a"><script>&x=1`)
	if strings.Contains(m.HTML, "<script>") || strings.Contains(m.Text, "&x=1") {
		t.Fatalf("token not escaped: %q / %q", m.HTML, m.Text)
	}
}

func TestLogSenderSendsNothing(t *testing.T) {
	var out bytes.Buffer
	l := Log{Logger: slog.New(slog.NewTextHandler(&out, nil))}
	if err := l.Send(context.Background(), Message{To: "a@x.test", Subject: "Hello", Text: "body"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "a@x.test") || !strings.Contains(out.String(), "Hello") {
		t.Fatalf("log = %q", out.String())
	}
}

type recorder struct{ got chan Message }

func (r recorder) Send(_ context.Context, m Message) error { r.got <- m; return nil }

func TestConfirmationNotifierSendsInTheBackground(t *testing.T) {
	r := recorder{got: make(chan Message, 1)}
	notify := ConfirmationNotifier(r, "https://d.test", slog.New(slog.NewTextHandler(io.Discard, nil)))

	notify(context.Background(), "reader@example.com", "tok")

	m := <-r.got
	if m.To != "reader@example.com" || !strings.Contains(m.Text, "/confirm?token=tok") {
		t.Fatalf("message = %+v", m)
	}
}

func TestManageLinkMessageLinksToTheManagePage(t *testing.T) {
	m := ManageLinkMessage("reader@example.com", "https://site.test/", "a+b/c")
	link := "https://site.test/manage?token=a%2Bb%2Fc"
	if m.To != "reader@example.com" || !strings.Contains(m.Text, link) || !strings.Contains(m.HTML, link) {
		t.Fatalf("message = %+v, want the manage link %s", m, link)
	}
}
