package session

import (
	"os/exec"
	"testing"
)

func TestCreateReviewPR_NoGHCLI(t *testing.T) {
	t.Parallel()

	// If gh CLI is available, skip this test as we can't mock the lookup easily.
	if _, err := exec.LookPath("gh"); err == nil {
		// gh is available, so we test that it returns an error when used
		// in a non-git directory.
		dir := t.TempDir()
		_, err := CreateReviewPR(dir, "main", "test PR", []string{"file.go"})
		if err == nil {
			t.Error("expected error when running in non-git directory")
		}
		return
	}

	// gh CLI not found -- should return error.
	dir := t.TempDir()
	_, err := CreateReviewPR(dir, "main", "test PR", []string{"file.go"})
	if err == nil {
		t.Fatal("expected error when gh CLI is not found")
	}
}
