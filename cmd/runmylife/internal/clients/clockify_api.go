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

const clockifyAPIBaseURL = "https://api.clockify.me/api/v1"

// ClockifyClient is a REST client for the Clockify API.
type ClockifyClient struct {
	httpClient  *http.Client
	apiKey      string
	workspaceID string
	userID      string
}

// ClockifyProject represents a Clockify project.
type ClockifyProject struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ClientName string `json:"clientName"`
	Color      string `json:"color"`
	Archived   bool   `json:"archived"`
}

// ClockifyTimeEntry represents a time entry.
type ClockifyTimeEntry struct {
	ID          string `json:"id"`
	ProjectID   string `json:"projectId"`
	ProjectName string `json:"project_name"`
	Description string `json:"description"`
	Start       string `json:"start"`
	End         string `json:"end"`
	Duration    int    `json:"duration_seconds"`
	Billable    bool   `json:"billable"`
	Tags        []string `json:"tags"`
}

// NewClockifyClient creates a Clockify client and fetches workspace/user info.
func NewClockifyClient(ctx context.Context, apiKey string) (*ClockifyClient, error) {
	c := &ClockifyClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		apiKey:     apiKey,
	}
	// Fetch user info to get workspace and user IDs.
	var user struct {
		ID               string `json:"id"`
		ActiveWorkspace  string `json:"activeWorkspace"`
	}
	if err := c.doGet(ctx, "/user", &user); err != nil {
		return nil, fmt.Errorf("get clockify user: %w", err)
	}
	c.workspaceID = user.ActiveWorkspace
	c.userID = user.ID
	return c, nil
}

// ListProjects returns all projects in the workspace.
func (c *ClockifyClient) ListProjects(ctx context.Context) ([]ClockifyProject, error) {
	var raw []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Color    string `json:"color"`
		Archived bool   `json:"archived"`
		ClientID string `json:"clientId"`
		ClientName string `json:"clientName"`
	}
	if err := c.doGet(ctx, fmt.Sprintf("/workspaces/%s/projects?page-size=100", c.workspaceID), &raw); err != nil {
		return nil, err
	}
	var projects []ClockifyProject
	for _, r := range raw {
		projects = append(projects, ClockifyProject{ID: r.ID, Name: r.Name, ClientName: r.ClientName, Color: r.Color, Archived: r.Archived})
	}
	return projects, nil
}

// RecentEntries returns recent time entries.
func (c *ClockifyClient) RecentEntries(ctx context.Context, limit int) ([]ClockifyTimeEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	var raw []struct {
		ID          string `json:"id"`
		Description string `json:"description"`
		ProjectID   string `json:"projectId"`
		Billable    bool   `json:"billable"`
		TimeInterval struct {
			Start    string `json:"start"`
			End      string `json:"end"`
			Duration string `json:"duration"`
		} `json:"timeInterval"`
		TagIds []string `json:"tagIds"`
	}
	path := fmt.Sprintf("/workspaces/%s/user/%s/time-entries?page-size=%d", c.workspaceID, c.userID, limit)
	if err := c.doGet(ctx, path, &raw); err != nil {
		return nil, err
	}
	var entries []ClockifyTimeEntry
	for _, r := range raw {
		dur := parseDuration(r.TimeInterval.Duration)
		entries = append(entries, ClockifyTimeEntry{ID: r.ID, ProjectID: r.ProjectID, Description: r.Description, Start: r.TimeInterval.Start, End: r.TimeInterval.End, Duration: dur, Billable: r.Billable, Tags: r.TagIds})
	}
	return entries, nil
}

// CurrentEntry returns the currently running time entry, if any.
func (c *ClockifyClient) CurrentEntry(ctx context.Context) (*ClockifyTimeEntry, error) {
	entries, err := c.RecentEntries(ctx, 1)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	// If end is empty, it's currently running.
	if entries[0].End == "" {
		return &entries[0], nil
	}
	return nil, nil
}

// StartTimer starts a new time entry.
func (c *ClockifyClient) StartTimer(ctx context.Context, description, projectID string) (*ClockifyTimeEntry, error) {
	body := map[string]interface{}{
		"start":       time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"description": description,
	}
	if projectID != "" {
		body["projectId"] = projectID
	}
	var raw struct {
		ID          string `json:"id"`
		Description string `json:"description"`
		ProjectID   string `json:"projectId"`
		TimeInterval struct {
			Start string `json:"start"`
		} `json:"timeInterval"`
	}
	path := fmt.Sprintf("/workspaces/%s/time-entries", c.workspaceID)
	if err := c.doPost(ctx, path, body, &raw); err != nil {
		return nil, err
	}
	return &ClockifyTimeEntry{ID: raw.ID, ProjectID: raw.ProjectID, Description: raw.Description, Start: raw.TimeInterval.Start}, nil
}

// StopTimer stops the currently running time entry.
func (c *ClockifyClient) StopTimer(ctx context.Context) (*ClockifyTimeEntry, error) {
	body := map[string]string{
		"end": time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}
	var raw struct {
		ID          string `json:"id"`
		Description string `json:"description"`
		TimeInterval struct {
			Start    string `json:"start"`
			End      string `json:"end"`
			Duration string `json:"duration"`
		} `json:"timeInterval"`
	}
	path := fmt.Sprintf("/workspaces/%s/user/%s/time-entries", c.workspaceID, c.userID)
	if err := c.doPatch(ctx, path, body, &raw); err != nil {
		return nil, err
	}
	dur := parseDuration(raw.TimeInterval.Duration)
	return &ClockifyTimeEntry{ID: raw.ID, Description: raw.Description, Start: raw.TimeInterval.Start, End: raw.TimeInterval.End, Duration: dur}, nil
}

// parseDuration parses ISO 8601 duration like PT1H30M into seconds.
func parseDuration(iso string) int {
	d, err := time.ParseDuration(isoDurationToGo(iso))
	if err != nil {
		return 0
	}
	return int(d.Seconds())
}

func isoDurationToGo(iso string) string {
	if len(iso) < 2 || iso[0] != 'P' {
		return "0s"
	}
	iso = iso[1:] // strip P
	if len(iso) > 0 && iso[0] == 'T' {
		iso = iso[1:]
	}
	result := ""
	num := ""
	for _, c := range iso {
		switch {
		case c >= '0' && c <= '9':
			num += string(c)
		case c == 'H':
			result += num + "h"
			num = ""
		case c == 'M':
			result += num + "m"
			num = ""
		case c == 'S':
			result += num + "s"
			num = ""
		}
	}
	if result == "" {
		return "0s"
	}
	return result
}

func (c *ClockifyClient) doGet(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, clockifyAPIBaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("clockify API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(body), API: "clockify"}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *ClockifyClient) doPost(ctx context.Context, path string, body interface{}, out interface{}) error {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, clockifyAPIBaseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("clockify API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(b), API: "clockify"}
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *ClockifyClient) doPatch(ctx context.Context, path string, body interface{}, out interface{}) error {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, clockifyAPIBaseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("clockify API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(b), API: "clockify"}
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
