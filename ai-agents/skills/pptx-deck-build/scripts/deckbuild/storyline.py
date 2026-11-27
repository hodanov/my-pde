from dataclasses import dataclass
from pathlib import Path

import yaml

STORYLINES_FILE = Path(__file__).resolve().parents[2] / "assets" / "storylines.yaml"


@dataclass(frozen=True)
class Role:
    id: str
    label: str
    optional: bool


@dataclass(frozen=True)
class Storyline:
    id: str
    title: str
    roles: tuple[Role, ...]

    def role_ids(self):
        return [role.id for role in self.roles]

    def label_of(self, role_id):
        return next((role.label for role in self.roles if role.id == role_id), role_id)

    def check_chapter_roles(self, chapter_role_ids):
        problems = []
        known = self.role_ids()
        for role_id in chapter_role_ids:
            if role_id not in known:
                problems.append(
                    f"章の role '{role_id}' は storyline '{self.id}' に無い (使える role: {', '.join(known)})"
                )
        used = [role_id for role_id in chapter_role_ids if role_id in known]
        if len(set(used)) != len(used):
            problems.append("章の role が重複している")
        if used != sorted(set(used), key=known.index):
            problems.append(f"章の順序が storyline '{self.id}' の role 順 ({', '.join(known)}) と違う")
        for role in self.roles:
            if not role.optional and role.id not in used:
                problems.append(f"storyline '{self.id}' に必須の章 '{role.id}' ({role.label}) が無い")
        return problems


def load_storylines():
    raw = yaml.safe_load(STORYLINES_FILE.read_text(encoding="utf-8"))
    return {
        storyline_id: Storyline(
            id=storyline_id,
            title=entry["title"],
            roles=tuple(
                Role(role["id"], role["label"], role.get("optional", False))
                for role in entry["roles"]
            ),
        )
        for storyline_id, entry in raw.items()
    }
