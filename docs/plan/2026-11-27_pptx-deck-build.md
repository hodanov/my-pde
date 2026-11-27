# Plan: python-pptx で編集できる資料を作る pptx-deck-build スキル

## Background

他の人が作った発表資料が、きれいで分かりやすかった。作り方は、python-pptx で AI が 9 割を作り、残りの 1 割を人が Google スライドで直すというもの。
既存の `marp-deck-export` は、品質（特に文章・構成の伝わりにくさ）と所要時間に不満がある。

目指す結果は、AI が速く 9 割まで作り、人が Google スライドで仕上げられる資料の作り方を、スキルとして持つこと。
python-pptx への移行だけでは伝わりにくさは直らない。移行で得られるのは、編集可能性・所要時間・デザインの一貫性。伝わりにくさには、構成を spec で機械的に縛ることと、本文を書く前の骨子ゲートで対処する。

## Current structure

`ai-agents/skills/marp-deck-export/` は、Markdown を正として Marp で PDF / PPTX / HTML に書き出す。手順は 10 ステップで、サブエージェントによる確認が 2 回（cold read、glance test）あり、描画は形式ごとに Chromium を起動する。

- PPTX は `marp --pptx` の出力で、スライドが画像として貼られるため、Google スライドで文字を直せない
- 構成の担保は、Step 3 の見出し骨子（散文のルール）と最後の cold read だけで、目次とまとめは手書きのため、章と二重管理になっている
- 文字量の lint が厳しい（投影用は箇条書き 3 個、1 行は全角約 28 文字）。一方、文章ルールは意味を運ぶ語を削らないよう求めており、圧縮と明確さの要求が衝突している可能性がある。これは仮説で、比較ベンチで確かめる

## Design policy

`marp-deck-export` は変更せず、新スキル `ai-agents/skills/pptx-deck-build/` を並行して作る。同じ題材で比べてから、統合するかを別途決める。試用中は `disable-model-invocation: true`（明示呼び出し限定）にして、`marp-deck-export` と trigger が重ならないようにする。

入力は YAML spec（主張・聞き手・storyline・章・スライド）で、PPTX は生成物とする。人が Google スライドで編集した後は PPTX が正になる一方向のパイプラインで、再生成すると手直しは消える。

構成は spec で縛る。目次・扉・まとめ・主張スライドは章から自動で作り、手書きさせない。章の役割（背景・目的・やったこと・結果・学び、など）の並びは `assets/storylines.yaml` だけで定義し、必須の章の欠落と順序違反は `check` が弾く。そのほか `check` は、結論を言い切っていない見出し、空のノート、証拠の型で見てほしい所（`focus`）が無いスライド、箱に収まらない文字量を build 前に弾く。上限値は各パターンのクラスだけが持つ。

骨子ゲートは、本文を書く前に、見出しの連鎖と聞き手の persona だけを新しいサブエージェントに読ませ、全体の主張が spec の `claim` に行き着くかを確かめる。最終の cold read は 1 回に絞り、glance test は既定から外す。

テンプレート（マスター・レイアウト・テーマ）は `template.py` が python-pptx の既定テンプレートを書き換えて作り、バイナリはコミットしない。フォントは Noto Sans JP を theme の latin と ea に設定し、実行時のフォント直書きをしない。そうすると、人が Google スライドで足したテキストも同じフォントになる。`bar` と `flow` は図形で描いて人が直せるようにし、複雑な図は PNG を貼る。文字量は、全角 1em・半角 0.55em の幅推定で箱に収まるかを確かめる。

実行は `uv run` で、依存の版と `--exclude-newer`（公開から 14 日以上）を `scripts/deck.sh` にだけ pin する。`mise.toml` は変更しない。pin の場所を 1 か所にするため、描画とテストも `deck.sh` のサブコマンド（`render`、`test`）にする。Python は `>=3.12` を指定する。

## Implementation steps

1. 環境を整える。`brew install --cask libreoffice font-noto-sans-jp`（PPTX を PNG にして崩れを確かめるため）
2. `template.py` でテンプレートを作り、4 レイアウト（Title Slide、Section Header、Title and Content、Title Only）で描画できることを確かめる（go / no-go ゲート）
3. `spec.py`、`storyline.py`、`check.py`、`report.py` で、spec の読み込み、検証、`outline` / `text` を作る
4. `patterns.py` に全パターン（title、agenda、divider、statement、bullets、cards、numbers、table、bar、flow、image、summary）と文字量の推定（`fit.py`）を実装し、`build.py` で書き出す
5. `render.py` で `soffice` → PDF → PNG（PyMuPDF）の描画を作る
6. `unittest` のテストと、見本 spec（`assets/spec-template.yaml`）を作る
7. SKILL.md と references（story、patterns、cold-read、writing-rules）を書く。`writing-rules.md` は `marp-deck-export` からコピーする。スキルは単体で配布されるため、相互参照できない
8. Google スライドで開いて確認し、`marp-deck-export` と同じ題材で比較する

## File changes

- 新規: `ai-agents/skills/pptx-deck-build/SKILL.md`
- 新規: `ai-agents/skills/pptx-deck-build/assets/spec-template.yaml`、`assets/storylines.yaml`
- 新規: `ai-agents/skills/pptx-deck-build/references/story.md`、`patterns.md`、`cold-read.md`、`writing-rules.md`
- 新規: `ai-agents/skills/pptx-deck-build/scripts/deck.sh`
- 新規: `ai-agents/skills/pptx-deck-build/scripts/deckbuild/` の実装とテスト
- 新規: この文書
- 変更なし: `ai-agents/skills/marp-deck-export/`、`mise.toml`、`ai-agents/scripts/verify-changed.sh`、`ai-agents/hooks/lint-changed.sh`

## Risks and mitigations

- **python-pptx でテーマとマスターを書き換える方式が、Google スライドで期待どおりに開くか。** 最大のリスク。Google スライドで開いて、フォントが置換されない、プレースホルダーを編集できる、レイアウト一覧が整理されている、ノートが残る、スライド番号が出る、を確かめる。うまくいかなければ、Google スライドで作ったマスター付きの PPTX を `assets/` に置き、土台を差し替える。パターンはレイアウト名を参照するので影響は小さい
- **LibreOffice の描画が Google スライドの描画と違う。** PNG は目安にとどめ、最終確認は人が行う。半角と日本語の間が広く見えるのは LibreOffice の仕様
- **sandbox の制約。** `soffice` は sandbox 内だと何も出力せずに失敗し、日本語フォントも見つけられない（同梱の fontconfig が存在しない `/usr/local` を探す）。`render` は sandbox 外で実行し、Homebrew の fontconfig 設定を自動で渡す。`uv` のキャッシュは `$TMPDIR` に向ける
- **Python が repo の lint 対象外。** `*.py` は `lint-changed.sh` と `verify-changed.sh` に対応が無い。検証は、スキル自身のテストと使い捨ての ruff で行う
- **伝わりにくさの原因が、素材の集め方にある場合。** spec と骨子ゲートだけでは直らない。比較ベンチで確かめる

## Validation

- `skills/pptx-deck-build/scripts/deck.sh test` が通る。描画のテストは `DECKBUILD_RENDER_TEST=1` を付けて sandbox 外で実行する
- テストの検出力は、実装を一時的に壊す変異（run のフォント直書き、スライド番号の省略）で、狙ったテストだけが失敗することで確かめる
- `ai-agents/scripts/verify-changed.sh` で、`shellcheck`、`shfmt`、`markdownlint-cli2`、`prettier` が通る
- 見本 spec を build して `deck.sh render` で PNG にし、全パターンの崩れを目視する
- Google スライドで開いて、Risks の確認項目を人が見る
- 同じ素材・同じ聞き手で、Marp 版と pptx 版を作り、待ち時間・骨子ゲートの初回通過・人の手直しの箇所と時間・並べて読んだ伝わりやすさを比べる

## Open questions

- Google スライドでの確認結果（上記）
- 比較ベンチの結果次第で、`marp-deck-export` を廃止・統合・併用のどれにするか
- 「密度 lint が伝わりにくさの一因」という仮説が正しいか
- glance test の移植は、必要になってから決める
- Python の linter（ruff など）の導入と、`verify-changed.sh` の `*.py` 対応を、別提案にするか
