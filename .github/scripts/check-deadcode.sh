#!/bin/sh
#
# Fail the build if `deadcode ./...` reports any unreachable (dead) function
# that is not explicitly waived in .github/deadcode-allow.txt.
#
# deadcode (golang.org/x/tools/cmd/deadcode) uses Rapid Type Analysis to find
# functions unreachable from any main in the module. We run it WITHOUT -test on
# purpose: with -test, production logic that is reachable only from its own
# tests would be hidden (a test would keep otherwise-dead code "alive"). We want
# that surfaced. Genuine test-only helpers (mocks / test seams) are the sole
# exception and are waived, with a justification each, in the allowlist file —
# this is the "ignore comment" equivalent for this gate.
#
# The default build tags (linux, non-production) exclude internal/gui, so GUI
# code is not analyzed here; its dead code stays covered by the existing
# production golangci-lint step.
set -eu

cd "$(git rev-parse --show-toplevel)"

ALLOWLIST=".github/deadcode-allow.txt"

# Prefer a deadcode binary on PATH (mise installs it in CI); otherwise fall back
# to a pinned `go run` so the gate is runner-independent and works locally.
if command -v deadcode >/dev/null 2>&1; then
  deadcode_cmd="deadcode"
else
  deadcode_cmd="go run golang.org/x/tools/cmd/deadcode@v0.48.0"
fi

# Each finding is "path/file.go:LINE:COL: unreachable func: Name". Strip the
# volatile ":LINE:COL" so matching is on the stable
# "path/file.go: unreachable func: Name" form.
findings=$(
  $deadcode_cmd ./... \
    | sed 's/\(\.go\):[0-9][0-9]*:[0-9][0-9]*:/\1:/'
)

# The allowlist, minus blank lines and '#' comments.
allow=$(sed -e 's/#.*$//' -e 's/[[:space:]]*$//' "${ALLOWLIST}" | grep -v '^[[:space:]]*$' || true)

offenders=""
old_ifs=$IFS
IFS='
'
for finding in ${findings}; do
  [ -n "${finding}" ] || continue
  if printf '%s\n' "${allow}" | grep -Fxq "${finding}"; then
    continue
  fi
  offenders="${offenders}${finding}
"
done
IFS=$old_ifs

if [ -n "${offenders}" ]; then
  echo "Dead code found: unreachable func(s) not waived in ${ALLOWLIST}:" >&2
  printf '%s' "${offenders}" >&2
  echo "Delete the dead code, or — only if it is a genuine test-only helper —" >&2
  echo "add its 'path/file.go: unreachable func: Name' line to ${ALLOWLIST} with a justification." >&2
  exit 1
fi

echo "Dead code gate OK: no unreachable funcs outside ${ALLOWLIST}."
