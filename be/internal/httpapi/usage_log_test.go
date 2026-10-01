package httpapi

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/naniiluja/ccw/internal/store"
)

// Bug G: a store write failure must be logged, not swallowed, so a lock or a
// full disk is visible instead of usage vanishing in silence.
func TestRecordUsageLogsStoreError(t *testing.T) {
	s, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	c, _ := s.CreateConnection("groq", "test", "gsk-abc")
	s.Close() // any write now fails

	var logBuf bytes.Buffer
	lg := slog.New(slog.NewTextHandler(&logBuf, nil))

	a := &api{store: s}
	a.recordUsage(lg, c.ID, "", []byte(`{"model":"m","usage":{"prompt_tokens":1,"completion_tokens":1}}`), "")

	if !strings.Contains(logBuf.String(), "usage.record.fail") {
		t.Errorf("store error was not logged: %q", logBuf.String())
	}
}
