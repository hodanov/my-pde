# Plan: my-pde セキュリティ対策

## Background

2025〜2026 年は、開発者の手元と CI を狙うサプライチェーン攻撃が続いている。
my-pde は public リポで、ホストの `~/.claude` / `~/.codex` / `~/.zshrc` へ配布される設定の正であり、週次の自動更新 PR と Cloud Routine が main に変更を入れる。
main が汚れるとホストでコードが実行され、手元の資格情報が盗まれると全リポへ書き込める。
この計画で攻撃経路を塞ぎ、受け入れるリスクは ADR-0011 に明記する。完了後は、他の管理リポへ `supply-chain-hardening` skill として展開する。

### 最近の事例と教訓

| 事例                                                         | 手口                                                                                                         | このリポへの教訓                                                     |
| ------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------- |
| tj-actions / reviewdog（2025-03）                            | `pull_request_target` から盗んだトークンで、Action のタグを悪性コミットへ書き換えた                          | SHA 固定（済）に加え、更新 PR の無審査マージを止める                 |
| Nx s1ngularity（2025-08）                                    | postinstall スクリプトが手元の AI CLI を `--dangerously-skip-permissions` で起動し、秘密を探させて流出させた | bypass モードの禁止、秘密ファイルの読み取り禁止、install script 停止 |
| Shai-Hulud（2025-09、2.0 は preinstall で実行）              | 盗んだ npm トークンで自己増殖した                                                                            | cooldown、`--ignore-scripts`、トークンの最小化                       |
| GhostAction（2025-09、2026-10 に再来）/ Megalodon（2026-05） | 乗っ取ったアカウントで workflow を push し、secrets を盗んだ                                                 | 既定トークンを read に、2FA、gh トークンのスコープ削減               |
| Clinejection（2026-02）/ Comment and Control                 | Issue や PR の文面によるプロンプトインジェクションで、CI 上のエージェントが乗っ取られた                      | Routine が外部テキストを取り込む段階で除外し、自動マージ経路を断つ   |
| mise GHSA-wcqh-j26q-g44x（2026-10）                          | 未信頼プロジェクトの設定経由で `GITHUB_TOKEN` が別ホストへ送られた                                           | mise を 2026.10.0 以上に保つ                                         |

### 参照したフレームワークとツール

- 理論: 脅威モデリング（STRIDE / アタックツリー、NIST SSDF PW.1.1）、多層防御、最小権限、侵害前提（Assume breach）と爆発半径の最小化。AI エージェントについては lethal trifecta（未信頼の入力・秘密へのアクセス・外部送信が同じセッションに揃うと漏れる）。
- フレームワーク: NIST SSDF（SP 800-218）、SLSA、OpenSSF Scorecard、OWASP Top 10 CI/CD Security Risks、OWASP Top 10 for Agentic Applications 2026。
- ツール: zizmor、actionlint、gitleaks と GitHub push protection、Dependabot `cooldown` と mise `minimum_release_age`、govulncheck、Claude Code の sandbox と deny ルール。

出典: [Wiz: GitHub Actions hardening](https://www.wiz.io/blog/github-actions-security-guide)、[Wiz: reviewdog](https://wiz.io/blog/new-github-action-supply-chain-attack-reviewdog-action-setup)、[GitGuardian: s1ngularity](https://blog.gitguardian.com/the-nx-s1ngularity-attack-inside-the-credential-leak/)、[Unit 42: Shai-Hulud](https://unit42.paloaltonetworks.com/?p=157750)、[GitGuardian: GhostAction](https://blog.gitguardian.com/ghostaction-campaign-3-325-secrets-stolen/)、[Megalodon](https://www.secpod.com/learn/security-research/megalodon-supply-chain-attack-compromises-5-500-git-hub-repositories-through-malicious-ci-cd-workflows)、[CSA: Claude Code Action prompt injection](https://labs.cloudsecurityalliance.org/research/csa-research-note-claude-code-github-action-prompt-injection/)、[OWASP Agentic Top 10](https://genai.owasp.org/resource/owasp-top-10-for-agentic-applications-for-2026)、[OWASP CI/CD Top 10](https://owasp.org/www-project-top-10-ci-cd-security-risks/)、[zizmor](https://docs.zizmor.sh/audits/)、[mise v2026.10.0](https://newreleases.io/project/github/jdx/mise/release/v2026.10.0)、[Claude Code sandboxing](https://docs.claude.com/en/docs/claude-code/sandboxing)

## Current structure

ワークフローの書き方は既に堅い。全ワークフローが `permissions:` を明示し、`uses:` は全て SHA 固定、checkout は `persist-credentials: false`、危険なトリガーは無く、zizmor・actionlint・gitleaks・govulncheck・CodeQL が動いている。
弱いのはリポ設定、自動マージの条件、AI エージェントの権限、外部入力の取り込みの 4 か所である。

### 資産

| ID  | 資産                                                                               | 盗まれる・汚されると何が起きるか                                                            |
| --- | ---------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| A1  | ホストの gh トークン（classic、`repo` / `workflow` / `admin:public_key` / `gist`） | 全リポへの書き込み、workflow の追加                                                         |
| A2  | my-pde の main                                                                     | hooks・settings・plugin・Routine のプロンプトの配布元。汚されるとホストでコードが実行される |
| A3  | CI の secrets                                                                      | `BUMP_APP_PRIVATE_KEY`（contents と PR の write）                                           |
| A4  | ホストの資格情報                                                                   | `~/.ssh`、`~/.codex/auth.json`、`~/.docker/config.json` など                                |

### 攻撃経路（優先度順）

| ID  | 経路                                                                                                                                | 対応           |
| --- | ----------------------------------------------------------------------------------------------------------------------------------- | -------------- |
| AP1 | 上流のリリース改ざん → tools-bump / Dependabot の PR が無審査で自動マージ → main → ホストとイメージ                                 | P1             |
| AP2 | 悪性の依存がコンテナ内で root 実行 → 読み書き可能な `~/workspace` の hooks を書き換え → ホストで実行 → A1・A4 を窃取 → 全リポを汚染 | P4（受容）、P2 |
| AP3 | 公開 Issue やコメントによるプロンプトインジェクション → Routine が push → `chore/bump-tool-versions*` ブランチなら自動マージ → main | P1、P3         |
| AP4 | このリポのローカルエージェント（sandbox OFF、`curl` は allow 済）が、読んだ内容で操られて秘密を読み、外へ送る                       | P2             |
| AP5 | gh トークンやアカウントの乗っ取り → workflow を push して secrets を窃取                                                            | P0             |

## Design policy

- 境界は OS の sandbox、組み込みの deny ルール、GitHub の設定に置く。正規表現のブロックリスト（`guard-dangerous-bash` / `guard-secret-read`）の穴埋めは追わない。
- 未信頼のテキストは「データとして扱え」とプロンプトで頼むのではなく、取得する段階で除外する。
- 新しいリリースは 14 日待ってから取り込む。Dependabot の `cooldown` と揃える。
- 判定できない自動マージは止める（fail-closed）。
- 全部は塞げない。コンテナは `~/workspace` に対する境界ではないこと、ai-bridge、スキャン系 Routine の残余リスクを ADR-0011 で受容する。

### やらないこと

| やらないこと                                               | 理由                                                                                                              |
| ---------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| harden-runner / Scorecard Action を CI に常駐させる        | 全ジョブに触れる第三者 Action が増える。publish 用の secrets を持たない個人リポでは効果が小さい                   |
| 正規表現ガードの穴埋め                                     | ブロックリストはいたちごっこになる                                                                                |
| main の承認レビューを必須にする                            | 1 人のリポでは自分の PR を承認できない。必須チェックと自動マージの条件で担保する                                  |
| コンテナの非 root 化やマウントの縮小                       | 2026-10-22 に見送りと決定。安い対策と ADR での受容にとどめる                                                      |
| Dockerfile の全取得物の SHA256 をリポに固定する            | tools-bump の大改修になる。cooldown を入れた後は利得が小さい                                                      |
| 同一リポ参照を `$/` 構文にする（zizmor `self-repository`） | actionlint 1.7.12 が未対応で CI が落ちる（rhysd/actionlint#711）。対応後に見直す                                  |
| gosec、コミット署名の必須化                                | gosec は ADR-0010 の見直し条件に当たらない。squash merge は GitHub が署名し、直 push は `enforce_admins` で止まる |

## Implementation steps

### P0: GitHub とアカウントの設定（`gh api` で操作。PR なし）

1. Actions の既定トークンを read にし、Actions による PR の承認を禁止する。
2. `sha_pinning_required=true` で SHA 固定を強制する。
3. 使っていない secret `CLAUDE_CODE_OAUTH_TOKEN`（ADR-0006）と environment `copilot`（ADR-0009）を削除する。
4. private vulnerability reporting を有効にする。secret scanning の汎用パターンと validity checks は GitHub Secret Protection が必要で、有効にできなかった。
5. main のブランチ保護で `enforce_admins` を有効にする。P1 のマージ後、必須チェックに `gitleaks` と `actions-lint` を足す。
6. gh トークンから `admin:public_key` と `gist` を外す（`gh auth refresh --remove-scopes`）。`workflow` は Cloud Routine が使うので残す。
7. アカウントを手で点検する。2FA が passkey かセキュリティキーか、security log、承認済みの OAuth App / GitHub App、SSH 鍵の用途。

### P1: 自動マージと CI の関門（AP1 / AP3）

1. Dependabot は `dependabot/fetch-metadata` で判定し、semver の patch / minor だけを自動マージする。`github_actions` は除外し、各グループに `update-types: [minor, patch]` を付けてメジャー更新を別 PR にする。`update-type` を判定できない PR（間接依存のセキュリティ更新や digest だけの更新。例: #916）は手動でマージする。
2. tools-bump の自動マージは、ブランチ名に加えて作成者 `hodanov-pde-bump[bot]` で判定する。`permissions` はジョブ単位にする。
3. actions-lint を全 PR で起動し、必須チェックにできるようにする。hadolint を PR で起動する。
4. tools-bump に 14 日の cooldown を入れる。値は `mise.toml` の `[settings] minimum_release_age` に一度だけ書く。`mise upgrade --bump` はこれを守り、ワークフローで公開日を取って選ぶ取得元（GitHub releases、nodejs.org、npm registry、HashiCorp releases API、crates.io）も同じ値から締め切りを計算する。Go と rustup は公開日を取れず、公式の配布元でチェックサムを検証しているので対象外にする。bump 対象に `actionlint` と `zizmor` を足し、`concurrency` を付ける。
5. CI の mise 本体は、14 か所の `jdx/mise-action` をローカルの composite action `.github/actions/setup-mise` に集約し、`minimum_release_age` で 14 日以上前のリリースを選ばせる。版を固定すると bump の仕組みが別に要るため、cooldown で同じ目的を満たす。
6. zizmor の `--min-severity high` を外し、対象を `.github` 全体（`dependabot.yml` と composite action を含む）に広げる。info の template-injection（tools-bump の `run:` への出力値の展開）は env 経由に直す。
7. 週次で全 Go モジュールの govulncheck を回す（ADR-0010 で受容した穴を塞ぐ）。
8. npm の Dependabot アラート 7 件（`environment/tools/node/package-lock.json`）を解消する。

### P2: AI エージェントとホストの設定（AP2 / AP4）

1. このリポの `.claude/settings.local.json`（未追跡）: sandbox 無効の上書きを消し、`docker *` を `excludedCommands` に足す。`curl:*`、`npx:*`、`claude *`、`docker container *`、`docker cp *`、`Read(//Users/hodanov/**)` の allow を消し、`docker exec *` は `docker exec nvim-dev *` に絞る。棚卸しは `.claude/skills/permission-review` に沿う。
2. ユーザー設定 `ai-agents/settings/claude/settings.json`:
   - `permissions.disableBypassPermissionsMode: "disable"` を足す。s1ngularity の手口と、ai-bridge 経由のオプション注入を無効にする。
   - 資格情報の読み取りを二層で禁止する。`permissions.deny` は Read ツールと `cat` などの認識できるコマンドにしか効かず、sandbox は既定でホーム全体を読めるため、sandbox 内の Bash とその子プロセスには `sandbox.filesystem.denyRead` が別に要る（ダミーファイルで `grep -r` が止まることを確認済み）。SSH は known_hosts と config を読めるよう、秘密鍵 `~/.ssh/id_*` だけを禁止する。Docker の `config.json` は sandbox 内の docker CLI がコンテキスト解決に使うので対象外にする。
   - `sandbox.allowUnsandboxedCommands: false` で sandbox 外での再実行を禁止し、足りないものは `excludedCommands` に明示する。
   - `enableWeakerNetworkIsolation` は main で編集中の設定なので、この計画では触らない。
3. Codex（`~/.codex/config.toml`、リポ管理外）: `[sandbox_workspace_write]` の下に入っている `notify` / `model` / `model_reasoning_effort` をトップレベルへ移す。`network_access = true` を外せるか試す。
4. `dotfiles/.zshrc` から、存在せず書き込み可能な `/usr/local/opt/curl/bin` の PATH 追加を消す。`.gitignore` に秘密ファイルのパターンと `.claude/settings.local.json` を足す。

### P3: Routine とプロンプトインジェクション（AP3）

1. Adopted-Issue PR Bot は Issue を `--author hodanov` で取得する。ラベルは triage 権限が要るが、本文は起票者が後から書き換えられる。
2. PR Care Bot と Monthly Routine Improve は、コメントを取得する時点で jq の `select(.user.login == "hodanov")` に絞る。
3. スキャン系 5 本は WebFetch と Bash と広い gh トークンが同居する。`allowed_tools` がパターン指定に対応していれば Bash を必要なコマンドに絞り、対応していなければ ADR-0011 で受容する。
4. クラウド環境のネットワーク設定を claude.ai で確認する。

### P4: コンテナ、ai-bridge、依存（AP2）

1. compose に `cap_drop: [ALL]` と `no-new-privileges` を足し、SECURITY NOTE を「コンテナは `~/workspace` に対する境界ではない」と書き直す。
2. `npm ci` に `--ignore-scripts` を付ける。
3. Python を `uv lock` し、`uv sync --locked` にする。
4. otel-lgtm のイメージを digest で固定し、Dependabot の対象に足す。
5. ai-bridge はプロンプトの前に `--` を入れ、`~/.ai-bridge` を 0700、`history.jsonl` を 0600 にし、cwd を `$HOME/workspace` 配下に制限する。
6. marp skill の `npx --yes …@latest` の版を固定する。

### P5: 文書化と展開

1. ADR-0011 に脅威モデル、自動マージの方針、受容したリスクを書く。ADR-0002（peter-evans 前提）と ADR-0006（secret 削除）のずれを直す。
2. 侵害時の手順書 `docs/runbook/supply-chain-incident.md` を短く書く。確認先（lockfile、該当期間の Actions 実行、新しい repo / workflow）と、失効させる資格情報（gh トークン、App の秘密鍵、Claude / Codex の認証、SSH 鍵）。
3. OpenSSF Scorecard の CLI を着手前と完了後に 1 回ずつ実行して比べる。
4. リポ設定、自動マージ条件、zizmor の項目を `ai-agents/skills/supply-chain-hardening` に足す。

### PR の分割

| PR   | 範囲       |
| ---- | ---------- |
| PR-A | P1-1〜3    |
| PR-B | P1-4〜7    |
| PR-C | P2-2、P2-4 |
| PR-D | P3         |
| PR-E | P4、P1-8   |
| PR-F | P5         |

P0、P2-1、P2-3 はリポの外の操作で、PR にしない。

## File changes

- PR-A: `.github/workflows/automation-deps-auto-merge.yml`、`.github/dependabot.yml`、`.github/workflows/ci-actions-lint.yml`、`.github/workflows/ci-dockerfile-lint.yml`
- PR-B: `mise.toml`、`.github/workflows/automation-tools-bump.yml`、新規 `.github/actions/setup-mise/action.yml`、`jdx/mise-action` を使う全ワークフロー、新規 `.github/zizmor.yml`、新規 `.github/workflows/automation-go-vuln-scan.yml` と `mise.toml` の `go:vuln` タスク、`.github/dependabot.yml`、`AGENTS.md`、`README.md`
- PR-C: `ai-agents/settings/claude/settings.json`、`dotfiles/.zshrc`、`.gitignore`
- PR-D: `routines/prompts/weekly-adopted-issue-pr-bot.md`、`routines/prompts/weekly-pr-care-bot.md`、`routines/prompts/monthly-routine-improve.md`、`routines/*.json`
- PR-E: `environment/docker/docker-compose.yml`、`environment/docker/nvim.dockerfile`、`environment/tools/python/`、`environment/tools/node/package-lock.json`、`environment/otel/docker-compose.yml`、`scripts/ai-bridge/internal/`、`ai-agents/skills/marp-deck-export/scripts/`
- PR-F: `docs/adr/0011-*.md`、`docs/adr/0002-*.md`、`docs/adr/0006-*.md`、`docs/runbook/supply-chain-incident.md`、`ai-agents/skills/supply-chain-hardening/`

## Risks and mitigations

| リスク                                                                       | 対策                                                                             |
| ---------------------------------------------------------------------------- | -------------------------------------------------------------------------------- |
| 間接依存のセキュリティ更新や digest 更新が自動マージされず、取り込みが遅れる | Dependabot alerts で気付ける。週次で手動マージする                               |
| 14 日の cooldown がセキュリティ修正の取り込みも遅らせる                      | Dependabot のセキュリティ更新は別経路で届く。急ぐものは手で `mise.toml` を上げる |
| `allowUnsandboxedCommands: false` で普段のコマンドが詰まる                   | permission-ledger を 1 週間見て、必要なものを `excludedCommands` に足す          |
| `cap_drop: [ALL]` で nvim の LSP や treesitter のビルドが壊れる              | 使い捨てコンテナで動作確認してから反映する                                       |
| actionlint の作者が活動停止気味で、新しい構文への追従が止まる                | zizmor を主に据え、actionlint は構文チェックに限る。止まったら代替を探す         |

## Validation

- P0: `gh api` で設定を読み直す。Scorecard を着手前と完了後で比べ、Token-Permissions・Branch-Protection・Pinned-Dependencies が改善していること。
- P1: `mise run lint` が通ること。tools-bump を `workflow_dispatch` で実行し、選ばれた版が全て 14 日以上前のリリースであること。次の Dependabot PR で patch / minor だけが自動マージされ、メジャー更新と `github_actions` が残ること。通常の PR で `gitleaks` と `actions-lint` が必須チェックとして出ること。
- P2: 新しいセッションの `/status` で設定元を確認し、このリポで sandbox が ON になっていること、秘密ファイルの読み取りと `claude --dangerously-skip-permissions` が拒否されること、`docker exec nvim-dev …` が動くこと。
- P3: Routine を手動実行し、取得コマンドに author フィルタが入っていることをログで確認する。
- P4: `mise run docker:build` と、コンテナ内での nvim のヘッドレス起動（LSP と treesitter）、`mise run ai-bridge:test` が通ること。
- 各 PR: リポのルートで `mise run verify:changed` と `mise run lint` を実行する。

## Open questions

- Routine の `allowed_tools` が `Bash(gh issue create:*)` のようなパターン指定に対応しているか。
- Codex の `network_access` を外して普段の作業が回るか。
