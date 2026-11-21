package mail

import (
	"context"
	"html/template"
	"log/slog"
	"strings"
	"time"
)

var adminHTML = template.Must(template.New("admin").Parse(`<!doctype html>
<html><body style="font-family:system-ui,sans-serif;max-width:480px;margin:0 auto;padding:16px;color:#111">
<h2 style="font-family:Georgia,serif">Sign in to Postpaper</h2>
<p>Press the button to sign in as an admin. The link works once and expires in 15 minutes.</p>
<p><a href="{{.Link}}" style="display:inline-block;background:#c8102e;color:#fff;padding:10px 16px;border-radius:4px;text-decoration:none">Sign in</a></p>
<p style="color:#5f6670;font-size:13px">If the button does not work, open this address:<br>{{.Link}}</p>
<p style="color:#5f6670;font-size:13px">If you did not ask to sign in, ignore this email: nothing happens until the link is used.</p>
</body></html>`))

// AdminLoginMessage is the sign-in link email for an admin address.
func AdminLoginMessage(to, webBase, token string) Message {
	link := tokenLink(webBase, "/admin/verify", token)

	var html strings.Builder
	_ = adminHTML.Execute(&html, struct{ Link string }{link})

	return Message{
		To:      to,
		Subject: "Your Postpaper admin sign-in link",
		HTML:    html.String(),
		Text: "Sign in to Postpaper as an admin.\n\n" +
			"Open this address and press the button (it works once and expires in 15 minutes):\n" + link + "\n\n" +
			"If you did not ask to sign in, ignore this email.\n",
	}
}

// AdminLoginNotifier returns the function the API calls with a new sign-in token. It sends in the background,
// so a slow provider never holds up the response; a failure is logged and the admin asks again.
func AdminLoginNotifier(s Sender, webBase string, logger *slog.Logger) func(ctx context.Context, email, token string) {
	return func(_ context.Context, email, token string) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := s.Send(ctx, AdminLoginMessage(email, webBase, token)); err != nil {
				logger.Error("admin sign-in mail failed", "err", err)
			}
		}()
	}
}
