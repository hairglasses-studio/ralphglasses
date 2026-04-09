#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="${1:-$(cd "${script_dir}/../.." && pwd)}"
allowlist_file="${repo_root}/.tracked-artifact-allowlist"

if ! git -C "${repo_root}" rev-parse --show-toplevel >/dev/null 2>&1; then
  echo "tracked artifact gate requires a git repository: ${repo_root}" >&2
  exit 1
fi

declare -A allowlist=()
if [[ -f "${allowlist_file}" ]]; then
  while IFS= read -r line || [[ -n "${line}" ]]; do
    line="${line%%#*}"
    line="${line#"${line%%[![:space:]]*}"}"
    line="${line%"${line##*[![:space:]]}"}"
    [[ -z "${line}" ]] && continue
    allowlist["${line}"]=1
  done < "${allowlist_file}"
fi

offenses=()

is_allowlisted() {
  local path="$1"
  [[ -n "${allowlist["${path}"]+set}" ]]
}

record_offense() {
  local path="$1"
  local reason="$2"
  offenses+=("${path}|${reason}")
}

check_path() {
  local path="$1"
  local base="${path##*/}"

  is_allowlisted "${path}" && return 0

  case "${base}" in
    .DS_Store)
      record_offense "${path}" "tracked Finder artifact (.DS_Store)"
      return 0
      ;;
    Thumbs.db)
      record_offense "${path}" "tracked Windows Explorer artifact (Thumbs.db)"
      return 0
      ;;
    *~)
      record_offense "${path}" "editor backup suffix (~)"
      return 0
      ;;
    *.tmp|*.temp|*.bak|*.orig|*.rej|*.old|*.disabled)
      record_offense "${path}" "temporary or backup suffix"
      return 0
      ;;
    placeholder|placeholder.*|placeholder-*|*-placeholder|*-placeholder.*|*.placeholder)
      record_offense "${path}" "placeholder output filename"
      return 0
      ;;
  esac

  case "/${path}/" in
    */tmp/*)
      record_offense "${path}" "tracked temp directory segment (/tmp/)"
      return 0
      ;;
    */temp/*)
      record_offense "${path}" "tracked temp directory segment (/temp/)"
      return 0
      ;;
    */temporary/*)
      record_offense "${path}" "tracked temporary directory segment (/temporary/)"
      return 0
      ;;
    */backup/*)
      record_offense "${path}" "tracked backup directory segment (/backup/)"
      return 0
      ;;
    */placeholder/*)
      record_offense "${path}" "tracked placeholder directory segment (/placeholder/)"
      return 0
      ;;
  esac
}

while IFS= read -r -d '' path; do
  check_path "${path}"
done < <(git -C "${repo_root}" ls-files -z)

if [[ "${#offenses[@]}" -gt 0 ]]; then
  echo "tracked artifact gate failed:" >&2
  for entry in "${offenses[@]}"; do
    path="${entry%%|*}"
    reason="${entry#*|}"
    echo " - ${path}: ${reason}" >&2
  done
  exit 1
fi

echo "tracked artifact gate: clean"
