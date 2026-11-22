package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxAdminBody   = 4 << 10
	maxGrantDays   = 366
	maxPaymentID   = 64
	auditShown     = 20
	unmatchedShown = 20
)

// audit records an admin action. A failure is logged and never undoes the action: the log line says who did it.
func (s *server) audit(r *http.Request, actor, action, target, detail string) {
	if err := s.Store.AddAudit(context.WithoutCancel(r.Context()), actor, action, target, detail); err != nil {
		s.Logger.Error("admin audit", "err", err, "by", actor, "action", action, "target", target)
	}
}

type proMemberView struct {
	Email    string `json:"email"`
	Until    string `json:"until"`
	Active   bool   `json:"active"`
	Username string `json:"username"`
	Payments int    `json:"payments"`
}

type unmatchedView struct {
	PaymentID string `json:"payment_id"`
	Email     string `json:"email"`
	Rupees    string `json:"rupees"`
	At        string `json:"at"`
}

type auditView struct {
	At     string `json:"at"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Target string `json:"target"`
	Detail string `json:"detail"`
}

// adminPro lists Pro members (search on the address), the payments that matched no plan, and the latest admin
// actions. It shows addresses and never a token.
func (s *server) adminPro(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, permProRead); !ok {
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
	rows, total, err := s.Store.ProMembers(r.Context(), q, size, offset)
	if err != nil {
		s.Logger.Error("admin pro", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load members")
		return
	}
	unmatched, err := s.Store.UnmatchedPayments(r.Context(), unmatchedShown)
	if err != nil {
		s.Logger.Error("admin pro unmatched", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load payments")
		return
	}
	log, err := s.Store.RecentAudit(r.Context(), auditShown)
	if err != nil {
		s.Logger.Error("admin pro audit", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load the log")
		return
	}

	now := time.Now()
	members := make([]proMemberView, 0, len(rows))
	for _, row := range rows {
		members = append(members, proMemberView{Email: row.Email, Until: row.Until.UTC().Format(time.RFC3339), Active: row.Until.After(now), Username: row.Username, Payments: row.Payments})
	}
	un := make([]unmatchedView, 0, len(unmatched))
	for _, u := range unmatched {
		un = append(un, unmatchedView{PaymentID: u.PaymentID, Email: u.Email, Rupees: strconv.FormatFloat(float64(u.Amount)/100, 'f', 2, 64), At: u.At.UTC().Format(time.RFC3339)})
	}
	au := make([]auditView, 0, len(log))
	for _, a := range log {
		au = append(au, auditView{At: a.At.UTC().Format(time.RFC3339), Actor: a.Actor, Action: a.Action, Target: a.Target, Detail: a.Detail})
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"members": members, "total": total, "page": page, "page_size": size, "unmatched": un, "audit": au})
}

type grantProRequest struct {
	Email string `json:"email"`
	Days  int    `json:"days"`
	// PaymentID, when set, is the Razorpay payment this grant settles (one from the unmatched list); repeating it
	// changes nothing. Empty makes a manual grant with its own id.
	PaymentID string `json:"payment_id"`
}

// adminGrantPro gives an address Pro for days, through the same ProExtend a payment uses, so it is recorded in
// pro_payments and a repeat of the same payment id cannot extend twice. A new member gets the welcome mail with
// their link, as a paying one does.
func (s *server) adminGrantPro(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requirePermission(w, r, permProWrite)
	if !ok {
		return
	}
	var req grantProRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAdminBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	email, ok := parseEmail(req.Email)
	if !ok {
		writeError(w, http.StatusBadRequest, "enter a valid email address")
		return
	}
	if req.Days < 1 || req.Days > maxGrantDays {
		writeError(w, http.StatusBadRequest, "days must be from 1 to "+strconv.Itoa(maxGrantDays))
		return
	}
	id := strings.TrimSpace(req.PaymentID)
	if len(id) > maxPaymentID {
		writeError(w, http.StatusBadRequest, "payment id is too long")
		return
	}
	if id == "" {
		b := make([]byte, 6)
		if _, err := rand.Read(b); err != nil {
			writeError(w, http.StatusInternalServerError, "could not grant")
			return
		}
		id = "manual-" + strconv.FormatInt(time.Now().Unix(), 10) + "-" + hex.EncodeToString(b)
	}
	token, err := newToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not grant")
		return
	}

	member, fresh, err := s.Store.ProExtend(r.Context(), id, email, req.Days, token)
	if err != nil {
		s.Logger.Error("admin grant pro", "err", err)
		writeError(w, http.StatusInternalServerError, "could not grant")
		return
	}
	if fresh {
		s.Logger.Info("admin granted pro", "by", admin, "days", req.Days)
		s.audit(r, admin, "pro.grant", email, strconv.Itoa(req.Days)+" days, payment "+id)
		if s.ProNotify != nil {
			s.ProNotify(r.Context(), member.Email, member.Token, member.Until)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"granted": fresh, "until": member.Until.UTC().Format(time.RFC3339)})
}

type revokeProRequest struct {
	Email string `json:"email"`
}

// adminRevokePro ends a member's period now. Their notes and payment records stay, and so does the membership row, so
// a later payment or grant extends it as before.
func (s *server) adminRevokePro(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requirePermission(w, r, permProWrite)
	if !ok {
		return
	}
	var req revokeProRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAdminBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	email, ok := parseEmail(req.Email)
	if !ok {
		writeError(w, http.StatusBadRequest, "enter a valid email address")
		return
	}
	found, err := s.Store.ProRevoke(r.Context(), email)
	if err != nil {
		s.Logger.Error("admin revoke pro", "err", err)
		writeError(w, http.StatusInternalServerError, "could not revoke")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "no such member")
		return
	}
	s.Logger.Info("admin revoked pro", "by", admin)
	s.audit(r, admin, "pro.revoke", email, "")

	w.WriteHeader(http.StatusNoContent)
}
