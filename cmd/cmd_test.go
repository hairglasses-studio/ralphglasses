package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestVersionDefault(t *testing.T) {
	if version != "dev" {
		t.Errorf("version = %q, want dev", version)
	}
}

func TestCommitDefault(t *testing.T) {
	if commit != "unknown" {
		t.Errorf("commit = %q, want unknown", commit)
	}
}

func TestRootCmdVersion(t *testing.T) {
	if rootCmd.Version != "dev" {
		t.Errorf("rootCmd.Version = %q, want dev", rootCmd.Version)
	}
}

func TestMCPServerVersionFormat(t *testing.T) {
	// The MCP server uses version+" ("+commit+")" as the version string.
	got := version + " (" + commit + ")"
	want := "dev (unknown)"
	if got != want {
		t.Errorf("MCP version string = %q, want %q", got, want)
	}
}

func TestScanPathDefault(t *testing.T) {
	// Default scan path should be set
	f := rootCmd.PersistentFlags().Lookup("scan-path")
	if f == nil {
		t.Fatal("scan-path flag not registered")
	}
	if f.DefValue != "~/hairglasses-studio" {
		t.Errorf("scan-path default = %q, want ~/hairglasses-studio", f.DefValue)
	}
}

func TestScanPathIsPersistent(t *testing.T) {
	// scan-path must be a persistent flag so subcommands (mcp) inherit it
	f := rootCmd.PersistentFlags().Lookup("scan-path")
	if f == nil {
		t.Fatal("scan-path not found in PersistentFlags — mcp subcommand won't inherit it")
	}
}

func TestMcpSubcommandRegistered(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Use == "mcp" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("mcp subcommand not registered on rootCmd")
	}
}

func TestMakefileLdflags(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Makefile integration test in short mode")
	}

	// Verify Makefile contains COMMIT and extended LDFLAGS
	data, err := os.ReadFile("../Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "COMMIT") {
		t.Error("Makefile missing COMMIT variable")
	}
	if !strings.Contains(content, "cmd.commit=$(COMMIT)") {
		t.Error("Makefile LDFLAGS missing cmd.commit injection")
	}
	if !strings.Contains(content, "cmd.version=$(VERSION)") {
		t.Error("Makefile LDFLAGS missing cmd.version injection")
	}
}

func TestMakefileTargets(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Makefile integration test in short mode")
	}

	data, err := os.ReadFile("../Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	content := string(data)

	for _, target := range []string{"mcp:", "dev-mcp:"} {
		if !strings.Contains(content, target) {
			t.Errorf("Makefile missing target %q", target)
		}
	}
}

func TestBuildReleaseLdflags(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build integration test in short mode")
	}

	// Build with ldflags and verify version output contains injected values
	out, err := exec.Command("go", "run",
		"-ldflags", "-X github.com/hairglasses-studio/ralphglasses/cmd.version=test-v1.2.3 -X github.com/hairglasses-studio/ralphglasses/cmd.commit=abc1234",
		"..", "version",
	).CombinedOutput()
	if err != nil {
		// version subcommand may not exist; just check it compiled
		outStr := string(out)
		if strings.Contains(outStr, "unknown command") {
			// No version subcommand — check --version flag instead
			out, err = exec.Command("go", "run",
				"-ldflags", "-X github.com/hairglasses-studio/ralphglasses/cmd.version=test-v1.2.3 -X github.com/hairglasses-studio/ralphglasses/cmd.commit=abc1234",
				"..", "--version",
			).CombinedOutput()
			if err != nil {
				t.Fatalf("go run --version failed: %v\n%s", err, out)
			}
		} else {
			t.Fatalf("go run version failed: %v\n%s", err, out)
		}
	}

	outStr := string(out)
	if !strings.Contains(outStr, "test-v1.2.3") {
		t.Errorf("version output = %q, want to contain test-v1.2.3", outStr)
	}
}

func TestMcpJsonFormat(t *testing.T) {
	data, err := os.ReadFile("../.mcp.json")
	if err != nil {
		t.Fatalf("read .mcp.json: %v", err)
	}

	var config struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
			CWD     string   `json:"cwd"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse .mcp.json: %v", err)
	}

	srv, ok := config.MCPServers["ralphglasses"]
	if !ok {
		t.Fatal("ralphglasses server not found in .mcp.json")
	}

	if srv.Command != "bash" {
		t.Errorf("command = %q, want bash (wrapper-based startup)", srv.Command)
	}
	if len(srv.Args) < 1 || srv.Args[0] != "./scripts/dev/run-mcp.sh" {
		t.Errorf("args = %v, want wrapper startup script", srv.Args)
	}
	if srv.CWD != "." {
		t.Errorf("cwd = %q, want .", srv.CWD)
	}
}

func TestGeminiSettingsFormat(t *testing.T) {
	data, err := os.ReadFile("../.gemini/settings.json")
	if err != nil {
		t.Fatalf("read .gemini/settings.json: %v", err)
	}

	var config struct {
		Context struct {
			FileName []string `json:"fileName"`
		} `json:"context"`
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
			CWD     string   `json:"cwd"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse .gemini/settings.json: %v", err)
	}

	if !containsString(config.Context.FileName, "AGENTS.md") {
		t.Error("Gemini context.fileName must include AGENTS.md")
	}
	if !containsString(config.Context.FileName, "GEMINI.md") {
		t.Error("Gemini context.fileName must include GEMINI.md")
	}

	srv, ok := config.MCPServers["ralphglasses"]
	if !ok {
		t.Fatal("ralphglasses server not found in .gemini/settings.json")
	}
	if srv.Command != "bash" {
		t.Errorf("Gemini command = %q, want bash", srv.Command)
	}
	if len(srv.Args) < 1 || srv.Args[0] != "./scripts/dev/run-mcp.sh" {
		t.Errorf("Gemini args = %v, want wrapper startup script", srv.Args)
	}
	if srv.CWD != "." {
		t.Errorf("Gemini cwd = %q, want .", srv.CWD)
	}
}

func TestClaudeSettingsFormat(t *testing.T) {
	data, err := os.ReadFile("../.claude/settings.json")
	if err != nil {
		t.Fatalf("read .claude/settings.json: %v", err)
	}

	var config struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
			CWD     string   `json:"cwd"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse .claude/settings.json: %v", err)
	}

	srv, ok := config.MCPServers["ralphglasses"]
	if !ok {
		t.Fatal("ralphglasses server not found in .claude/settings.json")
	}
	if srv.Command != "bash" {
		t.Errorf("Claude command = %q, want bash", srv.Command)
	}
	if len(srv.Args) < 1 || srv.Args[0] != "./scripts/dev/run-mcp.sh" {
		t.Errorf("Claude args = %v, want wrapper startup script", srv.Args)
	}
	if srv.CWD != "." {
		t.Errorf("Claude cwd = %q, want .", srv.CWD)
	}
}

func TestCodexConfigFormat(t *testing.T) {
	data, err := os.ReadFile("../.codex/config.toml")
	if err != nil {
		t.Fatalf("read .codex/config.toml: %v", err)
	}

	content := string(data)
	for _, server := range []string{
		"[mcp_servers.ralphglasses_review]",
		"[mcp_servers.ralphglasses_workspace]",
		"[mcp_servers.ralphglasses_research]",
	} {
		if !strings.Contains(content, server) {
			t.Errorf("expected %s in .codex/config.toml", server)
		}
	}
	if strings.Count(content, `args = ["./scripts/dev/run-mcp.sh", "--scan-path", "~/hairglasses-studio"]`) < 3 {
		t.Error("expected repo MCP servers to use ./scripts/dev/run-mcp.sh wrapper")
	}
	if strings.Count(content, `cwd = "."`) < 3 {
		t.Error("expected repo MCP servers to keep cwd = \".\"")
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
