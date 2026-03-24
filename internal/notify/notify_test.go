package notify

import "testing"

func TestEscapeOSA(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty string", in: "", want: ""},
		{name: "no special chars", in: "hello world", want: "hello world"},
		{name: "double quote", in: `say "hi"`, want: `say \"hi\"`},
		{name: "backslash", in: `path\to\file`, want: `path\\to\\file`},
		{name: "both", in: `he said "go to C:\"`, want: `he said \"go to C:\\\"`},
		{name: "consecutive quotes", in: `""`, want: `\"\"`},
		{name: "consecutive backslashes", in: `\\`, want: `\\\\`},
		{name: "single char quote", in: `"`, want: `\"`},
		{name: "single char backslash", in: `\`, want: `\\`},
		{name: "unicode passthrough", in: "hello 🌍", want: "hello 🌍"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := escapeOSA(tt.in)
			if got != tt.want {
				t.Errorf("escapeOSA(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
