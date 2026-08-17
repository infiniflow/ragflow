#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
"""Read-only SQL validation for LLM-generated queries."""

import logging
import re

import sqlglot
from sqlglot import expressions as exp
from sqlglot.errors import SqlglotError

logger = logging.getLogger(__name__)

_ALLOWED_READ_EXPRESSIONS = (
    exp.Select,
    exp.Union,
    exp.Except,
    exp.Intersect,
)

_CODE_FENCE_RE = re.compile(r"^\s*```(?:\w+)?\s*\n?(.*?)\n?```\s*$", re.DOTALL)
_ID_MARKER_RE = re.compile(r"\[ID:[0-9]+\]")

_DOC_ENGINE_DIALECTS = {
    "infinity": "postgres",
    "oceanbase": "mysql",
    "es": None,
}


class ReadOnlySqlViolation(ValueError):
    """Raised when SQL is not a single read-only SELECT-style statement."""


def sqlglot_dialect_for_doc_engine(doc_engine: str) -> str | None:
    """Map RAGFlow doc engine name to a sqlglot dialect."""
    return _DOC_ENGINE_DIALECTS.get(doc_engine)


def normalize_sql_for_validation(sql: str) -> str:
    """Strip code fences and ID markers before parsing."""
    sql = (sql or "").strip()
    match = _CODE_FENCE_RE.match(sql)
    if match:
        sql = match.group(1).strip()
    return _ID_MARKER_RE.sub("", sql).strip()


def _parse_sql_statements(sql: str, dialect: str | None) -> list[exp.Expression]:
    try:
        return [statement for statement in sqlglot.parse(sql, read=dialect) if statement]
    except SqlglotError as e:
        logger.warning(
            "SQL validation rejected: dialect=%s, stage=parse, reason=%s",
            dialect,
            type(e).__name__,
        )
        raise ReadOnlySqlViolation(f"Invalid SQL statement: {e}") from e


def ensure_read_only_sql(sql: str, *, doc_engine: str | None = None, dialect: str | None = None) -> None:
    """Validate that *sql* is exactly one read-only SELECT-style statement.

    Args:
        sql: SQL text (typically from an LLM).
        doc_engine: RAGFlow doc engine (`infinity`, `oceanbase`, `es`).
        dialect: Optional sqlglot dialect override.

    Raises:
        ReadOnlySqlViolation: If the statement is invalid or not read-only.
    """
    if dialect is None and doc_engine is not None:
        dialect = sqlglot_dialect_for_doc_engine(doc_engine)

    sql = normalize_sql_for_validation(sql)
    if not sql:
        raise ReadOnlySqlViolation("SQL must not be empty.")

    statements = _parse_sql_statements(sql, dialect)
    if len(statements) != 1:
        logger.warning(
            "SQL validation rejected: dialect=%s, stage=parse, reason=multiple_statements",
            dialect,
        )
        raise ReadOnlySqlViolation("For security reasons, only one read-only SQL statement is supported.")

    statement = statements[0]
    if not isinstance(statement, _ALLOWED_READ_EXPRESSIONS):
        logger.warning(
            "SQL validation rejected: dialect=%s, stage=type_check, statement_type=%s",
            dialect,
            type(statement).__name__,
        )
        raise ReadOnlySqlViolation("For security reasons, only read-only SELECT statements are supported.")

    unsafe = statement.find(
        exp.Insert,
        exp.Update,
        exp.Delete,
        exp.Drop,
        exp.Create,
        exp.Alter,
        exp.Command,
        exp.Lock,
        exp.Into,
    )
    if unsafe or statement.args.get("locks"):
        reason = f"unsafe_node={type(unsafe).__name__}" if unsafe else "locks_present"
        logger.warning(
            "SQL validation rejected: dialect=%s, stage=safety_check, %s",
            dialect,
            reason,
        )
        raise ReadOnlySqlViolation("For security reasons, only read-only SELECT statements are supported.")
