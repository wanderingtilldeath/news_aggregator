package store_test

import (
	"context"
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
