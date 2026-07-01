---
paths:
  - "docs/plan/**"
---

# Plan document rules

- Filename: `YYYY-MM-DD_slug.md` (date the plan is written, kebab-case slug).
- Title line: `# Plan: <短いタイトル>`.
- Use this section skeleton (omit sections only when truly N/A):
  `## Background` → `## Current structure` → `## Design policy` → `## Implementation steps`
  → `## File changes` → `## Risks and mitigations` → `## Validation` → `## Open questions`.
- `## Background` states the problem/need and intended outcome; `## File changes` names concrete paths.
- Plans are design records, not running logs. Keep them scannable; long rationale goes in prose, not nested lists.
