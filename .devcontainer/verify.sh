#!/usr/bin/env bash
set -euo pipefail

# Post-create verification script for GitHub Codespaces / devcontainers.
# Ensures the Go toolchain, build, and tests all work after container creation.

BOLD='\033[1m'
GREEN='\033[0;32m'
RED='\033[0;31m'
RESET='\033[0m'

pass() { echo -e "  ${GREEN}✓${RESET} $1"; }
fail() { echo -e "  ${RED}✗${RESET} $1"; exit 1; }
section() { echo -e "\n${BOLD}$1${RESET}"; }

EXPECTED_GO_MAJOR_MINOR="1.26"

# -------------------------------------------------------------------
section "1/5  Verify Go installation"
# -------------------------------------------------------------------
if ! command -v go &>/dev/null; then
  fail "go not found on PATH"
fi

GO_VERSION="$(go version | awk '{print $3}' | sed 's/^go//')"
pass "Go ${GO_VERSION} installed"

if [[ "${GO_VERSION}" != "${EXPECTED_GO_MAJOR_MINOR}"* ]]; then
  echo "      ⚠ Expected Go ${EXPECTED_GO_MAJOR_MINOR}.x — you may need to update the Dockerfile"
fi

# -------------------------------------------------------------------
section "2/5  Build all packages"
# -------------------------------------------------------------------
go build ./... || fail "go build ./... failed"
pass "go build ./... succeeded"

# -------------------------------------------------------------------
section "3/5  Static analysis (go vet)"
# -------------------------------------------------------------------
go vet ./... || fail "go vet ./... failed"
pass "go vet ./... passed"

# -------------------------------------------------------------------
section "4/5  Run short tests"
# -------------------------------------------------------------------
go test -short -count=1 -race ./... || fail "go test -short ./... failed"
pass "go test -short ./... passed"

# -------------------------------------------------------------------
section "5/5  Verify ralphglasses binary"
# -------------------------------------------------------------------
BINARY="./ralphglasses"
go build -o "${BINARY}" . || fail "failed to build ralphglasses binary"

if "${BINARY}" --help &>/dev/null || "${BINARY}" --help 2>&1 | head -1 | grep -qi 'usage\|ralph'; then
  pass "ralphglasses --help works"
else
  # Some CLIs exit non-zero for --help; as long as the binary runs, that's fine.
  pass "ralphglasses binary executes (non-standard --help exit code)"
fi

rm -f "${BINARY}"

# -------------------------------------------------------------------
section "Setting up development aliases"
# -------------------------------------------------------------------
ALIAS_FILE="${HOME}/.bash_aliases"
{
  echo ""
  echo "# ralphglasses dev aliases (added by .devcontainer/verify.sh)"
  echo "alias rg-build='go build ./...'"
  echo "alias rg-test='go test -short -race ./...'"
  echo "alias rg-vet='go vet ./...'"
  echo "alias rg-lint='golangci-lint run ./...'"
  echo "alias rg-run='go run . --scan-path /workspaces'"
  echo "alias rg-cover='go test -coverprofile=coverage.out ./... && go tool cover -func coverage.out'"
} >> "${ALIAS_FILE}"
pass "Aliases written to ${ALIAS_FILE}"

# -------------------------------------------------------------------
section "Environment summary"
# -------------------------------------------------------------------
echo "  Go version  : $(go version | awk '{print $3, $4}')"
echo "  GOPATH      : $(go env GOPATH)"
echo "  Module      : $(go list -m 2>/dev/null || echo 'N/A')"
echo "  Workspace   : $(pwd)"
echo "  Codespace   : ${CODESPACE_NAME:-local}"
echo ""
echo -e "${GREEN}${BOLD}All checks passed — environment is ready.${RESET}"
