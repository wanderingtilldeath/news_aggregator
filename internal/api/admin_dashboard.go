package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

const (
	maxSubscriberQuery = 100
	topSources         = 15
	recentCrawls       = 8
	// sourceSilentDays is how long a source may go without a new article before the dashboard flags it.
	sourceSilentDays = 3
)

type crawlRow struct {
	ID       int64     `json:"id"`
	Seed     string    `json:"seed"`
	Category string    `json:"category"`
	Created  time.Time `json:"created_at"`
	Total    int64     `json:"total"`
	Done     bool      `json:"done"`
	Pending  int64     `json:"pending"`
	Failed   int64     `json:"failed"`
	Blocked  int64     `json:"blocked"`
}

type contentView struct {
	Daily []struct {
		Day      string `json:"day"`
		Articles int    `json:"articles"`
	} `json:"daily"`
	Waiting struct {
		Summary    int `json:"summary"`
		Image      int `json:"image"`
		Enrichment int `json:"enrichment"`
		Detail     int `json:"detail"`
	} `json:"waiting"`
	// LatestArticle is when the newest article was first seen, empty when there is none.
	LatestArticle string `json:"latest_article"`
	Categories    []struct {
		Category string `json:"category"`
		Count    int    `json:"count"`
	} `json:"categories"`
	Sources []store.SourceCount `json:"sources"`
	Crawls  []crawlRow          `json:"crawls"`
	// SourceHealth is one row per source in sources.json that ran in the last 30 days, worst first: a source that
	// found no article in the last three days comes before the healthy ones.
	SourceHealth []sourceHealthRow `json:"source_health"`
	// Storage is the database size against its plan's limit, with the biggest tables, so a full free tier is seen
	// before it stops the crawl.
	Storage storageView `json:"storage"`
}

type storageView struct {
	DBBytes    int64 `json:"db_bytes"`
	LimitBytes int64 `json:"limit_bytes"`
	// Percent is DBBytes of LimitBytes, 0 when no limit is set; Warn is true from storageWarnPercent.
	Percent int  `json:"percent"`
	Warn    bool `json:"warn"`
	Tables  []struct {
		Name  string `json:"name"`
		Bytes int64  `json:"bytes"`
	} `json:"tables"`
}

// storageWarnPercent is where the Storage table turns to a warning.
const storageWarnPercent = 70

type sourceHealthRow struct {
	Source         string `json:"source"`
	Host           string `json:"host"`
	LastRun        string `json:"last_run"`
	LastOutcome    string `json:"last_outcome"`
	LastOK         string `json:"last_ok"`
	Runs           int    `json:"runs"`
	Articles       int    `json:"articles"`
	RecentArticles int    `json:"recent_articles"`
	Failed         int    `json:"failed"`
	// Silent is true when the source found no article in the last three days.
	Silent bool `json:"silent"`
}

// adminContent is the news side of the dashboard: articles per day, what each pipeline stage still has waiting,
// the last week's stories by category and source, and the latest crawls.
func (s *server) adminContent(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, permContent); !ok {
		return
	}
	days := 14
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 90 {
			writeError(w, http.StatusBadRequest, "days must be 1 to 90")
			return
		}
		days = n
	}

	c, err := s.Store.ContentHealth(r.Context(), days, editionZone.String())
	if err != nil {
		s.Logger.Error("admin content", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load the numbers")
		return
	}
	sources, err := s.Store.DigestSources(r.Context(), store.DigestQuery{Since: time.Now().AddDate(0, 0, -7), ArticlesOnly: true})
	if err != nil {
		s.Logger.Error("admin content: sources", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load the numbers")
		return
	}
	crawls, err := s.Frontier.ListCrawls(r.Context(), recentCrawls)
	if err != nil {
		s.Logger.Error("admin content: crawls", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load the numbers")
		return
	}

	health, err := s.Store.SourceHealth(r.Context(), time.Now().AddDate(0, 0, -30), time.Now().AddDate(0, 0, -sourceSilentDays))
	if err != nil {
		s.Logger.Error("admin content: source health", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load the numbers")
		return
	}

	var out contentView
	stats, err := s.Store.Storage(r.Context())
	if err != nil {
		s.Logger.Error("admin content: storage", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load the numbers")
		return
	}
	out.Storage.DBBytes = stats.DBBytes
	if s.DBLimitMB > 0 {
		out.Storage.LimitBytes = int64(s.DBLimitMB) << 20
		out.Storage.Percent = int(stats.DBBytes * 100 / out.Storage.LimitBytes)
		out.Storage.Warn = out.Storage.Percent >= storageWarnPercent
	}
	out.Storage.Tables = []struct {
		Name  string `json:"name"`
		Bytes int64  `json:"bytes"`
	}{}
	for _, t := range stats.Tables {
		out.Storage.Tables = append(out.Storage.Tables, struct {
			Name  string `json:"name"`
			Bytes int64  `json:"bytes"`
		}{t.Name, t.Bytes})
	}
	out.SourceHealth = []sourceHealthRow{}
	for _, h := range health {
		row := sourceHealthRow{Source: h.Source, Host: h.Host, LastRun: h.LastRun.UTC().Format(time.RFC3339), LastOutcome: h.LastOutcome, Runs: h.Runs,
			Articles: h.Articles, RecentArticles: h.RecentArticles, Failed: h.Failed, Silent: h.RecentArticles == 0}
		if !h.LastOK.IsZero() {
			row.LastOK = h.LastOK.UTC().Format(time.RFC3339)
		}
		out.SourceHealth = append(out.SourceHealth, row)
	}
	sort.SliceStable(out.SourceHealth, func(i, j int) bool {
		if a, b := out.SourceHealth[i], out.SourceHealth[j]; a.Silent != b.Silent {
			return a.Silent
		}
		return false
	})
	for _, d := range c.Daily {
		out.Daily = append(out.Daily, struct {
			Day      string `json:"day"`
			Articles int    `json:"articles"`
		}{d.Day, d.Articles})
	}
	out.Waiting.Summary, out.Waiting.Image, out.Waiting.Enrichment, out.Waiting.Detail = c.Waiting.Summary, c.Waiting.Image, c.Waiting.Enrichment, c.Waiting.Detail
	if !c.Latest.IsZero() {
		out.LatestArticle = c.Latest.UTC().Format(time.RFC3339)
	}

	byCategory := map[string]int{}
	for _, sc := range sources {
		cat := sc.Category
		if cat == "" {
			cat = "other"
		}
		byCategory[cat] += sc.Count
	}
	for cat, n := range byCategory {
		out.Categories = append(out.Categories, struct {
			Category string `json:"category"`
			Count    int    `json:"count"`
		}{cat, n})
	}
	sort.Slice(out.Categories, func(i, j int) bool {
		if out.Categories[i].Count != out.Categories[j].Count {
			return out.Categories[i].Count > out.Categories[j].Count
		}
		return out.Categories[i].Category < out.Categories[j].Category
	})
	sort.SliceStable(sources, func(i, j int) bool { return sources[i].Count > sources[j].Count })
	out.Sources = sources[:min(topSources, len(sources))]
	if out.Sources == nil {
		out.Sources = []store.SourceCount{}
	}

	out.Crawls = []crawlRow{}
	for _, cr := range crawls {
		v, err := s.crawlView(r.Context(), cr)
		if err != nil {
			s.Logger.Error("admin content: crawl stats", "err", err)
			writeError(w, http.StatusInternalServerError, "could not load the numbers")
			return
		}
		out.Crawls = append(out.Crawls, crawlRow{
			ID: cr.ID, Seed: cr.Seed, Category: cr.Category, Created: cr.CreatedAt, Total: v.Total, Done: v.Done,
			Pending: v.Stats["pending"] + v.Stats["leased"], Failed: v.Stats["failed"] + v.Stats["dead"], Blocked: v.Stats["blocked"],
		})
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

type subscriberView struct {
	ID        int64    `json:"id"`
	Email     string   `json:"email"`
	Status    string   `json:"status"`
	Mode      string   `json:"mode"`
	Topics    []string `json:"topics"`
	Source    string   `json:"source"`
	Created   string   `json:"created"`
	Confirmed string   `json:"confirmed"`
}

// adminSubscribers lists subscribers, newest first, with an optional search on the address. It shows addresses,
// so it needs its own permission; it never shows a token.
func (s *server) adminSubscribers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, permSubscribersRead); !ok {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > maxSubscriberQuery {
		writeError(w, http.StatusBadRequest, "search text is too long")
		return
	}
	page, size, offset, err := pageParams(r, 25, 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rows, total, err := s.Store.Subscribers(r.Context(), q, size, offset)
	if err != nil {
		s.Logger.Error("admin subscribers", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load subscribers")
		return
	}

	out := make([]subscriberView, 0, len(rows))
	for _, row := range rows {
		v := subscriberView{ID: row.ID, Email: row.Email, Status: row.Status, Mode: row.Mode, Topics: row.Topics, Source: row.Source, Created: row.Created.UTC().Format(time.RFC3339)}
		if v.Topics == nil {
			v.Topics = []string{}
		}
		if !row.Confirmed.IsZero() {
			v.Confirmed = row.Confirmed.UTC().Format(time.RFC3339)
		}
		out = append(out, v)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"subscribers": out, "total": total, "page": page, "page_size": size})
}

// adminRemoveSubscriber deletes a subscriber for good (their delivery records go with them). The log keeps who
// did it and the id, not the address.
func (s *server) adminRemoveSubscriber(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requirePermission(w, r, permSubscribersWrite)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "id must be a positive integer")
		return
	}
	found, err := s.Store.RemoveSubscriber(r.Context(), id)
	if err != nil {
		s.Logger.Error("admin remove subscriber", "err", err)
		writeError(w, http.StatusInternalServerError, "could not remove")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "no such subscriber")
		return
	}
	s.Logger.Info("admin removed a subscriber", "by", admin, "id", id)
	s.audit(r, admin, "subscriber.remove", strconv.FormatInt(id, 10), "")
	w.WriteHeader(http.StatusNoContent)
}
