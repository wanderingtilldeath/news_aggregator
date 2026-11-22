package digestmail

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/telegram"
	"github.com/wanderingtilldeath/news-aggregator/internal/track"
)

// ChannelOptions control posting the day's edition to Telegram channels.
type ChannelOptions struct {
	// Day is the edition day, "2006-01-02".
	Day string
	// Chats maps a track to its channel (for example "@postpaper_upsc").
	Chats map[string]string
	// Window, Top and MinArticles work as in Options; a channel post is a one-minute
	// read, so at most 5 stories.
	Window      time.Duration
	Top         int
	MinArticles int
	WebBase     string
	// DryRun posts through the poster (use the log poster) and records nothing.
	DryRun bool
	Now    func() time.Time
}

// RunChannels posts each track's edition to its channel, once per channel per day.
// It returns how many posts it made. A rate limit from Telegram ends the run with
// the posts so far; other failures are logged and the next channel is tried. When every post tried failed,
// it returns an error so the job turns red.
func RunChannels(ctx context.Context, st store.Store, p telegram.Poster, o ChannelOptions, logger *slog.Logger) (int, error) {
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	b := &builder{st: st, since: now().Add(-o.Window), top: min(o.Top, briefMax), cache: map[string][]entry{}, topics: map[string][]store.Item{}}

	posted, failed := 0, 0
	for _, tr := range track.All() {
		chat := o.Chats[tr]
		if chat == "" {
			continue
		}
		channel := "telegram:" + chat
		if !o.DryRun {
			done, err := st.ChannelPosted(ctx, o.Day, channel)
			if err != nil {
				return posted, err
			}
			if done {
				continue
			}
		}

		ed, err := b.edition(ctx, []string{tr}, track.Mode1, o.Day, nil)
		if err != nil {
			return posted, err
		}
		if ed.count() < o.MinArticles {
			logger.Info("channel post skipped: too few stories", "track", tr, "stories", ed.count())
			continue
		}

		err = p.Post(ctx, chat, telegramText(ed.Sections[0], o.Day, o.WebBase))
		switch {
		case errors.Is(err, telegram.ErrRateLimited):
			logger.Warn("telegram rate limited; stopping until the next run", "err", err)
			return posted, nil
		case err != nil:
			failed++
			logger.Error("telegram post failed", "track", tr, "err", err)
			continue
		}
		posted++
		if !o.DryRun {
			if err := st.MarkChannelPosted(ctx, o.Day, channel); err != nil {
				logger.Error("could not record the channel post", "track", tr, "err", err)
			}
		}
	}

	if failed > 0 && posted == 0 {
		return posted, fmt.Errorf("every telegram post failed (%d)", failed)
	}

	return posted, nil
}

// telegramText formats a section as a Telegram HTML message under the length limit.
// Feed text is escaped, and only http(s) links reach an href (pick already ensured it).
func telegramText(sec section, day, web string) string {
	date := day
	if t, err := time.Parse("2006-01-02", day); err == nil {
		date = t.Format("Mon 2 Jan")
	}

	var head strings.Builder
	head.WriteString("<b>Postpaper · " + html.EscapeString(sec.Title) + "</b> · " + html.EscapeString(date) + "\n")

	tail := ""
	if sec.Quiz != nil {
		link := strings.TrimRight(web, "/") + "/quiz?track=" + url.QueryEscape(sec.Track) + "&day=" + url.QueryEscape(day)
		tail = fmt.Sprintf("\nToday's quiz (%d questions): <a href=\"%s\">take it</a>", len(sec.Quiz.Questions), html.EscapeString(link))
	}

	out := head.String()
	for i, e := range sec.Entries {
		var item strings.Builder
		fmt.Fprintf(&item, "\n%d. <a href=\"%s\">%s</a>\n", i+1, html.EscapeString(storyLink(web, e)), html.EscapeString(e.Title))
		switch {
		case e.Why != "":
			item.WriteString("<i>" + html.EscapeString(e.Why) + "</i>\n")
		case len(e.Gist) > 0:
			item.WriteString("<i>" + html.EscapeString(e.Gist[0]) + "</i>\n")
		}
		// Stop adding stories when the next one would not fit.
		if len(out)+item.Len()+len(tail) > telegram.MaxMessage-100 {
			break
		}
		out += item.String()
	}

	return out + tail
}
