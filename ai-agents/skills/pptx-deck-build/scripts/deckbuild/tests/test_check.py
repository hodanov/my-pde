import tempfile
import unittest

from deckbuild.check import ERROR, WARNING, check_deck
from deckbuild.spec import load_deck

from .support import FIXTURES, TEMPLATE_SPEC, load_template_data, write_spec


def check_data(data):
    with tempfile.TemporaryDirectory() as directory:
        return check_deck(load_deck(write_spec(directory, data)))


def errors_of(problems):
    return [problem for problem in problems if problem.severity == ERROR]


class BrokenSpecTest(unittest.TestCase):
    def test_reports_every_expected_problem(self):
        problems = check_deck(load_deck(FIXTURES / "broken.yaml"))
        expected = (FIXTURES / "broken.expected").read_text(encoding="utf-8").splitlines()
        self.assertEqual([problem.format() for problem in problems], expected)


class TemplateSpecTest(unittest.TestCase):
    def test_template_has_no_problems(self):
        self.assertEqual(check_deck(load_deck(TEMPLATE_SPEC)), [])


class DeckFieldsTest(unittest.TestCase):
    def test_missing_claim_is_an_error(self):
        data = load_template_data()
        del data["deck"]["claim"]
        self.assertIn("error: deck: claim が無い", [problem.format() for problem in check_data(data)])

    def test_unknown_storyline_is_an_error(self):
        data = load_template_data()
        data["deck"]["storyline"] = "no-such-storyline"
        messages = [problem.message for problem in errors_of(check_data(data))]
        self.assertTrue(any("storyline 'no-such-storyline' は未定義" in message for message in messages))

    def test_chapter_without_conclusion_is_an_error(self):
        data = load_template_data()
        data["chapters"][1]["conclusion"] = ""
        self.assertIn("error: 章2: conclusion が無い", [problem.format() for problem in check_data(data)])


class ModeLimitsTest(unittest.TestCase):
    def over_projection_limit(self, mode):
        data = load_template_data()
        data["deck"]["mode"] = mode
        slide = data["chapters"][0]["slides"][1]
        slide["items"] = ["要点を書く"] * 5
        del slide["notes"]
        return check_data(data)

    def test_projection_rejects_five_bullets_without_notes(self):
        messages = [problem.message for problem in errors_of(self.over_projection_limit("projection"))]
        self.assertTrue(any("項目が 5 個 (上限 3)" in message for message in messages))
        self.assertTrue(any("notes が空" in message for message in messages))

    def test_handout_accepts_five_bullets_without_notes(self):
        self.assertEqual(errors_of(self.over_projection_limit("handout")), [])


class FitTest(unittest.TestCase):
    def test_overflowing_bullet_is_reported(self):
        data = load_template_data()
        data["chapters"][0]["slides"][1]["items"] = ["長い文" * 40]
        messages = [problem.message for problem in errors_of(check_data(data))]
        self.assertTrue(any("items[1] が" in message and "上限 2" in message for message in messages))

    def test_manual_line_breaks_count_toward_the_limit(self):
        data = load_template_data()
        data["chapters"][4]["slides"][1]["headline"] = "一行目\n二行目\n三行目\n四行目"
        messages = [problem.message for problem in errors_of(check_data(data))]
        self.assertTrue(any("headline が 4 行" in message for message in messages))


class BalanceWarningTest(unittest.TestCase):
    def test_short_duration_for_many_slides_warns_without_failing(self):
        data = load_template_data()
        data["deck"]["duration_min"] = 3
        problems = check_data(data)
        self.assertEqual(errors_of(problems), [])
        self.assertEqual([problem.severity for problem in problems], [WARNING])
        self.assertIn("duration_min=3", problems[0].message)

    def test_lopsided_chapters_warn(self):
        data = load_template_data()
        extra = dict(data["chapters"][0]["slides"][1])
        data["chapters"][0]["slides"].extend([extra] * 5)
        data["deck"]["duration_min"] = 15
        problems = check_data(data)
        self.assertEqual(errors_of(problems), [])
        self.assertTrue(any("章ごとの枚数が偏っている" in problem.message for problem in problems))
