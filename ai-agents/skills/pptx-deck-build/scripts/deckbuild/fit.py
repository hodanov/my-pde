import math
import unicodedata

from .template import PARAGRAPH_GAP

EMU_PER_POINT = 12700


def is_wide(char):
    return unicodedata.east_asian_width(char) in ("W", "F")


def display_width(text):
    return sum(2 if is_wide(char) else 1 for char in text)


def wrapped_lines(text, font_pt, width_emu):
    half_width_em = 0.55
    wrap_margin = 1.05
    width_pt = width_emu / EMU_PER_POINT
    total = 0
    for line in text.split("\n"):
        em = sum(1.0 if is_wide(char) else half_width_em for char in line)
        total += max(1, math.ceil(em * font_pt * wrap_margin / width_pt))
    return total


def block_height(line_count, paragraph_count, font_pt):
    line_height = 1.4
    return round((line_count * line_height + paragraph_count * PARAGRAPH_GAP) * font_pt * EMU_PER_POINT)
