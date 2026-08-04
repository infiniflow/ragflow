"""
Direct tests for each GaussDB metadata database adaptation point.
"""

import logging
from pathlib import Path

from peewee import ProgrammingError
from ruamel.yaml import YAML

REPO_ROOT = Path(__file__).resolve().parents[4]

from api.db import db_models
from api.db.services import connector_service
from common import settings


def read_repo_file(path: str) -> str:
    return (REPO_ROOT / path).read_text()


def load_yaml(path: str):
    yaml = YAML(typ="safe", pure=True)
    return yaml.load(read_repo_file(path))


def test_gaussdb_duplicate_column_migration_is_idempotent(monkeypatch, caplog):
    class Migrator:
        def add_column(self, table_name, column_name, column_type):
            return table_name, column_name, column_type

    def duplicate_column(*_operations):
        raise ProgrammingError('column "f_extra" already exists')

    monkeypatch.setattr(db_models, "migrate", duplicate_column)
    monkeypatch.setattr(settings, "DATABASE_TYPE", "gaussdb")

    caplog.set_level(logging.CRITICAL)
    db_models.alter_db_add_column(Migrator(), "tenant", "f_extra", object())

    assert not caplog.records


def test_gaussdb_migration_logs_unrelated_programming_errors(monkeypatch, caplog):
    class Migrator:
        def add_column(self, table_name, column_name, column_type):
            return table_name, column_name, column_type

    def syntax_error(*_operations):
        raise ProgrammingError("syntax error near invalid_sql")

    monkeypatch.setattr(db_models, "migrate", syntax_error)
    monkeypatch.setattr(settings, "DATABASE_TYPE", "gaussdb")

    caplog.set_level(logging.CRITICAL)
    db_models.alter_db_add_column(Migrator(), "tenant", "f_extra", object())

    assert "syntax error near invalid_sql" in caplog.text


def test_gaussdb_empty_string_compatible_migration_drops_not_null_only_for_gaussdb(monkeypatch):
    class FakeDB:
        queries = []

        @classmethod
        def execute_sql(cls, sql, params=None):
            cls.queries.append((sql, params))

    monkeypatch.setattr(db_models, "DB", FakeDB)
    monkeypatch.setattr(settings, "DATABASE_TYPE", "gaussdb")

    db_models.relax_gaussdb_empty_string_compatible_columns()

    expected_query_count = sum(len(columns) for _, columns in db_models.GAUSSDB_EMPTY_STRING_COMPATIBLE_COLUMNS)
    assert len(FakeDB.queries) == expected_query_count
    assert ('ALTER TABLE "user" ALTER COLUMN "nickname" DROP NOT NULL', None) in FakeDB.queries
    assert ('ALTER TABLE "tenant" ALTER COLUMN "llm_id" DROP NOT NULL', None) in FakeDB.queries
    assert ('ALTER TABLE "system_settings" ALTER COLUMN "value" DROP NOT NULL', None) in FakeDB.queries
    assert ('ALTER TABLE "task" ALTER COLUMN "task_type" DROP NOT NULL', None) in FakeDB.queries
    assert ('ALTER TABLE "sync_logs" ALTER COLUMN "error_msg" DROP NOT NULL', None) in FakeDB.queries
    assert ('ALTER TABLE "sync_logs" ALTER COLUMN "full_exception_trace" DROP NOT NULL', None) in FakeDB.queries
    assert ('ALTER TABLE "api_4_conversation" ALTER COLUMN "user_id" DROP NOT NULL', None) in FakeDB.queries
    assert ('ALTER TABLE "user_canvas" ALTER COLUMN "tags" DROP NOT NULL', None) in FakeDB.queries

    monkeypatch.setattr(settings, "DATABASE_TYPE", "mysql")
    db_models.relax_gaussdb_empty_string_compatible_columns()

    assert len(FakeDB.queries) == expected_query_count


def test_gaussdb_unique_email_migration_checks_unique_email_index_not_fixed_name(monkeypatch):
    class Cursor:
        def fetchone(self):
            return (1,)

    class FakeDB:
        queries = []
        params = []

        @classmethod
        def execute_sql(cls, sql, params=None):
            cls.queries.append(sql)
            cls.params.append(params)
            return Cursor()

    monkeypatch.setattr(settings, "DATABASE_TYPE", "gaussdb")
    monkeypatch.setattr(db_models, "DB", FakeDB)

    db_models.migrate_add_unique_email(object())

    assert len(FakeDB.queries) == 1
    assert "pg_indexes" in FakeDB.queries[0]
    assert "lower(indexdef) LIKE %s" in FakeDB.queries[0]
    assert FakeDB.params[0] == ("create unique index%", "%(email)%", '%("email")%')
    assert "indexname = 'user_email'" not in FakeDB.queries[0]


def test_gaussdb_deadlock_retry_recognizes_psycopg_sqlstates(monkeypatch):
    from peewee import OperationalError
    from api.db.services.common_service import _is_deadlock_error

    class PgError:
        def __init__(self, pgcode):
            self.pgcode = pgcode

    monkeypatch.setattr(settings, "DATABASE_TYPE", "gaussdb")

    assert _is_deadlock_error(OperationalError(PgError("40P01")))
    assert _is_deadlock_error(OperationalError(PgError("40001")))
    assert _is_deadlock_error(OperationalError(PgError("55P03")))
    assert not _is_deadlock_error(OperationalError("syntax error"))


def test_connector_poll_interval_uses_gaussdb_compatible_sql(monkeypatch):
    monkeypatch.setattr(connector_service.settings, "DATABASE_TYPE", "gaussdb")
    expr = connector_service._gaussdb_poll_interval_expr("refresh_freq")

    assert "t2.refresh_freq * INTERVAL '1 minute'" in expr.sql
    assert "make_interval" not in expr.sql
    assert "`t2`" not in expr.sql


def test_health_check_uses_generic_probe_for_gaussdb(monkeypatch):
    from api.utils import health_utils

    class Cursor:
        def fetchone(self):
            return (1,)

        def close(self):
            pass

    class FakeDB:
        queries = []

        @classmethod
        def execute_sql(cls, sql):
            cls.queries.append(sql)
            return Cursor()

    monkeypatch.setattr(settings, "DATABASE_TYPE", "gaussdb")
    monkeypatch.setattr(health_utils, "DB", FakeDB)

    status = health_utils.get_mysql_status()

    assert status == {
        "status": "alive",
        "message": {
            "database": "gaussdb",
            "result": 1,
        },
    }
    assert FakeDB.queries == ["SELECT 1;"]


def test_admin_config_selects_gaussdb_metadata_entry(monkeypatch):
    from admin.server import config as admin_config

    monkeypatch.setenv("DB_TYPE", "GaussDB")
    monkeypatch.setenv("GAUSSDB_METADATA_HOST", "metadata.example.com")
    monkeypatch.setenv("GAUSSDB_METADATA_PORT", "8000")
    monkeypatch.setenv("GAUSSDB_METADATA_USER", "metadata_user")
    monkeypatch.setenv("GAUSSDB_METADATA_PASSWORD", "metadata-secret")
    monkeypatch.setenv("GAUSSDB_METADATA_SCHEMA", "metadata_schema")
    monkeypatch.setattr(
        admin_config,
        "read_config",
        lambda _path: {
            "mysql": {"host": "mysql", "port": 3306, "user": "root", "password": "mysql-secret"},
            "gaussdb": {
                "host": "doc.example.com",
                "port": 19995,
                "database": "doc_db",
                "user": "doc_user",
                "password": "doc-secret",
                "schema": "doc_schema",
            },
        },
    )

    configs = admin_config.load_configurations("ignored.yaml")
    metadata_configs = [cfg for cfg in configs if cfg.service_type == "meta_data"]
    retrieval_configs = [cfg for cfg in configs if cfg.service_type == "retrieval"]

    assert len(metadata_configs) == 1
    assert len(retrieval_configs) == 1
    assert metadata_configs[0].name == "gaussdb"
    assert metadata_configs[0].meta_type == "gaussdb"
    assert metadata_configs[0].host == "metadata.example.com"
    assert metadata_configs[0].username == "metadata_user"
    assert metadata_configs[0].metadata_schema == "metadata_schema"
    assert metadata_configs[0].detail_func_name == "get_database_status"

    retrieval = retrieval_configs[0]
    assert retrieval.host == "doc.example.com"
    assert retrieval.port == 19995
    assert retrieval.database == "doc_db"
    assert retrieval.retrieval_schema == "doc_schema"
    assert retrieval.detail_func_name == "get_gaussdb_status"
    assert "password" not in str(retrieval.to_dict()).lower()


def test_admin_service_safe_serialization_redacts_credentials():
    from admin.server import config as admin_config

    metadata = admin_config.GaussDBMetadataConfig(
        id=1,
        name="gaussdb",
        host="metadata.example.com",
        port=8000,
        username="metadata_user",
        password="metadata-secret",
        metadata_schema="metadata_schema",
        service_type="meta_data",
        meta_type="gaussdb",
        detail_func_name="get_database_status",
    )

    serialized = metadata.to_dict()

    assert serialized["extra"]["username"] == "metadata_user"
    assert serialized["extra"]["schema"] == "metadata_schema"
    assert serialized["extra"]["password"] == "********"
    assert "metadata-secret" not in str(serialized)


def test_admin_config_falls_back_to_gaussdb_env(monkeypatch):
    from admin.server import config as admin_config

    monkeypatch.setenv("DB_TYPE", "gaussdb")
    monkeypatch.setenv("GAUSSDB_METADATA_HOST", "gaussdb-env.example.com")
    monkeypatch.setenv("GAUSSDB_METADATA_PORT", "19995")
    monkeypatch.setenv("GAUSSDB_METADATA_USER", "zws")
    monkeypatch.setenv("GAUSSDB_METADATA_PASSWORD", "env-secret")
    monkeypatch.setenv("GAUSSDB_METADATA_SCHEMA", "env_schema")
    monkeypatch.setattr(admin_config, "read_config", lambda _path: {})

    configs = admin_config.load_configurations("ignored.yaml")
    metadata_configs = [cfg for cfg in configs if cfg.service_type == "meta_data"]

    assert len(metadata_configs) == 1
    assert metadata_configs[0].name == "gaussdb"
    assert metadata_configs[0].host == "gaussdb-env.example.com"
    assert metadata_configs[0].port == 19995
    assert metadata_configs[0].username == "zws"
    assert metadata_configs[0].metadata_schema == "env_schema"


def test_docker_config_files_expose_gaussdb_metadata_settings():
    env_text = read_repo_file("docker/.env")
    template = read_repo_file("docker/service_conf.yaml.template")
    source_conf = read_repo_file("conf/service_conf.yaml")

    assert "DB_TYPE=${DB_TYPE:-mysql}" in env_text
    assert "GAUSSDB_METADATA_HOST=" in env_text
    assert "GAUSSDB_METADATA_PASSWORD=" in env_text
    assert "GAUSSDB_METADATA_SCHEMA=public" in env_text
    assert "gaussdb:" in template
    assert template.count("\ngaussdb:") == 1
    assert "GAUSSDB_METADATA_DBNAME" not in template
    assert "gaussdb:" not in source_conf


def test_docker_compose_metadata_profile_does_not_force_mysql_for_gaussdb():
    env_text = read_repo_file("docker/.env")
    base = load_yaml("docker/docker-compose-base.yml")
    compose = load_yaml("docker/docker-compose.yml")
    macos_compose = load_yaml("docker/docker-compose-macos.yml")
    cn_compose = load_yaml("docker/docker-compose-CN-oc9.yml")

    assert "METADATA_DB_PROFILE=${METADATA_DB_PROFILE:-mysql}" in env_text
    assert "COMPOSE_PROFILES=${DOC_ENGINE},${DEVICE},metadata-${METADATA_DB_PROFILE}" in env_text
    assert "metadata-mysql" in base["services"]["mysql"]["profiles"]
    assert "metadata-gaussdb" not in base["services"]["mysql"]["profiles"]

    for service_name in ["ragflow-cpu", "ragflow-gpu"]:
        mysql_dep = compose["services"][service_name]["depends_on"]["mysql"]
        assert mysql_dep["condition"] == "service_healthy"
        assert mysql_dep["required"] is False

    assert macos_compose["services"]["ragflow"]["depends_on"]["mysql"]["required"] is False
    for service_name in ["ragflow-cpu", "ragflow-gpu"]:
        assert cn_compose["services"][service_name]["depends_on"]["mysql"]["required"] is False


def test_docker_launch_scripts_skip_mysql_migration_for_gaussdb():
    entrypoint = read_repo_file("docker/entrypoint.sh")
    launcher = read_repo_file("docker/launch_backend_service.sh")

    assert 'DB_TYPE_NORMALIZED="${DB_TYPE:-mysql}"' in entrypoint
    assert 'if [[ "${DB_TYPE_NORMALIZED}" == "gaussdb" || "${DB_TYPE_NORMALIZED}" == "gauss" ]]; then' in entrypoint
    assert "Skipping MySQL-specific model provider table migrations" in entrypoint

    assert 'local db_type="${DB_TYPE:-mysql}"' in launcher
    assert 'if [ "$db_type" = "gaussdb" ] || [ "$db_type" = "gauss" ]; then' in launcher
    assert "Skipping MySQL-specific model provider table migrations" in launcher


def test_helm_values_and_template_do_not_require_mysql_for_gaussdb():
    values = load_yaml("helm/values.yaml")
    env_template = read_repo_file("helm/templates/env.yaml")

    assert values["env"]["DB_TYPE"] == "mysql"
    assert values["env"]["GAUSSDB_METADATA_SCHEMA"] == "public"
    assert '{{- $dbType := (default "mysql" .Values.env.DB_TYPE | lower) }}' in env_template
    assert '{{- $isGaussDB := or (eq $dbType "gaussdb") (eq $dbType "gauss") }}' in env_template
    assert "{{- if not $isGaussDB }}" in env_template
    assert 'required "env.MYSQL_HOST is required when mysql.enabled=false"' in env_template
    assert "{{- if or (not $isGaussDB) .Values.mysql.enabled }}" in env_template
    assert 'required "MYSQL_PASSWORD is required"' in env_template
