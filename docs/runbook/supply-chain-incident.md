# Runbook: Supply-chain incident response

使っている上流（Action、npm、mise のツール、Neovim プラグイン、Docker イメージ）の侵害が報じられたとき、または身に覚えのない push、workflow、リポジトリ、OAuth の承認を見つけたときに使う。前提の脅威モデルと受け入れたリスクは [ADR-0011](../adr/0011-set-security-baseline-and-accept-residual-risks.md) にある。

## Stop automation

調べている間に、自動で main に入る経路を止める。

```sh
gh workflow disable "[Automation] Deps Auto Merge"
gh workflow disable "[Automation] Tools Bump"
```

Cloud Routine は claude.ai の Routine 画面で一時停止する。調べ終わったら `gh workflow enable` で戻す。

## Check exposure

侵害された版が、いつ、どこに入ったかを確かめる。版は次の場所に固定されている。

| 依存              | 固定している場所                                                                            |
| ----------------- | ------------------------------------------------------------------------------------------- |
| GitHub Actions    | `.github/workflows/*.yml`、`.github/actions/*/action.yml`                                   |
| mise のツール     | `mise.toml`、`mise.lock`（Docker の `ARG` は `mise run pins:sync` で `mise.toml` から生成） |
| npm               | `environment/tools/node/package-lock.json`                                                  |
| Python            | `environment/tools/python/uv.lock`                                                          |
| Go                | `scripts/*/go.sum`                                                                          |
| Neovim プラグイン | `nvim/config/lazy-lock.json`                                                                |
| otel-lgtm         | `environment/otel/docker-compose.yml`（digest）                                             |
| marp skill の npx | `ai-agents/skills/marp-deck-export/scripts/*.sh`                                            |

```sh
git log -S'<侵害された版や SHA>' --format='%h %ad %s' --date=iso -- <ファイル>
gh run list --created '<侵害期間の開始>..<終了>' --limit 200
```

侵害期間に、その版を使う workflow が走ったか、ホストで `mise install`、`npm ci`、`mise run docker:build` を実行したかを確かめる。走っていれば、その環境にあった資格情報を漏れたものとして扱う。

## Look for traces

- GitHub の security log（<https://github.com/settings/security-log>）で、`repo.create`、`public_key.create`、`oauth_authorization.create`、`personal_access_token` の見覚えの無いイベントを探す。
- `gh repo list hodanov --limit 200 --json name,createdAt` で、見覚えの無いリポジトリが無いか確かめる。Shai-Hulud は盗んだ秘密を新しい公開リポジトリに置いた。
- 承認済みの OAuth App と GitHub App（<https://github.com/settings/applications>）、SSH 鍵（`gh ssh-key list`）に、見覚えの無いものが無いか確かめる。
- ホストで、`~/.claude/settings.json`、`~/.claude/hooks/`、`~/.codex/`、`~/.zshrc`、`~/workspace/*/.git/hooks/` に覚えの無い変更が無いか確かめる。配布元のこのリポジトリと食い違っていれば、`mise run settings-copy` で上書きする前に差分を残す。

## Revoke credentials

漏れた可能性がある資格情報を、影響の大きい順に失効させる。

| 資格情報                      | 置き場所                      | 失効のさせ方                                                                                      |
| ----------------------------- | ----------------------------- | ------------------------------------------------------------------------------------------------- |
| gh CLI のトークン             | キーチェーン                  | <https://github.com/settings/applications> で GitHub CLI の承認を取り消し、`gh auth login` し直す |
| bump 用 GitHub App の秘密鍵   | Secret `BUMP_APP_PRIVATE_KEY` | App の設定で鍵を作り直して Secret を更新し、古い鍵を削除する                                      |
| Claude Code と Routine の認証 | キーチェーン、claude.ai       | `claude` で `/logout` してログインし直す。Routine の GitHub 接続は `/web-setup` で作り直す        |
| Codex の認証                  | `~/.codex/auth.json`          | `codex logout` してログインし直す                                                                 |
| SSH 鍵                        | `~/.ssh/id_*`                 | GitHub と各サーバーの `authorized_keys` から公開鍵を外し、鍵を作り直す                            |
| AWS のアクセスキー            | `~/.aws`                      | IAM でキーを無効にして作り直す                                                                    |
| Docker のレジストリ認証       | `~/.docker/config.json`       | レジストリ側でアクセストークンを取り消し、`docker login` し直す                                   |

## Recover

1. 侵害されていない版に固定し直す。14 日の cooldown を待たずに、手で上げてよい。
2. 開発コンテナを作り直す（`mise run docker:build` の後に `docker compose up -d`）。
3. 止めた workflow と Routine を戻す。
4. 経緯を Issue に残す。判断を変えたなら ADR を足す。
