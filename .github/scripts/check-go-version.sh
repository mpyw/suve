#!/usr/bin/env bash
#
# Fail if the Go version pins drift apart. The `go` directive in go.mod is the
# source of truth; these must match it exactly:
#   - gui/go.mod        `go` directive
#   - mise.toml         `go = "..."` tool pin (local + CI mise jobs)
#   - compose.test.yaml `image: golang:<version>` of the e2e test-runner
# A test-runner image older than go.mod cannot build the module, so every
# `mise e2e-*` task fails locally while CI (which uses setup-go) still passes.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

want=$(awk '$1 == "go" { print $2; exit }' go.mod)
if [ -z "$want" ]; then
  echo "check-go-version: no go directive in go.mod" >&2
  exit 1
fi

status=0
check() {
  local where=$1 got=$2
  if [ "$got" != "$want" ]; then
    echo "check-go-version: $where has Go '${got:-<missing>}', go.mod has '$want'" >&2
    status=1
  fi
}

check "gui/go.mod" "$(awk '$1 == "go" { print $2; exit }' gui/go.mod)"
check "mise.toml" "$(sed -n 's/^go = "\([^"]*\)".*/\1/p' mise.toml | head -n1)"
check "compose.test.yaml (test-runner image)" "$(sed -n 's/^ *image: golang:\([^ ]*\).*/\1/p' compose.test.yaml | head -n1)"

if [ "$status" -ne 0 ]; then
  echo "Fix: make every pin above match the go directive in go.mod." >&2
  exit 1
fi
echo "Go version pins match go.mod ($want)."
