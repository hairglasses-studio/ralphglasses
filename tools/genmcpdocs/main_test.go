package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
)

func TestBuildTemplateDataLiveContract(t *testing.T) {
	t.Parallel()

	data := buildTemplateData()
	srv := mcpserver.NewServer(".")

	if data.TotalTools != mcpserver.TotalToolCount() {
		t.Fatalf("TotalTools = %d, want %d", data.TotalTools, mcpserver.TotalToolCount())
	}
	if data.ManagementToolCount != len(srv.ManagementTools()) {
		t.Fatalf("ManagementToolCount = %d, want %d", data.ManagementToolCount, len(srv.ManagementTools()))
	}
	if data.GroupToolCount+data.ManagementToolCount != data.TotalTools {
		t.Fatalf("group + management count mismatch: %d + %d != %d", data.GroupToolCount, data.ManagementToolCount, data.TotalTools)
	}
	if data.ToolGroupCount != len(mcpserver.ToolGroupNames) {
		t.Fatalf("ToolGroupCount = %d, want %d", data.ToolGroupCount, len(mcpserver.ToolGroupNames))
	}
	if len(data.Groups) != len(mcpserver.ToolGroupNames) {
		t.Fatalf("len(Groups) = %d, want %d", len(data.Groups), len(mcpserver.ToolGroupNames))
	}
	if data.ResourceCount != len(mcpserver.StaticResources()) {
		t.Fatalf("ResourceCount = %d, want %d", data.ResourceCount, len(mcpserver.StaticResources()))
	}
	if data.ResourceTemplateCount != len(mcpserver.ResourceTemplates()) {
		t.Fatalf("ResourceTemplateCount = %d, want %d", data.ResourceTemplateCount, len(mcpserver.ResourceTemplates()))
	}
	if data.PromptCount != len(mcpserver.Prompts()) {
		t.Fatalf("PromptCount = %d, want %d", data.PromptCount, len(mcpserver.Prompts()))
	}
	if len(data.Management) == 0 {
		t.Fatal("expected management tools in template data")
	}
}

func TestRenderSampleData(t *testing.T) {
	t.Parallel()

	data := templateData{
		TotalTools:            4,
		GroupToolCount:        3,
		ManagementToolCount:   1,
		ToolGroupCount:        2,
		ResourceCount:         1,
		ResourceTemplateCount: 1,
		PromptCount:           1,
		Management: []toolDoc{
			{Name: "ralphglasses_server_health", Description: "Show contract health"},
		},
		Groups: []groupDoc{
			{
				Name:        "core",
				Description: "Essential tools",
				Tools: []toolDoc{
					{Name: "ralphglasses_scan", Description: "Scan repos"},
					{
						Name:        "ralphglasses_status",
						Description: "Get repo status",
						Params: []paramDoc{
							{Name: "repo", Type: "string", Required: true, Description: "Repo name"},
							{Name: "include_config", Type: "boolean", Description: "Include config"},
						},
					},
				},
			},
			{
				Name:        "session",
				Description: "Session lifecycle",
				Tools: []toolDoc{
					{
						Name:        "ralphglasses_session_launch",
						Description: "Launch a session",
						Params: []paramDoc{
							{Name: "repo", Type: "string", Required: true, Description: "Repo name"},
						},
					},
				},
			},
		},
		Resources: []mcpserver.ResourceDef{
			{URI: "ralph:///catalog/server", Name: "Server catalog", Description: "Contract summary"},
		},
		ResourceTemplates: []mcpserver.ResourceTemplateDef{
			{URI: "ralph:///{repo}/status", Name: "Repo status", Description: "Status resource"},
		},
		Prompts: []mcpserver.PromptDef{
			{Name: "code-review", Description: "Review code"},
		},
	}

	var buf bytes.Buffer
	if err := mdTemplate.Execute(&buf, data); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	md := buf.String()
	for _, want := range []string{
		"# MCP Server Contract",
		"**4 tools**",
		"**2 deferred-load tool groups**",
		"## Management Tools",
		"`ralphglasses_server_health`",
		"## Tool Groups",
		"| `core` | 2 | Essential tools |",
		"## core",
		"### `ralphglasses_scan`",
		"*No parameters.*",
		"| `repo` | string | **required** | Repo name |",
		"| `include_config` | boolean | optional | Include config |",
		"## Resources",
		"## Prompts",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("rendered markdown missing %q\n%s", want, md)
		}
	}
}

func TestRenderEmpty(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := mdTemplate.Execute(&buf, templateData{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(buf.String(), "**0 tools**") {
		t.Fatal("expected zero-tool summary in output")
	}
}
