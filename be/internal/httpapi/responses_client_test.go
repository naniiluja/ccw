package httpapi

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/naniiluja/ccw/internal/store"
)

// ccw serves chat completions and messages to callers, not the Responses
// API, so a Responses call is an unknown URL rather than a forwarded request.
func TestResponsesIsNotServedToCallers(t *testing.T) {
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer s.Close()
	s.CreateConnection("groq", "g", "kg")
	h := New(s, nil)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, loopbackRequest("POST", "/v1/responses", strings.NewReader(`{"model":"groq/llama","input":"hi"}`)))
	if rec.Code != http.StatusNotFound {
		t.Errorf("code=%d body=%s, want 404", rec.Code, rec.Body.String())
	}
}
