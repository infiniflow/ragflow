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
"""Cross-dialect equivalence for the model provider migration.

Builds the same legacy schema and dataset on MySQL and PostgreSQL, runs the
migration exactly the way tools/scripts/run_migrations.sh does, and asserts the
two databases end up holding the same rows. A difference is a porting bug in one
of the dialects.

Requires two live servers and is skipped unless both are configured:

    MIGRATION_TEST_MYSQL_DSN=mysql://root:pw@127.0.0.1:3306/rag_flow_migtest
    MIGRATION_TEST_POSTGRES_DSN=postgres://user:pw@127.0.0.1:5432/rag_flow_migtest

Both databases are emptied before use, so point them at throwaway instances.
"""

import json
import os
import sys
from urllib.parse import urlparse

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "..", "tools", "scripts"))

import mysql_migration as mm
from migration_dialects import MYSQL, POSTGRES

MYSQL_DSN = os.getenv("MIGRATION_TEST_MYSQL_DSN")
POSTGRES_DSN = os.getenv("MIGRATION_TEST_POSTGRES_DSN")

pytestmark = pytest.mark.skipif(
    not (MYSQL_DSN and POSTGRES_DSN),
    reason="set MIGRATION_TEST_MYSQL_DSN and MIGRATION_TEST_POSTGRES_DSN to run",
)

APOS = chr(39)
T1 = "t0000000000000000000000000000001"
T2 = "t0000000000000000000000000000002"

# The two steps in tools/scripts/run_migrations.sh, with the versions it marks.
MIGRATION_STEPS = [
    (["tenant_model_provider", "tenant_model_instance", "tenant_model", "model_id_config"], "v0.26.0"),
    (["tenant_model_seeding", "model_type_merge", "tenant_model_id_migration"], "v0.27.0"),
]

# Legacy schema, in the logical types the dialect layer maps. tenant_llm_id and
# tenant_embd_id are INTEGER on purpose: that is the shape the migration has to
# widen to VARCHAR(32), and the statement doing it is dialect-specific.
LEGACY_TABLES = [
    (
        "tenant_llm",
        [
            ("tenant_id", "VARCHAR(32)", "NOT NULL"),
            ("llm_factory", "VARCHAR(128)", "NOT NULL"),
            ("model_type", "VARCHAR(128)", ""),
            ("llm_name", "VARCHAR(128)", ""),
            ("api_key", "TEXT", ""),
            ("api_base", "VARCHAR(255)", ""),
            ("max_tokens", "INT", ""),
            ("used_tokens", "INT", ""),
            ("status", "VARCHAR(1)", ""),
        ],
        "serial",
    ),
    (
        "system_settings",
        [
            ("name", "VARCHAR(255)", "NOT NULL"),
            ("source", "VARCHAR(64)", ""),
            ("data_type", "VARCHAR(32)", ""),
            ("value", "TEXT", ""),
            ("create_time", "BIGINT", ""),
            ("create_date", "TIMESTAMP", ""),
            ("update_time", "BIGINT", ""),
            ("update_date", "TIMESTAMP", ""),
        ],
        "name",
    ),
    (
        "tenant",
        [
            ("id", "VARCHAR(32)", "NOT NULL"),
            ("llm_id", "VARCHAR(255)", ""),
            ("embd_id", "VARCHAR(255)", ""),
            ("asr_id", "VARCHAR(255)", ""),
            ("img2txt_id", "VARCHAR(255)", ""),
            ("rerank_id", "VARCHAR(255)", ""),
            ("tts_id", "VARCHAR(255)", ""),
            ("tenant_llm_id", "INT", ""),
            ("tenant_embd_id", "INT", ""),
        ],
        None,
    ),
    (
        "knowledgebase",
        [
            ("id", "VARCHAR(32)", "NOT NULL"),
            ("tenant_id", "VARCHAR(32)", ""),
            ("embd_id", "VARCHAR(255)", ""),
            ("parser_config", "TEXT", ""),
        ],
        None,
    ),
    (
        "dialog",
        [
            ("id", "VARCHAR(32)", "NOT NULL"),
            ("tenant_id", "VARCHAR(32)", ""),
            ("llm_id", "VARCHAR(255)", ""),
            ("rerank_id", "VARCHAR(255)", ""),
        ],
        None,
    ),
]

SEED_ROWS = {
    "tenant_llm": (
        ["tenant_id", "llm_factory", "model_type", "llm_name", "api_key", "api_base", "max_tokens", "used_tokens", "status"],
        [
            (T1, "OpenAI", "chat", "gpt-4o", "sk-aaa", "", 8192, 0, "1"),
            (T1, "OpenAI", "embedding", "text-embedding-3-large", "sk-aaa", "", 8192, 0, "1"),
            # status "0" exercises the inactive path.
            (T1, "OpenAI", "rerank", "rerank-1", "sk-aaa", "", 8192, 0, "0"),
            (T1, "Ollama", "embedding", "bge-m3", "", "http://ollama:11434", 8192, 0, "1"),
            (T2, "Ollama", "chat", "qwen3:8b", "", "http://ollama:11434", 8192, 0, "1"),
            # Apostrophes in a model name and an API key: the batch INSERT
            # builders embed these as literals, so both dialects must escape.
            (T2, "LM-Studio", "chat", "weird" + APOS + "name", "sk-b" + APOS + "b", "", 8192, 0, "1"),
        ],
    ),
    "tenant": (
        ["id", "llm_id", "embd_id", "asr_id", "img2txt_id", "rerank_id", "tts_id"],
        [
            (T1, "gpt-4o@OpenAI", "text-embedding-3-large@OpenAI", "", "", "rerank-1@OpenAI", ""),
            (T2, "qwen3:8b@Ollama", "bge-m3@Ollama", "", "", "", ""),
        ],
    ),
    "knowledgebase": (
        ["id", "tenant_id", "embd_id", "parser_config"],
        [
            ("kb001", T1, "text-embedding-3-large@OpenAI", json.dumps({"raptor": {"use_raptor": False}})),
            ("kb002", T2, "bge-m3@Ollama", json.dumps({})),
        ],
    ),
    "dialog": (
        ["id", "tenant_id", "llm_id", "rerank_id"],
        [("dlg01", T1, "gpt-4o@OpenAI", "rerank-1@OpenAI")],
    ),
}

# Compared by value. Generated ids are excluded because they are UUIDs that
# differ per run by design; they are checked for presence instead, below.
COMPARED_TABLES = [
    ("tenant_model_provider", ["provider_name", "tenant_id"]),
    ("tenant_model_instance", ["instance_name", "api_key", "status", "extra"]),
    ("tenant_model", ["model_name", "model_type", "status", "extra"]),
    ("tenant", ["id", "llm_id", "embd_id", "rerank_id"]),
    ("knowledgebase", ["id", "embd_id", "parser_config"]),
    ("dialog", ["id", "llm_id", "rerank_id"]),
    ("system_settings", ["name", "value"]),
]

# Columns holding references to generated ids: compared as set-or-not.
RESOLVED_REFERENCE_COLUMNS = [
    ("tenant", ["tenant_llm_id", "tenant_embd_id"]),
    ("knowledgebase", ["tenant_embd_id"]),
    ("dialog", ["tenant_llm_id", "tenant_rerank_id"]),
]


def _config_from_dsn(dsn: str, db_type: str) -> mm.MigrationConfig:
    parsed = urlparse(dsn)
    return mm.MigrationConfig(
        host=parsed.hostname or "127.0.0.1",
        port=parsed.port,
        user=parsed.username or "root",
        password=parsed.password or "",
        database=(parsed.path or "/rag_flow").lstrip("/"),
        db_type=db_type,
    )


def _reset_and_seed(db: mm.MigrationDatabase):
    d = db.dialect
    created = [spec[0] for spec in LEGACY_TABLES] + [
        "tenant_model_provider",
        "tenant_model_instance",
        "tenant_model",
        "tenant_model_merge_tmp",
        "tenant_model_backup",
    ]
    for table in created:
        db.execute_sql(f"DROP TABLE IF EXISTS {d.quote(table)}")

    for table, columns, key in LEGACY_TABLES:
        parts = []
        if key == "serial":
            serial = "BIGINT AUTO_INCREMENT PRIMARY KEY" if d.name == MYSQL else "BIGSERIAL PRIMARY KEY"
            parts.append(f"{d.quote('id')} {serial}")
        for name, logical_type, extra in columns:
            piece = f"{d.quote(name)} {d.render_type(logical_type)}"
            if extra:
                piece += f" {extra}"
            parts.append(piece)
        if key and key != "serial":
            parts.append(f"PRIMARY KEY ({d.quote(key)})")
        db.execute_sql(f"CREATE TABLE {d.quote(table)} (" + ", ".join(parts) + ")" + d.table_options())

    for table, (columns, rows) in SEED_ROWS.items():
        collist = ", ".join(d.quote(c) for c in columns)
        marks = ", ".join(["%s"] * len(columns))
        for row in rows:
            db.execute_sql(f"INSERT INTO {d.quote(table)} ({collist}) VALUES ({marks})", row)
    db.db.commit()


def _snapshot(db: mm.MigrationDatabase) -> dict:
    d = db.dialect
    out = {}
    for table, columns in COMPARED_TABLES:
        if not db.table_exists(table):
            out[table] = "MISSING"
            continue
        collist = ", ".join(d.quote(c) for c in columns)
        cursor = db.execute_sql(f"SELECT {collist} FROM {d.quote(table)} ORDER BY {collist}")
        out[table] = [tuple("" if v is None else str(v) for v in row) for row in cursor.fetchall()]

    for table, columns in RESOLVED_REFERENCE_COLUMNS:
        for column in columns:
            key = f"{table}.{column}"
            if not db.column_exists(table, column):
                out[key] = "COLUMN MISSING"
                continue
            col = d.quote(column)
            cursor = db.execute_sql(f"SELECT {d.quote('id')}, ({col} IS NOT NULL AND {col} <> '') FROM {d.quote(table)} ORDER BY {d.quote('id')}")
            out[key] = [(str(row[0]), bool(row[1])) for row in cursor.fetchall()]
    return out


def _migrate(config: mm.MigrationConfig, boots: int = 1) -> dict:
    db = mm.MigrationDatabase(config)
    db.connect()
    _reset_and_seed(db)
    db.close()

    for _ in range(boots):
        for stages, version in MIGRATION_STEPS:
            mm.run_migration(
                config,
                stages=stages,
                dry_run=False,
                database_version=version,
                mark_database_version_on_success=True,
            )

    db = mm.MigrationDatabase(config)
    db.connect()
    try:
        return _snapshot(db)
    finally:
        db.close()


@pytest.fixture(scope="module")
def mysql_config():
    return _config_from_dsn(MYSQL_DSN, MYSQL)


@pytest.fixture(scope="module")
def postgres_config():
    return _config_from_dsn(POSTGRES_DSN, POSTGRES)


def test_postgres_and_mysql_migrate_to_the_same_state(mysql_config, postgres_config):
    mysql_state = _migrate(mysql_config)
    postgres_state = _migrate(postgres_config)

    assert set(postgres_state) == set(mysql_state)
    for key in sorted(mysql_state):
        assert postgres_state[key] == mysql_state[key], f"dialects disagree on {key}"


def test_postgres_and_mysql_agree_after_a_second_boot(mysql_config, postgres_config):
    # run_migrations.sh runs on every container start. The version marker is
    # what makes the second run a no-op, so a second boot must not change or
    # duplicate anything, on either dialect.
    mysql_state = _migrate(mysql_config, boots=2)
    postgres_state = _migrate(postgres_config, boots=2)

    for key in sorted(mysql_state):
        assert postgres_state[key] == mysql_state[key], f"dialects disagree on {key} after re-run"


def test_legacy_identifiers_are_rewritten_and_resolvable(postgres_config):
    state = _migrate(postgres_config)

    # The migration normalises model@provider to model@instance@provider, which
    # is what the application's name-based resolution path expects.
    assert ("kb001", "text-embedding-3-large@default@OpenAI", '{"raptor": {"use_raptor": false}}') in state["knowledgebase"]
    assert state["system_settings"] == [("mysql_migration.database.version", "v0.27.0")]

    # An apostrophe in a model name must survive as data, not break the INSERT.
    assert any(row[0] == "weird" + APOS + "name" for row in state["tenant_model"])

    # tenant_embd_id starts out INTEGER and has to end up holding a 32-char id.
    assert dict(state["knowledgebase.tenant_embd_id"])["kb001"] is True
