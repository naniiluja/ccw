package httpapi

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/naniiluja/ccw/internal/store"
)

// longChat builds a conversation big enough to pass the size floor, with the middle
// holding the words a real agent session accumulates.
func longChat(turns int, wordsPer int) []byte {
	msgs := []map[string]any{
		{"role": "system", "content": "You are an agent."},
		{"role": "user", "content": "Start the work: " + strings.Repeat("brief ", wordsPer)},
	}
	for i := 0; i < turns; i++ {
		msgs = append(msgs,
			map[string]any{"role": "assistant", "content": strings.Repeat("thinking ", wordsPer)},
			map[string]any{"role": "user", "content": strings.Repeat("more ", wordsPer)},
		)
	}
	msgs = append(msgs, map[string]any{"role": "user", "content": "the question"})
	body, _ := json.Marshal(map[string]any{"model": "groq/llama", "messages": msgs})
	return body
}

func chatOf(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var m struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("compacted body is not JSON: %v\n%s", err, body)
	}
	return m.Messages
}

// connectionByID reads a connection back, for a field SetStandby has just written.
func connectionByID(t *testing.T, s *store.Store, id string) store.Connection {
	t.Helper()
	conns, err := s.ListConnections()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range conns {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("connection %s is gone", id)
	return store.Connection{}
}

// The whole point: a 429 on a long conversation retries with the history shortened,
// so the account that answers next is not handed a prompt it will refuse again.
func TestCompactionShrinksALongConversationAndKeepsItsShape(t *testing.T) {
	s := openStore(t)
	a, _ := newServer(s, nil, nil)
	c2, _ := s.CreateConnection("groq", "two", "k2")

	body := longChat(12, 400)
	if len(body) < 24000 {
		t.Fatalf("fixture is only %d bytes, under the size floor", len(body))
	}
	out, st, ok := a.compactForRateLimit("groq", []store.Connection{c2}, body)
	if !ok {
		t.Fatal("a long conversation with an account behind it was not shortened")
	}
	if st.after >= st.before {
		t.Errorf("compaction made it longer: %d -> %d", st.before, st.after)
	}

	msgs := chatOf(t, out)
	// The system prompt and the opening request are untouched: the first is not
	// history, and the second is what the whole conversation is for.
	if msgs[0]["role"] != "system" {
		t.Errorf("first message = %v, want the system prompt kept", msgs[0]["role"])
	}
	if !strings.HasPrefix(msgs[1]["content"].(string), "Start the work") {
		t.Error("the opening request was not kept")
	}
	// One marker says what went, so a model reads the hole as a cut and not as a
	// memory it lost.
	var markers int
	for _, m := range msgs {
		if c, ok := m["content"].(string); ok && strings.HasPrefix(c, "[ccw]") {
			markers++
		}
	}
	if markers != 1 {
		t.Errorf("markers = %d, want exactly 1", markers)
	}
	if st.keptMiddle == 0 {
		t.Error("nothing from the middle was kept, so the conversation lost all its thread")
	}
	// The recent turns go through verbatim, tool calls included.
	if msgs[len(msgs)-1]["content"] != "the question" {
		t.Errorf("the last message = %v, want it untouched", msgs[len(msgs)-1])
	}
}

// The four conditions that make shortening lossless enough to be worth it. Each is
// checked by removing exactly one and seeing the compaction refuse.
func TestCompactionRefusesUnlessItHasSomewhereToSendTheShortenedRequest(t *testing.T) {
	s := openStore(t)
	a, _ := newServer(s, nil, nil)
	c1, _ := s.CreateConnection("groq", "one", "k1")
	c2, _ := s.CreateConnection("groq", "two", "k2")
	// Read back: SetStandby writes the database, and the connection this test
	// hands the guard must be the one the store now holds.
	if err := s.SetStandby(c2.ID, true); err != nil {
		t.Fatal(err)
	}
	c2 = connectionByID(t, s, c2.ID)
	body := longChat(12, 400)
	// 1. No account left to try: nowhere to send a shortened conversation.
	if _, _, ok := a.compactForRateLimit("groq", nil, body); ok {
		t.Error("shortened with no account behind it")
	}

	// 2. Only a standby left. Handing it a shortened conversation is not worth the
	// loss: it is the last resort for a provider with nothing else.
	if _, _, ok := a.compactForRateLimit("groq", []store.Connection{c2}, body); ok {
		t.Error("shortened with only a standby behind it")
	}

	// 3. A provider that keeps one session on one account has no second account,
	// so shortening would only have thrown history away.
	if err := s.SetSetting(rotationKey("groq"), `{"mode":"fallback","sticky":1}`); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := a.compactForRateLimit("groq", []store.Connection{c1, c2}, body); ok {
		t.Error("shortened under fallback rotation, where a session stays on one account")
	}
	if err := s.SetSetting(rotationKey("groq"), `{"mode":"round-robin","sticky":1}`); err != nil {
		t.Fatal(err)
	}

	// 4. Too small to be worth losing anything from.
	if _, _, ok := a.compactForRateLimit("groq", []store.Connection{c1, c2}, []byte(`{"model":"groq/llama","messages":[{"role":"user","content":"hi"}]}`)); ok {
		t.Error("shortened a conversation that was already short")
	}
}

// A policy setting that does not parse refuses lossy compaction rather than guessing,
// because guessing here means silently throwing a client's history away.
func TestAnUnreadablePolicyRefusesCompaction(t *testing.T) {
	s := openStore(t)
	a, _ := newServer(s, nil, nil)
	c1, _ := s.CreateConnection("groq", "one", "k1")
	c2, _ := s.CreateConnection("groq", "two", "k2")
	if err := s.SetSetting(rateLimitCompactKey, "{not json"); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := a.compactForRateLimit("groq", []store.Connection{c1, c2}, longChat(12, 400)); ok {
		t.Error("an unreadable policy left lossy compaction switched on")
	}

	// Switching it off deliberately is honoured.
	if err := s.SetSetting(rateLimitCompactKey, `{"enabled":false}`); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := a.compactForRateLimit("groq", []store.Connection{c1, c2}, longChat(12, 400)); ok {
		t.Error(`"enabled":false still compacted`)
	}
}

// The policy is read from settings, so an operator can change how much survives.
func TestThePolicyComesFromSettings(t *testing.T) {
	s := openStore(t)
	a, _ := newServer(s, nil, nil)
	c1, _ := s.CreateConnection("groq", "one", "k1")
	c2, _ := s.CreateConnection("groq", "two", "k2")

	if err := s.SetSetting(rateLimitCompactKey, `{"minBytes":99999999}`); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := a.compactForRateLimit("groq", []store.Connection{c1, c2}, longChat(12, 400)); ok {
		t.Error("a raised size floor was ignored")
	}

	// The marker can be switched off for a client that does not want it.
	if err := s.SetSetting(rateLimitCompactKey, `{"marker":false}`); err != nil {
		t.Fatal(err)
	}
	out, _, ok := a.compactForRateLimit("groq", []store.Connection{c1, c2}, longChat(12, 400))
	if !ok {
		t.Fatal("switching the marker off also switched the compaction off")
	}
	if strings.Contains(string(out), "[ccw]") {
		t.Error("the marker was written although the policy says not to")
	}
}

// A tool call in the middle is dropped rather than sent without its result: a request
// whose tool_result points at a tool_call that is gone is one providers refuse.
func TestCompactionNeverLeavesAOrphanToolResult(t *testing.T) {
	s := openStore(t)
	a, _ := newServer(s, nil, nil)
	c2, _ := s.CreateConnection("groq", "two", "k2")

	msgs := []map[string]any{
		{"role": "system", "content": "sys"},
		{"role": "user", "content": "do the thing"},
	}
	for i := 0; i < 8; i++ {
		msgs = append(msgs,
			map[string]any{"role": "assistant", "content": strings.Repeat("work ", 400),
				"tool_calls": []map[string]any{{"id": fmt.Sprintf("call_%d", i), "type": "function",
					"function": map[string]any{"name": "Bash", "arguments": `{"command":"ls"}`}}}},
			map[string]any{"role": "tool", "tool_call_id": fmt.Sprintf("call_%d", i),
				"content": strings.Repeat("output ", 400)},
		)
	}
	body, _ := json.Marshal(map[string]any{"model": "groq/llama", "messages": msgs})

	out, st, ok := a.compactForRateLimit("groq", []store.Connection{c2}, body)
	if !ok {
		t.Fatal("a long tool-heavy conversation was not shortened")
	}
	if st.droppedToolOnly == 0 {
		t.Error("nothing tool-only was dropped, so the test did not reach the case")
	}

	// Every tool result that survived has its call with it.
	var calls int
	seen := map[string]bool{}
	for _, m := range chatOf(t, out) {
		for _, c := range toAnySlice(m["tool_calls"]) {
			fn, _ := c.(map[string]any)["function"].(map[string]any)
			id, _ := c.(map[string]any)["id"].(string)
			seen[id] = true
			calls++
			_ = fn
		}
	}
	for _, m := range chatOf(t, out) {
		if m["role"] != "tool" {
			continue
		}
		if id, _ := m["tool_call_id"].(string); !seen[id] {
			t.Errorf("a tool result for %s survived without its call", id)
		}
	}
	if calls == 0 && st.keptMiddle == 0 {
		t.Error("the conversation kept neither words nor calls")
	}
}

func toAnySlice(v any) []any {
	l, _ := v.([]any)
	return l
}
