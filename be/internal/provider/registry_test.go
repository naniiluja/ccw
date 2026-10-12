package provider

import (
	"strings"
	"testing"
)

func TestLookupReturnsClassBProviders(t *testing.T) {
	p, ok := Lookup("groq")
	if !ok {
		t.Fatal("groq is not registered")
	}
	if p.BaseURL != "https://api.groq.com/openai/v1" {
		t.Errorf("BaseURL = %q", p.BaseURL)
	}
	if p.AuthHeader != "Authorization" || p.AuthPrefix != "Bearer " {
		t.Errorf("auth = %q %q, want Authorization / Bearer ", p.AuthHeader, p.AuthPrefix)
	}

	if _, ok := Lookup("nope"); ok {
		t.Error("an unknown id must not resolve")
	}
}

// Class A means the request must look like the genuine tool. The values come from
// a capture of Claude Code 2.1.278 taken on 2026-09-20.
func TestClaudeCarriesTheIdentityOfTheRealTool(t *testing.T) {
	p, ok := Lookup("claude")
	if !ok {
		t.Fatal("claude is not registered")
	}
	if p.BaseURL != "https://api.anthropic.com/v1" {
		t.Errorf("BaseURL = %q", p.BaseURL)
	}
	// Identity is what names the tool. A caller must never be able to replace it,
	// because the point of this provider is that the upstream sees Claude Code.
	for k, want := range map[string]string{
		"User-Agent": "claude-cli/2.1.278 (external, sdk-cli)",
		"X-App":      "cli",
	} {
		if p.Identity[k] != want {
			t.Errorf("Identity[%q] = %q, want %q", k, p.Identity[k], want)
		}
	}
	// Defaults are features, not identity. The caller decides them.
	if p.Defaults["Anthropic-Version"] != "2023-06-01" {
		t.Errorf("Anthropic-Version default = %q", p.Defaults["Anthropic-Version"])
	}
	if p.Defaults["Anthropic-Beta"] == "" {
		t.Error("Anthropic-Beta must have a default: without it the upstream refuses the oauth token")
	}
	if _, clash := p.Identity["Anthropic-Beta"]; clash {
		t.Error("Anthropic-Beta must not be identity: it selects features and the caller must keep control of it")
	}
}

func TestNewProvidersRegistered(t *testing.T) {
	for _, id := range []string{"nvidia", "openrouter", "typesafe"} {
		p, ok := Lookup(id)
		if !ok {
			t.Errorf("provider %q not registered", id)
			continue
		}
		if p.BaseURL == "" || p.AuthHeader != "Authorization" || p.AuthPrefix != "Bearer " {
			t.Errorf("provider %q has an incomplete entry: %+v", id, p)
		}
	}
}

func TestDiaProviderRegistered(t *testing.T) {
	p, ok := Lookup("dia")
	if !ok {
		t.Fatal("dia is not registered")
	}
	// The bridge runs in the ccw process on loopback, so the base is local and
	// never a Dia network endpoint.
	if p.BaseURL != "http://127.0.0.1:20140" {
		t.Errorf("BaseURL = %q, want http://127.0.0.1:20140", p.BaseURL)
	}
	if p.API != "anthropic" {
		t.Errorf("API = %q, want anthropic", p.API)
	}
	// The bridge attaches the handler's own per-turn credential, so ccw sends
	// none and a caller's headers must not reach a local process.
	if p.AuthHeader != "" || p.AuthPrefix != "" {
		t.Errorf("auth = %q %q, want both empty", p.AuthHeader, p.AuthPrefix)
	}
	if !p.Clean {
		t.Error("Clean must be true for dia so caller headers stay out of the bridge")
	}
	if p.DefaultSecret != "bridge" {
		t.Errorf("DefaultSecret = %q, want bridge", p.DefaultSecret)
	}
	if !p.Watch {
		t.Error("Watch must be true for dia")
	}
	if p.Setup != "none" {
		t.Errorf("Setup = %q, want none (the bridge holds the credential)", p.Setup)
	}
	if !p.NoModelList {
		t.Error("NoModelList must be true for dia")
	}
	// Every registered name was measured by switching to it after another model
	// had already run in the same turn, which is the path that routes to Vertex
	// and is the one that rejects names. Only bare ids are registered, and a
	// colon would make the handler split the bridge's path in the wrong place.
	wantModels := []string{
		"claude-opus-5-5",
		"claude-opus-5",
		"claude-opus-4-8",
		"claude-sonnet-5",
		"claude-opus-4-6",
	}
	for _, m := range p.Models {
		if strings.Contains(m, "[") || strings.Contains(m, "]") {
			t.Errorf("model %q carries a Claude Code suffix, want a bare id", m)
		}
		if strings.Contains(m, ":") {
			t.Errorf("model %q has a colon, which the handler's path split would cut at", m)
		}
	}
	if len(p.Models) != len(wantModels) {
		t.Fatalf("Models len = %d, want %d", len(p.Models), len(wantModels))
	}
	for i, m := range wantModels {
		if p.Models[i] != m {
			t.Errorf("Models[%d] = %q, want %q", i, p.Models[i], m)
		}
	}
	if p.Identity["User-Agent"] != ClaudeInteractiveUserAgent {
		t.Errorf("Identity[User-Agent] = %q, want %q", p.Identity["User-Agent"], ClaudeInteractiveUserAgent)
	}
	if p.Defaults["Anthropic-Version"] != "2023-06-01" {
		t.Errorf("Anthropic-Version = %q, want 2023-06-01", p.Defaults["Anthropic-Version"])
	}
	// Dia's Bedrock upstream rejects Claude Code beta flags with a 400.
	if beta, ok := p.Defaults["Anthropic-Beta"]; ok && beta != "" {
		t.Errorf("Dia must not have Anthropic-Beta default: Bedrock rejects it (got %q)", beta)
	}
}
