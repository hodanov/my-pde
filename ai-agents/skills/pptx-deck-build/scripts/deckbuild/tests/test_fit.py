import unittest

from pptx.util import Pt

from deckbuild.fit import block_height, display_width, wrapped_lines


class DisplayWidthTest(unittest.TestCase):
    def test_wide_characters_count_double(self):
        self.assertEqual(display_width("あい"), 4)

    def test_half_width_characters_count_single(self):
        self.assertEqual(display_width("ab1"), 3)


class WrappedLinesTest(unittest.TestCase):
    def test_wide_text_within_width_is_one_line(self):
        self.assertEqual(wrapped_lines("あ" * 10, 20, Pt(300)), 1)

    def test_wide_text_over_width_wraps(self):
        self.assertEqual(wrapped_lines("あ" * 15, 20, Pt(300)), 2)

    def test_half_width_text_is_narrower_than_wide_text(self):
        self.assertEqual(wrapped_lines("a" * 25, 20, Pt(300)), 1)
        self.assertEqual(wrapped_lines("a" * 30, 20, Pt(300)), 2)

    def test_explicit_line_breaks_each_count(self):
        self.assertEqual(wrapped_lines("あ\nあ\nあ", 20, Pt(300)), 3)

    def test_empty_text_still_takes_a_line(self):
        self.assertEqual(wrapped_lines("", 20, Pt(300)), 1)


class BlockHeightTest(unittest.TestCase):
    def test_more_lines_are_taller(self):
        self.assertGreater(block_height(2, 1, 20), block_height(1, 1, 20))

    def test_more_paragraphs_are_taller(self):
        self.assertGreater(block_height(2, 2, 20), block_height(2, 1, 20))

    def test_larger_font_is_taller(self):
        self.assertGreater(block_height(1, 1, 30), block_height(1, 1, 20))
