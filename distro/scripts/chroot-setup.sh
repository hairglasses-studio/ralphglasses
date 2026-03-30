#!/bin/bash
# chroot-setup.sh — Configure debootstrap rootfs for ralphglasses thin client
#
# This script runs inside a chroot created by debootstrap.
# It mirrors the Dockerfile setup: installs packages, creates the ralph user,
# configures i3, auto-login, and ralphglasses as the default application.
#
# Called by: make chroot (distro/Makefile)

set -euo pipefail

export DEBIAN_FRONTEND=noninteractive

GO_VERSION="${GO_VERSION:-1.24.1}"
NODE_MAJOR="${NODE_MAJOR:-20}"
NVIDIA_DRIVER_VERSION="${NVIDIA_DRIVER_VERSION:-550}"

echo "==> [chroot] Starting ralphglasses thin client setup..."

# ── Enable universe/multiverse repos ────────────────────────────────
cat > /etc/apt/sources.list <<'EOF'
deb http://archive.ubuntu.com/ubuntu noble main restricted universe multiverse
deb http://archive.ubuntu.com/ubuntu noble-updates main restricted universe multiverse
deb http://archive.ubuntu.com/ubuntu noble-security main restricted universe multiverse
EOF

# ── Install packages ────────────────────────────────────────────────
echo "==> [chroot] Installing system packages..."
apt-get update
apt-get install -y --no-install-recommends \
    linux-image-generic \
    systemd \
    systemd-sysv \
    dbus \
    grub-efi-amd64 \
    grub-efi-amd64-signed \
    shim-signed \
    efibootmgr \
    xorg \
    xinit \
    i3 \
    i3status \
    i3lock \
    alacritty \
    autorandr \
    arandr \
    "nvidia-driver-${NVIDIA_DRIVER_VERSION}" \
    "nvidia-utils-${NVIDIA_DRIVER_VERSION}" \
    "libnvidia-gl-${NVIDIA_DRIVER_VERSION}" \
    nvidia-settings \
    git \
    jq \
    tmux \
    curl \
    wget \
    htop \
    sudo \
    ca-certificates \
    gnupg \
    xdotool \
    network-manager \
    iproute2 \
    openssh-server \
    ethtool \
    linux-firmware \
    linux-modules-extra-generic \
    bolt \
    alsa-utils \
    bluez \
    bluez-tools \
    fwupd \
    squashfs-tools \
    dosfstools \
    parted \
    live-boot \
    live-boot-initramfs-tools \
    locales \
    console-setup \
    kbd

# ── Locale ──────────────────────────────────────────────────────────
locale-gen en_US.UTF-8
update-locale LANG=en_US.UTF-8

# ── Go runtime ──────────────────────────────────────────────────────
echo "==> [chroot] Installing Go ${GO_VERSION}..."
curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
    | tar -C /usr/local -xzf -
ln -sf /usr/local/go/bin/go /usr/local/bin/go
ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt

# ── Node.js (for Claude Code CLI) ──────────────────────────────────
echo "==> [chroot] Installing Node.js ${NODE_MAJOR}..."
curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" | bash -
apt-get install -y --no-install-recommends nodejs

# ── Claude Code CLI ────────────────────────────────────────────────
echo "==> [chroot] Installing Claude Code CLI..."
npm install -g @anthropic-ai/claude-code

# ── Create ralph user ──────────────────────────────────────────────
echo "==> [chroot] Creating user 'ralph'..."
if ! id ralph &>/dev/null; then
    useradd -m -s /bin/bash -G sudo,video,render,input,bluetooth,plugdev ralph
fi
echo "ralph ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/ralph
chmod 440 /etc/sudoers.d/ralph

# ── i3 config ──────────────────────────────────────────────────────
echo "==> [chroot] Installing i3 config..."
mkdir -p /home/ralph/.config/i3
if [ -d /tmp/distro-configs/i3 ]; then
    cp /tmp/distro-configs/i3/config /home/ralph/.config/i3/config
    [ -f /tmp/distro-configs/i3/i3blocks.conf ] && \
        cp /tmp/distro-configs/i3/i3blocks.conf /home/ralph/.config/i3/i3blocks.conf
fi
chown -R ralph:ralph /home/ralph/.config

# ── systemd services ──────────────────────────────────────────────
echo "==> [chroot] Installing systemd services..."
if [ -d /tmp/distro-configs/systemd ]; then
    cp /tmp/distro-configs/systemd/*.service /etc/systemd/system/ 2>/dev/null || true
    cp /tmp/distro-configs/systemd/*.timer /etc/systemd/system/ 2>/dev/null || true
    systemctl enable ralphglasses.service 2>/dev/null || true
    systemctl enable hw-detect.service 2>/dev/null || true
    systemctl enable rg-status-bar.timer 2>/dev/null || true
fi

# ── Autologin to i3 via getty on tty1 ──────────────────────────────
echo "==> [chroot] Configuring auto-login..."
mkdir -p /etc/systemd/system/getty@tty1.service.d
cat > /etc/systemd/system/getty@tty1.service.d/autologin.conf <<'EOF'
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin ralph --noclear %I $TERM
EOF

# ── .xinitrc: startx launches i3 ──────────────────────────────────
cat > /home/ralph/.xinitrc <<'EOF'
#!/bin/sh
exec i3
EOF
chmod 755 /home/ralph/.xinitrc
chown ralph:ralph /home/ralph/.xinitrc

# ── .bash_profile: auto startx on tty1 ────────────────────────────
cat >> /home/ralph/.bash_profile <<'PROFILE'
if [ -z "$DISPLAY" ] && [ "$(tty)" = "/dev/tty1" ]; then
  exec startx
fi
PROFILE
chown ralph:ralph /home/ralph/.bash_profile

# ── NVIDIA: dual GPU Xorg config ──────────────────────────────────
echo "==> [chroot] Configuring NVIDIA dual GPU..."
mkdir -p /etc/X11/xorg.conf.d
cat > /etc/X11/xorg.conf.d/20-nvidia.conf <<'XEOF'
Section "Device"
    Identifier "GPU0"
    Driver     "nvidia"
    BusID      "PCI:1:0:0"
EndSection

Section "Device"
    Identifier "GPU1"
    Driver     "nvidia"
    BusID      "PCI:2:0:0"
EndSection

Section "ServerLayout"
    Identifier "layout0"
    Screen 0   "Screen0"
    Screen 1   "Screen1" RightOf "Screen0"
EndSection

Section "Screen"
    Identifier "Screen0"
    Device     "GPU0"
    Option     "AllowEmptyInitialConfiguration" "True"
    Option     "ConnectedMonitor" "DP-1,DP-2,DP-3,HDMI-0"
EndSection

Section "Screen"
    Identifier "Screen1"
    Device     "GPU1"
    Option     "AllowEmptyInitialConfiguration" "True"
    Option     "ConnectedMonitor" "HDMI-1,DP-4,DP-5"
EndSection
XEOF

# ── Hostname ────────────────────────────────────────────────────────
echo "ralphglasses" > /etc/hostname

# ── Kernel module blacklist ─────────────────────────────────────────
mkdir -p /etc/modprobe.d
cat > /etc/modprobe.d/ralphglasses-blacklist.conf <<'EOF'
blacklist btmtk
blacklist nouveau
options nouveau modeset=0
EOF

# ── NetworkManager wired priority ───────────────────────────────────
mkdir -p /etc/NetworkManager/conf.d
cat > /etc/NetworkManager/conf.d/10-proart.conf <<'EOF'
[device-atlantic]
match-device=driver:atlantic
managed=1

[device-igc]
match-device=driver:igc
managed=1
EOF

# ── USB4 / Thunderbolt auto-authorize ──────────────────────────────
mkdir -p /etc/bolt
cat > /etc/bolt/boltd.conf <<'EOF'
[policy]
default-policy=auto
EOF

# ── Enable essential services ──────────────────────────────────────
systemctl enable NetworkManager 2>/dev/null || true
systemctl enable bolt 2>/dev/null || true
systemctl enable bluetooth 2>/dev/null || true
systemctl set-default graphical.target 2>/dev/null || true

# ── Workspace directory ────────────────────────────────────────────
mkdir -p /workspace
chown ralph:ralph /workspace

# ── Cleanup ────────────────────────────────────────────────────────
echo "==> [chroot] Cleaning up..."
apt-get clean
rm -rf /var/lib/apt/lists/* /tmp/* /root/.cache

echo "==> [chroot] Setup complete."
