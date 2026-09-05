---
paths:
  - "ai-agents/skills/**"
  - "ai-agents/personal/skills/**"
  - ".claude/skills/**"
---

# Skill authoring rules

## Where a skill belongs

Skills live in three roots. Pick by **where the skill can actually run**, not by topic.

| Root                         | Scope                                                | Distribution                                           |
| ---------------------------- | ---------------------------------------------------- | ------------------------------------------------------ |
| `ai-agents/skills/`          | Runs in any repository — the shared development set  | `mise run skills-copy` + plugin `ai-agents@my-pde`     |
| `ai-agents/personal/skills/` | Runs anywhere but is hobby/private life, not work    | `mise run skills-copy` + plugin `personal@my-pde`      |
| `.claude/skills/`            | Rewrites this repository's own paths, so my-pde only | None — it is part of the clone (and of cloud Routines) |

- A skill that writes to `ai-agents/**`, `routines/**`, or `agents/codex/` belongs in `.claude/skills/`.
- A skill a Stop hook launches from an arbitrary repository (`skill-observe`, `permission-prompt-tuner`) must stay in `ai-agents/skills/`, even when its records land in the my-pde checkout.
- `skill-observe` / `skill-improve` resolve a skill by searching the three roots in the order above.

## SKILL.md

- Required SKILL.md frontmatter: `name`, `description` (include the trigger phrases that should invoke it), and `metadata.version`. Add `argument-hint` for argument-taking skills, and `disable-model-invocation: true` (boolean, not the string `"true"`) for orchestrator-only skills.
- Body skeleton: `# /<name> スキル` → `## Goal` / `## Workflow` / `## Notes`. The `skill-scaffold` skill generates it; scaffold a new skill with it instead of hand-rolling SKILL.md.
- Keep SKILL.md focused on the procedure. Move long reference material into a `references/` subdirectory so it loads only when needed (progressive disclosure).
- Improvements flow through the Observe → Inspect → Amend → Evaluate loop (`/skill-observe`, `/skill-improve`). The Observe phase is auto-captured by the `skill-observe-nudge.sh` Stop hook (Claude records observations on session end); `/skill-observe` remains for manual/supplementary recording. Bump `metadata.version` when amending a skill.
