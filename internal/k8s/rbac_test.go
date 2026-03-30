package k8s

import (
	"encoding/json"
	"testing"
)

// -------------------------------------------------------------------
// OperatorClusterRole
// -------------------------------------------------------------------

func TestOperatorClusterRole_TypeMeta(t *testing.T) {
	cr := OperatorClusterRole()
	if cr.Kind != "ClusterRole" {
		t.Errorf("kind: got %s", cr.Kind)
	}
	if cr.APIVersion != "rbac.authorization.k8s.io/v1" {
		t.Errorf("apiVersion: got %s", cr.APIVersion)
	}
}

func TestOperatorClusterRole_Name(t *testing.T) {
	cr := OperatorClusterRole()
	if cr.Name != "ralphglasses-operator" {
		t.Errorf("name: got %s", cr.Name)
	}
}

func TestOperatorClusterRole_Labels(t *testing.T) {
	cr := OperatorClusterRole()
	expected := map[string]string{
		"app.kubernetes.io/name":       "ralphglasses",
		"app.kubernetes.io/component":  "operator",
		"app.kubernetes.io/managed-by": "ralphglasses-operator",
	}
	for k, v := range expected {
		if cr.Labels[k] != v {
			t.Errorf("label %s: expected %q, got %q", k, v, cr.Labels[k])
		}
	}
}

func TestOperatorClusterRole_Rules(t *testing.T) {
	cr := OperatorClusterRole()
	if len(cr.Rules) == 0 {
		t.Fatal("expected policy rules")
	}

	// Verify specific rule categories exist.
	tests := []struct {
		name      string
		apiGroup  string
		resources []string
	}{
		{"CRD management", GroupName, []string{"ralphsessions", "ralphfleets"}},
		{"pods", "", []string{"pods"}},
		{"secrets", "", []string{"secrets"}},
		{"configmaps", "", []string{"configmaps"}},
		{"events", "", []string{"events"}},
		{"leases", "coordination.k8s.io", []string{"leases"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := false
			for _, rule := range cr.Rules {
				for _, g := range rule.APIGroups {
					if g == tt.apiGroup {
						for _, r := range rule.Resources {
							for _, want := range tt.resources {
								if r == want {
									found = true
								}
							}
						}
					}
				}
			}
			if !found {
				t.Errorf("missing rule for apiGroup=%q resources=%v", tt.apiGroup, tt.resources)
			}
		})
	}
}

func TestOperatorClusterRole_CRDRuleVerbs(t *testing.T) {
	cr := OperatorClusterRole()

	// Find the CRD rule.
	for _, rule := range cr.Rules {
		for _, g := range rule.APIGroups {
			if g == GroupName {
				expectedVerbs := map[string]bool{
					"get": true, "list": true, "watch": true,
					"create": true, "update": true, "patch": true, "delete": true,
				}
				for _, v := range rule.Verbs {
					delete(expectedVerbs, v)
				}
				if len(expectedVerbs) > 0 {
					t.Errorf("CRD rule missing verbs: %v", expectedVerbs)
				}
				return
			}
		}
	}
	t.Fatal("CRD rule not found")
}

func TestOperatorClusterRole_SecretsReadOnly(t *testing.T) {
	cr := OperatorClusterRole()

	for _, rule := range cr.Rules {
		hasSecrets := false
		for _, r := range rule.Resources {
			if r == "secrets" {
				hasSecrets = true
				break
			}
		}
		if !hasSecrets {
			continue
		}

		// Secrets rule should only have read verbs.
		for _, v := range rule.Verbs {
			if v == "create" || v == "update" || v == "patch" || v == "delete" {
				t.Errorf("secrets rule should be read-only, but has verb %q", v)
			}
		}
	}
}

// -------------------------------------------------------------------
// OperatorClusterRoleBinding
// -------------------------------------------------------------------

func TestOperatorClusterRoleBinding_TypeMeta(t *testing.T) {
	crb := OperatorClusterRoleBinding("operator-sa", "ralph-system")
	if crb.Kind != "ClusterRoleBinding" {
		t.Errorf("kind: got %s", crb.Kind)
	}
	if crb.APIVersion != "rbac.authorization.k8s.io/v1" {
		t.Errorf("apiVersion: got %s", crb.APIVersion)
	}
}

func TestOperatorClusterRoleBinding_RoleRef(t *testing.T) {
	crb := OperatorClusterRoleBinding("operator-sa", "ralph-system")
	if crb.RoleRef.APIGroup != "rbac.authorization.k8s.io" {
		t.Errorf("roleRef apiGroup: got %s", crb.RoleRef.APIGroup)
	}
	if crb.RoleRef.Kind != "ClusterRole" {
		t.Errorf("roleRef kind: got %s", crb.RoleRef.Kind)
	}
	if crb.RoleRef.Name != "ralphglasses-operator" {
		t.Errorf("roleRef name: got %s", crb.RoleRef.Name)
	}
}

func TestOperatorClusterRoleBinding_Subject(t *testing.T) {
	crb := OperatorClusterRoleBinding("my-operator-sa", "kube-system")
	if len(crb.Subjects) != 1 {
		t.Fatalf("expected 1 subject, got %d", len(crb.Subjects))
	}
	subj := crb.Subjects[0]
	if subj.Kind != "ServiceAccount" {
		t.Errorf("subject kind: got %s", subj.Kind)
	}
	if subj.Name != "my-operator-sa" {
		t.Errorf("subject name: got %s", subj.Name)
	}
	if subj.Namespace != "kube-system" {
		t.Errorf("subject namespace: got %s", subj.Namespace)
	}
}

func TestOperatorClusterRoleBinding_Labels(t *testing.T) {
	crb := OperatorClusterRoleBinding("sa", "ns")
	if crb.Labels["app.kubernetes.io/component"] != "operator" {
		t.Error("missing component label")
	}
}

// -------------------------------------------------------------------
// SessionRole
// -------------------------------------------------------------------

func TestSessionRole_TypeMeta(t *testing.T) {
	role := SessionRole("default")
	if role.Kind != "Role" {
		t.Errorf("kind: got %s", role.Kind)
	}
	if role.APIVersion != "rbac.authorization.k8s.io/v1" {
		t.Errorf("apiVersion: got %s", role.APIVersion)
	}
}

func TestSessionRole_NameAndNamespace(t *testing.T) {
	role := SessionRole("my-namespace")
	if role.Name != "ralphglasses-session" {
		t.Errorf("name: got %s", role.Name)
	}
	if role.Namespace != "my-namespace" {
		t.Errorf("namespace: got %s", role.Namespace)
	}
}

func TestSessionRole_Labels(t *testing.T) {
	role := SessionRole("default")
	expected := map[string]string{
		"app.kubernetes.io/name":       "ralphglasses",
		"app.kubernetes.io/component":  "session",
		"app.kubernetes.io/managed-by": "ralphglasses-operator",
	}
	for k, v := range expected {
		if role.Labels[k] != v {
			t.Errorf("label %s: expected %q, got %q", k, v, role.Labels[k])
		}
	}
}

func TestSessionRole_MinimalPermissions(t *testing.T) {
	role := SessionRole("default")
	if len(role.Rules) != 2 {
		t.Fatalf("expected 2 rules (configmaps + secrets), got %d", len(role.Rules))
	}

	// ConfigMaps: read-only.
	cmRule := role.Rules[0]
	for _, r := range cmRule.Resources {
		if r == "configmaps" {
			for _, v := range cmRule.Verbs {
				if v == "create" || v == "update" || v == "delete" || v == "patch" {
					t.Errorf("configmaps should be read-only, got verb %q", v)
				}
			}
		}
	}

	// Secrets: get only.
	secretRule := role.Rules[1]
	for _, r := range secretRule.Resources {
		if r == "secrets" {
			if len(secretRule.Verbs) != 1 || secretRule.Verbs[0] != "get" {
				t.Errorf("secrets should only have 'get', got %v", secretRule.Verbs)
			}
		}
	}
}

// -------------------------------------------------------------------
// SessionRoleBinding
// -------------------------------------------------------------------

func TestSessionRoleBinding_TypeMeta(t *testing.T) {
	rb := SessionRoleBinding("session-sa", "default")
	if rb.Kind != "RoleBinding" {
		t.Errorf("kind: got %s", rb.Kind)
	}
	if rb.APIVersion != "rbac.authorization.k8s.io/v1" {
		t.Errorf("apiVersion: got %s", rb.APIVersion)
	}
}

func TestSessionRoleBinding_NameAndNamespace(t *testing.T) {
	rb := SessionRoleBinding("session-sa", "prod")
	if rb.Name != "ralphglasses-session" {
		t.Errorf("name: got %s", rb.Name)
	}
	if rb.Namespace != "prod" {
		t.Errorf("namespace: got %s", rb.Namespace)
	}
}

func TestSessionRoleBinding_RoleRef(t *testing.T) {
	rb := SessionRoleBinding("session-sa", "default")
	if rb.RoleRef.Kind != "Role" {
		t.Errorf("roleRef kind: got %s", rb.RoleRef.Kind)
	}
	if rb.RoleRef.Name != "ralphglasses-session" {
		t.Errorf("roleRef name: got %s", rb.RoleRef.Name)
	}
	if rb.RoleRef.APIGroup != "rbac.authorization.k8s.io" {
		t.Errorf("roleRef apiGroup: got %s", rb.RoleRef.APIGroup)
	}
}

func TestSessionRoleBinding_Subject(t *testing.T) {
	rb := SessionRoleBinding("my-sa", "staging")
	if len(rb.Subjects) != 1 {
		t.Fatalf("expected 1 subject, got %d", len(rb.Subjects))
	}
	subj := rb.Subjects[0]
	if subj.Kind != "ServiceAccount" {
		t.Errorf("kind: got %s", subj.Kind)
	}
	if subj.Name != "my-sa" {
		t.Errorf("name: got %s", subj.Name)
	}
	if subj.Namespace != "staging" {
		t.Errorf("namespace: got %s", subj.Namespace)
	}
}

func TestSessionRoleBinding_Labels(t *testing.T) {
	rb := SessionRoleBinding("sa", "ns")
	if rb.Labels["app.kubernetes.io/component"] != "session" {
		t.Error("missing session component label")
	}
}

// -------------------------------------------------------------------
// AllRBACManifests
// -------------------------------------------------------------------

func TestAllRBACManifests_Count(t *testing.T) {
	manifests := AllRBACManifests("op-sa", "sess-sa", "ralph-ns")
	if len(manifests) != 4 {
		t.Errorf("expected 4 manifests, got %d", len(manifests))
	}
}

func TestAllRBACManifests_Types(t *testing.T) {
	manifests := AllRBACManifests("op-sa", "sess-sa", "ralph-ns")

	types := make(map[string]bool)
	for _, m := range manifests {
		switch m.(type) {
		case *ClusterRole:
			types["ClusterRole"] = true
		case *ClusterRoleBinding:
			types["ClusterRoleBinding"] = true
		case *Role:
			types["Role"] = true
		case *RoleBinding:
			types["RoleBinding"] = true
		}
	}

	for _, expected := range []string{"ClusterRole", "ClusterRoleBinding", "Role", "RoleBinding"} {
		if !types[expected] {
			t.Errorf("missing manifest type: %s", expected)
		}
	}
}

func TestAllRBACManifests_ParameterPassthrough(t *testing.T) {
	manifests := AllRBACManifests("custom-op", "custom-sess", "custom-ns")

	// Check ClusterRoleBinding got the right SA.
	for _, m := range manifests {
		if crb, ok := m.(*ClusterRoleBinding); ok {
			if crb.Subjects[0].Name != "custom-op" {
				t.Errorf("operator SA: got %s", crb.Subjects[0].Name)
			}
			if crb.Subjects[0].Namespace != "custom-ns" {
				t.Errorf("operator namespace: got %s", crb.Subjects[0].Namespace)
			}
		}
		if rb, ok := m.(*RoleBinding); ok {
			if rb.Subjects[0].Name != "custom-sess" {
				t.Errorf("session SA: got %s", rb.Subjects[0].Name)
			}
			if rb.Namespace != "custom-ns" {
				t.Errorf("role binding namespace: got %s", rb.Namespace)
			}
		}
		if role, ok := m.(*Role); ok {
			if role.Namespace != "custom-ns" {
				t.Errorf("role namespace: got %s", role.Namespace)
			}
		}
	}
}

// -------------------------------------------------------------------
// JSON serialization of RBAC types
// -------------------------------------------------------------------

func TestClusterRole_JSONRoundTrip(t *testing.T) {
	cr := OperatorClusterRole()
	data, err := json.Marshal(cr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ClusterRole
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Kind != "ClusterRole" {
		t.Errorf("kind: got %s", decoded.Kind)
	}
	if decoded.Name != "ralphglasses-operator" {
		t.Errorf("name: got %s", decoded.Name)
	}
	if len(decoded.Rules) != len(cr.Rules) {
		t.Errorf("rules count: got %d, want %d", len(decoded.Rules), len(cr.Rules))
	}
}

func TestRoleBinding_JSONRoundTrip(t *testing.T) {
	rb := SessionRoleBinding("sa", "ns")
	data, err := json.Marshal(rb)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded RoleBinding
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Kind != "RoleBinding" {
		t.Errorf("kind: got %s", decoded.Kind)
	}
	if decoded.RoleRef.Name != "ralphglasses-session" {
		t.Errorf("roleRef name: got %s", decoded.RoleRef.Name)
	}
	if len(decoded.Subjects) != 1 {
		t.Errorf("subjects: got %d", len(decoded.Subjects))
	}
}

func TestPolicyRule_JSON(t *testing.T) {
	rule := PolicyRule{
		APIGroups: []string{"", "apps"},
		Resources: []string{"pods", "deployments"},
		Verbs:     []string{"get", "list"},
	}

	data, err := json.Marshal(rule)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded PolicyRule
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(decoded.APIGroups) != 2 {
		t.Errorf("apiGroups: got %d", len(decoded.APIGroups))
	}
	if len(decoded.Resources) != 2 {
		t.Errorf("resources: got %d", len(decoded.Resources))
	}
	if len(decoded.Verbs) != 2 {
		t.Errorf("verbs: got %d", len(decoded.Verbs))
	}
}
