package zen

import (
	"bytes"
	"encoding/json"
	"errors"
)

// requiredTools are the two names the free tier insists on. The agent sends a
// dozen; these are the floor. A request without them, or with other names, is
// refused with 403 on the gated models.
var requiredTools = []string{"read", "shell"}

func decodeObject(body []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, errors.New("the body is not JSON")
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("the body must be a JSON object")
	}
	return m, nil
}

func stripCaller(m map[string]any) {
	for _, f := range callerBodyFields {
		delete(m, f)
	}
}

// PrepareChat turns a Chat Completions body into one the free tier accepts. The
// caller's identity is dropped (the caller's session id was read from the
// original request already), and the gate's two conditions are added: the
// required tools and a streaming call. A caller who asked for a whole answer
// still gets one, assembled from the stream by the relay.
func PrepareChat(body []byte, pin map[string]any) ([]byte, error) {
	m, err := decodeObject(body)
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
	m["tools"] = withRequired(tools, func(name string) any { return chatTool(name) })
	m["stream"] = true
	return json.Marshal(m)
}

// PrepareSystemOne turns a System One body into one the upstream accepts. It is
// the mirror image of PrepareChat: the model name becomes the one the upstream
// serves (ccw sends the TypeSafe name jev-latest; the upstream answers 401 for
// anything but jev-1.13-free), and stream and tools go, because System One
// rejects either one with 400. state and questions are the call's own content
// and stay as they arrived.
func PrepareSystemOne(body []byte, upstreamModel string) ([]byte, error) {
	m, err := decodeObject(body)
	if err != nil {
		return nil, err
	}
	m["model"] = upstreamModel
	delete(m, "stream")
	delete(m, "tools")
	return json.Marshal(m)
}

// withRequired keeps the caller's tools and appends only the required ones that
// are missing, so a client that drives a real agent loop sees its own list.
func withRequired(tools []any, build func(name string) any) []any {
	have := map[string]bool{}
	for _, t := range tools {
		if n := toolName(t); n != "" {
			have[n] = true
		}
	}
	if tools == nil {
		tools = []any{}
	}
	for _, name := range requiredTools {
		if !have[name] {
			tools = append(tools, build(name))
		}
	}
	return tools
}

// toolName reads a tool's name, flat or nested under "function".
func toolName(t any) string {
	m, _ := t.(map[string]any)
	if n, ok := m["name"].(string); ok {
		return n
	}
	if fn, ok := m["function"].(map[string]any); ok {
		n, _ := fn["name"].(string)
		return n
	}
	return ""
}

// nestTool puts a flat {"name": ...} tool in the nested shape the gate reads. It
// looks for function.name, so a flat tool would be invisible to it; a tool that
// is already nested passes untouched.
func nestTool(t any) any {
	m, ok := t.(map[string]any)
	if !ok || m["function"] != nil {
		return t
	}
	name, ok := m["name"].(string)
	if !ok {
		return t
	}
	fn := map[string]any{"name": name}
	rest := map[string]any{}
	for k, v := range m {
		switch k {
		case "name", "type":
		case "description", "parameters", "strict":
			fn[k] = v
		default:
			rest[k] = v
		}
	}
	typ := m["type"]
	if typ == nil {
		typ = "function"
	}
	out := map[string]any{"type": typ, "function": fn}
	for k, v := range rest {
		out[k] = v
	}
	return out
}

func toolSpec(name string) (description string, parameters map[string]any) {
	props, required := map[string]any{}, []any{}
	if name != "shell" {
		props, required = map[string]any{"path": map[string]any{"type": "string"}}, []any{"path"}
	}
	return "The `" + name + "` tool, as exposed by the caller's agent session.",
		map[string]any{"type": "object", "properties": props, "required": required}
}

func chatTool(name string) map[string]any {
	d, p := toolSpec(name)
	return map[string]any{"type": "function", "function": map[string]any{"name": name, "description": d, "parameters": p}}
}
