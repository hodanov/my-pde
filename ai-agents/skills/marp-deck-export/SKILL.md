---
name: marp-deck-export
description: >-
  5〜10 分のプレゼン資料を Marp（Markdown）で作成し、PDF / PPTX / HTML に書き出す。
  会話やリポジトリの実状況からスライドを起こし、clean テーマの雛形で下書き →
  docs/slides/YYYY-MM-DD_slug.md に保存 → 文字量の lint → 初見の読み手テスト（cold read）→
  PNG で目視確認 → 描画する。
  「プレゼン資料を作りたい」「スライド」「発表資料」「Marp」「Google スライド用の pptx」
  などに言及されたときに使用する。
argument-hint: "[題材と聞き手 例: Claude Code の OTel 分析をチーム向けに 10 分・投影用]"
metadata:
  version: 4
---

# Marp Deck Export

## Goal

会話やリポジトリの実状況から、5〜10 分で話せるプレゼン資料を Marp Markdown で作り、
PDF / PPTX / HTML に書き出す。**正（source of truth）は `.md`**、描画物は生成物として扱う。
文字量は lint で、意味が伝わるかは cold read で、見た目は PNG で確かめてから書き出す。

## Workflow

1. **文脈を集める**: まず現在の会話から。不足分だけリポジトリ・PR・`docs/`・実データで補う。
   リポジトリを題材にする発表なら、**実ソース（コード・`docs/plan/`・設定）を読む。憶測で書かない**。
2. **相手と用途を決める**: 対象読者と、聞き手の現状（スタート）と発表後に期待する状態（ゴール）を決める。
   **投影用か配布用かをユーザーに確認する**（配布用なら frontmatter に `class: handout`）。長さが曖昧なら確認する。
   目安は **1 スライド 20〜40 秒** → 5〜10 分で**本文 10〜14 枚**（表紙・目次・扉・まとめは別）。
   聞き手のペルソナを 1〜3 行で書く。聞き手が知っている語・知らない語も書く（cold read にそのまま渡す）。
3. **骨子を先に作る**: 短い kebab-case スラッグを選び、章立てと各スライドの見出し（結論を言い切る文）だけを箇条書きで出す。
   見出しだけで話の筋が通るか、章ごとの枚数に偏りがないか、各見出しが `references/writing-rules.md` の 6 項目を満たすかを見てから本文を書く。
4. **下書き**: `assets/marp-template.md`（clean テーマ / 16:9）をコピーして書く。
   文字量の上限は `lint/slide-density.markdownlint-cli2.jsonc`、文の書き方は `references/writing-rules.md`、
   それ以外の判断は `references/design-rules.md` に従う。
5. **保存**: `docs/slides/YYYY-MM-DD_<slug>.md` に置く（ディレクトリが無ければ作る）。
6. **lint**: `scripts/lint_deck.sh <deck.md>` が通るまで直す。直し方は design-rules.md の「文字を減らす順序」に従う。
   このステップでは PNG を見ない。
7. **cold read**: `scripts/extract_visible_text.sh <deck.md>` で見える文字だけを取り出し、
   会話を引き継がない新しいサブエージェント（Claude Code なら `general-purpose`。fork は会話を引き継ぐので使わない）に
   `references/cold-read.md` のプロンプトで読ませる。指摘を直したら lint を再実行する。
8. **PNG で目視確認（1 回）**: `scripts/render_deck.sh <deck.md> png` を実行し、一覧画像（`sheet-*.png`）を見る。
   design-rules.md のチェックリストで気になるスライドだけ原寸の PNG を開く。直したら再描画し、直したスライドだけ見直す。
9. **書き出し**: `scripts/render_deck.sh <deck.md> pdf,pptx` など。
   発表者ノートは PPTX のノートと HTML の発表者ビューに入る。

## Content rules

- **数値・固有名詞は実データで裏取りする**。統計は `gh` やファイルから**その場で数え直す**。
  計画文書の古いスナップショット値をそのまま転記しない（実態とズレる）。
- 1 スライド 1 メッセージ。見出しはそのスライドの結論を言い切る文にする。
- ユーザーの言語・トーンに合わせる。ソースに無い主張を足さない。
- **文は聞き手が 1 回読んで意味を取れるように書く**（`references/writing-rules.md`）。
  lint を通すために文字を減らすときも、主体・対象・単位のような意味を運ぶ語は削らない。

## Styling gotchas

- テーマは `clean`（`assets/themes/clean.css`）。色・余白・レイアウトはテーマで決まるので、デッキに `style:` を書かない。
  見た目を変えたいときはテーマを直す（全デッキに効く）。
- テーマの `code {}` に背景色・文字色を付けたら、`pre` / `pre code` も必ず別に指定する。
  さもないとインラインコード用の配色がブロックコードにも効いてコントラストが死ぬ。
- Marp は一部の HTML タグをそのまま解釈する（`<b>` `<a>` `<span>` など）。
  プレースホルダーや本文に `<A>` のような英字の山括弧を書かない。

## Rendering

```bash
skills/marp-deck-export/scripts/lint_deck.sh <deck.md>
skills/marp-deck-export/scripts/extract_visible_text.sh <deck.md>
skills/marp-deck-export/scripts/render_deck.sh <deck.md> [formats]
```

- `formats` は既定 `pdf`。`pdf,pptx,html,png` のカンマ区切りで複数指定可。
- `png` は一時ディレクトリに出力し、そのパスを表示する。ImageMagick があれば 9 枚ずつの一覧画像 `sheet-*.png` も作る。
- テーマはスクリプトが `--theme-set` で読み込む。VS Code の Marp 拡張でプレビューするなら、`markdown.marp.themes` にテーマのパスを足す。
- **`--pdf` と `--pptx` は 1 回の marp 実行で同時指定できない** → スクリプトは形式ごとに実行を分ける。
- PDF / PPTX / PNG はヘッドレス Chromium を使う。`The browser is already running` や起動エラーで落ちるのは
  **サンドボックスが Chromium をブロックしているサイン**。サンドボックス外で再実行する（`/sandbox` で調整可）。
  HTML は Chromium 不要。Chrome が無く Brave がある環境では Brave を使う（`CHROME_PATH` で上書き可）。
- lint は markdownlint-cli2（無ければ `npx`）で動き、Chromium は不要。デッキは標準入力で渡すので、リポジトリ側の markdownlint 設定の影響を受けない。

## Google スライドで使う場合

`pptx` を書き出し → Google ドライブにアップ → 「Google スライドで開く」でインポートする。

## Files

- `assets/marp-template.md` — 構成の見本（コピーして使う雛形）
- `assets/themes/clean.css` — テーマ（色・余白・レイアウト用クラス）
- `references/design-rules.md` — 用途・文字を減らす順序・構成・色・クラス・目視チェックリスト
- `references/writing-rules.md` — 1 回で意味が取れる文の 6 項目・削ってよい語・書き換え例
- `references/cold-read.md` — 初見の読み手テストのプロンプトと結果の読み方
- `lint/slide-density.cjs` — スライドごとの文字量を数える markdownlint のカスタムルール
- `lint/slide-density.markdownlint-cli2.jsonc` — 文字量の上限（投影用 `projection` / 配布用 `handout`）
- `lint/fixtures/` — lint の回帰テスト用デッキ
- `scripts/lint_deck.sh` — 文字量の lint
- `scripts/extract_visible_text.sh` — cold read に渡す、見える文字だけのデッキ
- `scripts/render_deck.sh` — pdf / pptx / html / png への描画ヘルパー
- `scripts/test_lint_deck.sh` — lint の回帰テスト（ルールや上限を変えたら実行する）
