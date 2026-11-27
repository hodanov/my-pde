import unittest

from deckbuild.report import headline_chain, outline_text, visible_text
from deckbuild.spec import load_deck

from .support import TEMPLATE_SPEC


class ReportTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.deck = load_deck(TEMPLATE_SPEC)

    def test_headline_chain_lists_every_slide_in_order(self):
        lines = headline_chain(self.deck).splitlines()
        self.assertEqual(len(lines), 20)
        self.assertEqual(lines[0], f"1. {self.deck.title}")
        self.assertEqual(lines[1], f"2. {self.deck.claim}")
        self.assertEqual(lines[2], "3. 目次")
        self.assertEqual(lines[-1], "20. まとめ")

    def test_outline_shows_roles_labels_and_focus(self):
        outline = outline_text(self.deck)
        self.assertIn("1. [background] 背景・課題: 週次レポートに毎週3時間かかっていた", outline)
        self.assertIn("▶ 見てほしい所: 集計の棒が、ほかの2工程より長いこと", outline)

    def test_visible_text_omits_notes_and_numbers_slides(self):
        text = visible_text(self.deck)
        self.assertTrue(text.startswith("=== slide 1 ==="))
        self.assertIn("=== slide 20 ===", text)
        for note in (slide.notes for slide in self.deck.authored_slides()):
            self.assertNotIn(note, text)
