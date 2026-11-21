package api

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wanderingtilldeath/news-aggregator/internal/store"
)

// Sign in with Google. The web app sends the reader to Google and gets back a one-time code; this endpoint trades the
// code for an ID token at Google's token endpoint, over TLS and with the client secret, and reads who Google says the
// reader is. Because the token comes straight from Google that way, OpenID Connect lets the receiver skip checking its
// signature; the issuer, audience and expiry are still checked. Only Google's own subject id ("sub") is kept, never an
// email, and nothing is mailed. The same endpoint logs in, makes an account for a Google identity not seen before, or,
// when the caller already has an account session, links the identity to that account (which is what makes Google a way
// back in for an account whose password and recovery codes are lost).
const (
	googleProvider = "google"
	googleTokenURL = "https://oauth2.googleapis.com/token"
)

type googleRequest struct {
	Code         string `json:"code"`
	RedirectURI  string `json:"redirect_uri"`
	CodeVerifier string `json:"code_verifier"`
}

// googleIdentity exchanges the code and returns the verified subject, or an error message for the reader.
func (s *server) googleIdentity(r *http.Request, req googleRequest) (string, string) {
	endpoint := s.GoogleTokenURL
	if endpoint == "" {
		endpoint = googleTokenURL
	}
	client := s.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	form := url.Values{
		"code":          {req.Code},
		"client_id":     {s.GoogleClientID},
		"client_secret": {s.GoogleClientSecret},
		"redirect_uri":  {req.RedirectURI},
		"grant_type":    {"authorization_code"},
		"code_verifier": {req.CodeVerifier},
	}
	hreq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "Google sign-in did not work; please try again"
	}
	hreq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := client.Do(hreq)
	if err != nil {
		s.Logger.Error("google token", "err", err)
		return "", "Google could not be reached; please try again"
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	var tok struct {
		IDToken string `json:"id_token"`
	}
	if res.StatusCode != http.StatusOK || json.Unmarshal(body, &tok) != nil || tok.IDToken == "" {
		return "", "Google sign-in did not work; please try again"
	}

	parts := strings.Split(tok.IDToken, ".")
	if len(parts) != 3 {
		return "", "Google sign-in did not work; please try again"
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "Google sign-in did not work; please try again"
	}
	var claims struct {
		Iss string      `json:"iss"`
		Aud string      `json:"aud"`
		Sub string      `json:"sub"`
		Exp json.Number `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return "", "Google sign-in did not work; please try again"
	}
	exp, _ := claims.Exp.Int64()
	if (claims.Iss != "https://accounts.google.com" && claims.Iss != "accounts.google.com") || claims.Aud != s.GoogleClientID || claims.Sub == "" || len(claims.Sub) > 64 || exp <= time.Now().Unix() {
		return "", "Google sign-in did not work; please try again"
	}

	return claims.Sub, ""
}

// newGeneratedUsername is "reader_" and six letters or digits, for an account made from a Google sign-in.
func newGeneratedUsername() (string, error) {
	raw := make([]byte, 4)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}

	return "reader_" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw))[:6], nil
}

// accountGoogle signs in with Google, or links Google to the signed-in account.
func (s *server) accountGoogle(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAccountBody)
	var req googleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" || len(req.Code) > 2048 || len(req.RedirectURI) > 512 || len(req.CodeVerifier) > 256 {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	sub, msg := s.googleIdentity(r, req)
	if msg != "" {
		writeError(w, http.StatusUnauthorized, msg)
		return
	}

	// Signed in already: link this Google identity to the account.
	if secret := bearer(r); secret != "" {
		a, ok := s.accountFrom(r)
		if !ok {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, "log in first")
			return
		}
		linked, err := s.Store.LinkIdentity(r.Context(), a.ID, googleProvider, sub)
		if err != nil {
			s.Logger.Error("account google link", "err", err)
			writeError(w, http.StatusInternalServerError, "could not link Google")
			return
		}
		if !linked {
			writeError(w, http.StatusConflict, "that Google account is already linked to another Postpaper account")
			return
		}
		view, err := s.viewOf(r, a)
		if err != nil {
			s.Logger.Error("account google link", "err", err)
			writeError(w, http.StatusInternalServerError, "could not load the account")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, view)
		return
	}

	a, found, err := s.Store.AccountByIdentity(r.Context(), googleProvider, sub)
	if err != nil {
		s.Logger.Error("account google", "err", err)
		writeError(w, http.StatusInternalServerError, "could not sign in")
		return
	}
	if found {
		s.startSession(w, r, a, nil)
		return
	}
	s.createGoogleAccount(w, r, sub)
}

// createGoogleAccount makes an account for a Google identity that has none: a generated username, no password, and a
// set of recovery codes shown once. Sign-ups are held to the same limit as password sign-ups.
func (s *server) createGoogleAccount(w http.ResponseWriter, r *http.Request, sub string) {
	if !s.signups.allow(time.Now()) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "too many sign-ups right now, try again in a minute")
		return
	}
	codes, hashes, err := newRecoverySet()
	if err != nil {
		s.Logger.Error("account google: codes", "err", err)
		writeError(w, http.StatusInternalServerError, "could not create the account")
		return
	}
	var a store.Account
	created := false
	for try := 0; try < 5 && !created; try++ {
		name, err := newGeneratedUsername()
		if err != nil {
			s.Logger.Error("account google: username", "err", err)
			writeError(w, http.StatusInternalServerError, "could not create the account")
			return
		}
		if a, created, err = s.Store.CreateAccount(r.Context(), name, "", hashes); err != nil {
			s.Logger.Error("account google", "err", err)
			writeError(w, http.StatusInternalServerError, "could not create the account")
			return
		}
	}
	if !created {
		writeError(w, http.StatusInternalServerError, "could not create the account")
		return
	}
	linked, err := s.Store.LinkIdentity(r.Context(), a.ID, googleProvider, sub)
	if err != nil || !linked {
		// Another request made an account for the same Google identity first: drop this one and use that.
		_ = s.Store.DeleteAccount(r.Context(), a.ID)
		if first, ok, _ := s.Store.AccountByIdentity(r.Context(), googleProvider, sub); ok {
			s.startSession(w, r, first, nil)
			return
		}
		s.Logger.Error("account google link", "err", err)
		writeError(w, http.StatusInternalServerError, "could not create the account")
		return
	}
	s.startSession(w, r, a, codes)
}
