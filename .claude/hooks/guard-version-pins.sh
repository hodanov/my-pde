#!/usr/bin/env bash
# PreToolUse(Edit|Write|MultiEdit) guard.
# ピン留めされたツールバージョンの手動編集をブロックする。
# 更新は update-go-tools.sh / bump-tool-versions.yml 経由で行う想定。
set -eu

INPUT=$(cat)

HOOK_INPUT="$INPUT" python3 <<'PY'
import os, json, re

try:
    data = json.loads(os.environ.get("HOOK_INPUT", "") or "{}")
except json.JSONDecodeError:
    raise SystemExit(0)

ti = data.get("tool_input", {}) or {}
file_path = (ti.get("file_path") or "").replace("\\", "/")
if not file_path:
    raise SystemExit(0)

DOCKERFILE_SUFFIX = "environment/docker/nvim.dockerfile"
MISE_TOML_SUFFIX = "mise.toml"
PIN_MANIFESTS = (
    "environment/tools/go/go-tools.txt",
    "environment/tools/node/package.json",
)

ARG_PIN = re.compile(r"^\s*ARG\s+[A-Z0-9_]+(?:VERSION|TOOLCHAIN)\s*=", re.MULTILINE)
# mise.toml: [tools] pins (bare or quoted backend keys) and [env] version values.
# [tasks.*] entries (run/description/dir/depends) are free to edit. Requires
# spaces around "=" (tombi style) so shell assignments in task bodies don't match.
MISE_PIN = re.compile(
    r"^(?:\"[^\"]+\"|go|node|shfmt|shellcheck|stylua|hadolint|golangci-lint"
    r"|terraform-ls|tflint|[A-Z0-9_]+(?:VERSION|TOOLCHAIN)) = \"",
    re.MULTILINE,
)


def edited_text(ti):
    parts = []
    for key in ("old_string", "new_string", "content"):
        val = ti.get(key)
        if isinstance(val, str):
            parts.append(val)
    for edit in ti.get("edits", []) or []:
        for key in ("old_string", "new_string"):
            val = edit.get(key)
            if isinstance(val, str):
                parts.append(val)
    return "\n".join(parts)


blocked = ""
if file_path.endswith(DOCKERFILE_SUFFIX):
    if ARG_PIN.search(edited_text(ti)):
        blocked = "nvim.dockerfile の ARG ピン留めバージョン行"
elif file_path.endswith(MISE_TOML_SUFFIX):
    if MISE_PIN.search(edited_text(ti)):
        blocked = "mise.toml の [tools]/[env] ピン留めバージョン行"
elif any(file_path.endswith(m) for m in PIN_MANIFESTS):
    blocked = "%s（ツールバージョンのピン留めマニフェスト）" % os.path.basename(file_path)

if blocked:
    import sys

    sys.stderr.write(
        "[guard-version-pins] ブロック: %s を手動編集しようとしています。\n"
        "ピン留めされたツールバージョンは手動で変更しないでください。更新は次のいずれか経由で行ってください:\n"
        "  - mise use --pin <tool>@<version> （mise.toml の [tools]、Bash 経由）\n"
        "  - environment/tools/go/update-go-tools.sh （Go ツール）\n"
        "  - .github/workflows/bump-tool-versions.yml （Node/Go/Neovim/Rust/npm の週次自動更新）\n"
        % blocked
    )
    raise SystemExit(2)

raise SystemExit(0)
PY
