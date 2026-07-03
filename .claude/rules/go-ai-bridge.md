---
paths:
  - "scripts/ai-bridge/**"
---

# ai-bridge (Go) rules

Detailed conventions (table-driven tests, unique error-variable names, package scope, DDD layering,
mock generation) load automatically from `scripts/ai-bridge/CLAUDE.md` → `@AGENTS.md` when you touch
these files. Claude-specific workflow additions:

- Bug fixes are test-first: add a failing test that reproduces the issue before changing code.
- Verify with `mise run ai-bridge:test`, `go vet ./...`, and `golangci-lint run`; format with `goimports`.
