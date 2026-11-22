package store_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/testutil"
)

// accountsContract runs the same checks on every Store: a username is taken once, sessions are found until they
// expire or are deleted, new secrets end old sessions, and a Pro membership belongs to one account.
func accountsContract(t *testing.T, s store.Store) {
	ctx := context.Background()
	alice, ok, err := s.CreateAccount(ctx, "alice", "pw-hash", []string{"r1", "r2", "r3"})
	if err != nil || !ok || alice.ID == 0 || alice.Username != "alice" || alice.PasswordHash != "pw-hash" || alice.ProEmail != "" {
		t.Fatalf("CreateAccount = %+v, %v, %v", alice, ok, err)
	}
	if _, ok, _ := s.CreateAccount(ctx, "alice", "x", nil); ok {
		t.Fatal("a taken username must be refused")
	}
	bob, ok, _ := s.CreateAccount(ctx, "bob", "pw2", []string{"b1"})
	if !ok || bob.ID == alice.ID {
		t.Fatalf("bob = %+v, %v", bob, ok)
	}

	if got, ok, _ := s.AccountByUsername(ctx, "alice"); !ok || got.ID != alice.ID || got.PasswordHash != "pw-hash" {
		t.Fatalf("AccountByUsername = %+v, %v", got, ok)
	}
	if _, ok, _ := s.AccountByUsername(ctx, "nobody"); ok {
		t.Fatal("an unknown username must not be found")
	}

	// An email is unconfirmed until its link is used: it finds no account, and another account may claim the same
	// address meanwhile. Only the first to confirm it keeps it.
	if set, err := s.SetAccountEmail(ctx, alice.ID, "alice@example.com"); err != nil || !set {
		t.Fatalf("SetAccountEmail = %v, %v", set, err)
	}
	if _, ok, _ := s.AccountByEmail(ctx, "alice@example.com"); ok {
		t.Fatal("an unconfirmed email must not find the account")
	}
	if set, _ := s.SetAccountEmail(ctx, bob.ID, "alice@example.com"); !set {
		t.Fatal("an unconfirmed claim must not block another account")
	}
	if ok, err := s.VerifyAccountEmail(ctx, alice.ID, "alice@example.com"); err != nil || !ok {
		t.Fatalf("VerifyAccountEmail = %v, %v", ok, err)
	}
	if got, ok, _ := s.AccountByEmail(ctx, "alice@example.com"); !ok || got.ID != alice.ID || !got.EmailVerified {
		t.Fatalf("AccountByEmail = %+v, %v", got, ok)
	}
	if ok, _ := s.VerifyAccountEmail(ctx, bob.ID, "alice@example.com"); ok {
		t.Fatal("an address another account confirmed must not be confirmable")
	}
	if ok, _ := s.VerifyAccountEmail(ctx, alice.ID, "other@example.com"); ok {
		t.Fatal("confirming an address the account does not hold must fail")
	}

	// One-time links: found once, for their purpose, before they expire, while the address is unchanged.
	soon := time.Now().Add(time.Hour)
	if err := s.SaveEmailToken(ctx, "tok-a", alice.ID, "reset", "alice@example.com", soon); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, _ := s.UseEmailToken(ctx, "tok-a", "verify", time.Now()); ok {
		t.Fatal("a reset link must not work as a verify link")
	}
	if got, email, ok, _ := s.UseEmailToken(ctx, "tok-a", "reset", time.Now()); !ok || got.ID != alice.ID || email != "alice@example.com" {
		t.Fatalf("UseEmailToken = %+v %q %v", got, email, ok)
	}
	if _, _, ok, _ := s.UseEmailToken(ctx, "tok-a", "reset", time.Now()); ok {
		t.Fatal("a link must work once")
	}
	_ = s.SaveEmailToken(ctx, "tok-b", alice.ID, "reset", "alice@example.com", time.Now().Add(-time.Minute))
	if _, _, ok, _ := s.UseEmailToken(ctx, "tok-b", "reset", time.Now()); ok {
		t.Fatal("an expired link must not work")
	}
	_ = s.SaveEmailToken(ctx, "tok-c", alice.ID, "verify", "alice@example.com", soon)
	_ = s.SaveEmailToken(ctx, "tok-d", alice.ID, "verify", "alice@example.com", soon)
	if _, _, ok, _ := s.UseEmailToken(ctx, "tok-c", "verify", time.Now()); ok {
		t.Fatal("a newer link of the same purpose must replace the older one")
	}
	_ = s.SaveEmailToken(ctx, "tok-e", alice.ID, "reset", "alice@example.com", soon)
	if _, err := s.SetAccountEmail(ctx, alice.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, _ := s.UseEmailToken(ctx, "tok-e", "reset", time.Now()); ok {
		t.Fatal("changing or clearing the email must drop its links")
	}
	if _, ok, _ := s.AccountByEmail(ctx, "alice@example.com"); ok {
		t.Fatal("a cleared email must not be found")
	}
	if set, _ := s.SetAccountEmail(ctx, bob.ID, ""); !set {
		t.Fatal("clearing an email must work even when no account has one")
	}

	now := time.Now()
	if err := s.SaveAccountSession(ctx, "h1", alice.ID, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, ok, _ := s.AccountBySession(ctx, "h1", now); !ok || got.ID != alice.ID {
		t.Fatalf("session = %+v, %v", got, ok)
	}
	if _, ok, _ := s.AccountBySession(ctx, "h1", now.Add(2*time.Hour)); ok {
		t.Fatal("an expired session must not work")
	}
	if _, ok, _ := s.AccountBySession(ctx, "unknown", now); ok {
		t.Fatal("an unknown session must not work")
	}
	_ = s.SaveAccountSession(ctx, "h2", alice.ID, now.Add(time.Hour))
	_ = s.SaveAccountSession(ctx, "hb", bob.ID, now.Add(time.Hour))
	if err := s.DeleteAccountSession(ctx, "h2"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.AccountBySession(ctx, "h2", now); ok {
		t.Fatal("a deleted session must not work")
	}
	if err := s.DeleteAccountSession(ctx, "never"); err != nil {
		t.Fatalf("deleting an unknown session = %v, want nil", err)
	}

	// A new password replaces the old and leaves sessions alone; logging out everywhere ends only this account's.
	if err := s.SetAccountPassword(ctx, alice.ID, "new-pw"); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := s.AccountByUsername(ctx, "alice"); got.PasswordHash != "new-pw" {
		t.Fatalf("password = %+v", got)
	}
	if _, ok, _ := s.AccountBySession(ctx, "h1", now); !ok {
		t.Fatal("changing the password must not by itself end a session")
	}
	_ = s.SaveAccountSession(ctx, "h3", alice.ID, now.Add(time.Hour))
	if err := s.DeleteAccountSessions(ctx, alice.ID); err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"h1", "h3"} {
		if _, ok, _ := s.AccountBySession(ctx, h, now); ok {
			t.Fatalf("session %s must end when the account logs out everywhere", h)
		}
	}
	if _, ok, _ := s.AccountBySession(ctx, "hb", now); !ok {
		t.Fatal("logging out everywhere must not end another account's session")
	}

	// Recovery codes: each works once, only for its own account, and a new set replaces the old.
	if n, _ := s.RecoveryCodesLeft(ctx, alice.ID); n != 3 {
		t.Fatalf("codes left = %d, want 3", n)
	}
	if ok, _ := s.UseRecoveryCode(ctx, alice.ID, "r1"); !ok {
		t.Fatal("an unspent code must work")
	}
	if ok, _ := s.UseRecoveryCode(ctx, alice.ID, "r1"); ok {
		t.Fatal("a code must work only once")
	}
	if ok, _ := s.UseRecoveryCode(ctx, bob.ID, "r2"); ok {
		t.Fatal("a code must not work for another account")
	}
	if ok, _ := s.UseRecoveryCode(ctx, alice.ID, "nope"); ok {
		t.Fatal("an unknown code must not work")
	}
	if n, _ := s.RecoveryCodesLeft(ctx, alice.ID); n != 2 {
		t.Fatalf("codes left = %d, want 2", n)
	}
	if err := s.ReplaceRecoveryCodes(ctx, alice.ID, []string{"n1", "n2", "n3", "n4"}); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.RecoveryCodesLeft(ctx, alice.ID); n != 4 {
		t.Fatalf("codes left after a new set = %d, want 4", n)
	}
	if ok, _ := s.UseRecoveryCode(ctx, alice.ID, "r2"); ok {
		t.Fatal("a code from the replaced set must not work")
	}
	if n, _ := s.RecoveryCodesLeft(ctx, bob.ID); n != 1 {
		t.Fatalf("another account's codes = %d, want 1", n)
	}

	// An identity at a provider belongs to one account.
	if ok, err := s.LinkIdentity(ctx, alice.ID, "google", "sub-1"); err != nil || !ok {
		t.Fatalf("link identity = %v, %v", ok, err)
	}
	if ok, _ := s.LinkIdentity(ctx, alice.ID, "google", "sub-1"); !ok {
		t.Fatal("linking the same identity to the same account again must be allowed")
	}
	if ok, _ := s.LinkIdentity(ctx, bob.ID, "google", "sub-1"); ok {
		t.Fatal("an identity held by another account must be refused")
	}
	if got, ok, _ := s.AccountByIdentity(ctx, "google", "sub-1"); !ok || got.ID != alice.ID {
		t.Fatalf("AccountByIdentity = %+v, %v", got, ok)
	}
	if _, ok, _ := s.AccountByIdentity(ctx, "google", "unknown"); ok {
		t.Fatal("an unknown identity must not be found")
	}
	if got, _ := s.Identities(ctx, alice.ID); len(got) != 1 || got[0] != "google" {
		t.Fatalf("identities = %v", got)
	}
	if got, _ := s.Identities(ctx, bob.ID); len(got) != 0 {
		t.Fatalf("bob's identities = %v, want none", got)
	}

	// A Pro membership belongs to one account.
	if _, _, err := s.ProExtend(ctx, "pay-1", "member@x.test", 30, "tok-member"); err != nil {
		t.Fatal(err)
	}
	if m, ok, _ := s.ProByEmail(ctx, "member@x.test"); !ok || m.Token != "tok-member" {
		t.Fatalf("ProByEmail = %+v, %v", m, ok)
	}
	if _, ok, _ := s.ProByEmail(ctx, "nobody@x.test"); ok {
		t.Fatal("an unknown membership must not be found")
	}
	if ok, err := s.LinkProMembership(ctx, alice.ID, "member@x.test"); err != nil || !ok {
		t.Fatalf("link = %v, %v", ok, err)
	}
	if got, _, _ := s.AccountByUsername(ctx, "alice"); got.ProEmail != "member@x.test" {
		t.Fatalf("ProEmail = %q", got.ProEmail)
	}
	if ok, _ := s.LinkProMembership(ctx, bob.ID, "member@x.test"); ok {
		t.Fatal("a membership already held by another account must be refused")
	}
	if ok, _ := s.LinkProMembership(ctx, alice.ID, "member@x.test"); !ok {
		t.Fatal("linking the same membership again must be allowed")
	}

	// A payment id finds the membership it bought.
	if m, ok, _ := s.ProByPayment(ctx, "pay-1"); !ok || m.Email != "member@x.test" || m.Token != "tok-member" {
		t.Fatalf("ProByPayment = %+v, %v", m, ok)
	}
	if _, ok, _ := s.ProByPayment(ctx, "pay-unknown"); ok {
		t.Fatal("an unknown payment must not be found")
	}

	// What a free account carries between devices: empty until stored, then returned as stored, gone with the account.
	if d, err := s.AccountSync(ctx, alice.ID); err != nil || d.Bookmarks != "[]" || d.Cards != "{}" || d.Missed != "[]" || d.Streak != "" {
		t.Fatalf("empty sync = %+v, %v", d, err)
	}
	want := store.SyncData{Bookmarks: `[{"id":"s1","title":"T","source":"x","time":"","at":1}]`, Cards: `{"s1:0":{"box":2,"due":5}}`, Streak: "2026-11-22|3", Missed: "[]"}
	if err := s.SetAccountSync(ctx, alice.ID, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.AccountSync(ctx, alice.ID)
	if err != nil || got.Streak != want.Streak || !strings.Contains(got.Bookmarks, `"s1"`) || !strings.Contains(got.Cards, `"s1:0"`) {
		t.Fatalf("sync = %+v, %v; want what was stored", got, err)
	}
	if other, _ := s.AccountSync(ctx, bob.ID); other.Streak != "" || other.Bookmarks != "[]" {
		t.Fatalf("bob sees alice's items: %+v", other)
	}

	// Deleting an account removes it with its sessions and identity, and frees its membership and username.
	_ = s.SaveAccountSession(ctx, "hdel", alice.ID, now.Add(time.Hour))
	if err := s.DeleteAccount(ctx, alice.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.AccountByUsername(ctx, "alice"); ok {
		t.Fatal("a deleted account must be gone")
	}
	if _, ok, _ := s.AccountBySession(ctx, "hdel", now); ok {
		t.Fatal("a deleted account's session must not work")
	}
	if _, ok, _ := s.AccountByIdentity(ctx, "google", "sub-1"); ok {
		t.Fatal("a deleted account's identity must be freed")
	}
	if ok, _ := s.LinkProMembership(ctx, bob.ID, "member@x.test"); !ok {
		t.Fatal("a membership freed by a deleted account must be linkable again")
	}
	if _, ok, _ := s.CreateAccount(ctx, "alice", "again", nil); !ok {
		t.Fatal("a deleted account's username must be free")
	}
	if _, ok, _ := s.ProByEmail(ctx, "member@x.test"); !ok {
		t.Fatal("deleting an account must not delete the Pro membership")
	}
}

func TestMemoryAccounts(t *testing.T) { accountsContract(t, store.NewMemory()) }

func TestPostgresAccounts(t *testing.T) {
	s := store.NewPostgres(testutil.Postgres(t))
	defer s.Close()
	accountsContract(t, s)
}
