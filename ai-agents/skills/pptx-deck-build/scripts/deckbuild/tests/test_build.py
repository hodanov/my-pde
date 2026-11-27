import tempfile
import unittest
from pathlib import Path

from PIL import Image
from pptx import Presentation
from pptx.enum.dml import MSO_THEME_COLOR
from pptx.enum.shapes import MSO_SHAPE_TYPE
from pptx.opc.constants import RELATIONSHIP_TYPE as RT
from pptx.util import Pt

from deckbuild.build import build_deck
from deckbuild.check import ERROR, check_deck
from deckbuild.spec import load_deck
from deckbuild.template import CONTENT_BOX, LAYOUT_NAMES

from .support import TEMPLATE_SPEC, load_template_data, write_spec


def line_text(frame):
    return frame.text.replace("\v", "\n")


class BuildTemplateTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.directory = tempfile.TemporaryDirectory()
        cls.deck = load_deck(TEMPLATE_SPEC)
        output = Path(cls.directory.name) / "deck.pptx"
        build_deck(cls.deck, output)
        cls.presentation = Presentation(output)
        cls.specs = cls.deck.slides()

    @classmethod
    def tearDownClass(cls):
        cls.directory.cleanup()

    def test_slide_count_matches_the_deck_plan(self):
        self.assertEqual(len(self.presentation.slides), len(self.specs))
        self.assertEqual(len(self.presentation.slides), 20)

    def test_every_title_is_the_headline(self):
        for slide, spec in zip(self.presentation.slides, self.specs, strict=True):
            self.assertEqual(line_text(slide.shapes.title.text_frame), spec.headline, spec.where)

    def test_notes_are_kept(self):
        for slide, spec in zip(self.presentation.slides, self.specs, strict=True):
            if spec.notes:
                self.assertEqual(slide.notes_slide.notes_text_frame.text, spec.notes, spec.where)

    def test_only_the_four_layouts_remain(self):
        names = [layout.name for layout in self.presentation.slide_layouts]
        self.assertCountEqual(names, LAYOUT_NAMES)

    def test_slide_is_sixteen_by_nine(self):
        self.assertEqual(self.presentation.slide_width * 9, self.presentation.slide_height * 16)

    def test_theme_carries_the_font_and_accent_color(self):
        theme = self.presentation.slide_master.part.part_related_by(RT.THEME).blob.decode("utf-8")
        self.assertIn('<a:latin typeface="Noto Sans JP"', theme)
        self.assertIn('<a:ea typeface="Noto Sans JP"', theme)
        self.assertIn('<a:srgbClr val="5B4CDB"/>', theme)

    def test_runs_do_not_override_the_theme_font(self):
        for slide, spec in zip(self.presentation.slides, self.specs, strict=True):
            overrides = slide._element.xpath(".//a:rPr/a:latin | .//a:rPr/a:ea | .//a:rPr/a:cs")
            self.assertEqual(overrides, [], spec.where)

    def test_every_slide_but_the_cover_has_a_slide_number(self):
        for index, slide in enumerate(self.presentation.slides):
            fields = slide._element.xpath('.//a:fld[@type="slidenum"]')
            self.assertEqual(len(fields), 0 if index == 0 else 1, self.specs[index].where)

    def test_shapes_stay_inside_the_slide(self):
        width, height = self.presentation.slide_width, self.presentation.slide_height
        for slide, spec in zip(self.presentation.slides, self.specs, strict=True):
            for shape in slide.shapes:
                self.assertGreaterEqual(shape.left, 0, f"{spec.where}: {shape.name}")
                self.assertGreaterEqual(shape.top, 0, f"{spec.where}: {shape.name}")
                self.assertLessEqual(shape.left + shape.width, width, f"{spec.where}: {shape.name}")
                self.assertLessEqual(shape.top + shape.height, height, f"{spec.where}: {shape.name}")

    def test_highlighted_bar_uses_the_accent_theme_color(self):
        bar_slide = self.presentation.slides[4]
        fills = [
            shape.fill.fore_color.theme_color
            for shape in bar_slide.shapes
            if shape.shape_type == MSO_SHAPE_TYPE.AUTO_SHAPE
        ]
        self.assertEqual(len(fills), 3)
        self.assertEqual(fills.count(MSO_THEME_COLOR.ACCENT_1), 1)


class BuildHandoutTest(unittest.TestCase):
    def test_handout_shrinks_the_body_title(self):
        data = load_template_data()
        data["deck"]["mode"] = "handout"
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "handout.pptx"
            build_deck(load_deck(write_spec(directory, data)), output)
            presentation = Presentation(output)
        body_title = presentation.slides[4].shapes.title.text_frame.paragraphs[0].runs[0]
        self.assertEqual(body_title.font.size, Pt(24))


class BuildImageTest(unittest.TestCase):
    def build_with_image(self, directory, size):
        Image.new("RGB", size, "white").save(Path(directory) / "shot.png")
        data = load_template_data()
        data["chapters"][0]["slides"].append(
            {
                "pattern": "image",
                "headline": "画面の右上にある設定ボタンから変更できる",
                "focus": "右上の設定ボタン",
                "notes": "画面の一部を切り抜いて貼る。",
                "path": "shot.png",
                "caption": "設定画面",
            }
        )
        return load_deck(write_spec(directory, data))

    def test_picture_is_placed_inside_the_content_box(self):
        with tempfile.TemporaryDirectory() as directory:
            deck = self.build_with_image(directory, (1000, 500))
            self.assertEqual([p for p in check_deck(deck) if p.severity == ERROR], [])
            output = Path(directory) / "image.pptx"
            build_deck(deck, output)
            presentation = Presentation(output)
        slide = presentation.slides[6]
        pictures = [shape for shape in slide.shapes if shape.shape_type == MSO_SHAPE_TYPE.PICTURE]
        self.assertEqual(len(pictures), 1)
        picture = pictures[0]
        self.assertGreaterEqual(picture.left, CONTENT_BOX.left)
        self.assertLessEqual(picture.left + picture.width, CONTENT_BOX.left + CONTENT_BOX.width)
        self.assertLessEqual(picture.top + picture.height, CONTENT_BOX.top + CONTENT_BOX.height)
        self.assertEqual(picture.width * 500, picture.height * 1000)

    def test_oversized_screenshot_must_be_cropped(self):
        with tempfile.TemporaryDirectory() as directory:
            deck = self.build_with_image(directory, (2400, 1200))
            messages = [problem.message for problem in check_deck(deck) if problem.severity == ERROR]
        self.assertTrue(any("縮めて置く" in message for message in messages))

    def test_svg_is_rejected_with_a_hint(self):
        data = load_template_data()
        data["chapters"][0]["slides"].append(
            {
                "pattern": "image",
                "headline": "構成図は SVG ではなく PNG にして貼る",
                "focus": "中央の箱",
                "notes": "補足",
                "path": "diagram.svg",
            }
        )
        with tempfile.TemporaryDirectory() as directory:
            messages = [
                problem.message
                for problem in check_deck(load_deck(write_spec(directory, data)))
                if problem.severity == ERROR
            ]
        self.assertTrue(any("SVG は PNG に書き出して貼る" in message for message in messages))
