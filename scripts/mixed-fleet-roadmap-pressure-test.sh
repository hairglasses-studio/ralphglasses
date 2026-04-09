#!/usr/bin/env bash
# Mixed Fleet Roadmap Pressure Test
# 25 agents across ALL providers: Claude, Codex, Gemini, Cline (free + OpenRouter)
# Working on real ROADMAP.md items with model-tier-appropriate timeouts
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
TS=$(date +%Y%m%d-%H%M%S)
RESULTS_DIR="$REPO_DIR/.ralph/mixed-fleet-$TS"
mkdir -p "$RESULTS_DIR"

PIDS=()
STARTED=0

launch() {
  local AGENT_ID="$1" PROVIDER="$2" MODEL="$3" TIMEOUT="$4" TASK="$5"
  local MODEL_SHORT=$(echo "$MODEL" | sed 's|.*/||' | cut -c1-25)
  local OUTFILE="$RESULTS_DIR/${AGENT_ID}-${PROVIDER}-${MODEL_SHORT}.jsonl"

  echo "  ▸ $AGENT_ID [$PROVIDER:$MODEL_SHORT] timeout=${TIMEOUT}s"

  (
    if [ "$PROVIDER" = "cline" ]; then
      timeout "$TIMEOUT" cline task --yolo --json \
        --model "$MODEL" --cwd "$REPO_DIR" \
        --timeout "$((TIMEOUT - 10))" \
        --auto-condense "$TASK" > "$OUTFILE" 2>&1
    elif [ "$PROVIDER" = "claude" ]; then
      timeout "$TIMEOUT" claude --dangerously-skip-permissions -p \
        "$TASK" --output-format stream-json \
        2>&1 | head -200 > "$OUTFILE"
    elif [ "$PROVIDER" = "gemini" ]; then
      timeout "$TIMEOUT" gemini -p "$TASK" \
        2>&1 | head -200 > "$OUTFILE"
    elif [ "$PROVIDER" = "codex" ]; then
      timeout "$TIMEOUT" codex exec --full-auto --json \
        -m "$MODEL" "$TASK" \
        2>&1 | head -200 > "$OUTFILE"
    fi

    EXIT=$?
    if [ $EXIT -eq 0 ]; then
      echo "  ✅ $AGENT_ID [$PROVIDER:$MODEL_SHORT] completed"
    elif [ $EXIT -eq 124 ]; then
      echo "  ⏰ $AGENT_ID [$PROVIDER:$MODEL_SHORT] timed out"
    else
      echo "  ❌ $AGENT_ID [$PROVIDER:$MODEL_SHORT] failed (exit $EXIT)"
    fi
  ) &
  PIDS+=($!)
  STARTED=$((STARTED + 1))
  sleep 0.5
}

echo "╔══════════════════════════════════════════════════════════════════╗"
echo "║  MIXED FLEET ROADMAP PRESSURE TEST                              ║"
echo "║  25 agents • 4 providers • 8 model types • Real roadmap tasks   ║"
echo "║  Results: $RESULTS_DIR"
echo "╚══════════════════════════════════════════════════════════════════╝"
echo ""
echo "Fleet composition:"
echo "  🔵 Claude (sonnet)         — 2 agents: planning + verification"
echo "  🟣 Codex (gpt-5.4)        — 2 agents: complex implementation"
echo "  🟢 Gemini (3.1-pro)       — 2 agents: standard development"
echo "  🟡 Cline/MiniMax M2.5     — 8 agents: bulk analysis (80% rate)"
echo "  🟡 Cline/GLM-5            — 5 agents: code review (44% rate)"
echo "  🔴 Cline/DeepSeek R1:free — 3 agents: reasoning research"
echo "  🔴 Cline/Qwen3 235B:free  — 2 agents: broad analysis"
echo "  🟡 Cline/DeepSeek V3:free — 1 agent: implementation"
echo ""
echo "Launching fleet at $(date '+%H:%M:%S')..."
echo ""

# === L1 FRONTIER: Claude planner + verifier ===
launch "planner-01" "claude" "sonnet" 90 \
  "Read ROADMAP.md. List ALL open (unchecked) tasks grouped by section. For each, assign priority (P0/P1/P2), estimated effort (S/M/L), and suggest which internal/ Go package owns it. Output as a structured markdown document."

launch "verifier-01" "claude" "sonnet" 90 \
  "Read internal/session/cascade_routing.go. Verify that DefaultModelTiers(), FreeModelIDs(), IsFreeModel(), and ModelTierTimeout() are consistent — all free models in tiers should appear in FreeModelIDs(), timeouts should match documented behavior. Report any inconsistencies."

# === L1 FRONTIER: Codex complex implementation analysis ===
launch "codex-01" "codex" "gpt-5.4" 90 \
  "Read ROADMAP.md WM-2 (GitHub capability probing). Analyze the codebase to identify where gh CLI calls are made. Draft a Go implementation plan for a new internal/ghprobe/ package that checks: CLI auth, SSH key, push rights, repo-create rights. List all functions needed."

launch "codex-02" "codex" "gpt-5.4" 90 \
  "Read ROADMAP.md ATD-2 (durable auth bootstrap). Analyze internal/session/providers.go and cmd/doctor.go. Draft a concrete implementation: what checks to add, where to store probe results, how to integrate with session launch. Include function signatures."

# === L2 WORKER: Gemini standard dev ===
launch "gemini-01" "gemini" "gemini-3.1-pro" 120 \
  "Read ROADMAP.md PDC-2 (tmux continuity preflight). Analyze marathon.sh and cmd/tmux.go. Draft a tmux continuity checker that verifies: main session present, TPM bootstrap, resurrect availability, persistence health. Output as a Go function skeleton."

launch "gemini-02" "gemini" "gemini-3.1-pro" 120 \
  "Read ROADMAP.md ATD-5 (tranche receipt emission). Analyze docs/tranche-receipts/ directory and internal/mcpserver/autobuild_tranches.go. Design the receipt format (JSON schema) and identify where to hook emission into the existing autobuild pipeline."

# === L3 BULK: MiniMax M2.5 (8 agents, 80% completion rate) ===
launch "mm-01" "cline" "minimax/minimax-m2.5" 120 \
  "Read ROADMAP.md and count all tasks by status. Report: total, completed, open, by priority (P0/P1/P2), by effort (S/M/L). Output as JSON."

launch "mm-02" "cline" "minimax/minimax-m2.5" 120 \
  "Read internal/session/provider_capabilities.go. List all provider capabilities and which providers support them. Identify the 5 biggest capability gaps for Cline provider."

launch "mm-03" "cline" "minimax/minimax-m2.5" 120 \
  "Read docs/PROVIDER-PARITY-OBJECTIVES.md. Summarize the top 10 parity items and their current status. Which are blocking fleet operations?"

launch "mm-04" "cline" "minimax/minimax-m2.5" 120 \
  "Read internal/session/cascade_routing.go. List all model tiers, their providers, complexity limits, and costs. Verify the free model entries are correct."

launch "mm-05" "cline" "minimax/minimax-m2.5" 120 \
  "Read cmd/doctor.go and list every health check. For each, note what it checks and its severity. Suggest 3 new checks related to free model availability."

launch "mm-06" "cline" "minimax/minimax-m2.5" 120 \
  "Read internal/fleet/ directory. List all Go files and their main functions. Summarize the fleet management architecture."

launch "mm-07" "cline" "minimax/minimax-m2.5" 120 \
  "Read internal/events/ directory. Describe the event bus and list all event types with their purpose."

launch "mm-08" "cline" "minimax/minimax-m2.5" 120 \
  "Read Makefile. List all targets categorized as: build, test, lint, release, docs, other. Report as markdown table."

# === L3 BULK: GLM-5 (5 agents, 44% rate, longer timeout) ===
launch "glm-01" "cline" "z-ai/glm-5" 180 \
  "Read internal/session/budget.go. Explain the cost tracking and budget enforcement architecture. How does it handle free models?"

launch "glm-02" "cline" "z-ai/glm-5" 180 \
  "Read ROADMAP.md WM-* section. For each open task, draft a 2-sentence implementation approach."

launch "glm-03" "cline" "z-ai/glm-5" 180 \
  "Read internal/session/costnorm.go. Verify that all providers have cost rates. Explain how normalization works."

launch "glm-04" "cline" "z-ai/glm-5" 180 \
  "Count Go test files and test functions across the entire repo. Report the 10 packages with most tests."

launch "glm-05" "cline" "z-ai/glm-5" 180 \
  "Read ROADMAP.md ATD-* section. Group open tasks by theme (auth, publish, artifact, env). Which theme has the most open P0s?"

# === L4 R&D: OpenRouter :free models (longer timeouts for rate limits) ===
launch "dsr1-01" "cline" "deepseek/deepseek-r1:free" 300 \
  "Read ROADMAP.md completely. Using deep reasoning, identify the 3 highest-impact tasks that would unblock the most other tasks. Explain your reasoning."

launch "dsr1-02" "cline" "deepseek/deepseek-r1:free" 300 \
  "Read internal/session/cascade_routing.go completely. Analyze the cascade routing algorithm. Are there correctness bugs? Suggest improvements for free model handling."

launch "dsr1-03" "cline" "deepseek/deepseek-r1:free" 300 \
  "Read docs/CROSS-PROVIDER-SUBAGENT-FLEETS.md. Evaluate the fleet architecture. What are the top 3 risks in running mixed free/paid fleets?"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Fleet: $STARTED agents launched across 4 providers. Waiting..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

for pid in "${PIDS[@]}"; do
  wait "$pid" 2>/dev/null || true
done

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "MIXED FLEET COMPLETE at $(date '+%H:%M:%S')"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Results summary
echo ""
echo "Results by provider:"
TOTAL=$(ls "$RESULTS_DIR"/*.jsonl 2>/dev/null | wc -l)
for prov in claude codex gemini cline; do
  PROV_TOTAL=$(ls "$RESULTS_DIR"/*"-${prov}-"*.jsonl 2>/dev/null | wc -l)
  PROV_DONE=$(grep -l "completion_result\|## \|###" "$RESULTS_DIR"/*"-${prov}-"*.jsonl 2>/dev/null | wc -l)
  echo "  $prov: $PROV_DONE/$PROV_TOTAL completed"
done
echo "  Total: $TOTAL agents"
echo ""
echo "Result files: $RESULTS_DIR/"
