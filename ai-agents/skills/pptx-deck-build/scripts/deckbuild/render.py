import os
import shutil
import subprocess
import tempfile
from pathlib import Path

import pymupdf
from PIL import Image

SOFFICE_APP = Path("/Applications/LibreOffice.app/Contents/MacOS/soffice")
FONTCONFIG_FILES = (
    Path("/opt/homebrew/etc/fonts/fonts.conf"),
    Path("/usr/local/etc/fonts/fonts.conf"),
)
PNG_DPI = 96
SHEET_COLUMNS = 3
SHEET_ROWS = 3
THUMBNAIL_SIZE = (640, 360)
SHEET_GAP = 4


class RenderError(Exception):
    pass


def render_slides(pptx_path, out_dir=None):
    pptx_path = Path(pptx_path).resolve()
    out_dir = Path(out_dir) if out_dir else Path(tempfile.mkdtemp(prefix="pptx-deck-png-"))
    out_dir.mkdir(parents=True, exist_ok=True)
    pdf_path = _convert_to_pdf(pptx_path, out_dir)
    slides = _rasterize(pdf_path, out_dir)
    return out_dir, slides, _contact_sheets(slides, out_dir)


def _soffice_path():
    found = shutil.which("soffice") or (str(SOFFICE_APP) if SOFFICE_APP.exists() else None)
    if found is None:
        raise RenderError("soffice が見つからない (brew install --cask libreoffice)")
    return found


def _convert_to_pdf(pptx_path, out_dir):
    environment = dict(os.environ)
    if "FONTCONFIG_FILE" not in environment:
        fontconfig = next((path for path in FONTCONFIG_FILES if path.exists()), None)
        if fontconfig:
            environment["FONTCONFIG_FILE"] = str(fontconfig)
    profile = Path(tempfile.gettempdir()) / "pptx-deck-build-lo-profile"
    result = subprocess.run(
        [
            _soffice_path(),
            f"-env:UserInstallation={profile.as_uri()}",
            "--headless",
            "--convert-to",
            "pdf",
            "--outdir",
            str(out_dir),
            str(pptx_path),
        ],
        env=environment,
        capture_output=True,
        text=True,
        timeout=180,
        check=False,
    )
    pdf_path = out_dir / f"{pptx_path.stem}.pdf"
    if result.returncode != 0 or not pdf_path.exists():
        raise RenderError(
            "soffice の変換に失敗した。sandbox 内では soffice が何も出力せずに失敗する。"
            "sandbox 外で再実行する\n" + result.stderr.strip()
        )
    return pdf_path


def _rasterize(pdf_path, out_dir):
    paths = []
    with pymupdf.open(pdf_path) as document:
        for number, page in enumerate(document, start=1):
            path = out_dir / f"slide-{number:02d}.png"
            page.get_pixmap(dpi=PNG_DPI).save(path)
            paths.append(path)
    return paths


def _contact_sheets(slides, out_dir):
    per_sheet = SHEET_COLUMNS * SHEET_ROWS
    width, height = THUMBNAIL_SIZE
    sheets = []
    for sheet_number, start in enumerate(range(0, len(slides), per_sheet), start=1):
        sheet = Image.new(
            "RGB",
            (
                SHEET_COLUMNS * (width + SHEET_GAP) + SHEET_GAP,
                SHEET_ROWS * (height + SHEET_GAP) + SHEET_GAP,
            ),
            "#888888",
        )
        for offset, slide_path in enumerate(slides[start : start + per_sheet]):
            with Image.open(slide_path) as slide:
                thumbnail = slide.convert("RGB").resize(THUMBNAIL_SIZE)
            column, row = offset % SHEET_COLUMNS, offset // SHEET_COLUMNS
            sheet.paste(thumbnail, (SHEET_GAP + column * (width + SHEET_GAP), SHEET_GAP + row * (height + SHEET_GAP)))
        path = out_dir / f"sheet-{sheet_number}.png"
        sheet.save(path)
        sheets.append(path)
    return sheets
