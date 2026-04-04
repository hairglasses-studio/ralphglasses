# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in ralphglasses, please report it responsibly.

**Do NOT open a public GitHub issue for security vulnerabilities.**

Instead, please email **security@hairglasses-studio.dev** with:

1. A description of the vulnerability
2. Steps to reproduce (if applicable)
3. The potential impact
4. Any suggested fixes

## Response Timeline

- **Acknowledgment**: Within 48 hours
- **Initial assessment**: Within 1 week
- **Fix or mitigation**: Depends on severity, typically within 2 weeks for critical issues

## Scope

This policy applies to the ralphglasses Go binary, MCP server, and all packages within the repository. Security issues in dependencies should be reported to the respective upstream projects.

## Supported Versions

| Version | Supported |
|---------|-----------|
| v0.1.x  | Yes       |

## Security Considerations

ralphglasses manages LLM provider API keys and spawns child processes:

- **API keys** are loaded from `.env` files and environment variables, never hardcoded or logged
- **Child processes** (Claude, Gemini, Codex CLIs) run with process group isolation and signal management
- **MCP server** uses stdio transport by default (no network exposure)
- **Cost tracking** data is stored locally in `.ralph/` directories
