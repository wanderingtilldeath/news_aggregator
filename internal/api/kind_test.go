package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

func TestDigestAndStoryCarryTheKind(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for _, p := range []store.Page{
		{URLHash: "op", URL: "https://www.site.test/opinion/columns/why-it-matters", Title: "Why it matters", IsArticle: true, Published: time.Now().Add(-time.Hour)},
		{URLHash: "rep", URL: "https://www.site.test/news/india/a-report", Title: "A report", IsArticle: true, Published: time.Now().Add(-time.Hour)},
	} {
		if err := f.store.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	_, body, _ := f.do(t, "GET", "/digest", "")
	kinds := map[string]any{}
	for _, it := range body["items"].([]any) {
		m := it.(map[string]any)
		kinds[m["id"].(string)] = m["kind"]
	}
	if kinds["op"] != "opinion" || kinds["rep"] != nil {
		t.Fatalf("kinds = %v, want opinion for the column and none for the report", kinds)
	}

	if _, body, _ := f.do(t, "GET", "/story/op", ""); body["kind"] != "opinion" {
		t.Fatalf("story kind = %v", body["kind"])
	}
}

func TestIntroEndpoint(t *testing.T) {
	f := newFixture(t)
	_ = f.store.SaveIntro(context.Background(), store.Intro{Day: "2026-11-15", Track: "upsc", Text: "A paragraph."})

	status, body, hdr := f.do(t, "GET", "/intro?track=upsc", "")
	if status != 200 || body["text"] != "A paragraph." || body["day"] != "2026-11-15" || hdr.Get("Cache-Control") == "" {
		t.Fatalf("intro = %d %v %v", status, body, hdr)
	}
	for path, want := range map[string]int{
		"/intro?track=upsc&day=2026-11-14": 404, "/intro?track=engineering": 404,
		"/intro?track=bogus": 400, "/intro?track=upsc&day=nope": 400, "/intro": 400,
	} {
		if status, _, _ := f.do(t, "GET", path, ""); status != want {
			t.Errorf("%s = %d, want %d", path, status, want)
		}
	}
}

func TestStoryAlsoCarriesTheKindOfEachSource(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()
	for _, p := range []store.Page{
		{URLHash: "n1", URL: "https://www.theguardian.com/world/a", Title: "Donbas plan floated", IsArticle: true, Published: now.Add(-2 * time.Hour)},
		{URLHash: "o1", URL: "https://news.un.org/en/story/b", Title: "Donbas plan reported", IsArticle: true, Published: now.Add(-time.Hour)},
	} {
		if err := f.store.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	_ = f.store.SetStories(ctx, now.Add(-24*time.Hour), map[string]store.StoryGroup{"n1": {ID: "n1", Sources: 2}, "o1": {ID: "n1", Sources: 2}})

	_, body, _ := f.do(t, "GET", "/story/n1", "")
	also := body["also"].([]any)
	if len(also) != 1 || also[0].(map[string]any)["source_type"] != "official" || body["source_type"] != "news" {
		t.Fatalf("story = %v, want the UN story as official and this one as news", body)
	}
}

func TestCardsEndpointBuildsTheDeck(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()
	for hash, study := range map[string]*store.StudyCards{
		"s1": {Cards: []store.Card{{Q: "Q1?", A: "A1"}, {Q: "Q2?", A: "A2"}}},
		"s2": {Facts: []string{"A fact"}},
	} {
		_ = f.store.Save(ctx, store.Page{URLHash: hash, URL: "https://site.test/" + hash, Title: "Story " + hash + " with a long enough headline | Site", IsArticle: true, Published: now.Add(-time.Hour)})
		_ = f.store.SetEnrichment(ctx, hash, store.Enrichment{Importance: 3, Study: study})
	}

	status, body, hdr := f.do(t, "GET", "/cards", "")
	cards, _ := body["cards"].([]any)
	if status != 200 || len(cards) != 3 || hdr.Get("Cache-Control") == "" {
		t.Fatalf("cards = %d %v", status, body)
	}
	ids := map[string]bool{}
	for _, c := range cards {
		m := c.(map[string]any)
		ids[m["id"].(string)] = true
		if m["story"] == nil || m["title"] == "" || m["q"] == "" || m["a"] == "" {
			t.Fatalf("card = %v", m)
		}
		if title := m["title"].(string); title[len(title)-4:] == "Site" {
			t.Fatalf("the site tail stayed in %q", title)
		}
	}
	for _, want := range []string{"s1:0", "s1:1", "s2:0"} {
		if !ids[want] {
			t.Fatalf("ids = %v, want %s (positions count per story)", ids, want)
		}
	}
	for _, q := range []string{"days=0", "days=31", "limit=0", "limit=9999", "limit=x"} {
		if status, _, _ := f.do(t, "GET", "/cards?"+q, ""); status != 400 {
			t.Errorf("/cards?%s = %d, want 400", q, status)
		}
	}
}

func TestMonthEndpoints(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_ = f.store.SaveCompilation(ctx, store.Compilation{Month: "2026-11", Track: "upsc", Areas: []store.CompArea{{Name: "Economy (GS3)", Items: []store.CompItem{{ID: "a", Title: "T"}}}}})

	status, body, hdr := f.do(t, "GET", "/month?month=2026-11&track=upsc", "")
	areas, _ := body["areas"].([]any)
	if status != 200 || len(areas) != 1 || body["month"] != "2026-11" || hdr.Get("Cache-Control") == "" {
		t.Fatalf("month = %d %v", status, body)
	}
	for path, want := range map[string]int{
		"/month?month=2026-10&track=upsc": 404, "/month?month=2026-13&track=upsc": 400, "/month?month=nope&track=upsc": 400,
		"/month?track=upsc": 400, "/month?month=2026-11&track=bogus": 400, "/month?month=2026-11": 400,
	} {
		if status, _, _ := f.do(t, "GET", path, ""); status != want {
			t.Errorf("%s = %d, want %d", path, status, want)
		}
	}

	_, body, _ = f.do(t, "GET", "/months", "")
	if months, _ := body["months"].([]any); len(months) != 1 || months[0] != "2026-11" {
		t.Fatalf("months = %v", body)
	}
}

func TestDiscoveryEndpoints(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	_ = f.store.AddConceptTerms(ctx, []string{"repo rate", "writ"})
	_ = f.store.SetConcept(ctx, "repo rate", "The lending rate.")
	_, body, _ := f.do(t, "GET", "/concepts", "")
	if cs, _ := body["concepts"].([]any); len(cs) != 1 || cs[0].(map[string]any)["term"] != "repo rate" {
		t.Fatalf("concepts = %v, want only the explained term", body)
	}

	for hash, ents := range map[string][]string{"g1": {"India", "RBI"}, "g2": {"India"}} {
		_ = f.store.Save(ctx, store.Page{URLHash: hash, URL: "https://site.test/" + hash, Title: "Story " + hash, IsArticle: true, Published: now.Add(-time.Hour)})
		_ = f.store.SetEnrichment(ctx, hash, store.Enrichment{Entities: ents, Importance: 3})
	}
	_, body, _ = f.do(t, "GET", "/entities", "")
	es, _ := body["entities"].([]any)
	if len(es) != 2 || es[0].(map[string]any)["name"] != "India" || es[0].(map[string]any)["count"] != float64(2) {
		t.Fatalf("entities = %v, want India first with 2 stories", body)
	}
	for _, path := range []string{"/entities?days=61", "/entities?limit=0", "/entities?limit=301", "/entities?days=x"} {
		if status, _, _ := f.do(t, "GET", path, ""); status != 400 {
			t.Errorf("%s = %d, want 400", path, status)
		}
	}
}

type recordingPoster struct {
	posts []string
	err   error
}

func (r *recordingPoster) Post(_ context.Context, _, _, text string) error {
	r.posts = append(r.posts, text)

	return r.err
}

func TestTeamsConnectAndStop(t *testing.T) {
	st := store.NewMemory()
	poster := &recordingPoster{}
	srv := httptest.NewServer(NewHandler(Deps{Frontier: frontier.NewMemory(nil), Store: st, TeamPoster: poster, WebBase: "https://digest.test"}))
	t.Cleanup(srv.Close)

	send := func(path, body string) int {
		res, err := http.Post(srv.URL+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()

		return res.StatusCode
	}
	good := `{"url":"https://hooks.slack.com/services/T/B/x","track":"engineering"}`

	if got := send("/teams", good); got != http.StatusAccepted {
		t.Fatalf("connect = %d, want 202", got)
	}
	if len(poster.posts) != 1 || !strings.Contains(poster.posts[0], "https://digest.test/teams/stop?token=") {
		t.Fatalf("welcome posts = %v, want one carrying the stop link", poster.posts)
	}
	hooks, _ := st.TeamHooks(context.Background())
	if len(hooks) != 1 || hooks[0].Kind != "slack" || hooks[0].Track != "engineering" || !strings.Contains(poster.posts[0], "token="+hooks[0].Token) {
		t.Fatalf("hooks = %+v", hooks)
	}
	// The same address again answers the same, stores nothing more and posts nothing to the channel.
	if got := send("/teams", good); got != http.StatusAccepted {
		t.Fatalf("repeat = %d, want 202", got)
	}
	if hooks, _ := st.TeamHooks(context.Background()); len(hooks) != 1 || len(poster.posts) != 1 {
		t.Fatalf("a repeat left %d hooks and %d posts, want 1 and 1", len(hooks), len(poster.posts))
	}
	// A different edition for the same address changes it, keeps the first stop link and tells the channel.
	first := hooks[0].Token
	if got := send("/teams", `{"url":"https://hooks.slack.com/services/T/B/x","track":"upsc"}`); got != http.StatusAccepted {
		t.Fatalf("edition change = %d, want 202", got)
	}
	hooks, _ = st.TeamHooks(context.Background())
	if len(hooks) != 1 || hooks[0].Track != "upsc" || hooks[0].Token != first || len(poster.posts) != 2 || !strings.Contains(poster.posts[1], "token="+first) {
		t.Fatalf("after the edition change hooks = %+v, posts = %v; want the old token in the new welcome", hooks, poster.posts)
	}

	for name, body := range map[string]string{
		"not a webhook": `{"url":"https://evil.test/services/T/B/x","track":"upsc"}`,
		"plain http":    `{"url":"http://hooks.slack.com/services/T/B/x","track":"upsc"}`,
		"unknown track": `{"url":"https://hooks.slack.com/services/T/B/y","track":"nope"}`,
		"bad json":      `{`,
	} {
		before := len(poster.posts)
		if got := send("/teams", body); got != http.StatusBadRequest || len(poster.posts) != before {
			t.Errorf("%s = %d with %d new posts, want 400 and nothing posted", name, got, len(poster.posts)-before)
		}
	}

	// A channel that rejects the welcome message is not stored.
	poster.err = errors.New("no")
	if got := send("/teams", `{"url":"https://hooks.slack.com/services/T/B/z","track":"upsc"}`); got != http.StatusBadRequest {
		t.Fatalf("rejected welcome = %d, want 400", got)
	}
	if hooks, _ := st.TeamHooks(context.Background()); len(hooks) != 1 {
		t.Fatalf("a channel that rejected the test message was stored: %+v", hooks)
	}

	// A connected channel that refuses the notice of an edition change stays connected.
	if got := send("/teams", `{"url":"https://hooks.slack.com/services/T/B/x","track":"engineering"}`); got != http.StatusBadRequest {
		t.Fatalf("refused notice = %d, want 400", got)
	}
	if hooks, _ := st.TeamHooks(context.Background()); len(hooks) != 1 || hooks[0].Token != first {
		t.Fatalf("a refused edition notice dropped the channel: %+v", hooks)
	}

	// Stopping.
	hooks, _ = st.TeamHooks(context.Background())
	token := hooks[0].Token
	if got := send("/teams/stop", `{"token":"nope"}`); got != http.StatusNotFound {
		t.Fatalf("stop with a wrong token = %d, want 404", got)
	}
	if got := send("/teams/stop", `{"token":"`+token+`"}`); got != http.StatusOK {
		t.Fatalf("stop = %d, want 200", got)
	}
	if hooks, _ := st.TeamHooks(context.Background()); len(hooks) != 0 {
		t.Fatalf("hooks after stop = %+v", hooks)
	}
}

func TestTeamsRoutesNeedAPoster(t *testing.T) {
	srv := httptest.NewServer(NewHandler(Deps{Frontier: frontier.NewMemory(nil), Store: store.NewMemory()}))
	t.Cleanup(srv.Close)
	res, err := http.Post(srv.URL+"/teams", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound && res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST /teams without a poster = %d, want it not served", res.StatusCode)
	}
}

func TestAheadListsDatedEventsInTheNextTwoWeeks(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now().In(editionZone)
	day := func(n int) string { return now.AddDate(0, 0, n).Format("2006-01-02") }

	for hash, ev := range map[string]store.NextEvent{
		"soon":    {Text: "MPC announces the rate decision", Date: day(3)},
		"today":   {Text: "Budget session opens", Date: day(0)},
		"later":   {Text: "Too far away", Date: day(40)},
		"past":    {Text: "Already happened", Date: day(-2)},
		"undated": {Text: "Sometime soon"},
	} {
		_ = f.store.Save(ctx, store.Page{URLHash: hash, URL: "https://site.test/" + hash, Title: "Story " + hash, IsArticle: true, Published: now.Add(-time.Hour)})
		ev := ev
		_ = f.store.SetEnrichment(ctx, hash, store.Enrichment{Why: "w", Importance: 3, Extra: &store.Extra{Next: &ev}})
	}

	status, body, hdr := f.do(t, "GET", "/ahead", "")
	items, _ := body["items"].([]any)
	if status != 200 || len(items) != 2 || hdr.Get("Cache-Control") == "" {
		t.Fatalf("/ahead = %d %v, want the two events inside the window", status, body)
	}
	first, second := items[0].(map[string]any), items[1].(map[string]any)
	if first["id"] != "today" || second["id"] != "soon" || second["text"] != "MPC announces the rate decision" || second["date"] != day(3) {
		t.Fatalf("items = %v, want today's event first, then the one in three days", items)
	}
	if body["from"] != day(0) || body["to"] != day(aheadDays) {
		t.Fatalf("window = %v to %v", body["from"], body["to"])
	}
}

func TestStoryCarriesItsExtras(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_ = f.store.Save(ctx, store.Page{URLHash: "x1", URL: "https://site.test/x1", Title: "Story", IsArticle: true, Published: time.Now().Add(-time.Hour)})
	_ = f.store.SetEnrichment(ctx, "x1", store.Enrichment{Why: "w", Importance: 3, Extra: &store.Extra{Context: "Rates were cut twice.", Impact: "Borrowers", Next: &store.NextEvent{Text: "Next meeting", Date: "2099-01-01"}}})

	status, body, _ := f.do(t, "GET", "/story/x1", "")
	extra, _ := body["extra"].(map[string]any)
	next, _ := extra["next"].(map[string]any)
	if status != 200 || extra["context"] != "Rates were cut twice." || extra["impact"] != "Borrowers" || next["date"] != "2099-01-01" {
		t.Fatalf("/story = %d %v, want the extras on the story", status, body)
	}
}
