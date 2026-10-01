package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/naniiluja/ccw/internal/store"
)

// accountUpstream answers each account (told apart by its bearer) with the
// status its script names, and records the order the accounts were called in.
type accountUpstream struct {
	mu     sync.Mutex
	calls  []string
	status map[string]int
}

func (u *accountUpstream) start(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		who := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		u.mu.Lock()
		u.calls = append(u.calls, who)
		code := u.status[who]
		u.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if code != 0 && code != http.StatusOK {
			w.WriteHeader(code)
			fmt.Fprintf(w, `{"error":{"message":"%s answered %d"}}`, who, code)
			return
		}
		fmt.Fprintf(w, `{"ok":"%s"}`, who)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestFailoverAccountMatrix pins the decision the account loop takes for each
// combination of account count, standby, and status: a busy status moves on
// while another account remains, the last account's answer is relayed as it
// came, and a status that is not busy is relayed at once.
func TestFailoverAccountMatrix(t *testing.T) {
	cases := []struct {
		name      string
		accounts  int // 1: only ka; 2: ka normal, kb standby (always tried second)
		status    map[string]int
		wantCode  int
		wantBody  string
		wantCalls string
	}{
		{"one account busy is relayed", 1, map[string]int{"ka": 429}, 429, "ka answered 429", "ka"},
		{"one account ok", 1, nil, 200, `{"ok":"ka"}`, "ka"},
		{"busy first moves to the standby", 2, map[string]int{"ka": 429}, 200, `{"ok":"kb"}`, "ka,kb"},
		{"server error first moves on", 2, map[string]int{"ka": 500}, 200, `{"ok":"kb"}`, "ka,kb"},
		{"conflict first moves on", 2, map[string]int{"ka": 409}, 200, `{"ok":"kb"}`, "ka,kb"},
		{"every account busy relays the last", 2, map[string]int{"ka": 503, "kb": 503}, 503, "kb answered 503", "ka,kb"},
		{"a refusal is not retried", 2, map[string]int{"ka": 400}, 400, "ka answered 400", "ka"},
		{"a plain 401 is not retried", 2, map[string]int{"ka": 401}, 401, "ka answered 401", "ka"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := &accountUpstream{status: tc.status}
			url := u.start(t)
			s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
			defer s.Close()
			s.CreateConnection("groq", "a", "ka")
			if tc.accounts == 2 {
				b, _ := s.CreateConnection("groq", "b", "kb")
				if err := s.SetStandby(b.ID, true); err != nil {
					t.Fatal(err)
				}
			}
			h := New(s, map[string]string{"groq": url})
			rec := postV1(h, `{"model":"groq/llama","messages":[]}`)
			if rec.Code != tc.wantCode || !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("code=%d body=%s, want %d with %q", rec.Code, rec.Body.String(), tc.wantCode, tc.wantBody)
			}
			u.mu.Lock()
			got := strings.Join(u.calls, ",")
			u.mu.Unlock()
			if got != tc.wantCalls {
				t.Errorf("calls = %s, want %s", got, tc.wantCalls)
			}
		})
	}
}

// An OAuth account that keeps answering 401 is refreshed exactly once, sent
// exactly twice, and the caller sees the provider's second 401.
func TestFailoverRefreshesOAuthOnceOn401(t *testing.T) {
	var refreshes, sends int32
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&refreshes, 1)
		fmt.Fprintf(w, `{"access_token":"tok-%d","expires_in":3600}`, n)
	}))
	defer tokenSrv.Close()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&sends, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"still revoked"}}`))
	}))
	defer up.Close()

	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	c, _ := s.CreateConnection("claude", "P", "revoked-token")
	s.SetOAuth(c.ID, store.OAuthCreds{
		RefreshToken: "rt", TokenURL: tokenSrv.URL, ClientID: "cid",
		ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	})
	h := NewWithAuth(s, map[string]string{"claude": up.URL}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, loopbackRequest("POST", "/v1/messages", strings.NewReader(`{"model":"claude/m","max_tokens":1}`)))

	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "still revoked") {
		t.Errorf("code=%d body=%s, want the upstream 401 relayed", rec.Code, rec.Body.String())
	}
	if n := atomic.LoadInt32(&refreshes); n != 1 {
		t.Errorf("refreshes = %d, want exactly 1", n)
	}
	if n := atomic.LoadInt32(&sends); n != 2 {
		t.Errorf("upstream sends = %d, want 2 (first try and one retry)", n)
	}
}

// A Copilot bearer the upstream refuses is exchanged again and the call is
// sent once more with the new one.
func TestFailoverReexchangesCopilotOn401(t *testing.T) {
	var exchanges int32
	ex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&exchanges, 1)
		fmt.Fprintf(w, `{"token":"cop-%d","expires_at":%d}`, n, time.Now().Add(time.Hour).Unix())
	}))
	defer ex.Close()
	old := copilotTokenURL
	copilotTokenURL = ex.URL
	defer func() { copilotTokenURL = old }()

	var mu sync.Mutex
	var auths []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		mu.Lock()
		auths = append(auths, r.Header.Get("Authorization"))
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer cop-2" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":{"message":"bad bearer"}}`))
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer up.Close()
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	s.CreateConnection("github", "gh", "gho_abc")
	h := New(s, map[string]string{"github": up.URL})

	rec := postV1(h, `{"model":"github/gpt-4.1","messages":[]}`)
	if rec.Code != http.StatusOK || rec.Body.String() != `{"ok":true}` {
		t.Errorf("code=%d body=%s, want the retried answer", rec.Code, rec.Body.String())
	}
	mu.Lock()
	got := strings.Join(auths, ",")
	mu.Unlock()
	if got != "Bearer cop-1,Bearer cop-2" {
		t.Errorf("upstream auths = %s, want cop-1 then cop-2", got)
	}
	if n := atomic.LoadInt32(&exchanges); n != 2 {
		t.Errorf("exchanges = %d, want 2", n)
	}
}

// Copilot's 400 that names /responses sends the call there (the request is
// prepared a second time); any other 400 reaches the caller unchanged.
func TestFailoverCopilotResponsesRetry(t *testing.T) {
	cases := []struct {
		name      string
		model     string
		refusal   string
		wantPaths string
		wantCode  int
		wantBody  string
	}{
		{"responses only is retried there", "gpt-4.1-chartest-a",
			`{"error":{"message":"model is not accessible via the /chat/completions endpoint"}}`,
			"/chat/completions,/responses", 200, `"content":"from responses"`},
		{"another refusal is relayed", "gpt-4.1-chartest-b",
			`{"error":{"message":"max_tokens is too large"}}`,
			"/chat/completions", 400, "max_tokens is too large"},
	}
	ex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"token":"cop","expires_at":%d}`, time.Now().Add(time.Hour).Unix())
	}))
	defer ex.Close()
	old := copilotTokenURL
	copilotTokenURL = ex.URL
	defer func() { copilotTokenURL = old }()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			copilotResponsesModels.Delete(tc.model)
			t.Cleanup(func() { copilotResponsesModels.Delete(tc.model) })
			var mu sync.Mutex
			var paths []string
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				mu.Lock()
				paths = append(paths, r.URL.Path)
				mu.Unlock()
				switch r.URL.Path {
				case "/chat/completions":
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					io.WriteString(w, tc.refusal)
				case "/responses":
					w.Header().Set("Content-Type", "text/event-stream")
					io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"from responses\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n")
				}
			}))
			defer up.Close()
			s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
			defer s.Close()
			s.CreateConnection("github", "gh", "gho")
			h := New(s, map[string]string{"github": up.URL})

			rec := postV1(h, `{"model":"github/`+tc.model+`","messages":[{"role":"user","content":"hi"}]}`)
			if rec.Code != tc.wantCode || !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("code=%d body=%s, want %d with %q", rec.Code, rec.Body.String(), tc.wantCode, tc.wantBody)
			}
			mu.Lock()
			got := strings.Join(paths, ",")
			mu.Unlock()
			if got != tc.wantPaths {
				t.Errorf("paths = %s, want %s", got, tc.wantPaths)
			}
		})
	}
}

// An Antigravity account whose project cannot be found is skipped (an
// errSkipAccount, not a refusal of the request): the next account answers, and
// with no other account the caller hears that none was usable.
func TestFailoverSkipsAnAccountWithoutAProject(t *testing.T) {
	cases := []struct {
		name     string
		withB    bool
		wantCode int
		wantBody string
	}{
		{"the next account answers", true, 200, `"content":"OK"`},
		{"no account left", false, 500, "no usable account"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var bodies []string
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				switch {
				case strings.HasSuffix(r.URL.Path, ":loadCodeAssist"):
					w.WriteHeader(http.StatusForbidden)
				case strings.HasSuffix(r.URL.Path, ":fetchAvailableModels"):
					w.Write([]byte(`{"models":{"gemini-3-flash":{}}}`))
				default:
					mu.Lock()
					bodies = append(bodies, string(b))
					mu.Unlock()
					w.Header().Set("Content-Type", "text/event-stream")
					io.WriteString(w, `data: {"response":{"responseId":"r","modelVersion":"gemini-3-flash","candidates":[{"content":{"parts":[{"text":"OK"}]},"finishReason":"STOP"}]}}`+"\n\n")
				}
			}))
			defer up.Close()
			old := antigravityProdURL
			antigravityProdURL = up.URL
			defer func() { antigravityProdURL = old }()

			s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
			defer s.Close()
			s.CreateConnection("antigravity", "a", "fake-access-a")
			if tc.withB {
				b, _ := s.CreateConnection("antigravity", "b", "fake-access-b")
				s.SetMeta(b.ID, map[string]string{"projectId": "proj-b"})
				s.SetStandby(b.ID, true)
			}
			h := New(s, map[string]string{"antigravity": up.URL})
			rec := postV1(h, `{"model":"antigravity/gemini-3-flash","messages":[{"role":"user","content":"hi"}]}`)
			if rec.Code != tc.wantCode || !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("code=%d body=%s, want %d with %q", rec.Code, rec.Body.String(), tc.wantCode, tc.wantBody)
			}
			mu.Lock()
			defer mu.Unlock()
			if tc.withB && (len(bodies) != 1 || !strings.Contains(bodies[0], `"project":"proj-b"`)) {
				t.Errorf("generation calls = %q, want one, on proj-b", bodies)
			}
			if !tc.withB && len(bodies) != 0 {
				t.Errorf("generation calls = %q, want none", bodies)
			}
		})
	}
}

// A stream the provider breaks after its first event still reaches the caller
// up to the break, in both the passed-through and the translated shape, and
// the handler returns rather than waiting on the pipe.
func TestFailoverStreamCutMidway(t *testing.T) {
	cases := []struct {
		name string
		path string
		body string
	}{
		{"same shape", "/v1/chat/completions", `{"model":"groq/llama","stream":true,"messages":[]}`},
		{"translated", "/v1/messages", `{"model":"groq/llama","stream":true,"max_tokens":5,"messages":[{"role":"user","content":"hi"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, `data: {"id":"c1","object":"chat.completion.chunk","model":"llama","choices":[{"index":0,"delta":{"role":"assistant","content":"Hel"},"finish_reason":null}]}`+"\n\n")
				w.(http.Flusher).Flush()
				panic(http.ErrAbortHandler)
			}))
			defer up.Close()
			s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
			defer s.Close()
			s.CreateConnection("groq", "a", "ka")
			h := New(s, map[string]string{"groq": up.URL})

			done := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, loopbackRequest("POST", tc.path, strings.NewReader(tc.body)))
				done <- rec
			}()
			rec := waitFor(t, done, "the cut stream to be relayed")
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Hel") {
				t.Errorf("code=%d body=%s, want 200 with the first event", rec.Code, rec.Body.String())
			}
		})
	}
}
