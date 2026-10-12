package diabridge

import "strings"

// modelMarker is the segment of the Vertex-shaped path that precedes the model
// id. Dia's handler splits the path on the same text.
const modelMarker = "publishers/anthropic/models/"

// modelFromPath pulls the model id out of a Vertex-shaped request path such as
// /v1/projects/p/locations/l/publishers/anthropic/models/claude-opus-4-8:streamRawPredict
// and returns the bare id. It reports ok false when the path is not that shape,
// which is what a caller logging an unknown Dia request wants to know.
func modelFromPath(p string) (string, bool) {
	_, after, found := strings.Cut(p, modelMarker)
	if !found {
		return "", false
	}
	id := strings.TrimPrefix(after, "/")
	// The endpoint suffix is separated by a colon, and the handler's own
	// pattern for the model stops at the first colon too.
	if i := strings.IndexByte(id, ':'); i >= 0 {
		id = id[:i]
	}
	if id == "" {
		return "", false
	}
	return id, true
}
