#!/usr/bin/env bash
# ota.sh — System-level OTA update for the ralphglasses thin client.
# Handles kernel, rootfs overlay, and ralphglasses binary updates.
#
# Usage:
#   ota.sh check            — Check for available updates
#   ota.sh apply            — Download and apply updates
#   ota.sh rollback         — Revert to previous rootfs/kernel
#   ota.sh status           — Show current partition and version info
#
# Environment:
#   OTA_ENDPOINT            — Update server base URL (required)
#   OTA_CHANNEL             — Update channel (default: stable)
#   OTA_REBOOT              — Set to "1" to auto-reboot after apply (default: 0)

set -euo pipefail

readonly SCRIPT_NAME="$(basename "$0")"
readonly STATE_DIR="/var/lib/ralphglasses/ota"
readonly BACKUP_DIR="${STATE_DIR}/backup"
readonly VERSION_FILE="${STATE_DIR}/version"
readonly OVERLAY_ROOT="/opt/ralphglasses/overlay"
readonly BINARY_PATH="/usr/local/bin/ralphglasses"

OTA_CHANNEL="${OTA_CHANNEL:-stable}"
OTA_REBOOT="${OTA_REBOOT:-0}"

log() {
    echo "[$(date -u '+%Y-%m-%dT%H:%M:%SZ')] ${SCRIPT_NAME}: $*" >&2
}

die() {
    log "ERROR: $*"
    exit 1
}

require_root() {
    if [ "$(id -u)" -ne 0 ]; then
        die "must be run as root"
    fi
}

current_version() {
    if [ -f "${VERSION_FILE}" ]; then
        cat "${VERSION_FILE}"
    else
        echo "unknown"
    fi
}

# check — Query the update server for a newer release.
cmd_check() {
    [ -z "${OTA_ENDPOINT:-}" ] && die "OTA_ENDPOINT is not set"

    local url="${OTA_ENDPOINT}/v1/update/${OTA_CHANNEL}/linux/$(uname -m)"
    local cur
    cur="$(current_version)"
    log "checking for updates (current: ${cur}, channel: ${OTA_CHANNEL})"

    local http_code
    http_code="$(curl -sS -o /tmp/ota-check.json -w '%{http_code}' \
        -H "X-Current-Version: ${cur}" \
        "${url}" 2>/dev/null)" || die "failed to contact update server"

    case "${http_code}" in
        200)
            log "update available:"
            cat /tmp/ota-check.json
            echo
            return 0
            ;;
        204|304)
            log "already up-to-date"
            return 1
            ;;
        *)
            die "update server returned HTTP ${http_code}"
            ;;
    esac
}

# apply — Download, verify, and install the update.
cmd_apply() {
    require_root
    [ -z "${OTA_ENDPOINT:-}" ] && die "OTA_ENDPOINT is not set"

    # Fetch release metadata.
    if ! cmd_check 2>/dev/null; then
        log "no update available"
        return 0
    fi

    local version url checksum
    version="$(jq -r '.version' /tmp/ota-check.json)"
    url="$(jq -r '.url' /tmp/ota-check.json)"
    checksum="$(jq -r '.checksum' /tmp/ota-check.json)"

    [ -z "${version}" ] || [ "${version}" = "null" ] && die "missing version in release metadata"
    [ -z "${url}" ] || [ "${url}" = "null" ] && die "missing url in release metadata"
    [ -z "${checksum}" ] || [ "${checksum}" = "null" ] && die "missing checksum in release metadata"

    log "downloading ${version} from ${url}"

    local tmpdir
    tmpdir="$(mktemp -d /tmp/ota-download.XXXXXX)"
    trap 'rm -rf "${tmpdir}"' EXIT

    curl -sS -o "${tmpdir}/artifact.tar.gz" "${url}" || die "download failed"

    # Verify SHA256 checksum.
    local got_checksum
    got_checksum="$(sha256sum "${tmpdir}/artifact.tar.gz" | awk '{print $1}')"
    if [ "${got_checksum}" != "${checksum}" ]; then
        die "checksum mismatch: got ${got_checksum}, want ${checksum}"
    fi
    log "checksum verified"

    # Back up current state.
    mkdir -p "${BACKUP_DIR}"
    if [ -f "${BINARY_PATH}" ]; then
        cp -a "${BINARY_PATH}" "${BACKUP_DIR}/ralphglasses.bak"
        log "backed up current binary"
    fi
    if [ -f "${VERSION_FILE}" ]; then
        cp -a "${VERSION_FILE}" "${BACKUP_DIR}/version.bak"
    fi

    # Extract and install.
    tar xzf "${tmpdir}/artifact.tar.gz" -C "${tmpdir}"

    # Install binary if present in archive.
    if [ -f "${tmpdir}/ralphglasses" ]; then
        install -m 0755 "${tmpdir}/ralphglasses" "${BINARY_PATH}"
        log "installed binary"
    fi

    # Apply rootfs overlay if present.
    if [ -d "${tmpdir}/overlay" ]; then
        mkdir -p "${OVERLAY_ROOT}"
        # Back up current overlay.
        if [ -d "${OVERLAY_ROOT}/current" ]; then
            mv "${OVERLAY_ROOT}/current" "${OVERLAY_ROOT}/previous"
        fi
        cp -a "${tmpdir}/overlay" "${OVERLAY_ROOT}/current"
        log "applied rootfs overlay"
    fi

    # Install kernel if present.
    if [ -f "${tmpdir}/vmlinuz" ]; then
        if [ -f /boot/vmlinuz ]; then
            cp -a /boot/vmlinuz "${BACKUP_DIR}/vmlinuz.bak"
        fi
        install -m 0644 "${tmpdir}/vmlinuz" /boot/vmlinuz
        log "installed kernel"

        if [ -f "${tmpdir}/initrd.img" ]; then
            install -m 0644 "${tmpdir}/initrd.img" /boot/initrd.img
        fi
    fi

    # Record new version.
    mkdir -p "$(dirname "${VERSION_FILE}")"
    echo "${version}" > "${VERSION_FILE}"
    log "updated to version ${version}"

    if [ "${OTA_REBOOT}" = "1" ]; then
        log "rebooting in 5 seconds..."
        sleep 5
        reboot
    else
        log "update applied. reboot to complete kernel/overlay changes."
    fi
}

# rollback — Restore the previous version from backup.
cmd_rollback() {
    require_root

    if [ ! -d "${BACKUP_DIR}" ]; then
        die "no backup found at ${BACKUP_DIR}"
    fi

    if [ -f "${BACKUP_DIR}/ralphglasses.bak" ]; then
        install -m 0755 "${BACKUP_DIR}/ralphglasses.bak" "${BINARY_PATH}"
        log "restored binary"
    fi

    if [ -d "${OVERLAY_ROOT}/previous" ]; then
        rm -rf "${OVERLAY_ROOT}/current"
        mv "${OVERLAY_ROOT}/previous" "${OVERLAY_ROOT}/current"
        log "restored rootfs overlay"
    fi

    if [ -f "${BACKUP_DIR}/vmlinuz.bak" ]; then
        install -m 0644 "${BACKUP_DIR}/vmlinuz.bak" /boot/vmlinuz
        log "restored kernel"
    fi

    if [ -f "${BACKUP_DIR}/version.bak" ]; then
        cp -a "${BACKUP_DIR}/version.bak" "${VERSION_FILE}"
        log "restored version to $(cat "${VERSION_FILE}")"
    fi

    log "rollback complete. reboot to activate kernel/overlay changes."
}

# status — Display current OTA state.
cmd_status() {
    echo "Version:     $(current_version)"
    echo "Channel:     ${OTA_CHANNEL}"
    echo "Binary:      ${BINARY_PATH}"
    echo "State dir:   ${STATE_DIR}"
    echo "Overlay:     ${OVERLAY_ROOT}"
    if [ -d "${BACKUP_DIR}" ]; then
        echo "Backup:      available"
    else
        echo "Backup:      none"
    fi
    if [ -f "${BINARY_PATH}" ]; then
        echo "Binary size: $(stat --format='%s' "${BINARY_PATH}" 2>/dev/null || stat -f '%z' "${BINARY_PATH}" 2>/dev/null || echo 'unknown') bytes"
    fi
}

case "${1:-}" in
    check)    cmd_check ;;
    apply)    cmd_apply ;;
    rollback) cmd_rollback ;;
    status)   cmd_status ;;
    *)
        echo "Usage: ${SCRIPT_NAME} {check|apply|rollback|status}" >&2
        exit 1
        ;;
esac
