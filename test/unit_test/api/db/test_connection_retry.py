import pytest
from peewee import OperationalError
from playhouse.pool import PooledMySQLDatabase

from api.db.db_models import RetryingPooledMySQLDatabase, _is_mysql_connection_error


class DeadConnection:
    def __init__(self):
        self.closed = False

    def close(self):
        self.closed = True


def test_mysql_connection_loss_retries_once_outside_transaction(monkeypatch):
    db = RetryingPooledMySQLDatabase(None, max_retries=1, retry_delay=0)
    calls = []
    reconnects = []

    def execute(_self, sql, params=None, commit=True):
        calls.append(sql)
        if len(calls) == 1:
            raise OperationalError(2013, "Lost connection")
        return "cursor"

    monkeypatch.setattr(PooledMySQLDatabase, "execute_sql", execute)
    monkeypatch.setattr(db, "in_transaction", lambda: False)
    monkeypatch.setattr(db, "_handle_connection_loss", lambda: reconnects.append(None))

    assert db.execute_sql("SELECT 1") == "cursor"
    assert calls == ["SELECT 1", "SELECT 1"]
    assert reconnects == [None]


def test_mysql_connection_loss_poisoning_evicts_every_dead_transaction_connection(monkeypatch):
    db = RetryingPooledMySQLDatabase(None, max_retries=1, retry_delay=0)
    monkeypatch.setattr(db, "in_transaction", lambda: True)

    def execute(_self, sql, params=None, commit=True):
        raise OperationalError(2013, "Lost connection")

    monkeypatch.setattr(PooledMySQLDatabase, "execute_sql", execute)

    for _ in range(50):
        connection = DeadConnection()
        db._state.conn = connection
        db._state.transactions.append(object())
        db._in_use[db.conn_key(connection)] = object()
        db._connections = [(0, object(), connection)]

        with pytest.raises(OperationalError, match="Lost connection"):
            db.execute_sql("SELECT 1")

        assert connection.closed
        assert not db._in_use
        assert not db._connections
        db.pop_transaction()
        assert getattr(db._state, "connection_loss", None) is None


def test_mysql_non_connection_error_is_not_retried(monkeypatch):
    db = RetryingPooledMySQLDatabase(None, max_retries=1, retry_delay=0)
    calls = []

    def execute(_self, sql, params=None, commit=True):
        calls.append(sql)
        raise OperationalError("syntax error")

    monkeypatch.setattr(PooledMySQLDatabase, "execute_sql", execute)

    with pytest.raises(OperationalError, match="syntax error"):
        db.execute_sql("SELECT 1")
    assert calls == ["SELECT 1"]


@pytest.mark.parametrize("error", [OperationalError([]), OperationalError({})])
def test_mysql_connection_error_classifier_accepts_unhashable_codes(error):
    assert not _is_mysql_connection_error(error)
