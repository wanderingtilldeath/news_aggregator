package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

const maxBrevoBody = 64 << 10

// brevoEvent is the part of a Brevo transactional webhook this server reads.
type brevoEvent struct {
	Event  string `json:"event"`
	Email  string `json:"email"`
	Reason string `json:"reason"`
}

// brevoWebhook receives Brevo's delivery events. A hard bounce, a blocked address or an invalid one stops mail to
// it; a spam complaint also unsubscribes it. Other events (delivered, opened, soft bounce) are acknowledged and
// ignored. Brevo does not sign webhooks, so the secret in the address is the only check; a wrong one gets 401.
func (s *server) brevoWebhook(w http.ResponseWriter, r *http.Request) {
	if !sameSecret(r.URL.Query().Get("secret"), s.BrevoSecret) {
		writeError(w, http.StatusUnauthorized, "bad secret")
		return
	}

	var ev brevoEvent
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBrevoBody)).Decode(&ev); err != nil {
		writeError(w, http.StatusBadRequest, "bad body")
		return
	}

	complaint := ev.Event == "spam" || ev.Event == "complaint"
	switch ev.Event {
	case "hard_bounce", "blocked", "invalid_email", "invalid", "error", "spam", "complaint":
	default:
		w.WriteHeader(http.StatusOK)
		return
	}

	reason := ev.Event
	if why := strings.TrimSpace(ev.Reason); why != "" {
		reason += ": " + why[:min(len(why), 200)]
	}
	if _, err := s.Store.MarkBounced(r.Context(), strings.TrimSpace(ev.Email), reason, complaint); err != nil {
		s.Logger.Error("brevo webhook", "err", err)
		writeError(w, http.StatusInternalServerError, "could not record it")
		return
	}
	s.Logger.Info("mail suppressed", "event", ev.Event)

	w.WriteHeader(http.StatusOK)
}
