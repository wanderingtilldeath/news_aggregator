package mail

import (
	"context"
	"html/template"
	"log/slog"
	"strings"
	"time"
)

var proHTML = template.Must(template.New("pro").Parse(`<!doctype html>
<html><body style="font-family:system-ui,sans-serif;max-width:480px;margin:0 auto;padding:16px;color:#111">
<h2 style="font-family:Georgia,serif">Welcome to Postpaper Pro</h2>
<p>Thank you. Your Pro membership runs until <b>{{.Until}}</b>.</p>
<ul>
<li>Your daily email has no sponsor line and is sent first.</li>
<li>A printable edition (save it as a PDF) is linked at the end of every email.</li>
</ul>
<p><a href="{{.Link}}" style="display:inline-block;background:#c8213a;color:#fff;padding:10px 16px;border-radius:4px;text-decoration:none">Open today&rsquo;s printable edition</a></p>
<p style="color:#5f6670;font-size:13px">This link is yours: anyone who has it can open the printable editions, so do not share it.<br>{{.Link}}</p>
<p style="color:#5f6670;font-size:13px">Pro is applied to the email address you paid with. To get the daily email, subscribe with that address on the site.</p>
</body></html>`))

// ProWelcomeMessage is the email a new Pro member gets, with their private link.
func ProWelcomeMessage(to, webBase, token string, until time.Time) Message {
	link := tokenLink(webBase, "/pro/edition", token)
	day := until.Format("2 January 2006")

	var html strings.Builder
	_ = proHTML.Execute(&html, struct{ Link, Until string }{link, day})

	return Message{
		To:      to,
		Subject: "Welcome to Postpaper Pro",
		HTML:    html.String(),
		Text: "Welcome to Postpaper Pro. Your membership runs until " + day + ".\n\n" +
			"Your daily email has no sponsor line and is sent first, and a printable edition is linked at the end of every email.\n\n" +
			"Your private link (do not share it):\n" + link + "\n\n" +
			"Pro is applied to the email address you paid with. To get the daily email, subscribe with that address on the site.\n",
	}
}

// ProRenewedMessage is the short mail a member gets when a payment extends a membership that already existed: the new
// end date, and nothing they have already read in the welcome mail.
func ProRenewedMessage(to, webBase, token string, until time.Time) Message {
	link := tokenLink(webBase, "/pro/edition", token)
	day := until.Format("2 January 2006")

	return Message{
		To:      to,
		Subject: "Your Postpaper Pro membership is renewed",
		HTML: `<!doctype html><html><body style="font-family:system-ui,sans-serif;max-width:480px;margin:0 auto;padding:16px;color:#111">` +
			`<h2 style="font-family:Georgia,serif">Thank you for renewing</h2>` +
			`<p>Your Pro membership now runs until <b>` + day + `</b>. Your private link has not changed.</p>` +
			`<p><a href="` + link + `" style="display:inline-block;background:#c8213a;color:#fff;padding:10px 16px;border-radius:4px;text-decoration:none">Open today&rsquo;s printable edition</a></p>` +
			`</body></html>`,
		Text: "Thank you for renewing. Your Postpaper Pro membership now runs until " + day + ". Your private link has not changed:\n" + link + "\n",
	}
}

// ProExpiringMessage reminds a member that the paid period is about to end, with the way to renew.
func ProExpiringMessage(to, webBase string, until time.Time) Message {
	day := until.Format("2 January 2006")
	renew := strings.TrimRight(webBase, "/") + "/pro"

	return Message{
		To:      to,
		Subject: "Your Postpaper Pro membership ends on " + day,
		HTML: `<!doctype html><html><body style="font-family:system-ui,sans-serif;max-width:480px;margin:0 auto;padding:16px;color:#111">` +
			`<h2 style="font-family:Georgia,serif">Your Pro membership ends soon</h2>` +
			`<p>Your Postpaper Pro membership ends on <b>` + day + `</b>. After that the daily email carries the sponsor line again and the printable editions stop. Your saved notes stay yours.</p>` +
			`<p><a href="` + renew + `" style="display:inline-block;background:#c8213a;color:#fff;padding:10px 16px;border-radius:4px;text-decoration:none">Renew Pro</a></p>` +
			`<p style="color:#5f6670;font-size:13px">A renewal adds to the days you have left, so nothing is lost by renewing early. This is the only reminder we send for this period.</p>` +
			`</body></html>`,
		Text: "Your Postpaper Pro membership ends on " + day + ". After that the daily email carries the sponsor line again and the printable editions stop. Your saved notes stay yours.\n\n" +
			"Renew: " + renew + "\nA renewal adds to the days you have left, so nothing is lost by renewing early. This is the only reminder we send for this period.\n",
	}
}

// ProNotifier returns the function the API calls for a new payment: the welcome mail, or the short renewal mail when the
// member already existed. Like the confirmation mail it
// sends in the background and logs a failure; the member can still use the link from the daily email.
func ProNotifier(s Sender, webBase string, logger *slog.Logger) func(ctx context.Context, email, token string, until time.Time, renewed bool) {
	return func(_ context.Context, email, token string, until time.Time, renewed bool) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			msg := ProWelcomeMessage(email, webBase, token, until)
			if renewed {
				msg = ProRenewedMessage(email, webBase, token, until)
			}
			if err := s.Send(ctx, msg); err != nil {
				logger.Error("pro mail failed", "err", err, "renewed", renewed)
			}
		}()
	}
}
