package mcpserver

import (
	"strings"
	"testing"
)

func TestParamParser_String(t *testing.T) {
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
			got, errResult := pp.String(tt.key)
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

func TestParamParser_StringOpt(t *testing.T) {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pp := NewParamParser(tt.args)
			got := pp.StringOpt(tt.key, tt.defaultVal)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParamParser_Int(t *testing.T) {
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
			got, errResult := pp.Int(tt.key)
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

func TestParamParser_IntOpt(t *testing.T) {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pp := NewParamParser(tt.args)
			got := pp.IntOpt(tt.key, tt.defaultVal)
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParamParser_Float(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    map[string]any
		key     string
		want    float64
		wantErr string
	}{
		{name: "valid", args: map[string]any{"k": 3.14}, key: "k", want: 3.14},
		{name: "whole", args: map[string]any{"k": float64(7)}, key: "k", want: 7.0},
		{name: "missing", args: map[string]any{}, key: "k", wantErr: "k required"},
		{name: "wrong_type", args: map[string]any{"k": "nope"}, key: "k", wantErr: "k must be a number"},
		{name: "nil_map", args: nil, key: "k", wantErr: "k required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pp := NewParamParser(tt.args)
			got, errResult := pp.Float(tt.key)
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
					t.Errorf("got %f, want %f", got, tt.want)
				}
			}
		})
	}
}

func TestParamParser_Bool(t *testing.T) {
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
			got, errResult := pp.Bool(tt.key)
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
					t.Errorf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}
