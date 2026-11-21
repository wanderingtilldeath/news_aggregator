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
		"ta": {Email: "a@x.test", Status: "active", Mode: "7min", Topics: []string{"monsoon", "rbi"}},
		"tb": {Email: "b@x.test", Status: "unsubscribed", Mode: "1min", Topics: []string{"monsoon"}},
		"tc": {Email: "c@x.test", Status: "pending", Mode: "1min", Topics: []string{}},
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
	if _, ok, err := s.Subscription(ctx, "nope"); ok || err != nil {
		t.Fatalf("an unknown token = %v, %v; want not found", ok, err)
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
