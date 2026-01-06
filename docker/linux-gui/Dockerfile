FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=Asia/Tokyo

# Install dependencies for Wails GUI app
RUN apt-get update && apt-get install -y \
    # X11 and GUI dependencies
    libgtk-3-0 \
    libwebkit2gtk-4.1-0 \
    libayatana-appindicator3-1 \
    # X11 utilities
    x11-apps \
    xauth \
    dbus-x11 \
    # Build tools (for development)
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
    # Clean up
    && rm -rf /var/lib/apt/lists/*

# Install Wails
RUN go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Set working directory
WORKDIR /app

# Add Go bin to PATH
ENV PATH="/root/go/bin:${PATH}"

# Default command
CMD ["/bin/bash"]
