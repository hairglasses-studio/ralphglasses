# Notion Database IDs

Look up database IDs here when querying or writing to Notion databases.

## Databases

| Database | ID | Purpose |
|---|---|---|
| Habit Tracker | `PASTE_ID_HERE` | Daily check-ins (workout, journal, mood, sleep, etc.) |
| Contacts / CRM | `PASTE_ID_HERE` | People tracking with birthday alerts |
| Reading List | `PASTE_ID_HERE` | Books and articles with status and ratings |
| Job Tracker | `PASTE_ID_HERE` | Job search pipeline (Lead → Offer/Rejected) |

## Setup Instructions

1. Create each database in Notion UI (API can't create top-level databases)
2. Share each database with the hg-mcp integration
3. Replace `PASTE_ID_HERE` above with the actual database ID from the Notion URL
   - Open the database in Notion → the URL looks like `https://notion.so/YOUR_ID?v=...`
   - The ID is the 32-character hex string before the `?`

## Property Schemas

### Habit Tracker
| Property | Type | Values |
|---|---|---|
| Date | Title | YYYY-MM-DD |
| Workout | Checkbox | |
| Journal | Checkbox | |
| Medicine | Checkbox | |
| Reading | Checkbox | 20+ min |
| Budget Entry | Checkbox | |
| Mood | Select | Great / Good / Okay / Rough / Bad |
| Energy | Select | High / Medium / Low |
| Sleep Hours | Number | |
| Notes | Rich Text | |

### Contacts / CRM
| Property | Type | Values |
|---|---|---|
| Name | Title | |
| Category | Multi-Select | Friend / Family / Professional / Networking / Recruiter |
| Birthday | Date | |
| Last Contact | Date | |
| Follow-Up | Select | None / This Week / This Month / Quarterly |
| Context | Rich Text | How you know them |
| Company | Rich Text | |
| Email | Rich Text | |
| Notes | Rich Text | |

### Reading List
| Property | Type | Values |
|---|---|---|
| Title | Title | |
| Author | Rich Text | |
| Status | Select | Want to Read / Reading / Finished / Abandoned |
| Genre/Topic | Multi-Select | Finance / Fitness / Career / Technical / Productivity |
| Rating | Select | 5-star / 4-star / 3-star / 2-star / 1-star |
| Readwise URL | URL | |
| Key Takeaway | Rich Text | |

### Job Tracker
| Property | Type | Values |
|---|---|---|
| Company | Title | |
| Role | Rich Text | |
| Status | Select | Lead / Applied / Phone Screen / Technical / Onsite / Offer / Rejected |
| Contact | Rich Text | Recruiter name |
| Remote | Select | Remote / Hybrid / Onsite |
| Date Added | Date | |
| Last Activity | Date | |
| Next Step | Rich Text | |
| Notes | Rich Text | |
