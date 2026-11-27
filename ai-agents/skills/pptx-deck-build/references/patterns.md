# パターン（スライドの型）

spec の `chapters[].slides[]` に書く型と、各型のフィールド。全部入りの見本は `assets/spec-template.yaml`。
個数・行数の上限は `patterns.py` の各パターンだけが持つ。ここに値を書き写さず、`scripts/deck.sh limits [--mode projection|handout]` で見る。

どのスライドも `pattern`、`headline`、`notes`（投影用は必須）を持つ。証拠の型（`numbers` / `table` / `bar` / `flow` / `image`）は `focus` も必須。

## 証拠の型を選ぶ

| 見せたいこと                 | 型          | フィールド                                    |
| ---------------------------- | ----------- | --------------------------------------------- |
| 数字 1〜3 個                 | `numbers`   | `items: [{value, label, highlight?}]`         |
| 項目の大小・順位・推移       | `bar`       | `items: [{label, value, highlight}]`、`unit?` |
| 仕組み・流れ（5 ノード以内） | `flow`      | `nodes: [{text, highlight?}]`                 |
| 画面の見た目・操作           | `image`     | `path`、`caption?`                            |
| 正確な値を引いて読ませる     | `table`     | `columns`、`rows`、`highlight: {row, column}` |
| 並列の要点                   | `cards`     | `items: [{title, text}]`                      |
| 短い要点の列挙               | `bullets`   | `items: [文字列]`                             |
| 主張を 1 文で大きく          | `statement` | `headline` だけ。折り返したい所に `\n`        |

円グラフ・3D・2 軸のグラフは使わない。量の差が、角度や奥行きや軸の取り方で歪んで見える。

`title`・`agenda`・`divider`・`summary` は自動で生成される型で、spec には書けない（書くと `check` が弾く）。

## 書き方

```yaml
- pattern: numbers
  headline: 作成時間は180分から20分に減った
  focus: 右側の20分
  items:
    - { value: 180分, label: 自動化前の作成時間 }
    - { value: 20分, label: 自動化後の作成時間, highlight: true }
  notes: 直近4週間の平均。
```

- `numbers`: `highlight` は 1 つだけ（数字が 1 個のときは不要）。`value` は短く、単位まで含める
- `bar`: `value` は 0 以上の数値。`highlight` は 1 本だけ。値は棒の端に、名前は左に書かれる。軸・目盛り・凡例は描かない
- `flow`: `highlight` は 1 個以上、全ノード未満。同じ意味のノードだけを強調する（例: 人が行う箱）
- `table`: `highlight: {row: 2, column: 2}` は本文の行と列を 1 から数える。その行に薄い背景が敷かれ、見てほしいセルだけがアクセント色になる。ほかの行の文字は薄くなる
- `cards`: `title` は短く（1 行）、`text` は 1〜2 文
- `image`: PNG か JPG。パスは spec のあるディレクトリからの相対パス。SVG は貼れないので PNG に書き出す
- 文中の改行は `\n`（YAML の二重引用符の中）。半角英数字と日本語の間に空白は入れない

## 図

- 棒グラフと流れ図は、`bar` と `flow` で描く。図形なので、Google スライドで人が直せる
- 構成図など、それ以上に複雑な図は、draw.io などで描いて PNG に書き出し、`image` で貼る
- 色は `scripts/deckbuild/template.py` の色（`ACCENT`、`MUTED` など）だけを使う。アクセント色は見てほしい所だけに使い、ほかはグレー
- 色だけで区別せず、名前は図形の中か隣に書く。凡例は使わない
- 流れは左から右に置く

## スクリーンショット

- 画面全体を貼らない。見てほしい所を含む最小の範囲を切り抜く。大きすぎる画像を縮めて置くと文字が読めなくなるため、縮小率の下限を `check` が確かめる
- スライドと同じ明るさ（ライトテーマ）で撮る。暗い画面は、白いスライドの中で一番目立つ塊になる
- 切り抜きと枠は ImageMagick で付ける。座標は `幅x高さ+左+上`

```bash
magick shot.png -crop 1200x500+40+180 +repage cropped.png
magick cropped.png -fill none -stroke '#5B4CDB' -strokewidth 6 -draw 'roundrectangle 820,20 1180,90 8,8' boxed.png
```

枠の色は `template.py` の `ACCENT.hex` と同じにする。

## 見て分かるかの 5 項目

| 項目       | 問い                                                                      |
| ---------- | ------------------------------------------------------------------------- |
| 型         | 証拠の型が、見せたいことに合っているか                                    |
| 1 か所     | 見てほしい所は 1 か所か。アクセント色はそこだけか。強調は値に付いているか |
| 近さ       | ラベルや数字は、指す対象の隣にあるか                                      |
| 余計なもの | 主張に関係ない色分け・枠・グリッドが残っていないか                        |
| 大きさ     | 画像の中の文字は、表示した大きさで読めるか                                |

例: 採用率 74% → 87% → 73% を表で並べる → 3 本の棒にして、見てほしい 1 本だけをアクセント色にする。

## 文字があふれたとき

`deck.sh check` が上限超過を報告したら、次の順で直す。意味を運ぶ語は削らない（`writing-rules.md`）。

1. 説明・補足・出典・数値の条件を、ノート（`notes`）へ移す
2. スライドを分ける
3. 箇条書きを、証拠の型（`cards`・`numbers`・`bar` など）に置き換える

## 描画で確かめること

`deck.sh render` の PNG で、1 回だけ次を見る。`check` では分からないものだけを確かめる。

- 文字のはみ出し・切れ（特に表・カード・流れ図のノード）
- 見出しや箱の中の、変な位置での折り返し（1 文字だけの行が出ていないか）
- 章ごとの枚数の偏り、目次とまとめの対応

LibreOffice の描画は目安で、Google スライドの描画とは違う。半角英数字と日本語の間が広く見えるのは LibreOffice の仕様で、Google スライドでは出ない。
