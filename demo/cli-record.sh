#!/bin/bash
# CLI Demo Recording Script for suve
#
# This script sets up demo data and records the CLI demo
# Requires: vhs (https://github.com/charmbracelet/vhs)
#
# Usage: ./demo/cli-record.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

# LocalStack configuration
export AWS_ENDPOINT_URL="http://localhost:${SUVE_LOCALSTACK_EXTERNAL_PORT:-4566}"
export AWS_ACCESS_KEY_ID="test"
export AWS_SECRET_ACCESS_KEY="test"
export AWS_DEFAULT_REGION="us-east-1"

echo "=== Setting up demo environment ==="

# Build suve if needed
if [[ ! -f bin/suve ]]; then
    echo "Building suve..."
    make build
fi

# Reset LocalStack (clean slate)
echo "Resetting LocalStack..."
docker compose down -v
docker compose up -d
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
aws ssm put-parameter --endpoint-url=http://localhost:4566 \
    --name "/demo/api/url" \
    --value "https://api-v1.example.com" \
    --type String --overwrite >/dev/null
aws ssm add-tags-to-resource --endpoint-url=http://localhost:4566 \
    --resource-type "Parameter" --resource-id "/demo/api/url" \
    --tags "Key=Version,Value=v1" >/dev/null

aws ssm put-parameter --endpoint-url=http://localhost:4566 \
    --name "/demo/legacy/endpoint" \
    --value "https://old.example.com" \
    --type String --overwrite >/dev/null

aws ssm put-parameter --endpoint-url=http://localhost:4566 \
    --name "/demo/config" \
    --value '{"timeout":30}' \
    --type String --overwrite >/dev/null

echo "Demo data ready:"
./bin/suve param ls -R /demo

echo ""
echo "=== Recording CLI demo ==="
PATH="$PROJECT_DIR/bin:$PATH" vhs demo/cli-demo.tape

echo ""
echo "=== Done ==="
ls -la demo/cli-demo.gif
