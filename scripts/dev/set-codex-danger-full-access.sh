#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./env.sh
source "${script_dir}/env.sh"

repo_root="$(rg_repo_root)"
root_home="/root"
user_home="${SUDO_HOME:-${HOME}}"

declare -A seen=()
configs=()

add_config() {
  local path
  path="$1"
  [[ -z "${path}" ]] && return 0
  if [[ -n "${seen["${path}"]:-}" ]]; then
    return 0
  fi
  seen["${path}"]=1
  configs+=("${path}")
}

add_config "${root_home}/.codex/config.toml"
add_config "${user_home}/.codex/config.toml"
add_config "${repo_root}/.codex/config.toml"

for extra in "$@"; do
  add_config "${extra}"
done

ensure_danger_full_access() {
  local path dir
  path="$1"
  dir="$(dirname "${path}")"

  mkdir -p "${dir}"

  if [[ ! -f "${path}" ]]; then
    cat > "${path}" <<EOF
approval_policy = "never"
sandbox_mode = "danger-full-access"
EOF
    echo "created: ${path}"
    return 0
  fi

  if grep -q "^sandbox_mode = " "${path}"; then
    sed -i "s/^sandbox_mode = \".*\"/sandbox_mode = \"danger-full-access\"/" "${path}"
  else
    printf "\nsandbox_mode = \"danger-full-access\"\n" >> "${path}"
  fi

  echo "updated: ${path}"
  grep -n "^sandbox_mode = " "${path}" || true
}

for config in "${configs[@]}"; do
  ensure_danger_full_access "${config}"
done

echo
echo "Codex sandbox_mode is set to danger-full-access in the selected config files."
echo "Restart Codex sessions to pick up the new config."
