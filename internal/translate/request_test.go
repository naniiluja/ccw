package translate

import (
	"testing"
)

// requestSnapshots pins the exact bytes the request-side converters emit for
// inputs that reach most of their branches, so splitting them into helpers
// cannot reorder messages, blocks or fields.
var requestSnapshots = []struct {
	name    string
	convert func([]byte) ([]byte, bool, error)
	in      string
	want    string
}{
	{"AnthropicToOpenAI full", func(b []byte) ([]byte, bool, error) { o, err := AnthropicToOpenAI(b); return o, true, err },
		`{"model":"m","max_tokens":100,"temperature":0.50,"top_p":1,"stream":true,
		 "system":[{"type":"text","text":"s1"},{"type":"text","text":"s2"}],
		 "thinking":{"type":"enabled","budget_tokens":4000},
		 "stop_sequences":["X"],"metadata":{"user_id":"u1"},
		 "output_config":{"format":{"type":"json_schema","schema":{"type":"object","properties":{"a":{"type":"string"}}}}},
		 "tools":[{"name":"f","description":"d","input_schema":{"type":"object","properties":{"q":{"type":"string"}}}},{"type":"web_search_20250305","name":"web_search"}],
		 "tool_choice":{"type":"tool","name":"f","disable_parallel_tool_use":true},
		 "messages":[
		  {"role":"user","content":"hi"},
		  {"role":"assistant","content":[{"type":"text","text":"a"},{"type":"tool_use","id":"t1","name":"f","input":{"q":1.50}},{"type":"tool_use","id":"t2","name":"f","input":{}}]},
		  {"role":"user","content":[
		   {"type":"text","text":"after"},
		   {"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"r1"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"QQ"}}]},
		   {"type":"tool_result","tool_use_id":"t2","is_error":true,"content":[{"type":"image","source":{"type":"url","url":"https://x/y.png"}},{"type":"text","text":"r2"}]},
		   {"type":"image","source":{"type":"url","url":"https://x/z.png"}}]},
		  {"role":"assistant","content":[]},
		  {"role":"assistant","content":[{"type":"text","text":"done"}]}]}`,
		`{"max_tokens":100,"messages":[{"content":"s1\ns2","role":"system"},{"content":"hi","role":"user"},{"content":"a","role":"assistant","tool_calls":[{"function":{"arguments":"{\"q\":1.50}","name":"f"},"id":"t1","type":"function"},{"function":{"arguments":"{}","name":"f"},"id":"t2","type":"function"}]},{"content":"r1","role":"tool","tool_call_id":"t1"},{"content":"Error: r2","role":"tool","tool_call_id":"t2"},{"content":[{"image_url":{"url":"data:image/png;base64,QQ"},"type":"image_url"},{"image_url":{"url":"https://x/y.png"},"type":"image_url"},{"text":"after","type":"text"},{"image_url":{"url":"https://x/z.png"},"type":"image_url"}],"role":"user"},{"content":"done","role":"assistant"}],"model":"m","parallel_tool_calls":false,"reasoning_effort":"medium","response_format":{"json_schema":{"name":"response","schema":{"properties":{"a":{"type":"string"}},"type":"object"}},"type":"json_schema"},"stop":["X"],"stream":true,"stream_options":{"include_usage":true},"temperature":0.50,"tool_choice":{"function":{"name":"f"},"type":"function"},"tools":[{"function":{"description":"d","name":"f","parameters":{"properties":{"q":{"type":"string"}},"type":"object"}},"type":"function"}],"top_p":1,"user":"u1"}`},
	{"AnthropicToOpenAI string system, adaptive, server tools only", func(b []byte) ([]byte, bool, error) { o, err := AnthropicToOpenAI(b); return o, true, err },
		`{"model":"m","system":"sys","thinking":{"type":"adaptive"},"output_config":{"effort":"max"},"stream":false,
		 "output_format":{"type":"json_schema","schema":{"type":"object"}},
		 "tools":[{"type":"web_search_20250305","name":"web_search"}],"tool_choice":{"type":"any"},
		 "messages":[{"role":"user","content":[{"type":"text","text":"t1"},{"type":"text","text":"t2"}]}]}`,
		`{"messages":[{"content":"sys","role":"system"},{"content":"t1\nt2","role":"user"}],"model":"m","reasoning_effort":"high","response_format":{"json_schema":{"name":"response","schema":{"type":"object"}},"type":"json_schema"},"stream":false}`},
	{"OpenAIToAnthropic full", func(b []byte) ([]byte, bool, error) { o, err := OpenAIToAnthropic(b); return o, true, err },
		`{"model":"m","max_completion_tokens":20000,"temperature":0.2,"top_p":0.9,"stream":true,"reasoning_effort":"high",
		 "stop":"END","user":"u","parallel_tool_calls":false,"tool_choice":"auto",
		 "response_format":{"type":"json_schema","json_schema":{"schema":{"type":"object"}}},
		 "tools":[{"type":"function","function":{"name":"f","description":"d","parameters":{"type":"object","properties":{"n":{"type":"number","minimum":0.10}}}}},{"type":"function","function":{"name":"g"}},{"type":"x"}],
		 "messages":[
		  {"role":"system","content":"s"},{"role":"developer","content":[{"type":"text","text":"dev"},{"type":"image_url","image_url":{"url":"https://i"}}]},
		  {"role":"user","content":[{"type":"input_text","text":"u1"},{"type":"image_url","image_url":"data:image/jpeg;base64,AA"}]},
		  {"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"f","arguments":"{\"n\":1.0}"}},{"id":"c2","type":"function","function":{"name":"g","arguments":"[1]"}}]},
		  {"role":"tool","tool_call_id":"c1","content":"ok"},{"role":"tool","tool_call_id":"c2","content":""},
		  {"role":"user","content":"more"}]}`,
		`{"max_tokens":20000,"messages":[{"content":[{"text":"u1","type":"text"},{"source":{"data":"AA","media_type":"image/jpeg","type":"base64"},"type":"image"}],"role":"user"},{"content":[{"id":"c1","input":{"n":1.0},"name":"f","type":"tool_use"},{"id":"c2","input":{},"name":"g","type":"tool_use"}],"role":"assistant"},{"content":[{"content":[{"text":"ok","type":"text"}],"tool_use_id":"c1","type":"tool_result"},{"tool_use_id":"c2","type":"tool_result"},{"text":"more","type":"text"}],"role":"user"}],"metadata":{"user_id":"u"},"model":"m","output_config":{"format":{"schema":{"type":"object"},"type":"json_schema"}},"stop_sequences":["END"],"stream":true,"system":[{"text":"s","type":"text"},{"text":"dev","type":"text"}],"thinking":{"budget_tokens":16000,"type":"enabled"},"tool_choice":{"disable_parallel_tool_use":true,"type":"auto"},"tools":[{"description":"d","input_schema":{"properties":{"n":{"minimum":0.10,"type":"number"}},"type":"object"},"name":"f"},{"input_schema":{"properties":{},"type":"object"},"name":"g"}]}`},
	{"OpenAIToAnthropic forced tool keeps temperature", func(b []byte) ([]byte, bool, error) { o, err := OpenAIToAnthropic(b); return o, true, err },
		`{"model":"m","max_tokens":5000,"temperature":1,"reasoning_effort":"medium","stop":["a","b"],"tool_choice":{"type":"function","function":{"name":"f"}},
		 "tools":[{"type":"function","function":{"name":"f"}}],"messages":[{"role":"user","content":"q"}]}`,
		`{"max_tokens":5000,"messages":[{"content":[{"text":"q","type":"text"}],"role":"user"}],"model":"m","stop_sequences":["a","b"],"temperature":1,"tool_choice":{"name":"f","type":"tool"},"tools":[{"input_schema":{"properties":{},"type":"object"},"name":"f"}]}`},
	{"OpenAIToGemini claude thinking", func(b []byte) ([]byte, bool, error) {
		o, err := OpenAIToGemini(b, memSigs{"c2": "SIG2"})
		return o, true, err
	},
		`{"model":"claude-sonnet","max_tokens":1000,"temperature":0.30,"top_p":0.5,"stop":"S","reasoning_effort":"high",
		 "response_format":{"type":"json_schema","json_schema":{"schema":{"type":"object","properties":{"a":{"type":"string","format":"x"}}}}},
		 "tools":[{"type":"function","function":{"name":"get weather","description":"w","parameters":{"type":"object","properties":{"c":{"type":"string"}}}}},{"type":"function","function":{"name":"get_weather"}},{"type":"function","function":{"name":"9x"}}],
		 "tool_choice":{"type":"function","function":{"name":"get weather"}},
		 "messages":[
		  {"role":"system","content":"s1"},{"role":"developer","content":[{"type":"text","text":"s2"}]},
		  {"role":"assistant","content":"hello","tool_calls":[{"id":"c1","function":{"name":"get weather","arguments":"{\"c\":\"x\"}"}},{"id":"c2","function":{"name":"9x","arguments":""}},{"id":"c3","function":{"name":"9x","arguments":"{}"}}]},
		  {"role":"tool","tool_call_id":"c1","content":"{\"t\":20.50}"},{"role":"tool","tool_call_id":"c2","content":"plain"},
		  {"role":"user","content":[{"type":"text","text":"u"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AA"}},{"type":"image_url","image_url":"https://img"}]}]}`,
		`{"contents":[{"parts":[{"text":"..."}],"role":"user"},{"parts":[{"text":"hello"},{"functionCall":{"args":{"c":"x"},"id":"c1","name":"get_weather"},"thoughtSignature":"skip_thought_signature_validator"},{"functionCall":{"args":{},"id":"c2","name":"_9x"},"thoughtSignature":"SIG2"},{"functionCall":{"args":{},"id":"c3","name":"_9x"}}],"role":"model"},{"parts":[{"functionResponse":{"id":"c1","name":"get_weather","response":{"result":{"t":20.50}}}},{"functionResponse":{"id":"c2","name":"_9x","response":{"result":"plain"}}},{"text":"u"},{"inlineData":{"data":"AA","mimeType":"image/png"}},{"fileData":{"fileUri":"https://img","mimeType":"image/*"}}],"role":"user"}],"generationConfig":{"maxOutputTokens":20480,"responseMimeType":"application/json","responseSchema":{"properties":{"a":{"type":"string"}},"type":"object"},"stopSequences":["S"],"temperature":0.30,"thinkingConfig":{"includeThoughts":true,"thinkingBudget":16384},"topP":0.5},"systemInstruction":{"parts":[{"text":"s1\n\ns2"}],"role":"user"},"toolConfig":{"functionCallingConfig":{"allowedFunctionNames":["get_weather"],"mode":"ANY"}},"tools":[{"functionDeclarations":[{"description":"w","name":"get_weather","parameters":{"properties":{"c":{"type":"string"}},"type":"object"}},{"name":"_9x","parameters":{"properties":{"reason":{"type":"string"}},"required":["reason"],"type":"object"}}]}]}`},
	{"OpenAIToGemini level thinking", func(b []byte) ([]byte, bool, error) {
		o, err := OpenAIToGemini(b, memSigs{})
		return o, true, err
	},
		`{"model":"gemini-3","max_completion_tokens":100000,"reasoning_effort":"bogus","stop":["a"],"response_format":{"type":"json_object"},
		 "tools":[{"type":"function","function":{"name":"f"}}],"tool_choice":"none",
		 "messages":[{"role":"user","content":"q"}]}`,
		`{"contents":[{"parts":[{"text":"q"}],"role":"user"}],"generationConfig":{"maxOutputTokens":64000,"responseMimeType":"application/json","stopSequences":["a"],"thinkingConfig":{"includeThoughts":true,"thinkingLevel":"medium"}},"toolConfig":{"functionCallingConfig":{"mode":"NONE"}},"tools":[{"functionDeclarations":[{"name":"f","parameters":{"properties":{"reason":{"type":"string"}},"required":["reason"],"type":"object"}}]}]}`},
	{"OpenAIToGemini no config", func(b []byte) ([]byte, bool, error) {
		o, err := OpenAIToGemini(b, memSigs{})
		return o, true, err
	},
		`{"model":"gemini-3","reasoning_effort":"none","tools":[{"type":"function","function":{"name":"f"}}],"tool_choice":"required","messages":[{"role":"user","content":""}]}`,
		`{"contents":[{"parts":[{"text":"..."}],"role":"user"}],"generationConfig":{"maxOutputTokens":4096,"thinkingConfig":{"includeThoughts":false,"thinkingLevel":"minimal"}},"toolConfig":{"functionCallingConfig":{"mode":"ANY"}},"tools":[{"functionDeclarations":[{"name":"f","parameters":{"properties":{"reason":{"type":"string"}},"required":["reason"],"type":"object"}}]}]}`},
	{"HealAnthropic every repair", func(b []byte) ([]byte, bool, error) { o, ok := HealAnthropic(b); return o, ok, nil },
		`{"model":"m","thinking":{"type":"enabled","budget_tokens":2000},"messages":[
		  {"role":"system","content":"x"},
		  {"role":"user","content":"q"},
		  {"role":"assistant","content":[{"type":"thinking","thinking":"t","signature":""}]},
		  {"role":"user","content":[{"type":"text","text":"again"}]},
		  {"role":"assistant","content":[{"type":"thinking","thinking":"t","signature":""},{"type":"tool_use","id":"a1","name":"f","input":{}}]},
		  {"role":"assistant","content":[{"type":"tool_use","id":"a2","name":"f","input":{}},{"type":"tool_use","id":"a3","name":"f","input":{}}]},
		  {"role":"user","content":[{"type":"text","text":"lead"},{"type":"tool_result","tool_use_id":"a3","content":"r3"},{"type":"tool_result","tool_use_id":"zz","content":[{"type":"text","text":"o"},{"type":"image"}]}]},
		  {"role":"user","content":[{"type":"tool_result","tool_use_id":"a3","content":"dup"}]},
		  {"role":"assistant","content":[{"type":"text","text":"no think"},{"type":"tool_use","id":"b1","name":"f","input":{"n":2.50}}]},
		  {"role":"user","content":[{"type":"tool_result","tool_use_id":"b1","content":"rb"}]}]}`,
		`{"messages":[{"content":[{"text":"q","type":"text"},{"text":"again","type":"text"}],"role":"user"},{"content":[{"id":"a1","input":{},"name":"f","type":"tool_use"}],"role":"assistant"},{"content":[{"content":"[no result: the tool call was interrupted]","tool_use_id":"a1","type":"tool_result"}],"role":"user"},{"content":[{"id":"a2","input":{},"name":"f","type":"tool_use"},{"id":"a3","input":{},"name":"f","type":"tool_use"}],"role":"assistant"},{"content":[{"content":"[no result: the tool call was interrupted]","tool_use_id":"a2","type":"tool_result"},{"content":"r3","tool_use_id":"a3","type":"tool_result"},{"text":"lead","type":"text"},{"text":"[Tool Result (zz)]: o\n[image omitted]","type":"text"},{"text":"[Tool Result (a3)]: dup","type":"text"}],"role":"user"},{"content":[{"text":"no think","type":"text"},{"id":"b1","input":{"n":2.50},"name":"f","type":"tool_use"}],"role":"assistant"},{"content":[{"content":"rb","tool_use_id":"b1","type":"tool_result"}],"role":"user"}],"model":"m"}`},
}

func TestRequestConverterSnapshots(t *testing.T) {
	for _, c := range requestSnapshots {
		t.Run(c.name, func(t *testing.T) {
			got, ok, err := c.convert([]byte(c.in))
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				t.Fatal("reported no change")
			}
			if string(got) != c.want {
				t.Fatalf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}
