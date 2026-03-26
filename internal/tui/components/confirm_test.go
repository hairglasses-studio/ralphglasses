package components

import "testing"

func TestConfirmDialog_HandleKey_Enter(t *testing.T) {
	d := &ConfirmDialog{
		Active:   true,
		Title:    "Confirm",
		Message:  "Stop all loops?",
		Action:   "stopAll",
		Selected: 0,
	}

	// Yes selected
	result, dismissed := d.HandleKey("enter")
	if !dismissed {
		t.Error("expected dialog to be dismissed")
	}
	if result.Result != ConfirmYes {
		t.Errorf("result = %d, want ConfirmYes", result.Result)
	}
	if result.Action != "stopAll" {
		t.Errorf("action = %q", result.Action)
	}
}

func TestConfirmDialog_HandleKey_Navigate(t *testing.T) {
	d := &ConfirmDialog{Active: true, Selected: 0}

	d.HandleKey("right")
	if d.Selected != 1 {
		t.Errorf("after right: selected = %d, want 1", d.Selected)
	}
	d.HandleKey("right")
	if d.Selected != 2 {
		t.Errorf("after right: selected = %d, want 2", d.Selected)
	}
	d.HandleKey("right") // past end
	if d.Selected != 2 {
		t.Errorf("past end: selected = %d, want 2", d.Selected)
	}
	d.HandleKey("left")
	if d.Selected != 1 {
		t.Errorf("after left: selected = %d, want 1", d.Selected)
	}
}

func TestConfirmDialog_HandleKey_Escape(t *testing.T) {
	d := &ConfirmDialog{Active: true, Action: "test"}
	result, dismissed := d.HandleKey("esc")
	if !dismissed {
		t.Error("expected dismiss on escape")
	}
	if result.Result != ConfirmCancel {
		t.Errorf("result = %d, want ConfirmCancel", result.Result)
	}
}

func TestConfirmDialog_HandleKey_Y(t *testing.T) {
	d := &ConfirmDialog{
		Active: true,
		Title:  "Confirm",
		Action: "testY",
	}
	result, dismissed := d.HandleKey("y")
	if !dismissed {
		t.Error("expected dialog to be dismissed on 'y'")
	}
	if result.Result != ConfirmYes {
		t.Errorf("result = %d, want ConfirmYes", result.Result)
	}
	if result.Action != "testY" {
		t.Errorf("action = %q, want %q", result.Action, "testY")
	}
}

func TestConfirmDialog_HandleKey_N(t *testing.T) {
	d := &ConfirmDialog{
		Active: true,
		Title:  "Confirm",
		Action: "testN",
	}
	result, dismissed := d.HandleKey("n")
	if !dismissed {
		t.Error("expected dialog to be dismissed on 'n'")
	}
	if result.Result != ConfirmNo {
		t.Errorf("result = %d, want ConfirmNo", result.Result)
	}
	if result.Action != "testN" {
		t.Errorf("action = %q, want %q", result.Action, "testN")
	}
}

func TestConfirmDialog_View(t *testing.T) {
	d := &ConfirmDialog{
		Active:  true,
		Title:   "Test",
		Message: "Are you sure?",
		Width:   50,
	}
	view := d.View()
	if view == "" {
		t.Error("expected non-empty view")
	}

	// Check that title and message appear in rendered output (strip ANSI)
	stripped := StripAnsi(view)
	if !contains(stripped, "Test") {
		t.Errorf("view should contain title 'Test', got: %s", stripped)
	}
	if !contains(stripped, "Are you sure?") {
		t.Errorf("view should contain message 'Are you sure?', got: %s", stripped)
	}
	if !contains(stripped, "Yes") {
		t.Errorf("view should contain 'Yes' button, got: %s", stripped)
	}
	if !contains(stripped, "No") {
		t.Errorf("view should contain 'No' button, got: %s", stripped)
	}

	// Inactive should be empty
	d.Active = false
	if d.View() != "" {
		t.Error("inactive dialog should render empty")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && len(s) >= len(substr) && stringContains(s, substr)
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
