package awesome

import "testing"

func TestDiff_NilPrev(t *testing.T) {
	t.Parallel()
	current := &Index{
		Entries: []AwesomeEntry{
			{Name: "a", URL: "https://github.com/org/a"},
			{Name: "b", URL: "https://github.com/org/b"},
		},
	}

	d := Diff(nil, current)
	if len(d.New) != 2 {
		t.Errorf("expected 2 new, got %d", len(d.New))
	}
	if len(d.Removed) != 0 {
		t.Errorf("expected 0 removed, got %d", len(d.Removed))
	}
}

func TestDiff_NoChanges(t *testing.T) {
	t.Parallel()
	entries := []AwesomeEntry{
		{Name: "a", URL: "https://github.com/org/a"},
		{Name: "b", URL: "https://github.com/org/b"},
	}
	prev := &Index{Entries: entries}
	current := &Index{Entries: entries}

	d := Diff(prev, current)
	if len(d.New) != 0 {
		t.Errorf("expected 0 new, got %d", len(d.New))
	}
	if len(d.Removed) != 0 {
		t.Errorf("expected 0 removed, got %d", len(d.Removed))
	}
}

func TestDiff_NewAndRemoved(t *testing.T) {
	t.Parallel()
	prev := &Index{
		Entries: []AwesomeEntry{
			{Name: "a", URL: "https://github.com/org/a"},
			{Name: "b", URL: "https://github.com/org/b"},
			{Name: "c", URL: "https://github.com/org/c"},
		},
	}
	current := &Index{
		Entries: []AwesomeEntry{
			{Name: "a", URL: "https://github.com/org/a"},
			{Name: "d", URL: "https://github.com/org/d"},
		},
	}

	d := Diff(prev, current)
	if len(d.New) != 1 {
		t.Errorf("expected 1 new, got %d", len(d.New))
	}
	if d.New[0].Name != "d" {
		t.Errorf("new entry name = %q, want d", d.New[0].Name)
	}
	if len(d.Removed) != 2 {
		t.Errorf("expected 2 removed, got %d", len(d.Removed))
	}
}
