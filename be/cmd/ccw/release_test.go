package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// repoRoot returns the repository root: the nearest directory above the
// working directory that holds CLAUDE.md. The Go module lives in be/, so the
// root is no longer a fixed number of levels above this package.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("found no CLAUDE.md above the working directory")
		}
		dir = parent
	}
}

// repoFile reads a file at the repository root. The tests of this file guard the
// files a public release needs, so they must read the real tree, not a fixture.
func repoFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), name))
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

// samplePassword matches an example value for CCW_PASSWORD written in a
// comment. A comment may name the variable but must not show a password a
// reader could copy into a deployment.
var samplePassword = regexp.MustCompile(`CCW_PASSWORD=[^\s<>]`)

// TestCommentsShowNoSamplePassword keeps sample passwords out of the godoc and
// comments of every Go source file. The tests in main_test.go hold the rest of
// the invariant: the plaintext is in neither the database nor the startup log.
func TestCommentsShowNoSamplePassword(t *testing.T) {
	for _, name := range repoGoFiles(t, false) {
		for _, line := range strings.Split(repoFile(t, name), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") && samplePassword.MatchString(trimmed) {
				t.Errorf("%s: comment shows a sample password: %s", name, trimmed)
			}
		}
	}
}

// repoGoFiles lists the Go files of the repository, relative to its root. It
// skips hidden directories (.git, and .claude whose worktrees hold older copies
// of this tree) and node_modules. With tests set it lists only the _test.go
// files, otherwise only the others.
func repoGoFiles(t *testing.T, tests bool) []string {
	t.Helper()
	root := repoRoot(t)
	var names []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") != tests {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		names = append(names, rel)
		return nil
	})
	if err != nil {
		t.Fatalf("walk the repository: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("found no Go files")
	}
	return names
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

// TestReleaseWorkflowGatesPublishOnTests keeps a red gate of scripts/check.sh from
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
	for _, want := range []string{"go-version-file: be/go.mod", "scripts/check.sh"} {
		if !strings.Contains(test, want) {
			t.Errorf("test job misses %q", want)
		}
	}
	check := repoFile(t, "scripts/check.sh")
	for _, want := range []string{"gofmt -l", "go vet ./...", "staticcheck@", "gocyclo@", "go test -race ./..."} {
		if !strings.Contains(check, want) {
			t.Errorf("scripts/check.sh misses the %q gate", want)
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

// TestGoModuleLivesInBe keeps the Go module in be/ beside the frontend in
// fe/: the root holds only the shared files, never a second go.mod.
func TestGoModuleLivesInBe(t *testing.T) {
	root := repoRoot(t)
	if _, err := os.Stat(filepath.Join(root, "be", "go.mod")); err != nil {
		t.Errorf("be/go.mod: %v", err)
	}
	for _, name := range []string{"go.mod", "cmd", "internal"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			t.Errorf("the repository root still holds %s", name)
		}
	}
}

// TestFixturesLookLikePlaceholders keeps fake tokens out of a secret scanner's
// Google access-token pattern in every test file. The needle is built at run
// time so this file does not match itself.
func TestFixturesLookLikePlaceholders(t *testing.T) {
	needle := "ya" + "29."
	for _, name := range repoGoFiles(t, true) {
		if strings.Contains(repoFile(t, name), needle) {
			t.Errorf("%s still holds a %s fixture", name, needle)
		}
	}
}
