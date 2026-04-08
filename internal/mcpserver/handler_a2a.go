package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/mcpkit/a2a"
)

// A2A integration handlers — discover, send tasks to, and manage A2A agents.

func (s *Server) handleA2ADiscover(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	url := getStringArg(req, "url")
	if url == "" {
		return codedError(ErrInvalidParams, "url required"), nil
	}

	client := a2a.NewClient(url)
	card, err := client.GetAgentCard(context.Background())
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("discover agent: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"name":         card.Name,
		"description":  card.Description,
		"url":          card.URL,
		"version":      card.Version,
		"skills_count": len(card.Skills),
		"skills":       card.Skills,
		"capabilities": card.Capabilities,
	}), nil
}

func (s *Server) handleA2ASend(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	url := getStringArg(req, "url")
	message := getStringArg(req, "message")
	if url == "" || message == "" {
		return codedError(ErrInvalidParams, "url and message required"), nil
	}

	taskID := getStringArg(req, "task_id")
	if taskID == "" {
		taskID = fmt.Sprintf("ralph-%d", s.nextID())
	}

	client := a2a.NewClient(url)
	task, err := client.SendTask(context.Background(), a2a.TaskSendParams{
		ID: taskID,
		Messages: []a2a.Message{
			{Role: "user", Parts: []a2a.Part{a2a.TextPart(message)}},
		},
	})
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("send task: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"task_id": task.ID,
		"state":   task.State,
		"url":     url,
	}), nil
}

func (s *Server) handleA2AStatus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	url := getStringArg(req, "url")
	taskID := getStringArg(req, "task_id")
	if url == "" || taskID == "" {
		return codedError(ErrInvalidParams, "url and task_id required"), nil
	}

	client := a2a.NewClient(url)
	task, err := client.GetTask(context.Background(), taskID)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("get task: %v", err)), nil
	}

	result := map[string]any{
		"task_id":  task.ID,
		"state":    task.State,
		"messages": len(task.Messages),
	}

	// Extract last agent response
	for i := len(task.Messages) - 1; i >= 0; i-- {
		if task.Messages[i].Role == "agent" {
			for _, part := range task.Messages[i].Parts {
				if part.Type == "text" {
					result["response"] = part.Text
					break
				}
			}
			break
		}
	}

	return jsonResult(result), nil
}

func (s *Server) handleA2AAgentCard(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Generate our own agent card from the tool registry
	// TODO: wire *registry.ToolRegistry from mcpkit — requires adding field to Server struct
	// and plumbing it through NewServerWithBus. Currently nil produces a card with no skills list.
	card := a2a.AgentCardFromRegistry(nil,
		a2a.WithName("ralphglasses"),
		a2a.WithDescription("Multi-LLM agent fleet orchestration"),
		a2a.WithVersion("1.0.0"),
		a2a.WithOrganization("hairglasses-studio", "https://github.com/hairglasses-studio"),
	)

	cardJSON, _ := json.Marshal(card)
	return jsonResult(map[string]any{
		"agent_card": json.RawMessage(cardJSON),
	}), nil
}

var idCounter atomic.Uint64

func (s *Server) nextID() uint64 {
	return idCounter.Add(1)
}
