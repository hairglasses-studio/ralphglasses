// Package k8s provides Kubernetes custom resource definitions and operator
// logic for deploying ralphglasses sessions and fleets as cloud-native
// workloads. Types mirror the session.Session and fleet models but are
// expressed as Kubernetes-native CRDs with kubebuilder annotations.
//
// This package intentionally avoids pulling in controller-runtime or
// client-go as hard dependencies. All Kubernetes object types are defined
// as plain structs with JSON tags that are wire-compatible with the
// Kubernetes API. A thin interface layer (see controller.go) allows
// production code to use a real client while tests use fakes.
package k8s

import "time"

// -------------------------------------------------------------------
// Shared constants
// -------------------------------------------------------------------

const (
	// GroupName is the API group for ralphglasses custom resources.
	GroupName = "ralphglasses.hairglasses.studio"

	// Version is the current CRD API version.
	Version = "v1alpha1"
)

// -------------------------------------------------------------------
// Kubernetes meta types (minimal subset to avoid k8s.io deps)
// -------------------------------------------------------------------

// TypeMeta describes an individual object in an API response or request
// with strings representing the type of the object and its API schema version.
type TypeMeta struct {
	Kind       string `json:"kind,omitempty"`
	APIVersion string `json:"apiVersion,omitempty"`
}

// ObjectMeta is metadata that all persisted resources must have.
type ObjectMeta struct {
	Name              string            `json:"name,omitempty"`
	Namespace         string            `json:"namespace,omitempty"`
	UID               string            `json:"uid,omitempty"`
	ResourceVersion   string            `json:"resourceVersion,omitempty"`
	Generation        int64             `json:"generation,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	CreationTimestamp time.Time         `json:"creationTimestamp,omitempty"`
	DeletionTimestamp *time.Time        `json:"deletionTimestamp,omitempty"`
	Finalizers        []string          `json:"finalizers,omitempty"`
}

// Condition describes the state of a resource at a certain point.
type Condition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"` // "True", "False", "Unknown"
	LastTransitionTime time.Time `json:"lastTransitionTime,omitempty"`
	Reason             string    `json:"reason,omitempty"`
	Message            string    `json:"message,omitempty"`
}

// -------------------------------------------------------------------
// RalphSession CRD
// -------------------------------------------------------------------

// RalphSession is the Schema for the ralphsessions API.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Provider",type=string,JSONPath=`.spec.provider`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=rs
type RalphSession struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata,omitempty"`

	Spec   RalphSessionSpec   `json:"spec,omitempty"`
	Status RalphSessionStatus `json:"status,omitempty"`
}

// RalphSessionSpec defines the desired state of a RalphSession.
type RalphSessionSpec struct {
	// Provider selects the LLM backend: "claude", "gemini", or "codex".
	// +kubebuilder:validation:Enum=claude;gemini;codex
	// +kubebuilder:default=codex
	Provider string `json:"provider"`

	// Model is the specific model identifier (e.g. "claude-opus-4-20250514").
	// +optional
	Model string `json:"model,omitempty"`

	// Prompt is the initial instruction sent to the LLM session.
	// +kubebuilder:validation:MinLength=1
	Prompt string `json:"prompt"`

	// RepoPath is the working directory inside the pod.
	// +optional
	RepoPath string `json:"repoPath,omitempty"`

	// MaxTurns limits the conversation turn count. Zero means unlimited.
	// +kubebuilder:validation:Minimum=0
	// +optional
	MaxTurns int `json:"maxTurns,omitempty"`

	// BudgetUSD sets the maximum spend for this session.
	// +optional
	BudgetUSD float64 `json:"budgetUSD,omitempty"`

	// EnhancePrompt enables the prompt enhancement pipeline before execution.
	// +optional
	EnhancePrompt bool `json:"enhancePrompt,omitempty"`

	// Image overrides the default container image for the session pod.
	// +optional
	Image string `json:"image,omitempty"`

	// Resources defines CPU/memory requests and limits for the session pod.
	// +optional
	Resources *ResourceRequirements `json:"resources,omitempty"`

	// EnvFrom lists secret or configmap references to inject as environment variables.
	// +optional
	EnvFrom []EnvFromSource `json:"envFrom,omitempty"`

	// VolumeMounts defines additional volume mounts for the session container.
	// +optional
	VolumeMounts []VolumeMount `json:"volumeMounts,omitempty"`

	// Volumes defines additional volumes available to the session pod.
	// +optional
	Volumes []Volume `json:"volumes,omitempty"`

	// TeamName associates this session with a named team for fleet coordination.
	// +optional
	TeamName string `json:"teamName,omitempty"`
}

// RalphSessionStatus defines the observed state of a RalphSession.
type RalphSessionStatus struct {
	// Phase is the high-level lifecycle phase.
	// +kubebuilder:validation:Enum=Pending;Launching;Running;Completed;Errored;Stopped
	Phase string `json:"phase,omitempty"`

	// PodName is the name of the managed pod running this session.
	PodName string `json:"podName,omitempty"`

	// SpentUSD tracks cumulative cost.
	SpentUSD float64 `json:"spentUSD,omitempty"`

	// TurnCount tracks conversation turns completed.
	TurnCount int `json:"turnCount,omitempty"`

	// LastOutput holds the most recent output line from the session.
	LastOutput string `json:"lastOutput,omitempty"`

	// StartTime records when the session pod started.
	StartTime *time.Time `json:"startTime,omitempty"`

	// CompletionTime records when the session finished.
	CompletionTime *time.Time `json:"completionTime,omitempty"`

	// Conditions represent the latest available observations.
	Conditions []Condition `json:"conditions,omitempty"`
}

// -------------------------------------------------------------------
// RalphFleet CRD
// -------------------------------------------------------------------

// RalphFleet is the Schema for the ralphfleets API. A fleet manages a
// set of RalphSession resources as a coordinated group with shared
// budget, routing, and scaling policies.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readySessions`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:resource:shortName=rf
type RalphFleet struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata,omitempty"`

	Spec   RalphFleetSpec   `json:"spec,omitempty"`
	Status RalphFleetStatus `json:"status,omitempty"`
}

// RalphFleetSpec defines the desired state of a RalphFleet.
type RalphFleetSpec struct {
	// Replicas is the desired number of concurrent sessions.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	Replicas int `json:"replicas"`

	// SessionTemplate defines the spec applied to each session in the fleet.
	SessionTemplate RalphSessionSpec `json:"sessionTemplate"`

	// BudgetUSD is the total budget shared across all sessions in the fleet.
	// +optional
	BudgetUSD float64 `json:"budgetUSD,omitempty"`

	// MaxConcurrent limits how many sessions may run simultaneously.
	// Zero means all replicas run at once.
	// +optional
	MaxConcurrent int `json:"maxConcurrent,omitempty"`

	// RoutingStrategy selects how work is distributed: "round-robin", "least-cost", "capability".
	// +kubebuilder:validation:Enum=round-robin;least-cost;capability
	// +kubebuilder:default=round-robin
	// +optional
	RoutingStrategy string `json:"routingStrategy,omitempty"`
}

// RalphFleetStatus defines the observed state of a RalphFleet.
type RalphFleetStatus struct {
	// ReadySessions is the count of sessions in Running phase.
	ReadySessions int `json:"readySessions,omitempty"`

	// TotalSessions is the count of all managed sessions.
	TotalSessions int `json:"totalSessions,omitempty"`

	// TotalSpentUSD is the aggregate cost across all sessions.
	TotalSpentUSD float64 `json:"totalSpentUSD,omitempty"`

	// Conditions represent the latest available observations.
	Conditions []Condition `json:"conditions,omitempty"`
}

// -------------------------------------------------------------------
// List types (required for Kubernetes API machinery)
// -------------------------------------------------------------------

// RalphSessionList contains a list of RalphSession resources.
// +kubebuilder:object:root=true
type RalphSessionList struct {
	TypeMeta `json:",inline"`
	ListMeta `json:"metadata,omitempty"`
	Items    []RalphSession `json:"items"`
}

// RalphFleetList contains a list of RalphFleet resources.
// +kubebuilder:object:root=true
type RalphFleetList struct {
	TypeMeta `json:",inline"`
	ListMeta `json:"metadata,omitempty"`
	Items    []RalphFleet `json:"items"`
}

// ListMeta describes metadata for list responses.
type ListMeta struct {
	ResourceVersion string `json:"resourceVersion,omitempty"`
	Continue        string `json:"continue,omitempty"`
}

// -------------------------------------------------------------------
// Embedded resource types (k8s-compatible, dep-free)
// -------------------------------------------------------------------

// ResourceRequirements describes compute resource constraints.
type ResourceRequirements struct {
	Requests ResourceList `json:"requests,omitempty"`
	Limits   ResourceList `json:"limits,omitempty"`
}

// ResourceList is a map of resource name to quantity string (e.g. "500m", "128Mi").
type ResourceList map[string]string

// EnvFromSource selects a secret or configmap to inject as environment variables.
type EnvFromSource struct {
	// SecretRef names a Secret whose data entries become env vars.
	// +optional
	SecretRef *SecretReference `json:"secretRef,omitempty"`

	// ConfigMapRef names a ConfigMap whose data entries become env vars.
	// +optional
	ConfigMapRef *ConfigMapReference `json:"configMapRef,omitempty"`
}

// SecretReference holds a reference to a Secret by name.
type SecretReference struct {
	Name string `json:"name"`
}

// ConfigMapReference holds a reference to a ConfigMap by name.
type ConfigMapReference struct {
	Name string `json:"name"`
}

// VolumeMount describes a volume mount inside a container.
type VolumeMount struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly,omitempty"`
	SubPath   string `json:"subPath,omitempty"`
}

// Volume describes a named volume that can be mounted by containers.
type Volume struct {
	Name string `json:"name"`

	// SecretName populates the volume from a Kubernetes Secret.
	// +optional
	SecretName string `json:"secretName,omitempty"`

	// ConfigMapName populates the volume from a Kubernetes ConfigMap.
	// +optional
	ConfigMapName string `json:"configMapName,omitempty"`

	// EmptyDir uses an ephemeral empty directory.
	// +optional
	EmptyDir bool `json:"emptyDir,omitempty"`
}

// -------------------------------------------------------------------
// Validation helpers
// -------------------------------------------------------------------

// ValidProviders lists the accepted provider values.
var ValidProviders = map[string]bool{
	"claude": true,
	"gemini": true,
	"codex":  true,
}

// Validate checks that required fields are present and values are within bounds.
func (s *RalphSessionSpec) Validate() []string {
	var errs []string
	if s.Provider == "" {
		errs = append(errs, "spec.provider is required")
	} else if !ValidProviders[s.Provider] {
		errs = append(errs, "spec.provider must be one of: claude, gemini, codex")
	}
	if s.Prompt == "" {
		errs = append(errs, "spec.prompt is required")
	}
	if s.MaxTurns < 0 {
		errs = append(errs, "spec.maxTurns must be >= 0")
	}
	if s.BudgetUSD < 0 {
		errs = append(errs, "spec.budgetUSD must be >= 0")
	}
	return errs
}

// Validate checks that required fields are present and values are within bounds.
func (s *RalphFleetSpec) Validate() []string {
	var errs []string
	if s.Replicas < 0 {
		errs = append(errs, "spec.replicas must be >= 0")
	}
	if s.BudgetUSD < 0 {
		errs = append(errs, "spec.budgetUSD must be >= 0")
	}
	templateErrs := s.SessionTemplate.Validate()
	for _, e := range templateErrs {
		errs = append(errs, "spec.sessionTemplate."+e)
	}
	return errs
}
