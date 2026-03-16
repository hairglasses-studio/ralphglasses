package tui

// FilterState tracks the / filter input.
type FilterState struct {
	Active bool
	Text   string
}

// Type appends a character to the filter.
func (f *FilterState) Type(ch rune) {
	f.Text += string(ch)
}

// Backspace removes the last character.
func (f *FilterState) Backspace() {
	if len(f.Text) > 0 {
		f.Text = f.Text[:len(f.Text)-1]
	}
}

// Clear resets the filter.
func (f *FilterState) Clear() {
	f.Active = false
	f.Text = ""
}
