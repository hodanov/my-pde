# ADR-0007: Remove the tool-selection rule and rely on Claude Code defaults

- Status: Accepted (2026-10-06)

## Context

`ai-agents/settings/claude/rules/tool-selection.md` は、探索・検索・読み取りで Bash より Glob / Grep / Read を優先し、`cat` / `find` / `grep` / `rg` / `ls` / `head` / `tail` / `sed -n` を Bash で使わないよう指示する常時適用ルールだった（`7614a0f`、2026-09-06）。

Claude Code は macOS / Linux / WSL では Glob / Grep 専用ツールをデフォルトのツールセットに含めない。代わりに Bash の `find` / `grep` が組み込みの bfs / ugrep に差し替わり、読み取り専用として許可プロンプトなしで通る（公式ドキュメント tools-reference の「Glob tool behavior」、Claude Code 2.1.291 で確認）。Glob / Grep が戻るのは起動時の `--tools` / `--allowedTools` 指定などに限られ、settings.json の allow ルールでは戻らない。

この前提で、ルールの各行は次のようになる。

- Glob / Grep を使えという指示は、存在しないツールを指している。
- `find` / `grep` の禁止は、組み込みの読み取り専用経路と矛盾する。
- Read 優先・並列実行・`cd` せず絶対パス、という残りの指示は、標準の system prompt（Bash ツールの説明）が既に言っている。

同種の誘導は `docs/plan/2026-08-22_agent-stats-observability-expansion.md` §5 で、`agents.xml` への追加案として一度取り下げている。理由は「速度差がない」「permission prompt の削減にならない」「Bash の方が効率的な場面が多い」「auto mode の指示と衝突する」などで、`7614a0f` の Claude 専用ルールにも、4 CLI 共有という論点を除いて当てはまる。

## Options considered

| 案                                                           | 退けた理由                                                                                                                                                                                                |
| ------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 現状維持                                                     | 存在しないツールを指示し、許可済みの経路を禁止する。常時適用なので、毎セッション矛盾した指示を context に載せる。                                                                                         |
| Read 優先・絶対パスだけに縮小する                            | 残る指示は標準 prompt の重複になる。重複した指示は本体の変更に追随せず、今回のように実態とずれる。                                                                                                        |
| `--allowedTools Glob Grep` の alias で復活させ、ルールを維持 | 起動フラグへの依存が増え、alias の無い環境で再び不整合になる。復活させても組み込み bfs / ugrep と速度・許可の面で差がなく、§5 の論点（速度差なし・prompt 削減にならない）がそのまま残り、得るものが無い。 |
| `agents.xml` など共有指示へ移す                              | §5 で取り下げ済み。4 CLI 共有の指示に Claude Code のツール名を書くと、Codex などに存在しないツールの使用を指示することになる。                                                                            |

## Decision

`tool-selection.md` を削除し、ファイル探索のツール選択は Claude Code の既定に任せる。縮小版も代替ルールも置かない。

決め手は、ルールに残る内容が「存在しないツールへの誘導」と「標準 prompt との重複」だけで、固有の指示が無いこと。

ここから一般則を引く。**Claude Code 本体が担う挙動を rules で二重に指示しない。本体の既定が合わない具体的な回帰を観測してから、その回帰に絞って書く。** `AGENTS.md` の「formatter / linter が強制できる規約は rules に重複させない」と同じ発想を、本体の既定にも適用する。

## Consequences

macOS では、探索は Bash の `find` / `grep`（組み込み bfs / ugrep）で、内容の読み取りは Read で行われる。これは想定された動作であり、不整合ではない。

見直す条件は次の2つ。

- Glob / Grep がデフォルトで有効に戻った場合。
- 標準 prompt が効かず、`cat` / `sed -n` の乱用のような具体的な回帰を観測した場合。この場合は旧ルールを戻さず、観測した回帰に絞って新規に書く。

Bash を持たない subagent の `tools: Read, Grep, Glob` は変更しない。公式仕様で、subagent が `tools` に Glob / Grep を書き Bash を外すと、そのツールが subagent に戻る。`.claude/rules/agent-authoring.md` の規約もそのまま有効。

`ai-agents/scripts/copy-entries.sh` は上書きコピーのみで、削除を伝播しない。`scripts/config-diff` も、skills 以外では配布先にだけ残ったファイルを検出しない。今後 rules を削除するときは、`~/.claude/rules/` の実体を手動で消す。

この決定は Claude Code 2.1.291 の既定挙動を前提にする。前提が変わったら、新しい ADR を書いて本 ADR を Superseded にする。

関連: [#873](https://github.com/hodanov/my-pde/pull/873)、`docs/plan/2026-08-22_agent-stats-observability-expansion.md` §5
