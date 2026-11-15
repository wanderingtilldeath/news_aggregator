package mail

import (
	"strings"
	"testing"
	"time"
)

func TestProWelcomeMessageHasThePrivateLinkAndTheEndDate(t *testing.T) {
	m := ProWelcomeMessage("a@x.test", "https://site.test/", "tok-en", time.Date(2026, 12, 16, 0, 0, 0, 0, time.UTC))
	for _, want := range []string{"https://site.test/pro/edition?token=tok-en", "16 December 2026"} {
		if !strings.Contains(m.Text, want) || !strings.Contains(m.HTML, want) {
			t.Fatalf("welcome mail lacks %q:\n%s", want, m.Text)
		}
	}
	if m.To != "a@x.test" || m.Subject == "" {
		t.Fatalf("message = %+v", m)
	}
}
