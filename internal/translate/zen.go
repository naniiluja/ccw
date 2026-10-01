package translate

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"time"
)

// The two dialects the OpenCode Zen free tier answers in, both with habits a
// strict client does not expect: a frame after [DONE], bare hex ids,
// delta.name, a cost field, usage: null on every chunk, and the finish reason on
// the last content chunk. Whatever the dialect, the answer leaves through one
// conformer, so the caller sees the shape of the official OpenAI OpenAPI
// document (CreateChatCompletionStreamResponse) whatever the model.
const (
	// Zen is the free tier's Chat Completions.
	Zen = "zen"
	// ZenResponses is the free tier's Responses API, which some models answer
	// only on.
	ZenResponses = "zen-responses"
)

// The rules of the answer shape. Every Zen model maps onto them the same way, so
// they are constants: a field the shape does not name stays out, so a new
// upstream field never reaches a client until it is named here.
var (
	zenFinishes = map[string]bool{"stop": true, "length": true, "tool_calls": true, "content_filter": true, "function_call": true}
	// zenFinishAliases turn the upstream's own reasons into the shape's.
	zenFinishAliases = map[string]string{"max_tokens": "length", "max_output_tokens": "length", "tool_use": "tool_calls"}
	zenRoles         = map[string]bool{"developer": true, "system": true, "user": true, "assistant": true, "tool": true}
	// zenThinkingIn are the delta fields the thinking arrives in; it leaves under
	// every name of zenThinkingOut, because clients read one or the other
	// (OpenCode writes reasoning, most OpenAI-compatible clients read
	// reasoning_content).
	zenThinkingIn  = []string{"reasoning", "reasoning_content"}
	zenThinkingOut = []string{"reasoning", "reasoning_content"}
	// zenKeep are extra delta fields, arrays of items, kept as they are.
	zenKeep      = []string{"reasoning_details"}
	zenTextKeys  = []string{"content", "refusal"}
	zenUsageKeys = []string{"prompt_tokens", "completion_tokens", "total_tokens"}
	zenUsageSub  = map[string][]string{
		"prompt_tokens_details":     {"cached_tokens", "audio_tokens"},
		"completion_tokens_details": {"reasoning_tokens", "audio_tokens", "accepted_prediction_tokens", "rejected_prediction_tokens"},
	}
)

const zenIDPrefix = "chatcmpl-"

// ZenStreamToOpenAI reads the free tier's chat stream and writes conformed
// Chat Completions chunks, then [DONE]. Nothing after the upstream's [DONE] is
// read. A comment frame (the upstream's keep-alive) passes as it came, so a long
// thinking phase still moves bytes. A stream that stops with neither [DONE] nor
// a finish reason was cut, and ends in an error instead of passing for complete.
func ZenStreamToOpenAI(dst Flusher, src io.Reader) {
	s := &zenStream{opened: map[int64]bool{}, finish: map[int64]string{}, tools: map[int64]bool{}}
	write := func(v any) {
		b, _ := json.Marshal(v)
		io.WriteString(dst, "data: "+string(b)+"\n\n")
		dst.Flush()
	}
	done := false
	sc := bufio.NewScanner(src)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	var data []string
	frame := func() bool { // reports whether to read on
		payload := strings.Join(data, "\n")
		data = nil
		if strings.TrimSpace(payload) == "[DONE]" {
			done = true
			return false
		}
		ch, err := decode([]byte(payload))
		if err != nil {
			return true
		}
		if e := asObj(ch["error"]); e != nil {
			write(zenError(str(e["message"]), str(e["type"])))
			s.failed = true
			return false
		}
		for _, out := range s.feed(ch) {
			write(out)
		}
		return true
	}
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if len(data) > 0 && !frame() {
				goto end
			}
		case strings.HasPrefix(line, ":") && len(data) == 0:
			io.WriteString(dst, line+"\n\n")
			dst.Flush()
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(line[len("data:"):], " "))
		}
	}
	if len(data) > 0 {
		frame()
	}
end:
	if !s.failed {
		for _, out := range s.close(done) {
			write(out)
		}
	}
	io.WriteString(dst, "data: [DONE]\n\n")
	dst.Flush()
}

// ZenResponsesStreamToOpenAI does the same for the Responses API: its events
// become chat chunks first, then leave through the same conformer.
func ZenResponsesStreamToOpenAI(dst Flusher, src io.Reader) {
	pr, pw := io.Pipe()
	defer pr.Close()
	go func() {
		defer pw.Close()
		ResponsesStreamToOpenAI(pipeFlush{pw}, src)
	}()
	ZenStreamToOpenAI(dst, pr)
}

type pipeFlush struct{ *io.PipeWriter }

func (pipeFlush) Flush() error { return nil }

func zenError(message, kind string) obj {
	if message == "" {
		message = "the upstream response failed"
	}
	if kind == "" {
		kind = "server_error"
	}
	return obj{"error": obj{"message": message, "type": kind, "param": nil, "code": nil}}
}

// zenStream turns upstream chat chunks into conformed ones, one at a time. The
// finish reason is held back and sent alone at close, after the last content:
// a client that stops reading at the finish would lose the text of the chunk
// that carried it.
type zenStream struct {
	id, model string
	created   int64
	usage     obj
	opened    map[int64]bool
	finish    map[int64]string
	tools     map[int64]bool
	failed    bool
}

func (s *zenStream) chunk(choices []any) obj {
	if s.id == "" {
		s.id = newID(zenIDPrefix)
	}
	if s.created == 0 {
		s.created = time.Now().Unix()
	}
	return obj{"id": s.id, "object": "chat.completion.chunk", "created": s.created, "model": s.model, "choices": choices}
}

func (s *zenStream) feed(raw obj) []obj {
	if s.id == "" {
		if id := str(raw["id"]); id != "" {
			s.id = id
			if !strings.HasPrefix(id, zenIDPrefix) {
				s.id = zenIDPrefix + id
			}
		}
	}
	if s.model == "" {
		s.model = str(raw["model"])
	}
	if s.created == 0 {
		if c := num(raw["created"]); c > 0 {
			s.created = c
		}
	}
	if u := zenUsage(asObj(raw["usage"])); u != nil {
		s.usage = u
	}
	var choices []any
	for _, c := range list(raw["choices"]) {
		ch := asObj(c)
		if ch == nil {
			continue
		}
		idx := num(ch["index"])
		delta := zenDelta(asObj(ch["delta"]))
		finish := zenFinish(str(ch["finish_reason"]))
		if finish != "" {
			s.finish[idx] = finish
		}
		if delta["tool_calls"] != nil {
			s.tools[idx] = true
		}
		if s.opened[idx] {
			delete(delta, "role")
			if len(delta) == 0 {
				continue
			}
		} else {
			if len(delta) == 0 && finish == "" {
				continue
			}
			delta["role"] = "assistant"
			s.opened[idx] = true
		}
		choices = append(choices, obj{"index": idx, "delta": delta, "finish_reason": nil})
	}
	if len(choices) == 0 {
		return nil
	}
	return []obj{s.chunk(choices)}
}

func (s *zenStream) close(done bool) []obj {
	if !done && len(s.finish) == 0 {
		return []obj{zenError("the upstream stream ended before the answer was complete", "server_error")}
	}
	var out []obj
	if len(s.opened) == 0 {
		s.opened[0] = true
		out = append(out, s.chunk([]any{obj{"index": 0, "delta": obj{"role": "assistant", "content": ""}, "finish_reason": nil}}))
	}
	var finishes []any
	for idx := int64(0); len(finishes) < len(s.opened) && idx < 1<<16; idx++ {
		if !s.opened[idx] {
			continue
		}
		reason := s.finish[idx]
		if reason == "" {
			reason = "stop"
		}
		if s.tools[idx] && reason == "stop" {
			reason = "tool_calls"
		}
		finishes = append(finishes, obj{"index": idx, "delta": obj{}, "finish_reason": reason})
	}
	out = append(out, s.chunk(finishes))
	if s.usage != nil {
		u := s.chunk([]any{})
		u["usage"] = s.usage
		out = append(out, u)
	}
	return out
}

func zenFinish(raw string) string {
	switch {
	case raw == "":
		return ""
	case zenFinishes[raw]:
		return raw
	}
	if a := zenFinishAliases[raw]; a != "" {
		return a
	}
	return "stop"
}

// zenDelta keeps the fields of a delta that the shape names.
func zenDelta(raw obj) obj {
	out := obj{}
	if raw == nil {
		return out
	}
	if r := str(raw["role"]); zenRoles[r] {
		out["role"] = r
	}
	for _, k := range zenTextKeys {
		if s, ok := raw[k].(string); ok {
			out[k] = s
		}
	}
	var found string
	haveFound := false
	for _, k := range zenThinkingIn {
		if s, ok := raw[k].(string); ok {
			found, haveFound = s, true
			break
		}
	}
	if haveFound {
		// A name the upstream sent itself keeps its own value; the others
		// take the first thinking it sent.
		for _, k := range zenThinkingOut {
			if s, ok := raw[k].(string); ok {
				out[k] = s
			} else {
				out[k] = found
			}
		}
	}
	var calls []any
	for _, c := range list(raw["tool_calls"]) {
		if call := zenToolCall(asObj(c)); call != nil {
			calls = append(calls, call)
		}
	}
	if len(calls) > 0 {
		out["tool_calls"] = calls
	}
	for _, k := range zenKeep {
		if l := list(raw[k]); len(l) > 0 {
			out[k] = l
		}
	}
	return out
}

func zenToolCall(raw obj) obj {
	if raw == nil {
		return nil
	}
	out := obj{"index": num(raw["index"])}
	if id := str(raw["id"]); id != "" {
		out["id"] = id
	}
	if str(raw["type"]) == "function" {
		out["type"] = "function"
	}
	if fn := asObj(raw["function"]); fn != nil {
		kept := obj{}
		for _, k := range []string{"name", "arguments"} {
			if s, ok := fn[k].(string); ok {
				kept[k] = s
			}
		}
		if len(kept) > 0 {
			out["function"] = kept
		}
	}
	return out
}

// zenUsage keeps the token counts the shape names. It is nil unless the three
// totals are all whole numbers.
func zenUsage(raw obj) obj {
	if raw == nil {
		return nil
	}
	out := obj{}
	for _, k := range zenUsageKeys {
		n, ok := raw[k].(json.Number)
		if !ok {
			return nil
		}
		if _, err := n.Int64(); err != nil {
			return nil
		}
		out[k] = n
	}
	for group, keys := range zenUsageSub {
		details := asObj(raw[group])
		kept := obj{}
		for _, k := range keys {
			if n, ok := details[k].(json.Number); ok {
				if _, err := n.Int64(); err == nil {
					kept[k] = n
				}
			}
		}
		if len(kept) > 0 {
			out[group] = kept
		}
	}
	return out
}

// CollectZenStream folds the conformed chunks of a Zen answer into one
// chat.completion, for a caller who asked for a whole body: it must not be able
// to tell that the upstream call was streamed anyway, so everything the stream
// carried is folded in (text, reasoning under every name, kept fields, tool calls,
// finish reason, usage). Choices are folded by index. A stream that ended in an
// error, or carried no answer, comes back as an error body.
func CollectZenStream(src io.Reader) []byte {
	z := &zenCollector{choices: map[int64]*zenChoice{}}
	err := sseEvents(src, z.event)
	if z.failure != nil {
		b, _ := json.Marshal(obj{"error": z.failure})
		return b
	}
	if len(z.choices) == 0 {
		msg := "the stream carried no answer"
		if err != nil {
			msg += ": " + err.Error()
		}
		b, _ := json.Marshal(zenError(msg, "server_error"))
		return b
	}
	var outChoices []any
	for idx := int64(0); len(outChoices) < len(z.choices) && idx < 1<<16; idx++ {
		if cc := z.choices[idx]; cc != nil {
			outChoices = append(outChoices, cc.result(idx))
		}
	}
	id, created := z.id, z.created
	if id == "" {
		id = newID(zenIDPrefix)
	}
	if created == 0 {
		created = time.Now().Unix()
	}
	out := obj{"id": id, "object": "chat.completion", "created": created, "model": z.model, "choices": outChoices}
	if z.usage != nil {
		out["usage"] = z.usage
	}
	b, _ := json.Marshal(out)
	return b
}

// zenCollector is the state CollectZenStream folds the chunks into.
type zenCollector struct {
	choices   map[int64]*zenChoice
	id, model string
	created   int64
	usage     obj
	failure   obj
}

// zenChoice is one choice of a collected Zen answer.
type zenChoice struct {
	text      strings.Builder
	reasoning strings.Builder
	kept      map[string][]obj
	tools     map[int64]*obj
	finish    string
	gotText   bool
}

// event folds one SSE data payload. It returns false at [DONE] and at an
// error, which ends the answer.
func (z *zenCollector) event(_, data string) bool {
	if strings.TrimSpace(data) == "[DONE]" {
		return false
	}
	ch, err := decode([]byte(data))
	if err != nil {
		return true
	}
	if e := asObj(ch["error"]); e != nil {
		z.failure = e
		return false
	}
	if z.id == "" {
		z.id = str(ch["id"])
	}
	if z.model == "" {
		z.model = str(ch["model"])
	}
	if z.created == 0 {
		z.created = num(ch["created"])
	}
	if u := asObj(ch["usage"]); u != nil {
		z.usage = u
	}
	for _, c := range list(ch["choices"]) {
		cm := asObj(c)
		z.choice(num(cm["index"])).fold(cm)
	}
	return true
}

// choice returns the choice at idx, creating it on first use.
func (z *zenCollector) choice(idx int64) *zenChoice {
	cc := z.choices[idx]
	if cc == nil {
		cc = &zenChoice{kept: map[string][]obj{}, tools: map[int64]*obj{}}
		z.choices[idx] = cc
	}
	return cc
}

// fold adds one chunk's choice to the collected choice.
func (cc *zenChoice) fold(cm obj) {
	d := asObj(cm["delta"])
	if s, ok := d["content"].(string); ok {
		cc.text.WriteString(s)
		cc.gotText = true
	}
	if s, ok := d[zenThinkingOut[0]].(string); ok {
		cc.reasoning.WriteString(s)
	}
	for _, k := range zenKeep {
		for _, item := range list(d[k]) {
			if it := asObj(item); it != nil {
				cc.keep(k, it)
			}
		}
	}
	for _, raw := range list(d["tool_calls"]) {
		cc.toolFragment(asObj(raw))
	}
	if f := str(cm["finish_reason"]); f != "" {
		cc.finish = f
	}
}

// keep adds an item of the kept field k. Fragments repeat the shape of their
// item on every chunk, so items are stitched under their own index.
func (cc *zenChoice) keep(k string, it obj) {
	if at, hasAt := it["index"]; hasAt {
		for _, prev := range cc.kept[k] {
			if prev["index"] == at {
				if t, ok := it["text"].(string); ok {
					prev["text"] = str(prev["text"]) + t
				}
				return
			}
		}
	}
	cc.kept[k] = append(cc.kept[k], it)
}

// toolFragment adds one tool call fragment to the call at its index.
func (cc *zenChoice) toolFragment(tc obj) {
	at := num(tc["index"])
	slot := cc.tools[at]
	if slot == nil {
		slot = &obj{"id": "", "type": "function", "function": obj{"name": "", "arguments": ""}}
		cc.tools[at] = slot
	}
	if s := str(tc["id"]); s != "" {
		(*slot)["id"] = s
	}
	fn := asObj(tc["function"])
	sf := asObj((*slot)["function"])
	sf["name"] = str(sf["name"]) + str(fn["name"])
	sf["arguments"] = str(sf["arguments"]) + str(fn["arguments"])
}

// result is the collected choice as a chat.completion choice at idx.
func (cc *zenChoice) result(idx int64) obj {
	msg := obj{"role": "assistant", "content": cc.text.String(), "refusal": nil}
	if cc.reasoning.Len() > 0 {
		for _, k := range zenThinkingOut {
			msg[k] = cc.reasoning.String()
		}
	}
	for k, items := range cc.kept {
		if len(items) > 0 {
			msg[k] = items
		}
	}
	if len(cc.tools) > 0 {
		var calls []any
		for i := int64(0); i <= 1<<16 && len(calls) < len(cc.tools); i++ {
			if t := cc.tools[i]; t != nil {
				calls = append(calls, *t)
			}
		}
		msg["tool_calls"] = calls
		if cc.text.Len() == 0 {
			msg["content"] = nil
		}
	}
	var finish any
	if cc.finish != "" {
		finish = cc.finish
	}
	return obj{"index": idx, "message": msg, "finish_reason": finish, "logprobs": nil}
}

// ZenStatus reads the free tier's refusal of an unknown model. The upstream
// answers it with 401 ModelError, which a client reads as a bad key; OpenAI's own
// answer is 404 model_not_found. Any other answer is returned as it came.
func ZenStatus(status int, body []byte) (int, []byte) {
	if status != 401 {
		return status, body
	}
	m, err := decode(body)
	e := asObj(m["error"])
	if err != nil || e == nil || str(e["type"]) != "ModelError" {
		return status, body
	}
	b, _ := json.Marshal(obj{"error": obj{"message": str(e["message"]), "type": "invalid_request_error", "code": "model_not_found"}})
	return 404, b
}
