// prompt-improver is a CLI tool that enhances prompts with XML structure,
// specificity improvements, and task-type-aware formatting.
//
// Designed to run as a Claude Code UserPromptSubmit hook for automatic
// prompt enhancement, or as a standalone CLI.
//
// Usage:
//
//	echo "fix this bug" | prompt-improver
//	prompt-improver enhance "fix this bug"
//	prompt-improver analyze "fix this bug"
//	prompt-improver template troubleshoot --system resolume --symptoms "clips stuck"
//	prompt-improver templates
//	prompt-improver hook  (reads Claude Code UserPromptSubmit JSON from stdin)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
)

// version is injected at build time via -ldflags.
var version = "dev"

// hybridEngine is initialized once when LLM mode is needed.
var hybridEngine *enhancer.HybridEngine

func main() {
	args := os.Args[1:]

	// If no args and stdin has data, read from stdin (pipe mode)
	if len(args) == 0 {
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading stdin: %v\n", err)
			os.Exit(1)
		}
		raw := strings.TrimSpace(string(input))
		if raw == "" {
			fmt.Fprintln(os.Stderr, "usage: prompt-improver <command> [args] or pipe prompt via stdin")
			os.Exit(1)
		}
		runEnhance(raw, "")
		return
	}

	if err := dispatch(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// dispatch routes command-line args to the appropriate handler.
// Returns an error for usage issues; handlers that need to exit non-zero
// for other reasons (e.g., runHook exit 2) still call os.Exit directly.
func dispatch(args []string) error {
	switch args[0] {
	case "enhance":
		taskType := ""
		mode := ""
		quiet := false
		provider := ""
		targetProvider := ""
		prompt := ""
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--type":
				if i+1 < len(args) {
					taskType = args[i+1]
					i++
				}
			case "--mode":
				if i+1 < len(args) {
					mode = args[i+1]
					i++
				}
			case "--provider":
				if i+1 < len(args) {
					provider = args[i+1]
					i++
				}
			case "--target-provider":
				if i+1 < len(args) {
					targetProvider = args[i+1]
					i++
				}
			case "--quiet", "-q":
				quiet = true
			default:
				if prompt == "" {
					prompt = args[i]
				} else {
					prompt += " " + args[i]
				}
			}
		}
		if prompt == "" {
			prompt = readStdin()
		}
		if prompt == "" {
			return fmt.Errorf("usage: prompt-improver enhance <prompt> [--type T] [--mode local|llm|auto] [--provider P] [--target-provider P] [--quiet]")
		}
		if mode != "" || provider != "" || targetProvider != "" {
			if mode == "" {
				mode = "local"
			}
			runEnhanceWithMode(prompt, taskType, mode, quiet, provider, targetProvider)
		} else {
			runEnhanceQuiet(prompt, taskType, quiet)
		}

	case "improve":
		taskType := ""
		thinking := false
		feedback := ""
		quiet := false
		provider := ""
		prompt := ""
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--type":
				if i+1 < len(args) {
					taskType = args[i+1]
					i++
				}
			case "--thinking":
				thinking = true
			case "--feedback":
				if i+1 < len(args) {
					feedback = args[i+1]
					i++
				}
			case "--provider":
				if i+1 < len(args) {
					provider = args[i+1]
					i++
				}
			case "--quiet", "-q":
				quiet = true
			default:
				if prompt == "" {
					prompt = args[i]
				} else {
					prompt += " " + args[i]
				}
			}
		}
		if prompt == "" {
			prompt = readStdin()
		}
		if prompt == "" {
			return fmt.Errorf("usage: prompt-improver improve <prompt> [--thinking] [--feedback hint] [--type T] [--provider P] [--quiet]")
		}
		runImprove(prompt, taskType, thinking, feedback, quiet, provider)

	case "diff":
		prompt := strings.Join(args[1:], " ")
		if prompt == "" {
			prompt = readStdin()
		}
		if prompt == "" {
			return fmt.Errorf("usage: prompt-improver diff <prompt>")
		}
		runDiff(prompt)

	case "analyze":
		targetProvider := ""
		prompt := ""
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--target-provider":
				if i+1 < len(args) {
					targetProvider = args[i+1]
					i++
				}
			default:
				if prompt == "" {
					prompt = args[i]
				} else {
					prompt += " " + args[i]
				}
			}
		}
		if prompt == "" {
			prompt = readStdin()
		}
		if prompt == "" {
			return fmt.Errorf("usage: prompt-improver analyze <prompt> [--target-provider P]")
		}
		runAnalyze(prompt, targetProvider)

	case "template":
		if len(args) < 2 {
			return fmt.Errorf("usage: prompt-improver template <name> [--var value ...]")
		}
		runTemplate(args[1], args[2:])

	case "templates":
		fmt.Print(enhancer.TemplateListSummary())

	case "lint":
		prompt := strings.Join(args[1:], " ")
		if prompt == "" {
			prompt = readStdin()
		}
		if prompt == "" {
			return fmt.Errorf("usage: prompt-improver lint <prompt>")
		}
		runLint(prompt)

	case "cache-check":
		path := ""
		if len(args) > 1 {
			path = args[1]
		}
		runCacheCheck(path)

	case "check-claudemd":
		path := "./CLAUDE.md"
		if len(args) > 1 {
			path = args[1]
		}
		runCheckClaudeMD(path)

	case "mcp":
		runMCP()

	case "hook":
		// Hook mode: reads JSON from stdin (Claude Code UserPromptSubmit format)
		runHook()

	case "install":
		runInstall(args[1:])

	case "uninstall":
		runUninstall(args[1:])

	case "version":
		fmt.Printf("prompt-improver %s\n", version)

	case "help", "--help", "-h":
		printHelp()

	default:
		// Treat everything as a prompt to enhance
		prompt := strings.Join(args, " ")
		runEnhance(prompt, "")
	}
	return nil
}

func runEnhance(prompt, taskType string) {
	runEnhanceQuiet(prompt, taskType, false)
}

func runEnhanceQuiet(prompt, taskType string, quiet bool) {
	tt := enhancer.ValidTaskType(taskType)
	result := enhancer.Enhance(prompt, tt)
	result.Source = "local"

	if quiet {
		fmt.Println(result.Enhanced)
		return
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
}

func runEnhanceWithMode(prompt, taskType, mode string, quiet bool, provider, targetProvider string) {
	tt := enhancer.ValidTaskType(taskType)
	m := enhancer.ValidMode(mode)
	if m == "" {
		fmt.Fprintf(os.Stderr, "invalid mode: %s (use local, llm, or auto)\n", mode)
		os.Exit(1)
	}

	cfg := enhancer.ResolveConfig(".")
	cfg.LLM.Enabled = true // --mode flag implies LLM should be available
	if provider != "" {
		cfg.LLM.Provider = provider
		// Re-resolve TargetProvider to match the overridden LLM provider,
		// unless the caller explicitly supplied a --target-provider flag.
		if targetProvider == "" {
			cfg.TargetProvider = enhancer.DefaultTargetProviderForLLM(provider)
		}
	}
	if targetProvider != "" {
		cfg.TargetProvider = enhancer.ProviderName(targetProvider)
	}
	engine := getOrCreateEngine(cfg.LLM)

	result := enhancer.EnhanceHybrid(context.Background(), prompt, tt, cfg, engine, m, cfg.TargetProvider)

	if quiet {
		fmt.Println(result.Enhanced)
		return
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
}

func runImprove(prompt, taskType string, thinking bool, feedback string, quiet bool, provider string) {
	tt := enhancer.ValidTaskType(taskType)

	cfg := enhancer.ResolveConfig(".")
	cfg.LLM.Enabled = true
	if thinking {
		cfg.LLM.ThinkingEnabled = true
	}
	if provider != "" {
		cfg.LLM.Provider = provider
	}
	engine := getOrCreateEngine(cfg.LLM)
	if engine == nil {
		apiHint := "OPENAI_API_KEY"
		switch cfg.LLM.Provider {
		case "", "openai":
			apiHint = "OPENAI_API_KEY"
		case "gemini":
			apiHint = "GOOGLE_API_KEY"
		case "claude":
			apiHint = "ANTHROPIC_API_KEY"
		}
		fmt.Fprintf(os.Stderr, "error: %s not set — cannot use LLM improvement\n", apiHint)
		os.Exit(1)
	}

	opts := enhancer.ImproveOptions{
		ThinkingEnabled: thinking,
		TaskType:        tt,
		Feedback:        feedback,
		Provider:        engine.Client.Provider(),
	}

	result, err := engine.Client.Improve(context.Background(), prompt, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if quiet {
		fmt.Println(result.Enhanced)
		return
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
}

func runDiff(prompt string) {
	result := enhancer.Enhance(prompt, "")
	origLines := strings.Split(prompt, "\n")
	enhLines := strings.Split(result.Enhanced, "\n")

	fmt.Println("--- original")
	fmt.Println("+++ enhanced")
	fmt.Println()

	for _, line := range origLines {
		fmt.Printf("- %s\n", line)
	}
	fmt.Println()
	for _, line := range enhLines {
		fmt.Printf("+ %s\n", line)
	}

	if len(result.Improvements) > 0 {
		fmt.Printf("\n%d improvements:\n", len(result.Improvements))
		for _, imp := range result.Improvements {
			fmt.Printf("  • %s\n", imp)
		}
	}
}

func getOrCreateEngine(cfg enhancer.LLMConfig) *enhancer.HybridEngine {
	if hybridEngine != nil {
		return hybridEngine
	}
	hybridEngine = enhancer.NewHybridEngine(cfg)
	return hybridEngine
}

func runAnalyze(prompt string, targetProvider string) {
	result := enhancer.Analyze(prompt)
	if targetProvider != "" {
		tp := enhancer.ProviderName(targetProvider)
		lints := enhancer.Lint(prompt)
		report := enhancer.Score(prompt, result.TaskType, lints, &result, tp)
		result.ScoreReport = report
		legacyScore := max(report.Overall/10, 1)
		if legacyScore > 10 {
			legacyScore = 10
		}
		result.Score = legacyScore
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
}

func runLint(prompt string) {
	results := enhancer.Lint(prompt)
	if len(results) == 0 {
		fmt.Println("No issues found.")
		return
	}
	data, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(data))
}

func runCacheCheck(path string) {
	var text string
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading %s: %v\n", path, err)
			os.Exit(1)
		}
		text = string(data)
	} else {
		text = readStdin()
	}
	if text == "" {
		fmt.Fprintln(os.Stderr, "usage: prompt-improver cache-check <file> or pipe via stdin")
		os.Exit(1)
	}

	results := enhancer.VerifyCacheFriendlyOrder(text)
	if len(results) == 0 {
		fmt.Println("Cache-friendly: no ordering issues found.")
		return
	}
	data, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(data))
}

func runCheckClaudeMD(path string) {
	results, err := enhancer.CheckClaudeMD(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if len(results) == 0 {
		fmt.Println("CLAUDE.md looks healthy — no issues found.")
		return
	}
	data, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(data))
}

func runTemplate(name string, args []string) {
	tmpl := enhancer.GetTemplate(name)
	if tmpl == nil {
		fmt.Fprintf(os.Stderr, "unknown template: %s\n\nAvailable templates:\n", name)
		for _, t := range enhancer.ListTemplates() {
			fmt.Fprintf(os.Stderr, "  %s - %s\n", t.Name, t.Description)
		}
		os.Exit(1)
	}

	vars := parseFlags(args)
	filled := enhancer.FillTemplate(tmpl, vars)
	fmt.Println(filled)
}

// hookInput is the JSON Claude Code sends to UserPromptSubmit hooks on stdin
type hookInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	PermissionMode string `json:"permission_mode"`
	HookEventName  string `json:"hook_event_name"`
	Prompt         string `json:"prompt"`
}

// hookOutput is the JSON response for UserPromptSubmit hooks
type hookOutput struct {
	HookSpecificOutput *hookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

type hookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

func runHook() {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading stdin: %v\n", err)
		os.Exit(1)
	}

	var hi hookInput
	if err := json.Unmarshal(input, &hi); err != nil {
		// Not JSON — treat as raw prompt text
		raw := strings.TrimSpace(string(input))
		if raw != "" {
			result := enhancer.Enhance(raw, "")
			fmt.Println(result.Enhanced)
		}
		return
	}

	// If no prompt field, pass through
	if hi.Prompt == "" {
		os.Exit(0)
		return
	}

	// Load config with global fallback + env var overrides
	cfg := enhancer.Config{}
	if hi.Cwd != "" {
		cfg = enhancer.ResolveConfig(hi.Cwd)
	} else {
		cfg = enhancer.ResolveConfig("")
	}

	// Block patterns — reject prompts matching any block regex
	for _, pat := range cfg.BlockPatterns {
		if re, err := regexp.Compile(pat); err == nil {
			if re.MatchString(hi.Prompt) {
				fmt.Fprintf(os.Stderr, "prompt-improver: blocked (matches block pattern %q)\n", pat)
				os.Exit(2)
				return
			}
		}
	}

	// Smart filtering — skip short/conversational/already-structured prompts
	if !enhancer.ShouldEnhance(hi.Prompt, cfg) {
		fmt.Fprintf(os.Stderr, "prompt-improver: skipped (filtered: too short, conversational, or already structured)\n")
		os.Exit(0)
		return
	}

	// Score gate — skip enhancement if the prompt already scores well
	// LLM mode uses a lower default threshold since it adds more value
	threshold := cfg.Hook.SkipScoreThreshold
	if threshold <= 0 {
		if cfg.LLM.Enabled {
			threshold = 50
		} else {
			threshold = 75
		}
	}
	analysis := enhancer.Analyze(hi.Prompt)
	if analysis.ScoreReport != nil && analysis.ScoreReport.Overall >= threshold {
		fmt.Fprintf(os.Stderr, "prompt-improver: skipped (score %d >= threshold %d)\n", analysis.ScoreReport.Overall, threshold)
		os.Exit(0)
		return
	}

	// Enhance — use LLM if configured, otherwise local pipeline
	var result enhancer.EnhanceResult
	if cfg.LLM.Enabled {
		fmt.Fprintf(os.Stderr, "prompt-improver: enhancing via LLM...\n")
		start := time.Now()
		engine := getOrCreateEngine(cfg.LLM)
		result = enhancer.EnhanceHybrid(context.Background(), hi.Prompt, "", cfg, engine, enhancer.ModeAuto, cfg.TargetProvider)
		fmt.Fprintf(os.Stderr, "prompt-improver: enhanced via %s (%.1fs)\n", result.Source, time.Since(start).Seconds())
	} else {
		result = enhancer.EnhanceWithConfig(hi.Prompt, "", cfg)
		result.Source = "local"
	}

	// Write source to cache for statusline integration.
	_ = writePromptImproverLastSource(result.Source)

	// Lean output — XML-wrapped enhanced prompt with a short directive
	var ctxBuilder strings.Builder
	ctxBuilder.WriteString("<enhanced_prompt>\n")
	ctxBuilder.WriteString(result.Enhanced)
	ctxBuilder.WriteString("\n</enhanced_prompt>\nFollow the enhanced version above. It adds structure and specificity to the original request.")

	// Output structured JSON per Claude Code hook spec
	out := hookOutput{
		HookSpecificOutput: &hookSpecificOutput{
			HookEventName:     "UserPromptSubmit",
			AdditionalContext: ctxBuilder.String(),
		},
	}

	data, _ := json.Marshal(out)
	fmt.Println(string(data))
	os.Exit(0)
}

func readStdin() string {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return "" // no piped input
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(input))
}

func parseFlags(args []string) map[string]string {
	vars := make(map[string]string)
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") && i+1 < len(args) {
			key := strings.TrimPrefix(args[i], "--")
			vars[key] = args[i+1]
			i++
		}
	}
	return vars
}

func promptImproverLastSourcePath() string {
	if override := strings.TrimSpace(os.Getenv("PROMPT_IMPROVER_LAST_SOURCE_PATH")); override != "" {
		return override
	}
	if xdgRuntime := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); xdgRuntime != "" {
		return filepath.Join(xdgRuntime, "prompt-improver-last-source")
	}
	if cacheDir, err := os.UserCacheDir(); err == nil && strings.TrimSpace(cacheDir) != "" {
		return filepath.Join(cacheDir, "ralphglasses", "prompt-improver-last-source")
	}
	return filepath.Join(os.TempDir(), ".prompt-improver-last-source")
}

func writePromptImproverLastSource(source string) error {
	path := promptImproverLastSourcePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(source), 0o644)
}

func printHelp() {
	fmt.Printf("prompt-improver %s — multi-provider prompt optimization CLI\n", version)
	fmt.Print(`
USAGE:
  prompt-improver <prompt>                      Enhance a prompt (default, local pipeline)
  prompt-improver enhance <prompt> [--type T] [--mode M]   Enhance with optional LLM mode
  prompt-improver improve <prompt> [--thinking] [--feedback hint]   LLM-powered improvement
  prompt-improver analyze <prompt>              Multi-dimensional scoring, suggestions, tokens & effort
  prompt-improver lint <prompt>                 Deep lint with per-line findings
  prompt-improver cache-check <file>            Check prompt caching friendliness
  prompt-improver check-claudemd [path]         CLAUDE.md health check (default: ./CLAUDE.md)
  prompt-improver template <name> [--var val]   Fill a prompt template
  prompt-improver templates                     List available templates
  prompt-improver mcp                           MCP stdio server (4 tools)
  prompt-improver hook                          Claude Code hook mode (JSON stdin)
  prompt-improver install [--global] [flags]    Install hook and/or MCP into Claude Code settings
  prompt-improver uninstall [--global]          Remove prompt-improver from Claude Code settings
  echo "prompt" | prompt-improver               Pipe mode

INSTALL FLAGS:
  --global      Write to ~/.claude/settings.json (default: .claude/settings.json)
  --hook-only   Only install the UserPromptSubmit hook
  --mcp-only    Only install the MCP server

PIPELINE (13 stages):
  0  config_rules         Pattern-matched augmentations
  1  specificity          Replace vague phrases
  2  positive_reframe     Negative-to-positive reframing
  3  tone_downgrade       ALL-CAPS → normal case
  4  overtrigger_rewrite  Soften anti-laziness phrases (Claude 4.x)
  5  example_wrapping     Wrap bare examples in XML
  6  structure            Add XML role/instructions/constraints
  7  context_reorder      Long context before query
  8  format_enforcement   JSON/YAML/CSV format tags
  9  quote_grounding      Quote-first for long-context analysis
  10 self_check           Verification checklists
  11 overengineering_guard Prevent over-abstraction (code tasks)
  12 preamble_suppression Direct response instruction

TASK TYPES:
  code, creative, analysis, troubleshooting, workflow, general

LINT CHECKS:
  unmotivated-rule, negative-framing, aggressive-emphasis, vague-quantifier,
  overtrigger-phrase, over-specification, decomposition-needed, injection-risk,
  thinking-mode-redundant, example-quality, compaction-readiness

HOOK INTEGRATION:
  Quick setup (recommended):
    prompt-improver install --global       # hook + MCP for all projects
    prompt-improver install                # hook + MCP for current project only
    prompt-improver install --hook-only    # just the hook
    prompt-improver uninstall --global     # remove everything

  The hook automatically filters short/conversational prompts ("yes", "ok", "continue"),
  skips already-well-structured prompts, and only enhances prompts that score below 75.
  Configure thresholds in .prompt-improver.yaml:

    hook:
      skip_score_threshold: 75   # skip if score >= this (0 = always enhance)
      min_word_count: 5          # skip prompts shorter than this

  Exit code 0 = proceed, exit code 2 = block the prompt.
  The install command can target Codex, Claude, or both depending on your provider setup.

MCP SERVER (on-demand prompt tools):
  Add to project .mcp.json:
    { "mcpServers": { "prompt-improver": { "type": "stdio", "command": "prompt-improver", "args": ["mcp"] } } }

  Or register globally with your preferred MCP client or via prompt-improver install --global.

  Tools exposed: analyze_prompt, enhance_prompt, lint_prompt, improve_prompt

LLM-POWERED IMPROVEMENT (v2.0.0):
  prompt-improver improve "fix this bug"           # direct LLM improvement
  prompt-improver improve "fix this" --thinking    # with thinking scaffolding
  prompt-improver enhance "fix this" --mode auto   # try LLM, fall back to local
  prompt-improver enhance "fix this" --mode llm    # LLM only (fail if unavailable)
  prompt-improver enhance "fix this" --mode local  # deterministic pipeline only

  Requires the provider-specific API key environment variable.
  Configure in .prompt-improver.yaml:

    llm:
      enabled: true             # enable LLM in hook mode (default: false)
      thinking_enabled: true    # add thinking scaffolding
      provider: openai          # default LLM improver backend
      model: gpt-5.4            # model for meta-prompting
      timeout: 15s              # API call timeout
      api_key_env: OPENAI_API_KEY

  The LLM mode sends your prompt to the selected provider with a meta-prompt that adds:
  - Domain-specific role definition
  - Template variables for external data
  - Structured output sections (custom XML tags)
  - Scratchpad with seeded analysis points
  - Task-appropriate constraints

  Features circuit breaker (3 failures → 60s cooldown) and in-memory cache (10min TTL).
  In auto mode, falls back to the local 13-stage pipeline on LLM failure.
`)
}
