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

	// Inactive should be empty
	d.Active = false
	if d.View() != "" {
		t.Error("inactive dialog should render empty")
	}
}
