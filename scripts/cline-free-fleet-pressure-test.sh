#!/usr/bin/env bash
# Cline Free Model Fleet Pressure Test
# Launches 25 parallel free Cline sessions working on roadmap analysis tasks
# Models: GLM-5, MiniMax M2.5, KAT Coder Pro, Arcee Trinity
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
RESULTS_DIR="$REPO_DIR/.ralph/pressure-test-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$RESULTS_DIR"

# Free models available via Cline provider
MODELS=(
  "z-ai/glm-5"
  "minimax/minimax-m2.5"
  "z-ai/glm-5"
  "minimax/minimax-m2.5"
  "kwaipilot/kat-coder-pro"
  "minimax/minimax-m2.5"
  "z-ai/glm-5"
  "arcee-ai/trinity-large-preview:free"
  "minimax/minimax-m2.5"
  "z-ai/glm-5"
  "minimax/minimax-m2.5"
  "z-ai/glm-5"
  "kwaipilot/kat-coder-pro"
  "minimax/minimax-m2.5"
  "z-ai/glm-5"
  "arcee-ai/trinity-large-preview:free"
  "minimax/minimax-m2.5"
  "z-ai/glm-5"
  "minimax/minimax-m2.5"
  "kwaipilot/kat-coder-pro"
  "z-ai/glm-5"
  "minimax/minimax-m2.5"
  "z-ai/glm-5"
  "arcee-ai/trinity-large-preview:free"
  "minimax/minimax-m2.5"
)

# Roadmap tasks — each agent gets one analysis/planning task (read-only, no mutations)
TASKS=(
  "Read ROADMAP.md and list all open (unchecked) WM-* tasks. For each, rate implementation complexity 1-5 and suggest which Go package would own it. Output as a markdown table."
  "Read internal/session/providers.go and internal/session/provider_cline.go. Identify any missing Cline CLI flags that should be supported. List gaps as bullet points."
  "Read ROADMAP.md section 'Autonomous Tranche Delivery Notes'. For ATD-1 through ATD-7, draft a one-sentence implementation approach for each."
  "Read internal/session/cascade.go and cascade_routing.go. Summarize the cascade routing logic in 5 bullet points and identify potential edge cases."
  "Read ROADMAP.md PDC-* tasks. For each open task, identify which existing internal/ package is closest to owning it."
  "Count all Go test files in cmd/ and internal/. Report total test files, total test functions, and the 5 packages with the most tests."
  "Read internal/mcpserver/management_tools.go and list all MCP management tools. For each, describe what it does in one sentence."
  "Read docs/PROVIDER-PARITY-OBJECTIVES.md and summarize the top 5 parity gaps between providers."
  "Read internal/session/budget.go and costs.go. Explain the cost tracking architecture in 5 bullet points."
  "Read ROADMAP.md and count: total tasks, completed tasks, open P0 tasks, open P1 tasks. Report as JSON."
  "Read internal/tui/app.go and describe the BubbleTea app lifecycle: Init, Update, View pattern. List all view states."
  "Read cmd/doctor.go and list all health checks performed by the doctor command. Suggest 3 additional checks."
  "Read internal/session/provider_capabilities.go. Compare Cline capabilities vs Claude capabilities. Which features does Cline lack?"
  "Read internal/fleet/ directory structure. Summarize the fleet management architecture in 5 bullet points."
  "Read ROADMAP.md WM-2 task about GitHub capability probing. Draft a test plan with 5 test cases."
  "Read internal/mcpserver/autobuild_tranches.go. Explain the tranche priority algorithm and feedback scoring."
  "Read cmd/session.go and describe the CLI session management commands. Suggest improvements."
  "Read internal/discovery/scanner.go. Explain how repo discovery works. What improvements would make it faster?"
  "Read ROADMAP.md ATD-2 about durable auth bootstrap. List the 5 most important checks and their Go implementation approach."
  "Read marathon.sh. Explain what it does, its flags, and suggest 3 improvements for reliability."
  "Read internal/events/ directory. Describe the event bus architecture and list all event types."
  "Read docs/MARATHON.md and docs/LOOP-DEFAULTS.md. Summarize the marathon execution model in 5 bullets."
  "Read internal/session/types.go. For each type, write a one-sentence description of its purpose."
  "Read Makefile. List all targets and categorize them: build, test, lint, release, other."
  "Read ROADMAP.md PDC-2 about tmux continuity. Draft an implementation plan: what to check, where to store state, how to recover."
)

TIMEOUT=90
STARTED=0
COMPLETED=0
FAILED=0
PIDS=()

echo "╔══════════════════════════════════════════════════════════════╗"
echo "║  CLINE FREE MODEL FLEET PRESSURE TEST                       ║"
echo "║  25 parallel sessions • 4 free models • 0 cost              ║"
echo "║  Results: $RESULTS_DIR"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""
echo "Models in rotation:"
echo "  🟢 z-ai/glm-5          (Coding: 39.0, Best free coder)"
echo "  🟢 minimax/minimax-m2.5 (Coding: 37.4, Best free general)"
echo "  🟡 kwaipilot/kat-coder-pro (Coding: 18.3, Math-focused)"
echo "  🟡 arcee-ai/trinity-large-preview:free (400B MoE, general)"
echo ""
echo "Launching fleet at $(date '+%H:%M:%S')..."
echo ""

for i in $(seq 0 24); do
  MODEL="${MODELS[$i]}"
  TASK="${TASKS[$i]}"
  AGENT_ID=$(printf "agent-%02d" $((i+1)))
  MODEL_SHORT=$(echo "$MODEL" | sed 's|.*/||' | cut -c1-20)
  OUTFILE="$RESULTS_DIR/${AGENT_ID}-${MODEL_SHORT}.jsonl"

  echo "  ▸ $AGENT_ID [$MODEL_SHORT] launching..."

  (
    timeout "$TIMEOUT" cline task --yolo --json \
      --model "$MODEL" \
      --cwd "$REPO_DIR" \
      --timeout "$((TIMEOUT - 10))" \
      --auto-condense \
      "$TASK" > "$OUTFILE" 2>&1

    EXIT=$?
    if [ $EXIT -eq 0 ]; then
      echo "  ✅ $AGENT_ID [$MODEL_SHORT] completed"
    elif [ $EXIT -eq 124 ]; then
      echo "  ⏰ $AGENT_ID [$MODEL_SHORT] timed out"
    else
      echo "  ❌ $AGENT_ID [$MODEL_SHORT] failed (exit $EXIT)"
    fi
  ) &
  PIDS+=($!)
  STARTED=$((STARTED + 1))

  # Stagger launches slightly to avoid thundering herd
  sleep 0.3
done

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Fleet: $STARTED sessions launched. Waiting for completion..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Wait for all background jobs
for pid in "${PIDS[@]}"; do
  wait "$pid" 2>/dev/null || true
done

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "FLEET PRESSURE TEST COMPLETE at $(date '+%H:%M:%S')"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Summarize results
echo ""
echo "Results summary:"
TOTAL_FILES=$(ls "$RESULTS_DIR"/*.jsonl 2>/dev/null | wc -l)
COMPLETED_FILES=$(grep -l "completion_result" "$RESULTS_DIR"/*.jsonl 2>/dev/null | wc -l)
TIMEOUT_FILES=$(grep -l '"Timeout"' "$RESULTS_DIR"/*.jsonl 2>/dev/null | wc -l)
ERROR_FILES=$(grep -l '"error"' "$RESULTS_DIR"/*.jsonl 2>/dev/null | wc -l)

echo "  Total sessions:  $TOTAL_FILES"
echo "  Completed:       $COMPLETED_FILES"
echo "  Timed out:       $TIMEOUT_FILES"
echo "  Errors:          $ERROR_FILES"
echo ""

# Per-model breakdown
for MODEL in "glm-5" "minimax-m2.5" "kat-coder-pro" "trinity-large-preview"; do
  MODEL_TOTAL=$(ls "$RESULTS_DIR"/*"$MODEL"*.jsonl 2>/dev/null | wc -l)
  MODEL_COMPLETED=$(grep -l "completion_result" "$RESULTS_DIR"/*"$MODEL"*.jsonl 2>/dev/null | wc -l)
  echo "  $MODEL: $MODEL_COMPLETED/$MODEL_TOTAL completed"
done

echo ""
echo "Result files: $RESULTS_DIR/"
echo ""

# Extract completion results into a summary
SUMMARY="$RESULTS_DIR/SUMMARY.md"
{
  echo "# Cline Free Fleet Pressure Test Results"
  echo ""
  echo "**Date:** $(date '+%Y-%m-%d %H:%M:%S')"
  echo "**Sessions:** $TOTAL_FILES launched, $COMPLETED_FILES completed"
  echo "**Cost:** \$0.00 (all free models)"
  echo ""
  echo "## Results by Agent"
  echo ""

  for f in "$RESULTS_DIR"/agent-*.jsonl; do
    AGENT=$(basename "$f" .jsonl | cut -d- -f1-2)
    MODEL=$(basename "$f" .jsonl | cut -d- -f3-)
    RESULT=$(grep -o '"completion_result","text":"[^"]*"' "$f" 2>/dev/null | head -1 | sed 's/"completion_result","text":"//;s/"$//' || echo "NO RESULT")
    echo "### $AGENT ($MODEL)"
    echo ""
    if [ "$RESULT" != "NO RESULT" ]; then
      echo "$RESULT" | sed 's/\\n/\n/g'
    else
      LAST_ERROR=$(grep -o '"error"[^}]*' "$f" 2>/dev/null | tail -1 || echo "unknown")
      echo "_No completion. Last error: $LAST_ERROR_"
    fi
    echo ""
  done
} > "$SUMMARY"

echo "Summary written to: $SUMMARY"
