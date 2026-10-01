package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/naniiluja/ccw/internal/store"
)

func TestProxyForwardsPathBodyAndAuthUnchanged(t *testing.T) {
	var gotPath, gotAuth, gotBody, gotQuery, gotCT string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"upstream":"said this"}`))
	}))
	defer up.Close()

	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.CreateConnection("groq", "test", "gsk-abc")
	if err != nil {
		t.Fatal(err)
	}

	h := New(s, map[string]string{"groq": up.URL})
	// The provider prefix is the only byte range ccw changes.
	body := `{"model":"llama-3.3-70b-versatile","messages":[]}`
	req := loopbackRequest("POST", "/v1/chat/completions?beta=1",
		strings.NewReader(`{"model":"groq/llama-3.3-70b-versatile","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if gotPath != "/chat/completions" {
		t.Errorf("upstream path = %q, want /chat/completions", gotPath)
	}
	if gotQuery != "beta=1" {
		t.Errorf("query = %q, want beta=1: the query string must survive", gotQuery)
	}
	if gotBody != body {
		t.Errorf("body was altered:\n got %s\nwant %s", gotBody, body)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want it forwarded", gotCT)
	}
	if gotAuth != "Bearer gsk-abc" {
		t.Errorf("auth header = %q", gotAuth)
	}
	if rec.Body.String() != `{"upstream":"said this"}` {
		t.Errorf("response was altered: %s", rec.Body.String())
	}
}

// A caller must never be able to override the credential this proxy attaches.
func TestProxyReplacesAnyIncomingAuthorization(t *testing.T) {
	var gotAuth string
	var authCount int
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		authCount = len(r.Header.Values("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()

	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	s.CreateConnection("groq", "test", "gsk-real")

	h := New(s, map[string]string{"groq": up.URL})
	req := loopbackRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"groq/m"}`))
	req.Header.Set("Authorization", "Bearer attacker-supplied")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if gotAuth != "Bearer gsk-real" {
		t.Errorf("auth = %q, want the stored credential to win", gotAuth)
	}
	if authCount != 1 {
		t.Errorf("Authorization appeared %d times, want exactly 1", authCount)
	}
}

// A streamed response must reach the caller as it arrives, not after the upstream
// has finished. Buffering would break every SSE client.
func TestProxyStreamsWithoutBuffering(t *testing.T) {
	release := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		<-release
		io.WriteString(w, "data: second\n\n")
	}))
	defer up.Close()

	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	s.CreateConnection("groq", "test", "gsk-abc")

	srv := httptest.NewServer(New(s, map[string]string{"groq": up.URL}))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/stream", "application/json", strings.NewReader(`{"model":"groq/m"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	buf := make([]byte, len("data: first\n\n"))
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		close(release)
		t.Fatalf("first chunk did not arrive before the upstream finished: %v", err)
	}
	if string(buf) != "data: first\n\n" {
		close(release)
		t.Fatalf("first chunk = %q", buf)
	}
	close(release)
}

// A class A provider only works when the upstream believes it is talking to the
// genuine tool. Identity headers must therefore survive a caller that sends its
// own, and feature headers must not be overwritten when the caller did send one.
func TestProxyAppliesClassAIdentityAndDefaults(t *testing.T) {
	var gotUA, gotApp, gotVersion, gotBeta string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotApp = r.Header.Get("X-App")
		gotVersion = r.Header.Get("Anthropic-Version")
		gotBeta = r.Header.Get("Anthropic-Beta")
		w.Write([]byte(`{}`))
	}))
	defer up.Close()

	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.CreateConnection("claude", "test", "oauth-token")
	if err != nil {
		t.Fatal(err)
	}

	h := New(s, map[string]string{"claude": up.URL})
	req := loopbackRequest("POST", "/v1/messages", strings.NewReader(`{"model":"claude/m","max_tokens":1}`))
	// The caller sends its own identity and its own feature selection.
	req.Header.Set("User-Agent", "some-other-client/1.0")
	req.Header.Set("Anthropic-Beta", "caller-chose-this")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if gotUA != "claude-cli/2.1.278 (external, sdk-cli)" {
		t.Errorf("User-Agent = %q: identity must replace whatever the caller sent", gotUA)
	}
	if gotApp != "cli" {
		t.Errorf("X-App = %q, want cli", gotApp)
	}
	if gotVersion != "2023-06-01" {
		t.Errorf("Anthropic-Version = %q: a missing default must be filled", gotVersion)
	}
	if gotBeta != "caller-chose-this" {
		t.Errorf("Anthropic-Beta = %q: a default must not overwrite the caller's choice", gotBeta)
	}
}

func TestV1RejectsProviderWithNoAccount(t *testing.T) {
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	h := New(s, nil)
	req := loopbackRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"claude/m","max_tokens":1}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Error("the error body mentions the credential")
	}
}

// A stored connection of a provider this build does not know is never called.
func TestV1RefusesUnwiredProvider(t *testing.T) {
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	s.CreateConnection("somegateway", "imported", "token")

	h := New(s, nil)
	req := loopbackRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"somegateway/m"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for a provider this build cannot serve", rec.Code)
	}
}

// Passthrough is not a POST-only idea. A provider exposes GET endpoints too
// (model listings, quota), and a proxy that refuses them is not transparent.
func TestModelsListsEveryProviderWithItsPrefix(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/models" {
			t.Errorf("upstream got %s %s, want GET /models", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"data":[{"id":"llama"},{"id":"qwen"}]}`))
	}))
	defer up.Close()

	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	s.CreateConnection("groq", "a", "k")
	s.CreateConnection("nvidia", "b", "k")
	h := New(s, map[string]string{"groq": up.URL, "nvidia": up.URL})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, loopbackRequest("GET", "/v1/models", nil))
	body := rec.Body.String()
	for _, want := range []string{`"groq/llama"`, `"groq/qwen"`, `"nvidia/llama"`} {
		if !strings.Contains(body, want) {
			t.Errorf("models = %s, missing %s", body, want)
		}
	}
}

func TestOpenAICallerReachesAnthropicProvider(t *testing.T) {
	var gotPath, gotBody string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotPath, gotBody = r.URL.Path, string(b)
		if strings.Contains(gotBody, `"stream":true`) {
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"claude-x\",\"usage\":{\"input_tokens\":5}}}\n\n"+
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi\"}}\n\n"+
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n"+
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"msg_1","type":"message","model":"claude-x","content":[{"type":"text","text":"Hi"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":2}}`))
	}))
	defer up.Close()
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	c, _ := s.CreateConnection("claude", "c", "tok")
	h := New(s, map[string]string{"claude": up.URL})

	rec := postV1(h, `{"model":"claude/claude-x","messages":[{"role":"system","content":"sys"},{"role":"user","content":"hello"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if gotPath != "/messages" || !strings.Contains(gotBody, `"system":[{"text":"sys","type":"text"}]`) || !strings.Contains(gotBody, `"max_tokens":8192`) {
		t.Errorf("upstream got %s %s", gotPath, gotBody)
	}
	if b := rec.Body.String(); !strings.Contains(b, `"object":"chat.completion"`) || !strings.Contains(b, `"content":"Hi"`) {
		t.Errorf("caller got %s", b)
	}

	rec = postV1(h, `{"model":"claude/claude-x","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
	if b := rec.Body.String(); !strings.Contains(b, `"content":"Hi"`) || !strings.Contains(b, "data: [DONE]") {
		t.Errorf("stream caller got %s", b)
	}
	// Usage is counted from the provider's own answer, for both calls.
	rows, _ := s.Usage()
	if len(rows) != 1 || rows[0].ConnectionID != c.ID || rows[0].Requests != 2 || rows[0].InputTokens != 10 {
		t.Errorf("usage rows = %+v", rows)
	}
}

func TestAnthropicCallerReachesOpenAIProvider(t *testing.T) {
	var gotPath, gotBody string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotPath, gotBody = r.URL.Path, string(b)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, `data: {"id":"chatcmpl-1","model":"llama","choices":[{"index":0,"delta":{"content":"Yo"}}]}`+"\n\n"+
			`data: {"id":"chatcmpl-1","model":"llama","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n"+
			`data: {"id":"chatcmpl-1","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":1}}`+"\n\n"+"data: [DONE]\n\n")
	}))
	defer up.Close()
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	s.CreateConnection("groq", "g", "k")
	h := New(s, map[string]string{"groq": up.URL})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, loopbackRequest("POST", "/v1/messages",
		strings.NewReader(`{"model":"groq/llama","max_tokens":10,"stream":true,"system":"sys","messages":[{"role":"user","content":"hi"}]}`)))
	if gotPath != "/chat/completions" || !strings.Contains(gotBody, `"include_usage":true`) || !strings.Contains(gotBody, `"role":"system"`) {
		t.Errorf("upstream got %s %s", gotPath, gotBody)
	}
	b := rec.Body.String()
	for _, want := range []string{"event: message_start", `"text":"Yo"`, `"stop_reason":"end_turn"`, "event: message_stop"} {
		if !strings.Contains(b, want) {
			t.Errorf("caller stream missing %s:\n%s", want, b)
		}
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content-type = %q", ct)
	}
}

func TestTranslatedErrorKeepsStatusAndShape(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"bad model","type":"invalid_request_error"}}`))
	}))
	defer up.Close()
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	s.CreateConnection("groq", "g", "k")
	h := New(s, map[string]string{"groq": up.URL})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, loopbackRequest("POST", "/v1/messages", strings.NewReader(`{"model":"groq/x","max_tokens":5,"messages":[]}`)))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"type":"error"`) || !strings.Contains(rec.Body.String(), "bad model") {
		t.Errorf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCountTokensIsEstimatedForAnOpenAIProvider(t *testing.T) {
	called := false
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer up.Close()
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	s.CreateConnection("groq", "g", "k")
	h := New(s, map[string]string{"groq": up.URL})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, loopbackRequest("POST", "/v1/messages/count_tokens",
		strings.NewReader(`{"model":"groq/m","messages":[{"role":"user","content":"hello there"}]}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"input_tokens":`) || called {
		t.Errorf("code=%d body=%s upstream called=%v", rec.Code, rec.Body.String(), called)
	}
}
