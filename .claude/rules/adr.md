---
paths:
  - "docs/adr/**"
---

# ADR rules

- Filename: `NNNN-slug.md` (zero-padded sequential number, kebab-case slug). Numbers are never reused.
- Title line: `# ADR-NNNN: <English title>`. Second line: `- Status: <status>`.
- Status is one of `Accepted (YYYY-MM-DD)`, `Superseded by ADR-NNNN (YYYY-MM-DD)`, `Deprecated (YYYY-MM-DD)`.
- Section skeleton: `## Context` → `## Options considered` → `## Decision` → `## Consequences`.
  Headings in English, body in Japanese, matching `docs/plan/**`.
- `## Options considered` is a table of the rejected alternatives and why they lost. A decision
  without its alternatives cannot be re-derived later, so never omit this section.
- Write an ADR when a choice had real alternatives, departs from an existing convention, or accepts
  a constraint. Skip obvious choices, implementation steps, and one-off work logs.
- ADRs record why and stay current until superseded; `docs/plan/**` records how and becomes history
  once implemented. Do not duplicate one into the other — link instead.
- To reverse a decision, add a new ADR and set the old one's Status to `Superseded by ADR-NNNN`.
  Never rewrite the Decision of an accepted ADR.
