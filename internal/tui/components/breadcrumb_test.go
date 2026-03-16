package components

import "testing"

func TestBreadcrumbPushPop(t *testing.T) {
	b := Breadcrumb{Parts: []string{"Overview"}}
	b.Push("repo-detail")
	if len(b.Parts) != 2 {
		t.Errorf("after push: len = %d, want 2", len(b.Parts))
	}
	b.Pop()
	if len(b.Parts) != 1 {
		t.Errorf("after pop: len = %d, want 1", len(b.Parts))
	}
}

func TestBreadcrumbPopEmpty(t *testing.T) {
	b := Breadcrumb{}
	b.Pop() // should not panic
}

func TestBreadcrumbReset(t *testing.T) {
	b := Breadcrumb{Parts: []string{"Overview", "detail", "logs"}}
	b.Reset()
	if len(b.Parts) != 1 {
		t.Errorf("after reset: len = %d, want 1", len(b.Parts))
	}
}

func TestBreadcrumbView(t *testing.T) {
	b := Breadcrumb{Parts: []string{"Overview"}}
	if b.View() == "" {
		t.Error("single-part view should not be empty")
	}
}

func TestBreadcrumbViewEmpty(t *testing.T) {
	b := Breadcrumb{}
	if b.View() != "" {
		t.Error("empty breadcrumb view should be empty string")
	}
}
