# ralphglasses bootable Linux ISO
# Build: docker build -t ralphglasses-os distro/
# Convert to ISO: see Makefile `make iso`
#
# Base: Ubuntu 24.04 with Kairos-compatible layout
# The resulting image is converted to a bootable ISO using
# grub-mkrescue/xorriso or the Kairos AuroraBoot toolchain.

FROM ubuntu:24.04 AS base

ARG DEBIAN_FRONTEND=noninteractive
ARG GO_VERSION=1.26.1
ARG NODE_MAJOR=20
ARG NVIDIA_DRIVER_VERSION=550

# ── Core system packages ─────────────────────────────────────────────
RUN apt-get update && apt-get install -y --no-install-recommends \
    # Boot / init
    linux-image-generic \
    systemd \
    systemd-sysv \
    dbus \
    # UEFI boot
    grub-efi-amd64 \
    grub-efi-amd64-signed \
    shim-signed \
    efibootmgr \
    # Xorg + i3
    xorg \
    xinit \
    i3 \
    i3status \
    i3lock \
    # Terminal
    alacritty \
    # Multi-monitor
    autorandr \
    arandr \
    # NVIDIA proprietary drivers
    nvidia-driver-${NVIDIA_DRIVER_VERSION} \
    nvidia-utils-${NVIDIA_DRIVER_VERSION} \
    libnvidia-gl-${NVIDIA_DRIVER_VERSION} \
    nvidia-settings \
    # Tools
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
    # Network
    network-manager \
    iproute2 \
    openssh-server \
    ethtool \
    # ── ProArt X870E hardware drivers ──────────────────────────────────
    # Firmware blobs (Intel I226-V igc, Marvell AQC113 atlantic, MediaTek, etc.)
    linux-firmware \
    # Extra kernel modules not in linux-image-generic (atlantic 10GbE, etc.)
    linux-modules-extra-generic \
    # USB4 / Thunderbolt (ASMedia ASM4242) — userspace authorization daemon
    bolt \
    # Audio (Realtek ALC1220P via snd-hda-intel — not critical for agent fleet)
    alsa-utils \
    # Bluetooth stack (for TP-Link UB400 CSR8510 dongle via btusb)
    bluez \
    bluez-tools \
    # Firmware updates (UEFI, NVMe, NIC firmware via LVFS)
    fwupd \
    # Filesystem
    squashfs-tools \
    dosfstools \
    parted \
    # Live boot (required for boot=live toram kernel params)
    live-boot \
    live-boot-initramfs-tools \
    # Misc
    locales \
    console-setup \
    kbd \
    && rm -rf /var/lib/apt/lists/*

# ── Locale ────────────────────────────────────────────────────────────
RUN locale-gen en_US.UTF-8
ENV LANG=en_US.UTF-8

# ── Go runtime ────────────────────────────────────────────────────────
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
    | tar -C /usr/local -xzf - \
    && ln -s /usr/local/go/bin/go /usr/local/bin/go \
    && ln -s /usr/local/go/bin/gofmt /usr/local/bin/gofmt

# ── Node.js (for Claude Code CLI) ────────────────────────────────────
RUN curl -fsSL https://deb.nodesource.com/setup_${NODE_MAJOR}.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

# ── Claude Code CLI ──────────────────────────────────────────────────
RUN npm install -g @anthropic-ai/claude-code

# ── Non-root user ────────────────────────────────────────────────────
RUN useradd -m -s /bin/bash -G sudo,video,render,input,bluetooth,plugdev ralph \
    && echo "ralph ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/ralph

# ── Build ralphglasses from source ───────────────────────────────────
COPY . /build/ralphglasses
WORKDIR /build/ralphglasses
RUN go build -o /usr/local/bin/ralphglasses . \
    && chmod 755 /usr/local/bin/ralphglasses

# ── i3 config ────────────────────────────────────────────────────────
RUN mkdir -p /home/ralph/.config/i3 \
    && cp /build/ralphglasses/distro/i3/config /home/ralph/.config/i3/config \
    && chown -R ralph:ralph /home/ralph/.config

# ── autorandr profiles (if any) ──────────────────────────────────────
RUN mkdir -p /home/ralph/.config/autorandr \
    && cp -r /build/ralphglasses/distro/autorandr/* /home/ralph/.config/autorandr/ 2>/dev/null \
    || true \
    && chown -R ralph:ralph /home/ralph/.config/autorandr

# ── systemd: ralphglasses service ────────────────────────────────────
COPY distro/systemd/ralphglasses.service /etc/systemd/system/ralphglasses.service
RUN systemctl enable ralphglasses.service 2>/dev/null || true

# ── hw-detect: first-boot hardware detection ────────────────────────
COPY distro/scripts/hw-detect.sh /usr/local/bin/hw-detect.sh
RUN chmod 755 /usr/local/bin/hw-detect.sh
COPY distro/systemd/hw-detect.service /etc/systemd/system/hw-detect.service
RUN systemctl enable hw-detect.service 2>/dev/null || true

# ── install-to-disk script ──────────────────────────────────────────
COPY distro/scripts/install-to-disk.sh /usr/local/bin/install-to-disk.sh
RUN chmod 755 /usr/local/bin/install-to-disk.sh

# ── Autologin to i3 via getty ─────────────────────────────────────────
RUN mkdir -p /etc/systemd/system/getty@tty1.service.d \
    && printf '[Service]\nExecStart=\nExecStart=-/sbin/agetty --autologin ralph --noclear %%I $TERM\n' \
       > /etc/systemd/system/getty@tty1.service.d/autologin.conf

# ── .xinitrc for startx → i3 ─────────────────────────────────────────
RUN printf '#!/bin/sh\nexec i3\n' > /home/ralph/.xinitrc \
    && chmod 755 /home/ralph/.xinitrc \
    && chown ralph:ralph /home/ralph/.xinitrc

# ── .bash_profile: auto startx on tty1 ──────────────────────────────
RUN printf 'if [ -z "$DISPLAY" ] && [ "$(tty)" = "/dev/tty1" ]; then\n  exec startx\nfi\n' \
    >> /home/ralph/.bash_profile \
    && chown ralph:ralph /home/ralph/.bash_profile

# ── NVIDIA: configure Xorg for dual GPU ──────────────────────────────
RUN mkdir -p /etc/X11/xorg.conf.d \
    && cat > /etc/X11/xorg.conf.d/20-nvidia.conf <<'XEOF'
# Dual NVIDIA GPU configuration
# nvidia-xconfig --enable-all-gpus generates a more specific version
# at first boot. This is a safe starting point.
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

# ── Hostname ──────────────────────────────────────────────────────────
RUN echo "ralphglasses" > /etc/hostname

# ── ProArt X870E: kernel module config ──────────────────────────────
# Blacklist problematic modules; hw-detect.sh regenerates this at first boot
RUN mkdir -p /etc/modprobe.d && cat > /etc/modprobe.d/ralphglasses-blacklist.conf <<'EOF'
# MediaTek MT7927 Bluetooth — btmtk causes kernel panics on this board
blacklist btmtk
# Prevent nouveau from conflicting with nvidia
blacklist nouveau
options nouveau modeset=0
EOF

# ── ProArt X870E: NetworkManager wired connection priority ──────────
# User is connected to the Marvell 10GbE port — make it primary
RUN mkdir -p /etc/NetworkManager/conf.d && cat > /etc/NetworkManager/conf.d/10-proart.conf <<'EOF'
[device-atlantic]
# Marvell AQC113CS 10GbE — primary (user's active connection)
match-device=driver:atlantic
managed=1

[device-igc]
# Intel I226-V 2.5GbE — fallback
match-device=driver:igc
managed=1
EOF

# ── TP-Link UB400 (CSR8510 A10) Bluetooth dongle ───────────────────
# Many CSR8510 dongles are clones that need HCI mode forced via udev
RUN mkdir -p /etc/udev/rules.d && cat > /etc/udev/rules.d/97-csr8510-bluetooth.rules <<'EOF'
# CSR8510 A10 clone — force HCI mode (TP-Link UB400/UB500)
ATTR{idVendor}=="0a12", ATTR{idProduct}=="0001", RUN+="/lib/udev/hid2hci --mode=hci --method=csr2 --devpath=%p"
EOF

# ── USB4 / Thunderbolt: auto-authorize devices ─────────────────────
RUN mkdir -p /etc/bolt && cat > /etc/bolt/boltd.conf <<'EOF'
[policy]
# Auto-authorize Thunderbolt/USB4 devices (ASMedia ASM4242)
default-policy=auto
EOF

# ── Enable essential services ─────────────────────────────────────────
RUN systemctl enable NetworkManager 2>/dev/null || true \
    && systemctl enable bolt 2>/dev/null || true \
    && systemctl enable bluetooth 2>/dev/null || true \
    && systemctl set-default graphical.target 2>/dev/null || true

# ── Workspace directory ──────────────────────────────────────────────
RUN mkdir -p /workspace && chown ralph:ralph /workspace

# ── Cleanup build artifacts ──────────────────────────────────────────
RUN rm -rf /build/ralphglasses /root/.cache/go-build /tmp/* \
    && apt-get clean

# The resulting image is a full rootfs. Convert to ISO with:
#   make iso  (uses grub-mkrescue + xorriso)
