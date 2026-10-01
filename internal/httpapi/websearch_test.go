package httpapi

import (
	"context"
	"encoding/json"
	"errors"
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

type stubSearcher struct {
	mu      sync.Mutex
	queries []string
	res     []websearch.Result
	err     error
}

func (s *stubSearcher) Search(_ context.Context, q string, _ int) ([]websearch.Result, error) {
	s.mu.Lock()
	s.queries = append(s.queries, q)
	s.mu.Unlock()
	return s.res, s.err
}

// searchRig is ccw with a Zen account (a provider that is not Anthropic) and a
// stand-in search service.
func searchRig(t *testing.T, s *stubSearcher) (http.Handler, *zenUpstream) {
	t.Helper()
	h, up, a := zenRig(t)
	if s != nil {
		a.web.s = s
	}
	return h, up
}

// The request Claude Code's WebSearch tool sends.
const hostedSearchRequest = `{"model":"opencode/big-pickle","max_tokens":4000,"stream":%s,
 "system":[{"type":"text","text":"You are an assistant for performing a web search tool use"}],
 "tools":[{"type":"web_search_20250305","name":"web_search","max_uses":8}],
 "messages":[{"role":"user","content":[{"type":"text","text":"Perform a web search for the query: go 1.27 release"}]}]}`

func TestAHostedSearchIsRunByCcwAndItsResultsReachBothTheModelAndTheClient(t *testing.T) {
	stub := &stubSearcher{res: []websearch.Result{{Title: "Go 1.27", URL: "https://go.dev/doc/go1.27", Snippet: "Go 1.27 release notes", Age: "3 days ago"}}}
	h, up := searchRig(t, stub)
	rec := zenPost(h, "/v1/messages", strings.Replace(hostedSearchRequest, "%s", "false", 1))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(stub.queries) != 1 || stub.queries[0] != "go 1.27 release" {
		t.Errorf("queries = %v", stub.queries)
	}
	// The model got the results, and not the hosted tool it cannot use.
	c := up.last(t)
	// The upstream speaks Chat Completions, so the system prompt is a message.
	if !strings.Contains(c.raw, "Go 1.27 release notes") || !strings.Contains(c.raw, "https://go.dev/doc/go1.27") {
		t.Errorf("the results did not reach the model: %s", c.raw)
	}
	if strings.Contains(c.raw, "web_search") && strings.Contains(strings.Join(toolNamesOf(c.body), ","), "web_search") {
		t.Errorf("the hosted tool reached the upstream: %v", toolNamesOf(c.body))
	}
	// The client got the blocks Anthropic would have written, then the answer.
	var got struct {
		Content []map[string]any
		Usage   map[string]any
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got.Content) < 3 {
		t.Fatalf("%v: %s", err, rec.Body.String())
	}
	if got.Content[0]["type"] != "server_tool_use" || got.Content[1]["type"] != "web_search_tool_result" || got.Content[2]["type"] != "text" {
		t.Errorf("content types = %v %v %v", got.Content[0]["type"], got.Content[1]["type"], got.Content[2]["type"])
	}
	items, _ := got.Content[1]["content"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["url"] != "https://go.dev/doc/go1.27" {
		t.Errorf("result block = %v", got.Content[1])
	}
	if got.Content[1]["tool_use_id"] != got.Content[0]["id"] {
		t.Error("the result does not name the call")
	}
	if got.Usage["server_tool_use"] == nil {
		t.Errorf("usage = %v", got.Usage)
	}
}

func TestAHostedSearchStreamsItsBlocksBeforeTheAnswer(t *testing.T) {
	h, _ := searchRig(t, &stubSearcher{res: []websearch.Result{{Title: "T", URL: "https://u"}}})
	rec := zenPost(h, "/v1/messages", strings.Replace(hostedSearchRequest, "%s", "true", 1))
	body := rec.Body.String()
	i, j, k := strings.Index(body, `"type":"server_tool_use"`), strings.Index(body, `"type":"web_search_tool_result"`), strings.Index(body, `"type":"text_delta"`)
	if i < 0 || j < i || k < j || !strings.Contains(body, `"web_search_requests":1`) {
		t.Errorf("blocks are not in order (call %d, result %d, text %d):\n%s", i, j, k, body)
	}
}

func TestASearchThatFailsOrIsNotSetUpIsUnavailableNotInvented(t *testing.T) {
	for name, stub := range map[string]*stubSearcher{"failing": {err: errors.New("brave answered 401")}, "not set up": nil} {
		h, up := searchRig(t, stub)
		rec := zenPost(h, "/v1/messages", strings.Replace(hostedSearchRequest, "%s", "false", 1))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"error_code":"unavailable"`) {
			t.Errorf("%s: status = %d, body = %s", name, rec.Code, rec.Body.String())
		}
		if raw := up.last(t).raw; !strings.Contains(raw, "could not be run") || !strings.Contains(raw, "do not invent") {
			t.Errorf("%s: the model was not told: %s", name, raw)
		}
	}
}

func TestARequestWithNoHostedSearchToolIsLeftAlone(t *testing.T) {
	stub := &stubSearcher{}
	h, up := searchRig(t, stub)
	rec := zenPost(h, "/v1/messages", `{"model":"opencode/big-pickle","max_tokens":50,"tools":[{"name":"WebSearch","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"hi"}]}`)
	if rec.Code != http.StatusOK || len(stub.queries) != 0 {
		t.Errorf("status = %d, searches = %v", rec.Code, stub.queries)
	}
	if strings.Contains(rec.Body.String(), "server_tool_use") {
		t.Errorf("search blocks on a call that searched nothing: %s", rec.Body.String())
	}
	if names := toolNamesOf(up.last(t).body); !strings.Contains(strings.Join(names, ","), "WebSearch") {
		t.Errorf("the client's own WebSearch tool was dropped: %v", names)
	}
}

func TestSearcherIsBuiltFromTheEnvironmentOnce(t *testing.T) {
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	a, _ := newServer(s, nil, nil)
	t.Setenv("CCW_SEARCH_PROVIDER", "")
	if got, why := a.searcher(); got != nil || !strings.Contains(why, "CCW_SEARCH_PROVIDER") {
		t.Errorf("searcher = %v, why = %q", got, why)
	}
	a2, _ := newServer(s, nil, nil)
	t.Setenv("CCW_SEARCH_PROVIDER", "brave")
	t.Setenv("CCW_SEARCH_KEY", "k")
	if got, why := a2.searcher(); got == nil || why != "" {
		t.Errorf("searcher = %v, why = %q", got, why)
	}
	a3, _ := newServer(s, nil, nil)
	t.Setenv("CCW_SEARCH_PROVIDER", "brave")
	t.Setenv("CCW_SEARCH_KEY", "")
	if got, why := a3.searcher(); got != nil || !strings.Contains(why, "key") {
		t.Errorf("searcher = %v, why = %q", got, why)
	}
}

// The grounded answer of Gemini with Google Search, as the Antigravity backend
// streams it: the sources are Google's redirect links, named by their site.
const groundedStream = `data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"thought":true,"text":""}]}}]}}

data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"Go 1.27.1 is the latest release."}]},"finishReason":"STOP","groundingMetadata":{"webSearchQueries":["latest go release"],"searchEntryPoint":{"renderedContent":"<style></style>"},"groundingChunks":[{"web":{"uri":"%s/grounding-api-redirect/abc","title":"go.dev"}},{"web":{"uri":"https://plain.example/page","title":"plain.example"}}],"groundingSupports":[{"segment":{"text":"Go 1.27.1 is the latest release"},"groundingChunkIndices":[0,1]},{"segment":{"text":"It came out in September"},"groundingChunkIndices":[0]}]}}]}}

`

func TestAnAntigravityAccountSearchesThroughGoogleWhenNothingElseIsSet(t *testing.T) {
	t.Setenv("CCW_SEARCH_PROVIDER", "")
	var gotBody string
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://go.dev/doc/go1.27", http.StatusFound)
	}))
	defer redirect.Close()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		switch {
		case strings.HasSuffix(r.URL.Path, ":loadCodeAssist"):
			io.WriteString(w, `{"cloudaicompanionProject":"proj-7"}`)
		default:
			gotBody = string(b)
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, strings.Replace(groundedStream, "%s", redirect.URL, 1))
		}
	}))
	defer up.Close()
	old := antigravityProdURL
	antigravityProdURL = up.URL
	defer func() { antigravityProdURL = old }()

	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	s.CreateConnection("antigravity", "ag", "fake-access-token")
	a, h := newServer(s, map[string]string{"antigravity": up.URL, "opencode": up.URL}, nil)
	s.CreateConnection("opencode", "zen", "public")

	res, err := (antigravitySearch{a: a}).Search(context.Background(), "latest go release", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || res[0].Title != "go.dev" || res[0].URL != "https://go.dev/doc/go1.27" {
		t.Fatalf("results = %+v: Google's redirect link should give the page it stands for", res)
	}
	if res[0].Snippet != "Go 1.27.1 is the latest release It came out in September" || res[1].URL != "https://plain.example/page" {
		t.Errorf("results = %+v", res)
	}
	for _, want := range []string{`"googleSearch"`, "latest go release", `"model":"gemini-2.5-flash"`} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("the grounded call misses %s: %s", want, gotBody)
		}
	}
	// With no search set in the environment, the tool searches through that account.
	if got, why := a.searcher(); got == nil || why != "" {
		t.Errorf("searcher = %v, why = %q", got, why)
	}
	_ = h
}

func TestWithNoSearchAndNoAntigravityAccountTheToolIsUnavailable(t *testing.T) {
	t.Setenv("CCW_SEARCH_PROVIDER", "")
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	a, _ := newServer(s, nil, nil)
	if got, why := a.searcher(); got != nil || !strings.Contains(why, "Antigravity") {
		t.Errorf("searcher = %v, why = %q", got, why)
	}
}
