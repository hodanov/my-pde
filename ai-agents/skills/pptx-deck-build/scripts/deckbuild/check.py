import math
from dataclasses import dataclass

from .patterns import PATTERNS, Context
from .spec import MODES
from .storyline import load_storylines

ERROR = "error"
WARNING = "warning"


@dataclass(frozen=True)
class Problem:
    severity: str
    where: str
    message: str

    def format(self):
        return f"{self.severity}: {self.where}: {self.message}"


def check_outline(deck):
    return _check_deck_fields(deck) + _check_chapters(deck)


def check_deck(deck):
    problems = check_outline(deck)
    if not deck.chapters:
        return problems
    ctx = Context(deck.mode if deck.mode in MODES else "projection", deck.base_dir)
    problems.extend(_check_slides(deck, ctx, include_generated=not problems))
    if not any(problem.severity == ERROR for problem in problems):
        problems.extend(_check_balance(deck))
    return problems


def _error(where, message):
    return Problem(ERROR, where, message)


def _check_deck_fields(deck):
    storylines = load_storylines()
    problems = []
    for label, value in (("title", deck.title), ("claim", deck.claim), ("audience.persona", deck.audience.persona)):
        if not value:
            problems.append(_error("deck", f"{label} が無い"))
    if deck.mode not in MODES:
        problems.append(_error("deck", f"mode は {' か '.join(MODES)} にする (今は '{deck.mode}')"))
    if not (isinstance(deck.duration_min, int) and not isinstance(deck.duration_min, bool) and deck.duration_min > 0):
        problems.append(_error("deck", "duration_min は正の整数にする"))
    if deck.storyline not in storylines:
        problems.append(
            _error("deck", f"storyline '{deck.storyline}' は未定義 (使える storyline: {', '.join(storylines)})")
        )
    return problems


def _check_chapters(deck):
    if not deck.chapters:
        return [_error("chapters", "章が 1 つも無い")]
    problems = []
    for chapter in deck.chapters:
        where = f"章{chapter.index}"
        for label, value in (("role", chapter.role), ("title", chapter.title), ("conclusion", chapter.conclusion)):
            if not value:
                problems.append(_error(where, f"{label} が無い"))
        if not chapter.slides:
            problems.append(_error(where, "slides が空"))
    storyline = load_storylines().get(deck.storyline)
    if storyline:
        for message in storyline.check_chapter_roles([chapter.role for chapter in deck.chapters]):
            problems.append(_error("chapters", message))
    return problems


def _check_slides(deck, ctx, include_generated):
    problems = []
    for slide in deck.slides():
        if slide.generated and not include_generated:
            continue
        pattern = PATTERNS.get(slide.pattern)
        if pattern is None:
            problems.append(_error(slide.where, f"pattern '{slide.pattern}' は未定義 (使える: {_authorable_names()})"))
            continue
        if not slide.generated and not pattern.authorable:
            problems.append(_error(slide.where, f"pattern '{slide.pattern}' は自動生成する。spec には書かない"))
            continue
        for message in pattern.check(slide, ctx):
            problems.append(_error(slide.where, message))
    return problems


def _authorable_names():
    return ", ".join(name for name, pattern in PATTERNS.items() if pattern.authorable)


def _check_balance(deck):
    problems = []
    body_count = len(deck.authored_slides())
    seconds = deck.duration_min * 60
    low, high = math.ceil(seconds / 45), math.floor(seconds / 20)
    if not low <= body_count <= high:
        problems.append(
            Problem(WARNING, "deck", f"本文 {body_count} 枚 (duration_min={deck.duration_min} の目安は {low}〜{high} 枚)")
        )
    counts = [len(chapter.slides) for chapter in deck.chapters]
    if max(counts) > 2.5 * min(counts):
        problems.append(
            Problem(WARNING, "chapters", f"章ごとの枚数が偏っている ({', '.join(str(count) for count in counts)})")
        )
    return problems
