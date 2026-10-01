package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// loginBodyMax caps the sign-in body. The form carries one password field, and
// 4 KB leaves room for any password a person types, so a larger body comes from
// something else.
const loginBodyMax = 4 << 10

// deviceMaxAge keeps the device cookie longer than any session, because it is
// what keeps a known browser out of the public budget.
const deviceMaxAge = 400 * 24 * 3600

func (a *api) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if a.auth == nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, loginBodyMax)
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "bad form")
		return
	}
	// The attempt is charged before the password is checked, so a flood cannot
	// get more password checks than the lane allows.
	src := loginSourceOf(r, a.auth.DeviceID, a.login.ownerLoopback)
	if ok, wait := a.login.reserve(src, time.Now()); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "Too many attempts. Wait a few minutes.")
		return
	}
	if !a.auth.CheckPassword(r.PostFormValue("password")) {
		writeError(w, http.StatusUnauthorized, "Wrong password. Try again.")
		return
	}
	a.login.refund(src)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    a.auth.IssueSession(),
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   a.auth.TTLSeconds(),
	})
	a.setDeviceCookie(w, r)
	http.Redirect(w, r, "/", http.StatusFound)
}

// setDeviceCookie marks this browser as one that has signed in. The cookie
// grants nothing on its own: it only moves a later wrong password to a per-device
// budget, which a stranger cannot fill because a stranger cannot sign it. A
// browser keeps the device it already has, so its budget is not reset.
func (a *api) setDeviceCookie(w http.ResponseWriter, r *http.Request) {
	value := ""
	if ck, err := r.Cookie(deviceCookie); err == nil {
		if _, ok := a.auth.DeviceID(ck.Value); ok {
			value = ck.Value
		}
	}
	if value == "" {
		value = a.auth.IssueDevice()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     deviceCookie,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   requestIsTLS(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   deviceMaxAge,
	})
}

// requestIsTLS reports whether the browser's leg of the connection used TLS.
// The tunnel terminates TLS and forwards plain http, and it names the scheme in
// X-Forwarded-Proto. A Secure cookie on a plain connection is dropped.
func requestIsTLS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (a *api) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true,
		Secure: true, SameSite: http.SameSiteLaxMode, Expires: time.Unix(0, 0), MaxAge: -1,
	})
	http.Redirect(w, r, "/login", http.StatusFound)
}
