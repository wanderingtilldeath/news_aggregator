package api

import (
	"encoding/json"
	"math"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

// Reader accounts, with no email: a username and password, and a recovery code shown once at signup. An account
// is what holds a Pro membership (POST /account/pro), so a Pro member logs in on another device instead of pasting
// their Pro link. The session is a bearer token, kept by the web app in an httpOnly cookie.
const (
	accountSessionTTL = 30 * 24 * time.Hour
	maxAccountBody    = 4 << 10

	// A wrong password or recovery code counts against its username, and against everyone together; past the per-name
	// limit the answer is 429 until the window passes (loginWindow, see admin.go); past the overall one every try is slowed.
	maxAccountFailures    = 8
	maxAccountFailuresAll = 120
)

// globalSlowdown is how long every login, recovery and reset waits while the whole site has had too many wrong tries.
// shortcut: this holds a handler goroutine per request (PBKDF2 already makes each one costly), and the counters live in
// memory per instance; move to a shared limiter if the API ever runs on several instances.
var globalSlowdown = 2 * time.Second

var usernamePattern = regexp.MustCompile(`^[a-z0-9_]{3,24}$`)

// cleanEmail lowercases and checks an address; ok is false when it is not a plain address. An empty one is valid and
// means "none".
func cleanEmail(raw string) (string, bool) {
	e := strings.ToLower(strings.TrimSpace(raw))
	if e == "" {
		return "", true
	}
	a, err := mail.ParseAddress(e)

	return e, err == nil && a.Address == e && len(e) <= 254
}

// reservedUsernames cannot be registered, so nobody can pose as the site.
var reservedUsernames = map[string]bool{"admin": true, "administrator": true, "postpaper": true, "support": true, "root": true, "staff": true, "help": true}

type accountRequest struct {
	Username     string `json:"username"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	RecoveryCode string `json:"recovery_code"`
	NewPassword  string `json:"new_password"`
	Token        string `json:"token"`
	// PaymentID is the Razorpay payment id from a receipt, the other way to prove a Pro membership.
	PaymentID string `json:"payment_id"`
	// CurrentPassword proves who is asking for a change that only the owner should make.
	CurrentPassword string `json:"current_password"`
}

type accountSessionView struct {
	Session  string `json:"session"`
	Username string `json:"username"`
	Expires  string `json:"expires"`
	// RecoveryCodes are shown only when they were just made (signup, and "make new codes"); they cannot be read
	// again. CodesLeft is how many unspent ones the account has.
	RecoveryCodes []string `json:"recovery_codes,omitempty"`
	CodesLeft     int      `json:"codes_left"`
}

// decodeAccount reads a small JSON body.
func decodeAccount(w http.ResponseWriter, r *http.Request) (accountRequest, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAccountBody)
	var req accountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return req, false
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))

	return req, true
}

// checkPasswordRules refuses a password that is too short or too long, or is the username.
func checkPasswordRules(username, password string) string {
	switch {
	case len(password) < minPasswordLen:
		return "the password needs at least 10 characters"
	case len(password) > maxPasswordLen:
		return "the password is too long"
	case strings.EqualFold(password, username):
		return "the password cannot be your username"
	}

	return ""
}

// startSession stores a new session for the account and returns what the web app keeps. codes are the recovery codes
// that were just made, if any.
func (s *server) startSession(w http.ResponseWriter, r *http.Request, a store.Account, codes []string) {
	secret, err := newToken()
	if err != nil {
		s.Logger.Error("account session: token", "err", err)
		writeError(w, http.StatusInternalServerError, "could not sign in")
		return
	}
	expires := time.Now().Add(accountSessionTTL)
	if err := s.Store.SaveAccountSession(r.Context(), hashToken(secret), a.ID, expires); err != nil {
		s.Logger.Error("account session", "err", err)
		writeError(w, http.StatusInternalServerError, "could not sign in")
		return
	}
	left, _ := s.Store.RecoveryCodesLeft(r.Context(), a.ID)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, accountSessionView{Session: secret, Username: a.Username, Expires: expires.UTC().Format(time.RFC3339), RecoveryCodes: codes, CodesLeft: left})
}

// accountSignup makes an account. A taken username is 409; all callers together are held to the sign-up limit, so
// the table cannot be filled by a script.
func (s *server) accountSignup(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeAccount(w, r)
	if !ok {
		return
	}
	if !usernamePattern.MatchString(req.Username) {
		writeError(w, http.StatusBadRequest, "a username is 3 to 24 letters, digits or underscores")
		return
	}
	if reservedUsernames[req.Username] {
		writeError(w, http.StatusConflict, "that username is taken")
		return
	}
	email, ok := cleanEmail(req.Email)
	if !ok {
		writeError(w, http.StatusBadRequest, "that email address is not valid")
		return
	}
	if email != "" {
		if _, used, err := s.Store.AccountByEmail(r.Context(), email); err != nil || used {
			if err != nil {
				s.Logger.Error("account signup: email", "err", err)
				writeError(w, http.StatusInternalServerError, "could not create the account")
				return
			}
			writeError(w, http.StatusConflict, "that email is already used by an account")
			return
		}
	}
	if msg := checkPasswordRules(req.Username, req.Password); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if !s.signups.allow(time.Now()) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "too many sign-ups right now, try again in a minute")
		return
	}

	pw, err := hashPassword(req.Password)
	codes, hashes, err2 := newRecoverySet()
	if err != nil || err2 != nil {
		s.Logger.Error("account signup: secrets", "err", err, "err2", err2)
		writeError(w, http.StatusInternalServerError, "could not create the account")
		return
	}
	a, created, err := s.Store.CreateAccount(r.Context(), req.Username, pw, hashes)
	if err != nil {
		s.Logger.Error("account signup", "err", err)
		writeError(w, http.StatusInternalServerError, "could not create the account")
		return
	}
	if !created {
		writeError(w, http.StatusConflict, "that username is taken")
		return
	}
	// Taken in the instant since the check above: the account is made without it and it can be added on the account page.
	if email != "" {
		if set, err := s.Store.SetAccountEmail(r.Context(), a.ID, email); err == nil && set {
			a.Email = email
			s.mailAccountLink(r, a.ID, "verify", email)
		}
	}
	s.startSession(w, r, a, codes)
}

// throttled reports whether a username or everyone has had too many wrong tries lately (see admin.go's attempts).
func (s *server) throttled(w http.ResponseWriter, key string, now time.Time) bool {
	if s.accountTries.recent(key, now) >= maxAccountFailures {
		w.Header().Set("Retry-After", "900")
		writeError(w, http.StatusTooManyRequests, "too many wrong attempts; wait 15 minutes and try again")
		return true
	}
	// Many wrong tries across the site (a guessing run against many names) slow everyone down; they no longer lock
	// everyone out, which would let a script block every real login.
	if s.accountTries.recent("*", now) >= maxAccountFailuresAll {
		time.Sleep(globalSlowdown)
	}

	return false
}

func (s *server) accountLogin(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeAccount(w, r)
	if !ok {
		return
	}
	now := time.Now()
	// The name field takes a username or, with an @ in it, the account's email.
	who := strings.ToLower(strings.TrimSpace(req.Username))
	if s.throttled(w, "login:"+who, now) {
		return
	}
	find := s.Store.AccountByUsername
	if strings.Contains(who, "@") {
		find = s.Store.AccountByEmail
	}
	a, found, err := find(r.Context(), who)
	if err != nil {
		s.Logger.Error("account login", "err", err)
		writeError(w, http.StatusInternalServerError, "could not sign in")
		return
	}
	// The same work is done whether or not the username exists, and the answer is the same.
	valid := false
	if found && a.PasswordHash != "" {
		valid = checkPassword(req.Password, a.PasswordHash)
	} else {
		// No such account, or one that signs in with Google alone: the same work, the same answer.
		spendTheTime(req.Password)
	}
	if !valid {
		if found {
			s.accountTries.fail("login:"+who, now)
		}
		s.accountTries.fail("*", now)
		writeError(w, http.StatusUnauthorized, "username, email or password is not right")
		return
	}
	s.accountTries.clear("login:" + who)
	s.startSession(w, r, a, nil)
}

// accountRecover sets a new password from one of the account's recovery codes, which is spent, and ends the account's
// sessions. The other codes stay valid, so a printed sheet keeps working. It answers like login: one message for a
// wrong username or a wrong code.
func (s *server) accountRecover(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeAccount(w, r)
	if !ok {
		return
	}
	now := time.Now()
	if s.throttled(w, "recover:"+req.Username, now) {
		return
	}
	if msg := checkPasswordRules(req.Username, req.NewPassword); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	a, found, err := s.Store.AccountByUsername(r.Context(), req.Username)
	if err != nil {
		s.Logger.Error("account recover", "err", err)
		writeError(w, http.StatusInternalServerError, "could not reset the password")
		return
	}
	good := false
	if found {
		good, err = s.Store.UseRecoveryCode(r.Context(), a.ID, hashRecoveryCode(req.RecoveryCode))
		if err != nil {
			s.Logger.Error("account recover", "err", err)
			writeError(w, http.StatusInternalServerError, "could not reset the password")
			return
		}
	}
	if !found || !good {
		if found {
			s.accountTries.fail("recover:"+req.Username, now)
		}
		s.accountTries.fail("*", now)
		writeError(w, http.StatusUnauthorized, "username or recovery code is not right")
		return
	}

	pw, err := hashPassword(req.NewPassword)
	if err != nil {
		s.Logger.Error("account recover: password", "err", err)
		writeError(w, http.StatusInternalServerError, "could not reset the password")
		return
	}
	if err := s.Store.SetAccountPassword(r.Context(), a.ID, pw); err != nil {
		s.Logger.Error("account recover", "err", err)
		writeError(w, http.StatusInternalServerError, "could not reset the password")
		return
	}
	// Whoever held the account before the reset is logged out everywhere.
	if err := s.Store.DeleteAccountSessions(r.Context(), a.ID); err != nil {
		s.Logger.Error("account recover: sessions", "err", err)
		writeError(w, http.StatusInternalServerError, "could not reset the password")
		return
	}
	s.accountTries.clear("recover:" + req.Username)
	s.startSession(w, r, a, nil)
}

// accountFrom finds the account behind the request's bearer session.
func (s *server) accountFrom(r *http.Request) (store.Account, bool) {
	secret := bearer(r)
	if secret == "" || len(secret) > maxTokenLen {
		return store.Account{}, false
	}
	a, ok, err := s.Store.AccountBySession(r.Context(), hashToken(secret), time.Now())
	if err != nil {
		s.Logger.Error("account session", "err", err)
		return store.Account{}, false
	}

	return a, ok
}

func (s *server) accountLogout(w http.ResponseWriter, r *http.Request) {
	if secret := bearer(r); secret != "" && len(secret) <= maxTokenLen {
		if err := s.Store.DeleteAccountSession(r.Context(), hashToken(secret)); err != nil {
			s.Logger.Error("account logout", "err", err)
			writeError(w, http.StatusInternalServerError, "could not log out")
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

type proState struct {
	Linked bool   `json:"linked"`
	Active bool   `json:"active"`
	Until  string `json:"until,omitempty"`
	// DaysLeft counts whole days until the period ends, rounded up (0 once it has ended). Email is the address the
	// membership was paid with: the daily email is ad-free only when subscribed with that address. Edition is the
	// member's own printable-edition path, with their secret token: it goes only to the logged-in account that holds it.
	DaysLeft int    `json:"days_left"`
	Email    string `json:"email,omitempty"`
	Edition  string `json:"edition,omitempty"`
	// Payments are the member's payments on record, newest first (the payment id is on the Razorpay receipt).
	Payments []proPaymentView `json:"payments,omitempty"`
}

type proPaymentView struct {
	ID   string `json:"id"`
	Date string `json:"date"`
}

// maxPaymentsShown bounds the payment history an account sees.
const maxPaymentsShown = 20

type accountView struct {
	Username string `json:"username"`
	Email    string `json:"email,omitempty"`
	// EmailVerified is true once the reader opened the link mailed to Email.
	EmailVerified bool     `json:"email_verified"`
	Pro           proState `json:"pro"`
	// HasPassword is false for an account that signs in with Google alone. CodesLeft is its unspent recovery
	// codes, Identities the providers linked to it, and Roles what it is: always "account", plus "pro" while a
	// membership is paid up and "admin" when its username is on the admin list.
	HasPassword bool     `json:"has_password"`
	CodesLeft   int      `json:"codes_left"`
	Identities  []string `json:"identities"`
	Roles       []string `json:"roles"`
}

// viewOf describes an account for its owner.
func (s *server) viewOf(r *http.Request, a store.Account) (accountView, error) {
	pro, err := s.proOf(r, a)
	if err != nil {
		return accountView{}, err
	}
	left, err := s.Store.RecoveryCodesLeft(r.Context(), a.ID)
	if err != nil {
		return accountView{}, err
	}
	ids, err := s.Store.Identities(r.Context(), a.ID)
	if err != nil {
		return accountView{}, err
	}
	roles := []string{"account"}
	if pro.Active {
		roles = append(roles, "pro")
	}
	// "admin" means the account may take the admin step-up; it opens nothing by itself.
	if s.roleOf(a.Username) == roleAdmin {
		roles = append(roles, "admin")
	}

	return accountView{Username: a.Username, Email: a.Email, EmailVerified: a.EmailVerified, Pro: pro, HasPassword: a.PasswordHash != "", CodesLeft: left, Identities: ids, Roles: roles}, nil
}

// proOf says whether the account has a Pro membership and whether it is still paid up.
func (s *server) proOf(r *http.Request, a store.Account) (proState, error) {
	if a.ProEmail == "" {
		return proState{}, nil
	}
	m, ok, err := s.Store.ProByEmail(r.Context(), a.ProEmail)
	if err != nil || !ok {
		return proState{}, err
	}

	st := proState{Linked: true, Active: m.Until.After(time.Now()), Until: m.Until.UTC().Format("2006-01-02"), Email: m.Email}
	payments, err := s.Store.ProPayments(r.Context(), m.Email, maxPaymentsShown)
	if err != nil {
		return proState{}, err
	}
	for _, p := range payments {
		st.Payments = append(st.Payments, proPaymentView{ID: p.ID, Date: p.At.UTC().Format("2006-01-02")})
	}
	if st.Active {
		st.DaysLeft = int(math.Ceil(time.Until(m.Until).Hours() / 24))
		st.Edition = "/pro/edition?token=" + url.QueryEscape(m.Token)
	}

	return st, nil
}

func (s *server) accountMe(w http.ResponseWriter, r *http.Request) {
	a, ok := s.accountFrom(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "log in first")
		return
	}
	view, err := s.viewOf(r, a)
	if err != nil {
		s.Logger.Error("account me", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load the account")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, view)
}

// accountLinkPro ties a Pro membership to the account, on proof that the caller holds it: the Pro token (the secret
// in the member's emailed links) or the Razorpay payment id that bought it (printed on the receipt, so a member whose
// Pro mail never arrived can still claim it). A membership already held by another account is 409. Wrong guesses are
// counted per account and overall, like wrong passwords.
func (s *server) accountLinkPro(w http.ResponseWriter, r *http.Request) {
	a, ok := s.accountFrom(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "log in first")
		return
	}
	req, ok := decodeAccount(w, r)
	if !ok {
		return
	}
	req.Token, req.PaymentID = strings.TrimSpace(req.Token), strings.TrimSpace(req.PaymentID)
	if (req.Token == "") == (req.PaymentID == "") || len(req.Token) > maxTokenLen || len(req.PaymentID) > 64 {
		writeError(w, http.StatusBadRequest, "paste your Pro link, or the payment id from your receipt")
		return
	}
	key := "pro:" + strconv.FormatInt(a.ID, 10)
	now := time.Now()
	if s.throttled(w, key, now) {
		return
	}

	var m store.ProMember
	var found bool
	var err error
	if req.Token != "" {
		m, found, err = s.Store.ProByToken(r.Context(), req.Token)
	} else {
		m, found, err = s.Store.ProByPayment(r.Context(), req.PaymentID)
	}
	if err != nil {
		s.Logger.Error("account link pro", "err", err)
		writeError(w, http.StatusInternalServerError, "could not check the membership")
		return
	}
	if !found {
		s.accountTries.fail(key, now)
		s.accountTries.fail("*", now)
		writeError(w, http.StatusNotFound, "that Pro link or payment id is not valid")
		return
	}
	linked, err := s.Store.LinkProMembership(r.Context(), a.ID, m.Email)
	if err != nil {
		s.Logger.Error("account link pro", "err", err)
		writeError(w, http.StatusInternalServerError, "could not link the membership")
		return
	}
	if !linked {
		writeError(w, http.StatusConflict, "that membership is already linked to another account")
		return
	}
	s.accountTries.clear(key)
	a.ProEmail = m.Email
	view, err := s.viewOf(r, a)
	if err != nil {
		s.Logger.Error("account link pro", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load the account")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, view)
}
