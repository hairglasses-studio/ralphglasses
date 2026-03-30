package session

import "testing"

func TestInterpolatePrompt(t *testing.T) {
	tmpl := "Fix all lint issues in {{.Repo}} on branch {{.Branch}}"
	vars := map[string]string{
		"Repo":   "ralphglasses",
		"Branch": "main",
	}

	result := InterpolatePrompt(tmpl, vars)
	if result != "Fix all lint issues in ralphglasses on branch main" {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestInterpolatePrompt_Builtins(t *testing.T) {
	tmpl := "Running on {{.Date}} by {{.User}}"
	result := InterpolatePrompt(tmpl, map[string]string{})

	if result == tmpl {
		t.Error("builtins should have been interpolated")
	}
}

func TestListVariables(t *testing.T) {
	tmpl := "Fix {{.Repo}} lint on {{.Branch}} ({{.Repo}} again)"
	vars := ListVariables(tmpl)

	if len(vars) != 2 {
		t.Fatalf("expected 2 unique vars, got %d: %v", len(vars), vars)
	}
	if vars[0] != "Repo" || vars[1] != "Branch" {
		t.Errorf("unexpected vars: %v", vars)
	}
}

func TestListVariables_NoVars(t *testing.T) {
	vars := ListVariables("no variables here")
	if len(vars) != 0 {
		t.Errorf("expected 0 vars, got %v", vars)
	}
}
