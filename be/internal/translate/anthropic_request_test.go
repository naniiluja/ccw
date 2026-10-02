package translate

import (
	"strings"
	"testing"
)

func toOpenAI(t *testing.T, body string) map[string]any {
	t.Helper()
	out, err := AnthropicToOpenAI([]byte(body))
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	return mustJSON(t, out)
}

func TestAdaptiveThinkingKeepsTheEffort(t *testing.T) {
	cases := map[string]string{
		`{"model":"m","max_tokens":9,"thinking":{"type":"adaptive"},"messages":[]}`:                                     "high",
		`{"model":"m","max_tokens":9,"thinking":{"type":"adaptive"},"output_config":{"effort":"low"},"messages":[]}`:    "low",
		`{"model":"m","max_tokens":9,"thinking":{"type":"adaptive"},"output_config":{"effort":"max"},"messages":[]}`:    "high",
		`{"model":"m","max_tokens":9,"thinking":{"type":"adaptive"},"output_config":{"effort":"medium"},"messages":[]}`: "medium",
	}
	for in, want := range cases {
		if got := toOpenAI(t, in)["reasoning_effort"]; got != want {
			t.Errorf("%s -> %v, want %s", in, got, want)
		}
	}
}

func TestUnsupportedContentIsRefusedNotDropped(t *testing.T) {
	for _, body := range []string{
		`{"model":"m","max_tokens":9,"messages":[{"role":"user","content":[{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"QQ=="}},{"type":"text","text":"sum"}]}]}`,
		`{"model":"m","max_tokens":9,"messages":[{"role":"user","content":[{"type":"image","source":{"type":"file","file_id":"f1"}}]}]}`,
	} {
		if _, err := AnthropicToOpenAI([]byte(body)); err == nil || !strings.Contains(err.Error(), "not supported") {
			t.Errorf("%s: err = %v", body, err)
		}
	}
}

func TestToolResultImagesReachTheModel(t *testing.T) {
	m := toOpenAI(t, `{"model":"m","max_tokens":9,"messages":[
	 {"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"shot","input":{}}]},
	 {"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"r"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"QUJD"}}]}]}]}`)
	got := j(m["messages"])
	if !strings.Contains(got, `"role":"tool"`) || !strings.Contains(got, `"url":"data:image/png;base64,QUJD"`) {
		t.Errorf("messages = %s", got)
	}
}

func TestToolChoiceWithoutToolsIsDropped(t *testing.T) {
	m := toOpenAI(t, `{"model":"m","max_tokens":9,"tool_choice":{"type":"any"},"tools":[{"type":"web_search_20250305","name":"web_search"}],"messages":[]}`)
	if _, ok := m["tool_choice"]; ok {
		t.Errorf("tool_choice sent with no tools: %s", j(m))
	}
}

// Claude Code trims history, so a call can lose its result and a result its call.
func TestBrokenToolPairsAreHealedForAChatProvider(t *testing.T) {
	m := toOpenAI(t, `{"model":"m","max_tokens":9,"messages":[
	 {"role":"user","content":"go"},
	 {"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"a","input":{}},{"type":"tool_use","id":"t2","name":"b","input":{}}]},
	 {"role":"user","content":[{"type":"tool_result","tool_use_id":"t2","content":"two"},{"type":"tool_result","tool_use_id":"gone","content":"stray"},{"type":"text","text":"next"}]}]}`)
	msgs := list(m["messages"])
	var roles []string
	for _, raw := range msgs {
		roles = append(roles, str(asObj(raw)["role"]))
	}
	got := j(m["messages"])
	// t1 has no result: it gets a placeholder right after the assistant turn.
	if !strings.Contains(got, `"tool_call_id":"t1"`) || !strings.Contains(got, missingToolResult) {
		t.Errorf("t1 not answered: %s", got)
	}
	// The stray result has no call: it becomes user text, not a tool message.
	if strings.Contains(got, `"tool_call_id":"gone"`) || !strings.Contains(got, "[Tool Result (gone)]: stray") {
		t.Errorf("stray result not turned into text: %s", got)
	}
	// Every tool message follows the assistant turn that called it.
	if strings.Join(roles, ",") != "user,assistant,tool,tool,user,user" {
		t.Errorf("roles = %v", roles)
	}
}

// A strict chat provider refuses $schema and uri formats, and a schema with no
// shape; a parameter named like a keyword is still a parameter.
func TestToolSchemaIsCleanedForAChatProvider(t *testing.T) {
	m := toOpenAI(t, `{"model":"m","max_tokens":9,"messages":[{"role":"user","content":"hi"}],
	 "tools":[{"name":"t","input_schema":{"$schema":"http://json-schema.org/draft-07/schema#","type":"object",
	  "properties":{"url":{"type":"string","format":"uri"},"format":{"type":"string"},"n":{"type":"integer","minimum":0},
	   "flag":{"type":"boolean","default":false},"obj":{}},"additionalProperties":false,"required":["url"]}}]}`)
	got := j(m["tools"])
	for _, gone := range []string{"$schema", `"format":"uri"`} {
		if strings.Contains(got, gone) {
			t.Errorf("%s left in %s", gone, got)
		}
	}
	for _, want := range []string{`"format":{"type":"string"}`, `"minimum":0`, `"default":false`, `"additionalProperties":false`, `"obj":{"properties":{},"type":"object"}`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
}

// count_tokens sizes Claude Code's context. A screenshot is base64: counted as
// text it would read as a full context and force a compaction.
func TestEstimateTokensCountsAnImageAsAFixedCost(t *testing.T) {
	big := strings.Repeat("A", 1<<20)
	body := `{"model":"m","max_tokens":9,"messages":[{"role":"user","content":[{"type":"text","text":"abcdefgh"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + big + `"}}]}]}`
	// "user", "text" and "abcdefgh" are 16 characters, 4 tokens; the image is a
	// flat 1600 however large its data is.
	if got := EstimateTokens([]byte(body)); got != 4+imageTokens {
		t.Errorf("estimate = %d, want %d", got, 4+imageTokens)
	}
	// Bytes that are not a request keep the plain quarter-of-a-byte estimate.
	if got := EstimateTokens([]byte("not json at all")); got != 4 {
		t.Errorf("fallback = %d, want 4", got)
	}
	// cache_control is a marker, not text.
	if a, b := EstimateTokens([]byte(`{"messages":[{"role":"user","content":"abcdefgh"}]}`)),
		EstimateTokens([]byte(`{"messages":[{"role":"user","content":"abcdefgh","cache_control":{"type":"ephemeral"}}]}`)); a != b {
		t.Errorf("cache_control counted: %d vs %d", a, b)
	}
}

// Two calls of one turn, the first result carrying an image: the image must not
// come between the two tool messages, or the second call reads as unanswered.
func TestAToolResultImageDoesNotSeparateParallelResults(t *testing.T) {
	m := toOpenAI(t, `{"model":"m","max_tokens":9,"messages":[
	 {"role":"user","content":"go"},
	 {"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"shot","input":{}},{"type":"tool_use","id":"t2","name":"read","input":{}}]},
	 {"role":"user","content":[
	  {"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"first"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"QUJD"}}]},
	  {"type":"tool_result","tool_use_id":"t2","content":"second"}]}]}`)
	got := j(m["messages"])
	if strings.Contains(got, missingToolResult) || strings.Contains(got, "[Tool Result") {
		t.Errorf("a real result was replaced or demoted: %s", got)
	}
	var roles []string
	for _, raw := range list(m["messages"]) {
		roles = append(roles, str(asObj(raw)["role"]))
	}
	if strings.Join(roles, ",") != "user,assistant,tool,tool,user" {
		t.Errorf("roles = %v, want both tool messages before the image turn", roles)
	}
	if !strings.Contains(got, `"content":"first"`) || !strings.Contains(got, `"content":"second"`) || !strings.Contains(got, `data:image/png;base64,QUJD`) {
		t.Errorf("a result or the image was lost: %s", got)
	}
}
