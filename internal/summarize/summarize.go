// Package summarize turns an article into a few short bullets, either with an
// LLM (Gemini's free tier) or with a no-key extractive fallback, and runs the
// daily pass that fills in summaries for newly crawled articles.
package summarize

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

const (
	maxBullets = 3
	// maxBulletWords caps a bullet by words, so it never stops in the middle of one.
	maxBulletWords = 20
	// minClauseWords is how short a bullet may get when it is cut at a comma.
	minClauseWords = 8
)

// ErrRateLimited means the provider is out of quota; retrying later may work.
var ErrRateLimited = errors.New("summarize: rate limited")

// Article is what a summariser reads.
type Article struct {
	Title       string
	Description string
	Content     string
}

// Summarizer writes the gist of one article as a few short bullets.
type Summarizer interface {
	Summarize(ctx context.Context, a Article) ([]string, error)
}

// Extractive needs no key: it uses the meta description when there is one,
// otherwise the first sentences of the article.
type Extractive struct{}

func (Extractive) Summarize(_ context.Context, a Article) ([]string, error) {
	return ExtractiveGist(a.Description, a.Content), nil
}

// ExtractiveGist builds up to maxBullets bullets from description and text.
func ExtractiveGist(description, text string) []string {
	var bullets []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || len(bullets) >= maxBullets {
			return
		}
		bullets = append(bullets, truncateWords(s, maxBulletWords))
	}

	if d := strings.TrimSpace(description); d != "" {
		add(d)
	}
	for _, s := range sentences(text) {
		if len(bullets) >= maxBullets {
			break
		}
		if len(s) < 40 || (description != "" && strings.HasPrefix(description, s)) {
			continue
		}
		add(s)
	}

	return bullets
}

// sentences splits on ". ", "? " and "! " (and the Devanagari danda), which is
// good enough for a gist.
func sentences(text string) []string {
	text = strings.Join(strings.Fields(text), " ")

	var out []string
	start := 0
	runes := []rune(text)
	for i, r := range runes {
		end := r == '.' || r == '?' || r == '!' || r == '।'
		if end && (i+1 == len(runes) || runes[i+1] == ' ') {
			out = append(out, string(runes[start:i+1]))
			start = i + 1
		}
	}
	if start < len(runes) {
		out = append(out, string(runes[start:]))
	}

	return out
}

func truncate(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:maxRunes-1])) + "…"
}

// danglers are words a cut must not end on.
var danglers = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "but": true, "of": true, "to": true,
	"in": true, "on": true, "for": true, "with": true, "by": true, "at": true, "from": true,
	"as": true, "that": true, "which": true, "is": true, "are": true,
}

// truncateWords keeps at most maxWords words. A longer text is cut after its last
// comma, semicolon or colon (once at least minClauseWords words remain), else at
// the word limit, and ends with "…". A word is never split.
func truncateWords(s string, maxWords int) string {
	words := strings.Fields(s)
	if len(words) <= maxWords {
		return s
	}
	cut := words[:maxWords]
	for i := len(cut) - 1; i >= minClauseWords-1; i-- {
		if last := cut[i][len(cut[i])-1]; last == ',' || last == ';' || last == ':' {
			cut = cut[:i+1]
			break
		}
	}
	for len(cut) > 1 && danglers[strings.ToLower(strings.Trim(cut[len(cut)-1], ",;:.()\"'"))] {
		cut = cut[:len(cut)-1]
	}
	out := strings.Join(cut, " ")
	if last := out[len(out)-1]; last == '.' || last == '!' || last == '?' {
		return out
	}
	return strings.TrimRight(out, ",;: ") + "…"
}

// BatchSummarizer can summarise several articles in one request, which is how
// a day's run stays at a handful of LLM calls.
type BatchSummarizer interface {
	Summarizer
	// SummarizeBatch returns one entry per input; nil means "no gist for that
	// one" and the caller uses the extractive gist instead.
	SummarizeBatch(ctx context.Context, arts []Article) ([][]string, error)
}

// Options tune a Pass.
type Options struct {
	// Limit caps how many pending articles one pass considers.
	Limit int
	// Workers is how many LLM requests may be in flight at once.
	Workers int
	// BatchSize is how many articles share one LLM request (1 = no batching).
	// It only applies to a BatchSummarizer.
	BatchSize int
	// MinLLMRunes: an article with less text than this gets the extractive gist
	// without an LLM call, since there is nothing to condense.
	MinLLMRunes int
	// MinPending makes the pass do nothing while fewer articles than this are
	// waiting, so a pass run while crawling is still going only fires when it
	// can fill a batch. 0 means always run.
	MinPending int
}

// Pass summarises articles dated since `since` (the digest window) that have no
// summary yet. Thin articles get the extractive gist directly; the rest go to
// the LLM, BatchSize at a time, with up to Workers requests in flight (the
// provider spaces its own calls to stay under its rate limit). A failed request
// falls back to extractive gists for its articles, except when the provider is
// rate limited: the pass then stops, leaves the rest pending, and returns
// ErrRateLimited (alongside the count stored so far) so the caller can wait and
// try again. It returns how many articles it stored a summary for.
func Pass(ctx context.Context, st store.Store, s Summarizer, since time.Time, opt Options, logger *slog.Logger) (int, error) {
	pending, err := st.PendingSummaries(ctx, since, opt.Limit)
	if err != nil {
		return 0, err
	}
	if len(pending) == 0 || len(pending) < opt.MinPending {
		return 0, nil
	}
	if opt.Workers < 1 {
		opt.Workers = 1
	}
	if opt.BatchSize < 1 {
		opt.BatchSize = 1
	}

	caller := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu       sync.Mutex
		done     int
		firstErr error
		limited  bool
	)
	store1 := func(u store.Unsummarized, bullets []string) {
		if len(bullets) == 0 {
			bullets = ExtractiveGist(u.Description, u.Content)
		}
		if err := st.SetSummary(ctx, u.URLHash, bullets); err != nil {
			mu.Lock()
			if firstErr == nil {
				firstErr = err
			}
			mu.Unlock()
			cancel()
			return
		}
		mu.Lock()
		done++
		mu.Unlock()
	}

	// Thin articles need no LLM.
	var work []store.Unsummarized
	for _, u := range pending {
		if utf8.RuneCountInString(u.Content) < opt.MinLLMRunes {
			store1(u, nil)
			continue
		}
		work = append(work, u)
	}

	batcher, canBatch := s.(BatchSummarizer)
	size := 1
	if canBatch {
		size = opt.BatchSize
	}

	var batches [][]store.Unsummarized
	for i := 0; i < len(work); i += size {
		end := i + size
		if end > len(work) {
			end = len(work)
		}
		batches = append(batches, work[i:end])
	}

	jobs := make(chan []store.Unsummarized)
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for batch := range jobs {
			var results [][]string
			var err error

			arts := make([]Article, len(batch))
			for i, u := range batch {
				arts[i] = Article{Title: u.Title, Description: u.Description, Content: u.Content}
			}

			if canBatch && len(batch) > 1 {
				results, err = batcher.SummarizeBatch(ctx, arts)
			} else {
				var one []string
				one, err = s.Summarize(ctx, arts[0])
				results = [][]string{one}
			}

			switch {
			case errors.Is(err, ErrRateLimited):
				mu.Lock()
				if !limited {
					limited = true
					logger.Warn("summaries paused: rate limited", "done", done, "reason", err)
				}
				mu.Unlock()
				cancel()
				continue
			case ctx.Err() != nil:
				continue
			case err != nil:
				logger.Warn("summary request failed, using extractive gists", "articles", len(batch), "error", err)
				results = nil
			}

			for i, u := range batch {
				var bullets []string
				if i < len(results) {
					bullets = results[i]
				}
				store1(u, bullets)
			}
		}
	}

	for i := 0; i < opt.Workers; i++ {
		wg.Add(1)
		go worker()
	}

feed:
	for _, b := range batches {
		select {
		case jobs <- b:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()

	if firstErr != nil {
		return done, firstErr
	}
	if limited {
		return done, ErrRateLimited
	}

	return done, caller.Err()
}
