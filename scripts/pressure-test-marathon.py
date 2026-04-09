#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import signal
import subprocess
import sys
import time
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


STOPWORDS = {
    "the",
    "and",
    "for",
    "with",
    "from",
    "that",
    "this",
    "into",
    "then",
    "when",
    "after",
    "before",
    "during",
    "have",
    "has",
    "had",
    "will",
    "would",
    "should",
    "could",
    "must",
    "only",
    "over",
    "under",
    "same",
    "need",
    "adds",
    "add",
    "use",
    "uses",
    "using",
    "keep",
    "keep",
    "into",
    "per",
    "via",
    "task",
    "tasks",
    "repo",
    "roadmap",
    "pressure",
    "test",
    "note",
    "pattern",
}


def now_local() -> datetime:
    return datetime.now().astimezone()


def now_utc() -> datetime:
    return datetime.now(timezone.utc)


def iso_now() -> str:
    return now_local().isoformat(timespec="seconds")


def sh(cmd: list[str], *, cwd: Path | None = None, env: dict[str, str] | None = None, timeout: int | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        cmd,
        cwd=str(cwd) if cwd else None,
        env=env,
        text=True,
        capture_output=True,
        timeout=timeout,
        check=False,
    )


def tokens(text: str) -> set[str]:
    return {
        tok
        for tok in re.findall(r"[a-z0-9]{3,}", text.lower())
        if tok not in STOPWORDS
    }


def sanitize_repo_name(name: str) -> str:
    return re.sub(r"[^A-Za-z0-9._-]+", "-", name).strip("-")


def extract_json_array(raw: str) -> list[dict[str, Any]]:
    start = raw.find("[")
    if start == -1:
        return []
    depth = 0
    end = -1
    for idx, ch in enumerate(raw[start:], start=start):
        if ch == "[":
            depth += 1
        elif ch == "]":
            depth -= 1
            if depth == 0:
                end = idx + 1
                break
    if end == -1:
        return []
    try:
        parsed = json.loads(raw[start:end])
    except json.JSONDecodeError:
        return []
    return parsed if isinstance(parsed, list) else []


def read_json(path: Path, default: Any) -> Any:
    try:
        return json.loads(path.read_text())
    except Exception:
        return default


def read_jsonl(path: Path) -> list[dict[str, Any]]:
    if not path.exists():
        return []
    entries: list[dict[str, Any]] = []
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            entry = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(entry, dict):
            entries.append(entry)
    return entries


def parse_validate_results(output: str) -> dict[str, dict[str, Any]]:
    results: dict[str, dict[str, Any]] = {}
    for entry in extract_json_array(output):
        name = entry.get("name")
        if isinstance(name, str):
            results[name] = entry
    return results


def has_unchecked_roadmap_item(roadmap_path: Path) -> bool:
    if not roadmap_path.exists():
        return False
    for line in roadmap_path.read_text(errors="ignore").splitlines():
        if line.strip().startswith("- [ ]"):
            return True
    return False


def load_wrapped_roadmap(path: Path) -> tuple[dict[str, Any], dict[str, Any]]:
    outer = json.loads(path.read_text())
    content = outer.get("content")
    if not isinstance(content, list) or not content:
        raise ValueError(f"unexpected roadmap wrapper in {path}")
    first = content[0]
    if not isinstance(first, dict) or not isinstance(first.get("text"), str):
        raise ValueError(f"unexpected roadmap text payload in {path}")
    inner = json.loads(first["text"])
    if not isinstance(inner, dict):
        raise ValueError(f"unexpected roadmap body in {path}")
    return outer, inner


def save_wrapped_roadmap(path: Path, outer: dict[str, Any], inner: dict[str, Any]) -> None:
    outer["content"][0]["text"] = json.dumps(inner, indent=2)
    tmp = path.with_suffix(path.suffix + ".tmp")
    tmp.write_text(json.dumps(outer, indent=2) + "\n")
    tmp.replace(path)


def update_roadmap_note(path: Path, note: str, stamp: str) -> tuple[bool, str | None]:
    if not note.strip():
        return False, None
    outer, inner = load_wrapped_roadmap(path)
    tasks = inner.get("tasks")
    if not isinstance(tasks, list):
        return False, None
    unfinished: list[tuple[int, dict[str, Any]]] = []
    for idx, task in enumerate(tasks):
        if isinstance(task, dict) and not bool(task.get("done")):
            unfinished.append((idx, task))
    if not unfinished:
        return False, None

    note = " ".join(note.split())
    note_tokens = tokens(note)
    best_idx = unfinished[0][0]
    best_id = str(unfinished[0][1].get("id", ""))
    best_score = -1
    for idx, task in unfinished:
        task_id = str(task.get("id", ""))
        desc = str(task.get("description", ""))
        score = len(note_tokens & tokens(desc))
        if task_id and task_id in note:
            score += 100
        if score > best_score:
            best_score = score
            best_idx = idx
            best_id = task_id

    rendered = note
    if best_score <= 0:
        rendered = f"Unmapped pressure-test note: {note}"
    stamped = f"{stamp}: {rendered}"
    existing = str(tasks[best_idx].get("improvement_note", "")).strip()
    if stamped in existing:
        return False, best_id
    tasks[best_idx]["improvement_note"] = existing + ("\n" if existing else "") + stamped
    save_wrapped_roadmap(path, outer, inner)
    return True, best_id


def pick_primary_note(entries: list[dict[str, Any]], fallback: str) -> tuple[str, str]:
    for entry in reversed(entries):
        for key in ("suggest", "worked", "failed"):
            items = entry.get(key)
            if isinstance(items, list):
                for item in items:
                    if isinstance(item, str) and item.strip():
                        return item.strip(), key
        task_focus = entry.get("task_focus")
        if isinstance(task_focus, str) and task_focus.strip():
            return task_focus.strip(), "task_focus"
    return fallback.strip(), "fallback"


def sum_cost_observations(path: Path) -> float:
    observations = read_json(path, [])
    total = 0.0
    if isinstance(observations, list):
        for obs in observations:
            if isinstance(obs, dict):
                try:
                    total += float(obs.get("cost_usd", 0))
                except Exception:
                    continue
    return round(total, 6)


def tail_text(path: Path, lines: int = 40) -> str:
    if not path.exists():
        return ""
    content = path.read_text(errors="ignore").splitlines()
    return "\n".join(content[-lines:])


@dataclass
class RepoResult:
    repo: str
    source_repo: str
    canonical_path: str
    status: str
    note: str = ""
    note_source: str = "none"
    roadmap_task_id: str | None = None
    updated_roadmap: bool = False
    verify_status: str = "verify_not_run"
    verify_detail: str = ""
    cost_usd: float = 0.0
    launched_at: str | None = None
    finished_at: str | None = None
    elapsed_sec: float = 0.0
    exit_code: int | None = None
    clone_path: str | None = None
    log_path: str | None = None
    issue: str = ""


@dataclass
class RunningRepo:
    repo: str
    source_repo: Path
    clone_path: Path
    log_path: Path
    process: subprocess.Popen[str]
    started_at_epoch: float
    launched_at: str


class PressureRunner:
    def __init__(self, args: argparse.Namespace) -> None:
        self.args = args
        self.workspace = Path(args.scan_root).expanduser().resolve()
        self.control_repo = self.workspace / "ralphglasses"
        self.binary = (self.control_repo / "ralphglasses").resolve()
        self.script_root = (self.control_repo / "scripts").resolve()
        self.run_stamp = now_local().strftime("%Y%m%d-%H%M%S")
        self.date_stamp = now_local().strftime("%Y-%m-%d")
        self.run_dir = (self.control_repo / ".ralph" / f"pressure-test-{self.run_stamp}").resolve()
        self.logs_dir = self.run_dir / "logs"
        self.clones_dir = self.run_dir / "clones"
        self.shims_dir = self.run_dir / "shims"
        self.obs_path = self.run_dir / "fleet_observations.jsonl"
        self.status_path = self.run_dir / "status.json"
        self.ledger_path = self.control_repo / ".ralph" / f"pressure-test-ledger-{self.date_stamp}.md"
        self.results: dict[str, RepoResult] = {}
        self.running: dict[str, RunningRepo] = {}
        self.pending: list[Path] = []
        self.validate_results: dict[str, dict[str, Any]] = {}
        self.runtime_env = os.environ.copy()
        self.stop_requested = False
        self.ramped = False
        self.first_launch_epoch: float | None = None
        self.last_snapshot_epoch = 0.0

    def ensure_dirs(self) -> None:
        self.run_dir.mkdir(parents=True, exist_ok=True)
        self.logs_dir.mkdir(parents=True, exist_ok=True)
        self.clones_dir.mkdir(parents=True, exist_ok=True)
        self.shims_dir.mkdir(parents=True, exist_ok=True)

    def log(self, message: str) -> None:
        stamp = now_local().strftime("%Y-%m-%d %H:%M:%S")
        line = f"[{stamp}] {message}"
        print(line, flush=True)

    def setup_signal_handlers(self) -> None:
        def handle(signum: int, _frame: Any) -> None:
            self.stop_requested = True
            self.log(f"Received signal {signum}; stopping launches and terminating running marathons")

        signal.signal(signal.SIGINT, handle)
        signal.signal(signal.SIGTERM, handle)

    def setup_runtime(self) -> None:
        codex_js = self.find_codex_entrypoint()
        if codex_js is None:
            raise RuntimeError("Codex entrypoint not found under the current user or /home/hg")

        codex_shim = self.shims_dir / "codex"
        codex_shim.write_text(
            "#!/usr/bin/env bash\n"
            "set -euo pipefail\n"
            f"model={json.dumps(self.args.codex_model)}\n"
            "args=()\n"
            "rewrite_next=0\n"
            "model_seen=0\n"
            "for arg in \"$@\"; do\n"
            "  if [[ \"$rewrite_next\" == 1 ]]; then\n"
            "    args+=(\"$model\")\n"
            "    rewrite_next=0\n"
            "    model_seen=1\n"
            "    continue\n"
            "  fi\n"
            "  case \"$arg\" in\n"
            "    --model)\n"
            "      args+=(\"$arg\")\n"
            "      rewrite_next=1\n"
            "      ;;\n"
            "    --model=*)\n"
            "      args+=(\"--model=$model\")\n"
            "      model_seen=1\n"
            "      ;;\n"
            "    *)\n"
            "      args+=(\"$arg\")\n"
            "      ;;\n"
            "  esac\n"
            "done\n"
            "if [[ \"$model_seen\" == 0 && ${#args[@]} -gt 0 && \"${args[0]}\" == \"exec\" ]]; then\n"
            "  args=(\"exec\" \"--model\" \"$model\" \"${args[@]:1}\")\n"
            "fi\n"
            f"exec node {codex_js} \"${{args[@]}}\"\n"
        )
        git_shim = self.shims_dir / "git"
        git_shim.write_text(
            "#!/usr/bin/env bash\n"
            "set -euo pipefail\n"
            "if [[ $# -gt 0 ]]; then\n"
            "  case \"$1\" in\n"
            "    commit|tag|push|merge|rebase|cherry-pick|am)\n"
            "      echo \"git $1 blocked by pressure-test safety wrapper\" >&2\n"
            "      exit 1\n"
            "      ;;\n"
            "  esac\n"
            "fi\n"
            "exec /usr/bin/git \"$@\"\n"
        )
        codex_shim.chmod(0o755)
        git_shim.chmod(0o755)

        self.runtime_env["PATH"] = f"{self.shims_dir}:{self.runtime_env.get('PATH', '')}"
        self.runtime_env.setdefault("OPENAI_API_KEY", "dummy")

        probe = sh(
            [
                "node",
                str(codex_js),
                "exec",
                "--model",
                self.args.codex_model,
                "--json",
                "--skip-git-repo-check",
                "Reply with OK only.",
            ],
            cwd=self.control_repo,
            env=self.runtime_env,
            timeout=120,
        )
        probe_text = (probe.stdout or "") + (probe.stderr or "")
        if probe.returncode != 0 or "\"text\":\"OK\"" not in probe_text:
            raise RuntimeError(f"Codex runtime probe failed: {probe_text.strip()}")

    def scan_running_bottlenecks(self) -> dict[str, int]:
        counts: dict[str, int] = {}
        for run in self.running.values():
            repo_keys: set[str] = set()
            journal_entries = read_jsonl(run.clone_path / ".ralph" / "improvement_journal.jsonl")
            for entry in journal_entries[-20:]:
                lowered = json.dumps(entry).lower()
                if "selected model is at capacity" in lowered:
                    repo_keys.add("provider_capacity")
                if "usage limit" in lowered:
                    repo_keys.add("usage_limit")

            cycle_dir = run.clone_path / ".ralph" / "cycles"
            if cycle_dir.exists():
                recent_cycles = sorted(
                    cycle_dir.glob("*.json"),
                    key=lambda path: path.stat().st_mtime,
                    reverse=True,
                )[:12]
                for cycle_path in recent_cycles:
                    text = cycle_path.read_text(errors="ignore").lower()
                    if "require_baseline" in text:
                        repo_keys.add("baseline_safety")
                    if "max_concurrent_cycles" in text:
                        repo_keys.add("max_concurrent_cycles")

            log_tail = tail_text(run.log_path, 40).lower()
            if "consider demoting autonomy level" in log_tail:
                repo_keys.add("supervisor_failures")

            for key in repo_keys:
                counts[key] = counts.get(key, 0) + 1
        return counts

    def find_codex_entrypoint(self) -> Path | None:
        candidates = [
            Path.home() / ".local/lib/node_modules/@openai/codex/bin/codex.js",
            Path("/home/hg/.local/lib/node_modules/@openai/codex/bin/codex.js"),
        ]
        for candidate in candidates:
            if candidate.exists():
                return candidate.resolve()
        return None

    def discover_repos(self) -> list[Path]:
        repos: list[Path] = []
        for entry in sorted(self.workspace.iterdir()):
            if not entry.is_dir():
                continue
            roadmap_json = entry / ".ralph" / "roadmap_tasks.json"
            if roadmap_json.exists():
                repos.append(entry.resolve())
        return repos

    def collect_validate_results(self) -> None:
        proc = sh(
            [
                str(self.binary),
                "validate",
                "--scan-path",
                str(self.workspace),
                "--json",
            ],
            cwd=self.control_repo,
            env=self.runtime_env,
        )
        combined = (proc.stdout or "") + "\n" + (proc.stderr or "")
        self.validate_results = parse_validate_results(combined)

    def direct_preflight(self, repo: Path) -> tuple[bool, str]:
        git_dir = repo / ".git"
        if not git_dir.exists():
            return False, "no .git directory"
        if not git_dir.is_dir():
            return False, ".git is not a directory"
        roadmap = repo / "ROADMAP.md"
        if not roadmap.exists():
            return False, "ROADMAP.md not found"
        if not has_unchecked_roadmap_item(roadmap):
            return False, "ROADMAP.md has no unchecked items"
        return True, ""

    def classify_inventory(self) -> None:
        discovered = self.discover_repos()
        for repo in discovered:
            if repo.name == "ralphglasses" and not self.args.include_control_repo:
                result = RepoResult(
                    repo=repo.name,
                    source_repo=repo.name,
                    canonical_path=str(repo),
                    status="excluded_control_plane",
                    issue="reserved for monitoring and final ledger",
                )
                self.results[repo.name] = result
                continue

            validate = self.validate_results.get(repo.name)
            if validate:
                if validate.get("status") == "OK":
                    self.pending.append(repo)
                    continue
                issue = "; ".join(validate.get("issues", [])) or "validate error"
                result = RepoResult(
                    repo=repo.name,
                    source_repo=repo.name,
                    canonical_path=str(repo),
                    status="preflight_blocked",
                    issue=issue,
                    note=f"Pressure test preflight blocked: {issue}",
                    note_source="preflight",
                )
                self.results[repo.name] = result
                continue

            ok, issue = self.direct_preflight(repo)
            if ok:
                self.pending.append(repo)
            else:
                result = RepoResult(
                    repo=repo.name,
                    source_repo=repo.name,
                    canonical_path=str(repo),
                    status="preflight_blocked",
                    issue=issue,
                    note=f"Pressure test direct preflight blocked: {issue}",
                    note_source="direct_preflight",
                )
                self.results[repo.name] = result

        if self.args.max_repos is not None:
            self.pending = self.pending[: self.args.max_repos]

    def update_blocked_roadmaps(self) -> None:
        stamp = self.date_stamp
        for result in self.results.values():
            if result.status != "preflight_blocked":
                continue
            roadmap_path = Path(result.canonical_path) / ".ralph" / "roadmap_tasks.json"
            if not roadmap_path.exists():
                continue
            updated, task_id = update_roadmap_note(roadmap_path, result.note, stamp)
            result.updated_roadmap = updated
            result.roadmap_task_id = task_id

    def start_repo(self, repo: Path) -> None:
        repo_name = sanitize_repo_name(repo.name)
        clone_path = self.clones_dir / repo_name
        log_path = self.logs_dir / f"{repo_name}.log"
        if clone_path.exists():
            shutil.rmtree(clone_path)
        clone = sh(
            ["git", "clone", "--no-hardlinks", str(repo), str(clone_path)],
            cwd=self.run_dir,
            env=os.environ.copy(),
            timeout=1800,
        )
        if clone.returncode != 0:
            result = RepoResult(
                repo=repo_name,
                source_repo=repo.name,
                canonical_path=str(repo),
                status="launch_failed",
                issue=(clone.stderr or clone.stdout).strip(),
                note=f"Pressure test launch failed during clone: {(clone.stderr or clone.stdout).strip()}",
                note_source="clone",
                finished_at=iso_now(),
            )
            self.results[repo_name] = result
            return

        log_handle = log_path.open("w")
        cmd = [
            str(self.binary),
            "marathon",
            "--repo",
            str(clone_path),
            "--duration",
            self.args.duration,
            "--budget",
            f"{self.args.budget:.2f}",
            "--checkpoint-interval",
            self.args.checkpoint_interval,
        ]
        proc = subprocess.Popen(
            cmd,
            cwd=str(self.control_repo),
            env=self.runtime_env,
            text=True,
            stdout=log_handle,
            stderr=subprocess.STDOUT,
            start_new_session=True,
        )
        launched_at = iso_now()
        self.results[repo_name] = RepoResult(
            repo=repo_name,
            source_repo=repo.name,
            canonical_path=str(repo),
            status="running",
            launched_at=launched_at,
            clone_path=str(clone_path),
            log_path=str(log_path),
        )
        self.running[repo_name] = RunningRepo(
            repo=repo_name,
            source_repo=repo,
            clone_path=clone_path,
            log_path=log_path,
            process=proc,
            started_at_epoch=time.time(),
            launched_at=launched_at,
        )
        if self.first_launch_epoch is None:
            self.first_launch_epoch = time.time()
        self.log(f"Started {repo_name} at {clone_path}")

    def terminate_running(self) -> None:
        for run in self.running.values():
            if run.process.poll() is None:
                try:
                    os.killpg(run.process.pid, signal.SIGTERM)
                except ProcessLookupError:
                    continue

    def maybe_ramp(self) -> None:
        if self.ramped or self.first_launch_epoch is None:
            return
        if len([r for r in self.results.values() if r.status in {"running", "completed", "usage_limit", "launch_failed", "interrupted"}]) < self.args.canary_size:
            return
        if time.time() - self.first_launch_epoch < self.args.ramp_after_sec:
            return
        launched = [r for r in self.results.values() if r.launched_at]
        failures = [r for r in launched if r.status in {"launch_failed", "usage_limit", "provider_capacity", "interrupted"}]
        failure_rate = len(failures) / max(1, len(launched))
        alerts = self.latest_observation_alert_count()
        live_bottlenecks = self.scan_running_bottlenecks()
        if failure_rate <= 0.2 and alerts == 0 and not live_bottlenecks:
            self.ramped = True
            self.log(f"Canary healthy enough; ramping concurrency from {self.args.canary_concurrency} to {self.args.max_concurrency}")
        else:
            self.log(
                "Canary not healthy enough to ramp yet "
                f"(failure_rate={failure_rate:.2f}, alerts={alerts}, live_bottlenecks={live_bottlenecks})"
            )

    def current_concurrency(self) -> int:
        return self.args.max_concurrency if self.ramped else self.args.canary_concurrency

    def latest_observation_alert_count(self) -> int:
        if not self.obs_path.exists():
            return 0
        lines = self.obs_path.read_text().splitlines()
        for line in reversed(lines):
            try:
                entry = json.loads(line)
            except json.JSONDecodeError:
                continue
            if entry.get("tool") == "ralphglasses_marathon_dashboard":
                payload = entry.get("payload") or {}
                alerts = payload.get("alerts") or []
                return len(alerts) if isinstance(alerts, list) else 0
        return 0

    def snapshot_tools(self) -> None:
        if time.time() - self.last_snapshot_epoch < self.args.snapshot_interval_sec:
            return
        self.last_snapshot_epoch = time.time()
        for tool, params in (
            ("ralphglasses_marathon", {"action": "status"}),
            ("ralphglasses_marathon_dashboard", {}),
            ("ralphglasses_fleet_status", {"summary_only": "true"}),
        ):
            cmd = [str(self.binary), "mcp-call", tool]
            for key, value in params.items():
                cmd.extend(["-p", f"{key}={value}"])
            proc = sh(cmd, cwd=self.control_repo, env=self.runtime_env, timeout=120)
            payload: Any
            text = (proc.stdout or "").strip()
            try:
                outer = json.loads(text)
                if isinstance(outer, dict) and isinstance(outer.get("content"), list) and outer["content"]:
                    body = outer["content"][0].get("text")
                    payload = json.loads(body) if isinstance(body, str) else outer
                else:
                    payload = outer
            except Exception:
                payload = {"raw": text or (proc.stderr or "").strip()}
            with self.obs_path.open("a") as fh:
                fh.write(json.dumps({"ts": iso_now(), "tool": tool, "payload": payload}) + "\n")

    def finalize_repo(self, repo_name: str) -> None:
        run = self.running.pop(repo_name)
        result = self.results[repo_name]
        elapsed = round(time.time() - run.started_at_epoch, 3)
        log_tail = tail_text(run.log_path)
        journal_entries = read_jsonl(run.clone_path / ".ralph" / "improvement_journal.jsonl")
        cost_total = sum_cost_observations(run.clone_path / ".ralph" / "cost_observations.json")
        lowered = (log_tail + "\n" + json.dumps(journal_entries)).lower()

        if "selected model is at capacity" in lowered:
            status = "provider_capacity"
        elif "usage limit" in lowered:
            status = "usage_limit"
        elif run.process.returncode == 0:
            status = "completed"
        elif elapsed < 60:
            status = "launch_failed"
        else:
            status = "interrupted"

        fallback_note = ""
        if status == "provider_capacity":
            fallback_note = f"Codex model capacity exceeded during pressure test ({self.args.codex_model})"
        elif status == "usage_limit":
            fallback_note = "OpenAI usage limit hit during pressure test"
        elif result.issue:
            fallback_note = result.issue
        elif journal_entries:
            fallback_note = ""
        elif status == "completed":
            fallback_note = "No durable improvement note produced during pressure test run"
        elif log_tail.strip():
            fallback_note = log_tail.strip().splitlines()[-1]

        note, note_source = pick_primary_note(journal_entries, fallback_note)
        verify_status, verify_detail = self.verify_clone(run.clone_path)

        result.status = status
        result.note = note
        result.note_source = note_source
        result.verify_status = verify_status
        result.verify_detail = verify_detail
        result.finished_at = iso_now()
        result.elapsed_sec = elapsed
        result.exit_code = run.process.returncode
        result.cost_usd = cost_total

        roadmap_path = run.source_repo / ".ralph" / "roadmap_tasks.json"
        if roadmap_path.exists() and note:
            updated, task_id = update_roadmap_note(roadmap_path, note, self.date_stamp)
            result.updated_roadmap = updated
            result.roadmap_task_id = task_id

        run_record = {
            "repo": repo_name,
            "source_repo": run.source_repo.name,
            "status": result.status,
            "verify_status": verify_status,
            "exit_code": run.process.returncode,
            "elapsed_sec": elapsed,
            "cost_usd": cost_total,
            "note_source": note_source,
            "roadmap_task_id": result.roadmap_task_id,
            "updated_roadmap": result.updated_roadmap,
            "clone_path": str(run.clone_path),
            "log_path": str(run.log_path),
        }
        (self.run_dir / f"{repo_name}.json").write_text(json.dumps(run_record, indent=2) + "\n")
        self.log(f"Finished {repo_name} with status={result.status} verify={verify_status}")

    def verify_clone(self, clone_path: Path) -> tuple[str, str]:
        diff = sh(["git", "-C", str(clone_path), "status", "--porcelain"], cwd=clone_path, timeout=120)
        if diff.returncode != 0:
            return "verify_unavailable", (diff.stderr or diff.stdout).strip()
        if not diff.stdout.strip():
            return "verify_skipped_no_changes", ""
        if not (clone_path / "go.mod").exists():
            return "verify_skipped_non_go", "dirty clone without go.mod"

        steps = [
            ("build", ["go", "build", "./..."]),
            ("vet", ["go", "vet", "./..."]),
            ("test", ["go", "test", "./...", "-count=1"]),
        ]
        for name, cmd in steps:
            proc = sh(cmd, cwd=clone_path, env=os.environ.copy(), timeout=self.args.verify_timeout_sec)
            if proc.returncode != 0:
                detail = ((proc.stdout or "") + "\n" + (proc.stderr or "")).strip()
                detail = "\n".join(detail.splitlines()[-20:])
                return "verify_fail", f"{name} failed: {detail}"
        return "verify_pass", ""

    def write_status_files(self) -> None:
        live_bottlenecks = self.scan_running_bottlenecks()
        status = {
            "ts": iso_now(),
            "pending": [repo.name for repo in self.pending],
            "running": sorted(self.running.keys()),
            "results": {name: vars(result) for name, result in sorted(self.results.items())},
            "ramped": self.ramped,
            "concurrency": self.current_concurrency(),
            "live_bottlenecks": live_bottlenecks,
        }
        self.status_path.write_text(json.dumps(status, indent=2) + "\n")
        self.ledger_path.write_text(self.render_ledger())

    def render_ledger(self) -> str:
        counts: dict[str, int] = {}
        total_cost = 0.0
        bottlenecks: dict[str, int] = {}
        live_bottlenecks = self.scan_running_bottlenecks()
        for result in self.results.values():
            counts[result.status] = counts.get(result.status, 0) + 1
            total_cost += result.cost_usd
            key = ""
            if result.status == "preflight_blocked":
                key = "preflight_blocked"
            elif result.status == "provider_capacity":
                key = "provider_capacity"
            elif result.status == "usage_limit":
                key = "usage_limit"
            elif result.status == "launch_failed":
                key = "launch_failed"
            elif result.verify_status == "verify_fail":
                key = "verify_fail"
            elif result.verify_status == "verify_unavailable":
                key = "verify_unavailable"
            if key:
                bottlenecks[key] = bottlenecks.get(key, 0) + 1
        for key, count in live_bottlenecks.items():
            bottlenecks[f"live_{key}"] = count

        launched = [r for r in self.results.values() if r.launched_at]
        launched_passed = sum(
            1
            for r in launched
            if r.status == "completed"
            and r.verify_status in {"verify_pass", "verify_skipped_no_changes", "verify_skipped_non_go", "verify_not_run"}
        )
        launched_failed = sum(
            1
            for r in launched
            if r.status in {"launch_failed", "usage_limit", "provider_capacity", "interrupted"} or r.verify_status == "verify_fail"
        )
        pass_rate = 0.0 if not launched else round((launched_passed / max(1, launched_passed + launched_failed)) * 100, 1)

        lines = [
            f"# Ralphglasses Pressure Test Ledger ({self.date_stamp})",
            "",
            f"- Updated: `{iso_now()}`",
            f"- Run dir: `{self.run_dir}`",
            f"- Queue mode: canary `{self.args.canary_concurrency}` then ramp to `{self.args.max_concurrency}`",
            f"- Per-repo budget: `${self.args.budget:.2f}`",
            f"- Per-repo duration: `{self.args.duration}`",
            "",
            "## Summary",
            "",
            f"- Discovered roadmap repos: `{sum(1 for _ in self.discover_repos())}`",
            f"- Excluded control plane: `{counts.get('excluded_control_plane', 0)}`",
            f"- Queued launches: `{len(launched)}`",
            f"- Completed: `{counts.get('completed', 0)}`",
            f"- Interrupted: `{counts.get('interrupted', 0)}`",
            f"- Provider capacity: `{counts.get('provider_capacity', 0)}`",
            f"- Usage limit: `{counts.get('usage_limit', 0)}`",
            f"- Launch failed: `{counts.get('launch_failed', 0)}`",
            f"- Preflight blocked: `{counts.get('preflight_blocked', 0)}`",
            f"- Running now: `{counts.get('running', 0)}`",
            f"- Pass rate: `{pass_rate}%`",
            f"- Total observed cost: `${round(total_cost, 4):.4f}`",
            "",
            "## Bottlenecks",
            "",
        ]
        if bottlenecks:
            for key, count in sorted(bottlenecks.items(), key=lambda item: (-item[1], item[0])):
                lines.append(f"- `{key}`: `{count}`")
        else:
            lines.append("- None observed yet")

        lines.extend(["", "## Repo Results", ""])
        for result in sorted(self.results.values(), key=lambda item: item.repo):
            detail = result.issue or result.verify_detail or result.note
            detail = " ".join(detail.split())
            if len(detail) > 180:
                detail = detail[:177] + "..."
            lines.append(
                f"- `{result.repo}`: `{result.status}`"
                f", verify=`{result.verify_status}`"
                f", cost=`${result.cost_usd:.4f}`"
                f", roadmap_note={'yes' if result.updated_roadmap else 'no'}"
                + (f", task=`{result.roadmap_task_id}`" if result.roadmap_task_id else "")
                + (f" — {detail}" if detail else "")
            )
        lines.append("")
        return "\n".join(lines)

    def run(self) -> int:
        self.ensure_dirs()
        self.setup_signal_handlers()
        self.setup_runtime()
        self.collect_validate_results()
        self.classify_inventory()
        self.update_blocked_roadmaps()
        self.write_status_files()

        self.log(
            f"Queued {len(self.pending)} repos with {len([r for r in self.results.values() if r.status == 'preflight_blocked'])} blocked"
        )

        while (self.pending or self.running) and not self.stop_requested:
            self.maybe_ramp()
            self.snapshot_tools()
            self.write_status_files()

            while self.pending and len(self.running) < self.current_concurrency() and not self.stop_requested:
                repo = self.pending.pop(0)
                self.start_repo(repo)
                self.write_status_files()

            completed: list[str] = []
            for repo_name, run in list(self.running.items()):
                if run.process.poll() is not None:
                    completed.append(repo_name)
            for repo_name in completed:
                self.finalize_repo(repo_name)
                self.write_status_files()

            if not self.pending and not self.running:
                break
            time.sleep(self.args.poll_interval_sec)

        if self.stop_requested:
            self.terminate_running()
            grace_until = time.time() + 30
            while self.running and time.time() < grace_until:
                completed = []
                for repo_name, run in list(self.running.items()):
                    if run.process.poll() is not None:
                        completed.append(repo_name)
                for repo_name in completed:
                    self.finalize_repo(repo_name)
                    self.write_status_files()
                if self.running:
                    time.sleep(1)
            for repo_name, run in list(self.running.items()):
                if run.process.poll() is None:
                    try:
                        os.killpg(run.process.pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                self.finalize_repo(repo_name)
                self.write_status_files()

        self.snapshot_tools()
        self.write_status_files()
        self.log(f"Ledger written to {self.ledger_path}")
        return 0


def parse_args() -> argparse.Namespace:
    script_root = Path(__file__).resolve().parents[2]
    parser = argparse.ArgumentParser(description="Run the ralphglasses parallel pressure test")
    parser.add_argument("--scan-root", default=str(script_root))
    parser.add_argument("--duration", default="30m")
    parser.add_argument("--budget", type=float, default=2.0)
    parser.add_argument("--codex-model", default="gpt-5.4-mini")
    parser.add_argument("--checkpoint-interval", default="5m")
    parser.add_argument("--canary-concurrency", type=int, default=5)
    parser.add_argument("--max-concurrency", type=int, default=8)
    parser.add_argument("--ramp-after-sec", type=int, default=600)
    parser.add_argument("--snapshot-interval-sec", type=int, default=60)
    parser.add_argument("--poll-interval-sec", type=int, default=5)
    parser.add_argument("--verify-timeout-sec", type=int, default=300)
    parser.add_argument("--canary-size", type=int, default=5)
    parser.add_argument("--max-repos", type=int)
    parser.add_argument("--include-control-repo", action="store_true")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    runner = PressureRunner(args)
    try:
        return runner.run()
    except Exception as exc:
        print(f"pressure-test-marathon failed: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
