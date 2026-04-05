# Contributing to hairglasses-studio

This project supports development with **Claude Code**, **Gemini CLI**, and **OpenAI Codex CLI**. Any provider can lead development.

## Development Setup

### 1. Clone and build

```bash
git clone https://github.com/hairglasses-studio/openai_cli
cd openai_cli
make build       # or: go build ./... / npm install / pip install -e .
make test        # or: go test ./... / npm test / pytest
```

### 2. Verify with the shared pipeline

```bash
make pipeline-check   # build + vet + test (via shared pipeline)
```

Or use the pipeline script directly:

```bash
~/hairglasses-studio/dotfiles/scripts/hg-pipeline.sh
```

## Making Changes

1. Create a branch: `git checkout -b feat/my-change`
2. Make your changes
3. Run the pipeline: `make pipeline-check`
4. Commit with a descriptive message
5. Push and open a PR

## Code Style

- **Go**: `gofmt` formatting, `go vet` clean, golangci-lint passing. Editor settings in `.editorconfig`.
- **Node.js**: ESLint/Prettier where configured
- **Python**: ruff/black formatting

## Pre-commit Hooks

Install with:

```bash
make install-hooks
```

Runs vet + fast tests before each commit (Go), lint (Node.js), or ruff (Python).

## CI

All PRs trigger CI automatically: lint, test, and build. See `.github/workflows/ci.yml`.

## New Repos

Use the scaffolding script:

```bash
~/hairglasses-studio/dotfiles/scripts/hg-new-repo.sh my-new-project go
```

Creates all standard files (Makefile, .editorconfig, .golangci.yml, CI, LICENSE, etc.) in one command.

## Questions?

Open an issue or tag `@hairglasses` in your PR.
