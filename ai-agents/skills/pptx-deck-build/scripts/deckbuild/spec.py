from collections.abc import Mapping
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import yaml

MODES = ("projection", "handout")
MIN_CHAPTERS_FOR_DIVIDERS = 3


class SpecError(Exception):
    pass


def _text(value):
    return "" if value is None else str(value).strip()


@dataclass(frozen=True)
class Slide:
    pattern: str
    data: Mapping[str, Any]
    where: str
    generated: bool = False

    @property
    def headline(self):
        return _text(self.data.get("headline"))

    @property
    def notes(self):
        return _text(self.data.get("notes"))

    @property
    def focus(self):
        return _text(self.data.get("focus"))


@dataclass(frozen=True)
class Chapter:
    index: int
    role: str
    title: str
    conclusion: str
    slides: tuple[Slide, ...]


@dataclass(frozen=True)
class Audience:
    persona: str
    knows: tuple[str, ...]
    unknown: tuple[str, ...]


@dataclass(frozen=True)
class Deck:
    title: str
    subtitle: str
    date: str
    author: str
    mode: str
    duration_min: Any
    claim: str
    audience: Audience
    storyline: str
    chapters: tuple[Chapter, ...]
    base_dir: Path

    def authored_slides(self):
        return [slide for chapter in self.chapters for slide in chapter.slides]

    def slides(self):
        sequence = [
            Slide(
                "title",
                {"headline": self.title, "subtitle": self.subtitle, "date": self.date, "author": self.author},
                "表紙",
                generated=True,
            ),
            Slide("statement", {"headline": self.claim}, "主張", generated=True),
            Slide(
                "agenda",
                {"headline": "目次", "items": [chapter.title for chapter in self.chapters]},
                "目次",
                generated=True,
            ),
        ]
        with_dividers = len(self.chapters) >= MIN_CHAPTERS_FOR_DIVIDERS
        for chapter in self.chapters:
            if with_dividers:
                sequence.append(
                    Slide(
                        "divider",
                        {"headline": chapter.title, "number": chapter.index},
                        f"章{chapter.index} の扉",
                        generated=True,
                    )
                )
            sequence.extend(chapter.slides)
        sequence.append(
            Slide(
                "summary",
                {"headline": "まとめ", "items": [chapter.conclusion for chapter in self.chapters]},
                "まとめ",
                generated=True,
            )
        )
        return sequence


def load_deck(path):
    path = Path(path)
    raw = yaml.safe_load(path.read_text(encoding="utf-8"))
    if not isinstance(raw, dict):
        raise SpecError(f"{path}: トップレベルは deck と chapters を持つマッピングにする")
    deck_raw = raw.get("deck") if isinstance(raw.get("deck"), dict) else {}
    audience_raw = deck_raw.get("audience") if isinstance(deck_raw.get("audience"), dict) else {}
    chapters_raw = raw.get("chapters") if isinstance(raw.get("chapters"), list) else []
    return Deck(
        title=_text(deck_raw.get("title")),
        subtitle=_text(deck_raw.get("subtitle")),
        date=_text(deck_raw.get("date")),
        author=_text(deck_raw.get("author")),
        mode=_text(deck_raw.get("mode")),
        duration_min=deck_raw.get("duration_min"),
        claim=_text(deck_raw.get("claim")),
        audience=Audience(
            persona=_text(audience_raw.get("persona")),
            knows=_text_tuple(audience_raw.get("knows")),
            unknown=_text_tuple(audience_raw.get("unknown")),
        ),
        storyline=_text(deck_raw.get("storyline")),
        chapters=tuple(_chapter(index, entry) for index, entry in enumerate(chapters_raw, start=1)),
        base_dir=path.resolve().parent,
    )


def _text_tuple(value):
    return tuple(_text(item) for item in value) if isinstance(value, list) else ()


def _chapter(index, entry):
    entry = entry if isinstance(entry, dict) else {}
    slides_raw = entry.get("slides") if isinstance(entry.get("slides"), list) else []
    return Chapter(
        index=index,
        role=_text(entry.get("role")),
        title=_text(entry.get("title")),
        conclusion=_text(entry.get("conclusion")),
        slides=tuple(
            _slide(index, number, slide_raw) for number, slide_raw in enumerate(slides_raw, start=1)
        ),
    )


def _slide(chapter_index, number, raw):
    data = raw if isinstance(raw, dict) else {}
    return Slide(
        pattern=_text(data.get("pattern")),
        data=data,
        where=f"章{chapter_index} スライド{number}",
    )
