package api

import (
	"net/http"
	"strings"
	"time"
)

// Email on an account is confirmed by a mailed link, and a confirmed address can reset the password. A link is a random
// secret (only its hash is stored), works once, and names the address it was made for.
const (
	verifyLinkTTL = 24 * time.Hour
	resetLinkTTL  = time.Hour
)

// mailAccountLink makes a link for the account's current address and mails it. It does nothing without a mailer.
func (s *server) mailAccountLink(r *http.Request, id int64, purpose, email string) {
	if s.AccountNotify == nil || email == "" {
		return
	}
	secret, err := newToken()
	if err != nil {
		s.Logger.Error("account link: token", "err", err)
		return
	}
	ttl := verifyLinkTTL
	if purpose == "reset" {
		ttl = resetLinkTTL
	}
	if err := s.Store.SaveEmailToken(r.Context(), hashToken(secret), id, purpose, email, time.Now().Add(ttl)); err != nil {
		s.Logger.Error("account link: save", "err", err)
		return
	}
	s.AccountNotify(r.Context(), purpose, email, secret)
}

// accountVerifyEmail spends a verify link. It needs no session: the link is the proof, and it is opened from the mail,
// maybe on another device.
func (s *server) accountVerifyEmail(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeAccount(w, r)
	if !ok {
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" || len(req.Token) > maxTokenLen {
		writeError(w, http.StatusBadRequest, "this link is not valid")
		return
	}
	now := time.Now()
	if s.throttled(w, "verify:link", now) {
		return
	}
	a, email, found, err := s.Store.UseEmailToken(r.Context(), hashToken(req.Token), "verify", now)
	if err != nil {
		s.Logger.Error("account verify", "err", err)
		writeError(w, http.StatusInternalServerError, "could not confirm the email")
		return
	}
	if !found {
		s.accountTries.fail("verify:link", now)
		writeError(w, http.StatusNotFound, "this link is not valid, has expired or was already used")
		return
	}
	done, err := s.Store.VerifyAccountEmail(r.Context(), a.ID, email)
	if err != nil {
		s.Logger.Error("account verify", "err", err)
		writeError(w, http.StatusInternalServerError, "could not confirm the email")
		return
	}
	if !done {
		writeError(w, http.StatusConflict, "another account already confirmed this address")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// accountResendVerify mails a fresh verify link to the logged-in account's unconfirmed address.
func (s *server) accountResendVerify(w http.ResponseWriter, r *http.Request) {
	a, ok := s.accountFrom(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "log in first")
		return
	}
	if a.Email == "" || a.EmailVerified {
		writeError(w, http.StatusBadRequest, "there is no unconfirmed email on this account")
		return
	}
	if s.throttled(w, "resend:"+a.Username, time.Now()) {
		return
	}
	s.accountTries.fail("resend:"+a.Username, time.Now())
	s.mailAccountLink(r, a.ID, "verify", a.Email)
	w.WriteHeader(http.StatusNoContent)
}

// accountResetRequest mails a reset link to a confirmed address. It answers the same whether or not the address belongs
// to an account, so it cannot be used to find out who has one, and the number of requests per address is limited.
func (s *server) accountResetRequest(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeAccount(w, r)
	if !ok {
		return
	}
	email, valid := cleanEmail(req.Email)
	if !valid || email == "" {
		writeError(w, http.StatusBadRequest, "enter the email address of your account")
		return
	}
	now := time.Now()
	key := "resetreq:" + email
	if s.throttled(w, key, now) {
		return
	}
	s.accountTries.fail(key, now)
	if a, found, err := s.Store.AccountByEmail(r.Context(), email); err != nil {
		s.Logger.Error("account reset request", "err", err)
	} else if found {
		s.mailAccountLink(r, a.ID, "reset", email)
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "if that address belongs to an account, a link is on its way"})
}

// accountReset spends a reset link: it sets the new password, ends every session of the account and logs the reader in.
func (s *server) accountReset(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeAccount(w, r)
	if !ok {
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" || len(req.Token) > maxTokenLen {
		writeError(w, http.StatusBadRequest, "this link is not valid")
		return
	}
	now := time.Now()
	if s.throttled(w, "reset:link", now) {
		return
	}
	// A weak password is refused before the link is spent, so the reader can try again with the same link.
	if msg := checkPasswordRules("", req.NewPassword); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	a, _, found, err := s.Store.UseEmailToken(r.Context(), hashToken(req.Token), "reset", now)
	if err != nil {
		s.Logger.Error("account reset", "err", err)
		writeError(w, http.StatusInternalServerError, "could not reset the password")
		return
	}
	if !found {
		s.accountTries.fail("reset:link", now)
		writeError(w, http.StatusNotFound, "this link is not valid, has expired or was already used")
		return
	}
	if msg := checkPasswordRules(a.Username, req.NewPassword); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	pw, err := hashPassword(req.NewPassword)
	if err != nil {
		s.Logger.Error("account reset: password", "err", err)
		writeError(w, http.StatusInternalServerError, "could not reset the password")
		return
	}
	if err := s.Store.SetAccountPassword(r.Context(), a.ID, pw); err != nil {
		s.Logger.Error("account reset", "err", err)
		writeError(w, http.StatusInternalServerError, "could not reset the password")
		return
	}
	if err := s.Store.DeleteAccountSessions(r.Context(), a.ID); err != nil {
		s.Logger.Error("account reset: sessions", "err", err)
		writeError(w, http.StatusInternalServerError, "could not reset the password")
		return
	}
	s.startSession(w, r, a, nil)
}
