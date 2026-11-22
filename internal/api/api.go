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
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/chat"
	"github.com/wanderingtilldeath/news-aggregator/internal/frontier"
	"github.com/wanderingtilldeath/news-aggregator/internal/kind"
	"github.com/wanderingtilldeath/news-aggregator/internal/sourcetype"
	"github.com/wanderingtilldeath/news-aggregator/internal/store"
	"github.com/wanderingtilldeath/news-aggregator/internal/track"
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
	// ReadOnly leaves POST /crawls unregistered. It is the default (config READ_ONLY): the
	// crawler fetches whatever URL it is given, so only trusted callers get it.
	ReadOnly bool
	// Notify is called with the confirmation token of an address that still has
	// to confirm; the mailer sends the link. nil drops it.
	Notify func(ctx context.Context, email, token string)
	// RazorpaySecret is the webhook secret of the Razorpay account; empty leaves POST /billing/razorpay
	// unregistered. ProPlans maps an amount paid, in paise, to the days it buys; a payment of any other
	// amount is logged and ignored. ProNotify sends the new
	// member their link (nil drops it).
	RazorpaySecret string
	// BrevoSecret is the secret in the address of Brevo's bounce webhook; empty leaves POST /brevo/webhook unregistered.
	BrevoSecret string
	ProPlans    map[int]int
	ProNotify   func(ctx context.Context, email, token string, until time.Time)
	// ManageNotify mails a subscriber the link to manage their subscription (POST /subscription/link); nil drops it.
	ManageNotify func(ctx context.Context, email, token string)
	// GoogleClientID and GoogleClientSecret turn on sign in with Google (POST /account/google); with either empty the
	// route is not registered. GoogleTokenURL and HTTPClient are for tests; the defaults are Google's token endpoint and
	// a client with a ten-second timeout.
	GoogleClientID, GoogleClientSecret, GoogleTokenURL string
	HTTPClient                                         *http.Client
	// AdminUsernames are the accounts that may be admin and AdminPassword the password they enter after logging in
	// to their account. The admin routes (sign-in, the dashboard's data and GET /analytics) are registered only with
	// at least one username and a password of 12 or more characters.
	AdminUsernames []string
	AdminPassword  string
	// TeamPoster posts to a team's Slack or Discord webhook; nil leaves POST /teams unregistered.
	// WebBase is the public address of the web site, for the "stop these posts" link.
	TeamPoster chat.Poster
	WebBase    string
	Logger     *slog.Logger
}

// NewHandler returns the API mux.
func NewHandler(deps Deps) http.Handler {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	for i, u := range deps.AdminUsernames {
		deps.AdminUsernames[i] = strings.ToLower(strings.TrimSpace(u))
	}
	s := &server{Deps: deps}
	s.manageMails.gap = manageGap

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
	mux.HandleFunc("GET /story/{id}", s.story)
	mux.HandleFunc("GET /quiz", s.quiz)
	mux.HandleFunc("GET /intro", s.intro)
	mux.HandleFunc("GET /cards", s.cards)
	mux.HandleFunc("GET /month", s.month)
	mux.HandleFunc("GET /concepts", s.concepts)
	mux.HandleFunc("GET /entities", s.entities)
	mux.HandleFunc("GET /months", s.months)
	if deps.Images != nil {
		mux.Handle("GET /img/{id}", deps.Images)
	}
	// Subscribing stays open on a ReadOnly box: it only writes a subscriber row.
	mux.HandleFunc("POST /subscribe", s.subscribe)
	mux.HandleFunc("POST /confirm", s.confirm)
	mux.HandleFunc("POST /unsubscribe", s.unsubscribe)
	mux.HandleFunc("POST /account/signup", s.accountSignup)
	mux.HandleFunc("POST /account/login", s.accountLogin)
	mux.HandleFunc("POST /account/recover", s.accountRecover)
	mux.HandleFunc("POST /account/logout", s.accountLogout)
	mux.HandleFunc("GET /account/me", s.accountMe)
	mux.HandleFunc("POST /account/pro", s.accountLinkPro)
	mux.HandleFunc("POST /account/password", s.accountPassword)
	mux.HandleFunc("POST /account/email", s.accountEmail)
	mux.HandleFunc("POST /account/codes", s.accountNewCodes)
	mux.HandleFunc("POST /account/logout-all", s.accountLogoutAll)
	mux.HandleFunc("POST /account/delete", s.accountDelete)
	if deps.GoogleClientID != "" && deps.GoogleClientSecret != "" {
		mux.HandleFunc("POST /account/google", s.accountGoogle)
	}
	mux.HandleFunc("POST /subscription/link", s.manageLink)
	mux.HandleFunc("GET /subscription", s.subscription)
	mux.HandleFunc("POST /subscription", s.updateSubscription)
	if adminEnabled(deps) {
		mux.HandleFunc("POST /admin/login", s.adminLogin)
		mux.HandleFunc("POST /admin/logout", s.adminLogout)
		mux.HandleFunc("GET /admin/me", s.adminMe)
		mux.HandleFunc("GET /analytics", s.analytics)
		mux.HandleFunc("GET /admin/content", s.adminContent)
		mux.HandleFunc("GET /admin/subscribers", s.adminSubscribers)
		mux.HandleFunc("DELETE /admin/subscribers/{id}", s.adminRemoveSubscriber)
		mux.HandleFunc("GET /admin/pro", s.adminPro)
		mux.HandleFunc("POST /admin/pro", s.adminGrantPro)
		mux.HandleFunc("POST /admin/pro/revoke", s.adminRevokePro)
	}
	mux.HandleFunc("GET /pro", s.proStatus)
	mux.HandleFunc("GET /pro/notes", s.listProNotes)
	mux.HandleFunc("PUT /pro/notes/{id}", s.saveProNote)
	mux.HandleFunc("POST /pro/notes/import", s.importProNotes)
	mux.HandleFunc("DELETE /pro/notes/{id}", s.deleteProNote)
	mux.HandleFunc("DELETE /pro/notes", s.deleteProNote)
	if deps.TeamPoster != nil {
		mux.HandleFunc("POST /teams", s.joinTeam)
		mux.HandleFunc("POST /teams/stop", s.stopTeam)
	}
	if deps.RazorpaySecret != "" {
		mux.HandleFunc("POST /billing/razorpay", s.billing)
	}
	if deps.BrevoSecret != "" {
		mux.HandleFunc("POST /brevo/webhook", s.brevoWebhook)
	}
	mux.HandleFunc("GET /healthz", s.healthz)

	static, _ := fs.Sub(staticFiles, "static")
	mux.Handle("GET /", http.FileServerFS(static))
	// The news page also answers at its old address: shared links and the
	// Render health check use it.
	mux.HandleFunc("GET /digest.html", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, static, "index.html")
	})

	return mux
}

type server struct {
	Deps
	confirmMails recent
	logins       attempts
	accountTries attempts
	manageMails  recent
	signups      limiter
	manageLinks  limiter
	teams        limiter
}

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
// editionZone is the time zone an edition day (day=) is counted in.
var editionZone = func() *time.Location {
	if loc, err := time.LoadLocation("Asia/Kolkata"); err == nil {
		return loc
	}

	return time.FixedZone("IST", 5*3600+1800)
}()

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
	var until time.Time
	// day=YYYY-MM-DD asks for one edition day (an archive page), in India time.
	// shortcut: the zone is fixed; make it a setting when readers outside India matter.
	if v := r.URL.Query().Get("day"); v != "" {
		start, err := time.ParseInLocation("2006-01-02", v, editionZone)
		if err != nil {
			return store.DigestQuery{}, time.Time{}, errors.New("day must be YYYY-MM-DD")
		}
		since, until = start.UTC(), start.AddDate(0, 0, 1).UTC()
	}

	detail := r.URL.Query().Get("detail")
	return store.DigestQuery{
		Since:  since,
		Until:  until,
		Detail: detail == "1" || detail == "true",
		Query:  strings.TrimSpace(r.URL.Query().Get("q")),
		// Articles only by default so listing and index pages stay out of the digest.
		ArticlesOnly: r.URL.Query().Get("articles_only") != "false",
		Source:       strings.ToLower(strings.TrimSpace(r.URL.Query().Get("source"))),
		Category:     strings.ToLower(strings.TrimSpace(r.URL.Query().Get("category"))),
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
		items[i].SourceType = sourcetype.For(items[i].Source)
		items[i].Kind = kindOf(items[i].URL)
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

// quiz returns the quiz of a track for an edition day (day=YYYY-MM-DD), or the
// latest one when day is missing. The answers are part of the answer: the web page
// reveals them one question at a time, and a quiz is not a secret.
func (s *server) quiz(w http.ResponseWriter, r *http.Request) {
	tr := r.URL.Query().Get("track")
	if !track.Valid(tr) {
		writeError(w, http.StatusBadRequest, "track must be one of: "+strings.Join(track.All(), ", "))
		return
	}
	day := r.URL.Query().Get("day")
	if day != "" {
		if _, err := time.Parse("2006-01-02", day); err != nil {
			writeError(w, http.StatusBadRequest, "day must be YYYY-MM-DD")
			return
		}
	}

	q, ok, err := s.Store.GetQuiz(r.Context(), day, tr)
	if err != nil {
		s.Logger.Error("quiz failed", "error", err)
		writeError(w, http.StatusInternalServerError, "quiz failed")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "no quiz for that day yet")
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, q)
}

// intro returns the "Today in 60 seconds" paragraph of a track for an edition day (day=YYYY-MM-DD),
// or the latest one when day is missing.
func (s *server) intro(w http.ResponseWriter, r *http.Request) {
	tr := r.URL.Query().Get("track")
	if !track.Valid(tr) && tr != track.General {
		writeError(w, http.StatusBadRequest, "track must be one of: "+strings.Join(append(track.All(), track.General), ", "))
		return
	}
	day := r.URL.Query().Get("day")
	if day != "" {
		if _, err := time.Parse("2006-01-02", day); err != nil {
			writeError(w, http.StatusBadRequest, "day must be YYYY-MM-DD")
			return
		}
	}

	in, ok, err := s.Store.GetIntro(r.Context(), day, tr)
	if err != nil {
		s.Logger.Error("intro failed", "error", err)
		writeError(w, http.StatusInternalServerError, "intro failed")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "no intro for that day yet")
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, in)
}

// maxIDLen bounds a story id; real ids are 64 hex characters (a SHA-256).
const maxIDLen = 128

// story returns one article by id for its own page: the headline, summary, why it
// matters and tags. The long summary is left out, and the picture goes through the
// image proxy like in the digest.
func (s *server) story(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || len(id) > maxIDLen {
		writeError(w, http.StatusBadRequest, "invalid story id")
		return
	}

	it, ok, err := s.Store.Item(r.Context(), id)
	if err != nil {
		s.Logger.Error("story failed", "error", err)
		writeError(w, http.StatusInternalServerError, "story failed")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "story not found")
		return
	}
	it.SourceType = sourcetype.For(it.Source)
	it.Kind = kindOf(it.URL)
	for i := range it.Also {
		it.Also[i].SourceType = sourcetype.For(it.Also[i].Source)
	}
	if it.Image != "" && s.Images != nil {
		it.Image = "/img/" + it.ID
	}

	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, it)
}

// kindOf labels a story as opinion, analysis or an explainer from its URL path ("" for a report).
func kindOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	return kind.For(u.Path)
}
