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
	coverage, story := map[string]int{}, map[string]string{}
	for _, it := range items {
		coverage[it.ID], story[it.ID] = it.Coverage, it.Story
	}
	if coverage["a"] != 2 || coverage["b"] != 2 || coverage["c"] != 0 {
		t.Fatalf("digest coverage = %v", coverage)
	}
	// Every member of a group carries the group's id, so a list can show the event once; a lone story has none.
	if story["a"] != "a" || story["b"] != "a" || story["c"] != "" {
		t.Fatalf("digest story ids = %v, want a and b in group a and c alone", story)
	}
	if it.Story != "a" || lone0(t, s, "c") != "" {
		t.Fatalf("Item story ids: b = %q, c = %q, want a and none", it.Story, lone0(t, s, "c"))
	}

	// A later run that no longer finds the group clears it.
	if err := s.SetStories(ctx, since, map[string]store.StoryGroup{}); err != nil {
		t.Fatal(err)
	}
	if it, _, _ := s.Item(ctx, "b"); it.Coverage != 0 || len(it.Also) != 0 {
		t.Fatalf("stale group kept: %+v", it)
	}
}

func lone0(t *testing.T, s store.Store, id string) string {
	t.Helper()
	it, ok, err := s.Item(context.Background(), id)
	if err != nil || !ok {
		t.Fatalf("Item %s = %v, %v", id, ok, err)
	}

	return it.Story
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

	if m.Renewed || !renewed.Renewed {
		t.Fatalf("Renewed: first payment %v, second %v; want false then true", m.Renewed, renewed.Renewed)
	}

	if got, ok, _ := s.ProByToken(ctx, "tok-a"); !ok || got.Email != "a@x.test" {
		t.Fatalf("ProByToken = %+v, %v", got, ok)
	}
	active, err := s.ActiveProTokens(ctx)
	if err != nil || active["a@x.test"] != "tok-a" || len(active) != 1 {
		t.Fatalf("ActiveProTokens = %v, %v", active, err)
	}

	// Reminders: a member whose period ends soon comes up once, and a new period brings a new reminder.
	if soon, err := s.ProExpiring(ctx, 10*24*time.Hour); err != nil || len(soon) != 0 {
		t.Fatalf("ProExpiring far from the end = %v, %v; want none", soon, err)
	}
	if _, _, err := s.ProExtend(ctx, "pay_s", "soon@x.test", 2, "tok-soon"); err != nil {
		t.Fatal(err)
	}
	due, err := s.ProExpiring(ctx, 3*24*time.Hour)
	if err != nil || len(due) != 1 || due[0].Email != "soon@x.test" {
		t.Fatalf("ProExpiring = %v, %v; want soon@x.test", due, err)
	}
	if err := s.ProMarkReminded(ctx, due[0].Email, due[0].Until); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.ProExpiring(ctx, 3*24*time.Hour); len(again) != 0 {
		t.Fatalf("ProExpiring after the reminder = %v; want none", again)
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
	save := func(hash, title string, published time.Time, imp int, exam *store.StudyCards) {
		t.Helper()
		if err := s.Save(ctx, store.Page{URLHash: hash, URL: "https://site.test/" + hash, Title: title, IsArticle: true, Published: published, ContentHash: hash}); err != nil {
			t.Fatal(err)
		}
		if err := s.SetEnrichment(ctx, hash, store.Enrichment{Why: "w", Importance: imp, Study: exam}); err != nil {
			t.Fatal(err)
		}
	}
	save("c1", "Cards story", now.Add(-time.Hour), 4, &store.StudyCards{Cards: []store.Card{{Q: "Q1?", A: "A1"}, {Q: "Q2?", A: "A2"}}, Facts: []string{"F1"}})
	save("f1", "Facts only story", now.Add(-2*time.Hour), 5, &store.StudyCards{Facts: []string{"Fact one", "Fact two"}})
	save("n1", "Lens without cards or facts", now.Add(-time.Hour), 5, &store.StudyCards{})
	save("t1", "No lens at all", now.Add(-time.Hour), 5, nil)
	save("old", "Old story", now.AddDate(0, 0, -20), 5, &store.StudyCards{Facts: []string{"Old fact"}})
	// An exam-category story without a lens yet still gives one card, from its why line; a tech story does not.
	for hash, category := range map[string]string{"u1": "india", "u2": "technology"} {
		if err := s.Save(ctx, store.Page{URLHash: hash, URL: "https://site.test/" + hash, Title: "No lens " + hash, Category: category, IsArticle: true, Published: now.Add(-time.Hour), ContentHash: hash}); err != nil {
			t.Fatal(err)
		}
		if err := s.SetEnrichment(ctx, hash, store.Enrichment{Why: "It matters.", Importance: 2}); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.Cards(ctx, now.AddDate(0, 0, -14), 10)
	if err != nil || len(got) != 3 {
		t.Fatalf("Cards = %+v, %v; want the two stories with cards or facts and the exam story with a why line", got, err)
	}
	if got[0].ID != "f1" || got[1].ID != "c1" || got[2].ID != "u1" || got[2].Why != "It matters." {
		t.Fatalf("order = %s, %s, %s; want the more important story first, the why-only story last", got[0].ID, got[1].ID, got[2].ID)
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
	first, announce, err := s.ConnectTeamHook(ctx, "slack", "https://hooks.slack.com/services/T/B/x", "engineering", "tok1")
	if err != nil || !announce || first.Token != "tok1" || first.ID == 0 {
		t.Fatalf("first hook: %+v announce=%v err=%v", first, announce, err)
	}
	// The same address and edition again changes nothing and announces nothing.
	if h, announce, _ := s.ConnectTeamHook(ctx, "slack", "https://hooks.slack.com/services/T/B/x", "engineering", "tok2"); announce || h.Token != "tok1" || h.ID != first.ID {
		t.Fatalf("repeat join: %+v announce=%v, want the first hook and no announcement", h, announce)
	}
	// Another edition for the same address keeps the token and announces.
	if h, announce, _ := s.ConnectTeamHook(ctx, "slack", "https://hooks.slack.com/services/T/B/x", "upsc", "tok2"); !announce || h.Token != "tok1" || h.Track != "upsc" {
		t.Fatalf("edition change: %+v announce=%v", h, announce)
	}
	if _, announce, _ := s.ConnectTeamHook(ctx, "discord", "https://discord.com/api/webhooks/1/y", "upsc", "tok3"); !announce {
		t.Fatal("a second address was refused")
	}
	hooks, err := s.TeamHooks(ctx)
	if err != nil || len(hooks) != 2 || hooks[0].Track != "upsc" || hooks[0].Kind != "slack" || hooks[1].Token != "tok3" {
		t.Fatalf("TeamHooks = %+v, %v; want both, oldest first, the first with its new edition", hooks, err)
	}

	// Refusals add up, a good post clears them, and a repeat join clears them too.
	for want := 1; want <= 2; want++ {
		if n, err := s.RecordTeamPost(ctx, first.ID, false); err != nil || n != want {
			t.Fatalf("refusal %d counted %d, %v", want, n, err)
		}
	}
	if n, _ := s.RecordTeamPost(ctx, first.ID, true); n != 0 {
		t.Fatalf("a good post left %d refusals", n)
	}
	_, _ = s.RecordTeamPost(ctx, first.ID, false)
	if h, _, _ := s.ConnectTeamHook(ctx, "slack", "https://hooks.slack.com/services/T/B/x", "upsc", "tok9"); h.Fails != 0 {
		t.Fatalf("a repeat join left %d refusals", h.Fails)
	}
	if n, err := s.RecordTeamPost(ctx, 987654, false); err != nil || n != 0 {
		t.Fatalf("an unknown hook counted %d, %v", n, err)
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

// Editorials are detailed first, whatever their score, so they always get the long summary and lens; then
// the exam categories, whose lens feeds the flashcards, ahead of other categories.
func detailOrderContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	now := time.Now()
	hash := map[string]string{}
	for i, c := range []struct {
		cat string
		imp int
	}{{"technology", 5}, {"india", 5}, {"editorial", 1}, {"india", 4}} {
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
		t.Fatalf("PendingDetail = %+v, %v; want the editorial first, then the exam story of importance 5 before a tech story of 5", got, err)
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

// proNotesContract runs the same checks on every Store: a member's notes are kept apart from other members',
// come back newest first, are capped, can be replaced, imported without overwriting, and deleted one by one or all.
func proNotesContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	for _, email := range []string{"a@x.test", "b@x.test"} {
		if _, _, err := s.ProExtend(ctx, "pay-"+email, email, 30, "tok-"+email); err != nil {
			t.Fatal(err)
		}
	}
	note := func(id string, at int64) store.ProNote {
		return store.ProNote{ID: id, Story: "s1", Title: "Title", Text: "Sentence " + id, At: at}
	}

	for _, n := range []store.ProNote{note("n1", 100), note("n2", 300), note("n3", 200)} {
		if ok, err := s.SaveProNote(ctx, "a@x.test", n, 3); err != nil || !ok {
			t.Fatalf("save %s = %v, %v", n.ID, ok, err)
		}
	}
	got, err := s.ProNotes(ctx, "a@x.test")
	if err != nil || len(got) != 3 || got[0].ID != "n2" || got[1].ID != "n3" || got[2].ID != "n1" {
		t.Fatalf("notes = %+v, %v; want newest first: n2, n3, n1", got, err)
	}
	if other, _ := s.ProNotes(ctx, "b@x.test"); len(other) != 0 {
		t.Fatalf("another member's notes leaked: %+v", other)
	}

	// At the cap a new note is refused, but an existing one can still be edited.
	if ok, _ := s.SaveProNote(ctx, "a@x.test", note("n4", 400), 3); ok {
		t.Fatal("a new note over the cap must be refused")
	}
	edited := note("n1", 100)
	edited.Note, edited.Tags = "my thought", []string{"economy", "exam"}
	if ok, err := s.SaveProNote(ctx, "a@x.test", edited, 3); err != nil || !ok {
		t.Fatalf("editing at the cap = %v, %v; want allowed", ok, err)
	}
	got, _ = s.ProNotes(ctx, "a@x.test")
	if got[2].Note != "my thought" || len(got[2].Tags) != 2 || got[2].Tags[0] != "economy" || len(got) != 3 {
		t.Fatalf("edit not stored: %+v", got)
	}

	// Import adds only what is missing and never overwrites an edit.
	n, err := s.AddProNotes(ctx, "b@x.test", []store.ProNote{note("n1", 1), note("n2", 2), note("n1", 9)}, 10)
	if err != nil || n != 2 {
		t.Fatalf("import = %d, %v; want 2 (the repeat is not added twice)", n, err)
	}
	if n, _ := s.AddProNotes(ctx, "a@x.test", []store.ProNote{{ID: "n1", Story: "s", Title: "t", Text: "overwritten", At: 1}, note("n9", 9)}, 3); n != 0 {
		t.Fatalf("import over the cap added %d, want 0", n)
	}
	got, _ = s.ProNotes(ctx, "a@x.test")
	if got[2].Text != "Sentence n1" {
		t.Fatalf("an import overwrote an existing note: %+v", got[2])
	}

	if n, _ := s.DeleteProNote(ctx, "a@x.test", "n2"); n != 1 {
		t.Fatalf("delete one = %d, want 1", n)
	}
	if n, _ := s.DeleteProNote(ctx, "a@x.test", "n2"); n != 0 {
		t.Fatalf("delete again = %d, want 0", n)
	}
	if n, _ := s.DeleteProNote(ctx, "a@x.test", ""); n != 2 {
		t.Fatalf("delete all = %d, want 2", n)
	}
	if left, _ := s.ProNotes(ctx, "b@x.test"); len(left) != 2 {
		t.Fatalf("deleting a's notes touched b's: %+v", left)
	}
}

func TestMemoryProNotes(t *testing.T) { proNotesContract(t, store.NewMemory()) }

func TestPostgresProNotes(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	proNotesContract(t, s)
}

func sourceRunsContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	now := time.Now()
	runs := []store.SourceRun{
		{Source: "alpha", Host: "a.test", At: now.AddDate(0, 0, -5), Pages: 9, Articles: 4, Outcome: "ok"},
		{Source: "alpha", Host: "a.test", At: now.AddDate(0, 0, -1), Pages: 8, Articles: 0, Failed: 2, Outcome: "empty"},
		{Source: "beta", Host: "b.test", At: now.AddDate(0, 0, -1), Pages: 0, Articles: 0, Failed: 3, Outcome: "failed"},
		{Source: "old", Host: "o.test", At: now.AddDate(0, 0, -90), Pages: 1, Articles: 1, Outcome: "ok"},
	}
	if err := s.RecordSourceRuns(ctx, runs); err != nil {
		t.Fatal(err)
	}

	got, err := s.SourceHealth(ctx, now.AddDate(0, 0, -7), now.AddDate(0, 0, -3))
	if err != nil || len(got) != 2 || got[0].Source != "alpha" || got[1].Source != "beta" {
		t.Fatalf("SourceHealth = %+v, %v; want alpha and beta (the 90-day-old run is deleted)", got, err)
	}
	a, b := got[0], got[1]
	if a.Runs != 2 || a.Articles != 4 || a.RecentArticles != 0 || a.Failed != 2 || a.LastOutcome != "empty" || a.LastOK.IsZero() || a.Host != "a.test" {
		t.Fatalf("alpha = %+v; want 2 runs, 4 articles (none recent), last run empty, last ok set", a)
	}
	if b.LastOutcome != "failed" || !b.LastOK.IsZero() || b.Failed != 3 {
		t.Fatalf("beta = %+v; want failed and never ok", b)
	}

	// Articles per crawl: only article pages, only the crawls asked for.
	for i, c := range []struct {
		crawl   int64
		article bool
	}{{7, true}, {7, true}, {7, false}, {8, true}} {
		h := fmt.Sprintf("src%d", i)
		if err := s.Save(ctx, store.Page{URLHash: h, CrawlID: c.crawl, URL: "https://x.test/" + h, IsArticle: c.article, ContentHash: h}); err != nil {
			t.Fatal(err)
		}
	}
	counts, err := s.ArticlesByCrawl(ctx, []int64{7, 9})
	if err != nil || counts[7] != 2 || counts[8] != 0 || counts[9] != 0 {
		t.Fatalf("ArticlesByCrawl = %v, %v; want crawl 7 with 2", counts, err)
	}
	if none, err := s.ArticlesByCrawl(ctx, nil); err != nil || len(none) != 0 {
		t.Fatalf("no crawls = %v, %v", none, err)
	}
}

func TestMemorySourceRuns(t *testing.T) { sourceRunsContract(t, store.NewMemory()) }

func TestPostgresSourceRuns(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	sourceRunsContract(t, s)
}

// extraContract: the story extras round-trip through SetEnrichment and Item, an empty extra stores nothing, the
// Week ahead query keeps only dated events inside its window in date order, and stage 1 gets the stored gist.
func extraContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	now := time.Now()
	for i, h := range []string{"a", "b", "c", "d", "e"} {
		if err := s.Save(ctx, store.Page{URLHash: h, URL: "https://site.test/" + h, Title: "Story " + h, IsArticle: true, Content: strings.Repeat("Body. ", 40),
			ContentHash: h, Published: now.Add(-time.Duration(i+1) * time.Hour)}); err != nil {
			t.Fatal(err)
		}
		if err := s.SetSummary(ctx, h, []string{"Gist of " + h + "."}); err != nil {
			t.Fatal(err)
		}
	}

	pending, err := s.PendingEnrichment(ctx, now.Add(-24*time.Hour), 10)
	if err != nil || len(pending) != 5 {
		t.Fatalf("PendingEnrichment = %d, %v", len(pending), err)
	}
	for _, u := range pending {
		if len(u.Summary) != 1 || u.Summary[0] != "Gist of "+u.URLHash+"." {
			t.Fatalf("pending %s carries summary %v, want the stored gist", u.URLHash, u.Summary)
		}
	}

	next := func(text, date string) *store.Extra {
		return &store.Extra{Next: &store.NextEvent{Text: text, Date: date}}
	}
	for h, x := range map[string]*store.Extra{
		"a": {Context: "What led to it.", Impact: "Borrowers", Next: &store.NextEvent{Text: "MPC meets", Date: "2026-12-05"}},
		"b": next("Hearing", "2026-12-01"),
		"c": next("Far away", "2027-06-01"),
		"d": {},
		"e": next("No date yet", ""),
	} {
		if err := s.SetEnrichment(ctx, h, store.Enrichment{Why: "w", Importance: 3, Extra: x}); err != nil {
			t.Fatal(err)
		}
	}

	it, ok, err := s.Item(ctx, "a")
	if err != nil || !ok || it.Extra == nil || it.Extra.Context != "What led to it." || it.Extra.Impact != "Borrowers" || it.Extra.Next == nil || it.Extra.Next.Date != "2026-12-05" {
		t.Fatalf("Item a = %+v ok=%v err=%v, want its extras", it.Extra, ok, err)
	}
	if it, _, _ := s.Item(ctx, "d"); it.Extra != nil {
		t.Fatalf("an empty extra must read back as none, got %+v", it.Extra)
	}

	got, err := s.Ahead(ctx, "2026-11-30", "2026-12-31", 10)
	if err != nil || len(got) != 2 || got[0].ID != "b" || got[1].ID != "a" || got[0].Date != "2026-12-01" || got[1].Text != "MPC meets" || got[0].Source != "site.test" {
		t.Fatalf("Ahead = %+v, %v; want b (1 Dec) then a (5 Dec) only", got, err)
	}
	if got, _ := s.Ahead(ctx, "2026-11-30", "2026-12-31", 1); len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("Ahead with limit 1 = %+v, want the soonest", got)
	}
	if got, err := s.Ahead(ctx, "2030-01-01", "2030-01-31", 10); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("an empty window = %#v, %v, want an empty list, not nil", got, err)
	}

	// Re-enriching with no extra clears an old one: the story page must not show stale background.
	if err := s.SetEnrichment(ctx, "a", store.Enrichment{Why: "w2", Importance: 3}); err != nil {
		t.Fatal(err)
	}
	if it, _, _ := s.Item(ctx, "a"); it.Extra != nil {
		t.Fatalf("extras after a re-enrichment without them = %+v, want none", it.Extra)
	}
}

func TestMemoryExtra(t *testing.T) { extraContract(t, store.NewMemory()) }

func TestPostgresExtra(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	extraContract(t, s)
}

func TestPostgresStorageListsTheBiggestTables(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	st, err := s.Storage(context.Background())
	if err != nil || st.DBBytes <= 0 || len(st.Tables) == 0 || len(st.Tables) > 8 {
		t.Fatalf("Storage = %+v, %v; want a size and up to 8 tables", st, err)
	}
	var seenPages bool
	for i, tb := range st.Tables {
		if tb.Name == "pages" {
			seenPages = true
		}
		if tb.Bytes <= 0 || (i > 0 && tb.Bytes > st.Tables[i-1].Bytes) {
			t.Fatalf("tables = %+v, want positive sizes, biggest first", st.Tables)
		}
	}
	if !seenPages {
		t.Fatalf("tables = %+v, want the pages table among them", st.Tables)
	}
}
