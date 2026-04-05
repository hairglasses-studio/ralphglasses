package session

import (
	"fmt"
	"strings"
)

// allowedVerifyPrefixes is the set of command prefixes permitted for loop
// verify commands. Commands that do not match any prefix are rejected to
// prevent arbitrary shell execution through the MCP tool parameter.
var allowedVerifyPrefixes = []string{
	"go build", "go test", "go vet", "go run",
	"golangci-lint",
	"./scripts/", "make ", "npm test", "npm run", "pytest",
	"test ", "true", "exit ",
	"cargo ", "rustfmt", "clippy",
}

// ValidateVerifyCommand checks that cmd is safe to execute as a loop
// verification command. It rejects commands containing shell metacharacters
// and commands whose prefix is not in the allowlist.
func ValidateVerifyCommand(cmd string) error {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return fmt.Errorf("verify command must not be empty")
	}
	// Always reject metacharacters first, even if prefix matches.
	if strings.ContainsAny(cmd, ";|&`$(){}") {
		return fmt.Errorf("verify command contains shell metacharacters: %q", cmd)
	}
	for _, prefix := range allowedVerifyPrefixes {
		if strings.HasPrefix(cmd, prefix) {
			return nil
		}
	}
	return fmt.Errorf("verify command not in allowlist: %q", cmd)
}
