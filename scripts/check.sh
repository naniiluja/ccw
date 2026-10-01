#!/usr/bin/env bash
# Runs every gate the coding rules are enforced by, locally and in CI:
# gofmt, go vet, staticcheck (staticcheck.conf), gocyclo, and go test -race,
# which also runs the architecture and file size tests in
# internal/httpapi/arch_test.go. Every gate runs; the script names each one
# that failed and exits non-zero if any did.
#
# The lint tools run through `go run module@version`, pinned below, so CI and
# every machine use the same versions without adding them to go.mod.
set -uo pipefail

STATICCHECK_VERSION=v0.8.1 # staticcheck 2026.2.1
GOCYCLO_VERSION=v0.6.0
# staticcheck reads the export data of the toolchain that runs it, and v0.8.x
# needs Go 1.26 or newer while go.mod targets 1.25, so it runs on a pinned
# toolchain that the go command downloads when the local one differs.
STATICCHECK_GOTOOLCHAIN=go1.27.1
MAX_CYCLOMATIC=30

cd "$(dirname "$0")/.."

failed=()

gate() {
	local name=$1
	shift
	echo "==> $name"
	if "$@"; then
		echo "    ok: $name"
	else
		echo "    FAIL: $name"
		failed+=("$name")
	fi
}

# goFiles lists the module's Go files, skipping dot directories such as
# .git and .claude/worktrees, which hold whole copies of the repository.
goFiles() {
	find . -path './.*' -prune -o -name '*.go' -type f -print
}

gofmtClean() {
	local out
	out=$(goFiles | xargs gofmt -l) || return 1
	if [ -n "$out" ]; then
		echo "    files not formatted by gofmt:"
		echo "$out" | sed 's/^/      /'
		return 1
	fi
}

staticcheckClean() {
	GOTOOLCHAIN=$STATICCHECK_GOTOOLCHAIN go run "honnef.co/go/tools/cmd/staticcheck@$STATICCHECK_VERSION" ./...
}

gocycloClean() {
	local out status
	# gocyclo exits 1 when it reports a function, so the output decides; a
	# non-zero status with no output means the tool itself failed.
	out=$(goFiles | grep -v '_test\.go$' | xargs go run "github.com/fzipp/gocyclo/cmd/gocyclo@$GOCYCLO_VERSION" -over "$MAX_CYCLOMATIC")
	status=$?
	if [ -n "$out" ]; then
		echo "    functions over cyclomatic $MAX_CYCLOMATIC:"
		echo "$out" | sed 's/^/      /'
		return 1
	fi
	return $status
}

gate "gofmt" gofmtClean
gate "go vet" go vet ./...
gate "staticcheck $STATICCHECK_VERSION" staticcheckClean
gate "gocyclo -over $MAX_CYCLOMATIC" gocycloClean
gate "go test -race (incl. architecture and size tests)" go test -race ./...

if [ ${#failed[@]} -gt 0 ]; then
	echo
	echo "FAILED gates:"
	printf '  - %s\n' "${failed[@]}"
	exit 1
fi
echo
echo "All gates passed."
