package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/naniiluja/ccw/internal/provider"
	"github.com/naniiluja/ccw/internal/store"
	"github.com/naniiluja/ccw/internal/translate"
)

func TestDiaShapeForRoutesToTheBridge(t *testing.T) {
	p, ok := provider.Lookup("dia")
	if !ok {
		t.Fatal("dia provider not registered")
	}

	// Every model Dia's Vertex backend whitelists, so a new one cannot reach
	// the bridge through an unrouted path.
	for _, model := range []string{
		"claude-opus-4-8", "claude-opus-4-6",
	} {
		shape, path := shapeFor(p, model)
		if shape != translate.Anthropic {
			t.Errorf("model %q: shape = %q, want %q", model, shape, translate.Anthropic)
		}
		// The bridge rebuilds the Vertex-shaped path Dia's handler wants, so
		// ccw only names the bridge's own Messages endpoint.
		if path != "v1/messages" {
			t.Errorf("model %q: path = %q, want %q", model, path, "v1/messages")
		}
	}
}

func TestDiaMessagesRequestRelayedToEndpoint(t *testing.T) {
	var gotPath, gotAuth, gotVersion, gotUA string
	var gotBody map[string]any

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotVersion = r.Header.Get("Anthropic-Version")
		gotUA = r.Header.Get("User-Agent")

		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "msg_dia_test",
			"type": "message",
			"role": "assistant",
			"model": "claude-opus-4-8",
			"content": [{"type": "text", "text": "Hello from Dia!"}],
			"usage": {"input_tokens": 5, "output_tokens": 5}
		}`))
	}))
	defer up.Close()

	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.CreateConnection("dia", "dia-1", "bridge"); err != nil {
		t.Fatal(err)
	}

	h := New(s, map[string]string{"dia": up.URL})

	req := loopbackRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model": "dia/claude-opus-4-8",
		"max_tokens": 100,
		"messages": [{"role": "user", "content": "hi"}]
	}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if gotPath != "/v1/messages" {
		t.Errorf("upstream path = %q, want /v1/messages", gotPath)
	}
	// The dia entry has no AuthHeader: the bridge attaches the handler's own
	// per-turn credential, so ccw must send none of its own.
	if gotAuth != "" {
		t.Errorf("upstream auth = %q, want no credential from ccw", gotAuth)
	}
	if gotVersion != "2023-06-01" {
		t.Errorf("upstream Anthropic-Version = %q, want 2023-06-01", gotVersion)
	}
	if gotUA != provider.ClaudeInteractiveUserAgent {
		t.Errorf("upstream User-Agent = %q, want %q", gotUA, provider.ClaudeInteractiveUserAgent)
	}
	if gotBody["model"] != "claude-opus-4-8" {
		t.Errorf("upstream model = %v, want claude-opus-4-8", gotBody["model"])
	}
}

// Dia has no [1m] model: the suffix is Claude Code's annotation, so ccw cuts it
// off before the bridge, which would otherwise answer 400 on a colon-bearing
// model.
func TestDiaCutsTheOneMillionSuffixBeforeTheBridge(t *testing.T) {
	var gotPath string
	var gotBody map[string]any

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "msg_dia_test_1m",
			"type": "message",
			"role": "assistant",
			"model": "claude-opus-4-6",
			"content": [{"type": "text", "text": "Hello 1m"}],
			"usage": {"input_tokens": 5, "output_tokens": 5}
		}`))
	}))
	defer up.Close()

	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.CreateConnection("dia", "dia-1", "bridge"); err != nil {
		t.Fatal(err)
	}

	h := New(s, map[string]string{"dia": up.URL})

	req := loopbackRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model": "dia/claude-opus-4-6[1m]",
		"max_tokens": 100,
		"messages": [{"role": "user", "content": "hi"}]
	}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if gotPath != "/v1/messages" {
		t.Errorf("upstream path = %q, want /v1/messages", gotPath)
	}
	if gotBody["model"] != "claude-opus-4-6" {
		t.Errorf("upstream body model = %v, want claude-opus-4-6", gotBody["model"])
	}
}

func TestDiaBareModelRouting(t *testing.T) {
	var called bool

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "msg_dia_bare",
			"type": "message",
			"role": "assistant",
			"model": "claude-opus-4-8",
			"content": [{"type": "text", "text": "Bare routing ok"}],
			"usage": {"input_tokens": 5, "output_tokens": 5}
		}`))
	}))
	defer up.Close()

	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.CreateConnection("dia", "dia-1", "bridge"); err != nil {
		t.Fatal(err)
	}

	h := New(s, map[string]string{"dia": up.URL})

	req := loopbackRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model": "claude-opus-4-8",
		"max_tokens": 100,
		"messages": [{"role": "user", "content": "hi"}]
	}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !called {
		t.Error("expected upstream to be called for bare model claude-opus-4-8")
	}
}

func TestDiaModelCatalog(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test_cat.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.CreateConnection("dia", "dia-1", "bridge"); err != nil {
		t.Fatal(err)
	}

	h := New(s, nil)

	// Test GET /v1/models
	req := loopbackRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var listResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatal(err)
	}

	listed := map[string]bool{}
	for _, m := range listResp.Data {
		listed[m.ID] = true
	}
	// The two names Dia's Vertex backend whitelists for the dia-browser use
	// case. The others its agent specs mention answer HTTP 400, which makes Dia
	// abort the turn and kill the bridge.
	want := []string{
		"dia/claude-opus-4-8", "dia/claude-opus-4-6",
	}
	for _, id := range want {
		if !listed[id] {
			t.Errorf("GET /v1/models misses %s, got %v", id, listResp.Data)
		}
	}
	// Dia ships no [1m] model: the suffix belongs to Claude Code, and the
	// bridge rejects a colon before the handler ever sees the path.
	for _, m := range listResp.Data {
		if strings.Contains(m.ID, "[1m]") {
			t.Errorf("GET /v1/models lists %q, want no [1m] suffix on a Dia model", m.ID)
		}
	}

	// Test single model GET /v1/models/dia/claude-opus-4-8
	reqSingle := loopbackRequest(http.MethodGet, "/v1/models/dia/claude-opus-4-8", nil)
	recSingle := httptest.NewRecorder()
	h.ServeHTTP(recSingle, reqSingle)
	if recSingle.Code != http.StatusOK {
		t.Errorf("GET /v1/models/dia/claude-opus-4-8 status = %d", recSingle.Code)
	}

	// Test single model bare GET /v1/models/claude-opus-4-8
	reqBare := loopbackRequest(http.MethodGet, "/v1/models/claude-opus-4-8", nil)
	recBare := httptest.NewRecorder()
	h.ServeHTTP(recBare, reqBare)
	if recBare.Code != http.StatusOK {
		t.Errorf("GET /v1/models/claude-opus-4-8 status = %d", recBare.Code)
	}
}

func TestDiaFailoverAcrossAccounts(t *testing.T) {
	var tries int
	// The dia entry sends no credential: the bridge attaches the handler's own
	// per-turn token. So the account is told apart by the attempt, not by a
	// header, and the first account failing must still reach the second.
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tries++
		if tries == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": "rate limited"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "msg_dia_failover",
			"type": "message",
			"role": "assistant",
			"model": "claude-opus-4-8",
			"content": [{"type": "text", "text": "second account answered"}],
			"usage": {"input_tokens": 5, "output_tokens": 5}
		}`))
	}))
	defer up.Close()

	s, err := store.Open(filepath.Join(t.TempDir(), "test_fo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.CreateConnection("dia", "dia-acc1", "bridge"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateConnection("dia", "dia-acc2", "bridge"); err != nil {
		t.Fatal(err)
	}

	h := New(s, map[string]string{"dia": up.URL})

	req := loopbackRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{
		"model": "dia/claude-opus-4-8",
		"max_tokens": 50,
		"messages": [{"role": "user", "content": "hi"}]
	}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if tries < 2 {
		t.Errorf("tries = %d, want at least 2 for failover", tries)
	}
	if !strings.Contains(rec.Body.String(), "second account answered") {
		t.Errorf("body = %s, want second account answered", rec.Body.String())
	}
}

func TestDiaChatCompletionsTranslated(t *testing.T) {
	var gotPath string
	var gotBody map[string]any

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "msg_dia_translate",
			"type": "message",
			"role": "assistant",
			"model": "claude-opus-4-8",
			"content": [{"type": "text", "text": "Translated response ok"}],
			"usage": {"input_tokens": 10, "output_tokens": 10}
		}`))
	}))
	defer up.Close()

	s, err := store.Open(filepath.Join(t.TempDir(), "test_trans.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.CreateConnection("dia", "dia-1", "bridge"); err != nil {
		t.Fatal(err)
	}

	h := New(s, map[string]string{"dia": up.URL})

	req := loopbackRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model": "dia/claude-opus-4-8",
		"messages": [{"role": "user", "content": "hello from openai client"}]
	}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if gotPath != "/v1/messages" {
		t.Errorf("gotPath = %q, want /v1/messages", gotPath)
	}
	// Upstream should have received Anthropic shape
	if _, hasMaxTokens := gotBody["max_tokens"]; !hasMaxTokens {
		t.Errorf("upstream Anthropic request missing max_tokens: %v", gotBody)
	}

	// Caller should receive OpenAI Chat Completions shape
	var openAIResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &openAIResp); err != nil {
		t.Fatalf("decode openai response: %v", err)
	}
	if len(openAIResp.Choices) == 0 || openAIResp.Choices[0].Message.Content != "Translated response ok" {
		t.Errorf("unexpected openai response choices: %v", openAIResp.Choices)
	}
}
