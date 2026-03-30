package k8s

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// -------------------------------------------------------------------
// FakeClient — in-memory implementation of the Client interface for
// testing reconciler logic without a real Kubernetes cluster.
// -------------------------------------------------------------------

// FakeClient implements Client using in-memory maps.
type FakeClient struct {
	Sessions   map[string]*RalphSession // key: "namespace/name"
	Pods       map[string]*PodStatus    // key: "namespace/name"
	CreatedPods []*PodSpec
	DeletedPods []string // "namespace/name"
	Err         error    // if set, all operations return this error
}

// NewFakeClient creates an empty FakeClient.
func NewFakeClient() *FakeClient {
	return &FakeClient{
		Sessions: make(map[string]*RalphSession),
		Pods:     make(map[string]*PodStatus),
	}
}

func fakeKey(ns, name string) string { return ns + "/" + name }

func (f *FakeClient) GetSession(_ context.Context, namespace, name string) (*RalphSession, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	s := f.Sessions[fakeKey(namespace, name)]
	return s, nil
}

func (f *FakeClient) UpdateSessionStatus(_ context.Context, session *RalphSession) error {
	if f.Err != nil {
		return f.Err
	}
	f.Sessions[fakeKey(session.Namespace, session.Name)] = session
	return nil
}

func (f *FakeClient) ListSessions(_ context.Context, namespace string) (*RalphSessionList, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	var items []RalphSession
	for _, s := range f.Sessions {
		if namespace == "" || s.Namespace == namespace {
			items = append(items, *s)
		}
	}
	return &RalphSessionList{Items: items}, nil
}

func (f *FakeClient) GetPod(_ context.Context, namespace, name string) (*PodStatus, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Pods[fakeKey(namespace, name)], nil
}

func (f *FakeClient) CreatePod(_ context.Context, namespace string, pod *PodSpec) (string, error) {
	if f.Err != nil {
		return "", f.Err
	}
	f.CreatedPods = append(f.CreatedPods, pod)
	f.Pods[fakeKey(namespace, pod.Name)] = &PodStatus{
		Name:  pod.Name,
		Phase: "Pending",
	}
	return pod.Name, nil
}

func (f *FakeClient) DeletePod(_ context.Context, namespace, name string) error {
	if f.Err != nil {
		return f.Err
	}
	f.DeletedPods = append(f.DeletedPods, fakeKey(namespace, name))
	delete(f.Pods, fakeKey(namespace, name))
	return nil
}

func (f *FakeClient) AddFinalizer(_ context.Context, session *RalphSession, finalizer string) error {
	if f.Err != nil {
		return f.Err
	}
	if !hasFinalizer(session, finalizer) {
		session.Finalizers = append(session.Finalizers, finalizer)
	}
	return nil
}

func (f *FakeClient) RemoveFinalizer(_ context.Context, session *RalphSession, finalizer string) error {
	if f.Err != nil {
		return f.Err
	}
	var updated []string
	for _, f := range session.Finalizers {
		if f != finalizer {
			updated = append(updated, f)
		}
	}
	session.Finalizers = updated
	return nil
}

// -------------------------------------------------------------------
// Test helpers
// -------------------------------------------------------------------

func newTestSession(name, ns, provider, prompt string) *RalphSession {
	return &RalphSession{
		TypeMeta: TypeMeta{Kind: "RalphSession", APIVersion: GroupName + "/" + Version},
		ObjectMeta: ObjectMeta{
			Name:      name,
			Namespace: ns,
			UID:       "test-uid-" + name,
		},
		Spec: RalphSessionSpec{
			Provider: provider,
			Prompt:   prompt,
		},
	}
}

// -------------------------------------------------------------------
// Reconciler tests
// -------------------------------------------------------------------

func TestReconcile_SessionNotFound(t *testing.T) {
	fc := NewFakeClient()
	r := NewReconciler(fc, nil)

	result, err := r.Reconcile(context.Background(), ReconcileRequest{
		Namespace: "default",
		Name:      "nonexistent",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Requeue {
		t.Error("expected no requeue for missing session")
	}
}

func TestReconcile_AddsFinalizer(t *testing.T) {
	fc := NewFakeClient()
	session := newTestSession("test-1", "default", "claude", "hello world")
	fc.Sessions[fakeKey("default", "test-1")] = session

	r := NewReconciler(fc, nil)
	result, err := r.Reconcile(context.Background(), ReconcileRequest{
		Namespace: "default",
		Name:      "test-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Requeue {
		t.Error("expected requeue after adding finalizer")
	}
	if !hasFinalizer(session, FinalizerName) {
		t.Error("expected finalizer to be added")
	}
}

func TestReconcile_CreatesPod(t *testing.T) {
	fc := NewFakeClient()
	session := newTestSession("test-2", "default", "claude", "write tests")
	session.Finalizers = []string{FinalizerName}
	fc.Sessions[fakeKey("default", "test-2")] = session

	r := NewReconciler(fc, nil)
	result, err := r.Reconcile(context.Background(), ReconcileRequest{
		Namespace: "default",
		Name:      "test-2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Requeue {
		t.Error("expected requeue after pod creation")
	}
	if len(fc.CreatedPods) != 1 {
		t.Fatalf("expected 1 created pod, got %d", len(fc.CreatedPods))
	}
	if fc.CreatedPods[0].Name != "ralph-session-test-2" {
		t.Errorf("unexpected pod name: %s", fc.CreatedPods[0].Name)
	}
	if session.Status.Phase != "Launching" {
		t.Errorf("expected phase Launching, got %s", session.Status.Phase)
	}
}

func TestReconcile_SyncsRunningPod(t *testing.T) {
	fc := NewFakeClient()
	session := newTestSession("test-3", "default", "gemini", "analyze code")
	session.Finalizers = []string{FinalizerName}
	session.Status.Phase = "Launching"
	session.Status.PodName = "ralph-session-test-3"
	fc.Sessions[fakeKey("default", "test-3")] = session

	now := time.Now()
	fc.Pods[fakeKey("default", "ralph-session-test-3")] = &PodStatus{
		Name:      "ralph-session-test-3",
		Phase:     "Running",
		Ready:     true,
		StartTime: now,
	}

	r := NewReconciler(fc, nil)
	result, err := r.Reconcile(context.Background(), ReconcileRequest{
		Namespace: "default",
		Name:      "test-3",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Requeue {
		t.Error("expected requeue for running session")
	}
	if session.Status.Phase != "Running" {
		t.Errorf("expected phase Running, got %s", session.Status.Phase)
	}
}

func TestReconcile_HandlesCompletedPod(t *testing.T) {
	fc := NewFakeClient()
	session := newTestSession("test-4", "default", "codex", "refactor")
	session.Finalizers = []string{FinalizerName}
	session.Status.Phase = "Running"
	session.Status.PodName = "ralph-session-test-4"
	fc.Sessions[fakeKey("default", "test-4")] = session

	fc.Pods[fakeKey("default", "ralph-session-test-4")] = &PodStatus{
		Name:  "ralph-session-test-4",
		Phase: "Succeeded",
	}

	r := NewReconciler(fc, nil)
	result, err := r.Reconcile(context.Background(), ReconcileRequest{
		Namespace: "default",
		Name:      "test-4",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Requeue {
		t.Error("expected no requeue for completed session")
	}
	if session.Status.Phase != "Completed" {
		t.Errorf("expected phase Completed, got %s", session.Status.Phase)
	}
	if session.Status.CompletionTime == nil {
		t.Error("expected CompletionTime to be set")
	}
}

func TestReconcile_HandlesDeletion(t *testing.T) {
	fc := NewFakeClient()
	now := time.Now()
	session := newTestSession("test-5", "default", "claude", "cleanup")
	session.Finalizers = []string{FinalizerName}
	session.DeletionTimestamp = &now
	session.Status.PodName = "ralph-session-test-5"
	fc.Sessions[fakeKey("default", "test-5")] = session

	fc.Pods[fakeKey("default", "ralph-session-test-5")] = &PodStatus{
		Name:  "ralph-session-test-5",
		Phase: "Running",
	}

	r := NewReconciler(fc, nil)
	result, err := r.Reconcile(context.Background(), ReconcileRequest{
		Namespace: "default",
		Name:      "test-5",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Requeue {
		t.Error("expected no requeue after deletion cleanup")
	}
	if len(fc.DeletedPods) != 1 {
		t.Fatalf("expected 1 deleted pod, got %d", len(fc.DeletedPods))
	}
	if hasFinalizer(session, FinalizerName) {
		t.Error("expected finalizer to be removed")
	}
}

func TestReconcile_HandlesPodFailure(t *testing.T) {
	fc := NewFakeClient()
	session := newTestSession("test-6", "default", "claude", "fail test")
	session.Finalizers = []string{FinalizerName}
	session.Status.Phase = "Running"
	session.Status.PodName = "ralph-session-test-6"
	fc.Sessions[fakeKey("default", "test-6")] = session

	fc.Pods[fakeKey("default", "ralph-session-test-6")] = &PodStatus{
		Name:    "ralph-session-test-6",
		Phase:   "Failed",
		Message: "OOMKilled",
	}

	r := NewReconciler(fc, nil)
	_, err := r.Reconcile(context.Background(), ReconcileRequest{
		Namespace: "default",
		Name:      "test-6",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.Status.Phase != "Errored" {
		t.Errorf("expected phase Errored, got %s", session.Status.Phase)
	}
}

func TestReconcile_ClientError(t *testing.T) {
	fc := NewFakeClient()
	fc.Err = fmt.Errorf("connection refused")

	r := NewReconciler(fc, nil)
	_, err := r.Reconcile(context.Background(), ReconcileRequest{
		Namespace: "default",
		Name:      "test-err",
	})
	if err == nil {
		t.Fatal("expected error from client")
	}
}

// -------------------------------------------------------------------
// Pod template tests
// -------------------------------------------------------------------

func TestPodTemplateBuilder_Defaults(t *testing.T) {
	session := newTestSession("tmpl-1", "default", "claude", "build something")
	builder := NewPodTemplateBuilder(session)
	pod := builder.Build()

	if pod.Image != DefaultImage {
		t.Errorf("expected default image %s, got %s", DefaultImage, pod.Image)
	}
	if pod.Name != "ralph-session-tmpl-1" {
		t.Errorf("unexpected pod name: %s", pod.Name)
	}
	if pod.Labels["ralphglasses.studio/provider"] != "claude" {
		t.Errorf("missing provider label")
	}
	if pod.Resources == nil {
		t.Fatal("expected default resources")
	}
	if pod.Resources.Requests["cpu"] != "250m" {
		t.Errorf("unexpected CPU request: %s", pod.Resources.Requests["cpu"])
	}

	// Check workspace volume is always present.
	foundVol := false
	for _, v := range pod.Volumes {
		if v.Name == "workspace" && v.EmptyDir {
			foundVol = true
		}
	}
	if !foundVol {
		t.Error("expected workspace volume")
	}

	foundMount := false
	for _, m := range pod.VolumeMounts {
		if m.Name == "workspace" && m.MountPath == "/workspace" {
			foundMount = true
		}
	}
	if !foundMount {
		t.Error("expected workspace volume mount")
	}
}

func TestPodTemplateBuilder_CustomImage(t *testing.T) {
	session := newTestSession("tmpl-2", "default", "gemini", "custom image test")
	session.Spec.Image = "my-registry/ralph:v2"
	pod := NewPodTemplateBuilder(session).Build()

	if pod.Image != "my-registry/ralph:v2" {
		t.Errorf("expected custom image, got %s", pod.Image)
	}
}

func TestPodTemplateBuilder_ProviderCommands(t *testing.T) {
	tests := []struct {
		provider    string
		wantCommand string
	}{
		{"claude", "claude"},
		{"gemini", "gemini"},
		{"codex", "codex"},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			session := newTestSession("cmd-"+tt.provider, "default", tt.provider, "test")
			pod := NewPodTemplateBuilder(session).Build()
			if len(pod.Command) == 0 || pod.Command[0] != tt.wantCommand {
				t.Errorf("expected command %s, got %v", tt.wantCommand, pod.Command)
			}
		})
	}
}

func TestPodTemplateBuilder_EnvFromSecrets(t *testing.T) {
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
			session := newTestSession("env-"+tt.provider, "default", tt.provider, "test")
			pod := NewPodTemplateBuilder(session).Build()

			found := false
			for _, ef := range pod.EnvFrom {
				if ef.SecretRef != nil && ef.SecretRef.Name == tt.wantSecret {
					found = true
				}
			}
			if !found {
				t.Errorf("expected envFrom secret %s", tt.wantSecret)
			}
		})
	}
}

func TestPodTemplateBuilder_OwnerReference(t *testing.T) {
	session := newTestSession("owner-1", "default", "claude", "test")
	pod := NewPodTemplateBuilder(session).Build()

	if pod.OwnerRef == nil {
		t.Fatal("expected owner reference")
	}
	if pod.OwnerRef.Kind != "RalphSession" {
		t.Errorf("expected kind RalphSession, got %s", pod.OwnerRef.Kind)
	}
	if pod.OwnerRef.Name != "owner-1" {
		t.Errorf("expected owner name owner-1, got %s", pod.OwnerRef.Name)
	}
}

func TestPodTemplateBuilder_TeamLabel(t *testing.T) {
	session := newTestSession("team-1", "default", "claude", "test")
	session.Spec.TeamName = "alpha"
	pod := NewPodTemplateBuilder(session).Build()

	if pod.Labels["ralphglasses.studio/team"] != "alpha" {
		t.Error("expected team label")
	}
}

func TestPodTemplateBuilder_BudgetAnnotation(t *testing.T) {
	session := newTestSession("budget-1", "default", "claude", "test")
	session.Spec.BudgetUSD = 5.50
	pod := NewPodTemplateBuilder(session).Build()

	if pod.Annotations["ralphglasses.studio/budget-usd"] != "5.50" {
		t.Errorf("expected budget annotation 5.50, got %s", pod.Annotations["ralphglasses.studio/budget-usd"])
	}
}

// -------------------------------------------------------------------
// CRD validation tests
// -------------------------------------------------------------------

func TestRalphSessionSpec_Validate(t *testing.T) {
	tests := []struct {
		name     string
		spec     RalphSessionSpec
		wantErrs int
	}{
		{
			name:     "valid minimal",
			spec:     RalphSessionSpec{Provider: "claude", Prompt: "hello"},
			wantErrs: 0,
		},
		{
			name:     "missing provider",
			spec:     RalphSessionSpec{Prompt: "hello"},
			wantErrs: 1,
		},
		{
			name:     "invalid provider",
			spec:     RalphSessionSpec{Provider: "gpt4", Prompt: "hello"},
			wantErrs: 1,
		},
		{
			name:     "missing prompt",
			spec:     RalphSessionSpec{Provider: "claude"},
			wantErrs: 1,
		},
		{
			name:     "negative budget",
			spec:     RalphSessionSpec{Provider: "claude", Prompt: "hi", BudgetUSD: -1},
			wantErrs: 1,
		},
		{
			name:     "negative max turns",
			spec:     RalphSessionSpec{Provider: "gemini", Prompt: "hi", MaxTurns: -5},
			wantErrs: 1,
		},
		{
			name:     "multiple errors",
			spec:     RalphSessionSpec{},
			wantErrs: 2, // missing provider + missing prompt
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

func TestRalphFleetSpec_Validate(t *testing.T) {
	tests := []struct {
		name     string
		spec     RalphFleetSpec
		wantErrs int
	}{
		{
			name: "valid",
			spec: RalphFleetSpec{
				Replicas:        3,
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
			name: "invalid template",
			spec: RalphFleetSpec{
				Replicas:        1,
				SessionTemplate: RalphSessionSpec{Provider: "invalid"},
			},
			wantErrs: 2, // invalid provider + missing prompt
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
// RBAC tests
// -------------------------------------------------------------------

func TestOperatorClusterRole(t *testing.T) {
	cr := OperatorClusterRole()
	if cr.Kind != "ClusterRole" {
		t.Errorf("expected kind ClusterRole, got %s", cr.Kind)
	}
	if cr.Name != "ralphglasses-operator" {
		t.Errorf("expected name ralphglasses-operator, got %s", cr.Name)
	}
	if len(cr.Rules) == 0 {
		t.Error("expected at least one policy rule")
	}

	// Verify CRD rule exists.
	found := false
	for _, rule := range cr.Rules {
		for _, group := range rule.APIGroups {
			if group == GroupName {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected rule for API group %s", GroupName)
	}
}

func TestAllRBACManifests(t *testing.T) {
	manifests := AllRBACManifests("operator-sa", "session-sa", "ralph-system")
	if len(manifests) != 4 {
		t.Errorf("expected 4 RBAC manifests, got %d", len(manifests))
	}
}

// -------------------------------------------------------------------
// Condition helper tests
// -------------------------------------------------------------------

func TestSetCondition(t *testing.T) {
	var conditions []Condition

	// Add new condition.
	changed := setCondition(&conditions, Condition{
		Type:   "Ready",
		Status: "False",
		Reason: "Initializing",
	})
	if !changed {
		t.Error("expected changed=true for new condition")
	}
	if len(conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(conditions))
	}

	// Update existing condition.
	changed = setCondition(&conditions, Condition{
		Type:   "Ready",
		Status: "True",
		Reason: "PodRunning",
	})
	if !changed {
		t.Error("expected changed=true for updated condition")
	}
	if len(conditions) != 1 {
		t.Fatalf("expected 1 condition after update, got %d", len(conditions))
	}
	if conditions[0].Status != "True" {
		t.Errorf("expected status True, got %s", conditions[0].Status)
	}

	// No-op update.
	changed = setCondition(&conditions, Condition{
		Type:   "Ready",
		Status: "True",
		Reason: "PodRunning",
	})
	if changed {
		t.Error("expected changed=false for identical condition")
	}
}
