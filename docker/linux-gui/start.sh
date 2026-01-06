#!/bin/bash
# Script to start Linux GUI container with X11 forwarding on macOS
#
# Prerequisites:
#   1. Install XQuartz: brew install --cask xquartz
#   2. Open XQuartz preferences (Cmd+,) -> Security tab
#   3. Check "Allow connections from network clients"
#   4. Restart XQuartz after changing the setting

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# XQuartz paths
XHOST="/opt/X11/bin/xhost"

echo -e "${GREEN}=== Linux GUI Test Environment ===${NC}"

# Check XQuartz
if [ ! -d "/Applications/Utilities/XQuartz.app" ]; then
    echo -e "${RED}Error: XQuartz is not installed${NC}"
    echo "Install with: brew install --cask xquartz"
    exit 1
fi

# Check if XQuartz is running
if ! pgrep -x "XQuartz" > /dev/null && ! pgrep -x "X11.bin" > /dev/null; then
    echo -e "${YELLOW}Starting XQuartz...${NC}"
    open -a XQuartz
    sleep 3
fi

# Set DISPLAY if not set
if [ -z "$DISPLAY" ]; then
    export DISPLAY=:0
fi

# Allow connections from localhost
echo -e "${YELLOW}Allowing X11 connections (DISPLAY=$DISPLAY)...${NC}"
"$XHOST" +localhost 2>/dev/null || "$XHOST" + 2>/dev/null || true

# Export HOST_DISPLAY for docker compose
export HOST_DISPLAY="host.docker.internal:0"

echo -e "${GREEN}HOST_DISPLAY=$HOST_DISPLAY${NC}"
echo -e "${GREEN}X11 forwarding ready!${NC}"
