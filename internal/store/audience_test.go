package store_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/testutil"
)

// audienceContract runs the same checks on every Store: a reader is found by token with what they chose,
// and the audience numbers count the same people the same way.
func audienceContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	for _, e := range []string{"a", "b", "c"} {
		if _, _, err := s.Subscribe(ctx, e+"@x.test", "t"+e); err != nil {
			t.Fatal(err)
		}
	}
	_, _ = s.SetPreferences(ctx, "ta", store.Preferences{Mode: "7min", Topics: []string{"monsoon", "rbi"}, Source: "linkedin"})
	_, _ = s.SetPreferences(ctx, "tb", store.Preferences{Mode: "1min", Topics: []string{"monsoon"}, Source: "linkedin"})
	_, _ = s.Confirm(ctx, "ta")
	_, _ = s.Confirm(ctx, "tb")
	_, _ = s.Unsubscribe(ctx, "tb")

	active, _ := s.ConfirmedSubscribers(ctx)
	if len(active) != 1 {
		t.Fatalf("active = %+v, want only a", active)
	}
	today := time.Now().In(time.FixedZone("IST", 19800)).Format("2006-01-02")
	if loc, err := time.LoadLocation("Asia/Kolkata"); err == nil {
		today = time.Now().In(loc).Format("2006-01-02")
	}
	if err := s.MarkDelivered(ctx, active[0].ID, today); err != nil {
		t.Fatal(err)
	}

	for token, want := range map[string]store.Subscription{
		"ta": {Email: "a@x.test", Status: "active", Mode: "7min", Topics: []string{"monsoon", "rbi"}, Frequency: "daily"},
		"tb": {Email: "b@x.test", Status: "unsubscribed", Mode: "1min", Topics: []string{"monsoon"}, Frequency: "daily"},
		"tc": {Email: "c@x.test", Status: "pending", Mode: "1min", Topics: []string{}, Frequency: "daily"},
	} {
		got, ok, err := s.Subscription(ctx, token)
		if err != nil || !ok {
			t.Fatalf("Subscription(%s) = %v, %v", token, ok, err)
		}
		got.Tracks = nil
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Subscription(%s) = %+v, want %+v", token, got, want)
		}
	}
	// The weekly choice is kept, shows in the subscription and the send list, and an unknown value means daily.
	if ok, err := s.SetPreferences(ctx, "ta", store.Preferences{Mode: "7min", Frequency: "weekly"}); err != nil || !ok {
		t.Fatalf("SetPreferences weekly = %v, %v", ok, err)
	}
	if got, _, _ := s.Subscription(ctx, "ta"); got.Frequency != "weekly" {
		t.Fatalf("Frequency after choosing weekly = %q", got.Frequency)
	}
	if subs, err := s.ConfirmedSubscribers(ctx); err != nil || len(subs) != 1 || subs[0].Frequency != "weekly" {
		t.Fatalf("ConfirmedSubscribers = %+v, %v; want the one reader as weekly", subs, err)
	}
	if _, err := s.SetPreferences(ctx, "ta", store.Preferences{Mode: "7min", Frequency: "monthly"}); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := s.Subscription(ctx, "ta"); got.Frequency != "daily" {
		t.Fatalf("Frequency after an unknown value = %q, want daily", got.Frequency)
	}
	if _, ok, err := s.Subscription(ctx, "nope"); ok || err != nil {
		t.Fatalf("an unknown token = %v, %v; want not found", ok, err)
	}

	// Only an active subscriber has a manage token.
	for email, want := range map[string]struct {
		token string
		ok    bool
	}{"a@x.test": {"ta", true}, "b@x.test": {"", false}, "c@x.test": {"", false}, "nobody@x.test": {"", false}} {
		token, ok, err := s.ManageToken(ctx, email)
		if err != nil || ok != want.ok || token != want.token {
			t.Fatalf("ManageToken(%s) = %q, %v, %v; want %q, %v", email, token, ok, err, want.token, want.ok)
		}
	}

	// Saving new choices without a source keeps the source the signup had.
	_, _ = s.SetPreferences(ctx, "ta", store.Preferences{Mode: "7min", Topics: []string{"monsoon", "rbi"}})

	a, err := s.AudienceStats(ctx, 7, "Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	if a.Pending != 1 || a.Active != 1 || a.Unsubscribed != 1 {
		t.Fatalf("totals = %d pending, %d active, %d unsubscribed; want 1, 1, 1", a.Pending, a.Active, a.Unsubscribed)
	}
	if !reflect.DeepEqual(a.Modes, map[string]int{"7min": 1}) {
		t.Fatalf("modes = %v", a.Modes)
	}
	if !reflect.DeepEqual(a.Topics, []store.TopicCount{{Topic: "monsoon", Count: 1}, {Topic: "rbi", Count: 1}}) {
		t.Fatalf("topics = %+v", a.Topics)
	}
	if len(a.Daily) != 7 || a.Daily[6].Day != today {
		t.Fatalf("days = %+v, want 7 ending today (%s)", a.Daily, today)
	}
	if got, want := a.Daily[6], (store.AudienceDay{Day: today, Signups: 3, Confirmed: 2, Unsubscribed: 1, Sent: 1}); got != want {
		t.Fatalf("today = %+v, want %+v", got, want)
	}
	if a.Daily[0].Signups != 0 {
		t.Fatalf("a quiet day must be a zero, got %+v", a.Daily[0])
	}
	if want := []store.AudienceSource{{Source: "linkedin", Signups: 2, Confirmed: 2}, {Source: "", Signups: 1}}; !reflect.DeepEqual(a.Sources, want) {
		t.Fatalf("sources = %+v, want %+v", a.Sources, want)
	}

	if _, err := s.AudienceStats(ctx, 7, "Not/AZone"); err == nil {
		t.Fatal("an unknown time zone must be an error")
	}
}

func TestMemoryAudience(t *testing.T) { audienceContract(t, store.NewMemory()) }

func TestPostgresAudience(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	audienceContract(t, s)
}

// adminTokenContract runs the same checks on every Store: a sign-in link works once and only before it
// expires, a session can be read many times until it is deleted, and the two kinds never stand in for each other.
func adminTokenContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	now := time.Now()
	for _, tok := range []store.AdminToken{
		{Hash: "login1", Kind: "login", Email: "a@x.test", Expires: now.Add(time.Minute)},
		{Hash: "login-old", Kind: "login", Email: "a@x.test", Expires: now.Add(-time.Minute)},
		{Hash: "sess1", Kind: "session", Email: "a@x.test", Expires: now.Add(time.Hour)},
	} {
		if err := s.SaveAdminToken(ctx, tok); err != nil {
			t.Fatal(err)
		}
	}

	if email, ok, err := s.UseAdminToken(ctx, "login1", "login", true, now); err != nil || !ok || email != "a@x.test" {
		t.Fatalf("first use = %q, %v, %v; want the email", email, ok, err)
	}
	if _, ok, _ := s.UseAdminToken(ctx, "login1", "login", true, now); ok {
		t.Fatal("a sign-in link must work only once")
	}
	if _, ok, _ := s.UseAdminToken(ctx, "login-old", "login", true, now); ok {
		t.Fatal("an expired link must not work")
	}
	if _, ok, _ := s.UseAdminToken(ctx, "sess1", "login", true, now); ok {
		t.Fatal("a session must not work as a sign-in link")
	}
	for i := 0; i < 2; i++ {
		if _, ok, _ := s.UseAdminToken(ctx, "sess1", "session", false, now); !ok {
			t.Fatalf("session read %d failed; a session is read many times", i)
		}
	}
	if _, ok, _ := s.UseAdminToken(ctx, "sess1", "session", false, now.Add(2*time.Hour)); ok {
		t.Fatal("an expired session must not work")
	}
	if err := s.DeleteAdminToken(ctx, "sess1"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.UseAdminToken(ctx, "sess1", "session", false, now); ok {
		t.Fatal("a deleted session must not work")
	}
	if err := s.DeleteAdminToken(ctx, "never-existed"); err != nil {
		t.Fatalf("deleting an unknown token = %v, want nil", err)
	}
}

func TestMemoryAdminTokens(t *testing.T) { adminTokenContract(t, store.NewMemory()) }

func TestPostgresAdminTokens(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	adminTokenContract(t, s)
}

// contentContract runs the same checks on every Store: articles are counted per day, non-articles are left out,
// and each article is counted under the one stage it is waiting for.
func contentContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	now := time.Now()
	save := func(hash string, published time.Time, article bool, sourceURL string) {
		t.Helper()
		if err := s.Save(ctx, store.Page{URLHash: hash, URL: "https://x.test/" + hash, Title: hash, IsArticle: article, Published: published, SourceURL: sourceURL}); err != nil {
			t.Fatal(err)
		}
	}
	save("p1", now.Add(-time.Second), true, "")
	save("p2", now.Add(-2*time.Second), true, "")
	save("p3", now.Add(-3*time.Second), true, "https://src.test/p3")
	save("old", now.Add(-26*time.Hour), true, "")
	save("page", now.Add(-time.Second), false, "")
	_ = s.SetSummary(ctx, "p1", []string{"gist"})
	_ = s.SetEnrichment(ctx, "p1", store.Enrichment{Why: "w", Importance: 3})
	_ = s.SetSummary(ctx, "p2", []string{"gist"})
	_ = s.SetSummary(ctx, "old", []string{"gist"})
	_ = s.SetEnrichment(ctx, "old", store.Enrichment{Why: "w", Importance: 3})
	_ = s.SetDetail(ctx, "old", store.Detail{Long: "long"})

	c, err := s.ContentHealth(ctx, 3, "Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Daily) != 3 || c.Daily[2].Articles != 3 || c.Daily[0].Articles != 0 {
		t.Fatalf("daily = %+v, want 3 days: 0 articles, then the old one, then today's 3 (a non-article is not counted)", c.Daily)
	}
	if got := c.Daily[0].Articles + c.Daily[1].Articles; got != 1 {
		t.Fatalf("the 26-hour-old article is in neither of the earlier days: %+v", c.Daily)
	}
	if want := (store.ContentWaiting{Summary: 1, Image: 1, Enrichment: 1, Detail: 1}); c.Waiting != want {
		t.Fatalf("waiting = %+v, want %+v", c.Waiting, want)
	}
	if c.Latest.IsZero() {
		t.Fatal("the newest article's first-seen time is missing")
	}
	if _, err := s.ContentHealth(ctx, 3, "Not/AZone"); err == nil {
		t.Fatal("an unknown time zone must be an error")
	}
}

func TestMemoryContentHealth(t *testing.T) { contentContract(t, store.NewMemory()) }

func TestPostgresContentHealth(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	contentContract(t, s)
}

// subscribersContract runs the same checks on every Store: the admin's list is newest first, searchable (with
// wildcard characters taken literally), paged, and a removal is final.
func subscribersContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	for _, e := range []string{"a", "b", "c"} {
		if _, _, err := s.Subscribe(ctx, e+"@x.test", "t"+e); err != nil {
			t.Fatal(err)
		}
	}
	_, _ = s.SetPreferences(ctx, "ta", store.Preferences{Mode: "7min", Topics: []string{"rbi"}, Source: "linkedin"})
	_, _ = s.Confirm(ctx, "ta")
	_, _ = s.Unsubscribe(ctx, "tc")

	rows, total, err := s.Subscribers(ctx, "", 10, 0)
	if err != nil || total != 3 || len(rows) != 3 {
		t.Fatalf("Subscribers = %d rows of %d, %v; want 3", len(rows), total, err)
	}
	if rows[0].Email != "c@x.test" || rows[2].Email != "a@x.test" {
		t.Fatalf("order = %s, %s, %s; want newest first", rows[0].Email, rows[1].Email, rows[2].Email)
	}
	a := rows[2]
	if a.Status != "active" || a.Mode != "7min" || a.Source != "linkedin" || len(a.Topics) != 1 || a.Created.IsZero() || a.Confirmed.IsZero() {
		t.Fatalf("a's row = %+v", a)
	}
	if rows[0].Status != "unsubscribed" || rows[1].Status != "pending" || !rows[1].Confirmed.IsZero() {
		t.Fatalf("statuses = %s, %s", rows[0].Status, rows[1].Status)
	}

	if rows, total, _ := s.Subscribers(ctx, " B@X ", 10, 0); total != 1 || len(rows) != 1 || rows[0].Email != "b@x.test" {
		t.Fatalf("search = %v of %d; want b only, found without case or spaces", rows, total)
	}
	for _, wild := range []string{"%", "_"} {
		if _, total, _ := s.Subscribers(ctx, wild, 10, 0); total != 0 {
			t.Fatalf("searching %q matched %d; a wildcard must be a plain character", wild, total)
		}
	}
	if rows, total, _ := s.Subscribers(ctx, "", 2, 2); total != 3 || len(rows) != 1 || rows[0].Email != "a@x.test" {
		t.Fatalf("page 2 = %v of %d; want a alone of 3", rows, total)
	}

	id := rows[1].ID
	if ok, err := s.RemoveSubscriber(ctx, id); err != nil || !ok {
		t.Fatalf("remove = %v, %v", ok, err)
	}
	if ok, _ := s.RemoveSubscriber(ctx, id); ok {
		t.Fatal("removing the same subscriber twice must report nothing was there")
	}
	if _, total, _ := s.Subscribers(ctx, "", 10, 0); total != 2 {
		t.Fatalf("total after removal = %d, want 2", total)
	}
	if _, ok, _ := s.Subscription(ctx, "tb"); ok {
		t.Fatal("a removed subscriber's token must stop working")
	}
	if _, _, err := s.Subscribe(ctx, "b@x.test", "tb2"); err != nil {
		t.Fatal(err)
	}
	if rows, _, _ := s.Subscribers(ctx, "b@", 10, 0); len(rows) != 1 || rows[0].ID == id || rows[0].Status != "pending" {
		t.Fatalf("signing up again after removal = %+v, want a fresh pending row with a new id", rows)
	}
}

func TestMemorySubscribers(t *testing.T) { subscribersContract(t, store.NewMemory()) }

func TestPostgresSubscribers(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	subscribersContract(t, s)
}
