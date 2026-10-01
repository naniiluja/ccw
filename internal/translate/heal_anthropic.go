package translate

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
)

// HealAnthropic repairs a Messages request that Anthropic would refuse, before
// it goes to an Anthropic upstream untouched otherwise. Claude Code trims and
// compacts history, and an earlier turn may have been answered by another
// provider, so a history can arrive with:
//   - a thinking block whose signature is empty (ccw issues those for a
//     provider that has none; Anthropic verifies signatures and refuses them);
//   - a tool_use with no tool_result, or a tool_result with no tool_use;
//   - a tool_result that does not lead its user turn, or a tool result split
//     across two user turns;
//   - thinking on, in a tool loop whose last assistant turn does not open with
//     a thinking block.
//
// It returns the repaired body and true, or the body as it came and false when
// there was nothing to repair.
func HealAnthropic(body []byte) ([]byte, bool) {
	in, err := decode(body)
	if err != nil {
		return body, false
	}
	raw, ok := in["messages"].([]any)
	if !ok {
		return body, false
	}
	changed := false

	// Drop thinking blocks with no signature, and merge adjacent user turns so
	// that a tool_result split into a later turn still finds its tool_use.
	var msgs []obj
	for _, r := range raw {
		m := asObj(r)
		role := str(m["role"])
		if role != "user" && role != "assistant" {
			changed = true
			continue
		}
		content := m["content"]
		if blocks, isList := content.([]any); isList && role == "assistant" {
			kept := make([]any, 0, len(blocks))
			for _, b := range blocks {
				if bb := asObj(b); str(bb["type"]) == "thinking" && str(bb["signature"]) == "" {
					continue
				}
				kept = append(kept, b)
			}
			if len(kept) != len(blocks) {
				changed, content = true, kept
			}
			if len(kept) == 0 {
				changed = true
				continue
			}
		}
		if n := len(msgs); role == "user" && n > 0 && str(msgs[n-1]["role"]) == "user" {
			msgs[n-1]["content"] = append(anthropicBlocks(msgs[n-1]["content"]), anthropicBlocks(content)...)
			changed = true
			continue
		}
		msgs = append(msgs, withContent(m, content))
	}

	// Pair every tool_use with a tool_result at the head of the next user turn.
	var out []obj
	for i, m := range msgs {
		if str(m["role"]) == "assistant" {
			out = append(out, m)
			if ids := toolUseIDs(m["content"]); len(ids) > 0 && (i+1 >= len(msgs) || str(msgs[i+1]["role"]) != "user") {
				out = append(out, obj{"role": "user", "content": placeholderResults(ids)})
				changed = true
			}
			continue
		}
		var pending []string
		if n := len(out); n > 0 && str(out[n-1]["role"]) == "assistant" {
			pending = toolUseIDs(out[n-1]["content"])
		}
		blocks := anthropicBlocks(m["content"])
		results := map[string]any{}
		var rest []any
		sawResult := false
		for _, b := range blocks {
			bb := asObj(b)
			if str(bb["type"]) != "tool_result" {
				rest = append(rest, b)
				continue
			}
			sawResult = true
			if id := str(bb["tool_use_id"]); slices.Contains(pending, id) && results[id] == nil {
				results[id] = b
				continue
			}
			rest = append(rest, obj{"type": "text", "text": toolResultText(bb)})
			changed = true
		}
		if !sawResult && len(pending) == 0 {
			out = append(out, m)
			continue
		}
		var content []any
		for _, id := range pending {
			if r := results[id]; r != nil {
				content = append(content, r)
			} else {
				content = append(content, placeholderResult(id))
				changed = true
			}
		}
		content = append(content, rest...)
		if sameBlocks(content, blocks) {
			out = append(out, m)
			continue
		}
		changed = true
		if len(content) == 0 {
			content = []any{obj{"type": "text", "text": "(empty)"}}
		}
		out = append(out, withContent(m, content))
	}

	// Thinking on, in a tool loop whose last assistant turn does not open with
	// thinking: Anthropic refuses it, so this one request runs without thinking.
	if t := str(asObj(in["thinking"])["type"]); t != "" && t != "disabled" && len(out) >= 2 {
		last, before := out[len(out)-1], out[len(out)-2]
		first := anthropicBlocks(before["content"])
		if str(last["role"]) == "user" && hasBlockOfType(last["content"], "tool_result") &&
			str(before["role"]) == "assistant" && len(first) > 0 {
			if ft := str(asObj(first[0])["type"]); ft != "thinking" && ft != "redacted_thinking" {
				delete(in, "thinking")
				changed = true
			}
		}
	}

	if !changed {
		return body, false
	}
	healed := make([]any, len(out))
	for i, m := range out {
		healed[i] = m
	}
	in["messages"] = healed
	b, err := json.Marshal(in)
	if err != nil {
		return body, false
	}
	return b, true
}

// anthropicBlocks reads message content, a string or a list, as a list of blocks.
func anthropicBlocks(content any) []any {
	switch c := content.(type) {
	case string:
		if c == "" {
			return nil
		}
		return []any{obj{"type": "text", "text": c}}
	case []any:
		return slices.DeleteFunc(slices.Clone(c), func(b any) bool { return b == nil })
	}
	return nil
}

func withContent(m obj, content any) obj {
	c := make(obj, len(m))
	for k, v := range m {
		c[k] = v
	}
	c["content"] = content
	return c
}

func toolUseIDs(content any) []string {
	var ids []string
	for _, b := range anthropicBlocks(content) {
		if bb := asObj(b); str(bb["type"]) == "tool_use" && str(bb["id"]) != "" {
			ids = append(ids, str(bb["id"]))
		}
	}
	return ids
}

func hasBlockOfType(content any, typ string) bool {
	for _, b := range anthropicBlocks(content) {
		if str(asObj(b)["type"]) == typ {
			return true
		}
	}
	return false
}

func placeholderResult(id string) obj {
	return obj{"type": "tool_result", "tool_use_id": id, "content": missingToolResult}
}

func placeholderResults(ids []string) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = placeholderResult(id)
	}
	return out
}

// sameBlocks reports whether two block lists hold the very same blocks in the
// same order: a turn that needed no repair comes out as it went in.
func sameBlocks(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ma, mb := asObj(a[i]), asObj(b[i])
		if ma == nil || mb == nil || reflect.ValueOf(ma).Pointer() != reflect.ValueOf(mb).Pointer() {
			return false
		}
	}
	return true
}

// toolResultText writes a result that has no call as text, so its content is
// not lost with the block.
func toolResultText(b obj) string {
	label := "[Tool Result"
	if id := str(b["tool_use_id"]); id != "" {
		label += " (" + id + ")"
	}
	var text string
	switch c := b["content"].(type) {
	case string:
		text = c
	case []any:
		parts := make([]string, len(c))
		for i, p := range c {
			switch pb := asObj(p); str(pb["type"]) {
			case "text":
				parts[i] = str(pb["text"])
			case "image":
				parts[i] = "[image omitted]"
			default:
				j, _ := json.Marshal(p)
				parts[i] = string(j)
			}
		}
		text = strings.Join(parts, "\n")
	case nil:
	default:
		j, _ := json.Marshal(c)
		text = string(j)
	}
	return label + "]: " + text
}
