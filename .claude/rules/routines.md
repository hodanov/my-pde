---
paths:
  - "routines/**"
---

# Cloud routine rules

- Routine definitions in `routines/` are the source of truth; prompt bodies live in `routines/prompts/<name>.md` and the JSON `prompt` is a thin pointer to that file.
- Prompt body changes (`routines/prompts/*.md`) take effect on the next run once merged — no apply needed.
- Changing JSON-side fields (cron, model, allowed_tools, the pointer prompt itself) requires PR review, then a manual `/schedule` update; there is no CI auto-apply.
- Keep cron schedules in UTC and note the JST equivalent in the description.
