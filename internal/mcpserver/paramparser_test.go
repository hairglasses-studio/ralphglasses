package mcpserver

import (
	"strings"
	"testing"
)

func TestParamParser_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args map[string]any
		key  string
		want string
	}{
		{name: "valid", args: map[string]any{"k": "hello"}, key: "k", want: "hello"},
		{name: "empty_string", args: map[string]any{"k": ""}, key: "k", want: ""},
		{name: "missing", args: map[string]any{}, key: "k", want: ""},
		{name: "wrong_type_int", args: map[string]any{"k": 42.0}, key: "k", want: ""},
		{name: "wrong_type_bool", args: map[string]any{"k": true}, key: "k", want: ""},
		{name: "nil_map", args: nil, key: "k", want: ""},
		{name: "nil_value", args: map[string]any{"k": nil}, key: "k", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pp := NewParamParser(tt.args)
			got := pp.String(tt.key)
			if got != tt.want {
				t.Errorf("String(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestParamParser_StringOr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		args       map[string]any
		key        string
		defaultVal string
		want       string
	}{
		{name: "present", args: map[string]any{"k": "val"}, key: "k", defaultVal: "def", want: "val"},
		{name: "absent", args: map[string]any{}, key: "k", defaultVal: "def", want: "def"},
		{name: "wrong_type", args: map[string]any{"k": 99.0}, key: "k", defaultVal: "def", want: "def"},
		{name: "nil_map", args: nil, key: "k", defaultVal: "def", want: "def"},
		{name: "empty_default", args: map[string]any{}, key: "k", defaultVal: "", want: ""},
		{name: "empty_value", args: map[string]any{"k": ""}, key: "k", defaultVal: "def", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pp := NewParamParser(tt.args)
			got := pp.StringOr(tt.key, tt.defaultVal)
			if got != tt.want {
				t.Errorf("StringOr(%q, %q) = %q, want %q", tt.key, tt.defaultVal, got, tt.want)
			}
		})
	}
}

func TestParamParser_StringOpt_Alias(t *testing.T) {
	t.Parallel()
	pp := NewParamParser(map[string]any{"k": "v"})
	if got := pp.StringOpt("k", "d"); got != "v" {
		t.Errorf("StringOpt present = %q, want %q", got, "v")
	}
	if got := pp.StringOpt("missing", "d"); got != "d" {
		t.Errorf("StringOpt absent = %q, want %q", got, "d")
	}
}

func TestParamParser_Bool(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args map[string]any
		key  string
		want bool
	}{
		{name: "true", args: map[string]any{"k": true}, key: "k", want: true},
		{name: "false", args: map[string]any{"k": false}, key: "k", want: false},
		{name: "missing", args: map[string]any{}, key: "k", want: false},
		{name: "wrong_type_string", args: map[string]any{"k": "true"}, key: "k", want: false},
		{name: "wrong_type_int", args: map[string]any{"k": float64(1)}, key: "k", want: false},
		{name: "nil_map", args: nil, key: "k", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pp := NewParamParser(tt.args)
			got := pp.Bool(tt.key)
			if got != tt.want {
				t.Errorf("Bool(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestParamParser_Int(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args map[string]any
		key  string
		want int
	}{
		{name: "valid", args: map[string]any{"k": float64(7)}, key: "k", want: 7},
		{name: "negative", args: map[string]any{"k": float64(-3)}, key: "k", want: -3},
		{name: "zero", args: map[string]any{"k": float64(0)}, key: "k", want: 0},
		{name: "fractional_truncated", args: map[string]any{"k": 3.14}, key: "k", want: 3},
		{name: "missing", args: map[string]any{}, key: "k", want: 0},
		{name: "wrong_type", args: map[string]any{"k": "nope"}, key: "k", want: 0},
		{name: "nil_map", args: nil, key: "k", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pp := NewParamParser(tt.args)
			got := pp.Int(tt.key)
			if got != tt.want {
				t.Errorf("Int(%q) = %d, want %d", tt.key, got, tt.want)
			}
		})
	}
}

func TestParamParser_IntOr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		args       map[string]any
		key        string
		defaultVal int
		want       int
	}{
		{name: "present", args: map[string]any{"k": float64(10)}, key: "k", defaultVal: 5, want: 10},
		{name: "absent", args: map[string]any{}, key: "k", defaultVal: 5, want: 5},
		{name: "wrong_type", args: map[string]any{"k": "nope"}, key: "k", defaultVal: 5, want: 5},
		{name: "nil_map", args: nil, key: "k", defaultVal: 5, want: 5},
		{name: "zero_default", args: map[string]any{}, key: "k", defaultVal: 0, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pp := NewParamParser(tt.args)
			got := pp.IntOr(tt.key, tt.defaultVal)
			if got != tt.want {
				t.Errorf("IntOr(%q, %d) = %d, want %d", tt.key, tt.defaultVal, got, tt.want)
			}
		})
	}
}

func TestParamParser_IntOpt_Alias(t *testing.T) {
	t.Parallel()
	pp := NewParamParser(map[string]any{"k": float64(42)})
	if got := pp.IntOpt("k", 0); got != 42 {
		t.Errorf("IntOpt present = %d, want 42", got)
	}
	if got := pp.IntOpt("missing", 99); got != 99 {
		t.Errorf("IntOpt absent = %d, want 99", got)
	}
}

func TestParamParser_Float(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args map[string]any
		key  string
		want float64
	}{
		{name: "valid", args: map[string]any{"k": 3.14}, key: "k", want: 3.14},
		{name: "whole", args: map[string]any{"k": float64(7)}, key: "k", want: 7.0},
		{name: "negative", args: map[string]any{"k": -2.5}, key: "k", want: -2.5},
		{name: "zero", args: map[string]any{"k": float64(0)}, key: "k", want: 0},
		{name: "missing", args: map[string]any{}, key: "k", want: 0},
		{name: "wrong_type", args: map[string]any{"k": "nope"}, key: "k", want: 0},
		{name: "nil_map", args: nil, key: "k", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pp := NewParamParser(tt.args)
			got := pp.Float(tt.key)
			if got != tt.want {
				t.Errorf("Float(%q) = %f, want %f", tt.key, got, tt.want)
			}
		})
	}
}

func TestParamParser_StringSlice(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args map[string]any
		key  string
		want []string
	}{
		{
			name: "valid",
			args: map[string]any{"tags": []any{"a", "b", "c"}},
			key:  "tags",
			want: []string{"a", "b", "c"},
		},
		{
			name: "empty_slice",
			args: map[string]any{"tags": []any{}},
			key:  "tags",
			want: []string{},
		},
		{
			name: "mixed_types_skips_non_strings",
			args: map[string]any{"tags": []any{"a", 42.0, "b", true}},
			key:  "tags",
			want: []string{"a", "b"},
		},
		{
			name: "missing_key",
			args: map[string]any{},
			key:  "tags",
			want: nil,
		},
		{
			name: "wrong_type_not_slice",
			args: map[string]any{"tags": "not-a-slice"},
			key:  "tags",
			want: nil,
		},
		{
			name: "nil_map",
			args: nil,
			key:  "tags",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pp := NewParamParser(tt.args)
			got := pp.StringSlice(tt.key)
			if tt.want == nil {
				if got != nil {
					t.Errorf("StringSlice(%q) = %v, want nil", tt.key, got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("StringSlice(%q) len = %d, want %d", tt.key, len(got), len(tt.want))
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("StringSlice(%q)[%d] = %q, want %q", tt.key, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParamParser_Has(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args map[string]any
		key  string
		want bool
	}{
		{name: "present", args: map[string]any{"k": "v"}, key: "k", want: true},
		{name: "present_nil_value", args: map[string]any{"k": nil}, key: "k", want: true},
		{name: "absent", args: map[string]any{"other": "v"}, key: "k", want: false},
		{name: "empty_map", args: map[string]any{}, key: "k", want: false},
		{name: "nil_map", args: nil, key: "k", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pp := NewParamParser(tt.args)
			got := pp.Has(tt.key)
			if got != tt.want {
				t.Errorf("Has(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestParamParser_Required(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    map[string]any
		keys    []string
		wantErr string
	}{
		{
			name: "all_present",
			args: map[string]any{"a": "1", "b": "2"},
			keys: []string{"a", "b"},
		},
		{
			name:    "one_missing",
			args:    map[string]any{"a": "1"},
			keys:    []string{"a", "b"},
			wantErr: "b",
		},
		{
			name:    "multiple_missing",
			args:    map[string]any{},
			keys:    []string{"x", "y"},
			wantErr: "x",
		},
		{
			name: "no_keys",
			args: map[string]any{},
			keys: []string{},
		},
		{
			name:    "nil_map_all_missing",
			args:    nil,
			keys:    []string{"a"},
			wantErr: "a",
		},
		{
			name: "nil_value_counts_as_present",
			args: map[string]any{"a": nil},
			keys: []string{"a"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pp := NewParamParser(tt.args)
			err := pp.Required(tt.keys...)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestParamParser_Required_MultipleMissing(t *testing.T) {
	t.Parallel()
	pp := NewParamParser(map[string]any{})
	err := pp.Required("a", "b", "c")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	for _, k := range []string{"a", "b", "c"} {
		if !strings.Contains(msg, k) {
			t.Errorf("error %q does not mention missing key %q", msg, k)
		}
	}
}

// --- Error-returning variant tests ---

func TestParamParser_StringErr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    map[string]any
		key     string
		want    string
		wantErr string
	}{
		{name: "valid", args: map[string]any{"k": "hello"}, key: "k", want: "hello"},
		{name: "missing", args: map[string]any{}, key: "k", wantErr: "k required"},
		{name: "wrong_type", args: map[string]any{"k": 42.0}, key: "k", wantErr: "k must be a string"},
		{name: "nil_map", args: nil, key: "k", wantErr: "k required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pp := NewParamParser(tt.args)
			got, errResult := pp.StringErr(tt.key)
			if tt.wantErr != "" {
				if errResult == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !errResult.IsError {
					t.Fatal("expected IsError=true")
				}
				text := getResultText(errResult)
				if !strings.Contains(text, tt.wantErr) {
					t.Errorf("error text %q does not contain %q", text, tt.wantErr)
				}
			} else {
				if errResult != nil {
					t.Fatalf("unexpected error: %s", getResultText(errResult))
				}
				if got != tt.want {
					t.Errorf("got %q, want %q", got, tt.want)
				}
			}
		})
	}
}

func TestParamParser_IntErr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    map[string]any
		key     string
		want    int
		wantErr string
	}{
		{name: "valid", args: map[string]any{"k": float64(7)}, key: "k", want: 7},
		{name: "negative", args: map[string]any{"k": float64(-3)}, key: "k", want: -3},
		{name: "missing", args: map[string]any{}, key: "k", wantErr: "k required"},
		{name: "wrong_type", args: map[string]any{"k": "nope"}, key: "k", wantErr: "k must be a number"},
		{name: "not_integer", args: map[string]any{"k": 3.14}, key: "k", wantErr: "k must be an integer"},
		{name: "nil_map", args: nil, key: "k", wantErr: "k required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pp := NewParamParser(tt.args)
			got, errResult := pp.IntErr(tt.key)
			if tt.wantErr != "" {
				if errResult == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !errResult.IsError {
					t.Fatal("expected IsError=true")
				}
				text := getResultText(errResult)
				if !strings.Contains(text, tt.wantErr) {
					t.Errorf("error text %q does not contain %q", text, tt.wantErr)
				}
			} else {
				if errResult != nil {
					t.Fatalf("unexpected error: %s", getResultText(errResult))
				}
				if got != tt.want {
					t.Errorf("got %d, want %d", got, tt.want)
				}
			}
		})
	}
}

func TestParamParser_FloatErr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    map[string]any
		key     string
		want    float64
		wantErr string
	}{
		{name: "valid", args: map[string]any{"k": 3.14}, key: "k", want: 3.14},
		{name: "missing", args: map[string]any{}, key: "k", wantErr: "k required"},
		{name: "wrong_type", args: map[string]any{"k": "nope"}, key: "k", wantErr: "k must be a number"},
		{name: "nil_map", args: nil, key: "k", wantErr: "k required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pp := NewParamParser(tt.args)
			got, errResult := pp.FloatErr(tt.key)
			if tt.wantErr != "" {
				if errResult == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				text := getResultText(errResult)
				if !strings.Contains(text, tt.wantErr) {
					t.Errorf("error text %q does not contain %q", text, tt.wantErr)
				}
			} else {
				if errResult != nil {
					t.Fatalf("unexpected error: %s", getResultText(errResult))
				}
				if got != tt.want {
					t.Errorf("got %f, want %f", got, tt.want)
				}
			}
		})
	}
}

func TestParamParser_BoolErr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    map[string]any
		key     string
		want    bool
		wantErr string
	}{
		{name: "true", args: map[string]any{"k": true}, key: "k", want: true},
		{name: "false", args: map[string]any{"k": false}, key: "k", want: false},
		{name: "missing", args: map[string]any{}, key: "k", wantErr: "k required"},
		{name: "wrong_type", args: map[string]any{"k": "true"}, key: "k", wantErr: "k must be a boolean"},
		{name: "nil_map", args: nil, key: "k", wantErr: "k required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pp := NewParamParser(tt.args)
			got, errResult := pp.BoolErr(tt.key)
			if tt.wantErr != "" {
				if errResult == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				text := getResultText(errResult)
				if !strings.Contains(text, tt.wantErr) {
					t.Errorf("error text %q does not contain %q", text, tt.wantErr)
				}
			} else {
				if errResult != nil {
					t.Fatalf("unexpected error: %s", getResultText(errResult))
				}
				if got != tt.want {
					t.Errorf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}
