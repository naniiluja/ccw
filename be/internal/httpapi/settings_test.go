package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/naniiluja/ccw/internal/store"
)

type settingsBody struct {
	AuthMode          string `json:"authMode"`
	SessionTTLSeconds int    `json:"sessionTtlSeconds"`
	Timezone          string `json:"timezone"`
	Websearch         struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Count    int    `json:"count"`
		URL      string `json:"url"`
		KeySet   bool   `json:"keySet"`
	} `json:"websearch"`
}

func settingsServer(t *testing.T, gated bool) (*store.Store, http.Handler) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if !gated {
		_, h := newServer(s, nil, nil)
		return s, h
	}
	_, h := newServer(s, nil, authConfig())
	return s, h
}

func getSettings(t *testing.T, h http.Handler, token string) (*httptest.ResponseRecorder, settingsBody) {
	t.Helper()
	rec := sendAs(h, "GET", "/api/settings", "", token)
	var b settingsBody
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
			t.Fatalf("decode: %v: %s", err, rec.Body.String())
		}
	}
	return rec, b
}

func TestSettingsDefaultsWithNoEnvironment(t *testing.T) {
	for _, k := range []string{"CCW_SEARCH_PROVIDER", "CCW_SEARCH_KEY", "CCW_SEARCH_URL", "CCW_SEARCH_COUNT", "CCW_SEARCH_MODEL"} {
		t.Setenv(k, "")
	}
	_, h := settingsServer(t, true)
	rec, b := getSettings(t, h, "machine-tok")
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	if b.AuthMode != "password" || b.SessionTTLSeconds != 3600 {
		t.Errorf("auth=%q ttl=%d", b.AuthMode, b.SessionTTLSeconds)
	}
	w := b.Websearch
	if w.Provider != "" || w.URL != "" || w.KeySet || w.Count != 5 || w.Model != searchModel() {
		t.Errorf("websearch=%+v", w)
	}
}

func TestSettingsReportsTheSearchEnvironmentWithoutTheKey(t *testing.T) {
	cases := []struct {
		name, key string
		keySet    bool
		count     string
		wantCount int
	}{
		{"key present", "k-123", true, "8", 8},
		{"key empty", "", false, "8", 8},
		{"key blank", "   ", false, "", 5},
		{"count too big", "k-123", true, "99", 5},
		{"count zero", "k-123", true, "0", 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("CCW_SEARCH_PROVIDER", " Brave ")
			t.Setenv("CCW_SEARCH_KEY", c.key)
			t.Setenv("CCW_SEARCH_COUNT", c.count)
			t.Setenv("CCW_SEARCH_MODEL", "m-1")
			t.Setenv("CCW_SEARCH_URL", "https://user:pw-secret@search.example.com/v1/?apikey=q-secret#frag")
			_, h := settingsServer(t, true)
			rec, b := getSettings(t, h, "machine-tok")
			w := b.Websearch
			if w.Provider != "brave" || w.Model != "m-1" || w.Count != c.wantCount || w.KeySet != c.keySet {
				t.Errorf("websearch=%+v", w)
			}
			if w.URL != "https://search.example.com/v1" {
				t.Errorf("url=%q", w.URL)
			}
			for _, secret := range []string{"pw-secret", "q-secret", "k-123"} {
				if strings.Contains(rec.Body.String(), secret) {
					t.Errorf("body leaks %q: %s", secret, rec.Body.String())
				}
			}
		})
	}
}

func TestSettingsNeverContainsASecret(t *testing.T) {
	const canary = "canary-secret-value"
	for _, k := range []string{"CCW_SEARCH_KEY", "CCW_API_TOKEN", "CCW_SESSION_KEY", "CCW_PASSWORD", "CCW_ANTIGRAVITY_CLIENT_SECRET"} {
		t.Setenv(k, canary)
	}
	t.Setenv("CCW_SEARCH_PROVIDER", "brave")
	s, h := settingsServer(t, true)
	if err := s.SetSetting("password_hash", canary); err != nil {
		t.Fatal(err)
	}
	rec, _ := getSettings(t, h, "machine-tok")
	if rec.Code != 200 || strings.Contains(rec.Body.String(), canary) || strings.Contains(rec.Body.String(), "machine-tok") {
		t.Errorf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSettingsAuthMatrix(t *testing.T) {
	s, h := settingsServer(t, true)
	k, err := s.CreateAPIKey("laptop", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, token string
		want        int
	}{
		{"no token", "", 401},
		{"wrong token", "nope", 401},
		{"master token", "machine-tok", 200},
		{"dashboard key", k.Key, 200},
	} {
		if rec, _ := getSettings(t, h, c.token); rec.Code != c.want {
			t.Errorf("%s: code=%d want %d", c.name, rec.Code, c.want)
		}
	}
	rec := sendAs(h, "GET", "/api/settings", "", "")
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("401 content type %q", ct)
	}
	// A session cookie reads it too.
	cfg := authConfig()
	_, h2 := newServer(s, nil, cfg)
	req := loopbackRequest("GET", "/api/settings", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: cfg.IssueSession()})
	rr := httptest.NewRecorder()
	h2.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Errorf("session: code=%d", rr.Code)
	}
	for _, m := range []string{"POST", "PUT", "DELETE"} {
		if rec := sendAs(h, m, "/api/settings", `{}`, "machine-tok"); rec.Code != 405 && rec.Code != 404 {
			t.Errorf("%s: code=%d", m, rec.Code)
		}
	}
}

func TestSettingsWithNoGateReportsNone(t *testing.T) {
	_, h := settingsServer(t, false)
	rec, b := getSettings(t, h, "")
	if rec.Code != 200 || b.AuthMode != "none" {
		t.Errorf("code=%d mode=%q", rec.Code, b.AuthMode)
	}
}

// resetReportZone makes reportLocation read CCW_TZ again, and does so once more
// when the test ends so the cached zone never leaks into another test.
func resetReportZone(t *testing.T) {
	t.Helper()
	tzOnce = sync.Once{}
	t.Cleanup(func() { tzOnce = sync.Once{} })
}

func TestSettingsReportsTheEffectiveTimezone(t *testing.T) {
	cases := []struct {
		name, env, want string
	}{
		{"unset falls back to the fixed GMT+7 zone", "", "+07"},
		{"a valid IANA name is reported as given", "Asia/Tokyo", "Asia/Tokyo"},
		{"an unknown name falls back to the fixed GMT+7 zone", "Not/AZone", "+07"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("CCW_TZ", c.env)
			resetReportZone(t)
			_, h := settingsServer(t, true)
			rec, b := getSettings(t, h, "machine-tok")
			if rec.Code != 200 {
				t.Fatalf("code=%d", rec.Code)
			}
			if b.Timezone != c.want {
				t.Errorf("timezone=%q, want %q", b.Timezone, c.want)
			}
		})
	}
}
