package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

// Roles and what each may do. There is one role today, admin; a new role or permission is a line here and a
// requirePermission call on its route.
const (
	roleAdmin            = "admin"
	permAnalytics        = "analytics:read"
	permContent          = "content:read"
	permSubscribersRead  = "subscribers:read"
	permSubscribersWrite = "subscribers:write"

	// minAdminPassword is the shortest ADMIN_PASSWORD the API accepts; with a shorter one the admin routes are off.
	minAdminPassword = 12
	// sessionTTL is how long a signed-in session lasts.
	sessionTTL = 7 * 24 * time.Hour

	// A wrong password counts against its address, and against everyone together. Too many in the window and the
	// sign-in answers 429 until it passes.
	loginWindow         = 15 * time.Minute
	maxFailuresPerEmail = 8
	maxFailuresAll      = 60
)

var rolePermissions = map[string][]string{
	roleAdmin: {permAnalytics, permContent, permSubscribersRead, permSubscribersWrite},
}

// adminEnabled reports whether the admin routes are served: some account may be admin, and the password is long enough.
func adminEnabled(d Deps) bool {
	return len(d.AdminUsernames) > 0 && len(d.AdminPassword) >= minAdminPassword
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

// roleOf is the role an account username has, from the ADMIN_USERNAMES list; empty for everyone else. It is read on
// every request, so removing a username from the list ends its admin sessions at once.
func (s *server) roleOf(username string) string {
	for _, u := range s.AdminUsernames {
		if u == username {
			return roleAdmin
		}
	}

	return ""
}

// hashToken is what is stored in place of a session secret.
func hashToken(secret string) string {
	sum := sha256.Sum256([]byte(secret))

	return hex.EncodeToString(sum[:])
}

// sameSecret compares two secrets in constant time, whatever their lengths.
func sameSecret(a, b string) bool {
	x, y := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))

	return subtle.ConstantTimeCompare(x[:], y[:]) == 1
}

// attempts counts recent failed sign-ins per key.
// shortcut: per process and in memory, so a restart forgets them; there is one admin box, so it is enough.
type attempts struct {
	mu    sync.Mutex
	fails map[string][]time.Time
}

// recent returns how many failures key has in the window, dropping older ones.
func (a *attempts) recent(key string, now time.Time) int {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.prune(key, now)
}

func (a *attempts) prune(key string, now time.Time) int {
	kept := a.fails[key][:0]
	for _, t := range a.fails[key] {
		if now.Sub(t) < loginWindow {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(a.fails, key)
		return 0
	}
	a.fails[key] = kept

	return len(kept)
}

func (a *attempts) fail(key string, now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.fails == nil {
		a.fails = map[string][]time.Time{}
	}
	a.prune(key, now)
	a.fails[key] = append(a.fails[key], now)
}

func (a *attempts) clear(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.fails, key)
}

func bearer(r *http.Request) string {
	return strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
}

// principal finds who a request is: the account username behind its admin session and that username's role. ok is
// false for no session, an expired or unknown one, or an account that is no longer an admin. (The store keeps the
// username in the column it first kept an email in.)
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
	Password string `json:"password"`
}

type adminSessionView struct {
	Session  string `json:"session"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Expires  string `json:"expires"`
}

// adminLogin opens an admin session. The caller must already be logged in to an account (its session is the bearer
// token) whose username is on the ADMIN_USERNAMES list, and must give the admin password: logging in to an account is
// not enough, the password is a second step. A non-admin account and a wrong password get the same answer, so it does
// not say which usernames are admins. Wrong guesses are counted per account and overall, and past the limit the answer
// is 429 until the window passes.
func (s *server) adminLogin(w http.ResponseWriter, r *http.Request) {
	a, ok := s.accountFrom(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "log in to your account first")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSubscribeBody)
	var req adminLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	now := time.Now()
	key := "admin:" + a.Username
	if s.logins.recent(key, now) >= maxFailuresPerEmail || s.logins.recent("*", now) >= maxFailuresAll {
		w.Header().Set("Retry-After", strconv.Itoa(int(loginWindow.Seconds())))
		writeError(w, http.StatusTooManyRequests, "too many wrong attempts; wait 15 minutes and try again")
		return
	}

	// Both checks always run, so the time taken does not show which one failed.
	passwordOK := sameSecret(req.Password, s.AdminPassword)
	role := s.roleOf(a.Username)
	if !passwordOK || role == "" {
		if role != "" {
			s.logins.fail(key, now)
		}
		s.logins.fail("*", now)
		writeError(w, http.StatusUnauthorized, "that password is not right")
		return
	}
	s.logins.clear(key)

	secret, err := newToken()
	if err != nil {
		s.Logger.Error("admin login: token", "err", err)
		writeError(w, http.StatusInternalServerError, "could not sign in")
		return
	}
	expires := now.Add(sessionTTL)
	if err := s.Store.SaveAdminToken(r.Context(), store.AdminToken{Hash: hashToken(secret), Kind: "session", Email: a.Username, Expires: expires}); err != nil {
		s.Logger.Error("admin login", "err", err)
		writeError(w, http.StatusInternalServerError, "could not sign in")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, adminSessionView{Session: secret, Username: a.Username, Role: role, Expires: expires.UTC().Format(time.RFC3339)})
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
	writeJSON(w, http.StatusOK, map[string]any{"username": email, "role": role, "permissions": rolePermissions[role]})
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
