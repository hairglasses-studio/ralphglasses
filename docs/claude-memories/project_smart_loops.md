---
name: Smart loop and self-learning subsystems
description: Ralphglasses intelligent loop features — sane defaults, task discovery, dedup, 9 self-learning subsystems, reflexion, cascade routing, bandit selection
type: project
---

Ralphglasses evolved from simple loop manager to intelligent autonomous agent orchestrator with self-learning.

**Why:** Expensive ralph loops that waste tokens on duplicate work, never meet exit criteria, or require manual setup for every repo.

**How to apply:**

## Loop Intelligence
- MCP tools: discover, next_task, fleet, benchmark, breakglass
- Sane defaults via `.ralphrc` scaffolding
- Seed-based dedup (Jaccard similarity, DedupReason tracking)
- Budget propagation and hard caps

## 9 Self-Learning Subsystems (Phases F-I, validated)
- **Reflexion**: Cross-run persistence, broadened failure patterns
- **Episodic Memory**: Configurable k=5, trigram embedder
- **Cascade Routing**: Multi-provider tiered routing (Claude→Gemini→Codex)
- **Curriculum**: Difficulty scoring, FeedbackAnalyzer wired
- **Uncertainty/Confidence**: DecisionModel PredictConfidence adapter
- **Bandit (UCB1)**: Standalone provider selection, Thompson Sampling for cascade
- **Decision Model**: Hedge counting from output text
- **Blackboard**: Inter-agent shared state (two type layers: MCP + session)
- **CostPredictor**: Budget forecasting

**Wiring:** `wireSubsystems(server, sessMgr, ralphDir)` in helpers.go
