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
"""Check release-version references that must agree with docker/.env."""

import argparse
import re
from pathlib import Path


def _read(root: Path, relative_path: str, errors: list[str]) -> str:
    try:
        return (root / relative_path).read_text(encoding="utf-8")
    except OSError as exc:
        errors.append(f"{relative_path}: cannot read file: {exc}")
        return ""


def _configured_version(root: Path, errors: list[str]) -> str:
    env = _read(root, "docker/.env", errors)
    versions = re.findall(r"^RAGFLOW_IMAGE=infiniflow/ragflow:(v\d+\.\d+\.\d+)\s*$", env, re.MULTILINE)
    if len(versions) != 1:
        errors.append(f"docker/.env: expected exactly one pinned RAGFLOW_IMAGE, found {len(versions)}")
        return ""
    return versions[0]


def check_release_version_refs(root: Path) -> list[str]:
    """Return release-reference mismatches for a repository root."""
    errors: list[str] = []
    version = _configured_version(root, errors)
    if not version:
        return errors
    migration_dir = version.replace(".", "_")

    readme = _read(root, "README.md", errors)
    if f"downloads the `{version}` edition" not in readme:
        errors.append(f"README.md: Docker edition reference is not {version}")
    if f"git checkout {version}" not in readme:
        errors.append(f"README.md: checkout reference is not {version}")

    scripts_readme = _read(root, "tools/scripts/README.md", errors)
    expected_location = f"Example: Version `{version}` → Directory `tools/migrate/{migration_dir}/`"
    if expected_location not in scripts_readme:
        errors.append("tools/scripts/README.md: migration directory example does not match the release")

    schema_sync = _read(root, "tools/scripts/db_schema_sync.py", errors)
    concrete_versions = set(re.findall(r"--version\s+(v\d+\.\d+\.\d+)", schema_sync))
    if concrete_versions != {version}:
        errors.append(f"tools/scripts/db_schema_sync.py: --version examples are {sorted(concrete_versions)}, expected {version}")
    if f"'{version}' -> '{migration_dir}'" not in schema_sync:
        errors.append("tools/scripts/db_schema_sync.py: version-to-directory example is stale")

    migration_doc = _read(root, "docs/administrator/migration/database_schema_and_migration.md", errors)
    if f"tools/migrate/{migration_dir}/" not in migration_doc:
        errors.append("database migration documentation: version-specific directory example is stale")

    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[2], help="repository root")
    args = parser.parse_args()
    errors = check_release_version_refs(args.root)
    if errors:
        for error in errors:
            print(f"ERROR: {error}")
        return 1
    print("release-version-refs: passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
