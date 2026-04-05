package k8s

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"
)

// -------------------------------------------------------------------
// Client interface — abstracts Kubernetes API calls so tests can
// substitute a fake implementation without a real cluster.
// -------------------------------------------------------------------

// Client provides CRUD operations for Kubernetes resources.
// Production implementations wrap client-go or controller-runtime;
// tests use FakeClient (see controller_test.go).
type Client interface {
	// GetSession retrieves a RalphSession by namespace and name.
	GetSession(ctx context.Context, namespace, name string) (*RalphSession, error)

	// UpdateSessionStatus persists the status subresource of a RalphSession.
	UpdateSessionStatus(ctx context.Context, session *RalphSession) error

	// ListSessions returns all RalphSession resources in a namespace.
	// If namespace is empty, returns across all namespaces.
	ListSessions(ctx context.Context, namespace string) (*RalphSessionList, error)

	// GetPod retrieves a pod by namespace and name. Returns nil, nil if not found.
	GetPod(ctx context.Context, namespace, name string) (*PodStatus, error)

	// CreatePod creates a pod from the given spec. Returns the created pod name.
	CreatePod(ctx context.Context, namespace string, pod *PodSpec) (string, error)

	// DeletePod deletes a pod by namespace and name.
	DeletePod(ctx context.Context, namespace, name string) error

	// AddFinalizer adds a finalizer string to the object's metadata.
	AddFinalizer(ctx context.Context, session *RalphSession, finalizer string) error

	// RemoveFinalizer removes a finalizer string from the object's metadata.
	RemoveFinalizer(ctx context.Context, session *RalphSession, finalizer string) error
}

// PodStatus is a minimal representation of a running pod's state.
type PodStatus struct {
	Name      string    `json:"name"`
	Phase     string    `json:"phase"` // Pending, Running, Succeeded, Failed, Unknown
	Ready     bool      `json:"ready"`
	StartTime time.Time `json:"startTime,omitempty"`
	Message   string    `json:"message,omitempty"`
}

// PodSpec is a minimal pod specification used by CreatePod.
// See pod_template.go for the builder that produces these.
type PodSpec struct {
	Name         string                `json:"name"`
	Namespace    string                `json:"namespace"`
	Labels       map[string]string     `json:"labels,omitempty"`
	Annotations  map[string]string     `json:"annotations,omitempty"`
	Image        string                `json:"image"`
	Command      []string              `json:"command,omitempty"`
	Args         []string              `json:"args,omitempty"`
	Env          []EnvVar              `json:"env,omitempty"`
	EnvFrom      []EnvFromSource       `json:"envFrom,omitempty"`
	Resources    *ResourceRequirements `json:"resources,omitempty"`
	VolumeMounts []VolumeMount         `json:"volumeMounts,omitempty"`
	Volumes      []Volume              `json:"volumes,omitempty"`
	OwnerRef     *OwnerReference       `json:"ownerRef,omitempty"`
}

// EnvVar represents a single environment variable.
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`

	// SecretKeyRef populates the value from a secret key.
	SecretKeyRef *SecretKeySelector `json:"secretKeyRef,omitempty"`
}

// SecretKeySelector selects a key of a Secret.
type SecretKeySelector struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// OwnerReference contains enough information to let you identify an owning object.
type OwnerReference struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	UID        string `json:"uid"`
}

// -------------------------------------------------------------------
// Reconciler
// -------------------------------------------------------------------

const (
	// FinalizerName is the finalizer added to RalphSession resources
	// to ensure pod cleanup before deletion.
	FinalizerName = "ralphglasses.hairglasses.studio/session-cleanup"

	// DefaultImage is the container image used when spec.image is empty.
	DefaultImage = "ghcr.io/hairglasses-studio/ralphglasses:latest"
)

// Reconciler watches RalphSession custom resources and manages pod
// lifecycle accordingly. It implements a level-triggered reconciliation
// loop: on each call to Reconcile it compares desired state (the CRD spec)
// with observed state (the pod) and takes corrective action.
type Reconciler struct {
	client Client
	logger *slog.Logger
}

// NewReconciler creates a Reconciler that operates through the given Client.
func NewReconciler(client Client, logger *slog.Logger) *Reconciler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Reconciler{client: client, logger: logger}
}

// ReconcileRequest identifies a single object to reconcile.
type ReconcileRequest struct {
	Namespace string
	Name      string
}

// ReconcileResult tells the caller whether and when to requeue.
type ReconcileResult struct {
	Requeue      bool
	RequeueAfter time.Duration
}

// Reconcile performs a single reconciliation pass for the named RalphSession.
// It is designed to be called repeatedly by an external watch/informer loop.
func (r *Reconciler) Reconcile(ctx context.Context, req ReconcileRequest) (ReconcileResult, error) {
	log := r.logger.With("namespace", req.Namespace, "name", req.Name)

	session, err := r.client.GetSession(ctx, req.Namespace, req.Name)
	if err != nil {
		return ReconcileResult{}, fmt.Errorf("get session: %w", err)
	}
	if session == nil {
		// Object deleted before we could reconcile — nothing to do.
		log.Info("session not found, skipping")
		return ReconcileResult{}, nil
	}

	// Handle deletion with finalizer cleanup.
	if session.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, session, log)
	}

	// Ensure our finalizer is present.
	if !hasFinalizer(session, FinalizerName) {
		if err := r.client.AddFinalizer(ctx, session, FinalizerName); err != nil {
			return ReconcileResult{}, fmt.Errorf("add finalizer: %w", err)
		}
		return ReconcileResult{Requeue: true}, nil
	}

	// Check if a pod already exists for this session.
	podName := podNameForSession(session)
	pod, err := r.client.GetPod(ctx, session.Namespace, podName)
	if err != nil {
		return ReconcileResult{}, fmt.Errorf("get pod: %w", err)
	}

	if pod == nil {
		// No pod yet — create one.
		return r.createPod(ctx, session, log)
	}

	// Pod exists — sync status back to the CRD.
	return r.syncStatus(ctx, session, pod, log)
}

// handleDeletion cleans up the managed pod and removes the finalizer.
func (r *Reconciler) handleDeletion(ctx context.Context, session *RalphSession, log *slog.Logger) (ReconcileResult, error) {
	if !hasFinalizer(session, FinalizerName) {
		return ReconcileResult{}, nil
	}

	podName := podNameForSession(session)
	log.Info("cleaning up pod for deleted session", "pod", podName)

	if err := r.client.DeletePod(ctx, session.Namespace, podName); err != nil {
		return ReconcileResult{}, fmt.Errorf("delete pod: %w", err)
	}

	if err := r.client.RemoveFinalizer(ctx, session, FinalizerName); err != nil {
		return ReconcileResult{}, fmt.Errorf("remove finalizer: %w", err)
	}

	return ReconcileResult{}, nil
}

// createPod builds and creates a pod for the given session.
func (r *Reconciler) createPod(ctx context.Context, session *RalphSession, log *slog.Logger) (ReconcileResult, error) {
	builder := NewPodTemplateBuilder(session)
	spec := builder.Build()

	podName, err := r.client.CreatePod(ctx, session.Namespace, spec)
	if err != nil {
		// Update status to reflect the error.
		session.Status.Phase = "Errored"
		setCondition(&session.Status.Conditions, Condition{
			Type:               "PodReady",
			Status:             "False",
			Reason:             "CreateFailed",
			Message:            err.Error(),
			LastTransitionTime: time.Now(),
		})
		_ = r.client.UpdateSessionStatus(ctx, session)
		return ReconcileResult{}, fmt.Errorf("create pod: %w", err)
	}

	log.Info("created pod", "pod", podName)

	session.Status.Phase = "Launching"
	session.Status.PodName = podName
	setCondition(&session.Status.Conditions, Condition{
		Type:               "PodReady",
		Status:             "False",
		Reason:             "PodCreated",
		Message:            "Pod created, waiting for readiness",
		LastTransitionTime: time.Now(),
	})

	if err := r.client.UpdateSessionStatus(ctx, session); err != nil {
		return ReconcileResult{}, fmt.Errorf("update status: %w", err)
	}

	// Requeue to check pod readiness.
	return ReconcileResult{Requeue: true, RequeueAfter: 5 * time.Second}, nil
}

// syncStatus updates the RalphSession status based on the pod's current state.
func (r *Reconciler) syncStatus(ctx context.Context, session *RalphSession, pod *PodStatus, log *slog.Logger) (ReconcileResult, error) {
	var changed bool

	switch pod.Phase {
	case "Running":
		if session.Status.Phase != "Running" {
			session.Status.Phase = "Running"
			if pod.StartTime.IsZero() {
				now := time.Now()
				session.Status.StartTime = &now
			} else {
				session.Status.StartTime = &pod.StartTime
			}
			changed = true
		}
		if pod.Ready {
			changed = setCondition(&session.Status.Conditions, Condition{
				Type:               "PodReady",
				Status:             "True",
				Reason:             "PodRunning",
				Message:            "Pod is running and ready",
				LastTransitionTime: time.Now(),
			}) || changed
		}
	case "Succeeded":
		if session.Status.Phase != "Completed" {
			session.Status.Phase = "Completed"
			now := time.Now()
			session.Status.CompletionTime = &now
			changed = true
		}
	case "Failed":
		if session.Status.Phase != "Errored" {
			session.Status.Phase = "Errored"
			now := time.Now()
			session.Status.CompletionTime = &now
			setCondition(&session.Status.Conditions, Condition{
				Type:               "PodReady",
				Status:             "False",
				Reason:             "PodFailed",
				Message:            pod.Message,
				LastTransitionTime: time.Now(),
			})
			changed = true
		}
	case "Pending":
		// Still launching — requeue.
		return ReconcileResult{Requeue: true, RequeueAfter: 5 * time.Second}, nil
	default:
		log.Warn("unknown pod phase", "phase", pod.Phase)
	}

	if changed {
		if err := r.client.UpdateSessionStatus(ctx, session); err != nil {
			return ReconcileResult{}, fmt.Errorf("update status: %w", err)
		}
	}

	// If the session is still active, requeue for periodic health checks.
	if session.Status.Phase == "Running" || session.Status.Phase == "Launching" {
		return ReconcileResult{Requeue: true, RequeueAfter: 30 * time.Second}, nil
	}

	return ReconcileResult{}, nil
}

// -------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------

// podNameForSession deterministically derives a pod name from the session.
func podNameForSession(session *RalphSession) string {
	return fmt.Sprintf("ralph-session-%s", session.Name)
}

// hasFinalizer returns true if the session's metadata includes the given finalizer.
func hasFinalizer(session *RalphSession, finalizer string) bool {
	return slices.Contains(session.Finalizers, finalizer)
}

// setCondition updates or appends a condition. Returns true if the condition changed.
func setCondition(conditions *[]Condition, cond Condition) bool {
	for i, existing := range *conditions {
		if existing.Type == cond.Type {
			if existing.Status == cond.Status && existing.Reason == cond.Reason {
				return false
			}
			(*conditions)[i] = cond
			return true
		}
	}
	*conditions = append(*conditions, cond)
	return true
}
