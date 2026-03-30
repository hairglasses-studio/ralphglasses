package k8s

// RBAC types and helpers for generating ClusterRole, Role, and
// RoleBinding manifests needed by the ralphglasses operator.
// These are plain structs that serialize to valid Kubernetes RBAC
// objects without importing k8s.io/api.

// -------------------------------------------------------------------
// Types
// -------------------------------------------------------------------

// ClusterRole is a cluster-wide set of permissions.
type ClusterRole struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata,omitempty"`
	Rules      []PolicyRule `json:"rules"`
}

// Role is a namespaced set of permissions.
type Role struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata,omitempty"`
	Rules      []PolicyRule `json:"rules"`
}

// PolicyRule holds information that describes a policy rule.
type PolicyRule struct {
	APIGroups []string `json:"apiGroups"`
	Resources []string `json:"resources"`
	Verbs     []string `json:"verbs"`
}

// ClusterRoleBinding binds a ClusterRole to subjects.
type ClusterRoleBinding struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata,omitempty"`
	RoleRef    RoleRef   `json:"roleRef"`
	Subjects   []Subject `json:"subjects"`
}

// RoleBinding binds a Role to subjects within a namespace.
type RoleBinding struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata,omitempty"`
	RoleRef    RoleRef   `json:"roleRef"`
	Subjects   []Subject `json:"subjects"`
}

// RoleRef identifies the role to bind.
type RoleRef struct {
	APIGroup string `json:"apiGroup"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
}

// Subject identifies a user, group, or service account.
type Subject struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// -------------------------------------------------------------------
// Generators
// -------------------------------------------------------------------

// OperatorClusterRole returns the ClusterRole needed by the ralphglasses
// operator to manage sessions, fleets, pods, and events.
func OperatorClusterRole() *ClusterRole {
	return &ClusterRole{
		TypeMeta: TypeMeta{
			Kind:       "ClusterRole",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: ObjectMeta{
			Name: "ralphglasses-operator",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "ralphglasses",
				"app.kubernetes.io/component":  "operator",
				"app.kubernetes.io/managed-by": "ralphglasses-operator",
			},
		},
		Rules: []PolicyRule{
			// CRD management.
			{
				APIGroups: []string{GroupName},
				Resources: []string{"ralphsessions", "ralphsessions/status", "ralphfleets", "ralphfleets/status"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
			// Pod lifecycle.
			{
				APIGroups: []string{""},
				Resources: []string{"pods", "pods/log"},
				Verbs:     []string{"get", "list", "watch", "create", "delete"},
			},
			// Secrets (read-only, for API key verification).
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"get", "list", "watch"},
			},
			// ConfigMaps (for operator config).
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"get", "list", "watch", "create", "update"},
			},
			// Events (status reporting).
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"create", "patch"},
			},
			// Coordination (leader election).
			{
				APIGroups: []string{"coordination.k8s.io"},
				Resources: []string{"leases"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
		},
	}
}

// OperatorClusterRoleBinding returns a ClusterRoleBinding that grants
// the operator ClusterRole to the specified service account.
func OperatorClusterRoleBinding(serviceAccountName, namespace string) *ClusterRoleBinding {
	return &ClusterRoleBinding{
		TypeMeta: TypeMeta{
			Kind:       "ClusterRoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: ObjectMeta{
			Name: "ralphglasses-operator",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "ralphglasses",
				"app.kubernetes.io/component":  "operator",
				"app.kubernetes.io/managed-by": "ralphglasses-operator",
			},
		},
		RoleRef: RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "ralphglasses-operator",
		},
		Subjects: []Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccountName,
				Namespace: namespace,
			},
		},
	}
}

// SessionRole returns a namespaced Role with minimal permissions for
// a session pod — only enough to read its own configmaps and secrets.
func SessionRole(namespace string) *Role {
	return &Role{
		TypeMeta: TypeMeta{
			Kind:       "Role",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: ObjectMeta{
			Name:      "ralphglasses-session",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "ralphglasses",
				"app.kubernetes.io/component":  "session",
				"app.kubernetes.io/managed-by": "ralphglasses-operator",
			},
		},
		Rules: []PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"get"},
			},
		},
	}
}

// SessionRoleBinding returns a RoleBinding that grants the session Role
// to the specified service account.
func SessionRoleBinding(serviceAccountName, namespace string) *RoleBinding {
	return &RoleBinding{
		TypeMeta: TypeMeta{
			Kind:       "RoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: ObjectMeta{
			Name:      "ralphglasses-session",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "ralphglasses",
				"app.kubernetes.io/component":  "session",
				"app.kubernetes.io/managed-by": "ralphglasses-operator",
			},
		},
		RoleRef: RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "ralphglasses-session",
		},
		Subjects: []Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccountName,
				Namespace: namespace,
			},
		},
	}
}

// AllRBACManifests returns the complete set of RBAC objects needed for
// a standard ralphglasses operator deployment.
func AllRBACManifests(operatorSA, sessionSA, namespace string) []interface{} {
	return []interface{}{
		OperatorClusterRole(),
		OperatorClusterRoleBinding(operatorSA, namespace),
		SessionRole(namespace),
		SessionRoleBinding(sessionSA, namespace),
	}
}
