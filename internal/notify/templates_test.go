package notify

import "testing"

func TestRenderTemplate(t *testing.T) {
	tmpl := Template{
		Title: "Alert: {{.Type}}",
		Body:  "Session {{.SessionID}} spent ${{.Cost}}",
	}
	values := map[string]string{
		"Type":      "Budget Warning",
		"SessionID": "abc123",
		"Cost":      "4.50",
	}

	title, body := RenderTemplate(tmpl, values)
	if title != "Alert: Budget Warning" {
		t.Errorf("wrong title: %s", title)
	}
	if body != "Session abc123 spent $4.50" {
		t.Errorf("wrong body: %s", body)
	}
}

func TestDefaultTemplates(t *testing.T) {
	for _, et := range AllEventTypes() {
		if _, ok := DefaultTemplates[et]; !ok {
			t.Errorf("missing template for event type %s", et)
		}
	}
}
