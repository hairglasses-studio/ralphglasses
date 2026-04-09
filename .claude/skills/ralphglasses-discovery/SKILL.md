---
name: ralphglasses-discovery
description: Discover the live ralphglasses MCP contract, search the workflow and skill catalog, and load only the tool groups needed for the current task.
---

# Ralphglasses Discovery

Use this skill when you need to orient on the control-plane surface before doing work.

## Default workflow

1. Read the contract and catalogs first:
   - `ralph:///catalog/server`
   - `ralph:///catalog/tool-groups`
   - `ralph:///catalog/workflows`
   - `ralph:///catalog/skills`
2. Use discovery tools to avoid over-loading the surface:
   - `ralphglasses_server_health`
   - `ralphglasses_tool_groups`
   - `ralphglasses_load_tool_group`
3. When the task is fuzzy, search by intent instead of scanning every tool:
   - `ralphglasses_tool_groups` with `query`, `include_workflows`, or `include_skills`
4. Only after discovery, load the smallest set of tool groups needed for the task.

## Best-fit cases

- Contract discovery
- Tool-group selection
- Workflow routing
- Skill-family selection
- “What should I load/use next?” questions

## Guardrails

- Prefer catalog resources over guessing.
- Do not load broad tool groups if a smaller discovery path is enough.
- Treat `ralphglasses` as the exhaustive reference skill, not the default first read.
