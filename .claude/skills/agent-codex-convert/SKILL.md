---
name: agent-codex-convert
description: Convert Claude/Cursor markdown subagent files to Codex CLI TOML format. Parses YAML frontmatter and body, applies model/permission/tool mappings, and outputs to agents/codex/<name>.toml.
disable-model-invocation: true
metadata:
  version: 1
---

# Agent Codex Convert

## Goal

Convert one or more Claude/Cursor agent markdown files (in `agents/`)
to Codex CLI TOML format (in `agents/codex/`) using the helper script.

Default conversion helper script:

- `skills/agent-codex-convert/scripts/convert_agent_to_codex.sh`

## Workflow

1. Identify which agent markdown files need conversion.
2. Run the helper script for each file (or pass multiple).
3. Review the generated TOML file(s) for correctness.
4. If the source had `memory:` field, review the body for remaining
   memory-related instructions that may need manual removal.
5. After conversion, optionally update the Makefile `codex-agents-copy`
   case statement if a new agent name was added.

## Usage

Single file:

```bash
skills/agent-codex-convert/scripts/convert_agent_to_codex.sh agents/my-agent.md
```

Multiple files:

```bash
skills/agent-codex-convert/scripts/convert_agent_to_codex.sh agents/*.md
```

With options:

```bash
skills/agent-codex-convert/scripts/convert_agent_to_codex.sh \
  --reasoning-effort high \
  --output-dir agents/codex \
  --force \
  agents/investigation-diver.md
```

Preview without writing:

```bash
skills/agent-codex-convert/scripts/convert_agent_to_codex.sh \
  --dry-run agents/my-agent.md
```

## Script Options

- Positional: one or more `.md` file paths (required)
- `--reasoning-effort <low|medium|high>`: Override default `medium`
- `--output-dir <dir>`: Override default `agents/codex`
- `--force`: Overwrite existing TOML without prompting
- `--dry-run`: Print to stdout instead of writing files
- `-h, --help`: Show usage

## Conversion Mapping

| Source (frontmatter)             | Target (TOML)                       | Notes                          |
| -------------------------------- | ----------------------------------- | ------------------------------ |
| model: sonnet/opus/haiku/inherit | `model = "gpt-5.3-codex"`           | All map to same model          |
| (default)                        | `model_reasoning_effort = "medium"` | Overridable via CLI            |
| permissionMode: plan             | `sandbox_mode = "read-only"`        | Other modes omitted            |
| tools                            | Constraints section in body         | Injected before first heading  |
| memory                           | Comment + WARNING                   | Codex unsupported; review body |
| description                      | Header comment                      | Truncated to first sentence    |
| maxTurns, background             | Comment only                        | Codex unsupported              |

## Post-Conversion Checklist

- [ ] Verify `developer_instructions` reads naturally
- [ ] If memory was present, confirm memory-related paragraphs/sections removed
- [ ] Check Constraints section matches the original tool restrictions
- [ ] Verify no TOML-breaking characters in the body
- [ ] If new agent, add case entry to Makefile `codex-agents-copy` target
