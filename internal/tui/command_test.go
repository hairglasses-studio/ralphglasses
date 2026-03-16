package tui

import "testing"

func TestParseCommand(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantArgs []string
	}{
		{
			input:    "start mesmer",
			wantName: "start",
			wantArgs: []string{"mesmer"},
		},
		{
			input:    "stop my-repo",
			wantName: "stop",
			wantArgs: []string{"my-repo"},
		},
		{
			input:    "quit",
			wantName: "quit",
			wantArgs: []string{},
		},
		{
			input:    "  scan  ",
			wantName: "scan",
			wantArgs: []string{},
		},
		{
			input:    "",
			wantName: "",
			wantArgs: nil,
		},
		{
			input:    "   ",
			wantName: "",
			wantArgs: nil,
		},
		{
			input:    "start repo-a repo-b",
			wantName: "start",
			wantArgs: []string{"repo-a", "repo-b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cmd := ParseCommand(tt.input)
			if cmd.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", cmd.Name, tt.wantName)
			}
			if tt.wantArgs == nil {
				if cmd.Args != nil {
					t.Errorf("Args = %v, want nil", cmd.Args)
				}
			} else {
				if len(cmd.Args) != len(tt.wantArgs) {
					t.Errorf("Args len = %d, want %d", len(cmd.Args), len(tt.wantArgs))
				} else {
					for i, want := range tt.wantArgs {
						if cmd.Args[i] != want {
							t.Errorf("Args[%d] = %q, want %q", i, cmd.Args[i], want)
						}
					}
				}
			}
		})
	}
}
