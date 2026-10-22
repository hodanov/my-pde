# Plan: my-pde セキュリティ対策（2026-10-22）

## Context

2025〜2026 年は、開発者の手元と CI を狙うサプライチェーン攻撃が続いている（下の事例表）。
my-pde は **public リポ**で、次の 3 つの性質を持つ。

- ホストの `~/.claude` / `~/.codex` / `~/.zshrc` へ配布される設定の正
- 週次の自動更新 PR と Cloud Routine が main に変更を入れる経路
- 管理している他リポの対策の雛形（`supply-chain-hardening` / `credential-leak-prevention` skill）

このため、main が汚れるとホストでのコード実行につながり、手元の資格情報が盗まれると全リポへ書き込める。
ワークフローの書き方（SHA 固定、最小権限、`persist-credentials: false`、zizmor）は既に堅い。
弱いのは **リポ設定・自動マージ条件・AI エージェントの権限・外部入力の取り込み**の 4 か所である。
これらを塞ぎ、受け入れるリスクは ADR に明記する。

決定事項（2026-10-22）:

- 自動マージは**条件を締めて残す**
- このリポの sandbox は**再有効化し、docker だけ除外する**
- コンテナは**リスク受容を明記し、安い対策だけ入れる**

---

## 1. 調査: プロはどう守っているか

### 理論・原則

| 原則                                        | 中身                                                                                                                | このプランでの使い方                                                   |
| ------------------------------------------- | ------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| 脅威モデリング（STRIDE / アタックツリー）   | 資産 → 入口 → 攻撃経路 → 対策の順に考える（NIST SSDF PW.1.1）                                                       | §2 で経路を列挙し、対策を経路に紐付ける                                |
| 多層防御・最小権限                          | 1 つの対策が破られても次で止める。権限は必要最小限に絞る                                                            | 自動マージの条件、トークンの権限、sandbox と deny の二段構え           |
| 侵害前提（Assume breach）・爆発半径の最小化 | 侵入は起きる前提で、起きたときの被害範囲を絞る                                                                      | 資格情報の読み取り禁止、bypass モードの禁止、gh トークンのスコープ削減 |
| Lethal trifecta（AI エージェント固有）      | 「未信頼の入力」「秘密へのアクセス」「外部送信」の 3 つが同じセッションに揃うと、プロンプトインジェクションで漏れる | Routine とローカルのエージェントで、どれか 1 つを構造的に外す          |
| ブロックリストより境界                      | 正規表現で危険コマンドを列挙する方式はいたちごっこになる。OS の sandbox と deny-by-default を境界にする             | regex ガードの穴埋めは追わず、sandbox / deny に寄せる                  |
| リスク受容の明文化                          | 全部は塞げないので、何を受け入れたかを記録する                                                                      | ADR-0011 に書く                                                        |

### フレームワーク

- **NIST SSDF（SP 800-218）**: 安全な開発プロセスの基本セット
- **SLSA**: ビルドの完全性と来歴
- **OpenSSF Scorecard**: リポ設定とワークフローを自動採点する。Token-Permissions / Pinned-Dependencies / Branch-Protection / Dangerous-Workflow など
- **OWASP Top 10 CI/CD Security Risks（CICD-SEC-1〜10）**: フロー制御の不足、依存チェーンの悪用、Poisoned Pipeline Execution など
- **OWASP Top 10 for Agentic Applications 2026（ASI01〜10）**: ゴール乗っ取り（プロンプトインジェクション）、ツールの悪用、権限の悪用など

### ツール

| 用途                                           | ツール                                                                      | このリポでの状態                          |
| ---------------------------------------------- | --------------------------------------------------------------------------- | ----------------------------------------- |
| GitHub Actions の静的解析                      | zizmor、actionlint、pinact                                                  | 済（zizmor は high 以上だけを見ている）   |
| SHA 固定の強制                                 | リポ設定 `sha_pinning_required`                                             | 未                                        |
| 秘密情報の検出                                 | gitleaks、GitHub secret scanning と push protection                         | 済（gitleaks は必須チェックではない）     |
| 新しいリリースの取り込みを遅らせる（cooldown） | Dependabot `cooldown`、mise `minimum_release_age`、pnpm `minimumReleaseAge` | Dependabot だけ 14 日                     |
| 依存の脆弱性検査                               | govulncheck、osv-scanner、Dependabot alerts                                 | 済。ただし npm の open アラートが 7 件    |
| 署名・来歴の検証                               | Sigstore cosign、GitHub attestations                                        | mise.lock で一部済                        |
| AI エージェントの隔離                          | Claude Code の sandbox、deny ルール、`disableBypassPermissionsMode`         | 一部済。ただしこのリポでは sandbox が無効 |
| CI の外向き通信の監視                          | StepSecurity harden-runner                                                  | 未（見送り、§5）                          |

### 最近の事例と教訓

| 事例                                                         | 手口                                                                                                         | 教訓 → このリポで対応する箇所                                                           |
| ------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------- |
| tj-actions / reviewdog（2025-03）                            | `pull_request_target` から盗んだトークンで、Action のタグを悪性コミットへ書き換えた                          | SHA 固定（済）。更新 PR の無審査マージを止める → P1-1                                   |
| Nx s1ngularity（2025-08）                                    | postinstall スクリプトが手元の AI CLI を `--dangerously-skip-permissions` で起動し、秘密を探させて流出させた | bypass モードの禁止、秘密の読み取り禁止、install script の無効化 → P2、P4               |
| Shai-Hulud（2025-09、2.0 は preinstall で実行）              | 盗んだ npm トークンで自己増殖した                                                                            | cooldown、`--ignore-scripts`、トークンの最小化 → P1-3、P4                               |
| GhostAction（2025-09、2026-10 に再来）/ Megalodon（2026-05） | 乗っ取ったアカウントや使い捨てアカウントで workflow を push し、secrets を盗んだ                             | 既定トークンを read に、2FA（passkey）、gh トークンのスコープ、security log の確認 → P0 |
| Clinejection（2026-02）/ Comment and Control                 | Issue や PR の文面によるプロンプトインジェクションで、CI 上のエージェントが乗っ取られた                      | Routine が外部テキストを取り込む段階で除外し、自動マージへの経路を断つ → P3             |
| mise GHSA-wcqh-j26q-g44x（2026-10）                          | 未信頼プロジェクトの設定経由で `GITHUB_TOKEN` が別ホストへ送られた                                           | mise を 2026.10.0 以上にする（ローカルは確認済）。CI も版を固定して追従する → P1-4      |

出典: [Wiz: GitHub Actions hardening](https://www.wiz.io/blog/github-actions-security-guide)、[Wiz: reviewdog](https://wiz.io/blog/new-github-action-supply-chain-attack-reviewdog-action-setup)、[GitGuardian: s1ngularity](https://blog.gitguardian.com/the-nx-s1ngularity-attack-inside-the-credential-leak/)、[Unit 42: Shai-Hulud](https://unit42.paloaltonetworks.com/?p=157750)、[GitGuardian: GhostAction](https://blog.gitguardian.com/ghostaction-campaign-3-325-secrets-stolen/)、[StepSecurity: GhostAction](https://www.stepsecurity.io/blog/ghostaction-campaign-over-3-000-secrets-stolen-through-malicious-github-workflows)、[Megalodon](https://www.secpod.com/learn/security-research/megalodon-supply-chain-attack-compromises-5-500-git-hub-repositories-through-malicious-ci-cd-workflows)、[CSA: Claude Code Action prompt injection](https://labs.cloudsecurityalliance.org/research/csa-research-note-claude-code-github-action-prompt-injection/)、[OWASP Agentic Top 10](https://genai.owasp.org/resource/owasp-top-10-for-agentic-applications-for-2026)、[OWASP CI/CD Top 10](https://owasp.org/www-project-top-10-ci-cd-security-risks/)、[zizmor](https://docs.zizmor.sh/quickstart)、[mise v2026.10.0](https://newreleases.io/project/github/jdx/mise/release/v2026.10.0)、[mise security](https://mise.jdx.dev/security.html)、[Dependabot options](https://docs.github.com/en/enterprise-server@3.20/code-security/reference/supply-chain-security/dependabot-options-reference)、[Claude Code sandboxing](https://docs.claude.com/en/docs/claude-code/sandboxing)

---

## 2. 脅威モデル

### 資産

| ID  | 資産                                                                               | 盗まれる・汚されると何が起きるか                                                            |
| --- | ---------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| A1  | ホストの gh トークン（classic、`repo` / `workflow` / `admin:public_key` / `gist`） | 全リポへの書き込み、workflow の追加                                                         |
| A2  | my-pde の main                                                                     | hooks・settings・plugin・Routine のプロンプトの配布元。汚されるとホストでコードが実行される |
| A3  | CI の secrets                                                                      | `BUMP_APP_PRIVATE_KEY`（contents と PR の write）、未使用の `CLAUDE_CODE_OAUTH_TOKEN`       |
| A4  | ホストの資格情報                                                                   | `~/.ssh`、`~/.codex/auth.json`、`~/.docker/config.json` など                                |

### 主要な攻撃経路（優先度順）

| ID  | 経路                                                                                                                                                     | 主な対策                     |
| --- | -------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------- |
| AP1 | 上流のリリース改ざん → tools-bump / Dependabot の PR が無審査で自動マージ → main → ホストとイメージ                                                      | P1                           |
| AP2 | 悪性の依存（npm / プラグイン）がコンテナ内で root 実行 → `~/workspace` の hooks を書き換え → ホストで実行 → A1・A4 を窃取 → GhostAction 型で全リポを汚染 | P4（受容）、P2（窃取の阻止） |
| AP3 | 公開 Issue やコメントによるプロンプトインジェクション → Routine が push → `chore/bump-tool-versions*` ブランチなら自動マージ → main                      | P1-1、P3                     |
| AP4 | このリポのローカルエージェント（sandbox OFF、`curl` は allow 済）が、読んだ内容で操られて秘密を読み、外へ送る                                            | P2                           |
| AP5 | gh トークンやアカウントの乗っ取り → workflow を push して secrets を窃取                                                                                 | P0                           |

---

## 3. 対策

### P0: GitHub とアカウントの設定（コード変更なし。外部に効く操作なので実行前に 1 件ずつ確認する）

1. **Actions の既定トークンを read にする**
   - `gh api -X PUT repos/hodanov/my-pde/actions/permissions/workflow -f default_workflow_permissions=read -F can_approve_pull_request_reviews=false`
   - 既存ワークフローはすべて `permissions:` を明示しているので影響はない。
2. **SHA 固定を強制する**
   - `actions/permissions` に `sha_pinning_required=true` を設定する。
   - 全 37 箇所が固定済みなので壊れない。
3. **不要な資格情報を消す**
   - secret `CLAUDE_CODE_OAUTH_TOKEN` を削除する（ADR-0006 で不使用と決定済み。grep で参照 0 件）。
   - environment `copilot` を削除する（ADR-0009 で Copilot は撤去済み。参照 0 件）。
4. **secret scanning を拡張する**
   - `secret_scanning_non_provider_patterns` と `secret_scanning_validity_checks` の有効化を試す。プラン上使えなければ見送る。
   - private vulnerability reporting を有効にする。
5. **ブランチ保護**
   - `enforce_admins=true` にする（Claude や手元から main への直 push を止める）。
   - 必須チェックへの `gitleaks` / `actions-lint` の追加は、P1-2 のマージ後に行う。
6. **gh トークンを絞る**
   - `gh auth refresh --remove-scopes admin:public_key,gist` を実行する（リポ内の使用は 0 件。手で使っていないかは確認する）。
   - `workflow` は Routine に必要なので残す（Cloud Routine の GitHub 権限はローカルの gh トークンに由来し、workflow を触るのに必要）。
7. **アカウントを手で点検する（チェックリスト）**
   - 2FA が passkey かセキュリティキーになっているか（API では確認できなかった）
   - security log と、承認済みの OAuth App / GitHub App
   - SSH 鍵 3 本がそれぞれ何に使われているか

### P1: 自動マージと CI の関門（AP1 / AP3）

1. **自動マージの条件を締める**（`.github/workflows/automation-deps-auto-merge.yml`）
   - **Dependabot**
     - `dependabot/fetch-metadata`（SHA 固定）で判定し、`semver-patch` / `semver-minor` だけを自動マージする。
     - `package-ecosystem == github_actions` は除外する。ADR-0002:43 の「pin の bump は差分を確認する」と揃える。
     - グループ PR にメジャー更新が混ざらないよう、`.github/dependabot.yml` の各グループに `update-types: [minor, patch]` を付ける。メジャー更新は別 PR として届く。
     - `update-type` を判定できない PR は自動マージしない（fail-closed）。間接依存のセキュリティ更新（例: #916 のメタデータには `update-type` が無い）や、Docker の digest だけの更新が該当し、手動でマージする。
   - **tools-bump**
     - ブランチ名だけで判定せず、`github.event.pull_request.user.login == 'hodanov-pde-bump[bot]'` を AND 条件にする。
     - これで、Routine（hodanov 名義）がブランチ名を真似ても自動マージされなくなる。
   - `permissions` はトップレベルからジョブ単位へ移す。
2. **セキュリティチェックを必須にできる形にする**
   - `ci-actions-lint.yml` の `paths` フィルタを外して全 PR で走らせる。数十秒で終わる。
   - `lint:actions` と `lint:zizmor` が両方このジョブで走っているかを確認する。
   - `ci-secret-scan.yml`（`gitleaks`）は元から全 PR で走っている。
   - マージ後、ブランチ保護の必須チェックに `gitleaks` と `actions-lint` を追加する（P0-5 の残り）。自動マージはこれらの通過を待つようになる。
3. **tools-bump に cooldown を入れる**
   - `mise.toml` の `[settings]` に `minimum_release_age = "14d"` を設定する。Dependabot の 14 日と揃える。`mise upgrade --bump` にも効く。
   - `automation-tools-bump.yml` で GitHub の `releases/latest` から版を引いている箇所（Neovim、uv、terraform、LuaLS、marksman などの Docker ARG）を、`published_at` が 14 日以上前の最新リリースを選ぶ形に変える。
   - bump 対象に `actionlint` と `zizmor` を追加する（現状どちらも bump されない）。
   - `concurrency` を追加する（force-push の競合を防ぐ）。
4. **CI の mise 本体の版を固定する**
   - `jdx/mise-action` の 14 箇所が「最新の mise」を入れている。
   - 版の記述を 1 か所に集約する。mise-action が版ファイルを読めればそれを使い、読めなければ `.github/actions/setup-mise` の composite action に寄せる。
   - 版の更新は tools-bump に任せる。
5. **zizmor の範囲を広げる**
   - `mise.toml` の `lint:zizmor` から `--min-severity high` を外し、既定の persona にする。
   - 対象を `.github/` 全体（`dependabot.yml` を含む）にする。
   - 新たに出る指摘は修正するか、理由を書いて `zizmor.yml` で抑制する。
6. **小さな修正**
   - `ci-dockerfile-lint.yml` を `pull_request` で起動するようにする。存在しない `.hadolint.yml` を paths から外す。
   - npm の Dependabot アラート 7 件（markdown-it / braces など。`environment/tools/node/package-lock.json`）を triage する。上位ツールの更新か `overrides` で解消する。
7. **govulncheck を週次で回す**
   - ADR-0010 で受容した「触らないモジュールは検査されない」穴を、今回の脅威の変化に合わせて塞ぐ。
   - `ci-go-module.yml` を 6 モジュールの matrix で呼ぶ、schedule 起動のワークフローを 1 本足す。

### P2: AI エージェントとホストの設定（AP2 / AP4）

1. **このリポの `.claude/settings.local.json`**（未追跡のローカルファイル。コミットはしない）
   - `sandbox.enabled: false` の上書きを削除し、`excludedCommands` に `docker *` を足す。
   - 次の allow を削除する: `Bash(curl:*)`、`Bash(npx:*)`、`Bash(claude *)`、`Bash(docker container *)`、`Bash(docker cp *)`、`Read(//Users/hodanov/**)`。
   - `docker exec *` は `docker exec nvim-dev *` に絞る。
   - 既存の `.claude/skills/permission-review` skill に沿って棚卸しする。scratchpad パスを含む使い捨てルールも消す。
2. **ユーザー設定** `ai-agents/settings/claude/settings.json`（`deploy-ai-config` skill で配布する）
   - `permissions.disableBypassPermissionsMode: "disable"` を追加する。
     - s1ngularity の手口を封じる。
     - ai-bridge 経由で `--dangerously-skip-permissions` が注入されても無効になる。
     - リポ内で bypass を使っている箇所は 0 件（確認済）。
   - `permissions.deny` を拡充する:
     - 追加: `Read(~/.ssh/**)`、`Read(~/.config/gh/**)`、`Read(~/.codex/auth.json)`、`Read(~/.docker/config.json)`、`Read(~/.aws/**)`、`Read(~/.kube/**)`、`Read(**/.git-credentials)`、`Read(**/*.tfstate)`、`Read(**/.envrc)`
     - `Read(~/.codex/**)` の allow は deny が優先されるので、そのまま残してよい。
   - deny ルールで止まらない読み方がある。
     - 公式ドキュメントによると、deny は `cat` などには効くが、`grep -r` や任意のサブプロセスには効かない。
     - そこで、sandbox 下で `grep -r . ~/.ssh` が止まるかを実測する。止まらなければ、同じパスを `sandbox.filesystem.denyRead` にも入れる。
     - 二重に書くことになるので、実測で必要と分かった場合だけにする。
   - `sandbox.allowUnsandboxedCommands: false` にして、sandbox 外での再実行を禁止する。
     - 足りないものは `excludedCommands` に明示的に足す。
     - 1 週間は permission-ledger を見て、詰まりがないか確認する。
   - `enableWeakerNetworkIsolation: true` は gh / terraform 用の設定だが、どちらも `excludedCommands` に入っている。外して困らないかを確かめてから外す。
   - `guard-secret-read.sh` は結線しない。秘密の保護は組み込みの deny と sandbox に寄せる（機械的に判定できるものは組み込みの仕組みに寄せる）。
3. **Codex**（`~/.codex/config.toml`、リポ管理外）
   - `[sandbox_workspace_write]` テーブルの下に誤って入っている `notify` / `model` / `model_reasoning_effort` をトップレベルへ移す。
   - `network_access = true` を false にできるかを、普段の作業（go の取得など）で試す。できなければ現状を維持し、ADR に記録する。
4. **dotfiles とリポの衛生**
   - `dotfiles/.zshrc:129`: 存在せず、かつ書き込み可能なディレクトリ `/usr/local/opt/curl/bin` を PATH の先頭に置いているので削除する。`setopt HIST_IGNORE_SPACE` を足す。
   - `.gitignore` に `.env`、`.env.*`、`*.pem`、`*.key`、`.claude/settings.local.json` を足す。グローバル ignore が無いクラウドや別の clone でも守るため。

### P3: Routine とプロンプトインジェクション（AP3）

方針: 「外部入力はデータとして扱え」とプロンプトでお願いするのではなく、**未信頼のテキストを取得する段階で除外する**。

1. `routines/prompts/weekly-adopted-issue-pr-bot.md`
   - Issue の取得を `gh issue list --label adopted --author hodanov` に限定する。
   - ラベルは triage 権限がないと付けられない。一方、本文は起票者が後から書き換えられる。
2. `routines/prompts/weekly-pr-care-bot.md` と `monthly-routine-improve.md`
   - コメントの取得時に、jq で `select(.user.login == "hodanov")` に絞る。
   - CI ログは自前のワークフローの出力なので、そのまま扱う。
3. **スキャン系 5 本**（WebFetch と Bash が同じセッションにあり、gh トークンは広い権限 = lethal trifecta）
   - Routine の `allowed_tools` が `Bash(gh issue create:*)` のようなパターン指定に対応しているかを確かめる。
   - 対応していれば、Bash を必要なコマンドだけに絞って `/schedule` で反映する。
   - 対応していなければ、ADR に受容として記録する。出力は人の triage（`adopted` ラベル）を必ず通る。
4. クラウド環境（`env_01H6...`）のネットワーク設定を claude.ai で確認する（手作業）。
5. 自動マージへの経路は P1-1（作成者の判定）で断つ。

### P4: コンテナ、ai-bridge、依存（AP2。受容を前提に安い対策だけ入れる）

1. **`environment/docker/docker-compose.yml`**
   - `cap_drop: [ALL]` と `security_opt: [no-new-privileges:true]` を追加する。
   - SECURITY NOTE を書き直す。「コンテナは `~/workspace` に対する境界ではない」と事実を書き、ADR-0011 を参照する。
2. **npm の install script を止める**
   - `environment/docker/nvim.dockerfile:90` の `npm ci` と、`mise.toml` の `tools:node-install` に `--ignore-scripts` を付ける。
   - 現状の対象は core-js と yarn だけ。
3. **Python を lock する**
   - `environment/tools/python` で `uv lock` し、Dockerfile では `uv sync --locked` にする。推移的な依存がハッシュで固定される。
   - Dependabot の pip は uv.lock を追える。
4. **otel-lgtm のイメージ**
   - digest で固定し、`dependabot.yml` の docker の `directories` に `environment/otel` を足す。
5. **ai-bridge**（`scripts/ai-bridge/`）
   - `internal/domain/script.go:13`: プロンプトの前に `--` を入れ、オプション注入を防ぐ。テストを追加する。
   - `~/.ai-bridge` を 0700、`history.jsonl` を 0600 にする（`fsrepo.go:101,203`）。
   - cwd を `$HOME/workspace` 配下に制限する（`process_request.go:46`）。
6. **marp skill の `npx --yes …@latest`**（`ai-agents/skills/marp-deck-export/scripts/lint_deck.sh:21`、`render_deck.sh:41`）
   - 版を固定する。

### P5: 文書化、運用、他リポへの展開

1. **ADR-0011「脅威モデルとセキュリティベースライン」を書く**。中身は §2 の経路、自動マージの方針、受容したリスク。
   - コンテナが境界でないこと
   - regex ガードは best-effort であること
   - ai-bridge
   - スキャン系 Routine
   - Codex のネットワーク
2. **ADR のずれを直す**
   - ADR-0002: peter-evans 前提の記述を、git と gh で直接 PR を作る現状に合わせる。
   - ADR-0006: secret を削除したことを追記する。
3. **侵害時の手順書**（`docs/runbook/supply-chain-incident.md`、短く書く）
   - 使っている上流の侵害が報じられたときの確認先: `mise.lock`、各 lockfile、該当期間の Actions 実行、新しい repo や workflow の有無
   - 失効させる資格情報: gh トークン、App の秘密鍵、Claude / Codex の認証、SSH 鍵
   - 確認する場所: GitHub の security log
4. **ベースラインを測る**
   - 着手前と完了後に、OpenSSF Scorecard の CLI を 1 回ずつ実行する（`mise x aqua:ossf/scorecard -- --repo=github.com/hodanov/my-pde`）。
   - CI には常駐させない。
5. **他リポへ展開する**
   - 完了後、P0 のリポ設定、P1-1 の自動マージ条件、P1-5 の zizmor を `ai-agents/skills/supply-chain-hardening` に Phase として足す。
   - ホスト側の設定（P2-2）は全リポに効くので、展開は不要。

---

## 4. やらないこと（理由）

| やらないこと                                                    | 理由                                                                                                                                   |
| --------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| harden-runner / Scorecard Action を CI に常駐させる             | 全ジョブに触れる第三者 Action が増える。publish 系の secrets を持たない個人リポでは効果が小さい。CI が配布用の secret を持ったら見直す |
| `guard-dangerous-bash` / `guard-secret-read` の正規表現の穴埋め | ブロックリストはいたちごっこになる。境界は sandbox・deny・ask に置く                                                                   |
| main の承認レビューを必須にする                                 | 1 人のリポでは自分の PR を承認できない。代わりに必須チェックと自動マージの条件で担保する                                               |
| コンテナの非 root 化やマウントの縮小                            | 2026-10-22 に見送りと決定。§P4 の安い対策と ADR での受容にとどめる                                                                     |
| Dockerfile の全取得物の SHA256 をリポに固定する                 | tools-bump の大改修になる。14 日の cooldown を入れた後は利得が小さい。後で見直す                                                       |
| gosec                                                           | ADR-0010 の見直し条件（外部入力を受ける Go コードの追加）に当たっていない                                                              |
| コミット署名の必須化                                            | squash merge は GitHub が署名する。直 push は `enforce_admins` で止まる                                                                |

---

## 5. 進め方（PR の分割）

実装に入るときは `dev-workflow` skill を使う。ブランチを切ってから作業する（現状は main 上に未コミットの変更がある）。
まず本プランを `docs/plan/2026-10-22_security-hardening.md` として保存する。

1. **P0**: `gh api` で操作する。PR は不要。1 件ずつ確認してから実行する。
2. **PR-A（P1-1, 2, 6）**: 自動マージの条件、actions-lint を全 PR で起動、Dependabot のグループ、hadolint のトリガー、npm アラート
   - マージ後に必須チェックを追加する。
3. **PR-B（P1-3, 4, 5, 7）**: tools-bump の cooldown、mise の版固定、zizmor の範囲、週次 govulncheck
4. **PR-C（P2-2, P2-4）**: Claude のユーザー設定と dotfiles / .gitignore
   - P2-1 と P2-3 はローカル作業で、コミットしない。
5. **PR-D（P3）**: Routine のプロンプト。`allowed_tools` は `/schedule` で反映する。
6. **PR-E（P4）**: compose、npm、uv、otel、ai-bridge、marp
7. **PR-F（P5）**: ADR-0011、ADR の修正、手順書、supply-chain-hardening skill

---

## 6. 検証

- **P0**: `gh api` で設定を読み直して確認する。Scorecard を着手前と完了後で比べる（Token-Permissions / Branch-Protection / Pinned-Dependencies が改善していること）。
- **P1**
  - `mise run lint` が通ること（actionlint と、既定の persona の zizmor）。
  - tools-bump を `workflow_dispatch` で実行し、選ばれた版がすべて 14 日以上前のリリースであることを確認する。
  - 次の Dependabot PR で確かめる。
    - patch / minor は必須チェックの通過後に自動マージされる。
    - メジャー更新と github-actions は自動マージされずに残る。
    - 通常の PR で `gitleaks` と `actions-lint` が必須チェックとして表示される。
- **P2**: 新しいセッションで次を確認する。
  - `/status` で読み込まれた設定元
  - このリポで sandbox が ON になっていること（allowedDomains にないホストへの curl が止まる）
  - `cat ~/.ssh/id_ed25519` と、sandbox 下の `grep -r . ~/.config/gh` が拒否されること
  - `claude --dangerously-skip-permissions -p hi` が拒否されること
  - `docker exec nvim-dev …` が動くこと
- **P3**: `/schedule` で Adopted-Issue PR Bot と PR Care Bot を手動実行し、取得コマンドに author フィルタが入っていることをログで確認する。
- **P4**
  - `mise run docker:build` が通ること（イメージ内のスモークテストを含む）。
  - `docker compose up -d` の後、コンテナ内で nvim をヘッドレス起動し、LSP と treesitter が動くこと。
  - `mise run ai-bridge:test` が通ること（`--` のテストを含む）。
- **各 PR 共通**: リポのルートで `mise run verify:changed` と `mise run lint` を実行する。
