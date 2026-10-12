package diabridge

import (
	"encoding/json"
	"strings"
	"testing"
)

// The back face rewrites the request on every call, so its cost lands in every
// turn. This measures the rewrite alone, without a gateway, because that is the
// work ccw added on top of relaying the body unchanged.
const claudeCodeToolSet = `{"model":"claude-opus-5-5","max_tokens":2048,"stream":true,
	"thinking":{"type":"enabled","budget_tokens":51200},
	"messages":[{"role":"user","content":"hi"}],
	"tools":[
		{"name":"Bash","description":"run","input_schema":{"type":"object"}},
		{"name":"BashOutput","description":"poll","input_schema":{"type":"object"}},
		{"name":"KillBash","description":"stop","input_schema":{"type":"object"}},
		{"name":"Read","description":"read","input_schema":{"type":"object"}},
		{"name":"Write","description":"write","input_schema":{"type":"object"}},
		{"name":"Edit","description":"edit","input_schema":{"type":"object"}},
		{"name":"MultiEdit","description":"multi","input_schema":{"type":"object"}},
		{"name":"NotebookEdit","description":"nb","input_schema":{"type":"object"}},
		{"name":"Glob","description":"glob","input_schema":{"type":"object"}},
		{"name":"Grep","description":"grep","input_schema":{"type":"object"}},
		{"name":"TodoWrite","description":"todo","input_schema":{"type":"object"}},
		{"name":"Task","description":"task","input_schema":{"type":"object"}},
		{"name":"Skill","description":"skill","input_schema":{"type":"object"}},
		{"name":"WebFetch","description":"fetch","input_schema":{"type":"object"}},
		{"name":"WebSearch","description":"search","input_schema":{"type":"object"}}]}`

func BenchmarkApplyCompatClaudeCodeToolSet(b *testing.B) {
	raw := []byte(strings.TrimSpace(claudeCodeToolSet))
	b.ReportAllocs()
	for b.Loop() {
		var parsed map[string]any
		if err := json.Unmarshal(raw, &parsed); err != nil {
			b.Fatal(err)
		}
		applyCompat(parsed)
	}
}

// A reply arrives as a stream and is rewritten line by line, so that is the
// other place ccw touches every byte. A delta line carries no tool name and must
// come back untouched, which is the common case in a long answer.
func BenchmarkRestoreStreamNameOnATextDelta(b *testing.B) {
	const line = `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`
	reverse := reverseMapping(map[string]string{"shell": "Bash", "read": "Read"})
	b.ReportAllocs()
	for b.Loop() {
		if got := restoreStreamName(line, reverse); got != line {
			b.Fatalf("delta line was altered: %s", got)
		}
	}
}
