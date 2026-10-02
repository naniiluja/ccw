package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/naniiluja/ccw/internal/auth"
	"github.com/naniiluja/ccw/internal/store"
)

const (
	acceptJSON    = "application/json"
	acceptBrowser = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
)

func TestWantsJSONOnlyForAnExplicitJSONAccept(t *testing.T) {
	for _, tc := range []struct {
		accept string
		want   bool
	}{
		{"", false},
		{"*/*", false},
		{acceptBrowser, false},
		{"application/json", true},
		{"application/json, text/plain, */*", true},
		{"text/html, application/json;q=0.5", true},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		if tc.accept != "" {
			r.Header.Set("Accept", tc.accept)
		}
		if got := wantsJSON(r); got != tc.want {
			t.Errorf("wantsJSON(%q)=%v, want %v", tc.accept, got, tc.want)
		}
	}
}

func sessionHandler(t *testing.T, cfg *auth.Config) http.Handler {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return NewWithAuth(s, nil, cfg)
}

func do(h http.Handler, method, target, accept, body string, ck *http.Cookie) *httptest.ResponseRecorder {
	req := loopbackRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if ck != nil {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func jsonBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q is not JSON: %v", rec.Body.String(), err)
	}
	return m
}

func TestRequireSessionNegotiatesTheRefusal(t *testing.T) {
	cfg := authConfig()
	h := sessionHandler(t, cfg)
	valid := loginCookie(t, h, cfg)
	expired := &http.Cookie{Name: sessionCookie, Value: cfg.IssueSession() + "x"}
	for _, tc := range []struct {
		name   string
		accept string
		ck     *http.Cookie
		code   int
	}{
		{"browser no session", "", nil, 302},
		{"browser html accept", acceptBrowser, nil, 302},
		{"browser bad session", "", expired, 302},
		{"json no session", acceptJSON, nil, 401},
		{"json bad session", acceptJSON, expired, 401},
		{"browser valid session", "", valid, 200},
		{"json valid session", acceptJSON, valid, 200},
	} {
		rec := do(h, "GET", "/usage", tc.accept, "", tc.ck)
		if rec.Code != tc.code {
			t.Errorf("%s: code=%d, want %d", tc.name, rec.Code, tc.code)
			continue
		}
		switch tc.code {
		case 302:
			if loc := rec.Header().Get("Location"); loc != "/ui/login" {
				t.Errorf("%s: Location=%q, want /ui/login", tc.name, loc)
			}
		case 401:
			if jsonBody(t, rec)["error"] == nil {
				t.Errorf("%s: no error field in %q", tc.name, rec.Body.String())
			}
		}
	}
}

func TestLoginSubmitNegotiatesTheOutcome(t *testing.T) {
	cfg := authConfig()
	h := sessionHandler(t, cfg)

	// Right password: JSON gets 200 {"ok":true} with both cookies; browser 302 to "/".
	rec := do(h, "POST", "/login", acceptJSON, loginForm(testPassword), nil)
	if rec.Code != 200 || jsonBody(t, rec)["ok"] != true {
		t.Fatalf("json login: code=%d body=%q", rec.Code, rec.Body.String())
	}
	if cookieNamed(rec, sessionCookie) == nil || cookieNamed(rec, auth.DeviceCookie) == nil {
		t.Error("json login must set the session and device cookies")
	}
	rec = do(h, "POST", "/login", "", loginForm(testPassword), nil)
	if rec.Code != 302 || rec.Header().Get("Location") != "/" {
		t.Errorf("browser login: code=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}

	// Empty and wrong passwords: 401 JSON either way.
	for _, pw := range []string{"", "wrong-password-xx"} {
		rec = do(h, "POST", "/login", acceptJSON, loginForm(pw), nil)
		if rec.Code != 401 || jsonBody(t, rec)["error"] == nil || cookieNamed(rec, sessionCookie) != nil {
			t.Errorf("json login pw=%q: code=%d body=%q", pw, rec.Code, rec.Body.String())
		}
	}
}

func TestLoginSubmitJSONStillRateLimits(t *testing.T) {
	cfg := authConfig()
	h := sessionHandler(t, cfg)
	var last *httptest.ResponseRecorder
	for i := 0; i < 200; i++ {
		last = do(h, "POST", "/login", acceptJSON, loginForm("wrong-password-xx"), nil)
		if last.Code == 429 {
			break
		}
	}
	if last.Code != 429 || last.Header().Get("Retry-After") == "" {
		t.Fatalf("code=%d retry-after=%q, want 429 with Retry-After", last.Code, last.Header().Get("Retry-After"))
	}
	if jsonBody(t, last)["error"] == nil {
		t.Error("429 body must be a JSON error")
	}
}

func TestLogoutNegotiatesTheOutcome(t *testing.T) {
	cfg := authConfig()
	h := sessionHandler(t, cfg)
	rec := do(h, "POST", "/logout", acceptJSON, "", nil)
	if rec.Code != 200 || jsonBody(t, rec)["ok"] != true {
		t.Fatalf("json logout: code=%d body=%q", rec.Code, rec.Body.String())
	}
	if ck := cookieNamed(rec, sessionCookie); ck == nil || ck.MaxAge >= 0 {
		t.Error("logout must clear the session cookie")
	}
	rec = do(h, "POST", "/logout", "", "", nil)
	if rec.Code != 302 || rec.Header().Get("Location") != "/ui/login" {
		t.Errorf("browser logout: code=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestAccountWritesReturnJSONWhenAsked(t *testing.T) {
	cfg := authConfig()
	h := sessionHandler(t, cfg)
	ck := loginCookie(t, h, cfg)
	form := url.Values{"provider": {"groq"}, "label": {"Work"}, "secret": {"gsk-xyz"}}.Encode()

	rec := do(h, "POST", "/accounts", acceptJSON, form, ck)
	if rec.Code != 200 || jsonBody(t, rec)["ok"] != true {
		t.Fatalf("json create: code=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = do(h, "POST", "/accounts", "", form, ck)
	if rec.Code != 302 || rec.Header().Get("Location") != "/" {
		t.Fatalf("browser create: code=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}

	list := do(h, "GET", "/accounts", acceptJSON, "", ck)
	var listed struct {
		Accounts []struct {
			ID string `json:"id"`
		} `json:"accounts"`
	}
	rows := &listed.Accounts
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil || len(*rows) < 2 {
		t.Fatalf("list: %v %q", err, list.Body.String())
	}
	rec = do(h, "POST", "/accounts/"+(*rows)[0].ID+"/delete", acceptJSON, "", ck)
	if rec.Code != 200 || jsonBody(t, rec)["ok"] != true {
		t.Fatalf("json delete: code=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = do(h, "POST", "/accounts/"+(*rows)[1].ID+"/delete", "", "", ck)
	if rec.Code != 302 || rec.Header().Get("Location") != "/" {
		t.Fatalf("browser delete: code=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestSessionRouteReportsTheCallerState(t *testing.T) {
	cfg := authConfig()
	h := sessionHandler(t, cfg)
	valid := loginCookie(t, h, cfg)
	expired := &http.Cookie{Name: sessionCookie, Value: cfg.IssueSession() + "x"}

	for _, tc := range []struct {
		name string
		ck   *http.Cookie
		want [3]bool
	}{
		{"gate on, no session", nil, [3]bool{true, false, false}},
		{"gate on, bad session", expired, [3]bool{true, false, false}},
		{"gate on, session", valid, [3]bool{true, true, true}},
	} {
		rec := do(h, "GET", "/api/session", "", "", tc.ck)
		m := jsonBody(t, rec)
		if rec.Code != 200 || len(m) != 3 || m["authRequired"] != tc.want[0] || m["authenticated"] != tc.want[1] || m["admin"] != tc.want[2] {
			t.Errorf("%s: code=%d body=%q, want %v", tc.name, rec.Code, rec.Body.String(), tc.want)
		}
	}

	// A master token is not a session: the SPA is for browsers.
	req := loopbackRequest("GET", "/api/session", nil)
	req.Header.Set("Authorization", "Bearer machine-tok")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if jsonBody(t, rec)["authenticated"] != false {
		t.Error("a bearer token must not read as a signed-in session")
	}
	if strings.Contains(rec.Body.String(), "machine-tok") || strings.Contains(rec.Body.String(), valid.Value) {
		t.Error("session route leaked a secret")
	}

	// No gate.
	s, _ := store.Open(filepath.Join(t.TempDir(), "open.db"))
	defer s.Close()
	open := NewWithAuth(s, nil, nil)
	rec = do(open, "GET", "/api/session", "", "", nil)
	m := jsonBody(t, rec)
	if rec.Code != 200 || m["authRequired"] != false || m["authenticated"] != true || m["admin"] != true {
		t.Errorf("no gate: code=%d body=%q", rec.Code, rec.Body.String())
	}
}
