// Package common provides shared utilities for MCP tools.
package common

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// Handler is the function signature for MCP tool handlers.
type Handler func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)

// ActionRegistry maps action names to their handlers.
type ActionRegistry map[string]Handler

// DomainRegistry maps domain names to their action registries.
type DomainRegistry map[string]ActionRegistry

// Dispatcher routes gateway requests to the appropriate handler based on domain and action.
type Dispatcher struct {
	name    string
	domains DomainRegistry
}

// NewDispatcher creates a new handler dispatcher for a gateway.
func NewDispatcher(name string) *Dispatcher {
	return &Dispatcher{
		name:    name,
		domains: make(DomainRegistry),
	}
}

// Domain registers a domain with its action handlers.
func (d *Dispatcher) Domain(name string, actions ActionRegistry) *Dispatcher {
	d.domains[name] = actions
	return d
}

// Handle extracts domain and action parameters and routes to the appropriate handler.
func (d *Dispatcher) Handle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	domain, err := req.RequireString("domain")
	if err != nil {
		return CodedErrorResultf(ErrInvalidParam, "domain parameter is required"), nil
	}
	action, err := req.RequireString("action")
	if err != nil {
		return CodedErrorResultf(ErrInvalidParam, "action parameter is required"), nil
	}
	return d.Dispatch(ctx, req, domain, action)
}

// Dispatch routes a request to the appropriate handler.
func (d *Dispatcher) Dispatch(ctx context.Context, req mcp.CallToolRequest, domain, action string) (*mcp.CallToolResult, error) {
	actions, ok := d.domains[domain]
	if !ok {
		return CodedErrorResultf(ErrInvalidParam, "unknown %s domain: %s. Valid: %s", d.name, domain, d.ValidDomains()), nil
	}
	handler, ok := actions[action]
	if !ok {
		return CodedErrorResultf(ErrInvalidParam, "unknown %s %s action: %s. Valid: %s", d.name, domain, action, d.ValidActions(domain)), nil
	}
	return handler(ctx, req)
}

// ValidDomains returns a comma-separated list of valid domain names.
func (d *Dispatcher) ValidDomains() string {
	domains := make([]string, 0, len(d.domains))
	for domain := range d.domains {
		domains = append(domains, domain)
	}
	return strings.Join(domains, ", ")
}

// ValidActions returns a comma-separated list of valid action names for a domain.
func (d *Dispatcher) ValidActions(domain string) string {
	actions, ok := d.domains[domain]
	if !ok {
		return ""
	}
	names := make([]string, 0, len(actions))
	for action := range actions {
		names = append(names, action)
	}
	return strings.Join(names, ", ")
}

// DescribeActions returns a formatted list of all domain/action pairs.
func (d *Dispatcher) DescribeActions() string {
	var sb strings.Builder
	sb.WriteString("Domains & actions:\n")
	domains := make([]string, 0, len(d.domains))
	for domain := range d.domains {
		domains = append(domains, domain)
	}
	sort.Strings(domains)
	for _, domain := range domains {
		actions := make([]string, 0, len(d.domains[domain]))
		for action := range d.domains[domain] {
			actions = append(actions, action)
		}
		sort.Strings(actions)
		for _, action := range actions {
			sb.WriteString(fmt.Sprintf("- %s/%s\n", domain, action))
		}
	}
	return sb.String()
}

// DescribeActionsWithHints returns a formatted list of all domain/action pairs with descriptions.
func (d *Dispatcher) DescribeActionsWithHints(hints map[string]string) string {
	var sb strings.Builder
	sb.WriteString("Domains & actions:\n")
	domains := make([]string, 0, len(d.domains))
	for domain := range d.domains {
		domains = append(domains, domain)
	}
	sort.Strings(domains)
	for _, domain := range domains {
		actions := make([]string, 0, len(d.domains[domain]))
		for action := range d.domains[domain] {
			actions = append(actions, action)
		}
		sort.Strings(actions)
		for _, action := range actions {
			key := domain + "/" + action
			if hint, ok := hints[key]; ok {
				sb.WriteString(fmt.Sprintf("- %s: %s\n", key, hint))
			} else {
				sb.WriteString(fmt.Sprintf("- %s\n", key))
			}
		}
	}
	return sb.String()
}

// Handler returns the main handler function for use in tool registration.
func (d *Dispatcher) Handler() Handler {
	return d.Handle
}

// ActionDispatcher routes gateway requests based on action only (no domain).
type ActionDispatcher struct {
	name    string
	actions ActionRegistry
}

// NewActionDispatcher creates a new action-only dispatcher.
func NewActionDispatcher(name string) *ActionDispatcher {
	return &ActionDispatcher{
		name:    name,
		actions: make(ActionRegistry),
	}
}

// Action adds an action handler.
func (ad *ActionDispatcher) Action(name string, handler Handler) *ActionDispatcher {
	ad.actions[name] = handler
	return ad
}

// Handle extracts the action parameter and routes to the appropriate handler.
func (ad *ActionDispatcher) Handle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action, err := req.RequireString("action")
	if err != nil {
		return CodedErrorResultf(ErrInvalidParam, "action parameter is required"), nil
	}
	handler, ok := ad.actions[action]
	if !ok {
		return CodedErrorResultf(ErrInvalidParam, "unknown %s action: %s. Valid: %s", ad.name, action, ad.ValidActions()), nil
	}
	return handler(ctx, req)
}

// ValidActions returns a comma-separated list of valid action names.
func (ad *ActionDispatcher) ValidActions() string {
	names := make([]string, 0, len(ad.actions))
	for action := range ad.actions {
		names = append(names, action)
	}
	return strings.Join(names, ", ")
}

// DescribeActionsWithHints returns a formatted list of all actions with descriptions.
func (ad *ActionDispatcher) DescribeActionsWithHints(hints map[string]string) string {
	var sb strings.Builder
	sb.WriteString("Actions:\n")
	actions := make([]string, 0, len(ad.actions))
	for action := range ad.actions {
		actions = append(actions, action)
	}
	sort.Strings(actions)
	for _, action := range actions {
		if hint, ok := hints[action]; ok {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", action, hint))
		} else {
			sb.WriteString(fmt.Sprintf("- %s\n", action))
		}
	}
	return sb.String()
}

// Handler returns the main handler function for use in tool registration.
func (ad *ActionDispatcher) Handler() Handler {
	return ad.Handle
}
