#!/usr/bin/env bash
set -euo pipefail

rg_repo_root() {
  local script_dir
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  cd "${script_dir}/../.." && pwd
}

rg_go_version() {
  local repo_root
  repo_root="${1:-$(rg_repo_root)}"
  awk '/^go / { print $2; exit }' "${repo_root}/go.mod"
}

rg_tools_root() {
  local repo_root
  repo_root="${1:-$(rg_repo_root)}"
  printf '%s\n' "${repo_root}/.tools"
}

rg_go_root() {
  local repo_root version
  repo_root="${1:-$(rg_repo_root)}"
  version="${2:-$(rg_go_version "${repo_root}")}"
  printf '%s\n' "$(rg_tools_root "${repo_root}")/go/${version}"
}

rg_go_bin() {
  local repo_root version
  repo_root="${1:-$(rg_repo_root)}"
  version="${2:-$(rg_go_version "${repo_root}")}"
  printf '%s\n' "$(rg_go_root "${repo_root}" "${version}")/bin/go"
}

rg_local_bin_dir() {
  local repo_root
  repo_root="${1:-$(rg_repo_root)}"
  printf '%s\n' "$(rg_tools_root "${repo_root}")/bin"
}

rg_path_with_tools() {
  local repo_root
  repo_root="${1:-$(rg_repo_root)}"
  printf '%s:%s\n' "$(dirname "$(rg_go_bin "${repo_root}")")" "$(rg_local_bin_dir "${repo_root}")"
}

rg_has_matching_system_go() {
  local want
  want="${1}"
  if ! command -v go >/dev/null 2>&1; then
    return 1
  fi
  local got
  got="$(go version | awk '{print $3}' | sed 's/^go//')"
  [[ "${got}" == "${want}" ]]
}

rg_download_go() {
  local repo_root version go_root archive os arch url tmp_dir
  repo_root="${1}"
  version="${2}"
  go_root="$(rg_go_root "${repo_root}" "${version}")"

  case "$(uname -s)" in
    Linux) os="linux" ;;
    Darwin) os="darwin" ;;
    *)
      echo "unsupported OS for local Go bootstrap: $(uname -s)" >&2
      return 1
      ;;
  esac

  case "$os" in
    linux)
      case "$(uname -m)" in
        x86_64|amd64) arch="amd64" ;;
        arm64|aarch64)
          echo "linux arm64 is no longer supported for local Go bootstrap" >&2
          return 1
          ;;
        *)
          echo "unsupported architecture for local Go bootstrap: $(uname -m)" >&2
          return 1
          ;;
      esac
      ;;
    darwin)
      case "$(uname -m)" in
        x86_64|amd64) arch="amd64" ;;
        arm64|aarch64) arch="arm64" ;;
        *)
          echo "unsupported architecture for local Go bootstrap: $(uname -m)" >&2
          return 1
          ;;
      esac
      ;;
  esac

  archive="go${version}.${os}-${arch}.tar.gz"
  url="https://go.dev/dl/${archive}"
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "'"${tmp_dir}"'"' RETURN

  mkdir -p "$(dirname "${go_root}")"
  echo "bootstrapping Go ${version} into ${go_root}" >&2
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "${tmp_dir}/${archive}" "${url}"
  elif command -v wget >/dev/null 2>&1; then
    wget -q -O "${tmp_dir}/${archive}" "${url}"
  elif command -v python3 >/dev/null 2>&1; then
    python3 - <<PY
import urllib.request
urllib.request.urlretrieve("${url}", "${tmp_dir}/${archive}")
PY
  else
    echo "error: no download tool available (need curl, wget, or python3)" >&2
    return 1
  fi
  tar -xzf "${tmp_dir}/${archive}" -C "${tmp_dir}"
  rm -rf "${go_root}"
  mv "${tmp_dir}/go" "${go_root}"
}

rg_ensure_go() {
  local repo_root version go_bin
  repo_root="${1:-$(rg_repo_root)}"
  version="${2:-$(rg_go_version "${repo_root}")}"
  go_bin="$(rg_go_bin "${repo_root}" "${version}")"

  if [[ -x "${go_bin}" ]]; then
    printf '%s\n' "${go_bin}"
    return 0
  fi

  if rg_has_matching_system_go "${version}"; then
    command -v go
    return 0
  fi

  rg_download_go "${repo_root}" "${version}"
  printf '%s\n' "${go_bin}"
}
