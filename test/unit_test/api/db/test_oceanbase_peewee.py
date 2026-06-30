"""
Tests for OceanBase Peewee ORM support.
"""

import pytest
import peewee
from peewee import OperationalError, ProgrammingError, InterfaceError
from api.db.db_models import (
    RetryingPooledOceanBaseDatabase,
    RetryingPooledPostgresqlDatabase,
    PooledDatabase,
    DatabaseLock,
    TextFieldType,
)


class TestOceanBaseDatabase:
    """Test cases for OceanBase database support."""

    def test_oceanbase_database_class_exists(self):
        """Test that RetryingPooledOceanBaseDatabase class exists."""
        assert RetryingPooledOceanBaseDatabase is not None

    def test_oceanbase_in_pooled_database_enum(self):
        """Test that OCEANBASE is in PooledDatabase enum."""
        assert hasattr(PooledDatabase, 'OCEANBASE')
        assert PooledDatabase.OCEANBASE.value == RetryingPooledOceanBaseDatabase

    def test_oceanbase_in_database_lock_enum(self):
        """Test that OCEANBASE is in DatabaseLock enum."""
        assert hasattr(DatabaseLock, 'OCEANBASE')

    def test_oceanbase_in_text_field_type_enum(self):
        """Test that OCEANBASE is in TextFieldType enum."""
        assert hasattr(TextFieldType, 'OCEANBASE')
        # OceanBase should use LONGTEXT like MySQL
        assert TextFieldType.OCEANBASE.value == "LONGTEXT"

    def test_oceanbase_database_inherits_mysql(self):
        """Test that OceanBase database inherits from PooledMySQLDatabase."""
        from playhouse.pool import PooledMySQLDatabase
        assert issubclass(RetryingPooledOceanBaseDatabase, PooledMySQLDatabase)

    def test_oceanbase_database_init(self):
        """Test OceanBase database initialization."""
        db = RetryingPooledOceanBaseDatabase(
            "test_db",
            host="localhost",
            port=2881,
            user="root",
            password="password",
        )
        assert db is not None
        assert db.max_retries == 5  # default value
        assert db.retry_delay == 1  # default value

    def test_oceanbase_database_custom_retries(self):
        """Test OceanBase database with custom retry settings."""
        db = RetryingPooledOceanBaseDatabase(
            "test_db",
            host="localhost",
            max_retries=10,
            retry_delay=2,
        )
        assert db.max_retries == 10
        assert db.retry_delay == 2

    def test_pooled_database_enum_values(self):
        """Test PooledDatabase enum has all expected values."""
        expected = {'MYSQL', 'OCEANBASE', 'POSTGRES'}
        actual = {e.name for e in PooledDatabase}
        assert expected.issubset(actual), f"Missing: {expected - actual}"

    def test_database_lock_enum_values(self):
        """Test DatabaseLock enum has all expected values."""
        expected = {'MYSQL', 'OCEANBASE', 'POSTGRES'}
        actual = set(DatabaseLock.__members__.keys())
        assert expected.issubset(actual), f"Missing: {expected - actual}"


class TestOceanBaseConfiguration:
    """Test cases for OceanBase configuration via environment variables."""

    def test_settings_default_to_mysql(self):
        """Test that default DB_TYPE is mysql."""
        import os
        # Save original value
        original = os.environ.get('DB_TYPE')
        
        try:
            # Remove DB_TYPE to test default
            if 'DB_TYPE' in os.environ:
                del os.environ['DB_TYPE']
            
            # Reload settings
            from common import settings
            settings.DATABASE_TYPE = os.getenv("DB_TYPE", "mysql")
            
            assert settings.DATABASE_TYPE == "mysql"
        finally:
            # Restore original value
            if original:
                os.environ['DB_TYPE'] = original

    def test_settings_can_use_oceanbase(self):
        """Test that DB_TYPE can be set to oceanbase."""
        import os
        # Save original value
        original = os.environ.get('DB_TYPE')
        
        try:
            os.environ['DB_TYPE'] = 'oceanbase'
            
            # Reload settings
            from common import settings
            settings.DATABASE_TYPE = os.getenv("DB_TYPE", "mysql")
            
            assert settings.DATABASE_TYPE == "oceanbase"
        finally:
            # Restore original value
            if original:
                os.environ['DB_TYPE'] = original
            else:
                if 'DB_TYPE' in os.environ:
                    del os.environ['DB_TYPE']


class _FakeCursor:
    def __init__(self, conn, ctl):
        self.conn = conn
        self.ctl = ctl
        self.rowcount = 1
        self.lastrowid = 1

    def execute(self, sql, params=()):
        if self.ctl["fail_next"] > 0:
            self.ctl["fail_next"] -= 1
            self.conn.alive = False
            raise OperationalError(2013, "Lost connection to MySQL server during query")
        if not self.conn.alive:
            raise OperationalError(2013, "Lost connection to MySQL server during query")
        return self

    def fetchall(self):
        return []

    def fetchone(self):
        return None

    def close(self):
        pass


class _FakeConn:
    """Minimal stand-in for a DB-API connection so connection loss can be
    simulated deterministically through the real peewee/pool machinery."""

    def __init__(self, ctl):
        self.alive = True
        self._counted = False
        self.ctl = ctl
        ctl["created"] += 1

    def cursor(self):
        return _FakeCursor(self, self.ctl)

    def ping(self, *args):
        if not self.alive:
            raise OperationalError(2013, "Lost connection (ping)")

    def close(self):
        self.alive = False
        if not self._counted:
            self._counted = True
            self.ctl["closed"] += 1

    def rollback(self):
        if not self.alive:
            raise OperationalError(2013, "Lost connection during rollback")

    def commit(self):
        pass

    def autocommit(self, *args, **kwargs):
        pass

    def get_server_info(self):
        return "8.0.0"

    @property
    def closed(self):
        return not self.alive


@pytest.fixture
def mock_db(monkeypatch):
    """A RetryingPooledOceanBaseDatabase backed by a mocked driver connection.

    ``ctl`` controls/observes the fake driver: set ``fail_next`` to make the
    next N statements raise a "lost connection" error; ``created``/``closed``
    count the lifetime of the underlying sockets so leaks are detectable.
    """
    ctl = {"fail_next": 0, "created": 0, "closed": 0}
    monkeypatch.setattr(peewee.MySQLDatabase, "_connect", lambda self: _FakeConn(ctl))
    db = RetryingPooledOceanBaseDatabase("test_db", max_connections=900, max_retries=3, retry_delay=0)
    db.server_version = (8, 0, 0)  # skip the driver-dependent version probe
    yield db, ctl
    try:
        if not db.is_closed():
            db.close()
    except Exception:
        pass


def _run_request(db, ctl, *, in_transaction, fail_next):
    """Emulate `@DB.connection_context()` wrapping an optional transaction."""
    error = None
    if db.is_closed():
        db.connect()
    try:
        if in_transaction:
            with db.atomic():
                ctl["fail_next"] = fail_next
                db.execute_sql("UPDATE t SET x=1 WHERE id=1")
        else:
            ctl["fail_next"] = fail_next
            db.execute_sql("UPDATE t SET x=1 WHERE id=1")
    except Exception as e:  # noqa: BLE001
        error = e
    finally:
        ctl["fail_next"] = 0
        try:
            db.close()  # connection_context.__exit__
        except Exception as e:  # noqa: BLE001
            error = e
    return error


class TestConnectionLossRecovery:
    """Regression tests for issue #15198: connection-loss retry must not leak
    pooled connections nor corrupt state when it happens inside a transaction."""

    @pytest.mark.p1
    def test_recoverable_loss_outside_transaction_self_heals(self, mock_db):
        db, ctl = mock_db
        error = _run_request(db, ctl, in_transaction=False, fail_next=1)
        assert error is None, f"recoverable loss should be retried transparently, got {error!r}"
        assert len(db._in_use) == 0

    @pytest.mark.p1
    def test_mid_transaction_loss_does_not_leak_connections(self, mock_db):
        db, ctl = mock_db
        for _ in range(50):
            _run_request(db, ctl, in_transaction=True, fail_next=1)
        # No pooled connection left checked out, and every dead socket was closed.
        assert len(db._in_use) == 0, f"leaked in-use connections: {len(db._in_use)}"
        assert ctl["created"] == ctl["closed"], (
            f"leaked sockets: created={ctl['created']} closed={ctl['closed']}"
        )

    @pytest.mark.p1
    def test_mid_transaction_loss_surfaces_clean_error(self, mock_db):
        db, ctl = mock_db
        error = _run_request(db, ctl, in_transaction=True, fail_next=1)
        assert isinstance(error, OperationalError)
        # The original buggy code masked the real error with this message.
        assert "Connection already opened" not in str(error)

    @pytest.mark.p2
    def test_non_connection_error_is_not_retried(self, mock_db):
        db, ctl = mock_db
        db.connect()
        original = _FakeCursor.execute

        def boom(self, sql, params=()):
            raise ProgrammingError("syntax error")

        _FakeCursor.execute = boom
        try:
            with pytest.raises(ProgrammingError):
                db.execute_sql("SELECT bad")
        finally:
            _FakeCursor.execute = original

    @pytest.mark.p1
    def test_begin_recovers_from_connection_loss(self, mock_db):
        db, ctl = mock_db
        db.connect()
        # The BEGIN statement itself loses the connection once, then recovers.
        error = None
        try:
            ctl["fail_next"] = 1
            with db.atomic():
                db.execute_sql("UPDATE t SET x=1 WHERE id=1")
        except Exception as e:  # noqa: BLE001
            error = e
        finally:
            ctl["fail_next"] = 0
            db.close()
        assert error is None, f"begin() should recover and complete, got {error!r}"
        assert len(db._in_use) == 0


class _PgError(Exception):
    """Stand-in for a psycopg2 error carrying a SQLSTATE in ``pgcode``."""

    def __init__(self, msg, pgcode):
        super().__init__(msg)
        self.pgcode = pgcode


class TestPostgresConnectionLossDetection:
    """Regression tests for PostgreSQL connection-loss detection.

    The previous predicate matched a bare ``'connection'`` substring, so any
    error merely mentioning that word (e.g. a constraint violation on a
    ``connection_id`` column) was misclassified as a connection loss and would
    trigger a spurious retry / transaction abort.
    """

    _detect = staticmethod(RetryingPooledPostgresqlDatabase._is_connection_loss)

    @pytest.mark.p1
    @pytest.mark.parametrize("message", [
        "server closed the connection unexpectedly",
        "could not connect to server: Connection refused",
        "terminating connection due to administrator command",
        "SSL connection has been closed unexpectedly",
        "connection reset by peer",
    ])
    def test_genuine_connection_loss_is_detected(self, message):
        assert self._detect(OperationalError(message)) is True

    @pytest.mark.p1
    @pytest.mark.parametrize("message", [
        'duplicate key value violates unique constraint "ix_doc_connection_id"',
        "null value in column \"connection_id\" violates not-null constraint",
        "column connection_state does not exist",
    ])
    def test_unrelated_error_mentioning_connection_is_not_a_loss(self, message):
        # These contain the word "connection" but are not connection losses;
        # the old bare-'connection' substring wrongly matched all of them.
        assert self._detect(OperationalError(message)) is False

    @pytest.mark.p1
    def test_sqlstate_detected_via_peewee_orig(self):
        # peewee stores the original driver exception (which carries the
        # SQLSTATE) on ``.orig``; an opaque message must still be detected
        # through the code, not the text.
        wrapped = OperationalError(_PgError("backend crashed", "57P02"))
        assert getattr(wrapped, "orig").pgcode == "57P02"
        assert self._detect(wrapped) is True

    @pytest.mark.p2
    def test_interface_error_is_detected(self):
        assert self._detect(InterfaceError("connection already closed")) is True

    @pytest.mark.p2
    def test_plain_programming_error_is_not_a_loss(self):
        assert self._detect(ProgrammingError("syntax error at or near \"SELCT\"")) is False


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
