package zen

// Endpoint is the upstream path family a model answers on. The Zen gateway
// speaks three dialects, one per model: Chat Completions, the Responses API, and
// TypeSafe's System One. A model on the wrong one answers 500.
type Endpoint int

const (
	Chat Endpoint = iota
	Responses
	SystemOne
)

// Profile is what ccw knows about one model beyond its name.
type Profile struct {
	ID       string
	Endpoint Endpoint
	// Aliases are other names for the model: a request with one goes upstream
	// with ID.
	Aliases []string
	// Pin holds request fields sent with this value whatever the caller sent,
	// for a model that accepts only one. It never names model, messages, stream
	// or stream_options.
	Pin map[string]any
}

// The models measured on 2026-09-30. A model with no profile is still served:
// its endpoint comes from models.dev, else Chat, and its name goes upstream as
// the caller wrote it.
var profiles = []Profile{
	{ID: "big-pickle"},
	{ID: "deepseek-v4-flash-free"},
	{ID: "ling-3.0-flash-fin-free"},
	{ID: "longcat-2.5-preview-free"},
	{ID: "mimo-v2.5-free"},
	{ID: "mimo-v2.6-flash-free"},
	{ID: "nemotron-3-ultra-free"},
	{ID: "nemotron-3.5-lightning-free"},
	{ID: "space-bunny-free"},
	// The muse models answer only on /responses, and only tool_choice "auto".
	{ID: "muse-spark-1.2-contributor-free", Endpoint: Responses, Pin: map[string]any{"tool_choice": "auto"}},
	{ID: "muse-spark-1.3-contributor-free", Endpoint: Responses, Pin: map[string]any{"tool_choice": "auto"}},
	// jev speaks System One only. ccw calls it by the TypeSafe name.
	{ID: "jev-1.13-free", Endpoint: SystemOne, Aliases: []string{"jev-latest"}},
}

// Resolve finds a profile by its id or an alias.
func Resolve(name string) (Profile, bool) {
	for _, p := range profiles {
		if p.ID == name {
			return p, true
		}
		for _, a := range p.Aliases {
			if a == name {
				return p, true
			}
		}
	}
	return Profile{}, false
}

// Route says where a model goes: the name the upstream serves, the endpoint, and
// the request fields to pin. catalogue is asked only for a model with no profile,
// so a known model never waits on models.dev; it may be nil.
func Route(name string, catalogue func() *Catalogue) (id string, ep Endpoint, pin map[string]any) {
	if p, ok := Resolve(name); ok {
		return p.ID, p.Endpoint, p.Pin
	}
	if catalogue != nil {
		if cat := catalogue(); cat != nil && cat.Responses[name] {
			return name, Responses, nil
		}
	}
	return name, Chat, nil
}

// Hidden reports whether a model is kept out of the chat list: a System One
// model cannot answer a chat call.
func Hidden(name string) bool {
	p, ok := Resolve(name)
	return ok && p.Endpoint == SystemOne && p.ID == name
}

// SystemOneNames are the names a System One call can use.
func SystemOneNames() []string {
	var out []string
	for _, p := range profiles {
		if p.Endpoint == SystemOne {
			out = append(out, p.Aliases...)
		}
	}
	return out
}

// ProfileIDs are the chat and Responses models ccw knows, for the list a
// dead upstream leaves behind.
func ProfileIDs() []string {
	var out []string
	for _, p := range profiles {
		if p.Endpoint != SystemOne {
			out = append(out, p.ID)
		}
	}
	return out
}
