# claude-skills

Custom Claude skills for the hairglasses-studio org. Dual-purpose:

- **claude.ai**: Upload ZIPs at [claude.ai/customize/skills](https://claude.ai/customize/skills)
- **Claude Code**: Symlinked into `~/.claude/skills/` for global availability

## Skills

| Skill | Purpose |
|-------|---------|
| `mcpkit-go` | MCP framework API reference (handlers, middleware, registry) |
| `mcp-tool-scaffold` | Tool module/handler/test templates |
| `go-conventions` | Go coding standards for hairglasses-studio |
| `sway-rice` | Sway/Wayland desktop environment reference |
| `ralphglasses-ops` | Multi-LLM orchestration reference |
| `hairglasses-infra` | Homelab/cloud/network infrastructure reference |

## Usage

```bash
# Generate ZIPs and upload to claude.ai interactively
./upload-skills.sh

# Install as Claude Code global skills
for skill in */; do
  [ -f "$skill/SKILL.md" ] && ln -sf "$(pwd)/$skill/SKILL.md" ~/.claude/skills/"${skill%/}"/SKILL.md
done
```
