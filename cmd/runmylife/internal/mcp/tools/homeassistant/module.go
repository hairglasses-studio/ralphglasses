// Package homeassistant provides MCP tools for Home Assistant integration.
package homeassistant

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/clients"
	"github.com/hairglasses-studio/runmylife/internal/config"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

type Module struct{}

func (m *Module) Name() string        { return "homeassistant" }
func (m *Module) Description() string { return "Home Assistant smart home control" }

var haHints = map[string]string{
	"entities/list":       "List entities by domain (light, switch, sensor, etc.)",
	"entities/state":      "Get entity state",
	"entities/toggle":     "Toggle entity on/off",
	"entities/set":        "Set entity state via service call",
	"automations/list":    "List automations",
	"automations/trigger": "Trigger an automation",
	"scenes/list":         "List scenes",
	"scenes/activate":     "Activate a scene",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("homeassistant").
		Domain("entities", common.ActionRegistry{
			"list":   handleEntitiesList,
			"state":  handleEntitiesState,
			"toggle": handleEntitiesToggle,
			"set":    handleEntitiesSet,
		}).
		Domain("automations", common.ActionRegistry{
			"list":    handleAutomationsList,
			"trigger": handleAutomationsTrigger,
		}).
		Domain("scenes", common.ActionRegistry{
			"list":     handleScenesList,
			"activate": handleScenesActivate,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_homeassistant",
				mcp.WithDescription("Home Assistant gateway for smart home control.\n\n"+dispatcher.DescribeActionsWithHints(haHints)),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: entities, automations, scenes")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("entity_id", mcp.Description("Entity ID (e.g. light.living_room)")),
				mcp.WithString("entity_domain", mcp.Description("Entity domain filter (light, switch, sensor, etc.)")),
				mcp.WithString("service", mcp.Description("Service name (e.g. turn_on, turn_off)")),
				mcp.WithString("service_data", mcp.Description("JSON service data")),
			),
			Handler:             tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:            "homeassistant",
			Subcategory:         "gateway",
			Tags:                []string{"homeassistant", "smarthome", "iot"},
			Complexity:          tools.ComplexitySimple,
			IsWrite:             true,
			CircuitBreakerGroup: "homeassistant_api",
			Timeout:             30 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func haClient() (*clients.HomeAssistantClient, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	token := cfg.Credentials["homeassistant"]
	if token == "" {
		return nil, fmt.Errorf("Home Assistant token not configured — add 'homeassistant' to credentials")
	}
	baseURL := cfg.Credentials["homeassistant_url"]
	if baseURL == "" {
		baseURL = "http://homeassistant.local:8123"
	}
	return clients.NewHomeAssistantClient(baseURL, token), nil
}

func handleEntitiesList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := haClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add homeassistant token to config"), nil
	}
	domain := common.GetStringParam(req, "entity_domain", "")
	entities, err := client.ListEntities(ctx, domain)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	title := "Entities"
	if domain != "" {
		title = fmt.Sprintf("Entities: %s", domain)
	}
	md := common.NewMarkdownBuilder().Title(title)
	headers := []string{"Entity ID", "Name", "State", "Last Changed"}
	var rows [][]string
	for _, e := range entities {
		rows = append(rows, []string{e.EntityID, e.FriendlyName, e.State, e.LastChanged})
	}
	if len(rows) == 0 {
		md.EmptyList("entities")
	} else {
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}

func handleEntitiesState(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	entityID, ok := common.RequireStringParam(req, "entity_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "entity_id is required"), nil
	}
	client, err := haClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add homeassistant token to config"), nil
	}
	entity, err := client.GetEntityState(ctx, entityID)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title(entity.FriendlyName)
	md.KeyValue("Entity ID", entity.EntityID)
	md.KeyValue("State", entity.State)
	md.KeyValue("Domain", entity.Domain)
	md.KeyValue("Last Changed", entity.LastChanged)
	if len(entity.Attributes) > 0 {
		attrJSON, _ := json.MarshalIndent(entity.Attributes, "", "  ")
		md.Section("Attributes").Text("```json\n" + string(attrJSON) + "\n```")
	}
	return tools.TextResult(md.String()), nil
}

func handleEntitiesToggle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	entityID, ok := common.RequireStringParam(req, "entity_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "entity_id is required"), nil
	}
	client, err := haClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add homeassistant token to config"), nil
	}
	if err := client.ToggleEntity(ctx, entityID); err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	return tools.TextResult(fmt.Sprintf("Toggled %s.", entityID)), nil
}

func handleEntitiesSet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	entityID, ok := common.RequireStringParam(req, "entity_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "entity_id is required"), nil
	}
	svc := common.GetStringParam(req, "service", "")
	if svc == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "service is required (e.g. turn_on, turn_off)"), nil
	}
	client, err := haClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add homeassistant token to config"), nil
	}

	data := map[string]interface{}{"entity_id": entityID}
	if extra := common.GetStringParam(req, "service_data", ""); extra != "" {
		var extraData map[string]interface{}
		if err := json.Unmarshal([]byte(extra), &extraData); err == nil {
			for k, v := range extraData {
				data[k] = v
			}
		}
	}

	// Extract entity domain from entity_id (e.g. "light" from "light.living_room").
	entDomain := ""
	for i, c := range entityID {
		if c == '.' {
			entDomain = entityID[:i]
			break
		}
	}
	if entDomain == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "entity_id must include domain (e.g. light.living_room)"), nil
	}

	if err := client.SetEntityState(ctx, entDomain, svc, data); err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	return tools.TextResult(fmt.Sprintf("Called %s.%s on %s.", entDomain, svc, entityID)), nil
}

func handleAutomationsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := haClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add homeassistant token to config"), nil
	}
	automations, err := client.ListAutomations(ctx)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title("Automations")
	headers := []string{"ID", "Name", "State", "Last Triggered"}
	var rows [][]string
	for _, a := range automations {
		rows = append(rows, []string{a.ID, a.Alias, a.State, a.LastTriggered})
	}
	if len(rows) == 0 {
		md.EmptyList("automations")
	} else {
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}

func handleAutomationsTrigger(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	entityID, ok := common.RequireStringParam(req, "entity_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "entity_id is required"), nil
	}
	client, err := haClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add homeassistant token to config"), nil
	}
	if err := client.TriggerAutomation(ctx, entityID); err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	return tools.TextResult(fmt.Sprintf("Triggered automation %s.", entityID)), nil
}

func handleScenesList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := haClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add homeassistant token to config"), nil
	}
	scenes, err := client.ListScenes(ctx)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title("Scenes")
	headers := []string{"Entity ID", "Name"}
	var rows [][]string
	for _, s := range scenes {
		rows = append(rows, []string{s.EntityID, s.FriendlyName})
	}
	if len(rows) == 0 {
		md.EmptyList("scenes")
	} else {
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}

func handleScenesActivate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	entityID, ok := common.RequireStringParam(req, "entity_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "entity_id is required"), nil
	}
	client, err := haClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add homeassistant token to config"), nil
	}
	if err := client.ActivateScene(ctx, entityID); err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	return tools.TextResult(fmt.Sprintf("Activated scene %s.", entityID)), nil
}
