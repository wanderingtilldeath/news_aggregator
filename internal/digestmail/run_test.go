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
		Day: "2026-11-14", Window: 24 * time.Hour, Top: 8, MinArticles: 3, WebBase: "https://digest.test",
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
		_ = m.Save(ctx, store.Page{URLHash: fmt.Sprintf("h%d", i), URL: url, Title: title, IsArticle: true, Category: "tech", Published: time.Now().Add(-time.Duration(i+1) * time.Minute)})
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
	_, _ = m.SetPreferences(ctx, "tok-a@x.test", store.Preferences{Mode: "7min"})

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
			IsArticle: true, Published: time.Now().Add(-time.Duration(i+1) * time.Minute), ContentHash: "c", Category: "tech",
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

func TestTracksModesAndRanking(t *testing.T) {
	m := store.NewMemory()
	ctx := context.Background()
	add := func(hash, url, title, category string, age time.Duration) {
		_ = m.Save(ctx, store.Page{URLHash: hash, URL: url, Title: title, IsArticle: true, Category: category, Published: time.Now().Add(-age)})
		_ = m.SetSummary(ctx, hash, []string{"Gist of " + title + "."})
	}
	add("u1", "https://n.test/1", "Minor policy note", "india", time.Hour)
	add("u2", "https://n.test/2", "Major budget decision", "india", 3*time.Hour)
	add("u3", "https://w.test/3", "Treaty signed", "world", 2*time.Hour)
	add("t1", "https://e.test/1", "Database release", "tech", time.Hour)
	add("t2", "https://e.test/2", "Model launch", "ai", 2*time.Hour)
	add("o1", "https://o.test/1", "Uncategorised item", "other", time.Hour)
	_ = m.SetEnrichment(ctx, "u1", store.Enrichment{Why: "Small.", Importance: 1, Long: "Long minor."})
	_ = m.SetEnrichment(ctx, "u2", store.Enrichment{Why: "Changes taxes.", Tags: []string{"gs3-economy"}, Importance: 5, Long: "Long budget summary."})

	subscribe(t, m, "upsc@x.test", "eng@x.test", "both@x.test")
	_, _ = m.SetPreferences(ctx, "tok-upsc@x.test", store.Preferences{Tracks: []string{"upsc"}, Mode: "1min"})
	_, _ = m.SetPreferences(ctx, "tok-eng@x.test", store.Preferences{Tracks: []string{"engineering"}, Mode: "7min"})

	f := &fake{}
	o := opts()
	o.MinArticles = 2
	if res, err := Run(ctx, m, f, o, quiet); err != nil || res.Sent != 3 {
		t.Fatalf("run = %+v, %v", res, err)
	}
	by := map[string]mail.Message{}
	for _, msg := range f.got {
		by[msg.To] = msg
	}

	up := by["upsc@x.test"].Text
	if !strings.Contains(up, "Major budget decision") || strings.Contains(up, "Database release") || strings.Contains(up, "Uncategorised") {
		t.Fatalf("UPSC mail has the wrong stories:\n%s", up)
	}
	if strings.Index(up, "Major budget decision") > strings.Index(up, "Minor policy note") {
		t.Fatalf("the more important story must come first:\n%s", up)
	}
	if !strings.Contains(up, "Why it matters: Changes taxes.") || strings.Contains(up, "Long budget summary.") || !strings.Contains(up, "1-minute read") {
		t.Fatalf("the 1-minute read shows the why line and no long summary:\n%s", up)
	}

	eng := by["eng@x.test"].Text
	if !strings.Contains(eng, "Database release") || !strings.Contains(eng, "Model launch") || strings.Contains(eng, "Major budget decision") || !strings.Contains(eng, "7-minute read") {
		t.Fatalf("engineering mail wrong:\n%s", eng)
	}

	both := by["both@x.test"].Text
	if !strings.Contains(both, "UPSC CURRENT AFFAIRS") || !strings.Contains(both, "ENGINEERING") || strings.Index(both, "UPSC CURRENT AFFAIRS") > strings.Index(both, "ENGINEERING") {
		t.Fatalf("a reader of both tracks gets both sections, UPSC first:\n%s", both)
	}
}

func TestEditionOfAThinTrackIsSkippedForThatReaderOnly(t *testing.T) {
	m := seed(t, 5) // five tech stories, nothing for the UPSC track
	subscribe(t, m, "eng@x.test", "upsc@x.test")
	_, _ = m.SetPreferences(context.Background(), "tok-upsc@x.test", store.Preferences{Tracks: []string{"upsc"}})

	f := &fake{}
	res, _ := Run(context.Background(), m, f, opts(), quiet)
	if res.Sent != 1 || res.NoEdition != 1 || strings.Join(f.to(), ",") != "eng@x.test" {
		t.Fatalf("res = %+v, mails to %v", res, f.to())
	}
}

func TestQuizIsLinkedAndListedInTheSevenMinuteRead(t *testing.T) {
	m := seed(t, 5)
	ctx := context.Background()
	_ = m.SaveQuiz(ctx, store.Quiz{Day: "2026-11-14", Track: "engineering", Questions: []store.QuizQuestion{
		{Question: "Which index?", Options: []string{"B-tree", "Hash", "GIN", "BRIN"}, Answer: 2, Explanation: "e"},
		{Question: "Which lock?", Options: []string{"a", "b", "c", "d"}, Answer: 0, Explanation: "e"},
	}})
	subscribe(t, m, "brief@x.test", "deep@x.test")
	_, _ = m.SetPreferences(ctx, "tok-deep@x.test", store.Preferences{Mode: "7min"})

	f := &fake{}
	if _, err := Run(ctx, m, f, opts(), quiet); err != nil {
		t.Fatal(err)
	}
	by := map[string]mail.Message{}
	for _, msg := range f.got {
		by[msg.To] = msg
	}

	link := "https://digest.test/quiz?track=engineering&day=2026-11-14"
	brief := by["brief@x.test"].Text
	if !strings.Contains(brief, link) || strings.Contains(brief, "Which index?") {
		t.Fatalf("the 1-minute read links the quiz without listing it:\n%s", brief)
	}
	deep := by["deep@x.test"].Text
	if !strings.Contains(deep, link) || !strings.Contains(deep, "1. Which index?") || !strings.Contains(deep, "B-tree | Hash | GIN | BRIN") {
		t.Fatalf("the 7-minute read lists the questions:\n%s", deep)
	}
	if strings.Contains(deep, "GIN | BRIN | ") || strings.Contains(by["deep@x.test"].HTML, "explanation") {
		t.Fatalf("answers and explanations must not be in the mail")
	}
	if !strings.Contains(by["deep@x.test"].HTML, "check your answers") {
		t.Fatalf("HTML mail lacks the quiz block")
	}
}

func TestFollowedTopicsGetTheirOwnSectionWithoutRepeats(t *testing.T) {
	m := seed(t, 6)
	ctx := context.Background()
	for i, title := range []string{"Monsoon rains hit Kerala", "Monsoon session of Parliament begins", "Story 1 and the monsoon"} {
		cat := "india"
		if i == 2 {
			cat = "tech" // already in the engineering edition, so the topic section must skip it
		}
		_ = m.Save(ctx, store.Page{
			URLHash: fmt.Sprintf("m%d", i), URL: fmt.Sprintf("https://paper%d.test/m/%d", i, i), Title: title,
			IsArticle: true, Published: time.Now().Add(-time.Duration(i+1) * time.Minute), Description: "About the monsoon.", Category: cat,
		})
	}
	subscribe(t, m, "a@x.test", "plain@x.test")
	_, _ = m.SetPreferences(ctx, "tok-a@x.test", store.Preferences{Tracks: []string{"engineering"}, Topics: []string{"  Monsoon ", "monsoon", "zzz-nothing"}})
	_, _ = m.SetPreferences(ctx, "tok-plain@x.test", store.Preferences{Tracks: []string{"engineering"}})

	f := &fake{}
	if res, err := Run(ctx, m, f, opts(), quiet); err != nil || res.Sent != 2 {
		t.Fatalf("run = %+v, %v", res, err)
	}
	var a, plain mail.Message
	for _, msg := range f.got {
		if msg.To == "a@x.test" {
			a = msg
		} else {
			plain = msg
		}
	}
	if !strings.Contains(a.HTML, "Your topics") || !strings.Contains(a.Text, "YOUR TOPICS") || !strings.Contains(a.Text, "Monsoon rains hit Kerala") {
		t.Fatalf("the follower's mail has no topic section:\n%s", a.Text)
	}
	if strings.Count(a.Text, "Story 1 and the monsoon") != 1 {
		t.Fatalf("a story already in the edition was repeated in the topic section:\n%s", a.Text)
	}
	if strings.Contains(plain.Text, "YOUR TOPICS") {
		t.Fatalf("a reader who follows nothing got a topic section:\n%s", plain.Text)
	}
}

func TestTheEditionOpensWithTheTracksIntro(t *testing.T) {
	m := seed(t, 6)
	ctx := context.Background()
	_ = m.SaveIntro(ctx, store.Intro{Day: "2026-11-14", Track: "engineering", Text: "Compilers and chips led the day."})
	subscribe(t, m, "a@x.test")
	_, _ = m.SetPreferences(ctx, "tok-a@x.test", store.Preferences{Tracks: []string{"engineering"}})

	f := &fake{}
	if res, err := Run(ctx, m, f, opts(), quiet); err != nil || res.Sent != 1 {
		t.Fatalf("run = %+v, %v", res, err)
	}
	msg := f.got[0]
	if !strings.Contains(msg.HTML, "Today in 60 seconds.") || !strings.Contains(msg.Text, "Today in 60 seconds: Compilers and chips led the day.") {
		t.Fatalf("the intro is missing:\n%s", msg.Text)
	}
}

func TestSponsorLineNeedsTextAndASafeLink(t *testing.T) {
	for _, tc := range []struct {
		text, url string
		shown     bool
	}{
		{"Acme prep courses", "https://acme.test/offer?a=1&b=2", true},
		{"Acme prep courses", "javascript:alert(1)", false},
		{"Acme prep courses", "", false},
		{"  ", "https://acme.test", false},
	} {
		m := seed(t, 6)
		subscribe(t, m, "a@x.test")
		o := opts()
		o.SponsorText, o.SponsorURL = tc.text, tc.url
		f := &fake{}
		if res, err := Run(context.Background(), m, f, o, quiet); err != nil || res.Sent != 1 {
			t.Fatalf("run = %+v, %v", res, err)
		}
		msg := f.got[0]
		if got := strings.Contains(msg.HTML, "Sponsored") && strings.Contains(msg.Text, "SPONSORED: Acme"); got != tc.shown {
			t.Errorf("sponsor %q %q shown = %v, want %v", tc.text, tc.url, got, tc.shown)
		}
		if tc.shown && !strings.Contains(msg.HTML, `rel="sponsored"`) {
			t.Errorf("a sponsor link needs rel=sponsored: %s", msg.HTML)
		}
	}
}
