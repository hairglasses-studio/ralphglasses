package parity

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestCheckOllamaWarnsWhenEndpointUnavailable(t *testing.T) {
	orig := discoverDoctorOllamaInventory
	discoverDoctorOllamaInventory = func(context.Context, time.Duration) session.OllamaInventory {
		return session.OllamaInventory{
			BaseURL: "http://127.0.0.1:11434",
			Error:   "connection refused",
		}
	}
	t.Cleanup(func() { discoverDoctorOllamaInventory = orig })

	result := checkOllama(context.Background(), DoctorOptions{})
	if result.Status != StatusWarn {
		t.Fatalf("status = %q, want %q", result.Status, StatusWarn)
	}
	if !strings.Contains(result.Message, "connection refused") {
		t.Fatalf("message = %q, want endpoint error", result.Message)
	}
}

func TestCheckOllamaWarnsOnMissingRequiredLanes(t *testing.T) {
	orig := discoverDoctorOllamaInventory
	discoverDoctorOllamaInventory = func(context.Context, time.Duration) session.OllamaInventory {
		return session.OllamaInventory{
			BaseURL:               "http://127.0.0.1:11434",
			Reachable:             true,
			RequiredModels:        []string{"code-primary", "code-fast", "code-heavy"},
			ReadyRequiredModels:   []string{"code-primary", "code-fast"},
			MissingRequiredModels: []string{"code-heavy"},
		}
	}
	t.Cleanup(func() { discoverDoctorOllamaInventory = orig })

	result := checkOllama(context.Background(), DoctorOptions{})
	if result.Status != StatusWarn {
		t.Fatalf("status = %q, want %q", result.Status, StatusWarn)
	}
	if !strings.Contains(result.Message, "ready 2/3 required lanes") {
		t.Fatalf("message = %q, want readiness summary", result.Message)
	}
	if !strings.Contains(result.Message, "code-heavy") {
		t.Fatalf("message = %q, want missing lane", result.Message)
	}
}

func TestCheckOllamaWarnsOnAliasDrift(t *testing.T) {
	orig := discoverDoctorOllamaInventory
	discoverDoctorOllamaInventory = func(context.Context, time.Duration) session.OllamaInventory {
		return session.OllamaInventory{
			BaseURL:             "http://127.0.0.1:11434",
			Reachable:           true,
			RequiredModels:      []string{"code-primary", "code-fast"},
			ReadyRequiredModels: []string{"code-primary", "code-fast"},
			ManagedAliases: []session.OllamaAliasInventory{
				{Alias: "code-primary", Status: "installed"},
				{Alias: "code-fast", Status: "missing_alias"},
			},
		}
	}
	t.Cleanup(func() { discoverDoctorOllamaInventory = orig })

	result := checkOllama(context.Background(), DoctorOptions{})
	if result.Status != StatusWarn {
		t.Fatalf("status = %q, want %q", result.Status, StatusWarn)
	}
	if !strings.Contains(result.Message, "alias drift on code-fast") {
		t.Fatalf("message = %q, want alias drift guidance", result.Message)
	}
}

func TestCheckOllamaPassesWhenInventoryHealthy(t *testing.T) {
	orig := discoverDoctorOllamaInventory
	discoverDoctorOllamaInventory = func(context.Context, time.Duration) session.OllamaInventory {
		return session.OllamaInventory{
			BaseURL:             "http://127.0.0.1:11434",
			Reachable:           true,
			RequiredModels:      []string{"code-primary", "code-fast"},
			ReadyRequiredModels: []string{"code-primary", "code-fast"},
			ManagedAliases: []session.OllamaAliasInventory{
				{Alias: "code-primary", Status: "installed"},
				{Alias: "code-fast", Status: "installed"},
			},
			AvailableModelCount: 6,
		}
	}
	t.Cleanup(func() { discoverDoctorOllamaInventory = orig })

	result := checkOllama(context.Background(), DoctorOptions{})
	if result.Status != StatusPass {
		t.Fatalf("status = %q, want %q", result.Status, StatusPass)
	}
	if !strings.Contains(result.Message, "2/2 required lanes ready") {
		t.Fatalf("message = %q, want ready summary", result.Message)
	}
}
