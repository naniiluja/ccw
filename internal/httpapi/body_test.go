package httpapi

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/naniiluja/ccw/internal/store"
)

// bodyRig serves a groq account from a stub and reports what the stub got.
func bodyRig(t *testing.T) (h http.Handler, got func() (body, encoding string)) {
	t.Helper()
	var gotBody, gotEnc string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody, gotEnc = string(b), r.Header.Get("Content-Encoding")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(up.Close)
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.CreateConnection("groq", "test", "gsk-abc"); err != nil {
		t.Fatal(err)
	}
	return New(s, map[string]string{"groq": up.URL}), func() (string, string) { return gotBody, gotEnc }
}

func postEncoded(h http.Handler, body []byte, encoding string) *httptest.ResponseRecorder {
	req := loopbackRequest("POST", "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if encoding != "" {
		req.Header.Set("Content-Encoding", encoding)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

const plainBody = `{"model":"llama-3.3-70b-versatile","messages":[]}`

func TestACompressedRequestBodyIsReadAndForwardedPlain(t *testing.T) {
	for _, enc := range []string{"gzip", "deflate"} {
		h, got := bodyRig(t)
		var buf bytes.Buffer
		if enc == "gzip" {
			zw := gzip.NewWriter(&buf)
			zw.Write([]byte(`{"model":"groq/llama-3.3-70b-versatile","messages":[]}`))
			zw.Close()
		} else {
			zw := zlib.NewWriter(&buf)
			zw.Write([]byte(`{"model":"groq/llama-3.3-70b-versatile","messages":[]}`))
			zw.Close()
		}
		if rec := postEncoded(h, buf.Bytes(), enc); rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, body = %s", enc, rec.Code, rec.Body.String())
		}
		if body, gotEnc := got(); body != plainBody || gotEnc != "" {
			t.Errorf("%s: upstream got %q with Content-Encoding %q, want the plain JSON and no encoding", enc, body, gotEnc)
		}
	}
}

func TestAnEncodingCcwCannotReadIsRefusedByName(t *testing.T) {
	h, _ := bodyRig(t)
	rec := postEncoded(h, []byte("x"), "zstd")
	if rec.Code != http.StatusUnsupportedMediaType || !strings.Contains(rec.Body.String(), "zstd") {
		t.Errorf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestATooLargeRequestIsRefusedWhetherOrNotItIsCompressed(t *testing.T) {
	old := maxV1Body
	maxV1Body = 1 << 10
	defer func() { maxV1Body = old }()
	h, _ := bodyRig(t)

	if rec := postEncoded(h, bytes.Repeat([]byte("a"), 2<<10), ""); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("plain: status = %d, want 413", rec.Code)
	}
	// A few bytes on the wire that inflate past the limit are refused as well.
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write(bytes.Repeat([]byte("a"), 64<<10))
	zw.Close()
	if buf.Len() >= 1<<10 {
		t.Fatalf("test body did not compress small enough: %d", buf.Len())
	}
	if rec := postEncoded(h, buf.Bytes(), "gzip"); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("gzip bomb: status = %d, want 413", rec.Code)
	}
}

// An Anthropic upstream verifies what other providers let through: a call with
// no result and a thinking block with no signature both come back as a 400 from it.
func TestAnAnthropicUpstreamGetsAHealedHistory(t *testing.T) {
	var got string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer up.Close()
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	s.CreateConnection("claude", "c", "tok")
	h := New(s, map[string]string{"claude": up.URL})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, loopbackRequest("POST", "/v1/messages", strings.NewReader(`{"model":"claude/x","max_tokens":9,"messages":[
	 {"role":"user","content":"go"},
	 {"role":"assistant","content":[{"type":"thinking","thinking":"hm","signature":""},{"type":"tool_use","id":"t1","name":"a","input":{}}]},
	 {"role":"user","content":"where is it"}]}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(got, `"thinking":"hm"`) || !strings.Contains(got, `"tool_use_id":"t1"`) || !strings.Contains(got, `"model":"x"`) {
		t.Errorf("upstream got an unhealed history: %s", got)
	}
}

// Claude Code appends [1m] to a model id it has a 1M window for. The suffix is a
// note to the tool: the provider must get the model's own name.
func TestTheOneMillionMarkIsNotPartOfTheModelName(t *testing.T) {
	h, got := bodyRig(t)
	rec := postEncoded(h, []byte(`{"model":"groq/llama-3.3-70b-versatile[1m]","messages":[]}`), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if body, _ := got(); body != plainBody {
		t.Errorf("upstream got %q, want %q", body, plainBody)
	}
}
