#!/usr/bin/env python3
"""Generate provider-native role projections from .agents/roles/*.json.

This script is intentionally small and stdlib-only so it can run in any repo that
already depends on Python for lightweight tooling.
"""

from __future__ import annotations

import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
ROLE_DIR = ROOT / '.agents' / 'roles'
TARGETS = {
    'codex': ROOT / '.codex' / 'agents',
    'claude': ROOT / '.claude' / 'agents',
    'gemini': ROOT / '.gemini' / 'agents',
}


def load_roles() -> list[dict]:
    roles = []
    for path in sorted(ROLE_DIR.glob('*.json')):
        roles.append(json.loads(path.read_text()))
    return roles


def render_codex(role: dict) -> str:
    return (
        '# Generated from .agents/roles/{name}.json\n'
        'description = "{summary}"\n\n'
        'prompt = """\n{prompt}\n"""\n'
    ).format(name=role['name'], summary=role['summary'].replace('"', '\\"'), prompt=role['prompt'].rstrip())


def render_markdown(role: dict, provider: str) -> str:
    header = (
        '---\n'
        'description: {summary}\n'
        'source_manifest: .agents/roles/{name}.json\n'
        'provider: {provider}\n'
        '---\n\n'
    ).format(name=role['name'], summary=role['summary'], provider=provider)
    return header + role['prompt'].rstrip() + '\n'


def main() -> None:
    roles = load_roles()
    for target in TARGETS.values():
        target.mkdir(parents=True, exist_ok=True)
    for role in roles:
        (TARGETS['codex'] / f"{role['name']}.toml").write_text(render_codex(role))
        (TARGETS['claude'] / f"{role['name']}.md").write_text(render_markdown(role, 'claude'))
        (TARGETS['gemini'] / f"{role['name']}.md").write_text(render_markdown(role, 'gemini'))


if __name__ == '__main__':
    main()
