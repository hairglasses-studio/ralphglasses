# Research Bibliography

Academic papers and industry resources informing ralphglasses development.
**Last updated:** 2026-04-03 | **Total papers:** 87 | **Phases covered:** A through E

---

## Multi-Agent LLM Coordination

| ArXiv ID | Title | Key Finding | Phase |
|----------|-------|-------------|-------|
| 2512.08296 | Towards a Science of Scaling Agent Systems | Capability saturation at ~45% baseline; centralized coordination limits error amplification to 4.4x vs 17.2x independent | D3.1 |
| 2510.05174 | Emergent Coordination in Multi-Agent Language Models | Information-theoretic framework for measuring dynamical emergence in multi-agent LLM systems | D2.5 |
| 2508.04652 | LLM Collaboration With MARL (MAGRPO) | Group-relative policy optimization for credit assignment across agents | D3.1 |
| 2511.15755 | Multi-Agent LLM Orchestration for Incident Response | Multi-agent achieves 100% actionable recommendations vs 1.7% single-agent | D3 |

## Autonomous Software Development

| ArXiv ID | Title | Key Finding | Phase |
|----------|-------|-------------|-------|
| 2511.13646 | Live-SWE-agent | Self-evolving scaffold at runtime, 77.4% SWE-bench Verified (SOTA) | D2.2 |
| 2504.15228 | SICA: A Self-Improving Coding Agent | Agent edits its own codebase; 17%→53% improvement on SWE-Bench Verified | D2.2 |
| 2509.16941 | SWE-Bench Pro | Enterprise benchmark: 1,865 problems, 41 repos, tasks requiring hours to days | E1 |
| 2512.10398 | Confucius Code Agent | Three-perspective agent (AX/UX/DX) with persistent note-taking | D3.3 |

## LLM Routing and Model Selection

| ArXiv ID | Title | Key Finding | Phase |
|----------|-------|-------------|-------|
| 2603.04445 | Dynamic Model Routing and Cascading Survey | Unifies routing, cascading, and cascade routing taxonomy | D2.6 |
| 2510.08439 | xRouter | RL-trained cost-aware router on Pareto frontier | D2.6 |
| 2506.09033 | Router-R1 | Sequential think/route with cost reward interleaving | D2.6 |
| 2603.30035 | NeuralUCB Routing | NeuralUCB-based cost-aware routing from partial feedback | D2.6 |
| 2508.21141 | PILOT | LinUCB + multi-choice knapsack for budget-aware routing (EMNLP 2025) | D2.6, D4.4 |
| 2601.04861 | OI-MAS | Confidence-aware state-dependent routing | D2.6 |

## Agent Self-Improvement

| ArXiv ID | Title | Key Finding | Phase |
|----------|-------|-------------|-------|
| 2603.19461 | Hyperagents (Meta AI) | Recursive metacognitive self-modification (DGM-H), meta-improvements transfer across domains | D2.2 |
| 2510.16079 | EvolveR | Offline trajectory distillation into strategic principles + online retrieval | D2.3 |
| 2512.20845 | Multi-Agent Reflexion (MAR) | Multi-perspective critique with judge aggregation, +6.2 on HumanEval | D3.2 |
| 2512.17102 | SAGE: Skill-Augmented Agent | RL framework building reusable skill library for self-improvement | D2.4 |

## Prompt Optimization

| ArXiv ID | Title | Key Finding | Phase |
|----------|-------|-------------|-------|
| 2507.19457 | GEPA (ICLR 2026 Oral) | Reflective evolution outperforms GRPO by 6%, MIPROv2 by 10%+, 35x fewer rollouts | C3 |
| 2512.02840 | promptolution | Unified modular framework for prompt optimization, LLM-agnostic | C3 |
| 2502.16923 | Systematic Survey of Automatic Prompt Optimization | Taxonomy: program-synthesis, evolutionary, gradient-based, LLM-as-optimizer | C3 |

## Cost Optimization

| ArXiv ID | Title | Key Finding | Phase |
|----------|-------|-------------|-------|
| 2511.02755 | CoRL: Centralized Multi-Agent Budget Control | RL dual-reward for performance + cost with economy/balanced/performance modes | D2.6 |
| 2504.15989 | Nano Surge Token Optimization | 24.5-50% token reduction via context awareness + responsibility tuning | C3 |

## Knowledge Graphs for Code

| ArXiv ID | Title | Key Finding | Phase |
|----------|-------|-------------|-------|
| 2505.16901 | Code Graph Model (CGM) | Graph-integrated LLM for repo-level tasks, 43% SWE-bench Lite (SOTA open-source) | E2.2 |
| 2505.14394 | Knowledge Graph Repository-Level Code Generation | KG reduces search space from thousands to 20 candidates; 89.7% need 2+ hops | E2.2 |
| 2602.20478 | Codified Context | Three-tier context infrastructure (hot/warm/cold), 24.2% knowledge-to-code ratio | D3.4, E2.3 |

## Agent Evaluation and Benchmarking

| ArXiv ID | Title | Key Finding | Phase |
|----------|-------|-------------|-------|
| 2601.11868 | Terminal-Bench | 89 hard CLI tasks, frontier models under 65% | E1 |
| 2604.00594 | Agent Psychometrics | Task-level performance prediction via psychometric methods | E1 |

## Distributed Agent Systems

| ArXiv ID | Title | Key Finding | Phase |
|----------|-------|-------------|-------|
| 2601.13671 | Orchestration of Multi-Agent Systems | Unified framework: planning + policy + state + quality ops; MCP + A2A complementary | E1.1 |
| 2601.07526 | MegaFlow | Model/Agent/Environment three-service architecture with independent scaling | E1.1 |
| 2505.02279 | Agent Interoperability Protocols Survey | Comparative analysis of MCP, ACP, A2A, ANP | A2, D2 |
| 2506.12508 | AgentOrchestra (TEA Protocol) | Tool-Environment-Agent separation with dynamic tool instantiation | E1.1 |

## Agent Safety and Sandboxing

| ArXiv ID | Title | Key Finding | Phase |
|----------|-------|-------------|-------|
| 2512.12806 | Fault-Tolerant Sandboxing | Policy-based interception + transactional filesystem snapshots, 100% rollback | D4.2 |
| 2503.18666 | AgentSpec (ICSE 2026) | DSL for runtime safety constraints, 90%+ unsafe execution prevention | D2.2 |
| 2602.17753 | 2025 AI Agent Index | Audit: only 9/30 deployed agents have sandboxing | D4.2 |

## DAG Execution and Workflow

| ArXiv ID | Title | Key Finding | Phase |
|----------|-------|-------------|-------|
| 2601.11816 | POLARIS | Type-checked DAG synthesis with validator-gated repair loops | E1.2 |

---

## Industry Resources and Tools

### Models for Self-Hosting

| Model | Params | License | Key Capability | Integration |
|-------|--------|---------|----------------|-------------|
| Devstral Small 2 | 24B | Apache 2.0 | 68% SWE-bench, fits RTX 4090 | E4.3 (Ollama provider) |
| Qwen3-Coder-30B-A3B | 30B (3B active) | Apache 2.0 | MoE, native function calling, 5.9M downloads | E4.3 (local provider) |
| Qwen2.5-Coder-7B | 7B | Apache 2.0 | 91% HumanEval, speculative decoding draft | E4.3 |
| Qodo-Embed-1-1.5B | 1.5B | Apache 2.0 | SOTA code retrieval (70.06 CoIR) | E2.4 |
| Prometheus 2 | 7B | Apache 2.0 | Open-source LLM judge, highest human agreement | C3 scoring |
| RouteLLM | BERT-class | Apache 2.0 | 85% cost reduction at 95% quality retention | D2.6, E3.2 |
| Jina Embeddings v2 Code | 161M | Apache 2.0 | CPU-runnable code embeddings, GGUF available | E2.4 |

### Infrastructure

| Tool | Purpose | Integration |
|------|---------|-------------|
| Codebase-Memory MCP | Tree-Sitter + SQLite KG, 66 languages, 900+ stars | E2.1 |
| RAUC | A/B OTA updates, battle-tested (Steam Deck) | D1.2 |
| Rugix Ctrl v1.0 | Rust-based OTA for immutable Linux (Feb 2026) | D1.2 (alternative) |
| kubernetes-sigs/agent-sandbox | SandboxTemplate CRD, WarmPool, gVisor | D4.2 |
| llm-d (CNCF Sandbox) | Disaggregated LLM inference on K8s (Mar 2026) | D4 |

### Competitive Intelligence

| Project | Stars | Key Feature | Relationship |
|---------|-------|-------------|-------------|
| Agent Deck | ~2K | Go + BubbleTea, session forking, conductors | Closest competitor |
| Conduit | ~1K | Web UI + TUI parity | Feature we lack (C1) |
| Ruflo | ~500K downloads | 100+ agents, Hive Mind | Validates orchestration pattern |
| OpenCode | ~100K | Go + BubbleTea single-session | Validates tech stack |
| Kagent | CNCF | K8s-native AI agents, MCP tools | Validates D4 direction |

---

## Cross-Cutting Research Themes

### 1. Self-improvement is going recursive
Papers 2511.13646, 2504.15228, 2603.19461, 2510.16079 show agents editing their own scaffolding. Our `self_improve` should become self-modifiable (D2.2).

### 2. Routing must be learned, not heuristic
Papers 2603.04445, 2510.08439, 2506.09033, 2603.30035, 2508.21141 consistently show RL/bandit methods outperform static rules. Replace heuristic routing with NeuralUCB (D2.6).

### 3. Budget awareness as first-class constraint
Papers 2508.21141, 2511.02755, 2504.15989 show cost optimization requires explicit budget constraints in routing, not just post-hoc tracking. Implement knapsack-based allocation (D2.6).

### 4. Topology determines scaling success
Paper 2512.08296 shows naive agent scaling fails for sequential tasks and saturates at ~45% baseline. Fleet dispatch must be topology-aware (D3.1).

### 5. Safety needs formal specification
Papers 2512.12806, 2503.18666, 2602.17753 show ad-hoc guardrails are insufficient. A formal constraint language (AgentSpec-inspired) makes autonomy levels rigorous (D2.2).

### 6. Three-tier memory is the emerging standard
Papers 2602.20478 (Codified Context) and MAGMA's 4-graph schema show tiered memory (hot/warm/cold) is the optimal architecture for agent context (D3.4, E2.3).

### 7. Code graphs dramatically improve agent performance
Papers 2505.16901, 2505.14394 show graph-based code understanding reduces search space from thousands to ~20 candidates, with 89.7% of fixes requiring 2+ graph hops (E2).
