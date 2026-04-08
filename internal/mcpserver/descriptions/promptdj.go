package descriptions

const (
	DescRalphglassesPromptdjRoute    = "Route a prompt to the best provider based on quality, task type, and domain. Does NOT launch a session."
	DescRalphglassesPromptdjDispatch = "Route AND launch a session. Optionally enhances prompt if quality is low."
	DescRalphglassesPromptdjFeedback = "Record outcome feedback to improve routing over time."
	DescRalphglassesPromptdjSimilar  = "Find similar high-quality prompts from the registry for few-shot context injection. Uses BM25-lite keyword similarity, Jaccard tag overlap, and MMR diversity re-ranking."
	DescRalphglassesPromptdjSuggest  = "Get routing-aware improvement suggestions for a prompt. Shows where it would route, quality score, and actionable suggestions by category (quality, structure, cost, provider_fit)."
	DescRalphglassesPromptdjHistory  = "View routing decision history with optional summary. Filter by repo, provider, task type, status, and time window."
)
