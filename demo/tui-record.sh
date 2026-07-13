#!/usr/bin/env bash
# TUI Demo Recording Script for suve
#
# This script sets up demo data and records the TUI demo. It mirrors
# cli-record.sh: the TUI is pure Go and records over the same vhs terminal path
# as the CLI demo (no browser).
# Requires: vhs (https://github.com/charmbracelet/vhs)
#
# Usage: ./demo/tui-record.sh
#   Set SUVE_LOCALSTACK_EXTERNAL_PORT to a free port to avoid colliding with an
#   already-running LocalStack (e.g. another checkout's), e.g.:
#   SUVE_LOCALSTACK_EXTERNAL_PORT=4599 ./demo/tui-record.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

# LocalStack configuration. SUVE_LOCALSTACK_EXTERNAL_PORT is exported so docker
# compose (which interpolates it in compose.yaml) and the endpoint URL agree.
export SUVE_LOCALSTACK_EXTERNAL_PORT="${SUVE_LOCALSTACK_EXTERNAL_PORT:-4566}"
export AWS_ENDPOINT_URL="http://localhost:${SUVE_LOCALSTACK_EXTERNAL_PORT}"
export AWS_ACCESS_KEY_ID="test"
export AWS_SECRET_ACCESS_KEY="test"
export AWS_DEFAULT_REGION="us-east-1"
# Pin a deterministic staging data key so the staging steps never block on the
# macOS keychain (matches `mise test` and scripts/seed.sh). The TUI staging store
# reads it from the inherited environment.
export SUVE_STAGING_KEY="${SUVE_STAGING_KEY:-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=}"

# This demo owns the /demo/* namespace, kept deliberately separate from the
# `mise run seed-*` fixtures (/suve-demo/*), so recording and manual seeding
# never clobber each other.

echo "=== Setting up demo environment ==="

# Guard against colliding with a LocalStack this script did not start (e.g. a
# sibling checkout's). If the chosen port is already bound, refuse rather than
# tearing down containers we do not own — pick a free SUVE_LOCALSTACK_EXTERNAL_PORT.
if [ "$SUVE_LOCALSTACK_EXTERNAL_PORT" = "4566" ] && lsof -nP -iTCP:4566 -sTCP:LISTEN >/dev/null 2>&1; then
  echo "ERROR: port 4566 is already in use (another LocalStack may be running)." >&2
  echo "Re-run with a free port, e.g.: SUVE_LOCALSTACK_EXTERNAL_PORT=4599 ./demo/tui-record.sh" >&2
  exit 1
fi

# Always build the full GUI binary (bin/suve) so it matches the current bindings
# and API arity — a stale or CLI-only binary can drift out of sync.
echo "Building suve (GUI)..."
mise build-gui

# Reset LocalStack (clean slate). --profile "*" so the profile-gated localstack
# service is actually torn down (a bare `down` leaves profiled services running).
echo "Resetting LocalStack..."
docker compose --profile "*" down -v
docker compose --profile aws up -d
echo "Waiting for LocalStack to be ready..."
sleep 3

# Clear staging
echo "Clearing staging..."
./bin/suve stage reset --all 2>/dev/null || true

# Set up 3 existing parameters (identical to cli-record.sh):
# - /demo/api/url        -> will be edited (v1 -> v2)
# - /demo/legacy/endpoint -> will be deleted
# - /demo/config          -> untouched
echo "Creating demo parameters..."
./bin/suve param create /demo/api/url "https://api-v1.example.com"
./bin/suve param tag /demo/api/url Version=v1

./bin/suve param create /demo/legacy/endpoint "https://old.example.com"

./bin/suve param create /demo/config '{"timeout":30}'

echo "Demo data ready:"
./bin/suve param ls -R /demo

echo ""
echo "=== Recording TUI demo ==="
# Create temp tape with correct endpoint URL (rewrites the default 4566 port).
TEMP_TAPE=$(mktemp)
sed "s|http://localhost:4566|$AWS_ENDPOINT_URL|g" demo/tui-demo.tape > "$TEMP_TAPE"
PATH="$PROJECT_DIR/bin:$PATH" vhs "$TEMP_TAPE"
rm -f "$TEMP_TAPE"

echo ""
echo "=== Done ==="
ls -la demo/tui-demo.gif
