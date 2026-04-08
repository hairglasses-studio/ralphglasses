---
paths:
  - "internal/tui/**"
---

BubbleTea v2 TUI patterns:
- Styles in own package (`internal/tui/styles/`) to avoid import cycles — always import styles from there
- View stack: `CurrentView` + `ViewStack` for breadcrumb navigation
- Views in `views/` sub-package, components in `components/` sub-package
- Theme hot-swap via `styles/hotswap.go` using fsnotify + `tea.Program.Send()`
- `View()` returns `tea.View` (v2 API), not `string`
- View dispatch uses `viewDispatch` registry map (preferred) with switch fallback
- Keymap system via `keymap.go` — context-sensitive bindings
- Golden tests in `testdata/` for snapshot regression — update goldens when changing view output
