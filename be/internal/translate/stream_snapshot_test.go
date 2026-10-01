package translate

import (
	"bytes"
	"flag"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// updateSnapshots rewrites the files under testdata/snapshot from the current
// code. Review the diff before committing a rewrite: these files pin the exact
// bytes, event order and flush points the converters produce.
var updateSnapshots = flag.Bool("update-snapshots", false, "rewrite testdata/snapshot from the current output")

// flushRecorder records written bytes and marks every Flush, so a snapshot
// pins where a converter flushes as well as what it writes.
type flushRecorder struct{ b bytes.Buffer }

func (f *flushRecorder) Write(p []byte) (int, error) { return f.b.Write(p) }
func (f *flushRecorder) Flush() error {
	f.b.WriteString("<FLUSH>\n")
	return nil
}

// sigRecorder is a Signatures that remembers every Put.
type sigRecorder map[string]string

func (s sigRecorder) Get(id string) string { return s[id] }
func (s sigRecorder) Put(id, sig string)   { s[id] = sig }

// errReader hands over its data once, then fails with a read error.
type errReader struct {
	s    string
	done bool
}

func (r *errReader) Read(p []byte) (int, error) {
	if r.done || r.s == "" {
		return 0, io.ErrUnexpectedEOF
	}
	r.done = true
	return copy(p, r.s), nil
}

var (
	snapshotIDs     = regexp.MustCompile(`(chatcmpl-|msg_|toolu_|srvtoolu_|call_)[0-9a-f]{24}`)
	snapshotCreated = regexp.MustCompile(`"created":[0-9]{10,}`)
)

// normalizeSnapshot replaces the random ids and clock values a converter
// generates, so the rest of its output can be compared byte for byte.
func normalizeSnapshot(s string) string {
	s = snapshotIDs.ReplaceAllString(s, "${1}<id>")
	return snapshotCreated.ReplaceAllString(s, `"created":<time>`)
}

// checkSnapshot compares got with testdata/snapshot/<name>.golden.
func checkSnapshot(t *testing.T, name, got string) {
	t.Helper()
	got = normalizeSnapshot(got)
	path := filepath.Join("testdata", "snapshot", name+".golden")
	if *updateSnapshots {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot: %v (run with -update-snapshots to create it)", err)
	}
	if got != string(want) {
		t.Errorf("%s differs from its snapshot\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestCollectZenStreamSnapshots(t *testing.T) {
	cases := []struct {
		name string
		src  io.Reader
	}{
		{"collect_zen_full", strings.NewReader(sse(
			`{"id":"chatcmpl-z","model":"big","created":42,"choices":[{"index":1,"delta":{"content":"second"}}]}`,
			`not json`,
			`{"choices":[{"index":0,"delta":{"reasoning":"think ","reasoning_details":[{"index":0,"type":"reasoning.text","text":"a"},null,{"type":"reasoning.encrypted","data":"x"}]}}]}`,
			`{"choices":[{"index":0,"delta":{"reasoning":"more","reasoning_details":[{"index":0,"type":"reasoning.text","text":"b"},{"index":0,"type":"reasoning.text"}]}}]}`,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"t1","function":{"name":"g","arguments":"{\"b\""}}]}}]}`,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"t0","function":{"name":"f","arguments":""}},{"index":1,"function":{"arguments":":2}"}}]}}]}`,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"x","arguments":"{}"}}]},"finish_reason":"tool_calls"},{"index":1,"delta":{},"finish_reason":"stop"}]}`,
			`{"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`,
			`[DONE]`,
			`{"choices":[{"index":0,"delta":{"content":"after done"}}]}`,
		))},
		{"collect_zen_text_and_tools", strings.NewReader(sse(
			`{"choices":[{"index":0,"delta":{"content":"hi","tool_calls":[{"index":0,"id":"t","function":{"name":"f","arguments":"{}"}}]}}]}`,
		))},
		{"collect_zen_index_gap", strings.NewReader(sse(
			`{"id":"chatcmpl-g","created":1,"choices":[{"index":2,"delta":{"content":"two"}},{"index":0,"delta":{"content":"zero"}}]}`,
		))},
		{"collect_zen_no_ids", strings.NewReader(sse(
			`{"model":"m","choices":[{"index":0,"delta":{"content":""}}]}`,
		))},
		{"collect_zen_error", strings.NewReader(sse(
			`{"id":"chatcmpl-e","choices":[{"index":0,"delta":{"content":"par"}}]}`,
			`{"error":{"message":"boom","type":"server_error"}}`,
			`{"choices":[{"index":0,"delta":{"content":"tial"}}]}`,
		))},
		{"collect_zen_empty_cut", &errReader{}},
		{"collect_zen_empty_clean", strings.NewReader(sse(`[DONE]`))},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			checkSnapshot(t, c.name, string(CollectZenStream(c.src)))
		})
	}
}

func TestOpenAIStreamToAnthropicSnapshots(t *testing.T) {
	search := &WebSearch{Query: "go", Results: []WebResult{{Title: "Go", URL: "https://go.dev", Age: "1 day ago"}}}
	cases := []struct {
		name string
		src  io.Reader
		r    Reply
	}{
		{"oa2an_full", strings.NewReader(sse(
			`{"id":"chatcmpl-a","model":"up","choices":[{"delta":{"role":"assistant","reasoning_content":"plan "}}]}`,
			`not json`,
			`{"choices":[]}`,
			`{"choices":[{"delta":{"content":"<think>inner</think>Hello "}}]}`,
			`{"choices":[{"delta":{"content":"world","refusal":"no"}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"t0","function":{"name":"f","arguments":"{\"a\""}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":1}"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"id":"t9","function":{"name":"h","arguments":"{}"}}]}}]}`,
			`{"choices":[{"delta":{"function_call":{"name":"","arguments":"broken"}},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":6}}`,
			`[DONE]`,
		)), Reply{Model: "caller", InputTokens: 9, Thinking: true, Search: search}},
		{"oa2an_no_index_fragments", strings.NewReader(sse(
			`{"id":"c","model":"m","choices":[{"delta":{"tool_calls":[{"id":"t0","function":{"name":"f","arguments":"{\"x\""}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"function":{"arguments":":1}"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"id":"t1","function":{"name":"g","arguments":"{}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			`[DONE]`,
		)), Reply{}},
		{"oa2an_thinking_off", strings.NewReader(sse(
			`{"id":"chatcmpl-b","model":"m","choices":[{"delta":{"reasoning":"hidden","content":"<think>also hidden</think>shown"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			`[DONE]`,
		)), Reply{}},
		{"oa2an_finish_without_done", strings.NewReader(sse(
			`{"id":"chatcmpl-c","model":"m","choices":[{"delta":{"content":"done"},"finish_reason":"length"}]}`,
		)), Reply{}},
		{"oa2an_done_without_finish", strings.NewReader(sse(
			`{"id":"chatcmpl-d","model":"m","choices":[{"delta":{"content":"x"}}]}`,
			`[DONE]`,
		)), Reply{}},
		{"oa2an_error_chunk", strings.NewReader(sse(
			`{"id":"chatcmpl-e","model":"m","choices":[{"delta":{"content":"par"}}]}`,
			`{"error":{"type":"rate_limit_error","message":"slow down"}}`,
			`{"choices":[{"delta":{"content":"ignored"}}]}`,
		)), Reply{}},
		{"oa2an_error_first", strings.NewReader(sse(
			`{"error":{"type":"invalid_request_error","message":"bad"}}`,
		)), Reply{}},
		{"oa2an_cut_without_finish", strings.NewReader(sse(
			`{"id":"chatcmpl-f","model":"m","choices":[{"delta":{"content":"half"}}]}`,
		)), Reply{}},
		{"oa2an_read_error", &errReader{s: sse(
			`{"id":"chatcmpl-g","model":"m","choices":[{"delta":{"content":"half"},"finish_reason":"stop"}]}`,
		)}, Reply{}},
		{"oa2an_empty", strings.NewReader(""), Reply{Search: &WebSearch{Query: "q", Failure: "down"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var rec flushRecorder
			OpenAIStreamToAnthropic(&rec, c.src, c.r)
			checkSnapshot(t, c.name, rec.b.String())
		})
	}
}

func TestGeminiStreamToOpenAISnapshots(t *testing.T) {
	cases := []struct {
		name string
		src  io.Reader
		sigs bool
	}{
		{"gemini_full", strings.NewReader(sse(
			`{"responseId":"r1","modelVersion":"gem","candidates":[{"content":{"parts":[{"thought":true,"text":"ponder","thoughtSignature":"s1"}]}}]}`,
			`not json`,
			`{"candidates":[{"content":{"parts":[{"functionCall":{"id":"c1","name":"f","args":{"a":1}}}]}}]}`,
			`{"candidates":[{"content":{"parts":[{"text":"hello","thoughtSignature":"s2"},{"text":""},{"thought":true,"text":""}]}}]}`,
			`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"g","args":{}},"thoughtSignature":"s3"}]}}]}`,
			`{"candidates":[{"content":{"parts":[{"inlineData":{},"thoughtSignature":"s4"},{"functionCall":{"id":"c2","name":"h"}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"thoughtsTokenCount":2,"cachedContentTokenCount":4}}`,
		)), true},
		{"gemini_wrapped_unknown_finish", strings.NewReader(sse(
			`{"response":{"responseId":"r2","modelVersion":"gem","candidates":[{"content":{"parts":[{"text":"hi"}]},"finishReason":"OTHER"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1}}}`,
		)), false},
		{"gemini_max_tokens", strings.NewReader(sse(
			`{"responseId":"r3","candidates":[{"content":{"parts":[{"text":"long"}]},"finishReason":"MAX_TOKENS"}]}`,
		)), false},
		{"gemini_no_finish_with_call", strings.NewReader(sse(
			`{"responseId":"r4","candidates":[{"content":{"parts":[{"functionCall":{"id":"c","name":"f","args":{}},"thoughtSignature":"s"}]}}]}`,
		)), false},
		{"gemini_error", strings.NewReader(sse(
			`{"responseId":"r5","candidates":[{"content":{"parts":[{"text":"par"}]}}]}`,
			`{"error":{"message":"quota","status":"RESOURCE_EXHAUSTED"}}`,
			`{"candidates":[{"content":{"parts":[{"text":"ignored"}]}}]}`,
		)), false},
		{"gemini_empty", strings.NewReader(""), false},
		{"gemini_read_error", &errReader{s: sse(
			`{"responseId":"r6","candidates":[{"content":{"parts":[{"text":"half"}]},"finishReason":"STOP"}]}`,
		)}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var rec flushRecorder
			var sigs Signatures
			got := sigRecorder{}
			if c.sigs {
				sigs = got
			}
			GeminiStreamToOpenAI(&rec, c.src, sigs)
			out := rec.b.String()
			if c.sigs {
				var lines []string
				for id, sig := range got {
					lines = append(lines, normalizeSnapshot(id)+"="+sig)
				}
				sort.Strings(lines)
				out += "SIGNATURES\n" + strings.Join(lines, "\n") + "\n"
			}
			checkSnapshot(t, c.name, out)
		})
	}
}

func TestOpenAIToResponsesSnapshots(t *testing.T) {
	cases := []struct{ name, body string }{
		{"to_responses_full", `{"model":"gpt","messages":[
			{"role":"system","content":"be brief"},
			{"role":"developer","content":[{"type":"text","text":"dev rule"}]},
			{"role":"system","content":""},
			{"role":"user","content":[{"type":"text","text":"look"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AA","detail":"high"}},{"type":"image_url","image_url":"https://x/y.png"}]},
			{"role":"assistant","content":"calling","tool_calls":[{"id":"c1","function":{"name":"f","arguments":"{\"a\":1}"}}]},
			{"role":"tool","tool_call_id":"c1","content":"result"},
			{"role":"assistant","content":""},
			{"role":"unknown","content":"skip"}],
			"tools":[{"type":"function","function":{"name":"f","description":"does f","parameters":{"type":"object"}}},{"type":"function","function":{"name":"g"}},{"type":"other"}],
			"tool_choice":{"type":"function","function":{"name":"f"}},
			"reasoning_effort":"high","parallel_tool_calls":false,"service_tier":"flex","prompt_cache_key":"k","safety_identifier":"s",
			"verbosity":"low","response_format":{"type":"json_schema","json_schema":{"schema":{"type":"object"},"strict":true}}}`},
		{"to_responses_minimal", `{"model":"gpt","messages":[],"tool_choice":"auto","response_format":{"type":"json_object"}}`},
		{"to_responses_named_schema", `{"model":"gpt","messages":[{"role":"user","content":"hi"}],"tool_choice":{"type":"function","function":{}},"response_format":{"type":"json_schema","json_schema":{"name":"n","schema":{}}}}`},
		{"to_responses_text_format", `{"model":"gpt","messages":[{"role":"user","content":"hi"}],"response_format":{"type":"text"}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := OpenAIToResponses([]byte(c.body))
			if err != nil {
				t.Fatal(err)
			}
			checkSnapshot(t, c.name, string(out)+"\n")
		})
	}
	if _, err := OpenAIToResponses([]byte(`not json`)); err == nil {
		t.Error("a body that is not JSON was accepted")
	}
}
