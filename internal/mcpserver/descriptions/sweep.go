package descriptions

const (
	DescRalphglassesSweepGenerate = "Generate an optimized audit prompt using the 13-stage enhancer pipeline. Returns enhanced prompt text, quality score, and stages applied."
	DescRalphglassesSweepLaunch   = "Launch an enhanced prompt against multiple repos as parallel sessions. Returns a sweep_id for tracking all sessions as a group."
	DescRalphglassesSweepStatus   = "Dashboard for a sweep: per-repo status, total cost, completion percentage, stalled sessions, and optional output tails."
	DescRalphglassesSweepNudge    = "Detect and restart stalled sessions in a sweep. Identifies sessions idle beyond threshold and restarts them with the same prompt."
	DescRalphglassesSweepSchedule = "Set up recurring status checks for a sweep at configurable intervals. Optionally auto-nudges stalled sessions. Returns a task_id for cancellation."
	DescRalphglassesSweepReport   = "Generate a Markdown or JSON summary of a completed sweep: per-repo status, commits, costs, and changes."
	DescRalphglassesSweepRetry    = "Re-launch only failed or errored sessions from a sweep, preserving all original settings."
	DescRalphglassesSweepPush     = "Push all repos in a sweep that have unpushed commits to their remote."
)
