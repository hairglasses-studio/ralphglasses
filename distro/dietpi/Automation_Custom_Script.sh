#!/bin/bash
# DietPi post-install automation script for ralphglasses thin client.
# This runs automatically after first boot when placed in /boot/Automation_Custom_Script.sh
set -euo pipefail

RALPHGLASSES_REPO="https://github.com/hairglasses-studio/ralphglasses.git"
INSTALL_DIR="/opt/ralphglasses"
BIN_DIR="/usr/local/bin"

echo "=== Ralphglasses Thin Client Setup ==="

# --- Install system packages ---
apt-get update -qq
apt-get install -y -qq autorandr xdotool jq tmux

# --- Clone and build ralphglasses ---
if [[ ! -d "$INSTALL_DIR" ]]; then
    git clone "$RALPHGLASSES_REPO" "$INSTALL_DIR"
fi
cd "$INSTALL_DIR"
go build -o "$BIN_DIR/ralphglasses" ./...

# --- Install i3 config ---
mkdir -p /home/dietpi/.config/i3
cp "$INSTALL_DIR/distro/i3/config" /home/dietpi/.config/i3/config
chown -R dietpi:dietpi /home/dietpi/.config/i3

# --- Install autorandr profiles ---
if [[ -d "$INSTALL_DIR/distro/autorandr" ]]; then
    mkdir -p /home/dietpi/.config/autorandr
    cp -r "$INSTALL_DIR/distro/autorandr/"* /home/dietpi/.config/autorandr/ 2>/dev/null || true
    chown -R dietpi:dietpi /home/dietpi/.config/autorandr
fi

# --- Install systemd units ---
cp "$INSTALL_DIR/distro/systemd/ralphglasses.service" /etc/systemd/system/
systemctl daemon-reload
systemctl enable ralphglasses.service

# --- Configure autologin ---
mkdir -p /etc/systemd/system/getty@tty1.service.d
cat > /etc/systemd/system/getty@tty1.service.d/autologin.conf <<EOF
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin dietpi --noclear %I \$TERM
EOF

echo "=== Ralphglasses thin client setup complete ==="
echo "Reboot to start i3 + ralphglasses TUI."
