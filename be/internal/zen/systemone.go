package zen

import (
	"encoding/json"
	"log/slog"
)

// System One is TypeSafe's typed-answer endpoint, which jev answers on. Its
// answers keep their typed shape (noul, choice, score); only the fields outside
// this table, such as cost, are dropped.
var (
	systemOneTop     = map[string]bool{"model": true, "answers": true, "usage": true}
	systemOneUsage   = map[string]bool{"input_tokens": true, "output_tokens": true}
	systemOneAnswers = map[string][]string{
		"noul":   {"type", "noul"},
		"choice": {"type", "choice", "confidence", "probabilities"},
		"score":  {"type", "score", "confidence", "legend", "probabilities"},
	}
)

// SystemOneAnswer reduces a System One answer to the fields it names. It reports
// false for a body that is not a JSON object, which is then passed as it came.
func SystemOneAnswer(body []byte) ([]byte, bool) {
	var in map[string]any
	if json.Unmarshal(body, &in) != nil || in == nil {
		return body, false
	}
	out := map[string]any{}
	for k, v := range in {
		if !systemOneTop[k] {
			continue
		}
		switch x := v.(type) {
		case map[string]any:
			if k == "answers" {
				kept := map[string]any{}
				for name, a := range x {
					kept[name] = systemOneAnswer(a)
				}
				v = kept
			} else if k == "usage" {
				kept := map[string]any{}
				for f, n := range x {
					if systemOneUsage[f] {
						kept[f] = n
					}
				}
				v = kept
			}
		}
		out[k] = v
	}
	b, err := json.Marshal(out)
	if err != nil {
		return body, false
	}
	return b, true
}

func systemOneAnswer(a any) any {
	m, ok := a.(map[string]any)
	if !ok {
		return a
	}
	typ, _ := m["type"].(string)
	fields, known := systemOneAnswers[typ]
	if !known {
		// A primitive not named here yet: only its type leaves.
		// The answer is reduced to its type.
		slog.Warn("zen.systemone.unknown_type", "type", typ)
		return map[string]any{"type": m["type"]}
	}
	kept := map[string]any{}
	for _, f := range fields {
		if v, ok := m[f]; ok {
			kept[f] = v
		}
	}
	return kept
}
