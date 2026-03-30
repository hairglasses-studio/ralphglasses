package components

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// Action identifies a TUI action that can be triggered by a key binding.
type Action string

const (
	// Navigation
	ActionNavUp       Action = "nav.up"
	ActionNavDown     Action = "nav.down"
	ActionNavLeft     Action = "nav.left"
	ActionNavRight    Action = "nav.right"
	ActionNavPageUp   Action = "nav.page_up"
	ActionNavPageDown Action = "nav.page_down"
	ActionNavHome     Action = "nav.home"
	ActionNavEnd      Action = "nav.end"
	ActionNavBack     Action = "nav.back"
	ActionNavEnter    Action = "nav.enter"

	// Views
	ActionViewSessions Action = "view.sessions"
	ActionViewFleet    Action = "view.fleet"
	ActionViewLogs     Action = "view.logs"
	ActionViewCycles   Action = "view.cycles"
	ActionViewTeams    Action = "view.teams"
	ActionViewNextTab  Action = "view.next_tab"
	ActionViewPrevTab  Action = "view.prev_tab"

	// Sessions
	ActionSessionStart Action = "session.start"
	ActionSessionStop  Action = "session.stop"
	ActionSessionPause Action = "session.pause"
	ActionSessionRetry Action = "session.retry"

	// Toggles
	ActionToggleDetails Action = "toggle.details"
	ActionToggleHelp    Action = "toggle.help"

	// Filter / Sort
	ActionFilter   Action = "filter"
	ActionSort     Action = "sort"
	ActionSearch   Action = "search"
	ActionClearAll Action = "clear_all"

	// Global
	ActionQuit    Action = "quit"
	ActionRefresh Action = "refresh"
	ActionActions Action = "actions"
)

// Category groups related actions for the help overlay.
type Category string

const (
	CategoryNavigation Category = "Navigation"
	CategoryViews      Category = "Views"
	CategorySessions   Category = "Sessions"
	CategoryToggles    Category = "Toggles"
	CategoryFilter     Category = "Filter / Sort"
	CategoryGlobal     Category = "Global"
)

// actionCategory maps each action to its display category.
var actionCategory = map[Action]Category{
	ActionNavUp:       CategoryNavigation,
	ActionNavDown:     CategoryNavigation,
	ActionNavLeft:     CategoryNavigation,
	ActionNavRight:    CategoryNavigation,
	ActionNavPageUp:   CategoryNavigation,
	ActionNavPageDown: CategoryNavigation,
	ActionNavHome:     CategoryNavigation,
	ActionNavEnd:      CategoryNavigation,
	ActionNavBack:     CategoryNavigation,
	ActionNavEnter:    CategoryNavigation,

	ActionViewSessions: CategoryViews,
	ActionViewFleet:    CategoryViews,
	ActionViewLogs:     CategoryViews,
	ActionViewCycles:   CategoryViews,
	ActionViewTeams:    CategoryViews,
	ActionViewNextTab:  CategoryViews,
	ActionViewPrevTab:  CategoryViews,

	ActionSessionStart: CategorySessions,
	ActionSessionStop:  CategorySessions,
	ActionSessionPause: CategorySessions,
	ActionSessionRetry: CategorySessions,

	ActionToggleDetails: CategoryToggles,
	ActionToggleHelp:    CategoryToggles,

	ActionFilter:   CategoryFilter,
	ActionSort:     CategoryFilter,
	ActionSearch:   CategoryFilter,
	ActionClearAll: CategoryFilter,

	ActionQuit:    CategoryGlobal,
	ActionRefresh: CategoryGlobal,
	ActionActions: CategoryGlobal,
}

// actionLabel provides human-readable descriptions for each action.
var actionLabel = map[Action]string{
	ActionNavUp:       "Move up",
	ActionNavDown:     "Move down",
	ActionNavLeft:     "Move left",
	ActionNavRight:    "Move right",
	ActionNavPageUp:   "Page up",
	ActionNavPageDown: "Page down",
	ActionNavHome:     "Go to top",
	ActionNavEnd:      "Go to bottom",
	ActionNavBack:     "Go back",
	ActionNavEnter:    "Select / open",

	ActionViewSessions: "Sessions view",
	ActionViewFleet:    "Fleet view",
	ActionViewLogs:     "Logs view",
	ActionViewCycles:   "Cycles view",
	ActionViewTeams:    "Teams view",
	ActionViewNextTab:  "Next tab",
	ActionViewPrevTab:  "Previous tab",

	ActionSessionStart: "Start session",
	ActionSessionStop:  "Stop session",
	ActionSessionPause: "Pause / resume",
	ActionSessionRetry: "Retry session",

	ActionToggleDetails: "Toggle details",
	ActionToggleHelp:    "Toggle help",

	ActionFilter:   "Filter",
	ActionSort:     "Sort",
	ActionSearch:   "Search",
	ActionClearAll: "Clear filter",

	ActionQuit:    "Quit",
	ActionRefresh: "Refresh",
	ActionActions: "Actions menu",
}

// Binding maps a single key combination to an action.
type Binding struct {
	Key    Key
	Action Action
}

// Key represents a key or key combination that can trigger a binding.
// For special keys (arrows, function keys, etc.), Code is the tea key constant (non-zero).
// For character keys, Code is 0 and Rune holds the character.
// Ctrl, Alt, and Shift are modifier flags.
type Key struct {
	// Code is the tea key code for special keys (tea.KeyEnter, tea.KeyTab, etc.).
	// Zero means this is a character key — see Rune.
	Code  rune
	Rune  rune
	Ctrl  bool
	Alt   bool
	Shift bool // used for shift+tab
}

// String returns a human-readable representation of the key (e.g. "ctrl+c", "?", "pgdown").
func (k Key) String() string {
	var parts []string
	if k.Ctrl {
		parts = append(parts, "ctrl")
	}
	if k.Alt {
		parts = append(parts, "alt")
	}
	if k.Shift {
		parts = append(parts, "shift")
	}

	var name string
	switch {
	case k.Code == 0:
		// Character key
		if k.Rune == ' ' {
			name = "space"
		} else {
			name = string(k.Rune)
		}
	default:
		name = keyCodeName(k.Code)
	}
	parts = append(parts, name)
	return strings.Join(parts, "+")
}

// Equal checks if two keys are the same combination.
func (k Key) Equal(other Key) bool {
	return k.Code == other.Code && k.Rune == other.Rune && k.Ctrl == other.Ctrl && k.Alt == other.Alt && k.Shift == other.Shift
}

// RuneKey creates a Key for a single character.
func RuneKey(r rune) Key {
	return Key{Code: 0, Rune: r}
}

// CtrlKey creates a Key for a ctrl+letter combination.
func CtrlKey(r rune) Key {
	return Key{Code: 0, Rune: r, Ctrl: true}
}

// SpecialKey creates a Key for a special (non-rune) key code.
func SpecialKey(code rune) Key {
	return Key{Code: code}
}

// ShiftTabKey returns the Key representing shift+tab.
func ShiftTabKey() Key {
	return Key{Code: tea.KeyTab, Shift: true}
}

// KeyFromMsg converts a bubbletea KeyPressMsg into our Key type.
func KeyFromMsg(msg tea.KeyPressMsg) Key {
	k := Key{
		Code: msg.Code,
		Alt:  msg.Mod.Contains(tea.ModAlt),
		Ctrl: msg.Mod.Contains(tea.ModCtrl),
	}

	// Shift+Tab: tab with shift modifier.
	if msg.Code == tea.KeyTab && msg.Mod.Contains(tea.ModShift) {
		k.Shift = true
		return k
	}

	// For printable characters, Code is the rune itself; treat as rune key.
	if msg.Text != "" {
		runes := []rune(msg.Text)
		if len(runes) > 0 {
			k.Rune = runes[0]
			k.Code = 0 // treat as character key
		}
		return k
	}

	// Ctrl+letter: when Ctrl is held and the code is a printable letter,
	// treat the code as the rune and clear Code so key matching works correctly.
	if k.Ctrl && msg.Text == "" && msg.Code >= 'a' && msg.Code <= 'z' {
		k.Rune = msg.Code
		k.Code = 0
		return k
	}

	// Named special key — keep Code as-is, clear Ctrl for named specials that
	// share integer values with ctrl+letter sequences.
	if isNamedSpecialKey(msg.Code) {
		k.Ctrl = false
		return k
	}

	return k
}

// isNamedSpecialKey returns true for key codes that have well-known names
// and should not be treated as ctrl+letter combos.
func isNamedSpecialKey(code rune) bool {
	switch code {
	case tea.KeyEnter, tea.KeyTab, tea.KeyEscape, tea.KeyBackspace:
		return true
	}
	return false
}

// KeyMap holds the mapping from keys to actions and supports lookup in both directions.
type KeyMap struct {
	bindings []Binding
	byKey    map[Key]Action
	byAction map[Action][]Key
}

// NewKeyMap creates a KeyMap from a list of bindings.
// Later bindings override earlier ones for the same key.
func NewKeyMap(bindings []Binding) *KeyMap {
	km := &KeyMap{
		bindings: make([]Binding, len(bindings)),
		byKey:    make(map[Key]Action, len(bindings)),
		byAction: make(map[Action][]Key),
	}
	copy(km.bindings, bindings)
	for _, b := range bindings {
		km.byKey[b.Key] = b.Action
		km.byAction[b.Action] = append(km.byAction[b.Action], b.Key)
	}
	return km
}

// Resolve returns the action bound to a key, or "" if unbound.
func (km *KeyMap) Resolve(k Key) Action {
	return km.byKey[k]
}

// ResolveMsg converts a bubbletea KeyPressMsg and resolves the bound action.
func (km *KeyMap) ResolveMsg(msg tea.KeyPressMsg) Action {
	return km.Resolve(KeyFromMsg(msg))
}

// KeysFor returns all keys bound to an action.
func (km *KeyMap) KeysFor(action Action) []Key {
	return km.byAction[action]
}

// Bindings returns a copy of all bindings.
func (km *KeyMap) Bindings() []Binding {
	out := make([]Binding, len(km.bindings))
	copy(out, km.bindings)
	return out
}

// Conflicts returns pairs of bindings where the same key maps to different actions.
func (km *KeyMap) Conflicts() []Conflict {
	// Build a map of key -> all actions that claim it.
	keyActions := make(map[Key][]Action)
	for _, b := range km.bindings {
		found := false
		for _, a := range keyActions[b.Key] {
			if a == b.Action {
				found = true
				break
			}
		}
		if !found {
			keyActions[b.Key] = append(keyActions[b.Key], b.Action)
		}
	}

	var conflicts []Conflict
	for k, actions := range keyActions {
		if len(actions) > 1 {
			conflicts = append(conflicts, Conflict{Key: k, Actions: actions})
		}
	}
	// Sort for deterministic output.
	sort.Slice(conflicts, func(i, j int) bool {
		return conflicts[i].Key.String() < conflicts[j].Key.String()
	})
	return conflicts
}

// Conflict represents a single key bound to multiple different actions.
type Conflict struct {
	Key     Key
	Actions []Action
}

// String returns a readable description of the conflict.
func (c Conflict) String() string {
	actions := make([]string, len(c.Actions))
	for i, a := range c.Actions {
		actions[i] = string(a)
	}
	return fmt.Sprintf("key %q -> [%s]", c.Key.String(), strings.Join(actions, ", "))
}

// Merge creates a new KeyMap by applying overrides on top of a base map.
// Override bindings replace base bindings for the same key.
func Merge(base, overrides *KeyMap) *KeyMap {
	merged := make([]Binding, 0, len(base.bindings)+len(overrides.bindings))
	// Collect base bindings whose keys are not overridden.
	overrideKeys := make(map[Key]bool, len(overrides.bindings))
	for _, b := range overrides.bindings {
		overrideKeys[b.Key] = true
	}
	for _, b := range base.bindings {
		if !overrideKeys[b.Key] {
			merged = append(merged, b)
		}
	}
	// Append all overrides.
	merged = append(merged, overrides.bindings...)
	return NewKeyMap(merged)
}

// DefaultBindings returns the default keybinding set for the TUI.
func DefaultBindings() []Binding {
	return []Binding{
		// Navigation
		{RuneKey('k'), ActionNavUp},
		{SpecialKey(tea.KeyUp), ActionNavUp},
		{RuneKey('j'), ActionNavDown},
		{SpecialKey(tea.KeyDown), ActionNavDown},
		{RuneKey('h'), ActionNavLeft},
		{SpecialKey(tea.KeyLeft), ActionNavLeft},
		{RuneKey('l'), ActionNavRight},
		{SpecialKey(tea.KeyRight), ActionNavRight},
		{SpecialKey(tea.KeyPgUp), ActionNavPageUp},
		{SpecialKey(tea.KeyPgDown), ActionNavPageDown},
		{RuneKey('g'), ActionNavHome},
		{SpecialKey(tea.KeyHome), ActionNavHome},
		{RuneKey('G'), ActionNavEnd},
		{SpecialKey(tea.KeyEnd), ActionNavEnd},
		{SpecialKey(tea.KeyEscape), ActionNavBack},
		{SpecialKey(tea.KeyEnter), ActionNavEnter},

		// Views
		{RuneKey('1'), ActionViewSessions},
		{RuneKey('2'), ActionViewFleet},
		{RuneKey('3'), ActionViewLogs},
		{RuneKey('4'), ActionViewCycles},
		{RuneKey('5'), ActionViewTeams},
		{SpecialKey(tea.KeyTab), ActionViewNextTab},
		{ShiftTabKey(), ActionViewPrevTab},

		// Sessions
		{RuneKey('s'), ActionSessionStart},
		{RuneKey('x'), ActionSessionStop},
		{RuneKey('p'), ActionSessionPause},
		{RuneKey('R'), ActionSessionRetry},

		// Toggles
		{RuneKey('d'), ActionToggleDetails},
		{RuneKey('?'), ActionToggleHelp},

		// Filter / Sort
		{RuneKey('/'), ActionFilter},
		{RuneKey('o'), ActionSort},
		{CtrlKey('f'), ActionSearch},
		{SpecialKey(tea.KeyEscape), ActionClearAll},

		// Global
		{RuneKey('q'), ActionQuit},
		{CtrlKey('c'), ActionQuit},
		{CtrlKey('r'), ActionRefresh},
		{RuneKey('a'), ActionActions},
	}
}

// DefaultKeyMap returns a KeyMap with the default bindings.
func DefaultKeyMap() *KeyMap {
	return NewKeyMap(DefaultBindings())
}

// ParseBinding parses a single "key=action" string into a Binding.
// Supports formats: "ctrl+c", "shift+tab", "?", "space", "enter", "esc",
// "pgup", "pgdown", "home", "end", "tab", "up", "down", "left", "right",
// single characters, and function keys "f1"-"f12".
func ParseBinding(s string) (Binding, error) {
	parts := strings.SplitN(s, "=", 2)
	if len(parts) != 2 {
		return Binding{}, fmt.Errorf("invalid binding format %q: expected key=action", s)
	}
	keyStr := strings.TrimSpace(parts[0])
	actionStr := strings.TrimSpace(parts[1])

	if actionStr == "" {
		return Binding{}, fmt.Errorf("empty action in binding %q", s)
	}

	k, err := ParseKey(keyStr)
	if err != nil {
		return Binding{}, err
	}
	return Binding{Key: k, Action: Action(actionStr)}, nil
}

// ParseKey parses a key string like "ctrl+c", "?", "enter" into a Key.
func ParseKey(s string) (Key, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Key{}, fmt.Errorf("empty key string")
	}

	var k Key
	segments := strings.Split(s, "+")
	last := segments[len(segments)-1]
	mods := segments[:len(segments)-1]

	for _, m := range mods {
		switch strings.ToLower(m) {
		case "ctrl":
			k.Ctrl = true
		case "alt":
			k.Alt = true
		case "shift":
			k.Shift = true
		default:
			return Key{}, fmt.Errorf("unknown modifier %q in key %q", m, s)
		}
	}

	// Special key names.
	if code, ok := parseKeyName(strings.ToLower(last)); ok {
		k.Code = code
		return k, nil
	}

	// Single character.
	runes := []rune(last)
	if len(runes) == 1 {
		k.Code = 0
		k.Rune = runes[0]
		return k, nil
	}

	return Key{}, fmt.Errorf("unknown key %q", last)
}

// ParseOverrides parses a list of "key=action" strings into bindings.
// Stops and returns an error on the first invalid entry.
func ParseOverrides(entries []string) ([]Binding, error) {
	bindings := make([]Binding, 0, len(entries))
	for _, e := range entries {
		b, err := ParseBinding(e)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, b)
	}
	return bindings, nil
}

// categoryOrder controls the display order in the help overlay.
var categoryOrder = []Category{
	CategoryNavigation,
	CategoryViews,
	CategorySessions,
	CategoryToggles,
	CategoryFilter,
	CategoryGlobal,
}

// HelpOverlay renders a styled help panel showing all keybindings grouped by category.
type HelpOverlay struct {
	KeyMap *KeyMap
	Active bool
	Width  int
}

// Ensure HelpOverlay satisfies Modal at compile time.
var _ Modal = (*HelpOverlay)(nil)

// IsActive implements Modal.
func (h *HelpOverlay) IsActive() bool { return h.Active }

// Deactivate implements Modal.
func (h *HelpOverlay) Deactivate() { h.Active = false }

// ModalHandleKey implements Modal. Escape or ? dismisses the overlay.
func (h *HelpOverlay) ModalHandleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	k := KeyFromMsg(msg)
	// Dismiss on escape or the help toggle key.
	if k.Code == tea.KeyEscape || (k.Code == 0 && k.Rune == '?') {
		h.Active = false
		return nil, true
	}
	// Consume all keys while the overlay is active.
	return nil, true
}

// ModalView implements Modal.
func (h *HelpOverlay) ModalView(width, height int) string {
	return h.View()
}

// Toggle flips the overlay state.
func (h *HelpOverlay) Toggle() {
	h.Active = !h.Active
}

// View renders the help overlay content.
func (h *HelpOverlay) View() string {
	if !h.Active || h.KeyMap == nil {
		return ""
	}

	width := h.Width
	if width <= 0 {
		width = 60
	}
	innerWidth := width - 4

	var b strings.Builder
	b.WriteString(styles.TitleStyle.Render(" Keybindings "))
	b.WriteString("\n\n")

	// Group bindings by category.
	groups := h.groupedBindings()

	for _, cat := range categoryOrder {
		items, ok := groups[cat]
		if !ok || len(items) == 0 {
			continue
		}
		b.WriteString(styles.HeaderStyle.Render(string(cat)))
		b.WriteString("\n")
		for _, item := range items {
			keyStr := styles.CommandStyle.Render(fmt.Sprintf("%-14s", item.keyDisplay))
			label := styles.InfoStyle.Render(item.label)
			b.WriteString(fmt.Sprintf("  %s %s\n", keyStr, label))
		}
		b.WriteString("\n")
	}

	b.WriteString(styles.HelpStyle.Render("  Press ? or Esc to close"))

	return styles.ModalBoxStyle.Width(innerWidth).Render(b.String())
}

// helpItem pairs a display string with a label for a single row.
type helpItem struct {
	keyDisplay string
	label      string
}

// groupedBindings groups bindings by category, merging multiple keys for the same action.
func (h *HelpOverlay) groupedBindings() map[Category][]helpItem {
	// Collect unique actions in binding order.
	seen := make(map[Action]bool)
	var actions []Action
	for _, b := range h.KeyMap.bindings {
		if !seen[b.Action] {
			seen[b.Action] = true
			actions = append(actions, b.Action)
		}
	}

	groups := make(map[Category][]helpItem)
	for _, action := range actions {
		cat := actionCategory[action]
		if cat == "" {
			cat = CategoryGlobal
		}
		keys := h.KeyMap.KeysFor(action)
		keyStrs := make([]string, len(keys))
		for i, k := range keys {
			keyStrs[i] = k.String()
		}
		label := actionLabel[action]
		if label == "" {
			label = string(action)
		}
		groups[cat] = append(groups[cat], helpItem{
			keyDisplay: strings.Join(keyStrs, ", "),
			label:      label,
		})
	}
	return groups
}

// keyCodeName returns a display name for a bubbletea v2 special key code.
func keyCodeName(code rune) string {
	switch code {
	case tea.KeyEnter:
		return "enter"
	case tea.KeyTab:
		return "tab"
	case tea.KeyEscape:
		return "esc"
	case tea.KeyUp:
		return "up"
	case tea.KeyDown:
		return "down"
	case tea.KeyLeft:
		return "left"
	case tea.KeyRight:
		return "right"
	case tea.KeyPgUp:
		return "pgup"
	case tea.KeyPgDown:
		return "pgdown"
	case tea.KeyHome:
		return "home"
	case tea.KeyEnd:
		return "end"
	case tea.KeyBackspace:
		return "backspace"
	case tea.KeyDelete:
		return "delete"
	case tea.KeySpace:
		return "space"
	case tea.KeyF1:
		return "f1"
	case tea.KeyF2:
		return "f2"
	case tea.KeyF3:
		return "f3"
	case tea.KeyF4:
		return "f4"
	case tea.KeyF5:
		return "f5"
	case tea.KeyF6:
		return "f6"
	case tea.KeyF7:
		return "f7"
	case tea.KeyF8:
		return "f8"
	case tea.KeyF9:
		return "f9"
	case tea.KeyF10:
		return "f10"
	case tea.KeyF11:
		return "f11"
	case tea.KeyF12:
		return "f12"
	default:
		if code > 0 {
			return string(code)
		}
		return "unknown"
	}
}

// parseKeyName maps a lowercase key name to a bubbletea v2 key code rune.
func parseKeyName(name string) (rune, bool) {
	switch name {
	case "enter":
		return tea.KeyEnter, true
	case "tab":
		return tea.KeyTab, true
	case "esc", "escape":
		return tea.KeyEscape, true
	case "up":
		return tea.KeyUp, true
	case "down":
		return tea.KeyDown, true
	case "left":
		return tea.KeyLeft, true
	case "right":
		return tea.KeyRight, true
	case "pgup", "pageup":
		return tea.KeyPgUp, true
	case "pgdown", "pagedown":
		return tea.KeyPgDown, true
	case "home":
		return tea.KeyHome, true
	case "end":
		return tea.KeyEnd, true
	case "backspace":
		return tea.KeyBackspace, true
	case "delete":
		return tea.KeyDelete, true
	case "space":
		return tea.KeySpace, true
	case "f1":
		return tea.KeyF1, true
	case "f2":
		return tea.KeyF2, true
	case "f3":
		return tea.KeyF3, true
	case "f4":
		return tea.KeyF4, true
	case "f5":
		return tea.KeyF5, true
	case "f6":
		return tea.KeyF6, true
	case "f7":
		return tea.KeyF7, true
	case "f8":
		return tea.KeyF8, true
	case "f9":
		return tea.KeyF9, true
	case "f10":
		return tea.KeyF10, true
	case "f11":
		return tea.KeyF11, true
	case "f12":
		return tea.KeyF12, true
	default:
		return 0, false
	}
}
