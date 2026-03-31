#!/usr/bin/env bash
# validate-dockerfile.sh — Static validator for Dockerfiles.
#
# Usage:
#   bash distro/scripts/validate-dockerfile.sh [Dockerfile]
#
# Returns 0 on success, 1 on any validation failure.
# Verifies COPY sources exist, FROM is present, and referenced configs exist.
# Optionally runs hadolint if installed.

set -euo pipefail

DOCKERFILE="${1:-distro/Dockerfile.manjaro.hyprland}"
ERRORS=0

if [ ! -f "${DOCKERFILE}" ]; then
  echo "ERROR: Dockerfile not found at ${DOCKERFILE}" >&2
  exit 1
fi

echo "==> Validating ${DOCKERFILE}"

# Determine the build context (parent of the Dockerfile's directory).
# distro/Dockerfile.* files use ".." as context (the repo root).
DOCKERFILE_DIR=$(dirname "${DOCKERFILE}")
BUILD_CONTEXT=$(cd "${DOCKERFILE_DIR}/.." && pwd)
echo "    Build context: ${BUILD_CONTEXT}"

# ── 1. FROM base image present ───────────────────────────────────────────────
FROM_COUNT=$(grep -cE '^FROM\s' "${DOCKERFILE}" || true)
if [ "${FROM_COUNT}" -eq 0 ]; then
  echo "ERROR: no FROM directive found" >&2
  ERRORS=$((ERRORS + 1))
else
  FROM_IMAGE=$(grep -m1 -oP '^FROM\s+\K\S+' "${DOCKERFILE}" || true)
  echo "    Base image: ${FROM_IMAGE}"
fi

# ── 2. COPY source paths exist ──────────────────────────────────────────────
# Extract COPY directives (skip --from=builder stages).
while IFS= read -r line; do
  # Skip multi-stage COPY --from=
  [[ "${line}" =~ --from= ]] && continue
  # Extract source path (second field after COPY)
  src=$(echo "${line}" | awk '{print $2}')
  [ -z "${src}" ] && continue

  # Resolve relative to build context
  resolved="${BUILD_CONTEXT}/${src}"
  if [ ! -e "${resolved}" ]; then
    echo "ERROR: COPY source not found: ${src} (resolved: ${resolved})" >&2
    ERRORS=$((ERRORS + 1))
  fi
done < <(grep -E '^\s*COPY\s' "${DOCKERFILE}" || true)
echo "    COPY source paths checked"

# ── 3. Config files referenced in RUN cp commands exist ──────────────────────
# Look for 'cp distro/...' or 'cp /build/distro/...' patterns in RUN blocks.
# These reference files from the build context.
while IFS= read -r line; do
  # Extract source path from cp commands
  srcs=$(echo "${line}" | grep -oP 'cp\s+\K\S+' || true)
  for src in ${srcs}; do
    # Skip if it's a destination path (starts with /) or a variable
    [[ "${src}" =~ ^/ ]] && continue
    [[ "${src}" =~ ^\$ ]] && continue
    resolved="${BUILD_CONTEXT}/${src}"
    if [ ! -e "${resolved}" ]; then
      echo "WARNING: RUN cp source may not exist at build time: ${src}" >&2
    fi
  done
done < <(grep -E 'cp\s+distro/' "${DOCKERFILE}" || true)
echo "    RUN cp references checked"

# ── 4. No secrets or credentials accidentally copied ─────────────────────────
SENSITIVE_PATTERNS='\.env|credentials|\.key|\.pem|\.secret|password'
SENSITIVE=$(grep -iE "COPY.*($SENSITIVE_PATTERNS)" "${DOCKERFILE}" || true)
if [ -n "${SENSITIVE}" ]; then
  echo "ERROR: potentially sensitive files in COPY directives:" >&2
  echo "${SENSITIVE}" >&2
  ERRORS=$((ERRORS + 1))
else
  echo "    No sensitive file patterns in COPY"
fi

# ── 5. Hadolint (optional) ──────────────────────────────────────────────────
if command -v hadolint &> /dev/null; then
  echo "    Running hadolint..."
  if ! hadolint "${DOCKERFILE}"; then
    echo "WARNING: hadolint reported issues (non-fatal)" >&2
  fi
else
  echo "    hadolint not installed, skipping lint"
fi

# ── Summary ──────────────────────────────────────────────────────────────────
if [ "${ERRORS}" -gt 0 ]; then
  echo "FAIL: ${ERRORS} error(s) found in ${DOCKERFILE}" >&2
  exit 1
fi

echo "PASS: ${DOCKERFILE} validated successfully"
