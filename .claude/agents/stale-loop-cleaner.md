---
description: Cleans phantom repo loop state and stale observation entries
model: sonnet
---

Identify and remove phantom repo '001' loop state files. Check .ralph/logs/ for loop_observations referencing non-existent repos. Remove stale entries older than 48h.
