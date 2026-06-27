---
paths:
  - "scripts/ai-bridge/**"
---

# ai-bridge (Go) rules

- Detailed conventions live in `scripts/ai-bridge/AGENTS.md` (table-driven tests, unique error-variable names, package scope). Follow them.
- Bug fixes are test-first: add a failing test that reproduces the issue before changing code.
- Verify with `make ai-bridge-test`, `go vet ./...`, and `golangci-lint run`; format with `goimports`.
