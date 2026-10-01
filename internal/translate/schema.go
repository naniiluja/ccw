package translate

// schemaMaps are the keywords whose value maps a name to a schema. The map is
// not a schema itself: a parameter may be called "properties" or "format".
var schemaMaps = map[string]bool{"properties": true, "patternProperties": true, "$defs": true, "definitions": true}

// CleanChatSchema makes a tool schema safe for a chat provider that validates
// strictly. It drops what such a validator refuses ($schema, cache_control,
// encrypted, and uri formats) and gives a schema with no shape a type. Inside a
// schema 0, false, "" and null are values (minimum: 0, additionalProperties:
// false), so they are kept.
func CleanChatSchema(v any) any {
	if v == nil {
		return obj{"type": "object", "properties": obj{}}
	}
	return cleanChatNode(v)
}

func cleanChatNode(v any) any {
	switch s := v.(type) {
	case []any:
		out := make([]any, len(s))
		for i, e := range s {
			out[i] = cleanChatNode(e)
		}
		return out
	case obj:
		clean := obj{}
		for k, val := range s {
			if k == "$schema" || k == "cache_control" || k == "encrypted" {
				continue
			}
			if f := str(val); k == "format" && (f == "uri" || f == "uri-reference") {
				continue
			}
			if m := asObj(val); schemaMaps[k] && m != nil {
				sub := obj{}
				for name, node := range m {
					sub[name] = cleanChatNode(node)
				}
				clean[k] = sub
				continue
			}
			clean[k] = cleanChatNode(val)
		}
		// An empty schema carries no shape and a strict validator reads that as
		// a missing type. An anyOf or $ref shape is not handed a type it did not
		// ask for.
		if clean["type"] == nil && (clean["properties"] != nil || len(clean) == 0) {
			clean["type"] = "object"
		}
		if clean["type"] == "object" && clean["properties"] == nil {
			clean["properties"] = obj{}
		}
		return clean
	}
	return v
}
