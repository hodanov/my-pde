---
marp: true
theme: gaia
paginate: true
size: 16:9
header: "<HEADER TEXT>"
footer: "<PROJECT / DATE>"
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
  /* Style pre/pre code separately so inline-code colors don't bleed into
     ASCII diagrams / code blocks and kill contrast. */
  pre { background: #161b26; border-radius: 10px; padding: 14px 18px; box-shadow: 0 2px 8px rgba(0,0,0,.15); }
  pre code { background: transparent; color: #e8ecf8; font-size: 19px; line-height: 1.4; }
  .small { font-size: 20px; color: #556; }
  blockquote { border-left: 6px solid var(--accent); padding-left: 16px; color: #444; }
---

<!-- _class: lead -->

# <TITLE><br><SUBTITLE>

### <ONE-LINE TAGLINE>

<br>

**<CONTEXT LINE>**

<span class="small"><DATE></span>

---

## <一言で言うと / TL;DR>

> **<核となる主張を 1〜2 文で>**

- <ポイント 1>
- <ポイント 2>
- <ポイント 3>

---

## <セクション見出し>

<本文。1 スライド 1 メッセージ。>

- <短い箇条書き>
- <短い箇条書き>

---

## <図を置くスライド>

```text
<ASCII 図やフロー。pre のダーク背景でくっきり表示される>
```

**<図の要点を 1 行で>**

---

## <表を置くスライド>

| 列 A | 列 B | 列 C |
| ---- | ---- | ---- |
| ...  | ...  | ...  |

---

<!-- _class: lead -->

# まとめ

**<結論を 1〜2 文で>**

<br>

<span class="small">ご清聴ありがとうございました 🙌</span>
