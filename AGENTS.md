# Repository Guidelines

This is a personal development environment (PDE) for macOS (arm64) + Docker.
Neovim runs inside a Docker container; AI agent configs and dotfiles live on the host.

## Project Structure

- `environment/`: Docker image and toolchain pins.
- `nvim/`: Neovim configuration (`init.lua` + modular Lua).
- `scripts/ai-bridge/`: Go daemon bridging Neovim to host-side AI CLIs. See `scripts/ai-bridge/AGENTS.md`.
- `ai-agents/`: AI agent/skill definitions and settings deployed to `~/.claude`, `~/.cursor`, `~/.codex`.
  - `ai-agents/agents/`: 9 agent definitions (review, investigation, textlint).
  - `ai-agents/skills/`: 12 skills (commit, review, blog, log export, etc.).
  - `ai-agents/settings/`: Claude/Cursor settings and hooks.
  - `ai-agents/Makefile`: link/copy targets for deploying to each CLI (Claude, Cursor, Codex, Copilot).
- `dotfiles/`: Shell and terminal configs (`.zshrc`, `wezterm/`).
- `docs/plan/`: implementation plans. `docs/log/`: work logs.
- `assets/`: screenshots and static media.
- `.github/workflows/`: 9 CI workflows (lint, test, version bumps).

## Build, Test, and Development Commands

### Docker (dev container)

- `docker compose -f environment/docker/docker-compose.yml up -d` — build and start.
- `docker container exec -it nvim-dev bash --login` — enter the container.

### AI Bridge (Go)

- `make ai-bridge-build` — build the binary.
- `make ai-bridge-test` — run Go tests (`go test ./...`).
- `make ai-bridge-install` — sign and register with launchd.

### AI Agents / Skills deployment

- `make claude-link` — symlink `agents.xml` to `~/.claude/CLAUDE.md`.
- `make skills-copy` — copy skills to all CLIs.
- `make agents-copy` — copy agent definitions to Claude/Cursor.
- `make settings-copy` — copy settings and hooks to Claude/Cursor.

### Dotfiles

- `make dotfiles-link` — symlink WezTerm config to `~/.config/wezterm`.

### Tool version updates

- `./environment/tools/go/update-go-tools.sh` — refresh Go tool pins.
- Weekly CI (`bump-tool-versions.yml`) auto-bumps Node, Go, Neovim, Rust, npm.

## Coding Style

- Keep `ARG` lines in `environment/docker/nvim.dockerfile` unindented and single-line (CI automation edits them).
- Tool versions are pinned; update via workflows or scripts, not manual edits.
- For sub-directory conventions, see each directory's `AGENTS.md`.

## Testing & Linting

| File type  | Lint                                          | Format                      |
| ---------- | --------------------------------------------- | --------------------------- |
| Go         | `go vet ./...` / `golangci-lint run`          | `goimports`                 |
| Lua        | `stylua --check .`                            | `stylua .`                  |
| Markdown   | `markdownlint-cli2 *`                         | `markdownlint-cli2 --fix *` |
| TOML       | `tombi lint`                                  | `tombi format`              |
| JSON/YAML  | `prettier --check .`                          | `prettier --write .`        |
| Shell      | `shellcheck`                                  | `shfmt`                     |
| Dockerfile | `hadolint environment/docker/nvim.dockerfile` | —                           |
| Terraform  | `tflint`                                      | `terraform fmt`             |

- AI Bridge has Go unit tests: `make ai-bridge-test`.
- No repository-level test suite beyond per-directory checks.
- Terraform definition jump/completion is provided by `terraform-ls`. `tflint`'s AWS ruleset (`tflint-ruleset-aws`) is not bundled here; install it per Terraform project via `.tflint.hcl` + `tflint --init`.

## Commit & PR Guidelines

- Use short conventional prefixes: `feat:`, `fix:`, `refactor:`, `chore:`, `build(deps):`.
- Keep messages concise and action-focused (imperative mood).
- Note whether a container rebuild is required when changing tool versions.

## Platform

- macOS (arm64) + Docker. Adjust `environment/docker/nvim.dockerfile` for other platforms.
- Rebuild the image after changing tool versions or `environment/tools/node/package.json`.
