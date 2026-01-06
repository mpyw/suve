#!/usr/bin/env bash
# GUI Demo Recording Script for suve
#
# This script sets up demo data and records the GUI demo using Playwright.
# Follows the same scenario as the CLI demo (demo/cli-demo.tape).
#
# Usage: ./demo/gui-record.sh
#
# Output: demo/gui-demo.gif

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
FRONTEND_DIR="$PROJECT_DIR/internal/gui/frontend"
GUI_DIR="$PROJECT_DIR/gui"

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
./bin/suve param create /demo/api/url "https://api-v1.example.com"
./bin/suve param tag /demo/api/url Version=v1

./bin/suve param create /demo/legacy/endpoint "https://old.example.com"

./bin/suve param create /demo/config '{"timeout":30}'

echo "Demo data ready:"
./bin/suve param ls -R /demo

echo ""
echo "=== Starting wails dev ==="

# Clean up previous test artifacts
rm -rf "$FRONTEND_DIR/test-results-recording" 2>/dev/null || true

# Start wails dev in background (same as make gui-dev)
cd "$GUI_DIR"
wails dev -skipbindings -tags dev &
WAILS_PID=$!

# Wails dev server port (exposes Go methods to browser)
WAILS_PORT="${WAILS_PORT:-34115}"

# Wait for wails dev to be fully ready
echo "Waiting for wails dev to start..."
echo "Checking http://localhost:$WAILS_PORT ..."
MAX_RETRIES=60
RETRY_COUNT=0
while ! curl -s "http://localhost:$WAILS_PORT" > /dev/null 2>&1; do
    RETRY_COUNT=$((RETRY_COUNT + 1))
    if [ $RETRY_COUNT -ge $MAX_RETRIES ]; then
        echo "Error: wails dev did not start within ${MAX_RETRIES} seconds"
        kill $WAILS_PID 2>/dev/null || true
        exit 1
    fi
    sleep 1
done
echo "wails dev is ready!"
# Additional wait for app to be fully initialized
sleep 3

echo ""
echo "=== Recording GUI demo ==="

cd "$FRONTEND_DIR"

# Check if node_modules exists
if [[ ! -d node_modules ]]; then
    echo "Installing dependencies..."
    npm install
fi

# Run Playwright with recording config
# - NODE_PATH: for module resolution from demo/ directory
# - VITE_PORT: set to wails dev server port (not Vite's 5173)
NODE_PATH="$FRONTEND_DIR/node_modules" VITE_PORT="$WAILS_PORT" npx playwright test gui-demo.spec.ts \
    -c playwright.recording.config.ts \
    --headed \
    --project=chromium \
    --reporter=list

# Stop wails dev
echo "Stopping wails dev..."
kill $WAILS_PID 2>/dev/null || true

# Find the recorded video
VIDEO_FILE=$(find ./test-results-recording -name "*.webm" -type f 2>/dev/null | head -1)

if [[ -n "$VIDEO_FILE" ]]; then
    # Move to demo directory with proper name
    cp "$VIDEO_FILE" "$SCRIPT_DIR/gui-demo.webm"
    echo ""
    echo "=== Done ==="
    echo "Video saved to: demo/gui-demo.webm"

    # Convert to GIF (requires ffmpeg)
    if command -v ffmpeg &> /dev/null; then
        echo ""
        echo "Converting to GIF..."
        # High quality GIF conversion:
        # - ss 1.5: skip first 1.5 seconds to remove white screen
        # - fps=15: smoother animation
        # - scale=1280: match video resolution
        # - lanczos: high quality scaling
        # - palettegen with max_colors=256 and stats_mode=diff for better color accuracy
        ffmpeg -y -ss 1.5 -i "$SCRIPT_DIR/gui-demo.webm" \
            -vf "fps=15,scale=1280:-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=256:stats_mode=diff[p];[s1][p]paletteuse=dither=floyd_steinberg" \
            "$SCRIPT_DIR/gui-demo.gif" 2>/dev/null
        echo "GIF saved to: demo/gui-demo.gif"
        # Delete webm after successful conversion
        rm -f "$SCRIPT_DIR/gui-demo.webm"
        echo "Removed webm file"
    else
        echo ""
        echo "Warning: ffmpeg not found. Keeping webm file."
        echo "Install ffmpeg to auto-convert to GIF:"
        echo "  brew install ffmpeg"
    fi

    # Cleanup test-results
    rm -rf ./test-results-recording
else
    echo "Error: No video file found"
    # Stop wails dev on error
    kill $WAILS_PID 2>/dev/null || true
    exit 1
fi
