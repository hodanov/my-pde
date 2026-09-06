---
paths:
  - "**"
---

# 変更容易性・可読性の設計原則

コードを書く・直すときは、変更容易性と可読性を予防的に確保する。

## 核となる原則

関連する状態と規則は所有者に凝集させ、宣言は最も狭いスコープに置き、事実は一度だけ書く。
ただし、現在の要件にない抽象は足さない。

既存の言語仕様、リポジトリ固有ルール、外部との契約を優先する。
原則を満たすためだけに、変更対象外のコードまでリファクタリングしない。
例はGoで示すが、判断基準は言語を問わず適用する。

## 凝集

- データの不変条件や検証は、そのデータを所有する型へ寄せる。
- 手続きを1つの関数へ並べず、状態と規則を同じ場所へ置く。
- 状態に依存しない処理は、無理にメソッド化せず純粋関数のままにする。
- 具象型へのメソッド追加と、差し替えのためのinterface追加は別々に判断する。

## スコープ最小化

- 一箇所でしか使わない宣言は、言語仕様と所有する振る舞いが許す範囲で利用箇所のローカルに置く。
- 宣言位置から、その値や規則が影響する範囲を読み取れるようにする。
- 再利用が発生してから、必要な最小範囲へ引き上げる。
- メソッドを持つGoの型、通常の名前付き関数、パッケージ境界を表す型は、必要な最小のpackageスコープに置く。

避ける:

```go
const baseDelay = 100 * time.Millisecond

func retryDelay(attempt int) time.Duration {
 return time.Duration(attempt) * baseDelay
}
```

推奨:

```go
func retryDelay(attempt int) time.Duration {
 const baseDelay = 100 * time.Millisecond
 return time.Duration(attempt) * baseDelay
}
```

## コード内の単一の情報源

- 同時に変更される同一の事実を複数箇所へ直接書かない。
- 許容値、表示文言、派生値が同じ事実を表すなら、1つの定義から導出する。
- 定義を追加・変更したとき、別の記述だけが取り残される構造を避ける。
- 偶然同じ値になっているだけで変更理由が異なるものは、無理に共有しない。

避ける:

```go
func validateFormat(format string) error {
 allowedFormats := []string{"json", "text"}
 if !slices.Contains(allowedFormats, format) {
  return errors.New("format must be one of json, text")
 }
 return nil
}
```

推奨:

```go
func validateFormat(format string) error {
 allowedFormats := []string{"json", "text"}
 if !slices.Contains(allowedFormats, format) {
  return fmt.Errorf("format must be one of %s", strings.Join(allowedFormats, ", "))
 }
 return nil
}
```

## YAGNI

- 現在の要件にない差し替え可能性のために interface や間接化を追加しない。
- テストだけを理由に本番コードへ抽象や公開アクセサを追加しない。
- 具象型や小さな関数で十分なら、その形を維持する。
- 将来必要になった時点で、実際の変更軸に沿って抽象化する。
- 複数実装、既存のアーキテクチャ境界、明示された変更軸がある場合は、その要件に必要な最小範囲で抽象化する。

避ける:

```go
type LabelFormatter interface {
 Format(string) string
}

type labelFormatter struct{}

func (labelFormatter) Format(value string) string {
 return strings.TrimSpace(value)
}
```

推奨:

```go
func formatLabel(value string) string {
 return strings.TrimSpace(value)
}
```

## 命名

- 関数・メソッド名は、対象の何を調べるか、または何を変えるかが分かる名前にする。
- `process`、`handle`、`validate` のような広い動詞を使う場合も、対象や保証を名前から特定できるようにする。
- 名前を具体化できない場合は、複数の責務を抱えていないか見直す。
- 名前は呼び出し側が依存できる契約を表し、変更可能な実装手段を漏らさない。

避ける:

```go
func process(lines []string) string {
 return strings.Join(lines, "\n")
}
```

推奨:

```go
func renderSummary(lines []string) string {
 return strings.Join(lines, "\n")
}
```

## 引数列の凝集

- 複数の引数が常に一緒に渡され、呼び出し側ですでに1つの概念として扱われるなら、名前付きの型にまとめる。
- その値に固有の規則は、具象型のメソッドへ寄せる。
- 呼び出しが一度しかない偶然の引数集合や、単一のプリミティブを意味の薄い型で包むだけの変更は避ける。

避ける:

```go
func validateBounds(start, end int) error {
 if start > end {
  return fmt.Errorf("start must not exceed end")
 }
 return nil
}
```

推奨:

```go
type Range struct {
 Start int
 End   int
}

func (r Range) validateBounds() error {
 if r.Start > r.End {
  return fmt.Errorf("start must not exceed end")
 }
 return nil
}
```

## クエリとコマンド

- 値を返すクエリは、返すものや保証する事後条件が伝わる名前にする。
- 状態を変えるコマンドは、命令形の動詞と対象を名前に含める。
- `Fetch` や `Refresh` は、毎回取得・更新すること自体が呼び出し側の依存する契約である場合だけ使う。
- キャッシュ、遅延初期化、リトライなどが実装詳細なら、名前へ露出させない。
- 有効な値を保証し、必要な場合だけ処理する操作には、実装手段ではなく事後条件を表す名前を使う。

次の名前は、異なる契約を表す。例示のinterfaceは抽象化の追加を求めるものではない。

```go
type SnapshotStore interface {
 EnsureSnapshot(ctx context.Context) (Snapshot, error)
 InvalidateSnapshot()
}

type SnapshotGateway interface {
 FetchSnapshot(ctx context.Context) (Snapshot, error)
}
```

`EnsureSnapshot` は取得方法を規定せず有効な値を返す。`FetchSnapshot` は呼び出しごとの外部取得を契約に含む。

## 判断チェック

1. 状態と規則を同じ所有者へ置けないか。
2. 言語仕様と所有する振る舞いを保ったまま、宣言をさらに狭いスコープへ閉じられないか。
3. 同時に変更される同じ事実を、別の定義から導出できないか。
4. 抽象化を必要とする現在の要件または既存のアーキテクチャ境界があるか。
5. 名前だけで対象と操作する性質が分かるか。
6. 引数列は常に一緒に扱われ、固有の規則を持つ1つの値か。
7. 名前は呼び出し側が依存する契約か、変更可能な実装手段か。
8. 適用する原則が、既存の言語・リポジトリルールや外部契約と衝突しないか。
