# ralphglasses bootable Linux ISO
# Build: docker build -t ralphglasses-os distro/
# Convert to ISO: see Makefile `make iso`
#
# Base: Ubuntu 24.04 with Kairos-compatible layout
# The resulting image is converted to a bootable ISO using
# grub-mkrescue/xorriso or the Kairos AuroraBoot toolchain.

FROM ubuntu:24.04 AS base

ARG DEBIAN_FRONTEND=noninteractive
ARG GO_VERSION=1.22.5
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
    # Filesystem
    squashfs-tools \
    dosfstools \
    parted \
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
RUN useradd -m -s /bin/bash -G sudo,video,render,input ralph \
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

# ── Enable essential services ─────────────────────────────────────────
RUN systemctl enable NetworkManager 2>/dev/null || true \
    && systemctl enable systemd-networkd 2>/dev/null || true \
    && systemctl set-default graphical.target 2>/dev/null || true

# ── Workspace directory ──────────────────────────────────────────────
RUN mkdir -p /workspace && chown ralph:ralph /workspace

# ── Cleanup build artifacts ──────────────────────────────────────────
RUN rm -rf /build/ralphglasses /root/.cache/go-build /tmp/* \
    && apt-get clean

# The resulting image is a full rootfs. Convert to ISO with:
#   make iso  (uses grub-mkrescue + xorriso)
