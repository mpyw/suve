#!/bin/bash
# Script to start Linux GUI container with X11 forwarding on macOS

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Linux GUI Test Environment ===${NC}"

# Check XQuartz
if ! command -v xquartz &> /dev/null && [ ! -d "/Applications/Utilities/XQuartz.app" ]; then
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

# Allow connections from localhost
echo -e "${YELLOW}Allowing X11 connections from localhost...${NC}"
xhost +localhost 2>/dev/null || xhost + 2>/dev/null || true

# Get host IP for Docker
HOST_IP=$(ifconfig en0 | grep 'inet ' | awk '{print $2}')
if [ -z "$HOST_IP" ]; then
    HOST_IP="host.docker.internal"
fi

echo -e "${GREEN}Host IP: $HOST_IP${NC}"
echo -e "${GREEN}DISPLAY will be set to: $HOST_IP:0${NC}"

# Export for docker compose
export HOST_DISPLAY="$HOST_IP:0"

echo ""
echo -e "${GREEN}Environment ready! You can now run:${NC}"
echo "  make linux-gui        # Start interactive shell"
echo "  make linux-gui-build  # Build GUI in Linux"
echo "  make linux-gui-test   # Run GUI tests"
