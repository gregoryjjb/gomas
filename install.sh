#!/usr/bin/env bash
set -e

ARCH_RAW="$(uname -m)"
ARCH=""

if [ "$ARCH_RAW" = "aarch64" ]; then
    ARCH="arm64"
elif [ "$ARCH_RAW" = "armv7l" ]; then
    ARCH="arm32"
else
    echo "Error: Unsupported architecture '$ARCH_RAW'." >&2
    exit 1
fi

echo "Detected architecture: $ARCH"

# Download the appropriate archive to /tmp
wget -O "/tmp/gomas-$ARCH.tgz" "https://github.com/gregoryjjb/gomas/releases/download/latest/gomas-$ARCH.tgz"

# Remove old binary and extract new one
sudo rm -rf /usr/local/bin/gomas
sudo tar -C /usr/local/bin -xzf "/tmp/gomas-$ARCH.tgz"

# Verify installation
echo "Gomas has been installed:"
gomas --version
echo
