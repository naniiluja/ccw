package translate

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

func newID(prefix string) string {
	b := make([]byte, 12)
	rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

func num(v any) int64 {
	switch n := v.(type) {
	case json.Number:
		i, _ := n.Int64()
		return i
	case float64:
		return int64(n)
	}
	return 0
}

// ---- errors

// anthropicErrorTypes are the error types the Messages API defines.
var anthropicErrorTypes = map[string]bool{
	"invalid_request_error": true, "authentication_error": true, "permission_error": true,
	"not_found_error": true, "request_too_large": true, "rate_limit_error": true, "api_error": true,
	"timeout_error": true, "overloaded_error": true, "billing_error": true,
}

// openaiErrorTypes maps the types OpenAI-shaped upstreams write to Anthropic's.
var openaiErrorTypes = map[string]string{
	"rate_limit_exceeded": "rate_limit_error", "requests": "rate_limit_error", "tokens": "rate_limit_error",
	"server_error": "api_error", "insufficient_quota": "billing_error", "invalid_api_key": "authentication_error",
}

// AnthropicErrorType names the Messages error type for an upstream type and an
// HTTP status; the status decides when the type is not one Anthropic knows.
func AnthropicErrorType(status int, typ string) string {
	if anthropicErrorTypes[typ] {
		return typ
	}
	if t := openaiErrorTypes[typ]; t != "" {
		return t
	}
	switch {
	case status == 401:
		return "authentication_error"
	case status == 402:
		return "billing_error"
	case status == 403:
		return "permission_error"
	case status == 404:
		return "not_found_error"
	case status == 413:
		return "request_too_large"
	case status == 429:
		return "rate_limit_error"
	case status == 504:
		return "timeout_error"
	case status == 529 || status == 503:
		return "overloaded_error"
	case status >= 400 && status < 500:
		return "invalid_request_error"
	}
	return "api_error"
}

// Error converts an upstream error body to the caller's shape. A body that is
// not a recognised error is wrapped as the message.
func Error(status int, body []byte, to string) []byte {
	msg, typ, code := string(body), "", any(nil)
	if m, err := decode(body); err == nil {
		switch e := m["error"].(type) {
		case obj:
			if s := str(e["message"]); s != "" {
				msg = s
			}
			typ = str(e["type"])
			if s := str(e["code"]); s != "" {
				code = s
			}
		case string:
			msg = e
		}
	}
	if to == Anthropic {
		b, _ := json.Marshal(obj{"type": "error", "error": obj{"type": AnthropicErrorType(status, typ), "message": msg}})
		return b
	}
	if typ == "" {
		typ = "invalid_request_error"
		if status >= 500 {
			typ = "server_error"
		}
	}
	b, _ := json.Marshal(obj{"error": obj{"message": msg, "type": typ, "param": nil, "code": code}})
	return b
}

// ---- whole responses

var anthropicStop = map[string]string{
	"end_turn": "stop", "stop_sequence": "stop", "max_tokens": "length",
	"tool_use": "tool_calls", "pause_turn": "stop", "refusal": "content_filter",
}

var openaiStop = map[string]string{
	"stop": "end_turn", "length": "max_tokens", "tool_calls": "tool_use",
	"function_call": "tool_use", "content_filter": "refusal",
}

// AnthropicResponseToOpenAI converts a Messages response to a Chat Completion.
func AnthropicResponseToOpenAI(body []byte) ([]byte, error) {
	in, err := decode(body)
	if err != nil {
		return nil, err
	}
	var text, thinking strings.Builder
	var calls []any
	for _, b := range list(in["content"]) {
		blk := asObj(b)
		switch str(blk["type"]) {
		case "text":
			text.WriteString(str(blk["text"]))
		case "thinking":
			thinking.WriteString(str(blk["thinking"]))
		case "tool_use":
			args, _ := json.Marshal(blk["input"])
			calls = append(calls, obj{"id": blk["id"], "type": "function",
				"function": obj{"name": blk["name"], "arguments": string(args)}})
		}
	}
	msg := obj{"role": "assistant", "content": text.String()}
	if thinking.Len() > 0 {
		msg["reasoning_content"] = thinking.String()
	}
	if len(calls) > 0 {
		msg["tool_calls"] = calls
	}
	finish := anthropicStop[str(in["stop_reason"])]
	if finish == "" {
		finish = "stop"
	}
	out := obj{
		"id": "chatcmpl-" + strings.TrimPrefix(str(in["id"]), "msg_"), "object": "chat.completion",
		"created": time.Now().Unix(), "model": in["model"],
		"choices": []any{obj{"index": 0, "message": msg, "finish_reason": finish}},
	}
	if u := asObj(in["usage"]); u != nil {
		out["usage"] = openaiUsage(u)
	}
	return json.Marshal(out)
}

// openaiUsage folds Anthropic's cache counters into the prompt total, which is
// how OpenAI reports them, and keeps the cached part as a detail.
func openaiUsage(u obj) obj {
	cached := num(u["cache_read_input_tokens"])
	in := num(u["input_tokens"]) + cached + num(u["cache_creation_input_tokens"])
	outTok := num(u["output_tokens"])
	o := obj{"prompt_tokens": in, "completion_tokens": outTok, "total_tokens": in + outTok}
	if cached > 0 {
		o["prompt_tokens_details"] = obj{"cached_tokens": cached}
	}
	return o
}

// Reply is what the caller's request says about the Messages answer it expects.
type Reply struct {
	// Model is the id the caller sent. Empty keeps the upstream's.
	Model string
	// InputTokens goes into message_start: an OpenAI stream reports usage
	// only at its end, and clients size their context from message_start.
	InputTokens int64
	// Thinking is set when the caller enabled thinking. Only then does the
	// upstream's reasoning leave as thinking blocks, as the Messages API does.
	Thinking bool
	// Search is the search ccw ran for the caller, when it asked for one. Its
	// blocks open the answer.
	Search *WebSearch
}

// ReplyFor reads a Messages request for what its answer must carry. model is
// the id the caller sent, before ccw removed its provider prefix.
func ReplyFor(body []byte, model string) Reply {
	r := Reply{Model: model, InputTokens: EstimateTokens(body)}
	if in, err := decode(body); err == nil {
		switch str(asObj(in["thinking"])["type"]) {
		case "enabled", "adaptive":
			r.Thinking = true
		}
	}
	return r
}

// imageTokens is what one image or document adds to an estimate.
const imageTokens = 1600

// EstimateTokens counts a request at four characters per token, the estimate
// count_tokens answers with for a provider that cannot count. Base64 image data
// is not text, so an image is a fixed cost instead of a quarter of its bytes: a
// screenshot must not read as a full context and trigger a compaction.
func EstimateTokens(body []byte) int64 {
	in, err := decode(body)
	if err != nil {
		return int64(len(body)/4 + 1)
	}
	var chars, images int64
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			chars += int64(utf8.RuneCountInString(x))
		case []any:
			for _, e := range x {
				walk(e)
			}
		case obj:
			if t := str(x["type"]); t == "image" || t == "image_url" || t == "document" {
				images++
				return
			}
			for k, e := range x {
				if k != "data" && k != "cache_control" {
					walk(e)
				}
			}
		}
	}
	for _, k := range []string{"system", "messages", "tools"} {
		walk(in[k])
	}
	return (chars+3)/4 + images*imageTokens
}

func (r Reply) model(upstream any) any {
	if r.Model != "" {
		return r.Model
	}
	return upstream
}

// messageID keeps the upstream id under Anthropic's prefix, and mints one when
// the upstream sent none.
func messageID(id string) string {
	if id = strings.TrimPrefix(id, "chatcmpl-"); id == "" {
		return newID("msg_")
	}
	return "msg_" + id
}

func toolID(id string) string {
	if id == "" {
		return newID("toolu_")
	}
	return id
}

// reasoningOf reads the thinking of a delta or message under any of the names
// OpenAI-shaped upstreams use.
func reasoningOf(m obj) string {
	if s := str(m["reasoning_content"]); s != "" {
		return s
	}
	if s := str(m["reasoning"]); s != "" {
		return s
	}
	var b strings.Builder
	for _, d := range list(m["reasoning_details"]) {
		b.WriteString(str(asObj(d)["text"]))
	}
	return b.String()
}

// textOf reads message content given as a string or as text parts.
func textOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	var b strings.Builder
	for _, p := range list(v) {
		b.WriteString(str(asObj(p)["text"]))
	}
	return b.String()
}

// OpenAIResponseToAnthropic converts a Chat Completion to a Messages response.
func OpenAIResponseToAnthropic(body []byte, r Reply) ([]byte, error) {
	in, err := decode(body)
	if err != nil {
		return nil, err
	}
	// An error payload is not a message with no content: convert it to an
	// Anthropic error so the caller sees the failure, not an empty success.
	if e := asObj(in["error"]); e != nil {
		return json.Marshal(obj{"type": "error", "error": obj{"type": AnthropicErrorType(0, str(e["type"])), "message": str(e["message"])}})
	}
	content := []any{}
	if r.Search != nil {
		call, result := r.Search.blocks()
		content = append(content, call, result)
	}
	stop := "end_turn"
	if ch := asObj(firstOf(in["choices"])); ch != nil {
		msg := asObj(ch["message"])
		tagged, said := splitThinkTags(textOf(msg["content"]))
		if t := reasoningOf(msg) + tagged; t != "" && r.Thinking {
			content = append(content, obj{"type": "thinking", "thinking": t, "signature": ""})
		}
		if t := said + str(msg["refusal"]); t != "" {
			content = append(content, obj{"type": "text", "text": t})
		}
		calls := list(msg["tool_calls"])
		if fc := asObj(msg["function_call"]); fc != nil {
			calls = append(calls, obj{"function": fc})
		}
		for _, tc := range calls {
			t := asObj(tc)
			fn := asObj(t["function"])
			content = append(content, obj{"type": "tool_use", "id": toolID(str(t["id"])), "name": fn["name"],
				"input": parseArgs(str(fn["arguments"]))})
		}
		if s := openaiStop[str(ch["finish_reason"])]; s != "" {
			stop = s
		}
		if stop == "tool_use" && len(calls) == 0 {
			stop = "end_turn"
		}
	}
	out := obj{
		"id": messageID(str(in["id"])), "type": "message", "role": "assistant",
		"model": r.model(in["model"]), "content": content, "stop_reason": stop, "stop_sequence": nil,
		"usage": withSearchUsage(anthropicUsage(asObj(in["usage"])), r.Search),
	}
	return json.Marshal(out)
}

// anthropicUsage splits OpenAI's prompt total into Anthropic's three input
// counters, which do not overlap.
func anthropicUsage(u obj) obj {
	details := asObj(u["prompt_tokens_details"])
	cached, written := num(details["cached_tokens"]), num(details["cache_write_tokens"])
	o := obj{"input_tokens": max(num(u["prompt_tokens"])-cached-written, 0), "output_tokens": num(u["completion_tokens"])}
	if cached > 0 {
		o["cache_read_input_tokens"] = cached
	}
	if written > 0 {
		o["cache_creation_input_tokens"] = written
	}
	return o
}

func firstOf(v any) any {
	if l := list(v); len(l) > 0 {
		return l[0]
	}
	return nil
}

// ---- streams

// Flusher is the part of an http.ResponseWriter a stream converter needs.
type Flusher interface {
	io.Writer
	Flush() error
}

// sseReader yields the data payloads of a Server-Sent-Events stream, with the
// event name when the stream sets one.
// sseEvents returns nil when the stream ended cleanly (EOF, or the callback
// signalled a stop by returning false) and the scanner's error when the stream
// was cut short (io.ErrUnexpectedEOF, a read timeout, or a line past the
// buffer). A caller must not synthesize a clean finish on a non-nil error: a
// truncated stream is not a completed one.
func sseEvents(r io.Reader, each func(event, data string) bool) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	var event string
	var data []string
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if len(data) > 0 && !each(event, strings.Join(data, "\n")) {
				return nil
			}
			event, data = "", nil
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(line[len("event:"):])
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(line[len("data:"):], " "))
		}
	}
	if len(data) > 0 {
		each(event, strings.Join(data, "\n"))
	}
	return sc.Err()
}

// AnthropicStreamToOpenAI reads a Messages event stream and writes the
// equivalent Chat Completions chunks.
func AnthropicStreamToOpenAI(dst Flusher, src io.Reader) {
	id, model := newID("chatcmpl-"), ""
	created := time.Now().Unix()
	toolIndex := map[int64]int{} // content block index -> tool call index
	var usage obj
	finish := ""
	emit := func(delta obj, fin any, extra obj) {
		ch := obj{"index": 0, "delta": delta, "finish_reason": fin}
		c := obj{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []any{ch}}
		for k, v := range extra {
			c[k] = v
		}
		b, _ := json.Marshal(c)
		io.WriteString(dst, "data: "+string(b)+"\n\n")
		dst.Flush()
	}
	err := sseEvents(src, func(_, data string) bool {
		ev, err := decode([]byte(data))
		if err != nil {
			return true
		}
		switch str(ev["type"]) {
		case "message_start":
			m := asObj(ev["message"])
			model = str(m["model"])
			usage = asObj(m["usage"])
			emit(obj{"role": "assistant", "content": ""}, nil, nil)
		case "content_block_start":
			blk := asObj(ev["content_block"])
			if str(blk["type"]) == "tool_use" {
				i := len(toolIndex)
				toolIndex[num(ev["index"])] = i
				emit(obj{"tool_calls": []any{obj{"index": i, "id": blk["id"], "type": "function",
					"function": obj{"name": blk["name"], "arguments": ""}}}}, nil, nil)
			}
		case "content_block_delta":
			d := asObj(ev["delta"])
			switch str(d["type"]) {
			case "text_delta":
				emit(obj{"content": d["text"]}, nil, nil)
			case "thinking_delta":
				emit(obj{"reasoning_content": d["thinking"]}, nil, nil)
			case "input_json_delta":
				emit(obj{"tool_calls": []any{obj{"index": toolIndex[num(ev["index"])],
					"function": obj{"arguments": d["partial_json"]}}}}, nil, nil)
			}
		case "message_delta":
			if s := anthropicStop[str(asObj(ev["delta"])["stop_reason"])]; s != "" {
				finish = s
			}
			if u := asObj(ev["usage"]); u != nil {
				if usage == nil {
					usage = obj{}
				}
				for k, v := range u {
					usage[k] = v
				}
			}
		case "message_stop":
			if finish == "" {
				finish = "stop"
			}
			var extra obj
			if usage != nil {
				extra = obj{"usage": openaiUsage(usage)}
			}
			emit(obj{}, finish, extra)
			io.WriteString(dst, "data: [DONE]\n\n")
			dst.Flush()
			return false
		case "error":
			b, _ := json.Marshal(obj{"error": asObj(ev["error"])})
			io.WriteString(dst, "data: "+string(b)+"\n\n")
			dst.Flush()
			return false
		}
		return true
	})
	if err != nil {
		b, _ := json.Marshal(obj{"error": obj{"type": "api_error", "message": "upstream stream ended early: " + err.Error()}})
		io.WriteString(dst, "data: "+string(b)+"\n\n")
		dst.Flush()
	}
}

// OpenAIStreamToAnthropic reads Chat Completions chunks and writes the
// equivalent Messages event stream.
func OpenAIStreamToAnthropic(dst Flusher, src io.Reader, r Reply) {
	w := &anthropicStream{dst: dst, r: r, block: -1, tools: map[int64]*toolAcc{}, lastTool: -1}
	err := sseEvents(src, w.event)
	if w.done {
		return
	}
	// A stream that ended without [DONE] and without a finish reason was cut:
	// signal the error instead of a clean stop, so the caller does not treat
	// truncated text or a half-streamed tool call as a finished answer.
	if err != nil || w.stop == "" {
		msg := "upstream stream ended early"
		if err != nil {
			msg += ": " + err.Error()
		}
		w.start("", "")
		w.fail("api_error", msg)
		return
	}
	w.finish()
}

// toolAcc accumulates one streamed tool call.
type toolAcc struct{ id, name, args string }

// anthropicStream is the state of one OpenAIStreamToAnthropic run: whether
// message_start went out, the open content block, and the buffered tool calls.
type anthropicStream struct {
	dst       Flusher
	r         Reply
	started   bool
	done      bool
	block     int    // index of the open content block, -1 when none
	blockKind string // "thinking" or "text"
	next      int    // index of the next content block
	// Tool calls are buffered by their OpenAI index and emitted as whole blocks
	// at the end. Anthropic allows only one open content block at a time, so
	// streaming a second tool call would close the first and drop the argument
	// deltas that still arrive for it.
	tools     map[int64]*toolAcc
	toolOrder []int64
	lastTool  int64
	stop      string
	usage     obj
	// The reasoning some models write into the content joins the reasoning
	// field: a client that did not ask for thinking sees neither.
	split thinkSplitter
}

// send writes one Messages event and flushes it.
func (w *anthropicStream) send(event string, v obj) {
	b, _ := json.Marshal(v)
	io.WriteString(w.dst, "event: "+event+"\ndata: "+string(b)+"\n\n")
	w.dst.Flush()
}

// fail writes an error event.
func (w *anthropicStream) fail(typ, msg string) {
	w.send("error", obj{"type": "error", "error": obj{"type": typ, "message": msg}})
}

// start writes message_start and ping once, then the search blocks if any.
func (w *anthropicStream) start(id, model string) {
	if w.started {
		return
	}
	w.started = true
	w.send("message_start", obj{"type": "message_start", "message": obj{
		"id": messageID(id), "type": "message", "role": "assistant",
		"model": w.r.model(model), "content": []any{}, "stop_reason": nil, "stop_sequence": nil,
		"usage": obj{"input_tokens": w.r.InputTokens, "output_tokens": 0}}})
	w.send("ping", obj{"type": "ping"})
	if w.r.Search != nil {
		w.searchBlocks()
	}
}

// searchBlocks writes the search ccw ran as the first blocks of the answer.
func (w *anthropicStream) searchBlocks() {
	call, result := w.r.Search.blocks()
	// The hosted tool's call streams its input like any tool use; its
	// result arrives whole.
	input, _ := json.Marshal(asObj(call["input"]))
	call["input"] = obj{}
	w.open("search", call)
	w.send("content_block_delta", obj{"type": "content_block_delta", "index": w.block,
		"delta": obj{"type": "input_json_delta", "partial_json": string(input)}})
	w.closeBlock()
	w.open("search_result", result)
	w.closeBlock()
}

// closeBlock ends the open content block, if there is one.
func (w *anthropicStream) closeBlock() {
	if w.block < 0 {
		return
	}
	if w.blockKind == "thinking" {
		w.send("content_block_delta", obj{"type": "content_block_delta", "index": w.block,
			"delta": obj{"type": "signature_delta", "signature": ""}})
	}
	w.send("content_block_stop", obj{"type": "content_block_stop", "index": w.block})
	w.block, w.blockKind = -1, ""
}

// open closes the open block and starts the next one.
func (w *anthropicStream) open(kind string, cb obj) {
	w.closeBlock()
	w.block, w.blockKind = w.next, kind
	w.next++
	w.send("content_block_start", obj{"type": "content_block_start", "index": w.block, "content_block": cb})
}

// delta writes d into a block of the given kind, opening one when the open
// block is of another kind.
func (w *anthropicStream) delta(kind string, cb, d obj) {
	if w.blockKind != kind {
		w.open(kind, cb)
	}
	w.send("content_block_delta", obj{"type": "content_block_delta", "index": w.block, "delta": d})
}

// emitThink writes reasoning as a thinking delta when the caller asked for it.
func (w *anthropicStream) emitThink(t string) {
	if w.r.Thinking {
		w.delta("thinking", obj{"type": "thinking", "thinking": "", "signature": ""},
			obj{"type": "thinking_delta", "thinking": t})
	}
}

// emitText writes a text delta.
func (w *anthropicStream) emitText(t string) {
	w.delta("text", obj{"type": "text", "text": ""}, obj{"type": "text_delta", "text": t})
}

// tool finds the accumulator for one tool call fragment. A fragment with
// no index belongs to the call before it, unless it names a new id.
func (w *anthropicStream) tool(tc obj) *toolAcc {
	ti := num(tc["index"])
	if tc["index"] == nil {
		ti = max(w.lastTool, 0)
		if id := str(tc["id"]); id != "" && w.tools[ti] != nil && w.tools[ti].id != "" && w.tools[ti].id != id {
			ti = int64(len(w.toolOrder))
		}
	}
	w.lastTool = ti
	ta := w.tools[ti]
	if ta == nil {
		ta = &toolAcc{}
		w.tools[ti] = ta
		w.toolOrder = append(w.toolOrder, ti)
	}
	return ta
}

// finish writes the buffered tool calls, message_delta and message_stop.
func (w *anthropicStream) finish() {
	w.start("", "")
	w.split.flush(w.emitThink, w.emitText)
	w.closeBlock()
	for _, ti := range w.toolOrder {
		ta := w.tools[ti]
		w.open("tool", obj{"type": "tool_use", "id": toolID(ta.id), "name": ta.name, "input": obj{}})
		// The whole-body path reads arguments with parseArgs; a stream that
		// sent broken JSON gets the same object, not a parse error later.
		args, _ := json.Marshal(parseArgs(ta.args))
		w.send("content_block_delta", obj{"type": "content_block_delta", "index": w.block,
			"delta": obj{"type": "input_json_delta", "partial_json": string(args)}})
	}
	w.closeBlock()
	if w.stop == "" || (w.stop == "tool_use" && len(w.toolOrder) == 0) {
		w.stop = "end_turn"
	}
	u := obj{"output_tokens": 0}
	if w.usage != nil {
		u = anthropicUsage(w.usage)
	}
	u = withSearchUsage(u, w.r.Search)
	w.send("message_delta", obj{"type": "message_delta", "delta": obj{"stop_reason": w.stop, "stop_sequence": nil}, "usage": u})
	w.send("message_stop", obj{"type": "message_stop"})
}

// event handles one SSE data payload. It returns false once the stream is
// over: at [DONE] or at an upstream error.
func (w *anthropicStream) event(_, data string) bool {
	if strings.TrimSpace(data) == "[DONE]" {
		w.finish()
		w.done = true
		return false
	}
	ch, err := decode([]byte(data))
	if err != nil {
		return true
	}
	if e := asObj(ch["error"]); e != nil {
		w.start("", "")
		w.fail(AnthropicErrorType(0, str(e["type"])), str(e["message"]))
		w.done = true
		return false
	}
	w.start(str(ch["id"]), str(ch["model"]))
	if u := asObj(ch["usage"]); u != nil {
		w.usage = u
	}
	if c := asObj(firstOf(ch["choices"])); c != nil {
		w.choice(c)
	}
	return true
}

// choice writes the delta of the first choice of a chunk.
func (w *anthropicStream) choice(c obj) {
	d := asObj(c["delta"])
	if t := reasoningOf(d); t != "" && w.r.Thinking {
		w.delta("thinking", obj{"type": "thinking", "thinking": "", "signature": ""},
			obj{"type": "thinking_delta", "thinking": t})
	}
	w.split.push(str(d["content"]), w.emitThink, w.emitText)
	if t := str(d["refusal"]); t != "" {
		w.emitText(t)
	}
	calls := list(d["tool_calls"])
	if fc := asObj(d["function_call"]); fc != nil {
		calls = append(calls, obj{"index": json.Number("0"), "function": fc})
	}
	for _, raw := range calls {
		tc := asObj(raw)
		ta := w.tool(tc)
		fn := asObj(tc["function"])
		if id := str(tc["id"]); id != "" {
			ta.id = id
		}
		if n := str(fn["name"]); n != "" {
			ta.name = n
		}
		ta.args += str(fn["arguments"])
	}
	if s := openaiStop[str(c["finish_reason"])]; s != "" {
		w.stop = s
	}
}
