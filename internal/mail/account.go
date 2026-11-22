package mail

import (
	"context"
	"html"
	"log/slog"
	"strings"
	"time"
)

// Account mails: the link that confirms an address, and the link that sets a new password. Both go to an address a
// reader typed, so they say what was asked and that nothing happens if it was not them.

func accountMessage(to, subject, heading, intro, button, link, foot string) Message {
	body := `<!doctype html><html><body style="font-family:system-ui,sans-serif;max-width:480px;margin:0 auto;padding:16px;color:#111">` +
		`<h2 style="font-family:Georgia,serif">` + html.EscapeString(heading) + `</h2>` +
		`<p>` + html.EscapeString(intro) + `</p>` +
		`<p><a href="` + html.EscapeString(link) + `" style="display:inline-block;background:#c8213a;color:#fff;padding:10px 16px;border-radius:4px;text-decoration:none">` + html.EscapeString(button) + `</a></p>` +
		`<p style="color:#5f6670;font-size:13px">` + html.EscapeString(foot) + `<br>` + html.EscapeString(link) + `</p></body></html>`

	return Message{
		To:      to,
		Subject: subject,
		HTML:    body,
		Text:    heading + "\n\n" + intro + "\n\n" + link + "\n\n" + foot + "\n",
	}
}

// AccountVerifyMessage asks the owner of an address to confirm it for a Postpaper account.
func AccountVerifyMessage(to, webBase, token string) Message {
	return accountMessage(to, "Confirm your email for Postpaper", "Confirm your email",
		"Open this link to confirm this address for your Postpaper account. After that you can log in with it and reset your password by email. The link works once, for 24 hours.",
		"Confirm my email", tokenLink(webBase, "/verify-email", token),
		"If you did not add this address to a Postpaper account, ignore this mail: nothing changes.")
}

// AccountResetMessage carries the link that sets a new password.
func AccountResetMessage(to, webBase, token string) Message {
	return accountMessage(to, "Reset your Postpaper password", "Reset your password",
		"Open this link to choose a new password. It works once, for one hour, and logs the account out of every device.",
		"Choose a new password", tokenLink(webBase, "/reset", token),
		"If you did not ask for this, ignore this mail: your password stays as it is.")
}

// AccountNotifier returns the function the API calls to mail an account link: kind is "verify" or "reset". It sends in
// the background and logs a failure; the reader can ask again.
func AccountNotifier(s Sender, webBase string, logger *slog.Logger) func(ctx context.Context, kind, email, token string) {
	return func(_ context.Context, kind, email, token string) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			msg := AccountVerifyMessage(email, webBase, token)
			if strings.EqualFold(kind, "reset") {
				msg = AccountResetMessage(email, webBase, token)
			}
			if err := s.Send(ctx, msg); err != nil {
				logger.Error("account mail failed", "kind", kind, "err", err)
			}
		}()
	}
}
