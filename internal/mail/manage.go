package mail

import (
	"context"
	"html/template"
	"log/slog"
	"strings"
	"time"
)

var manageHTML = template.Must(template.New("manage").Parse(`<!doctype html>
<html><body style="font-family:system-ui,sans-serif;max-width:480px;margin:0 auto;padding:16px;color:#111">
<h2 style="font-family:Georgia,serif">Manage your Postpaper subscription</h2>
<p>Use this link to change how long your morning briefing is, pick topics to follow, or unsubscribe.</p>
<p><a href="{{.Link}}" style="display:inline-block;background:#c8102e;color:#fff;padding:10px 16px;border-radius:4px;text-decoration:none">Manage my subscription</a></p>
<p style="color:#5f6670;font-size:13px">If the button does not work, open this address:<br>{{.Link}}</p>
<p style="color:#5f6670;font-size:13px">If you did not ask for this, ignore this email. Anyone with this link can change your subscription, so please do not forward it.</p>
</body></html>`))

// ManageLinkMessage is the email that lets a subscriber manage their subscription.
func ManageLinkMessage(to, webBase, token string) Message {
	link := tokenLink(webBase, "/manage", token)

	var html strings.Builder
	_ = manageHTML.Execute(&html, struct{ Link string }{link})

	return Message{
		To:      to,
		Subject: "Manage your Postpaper subscription",
		Headers: map[string]string{"List-Unsubscribe": "<" + tokenLink(webBase, "/unsubscribe", token) + ">"},
		HTML:    html.String(),
		Text: "Manage your Postpaper subscription.\n\n" +
			"Change how long your briefing is, pick topics, or unsubscribe:\n" + link + "\n\n" +
			"If you did not ask for this, ignore this email. Anyone with this link can change your subscription, so please do not forward it.\n",
	}
}

// ManageLinkNotifier returns the function the API calls to send that email. It sends in the background, so a slow
// provider never holds up the response; a failure is logged and the reader asks again.
func ManageLinkNotifier(s Sender, webBase string, logger *slog.Logger) func(ctx context.Context, email, token string) {
	return func(_ context.Context, email, token string) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := s.Send(ctx, ManageLinkMessage(email, webBase, token)); err != nil {
				logger.Error("manage link mail failed", "err", err)
			}
		}()
	}
}
