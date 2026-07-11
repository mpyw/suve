#!/usr/bin/env bash
# CLI Demo Recording Script for suve
#
# This script sets up demo data and records the CLI demo
# Requires: vhs (https://github.com/charmbracelet/vhs)
#
# Usage: ./demo/cli-record.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

# LocalStack configuration
export AWS_ENDPOINT_URL="http://localhost:${SUVE_LOCALSTACK_EXTERNAL_PORT:-4566}"
export AWS_ACCESS_KEY_ID="test"
export AWS_SECRET_ACCESS_KEY="test"
export AWS_DEFAULT_REGION="us-east-1"
# Pin a deterministic staging data key so the staging steps never block on the
# macOS keychain (matches `mise test` and scripts/seed.sh).
export SUVE_STAGING_KEY="${SUVE_STAGING_KEY:-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=}"

# This demo owns the /demo/* namespace, kept deliberately separate from the
# `mise run seed-*` fixtures (/suve-demo/*), so recording and manual seeding
# never clobber each other.

echo "=== Setting up demo environment ==="

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

# Set up 3 existing parameters:
# - /demo/api/url        -> will be edited
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
echo "=== Recording CLI demo ==="
# Create temp tape with correct endpoint URL
TEMP_TAPE=$(mktemp)
sed "s|http://localhost:4566|$AWS_ENDPOINT_URL|g" demo/cli-demo.tape > "$TEMP_TAPE"
PATH="$PROJECT_DIR/bin:$PATH" vhs "$TEMP_TAPE"
rm -f "$TEMP_TAPE"

echo ""
echo "=== Done ==="
ls -la demo/cli-demo.gif
