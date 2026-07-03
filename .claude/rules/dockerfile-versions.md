---
paths:
  - "mise.toml"
  - "environment/docker/nvim.dockerfile"
  - "environment/tools/**"
---

# Toolchain version rules

- `mise.toml` at the repo root is the source of truth for tool versions: host tools in `[tools]`, Docker-only versions (Neovim/npm/Rust/Terraform) in `[env]`.
- `environment/tools/go/go-tools.txt` and the `ARG` defaults in `environment/docker/nvim.dockerfile` are generated from `mise.toml` by `mise run pins:sync` (CI verifies sync via `pins:check`). Keep `ARG` lines unindented and single-line; `sync-pins.sh` edits them by pattern.
- Do NOT manually bump pinned tool versions. Update via `mise use --pin <tool>@<version>` + `mise run pins:sync` (Bash), or the weekly `bump-versions.yml` workflow. This is enforced deterministically by the `guard-version-pins.sh` PreToolUse hook (`.claude/settings.json`), which blocks Edit/Write on `mise.toml` `[tools]`/`[env]` pin lines, `ARG *_VERSION=`/`*_TOOLCHAIN=` lines, and the generated manifests `environment/tools/go/go-tools.txt` and `environment/tools/node/package.json`.
- Rebuild the image after changing tool versions or `environment/tools/node/package.json`.
