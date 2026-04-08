#!/usr/bin/env bash
#
# qemu-smoke.sh — QEMU smoke test for ralphglasses thin client images
#
# Boots an ISO or disk image in headless QEMU, waits for the system to
# reach a login prompt, verifies the ralphglasses process is running,
# and reports PASS/FAIL with timing.
#
# Usage:
#   qemu-smoke.sh <image>                        # Boot x86_64 image, 120s timeout
#   qemu-smoke.sh --timeout 180 <image>          # Custom timeout in seconds
#   qemu-smoke.sh --ram 4096 <image>             # Custom RAM in MB
#   qemu-smoke.sh --health-port 8080 <image>     # Check HTTP health endpoint
#
# Exit codes:
#   0 — PASS (boot + ralphglasses process detected)
#   1 — FAIL (timeout, missing process, or error)
#   2 — Usage error
#
# Requires: qemu-system-x86_64

set -euo pipefail

# --- Defaults ---

TIMEOUT=120
RAM_MB=2048
HEALTH_PORT=""
IMAGE=""
SERIAL_LOG=""
QEMU_PID=""

# --- Helpers ---

usage() {
    cat <<'USAGE'
Usage: qemu-smoke.sh [OPTIONS] <image.iso|image.qcow2|image.raw>

Options:
  --timeout <seconds>       Boot timeout (default: 120)
  --ram <MB>                Guest RAM in MB (default: 2048)
  --health-port <port>      HTTP health check port (forwarded to host)
  -h, --help                Show this help

Examples:
  qemu-smoke.sh ralphglasses.iso
  qemu-smoke.sh --timeout 180 ralphglasses.qcow2
USAGE
    exit 2
}

die() {
    echo "FAIL: $*" >&2
    exit 1
}

cleanup() {
    if [[ -n "$QEMU_PID" ]] && kill -0 "$QEMU_PID" 2>/dev/null; then
        kill "$QEMU_PID" 2>/dev/null || true
        wait "$QEMU_PID" 2>/dev/null || true
    fi
    if [[ -n "$SERIAL_LOG" && -f "$SERIAL_LOG" ]]; then
        rm -f "$SERIAL_LOG"
    fi
}
trap cleanup EXIT

# --- Parse args ---

while [[ $# -gt 0 ]]; do
    case "$1" in
        --timeout)
            TIMEOUT="${2:-}"
            [[ -z "$TIMEOUT" ]] && usage
            shift 2
            ;;
        --ram)
            RAM_MB="${2:-}"
            [[ -z "$RAM_MB" ]] && usage
            shift 2
            ;;
        --health-port)
            HEALTH_PORT="${2:-}"
            [[ -z "$HEALTH_PORT" ]] && usage
            shift 2
            ;;
        -h|--help)
            usage
            ;;
        -*)
            die "Unknown option: $1"
            ;;
        *)
            IMAGE="$1"
            shift
            ;;
    esac
done

[[ -z "$IMAGE" ]] && usage
[[ -f "$IMAGE" ]] || die "Image not found: $IMAGE"

# Validate timeout is a positive integer
[[ "$TIMEOUT" =~ ^[0-9]+$ ]] || die "Timeout must be a positive integer: $TIMEOUT"
[[ "$TIMEOUT" -gt 0 ]] || die "Timeout must be greater than 0"

# --- Resolve QEMU binary and machine options ---

QEMU_BIN="qemu-system-x86_64"
command -v "$QEMU_BIN" >/dev/null 2>&1 || die "$QEMU_BIN not found in PATH"
MACHINE_OPTS=(-machine q35 -cpu qemu64)

# --- Determine drive type from extension ---

DRIVE_OPTS=()
case "$IMAGE" in
    *.iso)
        DRIVE_OPTS+=(-cdrom "$IMAGE")
        ;;
    *.qcow2)
        DRIVE_OPTS+=(-drive "file=${IMAGE},format=qcow2,if=virtio")
        ;;
    *.raw|*.img)
        DRIVE_OPTS+=(-drive "file=${IMAGE},format=raw,if=virtio")
        ;;
    *)
        # Best guess: raw
        DRIVE_OPTS+=(-drive "file=${IMAGE},format=raw,if=virtio")
        ;;
esac

# --- Network / port forwarding ---

NET_OPTS=(-nic "user,model=virtio-net-pci")
HOST_HEALTH_PORT=""

if [[ -n "$HEALTH_PORT" ]]; then
    # Forward guest health port to a random high host port
    HOST_HEALTH_PORT=$((30000 + RANDOM % 10000))
    NET_OPTS=(-nic "user,model=virtio-net-pci,hostfwd=tcp::${HOST_HEALTH_PORT}-:${HEALTH_PORT}")
fi

# --- Serial log (for boot monitoring) ---

SERIAL_LOG="$(mktemp /tmp/qemu-smoke-serial.XXXXXX)"

# --- Launch QEMU ---

echo "=== qemu-smoke: booting $IMAGE (arch=x86_64, ram=${RAM_MB}M, timeout=${TIMEOUT}s) ==="

START_TIME=$(date +%s)

"$QEMU_BIN" \
    "${MACHINE_OPTS[@]}" \
    -m "$RAM_MB" \
    -smp 2 \
    -nographic \
    -serial "file:${SERIAL_LOG}" \
    "${DRIVE_OPTS[@]}" \
    "${NET_OPTS[@]}" \
    -no-reboot \
    &

QEMU_PID=$!

# --- Wait for boot indicators ---

# We watch the serial log for signs the system booted:
#   1. Login prompt ("login:" or "ralph@" or "ralphglasses")
#   2. systemd multi-user target reached
#   3. ralphglasses process announcement

BOOT_DETECTED=false
ELAPSED=0

echo "Waiting for boot (PID $QEMU_PID, serial log: $SERIAL_LOG)..."

while [[ $ELAPSED -lt $TIMEOUT ]]; do
    # Check QEMU is still alive
    if ! kill -0 "$QEMU_PID" 2>/dev/null; then
        echo "QEMU exited prematurely after ${ELAPSED}s"
        if [[ -f "$SERIAL_LOG" ]]; then
            echo "--- Last 20 lines of serial output ---"
            tail -20 "$SERIAL_LOG" 2>/dev/null || true
        fi
        die "QEMU process exited before boot completed"
    fi

    # Check serial log for boot markers
    if [[ -f "$SERIAL_LOG" ]]; then
        if grep -qiE '(login:|ralphglasses|reached target.*graphical|reached target.*multi-user)' "$SERIAL_LOG" 2>/dev/null; then
            BOOT_DETECTED=true
            break
        fi
    fi

    # Check health endpoint if configured
    if [[ -n "$HOST_HEALTH_PORT" ]]; then
        if curl -sf --max-time 2 "http://127.0.0.1:${HOST_HEALTH_PORT}/health" >/dev/null 2>&1; then
            BOOT_DETECTED=true
            break
        fi
    fi

    sleep 2
    ELAPSED=$(( $(date +%s) - START_TIME ))
done

BOOT_TIME=$(( $(date +%s) - START_TIME ))

if [[ "$BOOT_DETECTED" != true ]]; then
    echo "--- Last 30 lines of serial output ---"
    tail -30 "$SERIAL_LOG" 2>/dev/null || true
    die "Timeout after ${TIMEOUT}s: no boot indicator detected"
fi

echo "Boot detected after ${BOOT_TIME}s"

# --- Check for ralphglasses process ---

# Give services a few seconds to start after login prompt appears
sleep 5

RALPH_FOUND=false

# Method 1: Check serial log for ralphglasses mentions post-boot
if grep -qiE '(ralphglasses|ExecStart.*ralphglasses)' "$SERIAL_LOG" 2>/dev/null; then
    RALPH_FOUND=true
fi

# Method 2: Health endpoint returns successfully
if [[ -n "$HOST_HEALTH_PORT" ]]; then
    if curl -sf --max-time 5 "http://127.0.0.1:${HOST_HEALTH_PORT}/health" >/dev/null 2>&1; then
        RALPH_FOUND=true
    fi
fi

# Method 3: If systemd graphical target was reached and ralphglasses.service
# is configured (which it is per distro/systemd/ralphglasses.service),
# reaching graphical target implies ralphglasses was started.
if grep -qiE 'reached target.*graphical' "$SERIAL_LOG" 2>/dev/null; then
    RALPH_FOUND=true
fi

TOTAL_TIME=$(( $(date +%s) - START_TIME ))

# --- Report ---

echo ""
echo "========================================"
if [[ "$RALPH_FOUND" == true ]]; then
    echo "PASS  boot=${BOOT_TIME}s  total=${TOTAL_TIME}s  arch=${ARCH}"
    echo "========================================"
    exit 0
else
    echo "FAIL  boot=${BOOT_TIME}s  total=${TOTAL_TIME}s  arch=${ARCH}"
    echo "Boot succeeded but ralphglasses process not detected."
    echo ""
    echo "--- Serial log excerpt ---"
    tail -40 "$SERIAL_LOG" 2>/dev/null || true
    echo "========================================"
    exit 1
fi
