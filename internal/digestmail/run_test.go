package digestmail

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/mail"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/testutil"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// fake records messages; failFor and limitAt choose which sends fail.
type fake struct {
	got      []mail.Message
	failFor  map[string]bool
	limitAt  int // the nth send (1-based) returns ErrRateLimited; 0 = never
	attempts int
}

func (f *fake) Send(_ context.Context, m mail.Message) error {
	f.attempts++
	if f.limitAt > 0 && f.attempts == f.limitAt {
		return mail.ErrRateLimited
	}
	if f.failFor[m.To] {
		return errors.New("boom")
	}
	f.got = append(f.got, m)

	return nil
}

func (f *fake) to() []string {
	var out []string
	for _, m := range f.got {
		out = append(out, m.To)
	}

	return out
}

func opts() Options {
	return Options{
		Day: "2026-11-14", Window: 24 * time.Hour, Top: 10, MinArticles: 3, WebBase: "https://digest.test",
		APIBase: "https://api.test", Footer: "Postpaper, Example Street",
	}
}

func seed(t *testing.T, articles int) *store.Memory {
	t.Helper()

	m := store.NewMemory()
	ctx := context.Background()
	for i := 0; i < articles; i++ {
		err := m.Save(ctx, store.Page{
			URLHash: fmt.Sprintf("h%d", i), URL: fmt.Sprintf("https://site%d.test/a/%d", i%4, i), Title: fmt.Sprintf("Story %d", i),
			IsArticle: true, Published: time.Now().Add(-time.Duration(i+1) * time.Minute), Description: "A description.", Category: "tech",
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	return m
}

func subscribe(t *testing.T, m *store.Memory, emails ...string) {
	t.Helper()

	for _, e := range emails {
		tok, _, _ := m.Subscribe(context.Background(), e, "tok-"+e)
		if ok, _ := m.Confirm(context.Background(), tok); !ok {
			t.Fatalf("confirm %s", e)
		}
	}
}

func TestSendsOncePerSubscriberPerDay(t *testing.T) {
	m := seed(t, 6)
	subscribe(t, m, "a@x.test", "b@x.test")
	// Pending and unsubscribed readers get nothing.
	_, _, _ = m.Subscribe(context.Background(), "pending@x.test", "tok-pending")
	subscribe(t, m, "gone@x.test")
	_, _ = m.Unsubscribe(context.Background(), "tok-gone@x.test")

	f := &fake{}
	res, err := Run(context.Background(), m, f, opts(), quiet)
	if err != nil || res.Sent != 2 || res.Eligible != 2 {
		t.Fatalf("first run = %+v, %v", res, err)
	}
	if got := strings.Join(f.to(), ","); got != "a@x.test,b@x.test" {
		t.Fatalf("recipients = %s", got)
	}

	res, _ = Run(context.Background(), m, f, opts(), quiet)
	if res.Sent != 0 || res.AlreadySent != 2 || len(f.got) != 2 {
		t.Fatalf("second run the same day = %+v, %d mails total", res, len(f.got))
	}

	next := opts()
	next.Day = "2026-11-15"
	if res, _ = Run(context.Background(), m, f, next, quiet); res.Sent != 2 {
		t.Fatalf("next day = %+v, want 2 sent", res)
	}
}

func TestSkipsAnEditionWithTooFewArticles(t *testing.T) {
	m := seed(t, 2)
	subscribe(t, m, "a@x.test")
	f := &fake{}

	res, err := Run(context.Background(), m, f, opts(), quiet)
	if err != nil || res.Skipped == "" || len(f.got) != 0 {
		t.Fatalf("res = %+v, %v, %d mails; want skipped with none sent", res, err, len(f.got))
	}
}

func TestSendCapLeavesTheRestForTheNextRun(t *testing.T) {
	m := seed(t, 6)
	subscribe(t, m, "a@x.test", "b@x.test", "c@x.test")
	f := &fake{}

	o := opts()
	o.MaxSends = 2
	res, _ := Run(context.Background(), m, f, o, quiet)
	if res.Sent != 2 || res.Capped != 1 {
		t.Fatalf("capped run = %+v", res)
	}
	res, _ = Run(context.Background(), m, f, o, quiet)
	if res.Sent != 1 || strings.Join(f.to(), ",") != "a@x.test,b@x.test,c@x.test" {
		t.Fatalf("next run = %+v, recipients %v", res, f.to())
	}
}

func TestRateLimitStopsTheRunAndResumesLater(t *testing.T) {
	m := seed(t, 6)
	subscribe(t, m, "a@x.test", "b@x.test", "c@x.test")
	f := &fake{limitAt: 2}

	res, err := Run(context.Background(), m, f, opts(), quiet)
	if err != nil || res.Sent != 1 || res.Capped != 2 {
		t.Fatalf("limited run = %+v, %v", res, err)
	}
	if res, _ = Run(context.Background(), m, f, opts(), quiet); res.Sent != 2 {
		t.Fatalf("resumed run = %+v", res)
	}
}

func TestOneFailureDoesNotBlockOthersOrGetRecorded(t *testing.T) {
	m := seed(t, 6)
	subscribe(t, m, "a@x.test", "bad@x.test", "c@x.test")
	f := &fake{failFor: map[string]bool{"bad@x.test": true}}

	res, err := Run(context.Background(), m, f, opts(), quiet)
	if err != nil || res.Sent != 2 || res.Failed != 1 {
		t.Fatalf("res = %+v, %v", res, err)
	}

	f.failFor = nil
	if res, _ = Run(context.Background(), m, f, opts(), quiet); res.Sent != 1 || f.got[len(f.got)-1].To != "bad@x.test" {
		t.Fatalf("retry = %+v: the failed address must be retried", res)
	}
}

func TestStopsWhenTheProviderKeepsFailing(t *testing.T) {
	m := seed(t, 6)
	fails := map[string]bool{}
	for i := 0; i < 8; i++ {
		e := fmt.Sprintf("u%d@x.test", i)
		subscribe(t, m, e)
		fails[e] = true
	}
	f := &fake{failFor: fails}

	if _, err := Run(context.Background(), m, f, opts(), quiet); err == nil || f.attempts != maxConsecutiveFailures {
		t.Fatalf("err = %v after %d attempts, want a stop after %d", err, f.attempts, maxConsecutiveFailures)
	}
}

func TestDryRunAndToSendOnlySamplesAndRecordNothing(t *testing.T) {
	m := seed(t, 6)
	subscribe(t, m, "a@x.test")

	f := &fake{}
	o := opts()
	o.DryRun = true
	res, _ := Run(context.Background(), m, f, o, quiet)
	if res.Eligible != 1 || res.Sent != 0 || strings.Join(f.to(), ",") != "sample@example.com" {
		t.Fatalf("dry run = %+v, mails to %v", res, f.to())
	}

	o = opts()
	o.To = "me@x.test"
	if res, _ = Run(context.Background(), m, f, o, quiet); res.Sent != 1 || f.got[len(f.got)-1].To != "me@x.test" {
		t.Fatalf("-to run = %+v", res)
	}

	// Neither marked anyone delivered: a real run still mails the subscriber.
	if res, _ = Run(context.Background(), m, f, opts(), quiet); res.Sent != 1 {
		t.Fatalf("real run after samples = %+v, want 1 sent", res)
	}
}

func TestEmailContent(t *testing.T) {
	m := store.NewMemory()
	ctx := context.Background()
	save := func(i int, url, title string, summary []string) {
		_ = m.Save(ctx, store.Page{URLHash: fmt.Sprintf("h%d", i), URL: url, Title: title, IsArticle: true, Published: time.Now().Add(-time.Duration(i+1) * time.Minute)})
		if summary != nil {
			_ = m.SetSummary(ctx, fmt.Sprintf("h%d", i), summary)
		}
	}
	save(0, "https://a.test/1", `<script>alert(1)</script> Budget & tax`, []string{"First point.", "Second <b>point</b>."})
	save(1, "javascript:alert(2)", "Bad link", nil)
	// Five stories from one site: only three may go in.
	for i := 2; i < 7; i++ {
		save(i, fmt.Sprintf("https://busy.test/%d", i), fmt.Sprintf("Busy %d", i), nil)
	}
	subscribe(t, m, "a@x.test")

	f := &fake{}
	o := opts()
	o.MinArticles = 1
	if _, err := Run(ctx, m, f, o, quiet); err != nil || len(f.got) != 1 {
		t.Fatalf("run: %v, %d mails", err, len(f.got))
	}
	msg := f.got[0]

	if strings.Contains(msg.HTML, "<script>") || strings.Contains(msg.HTML, "<b>point") || strings.Contains(msg.HTML, "javascript:") {
		t.Fatalf("feed text must be escaped and bad links dropped:\n%s", msg.HTML)
	}
	if !strings.Contains(msg.HTML, "Budget &amp; tax") || !strings.Contains(msg.Text, "- First point.") {
		t.Fatalf("content missing:\n%s\n%s", msg.HTML, msg.Text)
	}
	if n := strings.Count(msg.Text, "https://busy.test/"); n != maxPerSource {
		t.Fatalf("busy site has %d stories, want %d", n, maxPerSource)
	}
	if !strings.Contains(msg.Subject, "Saturday 14 November 2026") || !strings.Contains(msg.Text, "Postpaper, Example Street") {
		t.Fatalf("subject %q / footer missing", msg.Subject)
	}
	if !strings.Contains(msg.Text, "https://digest.test/unsubscribe?token=tok-a%40x.test") {
		t.Fatalf("unsubscribe link missing:\n%s", msg.Text)
	}
	if got := msg.Headers["List-Unsubscribe"]; got != "<https://api.test/unsubscribe?token=tok-a%40x.test>" || msg.Headers["List-Unsubscribe-Post"] != "List-Unsubscribe=One-Click" {
		t.Fatalf("headers = %v", msg.Headers)
	}
}

// The same guarantees against the real Postgres store (needs TEST_DATABASE_URL).
func TestPostgresRunSendsOncePerDay(t *testing.T) {
	st := store.NewPostgres(testutil.Postgres(t))
	defer st.Close()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		err := st.Save(ctx, store.Page{
			URLHash: fmt.Sprintf("h%d", i), URL: fmt.Sprintf("https://site%d.test/a/%d", i, i), Title: fmt.Sprintf("Story %d", i),
			IsArticle: true, Published: time.Now().Add(-time.Duration(i+1) * time.Minute), ContentHash: "c",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []string{"a@x.test", "b@x.test"} {
		tok, _, _ := st.Subscribe(ctx, e, "tok-"+e)
		_, _ = st.Confirm(ctx, tok)
	}

	f := &fake{}
	if res, err := Run(ctx, st, f, opts(), quiet); err != nil || res.Sent != 2 {
		t.Fatalf("first run = %+v, %v", res, err)
	}
	if res, _ := Run(ctx, st, f, opts(), quiet); res.Sent != 0 || res.AlreadySent != 2 || len(f.got) != 2 {
		t.Fatalf("second run = %+v, %d mails", res, len(f.got))
	}
	next := opts()
	next.Day = "2026-11-15"
	if res, _ := Run(ctx, st, f, next, quiet); res.Sent != 2 {
		t.Fatalf("next day = %+v", res)
	}
}
