package httpapi

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/naniiluja/ccw/internal/auth"
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
	src := auth.LoginSourceOf(r, a.auth.DeviceID, a.login.OwnerLoopback())
	if ok, wait := a.login.Reserve(src, time.Now()); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "Too many attempts. Wait a few minutes.")
		return
	}
	if !a.auth.CheckPassword(r.PostFormValue("password")) {
		writeError(w, http.StatusUnauthorized, "Wrong password. Try again.")
		return
	}
	a.login.Refund(src)
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
	if ck, err := r.Cookie(auth.DeviceCookie); err == nil {
		if _, ok := a.auth.DeviceID(ck.Value); ok {
			value = ck.Value
		}
	}
	if value == "" {
		value = a.auth.IssueDevice()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.DeviceCookie,
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

// guardRequest wraps the whole mux. It answers the two attacks that a bind
// address cannot stop: DNS rebinding against an ungated server, and a
// cross-site write from a page the operator has open. It also refuses framing.
func (a *api) guardRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		// With no gate, the Host is the only thing that tells the operator's own
		// browser apart from a name that resolves to 127.0.0.1.
		if a.auth == nil && !loopbackHost(r.Host) {
			writeError(w, http.StatusForbidden, "this server has no authentication, so it answers a loopback Host only")
			return
		}
		if !crossSiteWrite(r) {
			next.ServeHTTP(w, r)
			return
		}
		writeError(w, http.StatusForbidden, "cross-site request refused")
	})
}

// crossSiteWrite reports whether a request changes state and comes from another
// site. A request with no Origin header is not cross-site: an API client sends
// none, and every browser sends one on a cross-origin write.
func crossSiteWrite(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	// A sandboxed frame or a redirected form sends "null", which names no site.
	if origin == "null" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return true
	}
	// Host only, never scheme: a TLS-terminating tunnel gives the browser https
	// and the server plain http, and both carry the same host and port.
	return u.Host != r.Host
}

// loopbackHost reports whether the request's Host names this machine.
func loopbackHost(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

// principal is who sent a request. admin is set only by the master token, a
// signed-in session, and no-auth mode; no key name can set it.
type principal struct {
	admin bool
	keyID string // dashboard key id; empty for admin callers and internal jobs
	name  string // display label for drift and logs; never used for access
	// models lists the "<provider>/<model>" ids a dashboard key may call;
	// empty allows every model.
	models []string
}

// allowsAllModels reports whether p may call any model: admin callers, ccw's
// own jobs, and keys with no model list.
func (p principal) allowsAllModels() bool {
	return p.keyID == "" || len(p.models) == 0 || slices.Contains(p.models, "*")
}

// allowsModel reports whether p may call model, as the caller named it.
func (p principal) allowsModel(prov, upstreamModel, model string) bool {
	if prov != "" {
		return p.allowsProviderModel(prov, upstreamModel)
	}
	return p.allowsBareModel(model)
}

// allowsProviderModel matches the id exactly, case included, as the upstream
// sees it: "-high" on an allowed id is another model.
func (p principal) allowsProviderModel(prov, model string) bool {
	return p.allowsAllModels() || slices.Contains(p.models, prov+"/"+model)
}

// allowsBareModel reports whether some allowed "<provider>/<model>" has this id.
func (p principal) allowsBareModel(model string) bool {
	if p.allowsAllModels() {
		return true
	}
	for _, m := range p.models {
		if _, rest, ok := strings.Cut(m, "/"); ok && rest == model {
			return true
		}
	}
	return false
}

// principalKey carries the principal in a request's context.
type principalKey struct{}

// ccwJob marks a request ccw makes for itself: a review, a replay, a
// model test. It owns no key and it is not admin.
var ccwJob = principal{name: "intact-review"}

// principalOf reads who sent a request. A request that passed no gate (an
// internal call) gets the zero principal, which owns nothing.
func principalOf(r *http.Request) principal {
	p, _ := r.Context().Value(principalKey{}).(principal)
	return p
}

// withPrincipal returns r with p as its sender.
func withPrincipal(r *http.Request, p principal) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), principalKey{}, p))
}

// clientOf names who made a request, for drift and the error log: its API
// key's name, "env", or "internal" for ccw's own calls and an open server.
// It is a label, never an access decision.
func clientOf(r *http.Request) string {
	if n := principalOf(r).name; n != "" {
		return n
	}
	return "internal"
}
