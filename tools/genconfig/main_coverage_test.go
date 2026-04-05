package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

func TestFriendlyType_Primitives(t *testing.T) {
	tests := []struct {
		typ  reflect.Type
		want string
	}{
		{reflect.TypeFor[string](), "string"},
		{reflect.TypeFor[int](), "int"},
		{reflect.TypeFor[int64](), "int64"},
		{reflect.TypeFor[float64](), "float64"},
		{reflect.TypeFor[bool](), "bool"},
	}
	for _, tt := range tests {
		got := friendlyType(tt.typ)
		if got != tt.want {
			t.Errorf("friendlyType(%v) = %q, want %q", tt.typ, got, tt.want)
		}
	}
}

func TestFriendlyType_Duration(t *testing.T) {
	got := friendlyType(reflect.TypeFor[time.Duration]())
	if got != "duration" {
		t.Errorf("friendlyType(time.Duration) = %q, want duration", got)
	}
}

func TestFriendlyType_Slice(t *testing.T) {
	got := friendlyType(reflect.TypeFor[[]string]())
	if got != "[]string" {
		t.Errorf("friendlyType([]string) = %q, want []string", got)
	}
}

func TestFriendlyType_Ptr(t *testing.T) {
	got := friendlyType(reflect.TypeFor[*string]())
	if got != "*string" {
		t.Errorf("friendlyType(*string) = %q, want *string", got)
	}
}

func TestFriendlyType_Struct(t *testing.T) {
	type MyStruct struct{}
	got := friendlyType(reflect.TypeFor[MyStruct]())
	if got != "MyStruct" {
		t.Errorf("friendlyType(MyStruct) = %q, want MyStruct", got)
	}
}

func TestStructFieldDescription_Known(t *testing.T) {
	type testStruct struct {
		ScanPaths       []string
		DefaultProvider string
		MaxWorkers      int
	}
	ts := reflect.TypeFor[testStruct]()

	tests := []struct {
		fieldName string
		wantSub   string
	}{
		{"ScanPaths", "Directories"},
		{"DefaultProvider", "Default LLM provider"},
		{"MaxWorkers", "Maximum concurrent"},
	}

	for _, tt := range tests {
		f, ok := ts.FieldByName(tt.fieldName)
		if !ok {
			t.Fatalf("field %q not found in test struct", tt.fieldName)
		}
		got := structFieldDescription(f)
		if got == "" {
			t.Errorf("structFieldDescription(%q) = empty", tt.fieldName)
		}
		// The description should contain a recognizable substring.
		found := false
		for _, substr := range []string{tt.wantSub} {
			if len(got) >= len(substr) {
				for i := 0; i <= len(got)-len(substr); i++ {
					if got[i:i+len(substr)] == substr {
						found = true
						break
					}
				}
			}
			if found {
				break
			}
		}
		if !found {
			t.Errorf("structFieldDescription(%q) = %q, want it to contain %q", tt.fieldName, got, tt.wantSub)
		}
	}
}

func TestStructFieldDescription_Unknown(t *testing.T) {
	type testStruct struct {
		UnknownField string
	}
	ts := reflect.TypeFor[testStruct]()
	f, _ := ts.FieldByName("UnknownField")
	got := structFieldDescription(f)
	// Unknown fields return their name.
	if got != "UnknownField" {
		t.Errorf("structFieldDescription(unknown) = %q, want UnknownField", got)
	}
}

func TestStructFieldDefault_Known(t *testing.T) {
	type testStruct struct {
		MaxWorkers      int
		ScanPaths       []string
		DefaultProvider string
	}
	ts := reflect.TypeFor[testStruct]()

	knownFields := []string{"MaxWorkers", "ScanPaths", "DefaultProvider"}
	for _, name := range knownFields {
		f, ok := ts.FieldByName(name)
		if !ok {
			t.Fatalf("field %q not found", name)
		}
		got := structFieldDefault(f)
		if got == "" {
			t.Errorf("structFieldDefault(%q) returned empty", name)
		}
	}
}

func TestStructFieldDefault_Unknown(t *testing.T) {
	type testStruct struct {
		SomeOtherField string
	}
	ts := reflect.TypeFor[testStruct]()
	f, _ := ts.FieldByName("SomeOtherField")
	got := structFieldDefault(f)
	if got != "--" {
		t.Errorf("structFieldDefault(unknown) = %q, want --", got)
	}
}

func TestConfigKeyTypeName(t *testing.T) {
	tests := []struct {
		typ  model.ConfigKeyType
		want string
	}{
		{model.ConfigTypeString, "string"},
		{model.ConfigTypeInt, "int"},
		{model.ConfigTypeFloat, "float"},
		{model.ConfigTypeBool, "bool"},
		{model.ConfigKeyType(99), "unknown"},
	}
	for _, tt := range tests {
		got := configKeyTypeName(tt.typ)
		if got != tt.want {
			t.Errorf("configKeyTypeName(%v) = %q, want %q", tt.typ, got, tt.want)
		}
	}
}
