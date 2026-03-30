package autorandr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestHelperProcess is not a real test. It is used as a helper process
// for mocking exec.Command via the TestHelperProcess technique.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for i, arg := range args {
		if arg == "--" {
			args = args[i+1:]
			break
		}
	}
	if len(args) == 0 {
		os.Exit(1)
	}

	response := os.Getenv("MOCK_RESPONSE")
	exitCode := os.Getenv("MOCK_EXIT_CODE")

	if exitCode == "1" {
		fmt.Fprint(os.Stderr, "mock error")
		os.Exit(1)
	}

	fmt.Fprint(os.Stdout, response)
	os.Exit(0)
}

// fakeExecCommand returns a function that creates a Cmd which invokes
// this test binary as a helper process with the given mock response.
func fakeExecCommand(response string, fail bool) func(string, ...string) *exec.Cmd {
	return func(name string, args ...string) *exec.Cmd {
		csArgs := []string{"-test.run=TestHelperProcess", "--", name}
		csArgs = append(csArgs, args...)
		cmd := exec.Command(os.Args[0], csArgs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"MOCK_RESPONSE="+response,
		)
		if fail {
			cmd.Env = append(cmd.Env, "MOCK_EXIT_CODE=1")
		}
		return cmd
	}
}

func TestParseProfiles(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []Profile
	}{
		{
			name:  "single current",
			input: "docked (current)\n",
			expect: []Profile{
				{Name: "docked", Current: true, Detected: false},
			},
		},
		{
			name:  "multiple profiles",
			input: "docked (current)\nlaptop\ntriple (detected)\n",
			expect: []Profile{
				{Name: "docked", Current: true, Detected: false},
				{Name: "laptop", Current: false, Detected: false},
				{Name: "triple", Current: false, Detected: true},
			},
		},
		{
			name:  "current and detected",
			input: "home (current) (detected)\n",
			expect: []Profile{
				{Name: "home", Current: true, Detected: true},
			},
		},
		{
			name:   "empty input",
			input:  "",
			expect: nil,
		},
		{
			name:   "blank lines only",
			input:  "\n  \n\n",
			expect: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseProfiles([]byte(tt.input))
			if len(got) != len(tt.expect) {
				t.Fatalf("got %d profiles, want %d", len(got), len(tt.expect))
			}
			for i, p := range got {
				exp := tt.expect[i]
				if p.Name != exp.Name {
					t.Errorf("profile[%d].Name = %q, want %q", i, p.Name, exp.Name)
				}
				if p.Current != exp.Current {
					t.Errorf("profile[%d].Current = %v, want %v", i, p.Current, exp.Current)
				}
				if p.Detected != exp.Detected {
					t.Errorf("profile[%d].Detected = %v, want %v", i, p.Detected, exp.Detected)
				}
			}
		})
	}
}

func TestIsAvailable(t *testing.T) {
	c := NewClient()

	t.Run("found", func(t *testing.T) {
		origLookPath := execLookPath
		defer func() { execLookPath = origLookPath }()
		execLookPath = func(file string) (string, error) {
			return "/usr/bin/autorandr", nil
		}
		if !c.IsAvailable() {
			t.Error("expected IsAvailable to return true")
		}
	})

	t.Run("not found", func(t *testing.T) {
		origLookPath := execLookPath
		defer func() { execLookPath = origLookPath }()
		execLookPath = func(file string) (string, error) {
			return "", fmt.Errorf("not found")
		}
		if c.IsAvailable() {
			t.Error("expected IsAvailable to return false")
		}
	})
}

func TestListProfiles(t *testing.T) {
	orig := execCommand
	defer func() { execCommand = orig }()

	t.Run("success", func(t *testing.T) {
		execCommand = fakeExecCommand("docked (current)\nlaptop\ntriple (detected)\n", false)
		c := NewClient()
		profiles, err := c.ListProfiles()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(profiles) != 3 {
			t.Fatalf("got %d profiles, want 3", len(profiles))
		}
		if profiles[0].Name != "docked" || !profiles[0].Current {
			t.Errorf("unexpected first profile: %+v", profiles[0])
		}
	})

	t.Run("command fails", func(t *testing.T) {
		execCommand = fakeExecCommand("", true)
		c := NewClient()
		_, err := c.ListProfiles()
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "autorandr list") {
			t.Errorf("error should mention autorandr list: %v", err)
		}
	})
}

func TestCurrentProfile(t *testing.T) {
	orig := execCommand
	defer func() { execCommand = orig }()

	t.Run("has current", func(t *testing.T) {
		execCommand = fakeExecCommand("laptop\ndocked (current)\n", false)
		c := NewClient()
		name, err := c.CurrentProfile()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "docked" {
			t.Errorf("got %q, want %q", name, "docked")
		}
	})

	t.Run("no current", func(t *testing.T) {
		execCommand = fakeExecCommand("laptop\ndocked\n", false)
		c := NewClient()
		name, err := c.CurrentProfile()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "" {
			t.Errorf("got %q, want empty", name)
		}
	})
}

func TestSaveProfile(t *testing.T) {
	orig := execCommand
	defer func() { execCommand = orig }()

	t.Run("success", func(t *testing.T) {
		execCommand = fakeExecCommand("", false)
		c := NewClient()
		if err := c.SaveProfile("myprofile"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty name", func(t *testing.T) {
		c := NewClient()
		if err := c.SaveProfile(""); err == nil {
			t.Fatal("expected error for empty name")
		}
	})

	t.Run("command fails", func(t *testing.T) {
		execCommand = fakeExecCommand("", true)
		c := NewClient()
		err := c.SaveProfile("myprofile")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "autorandr save") {
			t.Errorf("error should mention autorandr save: %v", err)
		}
	})
}

func TestLoadProfile(t *testing.T) {
	orig := execCommand
	defer func() { execCommand = orig }()

	t.Run("success", func(t *testing.T) {
		execCommand = fakeExecCommand("", false)
		c := NewClient()
		if err := c.LoadProfile("docked"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty name", func(t *testing.T) {
		c := NewClient()
		if err := c.LoadProfile(""); err == nil {
			t.Fatal("expected error for empty name")
		}
	})

	t.Run("command fails", func(t *testing.T) {
		execCommand = fakeExecCommand("", true)
		c := NewClient()
		err := c.LoadProfile("docked")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "autorandr load") {
			t.Errorf("error should mention autorandr load: %v", err)
		}
	})
}

func TestDeleteProfile(t *testing.T) {
	orig := execCommand
	defer func() { execCommand = orig }()

	t.Run("success", func(t *testing.T) {
		execCommand = fakeExecCommand("", false)
		c := NewClient()
		if err := c.DeleteProfile("old"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty name", func(t *testing.T) {
		c := NewClient()
		if err := c.DeleteProfile(""); err == nil {
			t.Fatal("expected error for empty name")
		}
	})

	t.Run("command fails", func(t *testing.T) {
		execCommand = fakeExecCommand("", true)
		c := NewClient()
		err := c.DeleteProfile("old")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "autorandr remove") {
			t.Errorf("error should mention autorandr remove: %v", err)
		}
	})
}

func TestDetectAndApply(t *testing.T) {
	orig := execCommand
	defer func() { execCommand = orig }()

	t.Run("applies profile", func(t *testing.T) {
		execCommand = fakeExecCommand("docked", false)
		c := NewClient()
		result, err := c.DetectAndApply()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != "docked" {
			t.Errorf("got %q, want %q", result, "docked")
		}
	})

	t.Run("no match", func(t *testing.T) {
		execCommand = fakeExecCommand("", false)
		c := NewClient()
		result, err := c.DetectAndApply()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != "" {
			t.Errorf("got %q, want empty", result)
		}
	})

	t.Run("command fails", func(t *testing.T) {
		execCommand = fakeExecCommand("", true)
		c := NewClient()
		_, err := c.DetectAndApply()
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestWatchChanges(t *testing.T) {
	orig := execCommand
	defer func() { execCommand = orig }()

	t.Run("nil callback", func(t *testing.T) {
		c := NewClient()
		err := c.WatchChanges(context.Background(), nil)
		if err == nil {
			t.Fatal("expected error for nil callback")
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		// Use a mock that always returns "docked" as current.
		execCommand = fakeExecCommand("docked (current)\n", false)
		c := NewClient()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		called := make(chan string, 10)
		err := c.WatchChanges(ctx, func(profile string) {
			called <- profile
		})
		if err != context.DeadlineExceeded {
			t.Errorf("expected DeadlineExceeded, got %v", err)
		}
	})
}

func TestNewClient(t *testing.T) {
	c := NewClient()
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	if c.bin != "autorandr" {
		t.Errorf("bin = %q, want %q", c.bin, "autorandr")
	}
}
