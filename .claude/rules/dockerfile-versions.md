---
paths:
  - "environment/docker/nvim.dockerfile"
  - "environment/tools/**"
---

# Dockerfile / toolchain version rules

- Keep `ARG` lines in `environment/docker/nvim.dockerfile` unindented and single-line; CI automation (`bump-tool-versions.yml`) edits them by pattern.
- Do NOT manually bump pinned tool versions. Update via the version-bump workflows or `environment/tools/go/update-go-tools.sh`. This is enforced deterministically by the `guard-version-pins.sh` PreToolUse hook (`.claude/settings.json`), which blocks edits to `ARG *_VERSION=`/`*_TOOLCHAIN=` lines and to the pin manifests `environment/tools/go/go-tools.txt` and `environment/tools/node/package.json`.
- Rebuild the image after changing tool versions or `environment/tools/node/package.json`.
