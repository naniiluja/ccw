package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/naniiluja/ccw/internal/store"
	"github.com/naniiluja/ccw/internal/websearch"
)

// scriptedModel is an OpenAI-compatible upstream that answers each call with the
// next of its replies, and keeps what it was sent.
type scriptedModel struct {
	*httptest.Server
	mu      sync.Mutex
	replies []string
	bodies  []map[string]any
	status  int
}

func newScriptedModel(t *testing.T, replies ...string) *scriptedModel {
	m := &scriptedModel{replies: replies}
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		json.Unmarshal(b, &body)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.bodies = append(m.bodies, body)
		if m.status != 0 {
			w.WriteHeader(m.status)
			io.WriteString(w, `{"error":{"message":"upstream down"}}`)
			return
		}
		i := len(m.bodies) - 1
		if i >= len(m.replies) {
			i = len(m.replies) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, m.replies[i])
	}))
	t.Cleanup(m.Close)
	return m
}

func chatToolCall(id, name, args string) string {
	b, _ := json.Marshal(args)
	return `{"id":"c1","object":"chat.completion","model":"llama","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"id":"` +
		id + `","type":"function","function":{"name":"` + name + `","arguments":` + string(b) + `}}]}}],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`
}

func chatText(text string) string {
	return `{"id":"c2","object":"chat.completion","model":"llama","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"` +
		text + `"}}],"usage":{"prompt_tokens":9,"completion_tokens":2,"total_tokens":11}}`
}

func echoMCP(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     *int   `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name string         `json:"name"`
				Args map[string]any `json:"arguments"`
			} `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-06-18"}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "echo", "description": "Echo text",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}}}}}
		case "tools/call":
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "echo: " + req.Params.Args["text"].(string)}}}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": result})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func serverToolsRig(t *testing.T, up *scriptedModel) http.Handler {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.CreateConnection("groq", "g", "k"); err != nil {
		t.Fatal(err)
	}
	_, h := newServer(s, map[string]string{"groq": up.URL}, nil)
	return h
}

func mcpRequest(mcpURL, stream string) string {
	return `{"model":"groq/llama","max_tokens":100,"stream":` + stream + `,
	 "mcp_servers":[{"type":"url","url":"` + mcpURL + `","name":"demo"}],
	 "tools":[{"type":"mcp_toolset","mcp_server_name":"demo"},{"name":"Read","description":"r","input_schema":{"type":"object"}}],
	 "messages":[{"role":"user","content":"say hi with the echo tool"}]}`
}

func TestTheMCPConnectorIsRunByCcw(t *testing.T) {
	mcp := echoMCP(t)
	up := newScriptedModel(t, chatToolCall("call_1", "echo", `{"text":"hi"}`), chatText("It said hi."))
	h := serverToolsRig(t, up)

	rec := zenPost(h, "/v1/messages", mcpRequest(mcp.URL, "false"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Content    []map[string]any
		StopReason string `json:"stop_reason"`
		Usage      map[string]any
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, rec.Body.String())
	}
	if len(got.Content) != 3 || got.Content[0]["type"] != "mcp_tool_use" || got.Content[1]["type"] != "mcp_tool_result" || got.Content[2]["type"] != "text" {
		t.Fatalf("content = %v", got.Content)
	}
	if got.Content[0]["server_name"] != "demo" || got.Content[0]["name"] != "echo" {
		t.Errorf("mcp_tool_use = %v", got.Content[0])
	}
	res, _ := got.Content[1]["content"].([]any)
	if len(res) != 1 || res[0].(map[string]any)["text"] != "echo: hi" || got.Content[1]["tool_use_id"] != got.Content[0]["id"] {
		t.Errorf("mcp_tool_result = %v", got.Content[1])
	}
	if got.Content[2]["text"] != "It said hi." || got.StopReason != "end_turn" {
		t.Errorf("answer = %v, stop = %s", got.Content[2], got.StopReason)
	}
	if in, out := got.Usage["input_tokens"], got.Usage["output_tokens"]; in != float64(16) || out != float64(5) {
		t.Errorf("usage must add up both calls: %v", got.Usage)
	}

	if len(up.bodies) != 2 {
		t.Fatalf("the model was called %d times, want 2", len(up.bodies))
	}
	names := toolNamesOf(up.bodies[0])
	if strings.Join(names, ",") != "Read,echo" {
		t.Errorf("tools the model saw = %v", names)
	}
	if up.bodies[0]["mcp_servers"] != nil || up.bodies[0]["stream"] == true {
		t.Errorf("the first call = %v", up.bodies[0])
	}
	second, _ := json.Marshal(up.bodies[1]["messages"])
	if !strings.Contains(string(second), "echo: hi") {
		t.Errorf("the model was not given the tool's result: %s", second)
	}
}

func TestTheMCPConnectorAnswersAStreamWhenAsked(t *testing.T) {
	mcp := echoMCP(t)
	up := newScriptedModel(t, chatToolCall("call_1", "echo", `{"text":"hi"}`), chatText("Done."))
	rec := zenPost(serverToolsRig(t, up), "/v1/messages", mcpRequest(mcp.URL, "true"))
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, body = %s", ct, rec.Body.String())
	}
	body := rec.Body.String()
	i, j, k := strings.Index(body, `"type":"mcp_tool_use"`), strings.Index(body, `"type":"mcp_tool_result"`), strings.Index(body, `"text":"Done."`)
	if i < 0 || j < i || k < j || !strings.HasSuffix(strings.TrimSpace(body), `{"type":"message_stop"}`) {
		t.Errorf("events out of order (call %d, result %d, text %d):\n%s", i, j, k, body)
	}
}

func TestAnUnreachableMCPServerIsTheCallersError(t *testing.T) {
	up := newScriptedModel(t, chatText("x"))
	rec := zenPost(serverToolsRig(t, up), "/v1/messages", mcpRequest("http://127.0.0.1:1/mcp", "false"))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `cannot reach MCP server`) {
		t.Errorf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(up.bodies) != 0 {
		t.Error("the model was called though a server it needs was down")
	}
}

func TestAnUpstreamErrorDuringTheLoopReachesTheCaller(t *testing.T) {
	mcp := echoMCP(t)
	up := newScriptedModel(t, chatToolCall("call_1", "echo", `{"text":"hi"}`))
	up.status = http.StatusInternalServerError
	rec := zenPost(serverToolsRig(t, up), "/v1/messages", mcpRequest(mcp.URL, "false"))
	if rec.Code < 500 || !strings.Contains(rec.Body.String(), "upstream down") {
		t.Errorf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestToolSearchKeepsDeferredToolsOutUntilFound(t *testing.T) {
	up := newScriptedModel(t, chatToolCall("call_1", "tool_search_tool_regex", `{"pattern":"weather"}`), chatText("ok"))
	h := serverToolsRig(t, up)
	rec := zenPost(h, "/v1/messages", `{"model":"groq/llama","max_tokens":100,
	 "tools":[{"type":"tool_search_tool_regex_20251119","name":"tool_search_tool_regex"},
	  {"name":"get_weather","description":"Get the weather","input_schema":{"type":"object"},"defer_loading":true},
	  {"name":"send_email","description":"Send an email","input_schema":{"type":"object"},"defer_loading":true}],
	 "messages":[{"role":"user","content":"weather in Hue?"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if first := toolNamesOf(up.bodies[0]); strings.Join(first, ",") != "tool_search_tool_regex" {
		t.Errorf("the first call offered %v", first)
	}
	if second := toolNamesOf(up.bodies[1]); strings.Join(second, ",") != "tool_search_tool_regex,get_weather" {
		t.Errorf("after the search the model was offered %v", second)
	}
	var got struct{ Content []map[string]any }
	json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Content) < 3 || got.Content[0]["type"] != "server_tool_use" || got.Content[1]["type"] != "tool_search_tool_result" {
		t.Errorf("content = %v", got.Content)
	}
}

func TestAClientToolDeclaredByTypeGetsItsSchema(t *testing.T) {
	up := newScriptedModel(t, chatText("ok"))
	rec := zenPost(serverToolsRig(t, up), "/v1/messages", `{"model":"groq/llama","max_tokens":100,
	 "tools":[{"type":"bash_20250124","name":"bash"},{"type":"memory_20250818","name":"memory"},{"type":"advisor_20260301","name":"advisor"}],
	 "messages":[{"role":"user","content":"hi"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if names := toolNamesOf(up.bodies[0]); strings.Join(names, ",") != "bash,memory" {
		t.Errorf("tools the model saw = %v", names)
	}
	raw, _ := json.Marshal(up.bodies[0]["tools"])
	if !strings.Contains(string(raw), `"command"`) || !strings.Contains(string(raw), `"restart"`) {
		t.Errorf("bash has no schema: %s", raw)
	}
}

func TestASearchAndAFetchTogether(t *testing.T) {
	up := newScriptedModel(t, chatToolCall("call_1", "web_fetch", `{"url":"http://127.0.0.1:9/found"}`), chatText("done"))
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.CreateConnection("groq", "g", "k")
	a, h := newServer(s, map[string]string{"groq": up.URL}, nil)
	a.web.s = &stubSearcher{res: []websearch.Result{{Title: "Found", URL: "http://127.0.0.1:9/found", Snippet: "the release notes"}}}

	rec := zenPost(h, "/v1/messages", `{"model":"groq/llama","max_tokens":100,
	 "tools":[{"type":"web_search_20250305","name":"web_search"},{"type":"web_fetch_20250910","name":"web_fetch"}],
	 "messages":[{"role":"user","content":[{"type":"text","text":"Perform a web search for the query: go notes"}]}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Content []map[string]any
		Usage   map[string]any
	}
	json.Unmarshal(rec.Body.Bytes(), &got)
	var kinds []string
	for _, b := range got.Content {
		kinds = append(kinds, b["type"].(string))
	}
	if strings.Join(kinds, ",") != "server_tool_use,web_search_tool_result,server_tool_use,web_fetch_tool_result,text" {
		t.Fatalf("blocks = %v", kinds)
	}
	// The URL came only from the search: the fetch got past the conversation rule,
	// and stopped at the address rule (this machine).
	if e, _ := got.Content[3]["content"].(map[string]any); e["error_code"] != "url_not_allowed" {
		t.Errorf("fetch result = %v", got.Content[3])
	}
	// The model still had the search results on the second call.
	second, _ := json.Marshal(up.bodies[1]["messages"])
	if len(up.bodies) != 2 || !strings.Contains(mustString(up.bodies[1]), "the release notes") {
		t.Errorf("turn 2 lost the search results (%d calls): %s", len(up.bodies), second)
	}
	if names := toolNamesOf(up.bodies[0]); strings.Join(names, ",") != "web_fetch" {
		t.Errorf("the model saw tools %v", names)
	}
	if su, _ := got.Usage["server_tool_use"].(map[string]any); su["web_search_requests"] != float64(1) || su["web_fetch_requests"] != float64(1) {
		t.Errorf("usage = %v", got.Usage)
	}
}

func mustString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
