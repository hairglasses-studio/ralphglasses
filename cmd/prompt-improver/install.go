package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// settingsJSON represents the relevant parts of Claude Code's settings.json.
type settingsJSON struct {
	Hooks    map[string][]hookGroup     `json:"hooks,omitempty"`
	McpServers map[string]mcpServerEntry `json:"mcpServers,omitempty"`
	// Rest holds all other top-level keys we want to preserve.
	Rest map[string]json.RawMessage `json:"-"`
}

type hookGroup struct {
	Hooks []hookEntry `json:"hooks"`
}

type hookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

type mcpServerEntry struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

// runInstall installs the prompt-improver hook and/or MCP server into Claude Code settings.
func runInstall(args []string) {
	global := false
	hookOnly := false
	mcpOnly := false

	for _, a := range args {
		switch a {
		case "--global":
			global = true
		case "--hook-only":
			hookOnly = true
		case "--mcp-only":
			mcpOnly = true
		}
	}

	// Resolve binary path
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot resolve executable path: %v\n", err)
		os.Exit(1)
	}
	exe, _ = filepath.EvalSymlinks(exe)

	settingsPath := settingsPathFor(global)

	// Read existing settings
	settings, raw, err := readSettings(settingsPath)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "error reading %s: %v\n", settingsPath, err)
		os.Exit(1)
	}

	installHook := !mcpOnly
	installMCP := !hookOnly

	if installHook {
		addHookEntry(settings, exe)
		fmt.Printf("Installed UserPromptSubmit hook → %s\n", settingsPath)
	}

	if installMCP {
		addMCPEntry(settings, exe)
		fmt.Printf("Installed MCP server → %s\n", settingsPath)
	}

	if err := writeSettings(settingsPath, settings, raw); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", settingsPath, err)
		os.Exit(1)
	}
}

// runUninstall removes prompt-improver entries from Claude Code settings.
func runUninstall(args []string) {
	global := false
	for _, a := range args {
		if a == "--global" {
			global = true
		}
	}

	settingsPath := settingsPathFor(global)

	settings, raw, err := readSettings(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Nothing to uninstall — settings file does not exist.")
			return
		}
		fmt.Fprintf(os.Stderr, "error reading %s: %v\n", settingsPath, err)
		os.Exit(1)
	}

	removeHookEntry(settings)
	removeMCPEntry(settings)

	if err := writeSettings(settingsPath, settings, raw); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", settingsPath, err)
		os.Exit(1)
	}
	fmt.Printf("Uninstalled prompt-improver from %s\n", settingsPath)
}

func settingsPathFor(global bool) string {
	if global {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".claude", "settings.json")
	}
	return filepath.Join(".claude", "settings.json")
}

func readSettings(path string) (*settingsJSON, map[string]json.RawMessage, error) {
	s := &settingsJSON{
		Hooks:      make(map[string][]hookGroup),
		McpServers: make(map[string]mcpServerEntry),
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return s, nil, err
	}

	// First, unmarshal everything into a raw map to preserve unknown keys
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return s, nil, fmt.Errorf("invalid JSON: %w", err)
	}

	// Parse known keys
	if hooksRaw, ok := raw["hooks"]; ok {
		if err := json.Unmarshal(hooksRaw, &s.Hooks); err != nil {
			return s, raw, fmt.Errorf("invalid hooks JSON: %w", err)
		}
	}
	if mcpRaw, ok := raw["mcpServers"]; ok {
		if err := json.Unmarshal(mcpRaw, &s.McpServers); err != nil {
			return s, raw, fmt.Errorf("invalid mcpServers JSON: %w", err)
		}
	}

	return s, raw, nil
}

func writeSettings(path string, s *settingsJSON, raw map[string]json.RawMessage) error {
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	if raw == nil {
		raw = make(map[string]json.RawMessage)
	}

	// Marshal our known keys back into raw
	if len(s.Hooks) > 0 {
		data, _ := json.Marshal(s.Hooks)
		raw["hooks"] = data
	} else {
		delete(raw, "hooks")
	}

	if len(s.McpServers) > 0 {
		data, _ := json.Marshal(s.McpServers)
		raw["mcpServers"] = data
	} else {
		delete(raw, "mcpServers")
	}

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	return os.WriteFile(path, data, 0644)
}

const promptImproverCmd = "prompt-improver"

func addHookEntry(s *settingsJSON, exe string) {
	cmd := exe + " hook"
	eventName := "UserPromptSubmit"

	// Check if already installed (idempotent)
	for _, group := range s.Hooks[eventName] {
		for _, h := range group.Hooks {
			if strings.Contains(h.Command, promptImproverCmd) {
				return // already installed
			}
		}
	}

	entry := hookGroup{
		Hooks: []hookEntry{
			{
				Type:    "command",
				Command: cmd,
				Timeout: 30, // 30s allows LLM-backed improvement (~3-5s typical)
			},
		},
	}
	s.Hooks[eventName] = append(s.Hooks[eventName], entry)
}

func addMCPEntry(s *settingsJSON, exe string) {
	// Check if already installed
	if _, ok := s.McpServers[promptImproverCmd]; ok {
		return
	}

	s.McpServers[promptImproverCmd] = mcpServerEntry{
		Type:    "stdio",
		Command: exe,
		Args:    []string{"mcp"},
	}
}

func removeHookEntry(s *settingsJSON) {
	eventName := "UserPromptSubmit"
	groups := s.Hooks[eventName]
	var kept []hookGroup
	for _, group := range groups {
		var keptHooks []hookEntry
		for _, h := range group.Hooks {
			if !strings.Contains(h.Command, promptImproverCmd) {
				keptHooks = append(keptHooks, h)
			}
		}
		if len(keptHooks) > 0 {
			group.Hooks = keptHooks
			kept = append(kept, group)
		}
	}
	if len(kept) > 0 {
		s.Hooks[eventName] = kept
	} else {
		delete(s.Hooks, eventName)
	}
}

func removeMCPEntry(s *settingsJSON) {
	delete(s.McpServers, promptImproverCmd)
}
