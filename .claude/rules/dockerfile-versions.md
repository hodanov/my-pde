---
paths:
  - "mise.toml"
  - "environment/docker/nvim.dockerfile"
  - "environment/tools/**"
---

# Toolchain version rules

- `mise.toml` at the repo root is the source of truth for tool versions: host tools in `[tools]`, Docker-only versions (Neovim/npm/Rust/Terraform) in `[env]`.
- `environment/tools/go/go-tools.txt` and the `ARG` defaults in `environment/docker/nvim.dockerfile` are generated from `mise.toml` by `mise run pins:sync` (CI verifies sync via `pins:check`). Keep `ARG` lines unindented and single-line; `sync-pins.sh` edits them by pattern.
- Do NOT manually bump pinned tool versions. Update via `mise use --pin <tool>@<version>` + `mise run pins:sync` (Bash), or the weekly `automation-tools-bump.yml` workflow. This is enforced deterministically by the `guard-version-pins.sh` PreToolUse hook (`.claude/settings.json`), which blocks Edit/Write on `mise.toml` `[tools]`/`[env]` pin lines, `ARG *_VERSION=`/`*_TOOLCHAIN=` lines, and the generated manifests `environment/tools/go/go-tools.txt` and `environment/tools/node/package.json`.
- The `go` directive in every `scripts/*/go.mod` tracks the `[tools] go` pin. Nothing bumps it automatically (`automation-tools-bump.yml` moves the mise pin only), so raise it deliberately with `go mod edit -go=<version>` across all modules at once. `scripts/scaffold` copies the directive from the `--from` module's live `go.mod`, so a generated module inherits whatever the existing ones carry.
- Rebuild the image after changing tool versions or `environment/tools/node/package.json`.
