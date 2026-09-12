# 昇格候補の審査基準

`permission-audit` の出力を読み、候補を「昇格 / 一般化して昇格 / local のまま / 見送り」に振り分けるための基準。

## 集計値の読み方

| フィールド              | 意味                                                                                                                                    |
| ----------------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| `approved` / `rejected` | hook が記録したプロンプトのうち、transcript 上で実行された件数と、拒否・遮断された件数                                                  |
| `pending`               | 結果行が見つからない件数。セッション進行中か、transcript が `cleanupPeriodDays` で消えた                                                |
| `family_rejections`     | 同じ系統（Bash の先頭コマンド / WebFetch のドメイン / MCP ツール）の呼び出しが transcript 全体で拒否された件数。hook 導入前の拒否も含む |
| `repos`                 | そのルールでプロンプトが出たリポ                                                                                                        |
| `local_repos`           | そのルールを `.claude/settings.local.json` で既に許可しているリポ（「Yes, and don't ask again」の蓄積）                                 |
| `examples`              | 実際のコマンド・URL・パスの例（最大 3 件）                                                                                              |

## 許可判定の前提

- 複合コマンドはサブコマンドごとに判定され、全部が allow に当たって初めて自動承認される。提案もサブコマンド単位で出る。
- `Bash(x:*)` と `Bash(x *)` は同じ。`timeout` / `time` / `nice` / `nohup` / `stdbuf` と、フラグなしの `xargs` は剥がしてから照合される。
- deny と ask は allow より優先する。ask にあるものを allow に昇格させない。
- `sandbox.autoAllowBashIfSandboxed: true` なので、sandbox 内で動く Bash はルール無しで通る。プロンプトが出る Bash は主に
  `sandbox.excludedCommands`（git / gh / go / mise / terraform / tflint）か、sandbox の外で動かす必要があったもの。
- `cd` と出力 redirection が同居する複合コマンドは、組み込みガードで毎回確認になる。allow では消えないので、
  redirect 先を絶対パスにする書き方で避ける。

## 見送る

- 任意コード実行・任意通信・破壊的操作を丸ごと通す prefix。`Bash(curl *)` / `Bash(npx *)` / `Bash(bash *)` /
  `Bash(python3 -c *)` / `Bash(docker run *)` / `Bash(docker exec *)` / `Bash(rm *)` / `Bash(chmod *)` / `Bash(claude *)` など。
  サブコマンドまで絞れば安全なら「一般化して昇格」で拾う（例 `Bash(docker compose ps *)`）。
- 途中に `*` を挟むルール（例 `Bash(terraform -chdir=* show *)`）。`*` は間に差し込まれた任意のオプションまで吸収する。
- scratchpad や一時ディレクトリの絶対パスを含む完全一致ルール、`echo "exit=$?"` のような一回限りの完全一致ルール。二度と一致しない。
- `rejected` か `family_rejections` がある系統。拒否した理由を先にユーザーに確認する。
- deny に同じ系統がある（`Bash(rm -rf *)` に対する `Bash(rm *)` など）。
- MCP の書き込み系ツール（`create_*` / `send_*` / `update_*` / `delete_*`）。リポ数が多くても見送り寄りにする。

## 一般化して昇格する

- 同じサブコマンドの具体ルールが並ぶなら、サブコマンドまでの prefix にまとめる（`Bash(go env GOPATH)` と
  `Bash(go env GOOS)` → `Bash(go env *)`）。読み取り系サブコマンド（`version` / `list` / `show` / `status` / `config get` など）に
  留め、書き込み系サブコマンドまで広げない。
- 同じツールの読み取り系サブコマンドが複数リポの local にあるなら、グローバルへ上げる。

## local のまま

- 特定リポのスクリプト（`./ai-agents/scripts/*` など）、特定リポのパス、1 リポでしか使わないツールチェーン。
- 判断が割れるものは local のままにする。昇格は後からできるが、グローバルに入れた広いルールは全リポに効く。

## 昇格する

- 複数リポで承認・許可されている読み取り専用のコマンド・ドメイン・MCP 読み取りツール（`list_*` / `get_*` / `search_*`）。
- 1 リポでしか出ていなくても、どのリポでも同じ意味を持つ読み取り専用コマンド（`brew list *` など）。
