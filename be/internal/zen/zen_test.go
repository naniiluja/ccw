package zen

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func header(kv ...string) http.Header {
	h := http.Header{}
	for i := 0; i+1 < len(kv); i += 2 {
		h.Set(kv[i], kv[i+1])
	}
	return h
}

func TestCallerSessionIsReadFromTheHeadersOfEachClient(t *testing.T) {
	for _, tc := range []struct {
		name         string
		h            http.Header
		payload      map[string]any
		wantV, wantS string
	}{
		{"claude code header", header("x-claude-code-session-id", "cc-1"), nil, "cc-1", "header:x-claude-code-session-id"},
		{"a blank header is ignored", header("X-Claude-Code-Session-Id", "  ", "X-Session-Id", "s-2"), nil, "s-2", "header:x-session-id"},
		{"claude code body, header stripped", nil,
			map[string]any{"metadata": map[string]any{"user_id": `{"device_id":"d","session_id":"cc-7"}`}}, "cc-7", "body:metadata.user_id"},
		{"body field", nil, map[string]any{"conversation_id": "c-3"}, "c-3", "body:conversation_id"},
		{"metadata.session_id", nil, map[string]any{"metadata": map[string]any{"session_id": "m-1"}}, "m-1", "body:metadata.session_id"},
		{"a header wins over the body", header("X-Session-Id", "h"), map[string]any{"session_id": "b"}, "h", "header:x-session-id"},
		{"nothing at all", nil, map[string]any{"model": "x"}, "", ""},
		{"user_id that is not a record", nil, map[string]any{"metadata": map[string]any{"user_id": "plain"}}, "", ""},
	} {
		if v, s := Caller(tc.h, tc.payload); v != tc.wantV || s != tc.wantS {
			t.Errorf("%s: got (%q, %q), want (%q, %q)", tc.name, v, s, tc.wantV, tc.wantS)
		}
	}
}

func TestSessionIDHasOpenCodesShape(t *testing.T) {
	now := time.UnixMilli(0x0123456789ab)
	id := NewSessionID(now)
	if !IsSessionID(id) {
		t.Fatalf("%q is not shaped ses_ + 12 hex + 14 base62", id)
	}
	if id[4:16] != "0123456789ab" {
		t.Errorf("head = %s, want the millisecond timestamp in hex", id[4:16])
	}
	if IsSessionID("ses_short") || IsSessionID("xes_0123456789abABCDEFGHIJKLMN") || IsSessionID("ses_0123456789ABABCDEFGHIJKLMN") {
		t.Error("a malformed id was accepted")
	}
	if NewSessionID(now) == id {
		t.Error("two ids are the same: the tail must be random")
	}
}

func TestBodyKeyIgnoresOrderWhitespaceAndVolatileFields(t *testing.T) {
	a := BodyKey([]byte(`{"model":"m","messages":[{"role":"user","content":"hi"}],"request_id":"1"}`))
	b := BodyKey([]byte("{ \"messages\": [ {\"content\":\"hi\", \"role\":\"user\"} ],\n \"model\":\"m\", \"request_id\":\"2\", \"metadata\":{\"x\":1} }"))
	if a != b {
		t.Error("one conversation split into two keys")
	}
	if a == BodyKey([]byte(`{"model":"m","messages":[{"role":"user","content":"other"}]}`)) {
		t.Error("two conversations share a key")
	}
	if BodyKey([]byte("not json")) == "" {
		t.Error("a body that is not JSON must still get a key")
	}
}

func TestPoolKeepsOneSessionPerCallerAcrossGrowingTurns(t *testing.T) {
	p := NewPool(10*time.Minute, 0)
	first := p.For("conv-1", "header:x", []byte(`{"messages":[1]}`))
	again := p.For("conv-1", "header:x", []byte(`{"messages":[1,2,3]}`))
	other := p.For("conv-2", "header:x", []byte(`{"messages":[1]}`))
	if first.ID != again.ID || again.Uses != 2 {
		t.Errorf("one conversation got %q then %q (uses %d)", first.ID, again.ID, again.Uses)
	}
	if other.ID == first.ID {
		t.Error("two callers share a session even on the same body")
	}
	// Without a caller id the body is the key.
	once := p.For("", "", []byte(`{"a":1}`))
	twice := p.For("", "", []byte(`{"a":1}`))
	if once.ID != twice.ID {
		t.Error("the same body without a caller got two sessions")
	}
}

func TestPoolUsesAnOpenCodeCallersIDAsItStands(t *testing.T) {
	id := NewSessionID(time.Now())
	if got := NewPool(time.Minute, 0).For(id, "header:x-opencode-session", nil); got.ID != id {
		t.Errorf("session = %q, want the caller's %q", got.ID, id)
	}
}

func TestPoolReleasesIdleSessionsAndEvictsTheOldestAtTheCap(t *testing.T) {
	now := time.Now()
	p := NewPool(10*time.Minute, 2)
	p.now = func() time.Time { return now }
	a := p.For("a", "", nil)
	now = now.Add(time.Minute)
	p.For("b", "", nil)
	now = now.Add(time.Minute)
	p.For("c", "", nil) // the pool is full: a, the least recently used, goes
	if got := p.For("a", "", nil); got.ID == a.ID {
		t.Error("the least recently used session survived the cap")
	}
	now = now.Add(11 * time.Minute)
	if n := len(p.Live()); n != 0 {
		t.Errorf("%d sessions live after the ttl, want 0", n)
	}
}

func TestPoolReleaseMakesTheNextCallMintAnID(t *testing.T) {
	p := NewPool(time.Minute, 0)
	s := p.For("c", "", nil)
	p.Release(s.Key)
	if p.For("c", "", nil).ID == s.ID {
		t.Error("a released session came back")
	}
}

func decode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%s: %v", b, err)
	}
	return m
}

func toolNames(m map[string]any) []string {
	var out []string
	for _, t := range m["tools"].([]any) {
		out = append(out, toolName(t))
	}
	return out
}

func TestPrepareChatAddsTheGateConditions(t *testing.T) {
	out, err := PrepareChat([]byte(`{"model":"m","messages":[],"stream":false}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	m := decode(t, out)
	if m["stream"] != true || !reflect.DeepEqual(toolNames(m), []string{"read", "shell"}) {
		t.Errorf("gate conditions missing: %s", out)
	}
}

func TestPrepareChatKeepsTheCallersToolsAndAppendsOnlyWhatIsMissing(t *testing.T) {
	out, _ := PrepareChat([]byte(`{"model":"m","tools":[
	 {"type":"function","function":{"name":"read","parameters":{"type":"object"}}},
	 {"type":"function","function":{"name":"get_weather"}}]}`), nil)
	m := decode(t, out)
	if !reflect.DeepEqual(toolNames(m), []string{"read", "get_weather", "shell"}) {
		t.Errorf("tools = %v", toolNames(m))
	}
	// The caller's own read tool is untouched, not replaced by the stub.
	if !strings.Contains(string(out), `"parameters":{"type":"object"}`) {
		t.Errorf("the caller's read tool was replaced: %s", out)
	}
}

func TestPrepareChatNestsAFlatTool(t *testing.T) {
	out, _ := PrepareChat([]byte(`{"model":"m","tools":[{"name":"read","description":"d","parameters":{"type":"object"}}]}`), nil)
	m := decode(t, out)
	first := m["tools"].([]any)[0].(map[string]any)
	fn, _ := first["function"].(map[string]any)
	if fn["name"] != "read" || fn["description"] != "d" || first["type"] != "function" {
		t.Errorf("flat tool not nested: %v", first)
	}
	if !reflect.DeepEqual(toolNames(m), []string{"read", "shell"}) {
		t.Errorf("tools = %v", toolNames(m))
	}
}

func TestPrepareChatIgnoresANullToolsField(t *testing.T) {
	out, err := PrepareChat([]byte(`{"model":"m","tools":null}`), nil)
	if err != nil || !reflect.DeepEqual(toolNames(decode(t, out)), []string{"read", "shell"}) {
		t.Errorf("err = %v, out = %s", err, out)
	}
}

func TestPrepareChatDropsTheCallersIdentityAndKeepsTheRest(t *testing.T) {
	out, _ := PrepareChat([]byte(`{"model":"m","temperature":0.5,"user":"{\"device_id\":\"d\"}",
	 "metadata":{"user_id":"x"},"session_id":"s","sessionId":"s","conversation_id":"c","conversationId":"c",
	 "thread_id":"t","threadId":"t","messages":[]}`), nil)
	m := decode(t, out)
	for _, f := range callerBodyFields {
		if _, ok := m[f]; ok {
			t.Errorf("%s reached the upstream", f)
		}
	}
	if m["temperature"] != 0.5 || m["model"] != "m" {
		t.Errorf("the rest of the body changed: %s", out)
	}
}

func TestPrepareChatPinsTheProfilesFields(t *testing.T) {
	out, _ := PrepareChat([]byte(`{"model":"m","tool_choice":"required","messages":[]}`), map[string]any{"tool_choice": "auto"})
	if decode(t, out)["tool_choice"] != "auto" {
		t.Errorf("the pin did not win: %s", out)
	}
}

func TestPrepareRejectsABodyThatIsNotAnObject(t *testing.T) {
	for _, body := range []string{"nope", "[1]", `"s"`, ""} {
		if _, err := PrepareChat([]byte(body), nil); err == nil {
			t.Errorf("%q accepted", body)
		}
	}
}

func TestPrepareResponsesBuildsTheBodyFromAChatBody(t *testing.T) {
	out, err := PrepareResponses([]byte(`{"model":"muse","temperature":0.5,"top_p":0.9,"max_tokens":200,"reasoning_effort":"low",
	 "response_format":{"type":"json_schema","json_schema":{"name":"r","schema":{"type":"object"}}},
	 "tool_choice":{"type":"function","function":{"name":"lookup"}},"user":"u","metadata":{"a":1},"chat_only_field":1,
	 "messages":[{"role":"system","content":"be"},{"role":"developer","content":"brief"},
	  {"role":"user","content":[{"type":"text","text":"hi"},{"type":"image_url","image_url":{"url":"data:x"}}]},
	  {"role":"assistant","content":"calling","tool_calls":[{"id":"c1","type":"function","function":{"name":"lookup","arguments":"{\"q\":1}"}},{"id":"c2","function":{"name":"noargs"}}]},
	  {"role":"tool","tool_call_id":"c1","content":[{"type":"text","text":"found"}]}],
	 "tools":[{"type":"function","function":{"name":"lookup","description":"d","parameters":{"type":"object"}}}]}`),
		"ses_abc", map[string]any{"tool_choice": "auto"})
	if err != nil {
		t.Fatal(err)
	}
	m := decode(t, out)
	want := map[string]any{"model": "muse", "temperature": 0.5, "top_p": 0.9, "max_output_tokens": float64(200), "stream": true, "store": false,
		"prompt_cache_key": "ses_abc", "instructions": "be\n\nbrief", "tool_choice": "auto",
		"reasoning": map[string]any{"effort": "low"},
		"text":      map[string]any{"format": map[string]any{"type": "json_schema", "name": "r", "schema": map[string]any{"type": "object"}}}}
	for k, v := range want {
		if !reflect.DeepEqual(m[k], v) {
			t.Errorf("%s = %v, want %v", k, m[k], v)
		}
	}
	for _, gone := range []string{"user", "metadata", "chat_only_field", "messages", "reasoning_effort", "max_tokens", "response_format"} {
		if _, ok := m[gone]; ok {
			t.Errorf("%s reached the upstream", gone)
		}
	}
	tools := m["tools"].([]any)
	if len(tools) != 3 || toolName(tools[0]) != "lookup" || tools[0].(map[string]any)["name"] != "lookup" || tools[0].(map[string]any)["description"] != "d" ||
		tools[2].(map[string]any)["name"] != "shell" {
		t.Errorf("tools = %v, want the caller's lookup and flat read and shell", tools)
	}
	items := m["input"].([]any)
	var kinds []string
	for _, it := range items {
		im := it.(map[string]any)
		kinds = append(kinds, im["type"].(string))
	}
	if strings.Join(kinds, ",") != "message,message,function_call,function_call,function_call_output" {
		t.Errorf("input items = %v", kinds)
	}
	if got := items[3].(map[string]any)["arguments"]; got != "{}" {
		t.Errorf("a call with no arguments sent %v, want {}", got)
	}
	if got := items[4].(map[string]any)["output"]; got != "found" {
		t.Errorf("tool output = %v", got)
	}
	user := items[0].(map[string]any)["content"].([]any)
	if user[0].(map[string]any)["type"] != "input_text" || user[1].(map[string]any)["type"] != "input_image" {
		t.Errorf("user parts = %v", user)
	}
}

func TestPrepareSystemOneRenamesTheModelAndDropsChatFields(t *testing.T) {
	out, err := PrepareSystemOne([]byte(`{"model":"jev-latest","stream":true,"tools":[{}],"state":{"a":1},"questions":{"q":1}}`), "jev-1.13-free")
	if err != nil {
		t.Fatal(err)
	}
	m := decode(t, out)
	if m["model"] != "jev-1.13-free" || m["stream"] != nil || m["tools"] != nil || m["state"] == nil || m["questions"] == nil {
		t.Errorf("system one body = %s", out)
	}
}

func TestRouteReadsTheProfileThenTheCatalogue(t *testing.T) {
	if id, ep, pin := Route("muse-spark-1.3-contributor-free", nil); ep != Responses || pin["tool_choice"] != "auto" || id != "muse-spark-1.3-contributor-free" {
		t.Errorf("muse: %s %v %v", id, ep, pin)
	}
	if id, ep, _ := Route("jev-latest", nil); id != "jev-1.13-free" || ep != SystemOne {
		t.Errorf("alias: %s %v", id, ep)
	}
	if id, ep, _ := Route("brand-new-free", nil); id != "brand-new-free" || ep != Chat {
		t.Errorf("unknown: %s %v", id, ep)
	}
	cat := &Catalogue{Responses: map[string]bool{"brand-new-free": true}}
	if _, ep, _ := Route("brand-new-free", func() *Catalogue { return cat }); ep != Responses {
		t.Errorf("models.dev says responses, got %v", ep)
	}
}

func TestListKeepsTheFreeChatModelsAndReadsLimitsAndThinking(t *testing.T) {
	cat, ok := ParseCatalogue([]byte(`{"opencode":{"models":{
	 "big-pickle":{"reasoning":true,"cost":{"input":0,"output":0},"limit":{"context":200000,"output":128000}},
	 "paid-model":{"reasoning":true,"cost":{"input":1,"output":2}},
	 "deepseek-v4-flash-free":{"reasoning":false,"limit":{"context":1000}},
	 "muse-spark-1.3-contributor-free":{"reasoning":true,"provider":{"npm":"@ai-sdk/openai"}}}}}`))
	if !ok || !cat.Responses["muse-spark-1.3-contributor-free"] || !cat.PricedFree["big-pickle"] {
		t.Fatalf("catalogue = %+v ok=%v", cat, ok)
	}
	ids, doc := List([]string{"big-pickle", "paid-model", "deepseek-v4-flash-free", "jev-1.13-free", "unknown-free"}, &cat)
	if !reflect.DeepEqual(ids, []string{"big-pickle", "deepseek-v4-flash-free", "unknown-free"}) {
		t.Errorf("ids = %v: paid and System One models must stay out", ids)
	}
	s := string(doc)
	for _, want := range []string{`"supported_parameters":["reasoning","include_reasoning"]`, `"supported_parameters":[]`, `"context_length":200000`, `"max_completion_tokens":128000`} {
		if !strings.Contains(s, want) {
			t.Errorf("list misses %s:\n%s", want, s)
		}
	}
	if _, ok := ParseCatalogue([]byte(`{"opencode":{"models":{}}}`)); ok {
		t.Error("an empty catalogue must not replace one that worked")
	}
}

func TestSystemOneAnswerKeepsOnlyItsOwnFields(t *testing.T) {
	out, ok := SystemOneAnswer([]byte(`{"model":"jev","cost":"0","answers":{
	 "q1":{"type":"choice","choice":"a","confidence":0.9,"probabilities":{"a":1},"debug":1},
	 "q2":{"type":"mystery","x":1}},"usage":{"input_tokens":3,"output_tokens":1,"cost":5}}`))
	if !ok {
		t.Fatal("not read")
	}
	s := string(out)
	if strings.Contains(s, "cost") || strings.Contains(s, "debug") || strings.Contains(s, `"x"`) {
		t.Errorf("an unnamed field left: %s", s)
	}
	for _, want := range []string{`"choice":"a"`, `"confidence":0.9`, `"input_tokens":3`, `"q2":{"type":"mystery"}`} {
		if !strings.Contains(s, want) {
			t.Errorf("%s missing: %s", want, s)
		}
	}
	if _, ok := SystemOneAnswer([]byte("nope")); ok {
		t.Error("a body that is not JSON was read")
	}
}
