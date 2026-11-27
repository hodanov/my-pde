import contextlib
import io
import tempfile
import unittest
from pathlib import Path

from deckbuild.__main__ import main

from .support import FIXTURES, TEMPLATE_SPEC, load_template_data, write_spec


def run_cli(*arguments):
    out, err = io.StringIO(), io.StringIO()
    with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
        status = main([str(argument) for argument in arguments])
    return status, out.getvalue(), err.getvalue()


class CliTest(unittest.TestCase):
    def test_check_passes_for_the_template(self):
        status, _, err = run_cli("check", TEMPLATE_SPEC)
        self.assertEqual(status, 0)
        self.assertIn("エラー 0 件", err)

    def test_check_fails_for_a_broken_spec(self):
        status, _, err = run_cli("check", FIXTURES / "broken.yaml")
        self.assertEqual(status, 1)
        self.assertIn("エラー 16 件", err)

    def test_build_writes_nothing_when_the_check_fails(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "broken.pptx"
            status, _, _ = run_cli("build", FIXTURES / "broken.yaml", "-o", output)
            self.assertEqual(status, 1)
            self.assertFalse(output.exists())

    def test_build_writes_the_pptx_for_a_valid_spec(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "deck.pptx"
            status, out, _ = run_cli("build", TEMPLATE_SPEC, "-o", output)
            self.assertEqual(status, 0)
            self.assertTrue(output.exists())
            self.assertIn(f"built: {output}", out)

    def test_text_prints_nothing_for_a_broken_spec(self):
        status, out, _ = run_cli("text", FIXTURES / "broken.yaml")
        self.assertEqual(status, 1)
        self.assertEqual(out, "")

    def test_outline_accepts_a_skeleton_without_bodies(self):
        data = load_template_data()
        for chapter in data["chapters"]:
            chapter["slides"] = [
                {key: slide[key] for key in ("pattern", "headline", "focus") if key in slide}
                for slide in chapter["slides"]
            ]
        with tempfile.TemporaryDirectory() as directory:
            status, out, err = run_cli("outline", write_spec(directory, data))
        self.assertEqual(status, 0)
        self.assertIn("エラー 0 件", err)
        self.assertIn("[bar] 自動化前は、180分のうち集計が120分を占めていた", out)

    def test_outline_fails_when_required_chapters_are_missing(self):
        status, out, err = run_cli("outline", FIXTURES / "broken.yaml", "--headlines")
        self.assertEqual(status, 1)
        self.assertIn("1. 壊れた spec", out)
        self.assertIn("必須の章 'approach'", err)

    def test_limits_lists_every_pattern(self):
        status, out, _ = run_cli("limits", "--mode", "handout")
        self.assertEqual(status, 0)
        for name in ("bullets", "cards", "numbers", "table", "bar", "flow", "image"):
            self.assertIn(f"{name}:", out)

    def test_missing_spec_is_reported_as_an_error(self):
        status, out, err = run_cli("check", "does-not-exist.yaml")
        self.assertEqual(status, 1)
        self.assertEqual(out, "")
        self.assertIn("error:", err)
        self.assertIn("does-not-exist.yaml", err)
