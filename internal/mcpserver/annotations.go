package mcpserver

// ToolAnnotation describes behavioral metadata for an MCP tool.
type ToolAnnotation struct {
	ReadOnly    bool // Tool only reads state, no side effects
	Destructive bool // Tool can destroy or stop resources
	Idempotent  bool // Safe to call multiple times with same args
	OpenWorld   bool // Tool interacts with external systems (network, processes)
}

// ToolAnnotations maps tool names to their behavioral annotations.
var ToolAnnotations = map[string]ToolAnnotation{
	// Core tools
	"ralphglasses_scan":            {ReadOnly: true, Idempotent: true, OpenWorld: true},
	"ralphglasses_list":            {ReadOnly: true, Idempotent: true},
	"ralphglasses_status":          {ReadOnly: true, Idempotent: true},
	"ralphglasses_start":           {OpenWorld: true},
	"ralphglasses_stop":            {Destructive: true, OpenWorld: true},
	"ralphglasses_stop_all":        {Destructive: true, OpenWorld: true},
	"ralphglasses_pause":           {OpenWorld: true},
	"ralphglasses_logs":            {ReadOnly: true, Idempotent: true},
	"ralphglasses_config":          {ReadOnly: true, Idempotent: true},
	"ralphglasses_config_bulk":     {Idempotent: true},
	"ralphglasses_snapshot":        {ReadOnly: true, Idempotent: true},
	"ralphglasses_tool_groups":     {ReadOnly: true, Idempotent: true},
	"ralphglasses_load_tool_group": {Idempotent: true},

	// Session tools
	"ralphglasses_session_launch":   {OpenWorld: true},
	"ralphglasses_session_list":     {ReadOnly: true, Idempotent: true},
	"ralphglasses_session_status":   {ReadOnly: true, Idempotent: true},
	"ralphglasses_session_resume":   {OpenWorld: true},
	"ralphglasses_session_stop":     {Destructive: true, OpenWorld: true},
	"ralphglasses_session_stop_all": {Destructive: true, OpenWorld: true},
	"ralphglasses_session_budget":   {ReadOnly: true, Idempotent: true},
	"ralphglasses_session_retry":    {OpenWorld: true},
	"ralphglasses_session_output":   {ReadOnly: true, Idempotent: true},
	"ralphglasses_session_tail":     {ReadOnly: true, Idempotent: true},
	"ralphglasses_session_diff":     {ReadOnly: true, Idempotent: true},
	"ralphglasses_session_compare":  {ReadOnly: true, Idempotent: true},
	"ralphglasses_session_errors":   {ReadOnly: true, Idempotent: true},

	// Loop tools
	"ralphglasses_loop_start":     {OpenWorld: true},
	"ralphglasses_loop_status":    {ReadOnly: true, Idempotent: true},
	"ralphglasses_loop_step":      {OpenWorld: true},
	"ralphglasses_loop_stop":      {Destructive: true, OpenWorld: true},
	"ralphglasses_loop_benchmark": {ReadOnly: true, Idempotent: true},
	"ralphglasses_loop_baseline":  {Idempotent: true},
	"ralphglasses_loop_gates":     {ReadOnly: true, Idempotent: true},
	"ralphglasses_loop_await":     {ReadOnly: true},
	"ralphglasses_loop_poll":      {ReadOnly: true, Idempotent: true},
	"ralphglasses_self_test":      {OpenWorld: true, Idempotent: true},
	"ralphglasses_self_improve":   {OpenWorld: true},

	// Prompt tools
	"ralphglasses_prompt_analyze":        {ReadOnly: true, Idempotent: true},
	"ralphglasses_prompt_enhance":        {ReadOnly: true, Idempotent: true},
	"ralphglasses_prompt_lint":           {ReadOnly: true, Idempotent: true},
	"ralphglasses_prompt_improve":        {OpenWorld: true, Idempotent: true}, // calls LLM API
	"ralphglasses_prompt_classify":       {ReadOnly: true, Idempotent: true},
	"ralphglasses_prompt_should_enhance": {ReadOnly: true, Idempotent: true},
	"ralphglasses_prompt_templates":      {ReadOnly: true, Idempotent: true},
	"ralphglasses_prompt_template_fill":  {ReadOnly: true, Idempotent: true},

	// Fleet tools
	"ralphglasses_fleet_status":       {ReadOnly: true, Idempotent: true},
	"ralphglasses_fleet_analytics":    {ReadOnly: true, Idempotent: true},
	"ralphglasses_fleet_submit":       {OpenWorld: true},
	"ralphglasses_fleet_budget":       {Idempotent: true},
	"ralphglasses_fleet_workers":      {ReadOnly: true, Idempotent: true},
	"ralphglasses_marathon_dashboard": {ReadOnly: true, Idempotent: true},

	// Repo tools
	"ralphglasses_repo_health":    {ReadOnly: true, Idempotent: true, OpenWorld: true},
	"ralphglasses_repo_optimize":  {ReadOnly: true, Idempotent: true},
	"ralphglasses_repo_scaffold":  {OpenWorld: true},
	"ralphglasses_claudemd_check": {ReadOnly: true, Idempotent: true},

	// Roadmap tools
	"ralphglasses_roadmap_parse":    {ReadOnly: true, Idempotent: true},
	"ralphglasses_roadmap_analyze":  {ReadOnly: true, Idempotent: true},
	"ralphglasses_roadmap_research": {ReadOnly: true, Idempotent: true, OpenWorld: true},
	"ralphglasses_roadmap_expand":   {Idempotent: true},
	"ralphglasses_roadmap_export":   {ReadOnly: true, Idempotent: true},

	// Team tools
	"ralphglasses_team_create":    {OpenWorld: true},
	"ralphglasses_team_status":    {ReadOnly: true, Idempotent: true},
	"ralphglasses_team_delegate":  {OpenWorld: true},
	"ralphglasses_agent_define":   {Idempotent: true},
	"ralphglasses_agent_list":     {ReadOnly: true, Idempotent: true},
	"ralphglasses_agent_compose":  {OpenWorld: true},

	// Awesome tools
	"ralphglasses_awesome_fetch":   {ReadOnly: true, OpenWorld: true},
	"ralphglasses_awesome_analyze": {ReadOnly: true, Idempotent: true},
	"ralphglasses_awesome_diff":    {ReadOnly: true, Idempotent: true},
	"ralphglasses_awesome_report":  {ReadOnly: true, Idempotent: true},
	"ralphglasses_awesome_sync":    {OpenWorld: true},

	// Advanced tools (RC, events, HITL, autonomy, etc.)
	"ralphglasses_rc_status":               {ReadOnly: true, Idempotent: true},
	"ralphglasses_rc_send":                 {OpenWorld: true},
	"ralphglasses_rc_read":                 {ReadOnly: true, Idempotent: true},
	"ralphglasses_rc_act":                  {OpenWorld: true},
	"ralphglasses_event_list":              {ReadOnly: true, Idempotent: true},
	"ralphglasses_event_poll":              {ReadOnly: true},
	"ralphglasses_hitl_score":              {ReadOnly: true, Idempotent: true},
	"ralphglasses_hitl_history":            {ReadOnly: true, Idempotent: true},
	"ralphglasses_autonomy_level":          {ReadOnly: true, Idempotent: true},
	"ralphglasses_autonomy_decisions":      {ReadOnly: true, Idempotent: true},
	"ralphglasses_autonomy_override":       {OpenWorld: true},
	"ralphglasses_feedback_profiles":       {ReadOnly: true, Idempotent: true},
	"ralphglasses_provider_recommend":      {ReadOnly: true, Idempotent: true},
	"ralphglasses_tool_benchmark":          {ReadOnly: true, Idempotent: true},
	"ralphglasses_journal_read":            {ReadOnly: true, Idempotent: true},
	"ralphglasses_journal_write":           {},
	"ralphglasses_journal_prune":           {Destructive: true},
	"ralphglasses_workflow_define":         {Idempotent: true},
	"ralphglasses_workflow_run":            {OpenWorld: true},
	"ralphglasses_bandit_status":           {ReadOnly: true, Idempotent: true},
	"ralphglasses_confidence_calibration":  {ReadOnly: true, Idempotent: true},

	// Eval tools
	"ralphglasses_eval_counterfactual": {ReadOnly: true, Idempotent: true},
	"ralphglasses_eval_ab_test":        {ReadOnly: true, Idempotent: true},
	"ralphglasses_eval_changepoints":   {ReadOnly: true, Idempotent: true},

	// Fleet-H tools
	"ralphglasses_anomaly_detect": {ReadOnly: true, Idempotent: true},

	// Observability tools
	"ralphglasses_blackboard_query":    {ReadOnly: true, Idempotent: true},
	"ralphglasses_blackboard_put":      {},
	"ralphglasses_a2a_offers":          {ReadOnly: true, Idempotent: true, OpenWorld: true},
	"ralphglasses_cost_forecast":       {ReadOnly: true, Idempotent: true},
	"ralphglasses_observation_query":   {ReadOnly: true, Idempotent: true},
	"ralphglasses_observation_summary": {ReadOnly: true, Idempotent: true},
	"ralphglasses_coverage_report":     {ReadOnly: true, Idempotent: true, OpenWorld: true},
	"ralphglasses_cost_estimate":       {ReadOnly: true, Idempotent: true},
	"ralphglasses_merge_verify":        {ReadOnly: true, OpenWorld: true},

	// Scratchpad tools
	"ralphglasses_scratchpad_read":    {ReadOnly: true, Idempotent: true},
	"ralphglasses_scratchpad_append":  {},
	"ralphglasses_scratchpad_list":    {ReadOnly: true, Idempotent: true},
	"ralphglasses_scratchpad_resolve": {Idempotent: true},
}

// GetAnnotation returns the annotation for a tool, defaulting to read-only if not mapped.
func GetAnnotation(toolName string) ToolAnnotation {
	if a, ok := ToolAnnotations[toolName]; ok {
		return a
	}
	return ToolAnnotation{ReadOnly: true, Idempotent: true}
}
