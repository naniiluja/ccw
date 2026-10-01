package store

import (
	"path/filepath"
	"regexp"
	"testing"
)

// Antigravity answers a false 429 to these client sentences. A fresh install
// must already strip them, or Claude Code fails on their first call.
func TestAFreshDatabaseSeedsTheAntigravityFingerprintRules(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	list, err := s.ListFilters()
	if err != nil {
		t.Fatal(err)
	}
	for sentence, want := range map[string]bool{
		"You are a Claude agent, built on Anthropic's Claude Agent SDK": true,
		"You are Claude Code, Anthropic's official CLI for Claude.":     false,
		"Write code that a Claude agent, or a person, can read":         true,
	} {
		hit := false
		for _, f := range list {
			if f.Provider != "antigravity" || f.Kind != "system" || !f.Enabled {
				continue
			}
			re, err := regexp.Compile(f.Pattern)
			if err != nil {
				t.Fatalf("seeded pattern %q does not compile: %v", f.Pattern, err)
			}
			if re.MatchString(sentence) {
				hit = true
			}
		}
		if hit != want {
			t.Errorf("%q: stripped = %v, want %v", sentence, hit, want)
		}
	}
}
