import importlib.util
import json
from pathlib import Path

import pytest


SCRIPT = Path(__file__).parents[2] / "tools/scripts/check_uv_lock_metadata.py"
SPEC = importlib.util.spec_from_file_location("check_uv_lock_metadata", SCRIPT)
assert SPEC and SPEC.loader
CHECKER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CHECKER)


def write_metadata(tmp_path, *, project_dependencies, lock_dependencies, requires_dist):
    def strings(values):
        return "[" + ", ".join(json.dumps(value) for value in values) + "]"

    def entries(values):
        return "[" + ", ".join("{ name = " + json.dumps(value) + " }" for value in values) + "]"

    project = tmp_path / "pyproject.toml"
    lock = tmp_path / "uv.lock"
    project.write_text(
        '[project]\nname = "example"\ndependencies = '
        + strings(project_dependencies)
        + "\n"
    )
    lock.write_text(
        '[[package]]\nname = "example"\ndependencies = '
        + entries(lock_dependencies)
        + "\n[package.metadata]\nrequires-dist = "
        + entries(requires_dist)
        + "\n"
    )
    return project, lock


def test_check_accepts_pep503_name_normalization(tmp_path):
    project, lock = write_metadata(
        tmp_path,
        project_dependencies=["Demo_Package>=1"],
        lock_dependencies=["demo-package"],
        requires_dist=["demo-package"],
    )

    assert CHECKER.check_metadata(project, lock) == ({"demo-package"},) * 3


@pytest.mark.parametrize("field", ["lock_dependencies", "requires_dist"])
def test_check_exposes_missing_lock_metadata(tmp_path, field):
    values = {"lock_dependencies": ["demo-package"], "requires_dist": ["demo-package"]}
    values[field] = []
    project, lock = write_metadata(
        tmp_path,
        project_dependencies=["demo-package"],
        **values,
    )

    project_names, lock_names, requires_dist_names = CHECKER.check_metadata(project, lock)
    assert project_names == {"demo-package"}
    assert {"demo-package"} - (lock_names if field == "lock_dependencies" else requires_dist_names)
