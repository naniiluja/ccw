package diabridge

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// toolNames pulls the tool names out of a decoded request in declaration order.
func toolNames(t *testing.T, body map[string]any) []string {
	t.Helper()
	tools, ok := body["tools"].([]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(tools))
	for _, raw := range tools {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := entry["name"].(string)
		names = append(names, name)
	}
	return names
}

// decodeBody turns the request a test upstream received into a map.
func decodeBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode upstream body: %v", err)
	}
	return body
}

// serveCompat starts a bridge whose gateway records the request it was sent and
// replies with the given SSE, so a test can assert on both sides at once.
func serveCompat(t *testing.T, reply string) (*Bridge, *[]byte) {
	t.Helper()
	var seen []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	// The bridge is started first because it points itself at a fresh port
	// file; only then does that file name the fake gateway.
	b := startBridge(t)
	writeGateway(t, portOf(t, srv.URL), "sk-test")
	return b, &seen
}

func TestLowercaseToolNamesAreSentAsClaudeCodeTools(t *testing.T) {
	// Dia's gateway answers "Request contains an unrecognized tool" with a 400
	// for any name outside its Claude Code whitelist, so a client that uses its
	// own spelling gets nothing but a dead handler turn without this rewrite.
	b, seen := serveCompat(t, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\",\"model\":\"claude-opus-4-8\"}}\n\n")

	postBack(t, b, `{"model":"claude-opus-4-8","tools":[
		{"name":"read","description":"r","input_schema":{"type":"object"}},
		{"name":"shell","description":"s","input_schema":{"type":"object"}},
		{"name":"edit","description":"e","input_schema":{"type":"object"}},
		{"name":"glob","description":"g","input_schema":{"type":"object"}},
		{"name":"grep","description":"q","input_schema":{"type":"object"}},
		{"name":"todo","description":"d","input_schema":{"type":"object"}},
		{"name":"task","description":"k","input_schema":{"type":"object"}},
		{"name":"write","description":"w","input_schema":{"type":"object"}}]}`)

	got := toolNames(t, decodeBody(t, *seen))
	want := []string{"Read", "Bash", "Edit", "Glob", "Grep", "TodoWrite", "Task", "Write"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("tools sent upstream = %v, want %v", got, want)
	}
}

func TestClaudeCodeToolNamesPassThroughUntouched(t *testing.T) {
	// A Claude Code client pointed at ccw already speaks the gateway's dialect,
	// so its request must reach the gateway byte-identical in every tool name.
	b, seen := serveCompat(t, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\"}}\n\n")

	postBack(t, b, `{"model":"claude-opus-4-8","tools":[
		{"name":"Bash","description":"b","input_schema":{"type":"object"}},
		{"name":"Read","description":"r","input_schema":{"type":"object"}},
		{"name":"TodoWrite","description":"t","input_schema":{"type":"object"}}]}`)

	got := toolNames(t, decodeBody(t, *seen))
	want := []string{"Bash", "Read", "TodoWrite"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("tools sent upstream = %v, want %v", got, want)
	}
}

func TestToolsWithNoEquivalentAreDropped(t *testing.T) {
	// A tool the gateway does not know and that has no counterpart ends the
	// handler turn, so it is left out rather than sent under a wrong name. One
	// that does have a counterpart is renamed instead, which is the other case.
	b, seen := serveCompat(t, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\"}}\n\n")

	postBack(t, b, `{"model":"claude-opus-4-8","tools":[
		{"name":"read","description":"r","input_schema":{"type":"object"}},
		{"name":"some_client_builtin","description":"x","input_schema":{"type":"object"}}]}`)

	got := toolNames(t, decodeBody(t, *seen))
	if len(got) != 1 || got[0] != "Read" {
		t.Errorf("tools sent upstream = %v, want only Read", got)
	}
}

func TestWebToolsBorrowASpareGatewayName(t *testing.T) {
	// The gateway refuses WebSearch and WebFetch by name, so they borrow a
	// whitelisted name the request leaves unused, keeping their own schema.
	b, seen := serveCompat(t, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\"}}\n\n")

	postBack(t, b, `{"model":"claude-opus-4-8","tools":[
		{"name":"WebSearch","description":"search","input_schema":{"type":"object","properties":{"query":{"type":"string"}}}},
		{"name":"WebFetch","description":"fetch","input_schema":{"type":"object"}},
		{"name":"Read","description":"read","input_schema":{"type":"object"}},
		{"name":"Glob","description":"glob","input_schema":{"type":"object"}}]}`)

	tools, _ := decodeBody(t, *seen)["tools"].([]any)
	byDesc := map[string]string{}
	for _, raw := range tools {
		tool := raw.(map[string]any)
		byDesc[tool["description"].(string)] = tool["name"].(string)
	}
	if byDesc["glob"] != "Glob" || byDesc["read"] != "Read" {
		t.Errorf("tools the client declared lost their names: %v", byDesc)
	}
	if byDesc["search"] != "Grep" || byDesc["fetch"] != "TodoWrite" {
		t.Errorf("web tools = search:%q fetch:%q, want the first free slots Grep and TodoWrite", byDesc["search"], byDesc["fetch"])
	}
}

func TestABorrowedNameIsReadBackAsTheWebTool(t *testing.T) {
	// The model calls the borrowed name; the client must see its own WebSearch.
	b, _ := serveCompat(t, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"t1\",\"name\":\"Glob\",\"input\":{}}}\n\n")

	_, raw := postBack(t, b, `{"model":"claude-opus-4-8","stream":true,"tools":[
		{"name":"WebSearch","description":"search","input_schema":{"type":"object"}}]}`)

	if !strings.Contains(raw, `"name":"WebSearch"`) {
		t.Errorf("reply did not name WebSearch again: %s", raw)
	}
}

func TestToolsThatCollapseOntoOneNameAreDeduped(t *testing.T) {
	// Claude Code declares Bash, BashOutput and KillBash, which all have to
	// become the one tool Dia accepts for the shell. Sending that as three
	// tools named Bash is rejected outright with "tools: Tool names must be
	// unique", which also ends the handler turn.
	b, seen := serveCompat(t, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\"}}\n\n")

	postBack(t, b, `{"model":"claude-opus-5-5","tools":[
		{"name":"Bash","description":"run","input_schema":{"type":"object"}},
		{"name":"BashOutput","description":"poll","input_schema":{"type":"object"}},
		{"name":"KillBash","description":"stop","input_schema":{"type":"object"}},
		{"name":"Read","description":"read","input_schema":{"type":"object"}}]}`)

	got := toolNames(t, decodeBody(t, *seen))
	unique := make(map[string]bool, len(got))
	for _, name := range got {
		if unique[name] {
			t.Errorf("tools sent upstream = %v, which repeats %q", got, name)
		}
		unique[name] = true
	}
	if !unique["Bash"] || !unique["Read"] {
		t.Errorf("tools sent upstream = %v, want Bash and Read to survive", got)
	}
}

func TestTheFirstOfTwoCollidingToolsIsKept(t *testing.T) {
	// Which of the two survives is a choice, and it has to be the one declared
	// first: the reply can then name exactly the tool the caller expects.
	b, seen := serveCompat(t, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\"}}\n\n")

	postBack(t, b, `{"model":"claude-opus-5-5","tools":[
		{"name":"shell","description":"run","input_schema":{"type":"object","properties":{"command":{}}}},
		{"name":"bash_output","description":"poll","input_schema":{"type":"object"}}]}`)

	got := toolNames(t, decodeBody(t, *seen))
	// bash_output becomes Read, which no other tool in this request claims, so
	// both survive under different names.
	want := []string{"Bash", "Read"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("tools sent upstream = %v, want %v", got, want)
	}
}

func TestTheTaskChecklistToolsEachKeepTheirOwnName(t *testing.T) {
	// Claude Code offers TaskCreate, TaskGet, TaskList and TaskUpdate alongside
	// TodoWrite, and Dia's gateway accepts each of them separately. Folding them
	// into Task would collapse four tools onto one name, which the gateway
	// refuses with "Tool names must be unique".
	b, seen := serveCompat(t, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\"}}\n\n")

	postBack(t, b, `{"model":"claude-opus-5-5","tools":[
		{"name":"Task","description":"agent","input_schema":{"type":"object"}},
		{"name":"TaskCreate","description":"new","input_schema":{"type":"object"}},
		{"name":"TaskGet","description":"one","input_schema":{"type":"object"}},
		{"name":"TaskList","description":"all","input_schema":{"type":"object"}},
		{"name":"TaskUpdate","description":"set","input_schema":{"type":"object"}}]}`)

	got := toolNames(t, decodeBody(t, *seen))
	want := []string{"Task", "TaskCreate", "TaskGet", "TaskList", "TaskUpdate"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("tools sent upstream = %v, want %v", got, want)
	}
}

func TestLowercaseTaskToolNamesReachTheirDiaSpelling(t *testing.T) {
	// A client that spells the checklist tools in lower case still gets them,
	// rather than having them dropped for want of an entry.
	b, seen := serveCompat(t, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\"}}\n\n")

	postBack(t, b, `{"model":"claude-opus-5-5","tools":[
		{"name":"task_create","description":"new","input_schema":{"type":"object"}},
		{"name":"taskUpdate","description":"set","input_schema":{"type":"object"}}]}`)

	got := toolNames(t, decodeBody(t, *seen))
	want := []string{"TaskCreate", "TaskUpdate"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("tools sent upstream = %v, want %v", got, want)
	}
}

func TestARequestWithNoUsableToolsDropsTheToolKey(t *testing.T) {
	// A tool_choice naming a tool that was dropped would otherwise leave the
	// gateway looking for a tool the request never declared.
	b, seen := serveCompat(t, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\"}}\n\n")

	postBack(t, b, `{"model":"claude-opus-4-8","tools":[
		{"name":"only_this_client_builtin_exists","description":"w","input_schema":{"type":"object"}}],
		"tool_choice":{"type":"tool","name":"only_this_client_builtin_exists"}}`)

	body := decodeBody(t, *seen)
	if _, ok := body["tools"]; ok {
		t.Errorf("upstream body still declares tools: %v", body["tools"])
	}
	if _, ok := body["tool_choice"]; ok {
		t.Errorf("upstream body still pins a tool_choice: %v", body["tool_choice"])
	}
}

func TestToolChoiceIsRenamedWithItsTool(t *testing.T) {
	b, seen := serveCompat(t, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\"}}\n\n")

	postBack(t, b, `{"model":"claude-opus-4-8","tools":[
		{"name":"shell","description":"s","input_schema":{"type":"object"}}],
		"tool_choice":{"type":"tool","name":"shell"}}`)

	choice, _ := decodeBody(t, *seen)["tool_choice"].(map[string]any)
	if got, _ := choice["name"].(string); got != "Bash" {
		t.Errorf("tool_choice name = %q, want Bash", got)
	}
}

func TestToolUseInHistoryIsRenamedWithTheToolList(t *testing.T) {
	// A caller replays the previous turn as tool_use blocks. If those keep the
	// caller's spelling while the tool list uses Dia's, the gateway sees a call
	// to a tool it was never offered.
	b, seen := serveCompat(t, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\"}}\n\n")

	postBack(t, b, `{"model":"claude-opus-4-8","tools":[
		{"name":"shell","description":"s","input_schema":{"type":"object"}}],
		"messages":[{"role":"assistant","content":[
			{"type":"tool_use","id":"t1","name":"shell","input":{"command":"ls"}}]}]}`)

	body := decodeBody(t, *seen)
	messages, _ := body["messages"].([]any)
	msg, _ := messages[0].(map[string]any)
	blocks, _ := msg["content"].([]any)
	block, _ := blocks[0].(map[string]any)
	if got, _ := block["name"].(string); got != "Bash" {
		t.Errorf("tool_use name in history = %q, want Bash", got)
	}
}

func TestAStreamingReplyNamesTheCallersToolAgain(t *testing.T) {
	// The rename must be undone on the way out, or the client cannot match the
	// tool_use block to the tool it declared.
	b, _ := serveCompat(t, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"t1\",\"name\":\"Bash\",\"input\":{}}}\n\n")

	_, raw := postBack(t, b, `{"model":"claude-opus-4-8","stream":true,"tools":[
		{"name":"shell","description":"s","input_schema":{"type":"object"}}]}`)

	if !strings.Contains(raw, `"name":"shell"`) {
		t.Errorf("streamed reply does not name the caller's tool: %s", raw)
	}
}

func TestANonStreamingReplyNamesTheCallersToolAgain(t *testing.T) {
	b, _ := serveCompat(t, strings.Join([]string{
		"event: content_block_start",
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"t1","name":"Bash","input":{}}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"ls\"}"}}`,
		"",
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":12}}`,
		"",
		"",
	}, "\n"))

	_, raw := postBack(t, b, `{"model":"claude-opus-4-8","tools":[
		{"name":"shell","description":"s","input_schema":{"type":"object"}}]}`)

	var got struct {
		Content []struct {
			Type  string          `json:"type"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("decode assembled reply: %v (%s)", err, raw)
	}
	if len(got.Content) != 1 || got.Content[0].Type != "tool_use" {
		t.Fatalf("assembled content = %s, want one tool_use block", raw)
	}
	if got.Content[0].Name != "shell" {
		t.Errorf("tool_use name = %q, want the caller's shell", got.Content[0].Name)
	}
	if got.Content[0].ID == "" {
		t.Error("assembled tool_use has no id, but Anthropic requires one")
	}
	if string(got.Content[0].Input) != `{"command":"ls"}` {
		t.Errorf("tool_use input = %s, want the reassembled JSON", got.Content[0].Input)
	}
	if got.StopReason != "tool_use" {
		t.Errorf("stop_reason = %q, want tool_use", got.StopReason)
	}
}

func TestATruncatedToolCallStillProducesValidJSON(t *testing.T) {
	// An upstream that stops mid-tool would otherwise hand the caller invalid
	// JSON in a field it must be able to parse.
	b, _ := serveCompat(t, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"t1\",\"name\":\"Bash\",\"input\":{}}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"comm\"}}\n\n")

	_, raw := postBack(t, b, `{"model":"claude-opus-4-8","tools":[
		{"name":"shell","description":"s","input_schema":{"type":"object"}}]}`)

	var got struct {
		Content []struct {
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("decode assembled reply: %v (%s)", err, raw)
	}
	if len(got.Content) != 1 {
		t.Fatalf("assembled content = %s, want one block", raw)
	}
	if !json.Valid(got.Content[0].Input) {
		t.Errorf("tool_use input is not valid JSON: %s", got.Content[0].Input)
	}
}

func TestAFixedThinkingBudgetBecomesAdaptiveWithAnEffortLevel(t *testing.T) {
	// claude-opus-4-8 rejects thinking.type=enabled outright, which is what
	// OpenAI-shaped clients send for a thinking budget.
	b, seen := serveCompat(t, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\"}}\n\n")

	postBack(t, b, `{"model":"claude-opus-4-8","thinking":{"type":"enabled","budget_tokens":51200}}`)

	body := decodeBody(t, *seen)
	thinking, _ := body["thinking"].(map[string]any)
	if got, _ := thinking["type"].(string); got != "adaptive" {
		t.Errorf("thinking type sent upstream = %q, want adaptive", got)
	}
	if _, ok := thinking["budget_tokens"]; ok {
		t.Error("thinking still carries a budget_tokens, which this model refuses")
	}
	output, _ := body["output_config"].(map[string]any)
	if got, _ := output["effort"].(string); got != "high" {
		t.Errorf("effort for a 51200 budget = %q, want high", got)
	}
}

func TestEffortLevelsFollowTheThinkingBudget(t *testing.T) {
	cases := map[string]float64{"low": 1024, "medium": 8192, "high": 51200}
	for want, budget := range cases {
		t.Run(want, func(t *testing.T) {
			if got := effortForBudget(budget); got != want {
				t.Errorf("effortForBudget(%v) = %q, want %q", budget, got, want)
			}
		})
	}
}

func TestAdaptiveThinkingIsLeftAlone(t *testing.T) {
	b, seen := serveCompat(t, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\"}}\n\n")

	postBack(t, b, `{"model":"claude-opus-4-8","thinking":{"type":"adaptive"},"output_config":{"effort":"low"}}`)

	body := decodeBody(t, *seen)
	thinking, _ := body["thinking"].(map[string]any)
	if got, _ := thinking["type"].(string); got != "adaptive" {
		t.Errorf("thinking type = %q, want adaptive untouched", got)
	}
	output, _ := body["output_config"].(map[string]any)
	if got, _ := output["effort"].(string); got != "low" {
		t.Errorf("effort = %q, want the caller's low", got)
	}
}

func TestAnAdaptiveDisplayTheGatewayRefusesBecomesSummarized(t *testing.T) {
	b, seen := serveCompat(t, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\"}}\n\n")

	// Claude Code sends "updates"; Dia's upstream takes only summarized or omitted.
	postBack(t, b, `{"model":"claude-opus-4-8","thinking":{"type":"adaptive","display":"updates"}}`)

	thinking, _ := decodeBody(t, *seen)["thinking"].(map[string]any)
	if got, _ := thinking["display"].(string); got != "summarized" {
		t.Errorf("display = %q, want summarized", got)
	}
}

func TestAnAcceptedAdaptiveDisplayIsKept(t *testing.T) {
	b, seen := serveCompat(t, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\"}}\n\n")

	postBack(t, b, `{"model":"claude-opus-4-8","thinking":{"type":"adaptive","display":"omitted"}}`)

	thinking, _ := decodeBody(t, *seen)["thinking"].(map[string]any)
	if got, _ := thinking["display"].(string); got != "omitted" {
		t.Errorf("display = %q, want the caller's omitted", got)
	}
}

func TestDisabledThinkingIsLeftOut(t *testing.T) {
	b, seen := serveCompat(t, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\"}}\n\n")

	// Claude Code's WebSearch side request; Dia's upstream refuses "disabled".
	postBack(t, b, `{"model":"claude-opus-4-8","thinking":{"type":"disabled"},"messages":[]}`)

	if th, ok := decodeBody(t, *seen)["thinking"]; ok {
		t.Errorf("upstream body still carries thinking: %v", th)
	}
}

func TestARequestWithNoThinkingKeepsNone(t *testing.T) {
	b, seen := serveCompat(t, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\"}}\n\n")

	postBack(t, b, `{"model":"claude-opus-4-8","messages":[]}`)

	body := decodeBody(t, *seen)
	if _, ok := body["thinking"]; ok {
		t.Errorf("upstream body grew a thinking block: %v", body["thinking"])
	}
	if _, ok := body["output_config"]; ok {
		t.Errorf("upstream body grew an output_config: %v", body["output_config"])
	}
}

func TestToolNamesAreRestoredOnlyWhenTheyWereRenamed(t *testing.T) {
	// A Claude Code client gets Dia's names back unchanged, so a stream rewrite
	// must not fire on a name the caller already spelled the gateway's way.
	const line = `data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"t1","name":"Bash","input":{}}}`
	if got := restoreStreamName(line, nil); got != line {
		t.Errorf("restoreStreamName with no mapping changed the line: %s", got)
	}
	reverse := reverseMapping(map[string]string{"shell": "Bash"})
	if got := restoreStreamName(line, reverse); !strings.Contains(got, `"name":"shell"`) {
		t.Errorf("restoreStreamName did not put the caller's name back: %s", got)
	}
}

func TestStreamedTextIsNotAlteredByTheNameRewrite(t *testing.T) {
	// Every line the model emits that is not a tool_use start has to cross byte
	// for byte, or a streamed answer would be corrupted.
	const reply = "event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello Bash world"}}` + "\n\n"
	b, _ := serveCompat(t, reply)

	_, raw := postBack(t, b, `{"model":"claude-opus-4-8","stream":true,"tools":[
		{"name":"shell","description":"s","input_schema":{"type":"object"}}]}`)

	if !strings.Contains(raw, `"text":"hello Bash world"`) {
		t.Errorf("streamed text was altered: %s", raw)
	}
}

func TestAModelVertexRejectsIsNotOffered(t *testing.T) {
	// The handler answers the first model of a turn on Bedrock and every model
	// switch after that on Vertex. A name Vertex refuses therefore works alone
	// and fails mid-session, so it must not be on the list ccw advertises.
	if isSupportedDiaModel("claude-sonnet-5-5") {
		t.Error("claude-sonnet-5-5 is offered, but Vertex answers 400 for it after a model switch")
	}
	for _, m := range []string{"claude-opus-5-5", "claude-opus-5", "claude-opus-4-8",
		"claude-sonnet-5", "claude-opus-4-6"} {
		if !isSupportedDiaModel(m) {
			t.Errorf("%q is not offered, but it was measured to answer on both routes", m)
		}
	}
}

func TestEveryListedDiaModelIsAcceptedByTheBridge(t *testing.T) {
	// The bridge answers 400 before it dials, so a model on its list that it
	// then refuses would be a name that can never be served.
	for _, m := range diaModels {
		if !isSupportedDiaModel(m) {
			t.Errorf("diaModels lists %q but isSupportedDiaModel rejects it", m)
		}
	}
	for _, m := range []string{"claude-sonnet-4-6", "claude-haiku-4-5", "claude-opus-5-5[1m]"} {
		if isSupportedDiaModel(m) {
			t.Errorf("%q is accepted, but the gateway answers 400 for it", m)
		}
	}
}

func TestADiaModelWithoutTheOneMillionMarkIsStillServed(t *testing.T) {
	// Dia's agent specs spell its models with a [1m] suffix, which the gateway
	// does not know. The bare id is what has to be routable.
	b, seen := serveCompat(t, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\"}}\n\n")

	resp, _ := postBack(t, b, `{"model":"claude-opus-5-5"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got, _ := decodeBody(t, *seen)["model"].(string); got != "claude-opus-5-5" {
		t.Errorf("upstream model = %q, want the bare id", got)
	}
}
