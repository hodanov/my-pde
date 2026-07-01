---
paths:
  - "ai-agents/agents/**"
---

# Subagent authoring rules

- Required frontmatter: `name`, `description`, `tools`. The `description` MUST state the agent's phase/role
  and the trigger condition ("Use this when…", "Use after…") so the orchestrator routes to it correctly.
- Optional frontmatter as needed: `model`, `permissionMode` (e.g. `plan`), `memory` (e.g. `project`),
  `maxTurns`, `color`. Keep `tools` minimal — grant only what the role needs (read-only reviewers: `Read, Grep, Glob`).
- Body opens with "You are …" defining the role, then the procedure. Keep it focused; move long reference
  material out of the prompt.
- These are the reusable definitions deployed to Claude/Cursor via `make agents-copy`. Mirror the two-phase
  Scanner→Critic / Scout→Diver split already established here when adding pipeline agents.
