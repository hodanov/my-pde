from dataclasses import dataclass

from lxml import etree
from pptx.enum.lang import MSO_LANGUAGE_ID
from pptx.enum.shapes import MSO_CONNECTOR, MSO_SHAPE
from pptx.enum.text import MSO_ANCHOR, MSO_AUTO_SIZE, PP_ALIGN
from pptx.oxml.ns import qn
from pptx.util import Emu, Pt

from .template import ACCENT, INK, MUTED, Color


@dataclass(frozen=True)
class Line:
    text: str
    size_pt: float
    color: Color = INK
    bold: bool = False
    align: PP_ALIGN = PP_ALIGN.LEFT
    space_after_pt: float = 0


def add_box(slide, box, color, kind=MSO_SHAPE.RECTANGLE, brightness=0.0, corner=None):
    shape = slide.shapes.add_shape(kind, *box)
    _drop_theme_style(shape)
    shape.fill.solid()
    shape.fill.fore_color.theme_color = color.theme
    if brightness:
        shape.fill.fore_color.brightness = brightness
    shape.line.fill.background()
    if corner is not None:
        shape.adjustments[0] = corner
    return shape


def add_text(slide, box, lines, anchor=MSO_ANCHOR.TOP):
    shape = slide.shapes.add_textbox(*box)
    write_lines(shape.text_frame, lines, anchor, inset=0)
    return shape


def write_lines(frame, lines, anchor=MSO_ANCHOR.TOP, inset=None):
    frame.word_wrap = True
    frame.auto_size = MSO_AUTO_SIZE.NONE
    frame.vertical_anchor = anchor
    if inset is not None:
        frame.margin_left = frame.margin_right = Emu(inset)
        frame.margin_top = frame.margin_bottom = Emu(inset)
    for index, line in enumerate(lines):
        paragraph = frame.paragraphs[0] if index == 0 else frame.add_paragraph()
        paragraph.alignment = line.align
        paragraph.space_after = Pt(line.space_after_pt)
        for run in _append_runs(paragraph, line.text):
            run.font.size = Pt(line.size_pt)
            run.font.bold = line.bold
            run.font.color.theme_color = line.color.theme


def fill_placeholder(placeholder, texts, size_pt=None):
    frame = placeholder.text_frame
    for index, text in enumerate(texts):
        paragraph = frame.paragraphs[0] if index == 0 else frame.add_paragraph()
        for run in _append_runs(paragraph, text):
            if size_pt is not None:
                run.font.size = Pt(size_pt)


def _append_runs(paragraph, text):
    runs = []
    for index, part in enumerate(text.split("\n")):
        if index > 0:
            paragraph.add_line_break()
        run = paragraph.add_run()
        run.text = part
        run.font.language_id = MSO_LANGUAGE_ID.JAPANESE
        runs.append(run)
    return runs


def number_paragraphs(placeholder):
    for paragraph in placeholder.text_frame.paragraphs:
        properties = paragraph._p.get_or_add_pPr()
        bullet_color = etree.SubElement(properties, qn("a:buClr"))
        etree.SubElement(bullet_color, qn("a:schemeClr"), val=ACCENT.scheme)
        etree.SubElement(properties, qn("a:buFont"), typeface="+mj-lt")
        etree.SubElement(properties, qn("a:buAutoNum"), type="arabicPeriod")


def add_arrow(slide, start_shape, end_shape, color=MUTED, width_pt=2):
    connector = slide.shapes.add_connector(MSO_CONNECTOR.STRAIGHT, 0, 0, 0, 0)
    connector.begin_connect(start_shape, 3)
    connector.end_connect(end_shape, 1)
    _drop_theme_style(connector)
    connector.line.color.theme_color = color.theme
    connector.line.width = Pt(width_pt)
    etree.SubElement(connector.line._get_or_add_ln(), qn("a:tailEnd"), type="triangle")
    return connector


def _drop_theme_style(shape):
    style = shape._element.find(qn("p:style"))
    if style is not None:
        shape._element.remove(style)
