package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

// Roles and what each may do. There is one role today, admin, and one permission; a new role or permission is
// a line here and a requirePermission call on its route.
const (
	roleAdmin     = "admin"
	permAnalytics = "analytics:read"

	// adminGap is the least time between two sign-in mails to one address.
	adminGap = time.Minute
	// loginTTL is how long a sign-in link works; sessionTTL how long a signed-in session lasts.
	loginTTL   = 15 * time.Minute
	sessionTTL = 7 * 24 * time.Hour
)

var rolePermissions = map[string][]string{
	roleAdmin: {permAnalytics},
}

// can reports whether the role may do the thing.
func can(role, permission string) bool {
	for _, p := range rolePermissions[role] {
		if p == permission {
			return true
		}
	}

	return false
}

// roleOf is the role an address has, from the ADMIN_EMAILS list; empty for everyone else. It is read on
// every request, so removing an address from the list ends its sessions at once.
func (s *server) roleOf(email string) string {
	for _, e := range s.AdminEmails {
		if e == email {
			return roleAdmin
		}
	}

	return ""
}

// hashToken is what is stored in place of a sign-in or session secret.
func hashToken(secret string) string {
	sum := sha256.Sum256([]byte(secret))

	return hex.EncodeToString(sum[:])
}

func bearer(r *http.Request) string {
	return strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
}

// principal finds who a request is: the address behind its session and that address's role. ok is false
// for no session, an expired or unknown one, or an address that is no longer an admin.
func (s *server) principal(r *http.Request) (email, role string, ok bool) {
	secret := bearer(r)
	if secret == "" || len(secret) > maxTokenLen {
		return "", "", false
	}
	email, found, err := s.Store.UseAdminToken(r.Context(), hashToken(secret), "session", false, time.Now())
	if err != nil {
		s.Logger.Error("admin session", "err", err)
		return "", "", false
	}
	if role = s.roleOf(email); !found || role == "" {
		return "", "", false
	}

	return email, role, true
}

// requirePermission answers 401 for a caller who is not signed in and 403 for one whose role lacks the
// permission; it reports whether the handler may go on.
func (s *server) requirePermission(w http.ResponseWriter, r *http.Request, permission string) (email string, ok bool) {
	email, role, signedIn := s.principal(r)
	if !signedIn {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "sign in as an admin first")
		return "", false
	}
	if !can(role, permission) {
		writeError(w, http.StatusForbidden, "your role may not do this")
		return "", false
	}

	return email, true
}

type adminLoginRequest struct {
	Email string `json:"email"`
}

// adminLogin mails a one-time sign-in link to an admin address. The answer is the same whether or not the
// address is an admin, so the endpoint cannot be used to find out who is.
func (s *server) adminLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSubscribeBody)

	var req adminLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	email, ok := parseEmail(req.Email)
	if !ok {
		writeError(w, http.StatusBadRequest, "enter a valid email address")
		return
	}

	now := time.Now()
	// Only an admin address is ever mailed, and each at most once a minute, so this cannot be used to send mail to
	// anyone else or to flood an admin.
	if s.roleOf(email) != "" && s.adminMails.ready(email, now) {
		secret, err := newToken()
		if err != nil {
			s.Logger.Error("admin login: token", "err", err)
			writeError(w, http.StatusInternalServerError, "could not sign in")
			return
		}
		if err := s.Store.SaveAdminToken(r.Context(), store.AdminToken{Hash: hashToken(secret), Kind: "login", Email: email, Expires: now.Add(loginTTL)}); err != nil {
			s.Logger.Error("admin login", "err", err)
			writeError(w, http.StatusInternalServerError, "could not sign in")
			return
		}
		if s.AdminNotify != nil {
			s.AdminNotify(r.Context(), email, secret)
		}
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "if this address may sign in, a link is on its way"})
}

type adminSessionRequest struct {
	Token string `json:"token"`
}

type adminSessionView struct {
	Session string `json:"session"`
	Email   string `json:"email"`
	Role    string `json:"role"`
	Expires string `json:"expires"`
}

// adminSession trades a sign-in link's token for a session, once. It is a POST that the web page makes when
// the admin presses a button, so a mail scanner that opens the link cannot use it up.
func (s *server) adminSession(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSubscribeBody)

	var req adminSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" || len(req.Token) > maxTokenLen {
		writeError(w, http.StatusBadRequest, "invalid token")
		return
	}
	now := time.Now()
	email, found, err := s.Store.UseAdminToken(r.Context(), hashToken(req.Token), "login", true, now)
	if err != nil {
		s.Logger.Error("admin session", "err", err)
		writeError(w, http.StatusInternalServerError, "could not sign in")
		return
	}
	role := s.roleOf(email)
	if !found || role == "" {
		writeError(w, http.StatusUnauthorized, "this sign-in link is not valid or has expired")
		return
	}

	secret, err := newToken()
	if err != nil {
		s.Logger.Error("admin session: token", "err", err)
		writeError(w, http.StatusInternalServerError, "could not sign in")
		return
	}
	expires := now.Add(sessionTTL)
	if err := s.Store.SaveAdminToken(r.Context(), store.AdminToken{Hash: hashToken(secret), Kind: "session", Email: email, Expires: expires}); err != nil {
		s.Logger.Error("admin session", "err", err)
		writeError(w, http.StatusInternalServerError, "could not sign in")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, adminSessionView{Session: secret, Email: email, Role: role, Expires: expires.UTC().Format(time.RFC3339)})
}

// adminMe says who the session belongs to and what they may do.
func (s *server) adminMe(w http.ResponseWriter, r *http.Request) {
	email, role, ok := s.principal(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "sign in as an admin first")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"email": email, "role": role, "permissions": rolePermissions[role]})
}

// adminLogout ends the session; signing out twice, or with an unknown session, is not an error.
func (s *server) adminLogout(w http.ResponseWriter, r *http.Request) {
	if secret := bearer(r); secret != "" && len(secret) <= maxTokenLen {
		if err := s.Store.DeleteAdminToken(r.Context(), hashToken(secret)); err != nil {
			s.Logger.Error("admin logout", "err", err)
			writeError(w, http.StatusInternalServerError, "could not sign out")
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
