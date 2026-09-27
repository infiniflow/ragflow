from peewee import InterfaceError, OperationalError, ProgrammingError


class PgError(Exception):
    def __init__(self, pgcode: str, message: str = "driver error"):
        self.pgcode = pgcode
        self.args = (message,)


def peewee_error_with_context(exc_cls, pgcode: str, message: str):
    exc = exc_cls(message)
    # Peewee wraps psycopg2 exceptions and keeps the raw driver exception on
    # __context__; live GaussDB probing confirmed pgcode is not on the outer
    # Peewee exception itself.
    exc.__context__ = PgError(pgcode, message)
    return exc


def test_sqlstate_extraction_reads_peewee_context():
    from api.db.gaussdb_error_utils import sqlstate_from_exception

    exc = peewee_error_with_context(ProgrammingError, "42701", "Column already exists")

    assert sqlstate_from_exception(exc) == "42701"


def test_gaussdb_retryable_transaction_errors_include_live_sqlstates(monkeypatch):
    from api.db.gaussdb_error_utils import is_retryable_transaction_error
    from api.db.services import common_service

    monkeypatch.setattr(common_service.settings, "DATABASE_TYPE", "gaussdb")

    for sqlstate in ("40P01", "40001", "55P03"):
        exc = peewee_error_with_context(OperationalError, sqlstate, "retryable conflict")
        assert is_retryable_transaction_error(exc)
        assert common_service._is_deadlock_error(exc)

    syntax_error = peewee_error_with_context(OperationalError, "42601", "syntax error")
    assert not is_retryable_transaction_error(syntax_error)
    assert not common_service._is_deadlock_error(syntax_error)


def test_gaussdb_ddl_idempotency_error_helpers_use_live_sqlstates():
    from api.db.gaussdb_error_utils import (
        is_duplicate_column_error,
        is_duplicate_object_error,
        is_undefined_object_error,
    )

    assert is_duplicate_column_error(peewee_error_with_context(ProgrammingError, "42701", "Column already exists"))
    assert is_duplicate_object_error(peewee_error_with_context(ProgrammingError, "42P07", "Relation already exists"))
    assert is_undefined_object_error(peewee_error_with_context(ProgrammingError, "42704", "index does not exist"))

    assert not is_duplicate_column_error(peewee_error_with_context(ProgrammingError, "42601", "syntax error"))
    assert not is_duplicate_object_error(peewee_error_with_context(ProgrammingError, "23505", "duplicate key value"))
    assert not is_undefined_object_error(peewee_error_with_context(ProgrammingError, "42P07", "already exists"))


def test_connection_error_helper_covers_sqlstate_interface_and_gaussdb_ssl_close_text():
    from api.db.gaussdb_error_utils import is_psycopg_connection_error

    assert is_psycopg_connection_error(peewee_error_with_context(OperationalError, "08006", "connection failure"))
    assert is_psycopg_connection_error(InterfaceError("connection already closed"))
    assert is_psycopg_connection_error(OperationalError("SSL connection has been closed unexpectedly"))
    assert not is_psycopg_connection_error(OperationalError("syntax error at or near FROM"))


def test_non_connection_sqlstate_overrides_connection_shaped_text():
    from api.db.gaussdb_error_utils import is_psycopg_connection_error

    exc = peewee_error_with_context(OperationalError, "22P02", "invalid integer from connection-shaped input")

    assert is_psycopg_connection_error(exc) is False


def test_all_08_class_sqlstates_are_connection_errors():
    from api.db.gaussdb_error_utils import is_psycopg_connection_error

    assert is_psycopg_connection_error(peewee_error_with_context(OperationalError, "08002", "driver state 08002"))


def test_no_sqlstate_eof_reset_is_a_connection_error():
    from api.db.gaussdb_error_utils import is_psycopg_connection_error

    assert is_psycopg_connection_error(OperationalError("SSL SYSCALL error: EOF detected"))


def test_no_sqlstate_unrelated_connection_word_is_not_a_connection_error():
    from api.db.gaussdb_error_utils import is_psycopg_connection_error

    assert not is_psycopg_connection_error(OperationalError("invalid value for application connection label"))


def test_nested_non_connection_sqlstate_overrides_outer_interface_error():
    from api.db.gaussdb_error_utils import is_psycopg_connection_error

    exc = peewee_error_with_context(InterfaceError, "22P02", "invalid integer from connection-shaped input")

    assert is_psycopg_connection_error(exc) is False
