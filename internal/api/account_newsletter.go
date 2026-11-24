package api

import "net/http"

// accountNewsletter tells the logged-in account where its newsletter stands, so the account area can manage it
// without a mailed link. The proof of the address is the account's CONFIRMED email (the reader opened the link mailed
// to it): the same proof the mailed manage link stands for. So the answer carries the subscriber's manage token, which
// the web page uses with the same calls the manage page makes, and only for a confirmed email. An account with no
// confirmed email gets no token.
func (s *server) accountNewsletter(w http.ResponseWriter, r *http.Request) {
	a, ok := s.accountFrom(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "log in first")
		return
	}

	out := map[string]any{"email_confirmed": false, "subscribed": false}
	if a.Email != "" && a.EmailVerified {
		out["email"] = a.Email
		out["email_confirmed"] = true
		token, found, err := s.Store.SubscriberToken(r.Context(), a.Email)
		if err != nil {
			s.Logger.Error("account newsletter", "err", err)
			writeError(w, http.StatusInternalServerError, "could not load the newsletter")
			return
		}
		if found {
			out["subscribed"] = true
			out["token"] = token
		}
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}
