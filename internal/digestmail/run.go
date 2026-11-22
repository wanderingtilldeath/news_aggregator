package digestmail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/mail"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/track"
)

// maxConsecutiveFailures stops a run when the provider keeps failing.
const maxConsecutiveFailures = 5

// weeklyWindow is how far back the weekly edition counts articles.
const weeklyWindow = 7 * 24 * time.Hour

// Options control one run.
type Options struct {
	// Day is the edition day, "2006-01-02", in the product's time zone; a
	// subscriber gets one edition per day.
	Day string
	// Window is how far back articles count; Top how many stories each track
	// gets in the Telegram and Teams posts (the email's size is fixed in render.go);
	// MinArticles how few stories in total make an edition not worth sending.
	Window      time.Duration
	Top         int
	MinArticles int
	// MaxSends caps mails per edition day (those already sent today count), to stay under the provider's
	// daily quota. Those not reached get the edition on the next run of the day.
	MaxSends int
	// Pause is the wait between mails.
	Pause time.Duration
	// WebBase and APIBase are the public addresses of the web frontend and the
	// API; Footer is an extra line for every mail.
	WebBase, APIBase, Footer string
	// SponsorText and SponsorURL are one labelled sponsor line for every mail; it is left out unless
	// both are set and the link is http(s).
	SponsorText, SponsorURL string
	// DryRun sends one sample mail through sender (use the log sender) and
	// touches no subscriber or delivery record.
	DryRun bool
	// To sends one sample edition to this address and records nothing; its
	// unsubscribe link is a placeholder.
	To  string
	Now func() time.Time
}

// Result says what a run did.
type Result struct {
	Skipped     string
	Articles    int
	Eligible    int
	AlreadySent int
	Sent        int
	Failed      int
	// NoEdition counts subscribers skipped because the day had too few stories.
	NoEdition int
	// Capped counts subscribers left for the next run by MaxSends or a rate limit.
	Capped int
	// Unrecorded counts mails sent whose delivery record failed to save: those
	// subscribers may get the edition twice.
	Unrecorded int
}

// Run mails today's edition to every confirmed subscriber who has not had it.
func Run(ctx context.Context, st store.Store, sender mail.Sender, o Options, logger *slog.Logger) (Result, error) {
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}

	b := &builder{st: st, since: now().Add(-o.Window), top: o.Top, cache: map[string][]entry{}, topics: map[string][]store.Item{}}
	// The weekly edition looks back over the whole week instead of the last day.
	wb := &builder{st: st, since: now().Add(-weeklyWindow), top: o.Top, cache: map[string][]entry{}, topics: map[string][]store.Item{}}
	res := Result{}

	if o.To != "" || o.DryRun {
		// One sample in the longer read, the richest version.
		ed, err := b.general(ctx, track.Mode7, o.Day, nil)
		if err != nil {
			return res, err
		}
		res.Articles = ed.count()
		if res.Articles < o.MinArticles {
			res.Skipped = fmt.Sprintf("only %d articles in the last %s, need %d", res.Articles, o.Window, o.MinArticles)
			return res, nil
		}
		to := o.To
		if to == "" {
			to = "sample@example.com"
		}
		if err := sender.Send(ctx, render(ed, to, "SAMPLE-TOKEN", track.Daily, o.WebBase, o.APIBase, o.Footer, sponsorOf(o), "")); err != nil {
			return res, fmt.Errorf("digestmail: sample: %w", err)
		}
		res.Sent = 1
		if o.DryRun {
			subs, err := st.ConfirmedSubscribers(ctx)
			if err != nil {
				return res, err
			}
			res.Sent, res.Eligible = 0, len(subs)
		}

		return res, nil
	}

	subs, err := st.ConfirmedSubscribers(ctx)
	if err != nil {
		return res, fmt.Errorf("digestmail: load subscribers: %w", err)
	}
	delivered, err := st.DeliveredOn(ctx, o.Day)
	if err != nil {
		return res, fmt.Errorf("digestmail: load deliveries: %w", err)
	}

	// Pro members are mailed first, so a send cap or a rate limit never leaves one of them for the
	// next run, and their mail has no sponsor line and links the printable editions.
	pro, err := st.ActiveProTokens(ctx)
	if err != nil {
		return res, fmt.Errorf("digestmail: load pro members: %w", err)
	}
	sort.SliceStable(subs, func(i, j int) bool {
		return pro[strings.ToLower(subs[i].Email)] != "" && pro[strings.ToLower(subs[j].Email)] == ""
	})

	// A weekly subscriber is mailed only on the weekly day.
	if weekday, err := time.Parse("2006-01-02", o.Day); err == nil && weekday.Weekday() != track.WeeklyDay {
		due := subs[:0]
		for _, sub := range subs {
			if sub.Frequency != track.Weekly {
				due = append(due, sub)
			}
		}
		subs = due
	}

	res.Eligible = len(subs)
	failures := 0
	for i, sub := range subs {
		if delivered[sub.ID] {
			res.AlreadySent++
			continue
		}
		// The cap is per day, not per run: the workflow runs four times a day and the provider's quota is daily.
		if o.MaxSends > 0 && len(delivered)+res.Sent >= o.MaxSends {
			res.Capped = countUnsent(subs[i:], delivered)
			logger.Warn("send cap reached; the rest get the edition on the next run", "cap", o.MaxSends, "left", res.Capped)
			break
		}

		// Everyone gets the same top-news edition in the reading length they chose, plus
		// their own topics; the edition of a reading length is built once and reused.
		eb := b
		if sub.Frequency == track.Weekly {
			eb = wb
		}
		ed, err := eb.general(ctx, sub.Mode, o.Day, sub.Topics)
		if err != nil {
			return res, err
		}
		res.Articles = max(res.Articles, ed.count())
		if ed.count() < o.MinArticles {
			res.NoEdition++
			continue
		}

		proToken := pro[strings.ToLower(sub.Email)]
		err = sender.Send(ctx, render(ed, sub.Email, sub.Token, sub.Frequency, o.WebBase, o.APIBase, o.Footer, sponsorFor(o, proToken), proToken))
		switch {
		case errors.Is(err, mail.ErrRefused):
			// A bad key or an unpaid plan: nothing more can go out, and the run must fail so someone looks.
			res.Failed++
			return res, fmt.Errorf("digestmail: the mail provider refused the account, stopping: %w", err)
		case errors.Is(err, mail.ErrRateLimited):
			res.Capped = countUnsent(subs[i:], delivered)
			logger.Warn("provider rate limit reached; stopping until the next run", "left", res.Capped, "err", err)
			return res, nil
		case err != nil:
			res.Failed++
			failures++
			logger.Error("send failed", "subscriber", sub.ID, "err", err)
			if failures >= maxConsecutiveFailures {
				return res, fmt.Errorf("digestmail: %d sends in a row failed, stopping: %w", failures, err)
			}
			continue
		}
		failures = 0
		res.Sent++

		// shortcut: the mail is recorded after it is accepted, so a crash in between
		// can send one subscriber the edition twice; recording first would risk losing one.
		if err := st.MarkDelivered(ctx, sub.ID, o.Day); err != nil {
			res.Unrecorded++
			logger.Error("could not record delivery", "subscriber", sub.ID, "err", err)
			// Stop: sending on would mail the rest without a record, and the next run mails them again.
			return res, fmt.Errorf("digestmail: could not record a delivery, stopping so nobody is mailed twice: %w", err)
		}

		select {
		case <-ctx.Done():
			return res, ctx.Err()
		case <-time.After(o.Pause):
		}
	}

	if res.Sent == 0 && res.NoEdition > 0 && res.Failed == 0 {
		res.Skipped = fmt.Sprintf("only %d articles in the last %s, need %d", res.Articles, o.Window, o.MinArticles)
	}

	return res, nil
}

func countUnsent(subs []store.Subscriber, delivered map[int64]bool) int {
	n := 0
	for _, s := range subs {
		if !delivered[s.ID] {
			n++
		}
	}

	return n
}

// sponsorFor is the sponsor line of a mail: none for a Pro member.
func sponsorFor(o Options, proToken string) sponsorLine {
	if proToken != "" {
		return sponsorLine{}
	}

	return sponsorOf(o)
}
