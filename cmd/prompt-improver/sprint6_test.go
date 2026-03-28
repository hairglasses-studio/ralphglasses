package main

import (
	"testing"
)

func TestDispatch_HelpShortDash(t *testing.T) {
	err := dispatch([]string{"-h"})
	if err != nil {
		t.Errorf("dispatch -h should not error, got: %v", err)
	}
}

func TestDispatch_Enhance_NoPromptSprint6(t *testing.T) {
	err := dispatch([]string{"enhance"})
	if err == nil {
		t.Error("dispatch enhance without prompt should error")
	}
}

func TestDispatch_Improve_NoPromptSprint6(t *testing.T) {
	err := dispatch([]string{"improve"})
	if err == nil {
		t.Error("dispatch improve without prompt should error")
	}
}

func TestDispatch_Analyze_NoPromptSprint6(t *testing.T) {
	err := dispatch([]string{"analyze"})
	if err == nil {
		t.Error("dispatch analyze without prompt should error")
	}
}

func TestDispatch_Lint_NoPromptSprint6(t *testing.T) {
	err := dispatch([]string{"lint"})
	if err == nil {
		t.Error("dispatch lint without prompt should error")
	}
}

func TestDispatch_Diff_NoPromptSprint6(t *testing.T) {
	err := dispatch([]string{"diff"})
	if err == nil {
		t.Error("dispatch diff without prompt should error")
	}
}

func TestDispatch_Template_NoNameSprint6(t *testing.T) {
	err := dispatch([]string{"template"})
	if err == nil {
		t.Error("dispatch template without name should error")
	}
}

func TestDispatch_Enhance_WithLongPrompt(t *testing.T) {
	err := dispatch([]string{"enhance", "fix the bug in the login handler that causes a null pointer when the user session expires"})
	if err != nil {
		t.Errorf("dispatch enhance with prompt should not error, got: %v", err)
	}
}

func TestDispatch_Analyze_WithDetailedPrompt(t *testing.T) {
	err := dispatch([]string{"analyze", "refactor the database layer to use connection pooling and add retry logic"})
	if err != nil {
		t.Errorf("dispatch analyze with prompt should not error, got: %v", err)
	}
}

func TestDispatch_Lint_WithVaguePrompt(t *testing.T) {
	err := dispatch([]string{"lint", "do the thing properly and ensure everything works correctly"})
	if err != nil {
		t.Errorf("dispatch lint with prompt should not error, got: %v", err)
	}
}

func TestDispatch_UnknownCmd_TreatedAsPrompt(t *testing.T) {
	err := dispatch([]string{"some", "random", "prompt", "text"})
	if err != nil {
		t.Errorf("unknown command should be treated as prompt, got: %v", err)
	}
}

func TestParseFlags_NilArgs(t *testing.T) {
	vars := parseFlags(nil)
	if len(vars) != 0 {
		t.Errorf("parseFlags(nil) should return empty map, got %v", vars)
	}
}

func TestParseFlags_WithMultipleFlags(t *testing.T) {
	vars := parseFlags([]string{"--system", "resolume", "--symptoms", "clips stuck"})
	if vars["system"] != "resolume" {
		t.Errorf("system = %q, want %q", vars["system"], "resolume")
	}
	if vars["symptoms"] != "clips stuck" {
		t.Errorf("symptoms = %q, want %q", vars["symptoms"], "clips stuck")
	}
}

func TestParseFlags_TrailingFlagNoValue(t *testing.T) {
	vars := parseFlags([]string{"--key"})
	if len(vars) != 0 {
		t.Errorf("flag without value should be ignored, got %v", vars)
	}
}

func TestParseFlags_MixedPositionalAndFlags(t *testing.T) {
	vars := parseFlags([]string{"positional", "--key", "val", "another"})
	if vars["key"] != "val" {
		t.Errorf("key = %q, want %q", vars["key"], "val")
	}
	if _, ok := vars["positional"]; ok {
		t.Error("positional args should not be parsed as flags")
	}
}

// TestDispatch_CacheCheck_NoFileSprint6 skipped: cache-check calls os.Exit(1)
// when no file arg is provided and stdin is not a pipe.

func TestDispatch_Enhance_WithTypeFlag(t *testing.T) {
	err := dispatch([]string{"enhance", "add unit tests for the auth module", "--type", "code"})
	if err != nil {
		t.Errorf("dispatch enhance with --type should not error, got: %v", err)
	}
}

func TestDispatch_Enhance_WithQuietFlag(t *testing.T) {
	err := dispatch([]string{"enhance", "fix the bug", "--quiet"})
	if err != nil {
		t.Errorf("dispatch enhance with --quiet should not error, got: %v", err)
	}
}

func TestDispatch_Enhance_ShortQuietFlag(t *testing.T) {
	err := dispatch([]string{"enhance", "fix the bug", "-q"})
	if err != nil {
		t.Errorf("dispatch enhance with -q should not error, got: %v", err)
	}
}
