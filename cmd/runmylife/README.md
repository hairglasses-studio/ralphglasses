# runmylife

Go MCP server for personal life organization — tasks, calendar, email, and daily workflows.

## Quick Start

```bash
make build                    # build bin/runmylife-mcp
./bin/runmylife-mcp           # start stdio MCP server
./bin/runmylife-mcp -transport sse  # start SSE server on :8080
```

## Tools

| Tool | Purpose |
|------|---------|
| `runmylife_tasks` | Task management (Todoist integration) |
| `runmylife_calendar` | Google Calendar events & scheduling |
| `runmylife_gmail` | Email search, triage, drafts |
| `runmylife_sync` | Data sync orchestration |
| `runmylife_admin` | DB status, config, health checks |
| `runmylife_tool_*` | Tool discovery & schema introspection |

## Configuration

Create `~/.config/runmylife/config.json`:

```json
{
  "credentials": {
    "todoist": "your-todoist-api-token"
  },
  "location": {
    "city": "San Francisco, CA",
    "latitude": 37.7749,
    "longitude": -122.4194,
    "timezone": "America/Los_Angeles"
  }
}
```

## Data

The `data/` directory contains the filetree layer with journal entries, reference docs, financial records, and generated reports from the original life organizer system.

See [CLAUDE.md](CLAUDE.md) for full architecture documentation.
