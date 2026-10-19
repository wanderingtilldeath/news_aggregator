// Package api is the HTTP surface: start and inspect crawls, search the
// stored pages, health and Prometheus metrics, plus a one-page live view.
//
// The API has no authentication and POST /crawls makes this server fetch
// arbitrary URLs. Run it on a private network, never on the open internet.
package api

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/urlnorm"
)

//go:embed static
var staticFiles embed.FS

const (
	maxBodyBytes    = 1 << 20
	maxDepthLimit   = 10
	defaultMaxDepth = 2
)

// Deps are the handler's collaborators.
type Deps struct {
	Frontier frontier.Frontier
	Store    store.Store
	// Metrics serves GET /metrics (promhttp.HandlerFor in production).
	Metrics http.Handler
	// Checks are named dependency probes for GET /healthz.
	Checks map[string]func(ctx context.Context) error
	Logger *slog.Logger
}

// NewHandler returns the API mux.
func NewHandler(deps Deps) http.Handler {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	s := &server{Deps: deps}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /crawls", s.createCrawl)
	mux.HandleFunc("GET /crawls", s.listCrawls)
	mux.HandleFunc("GET /crawls/{id}/depths", s.crawlDepths)
	mux.HandleFunc("GET /crawls/{id}", s.getCrawl)
	mux.HandleFunc("GET /search", s.search)
	mux.HandleFunc("GET /digest", s.digest)
	mux.HandleFunc("GET /healthz", s.healthz)
	if deps.Metrics != nil {
		mux.Handle("GET /metrics", deps.Metrics)
	}

	static, _ := fs.Sub(staticFiles, "static")
	mux.Handle("GET /", http.FileServerFS(static))

	return mux
}

type server struct{ Deps }

type createCrawlRequest struct {
	Seed         string   `json:"seed"`
	MaxDepth     *int     `json:"max_depth"`
	AllowedHosts []string `json:"allowed_hosts"`
}

type crawlResponse struct {
	frontier.Crawl
	Stats map[string]int64 `json:"stats"`
	Total int64            `json:"total"`
	// Done is true once nothing is pending or in flight.
	Done bool `json:"done"`
}

func (s *server) createCrawl(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	var req createCrawlRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	seed, err := urlnorm.Canonicalize(req.Seed)
	if err != nil {
		writeError(w, http.StatusBadRequest, "seed must be an absolute http(s) URL")
		return
	}

	maxDepth := defaultMaxDepth
	if req.MaxDepth != nil {
		maxDepth = *req.MaxDepth
	}
	if maxDepth < 0 || maxDepth > maxDepthLimit {
		writeError(w, http.StatusBadRequest, "max_depth must be between 0 and "+strconv.Itoa(maxDepthLimit))
		return
	}

	crawl, err := frontier.StartCrawl(r.Context(), s.Frontier, seed, maxDepth, req.AllowedHosts)
	if err != nil {
		s.Logger.Error("create crawl failed", "error", err)
		writeError(w, http.StatusInternalServerError, "could not create crawl")
		return
	}

	w.Header().Set("Location", "/crawls/"+strconv.FormatInt(crawl.ID, 10))
	s.writeCrawl(w, r, http.StatusCreated, crawl)
}

func (s *server) getCrawl(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "crawl id must be a positive integer")
		return
	}

	crawl, err := s.Frontier.GetCrawl(r.Context(), id)
	if errors.Is(err, frontier.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no such crawl")
		return
	}
	if err != nil {
		s.Logger.Error("get crawl failed", "error", err)
		writeError(w, http.StatusInternalServerError, "could not load crawl")
		return
	}

	s.writeCrawl(w, r, http.StatusOK, crawl)
}

func (s *server) crawlView(ctx context.Context, crawl frontier.Crawl) (crawlResponse, error) {
	stats, err := s.Frontier.Stats(ctx, crawl.ID)
	if err != nil {
		return crawlResponse{}, err
	}

	resp := crawlResponse{Crawl: crawl, Stats: make(map[string]int64, len(frontier.AllStatuses)), Total: stats.Total()}
	for _, st := range frontier.AllStatuses {
		resp.Stats[string(st)] = stats[st]
	}
	resp.Done = resp.Total > 0 && stats.Active() == 0

	return resp, nil
}

func (s *server) writeCrawl(w http.ResponseWriter, r *http.Request, status int, crawl frontier.Crawl) {
	resp, err := s.crawlView(r.Context(), crawl)
	if err != nil {
		s.Logger.Error("crawl stats failed", "error", err)
		writeError(w, http.StatusInternalServerError, "could not load crawl stats")
		return
	}

	writeJSON(w, status, resp)
}

func (s *server) listCrawls(w http.ResponseWriter, r *http.Request) {
	crawls, err := s.Frontier.ListCrawls(r.Context(), 20)
	if err != nil {
		s.Logger.Error("list crawls failed", "error", err)
		writeError(w, http.StatusInternalServerError, "could not list crawls")
		return
	}

	out := make([]crawlResponse, 0, len(crawls))
	for _, crawl := range crawls {
		view, err := s.crawlView(r.Context(), crawl)
		if err != nil {
			s.Logger.Error("crawl stats failed", "error", err)
			writeError(w, http.StatusInternalServerError, "could not load crawl stats")
			return
		}
		out = append(out, view)
	}

	writeJSON(w, http.StatusOK, map[string]any{"crawls": out})
}

func (s *server) crawlDepths(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "crawl id must be a positive integer")
		return
	}

	if _, err := s.Frontier.GetCrawl(r.Context(), id); errors.Is(err, frontier.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no such crawl")
		return
	} else if err != nil {
		s.Logger.Error("get crawl failed", "error", err)
		writeError(w, http.StatusInternalServerError, "could not load crawl")
		return
	}

	rows, err := s.Frontier.DepthStats(r.Context(), id)
	if err != nil {
		s.Logger.Error("depth stats failed", "error", err)
		writeError(w, http.StatusInternalServerError, "could not load depth stats")
		return
	}
	if rows == nil {
		rows = []frontier.DepthRow{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"depths": rows})
}

func (s *server) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}

	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 100")
			return
		}
		limit = n
	}

	hits, err := s.Store.Search(r.Context(), q, limit)
	if err != nil {
		s.Logger.Error("search failed", "error", err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	if hits == nil {
		hits = []store.Hit{} // encode as [] rather than null
	}

	writeJSON(w, http.StatusOK, map[string]any{"query": q, "hits": hits})
}

// digest lists pages from the last `days` days (default 1, max 30), newest
// first, optionally narrowed by a full-text query q.
func (s *server) digest(w http.ResponseWriter, r *http.Request) {
	days := 1
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 30 {
			writeError(w, http.StatusBadRequest, "days must be between 1 and 30")
			return
		}
		days = n
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 200")
			return
		}
		limit = n
	}

	since := time.Now().UTC().AddDate(0, 0, -days)
	items, err := s.Store.Digest(r.Context(), since, strings.TrimSpace(r.URL.Query().Get("q")), limit)
	if err != nil {
		s.Logger.Error("digest failed", "error", err)
		writeError(w, http.StatusInternalServerError, "digest failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"since": since, "items": items})
}

func (s *server) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	failing := map[string]string{}
	for name, check := range s.Checks {
		if err := check(ctx); err != nil {
			failing[name] = err.Error()
		}
	}

	if len(failing) > 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "unhealthy", "failing": failing})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
