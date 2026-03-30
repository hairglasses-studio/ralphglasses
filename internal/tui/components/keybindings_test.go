package components

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRuneKey(t *testing.T) {
	k := RuneKey('j')
	if k.Type != tea.KeyRunes {
		t.Errorf("Type = %v, want KeyRunes", k.Type)
	}
	if k.Rune != 'j' {
		t.Errorf("Rune = %c, want j", k.Rune)
	}
	if k.Ctrl || k.Alt {
		t.Error("modifiers should be false")
	}
}

func TestCtrlKey(t *testing.T) {
	k := CtrlKey('c')
	if k.Type != tea.KeyRunes {
		t.Errorf("Type = %v, want KeyRunes", k.Type)
	}
	if k.Rune != 'c' {
		t.Errorf("Rune = %c, want c", k.Rune)
	}
	if !k.Ctrl {
		t.Error("Ctrl should be true")
	}
}

func TestSpecialKey(t *testing.T) {
	k := SpecialKey(tea.KeyEnter)
	if k.Type != tea.KeyEnter {
		t.Errorf("Type = %v, want KeyEnter", k.Type)
	}
	if k.Rune != 0 {
		t.Errorf("Rune = %c, want 0", k.Rune)
	}
}

func TestKeyEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b Key
		want bool
	}{
		{"same rune", RuneKey('j'), RuneKey('j'), true},
		{"diff rune", RuneKey('j'), RuneKey('k'), false},
		{"same special", SpecialKey(tea.KeyEnter), SpecialKey(tea.KeyEnter), true},
		{"diff special", SpecialKey(tea.KeyEnter), SpecialKey(tea.KeyEscape), false},
		{"rune vs special", RuneKey('j'), SpecialKey(tea.KeyUp), false},
		{"ctrl modifier", CtrlKey('c'), CtrlKey('c'), true},
		{"ctrl vs no ctrl", CtrlKey('c'), RuneKey('c'), false},
		{"alt modifier", Key{Type: tea.KeyRunes, Rune: 'x', Alt: true}, Key{Type: tea.KeyRunes, Rune: 'x', Alt: true}, true},
		{"alt vs no alt", Key{Type: tea.KeyRunes, Rune: 'x', Alt: true}, RuneKey('x'), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Equal(tt.b); got != tt.want {
				t.Errorf("Equal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKeyString(t *testing.T) {
	tests := []struct {
		key  Key
		want string
	}{
		{RuneKey('j'), "j"},
		{RuneKey('?'), "?"},
		{RuneKey(' '), "space"},
		{CtrlKey('c'), "ctrl+c"},
		{Key{Type: tea.KeyRunes, Rune: 'x', Alt: true}, "alt+x"},
		{Key{Type: tea.KeyRunes, Rune: 'z', Ctrl: true, Alt: true}, "ctrl+alt+z"},
		{SpecialKey(tea.KeyEnter), "enter"},
		{SpecialKey(tea.KeyEscape), "esc"},
		{SpecialKey(tea.KeyUp), "up"},
		{SpecialKey(tea.KeyTab), "tab"},
		{SpecialKey(tea.KeyShiftTab), "shift+tab"},
		{SpecialKey(tea.KeyPgUp), "pgup"},
		{SpecialKey(tea.KeyF1), "f1"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.key.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestKeyFromMsg_Rune(t *testing.T) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	k := KeyFromMsg(msg)
	if k.Type != tea.KeyRunes || k.Rune != 'j' {
		t.Errorf("got %+v, want rune 'j'", k)
	}
}

func TestKeyFromMsg_Special(t *testing.T) {
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	k := KeyFromMsg(msg)
	if k.Type != tea.KeyEnter {
		t.Errorf("got Type=%v, want KeyEnter", k.Type)
	}
}

func TestKeyFromMsg_CtrlC(t *testing.T) {
	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	k := KeyFromMsg(msg)
	if !k.Ctrl || k.Rune != 'c' || k.Type != tea.KeyRunes {
		t.Errorf("got %+v, want ctrl+c as Ctrl=true Rune='c'", k)
	}
}

func TestKeyFromMsg_Alt(t *testing.T) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}, Alt: true}
	k := KeyFromMsg(msg)
	if !k.Alt || k.Rune != 'x' {
		t.Errorf("got %+v, want alt+x", k)
	}
}

func TestNewKeyMap_Resolve(t *testing.T) {
	km := NewKeyMap([]Binding{
		{RuneKey('j'), ActionNavDown},
		{SpecialKey(tea.KeyUp), ActionNavUp},
	})

	if got := km.Resolve(RuneKey('j')); got != ActionNavDown {
		t.Errorf("Resolve(j) = %q, want %q", got, ActionNavDown)
	}
	if got := km.Resolve(SpecialKey(tea.KeyUp)); got != ActionNavUp {
		t.Errorf("Resolve(up) = %q, want %q", got, ActionNavUp)
	}
	if got := km.Resolve(RuneKey('z')); got != "" {
		t.Errorf("Resolve(z) = %q, want empty", got)
	}
}

func TestKeyMap_ResolveMsg(t *testing.T) {
	km := NewKeyMap([]Binding{
		{RuneKey('j'), ActionNavDown},
	})
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	if got := km.ResolveMsg(msg); got != ActionNavDown {
		t.Errorf("ResolveMsg = %q, want %q", got, ActionNavDown)
	}
}

func TestKeyMap_KeysFor(t *testing.T) {
	km := NewKeyMap([]Binding{
		{RuneKey('k'), ActionNavUp},
		{SpecialKey(tea.KeyUp), ActionNavUp},
		{RuneKey('j'), ActionNavDown},
	})

	keys := km.KeysFor(ActionNavUp)
	if len(keys) != 2 {
		t.Fatalf("got %d keys, want 2", len(keys))
	}
	// First should be 'k', second should be up arrow.
	if keys[0].Rune != 'k' {
		t.Errorf("keys[0] = %v, want 'k'", keys[0])
	}
	if keys[1].Type != tea.KeyUp {
		t.Errorf("keys[1] = %v, want KeyUp", keys[1])
	}

	// Unbound action.
	if keys := km.KeysFor(ActionQuit); len(keys) != 0 {
		t.Errorf("unbound action should have 0 keys, got %d", len(keys))
	}
}

func TestKeyMap_Bindings_IsCopy(t *testing.T) {
	km := NewKeyMap([]Binding{
		{RuneKey('j'), ActionNavDown},
	})
	bindings := km.Bindings()
	bindings[0].Action = "mutated"
	if km.Resolve(RuneKey('j')) == "mutated" {
		t.Error("Bindings() should return a copy")
	}
}

func TestKeyMap_Conflicts_None(t *testing.T) {
	km := NewKeyMap([]Binding{
		{RuneKey('j'), ActionNavDown},
		{RuneKey('k'), ActionNavUp},
	})
	if conflicts := km.Conflicts(); len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %v", conflicts)
	}
}

func TestKeyMap_Conflicts_Detected(t *testing.T) {
	km := NewKeyMap([]Binding{
		{RuneKey('j'), ActionNavDown},
		{RuneKey('j'), ActionNavUp}, // conflict: same key, different action
	})
	conflicts := km.Conflicts()
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	c := conflicts[0]
	if c.Key.Rune != 'j' {
		t.Errorf("conflict key = %v, want 'j'", c.Key)
	}
	if len(c.Actions) != 2 {
		t.Errorf("conflict actions = %d, want 2", len(c.Actions))
	}
}

func TestKeyMap_Conflicts_SameActionNoDuplicate(t *testing.T) {
	// Same key bound to same action twice should NOT be a conflict.
	km := NewKeyMap([]Binding{
		{RuneKey('j'), ActionNavDown},
		{RuneKey('j'), ActionNavDown},
	})
	if conflicts := km.Conflicts(); len(conflicts) != 0 {
		t.Errorf("same key same action should not conflict, got %v", conflicts)
	}
}

func TestConflictString(t *testing.T) {
	c := Conflict{
		Key:     RuneKey('j'),
		Actions: []Action{ActionNavDown, ActionNavUp},
	}
	s := c.String()
	if !strings.Contains(s, "j") || !strings.Contains(s, "nav.down") || !strings.Contains(s, "nav.up") {
		t.Errorf("unexpected conflict string: %s", s)
	}
}

func TestMerge_OverrideReplaces(t *testing.T) {
	base := NewKeyMap([]Binding{
		{RuneKey('j'), ActionNavDown},
		{RuneKey('k'), ActionNavUp},
	})
	overrides := NewKeyMap([]Binding{
		{RuneKey('j'), ActionSessionStart}, // rebind j
	})

	merged := Merge(base, overrides)
	if got := merged.Resolve(RuneKey('j')); got != ActionSessionStart {
		t.Errorf("merged j = %q, want session.start", got)
	}
	if got := merged.Resolve(RuneKey('k')); got != ActionNavUp {
		t.Errorf("merged k = %q, want nav.up", got)
	}
}

func TestMerge_OverrideAddsNew(t *testing.T) {
	base := NewKeyMap([]Binding{
		{RuneKey('j'), ActionNavDown},
	})
	overrides := NewKeyMap([]Binding{
		{RuneKey('z'), ActionQuit},
	})

	merged := Merge(base, overrides)
	if got := merged.Resolve(RuneKey('j')); got != ActionNavDown {
		t.Errorf("merged j = %q, want nav.down", got)
	}
	if got := merged.Resolve(RuneKey('z')); got != ActionQuit {
		t.Errorf("merged z = %q, want quit", got)
	}
}

func TestMerge_NoConflicts(t *testing.T) {
	base := NewKeyMap([]Binding{
		{RuneKey('j'), ActionNavDown},
	})
	overrides := NewKeyMap([]Binding{
		{RuneKey('j'), ActionSessionStart},
	})

	merged := Merge(base, overrides)
	// Should have no conflicts since override replaces base.
	if conflicts := merged.Conflicts(); len(conflicts) != 0 {
		t.Errorf("merged should have no conflicts, got %v", conflicts)
	}
}

func TestDefaultKeyMap_HasExpectedBindings(t *testing.T) {
	km := DefaultKeyMap()

	// Navigation keys.
	if got := km.Resolve(RuneKey('j')); got != ActionNavDown {
		t.Errorf("j = %q, want nav.down", got)
	}
	if got := km.Resolve(RuneKey('k')); got != ActionNavUp {
		t.Errorf("k = %q, want nav.up", got)
	}
	if got := km.Resolve(SpecialKey(tea.KeyUp)); got != ActionNavUp {
		t.Errorf("up = %q, want nav.up", got)
	}
	if got := km.Resolve(SpecialKey(tea.KeyEnter)); got != ActionNavEnter {
		t.Errorf("enter = %q, want nav.enter", got)
	}

	// View keys.
	if got := km.Resolve(RuneKey('1')); got != ActionViewSessions {
		t.Errorf("1 = %q, want view.sessions", got)
	}

	// Session keys.
	if got := km.Resolve(RuneKey('s')); got != ActionSessionStart {
		t.Errorf("s = %q, want session.start", got)
	}

	// Toggle keys.
	if got := km.Resolve(RuneKey('?')); got != ActionToggleHelp {
		t.Errorf("? = %q, want toggle.help", got)
	}

	// Global keys.
	if got := km.Resolve(RuneKey('q')); got != ActionQuit {
		t.Errorf("q = %q, want quit", got)
	}
}

func TestParseKey_Rune(t *testing.T) {
	k, err := ParseKey("j")
	if err != nil {
		t.Fatal(err)
	}
	if !k.Equal(RuneKey('j')) {
		t.Errorf("got %v, want RuneKey('j')", k)
	}
}

func TestParseKey_SpecialChar(t *testing.T) {
	k, err := ParseKey("?")
	if err != nil {
		t.Fatal(err)
	}
	if !k.Equal(RuneKey('?')) {
		t.Errorf("got %v, want RuneKey('?')", k)
	}
}

func TestParseKey_CtrlCombo(t *testing.T) {
	k, err := ParseKey("ctrl+c")
	if err != nil {
		t.Fatal(err)
	}
	if !k.Equal(CtrlKey('c')) {
		t.Errorf("got %v, want CtrlKey('c')", k)
	}
}

func TestParseKey_AltCombo(t *testing.T) {
	k, err := ParseKey("alt+x")
	if err != nil {
		t.Fatal(err)
	}
	expected := Key{Type: tea.KeyRunes, Rune: 'x', Alt: true}
	if !k.Equal(expected) {
		t.Errorf("got %v, want %v", k, expected)
	}
}

func TestParseKey_SpecialNames(t *testing.T) {
	tests := []struct {
		input string
		want  Key
	}{
		{"enter", SpecialKey(tea.KeyEnter)},
		{"esc", SpecialKey(tea.KeyEscape)},
		{"escape", SpecialKey(tea.KeyEscape)},
		{"tab", SpecialKey(tea.KeyTab)},
		{"up", SpecialKey(tea.KeyUp)},
		{"down", SpecialKey(tea.KeyDown)},
		{"left", SpecialKey(tea.KeyLeft)},
		{"right", SpecialKey(tea.KeyRight)},
		{"pgup", SpecialKey(tea.KeyPgUp)},
		{"pageup", SpecialKey(tea.KeyPgUp)},
		{"pgdown", SpecialKey(tea.KeyPgDown)},
		{"pagedown", SpecialKey(tea.KeyPgDown)},
		{"home", SpecialKey(tea.KeyHome)},
		{"end", SpecialKey(tea.KeyEnd)},
		{"space", SpecialKey(tea.KeySpace)},
		{"f1", SpecialKey(tea.KeyF1)},
		{"f12", SpecialKey(tea.KeyF12)},
		{"backspace", SpecialKey(tea.KeyBackspace)},
		{"delete", SpecialKey(tea.KeyDelete)},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			k, err := ParseKey(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if !k.Equal(tt.want) {
				t.Errorf("got %v, want %v", k, tt.want)
			}
		})
	}
}

func TestParseKey_CaseInsensitive(t *testing.T) {
	k, err := ParseKey("ENTER")
	if err != nil {
		t.Fatal(err)
	}
	if !k.Equal(SpecialKey(tea.KeyEnter)) {
		t.Errorf("got %v, want enter key", k)
	}
}

func TestParseKey_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"unknown multi-char", "foobar"},
		{"bad modifier", "super+c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseKey(tt.input)
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestParseBinding_Valid(t *testing.T) {
	b, err := ParseBinding("ctrl+c = quit")
	if err != nil {
		t.Fatal(err)
	}
	if !b.Key.Equal(CtrlKey('c')) {
		t.Errorf("key = %v, want ctrl+c", b.Key)
	}
	if b.Action != ActionQuit {
		t.Errorf("action = %q, want quit", b.Action)
	}
}

func TestParseBinding_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"no equals", "ctrl+c"},
		{"empty action", "j="},
		{"bad key", "super+x=quit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseBinding(tt.input)
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestParseOverrides_Valid(t *testing.T) {
	entries := []string{
		"j=nav.down",
		"ctrl+c=quit",
		"?=toggle.help",
	}
	bindings, err := ParseOverrides(entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 3 {
		t.Fatalf("got %d bindings, want 3", len(bindings))
	}
}

func TestParseOverrides_StopsOnError(t *testing.T) {
	entries := []string{
		"j=nav.down",
		"bad_entry",
		"k=nav.up",
	}
	_, err := ParseOverrides(entries)
	if err == nil {
		t.Error("expected error on bad entry")
	}
}

func TestHelpOverlay_Toggle(t *testing.T) {
	h := &HelpOverlay{KeyMap: DefaultKeyMap()}
	if h.Active {
		t.Error("should start inactive")
	}
	h.Toggle()
	if !h.Active {
		t.Error("should be active after toggle")
	}
	h.Toggle()
	if h.Active {
		t.Error("should be inactive after second toggle")
	}
}

func TestHelpOverlay_IsActive(t *testing.T) {
	h := &HelpOverlay{}
	if h.IsActive() {
		t.Error("should be inactive")
	}
	h.Active = true
	if !h.IsActive() {
		t.Error("should be active")
	}
}

func TestHelpOverlay_Deactivate(t *testing.T) {
	h := &HelpOverlay{Active: true}
	h.Deactivate()
	if h.Active {
		t.Error("should be inactive after Deactivate")
	}
}

func TestHelpOverlay_ModalHandleKey_Escape(t *testing.T) {
	h := &HelpOverlay{Active: true, KeyMap: DefaultKeyMap()}
	msg := tea.KeyMsg{Type: tea.KeyEscape}
	_, handled := h.ModalHandleKey(msg)
	if !handled {
		t.Error("escape should be handled")
	}
	if h.Active {
		t.Error("should be inactive after escape")
	}
}

func TestHelpOverlay_ModalHandleKey_QuestionMark(t *testing.T) {
	h := &HelpOverlay{Active: true, KeyMap: DefaultKeyMap()}
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}
	_, handled := h.ModalHandleKey(msg)
	if !handled {
		t.Error("? should be handled")
	}
	if h.Active {
		t.Error("should be inactive after ?")
	}
}

func TestHelpOverlay_ModalHandleKey_ConsumesOtherKeys(t *testing.T) {
	h := &HelpOverlay{Active: true, KeyMap: DefaultKeyMap()}
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	_, handled := h.ModalHandleKey(msg)
	if !handled {
		t.Error("other keys should be consumed")
	}
	if !h.Active {
		t.Error("should remain active for non-dismiss keys")
	}
}

func TestHelpOverlay_View_Inactive(t *testing.T) {
	h := &HelpOverlay{KeyMap: DefaultKeyMap()}
	if h.View() != "" {
		t.Error("inactive overlay should render empty")
	}
}

func TestHelpOverlay_View_NilKeyMap(t *testing.T) {
	h := &HelpOverlay{Active: true}
	if h.View() != "" {
		t.Error("nil KeyMap should render empty")
	}
}

func TestHelpOverlay_View_ContainsCategories(t *testing.T) {
	h := &HelpOverlay{Active: true, KeyMap: DefaultKeyMap(), Width: 60}
	view := h.View()
	stripped := StripAnsi(view)

	for _, cat := range []string{"Navigation", "Views", "Sessions", "Global"} {
		if !strings.Contains(stripped, cat) {
			t.Errorf("help view should contain category %q", cat)
		}
	}
}

func TestHelpOverlay_View_ContainsBindings(t *testing.T) {
	h := &HelpOverlay{Active: true, KeyMap: DefaultKeyMap(), Width: 60}
	view := h.View()
	stripped := StripAnsi(view)

	// Should contain some key names and action labels.
	for _, want := range []string{"Move up", "Move down", "Quit", "Toggle help"} {
		if !strings.Contains(stripped, want) {
			t.Errorf("help view should contain %q, got:\n%s", want, stripped)
		}
	}
}

func TestHelpOverlay_ModalView(t *testing.T) {
	h := &HelpOverlay{Active: true, KeyMap: DefaultKeyMap(), Width: 60}
	mv := h.ModalView(80, 40)
	if mv == "" {
		t.Error("ModalView should return content when active")
	}
}

func TestDefaultBindings_NoDuplicateKeyActionPairs(t *testing.T) {
	seen := make(map[string]bool)
	for _, b := range DefaultBindings() {
		key := b.Key.String() + "=" + string(b.Action)
		if seen[key] {
			t.Errorf("duplicate binding: %s", key)
		}
		seen[key] = true
	}
}

func TestParseKey_CtrlAltCombo(t *testing.T) {
	k, err := ParseKey("ctrl+alt+x")
	if err != nil {
		t.Fatal(err)
	}
	if !k.Ctrl || !k.Alt || k.Rune != 'x' {
		t.Errorf("got %+v, want ctrl+alt+x", k)
	}
}

func TestParseKey_Whitespace(t *testing.T) {
	k, err := ParseKey("  j  ")
	if err != nil {
		t.Fatal(err)
	}
	if !k.Equal(RuneKey('j')) {
		t.Errorf("got %v, want 'j'", k)
	}
}

func TestParseBinding_Whitespace(t *testing.T) {
	b, err := ParseBinding("  j  =  nav.down  ")
	if err != nil {
		t.Fatal(err)
	}
	if !b.Key.Equal(RuneKey('j')) {
		t.Errorf("key = %v, want 'j'", b.Key)
	}
	if b.Action != ActionNavDown {
		t.Errorf("action = %q, want nav.down", b.Action)
	}
}

func TestMerge_PreservesUnrelatedBindings(t *testing.T) {
	base := NewKeyMap([]Binding{
		{RuneKey('a'), ActionActions},
		{RuneKey('q'), ActionQuit},
		{RuneKey('?'), ActionToggleHelp},
	})
	overrides := NewKeyMap([]Binding{
		{RuneKey('q'), ActionRefresh}, // rebind q
	})

	merged := Merge(base, overrides)

	// a and ? should survive.
	if got := merged.Resolve(RuneKey('a')); got != ActionActions {
		t.Errorf("a = %q, want actions", got)
	}
	if got := merged.Resolve(RuneKey('?')); got != ActionToggleHelp {
		t.Errorf("? = %q, want toggle.help", got)
	}
	// q should be overridden.
	if got := merged.Resolve(RuneKey('q')); got != ActionRefresh {
		t.Errorf("q = %q, want refresh", got)
	}
}

func TestKeyFromMsg_CtrlR(t *testing.T) {
	msg := tea.KeyMsg{Type: tea.KeyCtrlR}
	k := KeyFromMsg(msg)
	if !k.Ctrl || k.Rune != 'r' || k.Type != tea.KeyRunes {
		t.Errorf("got %+v, want ctrl+r as Ctrl=true Rune='r'", k)
	}
}

func TestKeyMap_EmptyResolve(t *testing.T) {
	km := NewKeyMap(nil)
	if got := km.Resolve(RuneKey('j')); got != "" {
		t.Errorf("empty map should return empty, got %q", got)
	}
}
