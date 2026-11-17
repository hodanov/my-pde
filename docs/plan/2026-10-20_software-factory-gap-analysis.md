# Plan: ソフトウェアファクトリーとしてのギャップ分析と改善ロードマップ

my-pde の自律改善パイプラインを「ソフトウェアファクトリー」として見たとき、何ができていて何が足りないかを整理し、外部の理論・手法・ツールと照らして改善の順番を決める。

結論は 2 点。

- 半自律で回るループ（scan → triage → PR Bot → PR Care Bot → merge → digest → 月次メタループ）はすでに成立している。
- 次の手は 3 つ。
  - クラウド実行の前提を公式仕様に合わせる。
  - intake の質を上げる。
  - 結果の品質を測るセンサーを足す。

独立 verifier のような重い仕組みは、センサーの数値が事前に決めた条件を超えたときに入れる。

## Background

- 問い: 「このリポでソフトウェアファクトリーを実現するのに足りない部分は何か。逆にできている点は何か」。参考は [addyosmani/factory](https://github.com/addyosmani/factory)。
- 調べ方: 3 つを組み合わせた。
  - 外部の一次情報を調べた（末尾の References）。
  - repo の棚卸しをした（`routines/`、`scripts/`、`ai-agents/`、`.claude/`、`.github/workflows/`、ブランチ保護）。
  - 草案を独立したコンテキストで批判的にレビューした。
  - 数値はすべて 2026-10-20 時点で、`gh` と `git` で再確認している。
- addyosmani/factory の定義では、ソフトウェアファクトリーとは「ソフトウェア提供を繰り返し回すループ」のこと。具体的には次を決めておく運用モデルを指す。
  - エージェントが何を試してよいか
  - 作業がどう始まるか
  - どんな証拠を残すか
  - どこで人間が判断するか
  - 構成要素: CHARTER（人間が持つ方針）、CONTRACT（キューの状態とハンドオフ）、`gates.sh`（1 行の判定、fail-closed）、書き手と分離した verifier（revert-to-fail によるテストの証明）、monitor（検出専用）、tune（提案専用）。
- 判断の前提: このリポでは、観測された問題がない仕組みは却下されてきた。だから外部の手法は「どの実測ギャップに効くか」で選ぶ。

## Current structure

### 稼働中のループ

| 段階   | 実体                                                                                                                          | 人間の関与 |
| ------ | ----------------------------------------------------------------------------------------------------------------------------- | ---------- |
| 起票   | 5 本の scan Routine（`routines/prompts/weekly-*-scan.md`。週 1・最大 1 件・本文約 1,500 字）                                  | なし       |
| triage | ラベル `adopted` / `rejected`（却下理由をコメント）                                                                           | 必須       |
| 実装   | `routines/prompts/weekly-adopted-issue-pr-bot.md`（`auto/issue-<N>-<slug>`、`ai-agents/scripts/verify-changed.sh`、draft PR） | なし       |
| 保守   | `routines/prompts/weekly-pr-care-bot.md`（CI 失敗・コンフリクト・レビュー指摘。rebase / force-push はしない）                 | なし       |
| merge  | 人間が行う。auto-merge は Dependabot と tools-bump だけ（`automation-deps-auto-merge.yml`）                                   | 必須       |
| 計測   | `automation-pipeline-digest.yml` → `scripts/pipeline-metrics` → digest #532（alert と JSON）                                  | なし       |
| 改善   | `routines/prompts/monthly-routine-improve.md`（予告・コホート・最小件数・指標を最適化しない）                                 | merge のみ |

### 実績

- マージ済みの auto PR は 111 件。全マージ PR の約 40%。
- マージ率は 97%（115/119）。PR 作成からマージまでは p50 0.1 日、issue からマージまでは p50 4.1 日。
- 採用率は全体で 72%。月次では 79% → 77% → 68% → 60% と下がっている。`scan:scripts` は 1/13。
- マージ済み auto PR 111 件のうち、`scripts/**/*.go` を触ったのは 6 件、`nvim/config` を触ったのは 67 件。

### できている点

| 強み                         | 根拠                                                                                                                                          | 対応する理論・手法                                                                                                          |
| ---------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| 決定的な計測とメタ改善       | `scripts/pipeline-metrics`（golden test、`TestJSONContract`）と月次メタループの予告・答え合わせ                                               | Deming の PDSA、事前登録、Goodhart の法則への対策。addyosmani/factory は ROI 計測と自己改善を「意図的に持たない」としている |
| 状態を repo と GitHub に置く | Routine のプロンプトを repo から間接参照し、ラベルの規約は `routines/README.md` に置いている                                                  | addyosmani/factory の core bet（必要なものはすべて repo にコミットする）                                                    |
| 自律度を影響で決める         | merge は必ず人間が行う。auto-merge は依存更新だけ                                                                                             | addyosmani/factory の autonomy tracks consequence                                                                           |
| 発生源で WIP を絞る          | scan は週 1・最大 1 件・1,500 字                                                                                                              | Kanban の WIP 制限、TOC                                                                                                     |
| 計算的センサーが厚い         | `verify-changed.sh`（SKIP は PASS ではない）、PostToolUse の formatter、Stop hook、`guard-version-pins.sh`、nvim のスモークテスト（74dffb9b） | Böckeler の guides と sensors、fail-visible                                                                                 |
| コンテキスト設計             | scout / diver・scanner / critic の 2 段構成、worktree、context anchor                                                                         | Horthy の Research → Plan → Implement、intentional compaction                                                               |
| 指示は地図にとどめる         | `AGENTS.md` は目次、`.claude/rules/` は paths で絞って読み込む、仕様から自明なことは書かない                                                  | OpenAI の harness engineering                                                                                               |
| ローカルの可観測性           | `environment/otel/`、`scripts/agent-stats`、permission ledger                                                                                 | observability                                                                                                               |

## Design policy

### 足りない点（優先度順）

#### G1. クラウド実行の前提が公式仕様とずれている

- [cloud-environments](https://code.claude.com/docs/en/cloud-environments) によると、repo の `.claude/settings.json` に書いた `enabledPlugins` / `extraKnownMarketplaces` はクラウドセッションではインストールされない。
  - `routines/README.md` の「クラウドで有効」表と、`docs/plan/2026-08-23_routine-cloud-context-alignment.md` の Phase 2（plugin 化）は、この前提が崩れている。
- 同じドキュメントによると、GitHub proxy は GraphQL を 403 で拒否する。`gh pr` / `gh issue` もこの対象に入る。
  - 全 Routine のプロンプトは `gh issue list --json …` / `gh pr create` を前提にしている。実際には issue も PR も作れているので、モデルが毎回その場で回避している可能性が高い。そうだとすると手順は決定的ではない。
- クラウドの PR Bot に別のサブエージェントを渡す経路は、repo ローカルの `.claude/agents/` しかない。

#### G2. intake の質が制約になっている

- ボトルネックは実装ではなく「何を作るか」に移っている。採用率は月次で 79% から 60% へ下がった。
- scan は却下済み issue を `--json number,title,body` で読むだけで、オーナーが書いた却下理由のコメントを読んでいない（例: `routines/prompts/weekly-environment-scan.md`）。
- 却下した方針や方針転換（例: 74dffb9b で lazy-lock の固定へ切り替えた）は repo に記録がなく、クラウドの scan からは見えない。

#### G3. 結果の品質を測っていない

- 97% のマージ率は受け入れ率で、正しかったかどうかは表さない。DORA の rework rate に当たる指標がない。
- `pr-created` が付いているのに auto PR が無い issue（約 14 件）は、ラベル運用の崩れではなかった。オーナーが自分で実装し直したケースだった（例: #811 → #829）。「課題は採用、実装は不採用」という信号であり、`pr_created_rate` に混ざっている。
- Routine のコミットには `Claude-Session:` トレーラーが付く（公式仕様）。これを使えば、マージ前にオーナーが足した修正コミットを決定的に区別できる。
  - 概算では、08-08 以降の 62 件のうち 7 件（約 11%）に修正コミットが入っていた。そのうち 3 件はコメント削除で、#867 で対処済み。
- revert（例: #583 → #591）も集計していない。

#### G4. merge / push の境界がプロンプトと draft PR に頼っている

- repo は PUBLIC。scan は WebFetch で信頼できない外部の内容を読み、同時に Bash とオーナーのトークンも持っている。
- main のブランチ保護は `enforce_admins: false` で、Routine はオーナーと同じ identity で動く。
- proxy は push 先のブランチを制限しない。ドキュメントは rulesets の利用を勧めている。

#### G5. 書き手が自分の仕事を採点している（ただし今は証拠が弱い）

- PR Bot の品質確認は、diff を `.claude/rules/` に照らすセルフレビューだけ（#867、ADR-0006）。
- 一方で、マージ後の手戻りには好みの調整が多く、verifier では防げない。
- Go を触った auto PR は 6/111 件と少ないので、Go 向けの prove-test を入れても効きにくい。#673（cover-diff）も同じ理由で NOT_PLANNED になった。

#### G6. ハーネス自体に回帰テストがない

- skill の observation は 138 件中 failure が 0 件。自己申告なので、評価の信号として働いていない。
- `claude plugin eval`（[plugin-evals](https://code.claude.com/docs/en/plugin-evals)）で、cases と graders と with / without の比較が使える。

### 選び方の原則

- 決定的なセンサーを先に足す。LLM の判定は、それで分からない部分だけに使う。
- 重い仕組みは、導入条件を数値で事前に登録しておき、センサーがその条件を超えてから入れる。
- Routine の挙動を変える変更は `meta:routines` ラベルを付けて、検証予告と一緒に出す。既存のメタループに答え合わせをさせるため。
- ADR-0006 は維持する（LLM の自動化は Routine で行い、CI に Claude の認証情報を置かない）。

### addyosmani/factory から採らないもの

| 要素                                                 | 採らない理由                                                                                         |
| ---------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| `factory:*` ラベルの状態機械と決定的ブランチの claim | 週 1 回の単発実行なので、並行して取り合うことがない                                                  |
| `docs/factory/runs/` の実行記録ファイル              | PUBLIC repo のノイズになる。トレーラーと PR 本文のセッション URL で辿れる                            |
| tier 制度                                            | 対象は 1 repo・1 tier                                                                                |
| `factory-handoff:v1`（`done_when` など）             | 失敗が起きているのは起票段階で、実装段階ではない。scan の本文は 1,500 字を前提にしている             |
| STOP_IF（レビュー待ちの上限）                        | Open な auto PR は 0 件で、PR はほぼ当日にマージされている                                           |
| 既存テストの改変を禁じるガード                       | 該当事例が 0 件                                                                                      |
| Go 向けの prove-test                                 | 対象になる PR が 6/111 件しかない                                                                    |
| ローカルのテレメトリをクラウドへ送る                 | repo が PUBLIC。agent-stats の計測欠陥（`TODO_2026-08-28_agent-stats-measurement-fixes.md`）が未解決 |

## Implementation steps

各項目を 1 つの issue として起票する。採否はオーナーが判断する。

1. **A. クラウド前提の訂正**（G1）。土台なので最優先にする。
   - まず、最新の PR Bot と scan のセッションログに `GitHub GraphQL is not available` が出ているかを確かめる。
   - `routines/README.md` の表のうち plugin の行を「無効」に直す。
   - `docs/plan/2026-08-23_routine-cloud-context-alignment.md` に「Phase 2-2 は前提が崩れたため打ち切り」と追記する。
   - 403 が出ていた場合は、`routines/prompts/*.md` の `gh issue` / `gh pr` を `gh api repos/{owner}/{repo}/...` の REST に置き換える。
     - `/issues` の結果には PR も混ざるので除外する。
     - 1 ページは 100 件なので、ページングする。
     - `head:auto/` での検索は jq で代わりに絞る。
2. **B. intake の質を上げる**（G2）。
   - 却下済み issue を取得するときに、オーナーの最後のコメントも足す。
   - `scan:scripts` は、止めるか pull 型（オーナーが起票した困りごとにだけ応える）にするかを決める。
   - lazy-lock 固定への方針転換を ADR として記録する。
   - A のプロンプト書き換えと同じ PR にまとめて、コホートの境界を 1 回で済ませる。
   - マージは 11/2 のメタループ実行より後にする。Open な `meta:routines` PR があるとメタループは止まるし、#848 の答え合わせとも混ざるため。
3. **C. 結果センサー その 1: 作り直しと revert**（G3）。
   - `scripts/pipeline-metrics` に `owner_takeover` を足す。adopted かつ Close 済みで auto PR が無いものを数え、`pr_created_rate` の分母から外す。
   - `pr_label_without_pr` は Open な issue に限る。
   - revert はタイトルから検出する。
   - 最初は表示するだけで、アラートは出さない。
4. **D. main の ruleset とクラウド専用の merge ガード**（G4）。
   - main に ruleset を足す。force push の禁止、削除の禁止、PR の必須化、bypass なし。
   - 任意で `.claude/hooks/guard-cloud-merge.sh` を足す。`CLAUDE_CODE_REMOTE=true` のときだけ、merge と main への push を止める。
5. **E. 結果センサー その 2: オーナー修正コミット率と verifier の導入条件**（G3、G5）。
   - digest でトレーラーの無い修正コミットを数える。
   - 導入条件を事前に登録する。直近 20 件で修正率が 15% 以上、または正しさを理由にした revert が出たときに、次の 2 つを入れる。
     - `.claude/agents/pr-verifier.md`
     - PR Bot の `allowed_tools` に `Agent` を追加
   - 同じ PR で次の 2 つも直す。
     - PR Care Bot の「人のコミット」判定をトレーラー基準にする。
     - PR Bot がスキップした理由を issue にコメントする。
6. **F. ローカル専用の skill eval**（G6、次点）。
   - `ai-agents/evals/` に、dev-workflow と commit-and-draft-pr が起動するかを確かめるケースを置く。
   - `skill-improve` の Evaluate で、適用の前後に回す。
   - CI には組み込まない（ADR-0006）。

## File changes

| File                                                                                           | Change                                                              |
| ---------------------------------------------------------------------------------------------- | ------------------------------------------------------------------- |
| `routines/README.md`                                                                           | A: 「クラウドで有効」表の plugin 行を訂正                           |
| `docs/plan/2026-08-23_routine-cloud-context-alignment.md`                                      | A: Phase 2-2 の打ち切りを追記                                       |
| `routines/prompts/*.md`                                                                        | A: `gh issue` / `gh pr` を REST に置換。B: 却下理由のコメントを取得 |
| `routines/weekly-scripts-tooling-scan.json`                                                    | B: 停止する場合だけ `enabled:false`（`/schedule` で反映）           |
| `docs/adr/0011-*.md`                                                                           | B: lazy-lock 固定への方針転換                                       |
| `scripts/pipeline-metrics/internal/model/model.go`, `internal/metrics/metrics.go`, `testdata/` | C: `owner_takeover` と revert の検出。E: `--commits` 入力           |
| `.github/workflows/automation-pipeline-digest.yml`                                             | E: `fetch-depth: 0` とコミット一覧の受け渡し                        |
| GitHub ruleset（repo 設定）                                                                    | D: main の保護                                                      |
| `.claude/hooks/guard-cloud-merge.sh`, `.claude/settings.json`                                  | D（任意）: クラウド専用の merge / push ガード                       |
| `routines/prompts/weekly-pr-care-bot.md`, `weekly-adopted-issue-pr-bot.md`                     | E: トレーラー基準の判定と、スキップ理由のコメント                   |
| `ai-agents/evals/`, `ai-agents/skills/skill-improve/SKILL.md`                                  | F: eval ケースと Evaluate への組み込み                              |

## Risks and mitigations

| Risk                                                            | Mitigation                                                                                            |
| --------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| REST への置換で取得漏れ（PR が混ざる、ページング）              | `select(.pull_request \| not)` とページングを入れる。次回実行のログで件数を確認する                   |
| 却下理由を読ませた結果、scan がその領域を丸ごと避ける           | 予告する指標を `adopted_rate` だけでなく起票数とあわせて見る                                          |
| `owner_takeover` と revert の判定が誤検出する                   | 最初は表示だけにする。実データでの件数（takeover 約 14 件、revert は #583 を含む 1 件以上）で検証する |
| ruleset によって Dependabot / tools-bump の auto-merge が止まる | 導入後最初の週次 bump で確認する                                                                      |
| skill eval がサブスクの枠を食う                                 | ケースは 2〜3 本に絞り、回すのは `skill-improve` の適用前後だけにする                                 |

## Validation

- A: 次回の Routine 実行ログに 403 が出ていない。メタループの手順 1 で digest の JSON が取れる。
- B: マージ後に起票された分のコホートで、scan 別の `adopted_rate` が予告した値に届く。
- C: golden test と `TestJSONContract` が通る。実データで `owner_takeover` と revert が期待どおりの件数で出る。
- D: ローカルからの `git push origin HEAD:main` が ruleset に拒否される。依存更新の auto-merge は通る。
- E: digest にオーナー修正コミット率が出る。導入条件を満たしたかどうかを毎週判定できる。
- F: `claude plugin eval` で、対象のケースが with-plugin の側で通る。

## Open questions

- A: 403 は実際に出ているのか。出ていないなら何経路で回避しているのか（built-in の GitHub tools か、`gh api` への自発的な切り替えか）。
- B: `scan:scripts` は停止と pull 型のどちらにするか。
- D: ruleset だけで足りるか。クラウド専用 hook まで入れるか。
- E: 導入条件の閾値（15%・直近 20 件）は妥当か。
- F: 着手は E の結果を見てからでよいか。

## References

- [addyosmani/factory](https://github.com/addyosmani/factory)（README、ARCHITECTURE、ADVICE、LIMITS、template の CHARTER / CONTRACT / factory-verifier / prove-test.sh / factory-monitor / factory-tune）
- [Configure cloud environments](https://code.claude.com/docs/en/cloud-environments)
- [Test plugins with evals](https://code.claude.com/docs/en/plugin-evals)
- [How StrongDM's AI team build serious software without even looking at the code](https://simonw.substack.com/p/how-strongdms-ai-team-build-serious)
- [The Five Levels（Dan Shapiro, via Simon Willison）](https://simonwillison.net/2026/Jan/28/the-five-levels/)
- [Harness engineering（OpenAI）](https://openai.com/blog/harness-engineering)
- [Harness engineering for coding agent users（Birgitta Böckeler）](https://martinfowler.com/articles/harness-engineering.html)
- [The Ralph Wiggum loop](https://codecentric.de/en/knowledge-hub/blog/the-ralph-wiggum-loop-autonomous-code-generation-with-a-fresh-context)
- [Context Engineering for Complex Codebases（Dex Horthy）](https://ai.engineer/talks/rmvDxxNubIg-context-engineering-for-complex-codebases)
- [State of the Software Factory（Dex Horthy）](https://www.wearedevelopers.com/videos/100484-state-of-the-software-factory.md)
- [Demystifying evals for AI agents（Anthropic）](https://anthropic.com/engineering/demystifying-evals-for-ai-agents)
- [DORA metrics history](https://dora.dev/guides/dora-metrics/history/)
- [2025 State of AI-assisted Software Development（DORA）](https://services.google.com/fh/files/misc/2025_state_of_ai_assisted_software_development.pdf)
