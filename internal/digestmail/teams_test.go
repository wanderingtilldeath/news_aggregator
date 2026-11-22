package digestmail

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/chat"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

type posted struct{ kind, url, text string }

type fakePoster struct {
	got  []posted
	gone map[string]bool
	err  map[string]error
}

func (f *fakePoster) Post(_ context.Context, kind, webhook, text string) error {
	if f.gone[webhook] {
		return chat.ErrGone
	}
	if e := f.err[webhook]; e != nil {
		return e
	}
	f.got = append(f.got, posted{kind, webhook, text})

	return nil
}

func teamOpts() TeamOptions {
	return TeamOptions{Day: "2026-11-14", Window: 24 * 3600e9, Top: 5, MinArticles: 3, WebBase: "https://digest.test"}
}

func TestRunTeamsPostsOncePerChannelPerDayInTheirOwnFormat(t *testing.T) {
	m := seed(t, 6)
	ctx := context.Background()
	_, _, _ = m.ConnectTeamHook(ctx, chat.Slack, "https://hooks.slack.com/services/T/B/x", "engineering", "tok-slack")
	_, _, _ = m.ConnectTeamHook(ctx, chat.Discord, "https://discord.com/api/webhooks/1/y", "engineering", "tok-discord")
	p := &fakePoster{}

	n, err := RunTeams(ctx, m, p, teamOpts(), quiet)
	if err != nil || n != 2 || len(p.got) != 2 {
		t.Fatalf("RunTeams = %d, %v with %d posts, want 2", n, err, len(p.got))
	}
	slack, discord := p.got[0], p.got[1]
	if slack.kind != chat.Slack || !strings.Contains(slack.text, "*Postpaper · Engineering*") || !strings.Contains(slack.text, "<https://digest.test/story/h0|Story 0>") {
		t.Fatalf("slack post:\n%s", slack.text)
	}
	if discord.kind != chat.Discord || !strings.Contains(discord.text, "**Postpaper · Engineering**") || !strings.Contains(discord.text, "[Story 0](<https://digest.test/story/h0>)") {
		t.Fatalf("discord post:\n%s", discord.text)
	}
	for _, msg := range []posted{slack, discord} {
		if strings.Contains(msg.text, "site0.test") {
			t.Fatalf("a post links to the publisher; every story must open on the site: %s", msg.text)
		}
		if !strings.Contains(msg.text, "https://digest.test/teams/stop?token=tok-") || len(msg.text) > chat.MaxMessage {
			t.Fatalf("a post needs its stop link and must fit the limit (%d): %s", len(msg.text), msg.text)
		}
	}

	// A second run the same day posts nothing; the next day posts again.
	if n, _ := RunTeams(ctx, m, p, teamOpts(), quiet); n != 0 {
		t.Fatalf("second run posted %d, want 0", n)
	}
	next := teamOpts()
	next.Day = "2026-11-15"
	if n, _ := RunTeams(ctx, m, p, next, quiet); n != 2 {
		t.Fatalf("the next day posted %d, want 2", n)
	}
}

func TestRunTeamsDisconnectsDeletedWebhooksAndStopsOnARateLimit(t *testing.T) {
	m := seed(t, 6)
	ctx := context.Background()
	_, _, _ = m.ConnectTeamHook(ctx, chat.Slack, "https://hooks.slack.com/services/T/B/gone", "engineering", "tok-gone")
	_, _, _ = m.ConnectTeamHook(ctx, chat.Discord, "https://discord.com/api/webhooks/1/ok", "engineering", "tok-ok")
	_, _, _ = m.ConnectTeamHook(ctx, chat.Slack, "https://hooks.slack.com/services/T/B/slow", "engineering", "tok-slow")
	_, _, _ = m.ConnectTeamHook(ctx, chat.Slack, "https://hooks.slack.com/services/T/B/after", "engineering", "tok-after")
	p := &fakePoster{gone: map[string]bool{"https://hooks.slack.com/services/T/B/gone": true}, err: map[string]error{"https://hooks.slack.com/services/T/B/slow": chat.ErrRateLimited}}

	n, err := RunTeams(ctx, m, p, teamOpts(), quiet)
	if err != nil || n != 1 {
		t.Fatalf("RunTeams = %d, %v; want the healthy channel posted and the run ended at the rate limit", n, err)
	}
	for _, got := range p.got {
		if strings.Contains(got.url, "after") {
			t.Fatal("the run went on after a rate limit")
		}
	}
	hooks, _ := m.TeamHooks(ctx)
	if len(hooks) != 3 || hooks[0].Token != "tok-ok" {
		t.Fatalf("hooks = %+v, want the deleted one disconnected and the others kept", hooks)
	}
	// The rate-limited channel was not marked done, so the next run tries it (and the one after it).
	p.err = nil
	if n, _ := RunTeams(ctx, m, p, teamOpts(), quiet); n != 2 {
		t.Fatalf("retry posted %d, want the two channels that had not been reached", n)
	}
}

func TestRunTeamsDisconnectsAChannelThatKeepsRefusing(t *testing.T) {
	m := seed(t, 6)
	ctx := context.Background()
	const bad = "https://hooks.slack.com/services/T/B/bad"
	_, _, _ = m.ConnectTeamHook(ctx, chat.Slack, bad, "engineering", "tok-bad")
	p := &fakePoster{err: map[string]error{bad: chat.ErrRejected}}

	day := teamOpts()
	for i := 1; i <= maxRefusals; i++ {
		day.Day = fmt.Sprintf("2026-11-%02d", 10+i)
		n, err := RunTeams(ctx, m, p, day, quiet)
		if n != 0 || err == nil {
			t.Fatalf("day %d: posted %d, err %v; want an error when every post failed", i, n, err)
		}
		hooks, _ := m.TeamHooks(ctx)
		if want := i < maxRefusals; (len(hooks) == 1) != want {
			t.Fatalf("day %d: %d hooks left, kept = %v want %v", i, len(hooks), len(hooks) == 1, want)
		}
	}

	// One good post clears the count, so refusals must run in a row to disconnect.
	_, _, _ = m.ConnectTeamHook(ctx, chat.Slack, bad, "engineering", "tok-bad")
	p.err = map[string]error{bad: chat.ErrRejected}
	day.Day = "2026-11-20"
	_, _ = RunTeams(ctx, m, p, day, quiet)
	p.err = nil
	day.Day = "2026-11-21"
	if n, _ := RunTeams(ctx, m, p, day, quiet); n != 1 {
		t.Fatalf("posted %d after a refusal, want 1", n)
	}
	if hooks, _ := m.TeamHooks(ctx); len(hooks) != 1 || hooks[0].Fails != 0 {
		t.Fatalf("hooks = %+v, want the count back to 0", hooks)
	}
}

func TestRunTeamsPausesBetweenPosts(t *testing.T) {
	m := seed(t, 6)
	ctx := context.Background()
	_, _, _ = m.ConnectTeamHook(ctx, chat.Slack, "https://hooks.slack.com/services/T/B/a", "engineering", "tok-a")
	_, _, _ = m.ConnectTeamHook(ctx, chat.Slack, "https://hooks.slack.com/services/T/B/b", "engineering", "tok-b")
	o := teamOpts()
	o.Pause = 60 * time.Millisecond
	start := time.Now()
	if n, err := RunTeams(ctx, m, &fakePoster{}, o, quiet); err != nil || n != 2 {
		t.Fatalf("RunTeams = %d, %v", n, err)
	}
	if took := time.Since(start); took < o.Pause {
		t.Fatalf("two posts took %v, want a pause of %v between them", took, o.Pause)
	}
}

func TestRunTeamsSkipsAThinDayAndEscapesTitles(t *testing.T) {
	ctx := context.Background()
	thin := seed(t, 2)
	_, _, _ = thin.ConnectTeamHook(ctx, chat.Slack, "https://hooks.slack.com/services/T/B/x", "engineering", "tok")
	p := &fakePoster{}
	if n, _ := RunTeams(ctx, thin, p, teamOpts(), quiet); n != 0 || len(p.got) != 0 {
		t.Fatalf("a day with %d stories posted %d, want it skipped", 2, n)
	}

	sec := section{Title: "Engineering", Entries: []entry{{Title: "A & B <script> [x] *y*", URL: "https://a.test/1", Why: "Because <b>."}}}
	slack := teamText(chat.Slack, sec, "2026-11-14", "https://w.test", "t")
	if !strings.Contains(slack, "A &amp; B &lt;script&gt;") || strings.Contains(slack, "<script>") || strings.Contains(slack, "<b>") {
		t.Fatalf("slack text is not escaped: %s", slack)
	}
	discord := teamText(chat.Discord, sec, "2026-11-14", "https://w.test", "t")
	if strings.ContainsAny(strings.SplitN(strings.SplitN(discord, "[", 2)[1], "](", 2)[0], "[]*") {
		t.Fatalf("discord link text still has markup: %s", discord)
	}
	_ = store.Page{}
}
