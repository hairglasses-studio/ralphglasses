// Package contacts provides MCP tools for contact management.
package contacts

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

// Module implements the ToolModule interface for contact management.
type Module struct{}

func (m *Module) Name() string        { return "contacts" }
func (m *Module) Description() string { return "Contact management with search, merge, and import" }

var contactsHints = map[string]string{
	"manage/list":     "List contacts with pagination",
	"manage/search":   "Search contacts by name, email, or phone",
	"manage/get":      "Get full contact details",
	"manage/add":      "Add a new contact",
	"manage/update":   "Update contact fields",
	"manage/delete":   "Delete a contact",
	"manage/merge":    "Merge two duplicate contacts",
	"import/gmail":    "Import contacts from Gmail senders",
	"import/messages": "Import contacts from SMS conversations",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("contacts").
		Domain("manage", common.ActionRegistry{
			"list":   handleManageList,
			"search": handleManageSearch,
			"get":    handleManageGet,
			"add":    handleManageAdd,
			"update": handleManageUpdate,
			"delete": handleManageDelete,
			"merge":  handleManageMerge,
		}).
		Domain("import", common.ActionRegistry{
			"gmail":    handleImportGmail,
			"messages": handleImportMessages,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_contacts",
				mcp.WithDescription(
					"Contact management gateway. Store, search, and manage personal contacts.\n\n"+
						dispatcher.DescribeActionsWithHints(contactsHints),
				),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: manage, import")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("contact_id", mcp.Description("Contact ID")),
				mcp.WithString("target_id", mcp.Description("Target contact ID (for merge)")),
				mcp.WithString("name", mcp.Description("Contact name")),
				mcp.WithString("email", mcp.Description("Email address")),
				mcp.WithString("phone", mcp.Description("Phone number")),
				mcp.WithString("source", mcp.Description("Contact source (e.g. gmail, manual)")),
				mcp.WithString("notes", mcp.Description("Notes about the contact")),
				mcp.WithString("tags", mcp.Description("Comma-separated tags")),
				mcp.WithString("query", mcp.Description("Search query")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
				mcp.WithNumber("offset", mcp.Description("Pagination offset (default 0)")),
			),
			Handler:    tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:   "contacts",
			Tags:       []string{"contacts", "people", "networking"},
			Complexity: tools.ComplexityModerate,
			IsWrite:    true,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func handleManageList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	limit := common.GetLimitParam(req, 20)
	offset := common.GetOffsetParam(req)

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	rows, err := database.SqlDB().QueryContext(ctx,
		"SELECT id, name, email, phone, source, tags FROM contacts ORDER BY name ASC LIMIT ? OFFSET ?",
		limit, offset,
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Contacts")
	headers := []string{"Name", "Email", "Phone", "Source", "Tags"}
	var tableRows [][]string

	for rows.Next() {
		var id, name, email, phone, source, tags string
		if err := rows.Scan(&id, &name, &email, &phone, &source, &tags); err != nil {
			continue
		}
		tableRows = append(tableRows, []string{name, email, phone, source, tags})
	}

	if len(tableRows) == 0 {
		md.EmptyList("contacts")
	} else {
		md.Table(headers, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleManageSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query := common.GetStringParam(req, "query", "")
	if query == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "query is required for manage/search"), nil
	}
	limit := common.GetLimitParam(req, 20)

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	pattern := "%" + query + "%"
	rows, err := database.SqlDB().QueryContext(ctx,
		"SELECT id, name, email, phone, source, tags FROM contacts WHERE name LIKE ? OR email LIKE ? OR phone LIKE ? ORDER BY name ASC LIMIT ?",
		pattern, pattern, pattern, limit,
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Contacts: \"%s\"", query))
	headers := []string{"Name", "Email", "Phone", "Source", "Tags"}
	var tableRows [][]string

	for rows.Next() {
		var id, name, email, phone, source, tags string
		if err := rows.Scan(&id, &name, &email, &phone, &source, &tags); err != nil {
			continue
		}
		tableRows = append(tableRows, []string{name, email, phone, source, tags})
	}

	if len(tableRows) == 0 {
		md.EmptyList("matching contacts")
	} else {
		md.Table(headers, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleManageGet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contactID, ok := common.RequireStringParam(req, "contact_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "contact_id is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	var name, email, phone, source, notes, tags, createdAt, updatedAt string
	err = database.SqlDB().QueryRowContext(ctx,
		"SELECT name, COALESCE(email, ''), COALESCE(phone, ''), COALESCE(source, ''), COALESCE(notes, ''), COALESCE(tags, ''), created_at, COALESCE(updated_at, '') FROM contacts WHERE id = ?",
		contactID,
	).Scan(&name, &email, &phone, &source, &notes, &tags, &createdAt, &updatedAt)
	if err != nil {
		return common.CodedErrorResultf(common.ErrNotFound, "contact %s not found", contactID), nil
	}

	md := common.NewMarkdownBuilder().Title(name)
	md.KeyValue("ID", contactID)
	md.KeyValue("Email", email)
	md.KeyValue("Phone", phone)
	md.KeyValue("Source", source)
	md.KeyValue("Tags", tags)
	md.KeyValue("Notes", notes)
	md.KeyValue("Created", createdAt)
	if updatedAt != "" {
		md.KeyValue("Updated", updatedAt)
	}

	return tools.TextResult(md.String()), nil
}

func handleManageAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := common.GetStringParam(req, "name", "")
	if name == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "name is required for manage/add"), nil
	}
	email := common.GetStringParam(req, "email", "")
	phone := common.GetStringParam(req, "phone", "")
	source := common.GetStringParam(req, "source", "manual")
	notes := common.GetStringParam(req, "notes", "")
	tags := common.GetStringParam(req, "tags", "")

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	id := fmt.Sprintf("contact-%d", time.Now().UnixNano())
	_, err = database.SqlDB().ExecContext(ctx,
		`INSERT INTO contacts (id, name, email, phone, source, notes, tags, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
		id, name, email, phone, source, notes, tags,
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	return tools.TextResult(fmt.Sprintf("# Contact Added\n\n- **ID:** %s\n- **Name:** %s\n- **Email:** %s\n- **Phone:** %s",
		id, name, email, phone)), nil
}

func handleManageUpdate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contactID, ok := common.RequireStringParam(req, "contact_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "contact_id is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	name := common.GetStringParam(req, "name", "")
	email := common.GetStringParam(req, "email", "")
	phone := common.GetStringParam(req, "phone", "")
	source := common.GetStringParam(req, "source", "")
	notes := common.GetStringParam(req, "notes", "")
	tags := common.GetStringParam(req, "tags", "")

	if name != "" {
		database.SqlDB().ExecContext(ctx, "UPDATE contacts SET name = ?, updated_at = datetime('now') WHERE id = ?", name, contactID)
	}
	if email != "" {
		database.SqlDB().ExecContext(ctx, "UPDATE contacts SET email = ?, updated_at = datetime('now') WHERE id = ?", email, contactID)
	}
	if phone != "" {
		database.SqlDB().ExecContext(ctx, "UPDATE contacts SET phone = ?, updated_at = datetime('now') WHERE id = ?", phone, contactID)
	}
	if source != "" {
		database.SqlDB().ExecContext(ctx, "UPDATE contacts SET source = ?, updated_at = datetime('now') WHERE id = ?", source, contactID)
	}
	if notes != "" {
		database.SqlDB().ExecContext(ctx, "UPDATE contacts SET notes = ?, updated_at = datetime('now') WHERE id = ?", notes, contactID)
	}
	if tags != "" {
		database.SqlDB().ExecContext(ctx, "UPDATE contacts SET tags = ?, updated_at = datetime('now') WHERE id = ?", tags, contactID)
	}

	return tools.TextResult(fmt.Sprintf("Contact %s updated.", contactID)), nil
}

func handleManageDelete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contactID, ok := common.RequireStringParam(req, "contact_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "contact_id is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	_, err = database.SqlDB().ExecContext(ctx, "DELETE FROM contacts WHERE id = ?", contactID)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	return tools.TextResult(fmt.Sprintf("Contact %s deleted.", contactID)), nil
}

func handleManageMerge(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contactID, ok := common.RequireStringParam(req, "contact_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "contact_id is required (primary contact)"), nil
	}
	targetID := common.GetStringParam(req, "target_id", "")
	if targetID == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "target_id is required (contact to merge into primary)"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	// Read both contacts
	var primaryName, primaryEmail, primaryPhone, primarySource, primaryNotes, primaryTags string
	err = database.SqlDB().QueryRowContext(ctx,
		"SELECT name, COALESCE(email, ''), COALESCE(phone, ''), COALESCE(source, ''), COALESCE(notes, ''), COALESCE(tags, '') FROM contacts WHERE id = ?",
		contactID,
	).Scan(&primaryName, &primaryEmail, &primaryPhone, &primarySource, &primaryNotes, &primaryTags)
	if err != nil {
		return common.CodedErrorResultf(common.ErrNotFound, "primary contact %s not found", contactID), nil
	}

	var targetName, targetEmail, targetPhone, targetSource, targetNotes, targetTags string
	err = database.SqlDB().QueryRowContext(ctx,
		"SELECT name, COALESCE(email, ''), COALESCE(phone, ''), COALESCE(source, ''), COALESCE(notes, ''), COALESCE(tags, '') FROM contacts WHERE id = ?",
		targetID,
	).Scan(&targetName, &targetEmail, &targetPhone, &targetSource, &targetNotes, &targetTags)
	if err != nil {
		return common.CodedErrorResultf(common.ErrNotFound, "target contact %s not found", targetID), nil
	}

	// Merge: fill empty fields in primary from target
	if primaryEmail == "" && targetEmail != "" {
		primaryEmail = targetEmail
	}
	if primaryPhone == "" && targetPhone != "" {
		primaryPhone = targetPhone
	}
	if primarySource == "" && targetSource != "" {
		primarySource = targetSource
	}
	if primaryNotes == "" && targetNotes != "" {
		primaryNotes = targetNotes
	} else if primaryNotes != "" && targetNotes != "" {
		primaryNotes = primaryNotes + "\n" + targetNotes
	}
	if primaryTags == "" && targetTags != "" {
		primaryTags = targetTags
	} else if primaryTags != "" && targetTags != "" {
		primaryTags = primaryTags + "," + targetTags
	}

	// Update primary
	_, err = database.SqlDB().ExecContext(ctx,
		"UPDATE contacts SET email = ?, phone = ?, source = ?, notes = ?, tags = ?, updated_at = datetime('now') WHERE id = ?",
		primaryEmail, primaryPhone, primarySource, primaryNotes, primaryTags, contactID,
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	// Delete target
	_, err = database.SqlDB().ExecContext(ctx, "DELETE FROM contacts WHERE id = ?", targetID)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	return tools.TextResult(fmt.Sprintf("# Contacts Merged\n\n- **Primary:** %s (%s)\n- **Merged from:** %s (%s)\n- **Result:** %s now has combined data",
		primaryName, contactID, targetName, targetID, primaryName)), nil
}

func handleImportGmail(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return tools.TextResult("Configure Gmail OAuth to enable contact import from email senders."), nil
}

func handleImportMessages(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return tools.TextResult("Configure Google Messages pairing to enable contact import from SMS."), nil
}
