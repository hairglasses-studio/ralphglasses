package session

import (
	"testing"
)

func TestIsProtectedBranch_Main(t *testing.T) {
	if !IsProtectedBranch("main") {
		t.Error("IsProtectedBranch(main) should return true")
	}
}

func TestIsProtectedBranch_Master(t *testing.T) {
	if !IsProtectedBranch("master") {
		t.Error("IsProtectedBranch(master) should return true")
	}
}

func TestIsProtectedBranch_Feature(t *testing.T) {
	if IsProtectedBranch("feature/my-feature") {
		t.Error("IsProtectedBranch(feature branch) should return false")
	}
}

func TestIsProtectedBranch_Empty(t *testing.T) {
	if IsProtectedBranch("") {
		t.Error("IsProtectedBranch('') should return false")
	}
}
