package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/naniiluja/ccw/internal/auth"
	"github.com/naniiluja/ccw/internal/provider"
	"github.com/naniiluja/ccw/internal/store"
)

// testPassword opens the gate that authConfig builds. testPasswordHash is its
// PBKDF2 hash at 1000 iterations instead of the production 600000: the count
// is part of the hash, so the same verify path runs, and the tests that send a
// hundred wrong passwords stay fast under -race.
const (
	testPassword     = "test-password-123"
	testPasswordHash = "pbkdf2-sha256$1000$4yWu3QC0zOyO8JN/Wt1uzg$KprRgoM/MhgSr49JqucktkJA7D0kzJnL+LVrbl0Moo0"
)

func authConfig() *auth.Config {
	return auth.NewConfig(testPasswordHash, "machine-tok", []byte("k"), 3600)
}

// loginForm is the body a browser sends to POST /login.
func loginForm(password string) string {
	return url.Values{"password": {password}}.Encode()
}

func TestBrowserRoutesRequireSession(t *testing.T) {
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	h := NewWithAuth(s, nil, authConfig())

	for _, path := range []string{"/accounts", "/usage"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/ui/login" {
			t.Errorf("%s without session: code=%d loc=%q, want 302 -> /ui/login", path, rec.Code, rec.Header().Get("Location"))
		}
	}
}

func TestLoginWithPasswordThenReachDashboard(t *testing.T) {
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	h := NewWithAuth(s, nil, authConfig())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/login", strings.NewReader(loginForm(testPassword)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("login POST code=%d, want 302", rec.Code)
	}
	cookie := cookieNamed(rec, sessionCookie)
	if cookie == nil {
		t.Fatal("login set no session cookie")
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/accounts", nil)
	req2.AddCookie(cookie)
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("with session /accounts code=%d, want 200", rec2.Code)
	}
}

func TestLoginRejectsAWrongPasswordWith401(t *testing.T) {
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	h := NewWithAuth(s, nil, authConfig())
	for _, wrong := range []string{"", "wrong-password-1", testPassword + "x", testPassword[:len(testPassword)-1]} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/login", strings.NewReader(loginForm(wrong)))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("password %q: code=%d, want 401", wrong, rec.Code)
		}
		if len(rec.Result().Cookies()) > 0 {
			t.Errorf("password %q produced a cookie", wrong)
		}
	}
}

// The old form field no longer signs in, even with the right value.
func TestLoginIgnoresTheOldCodeField(t *testing.T) {
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	h := NewWithAuth(s, nil, authConfig())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/login", strings.NewReader(url.Values{"totp": {testPassword}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("the old field: code=%d, want 401", rec.Code)
	}
}

// After auth.LoginPerIPMax wrong passwords from one address the next attempt gets
// 429 with Retry-After, and even the right password waits for the window.
func TestLoginIsRateLimitedAfterRepeatedFailures(t *testing.T) {
	t.Setenv("CCW_OWNER_LOOPBACK", "")
	h := NewWithAuth(openStore(t), nil, authConfig())
	head := map[string]string{"CF-Connecting-IP": "198.51.100.77"}
	for i := 1; i <= auth.LoginPerIPMax; i++ {
		if rec := send(h, loginPost("wrong-password-1", addrTunnel, head)); rec.Code != http.StatusUnauthorized {
			t.Fatalf("wrong password %d: code=%d, want 401", i, rec.Code)
		}
	}
	rec := send(h, loginPost(testPassword, addrTunnel, head))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt %d: code=%d, want 429", auth.LoginPerIPMax+1, rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 without Retry-After")
	}
	if cookieNamed(rec, sessionCookie) != nil {
		t.Error("a rate-limited attempt opened a session")
	}
}

func TestProxyRequiresBearerToken(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer up.Close()
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	s.CreateConnection("groq", "x", "gsk")
	h := NewWithAuth(s, map[string]string{"groq": up.URL}, authConfig())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"groq/m"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no bearer: code=%d, want 401", rec.Code)
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"groq/m"}`))
	req2.Header.Set("Authorization", "Bearer machine-tok")
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("with bearer: code=%d, want 200", rec2.Code)
	}
}

// An Anthropic client sends its key as x-api-key. ccw accepts it as the API
// token and never forwards it to the provider.
func TestV1AcceptsXAPIKeyAndDoesNotForwardIt(t *testing.T) {
	var gotKey, gotAuth string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey, gotAuth = r.Header.Get("X-Api-Key"), r.Header.Get("Authorization")
		w.Write([]byte(`{}`))
	}))
	defer up.Close()
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	s.CreateConnection("claude", "c", "oauth-tok")
	cfg := authConfig()
	h := NewWithAuth(s, map[string]string{"claude": up.URL}, cfg)

	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"claude/claude-sonnet-5","max_tokens":1}`))
	req.Header.Set("X-Api-Key", cfg.APIToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if gotKey != "" {
		t.Errorf("x-api-key reached the provider: %q", gotKey)
	}
	if gotAuth != "Bearer oauth-tok" {
		t.Errorf("provider auth = %q, want the stored credential", gotAuth)
	}
}

const (
	addrLoopback = "127.0.0.1:54321"
	addrTunnel   = "127.0.0.1:54322" // cloudflared connects from this machine
	// wrongPassword is long enough to be a valid password, and is not testPassword.
	wrongPassword = "wrong-password-1"
)

// loginPost builds a POST /login the way a browser sends it. remoteAddr is the
// connection the server sees: the tunnel connects from loopback and names the
// real caller in CF-Connecting-IP.
func loginPost(password, remoteAddr string, header map[string]string, cookies ...*http.Cookie) *http.Request {
	form := url.Values{"password": {password}}
	r := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	r.Host = "127.0.0.1:20130"
	r.RemoteAddr = remoteAddr
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range header {
		r.Header.Set(k, v)
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	return r
}

func send(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// fillPublicBudget sends wrong passwords from a new address each time, so only the
// public budget can stop them. It returns the handler's last status.
func fillPublicBudget(t *testing.T, h http.Handler) {
	t.Helper()
	for i := 0; i < auth.LoginPublicMax; i++ {
		rec := send(h, loginPost(wrongPassword, addrTunnel, map[string]string{
			"CF-Connecting-IP": fmt.Sprintf("198.51.100.%d", i%256),
		}))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("fill attempt %d: code=%d, want 401", i, rec.Code)
		}
	}
}

// T1-1 (2) and T7-5: 200 concurrent wrong passwords through the real handler. A 401
// means the password was checked; a 429 means the guard stopped the request
// before that, so the count of 401 answers is the count of password checks.
func TestLoginHandlerEvaluatesNoMoreCodesThanTheCap(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfOf func(i int) string
		cap  int
	}{
		{"rotating addresses", func(i int) string { return fmt.Sprintf("198.51.100.%d", i) }, auth.LoginPublicMax},
		{"one address", func(int) string { return "198.51.100.7" }, auth.LoginPerIPMax},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := openStore(t)
			h := NewWithAuth(s, nil, authConfig())
			var evaluated, refused atomic.Int64
			var wg sync.WaitGroup
			for i := 0; i < 200; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					rec := send(h, loginPost(wrongPassword, addrTunnel, map[string]string{
						"CF-Connecting-IP": tc.cfOf(i),
					}))
					switch rec.Code {
					case http.StatusUnauthorized:
						evaluated.Add(1)
					case http.StatusTooManyRequests:
						refused.Add(1)
					default:
						t.Errorf("attempt %d: code=%d, want 401 or 429", i, rec.Code)
					}
				}(i)
			}
			wg.Wait()
			if got := int(evaluated.Load()); got > tc.cap {
				t.Fatalf("%d passwords checked, want at most %d", got, tc.cap)
			}
			if evaluated.Load()+refused.Load() != 200 {
				t.Fatalf("answers: %d evaluated + %d refused, want 200", evaluated.Load(), refused.Load())
			}
		})
	}
}

// T1-1 (3): wrong passwords below the cap never block the correct one.
func TestLoginAcceptsACorrectPasswordAfterFourWrongOnes(t *testing.T) {
	s := openStore(t)
	cfg := authConfig()
	h := NewWithAuth(s, nil, cfg)
	head := map[string]string{"CF-Connecting-IP": "198.51.100.30"}
	for i := 0; i < auth.LoginPerIPMax-1; i++ {
		if rec := send(h, loginPost(wrongPassword, addrTunnel, head)); rec.Code != http.StatusUnauthorized {
			t.Fatalf("wrong password %d: code=%d, want 401", i, rec.Code)
		}
	}
	rec := send(h, loginPost(testPassword, addrTunnel, head))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
		t.Fatalf("correct password after %d wrong ones: code=%d loc=%q, want 302 -> /",
			auth.LoginPerIPMax-1, rec.Code, rec.Header().Get("Location"))
	}
	if cookieNamed(rec, sessionCookie) == nil {
		t.Fatal("the correct password set no session cookie")
	}
}

// T1-1 (4): a correct password gives its charge back, so it leaves nothing behind.
func TestSuccessfulLoginLeavesNoEntryInAnyBudget(t *testing.T) {
	s := openStore(t)
	cfg := authConfig()
	a, h := newServer(s, nil, cfg)
	rec := send(h, loginPost(testPassword, addrTunnel,
		map[string]string{"CF-Connecting-IP": "198.51.100.31"}))
	if rec.Code != http.StatusFound {
		t.Fatalf("login code=%d, want 302", rec.Code)
	}
	public, addresses, devices := a.login.Counts()
	if public != 0 || addresses != 0 || devices != 0 {
		t.Fatalf("after a correct password: public=%d addresses=%d devices=%d, want 0 0 0", public, addresses, devices)
	}
}

// T1-2: a stranger who fills the public budget cannot keep the owner out. The
// owner path is a loopback connection with no forwarding header.
func TestAFullPublicBudgetNeverLocksOutTheOwnerPath(t *testing.T) {
	t.Setenv("CCW_OWNER_LOOPBACK", "1") // the owner lane exists only with the opt-in
	s := openStore(t)
	cfg := authConfig()
	h := NewWithAuth(s, nil, cfg)
	fillPublicBudget(t, h)

	// The budget is full: one more public attempt never reaches the password check.
	full := send(h, loginPost(wrongPassword, addrTunnel, map[string]string{"CF-Connecting-IP": "198.51.100.200"}))
	if full.Code != http.StatusTooManyRequests {
		t.Fatalf("public attempt with a full budget: code=%d, want 429", full.Code)
	}

	// (b) a correct password that arrives through the tunnel still gets 429.
	password := testPassword
	tun := send(h, loginPost(password, addrTunnel, map[string]string{"CF-Connecting-IP": "198.51.100.201"}))
	if tun.Code != http.StatusTooManyRequests {
		t.Fatalf("correct password from the tunnel with a full budget: code=%d, want 429", tun.Code)
	}

	// (a) the same password on the owner path signs in.
	own := send(h, loginPost(password, addrLoopback, nil))
	if own.Code != http.StatusFound || own.Header().Get("Location") != "/" {
		t.Fatalf("owner path with a full budget: code=%d loc=%q, want 302 -> /", own.Code, own.Header().Get("Location"))
	}
	if cookieNamed(own, sessionCookie) == nil {
		t.Fatal("the owner path set no session cookie")
	}
}

// The device lane: a browser that signed in before keeps its own budget, so a
// full public budget does not reach it. A forged cookie names no device.
func TestDeviceCookieSignsInWhileThePublicBudgetIsFull(t *testing.T) {
	s := openStore(t)
	cfg := authConfig()
	h := NewWithAuth(s, nil, cfg)
	fillPublicBudget(t, h)
	password := testPassword
	head := map[string]string{"CF-Connecting-IP": "198.51.100.210"}

	forged := &http.Cookie{Name: auth.DeviceCookie, Value: "0123456789abcdef.not-a-real-signature"}
	if rec := send(h, loginPost(password, addrTunnel, head, forged)); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("forged device cookie: code=%d, want 429 (the public lane)", rec.Code)
	}

	known := &http.Cookie{Name: auth.DeviceCookie, Value: cfg.IssueDevice()}
	rec := send(h, loginPost(password, addrTunnel, head, known))
	if rec.Code != http.StatusFound {
		t.Fatalf("known device with a full public budget: code=%d, want 302", rec.Code)
	}
	if cookieNamed(rec, sessionCookie) == nil {
		t.Fatal("the device lane set no session cookie")
	}
}

func TestSuccessfulLoginSetsADeviceCookie(t *testing.T) {
	s := openStore(t)
	cfg := authConfig()
	h := NewWithAuth(s, nil, cfg)
	rec := send(h, loginPost(testPassword, addrLoopback, nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("login code=%d, want 302", rec.Code)
	}
	ck := cookieNamed(rec, auth.DeviceCookie)
	if ck == nil {
		t.Fatal("login set no device cookie")
	}
	if _, ok := cfg.DeviceID(ck.Value); !ok {
		t.Errorf("the device cookie %q does not carry a valid signature", ck.Value)
	}
	if !ck.HttpOnly || ck.SameSite != http.SameSiteStrictMode || ck.MaxAge != deviceMaxAge {
		t.Errorf("device cookie flags: HttpOnly=%v SameSite=%v MaxAge=%d, want true Strict %d",
			ck.HttpOnly, ck.SameSite, ck.MaxAge, deviceMaxAge)
	}
	// A plain http request over loopback must not ask for Secure, or the
	// browser drops the cookie.
	if ck.Secure {
		t.Error("device cookie is Secure on a plain loopback request")
	}
}

// A browser keeps the device it already has, so its budget cannot be reset by
// signing in again.
func TestLoginKeepsAKnownDeviceCookie(t *testing.T) {
	s := openStore(t)
	cfg := authConfig()
	h := NewWithAuth(s, nil, cfg)
	known := &http.Cookie{Name: auth.DeviceCookie, Value: cfg.IssueDevice()}
	rec := send(h, loginPost(testPassword, addrLoopback, nil, known))
	if rec.Code != http.StatusFound {
		t.Fatalf("login code=%d, want 302", rec.Code)
	}
	if ck := cookieNamed(rec, auth.DeviceCookie); ck == nil || ck.Value != known.Value {
		t.Fatalf("device cookie changed on sign-in: %v", ck)
	}
}

// One attempt is charged to one lane only: a device pays from its own budget.
func TestDeviceLaneHasItsOwnBudget(t *testing.T) {
	s := openStore(t)
	cfg := authConfig()
	a, h := newServer(s, nil, cfg)
	known := &http.Cookie{Name: auth.DeviceCookie, Value: cfg.IssueDevice()}
	head := map[string]string{"CF-Connecting-IP": "198.51.100.220"}
	for i := 0; i < auth.LoginDeviceMax; i++ {
		if rec := send(h, loginPost(wrongPassword, addrTunnel, head, known)); rec.Code != http.StatusUnauthorized {
			t.Fatalf("device attempt %d: code=%d, want 401", i, rec.Code)
		}
	}
	if rec := send(h, loginPost(wrongPassword, addrTunnel, head, known)); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt %d from one device: code=%d, want 429", auth.LoginDeviceMax+1, rec.Code)
	}
	public, addresses, devices := a.login.Counts()
	if public != 0 || addresses != 0 {
		t.Errorf("device failures reached another budget: public=%d addresses=%d, want 0 0", public, addresses)
	}
	if devices != 1 {
		t.Errorf("device budgets=%d, want 1", devices)
	}
}

// A sign-in body carries one password field. Anything larger than the cap is
// refused before the form is read.
func TestLoginBodyIsCapped(t *testing.T) {
	s := openStore(t)
	h := NewWithAuth(s, nil, authConfig())
	body := "password=" + strings.Repeat("1", loginBodyMax+1)
	r := httptest.NewRequest("POST", "/login", strings.NewReader(body))
	r.Host = "127.0.0.1:20130"
	r.RemoteAddr = addrLoopback
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := send(h, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized login body: code=%d, want 400", rec.Code)
	}
}

// C2 (round 2): without the opt-in, a loopback caller with no forwarding header
// is public. A proxy that adds no header makes every internet caller look local.
// The lane decision itself is tested in internal/auth.
func TestLoopbackWithoutHeaderIsPublicByDefault(t *testing.T) {
	t.Setenv("CCW_OWNER_LOOPBACK", "")
	h := NewWithAuth(openStore(t), nil, authConfig())
	for i := 1; i <= auth.LoginPerIPMax; i++ {
		if rec := send(h, loginPost(wrongPassword, addrLoopback, nil)); rec.Code != http.StatusUnauthorized {
			t.Fatalf("wrong password %d: status %d, want 401", i, rec.Code)
		}
	}
	if rec := send(h, loginPost(wrongPassword, addrLoopback, nil)); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("wrong password %d: status %d, want 429", auth.LoginPerIPMax+1, rec.Code)
	}
}

// C2 (round 2): CCW_OWNER_LOOPBACK=1 gives the owner's unmetered lane back.
func TestOwnerLoopbackOptInRestoresLocalLane(t *testing.T) {
	t.Setenv("CCW_OWNER_LOOPBACK", "1")
	h := NewWithAuth(openStore(t), nil, authConfig())
	for i := 1; i <= 3*auth.LoginPerIPMax; i++ {
		if rec := send(h, loginPost(wrongPassword, addrLoopback, nil)); rec.Code != http.StatusUnauthorized {
			t.Fatalf("wrong password %d: status %d, want 401", i, rec.Code)
		}
	}
}

// guardReq builds a request with an explicit Host, which httptest.NewRequest
// otherwise fixes at example.com.
func guardReq(method, target, host string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, target, body)
	r.Host = host
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	return r
}

func serveGuard(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func defCount(t *testing.T, s *store.Store) int {
	t.Helper()
	defs, err := s.ProviderDefs()
	if err != nil {
		t.Fatalf("provider defs: %v", err)
	}
	return len(defs)
}

const evilDef = `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"put_provider_def",` +
	`"arguments":{"def":{"id":"evil","name":"Evil","kind":"apikey","api":"openai","baseUrl":"https://evil.example/v1"}}}}`

const evilDefJSON = `{"id":"evil","name":"Evil","kind":"apikey","api":"openai","baseUrl":"https://evil.example/v1"}`

// T6-4 (1) and T7-4: with no auth the Host is the only thing that separates a
// browser on the operator's machine from a rebound DNS name.
func TestNoAuthRefusesAForeignHost(t *testing.T) {
	s := openStore(t)
	h := New(s, nil)

	for _, host := range []string{"evil.example", "evil.example:1234", "ccw.example"} {
		if rec := serveGuard(h, guardReq("GET", "/accounts", host, nil)); rec.Code != http.StatusForbidden {
			t.Errorf("GET /accounts Host %q: code=%d, want 403", host, rec.Code)
		}
	}
	for _, host := range []string{"127.0.0.1:20130", "localhost:20130", "127.0.0.1", "localhost", "[::1]:20130"} {
		if rec := serveGuard(h, guardReq("GET", "/accounts", host, nil)); rec.Code != http.StatusOK {
			t.Errorf("GET /accounts Host %q: code=%d, want 200", host, rec.Code)
		}
	}
}

// T3-2 (2): a rebound name reaching /mcp must not run an admin tool.
func TestNoAuthRefusesMCPFromAForeignHost(t *testing.T) {
	s := openStore(t)
	h := New(s, nil)
	before := defCount(t, s)

	r := guardReq("POST", "/mcp", "evil.example:1234", strings.NewReader(evilDef))
	r.Header.Set("Origin", "http://evil.example:1234")
	if rec := serveGuard(h, r); rec.Code != http.StatusForbidden {
		t.Errorf("POST /mcp from a foreign Host: code=%d body=%s, want 403", rec.Code, rec.Body.String())
	}
	if after := defCount(t, s); after != before {
		t.Errorf("provider defs changed: %d -> %d", before, after)
	}
}

// T3-2 (1): a cross-site page posting to the loopback port must not run a tool.
func TestNoAuthRefusesMCPFromACrossSiteOrigin(t *testing.T) {
	s := openStore(t)
	h := New(s, nil)
	before := defCount(t, s)

	r := guardReq("POST", "/mcp", "127.0.0.1:20130", strings.NewReader(evilDef))
	r.Header.Set("Origin", "https://evil.example")
	if rec := serveGuard(h, r); rec.Code != http.StatusForbidden {
		t.Errorf("POST /mcp with a cross-site Origin: code=%d body=%s, want 403", rec.Code, rec.Body.String())
	}
	if after := defCount(t, s); after != before {
		t.Errorf("provider defs changed: %d -> %d", before, after)
	}
}

// T7-4 and T6-4 (2): the Origin rule, in every shape the ruling names.
func TestOriginRuleOnAWriteRoute(t *testing.T) {
	cases := []struct {
		name    string
		origin  string
		fetch   string
		refused bool
	}{
		{name: "cross-site origin", origin: "https://evil.example", refused: true},
		{name: "opaque origin", origin: "null", refused: true},
		{name: "cross-site fetch metadata", fetch: "cross-site", refused: true},
		{name: "same host, other scheme", origin: "https://127.0.0.1:20130"},
		{name: "same origin", origin: "http://127.0.0.1:20130"},
		{name: "no origin"},
		{name: "same-origin fetch metadata", origin: "http://127.0.0.1:20130", fetch: "same-origin"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := openStore(t)
			h := New(s, nil)
			before := defCount(t, s)

			r := guardReq("POST", "/provider-defs", "127.0.0.1:20130", strings.NewReader(evilDefJSON))
			if c.origin != "" {
				r.Header.Set("Origin", c.origin)
			}
			if c.fetch != "" {
				r.Header.Set("Sec-Fetch-Site", c.fetch)
			}
			rec := serveGuard(h, r)
			after := defCount(t, s)
			if c.refused {
				if rec.Code != http.StatusForbidden {
					t.Errorf("code=%d body=%s, want 403", rec.Code, rec.Body.String())
				}
				if after != before {
					t.Errorf("provider defs changed: %d -> %d", before, after)
				}
				return
			}
			if rec.Code == http.StatusForbidden {
				t.Errorf("code=403 body=%s, want the request through", rec.Body.String())
			}
			if after != before+1 {
				t.Errorf("provider defs = %d, want %d", after, before+1)
			}
		})
	}
}

// T7-4 and T6-4 (2): a GET carries no write, so the Origin rule leaves it alone.
func TestOriginRuleSkipsSafeMethods(t *testing.T) {
	s := openStore(t)
	h := New(s, nil)
	r := guardReq("GET", "/accounts", "127.0.0.1:20130", nil)
	r.Header.Set("Origin", "https://evil.example")
	if rec := serveGuard(h, r); rec.Code != http.StatusOK {
		t.Errorf("GET with a cross-site Origin: code=%d, want 200", rec.Code)
	}
}

// T7-4: with auth on, the Host allow-list does not apply, so the documented
// tunnel deployment keeps working.
func TestAuthOnKeepsAPublicHost(t *testing.T) {
	s := openStore(t)
	cfg := authConfig()
	h := NewWithAuth(s, nil, cfg)

	rec := serveGuard(h, guardReq("GET", "/accounts", "ccw.example", nil))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/ui/login" {
		t.Errorf("GET /accounts with no session: code=%d loc=%q, want 302 -> /ui/login", rec.Code, rec.Header().Get("Location"))
	}

	lr := guardReq("POST", "/login", "ccw.example", strings.NewReader(loginForm(testPassword)))
	lr.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	lr.Header.Set("Origin", "https://ccw.example")
	login := serveGuard(h, lr)
	if login.Code != http.StatusFound {
		t.Fatalf("login: code=%d, want 302", login.Code)
	}
	session := cookieNamed(login, sessionCookie)
	if session == nil {
		t.Fatal("login set no session cookie")
	}

	pr := guardReq("POST", "/provider-defs", "ccw.example", strings.NewReader(evilDefJSON))
	pr.Header.Set("Origin", "https://ccw.example")
	pr.AddCookie(session)
	if rec := serveGuard(h, pr); rec.Code != http.StatusOK {
		t.Errorf("same-origin write over the tunnel: code=%d body=%s, want 200", rec.Code, rec.Body.String())
	}
}

// T7-4: with auth on, a cross-site Origin is still refused.
func TestAuthOnStillRefusesACrossSiteOrigin(t *testing.T) {
	s := openStore(t)
	h := NewWithAuth(s, nil, authConfig())
	r := guardReq("POST", "/provider-defs", "ccw.example", strings.NewReader(evilDefJSON))
	r.Header.Set("Origin", "https://evil.example")
	if rec := serveGuard(h, r); rec.Code != http.StatusForbidden {
		t.Errorf("cross-site write with auth on: code=%d, want 403", rec.Code)
	}
}

// T6-4 (3): responses must not be framed.
func TestFramingHeadersOnEveryResponse(t *testing.T) {
	s := openStore(t)
	h := New(s, nil)
	for _, path := range []string{"/login", "/accounts"} {
		rec := serveGuard(h, guardReq("GET", path, "127.0.0.1:20130", nil))
		for header, want := range map[string]string{
			"Content-Security-Policy": "frame-ancestors 'none'",
			"X-Frame-Options":         "DENY",
			"X-Content-Type-Options":  "nosniff",
		} {
			if got := rec.Header().Get(header); got != want {
				t.Errorf("GET %s: %s = %q, want %q", path, header, got, want)
			}
		}
	}
}

// keyThroughDashboard makes an API key the way an operator does, so the test
// covers the name createKey stores.
func keyThroughDashboard(t *testing.T, h http.Handler, ck *http.Cookie, name string) store.APIKey {
	t.Helper()
	req := loopbackRequest("POST", "/keys", strings.NewReader(`{"name":`+quote(name)+`}`))
	req.AddCookie(ck)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var k store.APIKey
	json.Unmarshal(rec.Body.Bytes(), &k)
	if !strings.HasPrefix(k.Key, "sk-ccw-") {
		t.Fatalf("create key %q: %d %s", name, rec.Code, rec.Body.String())
	}
	return k
}

func quote(s string) string { b, _ := json.Marshal(s); return string(b) }

func getAs(h http.Handler, path, token string) *httptest.ResponseRecorder {
	req := loopbackRequest("GET", path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func getWithCookie(h http.Handler, path string, ck *http.Cookie) *httptest.ResponseRecorder {
	req := loopbackRequest("GET", path, nil)
	req.AddCookie(ck)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// A key's display name is free text. It must never decide admin rights.
func TestAKeyNameCannotGrantAdminRights(t *testing.T) {
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	cfg := authConfig()
	h := NewWithAuth(s, nil, cfg)
	ck := loginCookie(t, h, cfg)

	for _, name := range []string{"laptop", "internal", " internal ", "env"} {
		k := keyThroughDashboard(t, h, ck, name)
		add := rpc(t, h, k.Key, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"add_filter",
			"arguments":{"provider":"groq","kind":"field","pattern":"planted_by_a_key"}}}`)
		if text, isErr := toolText(t, add); !isErr || !strings.Contains(text, "master token") {
			t.Errorf("add_filter as %q: isErr=%v text=%s", name, isErr, text)
		}
		put := rpc(t, h, k.Key, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"put_provider_def",
			"arguments":{"def":{"id":"planted","kind":"apikey","api":"openai","baseUrl":"https://evil.example/v1"}}}}`)
		if text, isErr := toolText(t, put); !isErr || !strings.Contains(text, "master token") {
			t.Errorf("put_provider_def as %q: isErr=%v text=%s", name, isErr, text)
		}
	}
	list := rpc(t, h, cfg.APIToken, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_filters","arguments":{}}}`)
	if text, _ := toolText(t, list); strings.Contains(text, "planted_by_a_key") {
		t.Errorf("a key added a filter: %s", text)
	}
	defs, _ := s.ProviderDefs()
	for _, d := range defs {
		if d.ID == "planted" {
			t.Error("a key declared a provider")
		}
	}
	// The master token still runs the same tool.
	ok := rpc(t, h, cfg.APIToken, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"add_filter",
		"arguments":{"provider":"groq","kind":"field","pattern":"added_by_the_master_token"}}}`)
	if text, isErr := toolText(t, ok); isErr {
		t.Errorf("add_filter with the master token: %s", text)
	}
}

// Two keys can carry the same name, so the stored bodies belong to the key id.
func TestErrorBodiesBelongToTheKeyThatCausedThem(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), "secret-prompt-text") {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":{"message":"refused"},"detail":"secret-answer-text"}`)
			return
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"OK"}}]}`)
	}))
	defer up.Close()
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	s.CreateConnection("groq", "g", "k")
	cfg := authConfig()
	h := NewWithAuth(s, map[string]string{"groq": up.URL}, cfg)
	ck := loginCookie(t, h, cfg)

	keyA := keyThroughDashboard(t, h, ck, "key")
	// The same name, put straight into the store: a duplicate can already exist.
	keyB, err := s.CreateAPIKey("key", nil)
	if err != nil {
		t.Fatalf("create key B: %v", err)
	}
	keyC := keyThroughDashboard(t, h, ck, "env")

	if rec := postV1As(h, keyB.Key, `{"model":"groq/llama","messages":[{"role":"user","content":"secret-prompt-text"}]}`); rec.Code != 400 {
		t.Fatalf("key B call: %d %s", rec.Code, rec.Body.String())
	}
	list, _ := s.ListUpstreamErrors(store.ErrorFilter{})
	if len(list) != 1 {
		t.Fatalf("stored errors = %d", len(list))
	}
	id := list[0].ID
	if list[0].Client != "key" {
		t.Errorf("client label = %q, want the key's name", list[0].Client)
	}

	hidden := func(what, body string) {
		t.Helper()
		if strings.Contains(body, "secret-prompt-text") || strings.Contains(body, "secret-answer-text") {
			t.Errorf("%s reads another client's bodies: %s", what, body)
		}
	}
	shown := func(what, body string) {
		t.Helper()
		if !strings.Contains(body, "secret-prompt-text") || !strings.Contains(body, "secret-answer-text") {
			t.Errorf("%s lost the bodies: %s", what, body)
		}
	}
	for _, k := range []struct{ what, tok string }{{"key A", keyA.Key}, {`key named "env"`, keyC.Key}} {
		hidden(k.what, getAs(h, "/api/errors/"+itoa(id), k.tok).Body.String())
		hidden(k.what, getAs(h, "/api/errors", k.tok).Body.String())
	}
	shown("the key that caused it", getAs(h, "/api/errors/"+itoa(id), keyB.Key).Body.String())
	shown("the master token", getAs(h, "/api/errors/"+itoa(id), cfg.APIToken).Body.String())
	shown("the session", getWithCookie(h, "/errors/"+itoa(id), ck).Body.String())

	// A dashboard key still serves its own traffic.
	if rec := postV1As(h, keyA.Key, `{"model":"groq/llama","messages":[{"role":"user","content":"hello"}]}`); rec.Code != 200 {
		t.Errorf("/v1 with a key: %d %s", rec.Code, rec.Body.String())
	}
	if rec := getAs(h, "/api/quota", keyA.Key); rec.Code != 200 {
		t.Errorf("/api/quota with a key: %d %s", rec.Code, rec.Body.String())
	}
}

func postV1As(h http.Handler, token, body string) *httptest.ResponseRecorder {
	req := loopbackRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// A declared provider carries the credentials of every account that signs in
// through it, so a caller that is not admin reads them masked.
func TestProviderDefCredentialsAreMaskedForAKey(t *testing.T) {
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	cfg := authConfig()
	h := NewWithAuth(s, nil, cfg)
	ck := loginCookie(t, h, cfg)
	k := keyThroughDashboard(t, h, ck, "laptop")

	def := `{"id":"acme","kind":"oauth-code","api":"openai","baseUrl":"https://acme.example/v1",
		"headers":{"X-Org-Key":"h-secret"},
		"oauth":{"authorizeUrl":"https://acme.example/auth","tokenUrl":"https://acme.example/token",
		"redirectUri":"https://acme.example/cb","clientId":"cid","clientSecret":"s3cret"}}`
	req := loopbackRequest("POST", "/provider-defs", strings.NewReader(def))
	req.AddCookie(ck)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("declare: %d %s", rec.Code, rec.Body.String())
	}
	defer provider.SetDeclared(nil)

	secret := func(what, body string) {
		t.Helper()
		if strings.Contains(body, "s3cret") || strings.Contains(body, "h-secret") {
			t.Errorf("%s leaks a credential: %s", what, body)
		}
	}
	mcpList := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_provider_defs","arguments":{}}}`
	text, _ := toolText(t, rpc(t, h, k.Key, mcpList))
	secret("list_provider_defs with a key", text)
	secret("GET /api/provider-defs with a key", getAs(h, "/api/provider-defs", k.Key).Body.String())
	secret("GET /api/provider-defs/{id} with a key", getAs(h, "/api/provider-defs/acme", k.Key).Body.String())

	// An admin reads the def as stored, so an edit-and-save round trip never
	// writes a mask.
	full, _ := toolText(t, rpc(t, h, cfg.APIToken, mcpList))
	if !strings.Contains(full, "s3cret") || !strings.Contains(full, "h-secret") {
		t.Errorf("list_provider_defs with the master token = %s", full)
	}
	if b := getAs(h, "/api/provider-defs", cfg.APIToken).Body.String(); !strings.Contains(b, "s3cret") || !strings.Contains(b, "h-secret") {
		t.Errorf("GET /api/provider-defs with the master token = %s", b)
	}
	if b := getWithCookie(h, "/provider-defs/acme", ck).Body.String(); !strings.Contains(b, "s3cret") || !strings.Contains(b, "h-secret") {
		t.Errorf("GET /provider-defs/{id} with the session = %s", b)
	}
	// The stored def is untouched by a masked read.
	d, ok := provider.Declared("acme")
	if !ok || d.OAuth.ClientSecret != "s3cret" || d.Headers["X-Org-Key"] != "h-secret" {
		t.Errorf("the stored def changed: %+v", d)
	}
}
