package mcpserver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateRepoName rejects names that contain anything other than
// alphanumerics, dashes, underscores, and dots, and enforces a max length.
// Empty string is also rejected.
func ValidateRepoName(name string) error {
	if name == "" {
		return fmt.Errorf("repo name must not be empty")
	}
	if len(name) > 128 {
		return fmt.Errorf("repo name exceeds 128 characters")
	}
	for _, r := range name {
		if !isRepoNameRune(r) {
			return fmt.Errorf("repo name contains invalid character %q", r)
		}
	}
	return nil
}

func isRepoNameRune(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '-' || r == '_' || r == '.'
}

// ValidatePath checks that p is safe to use as a filesystem path in the
// context of this server. It rejects:
//   - empty strings
//   - paths containing null bytes
//   - paths containing shell metacharacters (; | & ` $ ( ) { } < > \ ! # ~ *)
//   - paths whose Clean form contains ".." components (directory traversal)
//   - absolute paths that escape scanRoot (when scanRoot is non-empty)
//   - symlink targets that escape scanRoot (when scanRoot is non-empty)
func ValidatePath(p, scanRoot string) error {
	if p == "" {
		return fmt.Errorf("path must not be empty")
	}
	if strings.ContainsRune(p, 0) {
		return fmt.Errorf("path contains null byte")
	}
	const shellMeta = ";|&`$(){}[]<>\\!#~*?"
	for _, r := range shellMeta {
		if strings.ContainsRune(p, r) {
			return fmt.Errorf("path contains shell metacharacter %q", r)
		}
	}

	// Reject ".." components regardless of form.
	clean := filepath.Clean(p)
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		if part == ".." {
			return fmt.Errorf("path contains '..' component")
		}
	}

	if scanRoot == "" {
		return nil
	}

	// For absolute paths, verify they stay within scanRoot.
	absP := clean
	if !filepath.IsAbs(absP) {
		// Relative paths are resolved against scanRoot.
		absP = filepath.Join(scanRoot, clean)
	}

	absRoot, err := filepath.Abs(scanRoot)
	if err != nil {
		return fmt.Errorf("resolving scan root: %w", err)
	}
	absP, err = filepath.Abs(absP)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	if !strings.HasPrefix(absP+string(filepath.Separator), absRoot+string(filepath.Separator)) &&
		absP != absRoot {
		return fmt.Errorf("path escapes scan root")
	}

	// Resolve symlinks to prevent symlink-based traversal (best-effort: ignore
	// errors for paths that don't exist yet).
	realP, err := filepath.EvalSymlinks(absP)
	if err == nil {
		if !strings.HasPrefix(realP+string(filepath.Separator), absRoot+string(filepath.Separator)) &&
			realP != absRoot {
			return fmt.Errorf("symlink target escapes scan root")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("evaluating symlinks: %w", err)
	}

	return nil
}

// SanitizeString strips null bytes from s. It is intended for free-text
// fields (prompts, descriptions) where arbitrary content is allowed but
// null bytes would cause issues with C-string APIs downstream.
func SanitizeString(s string) string {
	return strings.ReplaceAll(s, "\x00", "")
}
