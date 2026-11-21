package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

// recoveryCodeCount is how many recovery codes an account gets at once.
const recoveryCodeCount = 10

// newRecoverySet makes a set of recovery codes: the codes to show the reader once, and their hashes to keep.
func newRecoverySet() (codes, hashes []string, err error) {
	for len(codes) < recoveryCodeCount {
		code, err := newRecoveryCode()
		if err != nil {
			return nil, nil, err
		}
		codes, hashes = append(codes, code), append(hashes, hashRecoveryCode(code))
	}

	return codes, hashes, nil
}

// confirmOwner checks the password of the account for an action only its owner should take (a new password, new
// recovery codes, deleting it). An account that signs in with Google alone has no password, and its session is the
// proof. A wrong password counts against the account and overall; it reports whether to go on, having answered if not.
func (s *server) confirmOwner(w http.ResponseWriter, a store.Account, password string, now time.Time) bool {
	if a.PasswordHash == "" {
		return true
	}
	key := "owner:" + strconv.FormatInt(a.ID, 10)
	if s.throttled(w, key, now) {
		return false
	}
	if checkPassword(password, a.PasswordHash) {
		s.accountTries.clear(key)
		return true
	}
	s.accountTries.fail(key, now)
	s.accountTries.fail("*", now)
	writeError(w, http.StatusUnauthorized, "that password is not right")

	return false
}

// ownerRequest finds the account behind the session and reads the body, answering if either is missing.
func (s *server) ownerRequest(w http.ResponseWriter, r *http.Request) (store.Account, accountRequest, bool) {
	a, ok := s.accountFrom(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "log in first")
		return a, accountRequest{}, false
	}
	req, ok := decodeAccount(w, r)

	return a, req, ok
}

// accountPassword changes the password (or sets the first one, for an account that signs in with Google alone). It
// needs the current password, ends every session and starts a new one, so a stolen session does not outlive the change.
func (s *server) accountPassword(w http.ResponseWriter, r *http.Request) {
	a, req, ok := s.ownerRequest(w, r)
	if !ok {
		return
	}
	if msg := checkPasswordRules(a.Username, req.NewPassword); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if !s.confirmOwner(w, a, req.CurrentPassword, time.Now()) {
		return
	}
	pw, err := hashPassword(req.NewPassword)
	if err != nil {
		s.Logger.Error("account password", "err", err)
		writeError(w, http.StatusInternalServerError, "could not change the password")
		return
	}
	if err := s.Store.SetAccountPassword(r.Context(), a.ID, pw); err != nil {
		s.Logger.Error("account password", "err", err)
		writeError(w, http.StatusInternalServerError, "could not change the password")
		return
	}
	if err := s.Store.DeleteAccountSessions(r.Context(), a.ID); err != nil {
		s.Logger.Error("account password: sessions", "err", err)
		writeError(w, http.StatusInternalServerError, "could not change the password")
		return
	}
	s.startSession(w, r, a, nil)
}

type codesView struct {
	RecoveryCodes []string `json:"recovery_codes"`
	CodesLeft     int      `json:"codes_left"`
}

// accountNewCodes replaces every recovery code with a new set of ten, shown once; the old ones stop working.
func (s *server) accountNewCodes(w http.ResponseWriter, r *http.Request) {
	a, req, ok := s.ownerRequest(w, r)
	if !ok {
		return
	}
	if !s.confirmOwner(w, a, req.CurrentPassword, time.Now()) {
		return
	}
	codes, hashes, err := newRecoverySet()
	if err != nil {
		s.Logger.Error("account codes", "err", err)
		writeError(w, http.StatusInternalServerError, "could not make new codes")
		return
	}
	if err := s.Store.ReplaceRecoveryCodes(r.Context(), a.ID, hashes); err != nil {
		s.Logger.Error("account codes", "err", err)
		writeError(w, http.StatusInternalServerError, "could not make new codes")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, codesView{RecoveryCodes: codes, CodesLeft: len(codes)})
}

// accountLogoutAll ends every session of the account, this one included.
func (s *server) accountLogoutAll(w http.ResponseWriter, r *http.Request) {
	a, ok := s.accountFrom(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "log in first")
		return
	}
	if err := s.Store.DeleteAccountSessions(r.Context(), a.ID); err != nil {
		s.Logger.Error("account logout all", "err", err)
		writeError(w, http.StatusInternalServerError, "could not log out")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// accountDelete removes the account with its sessions, recovery codes and sign-ins. A Pro membership stays with the
// payer and can be linked to another account.
func (s *server) accountDelete(w http.ResponseWriter, r *http.Request) {
	a, req, ok := s.ownerRequest(w, r)
	if !ok {
		return
	}
	if !s.confirmOwner(w, a, req.CurrentPassword, time.Now()) {
		return
	}
	if err := s.Store.DeleteAccount(r.Context(), a.ID); err != nil {
		s.Logger.Error("account delete", "err", err)
		writeError(w, http.StatusInternalServerError, "could not delete the account")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
