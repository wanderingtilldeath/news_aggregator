package digestmail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/mail"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

// maxConsecutiveFailures stops a run when the provider keeps failing.
const maxConsecutiveFailures = 5

// Options control one run.
type Options struct {
	// Day is the edition day, "2006-01-02", in the product's time zone; a
	// subscriber gets one edition per day.
	Day string
	// Window is how far back articles count; Top how many go in the mail and
	// MinArticles how few make it not worth sending.
	Window      time.Duration
	Top         int
	MinArticles int
	// MaxSends caps mails per run, to stay under the provider's daily quota.
	// Those not reached get the edition on the next run of the day.
	MaxSends int
	// Pause is the wait between mails.
	Pause time.Duration
	// WebBase and APIBase are the public addresses of the web frontend and the
	// API; Footer is an extra line for every mail.
	WebBase, APIBase, Footer string
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

	items, _, err := st.Digest(ctx, store.DigestQuery{Since: now().Add(-o.Window), ArticlesOnly: true, Limit: 100})
	if err != nil {
		return Result{}, fmt.Errorf("digestmail: load articles: %w", err)
	}
	entries := pick(items, o.Top)
	res := Result{Articles: len(entries)}
	if len(entries) < o.MinArticles {
		res.Skipped = fmt.Sprintf("only %d articles in the last %s, need %d", len(entries), o.Window, o.MinArticles)
		return res, nil
	}

	if o.To != "" || o.DryRun {
		to := o.To
		if to == "" {
			to = "sample@example.com"
		}
		if err := sender.Send(ctx, render(entries, o.Day, to, "SAMPLE-TOKEN", o.WebBase, o.APIBase, o.Footer)); err != nil {
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

	res.Eligible = len(subs)
	failures := 0
	for i, sub := range subs {
		if delivered[sub.ID] {
			res.AlreadySent++
			continue
		}
		if o.MaxSends > 0 && res.Sent >= o.MaxSends {
			res.Capped = countUnsent(subs[i:], delivered)
			logger.Warn("send cap reached; the rest get the edition on the next run", "cap", o.MaxSends, "left", res.Capped)
			break
		}

		err := sender.Send(ctx, render(entries, o.Day, sub.Email, sub.Token, o.WebBase, o.APIBase, o.Footer))
		switch {
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
		}

		select {
		case <-ctx.Done():
			return res, ctx.Err()
		case <-time.After(o.Pause):
		}
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
