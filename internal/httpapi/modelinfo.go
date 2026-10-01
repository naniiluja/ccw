package httpapi

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// ModelInfo is what a provider's model list says about a model's thinking.
// Thinking is nil when the provider says nothing about it.
type ModelInfo struct {
	Thinking *bool `json:"thinking,omitempty"`
	// Always: the model cannot answer without thinking.
	Always bool `json:"always,omitempty"`
	// Efforts are the levels the request may ask for, as the provider names them.
	Efforts []string `json:"efforts,omitempty"`
	Default string   `json:"default,omitempty"`
	// Paid: the provider serves it only on a paid plan.
	Paid bool `json:"paid,omitempty"`
	// Token limits: the whole window, the prompt, the answer. 0 is unknown.
	Context int64 `json:"context,omitempty"`
	Input   int64 `json:"input,omitempty"`
	Output  int64 `json:"output,omitempty"`
}

// modelInfos reads thinking support from a model list answer. Each provider
// says it its own way:
//
//	Anthropic   capabilities.thinking.supported, capabilities.effort.<level>.supported
//	Codex       supported_reasoning_levels[].effort, default_reasoning_level
//	Copilot     capabilities.supports.reasoning_effort, adaptive_thinking, max_thinking_budget
//	OpenRouter  supported_parameters has "reasoning" / "reasoning_effort", reasoning.mandatory
//	Groq        supported_features has "reasoning"
//	Antigravity models.<id>.supportsThinking
//	Cloudflare  properties: reasoning, reasoning_effort{supported_efforts, default_effort, mandatory}
//
// When a provider says it of any model, a model it says nothing of is marked
// as not thinking; a provider that never says leaves every model unknown.
func modelInfos(raw []byte) map[string]ModelInfo {
	var d struct {
		Data   []map[string]any `json:"data"`
		Result []map[string]any `json:"result"`
		Models json.RawMessage  `json:"models"`
	}
	if json.Unmarshal(raw, &d) != nil {
		return nil
	}
	items := map[string]map[string]any{}
	for _, m := range d.Data {
		if id := firstNonEmpty(str(m["id"]), str(m["slug"]), str(m["name"])); id != "" {
			items[id] = m
		}
	}
	// Cloudflare: id is a UUID, the model is its name.
	for _, m := range d.Result {
		if id := str(m["name"]); id != "" {
			items[id] = m
		}
	}
	var list []map[string]any
	var byKey map[string]map[string]any
	if json.Unmarshal(d.Models, &list) == nil {
		for _, m := range list {
			if id := firstNonEmpty(str(m["slug"]), str(m["id"]), str(m["name"])); id != "" {
				items[id] = m
			}
		}
	} else if json.Unmarshal(d.Models, &byKey) == nil {
		for id, m := range byKey {
			items[id] = m
		}
	}
	out := map[string]ModelInfo{}
	said := map[string]bool{}
	for id, m := range items {
		info, ok := readInfo(m)
		info.Context, info.Input, info.Output = readLimits(m)
		if ok {
			said[id] = true
		}
		if ok || info.Context+info.Input+info.Output > 0 {
			out[id] = info
		}
	}
	// Limits alone say nothing of thinking, so thinking stays unknown.
	if len(said) == 0 {
		if len(out) == 0 {
			return nil
		}
		return out
	}
	no := false
	for id := range items {
		if !said[id] {
			info := out[id]
			info.Thinking = &no
			out[id] = info
		}
	}
	return out
}

// readLimits reads a model's token limits under each provider's own field
// names; the first field found wins.
func readLimits(m map[string]any) (context, input, output int64) {
	pick := func(dst *int64, v any) {
		if n := tokenCount(v); n > 0 && *dst == 0 {
			*dst = n
		}
	}
	caps, _ := m["capabilities"].(map[string]any)
	if l, ok := caps["limits"].(map[string]any); ok {
		pick(&context, l["max_context_window_tokens"])
		pick(&input, l["max_prompt_tokens"])
		pick(&output, l["max_output_tokens"])
	}
	if tp, ok := m["top_provider"].(map[string]any); ok {
		pick(&context, tp["context_length"])
		pick(&output, tp["max_completion_tokens"])
	}
	for _, k := range []string{"context_length", "context_window", "maxTokens"} {
		pick(&context, m[k])
	}
	for _, k := range []string{"max_input_tokens", "inputTokenLimit"} {
		pick(&input, m[k])
	}
	for _, k := range []string{"max_tokens", "max_completion_tokens", "maxOutputTokens", "outputTokenLimit"} {
		pick(&output, m[k])
	}
	if props, ok := m["properties"].([]any); ok {
		for _, p := range props {
			if o, _ := p.(map[string]any); str(o["property_id"]) == "context_window" {
				pick(&context, o["value"])
			}
		}
	}
	return context, input, output
}

// tokenCount reads a count given as a number or as a numeric string.
func tokenCount(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	}
	return 0
}

func readInfo(m map[string]any) (ModelInfo, bool) {
	var info ModelInfo
	said := false
	yes := func(v bool) {
		said = true
		if info.Thinking == nil || v {
			info.Thinking = &v
		}
	}
	addEffort := func(e string) {
		if e != "" && !contains(info.Efforts, e) {
			info.Efforts = append(info.Efforts, e)
		}
	}
	caps, _ := m["capabilities"].(map[string]any)
	// Anthropic.
	if t, ok := caps["thinking"].(map[string]any); ok {
		yes(t["supported"] == true)
	}
	if e, ok := caps["effort"].(map[string]any); ok && e["supported"] == true {
		for _, l := range effortOrder {
			if v, ok := e[l].(map[string]any); ok && v["supported"] == true {
				addEffort(l)
			}
		}
	}
	// Copilot.
	if s, ok := caps["supports"].(map[string]any); ok {
		for _, e := range strs(s["reasoning_effort"]) {
			addEffort(e)
		}
		if len(info.Efforts) > 0 || s["adaptive_thinking"] == true || num(s["max_thinking_budget"]) > 0 {
			yes(true)
		} else {
			yes(false)
		}
	}
	// Codex.
	if levels, ok := m["supported_reasoning_levels"].([]any); ok {
		for _, l := range levels {
			if o, ok := l.(map[string]any); ok {
				addEffort(str(o["effort"]))
			}
		}
		yes(len(levels) > 0)
		info.Default = str(m["default_reasoning_level"])
	}
	// OpenRouter.
	if params, ok := m["supported_parameters"].([]any); ok {
		p := strs(params)
		yes(contains(p, "reasoning") || contains(p, "include_reasoning"))
		if contains(p, "reasoning_effort") && len(info.Efforts) == 0 {
			info.Efforts = []string{"low", "medium", "high"}
		}
		if r, ok := m["reasoning"].(map[string]any); ok && r["mandatory"] == true {
			info.Always = true
		}
	}
	// Groq.
	if f, ok := m["supported_features"].([]any); ok {
		yes(contains(strs(f), "reasoning"))
	}
	// Antigravity.
	if v, ok := m["supportsThinking"].(bool); ok {
		yes(v)
	}
	// Cloudflare: properties [{property_id, value}].
	if props, ok := m["properties"].([]any); ok {
		thinks := false
		for _, p := range props {
			o, _ := p.(map[string]any)
			switch str(o["property_id"]) {
			case "reasoning":
				thinks = thinks || str(o["value"]) == "true"
			case "reasoning_effort":
				thinks = true
				if v, ok := o["value"].(map[string]any); ok {
					for _, e := range strs(v["supported_efforts"]) {
						addEffort(e)
					}
					info.Default = str(v["default_effort"])
					info.Always = v["mandatory"] == true
				}
			case "require_workers_paid":
				info.Paid = str(o["value"]) == "true"
			}
		}
		yes(thinks)
	}
	return info, said
}

var effortOrder = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"}

// infoFor merges what a list says of a model's variants into the model.
func foldInfos(infos map[string]ModelInfo, groups map[string]variantSet) map[string]ModelInfo {
	if infos == nil {
		return nil
	}
	for base, g := range groups {
		var merged ModelInfo
		for _, l := range g.Variants() {
			i, ok := infos[g.ids[l]]
			if ok && i.Thinking != nil && (merged.Thinking == nil || *i.Thinking) {
				merged.Thinking = i.Thinking
			}
			merged.Context = max(merged.Context, i.Context)
			merged.Input = max(merged.Input, i.Input)
			merged.Output = max(merged.Output, i.Output)
			delete(infos, g.ids[l])
		}
		merged.Efforts = g.Variants()
		merged.Default = g.def
		infos[base] = merged
	}
	return infos
}

func str(v any) string { s, _ := v.(string); return s }

func num(v any) float64 { f, _ := v.(float64); return f }

func strs(v any) []string {
	var out []string
	if a, ok := v.([]any); ok {
		for _, x := range a {
			if s, ok := x.(string); ok {
				out = append(out, strings.ToLower(s))
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) })
	return out
}

func rank(e string) int {
	for i, x := range effortOrder {
		if x == e {
			return i
		}
	}
	return len(effortOrder)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Antigravity lists one model several times, once per thinking level:
// gemini-3.8-flash-high, -medium, -low, and -tiered (the one the IDE uses
// without a level). ccw lists them as one model, gemini-3.8-flash, and
// picks the variant from the effort the client asks for.

// variantSuffixes are the levels a model id can end with, longest first so
// "-extra-low" is not read as "-low".
var variantSuffixes = []string{"extra-low", "tiered", "medium", "high", "low"}

// variantRank orders the thinking levels, for the nearest match.
var variantRank = map[string]int{"extra-low": 0, "low": 1, "medium": 2, "high": 3}

// variantSet is the variants of one base model: level → upstream id. The
// level "" is the id without a level, when the provider lists one.
type variantSet struct {
	ids map[string]string
	def string
}

// Variants returns the levels in a stable order, the default first.
func (v variantSet) Variants() []string {
	out := make([]string, 0, len(v.ids))
	for l := range v.ids {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i] == v.def) != (out[j] == v.def) {
			return out[i] == v.def
		}
		ri, iok := variantRank[out[i]]
		rj, jok := variantRank[out[j]]
		if iok != jok {
			return iok
		}
		if ri != rj {
			return ri > rj
		}
		return out[i] < out[j]
	})
	return out
}

func splitVariant(id string) (base, level string) {
	for _, s := range variantSuffixes {
		if b, ok := strings.CutSuffix(id, "-"+s); ok && b != "" {
			return b, s
		}
	}
	return id, ""
}

// groupVariants folds the ids that differ only by a level into their base.
// The default variant is the id without a level, else -tiered, else the
// highest level.
func groupVariants(ids []string) ([]string, map[string]variantSet) {
	groups := map[string]variantSet{}
	for _, id := range ids {
		base, level := splitVariant(id)
		g, ok := groups[base]
		if !ok {
			g = variantSet{ids: map[string]string{}}
		}
		g.ids[level] = id
		groups[base] = g
	}
	bases := make([]string, 0, len(groups))
	for base, g := range groups {
		bases = append(bases, base)
		switch {
		case g.ids[""] != "":
			g.def = ""
		case g.ids["tiered"] != "":
			g.def = "tiered"
		default:
			best := -1
			for l := range g.ids {
				if r, ok := variantRank[l]; ok && r > best {
					best, g.def = r, l
				}
			}
		}
		groups[base] = g
		if len(g.ids) == 1 && g.def == "" {
			delete(groups, base) // a plain model, nothing folded
		}
	}
	sort.Strings(bases)
	return bases, groups
}

// pick returns the upstream id for an effort: a level the model has, the
// nearest one it has, or the default for none.
func (v variantSet) pick(effort string) string {
	switch effort {
	case "", "none", "auto", "default":
		return v.ids[v.def]
	case "minimal":
		effort = "extra-low"
	case "xhigh", "max":
		effort = "high"
	}
	if id, ok := v.ids[effort]; ok {
		return id
	}
	want, ok := variantRank[effort]
	if !ok {
		return v.ids[v.def]
	}
	best, bestD := "", 99
	for l := range v.ids {
		if r, ok := variantRank[l]; ok {
			d := r - want
			if d < 0 {
				d = -d
			}
			// On a tie, prefer the higher level.
			if d < bestD || (d == bestD && r > variantRank[best]) {
				best, bestD = l, d
			}
		}
	}
	if best == "" {
		return v.ids[v.def]
	}
	return v.ids[best]
}

// effortOf reads the thinking effort a request asks for, in any of the
// shapes ccw accepts: OpenAI reasoning_effort, Responses reasoning.effort,
// Anthropic output_config.effort or thinking.budget_tokens, Gemini
// thinkingConfig. "" means the client did not say.
func effortOf(body []byte) string {
	var b struct {
		ReasoningEffort string `json:"reasoning_effort"`
		Reasoning       struct {
			Effort string `json:"effort"`
		} `json:"reasoning"`
		OutputConfig struct {
			Effort string `json:"effort"`
		} `json:"output_config"`
		Thinking struct {
			Type         string `json:"type"`
			BudgetTokens int    `json:"budget_tokens"`
		} `json:"thinking"`
		GenerationConfig struct {
			ThinkingConfig struct {
				ThinkingLevel  string `json:"thinkingLevel"`
				ThinkingBudget *int   `json:"thinkingBudget"`
			} `json:"thinkingConfig"`
		} `json:"generationConfig"`
	}
	if json.NewDecoder(bytes.NewReader(body)).Decode(&b) != nil {
		return ""
	}
	budget := func(n int) string {
		switch {
		case n == 0:
			return "none"
		case n < 0:
			return "high"
		case n <= 1500:
			return "low"
		case n <= 6000:
			return "medium"
		}
		return "high"
	}
	tc := b.GenerationConfig.ThinkingConfig
	for _, e := range []string{b.ReasoningEffort, b.Reasoning.Effort, b.OutputConfig.Effort, tc.ThinkingLevel} {
		if e != "" {
			return strings.ToLower(e)
		}
	}
	switch b.Thinking.Type {
	case "disabled":
		return "none"
	case "enabled":
		return budget(b.Thinking.BudgetTokens)
	}
	if tc.ThinkingBudget != nil {
		return budget(*tc.ThinkingBudget)
	}
	return ""
}

// variants returns a provider's folded models (nil for a provider without).
func (a *api) variants(prov string) map[string]variantSet {
	a.cat.mu.Lock()
	defer a.cat.mu.Unlock()
	return a.cat.m[prov].groups
}

// variantBase returns the listed model an upstream id belongs to: its base
// when it is a folded variant, else the id itself.
func (a *api) variantBase(prov, model string) string {
	if base, level := splitVariant(model); level != "" {
		if g, ok := a.variants(prov)[base]; ok && g.ids[level] == model {
			return base
		}
	}
	return model
}

// resolveVariant turns a listed base model into the upstream id for the
// effort the request asks; any other model is left as it is.
func (a *api) resolveVariant(prov, model string, body []byte) string {
	if g, ok := a.variants(prov)[model]; ok {
		if id := g.pick(effortOf(body)); id != "" {
			return id
		}
	}
	return model
}
