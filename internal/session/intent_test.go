package session

import "testing"

func TestIntentValidate_Launch(t *testing.T) {
	t.Parallel()
	i := &Intent{Type: IntentLaunch}
	if err := i.Validate(); err == nil {
		t.Error("expected error for launch without provider")
	}
	i.Provider = ProviderClaude
	if err := i.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIntentValidate_Stop(t *testing.T) {
	t.Parallel()
	i := &Intent{Type: IntentStop}
	if err := i.Validate(); err == nil {
		t.Error("expected error for stop without session_id")
	}
	i.SessionID = "sess-1"
	if err := i.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIntentValidate_Unknown(t *testing.T) {
	t.Parallel()
	i := &Intent{Type: "bogus"}
	if err := i.Validate(); err == nil {
		t.Error("expected error for unknown type")
	}
}

func TestIntentToEvent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		intentType IntentType
		eventType  SessionEventType
	}{
		{IntentLaunch, EventCreated},
		{IntentStop, EventStopped},
		{IntentPause, EventPaused},
		{IntentResume, EventResumed},
	}
	for _, tc := range cases {
		i := &Intent{Type: tc.intentType, SessionID: "s1"}
		e := i.ToEvent()
		if e.Type != tc.eventType {
			t.Errorf("%s: expected event %s, got %s", tc.intentType, tc.eventType, e.Type)
		}
	}
}

func TestIntentRouter_RouteNL(t *testing.T) {
	t.Parallel()
	r := NewIntentRouter()

	cases := []struct {
		input      string
		expectType IntentType
	}{
		{"start claude session", IntentLaunch},
		{"launch gemini", IntentLaunch},
		{"stop sess-1", IntentStop},
		{"kill sess-2", IntentStop},
		{"pause sess-1", IntentPause},
		{"resume sess-1", IntentResume},
		{"scale to 5", IntentScale},
		{"status", IntentQuery},
		{"list sessions", IntentQuery},
		{"show running", IntentQuery},
		{"help me", IntentEscalate},
		{"what is this?", IntentEscalate},
	}

	for _, tc := range cases {
		intent, err := r.RouteNL(tc.input)
		if err != nil {
			t.Errorf("RouteNL(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if intent.Type != tc.expectType {
			t.Errorf("RouteNL(%q): expected %s, got %s", tc.input, tc.expectType, intent.Type)
		}
		if intent.Source != "nl" {
			t.Errorf("RouteNL(%q): expected source 'nl', got %s", tc.input, intent.Source)
		}
	}
}

func TestIntentRouter_RouteNL_UnknownCommand(t *testing.T) {
	t.Parallel()
	r := NewIntentRouter()
	_, err := r.RouteNL("frobnicate the widget")
	if err == nil {
		t.Error("expected error for unclassifiable command")
	}
}

func TestIntentRouter_RouteMCP(t *testing.T) {
	t.Parallel()
	r := NewIntentRouter()

	intent, err := r.RouteMCP("ralphglasses_session_launch", map[string]string{
		"provider": "claude",
		"prompt":   "analyze this",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if intent.Type != IntentLaunch {
		t.Errorf("expected launch, got %s", intent.Type)
	}
	if intent.Provider != ProviderClaude {
		t.Errorf("expected claude, got %s", intent.Provider)
	}
}

func TestIntentRouter_DestructiveFlag(t *testing.T) {
	t.Parallel()
	r := NewIntentRouter()

	intent, _ := r.RouteNL("stop sess-1")
	if !intent.Destructive {
		t.Error("stop should be flagged as destructive")
	}

	intent, _ = r.RouteNL("status")
	if intent.Destructive {
		t.Error("status should not be destructive")
	}
}

func TestIntentRouter_ProviderClassification(t *testing.T) {
	t.Parallel()
	r := NewIntentRouter()

	tests := []struct {
		input    string
		provider Provider
	}{
		{"launch claude analysis", ProviderClaude},
		{"start gemini scan", ProviderGemini},
		{"launch codex fix", ProviderCodex},
		{"start something", ProviderClaude}, // default
	}

	for _, tc := range tests {
		intent, _ := r.RouteNL(tc.input)
		if intent.Provider != tc.provider {
			t.Errorf("RouteNL(%q): expected %s, got %s", tc.input, tc.provider, intent.Provider)
		}
	}
}
