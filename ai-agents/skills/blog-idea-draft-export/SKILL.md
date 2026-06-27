---
name: blog-idea-draft-export
description: Create or update a Markdown blog draft from recent work and export it with `YYYY-MM-DD_slug.md` naming. Output directory can be set via environment variable or CLI option; if missing, ask the user and pass it explicitly.
metadata:
  version: 1
---

# Blog Idea Draft Export

## Goal

Create a clean Markdown draft from recent work context and save it
with consistent naming and linted formatting.

Default export helper script:

- `skills/blog-idea-draft-export/scripts/export_blog_idea_draft.sh`

## Workflow

1. Gather source context from the current conversation first.
2. Supplement context only when needed
   (for example: changed files, PR title/body, key commands, or outcomes).
3. Choose a short kebab-case slug.
4. Draft the Markdown content in the assistant response first.
5. Export using the helper script (content goes through stdin):

```bash
cat <<'EOF' | \
  skills/blog-idea-draft-export/scripts/export_blog_idea_draft.sh \
  --slug "<slug>" --output-dir "<dir>"
# Title
...
EOF
```

1. Output directory resolution order:
   1. `--output-dir <dir>`
   2. `BLOG_IDEA_DRAFT_EXPORT_DIR` environment variable
   3. If neither is set, ask the user which directory to use,
      then pass `--output-dir` explicitly.
   4. The script can show an interactive prompt only when run from a TTY.
2. The script uses local date (`date +%F`) to build `YYYY-MM-DD_slug.md`
   unless `--date` is specified.
3. If the target file already exists, ask before overwriting (or use `--force`).
4. Return the output file path and a short summary of the draft structure.

## Draft Structure

Use this as the default skeleton, then adapt to the user request.

```markdown
# <Article title>

## TL;DR

<1-3 lines>

## Background

<Why this work was needed>

## What I Changed

- <Change 1>
- <Change 2>
- <Change 3>

## Key Implementation Notes

- <Design choice or tradeoff>
- <Operational detail>

## Pitfalls and Fixes

- <Issue>
- <How it was resolved>

## Result

<What improved, what became easier>

## Next

<Optional follow-up>
```

## Writing Rules

- Match the language and tone requested by the user.
- Keep technical details concrete (file paths, commands, settings).
- Prefer practical lessons over long background theory.
- Avoid adding claims that are not present in the source context.

## Script Notes

- Required: `--slug <kebab-case>`
- Optional: `--output-dir <dir>`, `--date <YYYY-MM-DD>`, `--force`
- Input: Markdown via stdin
- Output: prints final file path
