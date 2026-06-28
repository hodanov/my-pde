---
paths:
  - "routines/*.json"
---

# Cloud routine rules

- Routine JSON definitions in `routines/` are the source of truth.
- Changing a routine requires PR review, then a manual `/schedule` update to apply it; there is no CI auto-apply.
- Keep cron schedules in UTC and note the JST equivalent in the description.
