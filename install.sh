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

echo "Downloading latest version of gomas"
wget -q --show-progress -O "/tmp/gomas-$ARCH.tgz" "https://github.com/gregoryjjb/gomas/releases/latest/download/gomas-$ARCH.tgz"
echo

echo "Removing old version"
sudo rm -rf /usr/local/bin/gomas

echo "Installing"
sudo tar -C /usr/local/bin -xzf "/tmp/gomas-$ARCH.tgz"
echo

# Verify installation
echo "Gomas has been installed:"
gomas --version
echo

SERVICE="gomas.service"

if systemctl list-unit-files | grep -q "^$SERVICE"; then
    read -p "Restart gomas service? (y/N): " RESTART
    if [[ "$RESTART" =~ ^[Yy]$ ]]; then
        sudo systemctl restart "$SERVICE"
        echo "Service restarted."
    else
        echo "Service not restarted."
    fi

else
    echo "No systemd service for gomas exists."

    read -p "Would you like to create one? (y/N): " CREATE
    if [[ "$CREATE" =~ ^[Yy]$ ]]; then
        # TODO: create service file
                exit

        sudo systemctl daemon-reload
        sudo systemctl enable gomas.service
        sudo systemctl start gomas.service

        echo "Service created and started."
    else
        echo "Service not created."
    fi
fi
