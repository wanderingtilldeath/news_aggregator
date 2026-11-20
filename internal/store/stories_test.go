package store_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/testutil"
)

// storiesContract runs the same checks on every Store: grouping is written and read the same way.
func storiesContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	now := time.Now()
	for i, p := range []store.Page{
		{URLHash: "a", URL: "https://one.test/a", Title: "Donbas plan floated", IsArticle: true, Published: now.Add(-3 * time.Hour)},
		{URLHash: "b", URL: "https://two.test/b", Title: "Donbas plan reported", IsArticle: true, Published: now.Add(-2 * time.Hour)},
		{URLHash: "c", URL: "https://three.test/c", Title: "Budget day", IsArticle: true, Published: now.Add(-time.Hour)},
		{URLHash: "d", URL: "https://three.test/section/", Title: "Listing page", Published: now.Add(-time.Hour)},
	} {
		p.ContentHash = string(rune('a' + i))
		if err := s.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	since := now.Add(-24 * time.Hour)

	cands, err := s.GroupCandidates(ctx, since, 10)
	if err != nil || len(cands) != 3 || cands[0].ID != "c" || cands[0].Source != "three.test" || cands[0].Title != "Budget day" {
		t.Fatalf("GroupCandidates = %+v, %v; want the 3 articles newest first", cands, err)
	}
	if few, _ := s.GroupCandidates(ctx, since, 2); len(few) != 2 {
		t.Fatalf("limit ignored: %d candidates", len(few))
	}

	groups := map[string]store.StoryGroup{"a": {ID: "a", Sources: 2}, "b": {ID: "a", Sources: 2}}
	if err := s.SetStories(ctx, since, groups); err != nil {
		t.Fatal(err)
	}

	it, ok, err := s.Item(ctx, "b")
	if err != nil || !ok || it.Coverage != 2 || len(it.Also) != 1 || it.Also[0].ID != "a" || it.Also[0].Source != "one.test" || it.Also[0].Title != "Donbas plan floated" || it.Also[0].Time.IsZero() {
		t.Fatalf("Item b = %+v, %v, %v; want coverage 2 and a as the other story", it, ok, err)
	}
	if lone, _, _ := s.Item(ctx, "c"); lone.Coverage != 0 || len(lone.Also) != 0 {
		t.Fatalf("a story with no match must carry no coverage: %+v", lone)
	}
	items, _, _ := s.Digest(ctx, store.DigestQuery{Since: since, ArticlesOnly: true, Limit: 10})
	coverage := map[string]int{}
	for _, it := range items {
		coverage[it.ID] = it.Coverage
	}
	if coverage["a"] != 2 || coverage["b"] != 2 || coverage["c"] != 0 {
		t.Fatalf("digest coverage = %v", coverage)
	}

	// A later run that no longer finds the group clears it.
	if err := s.SetStories(ctx, since, map[string]store.StoryGroup{}); err != nil {
		t.Fatal(err)
	}
	if it, _, _ := s.Item(ctx, "b"); it.Coverage != 0 || len(it.Also) != 0 {
		t.Fatalf("stale group kept: %+v", it)
	}
}

func TestMemoryStories(t *testing.T) { storiesContract(t, store.NewMemory()) }

func TestPostgresStories(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	storiesContract(t, s)
}

func introContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	if _, ok, err := s.GetIntro(ctx, "", "upsc"); ok || err != nil {
		t.Fatalf("empty store: ok=%v err=%v", ok, err)
	}
	for _, in := range []store.Intro{
		{Day: "2026-11-14", Track: "upsc", Text: "Yesterday."},
		{Day: "2026-11-15", Track: "upsc", Text: "Today."},
		{Day: "2026-11-15", Track: "engineering", Text: "Code."},
	} {
		if err := s.SaveIntro(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SaveIntro(ctx, store.Intro{Day: "2026-11-15", Track: "upsc", Text: "Today, revised."}); err != nil {
		t.Fatal(err)
	}

	if in, ok, _ := s.GetIntro(ctx, "", "upsc"); !ok || in.Day != "2026-11-15" || in.Text != "Today, revised." {
		t.Fatalf("latest = %+v", in)
	}
	if in, ok, _ := s.GetIntro(ctx, "2026-11-14", "upsc"); !ok || in.Text != "Yesterday." {
		t.Fatalf("by day = %+v", in)
	}
	if _, ok, _ := s.GetIntro(ctx, "2026-11-13", "upsc"); ok {
		t.Fatal("a day with no intro must not fall back to another")
	}
}

func TestMemoryIntros(t *testing.T) { introContract(t, store.NewMemory()) }

func TestPostgresIntros(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	introContract(t, s)
}

func proContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	if _, ok, err := s.ProByToken(ctx, "nope"); ok || err != nil {
		t.Fatalf("unknown token: ok=%v err=%v", ok, err)
	}

	m, fresh, err := s.ProExtend(ctx, "pay_1", "a@x.test", 31, "tok-a")
	if err != nil || !fresh || m.Token != "tok-a" || time.Until(m.Until) < 30*24*time.Hour || time.Until(m.Until) > 32*24*time.Hour {
		t.Fatalf("first payment = %+v, fresh %v, %v; want a 31 day member", m, fresh, err)
	}

	// The same payment again (Razorpay retries and sends two events) changes nothing.
	again, fresh, err := s.ProExtend(ctx, "pay_1", "a@x.test", 31, "tok-other")
	if err != nil || fresh || !again.Until.Equal(m.Until) || again.Token != "tok-a" {
		t.Fatalf("repeated payment = %+v, fresh %v, %v; want no change", again, fresh, err)
	}

	// A new payment while active adds to the end of the period and keeps the token.
	renewed, fresh, _ := s.ProExtend(ctx, "pay_2", "a@x.test", 31, "tok-third")
	if !fresh || renewed.Token != "tok-a" || renewed.Until.Sub(m.Until) < 30*24*time.Hour {
		t.Fatalf("renewal = %+v, fresh %v; want the period added to the old end", renewed, fresh)
	}

	if got, ok, _ := s.ProByToken(ctx, "tok-a"); !ok || got.Email != "a@x.test" {
		t.Fatalf("ProByToken = %+v, %v", got, ok)
	}
	active, err := s.ActiveProTokens(ctx)
	if err != nil || active["a@x.test"] != "tok-a" || len(active) != 1 {
		t.Fatalf("ActiveProTokens = %v, %v", active, err)
	}
}

func TestMemoryPro(t *testing.T) { proContract(t, store.NewMemory()) }

func TestPostgresPro(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	proContract(t, s)
}

func cardsContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	now := time.Now()
	save := func(hash, title string, published time.Time, imp int, exam *store.ExamLens) {
		t.Helper()
		if err := s.Save(ctx, store.Page{URLHash: hash, URL: "https://site.test/" + hash, Title: title, IsArticle: true, Published: published, ContentHash: hash}); err != nil {
			t.Fatal(err)
		}
		if err := s.SetEnrichment(ctx, hash, store.Enrichment{Why: "w", Importance: imp, Exam: exam}); err != nil {
			t.Fatal(err)
		}
	}
	save("c1", "Cards story", now.Add(-time.Hour), 4, &store.ExamLens{Prelims: 2, Cards: []store.Card{{Q: "Q1?", A: "A1"}, {Q: "Q2?", A: "A2"}}, Facts: []string{"F1"}})
	save("f1", "Facts only story", now.Add(-2*time.Hour), 5, &store.ExamLens{Prelims: 2, Facts: []string{"Fact one", "Fact two"}})
	save("n1", "Lens without cards or facts", now.Add(-time.Hour), 5, &store.ExamLens{Prelims: 1})
	save("t1", "No lens at all", now.Add(-time.Hour), 5, nil)
	save("old", "Old story", now.AddDate(0, 0, -20), 5, &store.ExamLens{Facts: []string{"Old fact"}})

	got, err := s.Cards(ctx, now.AddDate(0, 0, -14), 10)
	if err != nil || len(got) != 2 {
		t.Fatalf("Cards = %+v, %v; want the two stories with cards or facts inside the window", got, err)
	}
	if got[0].ID != "f1" || got[1].ID != "c1" {
		t.Fatalf("order = %s, %s; want the more important story first", got[0].ID, got[1].ID)
	}
	if len(got[1].Cards) != 2 || got[1].Cards[0].Q != "Q1?" || len(got[0].Facts) != 2 {
		t.Fatalf("cards or facts lost: %+v", got)
	}
	if one, _ := s.Cards(ctx, now.AddDate(0, 0, -14), 1); len(one) != 1 {
		t.Fatalf("limit ignored: %d", len(one))
	}
}

func TestMemoryCards(t *testing.T) { cardsContract(t, store.NewMemory()) }

func TestPostgresCards(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	cardsContract(t, s)
}

func compilationContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	if _, ok, err := s.GetCompilation(ctx, "2026-11", "upsc"); ok || err != nil {
		t.Fatalf("empty store: ok=%v err=%v", ok, err)
	}
	if months, err := s.CompilationMonths(ctx); err != nil || len(months) != 0 {
		t.Fatalf("months = %v, %v", months, err)
	}

	area := func(name, id string) store.CompArea {
		return store.CompArea{Name: name, Items: []store.CompItem{{ID: id, URL: "https://site.test/" + id, Title: "T " + id, Source: "site.test", Why: "w", Bullets: []string{"b"}, Tags: []string{"x"}, Importance: 4, Time: time.Now().UTC().Truncate(time.Second)}}}
	}
	for _, c := range []store.Compilation{
		{Month: "2026-10", Track: "upsc", Areas: []store.CompArea{area("Economy (GS3)", "a")}},
		{Month: "2026-11", Track: "upsc", Areas: []store.CompArea{area("Economy (GS3)", "b")}},
		{Month: "2026-11", Track: "engineering", Areas: []store.CompArea{area("Kubernetes", "c")}},
	} {
		if err := s.SaveCompilation(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	// Saving again replaces.
	if err := s.SaveCompilation(ctx, store.Compilation{Month: "2026-11", Track: "upsc", Areas: []store.CompArea{area("Polity", "d")}}); err != nil {
		t.Fatal(err)
	}

	got, ok, err := s.GetCompilation(ctx, "2026-11", "upsc")
	if err != nil || !ok || len(got.Areas) != 1 || got.Areas[0].Name != "Polity" || got.Areas[0].Items[0].ID != "d" || got.Built.IsZero() {
		t.Fatalf("GetCompilation = %+v, %v, %v; want the replacement", got, ok, err)
	}
	if got.Areas[0].Items[0].Bullets[0] != "b" || got.Areas[0].Items[0].Tags[0] != "x" {
		t.Fatalf("an item lost its fields: %+v", got.Areas[0].Items[0])
	}
	months, _ := s.CompilationMonths(ctx)
	if len(months) != 2 || months[0] != "2026-11" || months[1] != "2026-10" {
		t.Fatalf("months = %v, want [2026-11 2026-10]", months)
	}
}

func TestMemoryCompilations(t *testing.T) { compilationContract(t, store.NewMemory()) }

func TestPostgresCompilations(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	compilationContract(t, s)
}

func leadContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	content := "Subscribe to our newsletter. Bangladesh has completed the legal procedures required to secure the return of ousted Prime Minister Sheikh Hasina from India, the foreign ministry said. " +
		"“We have completed everything required from our side as part of the legal process,” Prothom Alo quoted him as saying."
	if err := s.Save(ctx, store.Page{URLHash: "lead1", URL: "https://site.test/lead1", Title: "A story", Description: "A description cut mid-wo", IsArticle: true, Published: time.Now().Add(-time.Hour), Content: content, ContentHash: "lead1"}); err != nil {
		t.Fatal(err)
	}
	it, ok, err := s.Item(ctx, "lead1")
	if err != nil || !ok {
		t.Fatalf("Item = %v, %v", ok, err)
	}
	if !strings.HasPrefix(it.Lead, "Bangladesh has completed") || strings.Contains(it.Lead, "Subscribe") || !strings.HasSuffix(it.Lead, "as saying.") {
		t.Fatalf("Lead = %q, want the whole opening sentences without the page chrome", it.Lead)
	}

	// A digest list does not carry the lead: it is for the story page only.
	items, _, _ := s.Digest(ctx, store.DigestQuery{Since: time.Now().Add(-24 * time.Hour), ArticlesOnly: true, Limit: 10})
	if len(items) != 1 || items[0].Lead != "" {
		t.Fatalf("digest items = %+v, want no lead", items)
	}
}

func TestMemoryLead(t *testing.T) { leadContract(t, store.NewMemory()) }

func TestPostgresLead(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	leadContract(t, s)
}

func conceptsContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	if err := s.AddConceptTerms(ctx, []string{"Repo rate", "writ", "Repo Rate"}); err != nil { // the repeat differs only in case
		t.Fatal(err)
	}
	pending, err := s.PendingConcepts(ctx, 10)
	if err != nil || len(pending) != 2 || pending[0] != "Repo rate" || pending[1] != "writ" {
		t.Fatalf("PendingConcepts = %v, %v; want the two terms once, oldest first", pending, err)
	}
	if one, _ := s.PendingConcepts(ctx, 1); len(one) != 1 {
		t.Fatalf("limit ignored: %v", one)
	}

	if err := s.SetConcept(ctx, "repo rate", "The rate at which the central bank lends to banks."); err != nil {
		t.Fatal(err)
	}
	if pending, _ := s.PendingConcepts(ctx, 10); len(pending) != 1 || pending[0] != "writ" {
		t.Fatalf("pending after one is written = %v, want only writ", pending)
	}
	// A term that was explained is not queued again.
	_ = s.AddConceptTerms(ctx, []string{"Repo rate"})
	if pending, _ := s.PendingConcepts(ctx, 10); len(pending) != 1 {
		t.Fatalf("an explained term was queued again: %v", pending)
	}
	all, err := s.Concepts(ctx, 10)
	if err != nil || len(all) != 1 || all[0].Term != "Repo rate" || all[0].Body == "" {
		t.Fatalf("Concepts = %+v, %v; want only the explained term with its first spelling", all, err)
	}
}

func TestMemoryConcepts(t *testing.T) { conceptsContract(t, store.NewMemory()) }

func TestPostgresConcepts(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	conceptsContract(t, s)
}

func entitiesContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	now := time.Now()
	for hash, e := range map[string]struct {
		title    string
		age      time.Duration
		entities []string
	}{
		"e1": {"One", time.Hour, []string{"Sheikh Hasina", "India"}},
		"e2": {"Two", 2 * time.Hour, []string{"India"}},
		"e3": {"None", time.Hour, nil},
		"e4": {"Old", 30 * 24 * time.Hour, []string{"India"}},
	} {
		if err := s.Save(ctx, store.Page{URLHash: hash, URL: "https://site.test/" + hash, Title: e.title, IsArticle: true, Published: now.Add(-e.age), ContentHash: hash}); err != nil {
			t.Fatal(err)
		}
		if err := s.SetEnrichment(ctx, hash, store.Enrichment{Why: "w " + hash, Importance: 3, Entities: e.entities}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.EntityStories(ctx, now.AddDate(0, 0, -14))
	if err != nil || len(got) != 2 || got[0].ID != "e1" || got[1].ID != "e2" || got[0].Why != "w e1" || len(got[0].Entities) != 2 {
		t.Fatalf("EntityStories = %+v, %v; want e1 then e2 with their entities", got, err)
	}
	it, ok, _ := s.Item(ctx, "e1")
	if !ok || len(it.Entities) != 2 || it.Entities[0] != "Sheikh Hasina" {
		t.Fatalf("Item entities = %v, want them on the story", it.Entities)
	}
}

func TestMemoryEntities(t *testing.T) { entitiesContract(t, store.NewMemory()) }

func TestPostgresEntities(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	entitiesContract(t, s)
}

func hooksContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	if added, err := s.AddTeamHook(ctx, "slack", "https://hooks.slack.com/services/T/B/x", "engineering", "tok1"); err != nil || !added {
		t.Fatalf("first hook: added=%v err=%v", added, err)
	}
	if added, _ := s.AddTeamHook(ctx, "slack", "https://hooks.slack.com/services/T/B/x", "upsc", "tok2"); added {
		t.Fatal("the same address was registered twice")
	}
	if added, _ := s.AddTeamHook(ctx, "discord", "https://discord.com/api/webhooks/1/y", "upsc", "tok3"); !added {
		t.Fatal("a second address was refused")
	}
	hooks, err := s.TeamHooks(ctx)
	if err != nil || len(hooks) != 2 || hooks[0].Track != "engineering" || hooks[0].Kind != "slack" || hooks[1].Token != "tok3" {
		t.Fatalf("TeamHooks = %+v, %v; want both, oldest first, the first unchanged by the repeat", hooks, err)
	}
	if ok, _ := s.RemoveTeamHook(ctx, "nope"); ok {
		t.Fatal("an unknown token removed something")
	}
	if ok, _ := s.RemoveTeamHook(ctx, "tok1"); !ok {
		t.Fatal("a known token removed nothing")
	}
	if hooks, _ := s.TeamHooks(ctx); len(hooks) != 1 || hooks[0].Token != "tok3" {
		t.Fatalf("after removal = %+v", hooks)
	}
}

func TestMemoryTeamHooks(t *testing.T) { hooksContract(t, store.NewMemory()) }

func TestPostgresTeamHooks(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	hooksContract(t, s)
}

// Editorials are detailed first, whatever their score, so they always get the long summary and lens.
func detailOrderContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	now := time.Now()
	hash := map[string]string{}
	for i, c := range []struct {
		cat string
		imp int
	}{{"india", 5}, {"editorial", 1}, {"india", 4}} {
		p := store.Page{URLHash: fmt.Sprintf("d%d", i), URL: fmt.Sprintf("https://x.test/d%d", i), Title: "T", IsArticle: true, Category: c.cat, Published: now.Add(-time.Minute), Content: "body", ContentHash: fmt.Sprintf("d%d", i)}
		if err := s.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
		if err := s.SetEnrichment(ctx, p.URLHash, store.Enrichment{Why: "w", Importance: c.imp}); err != nil {
			t.Fatal(err)
		}
		hash[c.cat+fmt.Sprint(c.imp)] = p.URLHash
	}
	got, err := s.PendingDetail(ctx, now.Add(-time.Hour), 2)
	if err != nil || len(got) != 2 || got[0].URLHash != hash["editorial1"] || got[1].URLHash != hash["india5"] {
		t.Fatalf("PendingDetail = %+v, %v; want the editorial first, then importance 5", got, err)
	}
}

func TestMemoryDetailOrder(t *testing.T) { detailOrderContract(t, store.NewMemory()) }

func TestPostgresDetailOrder(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	detailOrderContract(t, s)
}

// A picture shared by many articles of one site is that site's logo and is hidden; the same picture
// run by many different sites (a wire photo) is a real picture and is kept. ImageStats counts both.
func imageRuleContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	now := time.Now()
	save := func(id, host, og, body string) {
		p := store.Page{URLHash: id, URL: "https://" + host + "/" + id, Title: id, IsArticle: true, Image: og, BodyImage: body, Published: now.Add(-time.Minute), Content: "x", ContentHash: id}
		if err := s.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 6; i++ { // one site, one logo
		save(fmt.Sprintf("logo%d", i), "one.test", "https://one.test/logo.jpg", "")
	}
	for i := 0; i < 6; i++ { // six sites, one wire photo
		save(fmt.Sprintf("wire%d", i), fmt.Sprintf("site%d.test", i), "https://wire.test/photo.jpg", "")
	}
	save("bare", "bare.test", "", "")

	if got, _ := s.ImageURL(ctx, "logo0"); got != "" {
		t.Errorf("a logo shared by six articles of one site is shown: %q", got)
	}
	if got, _ := s.ImageURL(ctx, "wire0"); got != "https://wire.test/photo.jpg" {
		t.Errorf("a photo run by six sites is hidden: %q", got)
	}
	c, err := s.ImageStats(ctx, now.Add(-time.Hour))
	if err != nil || c != (store.ImageCoverage{Shown: 6, Logo: 6, None: 1}) {
		t.Fatalf("ImageStats = %+v, %v; want 6 shown, 6 logo-only, 1 none", c, err)
	}
}

func TestMemoryImageRule(t *testing.T) { imageRuleContract(t, store.NewMemory()) }

func TestPostgresImageRule(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	imageRuleContract(t, s)
}

// The article length survives a save and comes back from the digest and from Item, in both stores, and a
// re-crawl that changes only the length updates it.
func wordsContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	p := store.Page{URLHash: "w1", URL: "https://x.test/w1", Title: "T", IsArticle: true, Published: time.Now().Add(-time.Minute), Content: "text", ContentHash: "h", Words: 900}
	if err := s.Save(ctx, p); err != nil {
		t.Fatal(err)
	}
	items, _, err := s.Digest(ctx, store.DigestQuery{Since: time.Now().Add(-time.Hour), ArticlesOnly: true, Limit: 10})
	if err != nil || len(items) != 1 || items[0].Words != 900 {
		t.Fatalf("Digest = %+v, %v; want Words 900", items, err)
	}
	if it, ok, _ := s.Item(ctx, "w1"); !ok || it.Words != 900 {
		t.Fatalf("Item.Words = %d, want 900", it.Words)
	}
	p.Words = 1200 // same content hash: only the length changed
	if err := s.Save(ctx, p); err != nil {
		t.Fatal(err)
	}
	if it, _, _ := s.Item(ctx, "w1"); it.Words != 1200 {
		t.Fatalf("after a re-crawl Item.Words = %d, want 1200", it.Words)
	}
}

func TestMemoryWords(t *testing.T) { wordsContract(t, store.NewMemory()) }

func TestPostgresWords(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	wordsContract(t, s)
}
