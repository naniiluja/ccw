package httpapi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// maxFileLines is the size limit for a non-test Go file. Tests are exempt:
// table-driven suites grow with their cases and split poorly.
const maxFileLines = 1000

const modulePath = "github.com/naniiluja/ccw"

// listedPackage is the subset of `go list -json` the architecture tests read.
type listedPackage struct {
	ImportPath string
	Name       string
	Dir        string
	GoFiles    []string
	Imports    []string
	Error      *struct{ Err string }
	DepsErrors []*struct{ Err string }
}

// moduleRoot returns the directory holding go.mod, so the tests do not
// depend on the working directory `go test` picks.
func moduleRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == os.DevNull {
		t.Fatal("go env GOMOD: not inside a module")
	}
	return filepath.Dir(gomod)
}

// listPackages runs `go list -e -json ./...` at the module root. The -e flag
// keeps an import cycle as a per-package error instead of aborting the list.
func listPackages(t *testing.T) []listedPackage {
	t.Helper()
	cmd := exec.Command("go", "list", "-e", "-json", "./...")
	cmd.Dir = moduleRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	var pkgs []listedPackage
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p listedPackage
		if err := dec.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		pkgs = append(pkgs, p)
	}
	if len(pkgs) == 0 {
		t.Fatal("go list returned no packages")
	}
	return pkgs
}

// internalName returns the path of p below internal/, or "" when p is not an
// internal package of this module.
func internalName(importPath string) string {
	return strings.TrimPrefix(importPath, modulePath+"/internal/")
}

func TestOnlyCmdImportsHTTPAPI(t *testing.T) {
	httpapiPath := modulePath + "/internal/httpapi"
	for _, p := range listPackages(t) {
		if p.ImportPath == httpapiPath || !strings.HasPrefix(p.ImportPath, modulePath+"/internal/") {
			continue
		}
		for _, imp := range p.Imports {
			if imp == httpapiPath {
				t.Errorf("%s imports internal/httpapi; only cmd/ccw may depend on the HTTP layer", p.ImportPath)
			}
		}
	}
}

func TestStoreDoesNotImportHigherLayers(t *testing.T) {
	forbidden := map[string]bool{"httpapi": true, "translate": true, "drift": true}
	for _, p := range listPackages(t) {
		if p.ImportPath != modulePath+"/internal/store" {
			continue
		}
		for _, imp := range p.Imports {
			if name := internalName(imp); name != imp && forbidden[name] {
				t.Errorf("internal/store imports internal/%s; store sits below it", name)
			}
		}
		return
	}
	t.Fatal("internal/store not found by go list")
}

func TestPackagesHaveNoImportCycles(t *testing.T) {
	for _, p := range listPackages(t) {
		if p.Error != nil {
			t.Errorf("%s: %s", p.ImportPath, p.Error.Err)
		}
		for _, e := range p.DepsErrors {
			t.Errorf("%s: %s", p.ImportPath, e.Err)
		}
	}
}

// TestEachPackageHasExactlyOnePackageComment keeps godoc single-sourced: a
// second "// Package x" comment in another file would be concatenated.
// Commands use the "// Command x" form instead.
func TestEachPackageHasExactlyOnePackageComment(t *testing.T) {
	for _, p := range listPackages(t) {
		prefix := "Package " + p.Name + " "
		if p.Name == "main" {
			prefix = "Command "
		}
		var with []string
		for _, f := range p.GoFiles {
			path := filepath.Join(p.Dir, f)
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.PackageClauseOnly|parser.ParseComments)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			if file.Doc != nil && strings.HasPrefix(file.Doc.Text(), prefix) {
				with = append(with, f)
			}
		}
		if len(with) != 1 {
			t.Errorf("%s: want exactly one %q comment, found it in %v", p.ImportPath, strings.TrimSpace(prefix), with)
		}
	}
}

func TestNonTestGoFilesStayUnderTheSizeLimit(t *testing.T) {
	root := moduleRoot(t)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			// Skip what the go tool skips: dot and underscore directories
			// (including .claude/worktrees copies of the repo) and testdata.
			if path != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		n, err := countLines(path)
		if err != nil {
			return err
		}
		if n > maxFileLines {
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s has %d lines, over the %d line limit; split it by topic", rel, n, maxFileLines)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}

func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		n++
	}
	return n, sc.Err()
}
