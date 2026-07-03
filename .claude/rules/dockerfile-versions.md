---
paths:
  - "mise.toml"
  - "environment/docker/nvim.dockerfile"
  - "environment/tools/**"
---

# Toolchain version rules

- `mise.toml` at the repo root is the source of truth for tool versions: host tools in `[tools]`, Docker-only versions (Neovim/npm/Rust/Terraform) in `[env]`.
- The `ARG` defaults in `environment/docker/nvim.dockerfile` are frozen fallbacks; `mise run docker:build` overrides them via `--build-arg`. Keep `ARG` lines unindented and single-line.
- Do NOT manually bump pinned tool versions. Update via `mise use --pin <tool>@<version>` (Bash), the version-bump workflows, or `environment/tools/go/update-go-tools.sh`. This is enforced deterministically by the `guard-version-pins.sh` PreToolUse hook (`.claude/settings.json`), which blocks Edit/Write on `mise.toml` `[tools]`/`[env]` pin lines, `ARG *_VERSION=`/`*_TOOLCHAIN=` lines, and the pin manifests `environment/tools/go/go-tools.txt` and `environment/tools/node/package.json`.
- Rebuild the image after changing tool versions or `environment/tools/node/package.json`.
