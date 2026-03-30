package gitutil

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCreateTag(t *testing.T) {
	t.Parallel()

	// Set up a temporary git repository.
	dir := t.TempDir()
	commands := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	// Need at least one commit before we can tag.
	f, err := os.Create(dir + "/README.md")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("test")
	f.Close()

	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "init"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	ctx := context.Background()
	if err := CreateTag(ctx, dir, "v1.0.0"); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	// Verify the tag exists.
	cmd := exec.Command("git", "tag", "-l", "v1.0.0")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git tag list: %v", err)
	}
	if strings.TrimSpace(string(out)) != "v1.0.0" {
		t.Errorf("tag not found in output: %q", out)
	}
}

func TestCreateTag_NilContext(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	commands := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	f, _ := os.Create(dir + "/file.txt")
	f.WriteString("test")
	f.Close()

	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "init"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	//nolint:staticcheck // intentionally passing nil ctx
	if err := CreateTag(nil, dir, "test-tag"); err != nil {
		t.Fatalf("CreateTag with nil context: %v", err)
	}
}

func TestCreateTag_InvalidRepo(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	err := CreateTag(ctx, t.TempDir(), "v1.0.0")
	if err == nil {
		t.Error("CreateTag in non-git directory should fail")
	}
}

func TestCreateTag_DuplicateTag(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	f, _ := os.Create(dir + "/f.txt")
	f.WriteString("x")
	f.Close()

	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "init"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	ctx := context.Background()
	if err := CreateTag(ctx, dir, "dup-tag"); err != nil {
		t.Fatal(err)
	}
	// Second create with same name should fail.
	if err := CreateTag(ctx, dir, "dup-tag"); err == nil {
		t.Error("duplicate tag creation should fail")
	}
}

func TestMarathonTag_Format(t *testing.T) {
	t.Parallel()
	tag := MarathonTag(5)
	if !strings.HasPrefix(tag, "marathon-5-") {
		t.Errorf("tag %q does not start with marathon-5-", tag)
	}
	// Should contain a timestamp portion.
	parts := strings.SplitN(tag, "-", 3)
	if len(parts) < 3 {
		t.Errorf("tag %q has unexpected format", tag)
	}
}

func TestMarathonTag_UniqueTimestamps(t *testing.T) {
	t.Parallel()
	tag1 := MarathonTag(1)
	tag2 := MarathonTag(2)
	if tag1 == tag2 {
		t.Error("different counts should produce different tags")
	}
}
