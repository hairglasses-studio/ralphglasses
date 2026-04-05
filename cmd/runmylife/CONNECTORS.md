# Connectors — External Integrations

## Active Connectors

### Google Calendar
- **Status:** Connected
- **Access:** Read events, check free/busy, create/update/delete events
- **Use case:** Pull real calendar data for daily briefings and weekly schedules instead of manual exports
- **Prompt example:**
  ```
  Use my Google Calendar to pull today's events. Generate output/daily-briefing-YYYY-MM-DD.md with the real schedule, top 3 priorities from Todoist, and upcoming deadlines.
  ```

### Gmail
- **Status:** Connected
- **Access:** Search messages, read threads, create drafts
- **Use case:** Triage unread emails, draft replies, surface action items
- **Prompt example:**
  ```
  Search my Gmail for unread messages from the past 3 days. Summarize each one in emails/triage-YYYY-MM-DD.md with sender, subject, urgency level, and suggested action. Draft replies for anything urgent.
  ```

### Todoist
- **Status:** Connected
- **Setup:** `codex mcp add todoist --url https://ai.todoist.net/mcp`
- **Claude compatibility:** `claude mcp add --transport http todoist https://ai.todoist.net/mcp`
- **Access:** Create/read/update/complete tasks, manage projects and sections
- **Use case:** Source of truth for all tasks. `todos/master.md` is an auto-generated snapshot.
- **Configuration:**
  - Project: "LifeOrganizer"
  - Sections: Financial, Career, Personal, Home, Projects
  - Priority mapping: P1 = Do First, P2 = Schedule, P3 = Delegate, P4 = Eliminate

### Open-Meteo (Weather)
- **Status:** Connected
- **Setup:** Added to `claude_desktop_config.json` as npx MCP server
- **Access:** Weather forecasts, historical weather data
- **Use case:** Weather section in daily briefings (3 lines: today, tomorrow, week trend)
- **Configuration:**
  - Location coordinates in `reference/location.md`
  - No API key required
  - Requires Node.js >= 22

### Notion (via hg-mcp)
- **Status:** Connected
- **Setup:** Local hg-mcp MCP server with `aftrs_notion_*` tools
- **Access:** Search, read pages/databases, create database entries, update pages, query with filters
- **Use case:** Structured data tracking for habits, contacts, reading list, job search
- **Databases:**

  | Database | Purpose |
  |---|---|
  | Habit Tracker | Daily check-ins (workout, journal, mood, sleep, etc.) |
  | Contacts / CRM | People tracking with birthday alerts |
  | Reading List | Books/articles with status and ratings |
  | Job Tracker | Job search pipeline Kanban |

- **Key tools:**
  - `aftrs_notion_create_db_entry` — add rows with typed properties
  - `aftrs_notion_update_page` — update existing rows
  - `aftrs_notion_database_query` — query with select/date/checkbox/number filters
- **IDs:** Stored in `reference/notion-databases.md`

## Recommended Additional Connectors

Set these up via **Settings > Connectors** in Claude Desktop when ready:

### Google Drive / Dropbox
- Sync reference documents automatically
- Pull shared docs into `reference/`

### Slack
- Surface important messages for daily briefings
- Track action items from channels

### Readwise (Deferred)
- When ready: `@readwise/readwise-mcp` + `READWISE_TOKEN` env var
- "This Week's Reading" section in weekly reviews with top highlights
- Generate book notes as `reference/reading-notes-[book-slug].md`
- Sync reading status to Notion Reading List database
