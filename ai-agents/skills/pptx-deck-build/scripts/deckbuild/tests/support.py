from pathlib import Path

import yaml

SKILL_DIR = Path(__file__).resolve().parents[3]
TEMPLATE_SPEC = SKILL_DIR / "assets" / "spec-template.yaml"
FIXTURES = Path(__file__).resolve().parent / "fixtures"


def load_template_data():
    return yaml.safe_load(TEMPLATE_SPEC.read_text(encoding="utf-8"))


def write_spec(directory, data, name="spec.yaml"):
    path = Path(directory) / name
    path.write_text(yaml.safe_dump(data, allow_unicode=True, sort_keys=False), encoding="utf-8")
    return path
