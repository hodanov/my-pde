# my-pde

My Personal Development Environment (PDE) built around Neovim on Docker, with AI agent integration.

![screenshot_nvim_window](assets/screenshot_nvim_window.avif)

## Overview

This repository manages the entire PDE in a single monorepo:

| Directory            | Description                                                                     |
| -------------------- | ------------------------------------------------------------------------------- |
| `nvim/`              | Neovim configuration (init.lua, plugins, LSP)                                   |
| `environment/`       | Docker environment (Dockerfile, docker-compose, tool versions)                  |
| `scripts/ai-bridge/` | AI Bridge daemon (Go) — connects Neovim to host-side AI CLIs                    |
| `ai-agents/`         | AI agent definitions, skills, and settings for Claude Code / Codex CLI / Cursor |
| `docs/`              | Plans and work logs                                                             |

## Getting Started

Clone the repo and start the container:

```sh
git clone git@github.com:hodanov/my-pde.git
docker network create my-nvim
cd my-pde
docker compose -f environment/docker/docker-compose.yml up -d
```

Attach to the container:

```sh
docker container exec -it nvim-dev bash --login
```

The `--login` option is required to read the `.bash_profile` file.

The Neovim dotfiles work in any environment. You can find [the dotfiles here](./nvim/config).

### Host toolchain (mise)

Host-side tool versions (Go, Node.js, linters) are pinned in `mise.toml` at the repo root and managed by [mise](https://mise.jdx.dev):

```sh
brew install mise
echo 'eval "$(mise activate zsh)"' >> ~/.zshrc
mise install    # run at the repo root
```

## Features

- **LSP / Linter / Formatter**: Go (gopls, golangci-lint), Python (pyright, ruff), JavaScript (eslint, prettier), Lua (stylua), Terraform (terraform-ls, tflint, terraform fmt), YAML, TOML, Markdown, etc.
- **Completion**: nvim-cmp
- **Fuzzy Finder**: telescope.nvim
- **File Manager**: fern.vim
- **Debugger**: nvim-dap
- **Plugin Manager**: lazy.nvim

Tool versions are pinned in `mise.toml` at the repo root; the `ARG` defaults in `environment/docker/nvim.dockerfile` are generated from it (`mise run pins:sync`). Versions are automatically updated via GitHub Actions and Dependabot.

## AI Bridge

Neovim (Docker container) from selected code to host-side AI CLI (Claude Code, Cursor, etc.) with context. See [docs/ai-bridge.md](docs/ai-bridge.md) for setup and usage.

## AI Agents

Agent definitions, skills, and settings for Claude Code, Codex CLI, and Cursor are managed under `ai-agents/`. Deployment is done via mise tasks (run at the repo root):

```sh
mise run claude-link      # Symlink agents.xml -> ~/.claude/CLAUDE.md
mise run skills-copy      # Copy skills to ~/.claude/, ~/.codex/, ~/.cursor/
mise run agents-copy      # Copy agent definitions
mise run settings-copy    # Copy settings and hooks
```

See `ai-agents/` for the full list of available agents and skills.

## Language-specific notes

### Python

Python environments are managed by uv. A base virtualenv is prebuilt at `/opt/python/.venv`.

```sh
# Use the prebuilt base venv (global tools)
source /opt/python/.venv/bin/activate

# Or create a project venv via uv
uv venv .venv
source .venv/bin/activate

# Sync dependencies from pyproject.toml (and uv.lock if present)
uv sync

# Deactivate when done
deactivate
```

### Node.js

CLI tools (eslint, typescript-language-server, textlint, etc.) are managed via `environment/tools/node/package.json` and installed into `/opt/npm-tools` during the image build. To add/update tools, edit the `package.json` and rebuild.

### Go

Go-based tools (gopls, dlv, golangci-lint, etc.) are installed during the image build from `environment/tools/go/go-tools.txt`, which is generated from the pins in `mise.toml` (`mise run pins:sync`). Versions are automatically updated weekly by GitHub Actions.
