package descriptions

const (
	DescRalphglassesDocsSearch          = "Full-text search across docs/research/ files using ripgrep. Returns matching file paths, line numbers, and previews."
	DescRalphglassesDocsCheckExisting   = "Check if research exists on a topic before starting new work. Searches SEARCH-GUIDE.md and all research files. Returns recommendation: read existing or proceed with new research."
	DescRalphglassesDocsWriteFinding    = "Write a research finding to docs/research/<domain>/<filename>. Validates domain and creates directory if needed."
	DescRalphglassesDocsPush            = "Commit and push all pending changes in the docs repo via push-docs.sh"
	DescRalphglassesMetaRoadmapStatus   = "Parse docs/strategy/META-ROADMAP.md and return phase count, task totals, completion percentage, and summary"
	DescRalphglassesMetaRoadmapNextTask = "Get the next incomplete task from META-ROADMAP.md, optionally filtered by phase name"
	DescRalphglassesRoadmapCrossRepo    = "Compare roadmap progress across all repos using docs/snapshots/roadmaps/. Returns repos sorted by completion (least complete first)."
	DescRalphglassesRoadmapAssignLoop   = "Create an R&D loop targeting a specific roadmap task. Returns loop_start parameters for the task."
)
