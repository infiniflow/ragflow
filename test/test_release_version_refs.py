#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
import importlib.util
from pathlib import Path


def _load_checker():
    repo_root = Path(__file__).resolve().parents[1]
    checker_path = repo_root / "tools" / "scripts" / "check_release_version_refs.py"
    spec = importlib.util.spec_from_file_location("check_release_version_refs", checker_path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _write_fixture(root: Path, *, readme_version: str = "v1.2.3"):
    version = "v1.2.3"
    migration_dir = "v1_2_3"
    (root / "docker").mkdir(parents=True)
    (root / "tools" / "scripts").mkdir(parents=True)
    (root / "docs" / "administrator" / "migration").mkdir(parents=True)
    (root / "docker" / ".env").write_text(f"RAGFLOW_IMAGE=infiniflow/ragflow:{version}\n", encoding="utf-8")
    (root / "README.md").write_text(
        f"downloads the `{readme_version}` edition\n$ git checkout {version}\n", encoding="utf-8"
    )
    (root / "tools" / "scripts" / "README.md").write_text(
        f"Example: Version `{version}` → Directory `tools/migrate/{migration_dir}/`\n", encoding="utf-8"
    )
    (root / "tools" / "scripts" / "db_schema_sync.py").write_text(
        f"--version {version}\n'{version}' -> '{migration_dir}'\n", encoding="utf-8"
    )
    (root / "docs" / "administrator" / "migration" / "database_schema_and_migration.md").write_text(
        f"tools/migrate/{migration_dir}/\n", encoding="utf-8"
    )


def test_current_release_references_are_consistent():
    module = _load_checker()
    repo_root = Path(__file__).resolve().parents[1]

    assert module.check_release_version_refs(repo_root) == []


def test_release_reference_checker_reports_stale_docker_edition(tmp_path):
    module = _load_checker()
    _write_fixture(tmp_path, readme_version="v0.27.0")

    errors = module.check_release_version_refs(tmp_path)

    assert any("README.md: Docker edition reference" in error for error in errors)
