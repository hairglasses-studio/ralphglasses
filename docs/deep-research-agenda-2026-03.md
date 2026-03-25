# Deep Research Agenda: Frontier Opportunities for Ralphglasses

**Date:** 2026-03-25
**Companion to:** `ai-platform-research-2026-03.md` (provider features & API capabilities)
**Scope:** Research frontiers where heuristic-based subsystems can be replaced with learned policies, multi-agent protocols, and predictive models.

---

## Methodology

The platform research document covers **what each provider offers**. This document covers **what the codebase should become** — grounded in specific code locations showing current limitations.

Each research thread follows a consistent structure:
1. **Code anchor** — exact file and line range showing the current heuristic
2. **Limitation** — what the heuristic cannot do
3. **Research questions** — precise, searchable queries
4. **Keyword clusters** — for arxiv, Google Scholar, and industry blog search
5. **Venues** — conferences, blogs, and adjacent fields most likely to have answers
6. **Leverage rating** — how many subsystems a single answer would improve

**Prioritization rubric:** Leverage (multi-system impact) x Feasibility (implementable in Go, no GPU dependency) x Urgency (blocks current roadmap Phase E/F items)

---

## 1. Bleeding-Edge Threads

### 1.1 Adaptive Provider Selection via Multi-Armed Bandits

**Leverage: HIGH** — resolves gaps in cascade routing, fleet optimizer, and auto-optimize simultaneously.

**Code anchors:**
- `internal/session/cascade.go:269-271` — cascade threshold hardcoded at 0.7, never updated from outcomes
- `internal/fleet/optimizer.go:56-103` — provider weight update uses exponential moving average (blend factor 0.7/0.3) with no exploration
- `internal/session/autooptimize.go:36-86` — feedback-driven budget adjustment but no exploration-exploitation tradeoff

**Limitation:** The system exploits its current best guess forever. It never explores whether a provider that historically underperformed on a task type has improved (e.g., after a model update). The EMA blend factor was chosen by hand and cannot adapt to non-stationary reward distributions (provider pricing and capability change over time).

**Research questions:**
1. What is the right bandit formulation for provider selection when each arm (provider) has non-stationary reward distributions? Standard UCB1 assumes stationarity — what discount factor or sliding window adapts to quarterly model updates?
2. How should Thompson Sampling be adapted for the 3-provider, 8-task-type, continuous-cost setting where rewards are a tuple (completion_rate, -cost) rather than a scalar?
3. What contextual bandit approach handles the feature vector `(task_type, prompt_complexity, historical_success_rate, budget_remaining)` for provider routing? LinUCB? Neural contextual bandits are too heavy for Go — what lightweight alternatives exist?
4. How do you handle cold-start (new task type with zero observations) in a bandit framework? The current system falls back to Claude for everything unknown.
5. What is the minimum sample size for a bandit to outperform the current static threshold? The system processes ~50-200 tasks/day — is that enough for convergence?

**Keyword clusters:**
- `multi-armed bandit provider routing`
- `contextual bandit LLM selection`
- `Thompson sampling non-stationary rewards`
- `UCB1 sliding window discounted`
- `online learning API routing production`
- `cold start bandit new arm`

**Venues:**
- NeurIPS (Bandit workshops), ICML, MLSys
- RecSys (cold-start problem)
- Production ML blogs: Uber (multi-armed bandit for A/B testing), Netflix (contextual bandits for recommendations), DoorDash (delivery routing under uncertainty)

**Adjacent fields:**
- Dynamic pricing (airlines, ride-sharing) — same non-stationary multi-armed bandit structure
- Clinical trial adaptive design — sequential allocation under uncertainty with ethical constraints analogous to budget constraints

**Go library candidates:**
- `gonum/stat` — Beta distribution sampling for Thompson Sampling
- `gonum/optimize` — Bayesian optimization for hyperparameter tuning
- Custom: the `BanditPolicy` interface is simple enough to implement from scratch (~200 lines for Thompson Sampling with Beta priors)

---

### 1.2 Learned Confidence Scoring & Threshold Adaptation

**Leverage: HIGH** — resolves gaps in uncertainty quantification, cascade escalation, and reflexion triggering simultaneously.

**Code anchors:**
- `internal/session/uncertainty.go:102-125` — 5-component weighted average with fixed weights `(0.30, 0.25, 0.20, 0.15, 0.10)`, chosen by hand
- `internal/session/cascade.go:278-337` — `computeConfidence` uses 4 equal-weight components (turn efficiency, verify passed, hedging language, error-free)
- `internal/session/uncertainty.go:129-137` — `ShouldTriggerReflexion` fires at static 0.3; `ShouldSkipVerification` fires at static 0.95

**Limitation:** The confidence scores are not calibrated — a score of 0.8 does not mean 80% probability of success. The two confidence functions (`ExtractConfidence` in uncertainty.go and `computeConfidence` in cascade.go) compute different scores from overlapping signals with incompatible scales. Thresholds were chosen once and never adapt to observed outcomes.

**Research questions:**
1. Can logistic regression or isotonic calibration trained on the `LoopObservation` JSONL data produce calibrated confidence scores where `P(verify_passed | confidence=0.8) ≈ 0.8`? The observation struct already contains `Confidence`, `VerifyPassed`, `CascadeEscalated`, `DifficultyScore`, `ReflexionApplied` — all usable as features/labels.
2. How should the cascade escalation threshold adapt online? Bayesian optimization with the objective `minimize(escalation_cost - saved_cost_from_avoiding_failures)` could tune the threshold, but what is the right acquisition function for a noisy, discrete objective?
3. What features beyond the current 5 are predictive of session success? Candidates: `diff_size`, `task_type`, `provider`, `time_of_day`, `consecutive_failures`, `prompt_length`. Which features have the highest marginal information gain?
4. Is there a lightweight concept drift detector that flags when the confidence model's calibration has degraded? The system needs to know when to retrain, not just when to predict.

**Keyword clusters:**
- `calibrated confidence estimation`
- `online threshold optimization Bayesian`
- `Platt scaling logistic calibration`
- `isotonic regression probability calibration`
- `concept drift detection lightweight`
- `adaptive threshold online learning`

**Venues:**
- ICML (uncertainty quantification workshops), AISTATS
- Production ML: Google (calibration in large-scale prediction), Meta (calibration for ad ranking)
- Scikit-learn documentation (calibration module design patterns, portable to Go)

**Unification opportunity:** A single calibrated confidence model replaces both `computeConfidence` in cascade.go AND `ExtractConfidence` in uncertainty.go. The `LoopObservation` JSONL already contains every field needed for training. One model, one threshold adaptation loop, two call sites eliminated.

---

### 1.3 Speculative & Parallel Execution

**Leverage: MEDIUM** — could dramatically reduce cascade latency, currently the biggest performance bottleneck in the routing path.

**Code anchors:**
- `internal/session/cascade.go:242-275` — `EvaluateCheapResult` runs sequentially: cheap provider first, evaluate, then escalate to expensive if confidence < threshold
- `internal/session/loop.go` — planner → worker → verifier pipeline is strictly sequential

**Limitation:** The cascade always pays the latency of the cheap attempt before discovering it needs the expensive provider. For tasks where the cheap provider has a ~40% failure rate, 40% of executions pay double latency (cheap attempt + expensive retry).

**Research questions:**
1. Under what cost/latency conditions is speculative execution (run cheap AND expensive in parallel, cancel the loser) net positive? Given current pricing (Gemini Flash $0.30/1M vs Claude Sonnet $3/1M), what is the breakeven failure rate for the cheap provider?
2. How should the speculation decision be made per-task? High-difficulty tasks (curriculum score > 0.8) should speculate; trivial tasks should not. Can the curriculum score serve as the speculation trigger?
3. Can the cascade router pre-classify tasks to skip the cheap provider entirely for known-hard tasks, analogous to CPU branch prediction? The curriculum sorter already computes difficulty — what threshold makes "always use expensive" cheaper than "try cheap first"?
4. What cancellation mechanisms do each provider's APIs and CLIs support? Claude CLI has no mid-stream cancel (must SIGKILL the process); Codex CLI supports SIGTERM gracefully; Gemini CLI supports SIGTERM. What are the cost implications of a killed-but-partially-completed request?

**Keyword clusters:**
- `speculative execution LLM routing`
- `hedged requests distributed systems`
- `parallel provider routing cancel loser`
- `branch prediction analogy machine learning`
- `tail latency optimization`

**Venues:**
- SOSP/OSDI — Google's "The Tail at Scale" paper (hedged requests for storage)
- MLSys — speculative decoding papers (different context but same pattern)
- Provider API documentation — cancellation semantics

**Adjacent fields:**
- CPU branch prediction — same structure (predict, speculatively execute, roll back on mispredict)
- Hedged requests in distributed storage — Google, Microsoft, Amazon all published on this
- Speculative decoding in LLMs — using a small model to draft tokens verified by a large model

---

### 1.4 Cross-Session Context & Knowledge Distillation

**Leverage: MEDIUM** — resolves the per-session isolation limitation that prevents fleet-wide learning.

**Code anchors:**
- `internal/session/episodic.go:95-150` — `FindSimilar` uses Jaccard word-set similarity plus fixed recency bonus (0.5 for 7 days, 0.25 for 30 days), no embeddings
- `internal/session/contextstore.go` — tracks file conflicts between sessions, not semantic knowledge
- `internal/session/journal.go` — `ConsolidatedPatterns` counts recurring patterns but does not summarize or compress

**Limitation:** Episodic memory retrieves by keyword overlap, which misses semantically similar but lexically different tasks ("refactor auth middleware" vs "restructure login handler"). Knowledge learned in one session (e.g., "this repo's tests require a running Postgres") is not distilled into a form that other sessions can consume.

**Research questions:**
1. Can Jaccard similarity be replaced with lightweight embeddings that run locally in Go? Candidates: ONNX Runtime Go bindings with a small sentence-transformer model (~50MB), or a provider API call to generate embeddings (Claude/OpenAI embedding endpoints at ~$0.0001/query). What is the retrieval quality vs latency tradeoff?
2. How should knowledge distillation work across sessions? The journal records `worked`, `failed`, and `suggest` entries — can these be periodically summarized into a compressed knowledge base that new sessions receive as context?
3. What data structure supports a shared knowledge base that multiple concurrent sessions read from and contribute to without write contention? Append-only log with periodic compaction? CRDT-based merge?
4. How should the Files API (Phase E remaining work) be used architecturally? Upload distilled knowledge as a file, reference by `file_id` in all sessions sharing that repo — eliminates redundant context tokens.

**Keyword clusters:**
- `knowledge distillation multi-session agents`
- `lightweight embedding inference Go ONNX`
- `shared memory multi-agent systems`
- `context compression LLM agents`
- `episodic memory neural retrieval`
- `append-only knowledge base concurrent`

**Venues:**
- NeurIPS (memory-augmented agents), EMNLP (retrieval-augmented generation)
- Provider documentation: Anthropic Files API, OpenAI Embeddings API
- Go ecosystem: `onnxruntime-go`, `go-sentence-transformers`

**Adjacent fields:**
- Retrieval-augmented generation (RAG) — same problem of finding relevant context from a knowledge store
- Memory-augmented neural networks — architectures for persistent agent memory
- Collaborative filtering — finding relevant episodes based on task similarity

---

## 2. Unification Opportunities

### 2.1 Unified Decision Framework: Confidence + Cascade + HITL

**Current state:** Three separate scoring systems compute overlapping signals independently:
- `uncertainty.go:ExtractConfidence` — 5-weight composite for general confidence
- `cascade.go:computeConfidence` — 4-weight composite for escalation decisions
- `hitl.go` — human-in-the-loop scoring for autonomy decisions

**Proposed unification:** A single `DecisionModel` that produces a calibrated probability distribution over outcomes (success, partial, failure) from a unified feature set. Each consumer (cascade, HITL, reflexion trigger, skip-verify) queries the model with its specific decision threshold.

**Research question:** What is the minimal feature set that serves all three decision points? Can a single logistic regression model with task-specific intercept terms handle cascade escalation, HITL triggering, and reflexion initiation?

---

### 2.2 Unified Context Lifecycle: Caching + Files API + Episodic Memory

**Current state:** Three mechanisms manage context persistence independently:
- Prompt caching (per-provider, per-request, TTL-based)
- Files API (upload once, reference by ID, cross-request)
- Episodic memory (per-repo JSONL, keyword retrieval)

**Proposed unification:** A `ContextManager` that tracks what knowledge is available in which persistence layer (hot cache, file store, episodic archive) and routes context injection based on cost and freshness:
- Hot path: prompt cache (free if warm, 90% discount)
- Warm path: Files API (upload cost amortized across sessions)
- Cold path: episodic retrieval (keyword/embedding lookup, token cost to inject)

**Research question:** What is the optimal eviction/promotion policy across the three tiers? Can a cost-aware LRU variant minimize total context cost while maintaining retrieval quality?

---

### 2.3 Unified Job Scheduler: Batch API + Curriculum + Fleet Queue

**Current state:** Three scheduling systems operate independently:
- Batch API (`internal/batch/`) — submits bulk requests for 50% discount, 24h turnaround
- Curriculum sorter — orders tasks by difficulty (easy → hard)
- Fleet work queue — priority-based assignment with budget gating

**Proposed unification:** A `SmartScheduler` that jointly optimizes task ordering, provider assignment, and batch/realtime routing:
- Tasks below difficulty threshold AND non-urgent → batch (50% off)
- Tasks above difficulty threshold OR urgent → realtime with curriculum ordering
- Provider assignment via bandit policy (Section 1.1) rather than static rules

**Research question:** How should the scheduler balance the latency penalty of batching against the 50% cost savings? What is the right urgency classifier — can it be derived from the task's position in a dependency graph?

---

### 2.4 Unified Schema Enforcement: Structured Outputs + Validation Middleware

**Current state:** Structured output validation is implemented per-provider:
- Claude: `output_config.format.json_schema`
- Gemini: `response_mime_type` + `response_json_schema`
- OpenAI: `strict: true` + `--output-schema`
- MCP middleware: `ValidationMiddleware` in `mcpserver/middleware.go`

**Proposed unification:** A provider-agnostic `SchemaRegistry` where task types declare their expected output schema once. The registry maps schemas to provider-specific enforcement mechanisms and validates responses uniformly regardless of which provider produced them.

**Research question:** Can JSON Schema validation be pushed entirely server-side (all 3 providers now support it), eliminating client-side validation overhead? What is the failure mode when a provider's schema enforcement produces valid-but-wrong output (schema-conformant but semantically incorrect)?

---

## 3. High-Leverage Research Questions (Ranked)

Ordered by estimated leverage — how many subsystems a single answer would improve.

| Rank | Question | Sections | Subsystems Improved |
|------|----------|----------|---------------------|
| 1 | What contextual bandit formulation handles 3 providers, 8 task types, continuous cost, and non-stationary rewards in a Go implementation without GPU? | 1.1 | cascade, fleet optimizer, auto-optimize, curriculum |
| 2 | Can logistic regression on LoopObservation JSONL produce calibrated confidence that replaces both `computeConfidence` and `ExtractConfidence`? | 1.2 | uncertainty, cascade, reflexion trigger, skip-verify |
| 3 | What is the minimal evaluation framework that enables counterfactual analysis of routing decisions using existing observation data? | 4.1 | all learning subsystems (prerequisite) |
| 4 | How should A2A protocol or blackboard architecture be integrated alongside MCP for agent-to-agent delegation in the fleet? | 4.2 | fleet coordination, cross-session context, worktree merge |
| 5 | What cost-aware eviction policy across cache/Files API/episodic tiers minimizes total context cost? | 2.2 | prompt caching, Files API, episodic memory |
| 6 | Under what failure-rate threshold does speculative parallel execution (cheap + expensive simultaneously) become net positive? | 1.3 | cascade latency, fleet throughput |
| 7 | Can lightweight ONNX embeddings in Go replace Jaccard word-set similarity for episodic retrieval without GPU dependency? | 1.4 | episodic memory, reflexion matching, curriculum scoring |
| 8 | What sliding-window anomaly detector on cost/latency can provide predictive budget alerts from LoopObservation data? | 4.3 | observability, budget management, fleet health |
| 9 | How should a batch/realtime routing decision be made jointly with curriculum ordering and provider selection? | 2.3 | batch API, curriculum, fleet queue |
| 10 | What concept drift detector flags when the confidence model needs retraining? | 1.2, 4.1 | uncertainty, cascade, all learning |

---

## 4. Prerequisite: Evaluation Framework & Counterfactual Analysis

**This section is ranked separately because it is a prerequisite for validating all other research threads.**

**Leverage: MEDIUM as a standalone system, but CRITICAL as an enabler.**

**Code anchors:**
- `internal/session/loopbench.go` — `LoopObservation` records 40+ signals per iteration to JSONL, but no offline analysis pipeline exists
- `internal/session/selflearn_test.go` — property/integration tests but no A/B framework
- `internal/mcpserver/toolbench.go` — tool regression detection, but no routing regression detection

**Limitation:** The system records everything needed to evaluate decisions (observation JSONL has provider, task type, cost, confidence, verify result, cascade escalation) but never analyzes this data offline. There is no way to answer "was the cascade threshold of 0.7 better or worse than 0.6 would have been?"

**Research questions:**
1. How should counterfactual evaluation work for cascade routing? The observation JSONL records `cascade_escalated` and `verify_passed` — can inverse propensity scoring estimate what would have happened under a different threshold?
2. What A/B testing framework is appropriate for a system with <200 daily decisions? Frequentist tests need thousands of samples. Bayesian A/B testing with Beta-Bernoulli models could work — what prior should be used?
3. How do you detect regression in self-learning effectiveness over time? The observation data has timestamps — can a changepoint detection algorithm identify when the system started performing worse?
4. What offline policy evaluation method validates the bandit policies proposed in Section 1.1 before deploying them? Doubly robust estimation? Direct method with the logistic regression model from Section 1.2?

**Keyword clusters:**
- `counterfactual evaluation offline policy`
- `Bayesian A/B testing small sample`
- `inverse propensity scoring off-policy`
- `doubly robust estimator`
- `changepoint detection time series`
- `bandit offline evaluation`

**Venues:**
- KDD, RecSys — offline policy evaluation is a core topic
- ICML — off-policy evaluation workshops
- Spotify engineering blog (Bayesian A/B testing at scale)
- Netflix engineering blog (counterfactual evaluation for recommendation)

---

## 5. Observability Beyond Metrics

**Leverage: MEDIUM**

**Code anchors:**
- `internal/tracing/tracing.go` — `Recorder` interface with span/metric recording, no analysis layer
- `internal/session/loopbench.go:286-372` — `AggregateObservations` computes rolling stats (completion rate, cost trend) but no anomaly detection

**Limitation:** The tracing system records and exposes metrics but cannot answer "why did cost spike?" or "will this session exceed budget?" Aggregation detects trends (cost increasing/decreasing) but not anomalies (sudden jumps) or root causes (which subsystem caused it).

**Research questions:**
1. Can a simple anomaly detector (z-score on sliding window, or IQR on rolling aggregates) on `cost_per_iter` and `total_latency_ms` in the observation JSONL provide useful alerts without a separate monitoring stack?
2. What causal inference method identifies whether enabling reflexion caused an improvement, or whether it was the task mix changing? The observation data has `reflexion_applied` (treatment) and `verify_passed` (outcome) — is propensity score matching feasible with ~200 daily observations?
3. Can predictive budget alerting ("this session is on track to exceed $X by turn 30") be implemented from the turn-level cost trajectory? Linear extrapolation? Exponential smoothing?

**Keyword clusters:**
- `anomaly detection time series lightweight`
- `causal inference observational data small sample`
- `predictive budget alerting cost trajectory`
- `root cause analysis automated`

**Venues:**
- SRE/DevOps literature (Google SRE book chapters on alerting)
- AIOps papers (automated root cause analysis)
- NSDI (network/system diagnostics)

**Adjacent fields:**
- Financial fraud detection — anomaly detection on transaction streams with similar volume (~hundreds/day)
- Clinical trial interim analysis — early stopping rules analogous to predictive budget alerts
- Industrial process control — real-time anomaly detection on sensor data

---

## 6. Multi-Agent Communication Protocols

**Leverage: HIGH**

**Code anchors:**
- `internal/fleet/server.go` — HTTP coordinator with register/heartbeat/poll/complete endpoints, no peer-to-peer
- `internal/fleet/queue.go` — single-coordinator work queue, coordinator is a single point of failure
- `internal/session/contextstore.go` — per-session file tracking, no shared semantic state
- `.ralph/tool_improvement_scratchpad.md` items 1-3 — worktree agent merge pain points

**Limitation:** Fleet coordination is hub-and-spoke (coordinator → workers). Workers cannot communicate with each other, share intermediate results, or negotiate task boundaries. The worktree merge pain points (scratchpad items 1-3) are a direct symptom: agents working in parallel cannot coordinate their file modifications.

**Research questions:**
1. How does Google's A2A (Agent-to-Agent) protocol compare to MCP for agent-to-agent delegation? MCP is tool-oriented (expose/consume tools); A2A is task-oriented (delegate/negotiate tasks). What would an A2A adapter in the fleet package look like?
2. Is a blackboard architecture (shared structured state that agents read/write) better than message-passing for the worktree coordination problem? What data structure minimizes write contention when N agents modify overlapping files?
3. What consensus protocol is appropriate for a 2-10 node fleet where the coordinator could fail? Full Raft is heavyweight — what about a simpler leader-election scheme using Tailscale peer discovery as the membership oracle?
4. How should MCP Elicitation (the bidirectional negotiation extension in the MCP spec) be used for workers to request clarification from the planner mid-task?
5. What conflict resolution strategy handles the worktree merge pain points when N agents modify overlapping files? Three-way merge with semantic awareness? CRDT-based concurrent editing? Lock-based file reservation?

**Keyword clusters:**
- `A2A protocol Google agent to agent`
- `MCP elicitation bidirectional negotiation`
- `blackboard architecture multi-agent`
- `lightweight consensus leader election`
- `AGNTCY interoperability standard`
- `CRDT conflict resolution code merge`

**Venues:**
- AAMAS (Autonomous Agents and Multi-Agent Systems conference)
- AAAI (multi-agent coordination track)
- Google DeepMind A2A specification repository
- MCP specification (modelcontextprotocol.io) — Elicitation extension
- Distributed systems conferences (SOSP, OSDI) — consensus and coordination

**Adjacent fields:**
- Distributed version control merge strategies — git's three-way merge, semantic merge tools
- Operational transform (OT) — Google Docs' approach to concurrent editing
- CRDT-based collaboration — Automerge, Yjs patterns for conflict-free concurrent state

---

## 7. Adjacent Fields to Investigate

Domains not referenced in the platform research document that likely contain directly applicable prior art:

### 7.1 Reinforcement Learning for System Configuration
**Why relevant:** The self-learning subsystems (cascade thresholds, curriculum weights, provider selection) are essentially system configuration parameters that should be tuned online. The RL for systems community has solved analogous problems.
**Search entry points:** "learned index structures", "RL for database tuning", "adaptive system configuration", "auto-tuning distributed systems"
**Key distinction:** These systems operate at similar scale (~hundreds of decisions/day) and have similar constraints (must be lightweight, no GPU).

### 7.2 Mixture-of-Experts Routing
**Why relevant:** The cascade router selects ONE provider per task. MoE architectures route different parts of a computation to different experts. Applied to fleet routing: different subtasks of a complex task could be routed to different providers based on their strengths.
**Search entry points:** "mixture of experts routing", "task decomposition multi-model", "expert selection gating network"
**Key distinction:** Standard MoE operates within a single model; here the "experts" are external API endpoints with different pricing and latency characteristics.

### 7.3 Federated Learning for Fleet-Wide Model Updates
**Why relevant:** Each worker node accumulates local observations. Currently these are forwarded as events to the coordinator. Federated learning could train the confidence/routing models locally and aggregate without shipping raw data.
**Search entry points:** "federated learning edge devices", "federated bandit", "distributed online learning"
**Key distinction:** The fleet is small (2-10 nodes) and trusted, so privacy is not the motivation — bandwidth and latency are.

### 7.4 AutoML for Pipeline Configuration
**Why relevant:** The prompt enhancer has 13 stages with configurable parameters. The self-learning subsystems have 15+ hardcoded weights. AutoML techniques could tune these jointly.
**Search entry points:** "AutoML pipeline optimization", "hyperparameter optimization black-box", "SMAC sequential model-based configuration"
**Key distinction:** The search space is moderate (~30 continuous parameters) and evaluations are expensive (~$0.10-$1.00 per evaluation). Bayesian optimization with GP surrogate is likely the right approach.

### 7.5 Debate and Constitutional AI for Fleet Governance
**Why relevant:** When multiple agents produce conflicting results (e.g., different code solutions for the same task), the system currently has no mechanism to adjudicate. Debate protocols (run two agents adversarially, have a judge pick the winner) and constitutional AI patterns (check outputs against principles) could provide governance.
**Search entry points:** "AI debate protocol", "constitutional AI", "adversarial verification LLM", "LLM judge evaluation"
**Key distinction:** The "judge" could be a cheap model (Gemini Flash-Lite at $0.10/1M) evaluating expensive outputs — leveraging the cost asymmetry between generation and verification.

---

## 8. Cross-Cutting Dependencies

| Theme | Sections | Shared Dependency |
|-------|----------|-------------------|
| Learning replaces heuristics | 1.1, 1.2 | `FeedbackAnalyzer` + `LoopObservation` as training data |
| Fleet becomes intelligent | 1.1, 6, 5 | `fleet/optimizer.go` + `fleet/queue.go` need `BanditPolicy` interface |
| Evaluation enables everything | 4 | Must exist before 1.1, 1.2, 1.3, 5 can be validated |
| Context flows across sessions | 6, 1.4 | `ContextStore` + episodic memory + Files API convergence |
| Governance at scale | 7.5, 6 | Multi-agent adjudication requires communication protocol |

---

## 9. Implementation Sequencing

Phased plan that respects dependencies (continues from Phase E in the platform research doc):

### Phase F: Evaluation Foundation
- Section 4 — Counterfactual logging + Bayesian A/B testing framework
- **Why first:** Without measurement, no other improvement can be validated
- **Estimated scope:** ~500 lines, new `internal/eval/` package
- **Go libraries:** `gonum/stat` (distributions), `gonum/optimize` (Bayesian optimization)

### Phase G: Core Learning (parallel tracks)
- Section 1.1 — Multi-armed bandit provider selection (`BanditPolicy` interface)
- Section 1.2 — Calibrated confidence model (logistic regression on observation data)
- **Why together:** Both consume the same training data (`LoopObservation` JSONL + `FeedbackAnalyzer` profiles)
- **Estimated scope:** ~800 lines total, modifications to `cascade.go`, `optimizer.go`, new `internal/bandit/`
- **Go libraries:** `gonum/stat` (Beta distribution for Thompson Sampling), `gonum/mat` (logistic regression)

### Phase H: Fleet Intelligence
- Section 6 — A2A/blackboard protocol adapter in fleet package
- Section 5 — Predictive cost modeling + cache-aware scheduling in costnorm
- **Prerequisite:** Phase G (routing must be adaptive before fleet coordination improves)
- **Estimated scope:** ~1200 lines, modifications to `fleet/`, `costnorm.go`

### Phase I: Advanced (independent tracks)
- Section 1.3 — Speculative execution in cascade router
- Section 5 — Anomaly detection in observation pipeline
- Section 1.4 — Embedding-based episodic retrieval
- **Each can proceed independently after Phase F is complete**

---

## 10. Search Strategy Appendix

### Author Watchlist
- **Shunyu Yao** — Reflexion, ReAct (agent reasoning patterns)
- **Noah Shinn** — Reflexion (iterative refinement for agents)
- **Lilian Weng** — LLM agent surveys (comprehensive overviews)
- **Google DeepMind A2A team** — Agent-to-Agent protocol specification
- **Shipra Agrawal** — Thompson Sampling theory (Columbia)
- **Csaba Szepesvari** — Bandit algorithms (DeepMind, "Bandit Algorithms" textbook)
- **Nando de Freitas** — Bayesian optimization (foundational work)

### Industry Blog Watchlist
- Anthropic engineering blog — Claude API updates, agent patterns
- Google Cloud AI blog — Gemini API updates, A2A protocol
- OpenAI research blog — Responses API, agent patterns
- Uber ML engineering — Multi-armed bandits in production
- Netflix engineering — Contextual bandits, counterfactual evaluation
- DoorDash engineering — Routing under uncertainty
- Spotify engineering — Bayesian A/B testing

### Go Library Candidates

| Section | Library | Purpose |
|---------|---------|---------|
| 1.1, 1.2 | `gonum/stat` | Beta distributions, statistical tests |
| 1.1, 1.2 | `gonum/optimize` | Bayesian optimization, function minimization |
| 1.2 | `gonum/mat` | Matrix ops for logistic regression |
| 1.4 | `onnxruntime-go` | Local embedding inference without GPU |
| 4 | `gonum/stat/distuv` | Bayesian A/B testing distributions |
| 5 | `gonum/floats` | Sliding window statistics, z-scores |
| 6 | Built-in `net/http` + Tailscale | A2A protocol adapter |
