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
	"strings"
	"syscall"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/chat"
	"github.com/wanderingtilldeath/news-aggregator/internal/config"
	"github.com/wanderingtilldeath/news-aggregator/internal/db"
	"github.com/wanderingtilldeath/news-aggregator/internal/digestmail"
	"github.com/wanderingtilldeath/news-aggregator/internal/mail"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/telegram"
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

	o := digestmail.Options{WebBase: cfg.WebBaseURL, APIBase: cfg.PublicAPIURL, Footer: cfg.MailFooter, SponsorText: cfg.MailSponsorText, SponsorURL: cfg.MailSponsorURL}
	fs := flag.NewFlagSet("senddigest", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "print a sample email and count the readers; send nothing")
	fs.StringVar(&o.To, "to", "", "send one sample edition to this address and record nothing")
	day := fs.String("day", "", "edition day YYYY-MM-DD; default today in the configured time zone")
	fs.IntVar(&o.MaxSends, "max-sends", 250, "most emails per run; Brevo's free plan allows 300 a day, and confirmations count too")
	fs.IntVar(&o.Top, "top", 8, "stories per track in the 7-minute read; the 1-minute read gets at most 5")
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

	telegramOn := len(cfg.TelegramChats) > 0
	teamsOn := cfg.TeamPosts
	emailOn := true
	switch {
	case o.DryRun:
		// A dry run must never reach a provider.
		sender = mail.Log{Logger: logger}
	case o.To == "":
		// The log provider prints every message, with its subscriber's unsubscribe
		// token, which must not end up in a CI log. Without Brevo only the Telegram
		// channels are posted.
		if cfg.MailProvider != "brevo" {
			if !telegramOn && !teamsOn {
				return errors.New("a real send needs MAIL_PROVIDER=brevo (or a Telegram channel or TEAM_POSTS=true); use -dry-run or -to to try it first")
			}
			emailOn = false
			logger.Info("email skipped: MAIL_PROVIDER is not brevo; posting to the channels only")
		} else if cfg.PublicAPIURL == "" {
			return errors.New("a real send needs PUBLIC_API_URL, the address the one-click unsubscribe header posts to")
		} else if strings.TrimSpace(cfg.MailFooter) == "" {
			return errors.New("a real send needs MAIL_FOOTER: your name and postal address, which anti-spam laws require in every mail")
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

	// Each step runs even when an earlier one failed: a mail outage must not silence Telegram and
	// the teams. The errors are joined and returned at the end.
	var errs []error
	var res digestmail.Result
	if emailOn {
		res, err = digestmail.Run(ctx, pages, sender, o, logger)
		logger.Info("digest email run",
			"day", o.Day, "articles", res.Articles, "eligible", res.Eligible, "sent", res.Sent, "already_sent", res.AlreadySent,
			"failed", res.Failed, "left_for_next_run", res.Capped, "no_edition", res.NoEdition, "unrecorded", res.Unrecorded, "skipped", res.Skipped)
		if err != nil {
			if ctx.Err() != nil {
				return err
			}
			errs = append(errs, err)
		}
	}

	// One reminder mail per Pro period, a few days before it ends. Mail only, and not for a -to test.
	if emailOn && o.To == "" {
		n, rerr := digestmail.ProReminders(ctx, pages, sender, o.WebBase, o.DryRun, logger)
		logger.Info("pro reminders", "sent", n)
		if rerr != nil {
			if ctx.Err() != nil {
				return rerr
			}
			errs = append(errs, rerr)
		}
	}

	// The Telegram channels: skipped for -to (a test of one address).
	if telegramOn && o.To == "" {
		var poster telegram.Poster = telegram.Bot{Token: cfg.TelegramBotToken}
		if o.DryRun {
			poster = telegram.Log{Logger: logger}
		}
		posted, perr := digestmail.RunChannels(ctx, pages, poster, digestmail.ChannelOptions{
			Day: o.Day, Chats: cfg.TelegramChats, Window: o.Window, Top: o.Top, MinArticles: o.MinArticles, WebBase: o.WebBase, DryRun: o.DryRun,
		}, logger)
		logger.Info("telegram posts", "day", o.Day, "posted", posted)
		if perr != nil {
			errs = append(errs, perr)
		}
	}

	// The teams' Slack and Discord channels: skipped for -to.
	if teamsOn && o.To == "" {
		var poster chat.Poster = chat.Webhook{}
		if o.DryRun {
			poster = chat.Log{Logger: logger}
		}
		posted, terr := digestmail.RunTeams(ctx, pages, poster, digestmail.TeamOptions{
			Day: o.Day, Window: o.Window, Top: o.Top, MinArticles: o.MinArticles, WebBase: o.WebBase, DryRun: o.DryRun, Pause: time.Second,
		}, logger)
		logger.Info("team posts", "day", o.Day, "posted", posted)
		if terr != nil {
			errs = append(errs, terr)
		}
	}

	if res.Failed > 0 || res.Unrecorded > 0 {
		errs = append(errs, fmt.Errorf("%d sends failed and %d were not recorded", res.Failed, res.Unrecorded))
	}

	return errors.Join(errs...)
}
