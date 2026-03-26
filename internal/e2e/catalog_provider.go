package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// ---------------------------------------------------------------------------
// Multi-provider scenarios
// ---------------------------------------------------------------------------

// GeminiWorkerBasic: single Gemini worker session doing basic code generation.
func GeminiWorkerBasic() Scenario {
	return Scenario{
		Name:     "gemini-worker-basic",
		Category: "feature",
		Provider: session.ProviderGemini,
		Tags:     []string{"multi-provider", "gemini"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go": "package main\n\nfunc main() {}\n",
			})
		},
		PlannerResponse: plannerJSON("Generate utility functions", "Create a utils.go file with string helper functions using Gemini"),
		WorkerBehavior: func(worktree string) error {
			content := "package main\n\nimport \"strings\"\n\n// Capitalize returns s with the first letter uppercased.\nfunc Capitalize(s string) string {\n\tif s == \"\" {\n\t\treturn s\n\t}\n\treturn strings.ToUpper(s[:1]) + s[1:]\n}\n"
			return os.WriteFile(filepath.Join(worktree, "utils.go"), []byte(content), 0o644)
		},
		VerifyCommands: []string{"test -f utils.go", "grep -q Capitalize utils.go"},
		ExpectedStatus: "idle",
		MockCostUSD:    0.08,
		MockTurnCount:  3,
		Constraints:    Constraints{MaxCostUSD: 0.5, MaxDurationSec: 30, MinCompletionRate: 0.85},
	}
}

// CodexWorkerBasic: single Codex worker session doing focused refactoring.
func CodexWorkerBasic() Scenario {
	return Scenario{
		Name:     "codex-worker-basic",
		Category: "refactor",
		Provider: session.ProviderCodex,
		Tags:     []string{"multi-provider", "codex"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"handler.go": "package main\n\nimport \"fmt\"\n\nfunc handleRequest(method string, path string, body string) {\n\tfmt.Println(method, path, body)\n}\n",
			})
		},
		PlannerResponse: plannerJSON("Refactor handler to use struct", "Refactor handleRequest to accept a Request struct instead of individual parameters"),
		WorkerBehavior: func(worktree string) error {
			content := "package main\n\nimport \"fmt\"\n\n// Request holds HTTP request parameters.\ntype Request struct {\n\tMethod string\n\tPath   string\n\tBody   string\n}\n\nfunc handleRequest(r Request) {\n\tfmt.Println(r.Method, r.Path, r.Body)\n}\n"
			return os.WriteFile(filepath.Join(worktree, "handler.go"), []byte(content), 0o644)
		},
		VerifyCommands: []string{"grep -q 'type Request struct' handler.go"},
		ExpectedStatus: "idle",
		MockCostUSD:    0.12,
		MockTurnCount:  4,
		Constraints:    Constraints{MaxCostUSD: 1.0, MaxDurationSec: 45, MinCompletionRate: 0.8},
	}
}

// MultiProviderTeam: Claude lead orchestrating Gemini + Codex workers.
func MultiProviderTeam() Scenario {
	return Scenario{
		Name:     "multi-provider-team",
		Category: "feature",
		Provider: session.ProviderClaude,
		Tags:     []string{"multi-provider", "team", "claude", "gemini", "codex"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go":       "package main\n\nfunc main() {}\n",
				"api/server.go": "package api\n\n// Server is a placeholder.\ntype Server struct{}\n",
			})
		},
		PlannerResponse: plannerJSON("Build API with multi-provider team",
			"Claude: architect API design. Gemini: generate endpoint handlers. Codex: refactor server struct with dependency injection."),
		WorkerBehavior: func(worktree string) error {
			// Simulates output from three providers working on different files
			files := map[string]string{
				"api/server.go": "package api\n\nimport \"net/http\"\n\n// Server handles HTTP requests with injected dependencies.\ntype Server struct {\n\tMux    *http.ServeMux\n\tAddr   string\n}\n\n// New creates a Server with defaults.\nfunc New(addr string) *Server {\n\treturn &Server{Mux: http.NewServeMux(), Addr: addr}\n}\n",
				"api/health.go": "package api\n\nimport \"net/http\"\n\n// HandleHealth returns 200 OK.\nfunc (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {\n\tw.WriteHeader(http.StatusOK)\n\tw.Write([]byte(`{\"status\":\"ok\"}`))\n}\n",
				"api/routes.go": "package api\n\n// RegisterRoutes wires all endpoint handlers.\nfunc (s *Server) RegisterRoutes() {\n\ts.Mux.HandleFunc(\"/health\", s.HandleHealth)\n}\n",
			}
			for name, content := range files {
				p := filepath.Join(worktree, name)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
					return err
				}
			}
			return nil
		},
		VerifyCommands: []string{
			"test -f api/server.go",
			"test -f api/health.go",
			"test -f api/routes.go",
			"grep -q RegisterRoutes api/routes.go",
		},
		ExpectedStatus: "idle",
		MockCostUSD:    0.85,
		MockTurnCount:  12,
		Constraints:    Constraints{MaxCostUSD: 5.0, MaxDurationSec: 120, MinCompletionRate: 0.7},
	}
}

// ProviderFailover: primary provider fails, system should failover to secondary.
func ProviderFailover() Scenario {
	return Scenario{
		Name:     "provider-failover",
		Category: "bug_fix",
		Provider: session.ProviderGemini,
		Tags:     []string{"multi-provider", "failover", "stress"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go": "package main\n\nfunc main() {\n\tprintln(\"before fix\")\n}\n",
				// .ralphrc with failover config
				".ralphrc": "PROJECT_NAME=\"e2e-failover\"\nFAILOVER_PROVIDER=\"claude\"\n",
			})
		},
		PlannerResponse: plannerJSON("Apply hotfix with failover", "Fix the output message; if Gemini fails, Claude should take over"),
		WorkerBehavior: func(worktree string) error {
			// Simulates the secondary provider (Claude) completing the work after Gemini failure
			return os.WriteFile(filepath.Join(worktree, "main.go"),
				[]byte("package main\n\nfunc main() {\n\tprintln(\"after fix\")\n}\n"), 0o644)
		},
		VerifyCommands: []string{"grep -q 'after fix' main.go"},
		ExpectedStatus: "idle",
		MockCostUSD:    0.30,
		MockTurnCount:  6,
		Constraints:    Constraints{MaxCostUSD: 2.0, MaxDurationSec: 60, MinCompletionRate: 0.7},
	}
}
