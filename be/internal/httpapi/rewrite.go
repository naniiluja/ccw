package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
)

// replyEditor edits an answer on its way to the caller, and nothing else about
// it. It puts back the model id the caller sent, because ccw removed its
// provider prefix on the way in, and it takes out usage that ccw asked for
// on the caller's behalf. The usage tap reads the upstream bytes before this
// writer, so the counter still sees everything.
type replyEditor struct {
	http.ResponseWriter
	model     []byte // the caller's model id as a JSON string, nil for no change
	dropUsage bool
	stream    bool
	off       bool // an encoded body passes through as it came
	pending   []byte
}

var (
	modelField = regexp.MustCompile(`"model"\s*:\s*"(?:[^"\\]|\\.)*"`)
	frameEnd   = regexp.MustCompile(`\r?\n\r?\n`)
	nullUsage  = regexp.MustCompile(`,\s*"usage"\s*:\s*null|"usage"\s*:\s*null\s*,?`)
)

// newReplyEditor returns w itself when there is nothing to edit.
func newReplyEditor(w http.ResponseWriter, callerModel, upstreamModel string, dropUsage bool) (http.ResponseWriter, func()) {
	e := &replyEditor{ResponseWriter: w, dropUsage: dropUsage}
	if callerModel != "" && callerModel != upstreamModel {
		e.model, _ = json.Marshal(callerModel)
	}
	if e.model == nil && !dropUsage {
		return w, func() {}
	}
	return e, e.finish
}

func (e *replyEditor) Unwrap() http.ResponseWriter { return e.ResponseWriter }

func (e *replyEditor) WriteHeader(code int) {
	h := e.Header()
	if h.Get("Content-Encoding") != "" {
		e.off = true
	} else {
		e.stream = strings.Contains(h.Get("Content-Type"), "event-stream")
		h.Del("Content-Length")
	}
	e.ResponseWriter.WriteHeader(code)
}

func (e *replyEditor) Write(p []byte) (int, error) {
	if e.off {
		return e.ResponseWriter.Write(p)
	}
	e.pending = append(e.pending, p...)
	if !e.stream {
		return len(p), nil
	}
	for {
		loc := frameEnd.FindIndex(e.pending)
		if loc == nil {
			return len(p), nil
		}
		frame := e.pending[:loc[1]]
		e.pending = e.pending[loc[1]:]
		if out := e.frame(frame); len(out) > 0 {
			if _, err := e.ResponseWriter.Write(out); err != nil {
				return 0, err
			}
		}
	}
}

// Flush sends whole frames only; a half frame waits for the rest of itself.
func (e *replyEditor) Flush() {
	_ = http.NewResponseController(e.ResponseWriter).Flush()
}

func (e *replyEditor) finish() {
	if len(e.pending) == 0 {
		return
	}
	out := e.pending
	e.pending = nil
	if e.stream {
		out = e.frame(out)
	} else {
		out = e.setModel(out)
	}
	e.ResponseWriter.Write(out)
	e.Flush()
}

// frame edits one SSE frame; nil drops it.
func (e *replyEditor) frame(f []byte) []byte {
	if e.dropUsage && bytes.Contains(f, []byte(`"usage"`)) {
		if usageOnly(f) {
			return nil
		}
		f = nullUsage.ReplaceAll(f, nil)
	}
	return e.setModel(f)
}

// setModel replaces the first model value, the top-level one in every shape
// ccw relays. A "model" inside a string is escaped and never matches.
func (e *replyEditor) setModel(b []byte) []byte {
	if e.model == nil {
		return b
	}
	loc := modelField.FindIndex(b)
	if loc == nil {
		return b
	}
	out := make([]byte, 0, len(b)+len(e.model))
	out = append(out, b[:loc[0]]...)
	out = append(out, `"model":`...)
	out = append(out, e.model...)
	return append(out, b[loc[1]:]...)
}

// usageOnly reports the chunk that carries usage and no choice.
func usageOnly(frame []byte) bool {
	for _, line := range bytes.Split(frame, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		var c struct {
			Choices []json.RawMessage `json:"choices"`
			Usage   json.RawMessage   `json:"usage"`
		}
		if json.Unmarshal(bytes.TrimSpace(line[len("data:"):]), &c) != nil {
			return false
		}
		return len(c.Choices) == 0 && len(c.Usage) > 0 && string(c.Usage) != "null"
	}
	return false
}

// withUsage asks a Chat Completions stream for its usage, so ccw can count
// it; it reports whether it changed the body. Only the stream_options value is
// rewritten, every other byte stays as the caller sent it.
func withUsage(body []byte) ([]byte, bool) {
	var in struct {
		Stream        bool `json:"stream"`
		StreamOptions *struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options"`
	}
	if json.Unmarshal(body, &in) != nil || !in.Stream {
		return body, false
	}
	if in.StreamOptions != nil && in.StreamOptions.IncludeUsage {
		return body, false
	}
	start, end, err := topLevelValue(body, "stream_options")
	if err != nil {
		i := bytes.IndexByte(body, '{')
		if i < 0 {
			return body, false
		}
		out := append([]byte{}, body[:i+1]...)
		out = append(out, `"stream_options":{"include_usage":true}`...)
		if rest := bytes.TrimSpace(body[i+1:]); len(rest) > 0 && rest[0] != '}' {
			out = append(out, ',')
		}
		return append(out, body[i+1:]...), true
	}
	var opts map[string]json.RawMessage
	if json.Unmarshal(body[start:end], &opts) != nil || opts == nil {
		opts = map[string]json.RawMessage{}
	}
	opts["include_usage"] = json.RawMessage("true")
	val, _ := json.Marshal(opts)
	out := append([]byte{}, body[:start]...)
	out = append(out, val...)
	return append(out, body[end:]...), true
}

// setModel returns body with its top-level "model" value replaced by model.
//
// It splices the bytes instead of decoding and re-encoding, so every other byte
// of the caller's request (key order, number formatting, whitespace) reaches the
// upstream as sent. A body that is not a JSON object, or has no top-level model,
// comes back unchanged with ok false.
func setModel(body []byte, model string) (out []byte, ok bool) {
	start, end, err := topLevelValue(body, "model")
	if err != nil {
		return body, false
	}
	val, err := json.Marshal(model)
	if err != nil {
		return body, false
	}
	out = make([]byte, 0, len(body)-(end-start)+len(val))
	out = append(out, body[:start]...)
	out = append(out, val...)
	out = append(out, body[end:]...)
	return out, true
}

// topLevelCount counts how often key appears at the top level of a JSON object.
// JSON allows a repeated key and readers disagree on which one wins, so the
// model path refuses a body that names the model twice.
func topLevelCount(body []byte, key string) int {
	dec := json.NewDecoder(bytes.NewReader(body))
	tok, err := dec.Token()
	if err != nil {
		return 0
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return 0
	}
	n := 0
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return n
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return n
		}
		if k, _ := kt.(string); k == key {
			n++
		}
	}
	return n
}

// topLevelValue finds the byte range of the value of key in a JSON object,
// looking at the top level only (a "model" nested in a message is not it).
func topLevelValue(body []byte, key string) (start, end int, err error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	tok, err := dec.Token()
	if err != nil {
		return 0, 0, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return 0, 0, errors.New("not a JSON object")
	}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return 0, 0, err
		}
		k, _ := kt.(string)
		// The offset sits just after the key; the value starts past the colon.
		keyEnd := int(dec.InputOffset())
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return 0, 0, err
		}
		if k != key {
			continue
		}
		valEnd := int(dec.InputOffset())
		i := bytes.IndexByte(body[keyEnd:valEnd], ':')
		if i < 0 {
			return 0, 0, errors.New("malformed object")
		}
		valStart := keyEnd + i + 1
		for valStart < valEnd && isSpace(body[valStart]) {
			valStart++
		}
		return valStart, valEnd, nil
	}
	return 0, 0, errors.New("key not found")
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
