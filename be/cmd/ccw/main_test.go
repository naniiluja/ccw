package main

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/naniiluja/ccw/internal/auth"
	"github.com/naniiluja/ccw/internal/store"
)

func TestAuthDecisionRefusesAnUngatedServer(t *testing.T) {
	err := authDecision(nil, false)
	if err == nil {
		t.Fatal("no sign-in gate and no opt-in: got nil, want an error")
	}
	if !strings.Contains(err.Error(), "-insecure-no-auth") {
		t.Errorf("message %q does not name the opt-in flag", err.Error())
	}
}

func TestAuthDecisionAcceptsTheExplicitOptIn(t *testing.T) {
	if err := authDecision(nil, true); err != nil {
		t.Errorf("with the opt-in: %v, want nil", err)
	}
}

func TestAuthDecisionAcceptsAConfiguredGate(t *testing.T) {
	if err := authDecision(&auth.Config{}, false); err != nil {
		t.Errorf("with a password gate: %v, want nil", err)
	}
}

// openTestStore opens a database in a fresh directory and returns its path.
func openTestStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ccw.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return s, path
}

// assertNoPlaintext fails when the password appears in any setting or in any
// file of the database (main file, WAL and shared memory).
func assertNoPlaintext(t *testing.T, s *store.Store, path, password string) {
	t.Helper()
	rows, err := s.DB.Query(`SELECT key, value FROM settings`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(v, password) {
			t.Errorf("setting %q holds the plaintext password", k)
		}
	}
	rows.Close()
	s.Close()
	files, _ := filepath.Glob(path + "*")
	if len(files) == 0 {
		t.Fatal("no database file to scan")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(b, []byte(password)) {
			t.Errorf("%s holds the plaintext password", filepath.Base(f))
		}
	}
}

var noticePassword = regexp.MustCompile(`Password: (\S+)`)

// On its first start an install generates a password, prints it once, and
// keeps only its hash. A restart prints nothing and keeps the same hash.
func TestFirstStartPrintsThePasswordOnceAndStoresOnlyAHash(t *testing.T) {
	s, path := openTestStore(t)
	var first bytes.Buffer
	cfg, err := setupAuth(s, "", false, &first)
	if err != nil || cfg == nil {
		t.Fatalf("first start: cfg=%v err=%v", cfg, err)
	}
	m := noticePassword.FindStringSubmatch(first.String())
	if m == nil {
		t.Fatalf("the first start printed no password:\n%s", first.String())
	}
	password := m[1]
	if n := strings.Count(first.String(), password); n != 1 {
		t.Errorf("the password is printed %d times, want once", n)
	}
	if !cfg.CheckPassword(password) {
		t.Error("the printed password does not open the gate")
	}
	hash, _ := s.GetSetting(passwordSetting)
	if !strings.HasPrefix(hash, "pbkdf2-sha256$") {
		t.Errorf("setting %q = %q, want a pbkdf2 hash", passwordSetting, hash)
	}

	var again bytes.Buffer
	cfg2, err := setupAuth(s, "", false, &again)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(again.String(), password) || noticePassword.MatchString(again.String()) {
		t.Errorf("a restart printed the password again:\n%s", again.String())
	}
	if saved, _ := s.GetSetting(passwordSetting); saved != hash {
		t.Error("a restart replaced the stored hash")
	}
	if !cfg2.CheckPassword(password) {
		t.Error("after a restart the password no longer opens the gate")
	}
	assertNoPlaintext(t, s, path, password)
}

// A password from the environment is never printed, never stored in clear, and
// wins over the stored one.
func TestAnEnvironmentPasswordIsNeitherLoggedNorStored(t *testing.T) {
	const password = "env-password-0123"
	s, path := openTestStore(t)
	var log bytes.Buffer
	if _, err := setupAuth(s, "", false, &log); err != nil {
		t.Fatal(err)
	}
	log.Reset()
	cfg, err := setupAuth(s, password, false, &log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(log.String(), password) {
		t.Errorf("the startup output holds the environment password:\n%s", log.String())
	}
	if !cfg.CheckPassword(password) {
		t.Error("the environment password does not open the gate")
	}
	assertNoPlaintext(t, s, path, password)
}

// A password shorter than 12 characters stops the start with an error that
// names the variable and the minimum, and does not repeat the value.
func TestAShortPasswordRefusesToStart(t *testing.T) {
	s, _ := openTestStore(t)
	defer s.Close()
	for _, short := range []string{"elevenchars", strings.Repeat("é", 11)} {
		var out bytes.Buffer
		cfg, err := setupAuth(s, short, false, &out)
		if err == nil || cfg != nil {
			t.Fatalf("%q: cfg=%v err=%v, want a refusal", short, cfg, err)
		}
		msg := err.Error()
		if !strings.Contains(msg, "CCW_PASSWORD") || !strings.Contains(msg, "12") {
			t.Errorf("message %q does not name CCW_PASSWORD and the minimum 12", msg)
		}
		if strings.Contains(msg, short) || strings.Contains(out.String(), short) {
			t.Errorf("the refusal repeats the password: %q", msg)
		}
	}
	if _, err := setupAuth(s, "twelve-chars", false, &bytes.Buffer{}); err != nil {
		t.Errorf("a 12-character password was refused: %v", err)
	}
}

// -reset-password replaces the stored hash: the old password stops working and
// the new one, printed once, opens the gate.
func TestResetPasswordRetiresTheOldOne(t *testing.T) {
	s, path := openTestStore(t)
	var first bytes.Buffer
	if _, err := setupAuth(s, "", false, &first); err != nil {
		t.Fatal(err)
	}
	old := noticePassword.FindStringSubmatch(first.String())[1]

	var reset bytes.Buffer
	if err := resetPassword(s, &reset); err != nil {
		t.Fatal(err)
	}
	m := noticePassword.FindStringSubmatch(reset.String())
	if m == nil {
		t.Fatalf("the reset printed no password:\n%s", reset.String())
	}
	fresh := m[1]
	if fresh == old {
		t.Fatal("the reset kept the old password")
	}
	cfg, err := setupAuth(s, "", false, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CheckPassword(old) {
		t.Error("the old password still opens the gate after a reset")
	}
	if !cfg.CheckPassword(fresh) {
		t.Error("the new password does not open the gate")
	}
	assertNoPlaintext(t, s, path, fresh)
}

// With the insecure opt-in and no password, the server starts open and says so.
func TestInsecureOptInStartsWithNoGate(t *testing.T) {
	s, _ := openTestStore(t)
	defer s.Close()
	var out bytes.Buffer
	cfg, err := setupAuth(s, "", true, &out)
	if err != nil || cfg != nil {
		t.Fatalf("cfg=%v err=%v, want no gate and no error", cfg, err)
	}
	if !strings.Contains(out.String(), "not authenticated") {
		t.Errorf("no warning about the open server:\n%s", out.String())
	}
	if hash, _ := s.GetSetting(passwordSetting); hash != "" {
		t.Error("an ungated start generated a password")
	}
}

// An idle keep-alive connection is closed, but no write deadline cuts a long
// stream.
func TestServerHasIdleTimeout(t *testing.T) {
	srv := newHTTPServer("127.0.0.1:0", http.NotFoundHandler())
	if srv.IdleTimeout <= 0 {
		t.Errorf("IdleTimeout = %v, want > 0", srv.IdleTimeout)
	}
	if srv.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %v, want 0 so a stream is not cut", srv.WriteTimeout)
	}
	if srv.ReadHeaderTimeout <= 0 {
		t.Errorf("ReadHeaderTimeout = %v, want > 0", srv.ReadHeaderTimeout)
	}
}
