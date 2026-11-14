package summarize

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

func TestParseQuizKeepsOnlyWellFormedQuestions(t *testing.T) {
	arts := []QuizArticle{{URL: "https://a.test/1"}, {URL: "https://a.test/2"}}
	text := `[
	 {"question":"  Which body? ","options":["A","B","C","D"],"answer":2,"explanation":"Because.","article":2},
	 {"question":"Three options","options":["A","B","C"],"answer":0,"explanation":"x","article":1},
	 {"question":"Bad answer","options":["A","B","C","D"],"answer":4,"explanation":"x","article":1},
	 {"question":"","options":["A","B","C","D"],"answer":0,"explanation":"x","article":1},
	 {"question":"Blank option","options":["A","","C","D"],"answer":0,"explanation":"x","article":1},
	 {"question":"Second","options":["A","B","C","D"],"answer":0,"explanation":"y","article":9},
	 {"question":"Third","options":["A","B","C","D"],"answer":3,"explanation":"z","article":1}
	]`
	qs, err := parseQuiz(text, arts)
	if err != nil || len(qs) != 3 {
		t.Fatalf("parseQuiz = %+v, %v, want 3 valid questions", qs, err)
	}
	if qs[0].Question != "Which body?" || qs[0].URL != "https://a.test/2" || qs[1].URL != "" {
		t.Fatalf("questions = %+v: text must be trimmed, the article link mapped, an unknown article left blank", qs)
	}
	if _, err := parseQuiz(`[{"question":"Only one","options":["A","B","C","D"],"answer":0,"explanation":"x","article":1}]`, arts); err == nil {
		t.Fatal("a quiz with fewer than 3 usable questions must be rejected")
	}
}

type fakeQuizzer struct {
	calls []string
	err   error
}

func (f *fakeQuizzer) GenerateQuiz(_ context.Context, tr string, arts []QuizArticle) ([]store.QuizQuestion, error) {
	f.calls = append(f.calls, tr)
	if f.err != nil {
		return nil, f.err
	}

	return []store.QuizQuestion{{Question: "Q about " + arts[0].Title, Options: []string{"a", "b", "c", "d"}, Answer: 1, Explanation: "e", URL: arts[0].URL}}, nil
}

func seedStories(t *testing.T, m *store.Memory, category string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		h := category + string(rune('a'+i))
		_ = m.Save(context.Background(), store.Page{URLHash: h, URL: "https://" + category + ".test/" + h, Title: "Story " + h, IsArticle: true, Category: category, Published: time.Now().Add(-time.Duration(i+1) * time.Minute)})
		_ = m.SetSummary(context.Background(), h, []string{"Gist " + h + "."})
	}
}

func TestQuizPassWritesOneQuizPerTrackPerDay(t *testing.T) {
	m := store.NewMemory()
	seedStories(t, m, "world", 5)
	seedStories(t, m, "tech", 2) // too few for a quiz
	f := &fakeQuizzer{}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()
	since := time.Now().Add(-24 * time.Hour)

	n, err := QuizPass(ctx, m, f, since, "2026-11-14", quiet)
	if err != nil || n != 1 || strings.Join(f.calls, ",") != "upsc" {
		t.Fatalf("QuizPass = %d, %v, calls %v: want only the track with enough stories", n, err, f.calls)
	}
	q, ok, _ := m.GetQuiz(ctx, "2026-11-14", "upsc")
	if !ok || len(q.Questions) != 1 {
		t.Fatalf("stored quiz = %+v, %v", q, ok)
	}
	if latest, ok, _ := m.GetQuiz(ctx, "", "upsc"); !ok || latest.Day != "2026-11-14" {
		t.Fatalf("latest quiz = %+v, %v", latest, ok)
	}

	// A second run the same day makes no new call for the track that has one.
	f.calls = nil
	if n, _ = QuizPass(ctx, m, f, since, "2026-11-14", quiet); n != 0 || len(f.calls) != 0 {
		t.Fatalf("second run = %d made, calls %v, want none", n, f.calls)
	}

	f.err = ErrRateLimited
	if _, err = QuizPass(ctx, m, f, since, "2026-11-15", quiet); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
}
