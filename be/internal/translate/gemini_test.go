package translate

import (
	"encoding/json"
	"testing"
)

// TestCleanGeminiSchemaCharacterization pins the output of CleanGeminiSchema
// for every branch of cleanSchema, so splitting it cannot change behavior.
func TestCleanGeminiSchemaCharacterization(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"nil schema", `null`,
			`{"properties":{"reason":{"type":"string"}},"required":["reason"],"type":"object"}`},
		{"allOf merges properties and fills missing keys",
			`{"properties":{"c":{"type":"boolean"}},"description":"keep","allOf":[
				{"properties":{"a":{"type":"string"}},"required":["a"],"description":"lost"},
				{"properties":{"b":{"type":"integer"}}}]}`,
			`{"description":"keep","properties":{"a":{"type":"string"},"b":{"type":"integer"},"c":{"type":"boolean"}},"required":["a"],"type":"object"}`},
		{"allOf without own properties",
			`{"allOf":[{"properties":{"a":{"type":"string"}}}]}`,
			`{"properties":{"a":{"type":"string"}},"type":"object"}`},
		{"items as a list keeps the first schema",
			`{"type":"array","items":[{"type":"string","minLength":1},{"type":"number"}]}`,
			`{"items":{"type":"string"},"type":"array"}`},
		{"items as a list whose first entry is not a schema",
			`{"type":"array","items":["x"]}`,
			`{"items":{},"type":"array"}`},
		{"items as an empty list falls back to string",
			`{"type":"array","items":[]}`,
			`{"items":{"type":"string"},"type":"array"}`},
		{"items as an object with a nullable type",
			`{"type":"array","items":{"type":["null","integer"]}}`,
			`{"items":{"type":"integer"},"type":"array"}`},
		{"array without items", `{"type":"array"}`,
			`{"items":{"type":"string"},"type":"array"}`},
		{"required naming no property is dropped",
			`{"type":"object","properties":{"a":{"type":"string"}},"required":["z"]}`,
			`{"properties":{"a":{"type":"string"}},"type":"object"}`},
		{"required keeps only known properties",
			`{"type":"object","properties":{"a":{"type":"string"},"b":{"type":"string"}},"required":["b","z","a"]}`,
			`{"properties":{"a":{"type":"string"},"b":{"type":"string"}},"required":["b","a"],"type":"object"}`},
		{"object without properties gets a reason",
			`{"type":"object","properties":{"bad":1}}`,
			`{"properties":{"reason":{"type":"string"}},"required":["reason"],"type":"object"}`},
		{"anyOf prefers an object branch",
			`{"anyOf":[{"type":"null"},{"type":"string"},{"type":"array","items":{"type":"number"}},{"type":"object","properties":{"o":{"type":"string"}}}]}`,
			`{"properties":{"o":{"type":"string"}},"type":"object"}`},
		{"oneOf prefers an array over a scalar",
			`{"oneOf":[{"type":"string"},{"type":"array","items":{"type":"number"}},"skip"]}`,
			`{"items":{"type":"number"},"type":"array"}`},
		{"anyOf does not override own keys",
			`{"type":"string","description":"own","anyOf":[{"type":"null"},{"type":"integer","description":"branch","maximum":3}]}`,
			`{"description":"own","maximum":3,"type":"string"}`},
		{"const and enum become strings",
			`{"properties":{"c":{"const":1},"e":{"type":"integer","enum":[1,null,true,"x",{"k":2}]},"z":{"enum":[null]}}}`,
			`{"properties":{"c":{"enum":["1"],"type":"string"},"e":{"enum":["1","true","x","{\"k\":2}"],"type":"string"},"z":{}},"type":"object"}`},
		{"unsupported keywords and x- keys go",
			`{"type":"string","format":"uri","x-extra":1,"$schema":"s","title":"t","pattern":"^a"}`,
			`{"pattern":"^a","type":"string"}`},
		{"numbers keep their text",
			`{"type":"number","minimum":1.50,"maximum":1e3}`,
			`{"maximum":1e3,"minimum":1.50,"type":"number"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var in any
			if c.in != "null" {
				m, err := decode([]byte(c.in))
				if err != nil {
					t.Fatal(err)
				}
				in = m
			}
			got, err := json.Marshal(CleanGeminiSchema(in))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != c.want {
				t.Fatalf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}
