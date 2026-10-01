package httpapi

import (
	"net/http"
	"strings"
)

// uiLoginPath is where a browser without a session is sent. GET /login has no
// route; the sign-in page belongs to the SPA.
const uiLoginPath = "/ui/login"

// wantsJSON reports whether the client asked for JSON. A browser navigation
// sends text/html and "*/*", and curl sends "*/*", so only an explicit
// application/json counts: those callers keep the redirect behavior.
func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

// redirectOrOK ends a form action: a JSON client gets 200 {"ok":true}, anyone
// else is redirected to where.
func redirectOrOK(w http.ResponseWriter, r *http.Request, where string) {
	if wantsJSON(r) {
		writeJSON(w, map[string]bool{"ok": true})
		return
	}
	http.Redirect(w, r, where, http.StatusFound)
}

// hasSession reports whether the request carries a live dashboard session.
func (a *api) hasSession(r *http.Request) bool {
	ck, err := r.Cookie(sessionCookie)
	return err == nil && a.auth != nil && a.auth.ValidSession(ck.Value)
}

// sessionState tells the SPA whether to show the sign-in page. It needs no
// login and carries no secret: three booleans. A bearer token is not a
// session, so only the cookie counts.
func (a *api) sessionState(w http.ResponseWriter, r *http.Request) {
	if a.auth == nil {
		writeJSON(w, map[string]bool{"authRequired": false, "authenticated": true, "admin": true})
		return
	}
	ok := a.hasSession(r)
	writeJSON(w, map[string]bool{"authRequired": true, "authenticated": ok, "admin": ok})
}
