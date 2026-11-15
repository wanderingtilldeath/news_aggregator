package summarize

import (
	"context"
	"errors"
	"testing"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

type fakeExplainer struct {
	calls int
	got   []string
	out   map[string]string
	err   error
}

func (f *fakeExplainer) Explain(_ context.Context, terms []string) (map[string]string, error) {
	f.calls++
	f.got = terms

	return f.out, f.err
}

func TestParseConceptsKeepsShortPlainExplainers(t *testing.T) {
	got, err := parseConcepts(`[{"term":"Repo Rate","explanation":"  The rate   at which the central bank lends to banks. "},{"term":"empty","explanation":"  "}]`)
	if err != nil || len(got) != 1 || got["repo rate"] != "The rate at which the central bank lends to banks." {
		t.Fatalf("parseConcepts = %v, %v; want one explainer keyed by the lower-cased term", got, err)
	}
	if _, err := parseConcepts(`not json`); err == nil {
		t.Fatal("a bad answer must be an error")
	}
}

func TestConceptPassExplainsTheQueueInOneCall(t *testing.T) {
	st := store.NewMemory()
	ctx := context.Background()
	_ = st.AddConceptTerms(ctx, []string{"repo rate", "writ", "gerrymandering"})
	ex := &fakeExplainer{out: map[string]string{"repo rate": "The bank's lending rate.", "writ": "A court order."}} // the model skipped one

	n, err := ConceptPass(ctx, st, ex, 10, quietLog)
	if err != nil || n != 2 || ex.calls != 1 || len(ex.got) != 3 {
		t.Fatalf("ConceptPass = %d, %v after %d calls with %v; want 2 stored from one call of three terms", n, err, ex.calls, ex.got)
	}
	if pending, _ := st.PendingConcepts(ctx, 10); len(pending) != 1 || pending[0] != "gerrymandering" {
		t.Fatalf("pending = %v, want the skipped term to wait for the next run", pending)
	}

	// Nothing waiting means no call at all.
	_ = st.SetConcept(ctx, "gerrymandering", "Drawing districts for advantage.")
	ex.calls = 0
	if n, _ := ConceptPass(ctx, st, ex, 10, quietLog); n != 0 || ex.calls != 0 {
		t.Fatalf("an empty queue made %d calls", ex.calls)
	}
}

func TestConceptPassLimitsTheBatchAndPassesOnARateLimit(t *testing.T) {
	st := store.NewMemory()
	ctx := context.Background()
	_ = st.AddConceptTerms(ctx, []string{"a term", "b term", "c term"})
	ex := &fakeExplainer{out: map[string]string{}}
	_, _ = ConceptPass(ctx, st, ex, 2, quietLog)
	if len(ex.got) != 2 {
		t.Fatalf("the limit asked for 2 terms, the call had %d", len(ex.got))
	}

	limited := &fakeExplainer{err: ErrRateLimited}
	if _, err := ConceptPass(ctx, st, limited, 5, quietLog); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	// Any other failure is logged and leaves the terms queued.
	broken := &fakeExplainer{err: errors.New("boom")}
	if n, err := ConceptPass(ctx, st, broken, 5, quietLog); err != nil || n != 0 {
		t.Fatalf("a failed call = %d, %v; want it ignored", n, err)
	}
}
