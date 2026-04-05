// Package drive provides MCP tools for Google Drive file management.
package drive

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/clients"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

// Module implements the ToolModule interface for Google Drive.
type Module struct{}

func (m *Module) Name() string        { return "drive" }
func (m *Module) Description() string { return "Google Drive file management" }

var driveHints = map[string]string{
	"files/list":    "List files in Drive or a specific folder",
	"files/search":  "Search files by name",
	"files/get":     "Get details for a specific file",
	"files/recent":  "List recently modified files",
	"folders/list":  "List all folders",
	"folders/tree":  "Show folder structure",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("drive").
		Domain("files", common.ActionRegistry{
			"list":   handleFilesList,
			"search": handleFilesSearch,
			"get":    handleFilesGet,
			"recent": handleFilesRecent,
		}).
		Domain("folders", common.ActionRegistry{
			"list": handleFoldersList,
			"tree": handleFoldersTree,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_drive",
				mcp.WithDescription(
					"Google Drive gateway. Browse, search, and inspect files.\n\n"+
						dispatcher.DescribeActionsWithHints(driveHints),
				),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: files, folders")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("file_id", mcp.Description("File ID (for get)")),
				mcp.WithString("folder_id", mcp.Description("Folder ID (for list)")),
				mcp.WithString("query", mcp.Description("Search query (for search)")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
			),
			Handler:             tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:            "drive",
			Subcategory:         "gateway",
			Tags:                []string{"drive", "files", "google"},
			Complexity:          tools.ComplexityModerate,
			IsWrite:             false,
			CircuitBreakerGroup: "drive_api",
			Timeout:             60 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

// loadDriveToken reads the Google OAuth access token from the token file.
func loadDriveToken() (string, error) {
	tokenPath := clients.GoogleTokenPath()
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return "", fmt.Errorf("no Google OAuth token found at %s", tokenPath)
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(data, &token); err != nil {
		return "", fmt.Errorf("parse token: %w", err)
	}
	if token.AccessToken == "" {
		return "", fmt.Errorf("access_token is empty in %s", tokenPath)
	}
	return token.AccessToken, nil
}

// getDriveClient loads the token and creates a DriveClient.
func getDriveClient() (*clients.DriveClient, error) {
	token, err := loadDriveToken()
	if err != nil {
		return nil, err
	}
	return clients.NewDriveClient(token), nil
}

func handleFilesList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := getDriveClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err,
			"Run Google OAuth flow to enable Drive access",
			"runmylife google-auth to authenticate"), nil
	}

	folderID := common.GetStringParam(req, "folder_id", "")
	limit := common.GetLimitParam(req, 20)

	files, err := client.ListFiles(ctx, folderID, limit)
	if err != nil {
		return common.CodedErrorResult(common.ClassifyClientError(err), err), nil
	}

	title := "Drive Files"
	if folderID != "" {
		title = fmt.Sprintf("Files in Folder %s", folderID)
	}
	return formatFileTable(title, files), nil
}

func handleFilesSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, ok := common.RequireStringParam(req, "query")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "query is required for search"), nil
	}

	client, err := getDriveClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err,
			"Run Google OAuth flow to enable Drive access"), nil
	}

	limit := common.GetLimitParam(req, 20)
	files, err := client.SearchFiles(ctx, query, limit)
	if err != nil {
		return common.CodedErrorResult(common.ClassifyClientError(err), err), nil
	}

	return formatFileTable(fmt.Sprintf("Search: %s", query), files), nil
}

func handleFilesGet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fileID, ok := common.RequireStringParam(req, "file_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "file_id is required"), nil
	}

	client, err := getDriveClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err,
			"Run Google OAuth flow to enable Drive access"), nil
	}

	file, err := client.GetFile(ctx, fileID)
	if err != nil {
		return common.CodedErrorResult(common.ClassifyClientError(err), err), nil
	}

	md := common.NewMarkdownBuilder().Title("File Details")
	md.KeyValue("ID", file.ID)
	md.KeyValue("Name", file.Name)
	md.KeyValue("Type", file.MimeType)
	md.KeyValue("Modified", file.ModifiedAt)
	md.KeyValue("Shared", fmt.Sprintf("%v", file.Shared))
	if file.Size > 0 {
		md.KeyValue("Size", formatFileSize(file.Size))
	}
	if file.WebLink != "" {
		md.KeyValue("Link", file.WebLink)
	}
	if len(file.Parents) > 0 {
		md.KeyValue("Parent", file.Parents[0])
	}

	return tools.TextResult(md.String()), nil
}

func handleFilesRecent(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := getDriveClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err,
			"Run Google OAuth flow to enable Drive access"), nil
	}

	limit := common.GetLimitParam(req, 20)
	files, err := client.ListFiles(ctx, "", limit)
	if err != nil {
		return common.CodedErrorResult(common.ClassifyClientError(err), err), nil
	}

	return formatFileTable("Recent Files", files), nil
}

func handleFoldersList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := getDriveClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err,
			"Run Google OAuth flow to enable Drive access"), nil
	}

	limit := common.GetLimitParam(req, 20)
	folders, err := client.ListFolders(ctx, limit)
	if err != nil {
		return common.CodedErrorResult(common.ClassifyClientError(err), err), nil
	}

	return formatFileTable("Folders", folders), nil
}

func handleFoldersTree(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := getDriveClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err,
			"Run Google OAuth flow to enable Drive access"), nil
	}

	limit := common.GetLimitParam(req, 50)
	folders, err := client.ListFolders(ctx, limit)
	if err != nil {
		return common.CodedErrorResult(common.ClassifyClientError(err), err), nil
	}

	md := common.NewMarkdownBuilder().Title("Folder Tree")

	if len(folders) == 0 {
		md.EmptyList("folders")
		return tools.TextResult(md.String()), nil
	}

	var items []string
	for _, f := range folders {
		item := fmt.Sprintf("%s (id: %s)", f.Name, f.ID)
		if f.ModifiedAt != "" {
			item += fmt.Sprintf(" — modified %s", f.ModifiedAt)
		}
		items = append(items, item)
	}
	md.List(items)

	return tools.TextResult(md.String()), nil
}

// formatFileTable creates a markdown table from a list of DriveFiles.
func formatFileTable(title string, files []clients.DriveFile) *mcp.CallToolResult {
	md := common.NewMarkdownBuilder().Title(title)

	if len(files) == 0 {
		md.EmptyList("files")
		return tools.TextResult(md.String())
	}

	headers := []string{"Name", "Type", "Modified", "Shared", "ID"}
	var rows [][]string
	for _, f := range files {
		mimeShort := shortMimeType(f.MimeType)
		modified := f.ModifiedAt
		if len(modified) > 10 {
			modified = modified[:10]
		}
		shared := "No"
		if f.Shared {
			shared = "Yes"
		}
		id := f.ID
		if len(id) > 12 {
			id = id[:12] + "..."
		}
		rows = append(rows, []string{f.Name, mimeShort, modified, shared, id})
	}
	md.Table(headers, rows)

	return tools.TextResult(md.String())
}

// shortMimeType returns a human-friendly version of a MIME type.
func shortMimeType(mime string) string {
	switch mime {
	case "application/vnd.google-apps.folder":
		return "Folder"
	case "application/vnd.google-apps.document":
		return "Doc"
	case "application/vnd.google-apps.spreadsheet":
		return "Sheet"
	case "application/vnd.google-apps.presentation":
		return "Slides"
	case "application/vnd.google-apps.form":
		return "Form"
	case "application/pdf":
		return "PDF"
	case "image/png":
		return "PNG"
	case "image/jpeg":
		return "JPEG"
	case "text/plain":
		return "Text"
	default:
		if len(mime) > 20 {
			return mime[:20] + "..."
		}
		return mime
	}
}

// formatFileSize returns a human-readable file size.
func formatFileSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
