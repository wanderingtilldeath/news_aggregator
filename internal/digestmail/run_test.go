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
	if n := strings.Count(msg.Text, "Busy "); n != maxPerSourceEmail {
		t.Fatalf("busy site has %d stories, want %d", n, maxPerSourceEmail)
	}
	if !strings.Contains(msg.Text, "https://digest.test/story/h0") || strings.Contains(msg.Text, "https://a.test/1") || strings.Contains(msg.Text, "https://busy.test/") {
		t.Fatalf("headlines must link to the story on this site, never the publisher:\n%s", msg.Text)
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

// An edition for everyone: the same stories whatever tracks a reader once chose, ranked across all categories,
// under plain headings and never an exam or a profession.
func TestTopNewsEditionIsTheSameForEveryoneUnderPlainHeadings(t *testing.T) {
	m := store.NewMemory()
	ctx := context.Background()
	// 14 stories from 14 sites: india 0,4,8,12; world 1,5,9,13; tech 2,6,10; ai 3,7,11. Newest first.
	cats := []string{"india", "world", "tech", "ai"}
	for i := 0; i < 14; i++ {
		hash := fmt.Sprintf("n%d", i)
		_ = m.Save(ctx, store.Page{
			URLHash: hash, URL: fmt.Sprintf("https://site%d.test/%d", i, i), Title: fmt.Sprintf("Headline %d", i),
			IsArticle: true, Category: cats[i%4], Published: time.Now().Add(-time.Duration(i+1) * time.Minute),
		})
		_ = m.SetSummary(ctx, hash, []string{fmt.Sprintf("Gist of %d.", i)})
	}
	_ = m.SetEnrichment(ctx, "n13", store.Enrichment{Why: "Changes taxes.", Importance: 5, Long: "Long budget summary."})

	subscribe(t, m, "brief@x.test", "deep@x.test", "upsc@x.test")
	_, _ = m.SetPreferences(ctx, "tok-deep@x.test", store.Preferences{Mode: "7min"})
	_, _ = m.SetPreferences(ctx, "tok-upsc@x.test", store.Preferences{Tracks: []string{"upsc"}, Mode: "1min"})

	f := &fake{}
	if res, err := Run(ctx, m, f, opts(), quiet); err != nil || res.Sent != 3 {
		t.Fatalf("run = %+v, %v", res, err)
	}
	by := map[string]mail.Message{}
	for _, msg := range f.got {
		by[msg.To] = msg
	}

	brief := by["brief@x.test"].Text
	if brief != strings.Replace(by["upsc@x.test"].Text, "tok-upsc%40x.test", "tok-brief%40x.test", -1) {
		t.Fatalf("a reader who chose the UPSC track must get the same mail as everyone:\n%s", by["upsc@x.test"].Text)
	}
	for _, banned := range []string{"UPSC", "ENGINEERING", "Engineering", "Prelims"} {
		if strings.Contains(brief, banned) {
			t.Fatalf("the mail names %q; headings must be plain:\n%s", banned, brief)
		}
	}
	if !strings.Contains(brief, "1-minute") && !strings.Contains(brief, "Top news") {
		t.Fatalf("the label is missing:\n%s", brief)
	}

	// The most important story leads, ahead of newer ones. The brief read has 10 on top, then each category's
	// leftovers: the 4 stories the top left out are one each in India, World, Technology and AI.
	if i := strings.Index(brief, "TOP STORIES"); i < 0 || strings.Index(brief, "Headline 13") > strings.Index(brief, "Headline 0") {
		t.Fatalf("the most important story must come first:\n%s", brief)
	}
	if !strings.Contains(brief, "Why it matters: Changes taxes.") || strings.Contains(brief, "Long budget summary.") {
		t.Fatalf("the brief read shows the why line and no long summary:\n%s", brief)
	}
	last := -1
	for _, h := range []string{"TOP STORIES", "INDIA", "WORLD", "TECHNOLOGY", "AI\n"} {
		i := strings.Index(brief, h)
		if i < 0 || i < last {
			t.Fatalf("heading %q missing or out of order:\n%s", h, brief)
		}
		last = i
	}
	if n := strings.Count(brief, "https://digest.test/story/n"); n != 14 {
		t.Fatalf("%d story links, want 14 (each story once, on this site):\n%s", n, brief)
	}
	if strings.Contains(brief, "https://site") {
		t.Fatalf("a publisher link is in the mail; every headline must open the story on Postpaper:\n%s", brief)
	}

	deep := by["deep@x.test"].Text
	if !strings.Contains(deep, "longer read") || !strings.Contains(deep, "Long budget summary.") || !strings.Contains(deep, "- Gist of 0.") {
		t.Fatalf("the longer read lists summaries:\n%s", deep)
	}
	// Six lead the longer read; at most three more per category follow.
	if n := strings.Count(deep, "https://digest.test/story/n"); n != 14 {
		t.Fatalf("%d story links in the longer read, want 14:\n%s", n, deep)
	}
}

func TestOneSiteCannotFillTheEmail(t *testing.T) {
	m := store.NewMemory()
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		cat := "india"
		if i%2 == 1 {
			cat = "world"
		}
		_ = m.Save(ctx, store.Page{
			URLHash: fmt.Sprintf("b%d", i), URL: fmt.Sprintf("https://busy.test/%d", i), Title: fmt.Sprintf("Busy %d", i),
			IsArticle: true, Category: cat, Published: time.Now().Add(-time.Duration(i+1) * time.Minute),
		})
	}
	for i := 0; i < 4; i++ {
		_ = m.Save(ctx, store.Page{
			URLHash: fmt.Sprintf("q%d", i), URL: fmt.Sprintf("https://quiet%d.test/", i), Title: fmt.Sprintf("Quiet %d", i),
			IsArticle: true, Category: "world", Published: time.Now().Add(-time.Duration(i+30) * time.Minute),
		})
	}
	subscribe(t, m, "a@x.test")

	f := &fake{}
	o := opts()
	o.MinArticles = 1
	if _, err := Run(ctx, m, f, o, quiet); err != nil || len(f.got) != 1 {
		t.Fatalf("run: %v, %d mails", err, len(f.got))
	}
	if n := strings.Count(f.got[0].Text, "Busy "); n != maxPerSourceEmail {
		t.Fatalf("busy site has %d stories across the whole email, want %d:\n%s", n, maxPerSourceEmail, f.got[0].Text)
	}
	if n := strings.Count(f.got[0].Text, "Quiet "); n != 4 {
		t.Fatalf("the other sites' stories must fill the rest, got %d:\n%s", n, f.got[0].Text)
	}
}

func TestFollowedTopicsGetTheirOwnSectionWithoutRepeats(t *testing.T) {
	m := seed(t, 30) // 30 tech stories: the edition keeps the newest 14 of them
	ctx := context.Background()
	for i, title := range []string{"Monsoon rains hit Kerala", "Monsoon session of Parliament begins", "Story 1 and the monsoon"} {
		age := 3 * time.Hour
		if i == 2 {
			age = time.Second // the newest story of all: already in the edition, so the topic section must skip it
		}
		_ = m.Save(ctx, store.Page{
			URLHash: fmt.Sprintf("m%d", i), URL: fmt.Sprintf("https://paper%d.test/m/%d", i, i), Title: title,
			IsArticle: true, Published: time.Now().Add(-age - time.Duration(i)*time.Minute), Description: "About the monsoon.", Category: "tech",
		})
	}
	subscribe(t, m, "a@x.test", "plain@x.test")
	_, _ = m.SetPreferences(ctx, "tok-a@x.test", store.Preferences{Topics: []string{"  Monsoon ", "monsoon", "zzz-nothing"}})

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
	if strings.Contains(plain.Text, "YOUR TOPICS") || strings.Contains(plain.Text, "Monsoon rains hit Kerala") {
		t.Fatalf("a reader who follows nothing got a topic section:\n%s", plain.Text)
	}
}

func TestTheEditionOpensWithTodayIn60Seconds(t *testing.T) {
	ctx := context.Background()
	send := func(m *store.Memory) mail.Message {
		subscribe(t, m, "a@x.test")
		f := &fake{}
		if res, err := Run(ctx, m, f, opts(), quiet); err != nil || res.Sent != 1 {
			t.Fatalf("run = %+v, %v", res, err)
		}
		return f.got[0]
	}

	m := seed(t, 6)
	_ = m.SaveIntro(ctx, store.Intro{Day: "2026-11-14", Track: "top", Text: "Compilers and chips led the day."})
	msg := send(m)
	if !strings.Contains(msg.HTML, "Today in 60 seconds.") || !strings.Contains(msg.Text, "Today in 60 seconds: Compilers and chips led the day.") {
		t.Fatalf("the intro is missing:\n%s", msg.Text)
	}

	// Without a general intro yet, the two tracks' intros stand in for it.
	m = seed(t, 6)
	_ = m.SaveIntro(ctx, store.Intro{Day: "2026-11-14", Track: "upsc", Text: "Parliament sat."})
	_ = m.SaveIntro(ctx, store.Intro{Day: "2026-11-14", Track: "engineering", Text: "A chip shipped."})
	if text := send(m).Text; !strings.Contains(text, "Today in 60 seconds: Parliament sat. A chip shipped.") {
		t.Fatalf("the track intros must stand in:\n%s", text)
	}

	// No intro at all: no empty heading.
	if text := send(seed(t, 6)).Text; strings.Contains(text, "Today in 60 seconds") {
		t.Fatalf("an empty intro must be left out:\n%s", text)
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

func TestProMembersGetTheirMailFirstWithoutASponsorAndWithPrintableLinks(t *testing.T) {
	m := seed(t, 6)
	ctx := context.Background()
	subscribe(t, m, "a@x.test", "pro@x.test", "b@x.test")
	// Mixed case on the payment side must still match the subscriber.
	if _, _, err := m.ProExtend(ctx, "pay_1", "pro@x.test", 31, "protok"); err != nil {
		t.Fatal(err)
	}

	o := opts()
	o.SponsorText, o.SponsorURL = "Acme prep courses", "https://acme.test/offer"
	f := &fake{}
	if res, err := Run(ctx, m, f, o, quiet); err != nil || res.Sent != 3 {
		t.Fatalf("run = %+v, %v", res, err)
	}

	if got := strings.Join(f.to(), ","); got != "pro@x.test,a@x.test,b@x.test" {
		t.Fatalf("send order = %s, want the Pro member first", got)
	}
	pro, plain := f.got[0], f.got[1]
	if strings.Contains(pro.HTML, "Sponsored") || strings.Contains(pro.Text, "SPONSORED") {
		t.Fatalf("a Pro mail carries the sponsor line:\n%s", pro.Text)
	}
	for _, want := range []string{"https://digest.test/pro/edition?token=protok&day=2026-11-14", "https://digest.test/pro/edition?token=protok&scope=week"} {
		if !strings.Contains(pro.Text, want) || !strings.Contains(pro.HTML, strings.ReplaceAll(want, "&", "&amp;")) {
			t.Fatalf("the Pro mail lacks %s:\n%s", want, pro.Text)
		}
	}
	if !strings.Contains(plain.Text, "SPONSORED: Acme") || strings.Contains(plain.Text, "/pro/edition") {
		t.Fatalf("a free mail should keep the sponsor and have no Pro links:\n%s", plain.Text)
	}
}
