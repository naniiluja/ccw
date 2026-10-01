package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoFile reads a file at the repository root. The tests of this file guard the
// files a public release needs, so they must read the real tree, not a fixture.
func repoFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func TestLicenseIsMIT(t *testing.T) {
	l := repoFile(t, "LICENSE")
	for _, want := range []string{"MIT License", "Copyright (c) 2026 Louis Phạm",
		"Permission is hereby granted, free of charge"} {
		if !strings.Contains(l, want) {
			t.Errorf("LICENSE misses %q", want)
		}
	}
}

func TestGitignoreExcludesEnvFiles(t *testing.T) {
	g := repoFile(t, ".gitignore")
	for _, want := range []string{"*.env", "ccw.env"} {
		if !strings.Contains(g, want) {
			t.Errorf(".gitignore misses %q", want)
		}
	}
}

// TestGodocDoesNotClaimAPassword pins T7-6 (1): login is TOTP-only.
func TestGodocDoesNotClaimAPassword(t *testing.T) {
	for _, name := range []string{"internal/auth/totp.go", "internal/httpapi/server.go"} {
		for _, line := range strings.Split(repoFile(t, name), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") && strings.Contains(line, "password") {
				t.Errorf("%s: comment still names a password: %s", name, strings.TrimSpace(line))
			}
		}
	}
}

// workflowJob returns the lines of one job under `jobs:` in a workflow file,
// from its `  name:` header up to the next job header at the same indent.
func workflowJob(workflow, name string) string {
	var job []string
	in := false
	for _, line := range strings.Split(workflow, "\n") {
		isHeader := strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") && strings.HasSuffix(strings.TrimSpace(line), ":")
		if isHeader {
			in = strings.TrimSpace(line) == name+":"
		}
		if in {
			job = append(job, line)
		}
	}
	return strings.Join(job, "\n")
}

// TestReleaseWorkflowGatesPublishOnTests keeps a red go vet or go test from
// shipping: the publish job must wait for the test job, the test job must also
// run on pushes to master and pull requests, and publish must stay tag-only.
func TestReleaseWorkflowGatesPublishOnTests(t *testing.T) {
	w := repoFile(t, ".github/workflows/github-packages.yml")
	for _, want := range []string{"pull_request", "branches: [master]"} {
		if !strings.Contains(w, want) {
			t.Errorf("workflow triggers miss %q", want)
		}
	}
	test := workflowJob(w, "test")
	if test == "" {
		t.Fatal("workflow has no test job")
	}
	for _, want := range []string{"go-version-file: go.mod", "go vet ./...", "go test -race ./..."} {
		if !strings.Contains(test, want) {
			t.Errorf("test job misses %q", want)
		}
	}
	publish := workflowJob(w, "publish")
	if publish == "" {
		t.Fatal("workflow has no publish job")
	}
	if !strings.Contains(publish, "needs: test") {
		t.Error("publish job does not declare needs: test")
	}
	if !strings.Contains(publish, "startsWith(github.ref, 'refs/tags/v')") {
		t.Error("publish job is not limited to v* tags")
	}
}

// TestFixturesLookLikePlaceholders keeps fake tokens out of a secret scanner's
// Google pattern (`ya29.`).
func TestFixturesLookLikePlaceholders(t *testing.T) {
	for _, name := range []string{"internal/httpapi/antigravity_test.go", "internal/httpapi/variants_test.go"} {
		if strings.Contains(repoFile(t, name), "ya29.") {
			t.Errorf("%s still holds a ya29. fixture", name)
		}
	}
}
