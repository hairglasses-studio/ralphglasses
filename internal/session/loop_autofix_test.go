package session

import "testing"

func TestClassifyCIFailure(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		output   string
		expected CIFailureType
	}{
		{
			name:     "go vet command",
			command:  "go vet ./...",
			output:   "# mypackage\n./main.go:10: unreachable code",
			expected: CIFailureVet,
		},
		{
			name:     "go test command with test failure",
			command:  "go test ./...",
			output:   "--- FAIL: TestFoo (0.01s)\n    foo_test.go:10: expected 1, got 2",
			expected: CIFailureTest,
		},
		{
			name:     "go test with build error",
			command:  "go test ./...",
			output:   "# mypackage\n./main.go:5:10: undefined: FooBar",
			expected: CIFailureBuild,
		},
		{
			name:     "go build command",
			command:  "go build ./...",
			output:   "# mypackage\n./main.go:3:2: imported and not used: \"fmt\"",
			expected: CIFailureBuild,
		},
		{
			name:     "golangci-lint command",
			command:  "golangci-lint run",
			output:   "main.go:10:5: SA1000: unused variable",
			expected: CIFailureLint,
		},
		{
			name:     "unknown command with syntax error pattern",
			command:  "make check",
			output:   "error: syntax error at line 5",
			expected: CIFailureBuild, // "syntax error" matches build pattern in fallback
		},
		{
			name:     "ci script with undefined error",
			command:  "./scripts/ci.sh",
			output:   "main.go:5: undefined: DoSomething",
			expected: CIFailureBuild,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyCIFailure(tc.command, tc.output)
			if got != tc.expected {
				t.Errorf("classifyCIFailure(%q, ...) = %q, want %q", tc.command, got, tc.expected)
			}
		})
	}
}

func TestBuildFixPrompt(t *testing.T) {
	prompt := buildFixPrompt(CIFailureBuild, "go build ./...", "undefined: FooBar")

	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
	if !contains(prompt, "Build Failure") {
		t.Error("expected prompt to contain 'Build Failure'")
	}
	if !contains(prompt, "go build ./...") {
		t.Error("expected prompt to contain the failed command")
	}
	if !contains(prompt, "undefined: FooBar") {
		t.Error("expected prompt to contain the error output")
	}
}

func TestBuildFixPrompt_Truncation(t *testing.T) {
	longOutput := make([]byte, 5000)
	for i := range longOutput {
		longOutput[i] = 'x'
	}

	prompt := buildFixPrompt(CIFailureTest, "go test", string(longOutput))
	if len(prompt) > 4000 {
		// Prompt should be truncated but include the truncation marker
		if !contains(prompt, "truncated") {
			t.Error("expected truncation marker in long output")
		}
	}
}

func TestBuildFixPrompt_AllTypes(t *testing.T) {
	types := []struct {
		ft       CIFailureType
		expected string
	}{
		{CIFailureBuild, "Build Failure"},
		{CIFailureTest, "Test Failure"},
		{CIFailureLint, "Lint Failure"},
		{CIFailureVet, "Vet Failure"},
		{CIFailureUnknown, "Verification Failure"},
	}

	for _, tc := range types {
		t.Run(string(tc.ft), func(t *testing.T) {
			prompt := buildFixPrompt(tc.ft, "cmd", "output")
			if !contains(prompt, tc.expected) {
				t.Errorf("expected prompt to contain %q for failure type %q", tc.expected, tc.ft)
			}
		})
	}
}

func TestMaxAutoFixRetries(t *testing.T) {
	// Disabled by default
	p := LoopProfile{}
	if got := maxAutoFixRetries(p); got != 0 {
		t.Errorf("maxAutoFixRetries(default) = %d, want 0", got)
	}

	// Enabled with default
	p.AutoFixOnVerifyFail = true
	if got := maxAutoFixRetries(p); got != 2 {
		t.Errorf("maxAutoFixRetries(enabled, default) = %d, want 2", got)
	}

	// Custom value
	p.MaxAutoFixRetries = 5
	if got := maxAutoFixRetries(p); got != 5 {
		t.Errorf("maxAutoFixRetries(enabled, 5) = %d, want 5", got)
	}
}

// contains and searchString are defined in agents_test.go
