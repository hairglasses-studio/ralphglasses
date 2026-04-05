package common

import (
	"fmt"
	"strings"
)

// MarkdownBuilder helps construct consistent markdown output.
type MarkdownBuilder struct {
	sb strings.Builder
}

// NewMarkdownBuilder creates a new markdown builder.
func NewMarkdownBuilder() *MarkdownBuilder {
	return &MarkdownBuilder{}
}

// Title adds a level-1 header.
func (m *MarkdownBuilder) Title(title string) *MarkdownBuilder {
	m.sb.WriteString("# ")
	m.sb.WriteString(title)
	m.sb.WriteString("\n\n")
	return m
}

// Section adds a level-2 header.
func (m *MarkdownBuilder) Section(title string) *MarkdownBuilder {
	m.sb.WriteString("## ")
	m.sb.WriteString(title)
	m.sb.WriteString("\n\n")
	return m
}

// Text adds a paragraph of text.
func (m *MarkdownBuilder) Text(text string) *MarkdownBuilder {
	m.sb.WriteString(text)
	m.sb.WriteString("\n\n")
	return m
}

// Bold adds a bold label with value.
func (m *MarkdownBuilder) Bold(label, value string) *MarkdownBuilder {
	m.sb.WriteString(fmt.Sprintf("**%s:** %s\n", label, value))
	return m
}

// KeyValue adds a key-value pair.
func (m *MarkdownBuilder) KeyValue(key, value string) *MarkdownBuilder {
	m.sb.WriteString(fmt.Sprintf("- **%s:** %s\n", key, value))
	return m
}

// List adds an unordered list.
func (m *MarkdownBuilder) List(items []string) *MarkdownBuilder {
	for _, item := range items {
		m.sb.WriteString("- ")
		m.sb.WriteString(item)
		m.sb.WriteString("\n")
	}
	m.sb.WriteString("\n")
	return m
}

// Table adds a markdown table.
func (m *MarkdownBuilder) Table(headers []string, rows [][]string) *MarkdownBuilder {
	m.sb.WriteString("| ")
	m.sb.WriteString(strings.Join(headers, " | "))
	m.sb.WriteString(" |\n|")
	for range headers {
		m.sb.WriteString("------|")
	}
	m.sb.WriteString("\n")
	for _, row := range rows {
		m.sb.WriteString("| ")
		m.sb.WriteString(strings.Join(row, " | "))
		m.sb.WriteString(" |\n")
	}
	m.sb.WriteString("\n")
	return m
}

// Pagination adds a standard pagination footer.
func (m *MarkdownBuilder) Pagination(shown, total, offset int) *MarkdownBuilder {
	m.sb.WriteString(fmt.Sprintf("\n---\n*Showing %d of %d (offset %d)*\n", shown, total, offset))
	return m
}

// EmptyList adds a standard empty-list message.
func (m *MarkdownBuilder) EmptyList(noun string) *MarkdownBuilder {
	m.sb.WriteString(fmt.Sprintf("No %s found.\n", noun))
	return m
}

// String returns the built markdown string.
func (m *MarkdownBuilder) String() string {
	return m.sb.String()
}

// TruncateWords truncates a string to a maximum number of characters.
func TruncateWords(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
