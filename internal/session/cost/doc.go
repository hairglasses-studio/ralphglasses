// Package cost implements cost tracking, budget enforcement, and FinOps
// for LLM sessions.
//
// It provides the cost engine (real-time cost accumulation from provider
// token streams), cost ledger (persistent audit trail), budget pools
// (shared budgets across teams), budget federation (hierarchical budget
// delegation), spend monitoring, anomaly detection, cost-aware routing,
// and retry budget management.
//
// Cost data flows in from provider adapters via normalized cost events,
// and flows out to the supervisor and TUI via the cost ledger and
// spend monitor interfaces.
package cost
