#!/usr/bin/env bash
# PreToolUse(Bash) hook: detect commands that would trigger a permission prompt
# and append them to a per-session buffer for permission-prompt-tuner to analyze.
# Pure observation — it never blocks or overrides the permission decision.
set -u

command -v python3 >/dev/null 2>&1 || exit 0

INPUT=$(cat)

PERM_HOOK_INPUT="$INPUT" python3 <<'PY' 2>/dev/null || true
import json
import os
import re
import time
from pathlib import Path


def load_input():
    raw = os.environ.get("PERM_HOOK_INPUT", "")
    try:
        return json.loads(raw) if raw else {}
    except Exception:
        return {}


def read_json(path):
    try:
        with open(path) as f:
            return json.load(f)
    except Exception:
        return {}


def bash_patterns(rules):
    out = []
    for r in rules or []:
        if isinstance(r, str) and r.startswith("Bash(") and r.endswith(")"):
            out.append(r[len("Bash(") : -1])
    return out


def pattern_matches(pattern, text):
    # Approximate Claude Code's Bash matching: '*' is a wildcard, the match is
    # anchored at the start. Not a full parser (quoting / $() are ignored).
    if "*" not in pattern:
        return text == pattern
    # A trailing " *" (space + wildcard) also covers the argument-less form:
    # e.g. "echo *" matches bare "echo". Without this, "echo" (no args) is
    # wrongly flagged missing-allow even though "Bash(echo *)" is allowed.
    if pattern.endswith(" *") and text == pattern[:-2]:
        return True
    parts = pattern.split("*")
    if not text.startswith(parts[0]):
        return False
    idx = len(parts[0])
    for part in parts[1:]:
        if part == "":
            continue
        found = text.find(part, idx)
        if found == -1:
            return False
        idx = found + len(part)
    return True


HEREDOC_MARKER = re.compile(r"<<-?\s*[\"']?([A-Za-z_][A-Za-z0-9_]*)[\"']?")

# Shell keywords / builtins (and our mask placeholders) that need no allow rule.
IGNORE_WORDS = {
    "[", "]", "[[", "]]", "test", ":", "true", "false",
    "then", "else", "elif", "fi", "do", "done", "esac", "Q", "S",
    "if", "for", "while", "until", "case", "select", "function", "time",
    "set", "local", "export", "declare", "readonly", "unset", "shift",
    "read", "return", "continue", "break",
}


def preprocess(cmd):
    # Join line continuations so "\<newline>" does not split a command.
    cmd = re.sub(r"\\\n", " ", cmd)
    # Drop heredoc bodies: keep the marker line, discard body + closing delimiter.
    # Done before masking so the quoted delimiter (<<'EOF') is still readable.
    lines = cmd.split("\n")
    kept, i = [], 0
    while i < len(lines):
        kept.append(lines[i])
        m = HEREDOC_MARKER.search(lines[i])
        if m:
            delim = m.group(1)
            i += 1
            while i < len(lines) and lines[i].strip() != delim:
                i += 1
        i += 1
    text = "\n".join(kept)
    # Mask quoted strings and command substitutions so operators / newlines
    # inside them neither split segments nor look like commands.
    text = re.sub(r'"(?:\\.|[^"\\])*"', " Q ", text, flags=re.DOTALL)
    text = re.sub(r"'[^']*'", " Q ", text)
    text = re.sub(r"\$\([^()]*\)", " S ", text)
    text = re.sub(r"`[^`]*`", " S ", text)
    return text


def split_segments(cmd):
    tokens = re.split(r"\s*(?:\|\||&&|;|\|)\s*|\n+", preprocess(cmd))
    return [t.strip() for t in tokens if t.strip()]


def strip_leading_env(segment):
    toks = segment.split()
    i = 0
    while i < len(toks) and re.match(r"^[A-Za-z_][A-Za-z0-9_]*=", toks[i]):
        i += 1
    return " ".join(toks[i:])


def find_project_root(cwd):
    try:
        base = Path(cwd)
    except Exception:
        return None
    for cand in [base, *base.parents]:
        if (cand / ".git").exists():
            return str(cand)
    return None


data = load_input()
command = (data.get("tool_input", {}) or {}).get("command", "") or ""
session_id = data.get("session_id", "") or "default"
cwd = data.get("cwd", "") or os.getcwd()

if not command.strip():
    raise SystemExit(0)

home = os.path.expanduser("~")
settings_paths = [
    os.path.join(home, ".claude", "settings.json"),
    os.path.join(home, ".claude", "settings.local.json"),
]
project_root = find_project_root(cwd)
if project_root:
    settings_paths += [
        os.path.join(project_root, ".claude", "settings.json"),
        os.path.join(project_root, ".claude", "settings.local.json"),
    ]

allow, deny = [], []
for path in settings_paths:
    perms = read_json(path).get("permissions", {}) or {}
    allow += perms.get("allow", []) or []
    deny += perms.get("deny", []) or []
allow_pats = bash_patterns(allow)
deny_pats = bash_patterns(deny)

segments = split_segments(command)
causes = []
for seg in segments:
    text = strip_leading_env(seg)
    toks = text.split()
    if not toks:
        continue
    word = toks[0]
    # Skip shell builtins/keywords and non-command junk (flags, stray punctuation).
    if word in IGNORE_WORDS or not re.match(r"^[A-Za-z0-9_./~]", word):
        continue
    if any(pattern_matches(p, text) for p in deny_pats):
        causes.append({"segment": text, "kind": "deny-hit"})
        continue
    if any(pattern_matches(p, text) for p in allow_pats):
        continue
    if re.match(r"^terraform\s+-chdir=", text):
        causes.append({"segment": text, "kind": "prefix-break"})
    else:
        causes.append({"segment": text, "kind": "missing-allow"})

has_cd = any(
    strip_leading_env(seg) == "cd" or strip_leading_env(seg).startswith("cd ")
    for seg in segments
)
has_file_redirect = bool(re.search(r">>?\s*[^&\s]", preprocess(command)))
if has_cd and has_file_redirect:
    causes.append({"segment": command.strip(), "kind": "builtin-cd-redirect"})

if not causes:
    raise SystemExit(0)

buffer_dir = os.path.join(
    os.environ.get("TMPDIR", "/tmp"), "claude-permission-prompt-tuner"
)
os.makedirs(buffer_dir, exist_ok=True)
buffer_path = os.path.join(buffer_dir, f"{session_id}.jsonl")

existing = set()
if os.path.exists(buffer_path):
    with open(buffer_path) as f:
        for line in f:
            try:
                existing.add(json.loads(line).get("command"))
            except Exception:
                continue
if command in existing:
    raise SystemExit(0)

record = {
    "ts": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
    "command": command,
    "cwd": cwd,
    "causes": causes,
}
with open(buffer_path, "a") as f:
    f.write(json.dumps(record, ensure_ascii=False) + "\n")
PY

exit 0
