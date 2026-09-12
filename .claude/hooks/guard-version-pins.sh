#!/usr/bin/env bash
# PreToolUse(Edit|Write|MultiEdit) guard.
# ピン留めされたツールバージョン（と per-asset SHA256）の手動編集をブロックする。
# 更新は mise use --pin / mise set / automation-tools-bump.yml 経由で行う想定。
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

PIN_KEY = r"[A-Z0-9_]+(?:VERSION|TOOLCHAIN|SHA256_(?:AMD64|ARM64))"
ARG_PIN = re.compile(r"^\s*ARG\s+" + PIN_KEY + r"\s*=", re.MULTILINE)
# mise.toml: [tools] pins (bare or quoted backend keys) and [env] version/checksum
# values. [tasks.*] entries (run/description/dir/depends) are free to edit. Requires
# spaces around "=" (tombi style) so shell assignments in task bodies don't match.
MISE_PIN = re.compile(
    r"^(?:\"[^\"]+\"|go|node|shfmt|shellcheck|stylua|hadolint|golangci-lint"
    r"|terraform-ls|tflint|" + PIN_KEY + r") = \"",
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
        "  - mise use --pin <tool>@<version>（[tools]）または mise set <KEY>=<value>（[env]）+ mise run pins:sync （Bash 経由）\n"
        "  - .github/workflows/automation-tools-bump.yml （週次自動更新）\n"
        % blocked
    )
    raise SystemExit(2)

raise SystemExit(0)
PY
