package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/naniiluja/ccw/internal/store"
)

// echoUpstream answers every call with body, as a stream when it starts with
// "data:", and keeps the last request body it saw.
func echoUpstream(t *testing.T, body string) (string, func() string) {
	t.Helper()
	var mu sync.Mutex
	var got string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Write([]byte(`{"data":[{"id":"llama"}]}`))
			return
		}
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = string(b)
		mu.Unlock()
		if strings.HasPrefix(body, "data:") || strings.HasPrefix(body, "event:") {
			w.Header().Set("Content-Type", "text/event-stream")
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(up.Close)
	return up.URL, func() string { mu.Lock(); defer mu.Unlock(); return got }
}

func echoEnv(t *testing.T, prov, url string) (http.Handler, *store.Store) {
	t.Helper()
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	t.Cleanup(func() { s.Close() })
	s.CreateConnection(prov, "a", "k")
	return New(s, map[string]string{prov: url}), s
}

func TestPassthroughEchoesTheCallersModel(t *testing.T) {
	url, _ := echoUpstream(t, `{"id":"chatcmpl-1","object":"chat.completion","model":"llama","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}]}`)
	h, _ := echoEnv(t, "groq", url)
	rec := postV1(h, `{"model":"groq/llama","messages":[]}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"model":"groq/llama"`) {
		t.Errorf("got %d %s", rec.Code, rec.Body.String())
	}
	if cl := rec.Header().Get("Content-Length"); cl != "" && cl != strconv.Itoa(rec.Body.Len()) {
		t.Errorf("Content-Length %s for a %d byte body", cl, rec.Body.Len())
	}
}

func TestPassthroughStreamCountsUsageTheCallerDidNotAskFor(t *testing.T) {
	stream := `data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"llama","choices":[{"index":0,"delta":{"role":"assistant","content":"hi"},"finish_reason":null}],"usage":null}` + "\n\n" +
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"llama","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":null}` + "\n\n" +
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"llama","choices":[],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9}}` + "\n\n" +
		"data: [DONE]\n\n"
	url, sent := echoUpstream(t, stream)
	h, s := echoEnv(t, "groq", url)
	rec := postV1(h, `{"model":"groq/llama","stream":true,"messages":[]}`)
	body := rec.Body.String()
	if !strings.Contains(sent(), `"include_usage":true`) {
		t.Errorf("upstream was not asked for usage: %s", sent())
	}
	if strings.Contains(body, `"usage"`) || strings.Contains(body, `"choices":[]`) {
		t.Errorf("the caller got usage it did not ask for:\n%s", body)
	}
	if strings.Count(body, `"model":"groq/llama"`) != 2 || !strings.HasSuffix(body, "data: [DONE]\n\n") {
		t.Errorf("stream = %s", body)
	}
	rows, _ := s.Usage()
	if len(rows) != 1 || rows[0].InputTokens != 7 || rows[0].OutputTokens != 2 {
		t.Errorf("usage rows = %+v", rows)
	}

	rec = postV1(h, `{"model":"groq/llama","stream":true,"stream_options":{"include_usage":true},"messages":[]}`)
	if !strings.Contains(rec.Body.String(), `"choices":[],"usage":{"prompt_tokens":7`) {
		t.Errorf("the usage the caller asked for is gone:\n%s", rec.Body.String())
	}
}

func TestAnthropicPassthroughEchoesTheCallersModel(t *testing.T) {
	stream := "event: message_start\n" + `data: {"type":"message_start","message":{"id":"msg_1","model":"claude-x","usage":{"input_tokens":3}}}` + "\n\n" +
		"event: message_stop\n" + `data: {"type":"message_stop"}` + "\n\n"
	url, _ := echoUpstream(t, stream)
	h, _ := echoEnv(t, "claude", url)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, loopbackRequest("POST", "/v1/messages", strings.NewReader(`{"model":"claude/claude-x","max_tokens":1,"stream":true}`)))
	if !strings.Contains(rec.Body.String(), `"model":"claude/claude-x"`) {
		t.Errorf("stream = %s", rec.Body.String())
	}
}

func TestSetModelSplicesOnlyTheTopLevelValue(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"model":"a","x":1}`, `{"model":"b","x":1}`},
		{`{ "stream" : true , "model" : "a" }`, `{ "stream" : true , "model" : "b" }`},
		{`{"messages":[{"model":"keep"}],"model":"a","n":1.50}`, `{"messages":[{"model":"keep"}],"model":"b","n":1.50}`},
		{`{"model":null}`, `{"model":"b"}`},
	}
	for _, c := range cases {
		got, ok := setModel([]byte(c.in), "b")
		if !ok || string(got) != c.want {
			t.Errorf("setModel(%s) = %s, %v; want %s", c.in, got, ok, c.want)
		}
	}
}

func TestSetModelLeavesOtherBodiesAlone(t *testing.T) {
	for _, in := range []string{``, `[1,2]`, `{"x":{"model":"a"}}`, `not json`} {
		got, ok := setModel([]byte(in), "b")
		if ok || string(got) != in {
			t.Errorf("setModel(%q) = %q, %v; want unchanged", in, got, ok)
		}
	}
}

func TestTopLevelCountSeesOnlyTheTopLevel(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{`{"model":"a","x":1}`, 1},
		{`{"model":"a","x":1,"model":"b"}`, 2},
		{`{"messages":[{"model":"keep"}],"model":"a"}`, 1},
		{`{"x":1}`, 0},
		{`not json`, 0},
		{`[1,2]`, 0},
	}
	for _, c := range cases {
		if got := topLevelCount([]byte(c.in), "model"); got != c.want {
			t.Errorf("topLevelCount(%s) = %d, want %d", c.in, got, c.want)
		}
	}
}

// A second top-level "model" key makes the model ccw checked different from
// the model the provider reads, so the request never reaches an account.
func TestV1RefusesADuplicateTopLevelModelKey(t *testing.T) {
	f := &fakeProvider{models: `{"data":[{"id":"on-model"}]}`}
	url := f.start(t)
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	s.CreateConnection("groq", "a", "ka")
	h := New(s, map[string]string{"groq": url})

	for _, body := range []string{
		`{"model":"groq/on-model","messages":[{"role":"user","content":"hi"}],"model":"off-model"}`,
		`{"model":"on-model","messages":[{"role":"user","content":"hi"}],"model":"off-model"}`,
	} {
		if rec := postV1(h, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: code = %d body = %s, want 400", body, rec.Code, rec.Body.String())
		}
	}
	f.mu.Lock()
	calls := append([]string(nil), f.calls...)
	f.mu.Unlock()
	if len(calls) != 0 {
		t.Errorf("upstream calls = %q, want none", calls)
	}

	// One "model" key still routes as before.
	rec := postV1(h, `{"model":"groq/on-model","messages":[{"role":"user","content":"hi"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("single key: code = %d body = %s", rec.Code, rec.Body.String())
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) != 1 || !strings.Contains(f.calls[0], `"model":"on-model"`) {
		t.Errorf("upstream calls = %q, want one call on on-model", f.calls)
	}
}
