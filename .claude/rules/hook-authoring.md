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
- A script sitting in `ai-agents/hooks/` but absent from `hooks.json` is **deliberately dormant**
  (`permission-prompt-detect.sh` / `permission-prompt-nudge.sh`). Do not wire one back up without asking.
- cursor and copilot keep their own copies under `ai-agents/settings/{cursor,copilot}/hooks/`. They differ
  from the claude version only in how the file path is pulled out of the event JSON (`get_file_path.py`).

## Distribution

`ai-agents/hooks/` currently ships **both** ways: through the plugin, and through
`mise run settings-copy` into `~/.claude/hooks/`. Both wirings are live, so a hook there fires twice
locally. This is a deliberate transitional state — see `docs/plan/` for which side gets dropped.

User-level `env` (such as `SKILL_OBSERVE_HOME`) can only be delivered by the settings copy, never by
the plugin. A hook that depends on one keeps that dependency on the settings path.
