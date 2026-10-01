package translate

import (
	"strings"
	"testing"
)

func runZen(t *testing.T, frames ...string) string {
	t.Helper()
	var out buf
	ZenStreamToOpenAI(&out, strings.NewReader(sse(frames...)))
	return out.String()
}

// dataFrames returns the JSON payloads written, in order, without [DONE].
func dataFrames(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if p, ok := strings.CutPrefix(l, "data: "); ok && p != "[DONE]" {
			out = append(out, p)
		}
	}
	return out
}

func TestZenStreamLeavesInTheOpenAIShape(t *testing.T) {
	s := runZen(t,
		`{"id":"abc123","model":"up","created":100,"choices":[{"index":0,"delta":{"role":"assistant","content":"He","name":"Space Bunny"},"finish_reason":null}],"usage":null}`,
		`{"choices":[{"index":0,"delta":{"reasoning":"hm","reasoning_details":[{"index":0,"text":"h"}]}}],"usage":null}`,
		`{"choices":[{"index":0,"delta":{"content":"llo"},"finish_reason":"max_tokens"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5,"cost":"0"}}`,
		"[DONE]",
		`{"choices":[],"cost":"0"}`)
	frames := dataFrames(s)
	if len(frames) != 5 {
		t.Fatalf("%d frames, want 5 (two content, thinking, finish, usage):\n%s", len(frames), s)
	}
	for _, drop := range []string{`"name"`, `"cost"`, `"usage":null`} {
		if strings.Contains(s, drop) {
			t.Errorf("%s reached the caller:\n%s", drop, s)
		}
	}
	// JSON keys leave sorted, so each check is on one key or value.
	if !strings.Contains(s, `"id":"chatcmpl-abc123"`) {
		t.Errorf("the id was not given its prefix:\n%s", s)
	}
	inOrder(t, s, `"content":"He"`,
		`"reasoning":"hm"`, `"reasoning_content":"hm"`, `"reasoning_details":[{"index":0,"text":"h"}]`,
		`"content":"llo"`, `"delta":{},"finish_reason":"length"`,
		`"choices":[]`, `"total_tokens":5`, "data: [DONE]")
	// The role is written once, and the finish is alone in its chunk.
	if strings.Count(s, `"role":"assistant"`) != 1 {
		t.Errorf("role repeated:\n%s", s)
	}
	if strings.Count(s, "[DONE]") != 1 || strings.HasSuffix(strings.TrimSpace(s), `"cost":"0"}`) {
		t.Errorf("something follows [DONE]:\n%s", s)
	}
}

func TestZenStreamMapsAStoppedToolCallToToolCalls(t *testing.T) {
	s := runZen(t,
		`{"id":"x","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"read","arguments":"{\"p"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ath\":1}"}}]},"finish_reason":"stop"}]}`,
		"[DONE]")
	inOrder(t, s, `"arguments":"{\"p"`, `"name":"read"`, `"arguments":"ath\":1}"`, `"finish_reason":"tool_calls"`)
	if !strings.Contains(s, `"id":"c1"`) || strings.Count(s, `"type":"function"`) != 1 {
		t.Errorf("the call's id or type is wrong:\n%s", s)
	}
}

func TestZenStreamPassesKeepAliveComments(t *testing.T) {
	var out buf
	ZenStreamToOpenAI(&out, strings.NewReader(": keep-alive\n\n"+sse(`{"choices":[{"index":0,"delta":{"content":"a"},"finish_reason":"stop"}]}`, "[DONE]")))
	if !strings.HasPrefix(out.String(), ": keep-alive\n\n") {
		t.Errorf("the comment did not pass first:\n%s", out.String())
	}
}

func TestZenStreamEndsInAnErrorWhenItIsCut(t *testing.T) {
	s := runZen(t, `{"choices":[{"index":0,"delta":{"content":"partial"}}]}`)
	if !strings.Contains(s, `"error"`) || strings.Contains(s, `"finish_reason":"stop"`) {
		t.Errorf("a cut stream passed for complete:\n%s", s)
	}
	s = runZen(t, `{"choices":[{"index":0,"delta":{"content":"a"}}]}`, `{"error":{"message":"boom","type":"overloaded"}}`)
	if !strings.Contains(s, `"message":"boom"`) || strings.Contains(s, `"finish_reason":"stop"`) {
		t.Errorf("an error frame did not end the stream:\n%s", s)
	}
}

func TestZenStreamOfNothingButDoneIsAnEmptyAnswer(t *testing.T) {
	s := runZen(t, `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`, "[DONE]")
	inOrder(t, s, `"role":"assistant"`, `"finish_reason":"stop"`, "[DONE]")
}

func TestZenResponsesStreamLeavesThroughTheSameConformer(t *testing.T) {
	src := sse(`{"type":"response.created","response":{"id":"resp_1","created_at":5,"model":"muse"}}`,
		`{"type":"response.reasoning_text.delta","delta":"think"}`,
		`{"type":"response.output_text.delta","delta":"ok"}`,
		`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":4,"output_tokens":2,"total_tokens":6}}}`)
	var out buf
	ZenResponsesStreamToOpenAI(&out, strings.NewReader(src))
	s := out.String()
	inOrder(t, s, `"role":"assistant"`, `"reasoning":"think"`, `"reasoning_content":"think"`, `"content":"ok"`,
		`"finish_reason":"stop"`, `"total_tokens":6`, "data: [DONE]")
}

func TestCollectZenStreamFoldsAWholeAnswer(t *testing.T) {
	var out buf
	ZenStreamToOpenAI(&out, strings.NewReader(sse(
		`{"id":"a","model":"up","choices":[{"index":0,"delta":{"role":"assistant","reasoning":"hm","reasoning_details":[{"index":0,"text":"h"}]}}]}`,
		`{"choices":[{"index":0,"delta":{"reasoning_details":[{"index":0,"text":"i"}],"content":"Hi "}}]}`,
		`{"choices":[{"index":0,"delta":{"content":"there","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"read","arguments":"{}"}}]},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
		"[DONE]")))
	whole := CollectZenStream(strings.NewReader(out.String()))
	m := mustJSON(t, whole)
	msg := asObj(asObj(firstOf(m["choices"]))["message"])
	if msg["content"] != "Hi there" || msg["reasoning"] != "hm" || msg["reasoning_content"] != "hm" || msg["refusal"] != nil {
		t.Errorf("message = %s", j(msg))
	}
	if got := j(msg["reasoning_details"]); got != `[{"index":0,"text":"hi"}]` {
		t.Errorf("reasoning_details = %s, want the fragments stitched", got)
	}
	if got := j(msg["tool_calls"]); !strings.Contains(got, `"name":"read"`) {
		t.Errorf("tool_calls = %s", got)
	}
	if fr := asObj(firstOf(m["choices"]))["finish_reason"]; fr != "tool_calls" {
		t.Errorf("finish_reason = %v", fr)
	}
	if m["object"] != "chat.completion" || asObj(m["usage"])["total_tokens"] == nil {
		t.Errorf("body = %s", whole)
	}
}

func TestCollectZenStreamReportsAnErrorAndAnEmptyStream(t *testing.T) {
	for name, src := range map[string]string{
		"error":   sse(`{"error":{"message":"boom","type":"server_error"}}`),
		"nothing": sse("[DONE]"),
	} {
		got := string(CollectZenStream(strings.NewReader(src)))
		if !strings.Contains(got, `"error"`) {
			t.Errorf("%s: %s", name, got)
		}
	}
}

func TestZenStatusTurnsAnUnknownModelIntoA404(t *testing.T) {
	status, body := ZenStatus(401, []byte(`{"error":{"type":"ModelError","message":"Model jev-latest is not supported"}}`))
	if status != 404 || !strings.Contains(string(body), `"code":"model_not_found"`) || !strings.Contains(string(body), "jev-latest") {
		t.Errorf("got %d %s", status, body)
	}
	// A real bad key stays a 401, and other statuses are left alone.
	for _, c := range []struct {
		status int
		body   string
	}{{401, `{"error":{"type":"AuthError","message":"bad key"}}`}, {401, "not json"}, {500, `{"error":{"type":"ModelError"}}`}} {
		if s, b := ZenStatus(c.status, []byte(c.body)); s != c.status || string(b) != c.body {
			t.Errorf("%d %s was changed to %d %s", c.status, c.body, s, b)
		}
	}
}
