package diabridge

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeHandler stands in for Dia's handler process: it refuses a request whose
// credential is not the one on file, and answers a Vertex-shaped path with an
// SSE chunk, the way the real one streams an answer back.
type fakeHandler struct {
	gotAuth   chan string
	gotPath   chan string
	gotMethod chan string
	front     string
}

func newFakeHandler(t *testing.T) *fakeHandler {
	t.Helper()
	f := &fakeHandler{
		gotAuth:   make(chan string, 8),
		gotPath:   make(chan string, 8),
		gotMethod: make(chan string, 8),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.gotAuth <- r.Header.Get("Authorization")
		f.gotPath <- r.URL.Path
		f.gotMethod <- r.Method
		if r.Header.Get("Authorization") != "Bearer "+wantToken(r) {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"Unauthorized"}`)
			return
		}
		if !strings.Contains(r.URL.Path, "publishers/anthropic/models/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl := w.(http.Flusher)
		for _, chunk := range []string{
			"event: message_start\ndata: {\"type\":\"message_start\"}\n\n",
			"event: content_block_delta\ndata: {\"delta\":{\"text\":\"OK\"}}\n\n",
		} {
			fmt.Fprint(w, chunk)
			fl.Flush()
		}
	}))
	t.Cleanup(srv.Close)
	f.front = srv.URL
	return f
}

// wantToken reads the credential the test last wrote, so the fake handler accepts
// the newest turn exactly as the real one does.
func wantToken(r *http.Request) string {
	raw, err := os.ReadFile(portFilePath())
	if err != nil {
		return ""
	}
	var g gateway
	if json.Unmarshal(raw, &g) != nil {
		return ""
	}
	return g.Token
}

// startBridge puts a Bridge on loopback with both faces on real ports and points
// it at a private port file, so no test touches Dia's real gateway file.
//
// Recovery is off: a test that leaves no gateway wants the plain 503 that the
// caller sees when this machine cannot type into Dia, and waiting out the
// recovery timeout would make every such test take twenty seconds.
func startBridge(t *testing.T) *Bridge {
	t.Helper()
	t.Setenv(portFileEnv, filepath.Join(t.TempDir(), "dia-handler.json"))
	b := New()
	b.recover = &recoverer{}
	if err := b.Start("127.0.0.1:0", "127.0.0.1:0"); err != nil {
		t.Fatalf("start bridge: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

// writeGateway publishes a port/token pair the way the patched handler does.
func writeGateway(t *testing.T, port int, token string) {
	t.Helper()
	raw, err := json.Marshal(gateway{Port: port, Token: token})
	if err != nil {
		t.Fatalf("marshal gateway: %v", err)
	}
	if err := os.WriteFile(portFilePath(), raw, 0o600); err != nil {
		t.Fatalf("write gateway file: %v", err)
	}
}

// portOf is the port an httptest server is listening on.
func portOf(t *testing.T, rawURL string) int {
	t.Helper()
	_, portStr, err := net.SplitHostPort(strings.TrimPrefix(rawURL, "http://"))
	if err != nil {
		t.Fatalf("split %q: %v", rawURL, err)
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		t.Fatalf("parse port %q: %v", portStr, err)
	}
	return port
}

// postBack sends one Messages call to the bridge's back face.
func postBack(t *testing.T, b *Bridge, body string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Post("http://"+b.BackAddr()+"/v1/messages", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post back face: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	raw := readAll(t, resp)
	return resp, raw
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(raw)
}

func TestBridgeReadsTokenFromHandlerFile(t *testing.T) {
	f := newFakeHandler(t)
	b := startBridge(t)
	writeGateway(t, portOf(t, f.front), "sk-test")

	// No Authorization header from ccw: the bridge must supply the credential
	// of the current Dia turn itself, which is the whole point of the back face.
	resp, body := postBack(t, b, `{"model":"claude-opus-4-8","stream":true}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %s", resp.StatusCode, body)
	}
	if got := <-f.gotAuth; got != "Bearer sk-test" {
		t.Errorf("handler saw Authorization %q, want %q", got, "Bearer sk-test")
	}
	if got := <-f.gotMethod; got != http.MethodPost {
		t.Errorf("handler saw method %q, want POST", got)
	}
	if !strings.Contains(body, "event: content_block_delta") || !strings.Contains(body, "OK") {
		t.Errorf("body did not carry the stream through: %s", body)
	}
}

func TestBridgeCallsTheVertexPathTheHandlerParses(t *testing.T) {
	f := newFakeHandler(t)
	b := startBridge(t)
	writeGateway(t, portOf(t, f.front), "sk-test")

	// Dia's Bedrock upstream only serves streamRawPredict.
	resp, _ := postBack(t, b, `{"model":"claude-opus-4-6"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	got := <-f.gotPath
	want := "/v1/projects/p/locations/l/publishers/anthropic/models/claude-opus-4-6:streamRawPredict"
	if got != want {
		t.Errorf("handler saw path %q, want %q", got, want)
	}
	// Streaming call routes to :streamRawPredict.
	resp2, _ := postBack(t, b, `{"model":"claude-opus-4-6","stream":true}`)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("streaming status = %d", resp2.StatusCode)
	}
	got2 := <-f.gotPath
	want2 := "/v1/projects/p/locations/l/publishers/anthropic/models/claude-opus-4-6:streamRawPredict"
	if got2 != want2 {
		t.Errorf("streaming handler saw path %q, want %q", got2, want2)
	}
}

func TestBridgeRereadsTokenEveryCall(t *testing.T) {
	f := newFakeHandler(t)
	b := startBridge(t)
	writeGateway(t, portOf(t, f.front), "sk-first")

	if resp, body := postBack(t, b, `{"model":"claude-opus-4-8"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("first turn status = %d, body %s", resp.StatusCode, body)
	}
	if got := <-f.gotAuth; got != "Bearer sk-first" {
		t.Fatalf("first turn Authorization = %q", got)
	}

	// Dia regenerates both port and token on every turn, so a cached credential
	// would start failing with 401 after the next chat message in Dia.
	writeGateway(t, portOf(t, f.front), "sk-second")
	if resp, body := postBack(t, b, `{"model":"claude-opus-4-8"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("second turn status = %d, body %s", resp.StatusCode, body)
	}
	if got := <-f.gotAuth; got != "Bearer sk-second" {
		t.Errorf("second turn Authorization = %q, want %q", got, "Bearer sk-second")
	}
}

func TestBridgeFrontFaceKeepsTheCredentialClaudeSends(t *testing.T) {
	f := newFakeHandler(t)
	b := startBridge(t)
	writeGateway(t, portOf(t, f.front), "sk-test")

	req, err := http.NewRequest(http.MethodPost,
		"http://"+b.Addr()+"/v1/projects/p/locations/l/publishers/anthropic/models/claude-opus-4-8:streamRawPredict",
		strings.NewReader(`{"model":"claude-opus-4-8"}`))
	if err != nil {
		t.Fatalf("build front request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer sk-test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post front face: %v", err)
	}
	defer resp.Body.Close()
	raw := readAll(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %s", resp.StatusCode, raw)
	}
	if got := <-f.gotAuth; got != "Bearer sk-test" {
		t.Errorf("handler saw Authorization %q, want the one claude sent", got)
	}
	if b.FrontModel() != "claude-opus-4-8" {
		t.Errorf("FrontModel() = %q, want claude-opus-4-8", b.FrontModel())
	}
}

func TestBridgeFrontFaceWithoutAGatewayIsUnavailable(t *testing.T) {
	b := startBridge(t)
	// No gateway file: Dia has not run a chat turn yet.
	req, err := http.NewRequest(http.MethodPost, "http://"+b.Addr()+"/v1/messages", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("build front request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post front face: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
}

func TestBridgeRejectsBadBody(t *testing.T) {
	f := newFakeHandler(t)
	b := startBridge(t)
	writeGateway(t, portOf(t, f.front), "sk-test")

	cases := map[string]string{
		"not json":     `not json at all`,
		"no model":     `{"stream":true}`,
		"empty body":   ``,
		"json null":    `null`,
		"model number": `{"model":42}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			resp, raw := postBack(t, b, body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body %q)", resp.StatusCode, body)
			}
			if !strings.Contains(raw, "bad request body") {
				t.Errorf("body = %s, want the bad-request message", raw)
			}
			select {
			case got := <-f.gotAuth:
				t.Errorf("handler was called with %q, want no call at all", got)
			default:
			}
		})
	}
}

func TestBridgeRejectsModelWithColon(t *testing.T) {
	f := newFakeHandler(t)
	b := startBridge(t)
	writeGateway(t, portOf(t, f.front), "sk-test")

	resp, raw := postBack(t, b, `{"model":"claude-opus-4-6:streamRawPredict"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if !strings.Contains(raw, "model must not contain a colon") {
		t.Errorf("body = %s, want the colon message", raw)
	}
	select {
	case got := <-f.gotPath:
		t.Errorf("handler was called on path %q, want no call at all", got)
	default:
	}
}

func TestBridgeWithoutHandlerFileIsUnavailable(t *testing.T) {
	b := startBridge(t)
	// No gateway file at all: ccw must be told to type 'bridge' in Dia.
	resp, raw := postBack(t, b, `{"model":"claude-opus-4-8"}`)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
	// The message has to name the action the caller can take, so it is checked
	// through the constant rather than a copy that can drift.
	if !strings.Contains(raw, noHandlerMsg) {
		t.Errorf("body = %s, want the not-ready message", raw)
	}
}

func TestBridgeRejectsAnUnusableGatewayFile(t *testing.T) {
	cases := map[string]string{
		"empty":        ``,
		"not json":     `oops`,
		"zero port":    `{"port":0,"token":"sk-test"}`,
		"empty token":  `{"port":49161,"token":""}`,
		"wrong shape":  `{"port":"49161","token":"sk-test"}`,
		"json array":   `[1,2,3]`,
		"json null":    `null`,
		"missing keys": `{}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			b := startBridge(t)
			if err := os.WriteFile(portFilePath(), []byte(raw), 0o600); err != nil {
				t.Fatalf("write gateway file: %v", err)
			}
			resp, body := postBack(t, b, `{"model":"claude-opus-4-8"}`)
			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Errorf("status = %d, want 503", resp.StatusCode)
			}
			if !strings.Contains(body, noHandlerMsg) {
				t.Errorf("body = %s, want the not-ready message", body)
			}
		})
	}
}

func TestBridgeHealthReportsTheLiveGateway(t *testing.T) {
	f := newFakeHandler(t)
	b := startBridge(t)

	resp, err := http.Get("http://" + b.BackAddr() + "/health")
	if err != nil {
		t.Fatalf("get health: %v", err)
	}
	defer resp.Body.Close()
	var empty struct {
		OK          bool `json:"ok"`
		HandlerPort int  `json:"handlerPort"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&empty); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if empty.OK || empty.HandlerPort != 0 {
		t.Errorf("health before any turn = %+v, want not ok with port 0", empty)
	}

	port := portOf(t, f.front)
	writeGateway(t, port, "sk-test")
	resp2, err := http.Get("http://" + b.BackAddr() + "/health")
	if err != nil {
		t.Fatalf("get health again: %v", err)
	}
	defer resp2.Body.Close()
	var live struct {
		OK          bool `json:"ok"`
		HandlerPort int  `json:"handlerPort"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&live); err != nil {
		t.Fatalf("decode health again: %v", err)
	}
	if !live.OK || live.HandlerPort != port {
		t.Errorf("health after a turn = %+v, want ok with port %d", live, port)
	}
}

func TestBridgeHandsTheStatusTheHandlerChose(t *testing.T) {
	// A handler that refuses the credential answers 401; the bridge must relay
	// that status instead of turning it into its own 503.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"Unauthorized"}`)
	}))
	defer srv.Close()
	b := startBridge(t)
	writeGateway(t, portOf(t, srv.URL), "sk-stale")

	resp, raw := postBack(t, b, `{"model":"claude-opus-4-8"}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want the handler's 401", resp.StatusCode)
	}
	if !strings.Contains(raw, "Unauthorized") {
		t.Errorf("body = %s, want the handler's own error body", raw)
	}
}

func TestBridgeSaysTheTurnIsOverWhenTheHandlerIsGone(t *testing.T) {
	// The port is still on file but nothing is listening: Dia's handler built a
	// gateway for one chat turn and exited with it. A stale file must read as
	// "not ready" rather than "upstream is broken", because 503 is the status
	// ccw retries and fails over on, while 502 is neither.
	b := startBridge(t)
	writeGateway(t, 1, "sk-test")

	resp, raw := postBack(t, b, `{"model":"claude-opus-4-8"}`)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
	if !strings.Contains(raw, noHandlerMsg) {
		t.Errorf("body = %s, want %q", raw, noHandlerMsg)
	}
}

func TestBridgeHealthReportsAGoneGatewayAsNotReady(t *testing.T) {
	// A stale gateway file otherwise makes health look healthy, which is how a
	// dead bridge passes for a working one.
	b := startBridge(t)
	writeGateway(t, 1, "sk-test")

	resp, err := http.Get("http://" + b.BackAddr() + "/health")
	if err != nil {
		t.Fatalf("get health: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	var got struct {
		OK          bool `json:"ok"`
		HandlerPort int  `json:"handlerPort"`
	}
	if err := json.Unmarshal([]byte(readAll(t, resp)), &got); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if got.OK {
		t.Error("ok = true, want false when the gateway on file is gone")
	}
	if got.HandlerPort != 0 {
		t.Errorf("handlerPort = %d, want 0 for a gateway nobody is listening on", got.HandlerPort)
	}
}

func TestBridgeAssemblesThinkingBlocks(t *testing.T) {
	// Dia's own request carries {"thinking":{"type":"adaptive"}}, so the handler
	// streams thinking_delta events. Dropping them loses the reasoning the
	// caller asked for, and returning the text alone is not the same reply.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+wantToken(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, chunk := range []string{
			`data: {"type":"message_start","message":{"id":"msg_1","model":"claude-opus-4-8","role":"assistant","usage":{"input_tokens":11}}}`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"weighing it"}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":" then answering"}}`,
			`data: {"type":"content_block_stop","index":0}`,
			`data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"OK"}}`,
			`data: {"type":"content_block_stop","index":1}`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":22}}`,
			"",
		} {
			fmt.Fprint(w, chunk+"\n")
			w.(http.Flusher).Flush()
		}
	}))
	t.Cleanup(srv.Close)

	b := startBridge(t)
	writeGateway(t, portOf(t, srv.URL), "sk-test")

	resp, raw := postBack(t, b, `{"model":"claude-opus-4-8"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, raw)
	}

	var got struct {
		Content []struct {
			Type     string `json:"type"`
			Thinking string `json:"thinking"`
			Text     string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("decode reply: %v\n%s", err, raw)
	}
	if len(got.Content) != 2 {
		t.Fatalf("content blocks = %d, want 2 (thinking then text): %s", len(got.Content), raw)
	}
	if got.Content[0].Type != "thinking" {
		t.Errorf("first block type = %q, want thinking", got.Content[0].Type)
	}
	if got.Content[0].Thinking != "weighing it then answering" {
		t.Errorf("thinking = %q, want both deltas joined", got.Content[0].Thinking)
	}
	if got.Content[1].Type != "text" || got.Content[1].Text != "OK" {
		t.Errorf("second block = %+v, want text OK", got.Content[1])
	}
	if got.Usage.InputTokens != 11 || got.Usage.OutputTokens != 22 {
		t.Errorf("usage = %+v, want 11 in / 22 out", got.Usage)
	}
}

func TestBridgeStreamsChunkByChunk(t *testing.T) {
	// A reply must reach ccw as the handler produces it, not in one burst at the
	// end, or a chat answer would stall until the whole turn finished.
	sent := make(chan struct{})
	release := make(chan struct{})
	b := startBridge(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl := w.(http.Flusher)
		fmt.Fprint(w, "event: content_block_delta\ndata: {\"delta\":{\"text\":\"one\"}}\n\n")
		fl.Flush()
		close(sent)
		<-release
		fmt.Fprint(w, "event: content_block_delta\ndata: {\"delta\":{\"text\":\"two\"}}\n\n")
		fl.Flush()
	}))
	defer srv.Close()
	writeGateway(t, portOf(t, srv.URL), "sk-test")

	type result struct {
		first string
		err   error
	}
	results := make(chan result, 1)
	go func() {
		resp, err := http.Post("http://"+b.BackAddr()+"/v1/messages", "application/json",
			strings.NewReader(`{"model":"claude-opus-4-8","stream":true}`))
		if err != nil {
			results <- result{err: err}
			return
		}
		defer resp.Body.Close()
		line := make([]byte, 64)
		n, err := resp.Body.Read(line)
		results <- result{first: string(line[:n]), err: err}
	}()

	<-sent
	close(release)
	got := <-results
	if got.err != nil {
		t.Fatalf("read first chunk: %v", got.err)
	}
	if !strings.Contains(got.first, "one") {
		t.Errorf("first chunk = %q, want it to arrive before the stream ends", got.first)
	}
}

func TestBearerAcceptsBothFormsOfTheToken(t *testing.T) {
	cases := map[string]string{
		"sk-test":        "Bearer sk-test",
		"Bearer sk-test": "Bearer sk-test",
		"bearer sk-test": "Bearer sk-test",
	}
	for token, want := range cases {
		if got := bearer(token); got != want {
			t.Errorf("bearer(%q) = %q, want %q", token, got, want)
		}
	}
}

func TestModelFromPath(t *testing.T) {
	cases := []struct {
		path   string
		want   string
		wantOK bool
	}{
		{"/v1/projects/p/locations/l/publishers/anthropic/models/claude-opus-4-8:streamRawPredict", "claude-opus-4-8", true},
		{"/v1/projects/p/locations/l/publishers/anthropic/models/claude-opus-4-6:rawPredict", "claude-opus-4-6", true},
		{"publishers/anthropic/models/gemini-3-pro", "gemini-3-pro", true},
		{"/v1/messages", "", false},
		{"/v1/projects/p/locations/l/publishers/anthropic/models/", "", false},
		{"/v1/projects/p/locations/l/publishers/anthropic/models/:streamRawPredict", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := modelFromPath(tc.path)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("modelFromPath(%q) = (%q, %t), want (%q, %t)", tc.path, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestStartFailsWithoutLeavingTheFrontHalfRunning(t *testing.T) {
	t.Setenv(portFileEnv, filepath.Join(t.TempDir(), "dia-handler.json"))
	// A listener on the back address makes the second bind fail; the front
	// listener must be released, not left holding the port.
	hold, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("hold back port: %v", err)
	}
	defer hold.Close()
	front := "127.0.0.1:0"
	b := New()
	if err := b.Start(front, hold.Addr().String()); err == nil {
		t.Fatal("Start succeeded, want an error for the taken back address")
	}
	if b.front != nil || b.back != nil {
		t.Error("Start left servers behind after a failed bind")
	}
}

func TestStartRejectsATakenFrontAddress(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer taken.Close()
	b := New()
	if err := b.Start(taken.Addr().String(), "127.0.0.1:0"); err == nil {
		t.Fatal("Start succeeded on a taken front address")
	}
}

func TestCloseIsSafeWithoutStart(t *testing.T) {
	if err := New().Close(); err != nil {
		t.Errorf("Close before Start = %v, want nil", err)
	}
}

func TestBackFaceIgnoresOtherPaths(t *testing.T) {
	b := startBridge(t)
	resp, err := http.Get("http://" + b.BackAddr() + "/v1/chat/completions")
	if err != nil {
		t.Fatalf("get wrong back path: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 on a path the bridge does not serve", resp.StatusCode)
	}
}

func TestStartDefaultsEmptyAddressesToTheKnownPorts(t *testing.T) {
	t.Setenv(portFileEnv, filepath.Join(t.TempDir(), "dia-handler.json"))
	b := New()
	if err := b.Start(DefaultFrontAddr, DefaultBackAddr); err != nil {
		t.Skipf("default ports are busy on this machine: %v", err)
	}
	defer b.Close()
	if b.Addr() != DefaultFrontAddr || b.BackAddr() != DefaultBackAddr {
		t.Errorf("addresses = %q/%q, want %q/%q", b.Addr(), b.BackAddr(), DefaultFrontAddr, DefaultBackAddr)
	}
}

func TestBridgePassesTheBodyThroughUntouched(t *testing.T) {
	// ccw sends Anthropic Messages JSON; the bridge must not reshape it, and the
	// handler must see the bytes ccw wrote.
	var seen string
	b := startBridge(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "cannot read body", http.StatusBadRequest)
			return
		}
		seen = string(raw)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	writeGateway(t, portOf(t, srv.URL), "sk-test")

	body := `{"anthropic_version":"vertex-2023-10-16","max_tokens":16,"messages":[{"content":"hi","role":"user"}],"model":"claude-opus-4-8","stream":true}`
	if resp, raw := postBack(t, b, body); resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %s", resp.StatusCode, raw)
	}
	if seen != body {
		t.Errorf("handler saw body %q, want %q", seen, body)
	}
}
