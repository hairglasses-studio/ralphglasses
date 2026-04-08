package descriptions

const (
	DescRalphglassesSessionTriage   = "Triage killed/interrupted sessions across all repos within a time window. Groups by kill reason, repo, provider. Shows cost wasted and recovery potential."
	DescRalphglassesSessionSalvage  = "Extract partial output from a killed session, classify what was accomplished vs lost, and generate a recovery task prompt."
	DescRalphglassesRecoveryPlan    = "Generate a prioritized recovery plan from killed sessions. Categorizes each: retry (transient error), salvage-and-close (non-recoverable), or escalate (unclear). Respects budget cap."
	DescRalphglassesRecoveryExecute = "Execute a recovery plan: batch re-launch retry sessions in parallel, tracked as a sweep with budget cap."
	DescRalphglassesIncidentReport  = "Generate a structured incident report in docs/research/incidents/. Includes timeline, affected sessions, salvaged outputs, recovery actions taken, and lessons learned."
	DescRalphglassesSessionDiscover = "Scan all repos' .ralph/ directories and Claude Code project dirs to discover session state beyond the local store. Finds orphaned processes and external session files."
)
