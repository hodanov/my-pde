import os
import tempfile
import unittest
from pathlib import Path

from deckbuild.build import build_deck
from deckbuild.render import render_slides
from deckbuild.spec import load_deck

from .support import TEMPLATE_SPEC


@unittest.skipUnless(
    os.environ.get("DECKBUILD_RENDER_TEST"),
    "DECKBUILD_RENDER_TEST=1 を付けて sandbox 外で実行する (soffice が要る)",
)
class RenderTest(unittest.TestCase):
    def test_every_slide_becomes_a_png(self):
        deck = load_deck(TEMPLATE_SPEC)
        with tempfile.TemporaryDirectory() as directory:
            pptx = Path(directory) / "deck.pptx"
            build_deck(deck, pptx)
            _, slides, sheets = render_slides(pptx, Path(directory) / "png")
            self.assertEqual(len(slides), len(deck.slides()))
            self.assertEqual(len(sheets), 3)
            self.assertTrue(all(path.stat().st_size > 0 for path in slides + sheets))
