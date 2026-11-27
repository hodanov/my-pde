---
name: pptx-deck-build
description: >-
  5〜10 分のプレゼン資料を、python-pptx で編集できる PPTX として作る。spec（YAML）に主張・章・見出しを書き、
  骨子を初見の読み手に読ませて筋を確かめてから本文を書き、`check` で構成と文字量を検証して build → PNG で崩れを確認 →
  初見の読み手テスト → Google スライドで人が仕上げる。成果報告・技術共有の storyline を持つ。
  「Google スライドで直せる pptx を作りたい」「発表資料を python-pptx で作りたい」「編集できるスライド」
  「成果発表の資料」などに言及されたときに使用する。
argument-hint: "[題材・伝えたいこと・聞き手 例: 週次レポートの自動化の成果を、チームに 8 分で報告したい。投影用]"
disable-model-invocation: true
metadata:
  version: 1
---

# /pptx-deck-build スキル

## Goal

会話やリポジトリの実状況から、5〜10 分で話せるスライドを**編集できる PPTX**として作る。
正（source of truth）は作る途中は spec（`.yaml`）。人が Google スライドで直した後は、PPTX が正になる（一方向）。

伝わりにくさは、書き終えてから直すのではなく、書く前と書く途中に spec で縛って防ぐ。

- 主張・章・見出し・ノート・見てほしい所の欠落は、`check` が build 前に弾く
- 目次・扉・まとめは章から自動で作る。手書きして章とずれることがない
- 本文を書く前に、見出しの連鎖だけを初見の読み手に読ませる（骨子ゲート）

作るのは 9 割まで。残りの 1 割（細かい見た目の調整）は人が Google スライドで直す前提で、PPTX は図形・表・テキストで作る（画像の貼り付けにしない）。

## Workflow

`SCRIPT` は `skills/pptx-deck-build/scripts/deck.sh`。

1. **文脈を集める**: まず現在の会話から。不足分だけリポジトリ・PR・`docs/`・実データで補う。リポジトリを題材にするなら実ソースを読む。憶測で書かない。数値・固有名詞は実データで裏取りする（古い計画文書の値を転記しない）。
2. **相手・用途・主張・storyline を決める**: `references/story.md` に従う。依頼に伝えたいことが無ければ、主張の候補を 2〜3 個作ってユーザーに聞く。投影用か配布用か（`mode`）、発表時間（`duration_min`）、storyline を確認する。ユーザーへの確認は 1 回の質問にまとめる。
3. **骨子 spec を書く**: `assets/spec-template.yaml` をコピーし、`docs/slides/<slug>/YYYY-MM-DD_<slug>.yaml` に置く（slug は短い kebab-case。画像は同じディレクトリに置く）。この段階では `deck`（主張・聞き手・storyline）と、章ごとの `role`・`title`・`conclusion`、スライドごとの `pattern`・`headline`・`focus` だけを書く。`items` と `notes` は空でよい。
4. **骨子ゲート**: `SCRIPT outline <spec>` で storyline の必須章と順序を確かめる。直したら `SCRIPT outline <spec> --headlines` の出力を、会話を引き継がない新しいサブエージェントに `references/cold-read.md` の A で読ませる。主張とずれたら骨子を直す。**本文はこの後に書く。**
5. **本文を書く**: `items`・`notes` などを書く。型ごとのフィールドは `references/patterns.md`、文の書き方は `references/writing-rules.md`。`SCRIPT check <spec>` のエラーが 0 件になるまで直す。文字があふれたら、`patterns.md` の「文字があふれたとき」の順に直す。
6. **build**: `SCRIPT build <spec>`。PPTX が spec の隣に出る。
7. **PNG で崩れを確認（1 回）**: `SCRIPT render <pptx>`（sandbox 外で実行）。一覧画像 `sheet-*.png` を見て、気になるスライドだけ `slide-NN.png` を開く。直したら再 build し、直したスライドだけ見直す。
8. **最終 cold read（1 回）**: `SCRIPT text <spec>` の出力を、新しいサブエージェントに `cold-read.md` の B で読ませる。指摘を直したら `check` と build をやり直す。
9. **引き渡し**: PPTX を Google ドライブにアップし、Google スライドで開いて仕上げるようユーザーに伝える。人が直した後は PPTX が正になる。spec から再 build すると、その手直しは消える。

## Notes

- **実行環境**: Python は `uv` で動く。依存の版は `scripts/deck.sh` にだけ pin してある。`--exclude-newer` は公開から 14 日以上経った版だけを使う日付で、版を上げるときは、日付と版を一緒に更新して `SCRIPT test` を通す。
- **sandbox**: `uv` のキャッシュは `UV_CACHE_DIR="$TMPDIR/uv"` に向ける。初回だけ `pypi.org` と `files.pythonhosted.org` への通信が要る。キャッシュができたら `UV_OFFLINE=1` で動く。
- **`render` は sandbox 外で実行する**。LibreOffice（`soffice`）は sandbox 内だと何も出力せずに失敗する。要るもの: `brew install --cask libreoffice font-noto-sans-jp`。日本語フォントを見つけるため、Homebrew の fontconfig 設定（`/opt/homebrew/etc/fonts/fonts.conf`）を自動で使う。
- **フォント**: テーマのフォントは Noto Sans JP（Google スライドにある）。人が足したテキストも同じフォントになる。LibreOffice の PNG は目安で、Google スライドの描画とは違う（半角と日本語の間が広く見える、など）。最終確認は人が Google スライドで行う。
- `focus` はスライドに描かれない。書き手の意図の記録と、骨子ゲートの確認用。
- 画像は PNG か JPG。SVG は貼れないので PNG に書き出す。複雑な図は draw.io などで描いて `image` で貼る。
- ルールや上限を変えたら `SCRIPT test` を実行する。描画のテストは `DECKBUILD_RENDER_TEST=1` を付け、sandbox 外で実行する。
- 試用中のため `disable-model-invocation: true`。`marp-deck-export` と同じ題材で比べ、どちらを残すかを決めるまで、この指定を外さない。
- glance test（目の行き先のテスト）は、まだ移していない。証拠のスライドで目の行き先が気になるときは、PNG と新しいサブエージェントで別途確かめる。

## Files

- `assets/spec-template.yaml` — 全パターンを使った見本 spec（コピーして書き換える）
- `assets/storylines.yaml` — storyline と章の役割の定義（唯一の定義）
- `references/story.md` — 主張・聞き手・storyline・章・見出し・ノート
- `references/patterns.md` — スライドの型、図・スクショ、文字があふれたとき
- `references/writing-rules.md` — 1 回で意味が取れる文の 8 項目
- `references/cold-read.md` — 骨子ゲートと最終 cold read のプロンプト
- `scripts/deck.sh` — `outline` / `check` / `text` / `build` / `render` / `limits` / `test`
- `scripts/deckbuild/` — 実装（`template.py` がマスターとテーマ、`patterns.py` が型ごとの検証と描画）
