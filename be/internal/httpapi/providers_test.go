package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/naniiluja/ccw/internal/store"
	"github.com/naniiluja/ccw/internal/zen"
)

// Claude Code always calls /v1/messages?beta=true. The query is the caller's,
// so it must not be appended to Antigravity's own ?alt=sse.
func TestAntigravityDropsTheCallersQueryOnATranslatedCall(t *testing.T) {
	var gotPath string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		switch {
		case strings.HasSuffix(r.URL.Path, ":loadCodeAssist"):
			w.Write([]byte(`{"cloudaicompanionProject":"proj-7"}`))
		default:
			gotPath = r.URL.Path + "?" + r.URL.RawQuery
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
	s.CreateConnection("antigravity", "ag", "fake-access-token")
	h := New(s, map[string]string{"antigravity": up.URL})

	req := loopbackRequest("POST", "/v1/messages?beta=true",
		strings.NewReader(`{"model":"antigravity/gemini-3-flash","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if gotPath != "/v1internal:streamGenerateContent?alt=sse" {
		t.Errorf("upstream = %s, want the provider's own query only", gotPath)
	}
}

func TestAntigravityWrapsAndTranslates(t *testing.T) {
	var gotPath, gotUA, gotBody string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		switch {
		case strings.HasSuffix(r.URL.Path, ":loadCodeAssist"):
			w.Write([]byte(`{"cloudaicompanionProject":"proj-7"}`))
		case strings.HasSuffix(r.URL.Path, ":fetchAvailableModels"):
			w.Write([]byte(`{"models":{"gemini-3-flash":{},"internal-x":{"isInternal":true}}}`))
		default:
			gotPath, gotUA, gotBody = r.URL.Path+"?"+r.URL.RawQuery, r.Header.Get("User-Agent"), string(b)
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, `data: {"response":{"responseId":"r","modelVersion":"gemini-3-flash","candidates":[{"content":{"parts":[{"text":"OK"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":1}}}`+"\n\n")
		}
	}))
	defer up.Close()
	old := antigravityProdURL
	antigravityProdURL = up.URL
	defer func() { antigravityProdURL = old }()

	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	c, _ := s.CreateConnection("antigravity", "ag", "fake-access-token")
	h := New(s, map[string]string{"antigravity": up.URL})

	rec := postV1(h, `{"model":"antigravity/gemini-3-flash","messages":[{"role":"system","content":"sys"},{"role":"user","content":"hi"}]}`)
	if b := rec.Body.String(); !strings.Contains(b, `"content":"OK"`) || !strings.Contains(b, `"object":"chat.completion"`) {
		t.Fatalf("caller got %s", b)
	}
	if gotPath != "/v1internal:streamGenerateContent?alt=sse" || !strings.HasPrefix(gotUA, "antigravity/ide/") {
		t.Errorf("upstream path=%s ua=%s", gotPath, gotUA)
	}
	for _, want := range []string{`"project":"proj-7"`, `"model":"gemini-3-flash"`, `"requestType":"agent"`, `"systemInstruction"`, `"sessionId":"-`} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("envelope missing %s: %s", want, gotBody)
		}
	}
	// The project is kept for the next call.
	list, _ := s.ListConnections()
	if list[0].Meta["projectId"] != "proj-7" {
		t.Errorf("meta = %v", list[0].Meta)
	}
	rows, _ := s.Usage()
	if len(rows) != 1 || rows[0].ConnectionID != c.ID || rows[0].InputTokens != 4 {
		t.Errorf("usage = %+v", rows)
	}
	mrec := httptest.NewRecorder()
	h.ServeHTTP(mrec, loopbackRequest("GET", "/v1/models", nil))
	if b := mrec.Body.String(); !strings.Contains(b, `"antigravity/gemini-3-flash"`) || strings.Contains(b, "internal-x") {
		t.Errorf("models = %s", b)
	}
}

func TestCopilotExchangesAndCachesTheToken(t *testing.T) {
	var exchanges int32
	ex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&exchanges, 1)
		if r.Header.Get("Authorization") != "token gho_abc" {
			t.Errorf("exchange auth = %q", r.Header.Get("Authorization"))
		}
		exp := time.Now().Add(30 * time.Minute).Unix()
		w.Write([]byte(`{"token":"cop-1","expires_at":` + itoa(exp) + `}`))
	}))
	defer ex.Close()
	old := copilotTokenURL
	copilotTokenURL = ex.URL
	defer func() { copilotTokenURL = old }()

	var gotAuth, gotIntegration, gotReqID, gotBody string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotAuth, gotIntegration, gotReqID, gotBody = r.Header.Get("Authorization"), r.Header.Get("Copilot-Integration-Id"), r.Header.Get("X-Request-Id"), string(b)
		w.Write([]byte(`{"ok":true,"copilot_extra":1}`))
	}))
	defer up.Close()
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	s.CreateConnection("github", "gh", "gho_abc")
	h := New(s, map[string]string{"github": up.URL})

	for i := 0; i < 2; i++ {
		rec := postV1(h, `{"model":"github/gpt-5.4","max_tokens":10,"messages":[]}`)
		if rec.Code != http.StatusOK || rec.Body.String() != `{"ok":true,"copilot_extra":1}` {
			t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
		}
	}
	if gotAuth != "Bearer cop-1" || gotIntegration != "vscode-chat" || len(gotReqID) != 36 {
		t.Errorf("auth=%q integration=%q request id=%q", gotAuth, gotIntegration, gotReqID)
	}
	if !strings.Contains(gotBody, `"max_completion_tokens":10`) || strings.Contains(gotBody, `"max_tokens"`) {
		t.Errorf("body = %s, want max_tokens renamed for gpt-5", gotBody)
	}
	if n := atomic.LoadInt32(&exchanges); n != 1 {
		t.Errorf("exchanges = %d, want 1 (cached)", n)
	}
}

func itoa(n int64) string {
	b := []byte{}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestCopilotRoutesClaudeToMessagesAndLearnsResponses(t *testing.T) {
	copilotResponsesModels.Delete("gpt-5-mini")
	ex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"token":"cop","expires_at":` + itoa(time.Now().Add(time.Hour).Unix()) + `}`))
	}))
	defer ex.Close()
	old := copilotTokenURL
	copilotTokenURL = ex.URL
	defer func() { copilotTokenURL = old }()
	paths := []string{}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/v1/messages":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"msg_1","type":"message","content":[{"type":"text","text":"from claude"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
		case "/chat/completions":
			w.WriteHeader(400)
			w.Write([]byte(`{"error":{"message":"The requested model is not supported.","code":"model_not_supported"}}`))
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

	if rec := postV1(h, `{"model":"github/claude-haiku-4.5","messages":[{"role":"user","content":"hi"}]}`); !strings.Contains(rec.Body.String(), `"content":"from claude"`) {
		t.Errorf("claude via messages: %s", rec.Body.String())
	}
	if rec := postV1(h, `{"model":"github/gpt-5-mini","messages":[{"role":"user","content":"hi"}]}`); !strings.Contains(rec.Body.String(), `"content":"from responses"`) {
		t.Errorf("gpt-5 via responses: %s", rec.Body.String())
	}
	postV1(h, `{"model":"github/gpt-5-mini","messages":[{"role":"user","content":"hi"}]}`)
	want := "/v1/messages,/chat/completions,/responses,/responses"
	if strings.Join(paths, ",") != want {
		t.Errorf("paths = %v, want %s (the second gpt-5 call goes straight to /responses)", paths, want)
	}
}

// zenUpstream stands in for opencode.ai/zen: it checks what the free tier's gate
// checks, keeps every call it got, and answers in the habits the real one has.
type zenUpstream struct {
	*httptest.Server
	mu    sync.Mutex
	calls []zenCall
	// status, when set, answers every call with it and the body instead.
	status int
	body   string
}

type zenCall struct {
	path   string
	header http.Header
	body   map[string]any
	raw    string
}

const zenChatStream = `data: {"id":"abc123","object":"chat.completion.chunk","created":100,"model":"upstream","choices":[{"index":0,"delta":{"role":"assistant","content":"Hel","name":"Space"},"finish_reason":null}],"usage":null}

data: {"choices":[{"index":0,"delta":{"reasoning":"hm","reasoning_details":[{"index":0,"text":"h"}]}}],"usage":null}

data: {"choices":[{"index":0,"delta":{"content":"lo"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5,"cost":"0"}}

data: [DONE]

data: {"choices":[],"cost":"0"}

`

const zenResponsesStream = `data: {"type":"response.created","response":{"id":"resp_1","created_at":5,"model":"muse"}}

data: {"type":"response.output_text.delta","delta":"ok"}

data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":4,"output_tokens":2,"total_tokens":6}}}

`

func newZenUpstream(t *testing.T) *zenUpstream {
	t.Helper()
	u := &zenUpstream{}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		json.Unmarshal(raw, &body)
		u.mu.Lock()
		u.calls = append(u.calls, zenCall{path: r.URL.Path, header: r.Header.Clone(), body: body, raw: string(raw)})
		status, canned := u.status, u.body
		u.mu.Unlock()
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"object":"list","data":[{"id":"big-pickle"},{"id":"paid-model"},{"id":"muse-spark-1.3-contributor-free"},{"id":"jev-1.13-free"}]}`)
			return
		}
		if status != 0 {
			w.WriteHeader(status)
			io.WriteString(w, canned)
			return
		}
		switch r.URL.Path {
		case "/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, zenChatStream)
		case "/responses":
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, zenResponsesStream)
		case "/systemone":
			io.WriteString(w, `{"model":"jev-1.13-free","cost":"0","answers":{"q":{"type":"choice","choice":"a","confidence":0.9,"trace":1}},"usage":{"input_tokens":3,"output_tokens":1,"cost":1}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(u.Close)
	return u
}

func (u *zenUpstream) last(t *testing.T) zenCall {
	t.Helper()
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.calls) == 0 {
		t.Fatal("the upstream got no call")
	}
	return u.calls[len(u.calls)-1]
}

// zenRig is ccw with one Zen account against a stand-in upstream.
func zenRig(t *testing.T) (http.Handler, *zenUpstream, *api) {
	t.Helper()
	up := newZenUpstream(t)
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.CreateConnection("opencode", "zen", "public"); err != nil {
		t.Fatal(err)
	}
	a, h := newServer(s, map[string]string{"opencode": up.URL}, nil)
	// models.dev stand-in: which model reasons, and which speaks Responses.
	cat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"opencode":{"models":{
		 "big-pickle":{"reasoning":true,"cost":{"input":0,"output":0},"limit":{"context":200000,"output":64000}},
		 "muse-spark-1.3-contributor-free":{"reasoning":true,"provider":{"npm":"@ai-sdk/openai"}}}}}`)
	}))
	t.Cleanup(cat.Close)
	a.zen.cat = &zen.Source{URL: cat.URL, TTL: 1 << 40}
	return h, up, a
}

func zenPost(h http.Handler, path, body string, kv ...string) *httptest.ResponseRecorder {
	req := loopbackRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(kv); i += 2 {
		req.Header.Set(kv[i], kv[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func toolNamesOf(body map[string]any) []string {
	var out []string
	for _, tl := range body["tools"].([]any) {
		m := tl.(map[string]any)
		if n, ok := m["name"].(string); ok {
			out = append(out, n)
		} else if fn, ok := m["function"].(map[string]any); ok {
			out = append(out, fn["name"].(string))
		}
	}
	return out
}

func TestZenCallCarriesWhatTheGateReads(t *testing.T) {
	h, up, _ := zenRig(t)
	rec := zenPost(h, "/v1/chat/completions", `{"model":"opencode/big-pickle","messages":[{"role":"user","content":"hi"}],"stream":false}`,
		"User-Agent", "python-httpx/0.27", "Anthropic-Beta", "x", "X-Stainless-Lang", "js", "Originator", "codex_cli_rs")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	c := up.last(t)
	if c.path != "/chat/completions" {
		t.Errorf("path = %s", c.path)
	}
	if ua := c.header.Get("User-Agent"); !strings.HasPrefix(ua, "opencode/") {
		t.Errorf("User-Agent = %q, the gate wants opencode/", ua)
	}
	sess := c.header.Get("X-Opencode-Session")
	if !zen.IsSessionID(sess) || c.header.Get("X-Session-Id") != sess || c.header.Get("X-Session-Affinity") != sess {
		t.Errorf("session headers = %q %q %q", sess, c.header.Get("X-Session-Id"), c.header.Get("X-Session-Affinity"))
	}
	if got := c.header.Get("Authorization"); got != "Bearer public" {
		t.Errorf("Authorization = %q, want the free tier's public key", got)
	}
	for _, leaked := range []string{"Anthropic-Beta", "X-Stainless-Lang", "Originator"} {
		if c.header.Get(leaked) != "" {
			t.Errorf("the caller's %s reached the upstream", leaked)
		}
	}
	if c.body["stream"] != true || strings.Join(toolNamesOf(c.body), ",") != "read,shell" || c.body["model"] != "big-pickle" {
		t.Errorf("body = %s", c.raw)
	}
}

func TestZenWholeAnswerLeavesInTheOpenAIShape(t *testing.T) {
	h, _, _ := zenRig(t)
	rec := zenPost(h, "/v1/chat/completions", `{"model":"opencode/big-pickle","messages":[{"role":"user","content":"hi"}]}`)
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, rec.Body.String())
	}
	msg := got["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "Hello" || msg["reasoning"] != "hm" || msg["reasoning_content"] != "hm" || got["choices"].([]any)[0].(map[string]any)["finish_reason"] != "stop" {
		t.Errorf("answer = %s", rec.Body.String())
	}
	if got["model"] != "opencode/big-pickle" {
		t.Errorf("model = %v, want the name the caller sent", got["model"])
	}
	if strings.Contains(rec.Body.String(), `"cost"`) || strings.Contains(rec.Body.String(), `"name"`) {
		t.Errorf("an upstream-only field reached the caller: %s", rec.Body.String())
	}
	if got["usage"] == nil {
		t.Error("usage was lost")
	}
}

func TestZenStreamedAnswerLeavesInTheOpenAIShape(t *testing.T) {
	h, _, _ := zenRig(t)
	rec := zenPost(h, "/v1/chat/completions", `{"model":"opencode/big-pickle","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	body := rec.Body.String()
	if !strings.Contains(rec.Header().Get("Content-Type"), "event-stream") || strings.Count(body, "[DONE]") != 1 || strings.HasSuffix(strings.TrimSpace(body), `"cost":"0"}`) {
		t.Errorf("stream = %s", body)
	}
	for _, want := range []string{`"content":"Hel"`, `"content":"lo"`, `"reasoning_content":"hm"`, `"finish_reason":"stop"`, `chatcmpl-abc123`} {
		if !strings.Contains(body, want) {
			t.Errorf("stream misses %s:\n%s", want, body)
		}
	}
}

// Claude Code names its session in a header and in metadata.user_id, and puts a
// device id in it: one conversation must keep one upstream session, and none of
// it may reach the upstream.
func TestZenKeepsOneSessionPerCallerAndSendsNoneOfItsIdentity(t *testing.T) {
	h, up, _ := zenRig(t)
	turn := func(session, extra string) string {
		zenPost(h, "/v1/messages?beta=true", `{"model":"opencode/big-pickle","max_tokens":50,`+extra+`"metadata":{"user_id":"{\"device_id\":\"dev-9\",\"session_id\":\"`+session+`\"}"},"messages":[{"role":"user","content":"hi"}]}`,
			"X-Claude-Code-Session-Id", session)
		return up.last(t).header.Get("X-Opencode-Session")
	}
	first := turn("conv-1", "")
	again := turn("conv-1", `"system":"more",`)
	other := turn("conv-2", "")
	if first != again || first == other || !zen.IsSessionID(first) {
		t.Errorf("sessions: first=%s again=%s other=%s", first, again, other)
	}
	last := up.last(t)
	if strings.Contains(last.raw, "dev-9") || strings.Contains(last.raw, "metadata") || last.body["user"] != nil {
		t.Errorf("the caller's identity reached the upstream: %s", last.raw)
	}
	if last.path != "/chat/completions" {
		t.Errorf("path = %s: the ?beta=true of Claude Code must not matter", last.path)
	}
}

func TestZenAnthropicCallerGetsAMessagesAnswer(t *testing.T) {
	h, _, _ := zenRig(t)
	rec := zenPost(h, "/v1/messages", `{"model":"opencode/big-pickle","max_tokens":50,"messages":[{"role":"user","content":"hi"}]}`)
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got["type"] != "message" || !strings.Contains(rec.Body.String(), "Hello") {
		t.Errorf("answer = %s (%v)", rec.Body.String(), err)
	}
}

func TestZenResponsesModelIsCalledOnResponses(t *testing.T) {
	h, up, _ := zenRig(t)
	rec := zenPost(h, "/v1/chat/completions", `{"model":"opencode/muse-spark-1.3-contributor-free","tool_choice":"required",
	 "messages":[{"role":"system","content":"be brief"},{"role":"user","content":"hi"}],
	 "tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"content":"ok"`) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	c := up.last(t)
	sess := c.header.Get("X-Opencode-Session")
	if c.path != "/responses" || c.body["stream"] != true || c.body["store"] != false || c.body["prompt_cache_key"] != sess || !zen.IsSessionID(sess) {
		t.Errorf("responses call: path=%s body=%s", c.path, c.raw)
	}
	if c.body["tool_choice"] != "auto" {
		t.Errorf("tool_choice = %v, the muse models take only auto", c.body["tool_choice"])
	}
	if strings.Join(toolNamesOf(c.body), ",") != "lookup,read,shell" {
		t.Errorf("tools = %v", toolNamesOf(c.body))
	}
	if c.body["instructions"] != "be brief" {
		t.Errorf("instructions = %v", c.body["instructions"])
	}
}

func TestZenSystemOneKeepsItsShape(t *testing.T) {
	h, up, _ := zenRig(t)
	rec := zenPost(h, "/v1/systemone", `{"model":"opencode/jev-latest","stream":true,"tools":[{}],"state":{"a":1},"questions":{"q":{"type":"choice"}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	c := up.last(t)
	if c.path != "/systemone" || c.body["model"] != "jev-1.13-free" || c.body["stream"] != nil || c.body["tools"] != nil || c.body["state"] == nil {
		t.Errorf("system one call: path=%s body=%s", c.path, c.raw)
	}
	if strings.Contains(rec.Body.String(), "cost") || strings.Contains(rec.Body.String(), "trace") || !strings.Contains(rec.Body.String(), `"choice":"a"`) {
		t.Errorf("answer = %s", rec.Body.String())
	}
}

func TestZenChatCallOfASystemOneModelIsRefusedByName(t *testing.T) {
	h, _, _ := zenRig(t)
	rec := zenPost(h, "/v1/chat/completions", `{"model":"opencode/jev-latest","messages":[{"role":"user","content":"hi"}]}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "System One") {
		t.Errorf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestZenUnknownModelIsA404NotABadKey(t *testing.T) {
	h, up, _ := zenRig(t)
	up.mu.Lock()
	up.status, up.body = 401, `{"error":{"type":"ModelError","message":"Model nope is not supported"}}`
	up.mu.Unlock()
	rec := zenPost(h, "/v1/chat/completions", `{"model":"opencode/nope","messages":[{"role":"user","content":"hi"}]}`)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "model_not_found") {
		t.Errorf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestZenModelListHoldsFreeChatModelsWithTheirLimits(t *testing.T) {
	h, _, _ := zenRig(t)
	req := loopbackRequest("GET", "/v1/models", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, `"opencode/big-pickle"`) || !strings.Contains(body, `"opencode/muse-spark-1.3-contributor-free"`) {
		t.Errorf("free models missing: %s", body)
	}
	if strings.Contains(body, "paid-model") || strings.Contains(body, "jev-1.13-free") {
		t.Errorf("a paid or System One model is listed: %s", body)
	}
	if !strings.Contains(body, `"context_length":200000`) || !strings.Contains(body, `"max_output_tokens":64000`) {
		t.Errorf("limits from models.dev missing: %s", body)
	}
}

func TestAZenAccountNeedsNoKey(t *testing.T) {
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	_, h := newServer(s, nil, nil)
	req := loopbackRequest("POST", "/accounts", strings.NewReader("provider=opencode&label=zen"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	list, _ := s.ListConnections()
	if len(list) != 1 || list[0].Provider != "opencode" {
		t.Errorf("connections = %+v", list)
	}
}

// The requests zencore, the Python gateway this provider replaces, sent to the
// upstream for the same inputs, recorded on 2026-09-30 by pointing it at a
// recorder (session and project ids masked). ccw must send the same: what the
// free tier's gate reads is an equivalence that a test has to hold.
func TestZenRequestsMatchTheGatewayItReplaces(t *testing.T) {
	raw, err := os.ReadFile("testdata/zen/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases map[string]struct {
		Path string
		Body map[string]any
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for name, c := range cases {
		raw, err := os.ReadFile("testdata/zen/" + name + ".golden.json")
		if err != nil {
			t.Fatal(err)
		}
		var golden struct {
			Path    string
			Headers map[string]string
			Body    map[string]any
		}
		if err := json.Unmarshal(raw, &golden); err != nil {
			t.Fatal(err)
		}
		// One difference is on purpose: zencore forwarded the caller's user field,
		// which for Claude Code holds a device id. ccw does not.
		delete(golden.Body, "user")
		h, up, _ := zenRig(t)
		// The caller names the model as ccw's callers do: with the provider.
		in := map[string]any{}
		for k, v := range c.Body {
			in[k] = v
		}
		in["model"] = "opencode/" + c.Body["model"].(string)
		body, _ := json.Marshal(in)
		if rec := zenPost(h, c.Path, string(body), "User-Agent", "python-test", "X-Claude-Code-Session-Id", "conv-A"); rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, body = %s", name, rec.Code, rec.Body.String())
		}
		got := up.last(t)
		if got.path != golden.Path {
			t.Errorf("%s: path = %s, zencore sent %s", name, got.path, golden.Path)
		}
		for k, want := range golden.Headers {
			if have := got.header.Get(k); have != want {
				t.Errorf("%s: header %s = %q, zencore sent %q", name, k, have, want)
			}
		}
		if key, ok := got.body["prompt_cache_key"]; ok {
			if key != got.header.Get("X-Opencode-Session") {
				t.Errorf("%s: prompt_cache_key %v is not the session header", name, key)
			}
			got.body["prompt_cache_key"] = "<session>"
		}
		if !reflect.DeepEqual(got.body, golden.Body) {
			g1, _ := json.MarshalIndent(got.body, "", " ")
			g2, _ := json.MarshalIndent(golden.Body, "", " ")
			t.Errorf("%s: body differs from what zencore sent.\n--- ccw\n%s\n--- zencore\n%s", name, g1, g2)
		}
	}
}

func TestZenSessionsListsWhoHoldsWhat(t *testing.T) {
	h, _, _ := zenRig(t)
	zenPost(h, "/v1/chat/completions", `{"model":"opencode/big-pickle","messages":[{"role":"user","content":"hi"}]}`, "X-Session-Id", "caller-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, loopbackRequest("GET", "/api/zen/sessions", nil))
	var got struct {
		Count    int
		Sessions []struct{ ID, Caller, Source string }
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Count != 1 {
		t.Fatalf("%v: %s", err, rec.Body.String())
	}
	if s := got.Sessions[0]; s.Caller != "caller-1" || s.Source != "header:x-session-id" || !zen.IsSessionID(s.ID) {
		t.Errorf("session = %+v", s)
	}
}

// The drift and error reviews settle their changes with Jev, a System One model.
// It reaches them through Zen as well as through TypeSafe, so the review must
// take opencode/jev-latest as its decision model, and keep Zen's chat models for
// the role of resolver.
func TestZenJevCanBeTheReviewsDecisionModel(t *testing.T) {
	_, up, a := zenRig(t)
	for model, want := range map[string]bool{"opencode/jev-latest": true, "opencode/jev-1.13-free": true, "opencode/big-pickle": false, "groq/llama": false, "jev-latest": false} {
		err := a.checkReviewConfig(ReviewConfig{DecisionModel: model, AckConfidence: 0.8})
		if (err == nil) != want {
			t.Errorf("decision model %s: err = %v, want accepted = %v", model, err, want)
		}
	}
	for model, want := range map[string]bool{"opencode/big-pickle": true, "opencode/jev-latest": false} {
		if err := a.checkReviewConfig(ReviewConfig{DecisionModel: "opencode/jev-latest", ResolverModel: model, AckConfidence: 0.8}); (err == nil) != want {
			t.Errorf("resolver model %s: err = %v, want accepted = %v", model, err, want)
		}
		if err := a.checkErrReviewConfig(ErrorReviewConfig{Model: model, MinErrors: 3}); (err == nil) != want {
			t.Errorf("error review model %s: err = %v, want accepted = %v", model, err, want)
		}
	}
	// And the review's own call reaches Zen as a System One call would.
	answers, err := a.askJev(context.Background(), "opencode/jev-latest", map[string]any{"facts": 1},
		map[string]any{"q": map[string]any{"type": "choice", "instructions": "pick", "criteria": map[string]string{"a": "x", "b": "y"}}})
	if err != nil || answers["q"].Choice != "a" {
		t.Fatalf("answers = %+v, err = %v", answers, err)
	}
	if c := up.last(t); c.path != "/systemone" || c.body["model"] != "jev-1.13-free" {
		t.Errorf("the review's call: path=%s body=%s", c.path, c.raw)
	}
}

// An account stored with no key (as it was before the provider carried a public
// one) must still send it.
func TestZenAccountStoredWithNoKeyStillSendsThePublicOne(t *testing.T) {
	up := newZenUpstream(t)
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	s.CreateConnection("opencode", "old", "")
	h := New(s, map[string]string{"opencode": up.URL})
	if rec := zenPost(h, "/v1/chat/completions", `{"model":"opencode/big-pickle","messages":[{"role":"user","content":"hi"}]}`); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := up.last(t).header.Get("Authorization"); got != "Bearer public" {
		t.Errorf("Authorization = %q, want Bearer public", got)
	}
}

// The dashboard offers a provider from GET /providers and adds an account from
// the form of its setup kind. OpenCode Zen is one that needs no key.
func TestZenIsOfferedByTheDashboardAsAProviderThatNeedsNoKey(t *testing.T) {
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	rec := httptest.NewRecorder()
	New(s, nil).ServeHTTP(rec, loopbackRequest("GET", "/providers", nil))
	var got struct {
		Providers []struct{ ID, Setup, Auth string }
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range got.Providers {
		if p.ID == "opencode" {
			found = true
			if p.Setup != "none" || p.Auth != "apikey" {
				t.Errorf("opencode = %+v, want setup none and an API provider", p)
			}
		}
	}
	if !found {
		t.Fatalf("opencode is not offered: %s", rec.Body.String())
	}
}
