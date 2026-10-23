# ADR-0011: Set a security baseline and accept residual risks

- Status: Accepted (2026-10-23)

## Context

2025〜2026 年は、開発者の手元と CI を狙うサプライチェーン攻撃が続いている。上流の Action や npm パッケージの乗っ取り（tj-actions、Shai-Hulud）、install script から手元の AI CLI を操る手口（Nx s1ngularity）、乗っ取ったアカウントで workflow を push して secrets を盗む手口（GhostAction）、Issue の文面で CI 上のエージェントを操る手口（Clinejection）が出ている。事例と出典は [`docs/plan/2026-10-22_security-hardening.md`](../plan/2026-10-22_security-hardening.md) にまとめた。

このリポジトリは public で、ホストの `~/.claude` / `~/.codex` / `~/.zshrc` に配る設定の正になっている。main には Dependabot、週次の tools-bump、Cloud Routine の 3 つが自動で変更を入れる。メンテナは 1 人しかいない。そのため、main が汚れるとホストでコードが実行され、ホストの資格情報が盗まれると全リポに書き込めてしまう。

着手前のワークフローの書き方は既に堅かった（`permissions:` の明示、全 `uses:` の SHA 固定、`persist-credentials: false`）。穴はワークフローの外にあった。

- 自動マージに条件が無く、Dependabot のメジャー更新や Action の更新もレビューなしで main に入っていた。
- リポジトリ既定の `GITHUB_TOKEN` が write 権限だった。
- このリポでは Claude Code の sandbox が無効で、秘密ファイルの読み取りも外部送信も止まらなかった。
- Routine は誰が書いた Issue やコメントでも取り込んでいた。
- tools-bump と CI の mise 本体は、上流の最新版を公開当日に取り込んでいた。

## Options considered

| 案                                                                          | 不採用の理由                                                                                                                                                                  |
| --------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 自動マージをやめ、全ての更新 PR を手でマージする                            | [ADR-0002](0002-use-github-app-for-workflow-bot-auth.md) が目指した無人の依存更新を捨てることになる。押さなかった週に更新が止まり、セキュリティ修正の取り込みがかえって遅れる |
| main への承認レビューを必須にする                                           | 1 人のリポでは自分の PR を承認できない。必須チェックと自動マージの条件で代わりに担保する                                                                                      |
| StepSecurity harden-runner や Scorecard Action を CI に常駐させる           | 全ジョブに触れる第三者 Action が増える。CI が配布用の secret を持たない個人リポでは、得るものより攻撃面の増加が大きい                                                         |
| `guard-dangerous-bash` / `guard-secret-read` の正規表現を足して穴を塞ぐ     | ブロックリストは書き方を変えれば抜けられ、いたちごっこになる                                                                                                                  |
| 未信頼のテキストを「データとして扱え」とプロンプトで指示する                | 指示はモデルの判断に依存し、プロンプトインジェクションで覆される。取得する段階で除外するほうが確実                                                                            |
| 開発コンテナを非 root にし、マウントを絞る                                  | 2026-10-22 に見送りと決めた。`~/workspace` 全体を編集する今の使い方が崩れ、コストに見合わない                                                                                 |
| Dockerfile が取得する全ファイルの SHA256 をリポに固定する                   | tools-bump の大改修になる。14 日の cooldown を入れた後は、得られる効果が小さい                                                                                                |
| CI の mise 本体の版を固定し、bump の仕組みで追う                            | 版を上げる経路がもう 1 つ要る。`jdx/mise-action` の `minimum_release_age` で、同じ目的（公開直後の版を避ける）を満たせる                                                      |
| スキャン系 Routine の `allowed_tools` を `Bash(gh issue create:*)` 等に絞る | 2026-10-22 時点のドキュメントに、Routine でのパターン指定の記述が無い。効くかどうか確かめられない設定に頼らない                                                               |

## Decision

次の 5 つを、このリポジトリのセキュリティの基準線とする。

1. **境界は GitHub の設定、OS の sandbox、組み込みの deny ルールに置く。** 正規表現ガードは、うっかりを止める補助にとどめる。
   - リポジトリ: 既定の `GITHUB_TOKEN` は read、Actions による PR の承認は禁止、SHA 固定を強制、main は `enforce_admins`。
   - Claude Code: bypass モードの禁止、秘密ファイルの読み取り禁止（`permissions.deny` と `sandbox.filesystem.denyRead` の二層）、sandbox 外での再実行の禁止。
   - Codex: sandbox の `network_access` を切る。Codex の sandbox はホームの秘密ファイルを読めるため、外部送信の側を断つ。
2. **自動で main に入る経路は、安全と判定できるものだけを通す（fail-closed）。**
   - Dependabot は semver の patch / minor だけを自動マージする。GitHub Actions の更新と、`update-type` を判定できない PR（間接依存のセキュリティ更新、digest だけの更新）は手でマージする。
   - tools-bump は、ブランチ名に加えて作成者が bump 用 App であるときだけ自動マージする。Routine がブランチ名を真似ても自動マージされない。
3. **新しいリリースは 14 日待ってから取り込む。** 値は `mise.toml` の `minimum_release_age` に一度だけ書き、tools-bump、`mise upgrade`、CI の mise 本体が同じ値を使う。Dependabot の `cooldown` も 14 日に揃える。
4. **未信頼のテキストは、取得する段階で除外する。** Routine は、hodanov が書いた Issue とコメントだけを取り込む。
5. **塞がない穴は、理由と緩和策をここに記録して受け入れる。**

| 受け入れるリスク                                                                                                                       | 受け入れる理由                                       | 緩和策                                                                                                                                                        |
| -------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 開発コンテナはホストへの境界ではない。コンテナ内のコードは `~/workspace` の git hooks や配布前の設定を書き換え、ホストで実行させられる | 非 root 化とマウントの縮小は、上の表のとおり見送った | `cap_drop: [ALL]` と `no-new-privileges`、npm の install script の停止、Python の推移依存のロック、依存の 14 日 cooldown                                      |
| ai-bridge により、コンテナからホストの AI CLI を起動できる                                                                             | エディタからホストの AI CLI に頼む機能そのものが目的 | プロンプトを `--` の後に渡してオプション注入を防ぐ。bypass モードはユーザー設定で禁止。`~/.ai-bridge` は 0700                                                 |
| スキャン系 Routine は、WebFetch と Bash と広い権限の gh トークンを同じセッションで持つ                                                 | `allowed_tools` を絞れるか確かめられない             | 出力は Issue で、人が `adopted` ラベルを付けるまで実装されない。Routine の push は作成者の判定で自動マージされず、main は `enforce_admins` で直 push できない |
| Go と rustup は、公開日を取れないので cooldown をかけていない                                                                          | 配布元が公開日を返さない                             | 公式の配布元でチェックサムを検証して取り込む                                                                                                                  |
| secret scanning の汎用パターンと validity checks を使えない                                                                            | 有料の GitHub Secret Protection が要る               | gitleaks を全 PR で回し、push protection を有効にしている                                                                                                     |
| `guard-dangerous-bash` / `guard-secret-read` は書き方次第で抜けられる                                                                  | 上の表のとおり、穴埋めは追わない                     | 境界は sandbox と deny ルールに置いている                                                                                                                     |

## Consequences

判定できない Dependabot の PR と GitHub Actions の更新は、手でマージする必要がある。週に数件の作業が残る。

14 日の cooldown は、セキュリティ修正の取り込みも遅らせる。修正版がある脆弱性は、cooldown を待たずに手で上げてよい（#923、#924）。修正版が無いものは、コードが動く限り open のまま残す。

Claude Code で sandbox 外への再実行を禁止したので、sandbox で動かないコマンドは `excludedCommands` に明示的に足す必要がある。marp skill が npx で取得するパッケージの版は Dependabot の対象外で、手で上げる。

2026-10-23 追記: sandbox 外での再実行の禁止（`allowUnsandboxedCommands: false`）は戻した。作業ディレクトリの外にある worktree への書き込みと、`.git/config` を書く `git worktree add` は sandbox の中では通らず、`excludedCommands` に足しても `git -C <worktree>` のように別の場所を指す呼び出しは sandbox の中で動く。再実行できないと、worktree を使う普段の流れが毎回止まる。外での再実行は 1 件ずつ承認を通るので、境界は承認に置く。あわせて `sandbox.filesystem.denyRead` から `~/.ssh/id_*` と `~/.config/gh/hosts.yml` を外し、`permissions.deny` の `Read(~/.config/gh/hosts.yml)` も外した。秘密鍵は `permissions.deny` の `Read(**/id_rsa*)` などが sandbox にも効くので読めないままで、gh のトークンはキーチェーンにあり `hosts.yml` には無い。

着手前と後で OpenSSF Scorecard（v5.5.0）を測った。コミットを指定して比べられる 9 項目の集計は 5.1 から 6.5 に上がった。差は Token-Permissions（0 → 10）で、自動マージのワークフローの write 権限をジョブ単位に移したことによる。全 18 項目の現状は 6.4 になる。残る低い項目は、1 人のリポであることに起因する Code-Review と Branch-Protection の承認関連、および今回の範囲外の License、Security-Policy、Fuzzing、CII-Best-Practices である。

次の条件で、この ADR を見直す。

- CI が配布用の secret（パッケージの公開トークンなど）を持つようになったとき: harden-runner の導入を再検討する。
- メンテナが 2 人以上になったとき: main の承認レビューを必須にする。
- Routine の `allowed_tools` がパターン指定に対応したとき: スキャン系の Bash を必要なコマンドに絞る。
- actionlint が `$/` 構文に対応したとき: zizmor の `self-repository` を有効に戻す。
- Docker Desktop がバインドマウントの所有者の見せ方を変えたとき: `cap_drop: [ALL]` で書き込めなくなるので、必要な capability だけを戻す。

2026-10-23 追記: Routine 8 本が使うクラウド環境（`routines/*.json` の `environment_id` は全て同じ）のネットワークを確かめた。Limited で、追加の許可ドメインは無く、既定のパッケージマネージャーのリストだけを許可していた。スキャン系 Routine が操られても、任意のドメインへは送れない。

2026-10-23 追記: ホストの gh トークンから `admin:public_key` を外した。盗まれたトークンで攻撃者が自分の SSH 鍵を登録すると、トークンを失効させても push し続けられるため。`gist` は gh CLI がログイン時に必ず要求するので外せず、受け入れる。秘密 gist への流出経路になりうるが、gh CLI を使う限り代わりが無い。

関連: [ADR-0002](0002-use-github-app-for-workflow-bot-auth.md)、[ADR-0005](0005-auto-bump-checksum-less-binaries-via-release-digests.md)、[ADR-0006](0006-run-llm-automation-on-claude-routines.md)、[ADR-0010](0010-scan-go-dependencies-with-dependabot-and-govulncheck.md)
