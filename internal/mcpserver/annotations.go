package mcpserver

import "github.com/mark3labs/mcp-go/mcp"

// boolPtr returns a pointer to a bool value (helper for ToolAnnotation fields).
func boolPtr(b bool) *bool { return &b }

// ToolAnnotations maps tool names to their MCP annotation hints.
// These describe behavioral properties per the MCP spec:
//   - ReadOnlyHint: tool does not modify its environment
//   - DestructiveHint: tool may perform destructive updates
//   - IdempotentHint: repeated calls with same args have no additional effect
//   - OpenWorldHint: tool interacts with external entities
var ToolAnnotations = map[string]mcp.ToolAnnotation{
	// ── core ──────────────────────────────────────────────────────────────
	"ralphglasses_scan":        {Title: "Scan Repos", ReadOnlyHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_list":        {Title: "List Repos", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_status":      {Title: "Repo Status", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_start":       {Title: "Start Loop", DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_stop":        {Title: "Stop Loop", DestructiveHint: boolPtr(true)},
	"ralphglasses_stop_all":    {Title: "Stop All Loops", DestructiveHint: boolPtr(true)},
	"ralphglasses_pause":       {Title: "Pause/Resume Loop", IdempotentHint: boolPtr(true)},
	"ralphglasses_logs":        {Title: "View Logs", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_config":      {Title: "Get/Set Config", IdempotentHint: boolPtr(true)},
	"ralphglasses_config_bulk": {Title: "Bulk Config", IdempotentHint: boolPtr(true)},

	// ── session ──────────────────────────────────────────────────────────
	"ralphglasses_session_launch":   {Title: "Launch Session", OpenWorldHint: boolPtr(true)},
	"ralphglasses_session_list":     {Title: "List Sessions", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_session_status":   {Title: "Session Status", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_session_resume":   {Title: "Resume Session", OpenWorldHint: boolPtr(true)},
	"ralphglasses_session_stop":     {Title: "Stop Session", DestructiveHint: boolPtr(true)},
	"ralphglasses_session_stop_all": {Title: "Stop All Sessions", DestructiveHint: boolPtr(true)},
	"ralphglasses_session_budget":   {Title: "Session Budget", IdempotentHint: boolPtr(true)},
	"ralphglasses_session_retry":    {Title: "Retry Session", OpenWorldHint: boolPtr(true)},
	"ralphglasses_session_output":   {Title: "Session Output", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_session_tail":     {Title: "Tail Session", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_session_diff":     {Title: "Session Diff", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_session_compare":  {Title: "Compare Sessions", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_session_errors":   {Title: "Session Errors", ReadOnlyHint: boolPtr(true)},

	// ── loop ─────────────────────────────────────────────────────────────
	"ralphglasses_loop_start":     {Title: "Start Dev Loop", OpenWorldHint: boolPtr(true)},
	"ralphglasses_loop_status":    {Title: "Loop Status", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_loop_step":      {Title: "Step Loop", OpenWorldHint: boolPtr(true)},
	"ralphglasses_loop_stop":      {Title: "Stop Loop", DestructiveHint: boolPtr(true)},
	"ralphglasses_loop_benchmark": {Title: "Loop Benchmark", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_loop_baseline":  {Title: "Loop Baseline", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_loop_gates":     {Title: "Loop Gates", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_self_test":      {Title: "Self Test", ReadOnlyHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_self_improve":   {Title: "Self Improve", DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_loop_prune":     {Title: "Loop Prune", DestructiveHint: boolPtr(true)},

	// ── prompt ───────────────────────────────────────────────────────────
	"ralphglasses_prompt_analyze":        {Title: "Analyze Prompt", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_prompt_enhance":        {Title: "Enhance Prompt", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_prompt_lint":           {Title: "Lint Prompt", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_prompt_improve":        {Title: "Improve Prompt", ReadOnlyHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_prompt_classify":       {Title: "Classify Prompt", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_prompt_should_enhance": {Title: "Should Enhance", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_prompt_templates":      {Title: "Prompt Templates", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_prompt_template_fill":  {Title: "Fill Template", ReadOnlyHint: boolPtr(true)},

	// ── fleet ────────────────────────────────────────────────────────────
	"ralphglasses_fleet_status":       {Title: "Fleet Status", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_fleet_analytics":    {Title: "Fleet Analytics", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_fleet_submit":       {Title: "Submit to Fleet", OpenWorldHint: boolPtr(true)},
	"ralphglasses_fleet_budget":       {Title: "Fleet Budget", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_fleet_workers":      {Title: "Fleet Workers"},
	"ralphglasses_fleet_dlq":          {Title: "Fleet DLQ", DestructiveHint: boolPtr(false)},
	"ralphglasses_marathon_dashboard": {Title: "Marathon Dashboard", ReadOnlyHint: boolPtr(true)},

	// ── repo ─────────────────────────────────────────────────────────────
	"ralphglasses_repo_health":    {Title: "Repo Health", ReadOnlyHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_repo_optimize":  {Title: "Optimize Repo", DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_repo_scaffold":  {Title: "Scaffold Repo", DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_claudemd_check": {Title: "CLAUDE.md Check", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_snapshot":       {Title: "Snapshot", ReadOnlyHint: boolPtr(true)},

	// ── roadmap ──────────────────────────────────────────────────────────
	"ralphglasses_roadmap_parse":    {Title: "Parse Roadmap", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_roadmap_analyze":  {Title: "Analyze Roadmap", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_roadmap_research": {Title: "Research Roadmap", ReadOnlyHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_roadmap_expand":   {Title: "Expand Roadmap", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_roadmap_export":   {Title: "Export Roadmap", ReadOnlyHint: boolPtr(true)},

	// ── team ─────────────────────────────────────────────────────────────
	"ralphglasses_team_create":   {Title: "Create Team", OpenWorldHint: boolPtr(true)},
	"ralphglasses_team_status":   {Title: "Team Status", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_team_delegate": {Title: "Delegate Task", OpenWorldHint: boolPtr(true)},
	"ralphglasses_agent_define":  {Title: "Define Agent", DestructiveHint: boolPtr(false)},
	"ralphglasses_agent_list":    {Title: "List Agents", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_agent_compose": {Title: "Compose Agents", OpenWorldHint: boolPtr(true)},

	// ── awesome ──────────────────────────────────────────────────────────
	"ralphglasses_awesome_fetch":   {Title: "Fetch Awesome", ReadOnlyHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_awesome_analyze": {Title: "Analyze Awesome", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_awesome_diff":    {Title: "Diff Awesome", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_awesome_report":  {Title: "Awesome Report", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_awesome_sync":    {Title: "Sync Awesome", DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(true)},

	// ── advanced ─────────────────────────────────────────────────────────
	"ralphglasses_rc_status":               {Title: "RC Status", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_rc_send":                 {Title: "RC Send", OpenWorldHint: boolPtr(true)},
	"ralphglasses_rc_read":                 {Title: "RC Read", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_rc_act":                  {Title: "RC Act", DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_event_list":              {Title: "List Events", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_event_poll":              {Title: "Poll Events", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_hitl_score":              {Title: "HITL Score", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_hitl_history":            {Title: "HITL History", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_autonomy_level":          {Title: "Autonomy Level", DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true)},
	"ralphglasses_supervisor_status":       {Title: "Supervisor Status", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_autonomy_decisions":      {Title: "Autonomy Decisions", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_autonomy_override":       {Title: "Override Autonomy", DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true)},
	"ralphglasses_feedback_profiles":       {Title: "Feedback Profiles", ReadOnlyHint: boolPtr(false), IdempotentHint: boolPtr(true)},
	"ralphglasses_provider_recommend":      {Title: "Provider Recommend", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_tool_benchmark":          {Title: "Tool Benchmark", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_journal_read":            {Title: "Read Journal", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_journal_write":           {Title: "Write Journal", DestructiveHint: boolPtr(false)},
	"ralphglasses_journal_prune":           {Title: "Prune Journal", DestructiveHint: boolPtr(true)},
	"ralphglasses_workflow_define":         {Title: "Define Workflow", DestructiveHint: boolPtr(false)},
	"ralphglasses_workflow_run":            {Title: "Run Workflow", OpenWorldHint: boolPtr(true)},
	"ralphglasses_workflow_delete":         {Title: "Delete Workflow", DestructiveHint: boolPtr(true)},
	"ralphglasses_bandit_status":           {Title: "Bandit Status", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_confidence_calibration":  {Title: "Confidence Calibration", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_circuit_reset":           {Title: "Reset Circuit Breaker", DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true)},

	// ── eval ─────────────────────────────────────────────────────────────
	"ralphglasses_eval_counterfactual": {Title: "Counterfactual Eval", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_eval_ab_test":        {Title: "A/B Test Eval", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_eval_changepoints":   {Title: "Changepoint Eval", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_eval_significance":   {Title: "Significance Test", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_anomaly_detect":      {Title: "Anomaly Detection", ReadOnlyHint: boolPtr(true)},

	// ── fleet_h ──────────────────────────────────────────────────────────
	"ralphglasses_blackboard_query": {Title: "Query Blackboard", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_blackboard_put":   {Title: "Put Blackboard", DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true)},
	"ralphglasses_a2a_offers":       {Title: "A2A Offers", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_cost_forecast":    {Title: "Cost Forecast", ReadOnlyHint: boolPtr(true)},

	// ── meta ─────────────────────────────────────────────────────────────
	"ralphglasses_tool_groups":    {Title: "Tool Groups", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_load_tool_group": {Title: "Load Tool Group", ReadOnlyHint: boolPtr(true)},

	// ── observability ────────────────────────────────────────────────────
	"ralphglasses_observation_query":   {Title: "Query Observations", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_observation_summary": {Title: "Observation Summary", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_scratchpad_read":     {Title: "Read Scratchpad", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_scratchpad_append":   {Title: "Append Scratchpad", DestructiveHint: boolPtr(false)},
	"ralphglasses_scratchpad_list":     {Title: "List Scratchpads", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_scratchpad_resolve":  {Title: "Resolve Scratchpad", DestructiveHint: boolPtr(false)},
	"ralphglasses_scratchpad_delete":   {Title: "Delete Scratchpad Finding", DestructiveHint: boolPtr(true)},
	"ralphglasses_scratchpad_validate": {Title: "Validate Scratchpad", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_scratchpad_context":  {Title: "Scratchpad Context", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_scratchpad_reason":   {Title: "Scratchpad Reason", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_loop_await":          {Title: "Await Loop", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_loop_poll":           {Title: "Poll Loop", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_coverage_report":     {Title: "Coverage Report", ReadOnlyHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_cost_estimate":       {Title: "Cost Estimate", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_merge_verify":        {Title: "Merge Verify", ReadOnlyHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_worktree_create":     {Title: "Worktree Create"},
	"ralphglasses_worktree_cleanup":    {Title: "Worktree Cleanup", DestructiveHint: boolPtr(true)},

	// ── rdcycle ──────────────────────────────────────────────────────────
	"ralphglasses_finding_to_task": {Title: "Finding to Task", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_cycle_baseline":  {Title: "Cycle Baseline", ReadOnlyHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_cycle_plan":      {Title: "Cycle Plan", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_cycle_merge":     {Title: "Cycle Merge", DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_cycle_schedule":  {Title: "Cycle Schedule", DestructiveHint: boolPtr(false)},

	// ── rdcycle tier 2 ──────────────────────────────────────────────────
	"ralphglasses_loop_replay":           {Title: "Loop Replay", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_budget_forecast":       {Title: "Budget Forecast", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_diff_review":           {Title: "Diff Review", ReadOnlyHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_finding_reason":        {Title: "Finding Reason", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_observation_correlate": {Title: "Observation Correlate", ReadOnlyHint: boolPtr(true), OpenWorldHint: boolPtr(true)},

	// ── cycle engine (state machine) ────────────────────────────────────
	"ralphglasses_cycle_create":     {Title: "Cycle Create", DestructiveHint: boolPtr(false)},
	"ralphglasses_cycle_advance":    {Title: "Cycle Advance", DestructiveHint: boolPtr(false)},
	"ralphglasses_cycle_status":     {Title: "Cycle Status", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_cycle_fail":       {Title: "Cycle Fail", DestructiveHint: boolPtr(false)},
	"ralphglasses_cycle_list":       {Title: "Cycle List", ReadOnlyHint: boolPtr(true)},
	"ralphglasses_cycle_synthesize": {Title: "Cycle Synthesize", DestructiveHint: boolPtr(false)},
	"ralphglasses_cycle_run":        {Title: "Cycle Run", DestructiveHint: boolPtr(false)},
}

// GetAnnotation returns the ToolAnnotation for a named tool, or an empty
// annotation if no entry exists.
func GetAnnotation(toolName string) mcp.ToolAnnotation {
	if a, ok := ToolAnnotations[toolName]; ok {
		return a
	}
	return mcp.ToolAnnotation{}
}
