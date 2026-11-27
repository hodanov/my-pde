from .patterns import PATTERNS
from .storyline import load_storylines


def _one_line(text):
    return text.replace("\n", " ")


def outline_text(deck):
    storyline = load_storylines().get(deck.storyline)
    lines = [
        f"主張: {_one_line(deck.claim)}",
        f"聞き手: {deck.audience.persona}",
        f"形式: {deck.mode} / {deck.duration_min} 分 / storyline {deck.storyline}",
        "",
    ]
    for chapter in deck.chapters:
        label = storyline.label_of(chapter.role) if storyline else chapter.role
        lines.append(f"{chapter.index}. [{chapter.role}] {label}: {chapter.title}")
        lines.append(f"   結論: {chapter.conclusion}")
        for number, slide in enumerate(chapter.slides, start=1):
            focus = f"  ▶ 見てほしい所: {slide.focus}" if slide.focus else ""
            lines.append(f"   {chapter.index}-{number} [{slide.pattern}] {_one_line(slide.headline)}{focus}")
    return "\n".join(lines)


def headline_chain(deck):
    return "\n".join(
        f"{number}. {_one_line(slide.headline)}" for number, slide in enumerate(deck.slides(), start=1)
    )


def visible_text(deck):
    return "\n\n".join(
        f"=== slide {number} ===\n" + "\n".join(PATTERNS[slide.pattern].visible_text(slide))
        for number, slide in enumerate(deck.slides(), start=1)
    )
