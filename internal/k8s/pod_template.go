package k8s

import "fmt"

// PodTemplateBuilder constructs a PodSpec from a RalphSession custom resource.
// It applies sensible defaults for resource limits, volume mounts, and
// environment variable injection while allowing the CRD spec to override
// any value.
type PodTemplateBuilder struct {
	session *RalphSession
}

// NewPodTemplateBuilder creates a builder for the given session.
func NewPodTemplateBuilder(session *RalphSession) *PodTemplateBuilder {
	return &PodTemplateBuilder{session: session}
}

// DefaultResources returns conservative resource requests and limits
// suitable for a headless LLM CLI session.
func DefaultResources() *ResourceRequirements {
	return &ResourceRequirements{
		Requests: ResourceList{
			"cpu":    "250m",
			"memory": "256Mi",
		},
		Limits: ResourceList{
			"cpu":    "1",
			"memory": "1Gi",
		},
	}
}

// Build produces a complete PodSpec ready for submission to the Kubernetes API.
func (b *PodTemplateBuilder) Build() *PodSpec {
	s := b.session
	spec := s.Spec

	image := spec.Image
	if image == "" {
		image = DefaultImage
	}

	resources := spec.Resources
	if resources == nil {
		resources = DefaultResources()
	}

	podName := podNameForSession(s)

	labels := map[string]string{
		"app.kubernetes.io/name":       "ralphglasses",
		"app.kubernetes.io/component":  "session",
		"app.kubernetes.io/managed-by": "ralphglasses-operator",
		"ralphglasses.studio/session":  s.Name,
		"ralphglasses.studio/provider": spec.Provider,
	}
	if spec.TeamName != "" {
		labels["ralphglasses.studio/team"] = spec.TeamName
	}

	// Merge user-provided labels.
	for k, v := range s.Labels {
		if _, reserved := labels[k]; !reserved {
			labels[k] = v
		}
	}

	annotations := map[string]string{
		"ralphglasses.studio/prompt-hash": hashPrompt(spec.Prompt),
	}
	if spec.BudgetUSD > 0 {
		annotations["ralphglasses.studio/budget-usd"] = fmt.Sprintf("%.2f", spec.BudgetUSD)
	}

	// Build command and args for the session container.
	command, args := b.buildCommand()

	// Environment variables — provider API keys are injected via envFrom
	// secrets, but we set metadata as plain env vars.
	env := []EnvVar{
		{Name: "RALPH_SESSION_NAME", Value: s.Name},
		{Name: "RALPH_PROVIDER", Value: spec.Provider},
		{Name: "RALPH_NAMESPACE", Value: s.Namespace},
	}
	if spec.Model != "" {
		env = append(env, EnvVar{Name: "RALPH_MODEL", Value: spec.Model})
	}
	if spec.MaxTurns > 0 {
		env = append(env, EnvVar{Name: "RALPH_MAX_TURNS", Value: fmt.Sprintf("%d", spec.MaxTurns)})
	}
	if spec.BudgetUSD > 0 {
		env = append(env, EnvVar{Name: "RALPH_BUDGET_USD", Value: fmt.Sprintf("%.2f", spec.BudgetUSD)})
	}

	// Default envFrom: inject provider API key secrets by convention.
	envFrom := append([]EnvFromSource{}, spec.EnvFrom...)
	envFrom = append(envFrom, defaultAPIKeyEnvFrom(spec.Provider)...)

	// Volumes: always include a workspace volume; user can add more.
	volumes := []Volume{
		{Name: "workspace", EmptyDir: true},
	}
	volumes = append(volumes, spec.Volumes...)

	volumeMounts := []VolumeMount{
		{Name: "workspace", MountPath: "/workspace"},
	}
	volumeMounts = append(volumeMounts, spec.VolumeMounts...)

	pod := &PodSpec{
		Name:      podName,
		Namespace: s.Namespace,
		Labels:    labels,
		Annotations: annotations,
		Image:        image,
		Command:      command,
		Args:         args,
		Env:          env,
		EnvFrom:      envFrom,
		Resources:    resources,
		VolumeMounts: volumeMounts,
		Volumes:      volumes,
	}

	// Set owner reference so the pod is garbage-collected with the CRD.
	if s.UID != "" {
		pod.OwnerRef = &OwnerReference{
			APIVersion: GroupName + "/" + Version,
			Kind:       "RalphSession",
			Name:       s.Name,
			UID:        s.UID,
		}
	}

	return pod
}

// buildCommand returns the container command and args for the session's provider.
func (b *PodTemplateBuilder) buildCommand() ([]string, []string) {
	spec := b.session.Spec

	switch spec.Provider {
	case "claude":
		args := []string{"--print", "--output-format", "stream-json"}
		if spec.Model != "" {
			args = append(args, "--model", spec.Model)
		}
		if spec.MaxTurns > 0 {
			args = append(args, "--max-turns", fmt.Sprintf("%d", spec.MaxTurns))
		}
		args = append(args, spec.Prompt)
		return []string{"claude"}, args

	case "gemini":
		args := []string{"-p", spec.Prompt}
		if spec.Model != "" {
			args = append(args, "--model", spec.Model)
		}
		return []string{"gemini"}, args

	case "codex":
		args := []string{"--prompt", spec.Prompt}
		if spec.Model != "" {
			args = append(args, "--model", spec.Model)
		}
		return []string{"codex"}, args

	default:
		// Fallback: generic ralphglasses runner.
		return []string{"ralphglasses", "run"}, []string{
			"--provider", spec.Provider,
			"--prompt", spec.Prompt,
		}
	}
}

// defaultAPIKeyEnvFrom returns EnvFromSource entries for the standard
// secret names used by each provider. These secrets are expected to
// exist in the same namespace as the session pod.
func defaultAPIKeyEnvFrom(provider string) []EnvFromSource {
	switch provider {
	case "claude":
		return []EnvFromSource{
			{SecretRef: &SecretReference{Name: "ralph-anthropic-api-key"}},
		}
	case "gemini":
		return []EnvFromSource{
			{SecretRef: &SecretReference{Name: "ralph-google-api-key"}},
		}
	case "codex":
		return []EnvFromSource{
			{SecretRef: &SecretReference{Name: "ralph-openai-api-key"}},
		}
	default:
		return nil
	}
}

// hashPrompt returns a short hash of the prompt for annotation tracking.
func hashPrompt(prompt string) string {
	// FNV-1a inspired, kept simple to avoid crypto imports.
	var h uint32 = 2166136261
	for i := 0; i < len(prompt); i++ {
		h ^= uint32(prompt[i])
		h *= 16777619
	}
	return fmt.Sprintf("%08x", h)
}
