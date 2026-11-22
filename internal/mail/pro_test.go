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

func TestProRenewedAndExpiringMessages(t *testing.T) {
	until := time.Date(2026, 12, 16, 0, 0, 0, 0, time.UTC)
	renewed := ProRenewedMessage("a@x.test", "https://site.test/", "tok-en", until)
	for _, want := range []string{"16 December 2026", "https://site.test/pro/edition?token=tok-en"} {
		if !strings.Contains(renewed.Text, want) || !strings.Contains(renewed.HTML, want) {
			t.Fatalf("renewal mail lacks %q:\n%s", want, renewed.Text)
		}
	}
	if strings.Contains(renewed.Subject, "Welcome") {
		t.Fatalf("a renewal must not read as a welcome: %q", renewed.Subject)
	}

	soon := ProExpiringMessage("a@x.test", "https://site.test/", until)
	if !strings.Contains(soon.Text, "16 December 2026") || !strings.Contains(soon.Text, "https://site.test/pro") || strings.Contains(soon.Text, "token=") {
		t.Fatalf("reminder = %s; want the end date and the renewal page, and no secret link", soon.Text)
	}
}

func TestAccountMessagesCarryTheirLink(t *testing.T) {
	v := AccountVerifyMessage("a@x.test", "https://site.test", "s3cret/+")
	r := AccountResetMessage("a@x.test", "https://site.test", "s3cret/+")
	for name, c := range map[string]struct {
		m    Message
		path string
	}{"verify": {v, "/verify-email?token=s3cret%2F%2B"}, "reset": {r, "/reset?token=s3cret%2F%2B"}} {
		if !strings.Contains(c.m.Text, "https://site.test"+c.path) || !strings.Contains(c.m.HTML, "https://site.test"+c.path) {
			t.Fatalf("%s mail lacks its link:\n%s", name, c.m.Text)
		}
	}
}
