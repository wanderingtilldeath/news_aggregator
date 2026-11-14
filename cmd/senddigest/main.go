// Command senddigest emails today's edition to every confirmed subscriber who has
// not had it yet. It runs after the crawl (see .github/workflows/crawl.yml) and is
// safe to run again on the same day: nobody is mailed twice.
//
//	senddigest -dry-run          print a sample email and count the readers; sends nothing
//	senddigest -to me@example.com   send one sample edition to that address; records nothing
//	senddigest                   the real send (needs MAIL_PROVIDER=brevo)
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/config"
	"github.com/wanderingtilldeath/news-aggregator/internal/db"
	"github.com/wanderingtilldeath/news-aggregator/internal/digestmail"
	"github.com/wanderingtilldeath/news-aggregator/internal/mail"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "senddigest:", err)
		os.Exit(1)
	}
}

func run() error {
	// The shared settings (database, mail, links, time zone) come from the
	// environment, the same variables crawlerx reads.
	cfg, err := config.Load(nil, os.Getenv, os.Stderr)
	if err != nil {
		return err
	}

	o := digestmail.Options{WebBase: cfg.WebBaseURL, APIBase: cfg.PublicAPIURL, Footer: cfg.MailFooter}
	fs := flag.NewFlagSet("senddigest", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "print a sample email and count the readers; send nothing")
	fs.StringVar(&o.To, "to", "", "send one sample edition to this address and record nothing")
	day := fs.String("day", "", "edition day YYYY-MM-DD; default today in the configured time zone")
	fs.IntVar(&o.MaxSends, "max-sends", 250, "most emails per run; Brevo's free plan allows 300 a day, and confirmations count too")
	fs.IntVar(&o.Top, "top", 10, "articles per email")
	fs.IntVar(&o.MinArticles, "min-articles", 3, "send nothing when fewer articles than this are new")
	fs.DurationVar(&o.Window, "window", 24*time.Hour, "how far back articles count")
	fs.DurationVar(&o.Pause, "pause", 250*time.Millisecond, "wait between emails")
	if err := fs.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	o.DryRun = *dryRun

	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return fmt.Errorf("time zone %q: %w", cfg.Timezone, err)
	}
	o.Day = *day
	if o.Day == "" {
		o.Day = time.Now().In(loc).Format("2006-01-02")
	} else if _, err := time.Parse("2006-01-02", o.Day); err != nil {
		return fmt.Errorf("-day must be YYYY-MM-DD: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	sender := mail.New(cfg.MailProvider, cfg.BrevoAPIKey, cfg.MailFrom, cfg.MailFromName, logger)

	switch {
	case o.DryRun:
		// A dry run must never reach a provider.
		sender = mail.Log{Logger: logger}
	case o.To == "":
		// The log provider prints every message, with its subscriber's unsubscribe
		// token, which must not end up in a CI log.
		if cfg.MailProvider != "brevo" {
			return errors.New("a real send needs MAIL_PROVIDER=brevo; use -dry-run or -to to try it first")
		}
		if cfg.PublicAPIURL == "" {
			return errors.New("a real send needs PUBLIC_API_URL, the address the one-click unsubscribe header posts to")
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL, 4)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}
	pages := store.NewPostgres(pool)
	defer pages.Close()

	res, err := digestmail.Run(ctx, pages, sender, o, logger)
	logger.Info("digest email run",
		"day", o.Day, "articles", res.Articles, "eligible", res.Eligible, "sent", res.Sent, "already_sent", res.AlreadySent,
		"failed", res.Failed, "left_for_next_run", res.Capped, "unrecorded", res.Unrecorded, "skipped", res.Skipped)
	if err != nil {
		return err
	}
	if res.Failed > 0 || res.Unrecorded > 0 {
		return fmt.Errorf("%d sends failed and %d were not recorded", res.Failed, res.Unrecorded)
	}

	return nil
}
