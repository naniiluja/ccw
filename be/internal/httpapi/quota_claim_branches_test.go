package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/naniiluja/ccw/internal/store"
)

// claimFixture is a signed-in server whose claude provider points at a fake
// that answers the usage read with usage() and counts reset claims.
type claimFixture struct {
	a      *api
	h      http.Handler
	s      *store.Store
	ck     *http.Cookie
	conn   store.Connection
	claims *atomic.Int32
}

// newClaimFixture builds a claimFixture. onUsage, when not nil, runs on every
// usage read before the answer is written.
func newClaimFixture(t *testing.T, usage func() map[string]any, onUsage func(f *claimFixture)) *claimFixture {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	f := &claimFixture{s: s, claims: &atomic.Int32{}}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/oauth/usage") {
			if onUsage != nil {
				onUsage(f)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(usage())
			return
		}
		if strings.Contains(r.URL.Path, "/reset_rate_limits") {
			f.claims.Add(1)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"result": "reset"})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(fake.Close)
	cfg := authConfig()
	f.a, f.h = newServer(s, map[string]string{"claude": fake.URL}, cfg)
	f.conn, _ = s.CreateConnection("claude", "acc", "token-x")
	s.SetMeta(f.conn.ID, map[string]string{"claudeOrgId": "11111111-2222-3333-4444-555555555555"})
	f.ck = loginCookie(t, f.h, cfg)
	return f
}

func weeklyAvailableUsage() map[string]any {
	return map[string]any{"juniper_tide": map[string]any{"eligible": true, "available": true}}
}

func claimOutcome(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var res struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return res.Outcome
}

// TestClaimPreflightRefusesBadRequests pins every early refusal of claimReset
// before it takes the connection lock or reads the provider.
func TestClaimPreflightRefusesBadRequests(t *testing.T) {
	f := newClaimFixture(t, weeklyAvailableUsage, nil)
	other, _ := f.s.CreateConnection("groq", "g", "gsk-x")
	good := `{"resetId":"weekly","requestId":"req-1"}`
	cases := []struct {
		name     string
		conn     string
		body     string
		ctype    string
		sfs      string
		wantCode int
		wantBody string
	}{
		{"same-site fetch", f.conn.ID, good, "application/json", "same-site", http.StatusForbidden, "cross-site request refused"},
		{"wrong content type", f.conn.ID, good, "text/plain", "", http.StatusBadRequest, "Content-Type must be application/json"},
		{"unknown connection", "nope", good, "application/json", "", http.StatusNotFound, "unknown connection"},
		{"provider without claimer", other.ID, good, "application/json", "", http.StatusNotFound, "provider has no claimer"},
		{"bad json", f.conn.ID, `{"resetId":`, "application/json", "", http.StatusBadRequest, "bad json body"},
		{"empty resetId", f.conn.ID, `{"resetId":"","requestId":"req-1"}`, "application/json", "", http.StatusBadRequest, "resetId must be 1-200 characters"},
		{"long resetId", f.conn.ID, `{"resetId":"` + strings.Repeat("x", 201) + `","requestId":"req-1"}`, "application/json", "", http.StatusBadRequest, "resetId must be 1-200 characters"},
		{"bad requestId", f.conn.ID, `{"resetId":"weekly","requestId":"bad id!"}`, "application/json", "", http.StatusBadRequest, "invalid requestId format"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := loopbackRequest("POST", "/quota/"+tc.conn+"/reset", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.ctype)
			if tc.sfs != "" {
				req.Header.Set("Sec-Fetch-Site", tc.sfs)
			}
			req.AddCookie(f.ck)
			rec := httptest.NewRecorder()
			f.h.ServeHTTP(rec, req)
			if rec.Code != tc.wantCode || !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("code = %d body = %s, want %d %q", rec.Code, rec.Body.String(), tc.wantCode, tc.wantBody)
			}
		})
	}
	if n := f.claims.Load(); n != 0 {
		t.Errorf("claims = %d, want 0", n)
	}
}

// TestClaimWithoutPathIDIsUnknownConnection covers a direct call with no path
// value, which the mux never produces.
func TestClaimWithoutPathIDIsUnknownConnection(t *testing.T) {
	f := newClaimFixture(t, weeklyAvailableUsage, nil)
	req := loopbackRequest("POST", "/quota//reset", strings.NewReader(`{"resetId":"weekly","requestId":"req-1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(f.ck)
	rec := httptest.NewRecorder()
	f.a.claimReset(rec, req)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "unknown connection") {
		t.Errorf("code = %d body = %s, want 404 unknown connection", rec.Code, rec.Body.String())
	}
}

// TestClaimForgetsDoneEntriesAfterFifteenMinutes shows that a stale done entry
// no longer answers already_done: the claim goes through to the provider.
func TestClaimForgetsDoneEntriesAfterFifteenMinutes(t *testing.T) {
	f := newClaimFixture(t, weeklyAvailableUsage, nil)
	key := f.conn.ID + ":req-old"
	f.a.claims.mu.Lock()
	f.a.claims.done[key] = claimDoneEntry{outcome: "reset", at: time.Now().Add(-16 * time.Minute)}
	f.a.claims.mu.Unlock()

	rec := postJSON(f.h, "/quota/"+f.conn.ID+"/reset", `{"resetId":"weekly","requestId":"req-old"}`, f.ck, "")
	if rec.Code != http.StatusOK || claimOutcome(t, rec) != "reset" {
		t.Fatalf("code = %d body = %s, want 200 reset", rec.Code, rec.Body.String())
	}
	if n := f.claims.Load(); n != 1 {
		t.Errorf("claims = %d, want 1", n)
	}
	f.a.claims.mu.Lock()
	de := f.a.claims.done[key]
	f.a.claims.mu.Unlock()
	if time.Since(de.at) > time.Minute {
		t.Errorf("done entry at = %v, want a fresh one", de.at)
	}
}

// TestClaimUnknownRetryLikelySpentWhenRowGone covers a retry of an unknown
// claim whose reset no longer appears in the fresh read.
func TestClaimUnknownRetryLikelySpentWhenRowGone(t *testing.T) {
	usage := func() map[string]any {
		return map[string]any{"cedar_ember": map[string]any{"eligible": true, "grants": []any{}}}
	}
	f := newClaimFixture(t, usage, nil)
	resetKey := f.conn.ID + ":grant:gone"
	f.a.claims.mu.Lock()
	f.a.claims.unknown[resetKey] = claimUnknownEntry{requestID: "req-u", left: 1, at: time.Now().Add(-65 * time.Second)}
	f.a.claims.mu.Unlock()

	rec := postJSON(f.h, "/quota/"+f.conn.ID+"/reset", `{"resetId":"grant:gone","requestId":"req-u"}`, f.ck, "")
	if rec.Code != http.StatusOK || claimOutcome(t, rec) != "likely_spent" {
		t.Fatalf("code = %d body = %s, want 200 likely_spent", rec.Code, rec.Body.String())
	}
	f.a.claims.mu.Lock()
	_, stillUnknown := f.a.claims.unknown[resetKey]
	de, done := f.a.claims.done[f.conn.ID+":req-u"]
	f.a.claims.mu.Unlock()
	if stillUnknown || !done || de.outcome != "likely_spent" {
		t.Errorf("unknown kept = %v, done = %v %q, want removed and likely_spent", stillUnknown, done, de.outcome)
	}
	if n := f.claims.Load(); n != 0 {
		t.Errorf("claims = %d, want 0", n)
	}
}

// TestClaimUnknownRetryWeeklyNeedsNextAt shows the weekly rule: a lower Left
// counts as spent only when the provider also names the next reset time.
func TestClaimUnknownRetryWeeklyNeedsNextAt(t *testing.T) {
	cases := []struct {
		name     string
		nextAt   string
		wantCode int
		wantBody string
	}{
		{"used with next time", "2026-10-08T00:00:00Z", http.StatusOK, `"likely_spent"`},
		{"not at limit", "", http.StatusConflict, "not_at_limit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			usage := func() map[string]any {
				jt := map[string]any{"eligible": true, "available": false}
				if tc.nextAt != "" {
					jt["next_available_at"] = tc.nextAt
				}
				return map[string]any{"juniper_tide": jt}
			}
			f := newClaimFixture(t, usage, nil)
			f.a.claims.mu.Lock()
			f.a.claims.unknown[f.conn.ID+":weekly"] = claimUnknownEntry{requestID: "req-w", left: 1, at: time.Now().Add(-65 * time.Second)}
			f.a.claims.mu.Unlock()

			rec := postJSON(f.h, "/quota/"+f.conn.ID+"/reset", `{"resetId":"weekly","requestId":"req-w"}`, f.ck, "")
			if rec.Code != tc.wantCode || !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("code = %d body = %s, want %d %q", rec.Code, rec.Body.String(), tc.wantCode, tc.wantBody)
			}
			if n := f.claims.Load(); n != 0 {
				t.Errorf("claims = %d, want 0", n)
			}
		})
	}
}

// TestClaimExpiredHoldIsReleased shows that a hold older than 60 seconds no
// longer blocks a claim even when Left has not dropped.
func TestClaimExpiredHoldIsReleased(t *testing.T) {
	f := newClaimFixture(t, weeklyAvailableUsage, nil)
	resetKey := f.conn.ID + ":weekly"
	f.a.claims.mu.Lock()
	f.a.claims.hold[resetKey] = claimHoldEntry{left: 1, at: time.Now().Add(-65 * time.Second)}
	f.a.claims.mu.Unlock()

	rec := postJSON(f.h, "/quota/"+f.conn.ID+"/reset", `{"resetId":"weekly","requestId":"req-h"}`, f.ck, "")
	if rec.Code != http.StatusOK || claimOutcome(t, rec) != "reset" {
		t.Fatalf("code = %d body = %s, want 200 reset", rec.Code, rec.Body.String())
	}
	f.a.claims.mu.Lock()
	he := f.a.claims.hold[resetKey]
	f.a.claims.mu.Unlock()
	if time.Since(he.at) > time.Minute {
		t.Errorf("hold at = %v, want the new hold of this reset", he.at)
	}
}

// TestClaimHTTP401RefreshesTheTokenInTheBackground shows that a claim the
// provider answers with 401 refreshes the OAuth token after the answer.
func TestClaimHTTP401RefreshesTheTokenInTheBackground(t *testing.T) {
	var tokenCalls atomic.Int32
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenCalls.Add(1)
		w.Write([]byte(`{"access_token":"fresh-token","expires_in":3600}`))
	}))
	defer tokenSrv.Close()
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/oauth/usage") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(weeklyAvailableUsage())
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer fake.Close()

	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	cfg := authConfig()
	_, h := newServer(s, map[string]string{"claude": fake.URL}, cfg)
	c, _ := s.CreateConnection("claude", "acc", "stale-token")
	s.SetMeta(c.ID, map[string]string{"claudeOrgId": "11111111-2222-3333-4444-555555555555"})
	s.SetOAuth(c.ID, store.OAuthCreds{
		RefreshToken: "rt",
		TokenURL:     tokenSrv.URL,
		ClientID:     "cid",
		ExpiresAt:    time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	})
	ck := loginCookie(t, h, cfg)

	rec := postJSON(h, "/quota/"+c.ID+"/reset", `{"resetId":"weekly","requestId":"req-401r"}`, ck, "")
	if rec.Code != http.StatusOK || claimOutcome(t, rec) != "failed" {
		t.Fatalf("code = %d body = %s, want 200 failed", rec.Code, rec.Body.String())
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if sec, _ := s.Secret(c.ID); sec == "fresh-token" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("token calls = %d, the token was never refreshed", tokenCalls.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestClaimTokenUnreadableAfterFreshRead answers 503 when the connection's
// credential cannot be read for the claim itself.
func TestClaimTokenUnreadableAfterFreshRead(t *testing.T) {
	f := newClaimFixture(t, weeklyAvailableUsage, func(f *claimFixture) {
		f.s.DeleteConnection(f.conn.ID)
	})
	rec := postJSON(f.h, "/quota/"+f.conn.ID+"/reset", `{"resetId":"weekly","requestId":"req-t"}`, f.ck, "")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "cannot read connection token") {
		t.Errorf("code = %d body = %s, want 503 cannot read connection token", rec.Code, rec.Body.String())
	}
	if n := f.claims.Load(); n != 0 {
		t.Errorf("claims = %d, want 0", n)
	}
}
