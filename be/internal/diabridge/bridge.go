// Package diabridge puts ccw between Dia's bundled Claude Code CLI and the
// handler process Dia spawns for it.
//
// Dia reaches its model in five hops: the bundled claude binary sends plain
// JSON to a handler-local HTTP server, the handler forwards the turn over IPC to
// the Dia app, and only the app encrypts it for the network. The bridge sits on
// the first hop, so it sees the plaintext body and the credential that hop
// carries.
//
// Dia rebuilds that gateway for every chat turn, on a fresh port with a fresh
// token. The patched handler writes both to a file and the bridge re-reads it on
// each request instead of caching, so a new turn in Dia refreshes ccw's
// credential with no manual step.
package diabridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/naniiluja/ccw/internal/websearch"
)

// Defaults for the two listening addresses. The front is the one Dia's claude
// is pointed at; the back is the one ccw's dia provider calls.
const (
	DefaultFrontAddr = "127.0.0.1:20139"
	DefaultBackAddr  = "127.0.0.1:20140"
)

// defaultPortFile is where scripts/dia-patch.sh tells the handler to leave the
// gateway it built.
const defaultPortFile = "/tmp/dia-handler.json"

// portFileEnv overrides that path. Tests set it so they never touch Dia's real
// file.
const portFileEnv = "DIA_HANDLER_PORT_FILE"

// endpointPath is the shape Dia's handler expects: Bedrock/Vertex raw prediction
// requires streaming.
const endpointPath = "v1/projects/p/locations/l/publishers/anthropic/models/%s:streamRawPredict"

// maxBody bounds the request the back face reads, matching the ceiling ccw
// applies to a Messages call.
const maxBody = 32 << 20

// noHandlerMsg is what ccw sees when no gateway is live.
//
// Recovery has already tried to build one, so this answers a machine where the
// attempt cannot land: macOS has not let osascript drive the interface, or Dia
// is closed. Both are fixed by hand, so the message names the hand step first.
// It holds nothing JSON escapes, so a caller can match it without decoding.
const noHandlerMsg = "dia handler not ready: send 'bridge' in a Dia chat and pick the second suggestion (Privacy and Security, Accessibility must allow osascript to control this computer for ccw to do it by itself)"

// probeTimeout bounds the liveness dial of a gateway port. A live handler
// answers a local connect immediately, so anything slower is a dead one.
const probeTimeout = 250 * time.Millisecond

// errNoHandler reports that no live gateway is on file.
var errNoHandler = errors.New(noHandlerMsg)

// gateway is the port/token pair the patched handler leaves behind for one turn
// of Dia's chat. The token is the bare key it generated; handler compares
// requests against `Bearer <token>`.
type gateway struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
}

// Bridge proxies Dia's claude traffic and answers ccw. The zero value is not
// usable; call New.
type Bridge struct {
	front, back *http.Server

	mu        sync.Mutex
	lastModel string

	frontAddr string
	backAddr  string
	portFile  string
	recover   *recoverer

	// search is the search service ccw runs for a client whose model cannot
	// search on its own. It is the same service every other provider gets, so
	// a ccw with an Antigravity account serves web search to Dia too.
	search websearch.Searcher
}

// New returns a Bridge that reads gateway files from the default path.
func New() *Bridge {
	return &Bridge{portFile: portFilePath(), recover: newRecoverer()}
}

// Start binds both addresses and begins serving. Both listeners are bound
// before either server is started, so a failure on the second address does not
// leave the front half running.
func (b *Bridge) Start(frontAddr, backAddr string) error {
	if frontAddr == "" {
		frontAddr = DefaultFrontAddr
	}
	if backAddr == "" {
		backAddr = DefaultBackAddr
	}
	b.portFile = portFilePath()

	frontLn, err := net.Listen("tcp", frontAddr)
	if err != nil {
		return fmt.Errorf("dia bridge front listen %s: %w", frontAddr, err)
	}
	backLn, err := net.Listen("tcp", backAddr)
	if err != nil {
		_ = frontLn.Close()
		return fmt.Errorf("dia bridge back listen %s: %w", backAddr, err)
	}
	b.frontAddr = frontLn.Addr().String()
	b.backAddr = backLn.Addr().String()

	frontMux := http.NewServeMux()
	frontMux.HandleFunc("/", b.forwardToHandler)
	backMux := http.NewServeMux()
	backMux.HandleFunc("POST /v1/messages", b.messages)
	backMux.HandleFunc("GET /health", b.health)

	b.front = &http.Server{Handler: frontMux, ReadHeaderTimeout: 10 * time.Second}
	b.back = &http.Server{Handler: backMux, ReadHeaderTimeout: 10 * time.Second}

	go func() { _ = b.front.Serve(frontLn) }()
	go func() { _ = b.back.Serve(backLn) }()
	return nil
}

// forwardToHandler is the front face: Dia's claude dials it instead of the
// handler. Every header crosses untouched, Authorization included, so the
// handler still sees the credential claude was given and still authenticates it.
func (b *Bridge) forwardToHandler(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		body, _ := io.ReadAll(io.LimitReader(r.Body, maxBody))
		// Dia's own claude declares the search tool too, and its gateway
		// refuses it the same way, so the front runs the same search the back
		// does. Nothing else is rewritten here: this client already speaks the
		// gateway's tool names.
		body = b.runSearch(r.Context(), body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		r.Header.Set("Content-Length", strconv.Itoa(len(body)))
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			if model, ok := modelFromPath(pr.In.URL.Path); ok {
				b.mu.Lock()
				b.lastModel = model
				b.mu.Unlock()
			}
			g, err := b.gateway()
			if err != nil {
				// Dia's own chat reached here but no gateway is alive, which
				// means the turn that would have built one is over. Recovery is
				// started from here as well as from the back face: this is the
				// request that proves Dia is mid-turn, so a keystroke lands
				// while its chat box is showing rather than being typed into
				// whatever happens to be on screen.
				go func() {
					if b.recover.waitFor(context.Background(), b.portFile) {
						slog.Info("dia.recover.front")
					}
				}()
				// Leaving Out.URL without a host sends the request to
				// ErrorHandler, which answers 503 instead of dialing a
				// stale port.
				return
			}
			pr.Out.URL.Scheme = "http"
			pr.Out.URL.Host = net.JoinHostPort("127.0.0.1", strconv.Itoa(g.Port))
			pr.Out.Host = pr.Out.URL.Host
			pr.SetURL(pr.Out.URL)
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			// No gateway file, or the gateway's handler has exited, or
			// there's a proxying error (e.g. disconnected).
			fmt.Fprintf(os.Stderr, "ReverseProxy ErrorHandler: %v\n", err)
			writeErr(w, http.StatusServiceUnavailable, noHandlerMsg)
		},
	}
	proxy.ServeHTTP(w, r)
}

// messages is the back face: ccw's entry point. It builds the Vertex-shaped path
// Dia's handler expects, attaches the credential of the current turn, and
// streams the reply back without parsing it.
func (b *Bridge) messages(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	var parsed map[string]any
	if json.Unmarshal(body, &parsed) != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	model, _ := parsed["model"].(string)
	if model == "" {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	// The handler pulls the model out of the path with ([^:]+), so a colon
	// here would land the request on a different endpoint.
	if strings.Contains(model, ":") {
		writeErr(w, http.StatusBadRequest, "model must not contain a colon")
		return
	}
	if !isSupportedDiaModel(model) {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("unsupported dia model %q: the gateway only serves %s", model, strings.Join(diaModels, ", ")))
		return
	}
	// No gateway means the last Dia turn ended, which took its listener with it.
	// Rather than turn the caller away, ask Dia for a new one and use it: the
	// wait sits inside this request, so the caller still sees one normal answer.
	g, err := b.liveGateway(r.Context())
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, noHandlerMsg)
		return
	}

	// The search runs first, on the bytes as the caller wrote them: it
	// recognises the client's own spelling of the search tool, and the
	// gateway refuses that tool outright, so ccw runs the search here rather
	// than sending a tool the handler turn would die on.
	body, renamed := b.prepare(r.Context(), body)
	// A fresh map: decoding into the old one would keep the keys prepare
	// removed, such as tools no Dia tool stands for.
	parsed = nil
	if json.Unmarshal(body, &parsed) != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	// Ensure Vertex AI Anthropic version is present in the payload.
	if _, ok := parsed["anthropic_version"]; !ok {
		parsed["anthropic_version"] = "vertex-2023-10-16"
	}
	reverse := reverseMapping(renamed)

	isStream, _ := parsed["stream"].(bool)
	// Dia's Bedrock upstream only serves streamRawPredict; non-streaming callers
	// are served by assembling the SSE chunks back into standard Messages JSON.
	parsed["stream"] = true

	outBody, err := json.Marshal(parsed)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "cannot marshal body")
		return
	}

	target := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort("127.0.0.1", strconv.Itoa(g.Port)),
		Path:   fmt.Sprintf(endpointPath, model),
	}
	out, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target.String(), bytes.NewReader(outBody))
	if err != nil {
		writeErr(w, http.StatusBadGateway, "cannot build handler request")
		return
	}
	out.Header.Set("Content-Type", "application/json")
	out.Header.Set("Authorization", bearer(g.Token))
	out.Header.Set("Anthropic-Version", "vertex-2023-10-16")
	// Dia's Bedrock endpoint does not take Anthropic-Beta headers and rejects
	// beta flags with an HTTP 400.
	resp, err := http.DefaultClient.Do(out)
	// A dial failure means the turn that built this gateway is over. Answering
	// 503 keeps ccw's failover moving: it retries 503 and moves to the next
	// account, while a 502 is neither retried nor a known-bad upstream.
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, noHandlerMsg)
		return
	}
	defer resp.Body.Close()

	if isStream {
		copyHeader(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		buf := make([]byte, 32*1024)
		flush := writerFlusher{w}
		// Tool names come back in Dia's spelling, so the stream is rewritten
		// line by line; every other byte crosses untouched.
		src := io.Reader(resp.Body)
		if len(reverse) > 0 {
			src = &restoredBody{body: resp.Body, reverse: reverse}
		}
		for {
			n, rerr := src.Read(buf)
			if n > 0 {
				if _, werr := w.Write(buf[:n]); werr != nil {
					return
				}
				flush.Flush()
			}
			if rerr != nil {
				return
			}
		}
	}

	// Non-streaming caller: assemble the SSE chunks into a standard Anthropic response.
	assembled, status, err := assembleSSE(resp, reverse)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "dia stream parse error: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(assembled)
}

// assembleSSE reads Anthropic SSE events from an HTTP response and packages
// them into a non-streaming Anthropic Messages response.
func assembleSSE(resp *http.Response, reverse map[string]string) ([]byte, int, error) {
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return raw, resp.StatusCode, nil
	}
	scanner := bufio.NewScanner(resp.Body)
	// The handler streams a Vertex-shaped reply in which content arrives as
	// indexed blocks: content_block_start names the type, each
	// content_block_delta carries a text_delta or a thinking_delta for that
	// index, and content_block_stop closes it. Dia's own request asks for
	// adaptive thinking, so blocks are reassembled in order rather than
	// flattened into one string.
	type block struct {
		kind      string
		toolName  string
		toolInput strings.Builder
		thinking  strings.Builder
		text      strings.Builder
		signature strings.Builder
	}
	var (
		msgID      string
		model      string
		role       = "assistant"
		stopReason = "end_turn"
		inTokens   int
		outTokens  int
		blocks     []*block
	)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		var payload struct {
			Type    string `json:"type"`
			Index   int    `json:"index"`
			Message struct {
				ID    string `json:"id"`
				Model string `json:"model"`
				Role  string `json:"role"`
				Usage struct {
					InputTokens int `json:"input_tokens"`
				} `json:"usage"`
			} `json:"message"`
			ContentBlock struct {
				Type string `json:"type"`
				Name string `json:"name"`
			} `json:"content_block"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				Thinking    string `json:"thinking"`
				Signature   string `json:"signature"`
				PartialJSON string `json:"partial_json"`
				StopReason  string `json:"stop_reason"`
			} `json:"delta"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(data), &payload) != nil {
			continue
		}
		switch payload.Type {
		case "message_start":
			msgID = payload.Message.ID
			model = payload.Message.Model
			if payload.Message.Role != "" {
				role = payload.Message.Role
			}
			inTokens = payload.Message.Usage.InputTokens
		case "content_block_start", "content_block_delta":
			for len(blocks) <= payload.Index {
				blocks = append(blocks, nil)
			}
			blk := blocks[payload.Index]
			if blk == nil {
				blk = &block{kind: "text"}
				blocks[payload.Index] = blk
			}
			if payload.Type == "content_block_start" {
				if kind := payload.ContentBlock.Type; kind != "" {
					blk.kind = kind
				}
				blk.toolName = payload.ContentBlock.Name
				continue
			}
			switch payload.Delta.Type {
			case "thinking_delta":
				blk.kind = "thinking"
				blk.thinking.WriteString(payload.Delta.Thinking)
			case "signature_delta":
				blk.signature.WriteString(payload.Delta.Signature)
			case "input_json_delta":
				blk.toolInput.WriteString(payload.Delta.PartialJSON)
			default:
				blk.text.WriteString(payload.Delta.Text)
			}
		case "message_delta":
			if payload.Delta.StopReason != "" {
				stopReason = payload.Delta.StopReason
			}
			if payload.Usage.OutputTokens != 0 {
				outTokens = payload.Usage.OutputTokens
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}
	content := make([]map[string]any, 0, len(blocks))
	for _, blk := range blocks {
		if blk == nil {
			continue
		}
		switch blk.kind {
		case "thinking":
			item := map[string]any{"type": "thinking", "thinking": blk.thinking.String()}
			if blk.signature.Len() > 0 {
				item["signature"] = blk.signature.String()
			}
			content = append(content, item)
		case "tool_use":
			name := blk.toolName
			if original, ok := reverse[name]; ok {
				name = original
			}
			// An upstream that stops mid-tool would leave invalid JSON, so a
			// block that never completed becomes an empty object rather than a
			// parse error the caller cannot act on.
			input := json.RawMessage(blk.toolInput.String())
			if len(input) == 0 || !json.Valid(input) {
				input = json.RawMessage("{}")
			}
			content = append(content, map[string]any{
				"type":  "tool_use",
				"id":    toolUseID(blk.toolName, len(content)),
				"name":  name,
				"input": input,
			})
		case "redacted_thinking":
			content = append(content, map[string]any{"type": "redacted_thinking", "data": ""})
		default:
			if blk.text.Len() == 0 {
				continue
			}
			content = append(content, map[string]any{"type": "text", "text": blk.text.String()})
		}
	}
	res := map[string]any{
		"id":          msgID,
		"type":        "message",
		"role":        role,
		"model":       model,
		"content":     content,
		"stop_reason": stopReason,
		"usage": map[string]int{
			"input_tokens":  inTokens,
			"output_tokens": outTokens,
		},
	}
	raw, err := json.Marshal(res)
	return raw, http.StatusOK, err
}

// toolUseID gives an assembled tool_use block an id when the stream did not
// carry one. Anthropic requires the field, and a caller matches the result back
// to this block by id, so it has to be stable for a given position.
func toolUseID(name string, index int) string {
	return fmt.Sprintf("toolu_ccw_%s_%d", name, index)
}

// health reports which gateway is live and which model last crossed the front
// hop, so one call says whether both halves of the bridge work. It answers 503
// when no gateway is live: the bridge itself is up, but it has nothing to talk
// to until Dia runs a chat turn.
func (b *Bridge) health(w http.ResponseWriter, r *http.Request) {
	port := 0
	g, err := b.gateway()
	if err == nil {
		port = g.Port
	}
	status := http.StatusOK
	if port == 0 {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]any{
		"ok":          port != 0,
		"handlerPort": port,
		"frontModel":  b.FrontModel(),
	})
}

// gateway reads the current port/token pair. It is re-read on every call because
// Dia regenerates both on each chat turn; caching either would leave ccw sending
// a credential the handler no longer accepts.
//
// A pair on file is not a live gateway: Dia's handler builds one gateway per
// chat turn and exits with that turn, so the file survives its listener. The
// port is dialled before the pair is trusted, which is a single local connect
// and keeps a stale file from reading as a working provider.
func (b *Bridge) gateway() (gateway, error) {
	g, err := readGateway(b.portFile)
	if err != nil {
		return gateway{}, err
	}
	if !listening(g.Port) {
		return gateway{}, errNoHandler
	}
	return g, nil
}

// liveGateway is the gateway a back-face request should use. When there is
// none, it asks Dia to build one and waits for it, so a caller arriving right
// after a turn ended is served instead of refused.
//
// The wait is bounded and cancellable: a caller that gives up, or a machine
// that cannot drive Dia's UI, still gets the ordinary 503 and the message that
// says what to type by hand.
func (b *Bridge) liveGateway(ctx context.Context) (gateway, error) {
	g, err := b.gateway()
	if err == nil {
		return g, nil
	}
	if b.recover.waitFor(ctx, b.portFile) {
		return b.gateway()
	}
	return gateway{}, errNoHandler
}

// readGateway parses the port/token pair the patched handler left behind. A
// file that is missing, unreadable, or not a complete pair means there is no
// gateway to talk to.
func readGateway(path string) (gateway, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return gateway{}, errNoHandler
	}
	var g gateway
	if json.Unmarshal(raw, &g) != nil || g.Port == 0 || g.Token == "" {
		return gateway{}, errNoHandler
	}
	return g, nil
}

// listening reports whether something still accepts a connection on a loopback
// port. Dia's handler holds the port only for the turn it built it for.
func listening(port int) bool {
	c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), probeTimeout)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// FrontModel is the model Dia's claude last asked for, empty when no turn has
// crossed the front hop yet.
func (b *Bridge) FrontModel() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastModel
}

// bearer turns the key the handler wrote into the header value it compares
// against. Handler matches `Bearer <key>` exactly, so any prefix is stripped
// and the canonical one put back: a lowercase or missing prefix would make the
// handler answer 401 on every call.
func bearer(token string) string {
	key := token
	if i := strings.IndexByte(key, ' '); i > 0 && strings.EqualFold(key[:i], "bearer") {
		key = strings.TrimLeft(key[i+1:], " ")
	}
	return "Bearer " + key
}

// portFilePath is the file the patched handler writes the gateway to.
func portFilePath() string {
	if p := os.Getenv(portFileEnv); p != "" {
		return p
	}
	return defaultPortFile
}

// Addr is the front address Dia's claude is pointed at.
func (b *Bridge) Addr() string { return b.frontAddr }

// BackAddr is the address ccw's dia provider calls.
func (b *Bridge) BackAddr() string { return b.backAddr }

// Close stops both listeners.
func (b *Bridge) Close() error {
	var first error
	for _, s := range []*http.Server{b.front, b.back} {
		if s == nil {
			continue
		}
		if err := s.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// writerFlusher flushes after every chunk so a streaming reply reaches ccw as it
// is produced instead of at the end.
type writerFlusher struct{ w http.ResponseWriter }

func (f writerFlusher) Flush() {
	if fl, ok := f.w.(http.Flusher); ok {
		fl.Flush()
	}
}

// copyHeader copies response headers, dropping the ones that describe a
// connection of the handler's own.
func copyHeader(dst, src http.Header) {
	for k, vs := range src {
		switch strings.ToLower(k) {
		case "connection", "keep-alive", "transfer-encoding", "content-length":
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

// writeJSON answers with a JSON body and status.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeErr answers with a JSON error body.
func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// diaModels are the models Dia's gateway serves on every hop of a turn, each
// measured by switching to it after another model had already run.
//
// claude-sonnet-5-5 is deliberately absent. The handler routes to Bedrock for
// the first model of a turn and to Vertex for every model switch after that,
// and Vertex answers 400 "not whitelisted for use-case [dia-browser]" for that
// one name. Offering it would mean a model that answers most turns and then
// fails the moment a session changes model.
var diaModels = []string{
	"claude-opus-5-5",
	"claude-opus-5",
	"claude-opus-4-8",
	"claude-sonnet-5",
	"claude-opus-4-6",
}

// isSupportedDiaModel reports whether model is whitelisted by Dia's Vertex backend.
func isSupportedDiaModel(m string) bool {
	return slices.Contains(diaModels, m)
}
