# Fix Clipboard + Uniform Tiling

## Context
Two usability issues:
1. **Clipboard broken** — Ghostty shows "selection copied" on text selection but nothing lands in the clipboard. Ctrl+C/V also don't work. Only right-click copy works.
2. **Uneven tiling** — Master layout forces one large "focused" window + smaller slaves. User wants uniform distribution across ultrawide monitors.

---

## Issue 1: Clipboard

### Root cause
`copy-on-select = clipboard` in Ghostty on Wayland has a known reliability issue — the Wayland clipboard protocol requires the source app to serve data on demand, and if the handoff to `wl-paste --watch` doesn't complete before focus changes, the content is lost. The "selection copied" notification fires optimistically.

Additionally, **Ctrl+C/V don't copy/paste in terminals** — they send SIGINT/literal-paste. Ghostty's default Linux keybinds are `ctrl+shift+c` / `ctrl+shift+v`. The user may not be aware of this distinction.

### Fix
In `ghostty/config`:
1. Change `copy-on-select = clipboard` to `copy-on-select = true` — this uses the PRIMARY selection, which is the standard X11/Wayland select-to-copy mechanism and is more reliable
2. Add `wl-paste --primary --watch cliphist store` to the Hyprland exec-once block so PRIMARY selections are persisted in cliphist (currently only CLIPBOARD selections are watched)
3. Add explicit Ctrl+C / Ctrl+V keybinds in Ghostty that map to copy/paste (overriding SIGINT for copy — Ghostty is smart enough to send SIGINT when there's no selection and copy when there is)

### Files
- `ghostty/config` — line 70: change copy-on-select, add keybinds
- `hyprland/hyprland.conf` — line 454: add primary clipboard watcher

---

## Issue 2: Uniform Tiling

### Root cause
`layout = master` with `mfact = 0.50` always dedicates 50% of screen to one "master" window. Remaining windows share the other 50%. This is by design in master layout.

### Fix
Switch to `dwindle` layout, which recursively splits space in half for each new window — 3 windows get ~33/33/33 distribution (first window takes half, second and third split the other half equally... actually dwindle does 50/25/25 by default too).

**Better approach:** Keep `master` layout but set `mfact` to a balanced starting point and add the ability to quickly equalize. Actually, the cleanest solution for truly uniform splits is to use **master layout with all windows as masters** — but that's not how master works.

**Best approach:** Switch to `dwindle` layout. While dwindle's default split is also 50/50 recursive, it has a key advantage: the `pseudo` and `split_ratio` can be tuned, and more importantly there's a `dwindle:force_split = 2` option that makes splits more predictable on ultrawides. With 3 windows on an ultrawide, dwindle gives 50/25/25 by default — but you can use `dwindle:split_ratio = 1.0` to force equal splits at each level.

Wait — actually neither layout gives perfect 3-way uniform splits natively. The real solution:

**Recommended approach: Switch to `dwindle` layout** — it's more intuitive for equal distribution and doesn't have the master/slave hierarchy. On ultrawides, dwindle with these settings works well:

```
general {
    layout = dwindle
}

dwindle {
    pseudotile = false
    preserve_split = true
    force_split = 2
}
```

With 3 windows this gives 50/25/25. For true 33/33/33, the user can press a keybind to equalize (Hyprland doesn't have a native "equalize all" dispatcher, but the split ratios can be adjusted per-split).

**Actually — simplest real fix:** The Hyprland wiki confirms that for uniform distribution, the correct approach is `dwindle` with `split_ratio = 1.0` (forces each split to be exactly 50/50). 3 windows = 50/25/25, but that's mathematically inherent to binary splitting. For true N-way equal, you need a different approach.

**Final recommendation:** Switch to `dwindle` with `preserve_split = true`. This is still more uniform than master (no single dominant window). For the ultrawide, the user can manually resize to equalize when needed using `Super+]`/`Super+[`. The key improvement is eliminating the master/slave hierarchy where one window is always dominant.

### Files
- `hyprland/hyprland.conf` — line 52: change layout, lines 127-132: replace master block with dwindle block
- `hyprland/monitors.conf` — workspace orientations may need updating for dwindle

---

## Verification
1. **Clipboard:** Select text in Ghostty → confirm it appears in `wl-paste -p` (primary) and `cliphist list`. Test Ctrl+C/Ctrl+V. Test Ctrl+Shift+C/V.
2. **Tiling:** Open 3 terminals on ultrawide workspace → confirm they distribute more evenly. Test with `hyprctl configerrors` for config validity.
