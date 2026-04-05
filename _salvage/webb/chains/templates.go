package chains

import (
	"fmt"
	"regexp"
	"strings"
)

// ChainTemplate represents a reusable chain template
type ChainTemplate struct {
	Name        string                 `yaml:"name" json:"name"`
	Description string                 `yaml:"description" json:"description"`
	Category    string                 `yaml:"category" json:"category"`
	Parameters  []TemplateParameter    `yaml:"parameters" json:"parameters"`
	Chain       ChainDefinition        `yaml:"chain" json:"chain"`
	Defaults    map[string]interface{} `yaml:"defaults" json:"defaults"`
}

// TemplateParameter defines a template parameter
type TemplateParameter struct {
	Name        string      `yaml:"name" json:"name"`
	Type        string      `yaml:"type" json:"type"` // string, int, bool, list
	Description string      `yaml:"description" json:"description"`
	Required    bool        `yaml:"required" json:"required"`
	Default     interface{} `yaml:"default" json:"default"`
	Validation  string      `yaml:"validation" json:"validation"` // regex pattern
}

// TemplateRegistry manages chain templates
type TemplateRegistry struct {
	templates map[string]*ChainTemplate
}

// NewTemplateRegistry creates a new template registry
func NewTemplateRegistry() *TemplateRegistry {
	return &TemplateRegistry{
		templates: make(map[string]*ChainTemplate),
	}
}

// Register adds a template to the registry
func (r *TemplateRegistry) Register(template *ChainTemplate) error {
	if template.Name == "" {
		return fmt.Errorf("template name is required")
	}
	r.templates[template.Name] = template
	return nil
}

// Get retrieves a template by name
func (r *TemplateRegistry) Get(name string) (*ChainTemplate, error) {
	template, exists := r.templates[name]
	if !exists {
		return nil, fmt.Errorf("template %q not found", name)
	}
	return template, nil
}

// List returns all template names
func (r *TemplateRegistry) List() []string {
	names := make([]string, 0, len(r.templates))
	for name := range r.templates {
		names = append(names, name)
	}
	return names
}

// ListByCategory returns templates in a category
func (r *TemplateRegistry) ListByCategory(category string) []*ChainTemplate {
	var templates []*ChainTemplate
	for _, t := range r.templates {
		if t.Category == category {
			templates = append(templates, t)
		}
	}
	return templates
}

// Instantiate creates a chain definition from a template
func (r *TemplateRegistry) Instantiate(templateName string, params map[string]interface{}) (*ChainDefinition, error) {
	template, err := r.Get(templateName)
	if err != nil {
		return nil, err
	}

	// Validate and fill in defaults
	resolvedParams := make(map[string]interface{})

	// Apply defaults first
	for k, v := range template.Defaults {
		resolvedParams[k] = v
	}

	// Apply parameter defaults
	for _, p := range template.Parameters {
		if p.Default != nil {
			resolvedParams[p.Name] = p.Default
		}
	}

	// Apply provided params
	for k, v := range params {
		resolvedParams[k] = v
	}

	// Validate required params
	for _, p := range template.Parameters {
		val, exists := resolvedParams[p.Name]
		if p.Required && !exists {
			return nil, fmt.Errorf("required parameter %q not provided", p.Name)
		}
		if exists && p.Validation != "" {
			if !validateParam(val, p.Validation) {
				return nil, fmt.Errorf("parameter %q failed validation: %s", p.Name, p.Validation)
			}
		}
	}

	// Clone the chain definition
	chain := cloneChainDefinition(&template.Chain)

	// Substitute parameters in the chain
	substituteParams(chain, resolvedParams)

	return chain, nil
}

func validateParam(value interface{}, pattern string) bool {
	strVal := fmt.Sprintf("%v", value)
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(strVal)
}

func cloneChainDefinition(orig *ChainDefinition) *ChainDefinition {
	clone := &ChainDefinition{
		Name:        orig.Name,
		Description: orig.Description,
		Category:    orig.Category,
		Tags:        append([]string{}, orig.Tags...),
		Trigger:     orig.Trigger,
		Variables:   copyStringMap(orig.Variables),
		Steps:       cloneSteps(orig.Steps),
		Timeout:     orig.Timeout,
	}
	if orig.OnError != nil {
		clone.OnError = &ErrorHandler{
			Action:   orig.OnError.Action,
			Fallback: orig.OnError.Fallback,
			Notify:   orig.OnError.Notify,
		}
	}
	return clone
}

func cloneSteps(steps []ChainStep) []ChainStep {
	if steps == nil {
		return nil
	}
	cloned := make([]ChainStep, len(steps))
	for i, s := range steps {
		cloned[i] = ChainStep{
			ID:          s.ID,
			Type:        s.Type,
			Name:        s.Name,
			Tool:        s.Tool,
			Params:      copyStringMap(s.Params),
			Chain:       s.Chain,
			Steps:       cloneSteps(s.Steps),
			Condition:   s.Condition,
			Branches:    cloneBranches(s.Branches),
			GateType:    s.GateType,
			Message:     s.Message,
			GateTimeout: s.GateTimeout,
			OnTimeout:   s.OnTimeout,
			ContinueOn:  s.ContinueOn,
			StoreAs:     s.StoreAs,
		}
		if s.Retry != nil {
			cloned[i].Retry = &RetryPolicy{
				MaxAttempts: s.Retry.MaxAttempts,
				Delay:       s.Retry.Delay,
				BackoffRate: s.Retry.BackoffRate,
			}
		}
	}
	return cloned
}

func cloneBranches(branches map[string][]ChainStep) map[string][]ChainStep {
	if branches == nil {
		return nil
	}
	cloned := make(map[string][]ChainStep)
	for k, v := range branches {
		cloned[k] = cloneSteps(v)
	}
	return cloned
}

func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	cp := make(map[string]string)
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

func substituteParams(chain *ChainDefinition, params map[string]interface{}) {
	// Substitute in name
	chain.Name = substituteString(chain.Name, params)

	// Substitute in variables
	for k, v := range chain.Variables {
		chain.Variables[k] = substituteString(v, params)
	}

	// Substitute in steps
	substituteStepsParams(chain.Steps, params)
}

func substituteStepsParams(steps []ChainStep, params map[string]interface{}) {
	for i := range steps {
		substituteStepParams(&steps[i], params)
	}
}

func substituteStepParams(step *ChainStep, params map[string]interface{}) {
	// Substitute in tool params
	for k, v := range step.Params {
		step.Params[k] = substituteString(v, params)
	}

	// Substitute in gate message
	step.Message = substituteString(step.Message, params)

	// Substitute in nested steps
	substituteStepsParams(step.Steps, params)

	// Substitute in branches
	for _, branchSteps := range step.Branches {
		substituteStepsParams(branchSteps, params)
	}
}

func substituteString(s string, params map[string]interface{}) string {
	result := s
	for k, v := range params {
		placeholder := fmt.Sprintf("${{%s}}", k)
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", v))
		// Also support {{ param }} style
		placeholder2 := fmt.Sprintf("{{ %s }}", k)
		result = strings.ReplaceAll(result, placeholder2, fmt.Sprintf("%v", v))
	}
	return result
}

// Built-in templates
var builtInTemplates = []*ChainTemplate{
	{
		Name:        "customer-investigation",
		Description: "Investigate a customer issue end-to-end",
		Category:    "customer",
		Parameters: []TemplateParameter{
			{Name: "customer", Type: "string", Description: "Customer name", Required: true},
			{Name: "ticket_id", Type: "string", Description: "Ticket ID", Required: false},
			{Name: "cluster", Type: "string", Description: "Cluster context", Required: false, Default: "headspace-v2"},
		},
		Chain: ChainDefinition{
			Name:        "customer-investigation-${{customer}}",
			Description: "Investigation for ${{customer}}",
			Category:    "customer",
			Steps: []ChainStep{
				{
					ID:   "snapshot",
					Type: StepTypeTool,
					Tool: "webb_customer_snapshot",
					Params: map[string]string{
						"customer": "${{customer}}",
					},
				},
				{
					ID:   "cluster_health",
					Type: StepTypeTool,
					Tool: "webb_cluster_health_full",
					Params: map[string]string{
						"context": "${{cluster}}",
					},
				},
				{
					ID:   "slack_search",
					Type: StepTypeTool,
					Tool: "webb_slack_search",
					Params: map[string]string{
						"query": "${{customer}}",
					},
				},
			},
		},
	},
	{
		Name:        "incident-channel",
		Description: "Create and set up an incident channel",
		Category:    "incident",
		Parameters: []TemplateParameter{
			{Name: "incident_id", Type: "string", Description: "Incident ID", Required: true},
			{Name: "severity", Type: "string", Description: "Incident severity", Required: true, Validation: "^(P0|P1|P2|P3)$"},
			{Name: "title", Type: "string", Description: "Incident title", Required: true},
		},
		Chain: ChainDefinition{
			Name:        "incident-channel-${{incident_id}}",
			Description: "Set up incident channel for ${{incident_id}}",
			Category:    "incident",
			Steps: []ChainStep{
				{
					ID:   "create_channel",
					Type: StepTypeTool,
					Tool: "webb_slack_channel_create",
					Params: map[string]string{
						"name":       "inc-${{incident_id}}",
						"is_private": "false",
					},
				},
				{
					ID:   "set_topic",
					Type: StepTypeTool,
					Tool: "webb_slack_channel_topic",
					Params: map[string]string{
						"channel": "{{ steps.create_channel.channel_id }}",
						"topic":   "${{severity}} Incident: ${{title}} | Status: Active",
					},
				},
			},
		},
	},
	{
		Name:        "health-check",
		Description: "Run a comprehensive health check",
		Category:    "operational",
		Parameters: []TemplateParameter{
			{Name: "cluster", Type: "string", Description: "Cluster context", Required: true},
			{Name: "include_database", Type: "bool", Description: "Include database checks", Required: false, Default: true},
		},
		Chain: ChainDefinition{
			Name:        "health-check-${{cluster}}",
			Description: "Health check for ${{cluster}}",
			Category:    "operational",
			Steps: []ChainStep{
				{
					ID:   "cluster",
					Type: StepTypeParallel,
					Steps: []ChainStep{
						{
							ID:   "cluster_health",
							Type: StepTypeTool,
							Tool: "webb_cluster_health_full",
							Params: map[string]string{
								"context": "${{cluster}}",
							},
						},
						{
							ID:   "alerts",
							Type: StepTypeTool,
							Tool: "webb_grafana_alerts",
							Params: map[string]string{
								"state": "firing",
							},
						},
					},
				},
			},
		},
	},
	// =============================================================================
	// Content Workflow Chains (Phase 4)
	// =============================================================================
	{
		Name:        "content-review",
		Description: "Review and audit content across platforms",
		Category:    "content",
		Parameters: []TemplateParameter{
			{Name: "platform", Type: "string", Description: "Content platform: gdocs, slides, confluence, vault", Required: true},
			{Name: "content_id", Type: "string", Description: "Content ID (doc_id, presentation_id, page_id, or path)", Required: true},
			{Name: "auto_fix", Type: "bool", Description: "Automatically apply suggested fixes", Required: false, Default: false},
		},
		Chain: ChainDefinition{
			Name:        "content-review-${{platform}}-${{content_id}}",
			Description: "Review content on ${{platform}}",
			Category:    "content",
			Steps: []ChainStep{
				{
					ID:   "audit",
					Type: StepTypeTool,
					Tool: "webb_content_audit",
					Params: map[string]string{
						"platform":   "${{platform}}",
						"content_id": "${{content_id}}",
						"check_all":  "true",
					},
				},
				{
					ID:   "score",
					Type: StepTypeTool,
					Tool: "webb_content_score",
					Params: map[string]string{
						"platform":   "${{platform}}",
						"content_id": "${{content_id}}",
					},
				},
			},
		},
	},
	{
		Name:        "content-publish",
		Description: "Publish content after quality review",
		Category:    "content",
		Parameters: []TemplateParameter{
			{Name: "source_platform", Type: "string", Description: "Source platform: gdocs, slides", Required: true},
			{Name: "source_id", Type: "string", Description: "Source content ID", Required: true},
			{Name: "target_platforms", Type: "string", Description: "Target platforms (comma-separated): confluence, vault", Required: true},
			{Name: "min_score", Type: "int", Description: "Minimum quality score required", Required: false, Default: 80},
		},
		Chain: ChainDefinition{
			Name:        "content-publish-${{source_id}}",
			Description: "Publish content from ${{source_platform}} to ${{target_platforms}}",
			Category:    "content",
			Steps: []ChainStep{
				{
					ID:   "quality_check",
					Type: StepTypeTool,
					Tool: "webb_content_score",
					Params: map[string]string{
						"platform":   "${{source_platform}}",
						"content_id": "${{source_id}}",
					},
				},
				{
					ID:        "quality_gate",
					Type:      StepTypeGate,
					Condition: "{{ steps.quality_check.overall_score >= params.min_score }}",
					Message:   "Content score is below minimum threshold",
				},
				{
					ID:   "migrate",
					Type: StepTypeTool,
					Tool: "webb_content_migrate",
					Params: map[string]string{
						"source_platform": "${{source_platform}}",
						"source_id":       "${{source_id}}",
						"target_platforms": "${{target_platforms}}",
					},
				},
			},
		},
	},
	{
		Name:        "content-sync",
		Description: "Synchronize content between platforms",
		Category:    "content",
		Parameters: []TemplateParameter{
			{Name: "source_platform", Type: "string", Description: "Source platform", Required: true},
			{Name: "source_id", Type: "string", Description: "Source content ID", Required: true},
			{Name: "target_platform", Type: "string", Description: "Target platform", Required: true},
			{Name: "target_id", Type: "string", Description: "Target content ID", Required: true},
		},
		Chain: ChainDefinition{
			Name:        "content-sync-${{source_id}}-${{target_id}}",
			Description: "Sync content from ${{source_platform}} to ${{target_platform}}",
			Category:    "content",
			Steps: []ChainStep{
				{
					ID:   "diff",
					Type: StepTypeTool,
					Tool: "webb_content_diff",
					Params: map[string]string{
						"source_platform": "${{source_platform}}",
						"source_id":       "${{source_id}}",
						"target_platform": "${{target_platform}}",
						"target_id":       "${{target_id}}",
					},
				},
				{
					ID:      "confirm",
					Type:    StepTypeGate,
					Message: "Review diff above. Proceed with sync?",
				},
				{
					ID:   "sync",
					Type: StepTypeTool,
					Tool: "webb_content_migrate",
					Params: map[string]string{
						"source_platform":  "${{source_platform}}",
						"source_id":        "${{source_id}}",
						"target_platforms": "${{target_platform}}",
					},
				},
			},
		},
	},
}

// LoadBuiltInTemplates loads the built-in templates into a registry
func LoadBuiltInTemplates(registry *TemplateRegistry) {
	for _, t := range builtInTemplates {
		registry.Register(t)
	}
}
