// genmcpdocs renders docs/MCP-TOOLS.md from the live MCP contract exported by
// internal/mcpserver. This keeps the public reference tied to the same tool,
// resource, and prompt metadata the server actually exposes.
//
// Usage:
//
//	go run ./tools/genmcpdocs
//	go run ./tools/genmcpdocs --output docs/MCP-TOOLS.md
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/template"

	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
)

type paramDoc struct {
	Name        string
	Type        string
	Required    bool
	Description string
}

type toolDoc struct {
	Name        string
	Description string
	Params      []paramDoc
}

type groupDoc struct {
	Name        string
	Description string
	Tools       []toolDoc
}

type templateData struct {
	TotalTools            int
	GroupToolCount        int
	ManagementToolCount   int
	ToolGroupCount        int
	ResourceCount         int
	ResourceTemplateCount int
	PromptCount           int
	Groups                []groupDoc
	Management            []toolDoc
	Resources             []mcpserver.ResourceDef
	ResourceTemplates     []mcpserver.ResourceTemplateDef
	Prompts               []mcpserver.PromptDef
}

func main() {
	output := flag.String("output", "", "Output file path (default: stdout)")
	flag.Parse()

	data := buildTemplateDataForRepo(".")
	rendered, err := renderTemplateData(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "genmcpdocs: render: %v\n", err)
		os.Exit(1)
	}

	var w io.Writer = os.Stdout
	if *output != "" {
		f, err := os.Create(*output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "genmcpdocs: create %s: %v\n", *output, err)
			os.Exit(1)
		}
		defer f.Close()
		w = f
	}

	if _, err := io.WriteString(w, rendered); err != nil {
		fmt.Fprintf(os.Stderr, "genmcpdocs: write: %v\n", err)
		os.Exit(1)
	}
}

func buildTemplateData() templateData {
	return buildTemplateDataForRepo(".")
}

func buildTemplateDataForRepo(repoRoot string) templateData {
	srv := mcpserver.NewServer(repoRoot)
	groups := srv.ToolGroups()
	managementEntries := srv.ManagementTools()

	groupDocs := make([]groupDoc, 0, len(groups))
	groupToolCount := 0
	for _, group := range groups {
		tools := make([]toolDoc, 0, len(group.Tools))
		for _, entry := range group.Tools {
			tools = append(tools, toolFromEntry(entry))
		}
		sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
		groupToolCount += len(tools)
		groupDocs = append(groupDocs, groupDoc{
			Name:        group.Name,
			Description: group.Description,
			Tools:       tools,
		})
	}

	managementDocs := make([]toolDoc, 0, len(managementEntries))
	for _, entry := range managementEntries {
		managementDocs = append(managementDocs, toolFromEntry(entry))
	}
	sort.Slice(managementDocs, func(i, j int) bool { return managementDocs[i].Name < managementDocs[j].Name })

	return templateData{
		TotalTools:            groupToolCount + len(managementDocs),
		GroupToolCount:        groupToolCount,
		ManagementToolCount:   len(managementDocs),
		ToolGroupCount:        len(groupDocs),
		ResourceCount:         len(mcpserver.StaticResources()),
		ResourceTemplateCount: len(mcpserver.ResourceTemplates()),
		PromptCount:           len(mcpserver.Prompts()),
		Groups:                groupDocs,
		Management:            managementDocs,
		Resources:             mcpserver.StaticResources(),
		ResourceTemplates:     mcpserver.ResourceTemplates(),
		Prompts:               mcpserver.Prompts(),
	}
}

func renderTemplateData(data templateData) (string, error) {
	var buf bytes.Buffer
	if err := mdTemplate.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func toolFromEntry(entry mcpserver.ToolEntry) toolDoc {
	required := make(map[string]struct{}, len(entry.Tool.InputSchema.Required))
	for _, name := range entry.Tool.InputSchema.Required {
		required[name] = struct{}{}
	}

	params := make([]paramDoc, 0, len(entry.Tool.InputSchema.Properties))
	for name, raw := range entry.Tool.InputSchema.Properties {
		param := paramDoc{Name: name}
		if schema, ok := raw.(map[string]any); ok {
			param.Type = schemaType(schema["type"])
			if desc, ok := schema["description"].(string); ok {
				param.Description = desc
			}
		}
		if param.Type == "" {
			param.Type = "string"
		}
		_, param.Required = required[name]
		params = append(params, param)
	}
	sort.Slice(params, func(i, j int) bool {
		if params[i].Required != params[j].Required {
			return params[i].Required
		}
		return params[i].Name < params[j].Name
	})

	return toolDoc{
		Name:        entry.Tool.Name,
		Description: entry.Tool.Description,
		Params:      params,
	}
}

func schemaType(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " | ")
	default:
		return ""
	}
}

var mdTemplate = template.Must(template.New("mcp-docs").Funcs(template.FuncMap{
	"requiredBadge": func(required bool) string {
		if required {
			return "**required**"
		}
		return "optional"
	},
}).Parse(`# MCP Server Contract

> Auto-generated by ` + "`" + `go run ./tools/genmcpdocs` + "`" + `. Do not edit manually.

Ralphglasses exposes **{{.TotalTools}} tools**: **{{.GroupToolCount}} grouped tools** across **{{.ToolGroupCount}} deferred-load tool groups** plus **{{.ManagementToolCount}} management tools** that are always available. The server also exposes **{{.ResourceCount}} static resources**, **{{.ResourceTemplateCount}} resource templates**, and **{{.PromptCount}} prompts**.

## Discovery Workflow

- Read ` + "`" + `ralph:///catalog/server` + "`" + ` for the live server contract.
- Read ` + "`" + `ralph:///catalog/tool-groups` + "`" + ` for grouped capabilities and counts.
- Read ` + "`" + `ralph:///catalog/skills` + "`" + ` for the focused workflow skill families.
- Read ` + "`" + `ralph:///catalog/workflows` + "`" + ` for common operator playbooks.
- Read ` + "`" + `ralph:///catalog/cli-parity` + "`" + ` when the task is about CLI-to-MCP/skill coverage.
- Read ` + "`" + `ralph:///bootstrap/checklist` + "`" + ` and ` + "`" + `ralph:///runtime/health` + "`" + ` when the task is bootstrap- or runtime-heavy.
- Call ` + "`" + `ralphglasses_tool_groups` + "`" + ` to list or search groups, skills, and workflows, then ` + "`" + `ralphglasses_load_tool_group` + "`" + ` before invoking non-core grouped tools in deferred mode.

## Management Tools

| Tool | Description |
|------|-------------|
{{- range .Management}}
| ` + "`" + `{{.Name}}` + "`" + ` | {{.Description}} |
{{- end}}

## Tool Groups

| Group | Tools | Description |
|-------|-------|-------------|
{{- range .Groups}}
| ` + "`" + `{{.Name}}` + "`" + ` | {{len .Tools}} | {{.Description}} |
{{- end}}

## Resources

### Static Resources

| URI | Name | Description |
|-----|------|-------------|
{{- range .Resources}}
| ` + "`" + `{{.URI}}` + "`" + ` | {{.Name}} | {{.Description}} |
{{- end}}

### Resource Templates

| URI | Name | Description |
|-----|------|-------------|
{{- range .ResourceTemplates}}
| ` + "`" + `{{.URI}}` + "`" + ` | {{.Name}} | {{.Description}} |
{{- end}}

## Prompts

| Prompt | Description |
|--------|-------------|
{{- range .Prompts}}
| ` + "`" + `{{.Name}}` + "`" + ` | {{.Description}} |
{{- end}}

{{range .Groups}}
## {{.Name}}

{{.Description}}

{{range .Tools}}
### ` + "`" + `{{.Name}}` + "`" + `

{{.Description}}
{{if .Params}}
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
{{- range .Params}}
| ` + "`" + `{{.Name}}` + "`" + ` | {{.Type}} | {{requiredBadge .Required}} | {{.Description}} |
{{- end}}
{{else}}
*No parameters.*
{{end}}

{{end}}
{{end}}`))
