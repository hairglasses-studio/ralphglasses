# Scheduling Guide for Claude Desktop Cowork

Set up these scheduled tasks in Claude Desktop via `/schedule` or the **Scheduled** sidebar.

---

## Required MCP Connections

Before scheduling, ensure these MCP servers are connected:

### Google Calendar (Built-in)
Already configured via Claude Desktop Settings > Connectors.

### Gmail (Built-in)
Already configured via Claude Desktop Settings > Connectors.

### Todoist
```
codex mcp add todoist --url https://ai.todoist.net/mcp
```

Claude Desktop / Claude Code compatibility:
```
claude mcp add --transport http todoist https://ai.todoist.net/mcp
```
Complete OAuth flow in Claude Desktop when prompted.

### Open-Meteo (Weather)
Add to Claude Desktop MCP config (`claude_desktop_config.json`):
```json
{
  "mcpServers": {
    "open-meteo": {
      "command": "npx",
      "args": ["-y", "-p", "open-meteo-mcp-server", "open-meteo-mcp-server"]
    }
  }
}
```
Requires Node.js >= 22. No API key needed.

### hg-mcp (Notion + Studio Tools)
Already configured as local MCP server. Provides `aftrs_notion_*` tools for database operations.

---

## 1. Daily Morning Briefing

**Schedule:** Every day at 7:00 AM
**Prompt:**

```
Working directory: \\wsl$\Ubuntu\home\hairglasses\Docs\hairglasses\LifeOrganizer

Generate output/daily-briefing-YYYY-MM-DD.md from these sources:

1. **Weather:** Read reference/location.md for coordinates. Use Open-Meteo MCP for
   today's forecast — high/low, conditions, precipitation %. 3-day outlook.
2. **Schedule:** Use Google Calendar MCP for today's events.
3. **Tasks:** Use Todoist MCP to get active tasks from "LifeOrganizer" project.
   Top 3 priorities (P1 first, then P2 by nearest deadline).
4. **Birthdays:** Query Contacts database (ID from reference/notion-databases.md)
   for birthdays today or within 3 days.
5. **Job follow-ups:** Query Job Tracker for entries where Last Activity > 7 days ago
   and Status is Lead or Applied.
6. **Emails:** Use Gmail MCP for unread messages needing action (3-5 max).
7. **Deadlines:** From Todoist, anything due within 7 days.

Also regenerate todos/master.md as a snapshot from Todoist.
Use today's date in the filename.
```

---

## 2. Weekly Review

**Schedule:** Every Sunday at 6:00 PM
**Prompt:**

```
Working directory: \\wsl$\Ubuntu\home\hairglasses\Docs\hairglasses\LifeOrganizer

Generate output/weekly-review-YYYY-MM-DD.md from:

1. **Completed tasks:** Todoist MCP — tasks completed this week
2. **Active tasks:** Todoist MCP — open tasks grouped by priority
3. **Habit scorecard:** Query Habit Tracker (reference/notion-databases.md) for this
   week. Show completion rates per habit and note streaks.
4. **Job search:** Query Job Tracker for status changes this week.
5. **Journals:** Review journal/ entries from this week.
6. **Daily briefings:** Review output/daily-briefing-*.md from this week.
7. **Next week preview:** Google Calendar MCP for next week's events.

Also regenerate todos/master.md snapshot. Use Sunday's date.
```

---

## 3. Inbox Sweep

**Schedule:** Weekdays (Mon-Fri) at 8:00 AM
**Prompt:**

```
Working directory: \\wsl$\Ubuntu\home\hairglasses\Docs\hairglasses\LifeOrganizer

Check inbox/ for any new files. If there are files:
1. Sort them into appropriate folders (todos/, calendar/, emails/, finances/, journal/, reference/)
2. Rename files descriptively using lowercase-kebab-case with dates
3. Append what was processed to output/inbox-triage-log.md with today's date

If inbox/ is empty (only .gitkeep), do nothing.
```

---

## 4. Monthly Budget Summary

**Schedule:** Last day of each month at 5:00 PM
**Prompt:**

```
Working directory: \\wsl$\Ubuntu\home\hairglasses\Docs\hairglasses\LifeOrganizer

Compile all expenses from finances/ for this month. Generate finances/monthly-summary-YYYY-MM.md with:
- Spending by category (table format)
- Total spent
- Comparison to last month (if finances/monthly-summary-YYYY-MM.md exists for prior month)
- Flag any charges over $100 or unusual patterns
```

---

## 5. Daily Journal Prompt

**Schedule:** Every day at 8:00 PM
**Prompt:**

```
Working directory: \\wsl$\Ubuntu\home\hairglasses\Docs\hairglasses\LifeOrganizer

Create journal/YYYY-MM-DD.md with today's date. Include these prompts with space for answers:
- What went well today?
- What was challenging?
- What am I grateful for?
- One thing I'd do differently?

If yesterday's journal exists and has been filled in (content beyond the template prompts), include a brief reflection noting themes or patterns between yesterday and today.
```

---

## 6. Evening Habit Check-In

**Schedule:** Every day at 9:00 PM
**Prompt:**

```
Working directory: \\wsl$\Ubuntu\home\hairglasses\Docs\hairglasses\LifeOrganizer

Check if today has an entry in the Habit Tracker (reference/notion-databases.md).
If not, create one.

Auto-detect what you can:
- Journal: check if journal/YYYY-MM-DD.md has content beyond template
- Medicine: check calendar for "Take medicine" event
- Budget Entry: check if finances/expenses-YYYY-MM.md was updated today
- Workout: check if today is a workout day per reference/weekly-workout-plan.md

For anything that can't be auto-detected (Reading, Mood, Energy, Sleep Hours),
ask me. Report what was logged.
```

---

## 7. Quarterly Life Review

**Schedule:** Last day of March, June, September, December at 4:00 PM
**Prompt:**

```
Working directory: \\wsl$\Ubuntu\home\hairglasses\Docs\hairglasses\LifeOrganizer

Analyze from the past 3 months:
- All journal entries in journal/
- Weekly reviews in output/weekly-review-*.md
- Todoist MCP: completed tasks from the quarter
- Habit Tracker (reference/notion-databases.md): habit trends and streaks
- Job Tracker: pipeline progression and outcomes

Generate output/quarterly-review-YYYY-QN.md with:
- Major accomplishments
- Recurring challenges
- Goal progress
- Habit scorecard (quarterly completion rates)
- Job search status and pipeline summary
- Energy and mood patterns (from journal entries and habit tracker)
- Recommended focus areas for next quarter
```

---

## Setup Checklist

- [ ] Open Claude Desktop on Windows
- [ ] Go to Settings > Cowork > Global instructions and paste from `.cowork-instructions.md`
- [ ] Verify MCP connections:
  - [ ] Google Calendar — connected via Settings > Connectors
  - [ ] Gmail — connected via Settings > Connectors
  - [ ] Todoist — run `codex mcp add todoist --url https://ai.todoist.net/mcp` or `claude mcp add --transport http todoist https://ai.todoist.net/mcp`, then complete OAuth
  - [ ] Open-Meteo — add to `claude_desktop_config.json` (see Required MCP Connections above)
  - [ ] hg-mcp — running as local MCP server with Notion tools
- [ ] Verify Node.js >= 22 is installed
- [ ] Create 4 Notion databases and fill in IDs in `reference/notion-databases.md`
- [ ] For each scheduled task above, use `/schedule` and paste the prompt
- [ ] Verify each appears in the **Scheduled** sidebar
- [ ] Keep Claude Desktop open and computer awake for tasks to run
- [ ] Windows path to this folder: `\\wsl$\Ubuntu\home\hairglasses\Docs\hairglasses\LifeOrganizer`
