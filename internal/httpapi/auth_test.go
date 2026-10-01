package httpapi

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/naniiluja/ccw/internal/auth"
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
		if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login" {
			t.Errorf("%s without session: code=%d loc=%q, want 302 -> /login", path, rec.Code, rec.Header().Get("Location"))
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
