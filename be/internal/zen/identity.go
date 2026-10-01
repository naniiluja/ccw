// Package zen is what ccw needs to reach the OpenCode Zen free tier.
//
// The free tier is meant to be called from inside the OpenCode agent, and the
// upstream reads the request it gets. Measured by replaying a captured OpenCode
// request with one element changed per run:
//
//   - User-Agent carries "opencode/" and a version >= 1.18.0, or it answers 426;
//   - X-Opencode-Session (or X-Session-Id) holds an id shaped "ses_" + 12
//     lowercase hex + 14 base62, or the gated models answer 403 "free tier can
//     only be used from within OpenCode";
//   - the body has "stream": true, and a tools array holding both "read" and
//     "shell".
//
// This package holds the parts of that which do not depend on HTTP: reading a
// caller's session id, the pool that maps it to an upstream one, and the request
// bodies the upstream accepts.
package zen

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

// callerHeaders are the headers a caller's session id can arrive in, most
// specific first. Each is per session: a header whose value changes per request
// (a request id) is left out, because it would mint a session per call and
// defeat the prompt cache the mapping exists to keep.
var callerHeaders = []string{
	"X-Claude-Code-Session-Id", // Claude Code
	"X-Session-Id",
	"X-Opencode-Session", // an OpenCode client: already an upstream id
}

// bodySessionFields name a session at the top of a body.
var bodySessionFields = []string{"session_id", "conversation_id", "thread_id"}

// callerBodyFields carry the caller's identity in a body: the fields above,
// their camelCase twins, and the records Claude Code packs its session and
// device into (metadata, and the user field that the Anthropic to chat
// conversion makes of metadata.user_id). They are read to key the session, then
// dropped: the upstream gets its identity from the headers ccw sets.
var callerBodyFields = []string{
	"session_id", "sessionId", "conversation_id", "conversationId",
	"thread_id", "threadId", "metadata", "user",
}

// sesShape is the upstream session id in OpenCode's own shape. A caller that
// already sends one is an OpenCode client, and its id is used as it stands.
var sesShape = regexp.MustCompile(`^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`)

// IsSessionID reports whether v is already an upstream session id.
func IsSessionID(v string) bool { return sesShape.MatchString(v) }

// Caller reads the caller's own session id and where it came from, or empty
// strings when the request carries none. Headers are read first: they are cheap
// and unambiguous. payload is the caller's body as it arrived, before any
// translation, because a translation drops the fields the id lives in.
func Caller(h http.Header, payload map[string]any) (value, source string) {
	for _, name := range callerHeaders {
		raw := strings.TrimSpace(h.Get(name))
		if raw == "" {
			continue
		}
		if raw != "" {
			return raw, "header:" + strings.ToLower(name)
		}
	}
	for _, f := range bodySessionFields {
		if v := text(payload[f]); v != "" {
			return v, "body:" + f
		}
	}
	if md, ok := payload["metadata"].(map[string]any); ok {
		if v := text(md["session_id"]); v != "" {
			return v, "body:metadata.session_id"
		}
		// Claude Code keeps its whole session record in user_id, as JSON text.
		if v := sessionInJSON(text(md["user_id"])); v != "" {
			return v, "body:metadata.user_id"
		}
	}
	return "", ""
}

// sessionInJSON reads session_id out of a JSON object held in a string, or "".
func sessionInJSON(s string) string {
	if !strings.HasPrefix(strings.TrimSpace(s), "{") {
		return ""
	}
	var rec map[string]any
	if json.Unmarshal([]byte(s), &rec) != nil {
		return ""
	}
	return text(rec["session_id"])
}

func text(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}
