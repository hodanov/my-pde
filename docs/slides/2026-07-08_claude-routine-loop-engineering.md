---
marp: true
theme: gaia
paginate: true
size: 16:9
header: "Claude Routine で回す自律改善ループ"
footer: "my-pde / 2026-07-08"
style: |
  :root {
    --accent: #7c5cff;
    --ink: #1f2430;
  }
  section {
    font-family: "Hiragino Sans", "Noto Sans JP", sans-serif;
    font-size: 26px;
    color: var(--ink);
  }
  h1, h2 { color: var(--accent); }
  section.lead h1 { font-size: 52px; line-height: 1.25; }
  section.lead { text-align: left; }
  strong { color: var(--accent); }
  table { font-size: 20px; }
  code { background: #f0ecff; color: #5a3ce0; }
  pre { background: #161b26; border-radius: 10px; padding: 14px 18px; box-shadow: 0 2px 8px rgba(0,0,0,.15); }
  pre code { background: transparent; color: #e8ecf8; font-size: 19px; line-height: 1.4; }
  .small { font-size: 20px; color: #556; }
  blockquote { border-left: 6px solid var(--accent); padding-left: 16px; color: #444; }
---

<!-- _class: lead -->

# Claude Routine で回す<br>自律改善ループ

### ― コードを書かない時間も、環境が育つ仕組み ―

<br>

**ループエンジニアリング手法の紹介**
`my-pde`（個人開発環境）での実践

<span class="small">2026-07-08</span>

---

## 一言で言うと

> **cron で起きる Claude が、リポジトリを自分で調べて改善を提案し、
> 人の GO サインで PR まで作る** ―― それを毎日/毎週まわす。

- 手を動かしていない間も、環境の改善が**細く・止まらず**進む
- 人間は「やる/やらない」を判断するだけ（**triage ゲート**）
- ループ自体を改善する**メタループ**まで含めて仕組み化

---

## 課題：個人開発環境は「育て続ける」のが難しい

`my-pde` は 1 つのリポジトリに全部入っている：

- `nvim/` … Neovim 設定
- `scripts/ai-bridge/` … Go 製デーモン
- `environment/` `dotfiles/` `mise.toml` … 環境・ツール
- `ai-agents/` … Skills / hooks / エージェント定義
- `.github/workflows/` … CI

**改善ネタは無限にあるが、手動で棚卸しし続けるのは現実的じゃない。**

---

## アイデア：Claude Routine（クラウド定期エージェント）

- claude.ai の **Routine（CCR）** = cron で隔離クラウドセッションを起動
- ローカル環境に依存せず、**repo を checkout → 調査 → ツール実行**まで自走
- GitHub Actions 版（`claude-code-action`）も試したが…
  - `--max-turns` 超過 / OAuth トークンの期限管理が手間
  - → **Routine に一本化**

> ポイント：単なる API 呼び出しではなく、**Claude Code ランタイムがフル稼働**する。

---

## 全体像：4 段階のループ

```text
 ①スキャン        ②採用判定        ③PR化           ④マージ判断
 ┌─────────┐     ┌─────────┐     ┌─────────┐     ┌─────────┐
 │ Routine │ ──▶ │  人手   │ ──▶ │ PR Bot  │ ──▶ │  人手   │
 │ が Issue│     │ adopted │     │ が draft│     │ merge / │
 │ 起票    │     │/rejected│     │ PR 作成 │     │ close   │
 └─────────┘     └─────────┘     └─────────┘     └─────────┘
      ▲                                                │
      └──────── メタループ（月次）でプロンプトへ還元 ◀─┘
```

**自動化するのは調査と実装。判断は人間が握る。**

---

## パイプライン：曜日で分散させて運用

| 曜日 | Routine                     | 役割                                        |
| ---- | --------------------------- | ------------------------------------------- |
| 毎日 | Daily Neovim Trend Scan     | Neovim 動向を調べ Issue 起票（最大1）       |
| 日   | Adopted-Issue PR Bot        | `adopted` Issue を実装しドラフト PR         |
| 月   | PR Care Bot                 | `auto/*` PR の CI/コンフリクト/レビュー対応 |
| 火   | Scripts Tooling Scan        | `scripts/` 向け提案                         |
| 水   | Environment Scan            | `environment/`・`dotfiles/`・`mise.toml`    |
| 木   | DevX Skills/Hooks Scan      | 汎用 hooks/skills 提案                      |
| 金   | CI Workflows Scan           | `.github/workflows/` 改善                   |
| 土   | Pipeline Digest（LLM 不要） | 滞留の可視化                                |

---

## スキャンの肝：ノイズを出さない工夫

- **1 回 = 最大 1 件**に絞る → ターン・コストを抑える
- **重複排除を二重に**
  - 提案済み（Open `scan:*`）
  - 一度不採用（Closed + `rejected`）
- **Skip ではなく「別角度」へ**
  - 有力候補が既存と被ったら、**被らない別の提案を選び直す**
  - ネタ切れで永遠に起票されない、を防ぐ

> 「毎回ちがう角度で 1 件」を狙う設計。

---

## 人間のゲート：ラベルで意思表示

- 採用 → **`adopted`**（PR Bot が拾う）
- 見送り → **`rejected`** を付けて **Close**
- **rejected の時は理由を一言コメント**で残す

<br>

これが後述の**メタループの学習データ**になる。
判断は軽い操作（ラベル付け）だけ = **triage ゲートは人間が維持**。

---

## PR 化 Bot：adopted → ドラフト PR

各 adopted Issue ごとに：

1. `main` から作業ブランチ（`auto/issue-<N>-<slug>`）
2. 提案を**最小差分**で実装
3. 検証（stylua / `golangci-lint` / `go test` など）
4. 命令形コミット & push
5. `gh pr create --draft`（body に `Closes #N`・何を/なぜ/検証結果）
6. Issue に `pr-created` ラベル（重複 PR 防止）

**すべて draft。最終マージ判断は人間。** `main` 直コミットはしない。

---

## PR Care Bot：作った後の面倒を見る

Open な `auto/*` PR に対して（月曜）：

- CI 失敗の修正
- コンフリクト解消（**merge のみ**、rebase / force-push 禁止）
- レビューコメント対応

<br>

- **人手 commit が後に積まれた PR はスキップ**（人の作業を壊さない）
- ready 化・マージはしない

---

## メタループ：ループ自身を改善する

**Monthly Routine Improve**（毎月2日）

- 直近 1 ヶ月の
  - `rejected` Issue の**不採用理由コメント**
  - 未マージ close の `auto/*` PR
  - レビュー指摘
- を分析し、`routines/prompts/*.md` への**最小差分の改善**を draft PR で提案

> Skills の _observe → improve_ に相当する仕組みを Routine にも。
> **プロンプトが運用実績で育つ。**

---

## 定義管理：repo を source of truth に

- `routines/*.json` … Routine 定義（cron / model / tools）
- `routines/prompts/*.md` … **プロンプト本文**

**プロンプトの間接参照化**がキモ：

- JSON の `prompt` は「checkout した repo の md を読んで従え」という薄いポインタ
- → **プロンプト変更はマージするだけで次回実行から反映**（手動 apply 不要）
- markdownlint / prettier の検証対象にもなる

---

## LLM が要らない所は LLM を使わない

**Pipeline Digest**（土曜・素の GitHub Actions）

- triage 待ち Issue / 滞留 `adopted` / Open な `auto/*` PR を集計
- `gh` + `jq` だけ、`digest` ラベルの**単一 Issue を上書き更新**

<br>

- Issue を増殖させない
- `scan:*` を付けず、スキャンの重複排除を汚さない

> 決定論的な定型処理に LLM は過剰。**適材適所。**

---

## 成果（直近の実績）

- scan Issue の**採用率 約 93%**（クローズ済み 29 件中 adopted 27 / rejected 2）
- `auto/*` PR **25 件中 23 件マージ**
- 週の各曜日に改善タスクが**自動で流れてくる**状態
- **採用＝丸呑みではない**：検討の余地はあるが要ブラッシュアップな提案は、
  Issue を種に**人が再スコープして実装**（例: worktree 案→hook 化 #517 / test-changed の設計見直し #521）
- 無駄打ち（rejected 後に PR 化）は稀で許容範囲

<br>

**「気づいたら環境が良くなっている」**が日常になった。

---

## 学び

- **人間はゲートに徹する**：判断を握れば自動化は怖くない（全部 draft）
- **1 件ずつ・別角度**：量より継続。ノイズを出さない設計が命
- **安全側のデフォルト**：merge のみ / 最小差分 / 人手 commit は触らない
- **仕組みを仕組みで直す**：メタループでプロンプトが育つ
- **適材適所**：定型処理は素の CI へ逃がす

---

<!-- _class: lead -->

# まとめ

**cron × Claude × 人間の triage ゲート**で、
個人開発環境が**自律的に・安全に育ち続ける**ループを作った。

<br>

鍵は「**全部 draft・判断は人間・1件ずつ・repo が正**」。

<br>

<span class="small">ご清聴ありがとうございました 🙌</span>
