package digestmail

import (
	"context"
	"strings"
	"testing"

	"github.com/wanderingtilldeath/news-aggregator/internal/mail"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

func TestProRemindersMailOncePerPeriod(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	for i, c := range []struct {
		email string
		days  int
	}{{"soon@x.test", 2}, {"later@x.test", 20}, {"gone@x.test", -2}} {
		if _, _, err := st.ProExtend(ctx, "pay-"+string(rune('a'+i)), c.email, c.days, "tok-"+c.email); err != nil {
			t.Fatal(err)
		}
	}

	f := &fake{}
	n, err := ProReminders(ctx, st, f, "https://site.test", false, quiet)
	if err != nil || n != 1 || len(f.got) != 1 || f.got[0].To != "soon@x.test" || !strings.Contains(f.got[0].Text, "/pro") {
		t.Fatalf("reminders = %d, %v, %v; want one mail to soon@x.test", n, err, f.to())
	}
	if n, _ := ProReminders(ctx, st, f, "https://site.test", false, quiet); n != 0 {
		t.Fatalf("second run sent %d; a period is reminded once", n)
	}

	// A renewal moves the end date, so the next period is reminded again once it is close.
	if _, _, err := st.ProExtend(ctx, "pay-renew", "soon@x.test", 1, "tok-new"); err != nil {
		t.Fatal(err)
	}
	if n, _ := ProReminders(ctx, st, f, "https://site.test", false, quiet); n != 1 {
		t.Fatalf("after a renewal that is still short the member gets a new reminder; sent %d", n)
	}
}

func TestProRemindersDryRunRecordsNothingAndRefusalStops(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	_, _, _ = st.ProExtend(ctx, "pay-1", "soon@x.test", 1, "tok")

	f := &fake{}
	if n, _ := ProReminders(ctx, st, f, "https://site.test", true, quiet); n != 1 {
		t.Fatalf("dry run sent %d, want 1", n)
	}
	if n, _ := ProReminders(ctx, st, f, "https://site.test", false, quiet); n != 1 {
		t.Fatalf("a dry run must not mark the member reminded; real run sent %d", n)
	}

	st2 := store.NewMemory()
	_, _, _ = st2.ProExtend(ctx, "pay-1", "soon@x.test", 1, "tok")
	if _, err := ProReminders(ctx, st2, &fake{refuse: true}, "https://site.test", false, quiet); err == nil || !strings.Contains(err.Error(), mail.ErrRefused.Error()) {
		t.Fatalf("err = %v, want the refusal", err)
	}
	if left, _ := st2.ProExpiring(ctx, ProReminderWindow); len(left) != 1 {
		t.Fatalf("a refused send must leave the member unreminded; %d left", len(left))
	}
}
