# Cowork Prompts

Ready-to-use prompts for Claude Desktop Cowork. Copy-paste these directly.

---

## Phase 2: One-Shot Tasks

### 1. Inbox Triage

```
Sort everything in inbox/ into the appropriate subfolders (todos/, calendar/, emails/, finances/, journal/, reference/). Rename files descriptively using lowercase-kebab-case. Create a log of what you moved and why in output/inbox-triage-log.md.
```

### 2. Todo Snapshot from Todoist

```
Use Todoist MCP to get all active tasks from the "LifeOrganizer" project. Generate todos/master.md as a read-only snapshot using the Eisenhower matrix categories:
- P1 → Urgent + Important (Do First)
- P2 → Important (Schedule)
- P3 → Urgent (Delegate)
- P4 → Neither (Eliminate)

Include due dates, sections, and a header noting this is auto-generated from Todoist.
Format: - [ ] **[DEADLINE: YYYY-MM-DD]** Task description (Section)
```

### 3. Weekly Schedule Builder

```
Read my calendar files in calendar/ and create a weekly schedule at calendar/week-of-YYYY-MM-DD.md (use this Monday's date). Include:
- Time blocks for each commitment
- Prep notes for meetings
- Suggested optimal times for deep work (2-hour blocks)
- Break suggestions (15 min every 90 min)
Format as a day-by-day schedule with headers for each day.
```

### 4. Email Draft Factory

```
Read the context in emails/ and draft responses. Save each draft as emails/draft-[recipient]-[YYYY-MM-DD].md. Each draft should include:
- **Original:** Brief summary of what they said
- **Tone:** formal / casual / friendly-professional
- **Draft:**
  The actual response text
- **Notes:** Any follow-up actions needed
```

### 5. Expense Report from Receipts

```
Extract vendor, date, amount, and category from each receipt in finances/. Compile into a spreadsheet at finances/expenses-YYYY-MM.xlsx with columns: Date, Vendor, Category, Amount, Notes. Include a totals row by category at the bottom. If you can't create xlsx, use finances/expenses-YYYY-MM.csv instead.
```

### 6. Personal Knowledge Audit

```
Audit my reference/ folder. Create reference/knowledge-map.md with:
- **Strong coverage:** Topics where I have solid material
- **Thin coverage:** Topics with minimal material
- **Gaps:** Topics I should research based on my interests
For each gap, suggest 3 specific resources (articles, books, courses).
```

### 7. Todoist Migration (One-Time)

```
Read todos/master.md. For each uncompleted task:
1. Create "LifeOrganizer" project in Todoist (if it doesn't exist)
2. Create sections: Financial, Career, Personal, Home, Projects
3. Add each task with:
   - Priority: "Do First" → P1, "Schedule" → P2, "Delegate" → P3, "Eliminate" → P4
   - Due date from any **[DEADLINE: YYYY-MM-DD]** marker
   - Appropriate section based on content
4. Archive todos/master.md as todos/master-archived-2026-03-08.md
5. Log results to output/todoist-migration-log.md
```

### 8. Seed Notion Contacts

```
Read reference/notion-databases.md for the Contacts / CRM database ID. Review:
- calendar/ for recurring contacts (meeting attendees)
- emails/ for correspondent names and context
- output/career-sync-prep-*.md for recruiter contacts

For each person found, use aftrs_notion_create_db_entry to add them to the Contacts database with:
- Name, Category (Friend/Family/Professional/Networking/Recruiter)
- Company, Email, Context (how I know them)
- Last Contact date (from most recent email/calendar entry)

Log results to output/notion-contacts-seed-log.md.
```

### 9. Seed Notion Reading List

```
Read reference/notion-databases.md for the Reading List database ID. Review:
- reference/knowledge-map.md for suggested books/resources
- todos/master.md for any reading-related tasks

For each book/article found, use aftrs_notion_create_db_entry to add to Reading List with:
- Title, Author (if known), Status (Want to Read / Reading), Genre/Topic

Log results to output/notion-reading-seed-log.md.
```

### 10. Seed Notion Job Tracker

```
Read reference/notion-databases.md for the Job Tracker database ID. Review:
- output/career-sync-prep-*.md for active job leads
- emails/ for recruiter correspondence

For each lead, use aftrs_notion_create_db_entry to add to Job Tracker with:
- Company, Role, Status, Contact (recruiter name), Remote preference
- Date Added, Last Activity, Next Step, Notes

Log results to output/notion-jobs-seed-log.md.
```

---

## Phase 3: Scheduled Tasks

### Daily Morning Briefing
**Schedule:** Daily at 7:00 AM

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

### Weekly Review
**Schedule:** Weekly, Sunday at 6:00 PM

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

### Inbox Sweep
**Schedule:** Weekdays at 8:00 AM

```
Check inbox/ for any new files. Sort them into appropriate folders. Rename descriptively. Update output/inbox-triage-log.md with what was processed and the date.
```

### Monthly Budget Summary
**Schedule:** Monthly, last day of month

```
Compile all expenses from finances/ for this month. Generate finances/monthly-summary-YYYY-MM.md with:
- Spending by category (table format)
- Total spent
- Comparison to last month (if a previous summary exists)
- Flag any charges over $100 or unusual patterns
```

---

## Phase 5: Journal & Reflection

### Daily Journal Prompt
**Schedule:** Daily at 8:00 PM

```
Create journal/YYYY-MM-DD.md with today's date. Include these prompts with space for answers:
- What went well today?
- What was challenging?
- What am I grateful for?
- One thing I'd do differently?

If yesterday's journal (journal/YYYY-MM-DD.md) has been filled in, include a brief reflection noting any themes or patterns.
```

### Evening Habit Check-In
**Schedule:** Daily at 9:00 PM

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

### Quarterly Life Review
**Schedule:** Last day of March, June, September, December

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

## Ad-Hoc Prompts

### Add Contact

```
Add a new contact to the Contacts database (reference/notion-databases.md).

Name: [NAME]
Category: [Friend / Family / Professional / Networking / Recruiter]
Company: [COMPANY]
Email: [EMAIL]
Context: [HOW I KNOW THEM]
Birthday: [YYYY-MM-DD or leave blank]
Follow-Up: [None / This Week / This Month / Quarterly]
```

### Update Job Status

```
Update the job entry for [COMPANY] in the Job Tracker (reference/notion-databases.md).
New status: [Lead / Applied / Phone Screen / Technical / Onsite / Offer / Rejected]
Last Activity: today
Next Step: [DESCRIPTION]
Notes: [ANY ADDITIONAL NOTES]
```

### Career Sync Prep (Notion)

```
Working directory: \\wsl$\Ubuntu\home\hairglasses\Docs\hairglasses\LifeOrganizer

Generate output/career-sync-prep-YYYY-MM-DD.md by querying the Job Tracker
(reference/notion-databases.md) for all active leads (Status not Rejected).
For each, include: Company, Role, Status, Contact, Remote preference, Last Activity, Next Step.
Add a comparison table and talking points for the sync.
```

### Log Habits

```
Log today's habits to the Habit Tracker (reference/notion-databases.md):
- Workout: [yes/no]
- Journal: [yes/no]
- Medicine: [yes/no]
- Reading: [yes/no]
- Budget Entry: [yes/no]
- Mood: [Great / Good / Okay / Rough / Bad]
- Energy: [High / Medium / Low]
- Sleep Hours: [NUMBER]
- Notes: [OPTIONAL]
```
