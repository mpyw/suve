# Linux desktop environment for GUI testing (Ubuntu 24.04, GTK4 + webkitgtk-6.0:
# the default Linux build)
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
    # Wails GUI dependencies (GTK4 + webkitgtk-6.0)
    libgtk-4-1 \
    libwebkitgtk-6.0-4 \
    libayatana-appindicator3-1 \
    # Build tools
    build-essential \
    pkg-config \
    libgtk-4-dev \
    libwebkitgtk-6.0-dev \
    # Utilities
    curl \
    git \
    ca-certificates \
    dbus-x11 \
    x11-xserver-utils \
    # Clean up
    && rm -rf /var/lib/apt/lists/*

# Install Go
ARG TARGETARCH
RUN curl -fsSL "https://go.dev/dl/go1.27.3.linux-${TARGETARCH}.tar.gz" | tar -C /usr/local -xzf -
ENV PATH="/usr/local/go/bin:/root/go/bin:${PATH}"

# Create startup script
RUN echo '#!/bin/bash\n\
openbox &\n\
tint2 &\n\
lxterminal\n\
' > /usr/local/bin/start-desktop && chmod +x /usr/local/bin/start-desktop

# Set working directory
WORKDIR /app

# Start desktop
CMD ["/usr/local/bin/start-desktop"]
