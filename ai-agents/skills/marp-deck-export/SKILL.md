---
name: marp-deck-export
description: >-
  5〜10 分のプレゼン資料を Marp（Markdown）で作成し、PDF / PPTX / HTML に書き出す。
  会話やリポジトリの実状況からスライドを起こし、gaia テーマの雛形で下書き →
  docs/slides/YYYY-MM-DD_slug.md に保存 → ヘルパースクリプトで描画する。
  「プレゼン資料を作りたい」「スライド」「発表資料」「Marp」「Google スライド用の pptx」
  などに言及されたときに使用する。
metadata:
  version: 1
---

# Marp Deck Export

## Goal

会話やリポジトリの実状況から、5〜10 分で話せるプレゼン資料を Marp Markdown で作り、
PDF / PPTX / HTML に書き出す。**正（source of truth）は `.md`**、描画物は生成物として扱う。

## Workflow

1. **文脈を集める**: まず現在の会話から。不足分だけリポジトリ・PR・`docs/`・実データで補う。
   リポジトリを題材にする発表なら、**実ソース（コード・`docs/plan/`・設定）を読む。憶測で書かない**。
2. **スラッグと構成を決める**: 短い kebab-case スラッグを選ぶ。対象読者・長さが曖昧なら確認する。
   目安は **1 スライド 20〜40 秒** → 5〜10 分で **10〜16 枚**。
3. **下書き**: `assets/marp-template.md` の雛形（gaia / 16:9）をベースにスライドを書く。
4. **保存**: `docs/slides/YYYY-MM-DD_<slug>.md` に置く（ディレクトリが無ければ作る）。
5. **描画**: ヘルパースクリプトで書き出す（下記 Rendering）。
6. **検証**: エラーなく描画され、スライド枚数が想定どおりで、はみ出し・配色崩れが無いこと。

## Content rules

- **数値・固有名詞は実データで裏取りする**。統計は `gh` やファイルから**その場で数え直す**。
  計画文書の古いスナップショット値をそのまま転記しない（実態とズレる）。
- 1 スライド 1 メッセージ。箇条書きは短く。図や ASCII は**フェンスドコードブロック**で置く。
- ユーザーの言語・トーンに合わせる。ソースに無い主張を足さない。

## Styling gotchas

- テーマは `gaia`。アクセント色などは frontmatter の `style:` ブロックで定義する。
- **`code {}` に背景色・文字色を付けたら、`pre` / `pre code` も必ず別に指定する。**
  さもないとインラインコード用の配色がブロックコード（ASCII 図など）にも効いてコントラストが死ぬ。
  `pre` は濃色背景（例 `#161b26`）＋淡色文字（例 `#e8ecf8`）にすると図がくっきりする。

## Rendering

```bash
skills/marp-deck-export/scripts/render_deck.sh <deck.md> [formats]
```

- `formats` は既定 `pdf`。`pdf,pptx,html` のカンマ区切りで複数指定可。
- **`--pdf` と `--pptx` は 1 回の marp 実行で同時指定できない** → スクリプトは形式ごとに実行を分ける。
- PDF / PPTX はヘッドレス Chromium を使う。`The browser is already running` や起動エラーで落ちるのは
  **サンドボックスが Chromium をブロックしているサイン**。サンドボックス外で再実行する（`/sandbox` で調整可）。
  HTML は Chromium 不要。

## Google スライドで使う場合

`pptx` を書き出し → Google ドライブにアップ → 「Google スライドで開く」でインポートする。

## Files

- `assets/marp-template.md` — スライドの骨格 + テーマ（コピーして使う雛形）
- `scripts/render_deck.sh` — pdf / pptx / html への描画ヘルパー
