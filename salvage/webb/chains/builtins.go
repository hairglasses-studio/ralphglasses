package chains

// GetBuiltInChains returns all built-in chain definitions
func GetBuiltInChains() []*ChainDefinition {
	return []*ChainDefinition{
		morningRoutineChain(),
		oncallHandoffChain(),
		weeklyReviewChain(),
		incidentResponseChain(),
		customerInvestigationChain(),
		quickTriageChain(),
		escalationChain(),
		contextSyncChain(),
		autoRemediationChain(),
		deployValidationChain(),
		prReviewChain(),
		deepRCAChain(),
		vaultGraphRebuildChain(),
		vaultOrphanLinkChain(),
		vaultFreshnessChain(),
		oncallActionReviewChain(),
		mlPipelineInvestigationChain(),
		// Phase 2 chains (v131)
		costAnomalyInvestigationChain(),
		securityIncidentResponseChain(),
		migrationValidationChain(),
		performanceOptimizationChain(),
		// Phase 3 chains (v132) - Customer lifecycle, incident lifecycle, continuous improvement
		customerHealthAlertChain(),
		incidentAutoPostmortemChain(),
		customerCostAnomalyChain(),
		customerOnboardingChain(),
		incidentKnowledgeCaptureChain(),
		databaseQueryOptimizationChain(),
		nightlyResearchSwarmChain(),
		weeklyComplianceReportChain(),
		// On-call workflow chain (v127)
		oncallFullCheckChain(),
		// Ephemeral cluster chain (v132)
		ephemeralClusterPreflightChain(),
	}
}

// RegisterBuiltInChains loads all built-in chains into a registry
func RegisterBuiltInChains(registry *Registry) error {
	for _, chain := range GetBuiltInChains() {
		if err := registry.Register(chain); err != nil {
			return err
		}
	}
	return nil
}

func morningRoutineChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "morning-routine",
		Description: "Daily morning standup preparation and briefing",
		Category:    CategoryOperational,
		Tags:        []string{"daily", "standup", "briefing"},
		Trigger: ChainTrigger{
			Type: TriggerScheduled,
			Cron: "0 0 9 * * 1-5",
		},
		Steps: []ChainStep{
			{
				ID:   "health_check",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "cluster_health",
						Type: StepTypeTool,
						Tool: "webb_cluster_health_full",
						Params: map[string]string{
							"context": "headspace-v2",
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
					{
						ID:   "incidents",
						Type: StepTypeTool,
						Tool: "webb_incidentio_list",
						Params: map[string]string{
							"status": "active",
						},
					},
					{
						ID:   "tickets",
						Type: StepTypeTool,
						Tool: "webb_pylon_my_queue",
					},
					{
						ID:   "slack_unread",
						Type: StepTypeTool,
						Tool: "webb_slack_inbox_unread",
					},
				},
			},
			{
				ID:        "assess_urgency",
				Type:      StepTypeBranch,
				Condition: "{{ steps.health_check.cluster_health.health_score }} < 70",
				Branches: map[string][]ChainStep{
					"true": {
						{
							ID:   "investigate_health",
							Type: StepTypeTool,
							Tool: "webb_k8s_events",
							Params: map[string]string{
								"context":  "headspace-v2",
								"severity": "Warning",
							},
						},
					},
				},
			},
			{
				ID:   "create_briefing",
				Type: StepTypeTool,
				Tool: "webb_tomorrow_prep",
			},
		},
		Timeout: "10m",
	}
}

func oncallHandoffChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "oncall-handoff",
		Description: "Generate handoff summary for shift change",
		Category:    CategoryOperational,
		Tags:        []string{"oncall", "handoff", "shift"},
		Trigger: ChainTrigger{
			Type: TriggerScheduled,
			Cron: "0 0 17 * * 1-5",
		},
		Steps: []ChainStep{
			{
				ID:   "gather_context",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "active_incidents",
						Type: StepTypeTool,
						Tool: "webb_incidentio_list",
						Params: map[string]string{
							"status": "active",
						},
					},
					{
						ID:   "open_tickets",
						Type: StepTypeTool,
						Tool: "webb_pylon_my_queue",
					},
					{
						ID:   "recent_deploys",
						Type: StepTypeTool,
						Tool: "webb_release_deploy_status",
						Params: map[string]string{
							"hours": "8",
						},
					},
					{
						ID:   "escalations",
						Type: StepTypeTool,
						Tool: "webb_escalation_status",
					},
				},
			},
			{
				ID:   "generate_summary",
				Type: StepTypeTool,
				Tool: "webb_shift_handoff_brief",
			},
		},
		Timeout: "5m",
	}
}

func weeklyReviewChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "weekly-review",
		Description: "Weekly planning and metrics review",
		Category:    CategoryOperational,
		Tags:        []string{"weekly", "planning", "review"},
		Trigger: ChainTrigger{
			Type: TriggerScheduled,
			Cron: "0 0 9 * * 1",
		},
		Steps: []ChainStep{
			{
				ID:   "gather_metrics",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "escalation_metrics",
						Type: StepTypeTool,
						Tool: "webb_escalation_metrics",
						Params: map[string]string{
							"days": "7",
						},
					},
					{
						ID:   "sentiment",
						Type: StepTypeTool,
						Tool: "webb_sentiment_trends",
						Params: map[string]string{
							"days": "7",
						},
					},
					{
						ID:   "month_analysis",
						Type: StepTypeTool,
						Tool: "webb_month_analysis",
						Params: map[string]string{
							"days": "7",
						},
					},
				},
			},
			{
				ID:   "triage_trends",
				Type: StepTypeTool,
				Tool: "webb_triage_trends",
			},
			{
				ID:   "backlog_scan",
				Type: StepTypeTool,
				Tool: "webb_backlog_scan",
			},
		},
		Timeout: "15m",
	}
}

func incidentResponseChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "incident-response",
		Description: "Automated incident response workflow",
		Category:    CategoryInvestigative,
		Tags:        []string{"incident", "response", "investigation"},
		Trigger: ChainTrigger{
			Type:   TriggerEvent,
			Event:  "incident.created",
			Filter: "severity in ['P0', 'P1']",
		},
		Variables: map[string]string{
			"incident_id": "{{ trigger.incident.id }}",
			"customer":    "{{ trigger.incident.customer }}",
			"cluster":     "{{ trigger.incident.cluster }}",
		},
		Steps: []ChainStep{
			{
				ID:   "acknowledge",
				Type: StepTypeTool,
				Tool: "webb_incidentio_add_update",
				Params: map[string]string{
					"incident_id": "{{ incident_id }}",
					"message":     "Investigation started by automation",
				},
			},
			{
				ID:   "gather_context",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "cluster_health",
						Type: StepTypeTool,
						Tool: "webb_cluster_health_full",
						Params: map[string]string{
							"context": "{{ cluster }}",
						},
					},
					{
						ID:   "recent_deploys",
						Type: StepTypeTool,
						Tool: "webb_release_pipeline_full",
						Params: map[string]string{
							"hours": "24",
						},
					},
					{
						ID:   "slack_context",
						Type: StepTypeTool,
						Tool: "webb_slack_search",
						Params: map[string]string{
							"query": "{{ customer }} error",
						},
					},
				},
			},
			{
				ID:   "analyze",
				Type: StepTypeTool,
				Tool: "webb_rca_suggest",
				Params: map[string]string{
					"symptoms": "{{ steps.gather_context }}",
				},
			},
			{
				ID:        "route_severity",
				Type:      StepTypeBranch,
				Condition: "{{ steps.gather_context.cluster_health.health_score }} < 50",
				Branches: map[string][]ChainStep{
					"true": {
						{
							ID:    "escalate",
							Type:  StepTypeChain,
							Chain: "escalation",
						},
					},
					"false": {
						{
							ID:   "create_ticket",
							Type: StepTypeTool,
							Tool: "webb_shortcut_create",
							Params: map[string]string{
								"title":    "Investigate: {{ incident_id }}",
								"priority": "medium",
							},
						},
					},
				},
			},
		},
		Timeout: "30m",
	}
}

func customerInvestigationChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "customer-investigation",
		Description: "End-to-end customer issue investigation",
		Category:    CategoryCustomer,
		Tags:        []string{"customer", "investigation", "support"},
		Trigger: ChainTrigger{
			Type:   TriggerEvent,
			Event:  "ticket.urgent",
			Filter: "priority in ['urgent', 'p0']",
		},
		Variables: map[string]string{
			"ticket_id": "{{ trigger.ticket.id }}",
			"customer":  "{{ trigger.ticket.customer }}",
		},
		Steps: []ChainStep{
			{
				ID:   "snapshot",
				Type: StepTypeTool,
				Tool: "webb_customer_snapshot",
				Params: map[string]string{
					"customer": "{{ customer }}",
				},
			},
			{
				ID:   "investigate",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "engagement",
						Type: StepTypeTool,
						Tool: "webb_engagement_context",
						Params: map[string]string{
							"customer": "{{ customer }}",
						},
					},
					{
						ID:   "slack_history",
						Type: StepTypeTool,
						Tool: "webb_slack_search",
						Params: map[string]string{
							"query": "{{ customer }}",
						},
					},
					{
						ID:   "incidents",
						Type: StepTypeTool,
						Tool: "webb_incidentio_list",
						Params: map[string]string{
							"customer": "{{ customer }}",
						},
					},
				},
			},
			{
				ID:        "check_known_issues",
				Type:      StepTypeBranch,
				Condition: "{{ steps.snapshot.has_known_issues }}",
				Branches: map[string][]ChainStep{
					"true": {
						{
							ID:   "quick_fix",
							Type: StepTypeTool,
							Tool: "webb_field_quickfix",
							Params: map[string]string{
								"customer": "{{ customer }}",
							},
						},
					},
					"false": {
						{
							ID:    "deep_investigation",
							Type:  StepTypeChain,
							Chain: "deep-rca",
						},
					},
				},
			},
		},
		Timeout: "45m",
	}
}

func quickTriageChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "quick-triage",
		Description: "Fast initial assessment of an issue",
		Category:    CategoryInvestigative,
		Tags:        []string{"triage", "quick", "assessment"},
		Trigger: ChainTrigger{
			Type: TriggerManual,
		},
		Steps: []ChainStep{
			{
				ID:   "gather",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "health",
						Type: StepTypeTool,
						Tool: "webb_cluster_health_full",
						Params: map[string]string{
							"context": "{{ input.cluster }}",
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
			{
				ID:   "score",
				Type: StepTypeTool,
				Tool: "webb_triage_score",
				Params: map[string]string{
					"symptoms": "{{ input.symptoms }}",
				},
			},
			{
				ID:   "suggest",
				Type: StepTypeTool,
				Tool: "webb_rca_suggest",
				Params: map[string]string{
					"symptoms": "{{ input.symptoms }}",
					"context":  "{{ steps.gather }}",
				},
			},
		},
		Timeout: "5m",
	}
}

func escalationChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "escalation",
		Description: "P0 escalation workflow with channel creation",
		Category:    CategoryInvestigative,
		Tags:        []string{"escalation", "p0", "incident"},
		Trigger: ChainTrigger{
			Type: TriggerManual,
		},
		Steps: []ChainStep{
			{
				ID:   "preflight",
				Type: StepTypeTool,
				Tool: "webb_escalation_preflight",
				Params: map[string]string{
					"incident_id": "{{ input.incident_id }}",
				},
			},
			{
				ID:          "approval_gate",
				Type:        StepTypeGate,
				GateType:    "human",
				Message:     "Confirm escalation for incident {{ input.incident_id }}?",
				GateTimeout: "5m",
				OnTimeout:   "abort",
			},
			{
				ID:   "create_channel",
				Type: StepTypeTool,
				Tool: "webb_slack_channel_create",
				Params: map[string]string{
					"name":       "inc-{{ input.incident_id }}",
					"is_private": "false",
				},
			},
			{
				ID:   "setup",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "set_topic",
						Type: StepTypeTool,
						Tool: "webb_slack_channel_topic",
						Params: map[string]string{
							"channel": "{{ steps.create_channel.channel_id }}",
							"topic":   "P0 Incident | Status: Active | Lead: TBD",
						},
					},
					{
						ID:   "invite_oncall",
						Type: StepTypeTool,
						Tool: "webb_slack_channel_invite",
						Params: map[string]string{
							"channel": "{{ steps.create_channel.channel_id }}",
							"users":   "{{ oncall_users }}",
						},
					},
				},
			},
			{
				ID:   "notify",
				Type: StepTypeTool,
				Tool: "webb_slack_post",
				Params: map[string]string{
					"channel": "#incidents",
					"message": "P0 Escalation: {{ input.incident_id }} - Bridge: {{ steps.create_channel.permalink }}",
				},
			},
		},
		Timeout: "15m",
	}
}

func contextSyncChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "context-sync",
		Description: "Sync context files with live data",
		Category:    CategoryOperational,
		Tags:        []string{"sync", "context", "automated"},
		Trigger: ChainTrigger{
			Type: TriggerScheduled,
			Cron: "0 0 * * * *",
		},
		Steps: []ChainStep{
			{
				ID:   "sync",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "sync_incidents",
						Type: StepTypeTool,
						Tool: "webb_context_update",
						Params: map[string]string{
							"file": "ongoing-incidents.md",
						},
					},
					{
						ID:   "sync_customers",
						Type: StepTypeTool,
						Tool: "webb_context_update",
						Params: map[string]string{
							"file": "customer-context.md",
						},
					},
				},
			},
		},
		Timeout: "5m",
	}
}

func autoRemediationChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "auto-remediation",
		Description: "Automated low-risk remediation for common issues",
		Category:    CategoryRemediation,
		Tags:        []string{"remediation", "automated", "self-healing"},
		Trigger: ChainTrigger{
			Type:   TriggerEvent,
			Event:  "alert.firing",
			Filter: "labels.remediation_enabled == 'true'",
		},
		Variables: map[string]string{
			"alert_name": "{{ trigger.alert.name }}",
			"cluster":    "{{ trigger.alert.cluster }}",
		},
		Steps: []ChainStep{
			{
				ID:   "assess",
				Type: StepTypeTool,
				Tool: "webb_triage_score",
				Params: map[string]string{
					"alert": "{{ alert_name }}",
				},
			},
			{
				ID:        "check_risk",
				Type:      StepTypeBranch,
				Condition: "{{ steps.assess.risk_score }} < 40",
				Branches: map[string][]ChainStep{
					"true": {
						{
							ID:   "execute_remediation",
							Type: StepTypeTool,
							Tool: "webb_field_quickfix",
							Params: map[string]string{
								"alert":   "{{ alert_name }}",
								"cluster": "{{ cluster }}",
							},
						},
						{
							ID:   "verify",
							Type: StepTypeTool,
							Tool: "webb_cluster_health_full",
							Params: map[string]string{
								"context": "{{ cluster }}",
							},
						},
					},
					"false": {
						{
							ID:          "require_approval",
							Type:        StepTypeGate,
							GateType:    "human",
							Message:     "High-risk remediation for {{ alert_name }}. Approve?",
							GateTimeout: "10m",
							OnTimeout:   "abort",
						},
					},
				},
			},
		},
		Timeout: "15m",
	}
}

func deployValidationChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "deploy-validation",
		Description: "Post-deployment validation checks",
		Category:    CategoryDevelopment,
		Tags:        []string{"deploy", "validation", "verification"},
		Trigger: ChainTrigger{
			Type:  TriggerEvent,
			Event: "deploy.completed",
		},
		Variables: map[string]string{
			"release":   "{{ trigger.deploy.release }}",
			"cluster":   "{{ trigger.deploy.cluster }}",
			"namespace": "{{ trigger.deploy.namespace }}",
		},
		Steps: []ChainStep{
			{
				ID:   "wait_stabilize",
				Type: StepTypeTool,
				Tool: "webb_k8s_rollout_status",
				Params: map[string]string{
					"context":   "{{ cluster }}",
					"namespace": "{{ namespace }}",
					"name":      "{{ release }}",
				},
			},
			{
				ID:   "validate",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "health",
						Type: StepTypeTool,
						Tool: "webb_cluster_health_full",
						Params: map[string]string{
							"context": "{{ cluster }}",
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
					{
						ID:   "events",
						Type: StepTypeTool,
						Tool: "webb_k8s_events",
						Params: map[string]string{
							"context":   "{{ cluster }}",
							"namespace": "{{ namespace }}",
						},
					},
				},
			},
			{
				ID:        "check_health",
				Type:      StepTypeBranch,
				Condition: "{{ steps.validate.health.health_score }} < 70",
				Branches: map[string][]ChainStep{
					"true": {
						{
							ID:   "alert_team",
							Type: StepTypeTool,
							Tool: "webb_slack_post",
							Params: map[string]string{
								"channel": "#pod-platform-engineering",
								"message": "Deploy {{ release }} may have issues. Health score: {{ steps.validate.health.health_score }}",
							},
						},
					},
				},
			},
		},
		Timeout: "10m",
	}
}

func prReviewChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "pr-review",
		Description: "Automated PR review and checks",
		Category:    CategoryDevelopment,
		Tags:        []string{"pr", "review", "ci"},
		Trigger: ChainTrigger{
			Type:  TriggerEvent,
			Event: "pr.opened",
		},
		Variables: map[string]string{
			"pr_number": "{{ trigger.pr.number }}",
			"repo":      "{{ trigger.pr.repo }}",
		},
		Steps: []ChainStep{
			{
				ID:   "analyze",
				Type: StepTypeTool,
				Tool: "webb_github_pr_diff",
				Params: map[string]string{
					"repo":   "{{ repo }}",
					"number": "{{ pr_number }}",
				},
			},
			{
				ID:   "check_ci",
				Type: StepTypeTool,
				Tool: "webb_github_pr_status",
				Params: map[string]string{
					"repo":   "{{ repo }}",
					"number": "{{ pr_number }}",
				},
			},
			{
				ID:        "ci_status",
				Type:      StepTypeBranch,
				Condition: "{{ steps.check_ci.all_passing }}",
				Branches: map[string][]ChainStep{
					"false": {
						{
							ID:   "wait_ci",
							Type: StepTypeTool,
							Tool: "webb_github_actions",
							Params: map[string]string{
								"repo":   "{{ repo }}",
								"branch": "{{ trigger.pr.head_ref }}",
							},
						},
					},
				},
			},
		},
		Timeout: "30m",
	}
}

func deepRCAChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "deep-rca",
		Description: "Deep root cause analysis sub-chain",
		Category:    CategoryInvestigative,
		Tags:        []string{"rca", "investigation", "deep-dive"},
		Trigger: ChainTrigger{
			Type: TriggerManual,
		},
		Steps: []ChainStep{
			{
				ID:   "gather_evidence",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "logs",
						Type: StepTypeTool,
						Tool: "webb_logs_search",
						Params: map[string]string{
							"query": "{{ input.symptoms }}",
							"hours": "24",
						},
					},
					{
						ID:   "metrics",
						Type: StepTypeTool,
						Tool: "webb_observability_correlation_full",
						Params: map[string]string{
							"time_window": "24h",
						},
					},
					{
						ID:   "deploys",
						Type: StepTypeTool,
						Tool: "webb_release_pipeline_full",
						Params: map[string]string{
							"hours": "48",
						},
					},
				},
			},
			{
				ID:   "correlate",
				Type: StepTypeTool,
				Tool: "webb_investigation_context_full",
				Params: map[string]string{
					"customer": "{{ input.customer }}",
				},
			},
			{
				ID:   "suggest_rca",
				Type: StepTypeTool,
				Tool: "webb_rca_suggest",
				Params: map[string]string{
					"evidence": "{{ steps.gather_evidence }}",
					"context":  "{{ steps.correlate }}",
				},
			},
			{
				ID:   "learn",
				Type: StepTypeTool,
				Tool: "webb_rca_learn",
				Params: map[string]string{
					"rca":     "{{ steps.suggest_rca }}",
					"outcome": "{{ input.outcome }}",
				},
			},
		},
		Timeout: "60m",
	}
}

// vaultGraphRebuildChain rebuilds the knowledge graph daily
func vaultGraphRebuildChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "vault-graph-rebuild",
		Description: "Rebuild knowledge graph from vault contents",
		Category:    CategoryOperational,
		Tags:        []string{"vault", "graph", "automation", "daily"},
		Trigger: ChainTrigger{
			Type: TriggerScheduled,
			Cron: "0 0 2 * * *", // Daily at 2am
		},
		Steps: []ChainStep{
			{
				ID:   "rebuild",
				Type: StepTypeTool,
				Tool: "webb_vault_ci_trigger",
				Params: map[string]string{
					"type": "graph_rebuild",
				},
			},
			{
				ID:        "notify_failure",
				Type:      StepTypeBranch,
				Condition: "{{ steps.rebuild.status }} == 'failed'",
				Branches: map[string][]ChainStep{
					"true": {
						{
							ID:   "slack_notify",
							Type: StepTypeTool,
							Tool: "webb_slack_post",
							Params: map[string]string{
								"channel": "#platform-discussions",
								"message": "Vault graph rebuild failed: {{ steps.rebuild.error }}",
							},
						},
					},
				},
			},
		},
		Timeout: "30m",
	}
}

// vaultOrphanLinkChain finds and reports orphan files weekly
func vaultOrphanLinkChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "vault-orphan-link",
		Description: "Find and link orphan vault files",
		Category:    CategoryOperational,
		Tags:        []string{"vault", "orphan", "automation", "weekly"},
		Trigger: ChainTrigger{
			Type: TriggerScheduled,
			Cron: "0 0 3 * * 0", // Weekly Sunday 3am
		},
		Steps: []ChainStep{
			{
				ID:   "find_orphans",
				Type: StepTypeTool,
				Tool: "webb_vault_ci_trigger",
				Params: map[string]string{
					"type": "orphan_linking",
				},
			},
			{
				ID:   "auto_link",
				Type: StepTypeTool,
				Tool: "webb_graph_auto_link",
			},
		},
		Timeout: "30m",
	}
}

// vaultFreshnessChain checks content freshness weekly
func vaultFreshnessChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "vault-freshness-check",
		Description: "Check vault content freshness and flag stale files",
		Category:    CategoryOperational,
		Tags:        []string{"vault", "freshness", "automation", "weekly"},
		Trigger: ChainTrigger{
			Type: TriggerScheduled,
			Cron: "0 0 4 * * 0", // Weekly Sunday 4am
		},
		Steps: []ChainStep{
			{
				ID:   "check",
				Type: StepTypeTool,
				Tool: "webb_vault_ci_trigger",
				Params: map[string]string{
					"type": "freshness_check",
				},
			},
			{
				ID:        "alert_stale",
				Type:      StepTypeBranch,
				Condition: "{{ steps.check.metrics.stale_count }} > 100",
				Branches: map[string][]ChainStep{
					"true": {
						{
							ID:   "slack_notify",
							Type: StepTypeTool,
							Tool: "webb_slack_post",
							Params: map[string]string{
								"channel": "#platform-discussions",
								"message": "High stale content: {{ steps.check.metrics.stale_count }} files need review",
							},
						},
					},
				},
			},
		},
		Timeout: "30m",
	}
}

// oncallActionReviewChain generates hourly action plans during on-call shifts
func oncallActionReviewChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "oncall-action-review",
		Description: "Hourly on-call action plan review - aggregates incidents, mentions, tickets, PRs into prioritized list",
		Category:    CategoryOperational,
		Tags:        []string{"oncall", "action-plan", "hourly", "workflow", "automation"},
		Trigger: ChainTrigger{
			Type: TriggerScheduled,
			Cron: "0 0 9-17 * * 1-5", // Every hour 9am-5pm weekdays
		},
		Variables: map[string]string{
			"user":       "mitch",
			"time_range": "1h",
		},
		Steps: []ChainStep{
			{
				ID:   "check_oncall",
				Type: StepTypeTool,
				Tool: "webb_oncall_current",
				Params: map[string]string{
					"schedule": "Platform Team",
				},
			},
			{
				ID:        "is_primary_oncall",
				Type:      StepTypeBranch,
				Condition: "{{ steps.check_oncall.primary_user }} == '{{ user }}'",
				Branches: map[string][]ChainStep{
					"true": {
						{
							ID:   "generate_action_plan",
							Type: StepTypeTool,
							Tool: "webb_oncall_action_plan",
							Params: map[string]string{
								"user":       "{{ user }}",
								"time_range": "{{ time_range }}",
								"limit":      "25",
							},
						},
						{
							ID:        "check_urgency",
							Type:      StepTypeBranch,
							Condition: "{{ steps.generate_action_plan.health_score }} < 60",
							Branches: map[string][]ChainStep{
								"true": {
									{
										ID:   "post_urgent",
										Type: StepTypeTool,
										Tool: "webb_slack_post",
										Params: map[string]string{
											"channel": "#platform-discussions",
											"message": "On-call action plan health: {{ steps.generate_action_plan.health_score }}/100 ({{ steps.generate_action_plan.status }}). {{ steps.generate_action_plan.total_items }} items - {{ steps.generate_action_plan.summary.high_priority }} high priority.",
										},
									},
								},
							},
						},
						{
							ID:   "save_to_vault",
							Type: StepTypeTool,
							Tool: "webb_obsidian_append",
							Params: map[string]string{
								"path":    "oncall/action-plans.md",
								"content": "## {{ now.Format \"2006-01-02 15:04\" }}\n\nHealth: {{ steps.generate_action_plan.health_score }}/100 | Items: {{ steps.generate_action_plan.total_items }}\n\n---\n",
							},
						},
					},
					"false": {
						// Not primary on-call, skip this run
					},
				},
			},
		},
		Timeout: "5m",
	}
}

// mlPipelineInvestigationChain investigates ML pipeline issues across Databricks, HuggingFace, and K8s
func mlPipelineInvestigationChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "ml-pipeline-investigation",
		Description: "Investigate ML pipeline issues across Databricks jobs, HuggingFace models, and K8s infrastructure",
		Category:    CategoryInvestigative,
		Tags:        []string{"ml", "pipeline", "databricks", "huggingface", "inference", "investigation"},
		Trigger: ChainTrigger{
			Type:   TriggerEvent,
			Event:  "alert.firing",
			Filter: "component in ['inference', 'ml-pipeline', 'model-serving', 'databricks', 'huggingface']",
		},
		Variables: map[string]string{
			"alert_name": "{{ trigger.alert.name }}",
			"cluster":    "{{ trigger.alert.cluster }}",
			"customer":   "{{ trigger.alert.customer }}",
		},
		Steps: []ChainStep{
			{
				ID:   "gather_context",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "databricks_jobs",
						Type: StepTypeTool,
						Tool: "webb_databricks_jobs",
						Params: map[string]string{
							"limit": "20",
						},
					},
					{
						ID:   "huggingface_models",
						Type: StepTypeTool,
						Tool: "webb_huggingface_models",
						Params: map[string]string{
							"limit": "10",
						},
					},
					{
						ID:   "k8s_health",
						Type: StepTypeTool,
						Tool: "webb_cluster_health_full",
						Params: map[string]string{
							"context": "{{ cluster }}",
						},
					},
					{
						ID:   "integration_health",
						Type: StepTypeTool,
						Tool: "webb_integration_health_full",
						Params: map[string]string{
							"context":             "{{ cluster }}",
							"include_databricks":  "true",
							"include_huggingface": "true",
						},
					},
				},
			},
			{
				ID:        "check_databricks",
				Type:      StepTypeBranch,
				Condition: "{{ steps.gather_context.integration_health.databricks.has_failed_jobs }}",
				Branches: map[string][]ChainStep{
					"true": {
						{
							ID:   "investigate_failed_job",
							Type: StepTypeTool,
							Tool: "webb_databricks_job_runs",
							Params: map[string]string{
								"job_id": "{{ steps.gather_context.integration_health.databricks.first_failed_job_id }}",
								"limit":  "5",
							},
						},
					},
				},
			},
			{
				ID:   "correlate",
				Type: StepTypeTool,
				Tool: "webb_xref_auto_detect",
				Params: map[string]string{
					"text": "{{ steps.gather_context }}",
				},
			},
			{
				ID:   "build_timeline",
				Type: StepTypeTool,
				Tool: "webb_infrastructure_timeline",
				Params: map[string]string{
					"customer":           "{{ customer }}",
					"context":            "{{ cluster }}",
					"time_range":         "6h",
					"include_databricks": "true",
					"include_k8s":        "true",
					"include_alerts":     "true",
				},
			},
			{
				ID:   "suggest_rca",
				Type: StepTypeTool,
				Tool: "webb_rca_suggest",
				Params: map[string]string{
					"symptoms": "ML pipeline alert: {{ alert_name }}",
					"evidence": "{{ steps.gather_context }}",
					"timeline": "{{ steps.build_timeline }}",
				},
			},
			{
				ID:        "notify_if_critical",
				Type:      StepTypeBranch,
				Condition: "{{ steps.gather_context.k8s_health.health_score }} < 50",
				Branches: map[string][]ChainStep{
					"true": {
						{
							ID:   "slack_notify",
							Type: StepTypeTool,
							Tool: "webb_slack_post",
							Params: map[string]string{
								"channel": "#platform-discussions",
								"message": "ML Pipeline Alert: {{ alert_name }}\nCluster health: {{ steps.gather_context.k8s_health.health_score }}/100\nIntegration health: {{ steps.gather_context.integration_health.health_score }}/100\nPotential root cause: {{ steps.suggest_rca.top_hypothesis }}",
							},
						},
					},
				},
			},
		},
		Timeout: "15m",
	}
}

// costAnomalyInvestigationChain investigates cost threshold breaches across AWS and GCP
func costAnomalyInvestigationChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "cost-anomaly-investigation",
		Description: "Investigate cost threshold breach and identify root cause",
		Category:    CategoryInvestigative,
		Tags:        []string{"cost", "anomaly", "investigation", "aws", "gcp", "finops"},
		Trigger: ChainTrigger{
			Type:   TriggerEvent,
			Event:  "cost.threshold.exceeded",
			Filter: "variance_percent > 20",
		},
		Variables: map[string]string{
			"threshold_percent": "{{ trigger.threshold_percent }}",
			"service":           "{{ trigger.service }}",
			"provider":          "{{ trigger.provider }}",
		},
		Steps: []ChainStep{
			{
				ID:   "gather_costs",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "aws_costs",
						Type: StepTypeTool,
						Tool: "webb_aws_cost_drivers",
						Params: map[string]string{
							"days": "7",
						},
					},
					{
						ID:   "gcp_costs",
						Type: StepTypeTool,
						Tool: "webb_gcp_cost_drivers",
						Params: map[string]string{
							"days": "7",
						},
					},
					{
						ID:   "cost_summary",
						Type: StepTypeTool,
						Tool: "webb_cost_summary",
					},
				},
			},
			{
				ID:   "detect_anomalies",
				Type: StepTypeTool,
				Tool: "webb_cost_anomalies",
				Params: map[string]string{
					"threshold_percent": "{{ threshold_percent }}",
				},
			},
			{
				ID:        "investigate_drivers",
				Type:      StepTypeBranch,
				Condition: "{{ len(steps.detect_anomalies.anomalies) }} > 0",
				Branches: map[string][]ChainStep{
					"true": {
						{
							ID:   "cascade_analysis",
							Type: StepTypeTool,
							Tool: "webb_cost_anomaly_cascade",
							Params: map[string]string{
								"anomalies": "{{ steps.detect_anomalies.anomalies }}",
							},
						},
						{
							ID:   "waste_detection",
							Type: StepTypeTool,
							Tool: "webb_cost_waste_detect",
						},
					},
				},
			},
			{
				ID:   "suggest_optimization",
				Type: StepTypeTool,
				Tool: "webb_aws_cost_optimize",
			},
			{
				ID:   "notify",
				Type: StepTypeTool,
				Tool: "webb_slack_post",
				Params: map[string]string{
					"channel": "#platform-discussions",
					"message": "Cost Anomaly Detected\nService: {{ service }} ({{ provider }})\nVariance: {{ threshold_percent }}%\nTop drivers: {{ steps.gather_costs.aws_costs.top_3 }}\nRecommendations: {{ steps.suggest_optimization.summary }}",
				},
			},
		},
		Timeout: "10m",
	}
}

// securityIncidentResponseChain handles automated security incident triage and containment
func securityIncidentResponseChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "security-incident-response",
		Description: "Automated security incident triage and containment workflow",
		Category:    CategoryRemediation,
		Tags:        []string{"security", "incident", "triage", "containment", "vulnerability"},
		Trigger: ChainTrigger{
			Type:   TriggerEvent,
			Event:  "security.alert.critical",
			Filter: "severity in ['critical', 'high']",
		},
		Variables: map[string]string{
			"alert_id":   "{{ trigger.alert.id }}",
			"alert_type": "{{ trigger.alert.type }}",
			"cluster":    "{{ trigger.alert.cluster }}",
		},
		Steps: []ChainStep{
			{
				ID:   "assess_posture",
				Type: StepTypeTool,
				Tool: "webb_security_audit_full",
				Params: map[string]string{
					"context": "{{ cluster }}",
				},
			},
			{
				ID:   "check_secrets",
				Type: StepTypeTool,
				Tool: "webb_secret_expiry_check",
				Params: map[string]string{
					"context": "{{ cluster }}",
				},
			},
			{
				ID:   "audit_network",
				Type: StepTypeTool,
				Tool: "webb_networkpolicy_audit",
				Params: map[string]string{
					"context": "{{ cluster }}",
				},
			},
			{
				ID:   "scan_vulnerabilities",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "dependabot",
						Type: StepTypeTool,
						Tool: "webb_pr_security_scan",
						Params: map[string]string{
							"repo": "hairglasses/acme",
						},
					},
					{
						ID:   "codeql",
						Type: StepTypeTool,
						Tool: "webb_code_scan_alerts",
						Params: map[string]string{
							"repo": "hairglasses/acme",
						},
					},
				},
			},
			{
				ID:          "notify_security",
				Type:        StepTypeGate,
				GateType:    "human",
				Message:     "Security incident {{ alert_id }} ({{ alert_type }}) requires review.\nPosture score: {{ steps.assess_posture.health_score }}/100\nExpiring secrets: {{ steps.check_secrets.expiring_count }}\nNetwork gaps: {{ steps.audit_network.unprotected_count }}\n\nApprove to proceed with containment?",
				GateTimeout: "15m",
				OnTimeout:   "abort",
			},
			{
				ID:   "create_incident",
				Type: StepTypeTool,
				Tool: "webb_incidentio_create",
				Params: map[string]string{
					"name":     "Security Alert: {{ alert_type }}",
					"severity": "critical",
					"summary":  "Automated security response for {{ alert_id }}. Posture score: {{ steps.assess_posture.health_score }}/100",
				},
			},
		},
		Timeout: "30m",
	}
}

// migrationValidationChain validates pre/post migration health
func migrationValidationChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "migration-validation",
		Description: "Pre/post migration health verification with data integrity checks",
		Category:    CategoryOperational,
		Tags:        []string{"migration", "validation", "preflight", "health", "data-integrity"},
		Trigger: ChainTrigger{
			Type: TriggerManual,
		},
		Variables: map[string]string{
			"customer":       "{{ input.customer }}",
			"source_cluster": "{{ input.source_cluster }}",
			"target_cluster": "{{ input.target_cluster }}",
			"migration_type": "{{ input.migration_type }}",
		},
		Steps: []ChainStep{
			{
				ID:   "preflight_checks",
				Type: StepTypeTool,
				Tool: "webb_preflight_full",
				Params: map[string]string{
					"context":  "{{ target_cluster }}",
					"customer": "{{ customer }}",
				},
			},
			{
				ID:   "baseline_health",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "source_health",
						Type: StepTypeTool,
						Tool: "webb_cluster_health_full",
						Params: map[string]string{
							"context": "{{ source_cluster }}",
						},
					},
					{
						ID:   "target_health",
						Type: StepTypeTool,
						Tool: "webb_cluster_health_full",
						Params: map[string]string{
							"context": "{{ target_cluster }}",
						},
					},
					{
						ID:   "database_health",
						Type: StepTypeTool,
						Tool: "webb_database_health_full",
						Params: map[string]string{
							"context": "{{ source_cluster }}",
						},
					},
				},
			},
			{
				ID:          "wait_for_migration",
				Type:        StepTypeGate,
				GateType:    "human",
				Message:     "Pre-migration health captured.\nSource: {{ steps.baseline_health.source_health.health_score }}/100\nTarget: {{ steps.baseline_health.target_health.health_score }}/100\nPreflight: {{ steps.preflight_checks.health_score }}/100 ({{ steps.preflight_checks.passed_count }}/{{ steps.preflight_checks.total_count }} passed)\n\nProceed with migration validation when ready.",
				GateTimeout: "4h",
				OnTimeout:   "abort",
			},
			{
				ID:   "post_migration_health",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "target_health_post",
						Type: StepTypeTool,
						Tool: "webb_cluster_health_full",
						Params: map[string]string{
							"context": "{{ target_cluster }}",
						},
					},
					{
						ID:   "database_health_post",
						Type: StepTypeTool,
						Tool: "webb_database_health_full",
						Params: map[string]string{
							"context": "{{ target_cluster }}",
						},
					},
					{
						ID:   "queue_health",
						Type: StepTypeTool,
						Tool: "webb_queue_health_full",
						Params: map[string]string{
							"context": "{{ target_cluster }}",
						},
					},
				},
			},
			{
				ID:   "validate_data",
				Type: StepTypeTool,
				Tool: "webb_migration_progress",
				Params: map[string]string{
					"customer": "{{ customer }}",
				},
			},
			{
				ID:   "report",
				Type: StepTypeTool,
				Tool: "webb_slack_post",
				Params: map[string]string{
					"channel": "#pod-platform-engineering",
					"message": "Migration Validation Complete: {{ customer }}\nSource -> Target: {{ source_cluster }} -> {{ target_cluster }}\n\nPre-migration health: {{ steps.baseline_health.target_health.health_score }}/100\nPost-migration health: {{ steps.post_migration_health.target_health_post.health_score }}/100\nMigration progress: {{ steps.validate_data.progress_percent }}%\n\nDatabase: {{ steps.post_migration_health.database_health_post.health_score }}/100\nQueues: {{ steps.post_migration_health.queue_health.health_score }}/100",
				},
			},
		},
		Timeout: "6h",
	}
}

// performanceOptimizationChain runs proactive performance analysis and generates recommendations
func performanceOptimizationChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "performance-optimization",
		Description: "Proactive performance analysis with actionable recommendations",
		Category:    CategoryOperational,
		Tags:        []string{"performance", "optimization", "weekly", "proactive", "recommendations"},
		Trigger: ChainTrigger{
			Type: TriggerScheduled,
			Cron: "0 0 6 * * 1", // Weekly Monday 6am
		},
		Variables: map[string]string{
			"context":    "headspace-v2",
			"time_range": "7d",
		},
		Steps: []ChainStep{
			{
				ID:   "gather_metrics",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "clickhouse_queries",
						Type: StepTypeTool,
						Tool: "webb_clickhouse_query_analyze",
						Params: map[string]string{
							"context": "{{ context }}",
						},
					},
					{
						ID:   "queue_health",
						Type: StepTypeTool,
						Tool: "webb_queue_health_full",
						Params: map[string]string{
							"context": "{{ context }}",
						},
					},
					{
						ID:   "pod_resources",
						Type: StepTypeTool,
						Tool: "webb_resource_audit",
						Params: map[string]string{
							"context": "{{ context }}",
						},
					},
					{
						ID:   "db_slow_queries",
						Type: StepTypeTool,
						Tool: "webb_db_slow_query_patterns",
						Params: map[string]string{
							"context": "{{ context }}",
						},
					},
				},
			},
			{
				ID:   "analyze_patterns",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "db_index_suggestions",
						Type: StepTypeTool,
						Tool: "webb_db_index_suggestions",
						Params: map[string]string{
							"context": "{{ context }}",
						},
					},
					{
						ID:   "scaling_audit",
						Type: StepTypeTool,
						Tool: "webb_scaling_audit",
						Params: map[string]string{
							"context": "{{ context }}",
						},
					},
				},
			},
			{
				ID:   "identify_bottlenecks",
				Type: StepTypeTool,
				Tool: "webb_performance_diagnosis",
				Params: map[string]string{
					"context":    "{{ context }}",
					"time_range": "{{ time_range }}",
				},
			},
			{
				ID:   "suggest_optimizations",
				Type: StepTypeTool,
				Tool: "webb_aws_rightsizing",
			},
			{
				ID:        "create_ticket",
				Type:      StepTypeBranch,
				Condition: "{{ steps.identify_bottlenecks.health_score }} < 80",
				Branches: map[string][]ChainStep{
					"true": {
						{
							ID:   "create_story",
							Type: StepTypeTool,
							Tool: "webb_shortcut_create",
							Params: map[string]string{
								"name":        "Performance Optimization: {{ context }} ({{ now.Format \"2006-01-02\" }})",
								"description": "## Weekly Performance Analysis\n\n**Health Score:** {{ steps.identify_bottlenecks.health_score }}/100\n\n### Bottlenecks\n{{ steps.identify_bottlenecks.summary }}\n\n### Recommendations\n- DB Indexes: {{ steps.analyze_patterns.db_index_suggestions.count }} suggestions\n- Scaling: {{ steps.analyze_patterns.scaling_audit.summary }}\n- Rightsizing: {{ steps.suggest_optimizations.summary }}\n\n### Queue Health\n{{ steps.gather_metrics.queue_health.summary }}",
								"story_type":  "chore",
								"labels":      "performance,optimization,automated",
							},
						},
						{
							ID:   "notify",
							Type: StepTypeTool,
							Tool: "webb_slack_post",
							Params: map[string]string{
								"channel": "#platform-discussions",
								"message": "Weekly Performance Report: {{ context }}\nHealth: {{ steps.identify_bottlenecks.health_score }}/100\nSlow queries: {{ steps.gather_metrics.db_slow_queries.count }}\nQueue backlogs: {{ steps.gather_metrics.queue_health.backlog_count }}\nTicket created: {{ steps.create_story.url }}",
							},
						},
					},
					"false": {
						{
							ID:   "notify_healthy",
							Type: StepTypeTool,
							Tool: "webb_slack_post",
							Params: map[string]string{
								"channel": "#platform-discussions",
								"message": "Weekly Performance Report: {{ context }} - Healthy ({{ steps.identify_bottlenecks.health_score }}/100). No action required.",
							},
						},
					},
				},
			},
		},
		Timeout: "20m",
	}
}

// Phase 3 chains (v132) - Customer lifecycle, incident lifecycle, and continuous improvement

func customerHealthAlertChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "customer-health-alert",
		Description: "Proactive daily alert for at-risk customers based on sentiment and ticket trends",
		Category:    CategoryCustomer,
		Tags:        []string{"customer", "health", "proactive", "alert", "sentiment"},
		Trigger: ChainTrigger{
			Type: TriggerScheduled,
			Cron: "0 0 9 * * 1-5",
		},
		Input: []ChainInput{
			{Name: "notification_channel", Type: "string", Required: false, Default: "#customer-health", Description: "Slack channel for notifications"},
		},
		OnError: &ErrorHandler{
			Action: "continue",
			Notify: "#ops-alerts",
		},
		Steps: []ChainStep{
			{
				ID:   "gather_sentiment",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "sentiment_trends",
						Type: StepTypeTool,
						Tool: "webb_sentiment_trends",
						Retry: &RetryPolicy{
							MaxAttempts: 3,
							Delay:       "5s",
							BackoffRate: 1.5,
						},
					},
					{
						ID:   "sentiment_alerts",
						Type: StepTypeTool,
						Tool: "webb_sentiment_alerts",
						Retry: &RetryPolicy{
							MaxAttempts: 3,
							Delay:       "5s",
							BackoffRate: 1.5,
						},
					},
				},
			},
			{
				ID:        "filter_at_risk",
				Type:      StepTypeBranch,
				Condition: "{{ len(steps.gather_sentiment.sentiment_alerts.alerts) }} > 0",
				Branches: map[string][]ChainStep{
					"true": {
						{
							ID:   "deep_dive",
							Type: StepTypeParallel,
							Steps: []ChainStep{
								{
									ID:   "customer_sentiment",
									Type: StepTypeTool,
									Tool: "webb_customer_sentiment",
									Params: map[string]string{
										"customer": "{{ range steps.gather_sentiment.sentiment_alerts.alerts }}{{ .customer }}{{ end }}",
									},
								},
								{
									ID:   "ticket_summary",
									Type: StepTypeTool,
									Tool: "webb_ticket_summary",
									Params: map[string]string{
										"customer": "{{ range steps.gather_sentiment.sentiment_alerts.alerts }}{{ .customer }}{{ end }}",
									},
								},
							},
						},
						{
							ID:   "notify",
							Type: StepTypeTool,
							Tool: "webb_slack_post",
							Params: map[string]string{
								"channel": "{{ notification_channel }}",
								"message": "Customer Health Alert\n\nAt-risk customers detected:\n{{ range steps.gather_sentiment.sentiment_alerts.alerts }}\n- {{ .customer }}: Score {{ .score }}/100 ({{ .trend }})\n{{ end }}\n\nReview recommended for proactive outreach.",
							},
						},
						{
							ID:   "document",
							Type: StepTypeTool,
							Tool: "webb_vault_note",
							Params: map[string]string{
								"type":    "customer-health",
								"title":   "Customer Health Alert - {{ now.Format \"2006-01-02\" }}",
								"content": "## At-Risk Customers\n\n{{ range steps.gather_sentiment.sentiment_alerts.alerts }}\n### {{ .customer }}\n- Score: {{ .score }}/100\n- Trend: {{ .trend }}\n- Open Tickets: {{ steps.deep_dive.ticket_summary.total }}\n{{ end }}",
								"tags":    "customer,health,alert",
							},
						},
					},
					"false": {
						{
							ID:   "notify_healthy",
							Type: StepTypeTool,
							Tool: "webb_slack_post",
							Params: map[string]string{
								"channel": "{{ notification_channel }}",
								"message": "Customer Health Check Complete\n\nAll customers healthy - no at-risk customers detected.\n\nNext check: Tomorrow 9am",
							},
						},
					},
				},
			},
		},
		Timeout: "10m",
	}
}

func incidentAutoPostmortemChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "incident-auto-postmortem",
		Description: "Automatically generate postmortem documentation from incident threads",
		Category:    CategoryOperational,
		Tags:        []string{"incident", "postmortem", "documentation", "rca"},
		Trigger: ChainTrigger{
			Type:  TriggerEvent,
			Event: "incident.status.closed",
		},
		Input: []ChainInput{
			{Name: "incident_id", Type: "string", Required: true, Description: "Incident ID from incident.io"},
			{Name: "thread_id", Type: "string", Required: false, Description: "Slack thread ID (optional)"},
		},
		OnError: &ErrorHandler{
			Action: "continue",
			Notify: "#ops-alerts",
		},
		Steps: []ChainStep{
			{
				ID:   "get_incident",
				Type: StepTypeTool,
				Tool: "webb_incidentio_get",
				Params: map[string]string{
					"incident_id": "{{ incident_id }}",
				},
				Retry: &RetryPolicy{
					MaxAttempts: 3,
					Delay:       "5s",
					BackoffRate: 1.5,
				},
			},
			{
				ID:   "get_thread",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "thread_content",
						Type: StepTypeTool,
						Tool: "webb_incident_thread_get",
						Params: map[string]string{
							"incident_id": "{{ incident_id }}",
						},
						Retry: &RetryPolicy{
							MaxAttempts: 3,
							Delay:       "5s",
							BackoffRate: 1.5,
						},
					},
					{
						ID:   "thread_timeline",
						Type: StepTypeTool,
						Tool: "webb_incident_thread_timeline",
						Params: map[string]string{
							"incident_id": "{{ incident_id }}",
						},
						Retry: &RetryPolicy{
							MaxAttempts: 3,
							Delay:       "5s",
							BackoffRate: 1.5,
						},
					},
				},
			},
			{
				ID:   "find_similar",
				Type: StepTypeTool,
				Tool: "webb_similar_incidents",
				Params: map[string]string{
					"incident_id": "{{ incident_id }}",
					"limit":       "5",
				},
			},
			{
				ID:   "extract_rca",
				Type: StepTypeTool,
				Tool: "webb_rca_learn",
				Params: map[string]string{
					"incident_id": "{{ incident_id }}",
					"timeline":    "{{ steps.get_thread.thread_timeline.timeline }}",
				},
				Retry: &RetryPolicy{
					MaxAttempts: 2,
					Delay:       "10s",
					BackoffRate: 2.0,
				},
			},
			{
				ID:   "generate_runbook",
				Type: StepTypeTool,
				Tool: "webb_runbook_generate",
				Params: map[string]string{
					"incident_id": "{{ incident_id }}",
					"symptoms":    "{{ steps.extract_rca.symptoms }}",
					"resolution":  "{{ steps.extract_rca.resolution }}",
				},
			},
			{
				ID:   "create_docs",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "gdoc_postmortem",
						Type: StepTypeTool,
						Tool: "webb_gdocs_create",
						Params: map[string]string{
							"template": "incident_report",
							"title":    "Postmortem: {{ steps.get_incident.title }} ({{ incident_id }})",
							"data":     "{{ steps.extract_rca | json }}",
						},
					},
					{
						ID:   "confluence_page",
						Type: StepTypeTool,
						Tool: "webb_confluence_create",
						Params: map[string]string{
							"title":     "Postmortem: {{ steps.get_incident.title }}",
							"space_key": "ENG",
							"content":   "## Incident Summary\n\n**ID:** {{ incident_id }}\n**Title:** {{ steps.get_incident.title }}\n**Severity:** {{ steps.get_incident.severity }}\n\n## Similar Incidents\n\n{{ range steps.find_similar.incidents }}\n- {{ .id }}: {{ .title }}\n{{ end }}\n\n## Timeline\n\n{{ steps.get_thread.thread_timeline.summary }}\n\n## Root Cause\n\n{{ steps.extract_rca.root_cause }}\n\n## Resolution\n\n{{ steps.extract_rca.resolution }}\n\n## Action Items\n\n{{ range steps.extract_rca.action_items }}\n- {{ . }}\n{{ end }}",
							"labels":    "postmortem,incident,{{ incident_id }}",
						},
					},
					{
						ID:   "vault_archive",
						Type: StepTypeTool,
						Tool: "webb_vault_note",
						Params: map[string]string{
							"type":    "postmortem",
							"title":   "{{ incident_id }} - {{ steps.get_incident.title }}",
							"content": "## Postmortem\n\n{{ steps.extract_rca.summary }}\n\n## Links\n- GDoc: {{ steps.create_docs.gdoc_postmortem.url }}\n- Confluence: {{ steps.create_docs.confluence_page.url }}",
							"tags":    "postmortem,incident,{{ steps.get_incident.severity }}",
						},
					},
				},
			},
			{
				ID:   "link_knowledge",
				Type: StepTypeTool,
				Tool: "webb_graph_link",
				Params: map[string]string{
					"source":       "incident:{{ incident_id }}",
					"target":       "runbook:{{ steps.generate_runbook.runbook_id }}",
					"relationship": "resolved_by",
				},
			},
			{
				ID:          "notify_gate",
				Type:        StepTypeGate,
				GateType:    "approval",
				Message:     "Engineer reviews before publishing postmortem. Auto-publishes after timeout.",
				GateTimeout: "24h",
				OnTimeout:   "continue",
			},
			{
				ID:   "notify_complete",
				Type: StepTypeTool,
				Tool: "webb_slack_post",
				Params: map[string]string{
					"channel": "#incidents",
					"message": "Postmortem Published\n\nIncident: {{ incident_id }} - {{ steps.get_incident.title }}\n\nDocumentation:\n- GDoc: {{ steps.create_docs.gdoc_postmortem.url }}\n- Confluence: {{ steps.create_docs.confluence_page.url }}\n- Runbook: {{ steps.generate_runbook.runbook_id }}",
				},
			},
		},
		Timeout: "30m",
	}
}

func customerCostAnomalyChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "customer-cost-anomaly",
		Description: "Detect and investigate customer-specific cost anomalies",
		Category:    CategoryCustomer,
		Tags:        []string{"customer", "cost", "anomaly", "finance"},
		Trigger: ChainTrigger{
			Type: TriggerScheduled,
			Cron: "0 0 8 * * 1-5",
		},
		Input: []ChainInput{
			{Name: "notification_channel", Type: "string", Required: false, Default: "#finance", Description: "Slack channel for cost alerts"},
			{Name: "threshold", Type: "string", Required: false, Default: "1.5", Description: "Cost deviation threshold multiplier"},
		},
		OnError: &ErrorHandler{
			Action: "continue",
			Notify: "#ops-alerts",
		},
		Steps: []ChainStep{
			{
				ID:   "find_outliers",
				Type: StepTypeTool,
				Tool: "webb_cost_outliers",
				Params: map[string]string{
					"threshold": "{{ threshold }}",
				},
				Retry: &RetryPolicy{
					MaxAttempts: 3,
					Delay:       "10s",
					BackoffRate: 1.5,
				},
			},
			{
				ID:        "has_outliers",
				Type:      StepTypeBranch,
				Condition: "{{ len(steps.find_outliers.outliers) }} > 0",
				Branches: map[string][]ChainStep{
					"true": {
						{
							ID:   "investigate_outliers",
							Type: StepTypeParallel,
							Steps: []ChainStep{
								{
									ID:   "cost_trends",
									Type: StepTypeTool,
									Tool: "webb_customer_cost_trend",
									Params: map[string]string{
										"customer":   "{{ steps.find_outliers.outliers | pluck \"customer\" | join \",\" }}",
										"time_range": "90d",
									},
									Retry: &RetryPolicy{
										MaxAttempts: 2,
										Delay:       "5s",
										BackoffRate: 1.5,
									},
								},
								{
									ID:   "cost_alerts",
									Type: StepTypeTool,
									Tool: "webb_customer_cost_alert",
									Params: map[string]string{
										"customer": "{{ steps.find_outliers.outliers | pluck \"customer\" | join \",\" }}",
									},
									Retry: &RetryPolicy{
										MaxAttempts: 2,
										Delay:       "5s",
										BackoffRate: 1.5,
									},
								},
								{
									ID:   "benchmarks",
									Type: StepTypeTool,
									Tool: "webb_cost_benchmark",
								},
							},
						},
						{
							ID:        "filter_significant",
							Type:      StepTypeBranch,
							Condition: "{{ steps.investigate_outliers.cost_alerts.significant_count }} > 0",
							Branches: map[string][]ChainStep{
								"true": {
									{
										ID:   "deep_investigation",
										Type: StepTypeTool,
										Tool: "webb_cost_investigation",
										Params: map[string]string{
											"time_range": "30d",
											"customers":  "{{ steps.find_outliers.outliers | pluck \"customer\" | join \",\" }}",
										},
									},
									{
										ID:   "document",
										Type: StepTypeTool,
										Tool: "webb_vault_note",
										Params: map[string]string{
											"type":    "cost-investigation",
											"title":   "Cost Anomaly Investigation - {{ now.Format \"2006-01-02\" }}",
											"content": "## Cost Anomalies Detected\n\n{{ range steps.find_outliers.outliers }}\n### {{ .customer }}\n- Current Cost: ${{ .current_cost }}\n- Expected: ${{ .expected_cost }}\n- Deviation: {{ .deviation_percent }}%\n{{ end }}\n\n## Top Drivers\n\n{{ steps.deep_investigation.summary }}",
											"tags":    "cost,anomaly,investigation",
										},
									},
									{
										ID:   "notify_finance",
										Type: StepTypeTool,
										Tool: "webb_slack_post",
										Params: map[string]string{
											"channel": "{{ notification_channel }}",
											"message": "Cost Anomaly Alert\n\n{{ len(steps.find_outliers.outliers) }} customer(s) with significant cost deviations detected.\n\nTop outliers:\n{{ range steps.find_outliers.outliers }}\n- {{ .customer }}: {{ .deviation_percent }}% above expected\n{{ end }}\n\nInvestigation documented in vault.",
										},
									},
									{
										ID:          "escalate_gate",
										Type:        StepTypeGate,
										GateType:    "approval",
										Message:     "Review before recommending customer actions. Auto-archives after timeout.",
										GateTimeout: "48h",
										OnTimeout:   "continue",
									},
								},
								"false": {
									{
										ID:   "notify_minor_anomalies",
										Type: StepTypeTool,
										Tool: "webb_slack_post",
										Params: map[string]string{
											"channel": "{{ notification_channel }}",
											"message": "Cost Monitoring Update\n\n{{ len(steps.find_outliers.outliers) }} outlier(s) detected but none exceed significance threshold.\n\nNo action required - continuing monitoring.",
										},
									},
								},
							},
						},
					},
					"false": {
						{
							ID:   "notify_healthy",
							Type: StepTypeTool,
							Tool: "webb_slack_post",
							Params: map[string]string{
								"channel": "{{ notification_channel }}",
								"message": "Cost Health Check: All Clear\n\nNo cost anomalies detected across customer base.\nThreshold: {{ threshold }}x expected cost.",
							},
						},
					},
				},
			},
		},
		Timeout: "15m",
	}
}

func customerOnboardingChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "customer-onboarding",
		Description: "Automated customer onboarding with health baseline and documentation",
		Category:    CategoryCustomer,
		Tags:        []string{"customer", "onboarding", "documentation", "baseline"},
		Trigger: ChainTrigger{
			Type: TriggerManual,
		},
		Input: []ChainInput{
			{Name: "customer", Type: "string", Required: true, Description: "Customer name"},
			{Name: "cluster", Type: "string", Required: true, Description: "Cluster context for the customer"},
			{Name: "tier", Type: "string", Required: false, Default: "standard", Description: "Customer tier (enterprise, growth, startup, standard)"},
			{Name: "notification_channel", Type: "string", Required: false, Default: "#customer-success", Description: "Slack channel for CSM notifications"},
		},
		OnError: &ErrorHandler{
			Action: "continue",
			Notify: "#ops-alerts",
		},
		Steps: []ChainStep{
			{
				ID:   "validate_cluster",
				Type: StepTypeTool,
				Tool: "webb_k8s_verify_cluster",
				Params: map[string]string{
					"context": "{{ cluster }}",
				},
				Retry: &RetryPolicy{
					MaxAttempts: 2,
					Delay:       "5s",
					BackoffRate: 1.5,
				},
			},
			{
				ID:   "get_config",
				Type: StepTypeTool,
				Tool: "webb_customer_get",
				Params: map[string]string{
					"customer": "{{ customer }}",
				},
				Retry: &RetryPolicy{
					MaxAttempts: 2,
					Delay:       "5s",
					BackoffRate: 1.5,
				},
			},
			{
				ID:   "baseline",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "customer_snapshot",
						Type: StepTypeTool,
						Tool: "webb_customer_snapshot",
						Params: map[string]string{
							"customer": "{{ customer }}",
						},
						Retry: &RetryPolicy{
							MaxAttempts: 2,
							Delay:       "5s",
							BackoffRate: 1.5,
						},
					},
					{
						ID:   "customer_lifecycle",
						Type: StepTypeTool,
						Tool: "webb_customer_lifecycle",
						Params: map[string]string{
							"customer": "{{ customer }}",
						},
					},
					{
						ID:   "engagement_context",
						Type: StepTypeTool,
						Tool: "webb_engagement_context",
						Params: map[string]string{
							"customer": "{{ customer }}",
						},
					},
				},
			},
			{
				ID:          "review_gate",
				Type:        StepTypeGate,
				GateType:    "approval",
				Message:     "Review and approve customer contacts and configuration. Auto-proceeds with standard tier after timeout.",
				GateTimeout: "24h",
				OnTimeout:   "continue",
			},
			{
				ID:   "create_docs",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "confluence_hub",
						Type: StepTypeTool,
						Tool: "webb_confluence_create",
						Params: map[string]string{
							"title":     "Customer Hub: {{ customer }}",
							"space_key": "CUSTOMERS",
							"content":   "## {{ customer }} Overview\n\n**Tier:** {{ tier }}\n**Cluster:** {{ cluster }}\n\n## Contacts\n\n{{ steps.baseline.engagement_context.contacts }}\n\n## Current Health\n\n- Health Score: {{ steps.baseline.customer_snapshot.health_score }}/100\n- Open Tickets: {{ steps.baseline.customer_snapshot.open_tickets }}\n- Active Incidents: {{ steps.baseline.customer_snapshot.active_incidents }}\n\n## Onboarding Date\n\n{{ now.Format \"2006-01-02\" }}",
							"labels":    "customer,hub,{{ customer }}",
						},
					},
					{
						ID:   "vault_log",
						Type: StepTypeTool,
						Tool: "webb_vault_note",
						Params: map[string]string{
							"type":    "customer-onboarding",
							"title":   "{{ customer }} - Onboarding",
							"content": "## Onboarding Summary\n\n- Customer: {{ customer }}\n- Cluster: {{ cluster }}\n- Tier: {{ tier }}\n- Date: {{ now.Format \"2006-01-02\" }}\n\n## Baseline Health\n\n{{ steps.baseline.customer_snapshot.summary }}\n\n## Links\n\n- Confluence: {{ steps.create_docs.confluence_hub.url }}",
							"tags":    "customer,onboarding,{{ customer }}",
						},
					},
					{
						ID:   "tracking_sheet",
						Type: StepTypeTool,
						Tool: "webb_gsheets_create",
						Params: map[string]string{
							"title": "{{ customer }} Health Tracking",
						},
					},
				},
			},
			{
				ID:   "populate_sheet",
				Type: StepTypeTool,
				Tool: "webb_gsheets_write_range",
				Params: map[string]string{
					"spreadsheet_id": "{{ steps.create_docs.tracking_sheet.spreadsheet_id }}",
					"range":          "A1:D1",
					"values":         "Date,Health Score,Open Tickets,Notes",
				},
			},
			{
				ID:   "notify_csm",
				Type: StepTypeTool,
				Tool: "webb_slack_post",
				Params: map[string]string{
					"channel": "{{ notification_channel }}",
					"message": "New Customer Onboarded\n\nCustomer: {{ customer }}\nTier: {{ tier }}\nCluster: {{ cluster }}\n\nBaseline Health: {{ steps.baseline.customer_snapshot.health_score }}/100\n\nDocumentation:\n- Hub: {{ steps.create_docs.confluence_hub.url }}\n- Tracking: {{ steps.create_docs.tracking_sheet.url }}",
				},
			},
		},
		Timeout: "15m",
	}
}

func incidentKnowledgeCaptureChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "incident-knowledge-capture",
		Description: "Extract knowledge artifacts from incidents to prevent repeats",
		Category:    CategoryOperational,
		Tags:        []string{"incident", "knowledge", "patterns", "prevention"},
		Trigger: ChainTrigger{
			Type:  TriggerEvent,
			Event: "incident.postmortem.created",
		},
		Input: []ChainInput{
			{Name: "incident_id", Type: "string", Required: true, Description: "Incident ID"},
		},
		OnError: &ErrorHandler{
			Action: "continue",
			Notify: "#ops-alerts",
		},
		Steps: []ChainStep{
			{
				ID:   "get_context",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "incident_details",
						Type: StepTypeTool,
						Tool: "webb_incidentio_get",
						Params: map[string]string{
							"incident_id": "{{ incident_id }}",
						},
						Retry: &RetryPolicy{
							MaxAttempts: 3,
							Delay:       "5s",
							BackoffRate: 1.5,
						},
					},
					{
						ID:   "find_postmortem",
						Type: StepTypeTool,
						Tool: "webb_vault_search",
						Params: map[string]string{
							"query": "postmortem {{ incident_id }}",
						},
						Retry: &RetryPolicy{
							MaxAttempts: 2,
							Delay:       "5s",
							BackoffRate: 1.5,
						},
					},
				},
			},
			{
				ID:   "check_existing",
				Type: StepTypeTool,
				Tool: "webb_pattern_match",
				Params: map[string]string{
					"incident_id": "{{ incident_id }}",
					"threshold":   "0.7",
				},
			},
			{
				ID:   "extract_patterns",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "symptom_resolution",
						Type: StepTypeTool,
						Tool: "webb_pattern_extract",
						Params: map[string]string{
							"incident_id": "{{ incident_id }}",
						},
						Retry: &RetryPolicy{
							MaxAttempts: 2,
							Delay:       "5s",
							BackoffRate: 1.5,
						},
					},
					{
						ID:   "generate_runbook",
						Type: StepTypeTool,
						Tool: "webb_rca_to_runbook",
						Params: map[string]string{
							"incident_id": "{{ incident_id }}",
						},
					},
				},
			},
			{
				ID:   "build_knowledge",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "add_resolution_link",
						Type: StepTypeTool,
						Tool: "webb_graph_add_resolution",
						Params: map[string]string{
							"incident_id": "{{ incident_id }}",
							"symptoms":    "{{ steps.extract_patterns.symptom_resolution.symptoms }}",
							"resolution":  "{{ steps.extract_patterns.symptom_resolution.resolution }}",
						},
					},
					{
						ID:   "link_similar",
						Type: StepTypeTool,
						Tool: "webb_graph_link",
						Params: map[string]string{
							"source":       "incident:{{ incident_id }}",
							"target":       "{{ steps.get_context.find_postmortem.path }}",
							"relationship": "documented_in",
						},
					},
					{
						ID:   "index_searchable",
						Type: StepTypeTool,
						Tool: "webb_semantic_index",
						Params: map[string]string{
							"type":    "incident",
							"id":      "{{ incident_id }}",
							"content": "{{ steps.extract_patterns.symptom_resolution.summary }}",
						},
					},
				},
			},
			{
				ID:        "update_playbooks",
				Type:      StepTypeBranch,
				Condition: "{{ steps.extract_patterns.symptom_resolution.is_new_pattern }} && !{{ steps.check_existing.is_duplicate }}",
				Branches: map[string][]ChainStep{
					"true": {
						{
							ID:   "create_playbook",
							Type: StepTypeTool,
							Tool: "webb_playbook_create",
							Params: map[string]string{
								"name":       "{{ steps.extract_patterns.symptom_resolution.pattern_name }}",
								"symptoms":   "{{ steps.extract_patterns.symptom_resolution.symptoms }}",
								"resolution": "{{ steps.extract_patterns.symptom_resolution.resolution }}",
							},
						},
						{
							ID:   "notify_new_playbook",
							Type: StepTypeTool,
							Tool: "webb_slack_post",
							Params: map[string]string{
								"channel": "#incidents",
								"message": "New Playbook Created\n\nIncident: {{ incident_id }}\nPattern: {{ steps.extract_patterns.symptom_resolution.pattern_name }}\n\nA new playbook has been automatically generated from this incident's resolution.",
							},
						},
					},
					"false": {
						{
							ID:   "log_duplicate",
							Type: StepTypeTool,
							Tool: "webb_vault_note",
							Params: map[string]string{
								"type":    "knowledge-capture",
								"title":   "{{ incident_id }} - Pattern Match Found",
								"content": "## Pattern Already Exists\n\nIncident {{ incident_id }} matches existing pattern.\n\n**Matched Pattern:** {{ steps.check_existing.matched_pattern }}\n**Similarity:** {{ steps.check_existing.similarity }}\n\nNo new playbook created - linked to existing knowledge.",
								"tags":    "incident,pattern,duplicate",
							},
						},
					},
				},
			},
			{
				ID:   "record_outcome",
				Type: StepTypeTool,
				Tool: "webb_remediation_record_outcome",
				Params: map[string]string{
					"incident_id": "{{ incident_id }}",
					"outcome":     "knowledge_captured",
				},
			},
		},
		Timeout: "15m",
	}
}

func databaseQueryOptimizationChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "database-query-optimization",
		Description: "Weekly analysis of slow queries with index recommendations",
		Category:    CategoryOperational,
		Tags:        []string{"database", "performance", "optimization", "weekly"},
		Trigger: ChainTrigger{
			Type: TriggerScheduled,
			Cron: "0 0 6 * * 6",
		},
		Input: []ChainInput{
			{Name: "context", Type: "string", Required: false, Description: "Cluster context (default: all)"},
			{Name: "notification_channel", Type: "string", Required: false, Default: "#platform-discussions", Description: "Slack channel for notifications"},
		},
		OnError: &ErrorHandler{
			Action: "continue",
			Notify: "#ops-alerts",
		},
		Steps: []ChainStep{
			{
				ID:   "check_connections",
				Type: StepTypeTool,
				Tool: "webb_db_connection_health",
				Params: map[string]string{
					"context": "{{ context }}",
				},
				Retry: &RetryPolicy{
					MaxAttempts: 2,
					Delay:       "10s",
					BackoffRate: 1.5,
				},
			},
			{
				ID:   "gather",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "slow_queries",
						Type: StepTypeTool,
						Tool: "webb_db_slow_query_patterns",
						Params: map[string]string{
							"time_range": "7d",
							"context":    "{{ context }}",
						},
						Retry: &RetryPolicy{
							MaxAttempts: 2,
							Delay:       "10s",
							BackoffRate: 1.5,
						},
					},
					{
						ID:   "index_suggestions",
						Type: StepTypeTool,
						Tool: "webb_db_index_suggestions",
						Params: map[string]string{
							"context": "{{ context }}",
						},
						Retry: &RetryPolicy{
							MaxAttempts: 2,
							Delay:       "10s",
							BackoffRate: 1.5,
						},
					},
					{
						ID:   "clickhouse_analysis",
						Type: StepTypeTool,
						Tool: "webb_clickhouse_query_analyze",
						Params: map[string]string{
							"time_range": "7d",
						},
						Retry: &RetryPolicy{
							MaxAttempts: 2,
							Delay:       "10s",
							BackoffRate: 1.5,
						},
					},
					{
						ID:   "long_running",
						Type: StepTypeTool,
						Tool: "webb_postgres_long_queries",
						Params: map[string]string{
							"context": "{{ context }}",
						},
					},
				},
			},
			{
				ID:   "check_memory",
				Type: StepTypeTool,
				Tool: "webb_clickhouse_memory_check",
			},
			{
				ID:   "analyze_top_queries",
				Type: StepTypeTool,
				Tool: "webb_db_query_analyzer",
				Params: map[string]string{
					"queries": "{{ steps.gather.slow_queries.top_queries }}",
					"limit":   "10",
				},
			},
			{
				ID:        "has_critical_issues",
				Type:      StepTypeBranch,
				Condition: "{{ steps.gather.slow_queries.critical_count }} > 0 || {{ steps.gather.clickhouse_analysis.anti_pattern_count }} > 5 || {{ steps.check_memory.memory_pressure }}",
				Branches: map[string][]ChainStep{
					"true": {
						{
							ID:   "create_ticket",
							Type: StepTypeTool,
							Tool: "webb_shortcut_create",
							Params: map[string]string{
								"name":        "Database Optimization: Week of {{ now.Format \"2006-01-02\" }}",
								"description": "## Weekly Database Analysis\n\n### Connection Health\n{{ steps.check_connections.summary }}\n\n### Critical Issues\n- Slow queries: {{ steps.gather.slow_queries.critical_count }}\n- ClickHouse anti-patterns: {{ steps.gather.clickhouse_analysis.anti_pattern_count }}\n- Long-running queries: {{ steps.gather.long_running.count }}\n- Memory pressure: {{ steps.check_memory.status }}\n\n### Index Recommendations\n{{ steps.gather.index_suggestions.summary }}\n\n### Top Query Analysis\n{{ steps.analyze_top_queries.recommendations }}",
								"story_type":  "chore",
								"labels":      "database,performance,optimization,automated",
							},
						},
						{
							ID:   "notify_with_ticket",
							Type: StepTypeTool,
							Tool: "webb_slack_post",
							Params: map[string]string{
								"channel": "{{ notification_channel }}",
								"message": "Weekly Database Health Report - Action Required\n\n- Slow queries analyzed: {{ steps.gather.slow_queries.total }}\n- Critical issues: {{ steps.gather.slow_queries.critical_count }}\n- Index suggestions: {{ steps.gather.index_suggestions.count }}\n- ClickHouse anti-patterns: {{ steps.gather.clickhouse_analysis.anti_pattern_count }}\n- Memory pressure: {{ steps.check_memory.status }}\n\nTicket: {{ steps.has_critical_issues.create_ticket.url }}",
							},
						},
					},
					"false": {
						{
							ID:   "notify_healthy",
							Type: StepTypeTool,
							Tool: "webb_slack_post",
							Params: map[string]string{
								"channel": "{{ notification_channel }}",
								"message": "Weekly Database Health Report - All Clear\n\n- Slow queries analyzed: {{ steps.gather.slow_queries.total }}\n- Critical issues: 0\n- Index suggestions: {{ steps.gather.index_suggestions.count }}\n- ClickHouse anti-patterns: {{ steps.gather.clickhouse_analysis.anti_pattern_count }}\n\nNo critical issues detected this week.",
							},
						},
					},
				},
			},
			{
				ID:   "archive",
				Type: StepTypeTool,
				Tool: "webb_vault_note",
				Params: map[string]string{
					"type":    "db-health",
					"title":   "Database Health Report - {{ now.Format \"2006-01-02\" }}",
					"content": "## Weekly Database Analysis\n\n### Connection Health\n{{ steps.check_connections.summary }}\n\n### Slow Queries\n{{ steps.gather.slow_queries.summary }}\n\n### Index Recommendations\n{{ steps.gather.index_suggestions.summary }}\n\n### ClickHouse Analysis\n{{ steps.gather.clickhouse_analysis.summary }}\n\n### Memory Status\n{{ steps.check_memory.summary }}\n\n### Query Analysis\n{{ steps.analyze_top_queries.summary }}",
					"tags":    "database,performance,weekly",
				},
			},
		},
		Timeout: "20m",
	}
}

func nightlyResearchSwarmChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "nightly-research-swarm",
		Description: "Autonomous nightly research for continuous webb improvement",
		Category:    CategoryOperational,
		Tags:        []string{"research", "swarm", "improvement", "nightly"},
		Trigger: ChainTrigger{
			Type: TriggerScheduled,
			Cron: "0 0 23 * * *",
		},
		Input: []ChainInput{
			{Name: "duration", Type: "string", Required: false, Default: "2h", Description: "Swarm run duration"},
			{Name: "notification_channel", Type: "string", Required: false, Default: "#platform-discussions", Description: "Channel for swarm notifications"},
		},
		OnError: &ErrorHandler{
			Action: "continue",
			Notify: "#ops-alerts",
		},
		Steps: []ChainStep{
			{
				ID:   "start_swarm",
				Type: StepTypeTool,
				Tool: "webb_swarm_start",
				Params: map[string]string{
					"duration":     "{{ duration }}",
					"local":        "true",
					"vault_log":    "true",
					"worker_types": "tool_auditor,security_auditor,performance_profiler,pattern_discovery,improvement_audit",
				},
				Retry: &RetryPolicy{
					MaxAttempts: 2,
					Delay:       "30s",
					BackoffRate: 1.5,
				},
			},
			{
				ID:   "check_health",
				Type: StepTypeTool,
				Tool: "webb_swarm_health",
				Retry: &RetryPolicy{
					MaxAttempts: 3,
					Delay:       "10s",
					BackoffRate: 1.5,
				},
			},
			{
				ID:        "health_check",
				Type:      StepTypeBranch,
				Condition: "{{ steps.check_health.health_score }} >= 50",
				Branches: map[string][]ChainStep{
					"true": {
						{
							ID:   "wait_completion",
							Type: StepTypeTool,
							Tool: "webb_swarm_status",
							Params: map[string]string{
								"wait":    "true",
								"timeout": "2h30m",
							},
						},
					},
					"false": {
						{
							ID:   "notify_unhealthy",
							Type: StepTypeTool,
							Tool: "webb_slack_post",
							Params: map[string]string{
								"channel": "{{ notification_channel }}",
								"message": "Nightly research swarm unhealthy - aborting\n\nHealth score: {{ steps.check_health.health_score }}/100\nIssues: {{ steps.check_health.issues }}",
							},
						},
					},
				},
			},
			// Sequential: get findings first, then deduplicate (depends on findings)
			{
				ID:   "get_findings",
				Type: StepTypeTool,
				Tool: "webb_swarm_findings",
				Retry: &RetryPolicy{
					MaxAttempts: 2,
					Delay:       "10s",
					BackoffRate: 1.5,
				},
			},
			{
				ID:   "deduplicate",
				Type: StepTypeTool,
				Tool: "webb_swarm_aggregate",
				Retry: &RetryPolicy{
					MaxAttempts: 2,
					Delay:       "10s",
					BackoffRate: 1.5,
				},
			},
			{
				ID:   "collect_metrics",
				Type: StepTypeTool,
				Tool: "webb_swarm_metrics",
			},
			{
				ID:   "update_knowledge",
				Type: StepTypeTool,
				Tool: "webb_graph_rebuild",
				Params: map[string]string{
					"incremental": "true",
				},
				Retry: &RetryPolicy{
					MaxAttempts: 2,
					Delay:       "15s",
					BackoffRate: 1.5,
				},
			},
			{
				ID:   "archive_findings",
				Type: StepTypeTool,
				Tool: "webb_vault_note",
				Params: map[string]string{
					"type":    "swarm-research",
					"title":   "Nightly Research - {{ now.Format \"2006-01-02\" }}",
					"content": "## Swarm Research Findings\n\n### Summary\n- Total findings: {{ steps.get_findings.total }}\n- Unique after dedup: {{ steps.deduplicate.unique_count }}\n- Token usage: {{ steps.collect_metrics.total_tokens }}\n- Quality score: {{ steps.collect_metrics.quality_score }}/100\n\n### Categories\n{{ steps.deduplicate.by_category }}\n\n### Top Improvements\n{{ range steps.deduplicate.top_improvements }}\n- {{ .title }} (Impact: {{ .impact }})\n{{ end }}",
					"tags":    "swarm,research,nightly",
				},
			},
			{
				ID:          "morning_review_gate",
				Type:        StepTypeGate,
				GateType:    "approval",
				Message:     "Team reviews findings at 9am standup - findings require human review before acting",
				GateTimeout: "12h",
				OnTimeout:   "abort",
			},
		},
		Timeout: "3h",
	}
}

func weeklyComplianceReportChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "weekly-compliance-report",
		Description: "Automated compliance and health reporting for governance",
		Category:    CategoryOperational,
		Tags:        []string{"compliance", "governance", "reporting", "weekly"},
		Trigger: ChainTrigger{
			Type: TriggerScheduled,
			Cron: "0 0 16 * * 5",
		},
		Input: []ChainInput{
			{Name: "notification_channel", Type: "string", Required: false, Default: "#compliance", Description: "Channel for compliance reports"},
			{Name: "leadership_channel", Type: "string", Required: false, Default: "#leadership-alerts", Description: "Channel for regression alerts"},
		},
		Variables: map[string]string{
			"regression_threshold": "0.95",
			"critical_threshold":   "0.80",
		},
		OnError: &ErrorHandler{
			Action: "continue",
			Notify: "#ops-alerts",
		},
		Steps: []ChainStep{
			{
				ID:   "collect",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "compliance_snapshot",
						Type: StepTypeTool,
						Tool: "webb_compliance_snapshot",
						Retry: &RetryPolicy{
							MaxAttempts: 3,
							Delay:       "10s",
							BackoffRate: 1.5,
						},
					},
					{
						ID:   "security_audit",
						Type: StepTypeTool,
						Tool: "webb_security_audit_full",
						Retry: &RetryPolicy{
							MaxAttempts: 3,
							Delay:       "10s",
							BackoffRate: 1.5,
						},
					},
					{
						ID:   "vault_health",
						Type: StepTypeTool,
						Tool: "webb_vault_health_full",
						Retry: &RetryPolicy{
							MaxAttempts: 3,
							Delay:       "10s",
							BackoffRate: 1.5,
						},
					},
					{
						ID:   "tool_quality",
						Type: StepTypeTool,
						Tool: "webb_tool_score_all",
						Retry: &RetryPolicy{
							MaxAttempts: 2,
							Delay:       "15s",
							BackoffRate: 1.5,
						},
					},
					{
						ID:   "remediation_stats",
						Type: StepTypeTool,
						Tool: "webb_remediation_stats",
						Retry: &RetryPolicy{
							MaxAttempts: 2,
							Delay:       "10s",
							BackoffRate: 1.5,
						},
					},
				},
			},
			// Run analyze and generate_docs in parallel
			{
				ID:   "process",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "analyze_trends",
						Type: StepTypeTool,
						Tool: "webb_compliance_trends",
						Params: map[string]string{
							"time_range": "30d",
						},
						Retry: &RetryPolicy{
							MaxAttempts: 2,
							Delay:       "10s",
							BackoffRate: 1.5,
						},
					},
					{
						ID:   "forecast",
						Type: StepTypeTool,
						Tool: "webb_compliance_forecast",
						Retry: &RetryPolicy{
							MaxAttempts: 2,
							Delay:       "10s",
							BackoffRate: 1.5,
						},
					},
				},
			},
			{
				ID:   "generate_docs",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "executive_summary",
						Type: StepTypeTool,
						Tool: "webb_gdocs_create",
						Params: map[string]string{
							"template": "compliance_report",
							"title":    "Weekly Compliance Report - {{ now.Format \"2006-01-02\" }}",
							"data":     "{{ steps.collect | json }}",
						},
					},
					{
						ID:   "visual_dashboard",
						Type: StepTypeTool,
						Tool: "webb_slides_create",
						Params: map[string]string{
							"title": "Compliance Dashboard - Week {{ now.Format \"W02\" }}",
						},
					},
					{
						ID:   "metrics_sheet",
						Type: StepTypeTool,
						Tool: "webb_gsheets_create",
						Params: map[string]string{
							"title": "Compliance Metrics - {{ now.Format \"2006-01-02\" }}",
						},
					},
				},
			},
			{
				ID:   "populate_metrics",
				Type: StepTypeTool,
				Tool: "webb_gsheets_write_range",
				Params: map[string]string{
					"spreadsheet_id": "{{ steps.generate_docs.metrics_sheet.id }}",
					"range":          "Sheet1!A1:E6",
					"values":         "[[\"Metric\",\"Score\",\"Trend\",\"Threshold\",\"Status\"],[\"Compliance\",\"{{ steps.collect.compliance_snapshot.score }}\",\"{{ steps.process.analyze_trends.compliance_trend }}\",\"{{ regression_threshold }}\",\"{{ if ge steps.collect.compliance_snapshot.score regression_threshold }}OK{{ else }}WARNING{{ end }}\"],[\"Security\",\"{{ steps.collect.security_audit.health_score }}\",\"{{ steps.process.analyze_trends.security_trend }}\",\"{{ regression_threshold }}\",\"{{ if ge steps.collect.security_audit.health_score regression_threshold }}OK{{ else }}WARNING{{ end }}\"],[\"Vault Health\",\"{{ steps.collect.vault_health.health_score }}\",\"{{ steps.process.analyze_trends.vault_trend }}\",\"{{ regression_threshold }}\",\"{{ if ge steps.collect.vault_health.health_score regression_threshold }}OK{{ else }}WARNING{{ end }}\"],[\"Tool Quality\",\"{{ steps.collect.tool_quality.average_score }}\",\"{{ steps.process.analyze_trends.tool_trend }}\",\"{{ regression_threshold }}\",\"{{ if ge steps.collect.tool_quality.average_score regression_threshold }}OK{{ else }}WARNING{{ end }}\"],[\"Forecast\",\"{{ steps.process.forecast.predicted_score }}\",\"{{ steps.process.forecast.trend }}\",\"\",\"{{ steps.process.forecast.recommendation }}\"]]",
				},
			},
			{
				ID:   "publish",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "slack_notify",
						Type: StepTypeTool,
						Tool: "webb_slack_post",
						Params: map[string]string{
							"channel": "{{ notification_channel }}",
							"message": "Weekly Compliance Report Ready\n\n**Compliance Score:** {{ steps.collect.compliance_snapshot.score }}/100\n**Security Score:** {{ steps.collect.security_audit.health_score }}/100\n**Vault Health:** {{ steps.collect.vault_health.health_score }}/100\n\n**Trend:** {{ steps.process.analyze_trends.trend }}\n**Forecast:** {{ steps.process.forecast.predicted_score }}/100 ({{ steps.process.forecast.trend }})\n\nDocuments:\n- Summary: {{ steps.generate_docs.executive_summary.url }}\n- Dashboard: {{ steps.generate_docs.visual_dashboard.url }}\n- Metrics: {{ steps.generate_docs.metrics_sheet.url }}",
						},
					},
					{
						ID:   "confluence_archive",
						Type: StepTypeTool,
						Tool: "webb_confluence_create",
						Params: map[string]string{
							"title":     "Compliance Report - {{ now.Format \"2006-01-02\" }}",
							"space_key": "COMPLIANCE",
							"content":   "## Weekly Compliance Summary\n\n| Metric | Score | Trend | Forecast |\n|--------|-------|-------|----------|\n| Compliance | {{ steps.collect.compliance_snapshot.score }} | {{ steps.process.analyze_trends.compliance_trend }} | {{ steps.process.forecast.compliance_forecast }} |\n| Security | {{ steps.collect.security_audit.health_score }} | {{ steps.process.analyze_trends.security_trend }} | {{ steps.process.forecast.security_forecast }} |\n| Vault Health | {{ steps.collect.vault_health.health_score }} | {{ steps.process.analyze_trends.vault_trend }} | {{ steps.process.forecast.vault_forecast }} |\n| Tool Quality | {{ steps.collect.tool_quality.average_score }} | {{ steps.process.analyze_trends.tool_trend }} | {{ steps.process.forecast.tool_forecast }} |\n\n## Links\n- [Executive Summary]({{ steps.generate_docs.executive_summary.url }})\n- [Dashboard]({{ steps.generate_docs.visual_dashboard.url }})\n- [Metrics]({{ steps.generate_docs.metrics_sheet.url }})",
							"labels":    "compliance,weekly,report",
						},
					},
				},
			},
			{
				ID:        "alert_regression",
				Type:      StepTypeBranch,
				Condition: "{{ steps.process.analyze_trends.has_regression }}",
				Branches: map[string][]ChainStep{
					"true": {
						{
							ID:   "alert_leadership",
							Type: StepTypeTool,
							Tool: "webb_slack_post",
							Params: map[string]string{
								"channel": "{{ leadership_channel }}",
								"message": "Compliance Regression Alert\n\nWeekly compliance report shows regression in:\n{{ range steps.process.analyze_trends.regressions }}\n- {{ .metric }}: {{ .previous }} -> {{ .current }} ({{ .change }})\n{{ end }}\n\nReview required: {{ steps.generate_docs.executive_summary.url }}",
							},
						},
					},
					"false": {
						{
							ID:   "celebrate_team",
							Type: StepTypeTool,
							Tool: "webb_slack_post",
							Params: map[string]string{
								"channel": "{{ notification_channel }}",
								"message": "All compliance metrics healthy this week! Great work team.\n\n- Compliance: {{ steps.collect.compliance_snapshot.score }}/100\n- Security: {{ steps.collect.security_audit.health_score }}/100\n- Vault: {{ steps.collect.vault_health.health_score }}/100\n\nForecast: {{ steps.process.forecast.trend }}",
							},
						},
					},
				},
			},
		},
		Timeout: "20m",
	}
}

// oncallFullCheckChain provides comprehensive on-call status check
func oncallFullCheckChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "oncall-full-check",
		Description: "Comprehensive on-call check: dashboard, alerts, SLA at-risk, action plan. Use for complete oncall status review.",
		Category:    CategoryOperational,
		Tags:        []string{"oncall", "dashboard", "alerts", "sla", "action-plan", "comprehensive"},
		Trigger: ChainTrigger{
			Type: TriggerManual,
		},
		Input: []ChainInput{
			{Name: "cluster", Type: "string", Required: false, Default: "headspace-v2", Description: "Primary cluster to check"},
			{Name: "sla_threshold", Type: "number", Required: false, Default: "75", Description: "SLA at-risk threshold percentage"},
			{Name: "refresh", Type: "bool", Required: false, Default: "false", Description: "Bypass cache for fresh data"},
		},
		Steps: []ChainStep{
			{
				ID:   "parallel_checks",
				Type: StepTypeParallel,
				Steps: []ChainStep{
					{
						ID:   "dashboard",
						Type: StepTypeTool,
						Tool: "webb_oncall_dashboard",
					},
					{
						ID:   "alerts",
						Type: StepTypeTool,
						Tool: "webb_alerts_active",
					},
					{
						ID:   "grafana_alerts",
						Type: StepTypeTool,
						Tool: "webb_grafana_alerts",
						Params: map[string]string{
							"cluster": "{{ cluster }}",
						},
					},
					{
						ID:   "incidents",
						Type: StepTypeTool,
						Tool: "webb_incidentio_list",
						Params: map[string]string{
							"status": "active",
						},
					},
					{
						ID:   "sla_at_risk",
						Type: StepTypeTool,
						Tool: "webb_sla_at_risk",
						Params: map[string]string{
							"threshold": "{{ sla_threshold }}",
						},
					},
					{
						ID:   "cluster_health",
						Type: StepTypeTool,
						Tool: "webb_cluster_health_full",
						Params: map[string]string{
							"context": "{{ cluster }}",
						},
					},
				},
			},
			{
				ID:   "action_plan",
				Type: StepTypeTool,
				Tool: "webb_oncall_action_plan",
				Params: map[string]string{
					"refresh": "{{ refresh }}",
					"limit":   "50",
				},
			},
		},
		Timeout: "5m",
	}
}

// ephemeralClusterPreflightChain runs preflight validation on ephemeral cluster ready (v132)
func ephemeralClusterPreflightChain() *ChainDefinition {
	return &ChainDefinition{
		Name:        "ephemeral-cluster-preflight",
		Description: "Auto-run preflight validation when ephemeral cluster becomes ready",
		Category:    CategoryOperational,
		Tags:        []string{"ephemeral", "preflight", "validation", "automation"},
		Trigger: ChainTrigger{
			Type:  TriggerEvent,
			Event: string(EventEphemeralReady),
		},
		Steps: []ChainStep{
			{
				ID:   "run_preflight",
				Type: StepTypeTool,
				Tool: "webb_preflight_full",
				Params: map[string]string{
					"context": "{{ context_name }}",
					"cloud":   "{{ cloud }}",
				},
			},
			{
				ID:   "record_result",
				Type: StepTypeTool,
				Tool: "webb_ephemeral_cluster_preflight_result",
				Params: map[string]string{
					"cluster_id": "{{ cluster_id }}",
				},
			},
			{
				ID:   "notify_slack",
				Type: StepTypeTool,
				Tool: "webb_slack_post",
				Params: map[string]string{
					"channel": "C08K653F0G2",
					"text":    "Ephemeral cluster {{ cluster_id }} ({{ cloud }}) preflight complete - Owner: {{ owner }}",
				},
			},
		},
		Timeout: "10m",
	}
}
