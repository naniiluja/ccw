package translate

// missingToolResult answers a call whose result the history does not hold; a
// chat provider refuses a tool call left without an answer.
const missingToolResult = "[no result: the tool call was interrupted]"

// healChatMessages runs healToolPairs over a chat history built as []any.
func healChatMessages(msgs []any) []any {
	if len(msgs) == 0 {
		return msgs
	}
	objs := make([]obj, len(msgs))
	for i, m := range msgs {
		objs[i] = asObj(m)
	}
	healed := healToolPairs(objs)
	out := make([]any, len(healed))
	for i, m := range healed {
		out[i] = m
	}
	return out
}

// healToolPairs gives every tool call an answer right after its assistant
// turn, and turns a result with no call into user text: a chat provider
// refuses a history where the two do not pair.
func healToolPairs(msgs []obj) []obj {
	var out, deferred []obj
	var pending []string // call ids of the last assistant turn still unanswered
	waiting := func(id string) int {
		for i, p := range pending {
			if p == id {
				return i
			}
		}
		return -1
	}
	orphan := func(m obj) obj {
		label := "[Tool Result]"
		if id := str(m["tool_call_id"]); id != "" {
			label = "[Tool Result (" + id + ")]"
		}
		return obj{"role": "user", "content": label + ": " + str(m["content"])}
	}
	flush := func() {
		for _, id := range pending {
			out = append(out, obj{"role": "tool", "tool_call_id": id, "content": missingToolResult})
		}
		pending = nil
		for _, m := range deferred {
			out = append(out, orphan(m))
		}
		deferred = nil
	}
	for _, m := range msgs {
		if str(m["role"]) == "tool" {
			id := str(m["tool_call_id"])
			if id == "" && len(pending) > 0 {
				id = pending[0]
				m["tool_call_id"] = id
			}
			switch i := waiting(id); {
			case i >= 0:
				pending = append(pending[:i], pending[i+1:]...)
				out = append(out, m)
			case len(pending) > 0:
				deferred = append(deferred, m)
			default:
				out = append(out, orphan(m))
			}
			continue
		}
		flush()
		if calls := list(m["tool_calls"]); len(calls) > 0 {
			for _, raw := range calls {
				c := asObj(raw)
				if str(c["id"]) == "" {
					c["id"] = newID("call_")
				}
				pending = append(pending, str(c["id"]))
			}
		}
		out = append(out, m)
	}
	flush()
	return out
}
