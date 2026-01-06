# Linux desktop environment for GUI testing
# Runs Openbox window manager via X11 forwarding to macOS XQuartz
#
# Prerequisites (macOS):
#   1. brew install --cask xquartz
#   2. XQuartz preferences -> Security -> "Allow connections from network clients"
#   3. Restart XQuartz after changing the setting
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=Asia/Tokyo

# Install Openbox and dependencies
RUN apt-get update && apt-get install -y \
    # Window manager and desktop components
    openbox \
    tint2 \
    pcmanfm \
    lxterminal \
    feh \
    # Wails GUI dependencies
    libgtk-3-0 \
    libwebkit2gtk-4.1-0 \
    libayatana-appindicator3-1 \
    # Build tools
    build-essential \
    pkg-config \
    libgtk-3-dev \
    libwebkit2gtk-4.1-dev \
    # Go
    golang-go \
    # Utilities
    curl \
    git \
    ca-certificates \
    dbus-x11 \
    x11-xserver-utils \
    # Clean up
    && rm -rf /var/lib/apt/lists/*

# Install Wails
RUN go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Create startup script
RUN echo '#!/bin/bash\n\
openbox &\n\
tint2 &\n\
lxterminal\n\
' > /usr/local/bin/start-desktop && chmod +x /usr/local/bin/start-desktop

# Set working directory
WORKDIR /app

# Add Go bin to PATH
ENV PATH="/root/go/bin:${PATH}"

# Start desktop
CMD ["/usr/local/bin/start-desktop"]
