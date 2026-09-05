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
"""Regression tests for issue #18628.

``docker/migration.sh`` had a hardcoded ``VOLUME_BASES=("mysql_data" "minio_data"
"redis_data" "esdata01")`` and matching ``BACKUP_FILES`` list. The fourth
entry (``esdata01`` / ``es_backup.tar.gz``) only matches the default
``DOC_ENGINE=elasticsearch``; for ``opensearch``, ``infinity`` or
``serenedb`` the script silently skipped the index volume (because the
named volume did not exist), printed "Backup completed successfully!" and
produced a backup set the same script then refused to restore ("Missing
backup files: es_backup.tar.gz").

The fix reads ``DOC_ENGINE`` from the environment and selects the
correct index volume / backup file per engine via
``get_index_volume_for_engine`` in ``docker/migration_volumes.sh``.

These tests source the helper in a sub-bash and pin the mapping for
every engine (including the bind-mount engines that return empty output
because the migration script cannot back up their index data).
"""

import shutil
import subprocess
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[3]
MIGRATION_VOLUMES_SH = REPO_ROOT / "docker" / "migration_volumes.sh"
MIGRATION_SH = REPO_ROOT / "docker" / "migration.sh"


def _call_get_index_volume(engine: str) -> str:
    """Source ``migration_volumes.sh`` in a sub-bash and call
    ``get_index_volume_for_engine`` for the given DOC_ENGINE.
    Returns the function's stdout (with trailing whitespace stripped).
    """
    if not MIGRATION_VOLUMES_SH.is_file():
        pytest.fail(f"missing helper: {MIGRATION_VOLUMES_SH}")
    out = subprocess.run(
        ["bash", "-c", f'source "{MIGRATION_VOLUMES_SH}" && get_index_volume_for_engine "{engine}"'],
        capture_output=True,
        text=True,
        check=True,
    )
    return out.stdout.strip()


def _call_migration_help() -> str:
    """Run ``migration.sh help`` and return stdout.

    The script's ``main()`` calls ``check_docker`` before any
    operation-dispatch, so this test requires Docker to be available.
    Skip if not -- the per-engine mapping tests above already cover the
    helper function in isolation.
    """
    if not MIGRATION_SH.is_file():
        pytest.fail(f"missing script: {MIGRATION_SH}")
    if shutil.which("bash") is None:
        pytest.skip("bash is not available on this platform")
    if shutil.which("docker") is None or subprocess.run(["docker", "info"], capture_output=True, text=True, check=False).returncode != 0:
        pytest.skip("Docker is not available; migration.sh's main() calls check_docker first")
    out = subprocess.run(
        ["bash", str(MIGRATION_SH), "help"],
        capture_output=True,
        text=True,
        check=False,
    )
    return out.stdout + out.stderr


# ---------------------------------------------------------------------------
# Per-engine volume mapping
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    ("engine", "expected_volume_base", "expected_backup_file"),
    [
        ("elasticsearch", "esdata01", "es_backup.tar.gz"),
        ("opensearch", "osdata01", "os_backup.tar.gz"),
        ("infinity", "infinity_data", "infinity_backup.tar.gz"),
        ("serenedb", "serenedb_data", "serenedb_backup.tar.gz"),
    ],
)
def test_named_volume_engines_pick_correct_volume_and_file(engine, expected_volume_base, expected_backup_file):
    """For every engine that uses a named Docker volume, the function
    returns the correct ``<volume_base> <backup_file>`` pair.
    """
    out = _call_get_index_volume(engine)
    parts = out.split()
    assert parts == [expected_volume_base, expected_backup_file], f"DOC_ENGINE={engine!r} should map to {expected_volume_base!r} / {expected_backup_file!r}; got {out!r}"


@pytest.mark.parametrize("engine", ["oceanbase", "seekdb"])
def test_bind_mount_engines_return_empty(engine):
    """oceanbase and seekdb use bind mounts (not named volumes), so the
    migration script cannot back up the index data. The function returns
    empty stdout so the script drops the index entry from its
    VOLUME_BASES / BACKUP_FILES arrays and the restore gate does not
    complain about a missing es_backup.tar.gz.
    """
    out = _call_get_index_volume(engine)
    assert out == "", f"DOC_ENGINE={engine!r} uses bind mounts and the migration script should not attempt an index backup; got {out!r}"


def test_unknown_engine_falls_back_to_elasticsearch():
    """An unrecognised DOC_ENGINE value falls back to the elasticsearch
    mapping (the historical default) so a typo does not silently disable
    the index backup.
    """
    out = _call_get_index_volume("totally-unknown-engine")
    parts = out.split()
    assert parts == ["esdata01", "es_backup.tar.gz"]


def test_default_engine_is_elasticsearch():
    """The function defaults to the elasticsearch mapping when called
    with no argument (mirrors the script's ``${DOC_ENGINE:-elasticsearch}``
    default).
    """
    out = subprocess.run(
        ["bash", "-c", f'source "{MIGRATION_VOLUMES_SH}" && get_index_volume_for_engine'],
        capture_output=True,
        text=True,
        check=True,
    ).stdout.strip()
    assert out.split() == ["esdata01", "es_backup.tar.gz"]


# ---------------------------------------------------------------------------
# migration.sh integration
# ---------------------------------------------------------------------------


def test_migration_sh_help_documents_per_engine_index_volume():
    """The script's ``help`` output must document the per-DOC_ENGINE
    index volume mapping so a user running with DOC_ENGINE=infinity
    knows the right volume / backup file name.
    """
    help_text = _call_migration_help()
    assert "INDEX VOLUME BY DOC_ENGINE" in help_text
    for engine, vol_base, backup_file in [
        ("elasticsearch", "esdata01", "es_backup.tar.gz"),
        ("opensearch", "osdata01", "os_backup.tar.gz"),
        ("infinity", "infinity_data", "infinity_backup.tar.gz"),
        ("serenedb", "serenedb_data", "serenedb_backup.tar.gz"),
    ]:
        assert vol_base in help_text, f"help text must mention {vol_base} for DOC_ENGINE={engine}; got: {help_text!r}"
        assert backup_file in help_text, f"help text must mention {backup_file} for DOC_ENGINE={engine}; got: {help_text!r}"
    assert "oceanbase" in help_text
    assert "seekdb" in help_text
    assert "bind mount" in help_text


def test_migration_sh_help_no_longer_advertises_esdata01_universally():
    """Regression guard: pre-fix, the help text listed ``esdata01`` as
    *the* index volume for every deployment, with no mention of the
    per-engine mapping. The fix replaces that line with a pointer to the
    INDEX VOLUME BY DOC_ENGINE table. This test pins that change so a
    future "fix-up" of the help text does not silently regress.
    """
    help_text = _call_migration_help()
    # The bare line "docker_esdata01       (Elasticsearch indices)" is
    # the pre-fix universal line. The new help text uses "<index volume>"
    # in the volume summary and a per-engine table instead.
    assert "esdata01       (Elasticsearch indices)" not in help_text
    assert "<index volume>" in help_text
