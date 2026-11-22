package digestmail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/chat"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/track"
)

// TeamOptions control posting the day's edition to the teams' Slack and Discord channels.
type TeamOptions struct {
	// Day is the edition day, "2006-01-02".
	Day string
	// Window, Top and MinArticles work as in ChannelOptions; a team post is a short read, at most 5 stories.
	Window      time.Duration
	Top         int
	MinArticles int
	WebBase     string
	// DryRun posts through the poster (use chat.Log) and records and removes nothing.
	DryRun bool
	Now    func() time.Time
	// Pause is the wait between two posts, so a run does not hit Slack's or Discord's rate limits.
	Pause time.Duration
}

// maxRefusals is how many days in a row a service may refuse a channel's post before it is disconnected.
const maxRefusals = 3

// RunTeams posts each connected channel's edition to it, once per channel per day. It returns how
// many posts it made. A channel whose webhook was deleted is disconnected, and so is one whose posts
// were refused (400, 401, 403) maxRefusals runs in a row. A rate limit ends the run, to be retried
// tomorrow; any other failure is logged and the next channel is tried. When every post tried failed, the
// run returns an error so the job turns red.
func RunTeams(ctx context.Context, st store.Store, p chat.Poster, o TeamOptions, logger *slog.Logger) (int, error) {
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	hooks, err := st.TeamHooks(ctx)
	if err != nil {
		return 0, err
	}
	b := &builder{st: st, since: now().Add(-o.Window), top: min(o.Top, briefMax), cache: map[string][]entry{}, topics: map[string][]store.Item{}}

	posted, failed := 0, 0
	for i, h := range hooks {
		channel := "team:" + strconv.FormatInt(h.ID, 10)
		if !o.DryRun {
			done, err := st.ChannelPosted(ctx, o.Day, channel)
			if err != nil {
				return posted, err
			}
			if done {
				continue
			}
		}

		ed, err := b.edition(ctx, []string{h.Track}, track.Mode1, o.Day, nil)
		if err != nil {
			return posted, err
		}
		if ed.count() < o.MinArticles {
			logger.Info("team post skipped: too few stories", "track", h.Track, "stories", ed.count())
			continue
		}

		if i > 0 && o.Pause > 0 {
			select {
			case <-ctx.Done():
				return posted, ctx.Err()
			case <-time.After(o.Pause):
			}
		}
		err = p.Post(ctx, h.Kind, h.URL, teamText(h.Kind, ed.Sections[0], o.Day, o.WebBase, h.Token))
		switch {
		case errors.Is(err, chat.ErrRateLimited):
			logger.Warn("team post rate limited; stopping until the next run", "err", err)
			return posted, nil
		case errors.Is(err, chat.ErrRejected):
			failed++
			logger.Warn("team post refused", "team", h.ID, "kind", h.Kind)
			if o.DryRun {
				continue
			}
			fails, rerr := st.RecordTeamPost(ctx, h.ID, false)
			if rerr != nil {
				logger.Error("could not record the refusal", "team", h.ID, "err", rerr)
				continue
			}
			if fails >= maxRefusals {
				logger.Info("team webhook keeps refusing; disconnecting it", "team", h.ID, "kind", h.Kind, "refusals", fails)
				if _, rerr := st.RemoveTeamHook(ctx, h.Token); rerr != nil {
					logger.Error("could not disconnect the team", "team", h.ID, "err", rerr)
				}
			}
			continue
		case errors.Is(err, chat.ErrGone):
			logger.Info("team webhook is gone; disconnecting it", "team", h.ID, "kind", h.Kind)
			if !o.DryRun {
				if _, rerr := st.RemoveTeamHook(ctx, h.Token); rerr != nil {
					logger.Error("could not disconnect the team", "team", h.ID, "err", rerr)
				}
			}
			continue
		case err != nil:
			failed++
			logger.Warn("team post failed", "team", h.ID, "kind", h.Kind, "err", err)
			continue
		}
		posted++
		if !o.DryRun {
			if _, err := st.RecordTeamPost(ctx, h.ID, true); err != nil {
				logger.Error("could not record the team post", "team", h.ID, "err", err)
			}
			if err := st.MarkChannelPosted(ctx, o.Day, channel); err != nil {
				logger.Error("could not record the team post", "team", h.ID, "err", err)
			}
		}
	}

	if failed > 0 && posted == 0 {
		return posted, fmt.Errorf("every team post failed (%d)", failed)
	}

	return posted, nil
}

// teamText formats a section for a Slack or Discord channel under the length limit. Feed text is
// escaped for the service, and only http(s) links reach a link (pick already ensured it).
func teamText(kind string, sec section, day, web, token string) string {
	date := day
	if t, err := time.Parse("2006-01-02", day); err == nil {
		date = t.Format("Mon 2 Jan")
	}
	stop := strings.TrimRight(web, "/") + "/teams/stop?token=" + url.QueryEscape(token)

	slack := kind == chat.Slack
	bold := func(s string) string {
		if slack {
			return "*" + s + "*"
		}
		return "**" + s + "**"
	}
	italic := func(s string) string { return "_" + s + "_" }
	if !slack {
		italic = func(s string) string { return "*" + s + "*" }
	}
	esc, link := chat.EscapeSlack, func(u, title string) string { return "<" + u + "|" + title + ">" }
	if !slack {
		esc, link = chat.EscapeDiscord, func(u, title string) string { return "[" + title + "](<" + u + ">)" }
	}

	head := bold("Postpaper · "+esc(sec.Title)) + " · " + esc(date) + "\n"
	tail := "\nStop these posts: " + stop
	out := head
	for i, e := range sec.Entries {
		var item strings.Builder
		fmt.Fprintf(&item, "\n%d. %s\n", i+1, link(storyLink(web, e), esc(e.Title)))
		switch {
		case e.Why != "":
			item.WriteString(italic(esc(e.Why)) + "\n")
		case len(e.Gist) > 0:
			item.WriteString(italic(esc(e.Gist[0])) + "\n")
		}
		// Stop adding stories when the next one would not fit.
		if len(out)+item.Len()+len(tail) > chat.MaxMessage {
			break
		}
		out += item.String()
	}

	return out + tail
}
