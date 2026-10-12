package translate

import (
	"strings"
	"testing"
)

func heal(t *testing.T, body string) (map[string]any, bool) {
	t.Helper()
	out, changed := HealAnthropic([]byte(body))
	if !changed {
		if string(out) != body {
			t.Fatalf("an untouched body must come back byte for byte:\n%s", out)
		}
		return nil, false
	}
	return mustJSON(t, out), true
}

func TestAHealthyAnthropicHistoryIsLeftAlone(t *testing.T) {
	_, changed := heal(t, `{"model":"m","max_tokens":9,"thinking":{"type":"enabled","budget_tokens":2000},"messages":[
	 {"role":"user","content":"go"},
	 {"role":"assistant","content":[{"type":"thinking","thinking":"hm","signature":"REAL"},{"type":"tool_use","id":"t1","name":"a","input":{}}]},
	 {"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"},{"type":"text","text":"more"}]}]}`)
	if changed {
		t.Error("a valid history was changed")
	}
}

func TestSystemTurnsReachTheSystemPrompt(t *testing.T) {
	// Claude Code puts the output style in a system turn; the Messages shape has
	// no such role, so dropping it lost the style on every Anthropic provider.
	out, changed := heal(t, `{"model":"m","system":[{"type":"text","text":"base"}],"messages":[
	 {"role":"user","content":"context"},
	 {"role":"system","content":"style one"},
	 {"role":"developer","content":[{"type":"text","text":"style two"}]},
	 {"role":"user","content":"hi"}]}`)
	if !changed {
		t.Fatal("a history with system turns was left alone")
	}
	var texts []string
	for _, s := range out["system"].([]any) {
		texts = append(texts, s.(map[string]any)["text"].(string))
	}
	if strings.Join(texts, "|") != "base|style one|style two" {
		t.Errorf("system = %v, want base then both turns in order", texts)
	}
	if msgs := out["messages"].([]any); len(msgs) != 1 || msgs[0].(map[string]any)["role"] != "user" {
		t.Errorf("messages = %v, want the two user turns merged into one", msgs)
	}
}

func TestAStringSystemPromptLeadsTheFoldedTurns(t *testing.T) {
	out, _ := heal(t, `{"model":"m","system":"base","messages":[
	 {"role":"system","content":"style"},{"role":"user","content":"hi"}]}`)
	system, _ := out["system"].([]any)
	if len(system) != 2 || system[0].(map[string]any)["text"] != "base" || system[1].(map[string]any)["text"] != "style" {
		t.Errorf("system = %v, want base then style", system)
	}
}

func TestAnEmptySystemTurnLeavesNoEmptyPrompt(t *testing.T) {
	out, _ := heal(t, `{"model":"m","messages":[{"role":"system","content":""},{"role":"user","content":"hi"}]}`)
	if _, ok := out["system"]; ok {
		t.Errorf("an empty system turn produced a system prompt: %v", out["system"])
	}
}

func TestAThinkingBlockWithNoSignatureIsDropped(t *testing.T) {
	m, changed := heal(t, `{"model":"m","max_tokens":9,"messages":[
	 {"role":"user","content":"go"},
	 {"role":"assistant","content":[{"type":"thinking","thinking":"from another provider","signature":""},{"type":"text","text":"hi"}]},
	 {"role":"user","content":"next"},
	 {"role":"assistant","content":[{"type":"thinking","thinking":"only thinking","signature":""}]},
	 {"role":"user","content":"again"}]}`)
	if !changed {
		t.Fatal("not healed")
	}
	got := j(m["messages"])
	if strings.Contains(got, "another provider") || strings.Contains(got, "only thinking") {
		t.Errorf("an unsigned thinking block was kept: %s", got)
	}
	// The assistant turn that held nothing else goes, and its neighbours merge.
	if strings.Count(got, `"role":"assistant"`) != 1 || !strings.Contains(got, `"text":"hi"`) {
		t.Errorf("messages = %s", got)
	}
}

func TestBrokenAnthropicToolPairsAreRepaired(t *testing.T) {
	m, changed := heal(t, `{"model":"m","max_tokens":9,"messages":[
	 {"role":"user","content":"go"},
	 {"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"a","input":{}},{"type":"tool_use","id":"t2","name":"b","input":{}}]},
	 {"role":"user","content":[{"type":"text","text":"note"},{"type":"tool_result","tool_use_id":"t2","content":"two"},{"type":"tool_result","tool_use_id":"gone","content":"stray"}]}]}`)
	if !changed {
		t.Fatal("not healed")
	}
	msgs := list(m["messages"])
	user := asObj(msgs[len(msgs)-1])
	blocks := list(user["content"])
	// t1 has no result and gets a placeholder; t2's result follows it; both lead
	// the turn, ahead of the note and the stray result turned into text.
	types := []string{}
	for _, b := range blocks {
		types = append(types, str(asObj(b)["type"])+":"+str(asObj(b)["tool_use_id"]))
	}
	if strings.Join(types, ",") != "tool_result:t1,tool_result:t2,text:,text:" {
		t.Fatalf("blocks = %v", types)
	}
	got := j(user["content"])
	if !strings.Contains(got, missingToolResult) || !strings.Contains(got, "[Tool Result (gone)]: stray") {
		t.Errorf("content = %s", got)
	}
}

func TestAToolUseWithNoNextTurnGetsAPlaceholderResult(t *testing.T) {
	m, changed := heal(t, `{"model":"m","max_tokens":9,"messages":[
	 {"role":"user","content":"go"},
	 {"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"a","input":{}}]}]}`)
	if !changed {
		t.Fatal("not healed")
	}
	msgs := list(m["messages"])
	if len(msgs) != 3 || !strings.Contains(j(msgs[2]), `"tool_use_id":"t1"`) {
		t.Errorf("messages = %s", j(m["messages"]))
	}
}

func TestThinkingIsSwitchedOffWhenAToolLoopHasNoThinkingBlock(t *testing.T) {
	m, changed := heal(t, `{"model":"m","max_tokens":9,"thinking":{"type":"enabled","budget_tokens":2000},"messages":[
	 {"role":"user","content":"go"},
	 {"role":"assistant","content":[{"type":"text","text":"calling"},{"type":"tool_use","id":"t1","name":"a","input":{}}]},
	 {"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}]}`)
	if !changed {
		t.Fatal("not healed")
	}
	if _, has := m["thinking"]; has {
		t.Errorf("thinking left on: %s", j(m))
	}
}
