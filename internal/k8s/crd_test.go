package k8s

import (
	"encoding/json"
	"testing"
	"time"
)

// -------------------------------------------------------------------
// RalphSessionSpec.Validate
// -------------------------------------------------------------------

func TestRalphSessionSpec_Validate_AllProviders(t *testing.T) {
	for _, p := range []string{"claude", "gemini", "codex"} {
		t.Run(p, func(t *testing.T) {
			spec := RalphSessionSpec{Provider: p, Prompt: "do something"}
			if errs := spec.Validate(); len(errs) != 0 {
				t.Errorf("expected no errors for provider %s, got %v", p, errs)
			}
		})
	}
}

func TestRalphSessionSpec_Validate_InvalidProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
	}{
		{"gpt4", "gpt4"},
		{"openai", "openai"},
		{"CLAUDE", "CLAUDE"}, // case-sensitive
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := RalphSessionSpec{Provider: tt.provider, Prompt: "hello"}
			errs := spec.Validate()
			if len(errs) == 0 {
				t.Errorf("expected validation error for provider %q", tt.provider)
			}
		})
	}
}

func TestRalphSessionSpec_Validate_BoundaryValues(t *testing.T) {
	tests := []struct {
		name     string
		spec     RalphSessionSpec
		wantErrs int
	}{
		{
			name:     "zero max turns is valid",
			spec:     RalphSessionSpec{Provider: "claude", Prompt: "hi", MaxTurns: 0},
			wantErrs: 0,
		},
		{
			name:     "positive max turns is valid",
			spec:     RalphSessionSpec{Provider: "claude", Prompt: "hi", MaxTurns: 100},
			wantErrs: 0,
		},
		{
			name:     "negative max turns is invalid",
			spec:     RalphSessionSpec{Provider: "claude", Prompt: "hi", MaxTurns: -1},
			wantErrs: 1,
		},
		{
			name:     "zero budget is valid",
			spec:     RalphSessionSpec{Provider: "claude", Prompt: "hi", BudgetUSD: 0},
			wantErrs: 0,
		},
		{
			name:     "positive budget is valid",
			spec:     RalphSessionSpec{Provider: "claude", Prompt: "hi", BudgetUSD: 10.5},
			wantErrs: 0,
		},
		{
			name:     "negative budget is invalid",
			spec:     RalphSessionSpec{Provider: "claude", Prompt: "hi", BudgetUSD: -0.01},
			wantErrs: 1,
		},
		{
			name:     "all invalid fields",
			spec:     RalphSessionSpec{Provider: "bad", Prompt: "", MaxTurns: -5, BudgetUSD: -10},
			wantErrs: 4, // invalid provider + missing prompt + negative turns + negative budget
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.spec.Validate()
			if len(errs) != tt.wantErrs {
				t.Errorf("expected %d errors, got %d: %v", tt.wantErrs, len(errs), errs)
			}
		})
	}
}

// -------------------------------------------------------------------
// RalphFleetSpec.Validate
// -------------------------------------------------------------------

func TestRalphFleetSpec_Validate_Comprehensive(t *testing.T) {
	tests := []struct {
		name     string
		spec     RalphFleetSpec
		wantErrs int
	}{
		{
			name: "valid fleet",
			spec: RalphFleetSpec{
				Replicas:        5,
				BudgetUSD:       100.0,
				SessionTemplate: RalphSessionSpec{Provider: "gemini", Prompt: "analyze"},
			},
			wantErrs: 0,
		},
		{
			name: "zero replicas is valid",
			spec: RalphFleetSpec{
				Replicas:        0,
				SessionTemplate: RalphSessionSpec{Provider: "claude", Prompt: "work"},
			},
			wantErrs: 0,
		},
		{
			name: "negative replicas",
			spec: RalphFleetSpec{
				Replicas:        -1,
				SessionTemplate: RalphSessionSpec{Provider: "claude", Prompt: "work"},
			},
			wantErrs: 1,
		},
		{
			name: "negative fleet budget",
			spec: RalphFleetSpec{
				Replicas:        1,
				BudgetUSD:       -50.0,
				SessionTemplate: RalphSessionSpec{Provider: "claude", Prompt: "work"},
			},
			wantErrs: 1,
		},
		{
			name: "template errors propagate with prefix",
			spec: RalphFleetSpec{
				Replicas:        1,
				SessionTemplate: RalphSessionSpec{Provider: "invalid", Prompt: ""},
			},
			wantErrs: 2, // spec.sessionTemplate.provider + spec.sessionTemplate.prompt
		},
		{
			name: "fleet and template errors combined",
			spec: RalphFleetSpec{
				Replicas:        -1,
				BudgetUSD:       -10,
				SessionTemplate: RalphSessionSpec{},
			},
			wantErrs: 4, // -replicas + -budget + missing provider + missing prompt
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.spec.Validate()
			if len(errs) != tt.wantErrs {
				t.Errorf("expected %d errors, got %d: %v", tt.wantErrs, len(errs), errs)
			}
		})
	}
}

// -------------------------------------------------------------------
// JSON serialization round-trip
// -------------------------------------------------------------------

func TestRalphSession_JSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second) // JSON loses sub-second precision with default format
	session := RalphSession{
		TypeMeta: TypeMeta{Kind: "RalphSession", APIVersion: GroupName + "/" + Version},
		ObjectMeta: ObjectMeta{
			Name:              "test-session",
			Namespace:         "prod",
			UID:               "abc-123",
			ResourceVersion:   "42",
			Generation:        3,
			Labels:            map[string]string{"env": "prod"},
			Annotations:       map[string]string{"note": "test"},
			CreationTimestamp: now,
			Finalizers:        []string{FinalizerName},
		},
		Spec: RalphSessionSpec{
			Provider:      "claude",
			Model:         "claude-opus-4-20250514",
			Prompt:        "build a thing",
			RepoPath:      "/workspace/repo",
			MaxTurns:      50,
			BudgetUSD:     25.0,
			EnhancePrompt: true,
			Image:         "custom:v1",
			Resources: &ResourceRequirements{
				Requests: ResourceList{"cpu": "500m"},
				Limits:   ResourceList{"cpu": "2"},
			},
			EnvFrom: []EnvFromSource{
				{SecretRef: &SecretReference{Name: "my-secret"}},
				{ConfigMapRef: &ConfigMapReference{Name: "my-config"}},
			},
			VolumeMounts: []VolumeMount{
				{Name: "data", MountPath: "/data", ReadOnly: true},
			},
			Volumes: []Volume{
				{Name: "data", SecretName: "data-secret"},
			},
			TeamName: "alpha",
		},
		Status: RalphSessionStatus{
			Phase:     "Running",
			PodName:   "ralph-session-test-session",
			SpentUSD:  3.50,
			TurnCount: 10,
		},
	}

	data, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded RalphSession
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Spot-check key fields survived the round-trip.
	if decoded.Kind != "RalphSession" {
		t.Errorf("kind: got %s", decoded.Kind)
	}
	if decoded.Name != "test-session" {
		t.Errorf("name: got %s", decoded.Name)
	}
	if decoded.Spec.Provider != "claude" {
		t.Errorf("provider: got %s", decoded.Spec.Provider)
	}
	if decoded.Spec.MaxTurns != 50 {
		t.Errorf("maxTurns: got %d", decoded.Spec.MaxTurns)
	}
	if decoded.Spec.BudgetUSD != 25.0 {
		t.Errorf("budgetUSD: got %f", decoded.Spec.BudgetUSD)
	}
	if decoded.Spec.Resources == nil || decoded.Spec.Resources.Requests["cpu"] != "500m" {
		t.Error("resources not preserved")
	}
	if len(decoded.Spec.EnvFrom) != 2 {
		t.Errorf("envFrom: got %d", len(decoded.Spec.EnvFrom))
	}
	if decoded.Status.Phase != "Running" {
		t.Errorf("phase: got %s", decoded.Status.Phase)
	}
	if decoded.Status.SpentUSD != 3.50 {
		t.Errorf("spentUSD: got %f", decoded.Status.SpentUSD)
	}
}

func TestRalphFleet_JSONRoundTrip(t *testing.T) {
	fleet := RalphFleet{
		TypeMeta: TypeMeta{Kind: "RalphFleet", APIVersion: GroupName + "/" + Version},
		ObjectMeta: ObjectMeta{
			Name:      "test-fleet",
			Namespace: "default",
		},
		Spec: RalphFleetSpec{
			Replicas:        3,
			BudgetUSD:       100.0,
			MaxConcurrent:   2,
			RoutingStrategy: "least-cost",
			SessionTemplate: RalphSessionSpec{
				Provider: "gemini",
				Prompt:   "work hard",
			},
		},
		Status: RalphFleetStatus{
			ReadySessions: 2,
			TotalSessions: 3,
			TotalSpentUSD: 15.75,
		},
	}

	data, err := json.Marshal(fleet)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded RalphFleet
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Spec.Replicas != 3 {
		t.Errorf("replicas: got %d", decoded.Spec.Replicas)
	}
	if decoded.Spec.RoutingStrategy != "least-cost" {
		t.Errorf("routing: got %s", decoded.Spec.RoutingStrategy)
	}
	if decoded.Status.TotalSpentUSD != 15.75 {
		t.Errorf("totalSpent: got %f", decoded.Status.TotalSpentUSD)
	}
}

func TestRalphSessionList_JSONRoundTrip(t *testing.T) {
	list := RalphSessionList{
		TypeMeta: TypeMeta{Kind: "RalphSessionList", APIVersion: GroupName + "/" + Version},
		ListMeta: ListMeta{ResourceVersion: "100", Continue: "token123"},
		Items: []RalphSession{
			{ObjectMeta: ObjectMeta{Name: "s1"}, Spec: RalphSessionSpec{Provider: "claude", Prompt: "a"}},
			{ObjectMeta: ObjectMeta{Name: "s2"}, Spec: RalphSessionSpec{Provider: "gemini", Prompt: "b"}},
		},
	}

	data, err := json.Marshal(list)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded RalphSessionList
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(decoded.Items) != 2 {
		t.Fatalf("items: got %d", len(decoded.Items))
	}
	if decoded.ListMeta.Continue != "token123" {
		t.Errorf("continue token: got %s", decoded.ListMeta.Continue)
	}
}

func TestRalphFleetList_JSONRoundTrip(t *testing.T) {
	list := RalphFleetList{
		TypeMeta: TypeMeta{Kind: "RalphFleetList"},
		Items: []RalphFleet{
			{ObjectMeta: ObjectMeta{Name: "f1"}},
		},
	}

	data, err := json.Marshal(list)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded RalphFleetList
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.Items) != 1 {
		t.Fatalf("items: got %d", len(decoded.Items))
	}
}

// -------------------------------------------------------------------
// Constants
// -------------------------------------------------------------------

func TestGroupNameAndVersion(t *testing.T) {
	if GroupName != "ralphglasses.hairglasses.studio" {
		t.Errorf("unexpected GroupName: %s", GroupName)
	}
	if Version != "v1alpha1" {
		t.Errorf("unexpected Version: %s", Version)
	}
}

// -------------------------------------------------------------------
// ValidProviders map
// -------------------------------------------------------------------

func TestValidProviders(t *testing.T) {
	expected := []string{"claude", "gemini", "codex"}
	for _, p := range expected {
		if !ValidProviders[p] {
			t.Errorf("expected %s to be a valid provider", p)
		}
	}
	invalid := []string{"gpt4", "openai", "CLAUDE", ""}
	for _, p := range invalid {
		if ValidProviders[p] {
			t.Errorf("expected %s to NOT be a valid provider", p)
		}
	}
}

// -------------------------------------------------------------------
// Embedded types: Volume, VolumeMount, EnvFromSource
// -------------------------------------------------------------------

func TestVolumeTypes_JSON(t *testing.T) {
	vol := Volume{Name: "secret-vol", SecretName: "my-secret"}
	data, err := json.Marshal(vol)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Volume
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Name != "secret-vol" || decoded.SecretName != "my-secret" {
		t.Errorf("unexpected volume: %+v", decoded)
	}

	// ConfigMap volume
	vol2 := Volume{Name: "config-vol", ConfigMapName: "my-cm"}
	data2, _ := json.Marshal(vol2)
	var decoded2 Volume
	_ = json.Unmarshal(data2, &decoded2)
	if decoded2.ConfigMapName != "my-cm" {
		t.Errorf("configmap name not preserved: %+v", decoded2)
	}

	// EmptyDir volume
	vol3 := Volume{Name: "tmp", EmptyDir: true}
	data3, _ := json.Marshal(vol3)
	var decoded3 Volume
	_ = json.Unmarshal(data3, &decoded3)
	if !decoded3.EmptyDir {
		t.Error("emptyDir not preserved")
	}
}

func TestCondition_JSON(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	c := Condition{
		Type:               "Ready",
		Status:             "True",
		LastTransitionTime: now,
		Reason:             "PodRunning",
		Message:            "All good",
	}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Condition
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Type != "Ready" || decoded.Status != "True" || decoded.Reason != "PodRunning" {
		t.Errorf("condition not preserved: %+v", decoded)
	}
}

// -------------------------------------------------------------------
// ObjectMeta with DeletionTimestamp
// -------------------------------------------------------------------

func TestObjectMeta_DeletionTimestamp(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	meta := ObjectMeta{
		Name:              "test",
		DeletionTimestamp: &now,
	}

	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ObjectMeta
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.DeletionTimestamp == nil {
		t.Fatal("expected deletion timestamp")
	}
}

func TestResourceRequirements_JSON(t *testing.T) {
	rr := ResourceRequirements{
		Requests: ResourceList{"cpu": "100m", "memory": "64Mi"},
		Limits:   ResourceList{"cpu": "500m", "memory": "256Mi"},
	}

	data, err := json.Marshal(rr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ResourceRequirements
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Requests["cpu"] != "100m" {
		t.Errorf("cpu request: got %s", decoded.Requests["cpu"])
	}
	if decoded.Limits["memory"] != "256Mi" {
		t.Errorf("memory limit: got %s", decoded.Limits["memory"])
	}
}
