package zen

import (
	"encoding/json"
	"strings"
)

// Some Zen models answer only POST /responses: on /chat/completions they return
// 500 (the muse models, measured 2026-09-29). models.dev marks them with
// provider.npm "@ai-sdk/openai", and the OpenCode agent picks its endpoint from
// the same field. Their callers keep one chat surface.

// PrepareResponses builds the Responses body for one Chat Completions body. Only
// known fields are copied, so a chat-only field never reaches a strict schema.
// The free tier's gate still needs the read and shell tools and a stream, and
// the agent keys the upstream prompt cache on its session id, so this does too.
//
// ccw's own Chat to Responses conversion is built for the ChatGPT backend
// Codex uses, which refuses fields this upstream takes (temperature,
// max_output_tokens) and takes ones this upstream has not been shown
// (include, reasoning.summary); the free tier gets its own.
func PrepareResponses(chat []byte, sessionID string, pin map[string]any) ([]byte, error) {
	m, err := decodeObject(chat)
	if err != nil {
		return nil, err
	}
	stripCaller(m)
	for k, v := range pin {
		m[k] = v
	}
	tools, _ := m["tools"].([]any)
	for i, t := range tools {
		tools[i] = nestTool(t)
	}
	tools = withRequired(tools, func(name string) any { return chatTool(name) })

	out := map[string]any{}
	for _, k := range []string{"model", "temperature", "top_p", "parallel_tool_calls"} {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	instructions, items := responsesInput(list(m["messages"]))
	if instructions != "" {
		out["instructions"] = instructions
	}
	out["input"] = items
	flat := make([]any, len(tools))
	for i, t := range tools {
		flat[i] = flatTool(t)
	}
	out["tools"] = flat
	if v, ok := m["tool_choice"]; ok {
		out["tool_choice"] = responsesToolChoice(v)
	}
	limit, ok := m["max_completion_tokens"]
	if !ok {
		limit = m["max_tokens"]
	}
	if n, ok := limit.(json.Number); ok {
		if _, err := n.Int64(); err == nil {
			out["max_output_tokens"] = n
		}
	}
	effort, _ := m["reasoning_effort"].(string)
	if effort == "" {
		if r, ok := m["reasoning"].(map[string]any); ok {
			effort, _ = r["effort"].(string)
		}
	}
	if effort != "" {
		out["reasoning"] = map[string]any{"effort": effort}
	}
	if f := responsesTextFormat(m["response_format"]); f != nil {
		out["text"] = map[string]any{"format": f}
	}
	out["stream"] = true
	out["store"] = false
	out["prompt_cache_key"] = sessionID
	return json.Marshal(out)
}

func list(v any) []any { l, _ := v.([]any); return l }

// contentText reads message content given as a string or as text parts.
func contentText(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	var b strings.Builder
	for _, p := range list(v) {
		if pm, ok := p.(map[string]any); ok {
			if s, ok := pm["text"].(string); ok {
				b.WriteString(s)
			}
		}
	}
	return b.String()
}

// contentParts turns message content into Responses parts of one text type.
func contentParts(v any, textType string) []any {
	if s, ok := v.(string); ok {
		if s == "" {
			return []any{}
		}
		return []any{map[string]any{"type": textType, "text": s}}
	}
	parts := []any{}
	for _, p := range list(v) {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		if s, ok := pm["text"].(string); ok {
			parts = append(parts, map[string]any{"type": textType, "text": s})
		} else if pm["type"] == "image_url" {
			url := pm["image_url"]
			if um, ok := url.(map[string]any); ok {
				url = um["url"]
			}
			if u, ok := url.(string); ok {
				parts = append(parts, map[string]any{"type": "input_image", "image_url": u})
			}
		}
	}
	return parts
}

// responsesInput splits chat messages into the instructions text and the input
// items.
func responsesInput(messages []any) (string, []any) {
	var system []string
	items := []any{}
	for _, raw := range messages {
		msg, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch msg["role"] {
		case "system", "developer":
			if t := contentText(msg["content"]); t != "" {
				system = append(system, t)
			}
		case "tool":
			id, _ := msg["tool_call_id"].(string)
			items = append(items, map[string]any{"type": "function_call_output", "call_id": id, "output": contentText(msg["content"])})
		case "assistant":
			if parts := contentParts(msg["content"], "output_text"); len(parts) > 0 {
				items = append(items, map[string]any{"type": "message", "role": "assistant", "content": parts})
			}
			for _, c := range list(msg["tool_calls"]) {
				call, _ := c.(map[string]any)
				fn, _ := call["function"].(map[string]any)
				id, _ := call["id"].(string)
				name, _ := fn["name"].(string)
				args, _ := fn["arguments"].(string)
				if args == "" {
					args = "{}"
				}
				items = append(items, map[string]any{"type": "function_call", "call_id": id, "name": name, "arguments": args})
			}
		case "user":
			items = append(items, map[string]any{"type": "message", "role": "user", "content": contentParts(msg["content"], "input_text")})
		}
	}
	return strings.Join(system, "\n\n"), items
}

// flatTool lifts a nested {"function": {...}} tool to the flat Responses form.
func flatTool(t any) any {
	m, ok := t.(map[string]any)
	fn, isFn := m["function"].(map[string]any)
	if !ok || !isFn {
		return t
	}
	out := map[string]any{"type": "function"}
	for k, v := range fn {
		out[k] = v
	}
	return out
}

func responsesToolChoice(v any) any {
	if m, ok := v.(map[string]any); ok {
		if fn, ok := m["function"].(map[string]any); ok {
			return map[string]any{"type": "function", "name": fn["name"]}
		}
	}
	return v
}

func responsesTextFormat(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	switch m["type"] {
	case "json_schema":
		if js, ok := m["json_schema"].(map[string]any); ok {
			out := map[string]any{"type": "json_schema"}
			for k, val := range js {
				out[k] = val
			}
			return out
		}
	case "json_object":
		return map[string]any{"type": "json_object"}
	}
	return nil
}
