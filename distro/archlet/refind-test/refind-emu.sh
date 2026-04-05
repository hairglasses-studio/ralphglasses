#!/usr/bin/env bash
# refind-emu — Test rEFInd boot manager in QEMU without rebooting
# Usage:
#   ./refind-emu.sh                           # boot with live ESP config
#   ./refind-emu.sh --config path/to/conf     # boot with custom refind.conf
#   ./refind-emu.sh --refresh                 # rebuild ESP image from live ESP
#   ./refind-emu.sh --screenshot /tmp/out.png # headless mode, save screenshot
#   ./refind-emu.sh --fallback                # test EFI/Boot fallback path
#   ./refind-emu.sh --no-drivers              # strip filesystem drivers from ESP

set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
ESP_IMG="$DIR/esp-test.img"
OVMF_CODE="/usr/share/edk2/x64/OVMF_CODE.4m.fd"
OVMF_VARS_TEMPLATE="/usr/share/edk2/x64/OVMF_VARS.4m.fd"
OVMF_VARS="$DIR/ovmf_vars.fd"

REFRESH=false
FALLBACK=false
NO_DRIVERS=false
CUSTOM_CONF=""
SCREENSHOT=""

for arg in "$@"; do
    case "$arg" in
        --refresh)    REFRESH=true ;;
        --fallback)   FALLBACK=true ;;
        --no-drivers) NO_DRIVERS=true ;;
        --config)     shift_next=config ;;
        --screenshot) shift_next=screenshot ;;
        -h|--help)
            echo "Usage: $0 [options]"
            echo "  --refresh          Rebuild ESP image from live /boot/efi"
            echo "  --config FILE      Use custom refind.conf"
            echo "  --no-drivers       Remove filesystem drivers from test image"
            echo "  --screenshot FILE  Headless: save screenshot and exit"
            echo "  --fallback         Test EFI/Boot/bootx64.efi fallback path"
            exit 0
            ;;
        *)
            if [[ "${shift_next:-}" == "config" ]]; then
                CUSTOM_CONF="$arg"
                shift_next=""
            elif [[ "${shift_next:-}" == "screenshot" ]]; then
                SCREENSHOT="$arg"
                shift_next=""
            fi
            ;;
    esac
done

# Build ESP disk image if missing or --refresh or any modifier flags are set
needs_rebuild=false
if [[ ! -f "$ESP_IMG" ]] || $REFRESH || [[ -n "$CUSTOM_CONF" ]] || $NO_DRIVERS; then
    needs_rebuild=true
fi

if $needs_rebuild; then
    echo "==> Building ESP test image from /boot/efi ..."
    dd if=/dev/zero of="$ESP_IMG" bs=1M count=128 status=progress 2>/dev/null
    mkfs.vfat -F 32 -n ESP_TEST "$ESP_IMG" >/dev/null

    MNT=$(mktemp -d)
    sudo mount -o loop "$ESP_IMG" "$MNT"
    sudo cp -a /boot/efi/EFI "$MNT/"

    # Apply custom config
    if [[ -n "$CUSTOM_CONF" ]]; then
        echo "    Using custom config: $CUSTOM_CONF"
        sudo cp "$CUSTOM_CONF" "$MNT/EFI/refind/refind.conf"
        # Also update the Boot fallback config
        sudo cp "$CUSTOM_CONF" "$MNT/EFI/Boot/refind.conf"
    fi

    # Remove filesystem drivers if requested
    if $NO_DRIVERS; then
        echo "    Stripping filesystem drivers"
        sudo rm -f "$MNT/EFI/refind/drivers_x64/"*.efi
        sudo rm -f "$MNT/EFI/Boot/drivers_x64/"*.efi
    fi

    # Ensure fallback boot path points to rEFInd
    if ! $FALLBACK; then
        sudo cp "$MNT/EFI/refind/refind_x64.efi" "$MNT/EFI/Boot/bootx64.efi"
    fi

    sudo umount "$MNT"
    rmdir "$MNT"
    echo "==> ESP image ready"
fi

# Create writable OVMF vars copy (always fresh for reproducible tests)
cp "$OVMF_VARS_TEMPLATE" "$OVMF_VARS"

# Headless screenshot mode
if [[ -n "$SCREENSHOT" ]]; then
    QMP_SOCK="/tmp/refind-qmp-$$.sock"
    echo "==> Headless mode: booting rEFInd and taking screenshot ..."

    qemu-system-x86_64 \
        -enable-kvm \
        -m 512M \
        -drive if=pflash,format=raw,readonly=on,file="$OVMF_CODE" \
        -drive if=pflash,format=raw,file="$OVMF_VARS" \
        -drive file="$ESP_IMG",format=raw \
        -vnc :99 \
        -qmp unix:"$QMP_SOCK",server,nowait \
        -daemonize

    # Wait for rEFInd to render
    sleep 8

    # Take screenshot via QMP
    python3 -c "
import socket, json, time
sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
sock.connect('$QMP_SOCK')
sock.recv(4096)
sock.sendall(json.dumps({'execute': 'qmp_capabilities'}).encode() + b'\n')
time.sleep(0.5)
sock.recv(4096)
sock.sendall(json.dumps({'execute': 'screendump', 'arguments': {'filename': '/tmp/refind-screendump-$$.ppm'}}).encode() + b'\n')
time.sleep(1)
sock.recv(4096)
sock.sendall(json.dumps({'execute': 'quit'}).encode() + b'\n')
sock.close()
"
    # Convert to PNG
    magick /tmp/refind-screendump-$$.ppm "$SCREENSHOT"
    rm -f /tmp/refind-screendump-$$.ppm "$QMP_SOCK"
    echo "==> Screenshot saved: $SCREENSHOT"
    exit 0
fi

# Interactive mode
echo "==> Booting rEFInd in QEMU (close window or Ctrl-C to stop) ..."
echo "    Note: OS entries will show but won't boot (no real disks attached)"

qemu-system-x86_64 \
    -enable-kvm \
    -m 512M \
    -drive if=pflash,format=raw,readonly=on,file="$OVMF_CODE" \
    -drive if=pflash,format=raw,file="$OVMF_VARS" \
    -drive file="$ESP_IMG",format=raw \
    -display gtk,window-close=on \
    -name "rEFInd Emulator" \
    -no-reboot
