package translate

import (
	"encoding/json"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Gemini is Google's generateContent shape, which Antigravity (Cloud Code
// Assist, v1internal) carries inside its own envelope.
const Gemini = "gemini"

// Antigravity is the Gemini shape inside Cloud Code Assist's envelope.
const Antigravity = "antigravity"

// DummyThoughtSignature is the value Google documents for a function call
// replayed without the signature the model gave it (for example a history
// written by another model). The model then skips signature validation.
const DummyThoughtSignature = "skip_thought_signature_validator"

// MaxGeminiOutputTokens is Antigravity's output cap; a larger value is refused.
const MaxGeminiOutputTokens = 64000

// Signatures looks up and records the thought signature of a function call by
// its id, so a later turn can return it with the call.
type Signatures interface {
	Get(callID string) string
	Put(callID, sig string)
}

// OpenAIToGemini converts a Chat Completions request to the inner Gemini
// request (contents, systemInstruction, generationConfig, tools).
func OpenAIToGemini(body []byte, sigs Signatures) ([]byte, error) {
	in, err := decode(body)
	if err != nil {
		return nil, err
	}
	out := obj{}
	contents, system := geminiContents(list(in["messages"]), sigs)
	if len(contents) == 0 || contents[0]["role"] != "user" {
		contents = append([]obj{{"role": "user", "parts": []any{obj{"text": "..."}}}}, contents...)
	}
	out["contents"] = contents
	if len(system) > 0 {
		out["systemInstruction"] = obj{"role": "user", "parts": []any{obj{"text": strings.Join(system, "\n\n")}}}
	}
	if gen := geminiGenerationConfig(in); len(gen) > 0 {
		out["generationConfig"] = gen
	}
	geminiTools(in, out)
	return json.Marshal(out)
}

// geminiContents converts OpenAI messages to Gemini contents, merging adjacent
// turns of one role, and returns the system texts apart.
func geminiContents(messages []any, sigs Signatures) ([]obj, []string) {
	var system []string
	var contents []obj
	names := map[string]string{} // tool call id -> function name
	add := func(role string, parts []any) {
		if len(parts) == 0 {
			return
		}
		if n := len(contents); n > 0 && contents[n-1]["role"] == role {
			contents[n-1]["parts"] = append(contents[n-1]["parts"].([]any), parts...)
			return
		}
		contents = append(contents, obj{"role": role, "parts": parts})
	}
	for _, raw := range messages {
		m := asObj(raw)
		switch str(m["role"]) {
		case "system", "developer":
			if t := contentText(m["content"]); t != "" {
				system = append(system, t)
			}
		case "user":
			add("user", geminiParts(m["content"]))
		case "assistant":
			parts := []any{}
			if t := contentText(m["content"]); t != "" {
				parts = append(parts, obj{"text": t})
			}
			for i, tc := range list(m["tool_calls"]) {
				t := asObj(tc)
				fn := asObj(t["function"])
				// The name must match the sanitized declaration name, or Gemini
				// rejects the call: a function named "get weather" is declared as
				// "get_weather", so the call and its response use that form too.
				id, name := str(t["id"]), geminiName(str(fn["name"]))
				names[id] = name
				part := obj{"functionCall": obj{"id": id, "name": name, "args": parseArgs(str(fn["arguments"]))}}
				// The first call of a turn carries the turn's signature.
				if sig := sigs.Get(id); sig != "" {
					part["thoughtSignature"] = sig
				} else if i == 0 {
					part["thoughtSignature"] = DummyThoughtSignature
				}
				parts = append(parts, part)
			}
			add("model", parts)
		case "tool":
			id := str(m["tool_call_id"])
			text := contentText(m["content"])
			var result any = obj{"result": text}
			if v := parseArgs(text); len(asObj(v)) > 0 {
				result = obj{"result": v}
			}
			add("user", []any{obj{"functionResponse": obj{"id": id, "name": names[id], "response": result}}})
		}
	}
	return contents, system
}

// geminiGenerationConfig builds generationConfig: sampling, stop sequences,
// thinking, the output token cap thinking depends on, and the JSON response
// format. It returns an empty object when the request sets none of them.
func geminiGenerationConfig(in obj) obj {
	model := str(in["model"])
	gen := obj{}
	if v, ok := in["temperature"]; ok {
		gen["temperature"] = v
	}
	if v, ok := in["top_p"]; ok {
		gen["topP"] = v
	}
	maxOut := num(in["max_tokens"])
	if maxOut == 0 {
		maxOut = num(in["max_completion_tokens"])
	}
	if maxOut > MaxGeminiOutputTokens {
		maxOut = MaxGeminiOutputTokens
	}
	switch s := in["stop"].(type) {
	case string:
		gen["stopSequences"] = []any{s}
	case []any:
		gen["stopSequences"] = s
	}
	if e := str(in["reasoning_effort"]); e != "" && strings.Contains(strings.ToLower(model), "claude") {
		// Claude behind Antigravity takes a token budget, not a level.
		if budget := claudeThinkingBudget(e); budget > 0 {
			gen["thinkingConfig"] = obj{"includeThoughts": true, "thinkingBudget": budget}
			// The budget is spent from the output tokens and must stay below them.
			if maxOut <= budget {
				maxOut = min(budget+4096, MaxGeminiOutputTokens)
			}
		}
	} else if e != "" {
		level := map[string]string{"none": "minimal", "minimal": "minimal", "low": "low", "medium": "medium", "high": "high", "xhigh": "high", "max": "high"}[e]
		if level == "" {
			level = "medium"
		}
		gen["thinkingConfig"] = obj{"thinkingLevel": level, "includeThoughts": level != "minimal"}
		// Thinking spends output tokens, so leave room for the answer.
		floor := map[string]int64{"minimal": 4096, "low": 8192, "medium": 16384, "high": MaxGeminiOutputTokens}[level]
		if maxOut < floor {
			maxOut = floor
		}
	}
	if maxOut > 0 {
		gen["maxOutputTokens"] = maxOut
	}
	if rf := asObj(in["response_format"]); str(rf["type"]) == "json_schema" || str(rf["type"]) == "json_object" {
		gen["responseMimeType"] = "application/json"
		if s := asObj(asObj(rf["json_schema"])["schema"]); s != nil {
			gen["responseSchema"] = CleanGeminiSchema(s)
		}
	}
	return gen
}

// geminiTools sets tools and toolConfig from the function tools of a request,
// one declaration per sanitized name, and leaves both out when none is left.
func geminiTools(in, out obj) {
	var decls []any
	seen := map[string]bool{}
	for _, t := range list(in["tools"]) {
		fn := asObj(asObj(t)["function"])
		name := geminiName(str(fn["name"]))
		if fn == nil || name == "" || seen[name] {
			continue
		}
		seen[name] = true
		d := obj{"name": name, "parameters": CleanGeminiSchema(fn["parameters"])}
		if s := str(fn["description"]); s != "" {
			d["description"] = s
		}
		decls = append(decls, d)
	}
	if len(decls) == 0 {
		return
	}
	out["tools"] = []any{obj{"functionDeclarations": decls}}
	fc := obj{"mode": "VALIDATED"}
	switch tc := in["tool_choice"].(type) {
	case string:
		if tc == "required" {
			fc["mode"] = "ANY"
		} else if tc == "none" {
			fc["mode"] = "NONE"
		}
	case obj:
		// The object form names one tool. Without allowedFunctionNames the
		// model stays free to answer in text.
		if name := geminiName(str(asObj(tc["function"])["name"])); name != "" {
			fc["mode"] = "ANY"
			fc["allowedFunctionNames"] = []any{name}
		}
	}
	out["toolConfig"] = obj{"functionCallingConfig": fc}
}

var geminiNameRE = regexp.MustCompile(`[^a-zA-Z0-9_.:-]`)

// geminiName makes a function name Gemini accepts: [a-zA-Z_][a-zA-Z0-9_.:-]{0,63}.
func geminiName(n string) string {
	n = geminiNameRE.ReplaceAllString(n, "_")
	if n != "" && !(n[0] == '_' || (n[0] >= 'a' && n[0] <= 'z') || (n[0] >= 'A' && n[0] <= 'Z')) {
		n = "_" + n
	}
	if len(n) > 64 {
		n = n[:64]
	}
	return n
}

// geminiParts converts OpenAI user content to Gemini parts.
func geminiParts(c any) []any {
	switch v := c.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []any{obj{"text": v}}
	case []any:
		var out []any
		for _, p := range v {
			part := asObj(p)
			switch str(part["type"]) {
			case "text", "input_text":
				if t := str(part["text"]); t != "" {
					out = append(out, obj{"text": t})
				}
			case "image_url":
				u := str(asObj(part["image_url"])["url"])
				if u == "" {
					u = str(part["image_url"])
				}
				if rest, ok := strings.CutPrefix(u, "data:"); ok {
					meta, data, found := strings.Cut(rest, ",")
					mime, _ := strings.CutSuffix(meta, ";base64")
					if found {
						out = append(out, obj{"inlineData": obj{"mimeType": mime, "data": data}})
					}
				} else if u != "" {
					out = append(out, obj{"fileData": obj{"fileUri": u, "mimeType": "image/*"}})
				}
			}
		}
		return out
	}
	return nil
}

// geminiDropKeys are JSON Schema keywords the Gemini schema has no field for;
// one of them anywhere rejects the whole request.
var geminiDropKeys = map[string]bool{}

func init() {
	for _, k := range strings.Fields(`minLength maxLength exclusiveMinimum exclusiveMaximum minItems maxItems
		format multipleOf uniqueItems contains unevaluatedProperties unevaluatedItems contentSchema
		prefixItems additionalItems default examples $schema $defs definitions $ref $comment $id
		deprecated readOnly writeOnly additionalProperties propertyNames patternProperties enumDescriptions
		not dependencies dependentSchemas dependentRequired title optional if then else
		contentMediaType contentEncoding strict encrypted cache_control example`) {
		geminiDropKeys[k] = true
	}
}

// CleanGeminiSchema rewrites a JSON Schema into the subset Gemini accepts:
// const becomes enum, anyOf/oneOf keep their best branch, allOf is merged,
// a type list keeps its first non-null type, and unsupported keywords go.
func CleanGeminiSchema(v any) any {
	s := asObj(v)
	if s == nil {
		return obj{"type": "object", "properties": obj{"reason": obj{"type": "string"}}, "required": []any{"reason"}}
	}
	return cleanSchema(s)
}

func cleanSchema(s obj) obj {
	foldCombinators(s)
	out := obj{}
	for k, val := range s {
		cleanSchemaKey(out, k, val)
	}
	finishSchema(out)
	return out
}

// foldCombinators merges anyOf and oneOf (their best branch) and allOf (every
// branch) into s itself; a key s already has wins over a branch's.
func foldCombinators(s obj) {
	for _, k := range []string{"anyOf", "oneOf"} {
		if branches := list(s[k]); len(branches) > 0 {
			best := pickBranch(branches)
			for bk, bv := range best {
				if _, has := s[bk]; !has {
					s[bk] = bv
				}
			}
		}
	}
	for _, b := range list(s["allOf"]) {
		for bk, bv := range asObj(b) {
			if bk == "properties" {
				props := asObj(s["properties"])
				if props == nil {
					props = obj{}
				}
				for pk, pv := range asObj(bv) {
					props[pk] = pv
				}
				s["properties"] = props
			} else if _, has := s[bk]; !has {
				s[bk] = bv
			}
		}
	}
}

// cleanSchemaKey writes the Gemini form of one schema keyword to out, or
// nothing for a combinator or a keyword Gemini has no field for.
func cleanSchemaKey(out obj, k string, val any) {
	switch {
	case k == "anyOf" || k == "oneOf" || k == "allOf" || geminiDropKeys[k] || strings.HasPrefix(k, "x-"):
	case k == "const":
		out["enum"] = []any{stringify(val)}
	case k == "enum":
		var e []any
		for _, x := range list(val) {
			if x != nil {
				e = append(e, stringify(x))
			}
		}
		// An empty enum must not become "enum": null, which Gemini rejects.
		if len(e) > 0 {
			out["enum"] = e
		}
	case k == "type":
		if l := list(val); l != nil {
			for _, t := range l {
				if t != "null" {
					out["type"] = t
					break
				}
			}
		} else {
			out["type"] = val
		}
	case k == "properties":
		props := obj{}
		for pk, pv := range asObj(val) {
			if ps := asObj(pv); ps != nil {
				props[pk] = cleanSchema(ps)
			}
		}
		out["properties"] = props
	case k == "items":
		if l := list(val); l != nil {
			if len(l) > 0 {
				out["items"] = cleanSchema(asObj(l[0]))
			}
		} else if is := asObj(val); is != nil {
			out["items"] = cleanSchema(is)
		}
	default:
		out[k] = val
	}
}

// finishSchema fills what Gemini requires once every keyword is in: a type,
// items for an array, and a property for an object, whose required list
// keeps only the properties it has.
func finishSchema(out obj) {
	if out["type"] == nil && out["properties"] != nil {
		out["type"] = "object"
	}
	if out["enum"] != nil {
		out["type"] = "string"
	}
	if out["type"] == "array" && out["items"] == nil {
		out["items"] = obj{"type": "string"}
	}
	if out["type"] != "object" {
		return
	}
	props := asObj(out["properties"])
	if len(props) == 0 {
		out["properties"] = obj{"reason": obj{"type": "string"}}
		out["required"] = []any{"reason"}
		return
	}
	if req := list(out["required"]); req != nil {
		var keep []any
		for _, r := range req {
			if _, ok := props[str(r)]; ok {
				keep = append(keep, r)
			}
		}
		if keep == nil {
			delete(out, "required")
		} else {
			out["required"] = keep
		}
	}
}

// pickBranch prefers an object branch, then an array, then any non-null one.
func pickBranch(branches []any) obj {
	var best obj
	rank := func(b obj) int {
		switch str(b["type"]) {
		case "object":
			return 3
		case "array":
			return 2
		case "null":
			return 0
		}
		return 1
	}
	for _, b := range branches {
		if bo := asObj(b); bo != nil && (best == nil || rank(bo) > rank(best)) {
			best = bo
		}
	}
	return best
}

func stringify(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

var geminiFinish = map[string]string{
	"STOP": "stop", "MAX_TOKENS": "length", "SAFETY": "content_filter", "RECITATION": "content_filter",
	"BLOCKLIST": "content_filter", "PROHIBITED_CONTENT": "content_filter", "SPII": "content_filter",
}

// GeminiStreamToOpenAI reads a Gemini (or Antigravity-wrapped) event stream
// and writes Chat Completions chunks. Thought signatures of function calls are
// recorded so the next turn can send them back.
func GeminiStreamToOpenAI(dst Flusher, src io.Reader, sigs Signatures) {
	g := &geminiStream{dst: dst, sigs: sigs, id: newID("chatcmpl-"), created: time.Now().Unix()}
	streamErr := sseEvents(src, g.event)
	if streamErr != nil {
		// A truncated stream must not close as a clean answer.
		b, _ := json.Marshal(obj{"error": obj{"message": "upstream stream ended early: " + streamErr.Error(), "type": "api_error"}})
		io.WriteString(dst, "data: "+string(b)+"\n\n")
		dst.Flush()
		return
	}
	if !g.started {
		g.emit(obj{"role": "assistant", "content": ""}, nil, nil)
	}
	finish := g.finish
	if finish == "" || (finish == "stop" && g.calls > 0) {
		if g.calls > 0 {
			finish = "tool_calls"
		} else {
			finish = "stop"
		}
	}
	var extra obj
	if g.usage != nil {
		extra = obj{"usage": geminiUsage(g.usage)}
	}
	g.emit(obj{}, finish, extra)
	io.WriteString(dst, "data: [DONE]\n\n")
	dst.Flush()
}

// geminiStream is the state of one GeminiStreamToOpenAI run.
type geminiStream struct {
	dst        Flusher
	sigs       Signatures
	id, model  string
	created    int64
	started    bool
	calls      int
	finish     string
	pendingSig string
	usage      obj
}

// emit writes one Chat Completions chunk and flushes it.
func (g *geminiStream) emit(delta obj, fin any, extra obj) {
	ch := obj{"index": 0, "delta": delta, "finish_reason": fin}
	c := obj{"id": g.id, "object": "chat.completion.chunk", "created": g.created, "model": g.model, "choices": []any{ch}}
	for k, v := range extra {
		c[k] = v
	}
	b, _ := json.Marshal(c)
	io.WriteString(g.dst, "data: "+string(b)+"\n\n")
	g.dst.Flush()
}

// event handles one SSE data payload. It returns false at an upstream error,
// which it relays.
func (g *geminiStream) event(_, data string) bool {
	ev, err := decode([]byte(data))
	if err != nil {
		return true
	}
	if e := asObj(ev["error"]); e != nil {
		b, _ := json.Marshal(obj{"error": obj{"message": str(e["message"]), "type": str(e["status"])}})
		io.WriteString(g.dst, "data: "+string(b)+"\n\n")
		g.dst.Flush()
		return false
	}
	r := asObj(ev["response"])
	if r == nil {
		r = ev
	}
	if !g.started {
		g.started = true
		if s := str(r["responseId"]); s != "" {
			g.id = "chatcmpl-" + s
		}
		g.model = str(r["modelVersion"])
		g.emit(obj{"role": "assistant", "content": ""}, nil, nil)
	}
	if u := asObj(r["usageMetadata"]); u != nil {
		g.usage = u
	}
	cand := asObj(firstOf(r["candidates"]))
	for _, p := range list(asObj(cand["content"])["parts"]) {
		g.part(asObj(p))
	}
	if f := str(cand["finishReason"]); f != "" {
		g.finish = geminiFinish[f]
		if g.finish == "" {
			g.finish = "stop"
		}
	}
	return true
}

// part writes one candidate part. A signature on a part that is not a
// function call waits for the next call.
func (g *geminiStream) part(part obj) {
	sig := str(part["thoughtSignature"])
	switch {
	case part["functionCall"] != nil:
		g.functionCall(asObj(part["functionCall"]), sig)
		return
	case part["thought"] == true:
		if t := str(part["text"]); t != "" {
			g.emit(obj{"reasoning_content": t}, nil, nil)
		}
	case part["text"] != nil:
		if t := str(part["text"]); t != "" {
			g.emit(obj{"content": t}, nil, nil)
		}
	}
	if sig != "" {
		g.pendingSig = sig
	}
}

// functionCall writes one function call as a tool call and records its
// signature, or the pending one when it carries none.
func (g *geminiStream) functionCall(fc obj, sig string) {
	cid := str(fc["id"])
	if cid == "" {
		cid = "call_" + newID("")
	}
	if sig == "" {
		sig = g.pendingSig
	}
	if sig != "" && g.sigs != nil {
		g.sigs.Put(cid, sig)
	}
	g.pendingSig = ""
	args, _ := json.Marshal(fc["args"])
	g.emit(obj{"tool_calls": []any{obj{"index": g.calls, "id": cid, "type": "function",
		"function": obj{"name": fc["name"], "arguments": string(args)}}}}, nil, nil)
	g.calls++
}

// geminiUsage converts Gemini usage metadata to Chat Completions usage.
// Thought tokens count as completion tokens.
func geminiUsage(usage obj) obj {
	in := num(usage["promptTokenCount"])
	thoughts := num(usage["thoughtsTokenCount"])
	out := num(usage["candidatesTokenCount"]) + thoughts
	us := obj{"prompt_tokens": in, "completion_tokens": out, "total_tokens": in + out}
	if c := num(usage["cachedContentTokenCount"]); c > 0 {
		us["prompt_tokens_details"] = obj{"cached_tokens": c}
	}
	if thoughts > 0 {
		us["completion_tokens_details"] = obj{"reasoning_tokens": thoughts}
	}
	return us
}

// claudeThinkingBudget maps a reasoning effort to the token budget Claude takes
// behind Antigravity. It is 0 when thinking is off; Claude's floor is 1024.
func claudeThinkingBudget(effort string) int64 {
	switch effort {
	case "low":
		return 2048
	case "medium":
		return 8192
	case "high":
		return 16384
	case "xhigh", "max":
		return 32000
	}
	return 0
}
