package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/naniiluja/ccw/internal/auth"
	"github.com/naniiluja/ccw/internal/store"
)

func TestAuthDecisionRefusesAnUngatedServer(t *testing.T) {
	err := authDecision(nil, false)
	if err == nil {
		t.Fatal("no TOTP secret and no opt-in: got nil, want an error")
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
		t.Errorf("with a TOTP secret: %v, want nil", err)
	}
}

// Each new install gets its own TOTP secret on its first start, kept in its database and
// shown once with the steps to add it to an authenticator app. An environment secret wins.
func TestTOTPSecretIsCreatedOncePerInstall(t *testing.T) {
	open := func(name string) *store.Store {
		s, err := store.Open(filepath.Join(t.TempDir(), name))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		return s
	}
	a, b := open("a.db"), open("b.db")
	secretA, fresh, err := totpSecret(a, "")
	if err != nil || !fresh || auth.ValidateSecret(secretA) != nil {
		t.Fatalf("first start: %q fresh=%v %v", secretA, fresh, err)
	}
	again, fresh, _ := totpSecret(a, "")
	if again != secretA || fresh {
		t.Errorf("restart: %q fresh=%v, want the same secret, not fresh", again, fresh)
	}
	if secretB, _, _ := totpSecret(b, ""); secretB == secretA {
		t.Error("two installs share one secret")
	}
	if got, fresh, _ := totpSecret(a, "JBSWY3DPEHPK3PXP"); got != "JBSWY3DPEHPK3PXP" || fresh {
		t.Errorf("the environment secret: %q fresh=%v", got, fresh)
	}
	guide := enrollmentGuide(secretA)
	for _, want := range []string{secretA, "otpauth://totp/ccw?secret=" + secretA, "authenticator", "-show-totp"} {
		if !strings.Contains(guide, want) {
			t.Errorf("the guide misses %q:\n%s", want, guide)
		}
	}
}
