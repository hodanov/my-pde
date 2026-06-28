---
paths:
  - "ai-agents/skills/**"
---

# Skill authoring rules

- Required SKILL.md frontmatter: `name`, `description` (include the trigger phrases that should invoke it), and `metadata.version`. Add `argument-hint` for argument-taking skills, and `disable-model-invocation: true` (boolean, not the string `"true"`) for orchestrator-only skills.
- Keep SKILL.md focused on the procedure. Move long reference material into a `references/` subdirectory so it loads only when needed (progressive disclosure).
- Improvements flow through the Observe → Inspect → Amend → Evaluate loop (`/skill-observe`, `/skill-improve`). The Observe phase is auto-captured by the `skill-observe-nudge.sh` Stop hook (Claude records observations on session end); `/skill-observe` remains for manual/supplementary recording. Bump `metadata.version` when amending a skill.
