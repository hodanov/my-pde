---
paths:
  - "**/*.{go,lua,sh,tf,yml,yaml,toml,py,ts,js}"
  - "**/*.dockerfile"
  - "**/Dockerfile"
  - "**/.zshrc"
---

# Code comment rules

- Do not write comments. Code, names, and structure carry the "what"; when an explanation feels necessary, rewrite the code instead of annotating it.
- The only allowed exceptions are mechanical: godoc on exported Go identifiers, linter/compiler directives with the reason they require (`//nolint:...`, `# shellcheck disable=...`, `-- selene: allow(...)`, `//go:generate`), shebangs, and license or generated-file headers.
- Rationale that does not fit those exceptions goes in the commit message, the PR description, or `docs/plan/**` — never in the source.
- This governs the lines you write or change. Do not strip existing comments as drive-by cleanup; drop one only when the code it describes is being rewritten anyway.
- Apply the same rule to code samples and diffs you put in issues, PRs, and plan documents: write the example the way the code should actually land.
