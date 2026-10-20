// Package api is the HTTP surface: start and inspect crawls, search the
// stored pages, health, plus a one-page live view.
//
// The API has no authentication and POST /crawls makes this server fetch
// arbitrary URLs. Run it on a private network, never on the open internet.
package api

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
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
	// Checks are named dependency probes for GET /healthz.
	Checks map[string]func(ctx context.Context) error
	// Images serves GET /img/{id}, the cached image proxy; nil disables it.
	Images http.Handler
	// ReadOnly leaves POST /crawls unregistered, for a public deployment: the
	// crawler fetches whatever URL it is given, so only trusted callers get it.
	ReadOnly bool
	Logger   *slog.Logger
}

// NewHandler returns the API mux.
func NewHandler(deps Deps) http.Handler {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	s := &server{Deps: deps}

	mux := http.NewServeMux()
	if !deps.ReadOnly {
		mux.HandleFunc("POST /crawls", s.createCrawl)
	}
	mux.HandleFunc("GET /crawls", s.listCrawls)
	mux.HandleFunc("GET /crawls/{id}/depths", s.crawlDepths)
	mux.HandleFunc("GET /crawls/{id}", s.getCrawl)
	mux.HandleFunc("GET /search", s.search)
	mux.HandleFunc("GET /digest", s.digest)
	mux.HandleFunc("GET /digest/sources", s.digestSources)
	if deps.Images != nil {
		mux.Handle("GET /img/{id}", deps.Images)
	}
	mux.HandleFunc("GET /healthz", s.healthz)

	static, _ := fs.Sub(staticFiles, "static")
	mux.Handle("GET /", http.FileServerFS(static))

	return mux
}

type server struct{ Deps }

type createCrawlRequest struct {
	Seed         string   `json:"seed"`
	MaxDepth     *int     `json:"max_depth"`
	AllowedHosts []string `json:"allowed_hosts"`
	PathPrefixes []string `json:"path_prefixes"`
	// ArticlePrefixes and SplitArticles shape the digest; see frontier.CrawlSpec.
	ArticlePrefixes []string `json:"article_prefixes"`
	SplitArticles   bool     `json:"split_articles"`
	ImageFromSource bool     `json:"image_from_source"`
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

	crawl, err := frontier.StartCrawlSpec(r.Context(), s.Frontier, frontier.CrawlSpec{
		Seed:            seed,
		MaxDepth:        maxDepth,
		AllowedHosts:    req.AllowedHosts,
		PathPrefixes:    req.PathPrefixes,
		ArticlePrefixes: req.ArticlePrefixes,
		SplitArticles:   req.SplitArticles,
		ImageFromSource: req.ImageFromSource,
	})
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

// pageParams reads page (1-based) and page_size (limit is accepted as an alias
// for older clients) and returns the page size and offset.
func pageParams(r *http.Request, defaultSize, maxSize int) (page, size, offset int, err error) {
	page, size = 1, defaultSize

	if v := r.URL.Query().Get("page"); v != "" {
		n, convErr := strconv.Atoi(v)
		if convErr != nil || n < 1 {
			return 0, 0, 0, errors.New("page must be 1 or more")
		}
		page = n
	}

	for _, key := range []string{"page_size", "limit"} {
		if v := r.URL.Query().Get(key); v != "" {
			n, convErr := strconv.Atoi(v)
			if convErr != nil || n < 1 || n > maxSize {
				return 0, 0, 0, fmt.Errorf("%s must be between 1 and %d", key, maxSize)
			}
			size = n
			break
		}
	}

	return page, size, (page - 1) * size, nil
}

// pageInfo is the pagination block added to list responses.
func pageInfo(page, size, total int) map[string]int {
	pages := (total + size - 1) / size
	return map[string]int{"page": page, "page_size": size, "total": total, "total_pages": pages}
}

func (s *server) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}

	page, size, offset, err := pageParams(r, 20, 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	hits, total, err := s.Store.Search(r.Context(), q, size, offset)
	if err != nil {
		s.Logger.Error("search failed", "error", err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	if hits == nil {
		hits = []store.Hit{} // encode as [] rather than null
	}

	resp := map[string]any{"query": q, "hits": hits}
	for k, v := range pageInfo(page, size, total) {
		resp[k] = v
	}
	writeJSON(w, http.StatusOK, resp)
}

// digestQuery builds the store query shared by /digest and /digest/sources.
func digestQuery(r *http.Request) (store.DigestQuery, time.Time, error) {
	days := 1
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 30 {
			return store.DigestQuery{}, time.Time{}, errors.New("days must be between 1 and 30")
		}
		days = n
	}

	since := time.Now().UTC().AddDate(0, 0, -days)
	return store.DigestQuery{
		Since: since,
		Query: strings.TrimSpace(r.URL.Query().Get("q")),
		// Articles only by default so listing and index pages stay out of the digest.
		ArticlesOnly: r.URL.Query().Get("articles_only") != "false",
		Source:       strings.ToLower(strings.TrimSpace(r.URL.Query().Get("source"))),
	}, since, nil
}

func (s *server) digest(w http.ResponseWriter, r *http.Request) {
	q, since, err := digestQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	page, size, offset, err := pageParams(r, 24, 200)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	q.Limit, q.Offset = size, offset

	items, total, err := s.Store.Digest(r.Context(), q)
	if err != nil {
		s.Logger.Error("digest failed", "error", err)
		writeError(w, http.StatusInternalServerError, "digest failed")
		return
	}

	// Images are served from our own origin (cached, no hotlink or mixed-content
	// failures); an item with no image gets an empty string.
	for i := range items {
		if items[i].Image != "" && s.Images != nil {
			items[i].Image = "/img/" + items[i].ID
		}
	}

	resp := map[string]any{"since": since, "items": items}
	for k, v := range pageInfo(page, size, total) {
		resp[k] = v
	}

	// The data changes once a day: let browsers and proxies reuse it for a few minutes.
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, resp)
}

// digestSources lists the hosts in the digest window with their item counts,
// for the page's source filter.
func (s *server) digestSources(w http.ResponseWriter, r *http.Request) {
	q, _, err := digestQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sources, err := s.Store.DigestSources(r.Context(), q)
	if err != nil {
		s.Logger.Error("digest sources failed", "error", err)
		writeError(w, http.StatusInternalServerError, "digest sources failed")
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, map[string]any{"sources": sources})
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
