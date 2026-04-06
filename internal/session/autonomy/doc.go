// Package autonomy implements graduated autonomy levels, decision
// logging, reflexion loops, episodic memory, and learning transfer
// for self-improving agent sessions.
//
// Autonomy levels range from Observe (level 0, logging only) through
// AutoRecover (level 1, auto-restart on transient errors), AutoOptimize
// (level 2, auto-tuning budgets and providers), to FullAutonomy
// (level 3, self-directed config changes and team scaling).
//
// The reflexion subsystem records agent decisions and outcomes,
// enabling multi-turn self-critique. Episodic and tiered memory
// provide long-term learning across sessions. Curriculum and adaptive
// depth control how aggressively the system explores new strategies.
package autonomy
