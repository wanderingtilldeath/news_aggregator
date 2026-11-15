package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxBillingBody = 64 << 10

// razorpayEvent is the part of a Razorpay webhook this server reads. A payment link sends
// payment_link.paid and payment.captured for the same payment; both carry the payment id.
type razorpayEvent struct {
	Event   string `json:"event"`
	Payload struct {
		Payment struct {
			Entity struct {
				ID     string `json:"id"`
				Email  string `json:"email"`
				Amount int    `json:"amount"` // paise
			} `json:"entity"`
		} `json:"payment"`
		PaymentLink struct {
			Entity struct {
				Customer struct {
					Email string `json:"email"`
				} `json:"customer"`
			} `json:"entity"`
		} `json:"payment_link"`
	} `json:"payload"`
}

// billing receives Razorpay's webhook and makes the payer a Pro member for the days their payment
// buys (ProPlans, by amount in paise). The
// body is trusted only when its HMAC-SHA256, made with the webhook secret, matches the signature
// header. Events other than a captured payment get 200 so Razorpay does not retry them; a bad
// signature gets 400.
func (s *server) billing(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBillingBody))
	if err != nil {
		writeError(w, http.StatusBadRequest, "body too large")
		return
	}

	mac := hmac.New(sha256.New, []byte(s.RazorpaySecret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(strings.ToLower(r.Header.Get("X-Razorpay-Signature")))) {
		writeError(w, http.StatusBadRequest, "bad signature")
		return
	}

	var ev razorpayEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if ev.Event != "payment_link.paid" && ev.Event != "payment.captured" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	id := strings.TrimSpace(ev.Payload.Payment.Entity.ID)
	email, ok := parseEmail(ev.Payload.Payment.Entity.Email)
	if !ok {
		email, ok = parseEmail(ev.Payload.PaymentLink.Entity.Customer.Email)
	}
	if id == "" || !ok {
		// Nothing to act on, and a retry would not help: say so in the log and acknowledge.
		s.Logger.Warn("billing: payment without an id or a usable email", "event", ev.Event, "payment", id)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	days, known := s.ProPlans[ev.Payload.Payment.Entity.Amount]
	if !known {
		// Acknowledge it (a retry would not help) but leave a trace: the owner sees a payment that
		// bought nothing and decides, rather than the server guessing a period.
		s.Logger.Warn("billing: payment amount matches no plan", "payment", id, "amount", ev.Payload.Payment.Entity.Amount)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	token, err := newToken()
	if err != nil {
		s.Logger.Error("billing: token", "err", err)
		writeError(w, http.StatusInternalServerError, "could not record the payment")
		return
	}
	member, fresh, err := s.Store.ProExtend(r.Context(), id, email, days, token)
	if err != nil {
		// A 500 makes Razorpay retry, which is right: the payment is not recorded yet.
		s.Logger.Error("billing: record payment", "err", err, "payment", id)
		writeError(w, http.StatusInternalServerError, "could not record the payment")
		return
	}
	if fresh && s.ProNotify != nil {
		s.ProNotify(r.Context(), member.Email, member.Token, member.Until)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// proStatus tells the web page whether a member's token is valid and until when. It never lists
// or reveals anything else about the member.
func (s *server) proStatus(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" || len(token) > maxTokenLen {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}
	m, ok, err := s.Store.ProByToken(r.Context(), token)
	if err != nil {
		s.Logger.Error("pro status", "err", err)
		writeError(w, http.StatusInternalServerError, "could not check the link")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "this link is not valid")
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"active": m.Until.After(time.Now()), "until": m.Until.UTC().Format("2006-01-02")})
}
