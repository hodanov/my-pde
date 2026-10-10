---
paths:
  - "ai-agents/hooks/**"
  - "ai-agents/settings/*/hooks/**"
---

# Hook authoring rules

## Where a hook belongs

Claude hooks live in two roots. Pick by **what the hook depends on**, not by which event it uses.

| Root                               | Scope                                                           | Wiring                                                   |
| ---------------------------------- | --------------------------------------------------------------- | -------------------------------------------------------- |
| `ai-agents/hooks/`                 | Runs anywhere — no dependency on this machine or this checkout  | `ai-agents/hooks/hooks.json` (plugin `ai-agents@my-pde`) |
| `ai-agents/settings/claude/hooks/` | Needs the local machine: macOS GUI, local FS layout, local mise | `ai-agents/settings/claude/settings.json` の `hooks`     |

- A hook that shells out to `osascript`, assumes the worktree layout under `$HOME`, or reports on the
  local toolchain belongs in the settings root. Everything else belongs in the plugin root.
- Keeping state in `${TMPDIR}` or inside the repository is portable. Reading `$HOME/.claude/...`
  by absolute path is not — resolve sidecar files relative to the script instead:
  `SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"`. `markdown-format.sh` reads its
  `.markdownlint-cli2.yaml` that way, so the same pair works under both distribution paths.
- Plugin hook commands use `"${CLAUDE_PLUGIN_ROOT}/hooks/<name>.sh"`. That path changes when the
  plugin updates, so never hardcode an installed location.
- A script sitting in `ai-agents/hooks/` but absent from `hooks.json` is **deliberately dormant**.
  Do not wire one back up without asking.
- codex keeps its own copy under `ai-agents/settings/codex/hooks/`, wired by `ai-agents/settings/codex/hooks.json`.
  `mise run codex-settings-copy` deploys the whole `settings/codex/` tree to `~/.codex`, so that
  `hooks.json` is the sole owner of `~/.codex/hooks.json`. The `PreToolUse(Bash)` contract currently
  matches claude's (`tool_input.command`, exit 2 blocks), but the copy is deliberate: do not point the
  codex wiring at `~/.claude/hooks/`.
- Codex runs a non-managed hook only after it is reviewed and trusted in `/hooks`, and the trust is tied to
  the hook's hash. Every edit to a deployed script or to `hooks.json` needs a new review.

## Distribution

`ai-agents/hooks/` currently ships **both** ways: through the plugin, and through
`mise run settings-copy` into `~/.claude/hooks/`. Both wirings are live, so a hook there fires twice
locally. This is a deliberate transitional state — see `docs/plan/` for which side gets dropped.

User-level `env` (such as `SKILL_OBSERVE_HOME`) can only be delivered by the settings copy, never by
the plugin. A hook that depends on one keeps that dependency on the settings path.
