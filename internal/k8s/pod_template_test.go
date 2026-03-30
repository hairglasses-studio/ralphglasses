package k8s

import (
	"fmt"
	"strings"
	"testing"
)

// -------------------------------------------------------------------
// NewPodTemplateBuilder
// -------------------------------------------------------------------

func TestNewPodTemplateBuilder_NotNil(t *testing.T) {
	session := &RalphSession{}
	b := NewPodTemplateBuilder(session)
	if b == nil {
		t.Fatal("expected non-nil builder")
	}
}

// -------------------------------------------------------------------
// DefaultResources
// -------------------------------------------------------------------

func TestDefaultResources(t *testing.T) {
	r := DefaultResources()
	if r == nil {
		t.Fatal("expected non-nil resources")
	}
	if r.Requests["cpu"] != "250m" {
		t.Errorf("expected cpu request 250m, got %s", r.Requests["cpu"])
	}
	if r.Requests["memory"] != "256Mi" {
		t.Errorf("expected memory request 256Mi, got %s", r.Requests["memory"])
	}
	if r.Limits["cpu"] != "1" {
		t.Errorf("expected cpu limit 1, got %s", r.Limits["cpu"])
	}
	if r.Limits["memory"] != "1Gi" {
		t.Errorf("expected memory limit 1Gi, got %s", r.Limits["memory"])
	}
}

// -------------------------------------------------------------------
// Build: image selection
// -------------------------------------------------------------------

func TestBuild_DefaultImage(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec:       RalphSessionSpec{Provider: "claude", Prompt: "hi"},
	}
	pod := NewPodTemplateBuilder(session).Build()
	if pod.Image != DefaultImage {
		t.Errorf("expected default image %s, got %s", DefaultImage, pod.Image)
	}
}

func TestBuild_CustomImage(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec:       RalphSessionSpec{Provider: "claude", Prompt: "hi", Image: "my-img:v2"},
	}
	pod := NewPodTemplateBuilder(session).Build()
	if pod.Image != "my-img:v2" {
		t.Errorf("expected my-img:v2, got %s", pod.Image)
	}
}

// -------------------------------------------------------------------
// Build: resource defaults vs custom
// -------------------------------------------------------------------

func TestBuild_DefaultResourcesApplied(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec:       RalphSessionSpec{Provider: "claude", Prompt: "hi"},
	}
	pod := NewPodTemplateBuilder(session).Build()
	if pod.Resources == nil {
		t.Fatal("expected default resources")
	}
	if pod.Resources.Requests["cpu"] != "250m" {
		t.Errorf("expected default cpu request, got %s", pod.Resources.Requests["cpu"])
	}
}

func TestBuild_CustomResourcesPreserved(t *testing.T) {
	custom := &ResourceRequirements{
		Requests: ResourceList{"cpu": "2", "memory": "4Gi"},
		Limits:   ResourceList{"cpu": "4", "memory": "8Gi"},
	}
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec:       RalphSessionSpec{Provider: "claude", Prompt: "hi", Resources: custom},
	}
	pod := NewPodTemplateBuilder(session).Build()
	if pod.Resources.Requests["cpu"] != "2" {
		t.Errorf("expected custom cpu 2, got %s", pod.Resources.Requests["cpu"])
	}
}

// -------------------------------------------------------------------
// Build: labels
// -------------------------------------------------------------------

func TestBuild_StandardLabels(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec:       RalphSessionSpec{Provider: "gemini", Prompt: "hi"},
	}
	pod := NewPodTemplateBuilder(session).Build()

	expected := map[string]string{
		"app.kubernetes.io/name":       "ralphglasses",
		"app.kubernetes.io/component":  "session",
		"app.kubernetes.io/managed-by": "ralphglasses-operator",
		"ralphglasses.studio/session":  "s1",
		"ralphglasses.studio/provider": "gemini",
	}
	for k, v := range expected {
		if pod.Labels[k] != v {
			t.Errorf("label %s: expected %q, got %q", k, v, pod.Labels[k])
		}
	}
}

func TestBuild_TeamLabel(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec:       RalphSessionSpec{Provider: "claude", Prompt: "hi", TeamName: "bravo"},
	}
	pod := NewPodTemplateBuilder(session).Build()
	if pod.Labels["ralphglasses.studio/team"] != "bravo" {
		t.Error("missing team label")
	}
}

func TestBuild_NoTeamLabelWhenEmpty(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec:       RalphSessionSpec{Provider: "claude", Prompt: "hi"},
	}
	pod := NewPodTemplateBuilder(session).Build()
	if _, ok := pod.Labels["ralphglasses.studio/team"]; ok {
		t.Error("team label should not be present when TeamName is empty")
	}
}

func TestBuild_UserLabelsDoNotOverrideReserved(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{
			Name:      "s1",
			Namespace: "ns",
			Labels: map[string]string{
				"app.kubernetes.io/name": "should-be-ignored",
				"custom-label":           "custom-value",
			},
		},
		Spec: RalphSessionSpec{Provider: "claude", Prompt: "hi"},
	}
	pod := NewPodTemplateBuilder(session).Build()

	// Reserved label should not be overwritten.
	if pod.Labels["app.kubernetes.io/name"] != "ralphglasses" {
		t.Errorf("reserved label was overwritten: got %s", pod.Labels["app.kubernetes.io/name"])
	}
	// Custom label should be merged.
	if pod.Labels["custom-label"] != "custom-value" {
		t.Error("custom label not merged")
	}
}

// -------------------------------------------------------------------
// Build: annotations
// -------------------------------------------------------------------

func TestBuild_PromptHashAnnotation(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec:       RalphSessionSpec{Provider: "claude", Prompt: "hello"},
	}
	pod := NewPodTemplateBuilder(session).Build()

	hash := pod.Annotations["ralphglasses.studio/prompt-hash"]
	if hash == "" {
		t.Fatal("missing prompt-hash annotation")
	}
	if len(hash) != 8 {
		t.Errorf("expected 8-char hash, got %q", hash)
	}
}

func TestBuild_BudgetAnnotation(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec:       RalphSessionSpec{Provider: "claude", Prompt: "hi", BudgetUSD: 12.34},
	}
	pod := NewPodTemplateBuilder(session).Build()

	if pod.Annotations["ralphglasses.studio/budget-usd"] != "12.34" {
		t.Errorf("budget annotation: got %s", pod.Annotations["ralphglasses.studio/budget-usd"])
	}
}

func TestBuild_NoBudgetAnnotationWhenZero(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec:       RalphSessionSpec{Provider: "claude", Prompt: "hi", BudgetUSD: 0},
	}
	pod := NewPodTemplateBuilder(session).Build()

	if _, ok := pod.Annotations["ralphglasses.studio/budget-usd"]; ok {
		t.Error("budget annotation should not be present when BudgetUSD is 0")
	}
}

// -------------------------------------------------------------------
// Build: command and args per provider
// -------------------------------------------------------------------

func TestBuild_ClaudeCommand(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec: RalphSessionSpec{
			Provider: "claude",
			Prompt:   "do it",
			Model:    "claude-opus-4-20250514",
			MaxTurns: 10,
		},
	}
	pod := NewPodTemplateBuilder(session).Build()

	if len(pod.Command) != 1 || pod.Command[0] != "claude" {
		t.Errorf("command: got %v", pod.Command)
	}
	args := strings.Join(pod.Args, " ")
	if !strings.Contains(args, "--print") {
		t.Error("missing --print flag")
	}
	if !strings.Contains(args, "--output-format stream-json") {
		t.Error("missing --output-format flag")
	}
	if !strings.Contains(args, "--model claude-opus-4-20250514") {
		t.Error("missing --model flag")
	}
	if !strings.Contains(args, "--max-turns 10") {
		t.Error("missing --max-turns flag")
	}
	// Prompt should be the last arg.
	if pod.Args[len(pod.Args)-1] != "do it" {
		t.Errorf("prompt should be last arg, got %s", pod.Args[len(pod.Args)-1])
	}
}

func TestBuild_ClaudeCommandMinimal(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec:       RalphSessionSpec{Provider: "claude", Prompt: "hi"},
	}
	pod := NewPodTemplateBuilder(session).Build()

	args := strings.Join(pod.Args, " ")
	if strings.Contains(args, "--model") {
		t.Error("should not include --model when empty")
	}
	if strings.Contains(args, "--max-turns") {
		t.Error("should not include --max-turns when 0")
	}
}

func TestBuild_GeminiCommand(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec: RalphSessionSpec{
			Provider: "gemini",
			Prompt:   "analyze this",
			Model:    "gemini-2.5-pro",
		},
	}
	pod := NewPodTemplateBuilder(session).Build()

	if len(pod.Command) != 1 || pod.Command[0] != "gemini" {
		t.Errorf("command: got %v", pod.Command)
	}
	if pod.Args[0] != "-p" || pod.Args[1] != "analyze this" {
		t.Errorf("expected -p <prompt> first, got %v", pod.Args)
	}
	args := strings.Join(pod.Args, " ")
	if !strings.Contains(args, "--model gemini-2.5-pro") {
		t.Error("missing model flag for gemini")
	}
}

func TestBuild_CodexCommand(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec: RalphSessionSpec{
			Provider: "codex",
			Prompt:   "fix bug",
			Model:    "o3",
		},
	}
	pod := NewPodTemplateBuilder(session).Build()

	if len(pod.Command) != 1 || pod.Command[0] != "codex" {
		t.Errorf("command: got %v", pod.Command)
	}
	if pod.Args[0] != "--prompt" || pod.Args[1] != "fix bug" {
		t.Errorf("expected --prompt <prompt>, got %v", pod.Args)
	}
	args := strings.Join(pod.Args, " ")
	if !strings.Contains(args, "--model o3") {
		t.Error("missing model flag for codex")
	}
}

func TestBuild_UnknownProviderFallback(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec:       RalphSessionSpec{Provider: "unknown-llm", Prompt: "hello"},
	}
	pod := NewPodTemplateBuilder(session).Build()

	if len(pod.Command) < 1 || pod.Command[0] != "ralphglasses" {
		t.Errorf("fallback command: got %v", pod.Command)
	}
	args := strings.Join(pod.Args, " ")
	if !strings.Contains(args, "--provider unknown-llm") {
		t.Error("fallback should include --provider")
	}
}

// -------------------------------------------------------------------
// Build: environment variables
// -------------------------------------------------------------------

func TestBuild_BaseEnvVars(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec:       RalphSessionSpec{Provider: "claude", Prompt: "hi"},
	}
	pod := NewPodTemplateBuilder(session).Build()

	envMap := make(map[string]string)
	for _, e := range pod.Env {
		envMap[e.Name] = e.Value
	}

	if envMap["RALPH_SESSION_NAME"] != "s1" {
		t.Errorf("RALPH_SESSION_NAME: got %s", envMap["RALPH_SESSION_NAME"])
	}
	if envMap["RALPH_PROVIDER"] != "claude" {
		t.Errorf("RALPH_PROVIDER: got %s", envMap["RALPH_PROVIDER"])
	}
	if envMap["RALPH_NAMESPACE"] != "ns" {
		t.Errorf("RALPH_NAMESPACE: got %s", envMap["RALPH_NAMESPACE"])
	}
}

func TestBuild_OptionalEnvVars(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec: RalphSessionSpec{
			Provider:  "claude",
			Prompt:    "hi",
			Model:     "opus",
			MaxTurns:  20,
			BudgetUSD: 5.0,
		},
	}
	pod := NewPodTemplateBuilder(session).Build()

	envMap := make(map[string]string)
	for _, e := range pod.Env {
		envMap[e.Name] = e.Value
	}

	if envMap["RALPH_MODEL"] != "opus" {
		t.Errorf("RALPH_MODEL: got %s", envMap["RALPH_MODEL"])
	}
	if envMap["RALPH_MAX_TURNS"] != "20" {
		t.Errorf("RALPH_MAX_TURNS: got %s", envMap["RALPH_MAX_TURNS"])
	}
	if envMap["RALPH_BUDGET_USD"] != "5.00" {
		t.Errorf("RALPH_BUDGET_USD: got %s", envMap["RALPH_BUDGET_USD"])
	}
}

func TestBuild_NoOptionalEnvWhenUnset(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec:       RalphSessionSpec{Provider: "claude", Prompt: "hi"},
	}
	pod := NewPodTemplateBuilder(session).Build()

	for _, e := range pod.Env {
		if e.Name == "RALPH_MODEL" || e.Name == "RALPH_MAX_TURNS" || e.Name == "RALPH_BUDGET_USD" {
			t.Errorf("unexpected optional env var: %s", e.Name)
		}
	}
}

// -------------------------------------------------------------------
// Build: envFrom (API key secrets)
// -------------------------------------------------------------------

func TestBuild_DefaultAPIKeySecrets(t *testing.T) {
	tests := []struct {
		provider   string
		wantSecret string
	}{
		{"claude", "ralph-anthropic-api-key"},
		{"gemini", "ralph-google-api-key"},
		{"codex", "ralph-openai-api-key"},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			session := &RalphSession{
				ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
				Spec:       RalphSessionSpec{Provider: tt.provider, Prompt: "hi"},
			}
			pod := NewPodTemplateBuilder(session).Build()

			found := false
			for _, ef := range pod.EnvFrom {
				if ef.SecretRef != nil && ef.SecretRef.Name == tt.wantSecret {
					found = true
				}
			}
			if !found {
				t.Errorf("missing default secret %s", tt.wantSecret)
			}
		})
	}
}

func TestBuild_UserEnvFromMerged(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec: RalphSessionSpec{
			Provider: "claude",
			Prompt:   "hi",
			EnvFrom: []EnvFromSource{
				{ConfigMapRef: &ConfigMapReference{Name: "user-config"}},
			},
		},
	}
	pod := NewPodTemplateBuilder(session).Build()

	// Should have user config + default secret.
	foundConfig := false
	foundSecret := false
	for _, ef := range pod.EnvFrom {
		if ef.ConfigMapRef != nil && ef.ConfigMapRef.Name == "user-config" {
			foundConfig = true
		}
		if ef.SecretRef != nil && ef.SecretRef.Name == "ralph-anthropic-api-key" {
			foundSecret = true
		}
	}
	if !foundConfig {
		t.Error("user envFrom config not merged")
	}
	if !foundSecret {
		t.Error("default API key secret missing")
	}
}

func TestDefaultAPIKeyEnvFrom_UnknownProvider(t *testing.T) {
	result := defaultAPIKeyEnvFrom("unknown")
	if result != nil {
		t.Errorf("expected nil for unknown provider, got %v", result)
	}
}

// -------------------------------------------------------------------
// Build: volumes and mounts
// -------------------------------------------------------------------

func TestBuild_WorkspaceVolumeAlwaysPresent(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec:       RalphSessionSpec{Provider: "claude", Prompt: "hi"},
	}
	pod := NewPodTemplateBuilder(session).Build()

	foundVol := false
	for _, v := range pod.Volumes {
		if v.Name == "workspace" && v.EmptyDir {
			foundVol = true
		}
	}
	if !foundVol {
		t.Error("workspace volume missing")
	}

	foundMount := false
	for _, m := range pod.VolumeMounts {
		if m.Name == "workspace" && m.MountPath == "/workspace" {
			foundMount = true
		}
	}
	if !foundMount {
		t.Error("workspace volume mount missing")
	}
}

func TestBuild_UserVolumesAppended(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec: RalphSessionSpec{
			Provider: "claude",
			Prompt:   "hi",
			Volumes: []Volume{
				{Name: "data", SecretName: "data-secret"},
			},
			VolumeMounts: []VolumeMount{
				{Name: "data", MountPath: "/data", ReadOnly: true},
			},
		},
	}
	pod := NewPodTemplateBuilder(session).Build()

	if len(pod.Volumes) != 2 {
		t.Errorf("expected 2 volumes (workspace + data), got %d", len(pod.Volumes))
	}
	if len(pod.VolumeMounts) != 2 {
		t.Errorf("expected 2 mounts, got %d", len(pod.VolumeMounts))
	}
}

// -------------------------------------------------------------------
// Build: owner reference
// -------------------------------------------------------------------

func TestBuild_OwnerRefSetWhenUIDPresent(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns", UID: "uid-123"},
		Spec:       RalphSessionSpec{Provider: "claude", Prompt: "hi"},
	}
	pod := NewPodTemplateBuilder(session).Build()

	if pod.OwnerRef == nil {
		t.Fatal("expected owner reference")
	}
	if pod.OwnerRef.APIVersion != GroupName+"/"+Version {
		t.Errorf("apiVersion: got %s", pod.OwnerRef.APIVersion)
	}
	if pod.OwnerRef.Kind != "RalphSession" {
		t.Errorf("kind: got %s", pod.OwnerRef.Kind)
	}
	if pod.OwnerRef.Name != "s1" {
		t.Errorf("name: got %s", pod.OwnerRef.Name)
	}
	if pod.OwnerRef.UID != "uid-123" {
		t.Errorf("uid: got %s", pod.OwnerRef.UID)
	}
}

func TestBuild_NoOwnerRefWhenNoUID(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "s1", Namespace: "ns"},
		Spec:       RalphSessionSpec{Provider: "claude", Prompt: "hi"},
	}
	pod := NewPodTemplateBuilder(session).Build()

	if pod.OwnerRef != nil {
		t.Error("should not set owner ref when UID is empty")
	}
}

// -------------------------------------------------------------------
// Build: pod name and namespace
// -------------------------------------------------------------------

func TestBuild_PodNameAndNamespace(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{Name: "my-session", Namespace: "prod"},
		Spec:       RalphSessionSpec{Provider: "claude", Prompt: "hi"},
	}
	pod := NewPodTemplateBuilder(session).Build()

	if pod.Name != "ralph-session-my-session" {
		t.Errorf("pod name: got %s", pod.Name)
	}
	if pod.Namespace != "prod" {
		t.Errorf("namespace: got %s", pod.Namespace)
	}
}

// -------------------------------------------------------------------
// hashPrompt
// -------------------------------------------------------------------

func TestHashPrompt_Deterministic(t *testing.T) {
	h1 := hashPrompt("hello world")
	h2 := hashPrompt("hello world")
	if h1 != h2 {
		t.Errorf("same input produced different hashes: %s vs %s", h1, h2)
	}
}

func TestHashPrompt_DifferentInputsDifferentHashes(t *testing.T) {
	h1 := hashPrompt("hello")
	h2 := hashPrompt("world")
	if h1 == h2 {
		t.Error("different inputs should produce different hashes")
	}
}

func TestHashPrompt_Format(t *testing.T) {
	h := hashPrompt("test")
	if len(h) != 8 {
		t.Errorf("expected 8-char hex hash, got %q (len=%d)", h, len(h))
	}
	// Should be valid hex.
	for _, c := range h {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("non-hex character in hash: %c", c)
		}
	}
}

func TestHashPrompt_EmptyString(t *testing.T) {
	h := hashPrompt("")
	if h == "" {
		t.Error("empty string should still produce a hash")
	}
	if len(h) != 8 {
		t.Errorf("expected 8-char hash, got %q", h)
	}
}

// -------------------------------------------------------------------
// Full integration-style tests
// -------------------------------------------------------------------

func TestBuild_FullSession_AllFields(t *testing.T) {
	session := &RalphSession{
		ObjectMeta: ObjectMeta{
			Name:      "full-test",
			Namespace: "staging",
			UID:       "uid-full",
			Labels:    map[string]string{"env": "staging"},
		},
		Spec: RalphSessionSpec{
			Provider:      "claude",
			Model:         "opus",
			Prompt:        "do everything",
			MaxTurns:      100,
			BudgetUSD:     50.0,
			EnhancePrompt: true,
			Image:         "custom:latest",
			TeamName:      "core",
			Resources: &ResourceRequirements{
				Requests: ResourceList{"cpu": "4"},
				Limits:   ResourceList{"cpu": "8"},
			},
			EnvFrom: []EnvFromSource{
				{SecretRef: &SecretReference{Name: "extra-secret"}},
			},
			Volumes: []Volume{
				{Name: "cache", EmptyDir: true},
			},
			VolumeMounts: []VolumeMount{
				{Name: "cache", MountPath: "/cache"},
			},
		},
	}

	pod := NewPodTemplateBuilder(session).Build()

	// Verify all the pieces came together.
	if pod.Name != "ralph-session-full-test" {
		t.Errorf("name: %s", pod.Name)
	}
	if pod.Namespace != "staging" {
		t.Errorf("namespace: %s", pod.Namespace)
	}
	if pod.Image != "custom:latest" {
		t.Errorf("image: %s", pod.Image)
	}
	if pod.Labels["ralphglasses.studio/team"] != "core" {
		t.Error("team label missing")
	}
	if pod.Labels["env"] != "staging" {
		t.Error("user label missing")
	}
	if pod.Resources.Requests["cpu"] != "4" {
		t.Error("custom resources not applied")
	}
	if pod.OwnerRef == nil {
		t.Error("owner ref missing")
	}
	if len(pod.Volumes) != 2 {
		t.Errorf("expected 2 volumes, got %d", len(pod.Volumes))
	}
	if len(pod.VolumeMounts) != 2 {
		t.Errorf("expected 2 mounts, got %d", len(pod.VolumeMounts))
	}

	// envFrom should have user secret + default API key secret.
	if len(pod.EnvFrom) < 2 {
		t.Errorf("expected at least 2 envFrom entries, got %d", len(pod.EnvFrom))
	}

	// Budget annotation.
	if pod.Annotations["ralphglasses.studio/budget-usd"] != fmt.Sprintf("%.2f", 50.0) {
		t.Errorf("budget annotation: %s", pod.Annotations["ralphglasses.studio/budget-usd"])
	}
}
