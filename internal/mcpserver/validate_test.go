package mcpserver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRepoName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Valid
		{"simple alphanumeric", "myrepo", false},
		{"with dash", "my-repo", false},
		{"with underscore", "my_repo", false},
		{"with dot", "my.repo", false},
		{"mixed", "My_Repo-v1.2", false},
		{"numbers only", "123", false},
		{"max length (128)", string(make([]byte, 128)), false}, // filled below

		// Invalid
		{"empty string", "", true},
		{"too long (129)", string(make([]byte, 129)), true}, // filled below
		{"contains slash", "repo/evil", true},
		{"contains space", "my repo", true},
		{"contains null byte", "repo\x00", true},
		{"contains semicolon", "repo;drop", true},
		{"contains newline", "repo\n", true},
		{"contains dollar", "repo$HOME", true},
		{"contains backtick", "repo`cmd`", true},
		{"contains parentheses", "repo(x)", true},
		{"path traversal", "../evil", true},
		{"absolute path", "/etc/passwd", true},
	}

	// Fill length-boundary cases with safe characters.
	for i := range tests {
		switch tests[i].name {
		case "max length (128)":
			buf := make([]byte, 128)
			for j := range buf {
				buf[j] = 'a'
			}
			tests[i].input = string(buf)
		case "too long (129)":
			buf := make([]byte, 129)
			for j := range buf {
				buf[j] = 'a'
			}
			tests[i].input = string(buf)
		}
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRepoName(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidatePath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// Create a subdirectory inside root for valid path tests.
	subDir := filepath.Join(root, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a symlink that escapes root (best-effort; skip on failure).
	escapeDest := t.TempDir() // another temp dir outside root
	symlinkPath := filepath.Join(root, "escape-link")
	symlinkOK := os.Symlink(escapeDest, symlinkPath) == nil

	tests := []struct {
		name     string
		path     string
		scanRoot string
		wantErr  bool
	}{
		// Valid paths (no scan root constraint)
		{"clean relative, no root", "some/path", "", false},
		{"clean absolute, no root", "/tmp/foo", "", false},
		{"path inside root (abs)", subDir, root, false},
		{"path inside root (rel)", "subdir", root, false},
		{"root itself", root, root, false},

		// Null bytes
		{"null byte in path", "foo\x00bar", "", true},

		// Shell metacharacters
		{"semicolon", "foo;bar", "", true},
		{"pipe", "foo|bar", "", true},
		{"ampersand", "foo&bar", "", true},
		{"backtick", "foo`bar`", "", true},
		{"dollar sign", "foo$HOME", "", true},
		{"open paren", "foo(bar", "", true},
		{"close paren", "foo)bar", "", true},
		{"open brace", "foo{bar", "", true},
		{"close brace", "foo}bar", "", true},
		{"less than", "foo<bar", "", true},
		{"greater than", "foo>bar", "", true},
		{"backslash", "foo\\bar", "", true},
		{"exclamation", "foo!bar", "", true},
		{"hash", "foo#bar", "", true},
		{"tilde", "foo~bar", "", true},
		{"asterisk", "foo*bar", "", true},
		{"question mark", "foo?bar", "", true},

		// Directory traversal
		{"double dot component", "../outside", root, true},
		{"embedded double dot", "subdir/../../outside", root, true},
		{"absolute escape", "/etc/passwd", root, true},
		{"absolute escape (home)", "/root", root, true},

		// Empty path
		{"empty path", "", root, true},
		{"empty path, no root", "", "", true},
	}

	if symlinkOK {
		tests = append(tests, struct {
			name     string
			path     string
			scanRoot string
			wantErr  bool
		}{"symlink escapes root", symlinkPath, root, true})
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePath(tc.path, tc.scanRoot)
			if tc.wantErr && err == nil {
				t.Errorf("expected error for path %q (root %q), got nil", tc.path, tc.scanRoot)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for path %q (root %q): %v", tc.path, tc.scanRoot, err)
			}
		})
	}
}

func TestSanitizeString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"hello world", "hello world"},
		{"with\x00null", "withnull"},
		{"\x00prefix", "prefix"},
		{"suffix\x00", "suffix"},
		{"multi\x00ple\x00nulls", "multiplenulls"},
		{"", ""},
		{"no nulls here", "no nulls here"},
	}
	for _, tc := range tests {
		got := SanitizeString(tc.input)
		if got != tc.want {
			t.Errorf("SanitizeString(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
