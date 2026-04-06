# Non-Go Migration Map

Generated: 2026-04-05 by Research Agent 3
Policy: Copy latest code, delete originals. No git history preservation.

## Global Exclusion List (NEVER COPY)

All migrations exclude these patterns:

```
.git/
__pycache__/
*.pyc
.venv/
venv/
my_env/
node_modules/
.pytest_cache/
target/          # Rust
.env             # Secrets
.ralph/          # Ralph runtime state
.ralphrc         # Ralph runtime config
.claude/         # Claude session artifacts (except skills content)
.codex/          # Codex config
.gemini/         # Gemini config
.github/         # GH workflows (target repos have their own)
.editorconfig    # Target repos have their own
.envrc           # Target repos have their own
*.Zone.Identifier  # Windows zone ID files
```

## Global Conflict Resolution (per-file policy)

Files that appear in EVERY source repo and conflict with target repos:

| File | Policy |
|------|--------|
| `CLAUDE.md` | RENAME to `CLAUDE-<source-repo>.md` in destination subdir |
| `README.md` | RENAME to `README.md` inside destination subdir (OK, no conflict) |
| `AGENTS.md` | DROP (target repos have their own) |
| `GEMINI.md` | DROP (target repos have their own) |
| `CONTRIBUTING.md` | DROP (target repos have their own) |
| `LICENSE` | DROP (all MIT, target repos have their own) |
| `ROADMAP.md` | RENAME to `ROADMAP-<source-repo>.md` in destination subdir |
| `JOURNAL.md` | KEEP (rename to `JOURNAL.md` in subdir) |
| `.mcp.json` | DROP (target repos have their own) |
| `.gitignore` | MERGE relevant patterns into target's .gitignore |
| `Makefile` | KEEP only if meaningful (rename if conflict with target) |
| `pyproject.toml` | KEEP (in subdir, no conflict) |
| `requirements.txt` | KEEP (in subdir, no conflict) |

---

## Migration 1: [private] -> [private]

**Source:** `~/hairglasses-studio/[private]/`
**Target:** `~/hairglasses-studio/[private]/python/[private]/`
**Conflicts:** None ([private]/python/ does not exist)

### Copy command

```bash
mkdir -p ~/hairglasses-studio/[private]/python/[private]
rsync -av --exclude='.git/' --exclude='__pycache__/' --exclude='*.pyc' \
  --exclude='.venv/' --exclude='venv/' --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.claude/' --exclude='.codex/' --exclude='.gemini/' \
  --exclude='.github/' --exclude='.editorconfig' --exclude='.envrc' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' --exclude='CONTRIBUTING.md' \
  --exclude='LICENSE' --exclude='.mcp.json' --exclude='.env' \
  ~/hairglasses-studio/[private]/ \
  ~/hairglasses-studio/[private]/python/[private]/
```

### Destination layout

```
[private]/python/[private]/
  CLAUDE.md                  (keep as-is in subdir)
  README.md
  ROADMAP.md
  JOURNAL.md
  pyproject.toml
  uv.lock
  .python-version
  .gitignore                 -> MERGE into [private]/.gitignore
  data/                      (application data, cover letters, evidence)
    gdrive/
  resume/                    (PDFs, CSS, build scripts)
  src/[private]/              (Python MCP server code)
    ashby.py, config.py, greenhouse.py, models.py, prompts.py, ...
    services/                (achievement_extractor, ats_scorer, cache, etc.)
    tools/                   (compare, insights, interactive, interview, etc.)
    utils/
```

### .gitignore additions for [private]

```
# Python ([private], crabravee)
python/**/__pycache__/
python/**/*.pyc
python/**/.venv/
python/**/.pytest_cache/
python/**/uv.lock
```

### SENSITIVE DATA WARNING

[private]/data/ contains personal job search data (cover letters, salary info,
referral emails, resumes). [private] is already marked NEVER PUBLISH so this is fine.

---

## Migration 2: crabravee -> [private]

**Source:** `~/hairglasses-studio/crabravee/`
**Target:** `~/hairglasses-studio/[private]/python/crabravee/`
**Conflicts:** Near-identical data/ and resume/ dirs as [private] (same files, likely newer versions in crabravee)

### Copy command

```bash
mkdir -p ~/hairglasses-studio/[private]/python/crabravee
rsync -av --exclude='.git/' --exclude='__pycache__/' --exclude='*.pyc' \
  --exclude='.venv/' --exclude='venv/' --exclude='.pytest_cache/' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.claude/' --exclude='.codex/' --exclude='.gemini/' \
  --exclude='.github/' --exclude='.editorconfig' --exclude='.envrc' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' --exclude='CONTRIBUTING.md' \
  --exclude='LICENSE' --exclude='.mcp.json' --exclude='.env' \
  --exclude='.claude-audit-report.md' --exclude='.claude-fix-report.md' \
  ~/hairglasses-studio/crabravee/ \
  ~/hairglasses-studio/[private]/python/crabravee/
```

### Destination layout

```
[private]/python/crabravee/
  CLAUDE.md
  README.md
  ROADMAP.md
  JOURNAL.md
  pyproject.toml
  uv.lock
  .python-version
  data/                      (same structure as [private], likely newer)
  resume/
  src/[private]/              (same package name as [private])
    services/
    tools/
    utils/
  tests/                     (test_achievement_extractor, test_models, etc.)
```

### Conflict note

[private] and crabravee have overlapping data/ and resume/ dirs with identical
filenames. Keep BOTH under separate subdirs. crabravee appears to be the
evolved version with tests/ dir and audit reports.

---

## Migration 3: openai_cli -> ralphglasses

**Source:** `~/hairglasses-studio/openai_cli/`
**Target:** `~/hairglasses-studio/ralphglasses/python/openai_cli/`
**Conflicts:** None (ralphglasses/python/ does not exist)

### Copy command

```bash
mkdir -p ~/hairglasses-studio/ralphglasses/python/openai_cli
rsync -av --exclude='.git/' --exclude='__pycache__/' --exclude='*.pyc' \
  --exclude='venv/' --exclude='my_env/' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.github/' --exclude='.editorconfig' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' --exclude='CONTRIBUTING.md' \
  --exclude='LICENSE' --exclude='.env' \
  --exclude='get-pip.py' \
  ~/hairglasses-studio/openai_cli/ \
  ~/hairglasses-studio/ralphglasses/python/openai_cli/
```

### CRITICAL WARNINGS

1. **EXCLUDE .env** -- openai_cli/.env exists (583 bytes) and contains secrets
2. **EXCLUDE venv/** -- 539MB virtual environment
3. **EXCLUDE my_env/** -- 26MB environment
4. **EXCLUDE get-pip.py** -- vendored pip installer, not needed

### Destination layout

```
ralphglasses/python/openai_cli/
  CLAUDE.md
  README.md
  ROADMAP.md
  CHANGELOG.md
  EXPLAINER.md
  OPTIMIZATION_SUMMARY.md
  chat_scraper.py
  config.py
  logging_config.py
  media_scraper.py
  oai_cli.py
  sora_downloader.py
  sora_archive_schema.sql
  requirements.txt
  pytest.ini
  docker-compose.yml
  Dockerfile
  crontab.txt
  apply_patch.sh
  backup_to_unraid.sh
  install.sh
  start.sh
  start_web.sh
  scripts/setup_dev.sh
  db/
    database.py
    insert_image_metadata.py
  monitoring/health_check.py
  tests/
  web/
    app.py
    templates/ (404.html, base.html, dashboard.html, error.html)
```

### .gitignore additions for ralphglasses

```
# Python (openai_cli)
python/**/__pycache__/
python/**/*.pyc
python/**/.venv/
python/**/venv/
python/**/my_env/
python/**/.env
python/**/*.sqlite
```

---

## Migration 4: sam3-video-segmenter -> art-mcp

**Source:** `~/hairglasses-studio/sam3-video-segmenter/`
**Target:** `~/hairglasses-studio/art-mcp/sam3-segmenter/` (ALREADY EXISTS)
**Conflicts:** MAJOR -- art-mcp/sam3-segmenter/ already contains the Python code + Go wrapper

### STATUS: ALREADY MIGRATED

art-mcp/sam3-segmenter/ already contains:
- sam3_segmenter/ Python package (cli.py, config.py, segmenter.py, utils.py, video_io.py)
- Go wrapper (cmd/, internal/)
- pyproject.toml

**ACTION:** Only copy missing files from source that art-mcp lacks:

```bash
# Copy docs, examples, tests, scripts that may not have been brought over
rsync -av --ignore-existing \
  --exclude='.git/' --exclude='__pycache__/' --exclude='*.pyc' \
  --exclude='.venv/' --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.claude/' --exclude='.codex/' --exclude='.gemini/' \
  --exclude='.github/' --exclude='.editorconfig' --exclude='.envrc' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' --exclude='CONTRIBUTING.md' \
  --exclude='LICENSE' --exclude='.env' --exclude='.env.example' \
  --exclude='CLAUDE.local.md.template' \
  --exclude='sam3_segmenter/'  \
  ~/hairglasses-studio/sam3-video-segmenter/ \
  ~/hairglasses-studio/art-mcp/sam3-segmenter/
```

Files to verify exist in target: docs/, examples/, tests/, scripts/

---

## Migration 5: procgen-videoclip -> art-mcp

**Source:** `~/hairglasses-studio/procgen-videoclip/`
**Target:** `~/hairglasses-studio/art-mcp/procgen-visual/` (ALREADY EXISTS)
**Conflicts:** MAJOR -- art-mcp/procgen-visual/ already has Python + Go wrapper

### STATUS: ALREADY MIGRATED (mostly)

art-mcp/procgen-visual/ has Go wrapper + scripts/. Source has extra files
not in art-mcp: wallpapers/, logos/, archive/, README_CATEGORIES.md,
SETUP_COMPLETE.md, *.sh install/setup scripts.

**ACTION:** Copy missing assets only:

```bash
rsync -av --ignore-existing \
  --exclude='.git/' --exclude='__pycache__/' --exclude='*.pyc' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.claude/' --exclude='.codex/' --exclude='.gemini/' \
  --exclude='.github/' --exclude='.editorconfig' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' --exclude='CONTRIBUTING.md' \
  --exclude='LICENSE' --exclude='ROADMAP.md' \
  ~/hairglasses-studio/procgen-videoclip/ \
  ~/hairglasses-studio/art-mcp/procgen-visual/
```

NOTE: wallpapers/ and logos/ may be large binary assets. Consider whether
they belong in git or should be excluded.

---

## Migration 6: wallpaper-procgen -> art-mcp

**Source:** `~/hairglasses-studio/wallpaper-procgen/`
**Target:** `~/hairglasses-studio/art-mcp/procgen-visual/` (MERGE with #5)
**Conflicts:** Overlapping filenames with procgen-videoclip

### STATUS: PARTIAL OVERLAP

wallpaper-procgen is a SUBSET of procgen-videoclip (same generate_wallpaper.py,
generate_wallpapers_multi.py, etc.) but lacks the extra generator scripts.
art-mcp/procgen-visual/ already has the consolidated version.

**ACTION:** Verify no unique files remain, then mark as SKIP:

```bash
# Diff to find files in wallpaper-procgen not in art-mcp/procgen-visual
diff <(cd ~/hairglasses-studio/wallpaper-procgen && find . -not -path '*/.git/*' \
  -not -path '*/__pycache__/*' -not -name '*.pyc' -type f | sort) \
  <(cd ~/hairglasses-studio/art-mcp/procgen-visual && find . -not -path '*/.git/*' \
  -not -path '*/__pycache__/*' -not -name '*.pyc' -type f | sort)
```

If all content is covered by procgen-videoclip migration, skip this entirely.

---

## Migration 7: video-ai-toolkit -> art-mcp

**Source:** `~/hairglasses-studio/video-ai-toolkit/`
**Target:** `~/hairglasses-studio/art-mcp/video-toolkit/` (ALREADY EXISTS)
**Conflicts:** MAJOR -- art-mcp/video-toolkit/ already has Python + Go wrapper

### STATUS: ALREADY MIGRATED

art-mcp/video-toolkit/ has the full video_toolkit/ Python package.

**ACTION:** Copy missing supplemental files only:

```bash
rsync -av --ignore-existing \
  --exclude='.git/' --exclude='__pycache__/' --exclude='*.pyc' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.claude/' --exclude='.codex/' --exclude='.gemini/' \
  --exclude='.github/' --exclude='.editorconfig' --exclude='.envrc' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' --exclude='CONTRIBUTING.md' \
  --exclude='LICENSE' --exclude='.env' --exclude='.env.example' \
  --exclude='video_toolkit/' \
  ~/hairglasses-studio/video-ai-toolkit/ \
  ~/hairglasses-studio/art-mcp/video-toolkit/
```

Files to verify: examples/, scripts/, tests/

---

## Migration 8: github-stars-catalog -> docs

**Source:** `~/hairglasses-studio/github-stars-catalog/`
**Target:** `~/hairglasses-studio/docs/tools/github-stars-catalog/`
**Conflicts:** None (docs/tools/ does not exist)

### Copy command

```bash
mkdir -p ~/hairglasses-studio/docs/tools/github-stars-catalog
rsync -av --exclude='.git/' --exclude='__pycache__/' --exclude='*.pyc' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.github/' --exclude='.editorconfig' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' --exclude='CONTRIBUTING.md' \
  --exclude='LICENSE' --exclude='ROADMAP.md' \
  --exclude='*.Zone.Identifier' \
  ~/hairglasses-studio/github-stars-catalog/ \
  ~/hairglasses-studio/docs/tools/github-stars-catalog/
```

### Destination layout

```
docs/tools/github-stars-catalog/
  CLAUDE.md
  README.md
  requirements.txt
  scripts/fetch_and_render.py
  templates/
    readme_section.j2
    readme_section.language.j2
    readme_section.topic.j2
```

### .gitignore additions for docs

```
# Python tools
tools/**/__pycache__/
tools/**/*.pyc
```

---

## Migration 9: sway-mcp -> dotfiles

**Source:** `~/hairglasses-studio/sway-mcp/`
**Target:** `~/hairglasses-studio/dotfiles/mcp/sway/`
**Conflicts:** None (dotfiles/mcp/ does not exist)

### Copy command

```bash
mkdir -p ~/hairglasses-studio/dotfiles/mcp/sway
rsync -av --exclude='.git/' --exclude='node_modules/' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.github/' --exclude='.editorconfig' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' --exclude='CONTRIBUTING.md' \
  --exclude='LICENSE' --exclude='ROADMAP.md' \
  ~/hairglasses-studio/sway-mcp/ \
  ~/hairglasses-studio/dotfiles/mcp/sway/
```

### Destination layout

```
dotfiles/mcp/sway/
  CLAUDE.md
  README.md
  package.json
  package-lock.json
  src/
    index.js
    tools/
      clipboard.js
      display.js
      input.js
      screenshot.js
      windows.js
```

### .gitignore additions for dotfiles

```
# JS MCP servers
mcp/**/node_modules/
```

---

## Migration 10: mac-mcp -> dotfiles

**Source:** `~/hairglasses-studio/mac-mcp/`
**Target:** `~/hairglasses-studio/dotfiles/mcp/mac/`
**Conflicts:** None

### Copy command

```bash
mkdir -p ~/hairglasses-studio/dotfiles/mcp/mac
rsync -av --exclude='.git/' --exclude='node_modules/' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.github/' --exclude='.editorconfig' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' --exclude='CONTRIBUTING.md' \
  --exclude='LICENSE' --exclude='ROADMAP.md' \
  ~/hairglasses-studio/mac-mcp/ \
  ~/hairglasses-studio/dotfiles/mcp/mac/
```

### Destination layout

```
dotfiles/mcp/mac/
  CLAUDE.md
  README.md
  package.json
  package-lock.json
  src/index.js
```

---

## Migration 11: allthelinks -> dotfiles

**Source:** `~/hairglasses-studio/allthelinks/`
**Target:** `~/hairglasses-studio/dotfiles/web/allthelinks/`
**Conflicts:** None (dotfiles/web/ does not exist)

Recommendation: Use dotfiles/web/ rather than dotfiles/mcp/ since this is a
web app (index.html + app.js), not an MCP server.

### Copy command

```bash
mkdir -p ~/hairglasses-studio/dotfiles/web/allthelinks
rsync -av --exclude='.git/' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.github/' --exclude='.editorconfig' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' --exclude='CONTRIBUTING.md' \
  --exclude='LICENSE' --exclude='ROADMAP.md' \
  ~/hairglasses-studio/allthelinks/ \
  ~/hairglasses-studio/dotfiles/web/allthelinks/
```

### Destination layout

```
dotfiles/web/allthelinks/
  CLAUDE.md
  README.md
  PLAN.md
  RESEARCH.md
  app.js
  index.html
  manifest.json
  schema.json
```

---

## Migration 12: open-multi-agent -> mcpkit

**Source:** `~/hairglasses-studio/open-multi-agent/`
**Target:** `~/hairglasses-studio/mcpkit/js/open-multi-agent/`
**Conflicts:** None (mcpkit/js/ does not exist)

### Copy command

```bash
mkdir -p ~/hairglasses-studio/mcpkit/js/open-multi-agent
rsync -av --exclude='.git/' --exclude='node_modules/' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.github/' --exclude='.editorconfig' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' --exclude='CONTRIBUTING.md' \
  --exclude='LICENSE' --exclude='ROADMAP.md' \
  ~/hairglasses-studio/open-multi-agent/ \
  ~/hairglasses-studio/mcpkit/js/open-multi-agent/
```

### Destination layout

```
mcpkit/js/open-multi-agent/
  CLAUDE.md
  README.md
  package.json
  package-lock.json
  tsconfig.json
  examples/
    01-single-agent.ts
    02-team-collaboration.ts
    03-task-pipeline.ts
    04-multi-model-team.ts
  src/
    index.ts
    types.ts
    agent/       (agent.ts, pool.ts, runner.ts)
    llm/         (adapter.ts, anthropic.ts, openai.ts)
    memory/      (shared.ts, store.ts)
    orchestrator/ (orchestrator.ts, scheduler.ts)
    task/        (queue.ts, task.ts)
    team/        (messaging.ts, team.ts)
    tool/        (executor.ts, framework.ts, built-in/)
    utils/       (semaphore.ts)
```

### .gitignore additions for mcpkit

```
# JS/TS (open-multi-agent)
js/**/node_modules/
js/**/dist/
js/**/*.js.map
```

---

## Migration 13: makima -> dotfiles

**Source:** `~/hairglasses-studio/makima/`
**Target:** `~/hairglasses-studio/dotfiles/makima/src/` (CONFLICT: dotfiles/makima/ exists with config TOMLs)
**Conflicts:** YES -- dotfiles/makima/ already contains makima config TOML files

### Conflict resolution

dotfiles/makima/ has device config TOMLs (MX Master 4, Xbox Controller).
The Rust source goes into dotfiles/makima/src/ to coexist:

```bash
rsync -av --exclude='.git/' --exclude='target/' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.github/' --exclude='.gitignore' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' \
  --exclude='LICENSE' \
  ~/hairglasses-studio/makima/ \
  ~/hairglasses-studio/dotfiles/makima/upstream/
```

### Destination layout

```
dotfiles/makima/
  # EXISTING config files (DO NOT OVERWRITE):
  examples/
  Microsoft Xbox Series S|X Controller.toml
  MX Master 4 Mouse.toml
  MX Master 4 Mouse::com.mitchellh.ghostty.toml
  ... (other .toml configs)

  # NEW from makima repo:
  upstream/
    CLAUDE.md
    README.md
    Cargo.toml
    Cargo.lock
    50-makima.rules
    makima.service
    install.sh
    examples/             (config-keyboard.toml, config-xbox.toml, etc.)
    src/
      main.rs
      active_client.rs
      config.rs
      event_reader.rs
      udev_monitor.rs
      virtual_devices.rs
```

NOTE: dotfiles/systemd/makima.service already exists. The source's
makima.service should be placed in upstream/ as reference only.

### .gitignore additions for dotfiles

```
# Rust (makima)
makima/upstream/target/
```

---

## Migration 14: dotfiles-arch -> dotfiles

**Source:** `~/hairglasses-studio/dotfiles-arch/`
**Target:** `~/hairglasses-studio/dotfiles/arch/`
**Conflicts:** None (dotfiles/arch/ does not exist)

### Copy command

```bash
mkdir -p ~/hairglasses-studio/dotfiles/arch
rsync -av --exclude='.git/' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.github/' --exclude='.editorconfig' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' --exclude='CONTRIBUTING.md' \
  --exclude='LICENSE' \
  ~/hairglasses-studio/dotfiles-arch/ \
  ~/hairglasses-studio/dotfiles/arch/
```

### Destination layout

```
dotfiles/arch/
  CLAUDE.md
  README.md
  ROADMAP.md
  INSTALL.md
  CHANGELOG.md
  COMMIT_MESSAGE.md
  SUMMARY.md
  UPDATE_SUMMARY.md
  install.sh
  setup.sh
  init-repo.sh
  backup/backup-configs.sh
  configs/
    electron-flags.conf
    starship-presets/
    zram-generator.conf
  docs/
    APP_RECOMMENDATIONS.md
    MIGRATION_GUIDE.md
    QUICK_REFERENCE.md
    TROUBLESHOOTING.md
  lists/
    development.txt
    essential.txt
    media.txt
    productivity.txt
  scripts/
    01-base-system.sh through 07-shell-enhancements.sh
    arch-installer.sh
    arch-extended-installer.sh
    helpers/
      configure-services.sh
      optimize-electron.sh
```

---

## Migration 15: luke-toolkit -> dotfiles

**Source:** `~/hairglasses-studio/luke-toolkit/`
**Target:** `~/hairglasses-studio/dotfiles/tools/luke-toolkit/`
**Conflicts:** None (dotfiles/tools/ does not exist)

### Copy command

```bash
mkdir -p ~/hairglasses-studio/dotfiles/tools/luke-toolkit
rsync -av --exclude='.git/' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.gemini/' --exclude='.github/' --exclude='.editorconfig' \
  --exclude='.envrc' --exclude='.gitignore' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' --exclude='CONTRIBUTING.md' \
  --exclude='LICENSE' \
  ~/hairglasses-studio/luke-toolkit/ \
  ~/hairglasses-studio/dotfiles/tools/luke-toolkit/
```

### Destination layout

```
dotfiles/tools/luke-toolkit/
  CLAUDE.md
  README.md
  ROADMAP.md
  configs/
    midi-defaults.yaml
    osc-defaults.yaml
  tools/
    automation/session.sh
    music/setlist.sh
    touchdesigner/
    video/convert.sh
  tutorials/
    00-terminal-basics.md through 17-realtime-av-sync.md
```

---

## Migration 16: caper-bush -> dotfiles

**Source:** `~/hairglasses-studio/caper-bush/`
**Target:** `~/hairglasses-studio/dotfiles/zsh/plugins/caper-bush/`
**Conflicts:** None (dotfiles/zsh/ exists with config files, but no plugins/ subdir)

### Copy command

```bash
mkdir -p ~/hairglasses-studio/dotfiles/zsh/plugins/caper-bush
rsync -av --exclude='.git/' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.github/' --exclude='.editorconfig' --exclude='.gitignore' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' --exclude='CONTRIBUTING.md' \
  --exclude='LICENSE' --exclude='LICENSE.md' \
  ~/hairglasses-studio/caper-bush/ \
  ~/hairglasses-studio/dotfiles/zsh/plugins/caper-bush/
```

### Destination layout

```
dotfiles/zsh/plugins/caper-bush/
  CLAUDE.md
  README.md
  ROADMAP.md
  caper-bush.plugin.zsh
```

---

## Migration 17: tmux-ssh-syncing -> dotfiles

**Source:** `~/hairglasses-studio/tmux-ssh-syncing/`
**Target:** `~/hairglasses-studio/dotfiles/zsh/plugins/tmux-ssh-syncing/`
**Conflicts:** None

### Copy command

```bash
mkdir -p ~/hairglasses-studio/dotfiles/zsh/plugins/tmux-ssh-syncing
rsync -av --exclude='.git/' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.github/' --exclude='.editorconfig' --exclude='.gitignore' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' --exclude='CONTRIBUTING.md' \
  --exclude='LICENSE' \
  ~/hairglasses-studio/tmux-ssh-syncing/ \
  ~/hairglasses-studio/dotfiles/zsh/plugins/tmux-ssh-syncing/
```

### Destination layout

```
dotfiles/zsh/plugins/tmux-ssh-syncing/
  CLAUDE.md
  README.md
  ROADMAP.md
  tmux-ssh-syncing.plugin.zsh
  src/ssh.zsh
  doc/
    screencast.gif
    screencast.mp4
    screencast.png
```

---

## Migration 18: visual-projects -> art-mcp

**Source:** `~/hairglasses-studio/visual-projects/`
**Target:** `~/hairglasses-studio/art-mcp/visual/`
**Conflicts:** None (art-mcp/visual/ does not exist)

### Copy command

```bash
mkdir -p ~/hairglasses-studio/art-mcp/visual
rsync -av --exclude='.git/' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.codex/' --exclude='.gemini/' \
  --exclude='.github/' --exclude='.editorconfig' --exclude='.envrc' \
  --exclude='.gitignore' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' \
  --exclude='LICENSE' \
  ~/hairglasses-studio/visual-projects/ \
  ~/hairglasses-studio/art-mcp/visual/
```

### Destination layout

```
art-mcp/visual/
  CLAUDE.md
  README.md
  Architecture.md
  Roadmap.md
  ROADMAP.md          (NOTE: two roadmap files exist, different casing)
  resolume/
    compositions/
      BOC NYD.avc
      Example.avc
      LUKE_2.18.avc through LUKE_2.21.avc
  touchdesigner/
    components/colour_lovers_picker_1.tox
    projects/
      DEAD435X_autobuild.4.toe
      ... (14 .toe project files)
```

### .gitignore additions for art-mcp

```
# TouchDesigner/Resolume binary projects
# (These are binary but intentionally tracked)
```

NOTE: .toe and .avc files are binary. Consider Git LFS if total size is large.

---

## Migration 19: opnsense-monolith -> docs

**Source:** `~/hairglasses-studio/opnsense-monolith/`
**Target:** `~/hairglasses-studio/docs/infrastructure/opnsense/`
**Conflicts:** None (docs/infrastructure/ does not exist)

### Copy command

```bash
mkdir -p ~/hairglasses-studio/docs/infrastructure/opnsense
rsync -av --exclude='.git/' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.codex/' --exclude='.gemini/' \
  --exclude='.github/' --exclude='.editorconfig' --exclude='.envrc' \
  --exclude='.pre-commit-config.yaml' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' \
  --exclude='LICENSE' \
  ~/hairglasses-studio/opnsense-monolith/ \
  ~/hairglasses-studio/docs/infrastructure/opnsense/
```

### Destination layout

```
docs/infrastructure/opnsense/
  CLAUDE.md
  README.md
  Architecture.md
  Roadmap.md
  ROADMAP.md
  CHANGELOG.md
  IP_MIGRATION_REPORT.md
  MASTER_INDEX.md
  centralization-tools/
    coordinate_topology.sh
    setup_monitoring.sh
    sync_configs.sh
  helper-scripts/
    backup_all_configs.sh
    coordinate_apis.sh
    fix_ip_migration.sh
    test_network_integration.sh
  opnsense-wiki/
    automation/
    configurations/
    llm-agents/
    networking/
  quick-reference/quick-start.md
```

---

## Migration 20: unraid-monolith -> docs

**Source:** `~/hairglasses-studio/unraid-monolith/`
**Target:** `~/hairglasses-studio/docs/infrastructure/unraid/`
**Conflicts:** None

### Copy command

```bash
mkdir -p ~/hairglasses-studio/docs/infrastructure/unraid
rsync -av --exclude='.git/' --exclude='__pycache__/' --exclude='*.pyc' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.codex/' --exclude='.gemini/' \
  --exclude='.github/' --exclude='.editorconfig' --exclude='.envrc' \
  --exclude='*.Zone.Identifier' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' --exclude='CONSOLIDATION_PLAN.md' \
  --exclude='LICENSE' \
  ~/hairglasses-studio/unraid-monolith/ \
  ~/hairglasses-studio/docs/infrastructure/unraid/
```

### IMPORTANT: Exclude Zone.Identifier files

The unraid-monolith repo has numerous `*.Zone.Identifier` files (Windows
Alternate Data Stream markers). These MUST be excluded.

### Destination layout

```
docs/infrastructure/unraid/
  CLAUDE.md
  README.md
  Architecture.md
  Roadmap.md
  ROADMAP.md
  CHANGELOG.md
  MIGRATION_SUMMARY.md
  agents/
    llmagent-unraid/     (Python MCP agent for Unraid)
    unraid-llm-agent/    (Python CLI tool)
    unraid_llm_agent/
  community/lanjelin/    (community scripts)
  config-backup/
  docs/
  monitoring/
  plugins/
  scripts/
```

---

## Migration 21: open-sourcing-research -> docs

**Source:** `~/hairglasses-studio/open-sourcing-research/`
**Target:** `~/hairglasses-studio/docs/strategy/open-sourcing/`
**Conflicts:** None (docs/strategy/open-sourcing/ does not exist)

### Copy command

```bash
mkdir -p ~/hairglasses-studio/docs/strategy/open-sourcing
rsync -av --exclude='.git/' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.github/' --exclude='.gitignore' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' \
  --exclude='LICENSE' \
  ~/hairglasses-studio/open-sourcing-research/ \
  ~/hairglasses-studio/docs/strategy/open-sourcing/
```

### Destination layout

```
docs/strategy/open-sourcing/
  CLAUDE.md
  README.md
  ROADMAP.md
  strategy.md
  competitive-analysis/
    claudekit.md, cr8-cli.md, dotfiles.md, hg-mcp.md,
    mcpkit.md, prompt-improver.md, ralphglasses.md,
    small-mcp-servers.md, summary.md
  readiness-audits/
    claudekit.md, cr8-cli.md, dotfiles.md, hg-mcp.md,
    mcpkit.md, ralphglasses.md
  scoring/
    all-repos.md, mcpkit-baseline.md
  tools/
    oss-best-practices.md, oss-scoring-spec.md
```

---

## Migration 22: dj-archive -> docs

**Source:** `~/hairglasses-studio/dj-archive/`
**Target:** `~/hairglasses-studio/docs/infrastructure/dj-archive/`
**Conflicts:** None

### Copy command

```bash
mkdir -p ~/hairglasses-studio/docs/infrastructure/dj-archive
rsync -av --exclude='.git/' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.codex/' --exclude='.gemini/' \
  --exclude='.github/' --exclude='.editorconfig' --exclude='.envrc' \
  --exclude='.pre-commit-config.yaml' --exclude='.gitignore' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' \
  --exclude='LICENSE' \
  ~/hairglasses-studio/dj-archive/ \
  ~/hairglasses-studio/docs/infrastructure/dj-archive/
```

### Destination layout

```
docs/infrastructure/dj-archive/
  CLAUDE.md
  README.md
  ROADMAP.md
  CHANGELOG.md
  docs/
    COST-ANALYSIS.md
    FORMAT-GUIDE.md
    MIGRATION-GUIDE.md
    RESEARCH.md
  scripts/
    cost-report.sh, export-metadata.sh, list-archive.sh,
    restore-files.sh, sync-to-s3.sh, validate-audio.sh
  terraform/
    backend.tf, main.tf, outputs.tf, variables.tf
```

### .gitignore additions for docs

```
# Terraform state
infrastructure/**/.terraform/
infrastructure/**/*.tfstate
infrastructure/**/*.tfstate.backup
```

---

## Migration 23: vj-archive -> docs

**Source:** `~/hairglasses-studio/vj-archive/`
**Target:** `~/hairglasses-studio/docs/infrastructure/vj-archive/`
**Conflicts:** None

### Copy command

```bash
mkdir -p ~/hairglasses-studio/docs/infrastructure/vj-archive
rsync -av --exclude='.git/' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.codex/' --exclude='.gemini/' \
  --exclude='.github/' --exclude='.editorconfig' --exclude='.envrc' \
  --exclude='.pre-commit-config.yaml' --exclude='.gitignore' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' \
  --exclude='LICENSE' \
  ~/hairglasses-studio/vj-archive/ \
  ~/hairglasses-studio/docs/infrastructure/vj-archive/
```

### Destination layout

```
docs/infrastructure/vj-archive/
  CLAUDE.md
  README.md
  ROADMAP.md
  CHANGELOG.md
  docs/
    COST-ANALYSIS.md, DXV-WORKFLOW.md, MIGRATION-GUIDE.md,
    RESEARCH.md, TROUBLESHOOTING.md
  scripts/
    cost-report.sh, download-from-s3.sh, list-archive.sh,
    restore-files.sh, storage-report.sh, sync-to-s3.sh,
    verify-integrity.sh
  terraform/
    backend.tf, main.tf, outputs.tf, variables.tf
```

---

## Migration 24: whiteclaw -> docs

**Source:** `~/hairglasses-studio/whiteclaw/`
**Target:** `~/hairglasses-studio/docs/research/claude-code-analysis/`
**Conflicts:** None

### CRITICAL: Size and exclusion management

Total whiteclaw size breakdown:
- claude-code-sourcemap/: 183MB (includes .tgz, vendor binaries, restored source)
- archives/: 37MB (zip files of git histories)
- claude-code-source/: 34MB
- claw-code/: 6.8MB
- sanbuphy-claude-code-source-code/: 316KB
- analysis/: 36KB
- docs/: 84KB
- scripts/: 20KB
- *.map files: 228MB total (4 files)

EXCLUDE: sourcemaps (*.map), vendor binaries (.node, rg, rg.exe),
.tgz archives, zip archives.

### Copy command

```bash
mkdir -p ~/hairglasses-studio/docs/research/claude-code-analysis
rsync -av --exclude='.git/' --exclude='node_modules/' \
  --exclude='*.map' --exclude='*.tgz' \
  --exclude='*.zip' \
  --exclude='vendor/' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.github/' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' \
  --exclude='LICENSE' --exclude='ROADMAP.md' \
  ~/hairglasses-studio/whiteclaw/ \
  ~/hairglasses-studio/docs/research/claude-code-analysis/
```

### Size after exclusions

Excluding *.map (228MB), *.tgz, *.zip (37MB), vendor/ (~50MB), the
remaining content should be approximately 40-50MB. The bulk is the
restored TypeScript source code which is the analysis value.

### Destination layout

```
docs/research/claude-code-analysis/
  README.md
  CLAUDE.md
  analysis/
    dependency-diff.json, inventory.json, overlap-matrix.json,
    source-diff-summary.md, tree-comparison.md
  claude-code-source/     (bun.lock, LICENSE.md)
  claude-code-source-code/ (README.md)
  claude-code-sourcemap/
    README.md
    extract-sources.js
    package/ (cli.js, LICENSE.md, package.json, sdk-tools.d.ts)
    restored-src/src/     (the actual analysis value: TS source files)
  claw-code/
  docs/
  sanbuphy-claude-code-source-code/
  scripts/
```

### .gitignore additions for docs

```
# Whiteclaw exclusions
research/claude-code-analysis/**/*.map
research/claude-code-analysis/**/*.tgz
research/claude-code-analysis/**/vendor/
research/claude-code-analysis/archives/
```

---

## Migration 25: claude-skills -> ralphglasses

**Source:** `~/hairglasses-studio/claude-skills/`
**Target:** `~/hairglasses-studio/ralphglasses/skills/`
**Conflicts:** ralphglasses/.claude/skills/ exists (different content: sweep/monitor skills)

### Conflict resolution

ralphglasses/.claude/skills/ has operational skills (audit-sweep, fix-sweep,
monitor-supervisor, parallel-roadmap-sprint-*). The claude-skills repo has
SKILL.md reference files and zip bundles. These are complementary, not conflicting.

Target: ralphglasses/skills/ (new dir, separate from .claude/skills/)

### Copy command

```bash
mkdir -p ~/hairglasses-studio/ralphglasses/skills
rsync -av --exclude='.git/' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.github/' --exclude='.gitignore' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' \
  --exclude='LICENSE' --exclude='ROADMAP.md' \
  ~/hairglasses-studio/claude-skills/ \
  ~/hairglasses-studio/ralphglasses/skills/
```

### Destination layout

```
ralphglasses/skills/
  CLAUDE.md
  README.md
  upload-skills.sh
  go-conventions/SKILL.md
  go-conventions.zip
  hairglasses-infra/SKILL.md
  hairglasses-infra.zip
  mcpkit-go/SKILL.md
  mcpkit-go.zip
  mcp-tool-scaffold/SKILL.md
  mcp-tool-scaffold.zip
  ralphglasses-ops/SKILL.md
  ralphglasses-ops.zip
  sway-rice/SKILL.md
  sway-rice.zip
```

---

## Migration 26: archlet -> ralphglasses

**Source:** `~/hairglasses-studio/archlet/`
**Target:** `~/hairglasses-studio/ralphglasses/distro/archlet/`
**Conflicts:** None (ralphglasses/distro/ exists but has no archlet/ subdir)

### Copy command

```bash
mkdir -p ~/hairglasses-studio/ralphglasses/distro/archlet
rsync -av --exclude='.git/' \
  --exclude='.ralph/' --exclude='.ralphrc' \
  --exclude='.github/' --exclude='.gitignore' \
  --exclude='AGENTS.md' --exclude='GEMINI.md' \
  --exclude='LICENSE' --exclude='ROADMAP.md' \
  ~/hairglasses-studio/archlet/ \
  ~/hairglasses-studio/ralphglasses/distro/archlet/
```

### Destination layout

```
ralphglasses/distro/archlet/
  CLAUDE.md
  README.md
  refind-test/
    esp-test.img
    ovmf_vars.fd
    ovmf_vars_test.fd
    ovmf_vars_vnc.fd
    refind.conf.fixed
    refind-emu.sh
    theme/
      build/        (PNGs: backgrounds, icons, selections)
      build.sh
      deploy.sh
      generate-background.py
      generate-font.py
      generate-icons.py
      generate-selections.py
      palette.sh
      sources/
      theme.conf
```

NOTE: Contains UEFI/OVMF binary images (.fd, .img). Consider Git LFS.

---

## Summary: .gitignore Additions by Target Repo

### [private]/.gitignore (add)

```
# Python ([private], crabravee)
python/**/__pycache__/
python/**/*.pyc
python/**/.venv/
python/**/.pytest_cache/
```

### ralphglasses/.gitignore (add)

```
# Python (openai_cli)
python/**/__pycache__/
python/**/*.pyc
python/**/.venv/
python/**/venv/
python/**/my_env/
python/**/.env
python/**/*.sqlite

# Skills zip bundles (optional, may want to track)
# skills/**/*.zip
```

### dotfiles/.gitignore (add)

```
# JS MCP servers
mcp/**/node_modules/

# Rust (makima)
makima/upstream/target/
```

### mcpkit/.gitignore (add)

```
# JS/TS (open-multi-agent)
js/**/node_modules/
js/**/dist/
js/**/*.js.map
```

### docs/.gitignore (add)

```
# Python tools
tools/**/__pycache__/
tools/**/*.pyc

# Terraform state
infrastructure/**/.terraform/
infrastructure/**/*.tfstate
infrastructure/**/*.tfstate.backup

# Whiteclaw exclusions
research/claude-code-analysis/**/*.map
research/claude-code-analysis/**/*.tgz
research/claude-code-analysis/**/vendor/
research/claude-code-analysis/archives/
```

### art-mcp/.gitignore (add)

```
# Python
**/__pycache__/
**/*.pyc
```

---

## Execution Order

Recommended order (independent migrations can run in parallel):

### Phase 1: No conflicts (parallel-safe)
1. [private] -> [private]/python/[private]/
2. crabravee -> [private]/python/crabravee/
3. openai_cli -> ralphglasses/python/openai_cli/
8. github-stars-catalog -> docs/tools/
9. sway-mcp -> dotfiles/mcp/sway/
10. mac-mcp -> dotfiles/mcp/mac/
11. allthelinks -> dotfiles/web/allthelinks/
12. open-multi-agent -> mcpkit/js/
14. dotfiles-arch -> dotfiles/arch/
15. luke-toolkit -> dotfiles/tools/
16. caper-bush -> dotfiles/zsh/plugins/
17. tmux-ssh-syncing -> dotfiles/zsh/plugins/
18. visual-projects -> art-mcp/visual/
19. opnsense-monolith -> docs/infrastructure/opnsense/
20. unraid-monolith -> docs/infrastructure/unraid/
21. open-sourcing-research -> docs/strategy/open-sourcing/
22. dj-archive -> docs/infrastructure/dj-archive/
23. vj-archive -> docs/infrastructure/vj-archive/
25. claude-skills -> ralphglasses/skills/
26. archlet -> ralphglasses/distro/archlet/

### Phase 2: Requires merge/verification
13. makima -> dotfiles/makima/upstream/ (conflict with existing configs)
24. whiteclaw -> docs/research/claude-code-analysis/ (size management)

### Phase 3: Already migrated (verify only)
4. sam3-video-segmenter -> art-mcp/sam3-segmenter/ (verify supplemental files)
5. procgen-videoclip -> art-mcp/procgen-visual/ (verify assets)
6. wallpaper-procgen -> art-mcp/procgen-visual/ (likely SKIP)
7. video-ai-toolkit -> art-mcp/video-toolkit/ (verify supplemental files)
