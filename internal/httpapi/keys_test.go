package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/naniiluja/ccw/internal/store"
)

// postKey runs one /keys/{id}/… call. A nil cookie sends no session.
func postKey(h http.Handler, path, body string, ck *http.Cookie, token string) *httptest.ResponseRecorder {
	req := loopbackRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if ck != nil {
		req.AddCookie(ck)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// The three key routes are the dashboard's only way to read a full API key, so
// each one must stay behind the session.
func TestKeyRoutesNeedTheSession(t *testing.T) {
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	cfg := authConfig()
	h := NewWithAuth(s, nil, cfg)
	ck := loginCookie(t, h, cfg)
	k := keyThroughDashboard(t, h, ck, "laptop")

	paths := []string{"/keys/" + k.ID + "/reveal", "/keys/" + k.ID + "/active", "/keys/" + k.ID + "/delete"}
	bodies := []string{"", `{"active":false}`, ""}

	for i, p := range paths {
		rec := postKey(h, p, bodies[i], nil, "")
		if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login" {
			t.Errorf("%s with no session: code=%d loc=%q, want 302 -> /login", p, rec.Code, rec.Header().Get("Location"))
		}
		if rec := postKey(h, p, bodies[i], nil, cfg.APIToken); rec.Code == http.StatusOK {
			t.Errorf("%s answered the master token", p)
		}
		if rec := postKey(h, p, bodies[i], nil, k.Key); rec.Code == http.StatusOK {
			t.Errorf("%s answered a dashboard key", p)
		}
	}

	rec := postKey(h, paths[0], "", ck, "")
	var got struct {
		Key string `json:"key"`
	}
	json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != http.StatusOK || got.Key != k.Key || !strings.HasPrefix(got.Key, "sk-ccw-") {
		t.Errorf("reveal with the session: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := postKey(h, paths[1], `{"active":false}`, ck, ""); rec.Code != http.StatusOK {
		t.Errorf("active with the session: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if s.ValidAPIKey(k.Key) {
		t.Error("the key still works after it was switched off")
	}
	if rec := postKey(h, paths[2], "", ck, ""); rec.Code != http.StatusOK {
		t.Errorf("delete with the session: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if list, _ := s.ListAPIKeys(); len(list) != 0 {
		t.Errorf("the key was not deleted: %+v", list)
	}
}

// A key carries no trusted flag: the create answer and the listing have no
// "trusted" field, and the route that set it is gone.
func TestKeyCreationHasNoTrustedFlag(t *testing.T) {
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	cfg := authConfig()
	h := NewWithAuth(s, nil, cfg)
	ck := loginCookie(t, h, cfg)

	req := loopbackRequest("POST", "/keys", strings.NewReader(`{"name":"laptop"}`))
	req.AddCookie(ck)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created["id"] == nil {
		t.Fatalf("create key: %d %s", rec.Code, rec.Body.String())
	}
	if _, ok := created["trusted"]; ok {
		t.Errorf("create answer has a trusted field: %s", rec.Body.String())
	}

	req = loopbackRequest("GET", "/keys", nil)
	req.AddCookie(ck)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var listed struct {
		Keys []map[string]any `json:"keys"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil || len(listed.Keys) != 1 {
		t.Fatalf("list keys: %d %s", rec.Code, rec.Body.String())
	}
	if _, ok := listed.Keys[0]["trusted"]; ok {
		t.Errorf("listing has a trusted field: %s", rec.Body.String())
	}

	id, _ := created["id"].(string)
	if rec := postKey(h, "/keys/"+id+"/trusted", `{"trusted":true}`, ck, ""); rec.Code == http.StatusOK {
		t.Errorf("POST /keys/{id}/trusted answered 200: %s", rec.Body.String())
	}
}

func TestKeyLimiterSlidesOverSixtySeconds(t *testing.T) {
	l := keyLimiter{seen: map[string][]time.Time{}}
	t0 := time.Unix(1_000_000, 0)
	for i, at := range []time.Duration{0, 20 * time.Second} {
		if ok, _ := l.allow("k", 2, t0.Add(at)); !ok {
			t.Fatalf("request %d refused", i)
		}
	}
	// Third within the minute: refused until the first one is 60 s old.
	if ok, wait := l.allow("k", 2, t0.Add(30*time.Second)); ok || wait != 30*time.Second {
		t.Errorf("third: ok=%v wait=%v, want refused for 30s", ok, wait)
	}
	if n := l.count("k", t0.Add(30*time.Second)); n != 2 {
		t.Errorf("count = %d, the refused request must not count", n)
	}
	if ok, _ := l.allow("k", 2, t0.Add(61*time.Second)); !ok {
		t.Error("refused after the first request left the window")
	}
	// A key that went quiet holds no memory.
	l.count("k", t0.Add(10*time.Minute))
	if _, held := l.seen["k"]; held {
		t.Error("an idle key is still held")
	}
}

func TestUsageDayIn(t *testing.T) {
	// 2026-01-01 23:30 UTC is 2026-01-02 06:30 in GMT+7: the request counts
	// toward the second, not the first.
	in := time.Date(2026, 1, 1, 23, 30, 0, 0, time.UTC)
	if got := usageDayIn(in, time.FixedZone("+07", 7*60*60)); got != "2026-01-02" {
		t.Fatalf("GMT+7 day = %s, want 2026-01-02", got)
	}
	if got := usageDayIn(in, time.UTC); got != "2026-01-01" {
		t.Fatalf("UTC day = %s, want 2026-01-01", got)
	}
}

func TestReportLocationDefaultsToGMT7(t *testing.T) {
	os.Unsetenv("CCW_TZ")
	tzOnce = sync.Once{}
	loc := reportLocation()
	_, offset := time.Now().In(loc).Zone()
	if offset != 7*60*60 {
		t.Fatalf("default offset = %d, want %d", offset, 7*60*60)
	}
}
