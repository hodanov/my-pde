from dataclasses import dataclass
from pathlib import Path

from lxml import etree
from PIL import Image as PILImage
from pptx.enum.shapes import MSO_SHAPE
from pptx.enum.text import MSO_ANCHOR, PP_ALIGN
from pptx.oxml.ns import qn
from pptx.util import Inches, Pt

from .drawing import (
    Line,
    add_arrow,
    add_box,
    add_text,
    fill_placeholder,
    number_paragraphs,
    write_lines,
)
from .fit import block_height, display_width, wrapped_lines
from .template import (
    ACCENT,
    BACKGROUND,
    BODY_PT,
    BULLET_INDENT,
    CAPTION_PT,
    CONTENT_BOX,
    CONTENT_LAYOUT,
    COVER_PT,
    DETAIL_PT,
    INK,
    MARGIN_X,
    MUTED,
    NUMBER_PT,
    RULE,
    SECTION_LAYOUT,
    SLIDE_WIDTH,
    STATEMENT_PT,
    SURFACE,
    TINT,
    TITLE_BOX,
    TITLE_LAYOUT,
    TITLE_ONLY_LAYOUT,
    TITLE_PT,
    Box,
)

HANDOUT_SCALE = 0.8
HEADLINE_MIN_WIDTH = 14
HEADLINE_MAX_LINES = 2
PX_EMU = SLIDE_WIDTH // 1280
NO_STYLE_TABLE_ID = "{2D5ABB26-0587-4C30-8999-92F81FD0307C}"


@dataclass(frozen=True)
class Context:
    mode: str
    base_dir: Path

    def pt(self, size):
        return size * HANDOUT_SCALE if self.mode == "handout" else size


def _filled(value):
    return value is not None and str(value).strip() != ""


def _mapping_entries(slide, key, fields):
    raw = slide.data.get(key)
    if not isinstance(raw, list) or not raw:
        return [], [f"{key} は 1 件以上のリストにする"]
    problems = []
    valid = []
    for number, entry in enumerate(raw, start=1):
        if not isinstance(entry, dict):
            problems.append(f"{key}[{number}] はマッピングにする")
            continue
        missing = [field for field in fields if not _filled(entry.get(field))]
        if missing:
            problems.append(f"{key}[{number}] に {', '.join(missing)} が無い")
            continue
        valid.append(entry)
    return valid, problems


def _count_problems(count, limit, noun):
    problems = []
    if count < limit["min_items"]:
        problems.append(f"{noun}が {count} 個 (下限 {limit['min_items']})")
    if count > limit["max_items"]:
        problems.append(f"{noun}が {count} 個 (上限 {limit['max_items']})")
    return problems


def _highlight_problems(entries, noun, allow_single=False):
    count = sum(1 for entry in entries if entry.get("highlight") is True)
    if allow_single and len(entries) == 1:
        return []
    if count != 1:
        return [f"{noun}の highlight は 1 つだけにする (今は {count} 個)"]
    return []


class Pattern:
    name = ""
    layout = TITLE_ONLY_LAYOUT
    authorable = True
    needs_focus = False
    numbered = True
    headline_pt = TITLE_PT
    limits = {"projection": {}, "handout": {}}

    def title_override_pt(self, ctx):
        return ctx.pt(TITLE_PT) if ctx.mode == "handout" else None

    def check(self, slide, ctx):
        problems = [] if slide.generated else self._check_authored(slide, ctx)
        problems.extend(self._check_headline_fit(slide, ctx))
        problems.extend(self.check_body(slide, ctx))
        return problems

    def check_body(self, slide, ctx):
        return []

    def body_text(self, slide):
        return []

    def visible_text(self, slide):
        return [slide.headline, *self.body_text(slide)]

    def build(self, builder, slide):
        slide_obj = builder.new_slide(self, slide)
        self.draw(builder, slide_obj, slide)

    def draw(self, builder, slide_obj, slide):
        return None

    def _check_authored(self, slide, ctx):
        problems = []
        if not slide.headline:
            problems.append("headline が無い")
        elif display_width(slide.headline) < HEADLINE_MIN_WIDTH:
            problems.append(
                f"headline が短い ({display_width(slide.headline)} < {HEADLINE_MIN_WIDTH}): "
                "ラベルではなく、そのスライドの結論を言い切る文にする"
            )
        if ctx.mode == "projection" and not slide.notes:
            problems.append("notes が空 (説明・補足・出典はノートに書く)")
        if self.needs_focus and not slide.focus:
            problems.append("focus が無い (見てほしい所を 1 か所書く)")
        return problems

    def _check_headline_fit(self, slide, ctx):
        if not slide.headline:
            return []
        size = self.title_override_pt(ctx) or self.headline_pt
        lines = wrapped_lines(slide.headline, size, TITLE_BOX.width)
        if lines > HEADLINE_MAX_LINES:
            return [f"headline が {lines} 行 (上限 {HEADLINE_MAX_LINES})"]
        return []


class TitlePattern(Pattern):
    name = "title"
    layout = TITLE_LAYOUT
    authorable = False
    numbered = False
    headline_pt = COVER_PT

    def title_override_pt(self, ctx):
        return None

    def body_text(self, slide):
        meta = " / ".join(part for part in (slide.data["date"], slide.data["author"]) if part)
        return [text for text in (slide.data["subtitle"], meta) if text]

    def draw(self, builder, slide_obj, slide):
        subtitle = slide_obj.placeholders[1]
        lines = self.body_text(slide)
        if lines:
            fill_placeholder(subtitle, lines)
        else:
            subtitle._element.getparent().remove(subtitle._element)


class DividerPattern(Pattern):
    name = "divider"
    layout = SECTION_LAYOUT
    authorable = False
    headline_pt = COVER_PT

    def title_override_pt(self, ctx):
        return None

    def body_text(self, slide):
        return [f"{slide.data['number']:02d}"]

    def draw(self, builder, slide_obj, slide):
        fill_placeholder(slide_obj.placeholders[1], self.body_text(slide))


class StatementPattern(Pattern):
    name = "statement"
    headline_pt = STATEMENT_PT
    needs_focus = False

    def title_override_pt(self, ctx):
        return ctx.pt(STATEMENT_PT)

    def draw(self, builder, slide_obj, slide):
        title = slide_obj.shapes.title
        title.left, title.top, title.width, title.height = Box(
            MARGIN_X, Inches(2.0), TITLE_BOX.width, Inches(3.5)
        )
        title.text_frame.vertical_anchor = MSO_ANCHOR.MIDDLE
        title.text_frame.paragraphs[0].alignment = PP_ALIGN.CENTER


class ListPattern(Pattern):
    layout = CONTENT_LAYOUT
    numbered_list = False

    def check_body(self, slide, ctx):
        items = slide.data.get("items")
        if not (isinstance(items, list) and items and all(isinstance(item, str) and item.strip() for item in items)):
            return ["items は空でない文字列のリスト (1 件以上) にする"]
        limit = self.limits[ctx.mode]
        problems = _count_problems(len(items), limit, "項目")
        size = ctx.pt(BODY_PT)
        width = CONTENT_BOX.width - BULLET_INDENT
        total_lines = 0
        for number, item in enumerate(items, start=1):
            lines = wrapped_lines(item, size, width)
            total_lines += lines
            if lines > limit["max_lines"]:
                problems.append(f"items[{number}] が {lines} 行 (上限 {limit['max_lines']})")
        if block_height(total_lines, len(items), size) > CONTENT_BOX.height:
            problems.append("項目全体が本文の箱に収まらない")
        return problems

    def body_text(self, slide):
        return list(slide.data["items"])

    def draw(self, builder, slide_obj, slide):
        placeholder = slide_obj.placeholders[1]
        size = builder.ctx.pt(BODY_PT) if builder.ctx.mode == "handout" else None
        fill_placeholder(placeholder, slide.data["items"], size)
        if self.numbered_list:
            number_paragraphs(placeholder)


class BulletsPattern(ListPattern):
    name = "bullets"
    limits = {
        "projection": {"min_items": 1, "max_items": 3, "max_lines": 2},
        "handout": {"min_items": 1, "max_items": 5, "max_lines": 3},
    }


class AgendaPattern(ListPattern):
    name = "agenda"
    authorable = False
    numbered_list = True
    limits = {
        "projection": {"min_items": 1, "max_items": 6, "max_lines": 1},
        "handout": {"min_items": 1, "max_items": 8, "max_lines": 1},
    }


class SummaryPattern(ListPattern):
    name = "summary"
    authorable = False
    numbered_list = True
    limits = {
        "projection": {"min_items": 1, "max_items": 6, "max_lines": 2},
        "handout": {"min_items": 1, "max_items": 8, "max_lines": 2},
    }


class CardsPattern(Pattern):
    name = "cards"
    limits = {
        "projection": {"min_items": 2, "max_items": 3, "max_title_lines": 1, "max_text_lines": 4},
        "handout": {"min_items": 2, "max_items": 4, "max_title_lines": 1, "max_text_lines": 5},
    }
    gap = Inches(0.25)
    min_height = Inches(1.8)
    inset = Inches(0.3)
    title_gap_pt = 8

    def check_body(self, slide, ctx):
        entries, problems = _mapping_entries(slide, "items", ("title", "text"))
        limit = self.limits[ctx.mode]
        problems.extend(_count_problems(len(slide.data.get("items") or []), limit, "カード"))
        if not entries:
            return problems
        inner = self._card_width(len(entries)) - 2 * self.inset
        for number, entry in enumerate(entries, start=1):
            title_lines = wrapped_lines(str(entry["title"]), ctx.pt(BODY_PT), inner)
            text_lines = wrapped_lines(str(entry["text"]), ctx.pt(DETAIL_PT), inner)
            if title_lines > limit["max_title_lines"]:
                problems.append(f"items[{number}].title が {title_lines} 行 (上限 {limit['max_title_lines']})")
            if text_lines > limit["max_text_lines"]:
                problems.append(f"items[{number}].text が {text_lines} 行 (上限 {limit['max_text_lines']})")
        if not problems and self._card_height(entries, ctx) > CONTENT_BOX.height:
            problems.append("カードの文字が本文の箱に収まらない")
        return problems

    def body_text(self, slide):
        return [f"{entry['title']}: {entry['text']}" for entry in slide.data["items"]]

    def draw(self, builder, slide_obj, slide):
        ctx = builder.ctx
        entries = slide.data["items"]
        width = self._card_width(len(entries))
        height = self._card_height(entries, ctx)
        top = CONTENT_BOX.top + (CONTENT_BOX.height - height) // 2
        for index, entry in enumerate(entries):
            card = add_box(
                slide_obj,
                Box(CONTENT_BOX.left + index * (width + self.gap), top, width, height),
                SURFACE,
                MSO_SHAPE.ROUNDED_RECTANGLE,
                corner=0.06,
            )
            write_lines(
                card.text_frame,
                [
                    Line(str(entry["title"]), ctx.pt(BODY_PT), ACCENT, bold=True, space_after_pt=self.title_gap_pt),
                    Line(str(entry["text"]), ctx.pt(DETAIL_PT), INK),
                ],
                MSO_ANCHOR.TOP,
                inset=self.inset,
            )

    def _card_width(self, count):
        return (CONTENT_BOX.width - self.gap * (count - 1)) // count

    def _card_height(self, entries, ctx):
        inner = self._card_width(len(entries)) - 2 * self.inset
        title_size, text_size = ctx.pt(BODY_PT), ctx.pt(DETAIL_PT)
        content = max(
            block_height(wrapped_lines(str(entry["title"]), title_size, inner), 0, title_size)
            + Pt(self.title_gap_pt)
            + block_height(wrapped_lines(str(entry["text"]), text_size, inner), 0, text_size)
            for entry in entries
        )
        return max(self.min_height, content + 2 * self.inset)


class NumbersPattern(Pattern):
    name = "numbers"
    needs_focus = True
    limits = {
        "projection": {"min_items": 1, "max_items": 3, "max_label_lines": 2},
        "handout": {"min_items": 1, "max_items": 4, "max_label_lines": 3},
    }
    gap = Inches(0.6)
    top = Inches(0.8)
    height = Inches(2.8)

    def check_body(self, slide, ctx):
        entries, problems = _mapping_entries(slide, "items", ("value", "label"))
        limit = self.limits[ctx.mode]
        problems.extend(_count_problems(len(slide.data.get("items") or []), limit, "数字"))
        if not entries:
            return problems
        problems.extend(_highlight_problems(entries, "数字", allow_single=True))
        width = self._column_width(len(entries))
        for number, entry in enumerate(entries, start=1):
            if wrapped_lines(str(entry["value"]), ctx.pt(NUMBER_PT), width) > 1:
                problems.append(f"items[{number}].value '{entry['value']}' が列の幅に収まらない")
            label_lines = wrapped_lines(str(entry["label"]), ctx.pt(DETAIL_PT), width)
            if label_lines > limit["max_label_lines"]:
                problems.append(f"items[{number}].label が {label_lines} 行 (上限 {limit['max_label_lines']})")
        return problems

    def body_text(self, slide):
        return [f"{entry['value']} {entry['label']}" for entry in slide.data["items"]]

    def draw(self, builder, slide_obj, slide):
        ctx = builder.ctx
        entries = slide.data["items"]
        width = self._column_width(len(entries))
        for index, entry in enumerate(entries):
            highlighted = entry.get("highlight") is True or len(entries) == 1
            add_text(
                slide_obj,
                Box(CONTENT_BOX.left + index * (width + self.gap), CONTENT_BOX.top + self.top, width, self.height),
                [
                    Line(str(entry["value"]), ctx.pt(NUMBER_PT), ACCENT if highlighted else INK, bold=True, space_after_pt=6),
                    Line(str(entry["label"]), ctx.pt(DETAIL_PT), MUTED),
                ],
            )

    def _column_width(self, count):
        return (CONTENT_BOX.width - self.gap * (count - 1)) // count


class TablePattern(Pattern):
    name = "table"
    needs_focus = True
    limits = {
        "projection": {"min_items": 1, "max_items": 4, "max_columns": 3, "max_cell_lines": 2},
        "handout": {"min_items": 1, "max_items": 6, "max_columns": 4, "max_cell_lines": 2},
    }
    margin_left = Inches(0.12)
    margin_right = Inches(0.2)
    margin_y = Inches(0.1)
    min_row_height = Inches(0.55)

    def check_body(self, slide, ctx):
        columns = slide.data.get("columns")
        rows = slide.data.get("rows")
        highlight = slide.data.get("highlight")
        if not (isinstance(columns, list) and columns and all(_filled(column) for column in columns)):
            return ["columns は空でない文字列のリストにする"]
        if not (isinstance(rows, list) and rows):
            return ["rows は 1 行以上のリストにする"]
        problems = []
        limit = self.limits[ctx.mode]
        if len(columns) > limit["max_columns"]:
            problems.append(f"列が {len(columns)} 個 (上限 {limit['max_columns']})")
        problems.extend(_count_problems(len(rows), limit, "行"))
        for number, row in enumerate(rows, start=1):
            if not (isinstance(row, list) and len(row) == len(columns) and all(_filled(cell) for cell in row)):
                problems.append(f"rows[{number}] は columns と同じ {len(columns)} 個の文字列にする")
        if problems:
            return problems
        if not (
            isinstance(highlight, dict)
            and isinstance(highlight.get("row"), int)
            and isinstance(highlight.get("column"), int)
            and 1 <= highlight["row"] <= len(rows)
            and 1 <= highlight["column"] <= len(columns)
        ):
            return [f"highlight は {{row: 1〜{len(rows)}, column: 1〜{len(columns)}}} の 1 セルにする"]
        inner = self._column_width(len(columns)) - self.margin_left - self.margin_right
        for number, row in enumerate([columns, *rows]):
            for cell in row:
                lines = wrapped_lines(str(cell), ctx.pt(DETAIL_PT), inner)
                if lines > limit["max_cell_lines"]:
                    where = "columns" if number == 0 else f"rows[{number}]"
                    problems.append(f"{where} の '{cell}' が {lines} 行 (上限 {limit['max_cell_lines']})")
        return problems

    def body_text(self, slide):
        return [" | ".join(str(cell) for cell in row) for row in [slide.data["columns"], *slide.data["rows"]]]

    def draw(self, builder, slide_obj, slide):
        ctx = builder.ctx
        grid = [slide.data["columns"], *slide.data["rows"]]
        highlight = slide.data["highlight"]
        size = ctx.pt(DETAIL_PT)
        column_width = self._column_width(len(grid[0]))
        inner = column_width - self.margin_left - self.margin_right
        heights = [self._row_height(row, size, inner) for row in grid]
        top = CONTENT_BOX.top + max(0, (CONTENT_BOX.height - sum(heights)) // 2)
        frame = slide_obj.shapes.add_table(
            len(grid), len(grid[0]), CONTENT_BOX.left, top, column_width * len(grid[0]), sum(heights)
        )
        table = frame.table
        table.horz_banding = False
        table._tbl.tblPr.find(qn("a:tableStyleId")).text = NO_STYLE_TABLE_ID
        for column in table.columns:
            column.width = column_width
        for row_number, row in enumerate(grid):
            table.rows[row_number].height = heights[row_number]
            for column_number, text in enumerate(row):
                self._style_cell(
                    table.cell(row_number, column_number),
                    str(text),
                    size,
                    header=row_number == 0,
                    row_highlighted=row_number == highlight["row"],
                    cell_highlighted=row_number == highlight["row"] and column_number + 1 == highlight["column"],
                )

    def _style_cell(self, cell, text, size, header, row_highlighted, cell_highlighted):
        cell.margin_left, cell.margin_right = self.margin_left, self.margin_right
        cell.margin_top = cell.margin_bottom = self.margin_y
        cell.vertical_anchor = MSO_ANCHOR.MIDDLE
        self._set_bottom_border(cell, INK if header else RULE, 2 if header else 1)
        if row_highlighted:
            cell.fill.solid()
            cell.fill.fore_color.theme_color = TINT.theme
        else:
            cell.fill.background()
        if cell_highlighted:
            line = Line(text, size, ACCENT, bold=True)
        elif header:
            line = Line(text, size, MUTED, bold=True)
        else:
            line = Line(text, size, INK if row_highlighted else MUTED)
        write_lines(cell.text_frame, [line], MSO_ANCHOR.MIDDLE)

    def _set_bottom_border(self, cell, color, width_pt):
        border = etree.SubElement(
            cell._tc.get_or_add_tcPr(), qn("a:lnB"), w=str(Pt(width_pt)), cap="flat", cmpd="sng", algn="ctr"
        )
        fill = etree.SubElement(border, qn("a:solidFill"))
        etree.SubElement(fill, qn("a:schemeClr"), val=color.scheme)
        etree.SubElement(border, qn("a:prstDash"), val="solid")

    def _row_height(self, row, size, inner):
        lines = max(wrapped_lines(str(cell), size, inner) for cell in row)
        return max(self.min_row_height, block_height(lines, 0, size) + 2 * self.margin_y)

    def _column_width(self, count):
        return CONTENT_BOX.width // count


class BarPattern(Pattern):
    name = "bar"
    needs_focus = True
    limits = {
        "projection": {"min_items": 2, "max_items": 6},
        "handout": {"min_items": 2, "max_items": 8},
    }
    label_width = Inches(3.2)
    max_bar_width = Inches(6.0)
    value_gap = Inches(0.15)
    value_width = Inches(1.6)
    max_pitch = Inches(0.65)

    def check_body(self, slide, ctx):
        entries, problems = _mapping_entries(slide, "items", ("label", "value"))
        problems.extend(_count_problems(len(slide.data.get("items") or []), self.limits[ctx.mode], "棒"))
        if not entries:
            return problems
        for number, entry in enumerate(entries, start=1):
            value = entry["value"]
            if isinstance(value, bool) or not isinstance(value, (int, float)) or value < 0:
                problems.append(f"items[{number}].value は 0 以上の数値にする")
            elif wrapped_lines(str(entry["label"]), ctx.pt(DETAIL_PT), self.label_width) > 1:
                problems.append(f"items[{number}].label '{entry['label']}' が 1 行に収まらない")
        if not problems and max(entry["value"] for entry in entries) <= 0:
            problems.append("value がすべて 0 (棒の長さを決められない)")
        problems.extend(_highlight_problems(entries, "棒"))
        return problems

    def body_text(self, slide):
        unit = slide.data.get("unit", "")
        return [f"{entry['label']} {entry['value']:g}{unit}" for entry in slide.data["items"]]

    def draw(self, builder, slide_obj, slide):
        ctx = builder.ctx
        entries = slide.data["items"]
        unit = str(slide.data.get("unit", ""))
        largest = max(entry["value"] for entry in entries)
        pitch = min(self.max_pitch, CONTENT_BOX.height // len(entries))
        top = CONTENT_BOX.top + (CONTENT_BOX.height - pitch * len(entries)) // 2
        bar_height = round(pitch * 0.6)
        bar_left = CONTENT_BOX.left + self.label_width
        for index, entry in enumerate(entries):
            highlighted = entry.get("highlight") is True
            row_top = top + index * pitch
            add_text(
                slide_obj,
                Box(CONTENT_BOX.left, row_top, self.label_width, pitch),
                [Line(str(entry["label"]), ctx.pt(DETAIL_PT), ACCENT if highlighted else INK, bold=highlighted)],
                MSO_ANCHOR.MIDDLE,
            )
            bar_width = round(self.max_bar_width * entry["value"] / largest)
            if bar_width > 0:
                add_box(
                    slide_obj,
                    Box(bar_left, row_top + (pitch - bar_height) // 2, bar_width, bar_height),
                    ACCENT if highlighted else MUTED,
                    brightness=0.0 if highlighted else 0.55,
                )
            add_text(
                slide_obj,
                Box(bar_left + bar_width + self.value_gap, row_top, self.value_width, pitch),
                [Line(f"{entry['value']:g}{unit}", ctx.pt(DETAIL_PT), ACCENT if highlighted else MUTED, bold=highlighted)],
                MSO_ANCHOR.MIDDLE,
            )


class FlowPattern(Pattern):
    name = "flow"
    needs_focus = True
    limits = {
        "projection": {"min_items": 2, "max_items": 5, "max_lines": 3},
        "handout": {"min_items": 2, "max_items": 6, "max_lines": 3},
    }
    gap = Inches(0.5)
    height = Inches(1.6)
    inset = Inches(0.15)

    def check_body(self, slide, ctx):
        entries, problems = _mapping_entries(slide, "nodes", ("text",))
        limit = self.limits[ctx.mode]
        problems.extend(_count_problems(len(slide.data.get("nodes") or []), limit, "ノード"))
        if not entries:
            return problems
        highlights = sum(1 for entry in entries if entry.get("highlight") is True)
        if not 1 <= highlights < len(entries):
            problems.append(f"highlight は 1 個以上、全ノード未満にする (今は {highlights} 個)")
        inner = self._node_width(len(entries)) - 2 * self.inset
        for number, entry in enumerate(entries, start=1):
            lines = wrapped_lines(str(entry["text"]), ctx.pt(DETAIL_PT), inner)
            if lines > limit["max_lines"]:
                problems.append(f"nodes[{number}] が {lines} 行 (上限 {limit['max_lines']})")
        return problems

    def body_text(self, slide):
        return [" → ".join(str(entry["text"]) for entry in slide.data["nodes"])]

    def draw(self, builder, slide_obj, slide):
        ctx = builder.ctx
        entries = slide.data["nodes"]
        width = self._node_width(len(entries))
        top = CONTENT_BOX.top + (CONTENT_BOX.height - self.height) // 2
        previous = None
        for index, entry in enumerate(entries):
            highlighted = entry.get("highlight") is True
            node = add_box(
                slide_obj,
                Box(CONTENT_BOX.left + index * (width + self.gap), top, width, self.height),
                ACCENT if highlighted else SURFACE,
                MSO_SHAPE.ROUNDED_RECTANGLE,
                corner=0.12,
            )
            write_lines(
                node.text_frame,
                [Line(str(entry["text"]), ctx.pt(DETAIL_PT), BACKGROUND if highlighted else INK, align=PP_ALIGN.CENTER)],
                MSO_ANCHOR.MIDDLE,
                inset=self.inset,
            )
            if previous is not None:
                add_arrow(slide_obj, previous, node)
            previous = node

    def _node_width(self, count):
        return (CONTENT_BOX.width - self.gap * (count - 1)) // count


class ImagePattern(Pattern):
    name = "image"
    needs_focus = True
    limits = {"projection": {"min_scale": 0.6}, "handout": {"min_scale": 0.45}}
    supported_suffixes = (".png", ".jpg", ".jpeg")
    caption_height = Inches(0.45)

    def check_body(self, slide, ctx):
        raw_path = slide.data.get("path")
        if not _filled(raw_path):
            return ["path が無い (spec からの相対パス)"]
        path = ctx.base_dir / str(raw_path)
        if path.suffix.lower() not in self.supported_suffixes:
            return [f"path '{raw_path}' は {', '.join(self.supported_suffixes)} にする (SVG は PNG に書き出して貼る)"]
        if not path.is_file():
            return [f"画像が見つからない: {raw_path} (spec からの相対パス)"]
        with PILImage.open(path) as image:
            size = image.size
        scale = self._placement(size, bool(_filled(slide.data.get("caption"))))[1]
        minimum = self.limits[ctx.mode]["min_scale"]
        if scale < minimum:
            return [f"画像を {scale:.2f} 倍に縮めて置くことになる (下限 {minimum})。必要な範囲を切り抜く"]
        return []

    def body_text(self, slide):
        caption = str(slide.data.get("caption") or "").strip()
        return ["[画像]", caption] if caption else ["[画像]"]

    def draw(self, builder, slide_obj, slide):
        path = builder.ctx.base_dir / str(slide.data["path"])
        caption = str(slide.data.get("caption") or "").strip()
        with PILImage.open(path) as image:
            size = image.size
        box = self._placement(size, bool(caption))[0]
        picture = slide_obj.shapes.add_picture(str(path), *box)
        picture._element.nvPicPr.cNvPr.set("descr", caption or slide.headline)
        if caption:
            add_text(
                slide_obj,
                Box(CONTENT_BOX.left, box.top + box.height + Inches(0.1), CONTENT_BOX.width, self.caption_height),
                [Line(caption, CAPTION_PT, MUTED, align=PP_ALIGN.CENTER)],
            )

    def _placement(self, size, has_caption):
        width_px, height_px = size
        reserved = self.caption_height + Inches(0.1) if has_caption else 0
        area_height = CONTENT_BOX.height - reserved
        emu_per_px = min(CONTENT_BOX.width / width_px, area_height / height_px, PX_EMU)
        width, height = round(width_px * emu_per_px), round(height_px * emu_per_px)
        box = Box(
            CONTENT_BOX.left + (CONTENT_BOX.width - width) // 2,
            CONTENT_BOX.top + (area_height - height) // 2,
            width,
            height,
        )
        return box, emu_per_px / PX_EMU


PATTERNS = {
    pattern.name: pattern
    for pattern in (
        TitlePattern(),
        AgendaPattern(),
        DividerPattern(),
        StatementPattern(),
        BulletsPattern(),
        CardsPattern(),
        NumbersPattern(),
        TablePattern(),
        BarPattern(),
        FlowPattern(),
        ImagePattern(),
        SummaryPattern(),
    )
}


def describe_limits(mode):
    lines = [
        f"headline: min_width={HEADLINE_MIN_WIDTH}, max_lines={HEADLINE_MAX_LINES}",
        f"handout のフォント倍率: {HANDOUT_SCALE}",
    ]
    for name, pattern in PATTERNS.items():
        values = ", ".join(f"{key}={value}" for key, value in pattern.limits[mode].items())
        lines.append(f"{name}: {values}" if values else f"{name}: (個数・行数の上限なし。箱に収まるかだけ見る)")
    return lines
