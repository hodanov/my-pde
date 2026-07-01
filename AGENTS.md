# Repository Guidelines

This is a personal development environment (PDE) for macOS (arm64) + Docker.
Neovim runs inside a Docker container; AI agent configs and dotfiles live on the host.

## Project Structure

- `environment/`: Docker image and toolchain pins.
- `nvim/`: Neovim configuration (`init.lua` + modular Lua).
- `scripts/ai-bridge/`: Go daemon bridging Neovim to host-side AI CLIs. See `scripts/ai-bridge/AGENTS.md`.
- `ai-agents/`: AI agent/skill definitions and settings deployed to `~/.claude`, `~/.cursor`, `~/.codex`.
  - `ai-agents/agents/`: subagent definitions (review, investigation).
  - `ai-agents/skills/`: reusable skills (commit, review, blog, log export, etc.).
  - `ai-agents/settings/`: Claude/Cursor settings, hooks, and shared rules.
  - `ai-agents/Makefile`: link/copy targets for deploying to each CLI (Claude, Cursor, Codex, Copilot).
- `dotfiles/`: Shell and terminal configs (`.zshrc`, `wezterm/`).
- `docs/plan/`: implementation plans. `docs/log/`: work logs.
- `assets/`: screenshots and static media.
- `.github/workflows/`: CI workflows (lint, test, version bumps).

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
- Claude Code: the `deploy-ai-config` skill wraps this flow (which target for which edit, plus verification).

### Dotfiles

- `make dotfiles-link` — symlink WezTerm config to `~/.config/wezterm`.

### Tool version updates

- `./environment/tools/go/update-go-tools.sh` — refresh Go tool pins.
- Weekly CI (`bump-tool-versions.yml`) auto-bumps Node, Go, Neovim, Rust, npm.

## Coding Style

- Per-language lint/format conventions load on demand from path-scoped rules in `.claude/rules/` (and personal rules in `~/.claude/rules/`) when you touch matching files.
- Dockerfile/toolchain version constraints live in `.claude/rules/dockerfile-versions.md`; tool versions are pinned and updated via workflows or scripts, not manual edits. For Claude this is enforced by the `guard-version-pins.sh` PreToolUse hook.
- For sub-directory conventions, see each directory's `AGENTS.md`. (`scripts/ai-bridge/` also has a `CLAUDE.md` importing its `AGENTS.md` so the Go conventions auto-load for Claude.)

## Testing & Linting

- Per-language lint/format commands load on demand as path-scoped rules (`.claude/rules/`, `~/.claude/rules/`): Go, Lua, Markdown, TOML, JSON/YAML, Shell, Terraform.
- Not covered by rules: Dockerfile lint (`hadolint environment/docker/nvim.dockerfile`).
- AI Bridge has Go unit tests: `make ai-bridge-test`.
- No repository-level test suite beyond per-directory checks.

## Commit & PR Guidelines

- Use short conventional prefixes: `feat:`, `fix:`, `refactor:`, `chore:`, `build(deps):`.
- Keep messages concise and action-focused (imperative mood).
- Note whether a container rebuild is required when changing tool versions.

## Platform

- macOS (arm64) + Docker. Adjust `environment/docker/nvim.dockerfile` for other platforms.
- Rebuild the image after changing tool versions or `environment/tools/node/package.json`.
