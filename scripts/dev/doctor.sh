#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./env.sh
source "${script_dir}/env.sh"

strict=0
if [[ "${1:-}" == "--strict" ]]; then
  strict=1
fi

repo_root="$(rg_repo_root)"
rg_load_host_env "${repo_root}"
failures=0
warnings=0

check() {
  local name status detail
  name="$1"
  status="$2"
  detail="$3"
  printf '%-18s %-7s %s\n' "${name}" "${status}" "${detail}"
  if [[ "${status}" == "fail" ]]; then
    failures=$((failures + 1))
  elif [[ "${status}" == "warn" ]]; then
    warnings=$((warnings + 1))
  fi
}

go_bin="$(rg_go_bin "${repo_root}")"
if [[ -x "${go_bin}" ]]; then
  check "go" "ok" "$("${go_bin}" version)"
elif command -v go >/dev/null 2>&1; then
  check "go" "warn" "system go present but bootstrap-managed Go not installed"
else
  check "go" "fail" "run ./scripts/bootstrap-toolchain.sh"
fi

if command -v make >/dev/null 2>&1; then
  check "make" "ok" "$(command -v make)"
else
  check "make" "fail" "make is required; install via system package manager"
fi

if command -v git >/dev/null 2>&1; then
  check "git" "ok" "$(git --version)"
else
  check "git" "fail" "git is required; install via system package manager or Xcode CLT"
fi

for tool in shellcheck bats; do
  if command -v "${tool}" >/dev/null 2>&1; then
    check "${tool}" "ok" "$(command -v "${tool}")"
  else
    check "${tool}" "warn" "missing on host; use devcontainer or install via system package manager"
  fi
done

# macOS-specific checks
if [[ "$(uname -s)" == "Darwin" ]]; then
  if command -v brew >/dev/null 2>&1; then
    check "brew" "ok" "$(command -v brew)"
  else
    check "brew" "warn" "Homebrew not found; recommended for installing dependencies"
  fi

  if xcode-select -p >/dev/null 2>&1; then
    check "xcode-clt" "ok" "$(xcode-select -p)"
  else
    check "xcode-clt" "warn" "Xcode CLT not installed; run: xcode-select --install"
  fi
fi

# Linux-specific checks
if [[ "$(uname -s)" == "Linux" ]]; then
  if command -v pacman >/dev/null 2>&1; then
    check "pacman" "ok" "$(command -v pacman)"
  else
    check "pacman" "warn" "Package manager not found"
  fi

  if command -v sway >/dev/null 2>&1; then
    check "sway" "ok" "$(sway --version 2>/dev/null || command -v sway)"
  else
    check "sway" "warn" "Sway compositor not found"
  fi

  if command -v waybar >/dev/null 2>&1; then
    check "waybar" "ok" "$(command -v waybar)"
  else
    check "waybar" "warn" "Waybar status bar not found"
  fi

  if command -v alacritty >/dev/null 2>&1; then
    check "alacritty" "ok" "$(command -v alacritty)"
  else
    check "alacritty" "warn" "Terminal emulator not found"
  fi

  if command -v wl-copy >/dev/null 2>&1; then
    check "wl-copy" "ok" "$(command -v wl-copy)"
  else
    check "wl-copy" "warn" "Wayland clipboard (wl-copy) not found"
  fi
fi

if command -v cc >/dev/null 2>&1 || command -v gcc >/dev/null 2>&1; then
  check "c-compiler" "ok" "$(command -v cc || command -v gcc)"
else
  check "c-compiler" "warn" "missing; go test -race will fail"
fi

if [[ -x "$(rg_local_bin_dir "${repo_root}")/golangci-lint" ]] || command -v golangci-lint >/dev/null 2>&1; then
  check "golangci-lint" "ok" "$(command -v golangci-lint || printf '%s' "$(rg_local_bin_dir "${repo_root}")/golangci-lint")"
else
  check "golangci-lint" "warn" "run ./scripts/bootstrap-toolchain.sh"
fi

for cli in codex gemini claude; do
  if command -v "${cli}" >/dev/null 2>&1; then
    check "${cli}" "ok" "$(command -v "${cli}")"
  else
    check "${cli}" "warn" "provider CLI missing"
  fi
done

for env_var in OPENAI_API_KEY GOOGLE_API_KEY ANTHROPIC_API_KEY; do
  if [[ -n "${!env_var:-}" ]]; then
    check "${env_var}" "ok" "set"
  else
    check "${env_var}" "warn" "not set"
  fi
done

if [[ -f "${repo_root}/.codex/config.toml" ]]; then
  if grep -q 'command = "go"' "${repo_root}/.codex/config.toml"; then
    check ".codex/config" "fail" "still uses raw go; expected wrapper script"
  else
    check ".codex/config" "ok" "wrapper-based startup configured"
  fi
else
  check ".codex/config" "warn" "file not found: .codex/config.toml"
fi

if [[ -f "${repo_root}/.codex/config.toml" ]]; then
    if grep -q "sandbox_mode = \"danger-full-access\"" "${repo_root}/.codex/config.toml"; then
    check ".codex/sandbox" "ok" "danger-full-access configured"
  elif grep -q "^sandbox_mode = " "${repo_root}/.codex/config.toml"; then
    check ".codex/sandbox" "warn" "repo-local sandbox overrides global policy; run bash ./scripts/dev/set-codex-danger-full-access.sh"
  else
    check ".codex/sandbox" "warn" "sandbox_mode not set in repo-local config"
  fi
else
  check ".codex/sandbox" "warn" "file not found: .codex/config.toml"
fi

if [[ -f "${repo_root}/.mcp.json" ]]; then
  if grep -q '"command": "go"' "${repo_root}/.mcp.json"; then
    check ".mcp.json" "fail" "still uses raw go; expected wrapper script"
  else
    check ".mcp.json" "ok" "wrapper-based startup configured"
  fi
else
  check ".mcp.json" "warn" "file not found: .mcp.json"
fi

if [[ "${strict}" -eq 1 && "${warnings}" -gt 0 ]]; then
  exit 1
fi

if [[ "${failures}" -gt 0 ]]; then
  exit 1
fi
