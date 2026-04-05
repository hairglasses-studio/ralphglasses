#!/usr/bin/env bash
# Upload helper for claude.ai custom skills
# Steps through each skill, copies ZIP path to clipboard, opens browser on first run

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SKILLS=(mcpkit-go mcp-tool-scaffold go-conventions sway-rice ralphglasses-ops hairglasses-infra)
URL="https://claude.ai/customize/skills"
OPENED=false

bold="\033[1m"
cyan="\033[36m"
green="\033[32m"
yellow="\033[33m"
magenta="\033[35m"
reset="\033[0m"

echo -e "${bold}${cyan}Claude.ai Custom Skills Uploader${reset}"
echo -e "${cyan}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${reset}"
echo ""
echo -e "This script walks through ${bold}${#SKILLS[@]} skills${reset}."
echo -e "For each skill, it copies the ZIP path to your clipboard"
echo -e "and shows a summary. Upload at: ${cyan}${URL}${reset}"
echo ""

for i in "${!SKILLS[@]}"; do
    skill="${SKILLS[$i]}"
    zip_path="${SCRIPT_DIR}/${skill}.zip"
    skill_md="${SCRIPT_DIR}/${skill}/SKILL.md"
    num=$((i + 1))

    echo -e "${bold}${magenta}[${num}/${#SKILLS[@]}] ${skill}${reset}"
    echo -e "${cyan}─────────────────────────────────${reset}"

    # Extract description from frontmatter
    desc=$(sed -n '/^description:/,/^---/{/^description:/s/^description: //p}' "$skill_md" | head -1)
    echo -e "${yellow}Description:${reset} ${desc}"
    echo -e "${yellow}ZIP:${reset} ${zip_path}"
    echo -e "${yellow}Size:${reset} $(du -h "$zip_path" | cut -f1)"
    echo ""

    # Copy directory path to clipboard (for Ctrl+L in file picker)
    echo -n "$SCRIPT_DIR" | wl-copy
    echo -e "${green}✓ Directory path copied to clipboard${reset}"
    echo ""
    echo -e "  ${bold}1.${reset} Click 'Add skill' / upload button on claude.ai"
    echo -e "  ${bold}2.${reset} In the file picker, press ${bold}Ctrl+L${reset} to open the path bar"
    echo -e "  ${bold}3.${reset} Paste (${bold}Ctrl+V${reset}) and press Enter to navigate"
    echo -e "  ${bold}4.${reset} Select ${bold}${skill}.zip${reset}"

    # Open browser on first skill
    if [[ "$OPENED" == false ]]; then
        xdg-open "$URL" 2>/dev/null &
        OPENED=true
        echo -e ""
        echo -e "${green}✓ Opened ${URL}${reset}"
    fi

    echo ""

    if [[ $num -lt ${#SKILLS[@]} ]]; then
        echo -e "Press ${bold}Enter${reset} after uploading for the next skill..."
        read -r
        echo ""
    else
        echo -e "${bold}${green}All ${#SKILLS[@]} skills ready! Upload the last one above.${reset}"
    fi
done

echo ""
echo -e "${bold}${cyan}Done!${reset} Skills are also installed as Claude Code global skills at:"
echo -e "  ${cyan}~/.claude/skills/*${reset}"
