# Repository Guidelines

This is a personal development environment (PDE) for macOS (arm64) + Docker.
Neovim runs inside a Docker container; AI agent configs and dotfiles live on the host.

## Project Structure

- `environment/`: Docker image and toolchain pins.
- `nvim/`: Neovim configuration (`init.lua` + modular Lua).
- `scripts/`: Go apps, one module per directory (`agent-stats`, `ai-bridge`, `config-diff`, `go-verify`, `nvim-sync`, `pipeline-metrics`, `scaffold`). Each has its own `README.md`.
  - `scripts/ai-bridge/`: Go daemon bridging Neovim to host-side AI CLIs. See `scripts/ai-bridge/AGENTS.md`.
- `ai-agents/`: AI agent/skill definitions and settings deployed to `~/.claude`, `~/.cursor`, `~/.codex`.
  - `ai-agents/agents/`: subagent definitions (review, investigation).
  - `ai-agents/skills/`: shared skills that run in any repository (dev workflow, review, plan export, etc.).
  - `ai-agents/personal/skills/`: hobby / private-life skills, distributed as the separate `personal` plugin.
  - `ai-agents/settings/`: Claude/Cursor settings, hooks, and shared rules.
  - Skills that rewrite this repository's own paths live in `.claude/skills/` instead and are not distributed. Placement rules: `.claude/rules/skill-authoring.md`.
  - Deployment to each CLI (Claude, Cursor, Codex, Copilot) is done via mise tasks (`mise.toml` at the repo root).
- `dotfiles/`: Shell and terminal configs (`.zshrc`, `wezterm/`). Both are deployed as symlinks (`mise run zshrc-link` / `dotfiles-link`), so host edits show up as repo diffs.
- `docs/plan/`: implementation plans. `docs/log/`: work logs.
- `assets/`: screenshots and static media.
- `.github/workflows/`: CI workflows (lint, test, version bumps).

## Build, Test, and Development Commands

### Docker (dev container)

- `docker compose -f environment/docker/docker-compose.yml up -d` — build and start.
- `docker container exec -it nvim-dev bash --login` — enter the container.
- Both `mise run docker:build` and the compose service use the image tag `my-pde-nvim:dev`, so an image built by the former is what `up -d` starts.

### Task runner (mise)

Tasks and host tool versions are managed by [mise](https://mise.jdx.dev) via `mise.toml` at the repo root. Run `mise tasks ls` for the full list.

### Go apps under `scripts/` (agent-stats, ai-bridge, config-diff, go-verify, nvim-sync, pipeline-metrics, scaffold)

- `mise run <app>:build` — build the binary (e.g. `mise run ai-bridge:build`).
- `mise run <app>:test` — run Go tests; `mise run go:test` runs all apps.
- `mise run <app>:lint` — golangci-lint + goimports check; `mise run go:lint` runs all apps.
- `mise run ai-bridge:install` — sign and register with launchd (local only).
- `mise run ai-bridge:generate` — regenerate mocks.

### AI Agents / Skills deployment

- `mise run claude-link` — symlink `agents.xml` to `~/.claude/CLAUDE.md`.
- `mise run skills-copy` — copy skills to all CLIs.
- `mise run agents-copy` — copy agent definitions to Claude/Cursor.
- `mise run settings-copy` — copy settings and hooks to Claude/Cursor.
- Claude Code: the `deploy-ai-config` skill wraps this flow (which task for which edit, plus verification).

### Dotfiles

- `mise run dotfiles-link` — symlink WezTerm config to `~/.config/wezterm`.
- `mise run zshrc-link` — symlink `dotfiles/.zshrc` to `~/.zshrc` (backs up an existing regular file; `zshrc-unlink` reverts). Kept separate from `dotfiles-link` because it replaces the login shell config.

### Tool version updates

- `mise.toml` is the single source of truth for tool versions. Weekly CI (`bump-versions.yml`) bumps the pins and regenerates the derived artifacts.
- `mise run pins:sync` — regenerate `environment/tools/go/go-tools.txt` and the Dockerfile ARG defaults from `mise.toml` (CI verifies sync via `pins:check`).

## Parallel Work (git worktrees)

- Worktrees live outside the checkout: `~/workspace/.worktrees/<repo>/<name>/` (override the base with `WORKTREE_BASE`). Never create worktrees inside the repository.
- Claude Code: `claude --worktree [<name>]` and subagent `isolation: worktree` follow this layout automatically via the `WorktreeCreate` hook (`ai-agents/settings/claude/hooks/worktree-create.sh`). `<name>` is disposable — omit it to auto-generate one.
- The hook creates a placeholder branch named after `<name>`. Once the task is understood, rename it to a plain descriptive slug (no `wt/` or `agent/` prefix): `git branch -m <slug>`. Renaming works while checked out; the `commit-and-draft-pr` skill enforces this before push.
- Other CLIs / manual use: `git worktree add ~/workspace/.worktrees/<repo>/<name> -b <slug> origin/main`.
- Remove with `git worktree remove <path>`; git refuses dirty worktrees unless `--force` is given.
- mise: worktree checkouts contain their own `mise.toml`, which mise refuses until trusted. One-time setup per machine: `mise settings add trusted_config_paths "~/workspace/.worktrees"` (trusts every config under the worktree base — same trust you would grant per repo with `mise trust`).

## Coding Style

- Per-language conventions that a linter cannot check load on demand from path-scoped rules in `.claude/rules/` (and personal rules in `~/.claude/rules/`) when you touch matching files. Conventions a formatter/linter _can_ enforce are left to hooks rather than duplicated as rules.
- Comments: don't write them. Code carries the "what"; the "why" goes in commit messages, PRs, and `docs/plan/**`. Mechanical exceptions only (godoc on exported Go identifiers, linter directives, shebangs) — see `.claude/rules/code-comments.md`. This applies to code samples and diffs in issues and PRs too, not just to files you edit.
- Dockerfile/toolchain version constraints live in `.claude/rules/dockerfile-versions.md`; tool versions are pinned and updated via workflows or scripts, not manual edits. For Claude this is enforced by the `guard-version-pins.sh` PreToolUse hook.
- For sub-directory conventions, see each directory's `AGENTS.md`. (`scripts/ai-bridge/` also has a `CLAUDE.md` importing its `AGENTS.md` so the Go conventions auto-load for Claude.)

## Testing & Linting

- `mise run lint` runs every repo-wide linter (CI parity); `mise run verify:changed` maps changed files to the matching tests/linters. Run them from the repository root — markdownlint and friends resolve their config relative to cwd.
- In Claude this is automatic: `PostToolUse` hooks format on write, and the `lint-changed.sh` Stop hook reports what still fails, naming the tool.
- AI Bridge has Go unit tests: `mise run ai-bridge:test`.
- No repository-level test suite beyond per-directory checks.
- In Claude, the repo-local Stop hook `.claude/hooks/test-changed.sh` auto-runs `go test` for changed `scripts/<app>` apps (report-only, non-blocking; opt out with `TEST_CHANGED_DISABLE=1`).

## Commit & PR Guidelines

- Use short conventional prefixes: `feat:`, `fix:`, `refactor:`, `chore:`, `build(deps):`.
- Keep messages concise and action-focused (imperative mood).
- Note whether a container rebuild is required when changing tool versions.

## Platform

- macOS (arm64) + Docker. Adjust `environment/docker/nvim.dockerfile` for other platforms.
- Rebuild the image after changing tool versions or `environment/tools/node/package.json`.
