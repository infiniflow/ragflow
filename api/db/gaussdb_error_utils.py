#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
"""GaussDB metadata adapter exception classification helpers.

This module deliberately avoids importing ``db_models`` or service classes so
the GaussDB pool, migrations, and retry decorator do not form import cycles.
"""

from __future__ import annotations

from collections.abc import Iterable


# Connection-related SQLSTATE values reported by GaussDB/PostgreSQL/libpq.
# GaussDB exposes errors through psycopg2/libpq, so connection failures and
# server rejections should be classified by SQLSTATE instead of English error
# text. Text matching is only a fallback when the driver omits pgcode. The
# 08xxx class covers connection exceptions, while 57P0x covers shutdowns and
# temporarily unavailable servers.
PSYCOPG_CONNECTION_SQLSTATES = {
    "08000",  # connection_exception
    "08001",  # sqlclient_unable_to_establish_sqlconnection
    "08003",  # connection_does_not_exist
    "08004",  # sqlserver_rejected_establishment_of_sqlconnection
    "08006",  # connection_failure
    "08007",  # transaction_resolution_unknown
    "08P01",  # protocol_violation
    "57P01",  # admin_shutdown
    "57P02",  # crash_shutdown
    "57P03",  # cannot_connect_now
}

# Transaction-conflict SQLSTATE values observed on GaussDB:
# 40P01: deadlock detected;
# 40001: could not serialize access due to concurrent update;
# 55P03: FOR UPDATE NOWAIT could not acquire the row lock.
# Each indicates a statement or transaction that can be retried as a complete
# RAGFlow database operation.
PSYCOPG_RETRYABLE_TRANSACTION_SQLSTATES = {"40P01", "40001", "55P03"}

# SQLSTATE values used to make DDL migrations idempotent. Migration code must
# distinguish an already-applied change from a real failure. GaussDB reports
# duplicate columns as 42701, duplicate table/index names as 42P07, and missing
# index/constraint objects as 42704.
PSYCOPG_DUPLICATE_COLUMN_SQLSTATES = {"42701"}
PSYCOPG_DUPLICATE_OBJECT_SQLSTATES = {"42P07"}
PSYCOPG_UNDEFINED_OBJECT_SQLSTATES = {"42704"}

CONNECTION_ERROR_MESSAGES = (
    "server closed",
    "connection refused",
    "no connection to the server",
    "terminating connection",
    "ssl connection has been closed unexpectedly",
    "ssl syscall error: eof detected",
)


def _iter_exception_chain(exc: BaseException) -> Iterable[BaseException | object]:
    """Iterate through Peewee wrappers, chained errors, and driver errors in args."""
    seen: set[int] = set()
    stack: list[BaseException | object] = [exc]
    while stack:
        current = stack.pop(0)
        current_id = id(current)
        if current_id in seen:
            continue
        seen.add(current_id)
        yield current

        for attr in ("__cause__", "__context__", "orig", "original_exception"):
            nested = getattr(current, attr, None)
            if nested is not None:
                stack.append(nested)

        for arg in getattr(current, "args", ()) or ():
            if hasattr(arg, "pgcode"):
                stack.append(arg)


def sqlstate_from_exception(exc: BaseException) -> str | None:
    """Extract SQLSTATE from a psycopg2 error or a Peewee-wrapped error."""
    for current in _iter_exception_chain(exc):
        pgcode = getattr(current, "pgcode", None)
        if pgcode:
            return str(pgcode)

        diag = getattr(current, "diag", None)
        sqlstate = getattr(diag, "sqlstate", None) if diag else None
        if sqlstate:
            return str(sqlstate)

    return None


def exception_text(exc: BaseException) -> str:
    return str(exc).lower()


def mysql_errno_from_exception(exc: BaseException) -> int | None:
    args = getattr(exc, "args", ())
    if args and isinstance(args[0], int):
        return args[0]
    return None


def is_psycopg_connection_error(exc: BaseException) -> bool:
    sqlstate = sqlstate_from_exception(exc)
    if sqlstate is not None:
        return sqlstate.startswith("08") or sqlstate in {"57P01", "57P02", "57P03"}

    if exc.__class__.__name__ == "InterfaceError":
        return True

    text = exception_text(exc)
    return any(message in text for message in CONNECTION_ERROR_MESSAGES)


def is_retryable_transaction_error(exc: BaseException) -> bool:
    sqlstate = sqlstate_from_exception(exc)
    if sqlstate in PSYCOPG_RETRYABLE_TRANSACTION_SQLSTATES:
        return True

    text = exception_text(exc)
    return "deadlock detected" in text or "could not serialize access" in text or "could not obtain lock" in text


def is_duplicate_column_error(exc: BaseException) -> bool:
    if mysql_errno_from_exception(exc) == 1060:
        return True

    sqlstate = sqlstate_from_exception(exc)
    if sqlstate in PSYCOPG_DUPLICATE_COLUMN_SQLSTATES:
        return True

    text = exception_text(exc)
    return "duplicate column" in text or ("column" in text and "already exists" in text)


def is_duplicate_object_error(exc: BaseException) -> bool:
    if mysql_errno_from_exception(exc) == 1061:
        return True

    sqlstate = sqlstate_from_exception(exc)
    if sqlstate in PSYCOPG_DUPLICATE_OBJECT_SQLSTATES:
        return True

    text = exception_text(exc)
    return "duplicate key name" in text or "already exists" in text


def is_undefined_object_error(exc: BaseException) -> bool:
    if mysql_errno_from_exception(exc) == 1091:
        return True

    sqlstate = sqlstate_from_exception(exc)
    if sqlstate in PSYCOPG_UNDEFINED_OBJECT_SQLSTATES:
        return True

    text = exception_text(exc)
    return "can't drop" in text or "does not exist" in text
