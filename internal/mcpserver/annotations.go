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
	"ralphglasses_scan":        {Title: "Scan Repos", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_list":        {Title: "List Repos", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_status":      {Title: "Repo Status", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_start":       {Title: "Start Loop", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_stop":        {Title: "Stop Loop", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(true), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_stop_all":    {Title: "Stop All Loops", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(true), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_pause":       {Title: "Pause/Resume Loop", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_logs":        {Title: "View Logs", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_config":      {Title: "Get/Set Config", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_config_bulk": {Title: "Bulk Config", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},

	// ── session ──────────────────────────────────────────────────────────
	"ralphglasses_session_launch":      {Title: "Launch Session", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_session_list":        {Title: "List Sessions", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_session_status":      {Title: "Session Status", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_session_resume":      {Title: "Resume Session", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_session_stop":        {Title: "Stop Session", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(true), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_session_stop_all":    {Title: "Stop All Sessions", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(true), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_session_budget":      {Title: "Session Budget", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_session_retry":       {Title: "Retry Session", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_session_output":      {Title: "Session Output", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_session_tail":        {Title: "Tail Session", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_session_diff":        {Title: "Session Diff", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_session_compare":     {Title: "Compare Sessions", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_session_errors":      {Title: "Session Errors", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_session_export":      {Title: "Export Session", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_session_fork":        {Title: "Fork Session", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_session_handoff":     {Title: "Session Handoff", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_session_replay_diff": {Title: "Replay Diff", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},

	// ── tasks ────────────────────────────────────────────────────────────
	"ralphglasses_tasks_get":    {Title: "Get Task", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_tasks_list":   {Title: "List Tasks", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_tasks_cancel": {Title: "Cancel Task", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(true), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},

	// ── loop ─────────────────────────────────────────────────────────────
	"ralphglasses_loop_start":     {Title: "Start Dev Loop", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_loop_status":    {Title: "Loop Status", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_loop_step":      {Title: "Step Loop", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_loop_stop":      {Title: "Stop Loop", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(true), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_loop_benchmark": {Title: "Loop Benchmark", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_loop_baseline":  {Title: "Loop Baseline", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_loop_gates":     {Title: "Loop Gates", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_self_test":      {Title: "Self Test", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_self_improve":   {Title: "Self Improve", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_loop_prune":     {Title: "Loop Prune", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(true), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(false)},

	// ── prompt ───────────────────────────────────────────────────────────
	"ralphglasses_prompt_analyze":        {Title: "Analyze Prompt", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_prompt_ab_test":        {Title: "Prompt A/B Test", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_prompt_enhance":        {Title: "Enhance Prompt", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_prompt_lint":           {Title: "Lint Prompt", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_prompt_improve":        {Title: "Improve Prompt", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_prompt_classify":       {Title: "Classify Prompt", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_prompt_should_enhance": {Title: "Should Enhance", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_prompt_templates":      {Title: "Prompt Templates", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_prompt_template_fill":  {Title: "Fill Template", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},

	// ── sweep ────────────────────────────────────────────────────────────
	"ralphglasses_sweep_generate": {Title: "Generate Sweep Prompt", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_sweep_launch":   {Title: "Launch Sweep", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_sweep_status":   {Title: "Sweep Status", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_sweep_nudge":    {Title: "Nudge Sweep", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_sweep_schedule": {Title: "Schedule Sweep Monitor", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_sweep_report":   {Title: "Sweep Report", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_sweep_retry":    {Title: "Retry Sweep", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_sweep_push":     {Title: "Push Sweep Repos", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},

	// ── fleet ────────────────────────────────────────────────────────────
	"ralphglasses_fleet_status":        {Title: "Fleet Status", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_fleet_analytics":     {Title: "Fleet Analytics", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_fleet_submit":        {Title: "Submit to Fleet", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_fleet_budget":        {Title: "Fleet Budget", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_fleet_capacity_plan": {Title: "Fleet Capacity Plan", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_fleet_workers":       {Title: "Fleet Workers", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_fleet_dlq":           {Title: "Fleet DLQ", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_marathon_dashboard":  {Title: "Marathon Dashboard", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},

	// ── repo ─────────────────────────────────────────────────────────────
	"ralphglasses_repo_health":    {Title: "Repo Health", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_repo_optimize":  {Title: "Optimize Repo", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_repo_scaffold":  {Title: "Scaffold Repo", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_claudemd_check": {Title: "CLAUDE.md Check", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_snapshot":       {Title: "Snapshot", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},

	// ── roadmap ──────────────────────────────────────────────────────────
	"ralphglasses_roadmap_parse":      {Title: "Parse Roadmap", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_roadmap_analyze":    {Title: "Analyze Roadmap", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_roadmap_research":   {Title: "Research Roadmap", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_roadmap_expand":     {Title: "Expand Roadmap", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_roadmap_export":     {Title: "Export Roadmap", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_roadmap_prioritize": {Title: "Prioritize Roadmap", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},

	// ── team ─────────────────────────────────────────────────────────────
	"ralphglasses_team_create":   {Title: "Create Team", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_team_status":   {Title: "Team Status", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_team_delegate": {Title: "Delegate Task", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_agent_define":  {Title: "Define Agent", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_agent_list":    {Title: "List Agents", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_agent_compose": {Title: "Compose Agents", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},

	// ── awesome ──────────────────────────────────────────────────────────
	"ralphglasses_awesome_fetch":   {Title: "Fetch Awesome", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_awesome_analyze": {Title: "Analyze Awesome", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_awesome_diff":    {Title: "Diff Awesome", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_awesome_report":  {Title: "Awesome Report", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_awesome_sync":    {Title: "Sync Awesome", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},

	// ── advanced ─────────────────────────────────────────────────────────
	"ralphglasses_rc_status":              {Title: "RC Status", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_rc_send":                {Title: "RC Send", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_rc_read":                {Title: "RC Read", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_rc_act":                 {Title: "RC Act", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_event_list":             {Title: "List Events", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_event_poll":             {Title: "Poll Events", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(false)},
	"ralphglasses_hitl_score":             {Title: "HITL Score", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_hitl_history":           {Title: "HITL History", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_autonomy_level":         {Title: "Autonomy Level", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_supervisor_status":      {Title: "Supervisor Status", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_autonomy_decisions":     {Title: "Autonomy Decisions", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_autonomy_override":      {Title: "Override Autonomy", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_feedback_profiles":      {Title: "Feedback Profiles", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_provider_benchmark":     {Title: "Provider Benchmark", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_provider_recommend":     {Title: "Provider Recommend", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_tool_benchmark":         {Title: "Tool Benchmark", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_journal_read":           {Title: "Read Journal", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_journal_write":          {Title: "Write Journal", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(false)},
	"ralphglasses_journal_prune":          {Title: "Prune Journal", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(true), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(false)},
	"ralphglasses_workflow_define":        {Title: "Define Workflow", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_workflow_run":           {Title: "Run Workflow", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_workflow_delete":        {Title: "Delete Workflow", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(true), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_bandit_status":          {Title: "Bandit Status", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_confidence_calibration": {Title: "Confidence Calibration", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_circuit_reset":          {Title: "Reset Circuit Breaker", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},

	// ── eval ─────────────────────────────────────────────────────────────
	"ralphglasses_eval_counterfactual": {Title: "Counterfactual Eval", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_eval_ab_test":        {Title: "A/B Test Eval", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_eval_changepoints":   {Title: "Changepoint Eval", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_eval_significance":   {Title: "Significance Test", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_anomaly_detect":      {Title: "Anomaly Detection", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},

	// ── fleet_h ──────────────────────────────────────────────────────────
	"ralphglasses_blackboard_query": {Title: "Query Blackboard", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_blackboard_put":   {Title: "Put Blackboard", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_a2a_offers":       {Title: "A2A Offers", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_cost_forecast":    {Title: "Cost Forecast", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},

	// ── meta ─────────────────────────────────────────────────────────────
	"ralphglasses_tool_groups":     {Title: "Tool Groups", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_load_tool_group": {Title: "Load Tool Group", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_skill_export":    {Title: "Skill Export", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},

	// ── observability ────────────────────────────────────────────────────
	"ralphglasses_observation_query":   {Title: "Query Observations", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_observation_summary": {Title: "Observation Summary", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_scratchpad_read":     {Title: "Read Scratchpad", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_scratchpad_append":   {Title: "Append Scratchpad", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(false)},
	"ralphglasses_scratchpad_list":     {Title: "List Scratchpads", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_scratchpad_resolve":  {Title: "Resolve Scratchpad", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_scratchpad_delete":   {Title: "Delete Scratchpad Finding", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(true), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_scratchpad_validate": {Title: "Validate Scratchpad", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_scratchpad_context":  {Title: "Scratchpad Context", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_scratchpad_reason":   {Title: "Scratchpad Reason", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_loop_await":          {Title: "Await Loop", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(false)},
	"ralphglasses_loop_poll":           {Title: "Poll Loop", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(false)},
	"ralphglasses_coverage_report":     {Title: "Coverage Report", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_cost_estimate":       {Title: "Cost Estimate", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_merge_verify":        {Title: "Merge Verify", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_worktree_create":     {Title: "Worktree Create", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_worktree_cleanup":    {Title: "Worktree Cleanup", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(true), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},

	// ── rdcycle ──────────────────────────────────────────────────────────
	"ralphglasses_finding_to_task": {Title: "Finding to Task", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_cycle_baseline":  {Title: "Cycle Baseline", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_cycle_plan":      {Title: "Cycle Plan", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_cycle_merge":     {Title: "Cycle Merge", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_cycle_schedule":  {Title: "Cycle Schedule", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},

	// ── rdcycle tier 2 ──────────────────────────────────────────────────
	"ralphglasses_loop_replay":           {Title: "Loop Replay", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_budget_forecast":       {Title: "Budget Forecast", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_diff_review":           {Title: "Diff Review", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_finding_reason":        {Title: "Finding Reason", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_observation_correlate": {Title: "Observation Correlate", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(true)},

	// ── cycle engine (state machine) ────────────────────────────────────
	"ralphglasses_cycle_create":     {Title: "Cycle Create", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(false)},
	"ralphglasses_cycle_advance":    {Title: "Cycle Advance", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(false)},
	"ralphglasses_cycle_status":     {Title: "Cycle Status", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_cycle_fail":       {Title: "Cycle Fail", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_cycle_list":       {Title: "Cycle List", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_cycle_synthesize": {Title: "Cycle Synthesize", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(false)},
	"ralphglasses_cycle_run":        {Title: "Cycle Run", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},

	// ── plugin ──────────────────────────────────────────────────────────

	// ── eval ─────────────────────────────────────────────────────────────
	"ralphglasses_eval_define": {Title: "Define A/B Test", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},

	// ── cost recommend ──────────────────────────────────────────────────
	"ralphglasses_cost_recommend": {Title: "Cost Recommendations", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_fleet_schedule": {Title: "Fleet Schedule", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_fleet_grafana":  {Title: "Grafana Export", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},

	// ── plugin ──────────────────────────────────────────────────────────
	"ralphglasses_plugin_list":    {Title: "List Plugins", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_plugin_info":    {Title: "Plugin Info", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_plugin_enable":  {Title: "Enable Plugin", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	"ralphglasses_plugin_disable": {Title: "Disable Plugin", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(false)},

	// ── docs ─────────────────────────────────────────────────────────────
	"ralphglasses_docs_search":          {Title: "Search Docs", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_docs_check_existing":  {Title: "Check Existing Docs", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_docs_write_finding":   {Title: "Write Finding", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_docs_push":            {Title: "Push Docs", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
	"ralphglasses_meta_roadmap_status":  {Title: "Meta Roadmap Status", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_meta_roadmap_next_task": {Title: "Meta Roadmap Next Task", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_roadmap_cross_repo":   {Title: "Cross-Repo Roadmap", ReadOnlyHint: boolPtr(true), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(true), OpenWorldHint: boolPtr(true)},
	"ralphglasses_roadmap_assign_loop":  {Title: "Assign Roadmap Loop", ReadOnlyHint: boolPtr(false), DestructiveHint: boolPtr(false), IdempotentHint: boolPtr(false), OpenWorldHint: boolPtr(true)},
}

// GetAnnotation returns the ToolAnnotation for a named tool, or an empty
// annotation if no entry exists.
func GetAnnotation(toolName string) mcp.ToolAnnotation {
	if a, ok := ToolAnnotations[toolName]; ok {
		return a
	}
	return mcp.ToolAnnotation{}
}
