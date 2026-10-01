package httpapi

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/naniiluja/ccw/internal/drift"
	"github.com/naniiluja/ccw/internal/filter"
	"github.com/naniiluja/ccw/internal/provider"
	"github.com/naniiluja/ccw/internal/store"
	"github.com/naniiluja/ccw/internal/translate"
	"github.com/naniiluja/ccw/internal/usage"
)

// The tap keeps a bounded copy of one response to read the token counts. A
// stream carries its usage at the tail (OpenAI's final chunk, Anthropic's
// message_delta) while Anthropic's input count sits near the head, so the tap
// keeps both ends and drops the middle. This bounds memory on a large stream.
var (
	usageTapHeadLimit = 1 << 20
	usageTapTailLimit = 256 << 10
)

// hopByHop headers belong to a single transport hop and must not be forwarded.
var hopByHop = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

// newOutbound builds the upstream request for one connection: the target URL,
// the caller's headers minus this hop's, the provider identity and defaults, and
// the stored credential. Accept-Encoding is forced to identity so the usage tap
// always reads plain bytes.
func (a *api) newOutbound(r *http.Request, p provider.Provider, providerID, path, secret string, body io.Reader) (*http.Request, error) {
	base := p.BaseURL
	if over, ok := a.baseOverride[providerID]; ok {
		base = over
	}
	target := base + "/" + path
	// The caller's query belongs to the caller's endpoint. It goes upstream only
	// when the call stays on that endpoint: on a translated one it would land on
	// the provider's own query (Claude Code's ?beta=true after ?alt=sse).
	if r.URL.RawQuery != "" && path == r.PathValue("path") {
		target += "?" + r.URL.RawQuery
	}
	out, err := http.NewRequestWithContext(r.Context(), r.Method, target, body)
	if err != nil {
		return nil, err
	}
	for k, vs := range r.Header {
		// Authorization and X-Api-Key carry ccw's own token, which must never
		// reach a provider; the account's credential replaces them below. A clean
		// provider gets none of the caller's headers.
		if p.Clean || hopByHop[k] || k == "Authorization" || k == "X-Api-Key" || k == "Host" || k == "Accept-Encoding" {
			continue
		}
		for _, v := range vs {
			out.Header.Add(k, v)
		}
	}
	out.Header.Set("Accept-Encoding", "identity")
	for k, v := range p.Defaults {
		if out.Header.Get(k) == "" {
			out.Header.Set(k, v)
		}
	}
	for k, v := range p.Identity {
		out.Header.Set(k, v)
	}
	if p.RequestIDHeader != "" {
		out.Header.Set(p.RequestIDHeader, newRequestID())
	}
	if p.AccountHeader != "" {
		if id := chatgptAccountID(secret); id != "" {
			out.Header.Set(p.AccountHeader, id)
		}
	}
	if s := zenSessionOf(r); s != nil && s.id != "" {
		for _, h := range p.SessionHeaders {
			out.Header.Set(h, s.id)
		}
	}
	if p.SessionHeader != "" && out.Header.Get(p.SessionHeader) == "" {
		out.Header.Set(p.SessionHeader, sessionFor(providerID, secret))
	}
	// Header filters run after the defaults and the identity, so they can drop
	// one of those too; the credential itself is never dropped.
	for _, h := range filter.Headers(a.rulesFor(providerID)) {
		if h != http.CanonicalHeaderKey(p.AuthHeader) {
			out.Header.Del(h)
		}
	}
	if p.AuthHeader != "" {
		out.Header.Set(p.AuthHeader, p.AuthPrefix+secret)
	}
	return out, nil
}

// relayObserved relays a passthrough answer and shows it to the drift observer.
func (a *api) relayObserved(w http.ResponseWriter, resp *http.Response, connID, provider, path string) {
	a.rate.capture(connID, resp.Header)
	for k, vs := range resp.Header {
		if hopByHop[k] {
			continue
		}
		copyHeader(w.Header(), k, vs)
	}
	w.WriteHeader(resp.StatusCode)
	tapped := streamBody(w, resp)
	a.recordUsage(connID, keyIDOf(resp), tapped, resp.Header.Get("Content-Encoding"))
	if resp.StatusCode < 300 && resp.Header.Get("Content-Encoding") == "" && watched(provider) {
		a.drift.Observe(drift.Response, provider, path, tapped, isEventStream(resp, tapped))
	}
}

// isEventStream tells a stream from a whole body, by header or by content
// (Codex labels its stream application/json).
func isEventStream(resp *http.Response, body []byte) bool {
	if strings.Contains(resp.Header.Get("Content-Type"), "event-stream") {
		return true
	}
	return bytes.HasPrefix(body, []byte("event:")) || bytes.HasPrefix(body, []byte("data:"))
}

// recordUsage reads the token counts out of a response the proxy already sent
// and folds them into the daily counter. It never changes what the caller
// received, and a body with no usage adds no row.
//
// A client that sends Accept-Encoding: gzip gets a gzip body that Go does not
// auto-decompress, so the tap holds compressed bytes. The counter decompresses
// its own copy; the caller still gets the original bytes untouched.
func (a *api) recordUsage(connID, keyID string, body []byte, contentEncoding string) {
	if contentEncoding == "gzip" {
		if plain, err := gunzip(body); err == nil {
			body = plain
		} else {
			return
		}
	}
	c := usage.Parse(body)
	if !c.Found {
		return
	}
	day := usageDay(time.Now())
	// A failure must not affect the request the caller already has; the counter
	// is a convenience, not part of the proxy contract. Log it so a store fault
	// (a lock, a full disk) is visible instead of losing counts in silence.
	if err := a.store.AddUsage(day, connID, c.Model, c.InputTokens, c.OutputTokens); err != nil {
		log.Printf("record usage for connection %s: %v", connID, err)
	}
	if keyID != "" {
		if err := a.store.AddKeyUsage(day, keyID, c.Model, c.InputTokens, c.OutputTokens); err != nil {
			log.Printf("record usage for api key %s: %v", keyID, err)
		}
	}
}

// gunzip decompresses a gzip body, bounded so a hostile stream cannot exhaust
// memory through the tap.
func gunzip(b []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(io.LimitReader(zr, int64(usageTapHeadLimit+usageTapTailLimit)))
}

// streamBody copies the upstream body to the caller and flushes each chunk as it
// arrives. A plain io.Copy would let the server buffer, which holds a streamed
// response until the upstream finishes and breaks every SSE client.
//
// It returns a bounded copy of the body so the usage counter can be read without
// a second upstream call. The tap only reads; the caller's bytes are written
// first and are never altered by it.
func streamBody(w http.ResponseWriter, resp *http.Response) []byte {
	rc := http.NewResponseController(w)
	tap := &respTap{headLimit: usageTapHeadLimit, tailLimit: usageTapTailLimit}
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return tap.bytes()
			}
			tap.write(buf[:n])
			// A flush failure only means this writer cannot flush; keep copying.
			_ = rc.Flush()
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				// A stream cut in the middle still reaches the caller as a 200
				// with a short body, so only this line says why it stopped.
				log.Printf("upstream stream ended early: %v", readErr)
			}
			return tap.bytes()
		}
	}
}

// respTap keeps the head and the tail of a response and drops the middle. Usage
// lives at one end or the other, so both ends together carry it while memory
// stays bounded to headLimit + tailLimit.
type respTap struct {
	head      bytes.Buffer
	tail      []byte
	headLimit int
	tailLimit int
	total     int
}

func (t *respTap) write(p []byte) {
	t.total += len(p)
	if room := t.headLimit - t.head.Len(); room > 0 {
		if room >= len(p) {
			t.head.Write(p)
		} else {
			t.head.Write(p[:room])
		}
	}
	t.tail = append(t.tail, p...)
	if len(t.tail) > t.tailLimit {
		t.tail = t.tail[len(t.tail)-t.tailLimit:]
	}
}

// bytes returns the captured body. When the response fit inside the head, that
// is the whole body. Otherwise it joins head and tail with a newline; a line
// split across the gap fails to parse and is skipped, which is harmless because
// usage is a whole line at one end.
func (t *respTap) bytes() []byte {
	if t.total <= t.headLimit {
		return t.head.Bytes()
	}
	out := make([]byte, 0, t.head.Len()+1+len(t.tail))
	out = append(out, t.head.Bytes()...)
	out = append(out, '\n')
	out = append(out, t.tail...)
	return out
}

func (a *api) connection(id string) (store.Connection, error) {
	list, err := a.store.ListConnections()
	if err != nil {
		return store.Connection{}, err
	}
	for _, c := range list {
		if c.ID == id {
			return c, nil
		}
	}
	return store.Connection{}, errors.New("not found")
}

// newRequestID returns a random UUIDv4 for a per-request id header.
func newRequestID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// chatgptAccountID reads the ChatGPT account id from an OpenAI access token,
// a JWT whose "https://api.openai.com/auth" claim carries it.
func chatgptAccountID(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Auth struct {
			AccountID string `json:"chatgpt_account_id"`
		} `json:"https://api.openai.com/auth"`
	}
	if json.Unmarshal(raw, &claims) != nil {
		return ""
	}
	return claims.Auth.AccountID
}

// sessionFor is a stable session id per provider and credential, so the
// upstream can keep its prompt cache across a conversation's calls.
func sessionFor(providerID, secret string) string {
	h := sha256.Sum256([]byte(providerID + "\x00" + secret))
	b := h[:16]
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// keyIDOf names the API key behind a response: the outbound request carries
// the caller's context, and with it the caller.
func keyIDOf(resp *http.Response) string {
	if resp == nil || resp.Request == nil {
		return ""
	}
	return principalOf(resp.Request).keyID
}

// copyHeader relays one upstream header. A request id replaces ccw's own
// instead of adding a second value.
func copyHeader(h http.Header, k string, vs []string) {
	if slices.Contains(requestIDHeaders, k) {
		h[k] = append([]string(nil), vs...)
		return
	}
	for _, v := range vs {
		h.Add(k, v)
	}
}

// A request reaches a provider in the provider's own shape. When the caller
// speaks the same shape, the bytes pass through untouched both ways; that is
// the point of ccw. The one edit is the model id, which ccw stripped of
// its provider prefix on the way in and puts back on the way out (replyEditor). Only when the shapes differ is the request translated,
// with OpenAI Chat Completions as the hub: caller → chat → provider on the way
// in, provider → chat chunks → caller on the way out.

// clientShape names the shape of a caller's request from its path, or "" for a
// path ccw forwards untouched (embeddings, …).
func clientShape(path string) string {
	switch strings.Trim(path, "/") {
	case "chat/completions":
		return translate.OpenAI
	case "messages":
		return translate.Anthropic
	}
	return ""
}

// shapeOf names the request shape a provider speaks by default.
func shapeOf(p provider.Provider) string {
	if p.API == "" {
		return translate.OpenAI
	}
	return p.API
}

// copilotResponsesModels are the Copilot models learned to answer only on
// /responses: Copilot says so with a 400, and the call is retried there.
var copilotResponsesModels sync.Map

var copilotClaude = regexp.MustCompile(`(?i)claude`)

// shapeFor names the shape and endpoint path a provider takes for one model.
func shapeFor(p provider.Provider, model string) (shape, path string) {
	if p.Exchange == "copilot" {
		if copilotClaude.MatchString(model) {
			return translate.Anthropic, "v1/messages"
		}
		if _, ok := copilotResponsesModels.Load(model); ok {
			return translate.Responses, "responses"
		}
	}
	switch s := shapeOf(p); s {
	case translate.Antigravity:
		return s, "v1internal:streamGenerateContent?alt=sse"
	case translate.Anthropic:
		return s, "messages"
	case translate.Responses:
		return s, "responses"
	case translate.OpenAI:
		return s, "chat/completions"
	default:
		return s, ""
	}
}

// translatable reports whether ccw can reach a provider shape from a caller.
func translatable(shape string) bool {
	switch shape {
	case translate.OpenAI, translate.Anthropic, translate.Responses, translate.Antigravity:
		return true
	}
	return false
}

// toProvider converts a caller's request to the provider's shape.
func toProvider(body []byte, client, want string) ([]byte, error) {
	hub := body
	var err error
	switch client {
	case translate.Anthropic:
		hub, err = translate.AnthropicToOpenAI(body)
	}
	if err != nil {
		return nil, err
	}
	switch want {
	case translate.Anthropic:
		return translate.OpenAIToAnthropic(hub)
	case translate.Responses:
		return translate.OpenAIToResponses(hub)
	}
	return hub, nil
}

// alwaysStreams reports whether a provider shape is only ever called streaming.
func alwaysStreams(shape string) bool {
	switch shape {
	case translate.Responses, translate.Antigravity, translate.Zen, translate.ZenResponses:
		return true
	}
	return false
}

// maxTranslatedBody bounds a whole (non-streamed) response read for translation.
const maxTranslatedBody = 32 << 20

// relayVia answers the caller in its own shape (to) from a response in the
// provider's shape (via). stream is what the caller asked for; reply says what
// a Messages caller's answer must carry.
func (a *api) relayVia(w http.ResponseWriter, resp *http.Response, connID, to, via string, stream bool, reply translate.Reply, provider, path string) {
	a.rate.capture(connID, resp.Header)
	for k, vs := range resp.Header {
		if hopByHop[k] || k == "Content-Length" || k == "Content-Type" || k == "Content-Encoding" {
			continue
		}
		copyHeader(w.Header(), k, vs)
	}
	// Codex (the provider) streams its events under a Content-Type of application/json, so a
	// shape that only ever streams is read as a stream whatever the header says.
	isSSE := strings.Contains(resp.Header.Get("Content-Type"), "event-stream") || alwaysStreams(via)
	if resp.StatusCode >= 400 || !isSSE {
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxTranslatedBody))
		if err != nil {
			writeError(w, http.StatusBadGateway, "cannot read the upstream answer")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// An error body under 200 is still an error: the caller's SDK would
		// otherwise read it as an answer.
		status := resp.StatusCode
		if status < 400 && isErrorBody(body) {
			status = http.StatusBadGateway
		}
		if via == translate.Zen || via == translate.ZenResponses {
			status, body = translate.ZenStatus(status, body)
		}
		if status >= 400 {
			w.WriteHeader(status)
			w.Write(translate.Error(status, body, to))
			return
		}
		out, err := wholeToClient(body, via, to, reply)
		if err != nil {
			writeError(w, http.StatusBadGateway, "cannot translate the upstream answer")
			return
		}
		w.WriteHeader(resp.StatusCode)
		w.Write(out)
		a.recordUsage(connID, keyIDOf(resp), body, "")
		if watched(provider) {
			a.drift.Observe(drift.Response, provider, path, body, false)
		}
		return
	}

	// Stream: provider events → chat chunks (in a goroutine) → the caller.
	// The provider's own bytes are tapped too, for the drift observer.
	raw := &respTap{headLimit: usageTapHeadLimit, tailLimit: usageTapTailLimit}
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		a.toChatChunks(pipeFlusher{pw}, io.TeeReader(resp.Body, tapWriter{raw}), via)
	}()
	tap := &respTap{headLimit: usageTapHeadLimit, tailLimit: usageTapTailLimit}
	chunks := io.TeeReader(pr, tapWriter{tap})
	out := flushWriter{w: w, rc: http.NewResponseController(w)}
	switch {
	case stream && to == translate.OpenAI:
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(resp.StatusCode)
		copyFlushing(out, chunks)
	case stream:
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(resp.StatusCode)
		translate.OpenAIStreamToAnthropic(out, chunks, reply)
	default:
		whole := collectChunks(via, chunks)
		status := resp.StatusCode
		// A provider can stream an error event inside an HTTP 200. The collected
		// body is then an error, not an answer, so the caller must not see 200.
		if status < 400 && isErrorBody(whole) {
			status = http.StatusBadGateway
		}
		switch {
		case to == translate.Anthropic:
			if b, err := translate.OpenAIResponseToAnthropic(whole, reply); err == nil {
				whole = b
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(whole)
	}
	// Close the read end rather than drain it. On a normal finish the reader is
	// already at EOF; on a client disconnect a drain would block on the producer
	// still reading a slow upstream, so close it and let the deferred
	// resp.Body.Close release that goroutine.
	pr.Close()
	a.recordUsage(connID, keyIDOf(resp), tap.bytes(), "")
	if resp.StatusCode < 300 && watched(provider) {
		a.drift.Observe(drift.Response, provider, path, raw.bytes(), true)
	}
}

// isErrorBody reports whether a JSON body is an error envelope: a top-level
// "error" key, which a provider streams instead of a completion when it fails.
func isErrorBody(b []byte) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(b, &m) != nil {
		return false
	}
	_, ok := m["error"]
	return ok
}

// toChatChunks converts a provider's event stream to Chat Completions chunks.
func (a *api) toChatChunks(dst translate.Flusher, src io.Reader, via string) {
	switch via {
	case translate.Antigravity:
		translate.GeminiStreamToOpenAI(dst, src, &a.sigs)
	case translate.Anthropic:
		translate.AnthropicStreamToOpenAI(dst, src)
	case translate.Responses:
		translate.ResponsesStreamToOpenAI(dst, src)
	case translate.Zen:
		translate.ZenStreamToOpenAI(dst, src)
	case translate.ZenResponses:
		translate.ZenResponsesStreamToOpenAI(dst, src)
	default:
		io.Copy(dst, src)
	}
}

// wholeToClient converts a whole (non-streamed) response.
func wholeToClient(body []byte, via, to string, reply translate.Reply) ([]byte, error) {
	hub := body
	if via == translate.Anthropic {
		var err error
		if hub, err = translate.AnthropicResponseToOpenAI(body); err != nil {
			return nil, err
		}
	}
	switch to {
	case translate.Anthropic:
		return translate.OpenAIResponseToAnthropic(hub, reply)
	}
	return hub, nil
}

func copyFlushing(dst flushWriter, src io.Reader) error {
	buf := make([]byte, 32<<10)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return werr
			}
			dst.Flush()
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// pipeFlusher lets a stream converter write into a pipe.
type pipeFlusher struct{ w *io.PipeWriter }

func (p pipeFlusher) Write(b []byte) (int, error) { return p.w.Write(b) }
func (p pipeFlusher) Flush() error                { return nil }

// tapWriter feeds a respTap from a TeeReader.
type tapWriter struct{ t *respTap }

func (t tapWriter) Write(p []byte) (int, error) { t.t.write(p); return len(p), nil }

// flushWriter writes to the caller and flushes each write, so a translated
// stream reaches the caller as it is produced.
type flushWriter struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

func (f flushWriter) Write(p []byte) (int, error) { return f.w.Write(p) }

// Flush ignores a writer that cannot flush; the bytes still arrive at the end.
func (f flushWriter) Flush() error { f.rc.Flush(); return nil }

// anyAnthropic reports whether one of the accounts speaks the Anthropic shape.
func (a *api) anyAnthropic(targets []store.Connection) bool {
	for _, c := range targets {
		if p, ok := a.providerFor(c); ok && shapeOf(p) == translate.Anthropic {
			return true
		}
	}
	return false
}

// countTokensEstimate answers Anthropic's count_tokens for a provider that has
// no such endpoint, with the usual four characters per token. Clients use it to
// size a context, so an estimate serves them better than an error.
func countTokensEstimate(w http.ResponseWriter, body []byte) {
	writeJSON(w, map[string]any{"input_tokens": translate.EstimateTokens(body)})
}

// collectChunks folds chat chunks into one answer. The free tier's own answer
// carries more than the plain fold keeps (reasoning under every name, kept
// fields), so it has its own.
func collectChunks(via string, chunks io.Reader) []byte {
	if via == translate.Zen || via == translate.ZenResponses {
		return translate.CollectZenStream(chunks)
	}
	return translate.CollectOpenAIStream(chunks)
}
