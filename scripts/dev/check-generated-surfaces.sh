#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"
go_cmd="${GO_CMD:-${repo_root}/scripts/dev/go.sh}"
python_bin="${PYTHON_BIN:-python3}"

cd "${repo_root}"

echo "generated surface gate: provider role projections"
"${python_bin}" "${repo_root}/scripts/sync-provider-roles.py" --check

echo "generated surface gate: generated docs and registry surfaces"
"${go_cmd}" test -count=1 \
  ./tools/genconfig \
  ./tools/gendoc \
  ./tools/genskilldoc \
  ./tools/genmcpdocs \
  ./tools/genskillsurface \
  ./internal/mcpserver \
  -run '^(TestRenderConfigReference_MatchesCheckedInDoc|TestRenderManpages_MatchesCheckedInTree|TestRenderSkillMarkdown_MatchesCheckedInDoc|TestRenderTemplateData_MatchesCheckedInDoc|TestCheckSkillSurfaces_MatchesCheckedInFiles|TestBuildToolGroups_ExpectedCounts)$'

echo "generated surface gate: clean"
