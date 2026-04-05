package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HomeAssistantClient is a REST client for the Home Assistant API.
type HomeAssistantClient struct {
	httpClient *http.Client
	baseURL    string
	token      string
}

// HAEntity represents a Home Assistant entity.
type HAEntity struct {
	EntityID     string                 `json:"entity_id"`
	State        string                 `json:"state"`
	FriendlyName string                `json:"friendly_name"`
	Domain       string                 `json:"domain"`
	Attributes   map[string]interface{} `json:"attributes"`
	LastChanged  string                 `json:"last_changed"`
}

// HAAutomation represents a Home Assistant automation.
type HAAutomation struct {
	ID            string `json:"id"`
	Alias         string `json:"alias"`
	State         string `json:"state"`
	LastTriggered string `json:"last_triggered"`
}

// HAScene represents a Home Assistant scene.
type HAScene struct {
	EntityID     string `json:"entity_id"`
	FriendlyName string `json:"friendly_name"`
}

// NewHomeAssistantClient creates a new Home Assistant API client.
func NewHomeAssistantClient(baseURL, token string) *HomeAssistantClient {
	return &HomeAssistantClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		baseURL:    baseURL,
		token:      token,
	}
}

// ListEntities returns all entities, optionally filtered by domain.
func (h *HomeAssistantClient) ListEntities(ctx context.Context, domain string) ([]HAEntity, error) {
	var raw []struct {
		EntityID   string                 `json:"entity_id"`
		State      string                 `json:"state"`
		Attributes map[string]interface{} `json:"attributes"`
		LastChanged string                `json:"last_changed"`
	}
	if err := h.doGet(ctx, "/api/states", &raw); err != nil {
		return nil, err
	}
	var entities []HAEntity
	for _, r := range raw {
		d := entityDomain(r.EntityID)
		if domain != "" && d != domain {
			continue
		}
		name, _ := r.Attributes["friendly_name"].(string)
		entities = append(entities, HAEntity{EntityID: r.EntityID, State: r.State, FriendlyName: name, Domain: d, Attributes: r.Attributes, LastChanged: r.LastChanged})
	}
	return entities, nil
}

// GetEntityState returns the state of a single entity.
func (h *HomeAssistantClient) GetEntityState(ctx context.Context, entityID string) (*HAEntity, error) {
	var raw struct {
		EntityID   string                 `json:"entity_id"`
		State      string                 `json:"state"`
		Attributes map[string]interface{} `json:"attributes"`
		LastChanged string                `json:"last_changed"`
	}
	if err := h.doGet(ctx, "/api/states/"+entityID, &raw); err != nil {
		return nil, err
	}
	name, _ := raw.Attributes["friendly_name"].(string)
	return &HAEntity{EntityID: raw.EntityID, State: raw.State, FriendlyName: name, Domain: entityDomain(raw.EntityID), Attributes: raw.Attributes, LastChanged: raw.LastChanged}, nil
}

// ToggleEntity toggles an entity on/off.
func (h *HomeAssistantClient) ToggleEntity(ctx context.Context, entityID string) error {
	body := map[string]string{"entity_id": entityID}
	return h.doPost(ctx, "/api/services/homeassistant/toggle", body, nil)
}

// SetEntityState calls a service to set entity state.
func (h *HomeAssistantClient) SetEntityState(ctx context.Context, domain, service string, data map[string]interface{}) error {
	return h.doPost(ctx, fmt.Sprintf("/api/services/%s/%s", domain, service), data, nil)
}

// ListAutomations returns all automations.
func (h *HomeAssistantClient) ListAutomations(ctx context.Context) ([]HAAutomation, error) {
	entities, err := h.ListEntities(ctx, "automation")
	if err != nil {
		return nil, err
	}
	var automations []HAAutomation
	for _, e := range entities {
		lastTriggered, _ := e.Attributes["last_triggered"].(string)
		automations = append(automations, HAAutomation{ID: e.EntityID, Alias: e.FriendlyName, State: e.State, LastTriggered: lastTriggered})
	}
	return automations, nil
}

// TriggerAutomation triggers an automation by entity_id.
func (h *HomeAssistantClient) TriggerAutomation(ctx context.Context, entityID string) error {
	body := map[string]string{"entity_id": entityID}
	return h.doPost(ctx, "/api/services/automation/trigger", body, nil)
}

// ListScenes returns all scenes.
func (h *HomeAssistantClient) ListScenes(ctx context.Context) ([]HAScene, error) {
	entities, err := h.ListEntities(ctx, "scene")
	if err != nil {
		return nil, err
	}
	var scenes []HAScene
	for _, e := range entities {
		scenes = append(scenes, HAScene{EntityID: e.EntityID, FriendlyName: e.FriendlyName})
	}
	return scenes, nil
}

// ActivateScene activates a scene.
func (h *HomeAssistantClient) ActivateScene(ctx context.Context, entityID string) error {
	body := map[string]string{"entity_id": entityID}
	return h.doPost(ctx, "/api/services/scene/turn_on", body, nil)
}

func entityDomain(entityID string) string {
	for i, c := range entityID {
		if c == '.' {
			return entityID[:i]
		}
	}
	return entityID
}

func (h *HomeAssistantClient) doGet(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("home assistant API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(body), API: "homeassistant"}
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (h *HomeAssistantClient) doPost(ctx context.Context, path string, body interface{}, out interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("home assistant API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(b), API: "homeassistant"}
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
