package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/chat"
	"github.com/wanderingtilldeath/news-aggregator/internal/track"
)

const maxTeamBody = 2 << 10

type teamRequest struct {
	URL   string `json:"url"`
	Track string `json:"track"`
}

// joinTeam connects a Slack or Discord channel to a daily edition. The address is the channel's
// incoming webhook, a secret its owner made, so it is checked hard (package chat). It is stored, then tried
// once with a welcome message that carries the link to stop the posts; if the channel refuses the welcome the
// new connection is removed. Joining again with the same edition posts nothing and answers the same.
func (s *server) joinTeam(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxTeamBody)

	var req teamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	hook := strings.TrimSpace(req.URL)
	kind, ok := chat.Kind(hook)
	if !ok {
		writeError(w, http.StatusBadRequest, "paste the incoming webhook address of a Slack or Discord channel")
		return
	}
	if !track.Valid(req.Track) {
		writeError(w, http.StatusBadRequest, "unknown edition")
		return
	}
	if !s.teams.allow(time.Now()) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "too many requests right now, try again in a minute")
		return
	}

	token, err := newToken()
	if err != nil {
		s.Logger.Error("teams: token", "err", err)
		writeError(w, http.StatusInternalServerError, "could not connect the channel")
		return
	}
	// Stored first, so the stop link in the welcome is one that works: the token kept for an address already
	// connected is the one it was given the first time, not the new one made here.
	h, announce, err := s.Store.ConnectTeamHook(r.Context(), kind, hook, req.Track, token)
	if err != nil {
		s.Logger.Error("teams: store", "err", err)
		writeError(w, http.StatusInternalServerError, "could not connect the channel")
		return
	}
	// A repeat of the same join posts nothing, so pasting an address again cannot make the channel noisy.
	if announce {
		welcome := "Postpaper is connected. This channel will get the " + track.Title(h.Track) + " edition once a day. Stop these posts any time: " +
			strings.TrimRight(s.WebBase, "/") + "/teams/stop?token=" + url.QueryEscape(h.Token)
		if err := s.TeamPoster.Post(r.Context(), kind, hook, welcome); err != nil {
			// The address is a secret and stays out of the log.
			s.Logger.Warn("teams: the welcome post failed", "kind", kind, "err", err)
			// A channel that took no test message is not kept; an existing one only changed edition, so it stays.
			if h.Token == token {
				if _, rerr := s.Store.RemoveTeamHook(r.Context(), token); rerr != nil {
					s.Logger.Error("teams: remove after a failed welcome", "err", rerr)
				}
			}
			writeError(w, http.StatusBadRequest, "the channel did not accept the test message: check the webhook address")
			return
		}
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "connected"})
}

// stopTeam disconnects the channel whose stop link carried this token.
func (s *server) stopTeam(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxTeamBody)

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" || len(req.Token) > maxTokenLen {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}
	removed, err := s.Store.RemoveTeamHook(r.Context(), req.Token)
	if err != nil {
		s.Logger.Error("teams: remove", "err", err)
		writeError(w, http.StatusInternalServerError, "could not stop the posts")
		return
	}
	if !removed {
		writeError(w, http.StatusNotFound, "this link is not valid or the posts already stopped")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}
