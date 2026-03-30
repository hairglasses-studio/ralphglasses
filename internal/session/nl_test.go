package session

import (
	"context"
	"testing"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"start 5 sessions", []string{"start", "5", "sessions"}},
		{"  STOP all Claude sessions  ", []string{"stop", "all", "claude", "sessions"}},
		{"show cost for today!", []string{"show", "cost", "for", "today"}},
		{"", nil},
		{"???", nil},
		{"scale fleet to 10", []string{"scale", "fleet", "to", "10"}},
		{"pause session 3", []string{"pause", "session", "3"}},
	}
	for _, tt := range tests {
		got := tokenize(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("tokenize(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("tokenize(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestDetectIntent(t *testing.T) {
	tests := []struct {
		tokens []string
		want   string
	}{
		{[]string{"start", "5", "sessions"}, "start"},
		{[]string{"launch", "a", "session"}, "start"},
		{[]string{"kill", "session", "3"}, "stop"},
		{[]string{"show", "cost"}, "report"},
		{[]string{"scale", "fleet", "to", "10"}, "scale"},
		{[]string{"pause", "session", "3"}, "pause"},
		{[]string{"resume", "session", "3"}, "resume"},
		{[]string{"status"}, "status"},
		{[]string{"something", "random"}, ""},
	}
	for _, tt := range tests {
		got := detectIntent(tt.tokens)
		if got != tt.want {
			t.Errorf("detectIntent(%v) = %q, want %q", tt.tokens, got, tt.want)
		}
	}
}

func TestExtractCount(t *testing.T) {
	tests := []struct {
		tokens []string
		want   int
	}{
		{[]string{"start", "5", "sessions"}, 5},
		{[]string{"scale", "fleet", "to", "10"}, 10},
		{[]string{"no", "numbers"}, 0},
		{[]string{"0", "is", "not", "positive"}, 0},
	}
	for _, tt := range tests {
		got := extractCount(tt.tokens)
		if got != tt.want {
			t.Errorf("extractCount(%v) = %d, want %d", tt.tokens, got, tt.want)
		}
	}
}

func TestExtractProvider(t *testing.T) {
	tests := []struct {
		tokens   []string
		wantProv Provider
		wantOK   bool
	}{
		{[]string{"stop", "all", "claude", "sessions"}, ProviderClaude, true},
		{[]string{"start", "gemini", "session"}, ProviderGemini, true},
		{[]string{"start", "openai", "session"}, ProviderCodex, true},
		{[]string{"start", "session"}, "", false},
	}
	for _, tt := range tests {
		p, ok := extractProvider(tt.tokens)
		if p != tt.wantProv || ok != tt.wantOK {
			t.Errorf("extractProvider(%v) = (%q, %v), want (%q, %v)", tt.tokens, p, ok, tt.wantProv, tt.wantOK)
		}
	}
}

func TestExtractSessionID(t *testing.T) {
	tests := []struct {
		tokens []string
		want   string
	}{
		{[]string{"pause", "session", "3"}, "3"},
		{[]string{"stop", "session", "42"}, "42"},
		{[]string{"stop", "all"}, ""},
		{[]string{"session"}, ""},
	}
	for _, tt := range tests {
		got := extractSessionID(tt.tokens)
		if got != tt.want {
			t.Errorf("extractSessionID(%v) = %q, want %q", tt.tokens, got, tt.want)
		}
	}
}

func TestExtractProject(t *testing.T) {
	tests := []struct {
		tokens []string
		want   string
	}{
		{[]string{"start", "5", "sessions", "on", "project", "myapp"}, "myapp"},
		{[]string{"start", "sessions", "on", "ralphglasses"}, "ralphglasses"},
		{[]string{"start", "sessions"}, ""},
	}
	for _, tt := range tests {
		got := extractProject(tt.tokens)
		if got != tt.want {
			t.Errorf("extractProject(%v) = %q, want %q", tt.tokens, got, tt.want)
		}
	}
}

func TestParseStartSessions(t *testing.T) {
	ctrl := NewNLController(NewManager())
	cmd, err := ctrl.Parse("start 5 sessions on project myapp")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Action != ActionStart {
		t.Errorf("action = %q, want %q", cmd.Action, ActionStart)
	}
	if cmd.Parameters["count"] != "5" {
		t.Errorf("count = %q, want %q", cmd.Parameters["count"], "5")
	}
	if cmd.Parameters["project"] != "myapp" {
		t.Errorf("project = %q, want %q", cmd.Parameters["project"], "myapp")
	}
	if cmd.Target != "sessions" {
		t.Errorf("target = %q, want %q", cmd.Target, "sessions")
	}
}

func TestParseStopAllClaude(t *testing.T) {
	ctrl := NewNLController(NewManager())
	cmd, err := ctrl.Parse("stop all claude sessions")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Action != ActionStop {
		t.Errorf("action = %q, want %q", cmd.Action, ActionStop)
	}
	if cmd.Parameters["all"] != "true" {
		t.Errorf("all = %q, want %q", cmd.Parameters["all"], "true")
	}
	if cmd.Parameters["provider"] != string(ProviderClaude) {
		t.Errorf("provider = %q, want %q", cmd.Parameters["provider"], string(ProviderClaude))
	}
}

func TestParseShowCostToday(t *testing.T) {
	ctrl := NewNLController(NewManager())
	cmd, err := ctrl.Parse("show cost for today")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Action != ActionReport {
		t.Errorf("action = %q, want %q", cmd.Action, ActionReport)
	}
	if cmd.Target != "cost" {
		t.Errorf("target = %q, want %q", cmd.Target, "cost")
	}
	if cmd.Parameters["time_range"] != "today" {
		t.Errorf("time_range = %q, want %q", cmd.Parameters["time_range"], "today")
	}
}

func TestParseScaleFleet(t *testing.T) {
	ctrl := NewNLController(NewManager())
	cmd, err := ctrl.Parse("scale fleet to 10")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Action != ActionScale {
		t.Errorf("action = %q, want %q", cmd.Action, ActionScale)
	}
	if cmd.Target != "fleet" {
		t.Errorf("target = %q, want %q", cmd.Target, "fleet")
	}
	if cmd.Parameters["count"] != "10" {
		t.Errorf("count = %q, want %q", cmd.Parameters["count"], "10")
	}
}

func TestParsePauseSession(t *testing.T) {
	ctrl := NewNLController(NewManager())
	cmd, err := ctrl.Parse("pause session 3")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Action != ActionPause {
		t.Errorf("action = %q, want %q", cmd.Action, ActionPause)
	}
	if cmd.Parameters["session_id"] != "3" {
		t.Errorf("session_id = %q, want %q", cmd.Parameters["session_id"], "3")
	}
}

func TestParseResumeSession(t *testing.T) {
	ctrl := NewNLController(NewManager())
	cmd, err := ctrl.Parse("resume session 7")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Action != ActionResume {
		t.Errorf("action = %q, want %q", cmd.Action, ActionResume)
	}
	if cmd.Parameters["session_id"] != "7" {
		t.Errorf("session_id = %q, want %q", cmd.Parameters["session_id"], "7")
	}
}

func TestParseLaunchAlias(t *testing.T) {
	ctrl := NewNLController(NewManager())
	cmd, err := ctrl.Parse("launch 3 gemini sessions")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Action != ActionStart {
		t.Errorf("action = %q, want %q", cmd.Action, ActionStart)
	}
	if cmd.Parameters["count"] != "3" {
		t.Errorf("count = %q, want %q", cmd.Parameters["count"], "3")
	}
	if cmd.Parameters["provider"] != string(ProviderGemini) {
		t.Errorf("provider = %q, want %q", cmd.Parameters["provider"], string(ProviderGemini))
	}
}

func TestParseKillAlias(t *testing.T) {
	ctrl := NewNLController(NewManager())
	cmd, err := ctrl.Parse("kill session 5")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Action != ActionStop {
		t.Errorf("action = %q, want %q", cmd.Action, ActionStop)
	}
	if cmd.Parameters["session_id"] != "5" {
		t.Errorf("session_id = %q, want %q", cmd.Parameters["session_id"], "5")
	}
}

func TestParseEmptyInput(t *testing.T) {
	ctrl := NewNLController(NewManager())
	_, err := ctrl.Parse("")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParseUnknownIntent(t *testing.T) {
	ctrl := NewNLController(NewManager())
	_, err := ctrl.Parse("foobar baz quux")
	if err == nil {
		t.Fatal("expected error for unrecognizable intent")
	}
}

func TestParseStatus(t *testing.T) {
	ctrl := NewNLController(NewManager())
	cmd, err := ctrl.Parse("status")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Action != ActionStatus {
		t.Errorf("action = %q, want %q", cmd.Action, ActionStatus)
	}
	if cmd.Target != "fleet" {
		t.Errorf("target = %q, want %q", cmd.Target, "fleet")
	}
}

func TestExecuteStart(t *testing.T) {
	ctrl := NewNLController(NewManager())
	cmd, err := ctrl.Parse("start 5 sessions on project myapp")
	if err != nil {
		t.Fatal(err)
	}
	if err := ctrl.Execute(context.Background(), cmd); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
}

func TestExecuteStopRequiresTarget(t *testing.T) {
	ctrl := NewNLController(NewManager())
	cmd := &Command{
		Action:     ActionStop,
		Parameters: map[string]string{},
	}
	err := ctrl.Execute(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected error for stop without target")
	}
}

func TestExecutePauseRequiresID(t *testing.T) {
	ctrl := NewNLController(NewManager())
	cmd := &Command{
		Action:     ActionPause,
		Parameters: map[string]string{},
	}
	err := ctrl.Execute(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected error for pause without session ID")
	}
}

func TestExecuteResumeRequiresID(t *testing.T) {
	ctrl := NewNLController(NewManager())
	cmd := &Command{
		Action:     ActionResume,
		Parameters: map[string]string{},
	}
	err := ctrl.Execute(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected error for resume without session ID")
	}
}

func TestExecuteScaleRequiresCount(t *testing.T) {
	ctrl := NewNLController(NewManager())
	cmd := &Command{
		Action:     ActionScale,
		Parameters: map[string]string{},
	}
	err := ctrl.Execute(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected error for scale without count")
	}
}

func TestExecuteScaleOverMax(t *testing.T) {
	ctrl := NewNLController(NewManager())
	cmd := &Command{
		Action:     ActionScale,
		Target:     "fleet",
		Parameters: map[string]string{"count": "200"},
	}
	err := ctrl.Execute(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected error for scale count > 100")
	}
}

func TestExecuteNilCommand(t *testing.T) {
	ctrl := NewNLController(NewManager())
	err := ctrl.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil command")
	}
}

func TestExecuteUnknownAction(t *testing.T) {
	ctrl := NewNLController(NewManager())
	cmd := &Command{
		Action:     "dance",
		Parameters: map[string]string{},
	}
	err := ctrl.Execute(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
}

func TestRegisterCustomHandler(t *testing.T) {
	ctrl := NewNLController(NewManager())
	called := false
	ctrl.RegisterHandler("start", func(_ context.Context, _ *Manager, _ *Command) error {
		called = true
		return nil
	})
	cmd, err := ctrl.Parse("start 2 sessions")
	if err != nil {
		t.Fatal(err)
	}
	if err := ctrl.Execute(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("custom handler was not called")
	}
}

func TestParseShowSpending(t *testing.T) {
	ctrl := NewNLController(NewManager())
	cmd, err := ctrl.Parse("show spending for yesterday")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Action != ActionReport {
		t.Errorf("action = %q, want %q", cmd.Action, ActionReport)
	}
	if cmd.Target != "cost" {
		t.Errorf("target = %q, want %q", cmd.Target, "cost")
	}
	if cmd.Parameters["time_range"] != "yesterday" {
		t.Errorf("time_range = %q, want %q", cmd.Parameters["time_range"], "yesterday")
	}
}

func TestParseReportHealth(t *testing.T) {
	ctrl := NewNLController(NewManager())
	cmd, err := ctrl.Parse("report health")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Action != ActionReport {
		t.Errorf("action = %q, want %q", cmd.Action, ActionReport)
	}
	if cmd.Target != "health" {
		t.Errorf("target = %q, want %q", cmd.Target, "health")
	}
}

func TestExecuteReport(t *testing.T) {
	ctrl := NewNLController(NewManager())
	cmd, err := ctrl.Parse("show cost for today")
	if err != nil {
		t.Fatal(err)
	}
	if err := ctrl.Execute(context.Background(), cmd); err != nil {
		t.Fatalf("Execute report failed: %v", err)
	}
}

func TestExecuteStatus(t *testing.T) {
	ctrl := NewNLController(NewManager())
	cmd, err := ctrl.Parse("status")
	if err != nil {
		t.Fatal(err)
	}
	if err := ctrl.Execute(context.Background(), cmd); err != nil {
		t.Fatalf("Execute status failed: %v", err)
	}
}
