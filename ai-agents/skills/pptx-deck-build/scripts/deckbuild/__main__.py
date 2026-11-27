import argparse
import sys
import unittest
from pathlib import Path

from .build import build_deck
from .check import ERROR, check_deck, check_outline
from .patterns import describe_limits
from .render import RenderError, render_slides
from .report import headline_chain, outline_text, visible_text
from .spec import MODES, SpecError, load_deck


def main(argv=None):
    parser = argparse.ArgumentParser(prog="deck.sh")
    commands = parser.add_subparsers(dest="command", required=True)

    outline = commands.add_parser("outline", help="章と見出しの一覧を出す")
    outline.add_argument("spec")
    outline.add_argument("--headlines", action="store_true", help="全スライドの見出しだけを順に出す")

    for name, help_text in (
        ("check", "spec を検証する"),
        ("text", "初見の読み手に渡す、見える文字だけを出す"),
        ("build", "spec から pptx を書き出す"),
    ):
        command = commands.add_parser(name, help=help_text)
        command.add_argument("spec")
        if name == "build":
            command.add_argument("-o", "--output", help="出力先 (既定: spec と同じ場所の .pptx)")

    render = commands.add_parser("render", help="pptx を PNG にする (LibreOffice が要る)")
    render.add_argument("pptx")
    render.add_argument("-o", "--out-dir")

    limits = commands.add_parser("limits", help="パターンごとの上限を出す")
    limits.add_argument("--mode", choices=MODES, default="projection")

    commands.add_parser("test", help="このスキルのテストを実行する")

    arguments = parser.parse_args(argv)
    try:
        return _COMMANDS[arguments.command](arguments)
    except (SpecError, RenderError, OSError) as error:
        print(f"error: {error}", file=sys.stderr)
        return 1


def _run_outline(arguments):
    deck = load_deck(arguments.spec)
    print(headline_chain(deck) if arguments.headlines else outline_text(deck))
    return _report_problems(check_outline(deck))


def _run_check(arguments):
    return _report_problems(check_deck(load_deck(arguments.spec)))


def _run_text(arguments):
    deck = load_deck(arguments.spec)
    status = _report_problems(check_deck(deck))
    if status == 0:
        print(visible_text(deck))
    return status


def _run_build(arguments):
    deck = load_deck(arguments.spec)
    status = _report_problems(check_deck(deck))
    if status != 0:
        return status
    output = Path(arguments.output) if arguments.output else Path(arguments.spec).with_suffix(".pptx")
    build_deck(deck, output)
    print(f"built: {output}")
    return 0


def _run_render(arguments):
    out_dir, slides, sheets = render_slides(arguments.pptx, arguments.out_dir)
    print(f"slides: {out_dir} ({len(slides)} 枚)")
    for sheet in sheets:
        print(f"sheet: {sheet}")
    return 0


def _run_limits(arguments):
    print("\n".join(describe_limits(arguments.mode)))
    return 0


def _run_test(arguments):
    package_dir = Path(__file__).resolve().parent
    suite = unittest.defaultTestLoader.discover(
        str(package_dir / "tests"), top_level_dir=str(package_dir.parent)
    )
    result = unittest.TextTestRunner(verbosity=1).run(suite)
    return 0 if result.wasSuccessful() else 1


def _report_problems(problems):
    for problem in problems:
        print(problem.format(), file=sys.stderr)
    errors = sum(1 for problem in problems if problem.severity == ERROR)
    print(f"check: エラー {errors} 件、警告 {len(problems) - errors} 件", file=sys.stderr)
    return 1 if errors else 0


_COMMANDS = {
    "outline": _run_outline,
    "check": _run_check,
    "text": _run_text,
    "build": _run_build,
    "render": _run_render,
    "limits": _run_limits,
    "test": _run_test,
}


if __name__ == "__main__":
    sys.exit(main())
