"""Real previous-release PostgreSQL initializer, data preservation and restore.

Opt in only with a dedicated disposable cluster: RAGFLOW_UPGRADE_TEST_POSTGRES_DSN,
RAGFLOW_UPGRADE_TEST_POSTGRES_CONTAINER, RAGFLOW_UPGRADE_TEST_DISPOSABLE=1.
No application/shared database is changed: randomly named t1_upgrade_* DBs are
created and dropped, and cleanup is verified. Uses installed dependencies for
both revisions; this is data compatibility, not old release dependency replay.
"""

import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tarfile
import tomllib
from uuid import uuid4

import psycopg2
from psycopg2 import sql
from psycopg2.extensions import parse_dsn
import pytest


ROOT = Path(__file__).resolve().parents[2]
PREVIOUS = "3cf71a547e2a7610da908163a19a0e2d595e01c8"
PROBE = Path(__file__).parent / "fixtures" / "upgrade_data_probe.py"


def _sorted_rows(rows):
    return sorted(rows, key=lambda value: json.dumps(value, sort_keys=True, default=str))


def assert_previous_rows_preserved(previous, current):
    """Compare previous-release columns while allowing additive current columns/tables."""

    for table_name, previous_rows in previous.items():
        assert table_name in current, f"Previous-release table disappeared: {table_name}"
        if not previous_rows:
            assert current[table_name] == [], f"Upgrade unexpectedly populated {table_name}"
            continue
        previous_columns = set().union(*(row.keys() for row in previous_rows))
        projected = [{column: row[column] for column in previous_columns} for row in current[table_name]]
        assert _sorted_rows(projected) == _sorted_rows(previous_rows), f"Previous-release rows changed in {table_name}"


def assert_business_document_additions(snapshot):
    document = snapshot["business_document"]
    assert len(document) == 1
    assert document[0]["title_key"] == hashlib.sha256("synthetic requirements".encode()).hexdigest()
    bindings = snapshot["business_document_eva_binding"]
    assert len(bindings) == 1
    binding = bindings[0]["binding"]
    assert binding["last_pulled_content_hash"] == "sha256:t1-remote-content"
    assert binding["last_pull_event_id"] == "t1-pull-event"
    assert binding["last_pull_review_cycle"] == 2
    assert binding["remote_version"] == "7"
    assert snapshot["business_document_export_stage"] == []


def _run(arguments, *, cwd=ROOT, env=None, input_data=None):
    result = subprocess.run(arguments, cwd=cwd, env=env, input=input_data, capture_output=True, timeout=180, check=False)
    assert result.returncode == 0, result.stderr.decode("utf-8", "replace")[-5000:]
    return result.stdout


@pytest.mark.p0
def test_previous_release_data_survives_initializer_and_dump_restore(tmp_path):
    dsn = os.getenv("RAGFLOW_UPGRADE_TEST_POSTGRES_DSN")
    container = os.getenv("RAGFLOW_UPGRADE_TEST_POSTGRES_CONTAINER")
    if not dsn or not container or os.getenv("RAGFLOW_UPGRADE_TEST_DISPOSABLE") != "1":
        pytest.skip("Requires explicitly disposable PostgreSQL cluster and its container for dump/restore")
    params = parse_dsn(dsn)
    assert params.get("host") in {"127.0.0.1", "localhost", "::1"}, "Disposable cluster must be loopback-only"
    assert re.fullmatch(r"[A-Za-z0-9_.-]+", container)
    history_repo = Path(os.getenv("RAGFLOW_UPGRADE_TEST_HISTORY_REPO", ROOT)).resolve()
    previous_project = _run(["git", "show", f"{PREVIOUS}:pyproject.toml"], cwd=history_repo)
    assert tomllib.loads(previous_project.decode())["project"]["version"] == "1.12.0"
    candidate_commit = os.getenv("RAGFLOW_UPGRADE_TEST_CANDIDATE_COMMIT") or _run(["git", "rev-parse", "HEAD"], cwd=history_repo).decode().strip()
    _run(["git", "merge-base", "--is-ancestor", PREVIOUS, candidate_commit], cwd=history_repo)
    old_source = tmp_path / "previous"
    old_source.mkdir()
    archive = tmp_path / "previous.tar"
    _run(["git", "archive", "--format=tar", f"--output={archive}", PREVIOUS, "api", "common", "rag", "memory", "conf"], cwd=history_repo)
    with tarfile.open(archive) as source:
        source.extractall(old_source, filter="data")
    # Runtime tokenizer bytes are provisioned separately from Git in both releases.
    encoding = Path(os.getenv("RAGFLOW_UPGRADE_TEST_TOKENIZER", ROOT / "ragflow_deps" / "cl100k_base.tiktoken"))
    assert encoding.is_file(), "Provision cl100k_base.tiktoken before this offline lane"
    # Token caches/config runtime copies are writable; source snapshots stay untouched.
    runtimes = {}
    for label, source in (("old-runtime", old_source), ("current-runtime", ROOT)):
        runtime = tmp_path / label
        (runtime / "conf").mkdir(parents=True)
        (runtime / "ragflow_deps").mkdir()
        shutil.copyfile(source / "conf" / "service_conf.yaml", runtime / "conf" / "service_conf.yaml")
        shutil.copyfile(encoding, runtime / "ragflow_deps" / "cl100k_base.tiktoken")
        runtimes[source] = runtime
    evidence_dir = Path(os.environ.get("RAGFLOW_UPGRADE_TEST_EVIDENCE_DIR", tmp_path / "evidence"))
    evidence_dir.mkdir(parents=True, exist_ok=True)
    token = uuid4().hex[:16]
    names = [f"t1_upgrade_{token}_{suffix}" for suffix in ("main", "current_restore", "failed", "rollback")]
    admin = psycopg2.connect(dsn)
    admin.autocommit = True
    created = []
    environment = {
        **os.environ,
        "DB_TYPE": "postgres",
        "LITELLM_LOCAL_MODEL_COST_MAP": "True",
        "HF_HUB_OFFLINE": "1",
        "TRANSFORMERS_OFFLINE": "1",
        "RAGFLOW_CREDENTIALS_KEY": "t1-synthetic-credential-key",
        "PGPASSWORD": params.get("password", ""),
        "PYTHONDONTWRITEBYTECODE": "1",
    }

    def probe(source, mode, database, label):
        output = evidence_dir / f"{label}.json"
        config = {
            "name": database,
            "host": params["host"],
            "port": int(params.get("port", 5432)),
            "user": params.get("user", "postgres"),
            "password": params.get("password", ""),
            "options": "-c statement_timeout=30000 -c lock_timeout=10000",
        }
        env = {**environment, "RAGFLOW_UPGRADE_PROBE_DB": json.dumps(config), "RAG_PROJECT_BASE": str(runtimes[source])}
        result = subprocess.run([sys.executable, str(PROBE), str(source), mode, str(output)], env=env, cwd=source, capture_output=True, timeout=180, check=False)
        (evidence_dir / f"{label}.log").write_bytes(result.stdout + result.stderr)
        assert result.returncode == 0, f"{label} failed; see {evidence_dir / (label + '.log')}"
        return json.loads(output.read_text(encoding="utf-8"))

    def postgres_tool(tool, database, data=None):
        arguments = ["docker", "exec", "-i", "-e", "PGPASSWORD", container, tool, "-U", params.get("user", "postgres"), "-d", database]
        arguments += ["-Fc", "--no-owner", "--no-acl"] if tool == "pg_dump" else ["--exit-on-error", "--no-owner", "--no-acl"]
        return _run(arguments, env=environment, input_data=data)

    try:
        with admin.cursor() as cursor:
            for name in names:
                cursor.execute(sql.SQL("CREATE DATABASE {} TEMPLATE template0").format(sql.Identifier(name)))
                created.append(name)
        old = probe(old_source, "seed", names[0], "old-v1.12.0")
        assert old["business_document_revision"] and old["business_document_evidence_snapshot"] and old["system_audit_event"]
        assert not any(name.startswith("access_group") for name in old), "AccessGroup did not exist in verified v1.12.0"
        old_dump = postgres_tool("pg_dump", names[0])
        (evidence_dir / "old.dump").write_bytes(old_dump)
        upgraded = probe(ROOT, "upgrade", names[0], "current-upgraded")
        assert_previous_rows_preserved(old, upgraded)
        assert_business_document_additions(upgraded)
        for name in ("access_group", "access_group_user", "access_group_dataset", "access_group_section"):
            assert upgraded[name] == []
        grouped = probe(ROOT, "groups", names[0], "current-with-grants")
        assert_previous_rows_preserved(old, grouped)
        assert len(grouped["access_group_section"]) == 2
        repeated = probe(ROOT, "upgrade", names[0], "current-repeat")
        assert repeated == grouped, "Repeated initializer changed persisted rows"
        current_dump = postgres_tool("pg_dump", names[0])
        (evidence_dir / "current.dump").write_bytes(current_dump)
        postgres_tool("pg_restore", names[1], current_dump)
        assert probe(ROOT, "read", names[1], "current-restored") == grouped

        postgres_tool("pg_restore", names[2], old_dump)
        failed = probe(ROOT, "fail_upgrade", names[2], "failed-upgrade")
        assert failed["injected_failure_detected"] is True
        assert_previous_rows_preserved(old, failed["snapshot"])
        postgres_tool("pg_restore", names[3], old_dump)
        assert probe(old_source, "read", names[3], "old-rollback-restored") == old
        recovered = probe(ROOT, "upgrade", names[3], "recovered-upgrade")
        assert_previous_rows_preserved(old, recovered)
        assert_business_document_additions(recovered)
        (evidence_dir / "result.json").write_text(
            json.dumps(
                {
                    "previous_commit": PREVIOUS,
                    "previous_version": "1.12.0",
                    "candidate_commit": candidate_commit,
                    "candidate_models_sha256": hashlib.sha256((ROOT / "api" / "db" / "db_models.py").read_bytes()).hexdigest(),
                    "old_tables_checked": len(old),
                    "current_tables_checked": len(grouped),
                    "data_preserved": True,
                    "failed_initializer_detected": True,
                    "old_restore_verified": True,
                    "current_grants_restore_verified": True,
                    "scope": "Actual DB initializer and synthetic model rows; no object-store bytes, API scenario or deployment-script replay",
                },
                indent=2,
            ),
            encoding="utf-8",
        )
    finally:
        with admin.cursor() as cursor:
            for name in created:
                assert name.startswith(f"t1_upgrade_{token}_")
                cursor.execute(sql.SQL("DROP DATABASE {} WITH (FORCE)").format(sql.Identifier(name)))
            cursor.execute("SELECT datname FROM pg_database WHERE datname = ANY(%s)", (created,))
            assert cursor.fetchall() == [], "Disposable DB cleanup failed"
        admin.close()
