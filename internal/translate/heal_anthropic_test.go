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
