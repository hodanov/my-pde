from copy import deepcopy
from typing import NamedTuple

from lxml import etree
from pptx import Presentation
from pptx.enum.dml import MSO_THEME_COLOR
from pptx.enum.shapes import PP_PLACEHOLDER
from pptx.opc.constants import RELATIONSHIP_TYPE as RT
from pptx.oxml.ns import qn
from pptx.oxml.shapes.autoshape import CT_Shape
from pptx.shapes.autoshape import Shape
from pptx.util import Emu, Inches

FONT_FAMILY = "Noto Sans JP"

TITLE_LAYOUT = "Title Slide"
SECTION_LAYOUT = "Section Header"
CONTENT_LAYOUT = "Title and Content"
TITLE_ONLY_LAYOUT = "Title Only"

COVER_PT = 42
TITLE_PT = 30
BODY_PT = 24
DETAIL_PT = 20
CAPTION_PT = 16
NUMBER_PT = 54
STATEMENT_PT = 36
SLIDE_NUMBER_PT = 14
PARAGRAPH_GAP = 0.5

SLIDE_WIDTH = Emu(12192000)
SLIDE_HEIGHT = Emu(6858000)
MARGIN_X = Inches(0.833)
BULLET_INDENT = Inches(0.375)


class Box(NamedTuple):
    left: int
    top: int
    width: int
    height: int


CONTENT_WIDTH = SLIDE_WIDTH - 2 * MARGIN_X
TITLE_BOX = Box(MARGIN_X, Inches(0.55), CONTENT_WIDTH, Inches(1.15))
CONTENT_BOX = Box(MARGIN_X, Inches(1.95), CONTENT_WIDTH, Inches(4.7))


class Color(NamedTuple):
    slot: str
    theme: MSO_THEME_COLOR
    scheme: str
    hex: str


INK = Color("dk1", MSO_THEME_COLOR.TEXT_1, "tx1", "1F2430")
BACKGROUND = Color("lt1", MSO_THEME_COLOR.BACKGROUND_1, "bg1", "FFFFFF")
MUTED = Color("dk2", MSO_THEME_COLOR.TEXT_2, "tx2", "6B7280")
SURFACE = Color("lt2", MSO_THEME_COLOR.BACKGROUND_2, "bg2", "F4F5F7")
ACCENT = Color("accent1", MSO_THEME_COLOR.ACCENT_1, "accent1", "5B4CDB")
TINT = Color("accent2", MSO_THEME_COLOR.ACCENT_2, "accent2", "EFEDFC")
RULE = Color("accent3", MSO_THEME_COLOR.ACCENT_3, "accent3", "DFE2E8")

THEME_SLOTS = {
    **{color.slot: color.hex for color in (INK, BACKGROUND, MUTED, SURFACE, ACCENT, TINT, RULE)},
    "accent4": MUTED.hex,
    "accent5": INK.hex,
    "accent6": ACCENT.hex,
    "hlink": ACCENT.hex,
    "folHlink": MUTED.hex,
}

LAYOUT_NAMES = (TITLE_LAYOUT, SECTION_LAYOUT, CONTENT_LAYOUT, TITLE_ONLY_LAYOUT)


def make_base_presentation():
    prs = Presentation()
    prs.slide_width = SLIDE_WIDTH
    prs.slide_height = SLIDE_HEIGHT
    master = prs.slide_master
    _apply_theme(master.part.part_related_by(RT.THEME))
    _drop_unused_layouts(prs)
    _style_master(master)
    for layout in prs.slide_layouts:
        _remove_placeholders(layout, (PP_PLACEHOLDER.DATE, PP_PLACEHOLDER.FOOTER))
    _style_title_layout(layout_by_name(prs, TITLE_LAYOUT))
    _style_section_layout(layout_by_name(prs, SECTION_LAYOUT))
    return prs


def layout_by_name(prs, name):
    return next(layout for layout in prs.slide_layouts if layout.name == name)


def attach_slide_number(slide):
    source = next(
        placeholder
        for placeholder in slide.slide_layout.placeholders
        if placeholder.placeholder_format.type == PP_PLACEHOLDER.SLIDE_NUMBER
    )
    element = deepcopy(source._element)
    transform = element.spPr.find(qn("a:xfrm"))
    if transform is not None:
        element.spPr.remove(transform)
    element.nvSpPr.cNvPr.set("id", str(slide.shapes._next_shape_id))
    slide.shapes._spTree.append(element)


def _apply_theme(theme_part):
    ns = {"a": "http://schemas.openxmlformats.org/drawingml/2006/main"}
    root = etree.fromstring(theme_part.blob)
    scheme = root.find(".//a:clrScheme", ns)
    scheme.set("name", "Clean")
    for slot, hex_value in THEME_SLOTS.items():
        element = scheme.find(f"a:{slot}", ns)
        for child in list(element):
            element.remove(child)
        etree.SubElement(element, qn("a:srgbClr"), val=hex_value)
    fonts = root.find(".//a:fontScheme", ns)
    fonts.set("name", "Clean")
    for kind in ("majorFont", "minorFont"):
        font = fonts.find(f"a:{kind}", ns)
        font.find("a:latin", ns).set("typeface", FONT_FAMILY)
        font.find("a:ea", ns).set("typeface", FONT_FAMILY)
        for script in font.findall("a:font", ns):
            if script.get("script") == "Jpan":
                script.set("typeface", FONT_FAMILY)
    theme_part._blob = etree.tostring(
        root, xml_declaration=True, encoding="UTF-8", standalone=True
    )


def _drop_unused_layouts(prs):
    for layout in list(prs.slide_layouts):
        if layout.name not in LAYOUT_NAMES:
            prs.slide_layouts.remove(layout)


def _style_master(master):
    styles = master._element.find(qn("p:txStyles"))
    title_level = styles.find(qn("p:titleStyle")).find(qn("a:lvl1pPr"))
    title_level.set("algn", "l")
    title_font = title_level.find(qn("a:defRPr"))
    title_font.set("sz", str(TITLE_PT * 100))
    title_font.set("b", "1")
    body_style = styles.find(qn("p:bodyStyle"))
    for level, size in enumerate((BODY_PT, DETAIL_PT, 18, CAPTION_PT, CAPTION_PT), start=1):
        paragraph_style = body_style.find(qn(f"a:lvl{level}pPr"))
        paragraph_style.find(qn("a:defRPr")).set("sz", str(size * 100))
        paragraph_style.set("marL", str(BULLET_INDENT * level))
        paragraph_style.set("indent", str(-BULLET_INDENT))
        paragraph_style.find(qn("a:spcBef")).find(qn("a:spcPct")).set("val", str(round(PARAGRAPH_GAP * 100000)))
    _remove_placeholders(master, (PP_PLACEHOLDER.DATE, PP_PLACEHOLDER.FOOTER))
    for placeholder in master.placeholders:
        kind = placeholder.placeholder_format.type
        if kind == PP_PLACEHOLDER.TITLE:
            _place(placeholder, TITLE_BOX, anchor="t")
        elif kind == PP_PLACEHOLDER.BODY:
            _place(placeholder, CONTENT_BOX, anchor="ctr")
        elif kind == PP_PLACEHOLDER.SLIDE_NUMBER:
            _place(
                placeholder,
                Box(SLIDE_WIDTH - MARGIN_X - Inches(1), Inches(6.95), Inches(1), Inches(0.35)),
                anchor="ctr",
            )
            _set_list_style(placeholder, SLIDE_NUMBER_PT, MUTED, align="r")


def _style_title_layout(layout):
    _add_layout_rectangle(
        layout, Box(MARGIN_X, Inches(2.2), Inches(1.0), Inches(0.083)), ACCENT
    )
    for placeholder in layout.placeholders:
        kind = placeholder.placeholder_format.type
        if kind == PP_PLACEHOLDER.CENTER_TITLE:
            _place(
                placeholder, Box(MARGIN_X, Inches(2.5), CONTENT_WIDTH, Inches(1.7)), anchor="t"
            )
            _set_list_style(placeholder, COVER_PT, INK, bold=True)
        elif kind == PP_PLACEHOLDER.SUBTITLE:
            _place(
                placeholder, Box(MARGIN_X, Inches(4.35), CONTENT_WIDTH, Inches(1.3)), anchor="t"
            )
            _set_list_style(placeholder, BODY_PT, MUTED)


def _style_section_layout(layout):
    layout.background.fill.solid()
    layout.background.fill.fore_color.theme_color = ACCENT.theme
    for placeholder in layout.placeholders:
        kind = placeholder.placeholder_format.type
        if kind == PP_PLACEHOLDER.TITLE:
            _place(
                placeholder, Box(MARGIN_X, Inches(3.15), CONTENT_WIDTH, Inches(1.5)), anchor="t"
            )
            _set_list_style(placeholder, COVER_PT, BACKGROUND, bold=True)
        elif kind == PP_PLACEHOLDER.BODY:
            _place(
                placeholder, Box(MARGIN_X, Inches(2.5), CONTENT_WIDTH, Inches(0.6)), anchor="b"
            )
            _set_list_style(placeholder, BODY_PT, BACKGROUND, bold=True, alpha=75)
        elif kind == PP_PLACEHOLDER.SLIDE_NUMBER:
            _set_list_style(placeholder, SLIDE_NUMBER_PT, BACKGROUND, align="r", alpha=75)


def _remove_placeholders(owner, kinds):
    for placeholder in list(owner.placeholders):
        if placeholder.placeholder_format.type in kinds:
            placeholder._element.getparent().remove(placeholder._element)


def _place(placeholder, box, anchor):
    placeholder.left, placeholder.top, placeholder.width, placeholder.height = box
    body_properties = placeholder._element.txBody.find(qn("a:bodyPr"))
    for child in list(body_properties):
        body_properties.remove(child)
    body_properties.set("anchor", anchor)
    body_properties.set("lIns", "0")
    body_properties.set("rIns", "0")
    etree.SubElement(body_properties, qn("a:noAutofit"))


def _set_list_style(placeholder, size_pt, color, bold=False, align="l", alpha=None):
    list_style = placeholder._element.txBody.find(qn("a:lstStyle"))
    for child in list(list_style):
        list_style.remove(child)
    level = etree.SubElement(list_style, qn("a:lvl1pPr"), marL="0", indent="0", algn=align)
    etree.SubElement(level, qn("a:buNone"))
    font = etree.SubElement(level, qn("a:defRPr"), sz=str(size_pt * 100), b="1" if bold else "0")
    fill = etree.SubElement(font, qn("a:solidFill"))
    color = etree.SubElement(fill, qn("a:schemeClr"), val=color.scheme)
    if alpha is not None:
        etree.SubElement(color, qn("a:alpha"), val=str(alpha * 1000))


def _add_layout_rectangle(layout, box, color):
    shapes = layout.shapes
    element = CT_Shape.new_autoshape_sp(shapes._next_shape_id, "Accent Bar", "rect", *box)
    element.remove(element.find(qn("p:style")))
    shapes._spTree.append(element)
    shape = Shape(element, shapes)
    shape.fill.solid()
    shape.fill.fore_color.theme_color = color.theme
    shape.line.fill.background()
