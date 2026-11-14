package mail

import (
	"context"
	"html/template"
	"log/slog"
	"net/url"
	"strings"
	"time"
)

var confirmHTML = template.Must(template.New("confirm").Parse(`<!doctype html>
<html><body style="font-family:system-ui,sans-serif;max-width:480px;margin:0 auto;padding:16px;color:#111">
<h2 style="font-family:Georgia,serif">Confirm your subscription</h2>
<p>Press the button to start receiving The Daily Digest, one short email a day.</p>
<p><a href="{{.Link}}" style="display:inline-block;background:#c8102e;color:#fff;padding:10px 16px;border-radius:4px;text-decoration:none">Confirm my email address</a></p>
<p style="color:#5f6670;font-size:13px">If the button does not work, open this address:<br>{{.Link}}</p>
<p style="color:#5f6670;font-size:13px">If you did not ask for this, ignore this email and nothing will be sent.</p>
</body></html>`))

// tokenLink is the web page at path with the token as its query.
func tokenLink(webBase, path, token string) string {
	return strings.TrimRight(webBase, "/") + path + "?token=" + url.QueryEscape(token)
}

// ConfirmationMessage is the double opt-in email for the address `to`.
func ConfirmationMessage(to, webBase, token string) Message {
	link := tokenLink(webBase, "/confirm", token)

	var html strings.Builder
	_ = confirmHTML.Execute(&html, struct{ Link string }{link})

	return Message{
		To:      to,
		Subject: "Confirm your subscription to The Daily Digest",
		HTML:    html.String(),
		Text: "Confirm your subscription to The Daily Digest.\n\n" +
			"Open this address and press the button:\n" + link + "\n\n" +
			"If you did not ask for this, ignore this email and nothing will be sent.\n",
	}
}

// ConfirmationNotifier returns the function the API calls when an address still
// has to confirm. It sends in the background, so a slow provider never holds up
// the signup response; a failure is logged and the reader can sign up again to
// get a new mail.
func ConfirmationNotifier(s Sender, webBase string, logger *slog.Logger) func(ctx context.Context, email, token string) {
	return func(_ context.Context, email, token string) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := s.Send(ctx, ConfirmationMessage(email, webBase, token)); err != nil {
				logger.Error("confirmation mail failed", "err", err)
			}
		}()
	}
}
