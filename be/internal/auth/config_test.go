package auth

import (
	"testing"
)

func newTestConfig(t *testing.T) *Config {
	t.Helper()
	return cheapConfig(t, "test-password-123")
}

func TestCheckAPIToken(t *testing.T) {
	c := newTestConfig(t)
	if !c.CheckAPIToken("Bearer machine-token-abc") {
		t.Error("correct bearer token rejected")
	}
	if c.CheckAPIToken("Bearer nope") || c.CheckAPIToken("machine-token-abc") || c.CheckAPIToken("") {
		t.Error("bad authorization accepted")
	}
}

func TestSessionRoundTripThroughConfig(t *testing.T) {
	c := newTestConfig(t)
	tok := c.IssueSession()
	if !c.ValidSession(tok) {
		t.Error("issued session did not validate")
	}
	if c.ValidSession("garbage") {
		t.Error("garbage session validated")
	}
}

// A password is not single use: the same one opens a second session.
func TestCheckPasswordAcceptsTheSamePasswordTwice(t *testing.T) {
	c := newTestConfig(t)
	if !c.CheckPassword("test-password-123") {
		t.Fatal("the correct password was refused on the first sign-in")
	}
	if !c.CheckPassword("test-password-123") {
		t.Error("the correct password was refused on a repeated sign-in")
	}
}

func TestDeviceCookieRoundTrip(t *testing.T) {
	c := newTestConfig(t)
	value := c.IssueDevice()
	id, ok := c.DeviceID(value)
	if !ok || id == "" {
		t.Fatalf("DeviceID(%q) = %q, %v, want an id and true", value, id, ok)
	}
	if again, _ := c.DeviceID(c.IssueDevice()); again == id {
		t.Error("two devices got the same id")
	}
	other := NewConfig(c.passwordHash, c.APIToken, []byte("another-key"), 3600)
	for _, bad := range []string{"", "no-dot", id, id + ".", id + ".00", value + "x", "x" + value} {
		if _, ok := c.DeviceID(bad); ok {
			t.Errorf("DeviceID accepted a forged value %q", bad)
		}
	}
	if _, ok := other.DeviceID(value); ok {
		t.Error("a device cookie validated under another session key")
	}
}

// FromHash reads the session settings from the environment as before.
func TestFromHashReadsTheSessionEnvironment(t *testing.T) {
	hash, err := hashPassword("test-password-123", testIterations)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CCW_API_TOKEN", "env-token")
	t.Setenv("CCW_SESSION_TTL", "120")
	t.Setenv("CCW_SESSION_KEY", "env-key")
	c, err := FromHash(hash)
	if err != nil {
		t.Fatal(err)
	}
	if c.APIToken != "env-token" || c.TTLSeconds() != 120 || string(c.sessionKey) != "env-key" {
		t.Errorf("FromHash: token=%q ttl=%d key=%q", c.APIToken, c.TTLSeconds(), c.sessionKey)
	}
	if !c.CheckPassword("test-password-123") {
		t.Error("FromHash config refused its password")
	}
}
