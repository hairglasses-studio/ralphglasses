#!/usr/bin/env python3
"""Generate provider-native role projections from .agents/roles/*.json.

This script is intentionally small and stdlib-only so it can run in any repo that
already depends on Python for lightweight tooling.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
ROLE_DIR = ROOT / ".agents" / "roles"
TARGETS = {
    "codex": ROOT / ".codex" / "agents",
    "claude": ROOT / ".claude" / "agents",
    "gemini": ROOT / ".gemini" / "agents",
}


def load_roles() -> list[dict]:
    roles = []
    for path in sorted(ROLE_DIR.glob("*.json")):
        roles.append(json.loads(path.read_text(encoding="utf-8")))
    return roles


def target_path_for(role: dict, provider: str) -> Path:
    override = role.get("provider_overrides", {}).get(provider, {})
    surface = override.get("surface")
    if surface:
        return ROOT / surface
    suffix = ".toml" if provider == "codex" else ".md"
    return TARGETS[provider] / f"{role['name']}{suffix}"


def merged_provider_config(role: dict, provider: str) -> dict:
    override = dict(role.get("provider_overrides", {}).get(provider, {}))
    path = target_path_for(role, provider)
    return {
        "role_name": role["name"],
        "name": path.stem,
        "provider": provider,
        "path": path,
        "description": override.get("description", role["summary"]),
        "prompt": override.get("prompt", role["prompt"]),
        "model": override.get("model"),
        "model_reasoning_effort": override.get("model_reasoning_effort"),
        "sandbox_mode": override.get("sandbox_mode"),
        "tools": override.get("tools") or [],
        "max_turns": override.get("max_turns"),
    }


def toml_escape(value: str) -> str:
    return value.replace("\\", "\\\\").replace('"', '\\"')


def render_codex(config: dict) -> str:
    lines = [
        f"# Generated from .agents/roles/{config['role_name']}.json",
        f'name = "{toml_escape(config["name"])}"',
        f'description = "{toml_escape(config["description"])}"',
    ]
    if config.get("model"):
        lines.append(f'model = "{toml_escape(config["model"])}"')
    if config.get("model_reasoning_effort"):
        lines.append(
            f'model_reasoning_effort = "{toml_escape(config["model_reasoning_effort"])}"'
        )
    if config.get("sandbox_mode"):
        lines.append(f'sandbox_mode = "{toml_escape(config["sandbox_mode"])}"')
    lines.extend(
        [
            "",
            'developer_instructions = """',
            config["prompt"].rstrip(),
            '"""',
            "",
        ]
    )
    return "\n".join(lines)


def render_markdown(config: dict) -> str:
    lines = [
        "---",
        f"name: {config['name']}",
        f"description: {config['description']}",
    ]
    if config.get("model"):
        lines.append(f"model: {config['model']}")
    if config.get("tools"):
        tools = ", ".join(config["tools"])
        lines.append(f"tools: [{tools}]")
    if config.get("max_turns"):
        # Gemini CLI uses maxTurns in its validation schema for Local Agents.
        lines.append(f"maxTurns: {config['max_turns']}")
    lines.append("---")
    # Move source tracking metadata to markdown comments to keep the frontmatter clean
    # and strictly compliant with Gemini CLI validation.
    lines.extend([
        "",
        f"<!-- source_manifest: .agents/roles/{config['role_name']}.json -->",
        f"<!-- provider: {config['provider']} -->",
        "",
        config["prompt"].rstrip(),
        "",
    ])
    return "\n".join(lines)


def expected_outputs(roles: list[dict]) -> dict[Path, str]:
    outputs: dict[Path, str] = {}
    for role in roles:
        for provider in TARGETS:
            config = merged_provider_config(role, provider)
            content = render_codex(config) if provider == "codex" else render_markdown(config)
            outputs[config["path"]] = content
    return outputs


def write_outputs(outputs: dict[Path, str]) -> None:
    for target in TARGETS.values():
        target.mkdir(parents=True, exist_ok=True)
    for path, content in outputs.items():
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8", newline="\n")


def check_outputs(outputs: dict[Path, str]) -> int:
    drift: list[str] = []
    for path, content in outputs.items():
        if not path.exists():
            drift.append(f"missing {path.relative_to(ROOT)}")
            continue
        actual = path.read_text(encoding="utf-8")
        if actual != content:
            drift.append(f"drift {path.relative_to(ROOT)}")
    if not drift:
        return 0
    print("provider role projections are out of date:", file=sys.stderr)
    for item in drift:
        print(f"- {item}", file=sys.stderr)
    print("run: python3 scripts/sync-provider-roles.py", file=sys.stderr)
    return 1


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="fail when checked-in projections drift")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    outputs = expected_outputs(load_roles())
    if args.check:
        return check_outputs(outputs)
    write_outputs(outputs)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
