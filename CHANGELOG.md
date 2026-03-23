# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

### Features

- feat: add awesome-claude-code research pipeline (5 MCP tools) ([62eaafb](../../commit/62eaafb88800317f5bb6b069819e9cd5386f03b2))
- feat: add Gemini & Codex project configs, multi-provider contribution guide ([5b4e3b3](../../commit/5b4e3b3d66ce6793cc0e5eaa6147d5ef9cfd7ab5))
- feat: persist session state to disk for cross-process visibility ([224a7f8](../../commit/224a7f8f5996aa7afcabbadf6d6ef9610493a18f))
- feat: dense TUI dashboard with Nerd Font icons, inline gauges, and sparklines ([2492acc](../../commit/2492acc157f2c1be1d18bac3fba546e680b87e5c))
- feat: add perpetual self-improvement loop with journal system ([f7a244e](../../commit/f7a244e8bf3815fa9c46ea75bfb01065dfe38f4d))
- feat: add TUI interactive components, views, and desktop notifications ([f86d3f2](../../commit/f86d3f2367afcdf7e3778bd4bd3b2c96ef0a568e))
- feat: wire event bus into infrastructure, add 11 new MCP tools ([12d091a](../../commit/12d091ae749967e8cf45fd5a2ff5ea4d94838f3b))
- feat: add hook system for event-driven shell command execution ([d97cu8b](../../commit/d97cu8b63574be3f335c5b45afb8e6bf9277768b))
- feat: add internal event bus with pub/sub and ring buffer history ([0204fc0](../../commit/0204fc09ee42cfe247270697857802bae504f8c1))
- feat: 4-tab TUI fleet monitoring with bubbles integration ([7ef277e](../../commit/7ef277e38d05857d64768ae81b1f327641f5ef30))
- feat: add ralphglasses_fleet_status MCP tool for fleet-wide monitoring ([06303ef](../../commit/06303ef046d7679553d4b87403b725509c648a24))
- feat: live multi-agent test readiness — env validation, delegation prompt, warnings ([105636b](../../commit/105636bc32a2f68afc88948b5b3c010bd1d63950))
- feat: multi-LLM orchestration — Claude, Gemini, and Codex session providers ([07a89e3](../../commit/07a89e39c73292c0a7663c1d47c9f4ce11e9ac30))
- feat: add Claude Code session orchestration via headless SDK (claude -p) ([acf8b59](../../commit/acf8b5961481824a2d13106263f60db3a7275197))

### Bug Fixes

- fix: sanitize provider stderr, thread-safe team delegate, session persistence tests ([6e64d5e](../../commit/6e64d5ee468726bbbd77baab6db79ad0eb3884d8))
- fix: prevent table row wrapping and broken filter/sort with ANSI content ([7850592](../../commit/78505923e033a03c97c35fba4041984bc379a869))
- fix: correct Gemini CLI flags (-p requires prompt value, --approval-mode yolo) ([d22749e](../../commit/d22749e02ffb1e95c6a5610b3a8bb600337b446d))
- fix: strip CLAUDECODE env var from child sessions to prevent nesting error ([f6977e9](../../commit/f6977e9541119700708fe592e52f03be0bada037))
- fix: remove invalid -n flag from Claude CLI command builder ([a06e004](../../commit/a06e0046f86e49adb47ea713a48a0afa50706aa9))
- fix: use ldflags version in MCP server, switch .mcp.json to go run ([510e16f](../../commit/510e16f66d48ea5b061cbfe76eec6d0a78a6600d))
- fix: wire hook system, add missing view cases, init HTTPClient, update docs ([1e49070](../../commit/1e49070182f72b01a99c51884b28c92ce4e85b16))
- fix: add --verbose flag to buildClaudeCmd for stream-json compatibility ([ba0dd63](../../commit/ba0dd63a1779603c5d04d71a96bb2cc5697b98b5))
- fix(watcher): propagate errors, add backoff and polling fallback ([a5591ad](../../commit/a5591ade0920739cac4371450402c12e244d4f19))
- fix(model): surface RefreshRepo parse errors instead of silently discarding ([96d26d5](../../commit/96d26d51e8a0184af5254399d0d5069ea3a9d8f6))
- fix(marathon): use bc for float duration/checkpoint arithmetic ([0916185](../../commit/0916185ae3d6b447f1ea9d61013922aec786a2b1))
- Fix all 13 backlog issues: watcher timeout, error propagation, portability, coverage ([09a29c7](../../commit/09a29c734c34d7047873e0d66b5e876099ed7e64))
- Fix marathon.sh: unset ANTHROPIC_API_KEY, add AGENT.md, restore configs ([0b19b70](../../commit/0b19b7028da0c892a09846989a17cae80b5e88e8))

### Other

- Add bootstrap tooling and codex loop control ([88f2ac1](../../commit/88f2ac10fb106c98b23d9a3c6d2d87a0e7d20d33))
- Implement seed dedup, breakglass criteria, and benchmark logging ([a14bb9c](../../commit/a14bb9cf7962ce27f71fbf81d034b2bbbfb7e083))
- Add PID file management for orphan detection and process recovery ([5740a81](../../commit/5740a811fa566815557ff74636fd29d28c8ae91e))
- refactor: extract handlers and fleet builder from app.go ([6755ee9](../../commit/6755ee95d9fe21e8cfaedbef01841d5f7d34345c))
- configure Ralph for Phase 0.5 automated dev cycles ([f77e94a](../../commit/f77e94a5f9883d06755b353d0814bd15521f0c06))
- Add 7 MCP tools: roadmap automation + repo file management ([2e11d03](../../commit/2e11d03377d9d0752e6d0a71e8fd8e1d52a0cbde))
- Expand ROADMAP.md to 440 items across 11 phases with 4 new phases ([9197d37](../../commit/9197d37044654d55bd0355c6ce69a3eb82ec5ec7))
- Expand ROADMAP.md to 190 subtasks with parallel/blocker annotations ([e538374](../../commit/e538374e7938b2025ffb93a1fd1d84e4de880bef))
- Add ProArt X870E hardware drivers, hw-detect, and driver parity fixes ([2ae1165](../../commit/2ae1165a3b36d49d36a33c176ba28e19699bfc33))
- Add cross-repo improvements: cost ledger, auto-restart, batch config ([673b6a7](../../commit/673b6a7d8dae68469cd9599246a01168e546c08f))
- Update config and docs for $100/12h marathon on new machine ([e2f9790](../../commit/e2f97903b8ba4ffcbe162424b107fd88dbfe1885))
- Expand test suite: 220 tests, fuzz, benchmarks, integration, BATS, CI overhaul ([836485e](../../commit/836485e96a4eaf69cedcb77b15f897a2bc69309c))
- Rewrite marathon.sh as a proper supervisor with budget/duration enforcement ([0b042cc](../../commit/0b042cc5242ec489e19b185e1a8609693aa3b0b4))
- Add bootable ISO build system, test suite, and CI pipeline ([65ce9a7](../../commit/65ce9a798edddec62efc43b0b8778865917c2597))
- Add roadmap, research docs, and thin client scaffold ([1ac3626](../../commit/1ac3626ca0c4a40b3771f6532bf016d1691fe680))
- Implement ralphglasses TUI + MCP server + marathon launcher ([6f60530](../../commit/6f60530d5f0c9018af4cd64fbbaaa11b77a02891))
- Initial commit ([94c98ff](../../commit/94c98ffef263c6d845c6dfc3ab852617c27d7416))
