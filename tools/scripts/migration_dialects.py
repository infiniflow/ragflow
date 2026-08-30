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
"""SQL dialect adapters for the model provider migration.

The migration itself is dialect-agnostic Python: it reads rows, rewrites model
identifiers, and writes rows back. Only a small, enumerable set of statements
is dialect-specific — identifier quoting, catalog lookups, upserts, epoch-to-
timestamp conversion, and DDL. Those live here so the stage classes can stay
declarative and so each dialect's rendering is unit-testable without a server.

Adding a dialect means implementing one subclass, not touching the stages.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import ClassVar

from peewee import MySQLDatabase, PostgresqlDatabase
from playhouse.migrate import MySQLMigrator, PostgresqlMigrator

MYSQL = "mysql"
POSTGRES = "postgres"

# DB_TYPE values that are wire-compatible with a dialect we already implement.
# OceanBase speaks the MySQL protocol; SereneDB speaks the PostgreSQL one.
DB_TYPE_ALIASES: dict[str, str] = {
    "mysql": MYSQL,
    "mariadb": MYSQL,
    "oceanbase": MYSQL,
    "postgres": POSTGRES,
    "postgresql": POSTGRES,
    "serenedb": POSTGRES,
}

DEFAULT_PORTS = {MYSQL: 3306, POSTGRES: 5432}

# Connection settings carried over from the deployment's config block, so the
# migration connects on the same terms as the application (which forwards its
# whole block to the driver). TLS is not forced here: the application does not
# require it either, and doing so would break every deployment that does not
# already run TLS.
CONNECT_OPTION_KEYS: dict[str, tuple[str, ...]] = {
    MYSQL: ("ssl_ca", "ssl_cert", "ssl_key", "ssl_verify_cert", "ssl_verify_identity", "connect_timeout"),
    POSTGRES: ("sslmode", "sslrootcert", "sslcert", "sslkey", "connect_timeout"),
}


def normalize_db_type(db_type: str | None) -> str:
    """Map a DB_TYPE / service_conf key onto a dialect name.

    Unknown values fall back to MySQL, preserving the behaviour this script had
    before PostgreSQL support existed.
    """
    if not db_type:
        return MYSQL
    return DB_TYPE_ALIASES.get(db_type.strip().lower(), MYSQL)


@dataclass
class Column:
    """One column in a table this migration creates."""

    name: str
    type: str  # logical type; see SqlDialect.render_type
    null: bool = True
    default: str | None = None  # rendered verbatim, so quote string literals
    primary_key: bool = False


@dataclass
class Index:
    """One index on a table this migration creates."""

    name: str
    columns: list[str]
    unique: bool = False


@dataclass
class TableSpec:
    """A table this migration creates, described once for every dialect."""

    name: str
    columns: list[Column]
    indexes: list[Index] = field(default_factory=list)


class SqlDialect:
    """Renders the dialect-specific statements the migration needs."""

    name: str = ""
    quote_char: str = '"'
    # Logical type -> physical type. Only types this migration actually uses.
    type_map: ClassVar[dict[str, str]] = {}

    def make_database(self, config):
        raise NotImplementedError

    def make_migrator(self, db):
        raise NotImplementedError

    def prepare_session(self, db, config) -> None:
        """Configure a freshly opened connection. No-op unless overridden."""

    # --- identifiers -----------------------------------------------------

    def quote(self, identifier: str) -> str:
        """Quote one identifier.

        Rejects the quote character instead of escaping it: every identifier
        this migration touches is a fixed table or column name from the schema,
        so a quote character in one means a caller bug, not user input.
        """
        if self.quote_char in identifier:
            raise ValueError(f"Refusing to quote identifier containing {self.quote_char!r}: {identifier!r}")
        return f"{self.quote_char}{identifier}{self.quote_char}"

    # --- catalog ---------------------------------------------------------

    def catalog_scope(self, config) -> str:
        """Value to compare against information_schema.*.table_schema.

        MySQL's table_schema is the database; PostgreSQL's is the namespace.
        """
        raise NotImplementedError

    # --- expressions -----------------------------------------------------

    def epoch_to_timestamp(self, seconds_expr: str) -> str:
        """Wrap an epoch-seconds expression so it lands in a timestamp column."""
        raise NotImplementedError

    def upsert_system_setting_sql(self) -> str:
        """INSERT ... ON CONFLICT/DUPLICATE for the migration version marker.

        Takes eight parameters: name, source, data_type, value, create_time,
        create_date (epoch seconds), update_time, update_date (epoch seconds).
        """
        raise NotImplementedError

    # --- DDL -------------------------------------------------------------

    def render_type(self, logical_type: str) -> str:
        """Map a logical type onto this dialect, keeping any length suffix.

        "VARCHAR(32)" is looked up as VARCHAR and re-rendered with its "(32)",
        so specs stay readable and only the base name needs a mapping.
        """
        base, _, rest = logical_type.partition("(")
        mapped = self.type_map.get(base.strip().upper(), base.strip())
        return f"{mapped}({rest}" if rest else mapped

    def table_options(self) -> str:
        return ""

    def supports_inline_index(self) -> bool:
        """Whether CREATE TABLE accepts index definitions in the column list."""
        raise NotImplementedError

    def create_table_statements(self, spec: TableSpec) -> list[str]:
        """Render CREATE TABLE (+ CREATE INDEX where indexes can't be inline).

        Always idempotent: every statement carries IF NOT EXISTS, so a partially
        completed migration can be re-run.
        """
        parts = []
        for col in spec.columns:
            piece = f"{self.quote(col.name)} {self.render_type(col.type)}"
            if not col.null:
                piece += " NOT NULL"
            if col.default is not None:
                piece += f" DEFAULT {col.default}"
            if col.primary_key:
                piece += " PRIMARY KEY"
            parts.append(piece)

        statements = []
        if self.supports_inline_index():
            for idx in spec.indexes:
                cols = ", ".join(self.quote(c) for c in idx.columns)
                kind = "UNIQUE INDEX" if idx.unique else "INDEX"
                parts.append(f"{kind} {self.quote(idx.name)} ({cols})")
        body = ",\n    ".join(parts)
        statements.append(f"CREATE TABLE IF NOT EXISTS {self.quote(spec.name)} (\n    {body}\n){self.table_options()}")

        if not self.supports_inline_index():
            for idx in spec.indexes:
                cols = ", ".join(self.quote(c) for c in idx.columns)
                kind = "CREATE UNIQUE INDEX" if idx.unique else "CREATE INDEX"
                statements.append(f"{kind} IF NOT EXISTS {self.quote(idx.name)} ON {self.quote(spec.name)} ({cols})")
        return statements

    def add_surrogate_id_statements(self, table: str, column: str) -> list[str]:
        """Add a unique auto-incrementing column, back-filling existing rows.

        UNIQUE rather than PRIMARY KEY: the tables this runs against are old
        enough to predate the column but may already have a primary key.
        """
        raise NotImplementedError

    def relax_to_nullable_varchar_statements(self, table: str, column: str, length: int) -> list[str]:
        """Widen a column to a nullable VARCHAR(length), whatever it was before."""
        raise NotImplementedError


class MySQLDialect(SqlDialect):
    name = MYSQL
    quote_char = "`"
    type_map: ClassVar[dict[str, str]] = {
        "VARCHAR": "VARCHAR",
        "TEXT": "TEXT",
        "INT": "INT",
        "BIGINT": "BIGINT",
        "TIMESTAMP": "DATETIME",
    }

    def make_database(self, config):
        return MySQLDatabase(
            config.database,
            host=config.host,
            port=config.port,
            user=config.user,
            password=config.password,
            charset="utf8mb4",
            **config.connect_options,
        )

    def make_migrator(self, db):
        return MySQLMigrator(db)

    def catalog_scope(self, config) -> str:
        return config.database

    def epoch_to_timestamp(self, seconds_expr: str) -> str:
        return f"FROM_UNIXTIME({seconds_expr})"

    def upsert_system_setting_sql(self) -> str:
        return """
        INSERT INTO `system_settings`
        (`name`, `source`, `data_type`, `value`, `create_time`, `create_date`, `update_time`, `update_date`)
        VALUES (%s, %s, %s, %s, %s, FROM_UNIXTIME(%s), %s, FROM_UNIXTIME(%s))
        ON DUPLICATE KEY UPDATE
          `source` = VALUES(`source`),
          `data_type` = VALUES(`data_type`),
          `value` = VALUES(`value`),
          `update_time` = VALUES(`update_time`),
          `update_date` = VALUES(`update_date`)
        """

    def table_options(self) -> str:
        return " ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"

    def supports_inline_index(self) -> bool:
        return True

    def add_surrogate_id_statements(self, table: str, column: str) -> list[str]:
        # MySQL back-fills existing rows with sequential values automatically.
        return [f"ALTER TABLE {self.quote(table)} ADD COLUMN {self.quote(column)} BIGINT AUTO_INCREMENT UNIQUE"]

    def relax_to_nullable_varchar_statements(self, table: str, column: str, length: int) -> list[str]:
        return [f"ALTER TABLE {self.quote(table)} MODIFY COLUMN {self.quote(column)} VARCHAR({length}) NULL"]


class PostgresDialect(SqlDialect):
    name = POSTGRES
    quote_char = '"'
    type_map: ClassVar[dict[str, str]] = {
        "VARCHAR": "VARCHAR",
        "TEXT": "TEXT",
        "INT": "INTEGER",
        "BIGINT": "BIGINT",
        "TIMESTAMP": "TIMESTAMP",
    }

    def make_database(self, config):
        return PostgresqlDatabase(
            config.database,
            host=config.host,
            port=config.port,
            user=config.user,
            password=config.password,
            **config.connect_options,
        )

    def make_migrator(self, db):
        return PostgresqlMigrator(db)

    def prepare_session(self, db, config) -> None:
        """Bind the session to the schema the catalog lookups inspect.

        Unqualified DDL otherwise lands in whatever the server's default
        search_path resolves to, so a non-public deployment would check one
        namespace and create tables in another.
        """
        db.execute_sql(f"SET search_path TO {self.quote(config.schema)}")

    def catalog_scope(self, config) -> str:
        # information_schema.table_schema is the namespace, not the database.
        return config.schema

    def epoch_to_timestamp(self, seconds_expr: str) -> str:
        return f"to_timestamp({seconds_expr})"

    def upsert_system_setting_sql(self) -> str:
        # ON CONFLICT needs a unique constraint on "name". system_settings is
        # created by peewee from db_models.SystemSettings, where name is the
        # primary key, so the inference target is always present.
        return """
        INSERT INTO "system_settings"
        ("name", "source", "data_type", "value", "create_time", "create_date", "update_time", "update_date")
        VALUES (%s, %s, %s, %s, %s, to_timestamp(%s), %s, to_timestamp(%s))
        ON CONFLICT ("name") DO UPDATE SET
          "source" = EXCLUDED."source",
          "data_type" = EXCLUDED."data_type",
          "value" = EXCLUDED."value",
          "update_time" = EXCLUDED."update_time",
          "update_date" = EXCLUDED."update_date"
        """

    def supports_inline_index(self) -> bool:
        return False

    def add_surrogate_id_statements(self, table: str, column: str) -> list[str]:
        # Adding a BIGSERIAL column rewrites the table and assigns nextval() per
        # existing row, which is the back-fill MySQL gives us for free.
        return [
            f"ALTER TABLE {self.quote(table)} ADD COLUMN {self.quote(column)} BIGSERIAL",
            f"ALTER TABLE {self.quote(table)} ADD CONSTRAINT {self.quote(f'{table}_{column}_key')} UNIQUE ({self.quote(column)})",
        ]

    def relax_to_nullable_varchar_statements(self, table: str, column: str, length: int) -> list[str]:
        # PostgreSQL splits MySQL's MODIFY COLUMN into a type change and a
        # nullability change, and needs an explicit cast off non-text types.
        return [
            f"ALTER TABLE {self.quote(table)} ALTER COLUMN {self.quote(column)} TYPE VARCHAR({length}) USING {self.quote(column)}::VARCHAR({length})",
            f"ALTER TABLE {self.quote(table)} ALTER COLUMN {self.quote(column)} DROP NOT NULL",
        ]


DIALECTS = {MYSQL: MySQLDialect, POSTGRES: PostgresDialect}


def get_dialect(db_type: str | None) -> SqlDialect:
    return DIALECTS[normalize_db_type(db_type)]()
