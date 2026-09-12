---
paths:
  - "mise.toml"
  - "environment/docker/nvim.dockerfile"
  - "environment/tools/**"
---

# Toolchain version rules

- `mise.toml` at the repo root is the source of truth for tool versions: host tools in `[tools]`, Docker-only versions (Neovim/npm/Rust/Terraform/lua-language-server/marksman) in `[env]`. `[env]` also holds the per-asset SHA256 of release binaries whose upstream publishes no checksum file (`LUA_LS_SHA256_*` / `MARKSMAN_SHA256_*`); the weekly bump resolves them from GitHub release asset digests (ADR-0005).
- `environment/tools/go/go-tools.txt` and the `ARG` defaults in `environment/docker/nvim.dockerfile` are generated from `mise.toml` by `mise run pins:sync` (CI verifies sync via `pins:check`). Keep `ARG` lines unindented and single-line; `sync-pins.sh` edits them by pattern.
- Do NOT manually bump pinned tool versions. Update via `mise use --pin <tool>@<version>` (`[tools]`) or `mise set <KEY>=<value>` (`[env]`) + `mise run pins:sync` (Bash), or the weekly `automation-tools-bump.yml` workflow. This is enforced deterministically by the `guard-version-pins.sh` PreToolUse hook (`.claude/settings.json`), which blocks Edit/Write on `mise.toml` `[tools]`/`[env]` pin lines, `ARG *_VERSION=`/`*_TOOLCHAIN=`/`*_SHA256_AMD64=`/`*_SHA256_ARM64=` lines, and the generated manifests `environment/tools/go/go-tools.txt` and `environment/tools/node/package.json`.
- Rebuild the image after changing tool versions or `environment/tools/node/package.json`.
