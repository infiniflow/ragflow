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
"""SQLAlchemy 2.0 engine, session factory, and declarative base.

This module is the Phase 1 foundation for the Peewee → SQLAlchemy migration
(see docs/proposals/peewee-to-sqlalchemy.md).  No Peewee code is removed here;
the two ORMs coexist during the incremental Phase 2 service migration.

Usage
-----
New services should inject the session via ``get_db_session()``:

    from api.db.sqlalchemy_engine import get_db_session

    def my_service_function():
        with get_db_session() as session:
            result = session.execute(select(MyModel).where(...)).scalars().all()
            return result

The Quart ``teardown_request`` hook in ``api/apps/__init__.py`` calls
``close_db_session()`` at the end of every request to return the thread-local
session to the pool.
"""

import logging
from collections.abc import Generator
from contextlib import contextmanager

from sqlalchemy import create_engine, event, text
from sqlalchemy.exc import DBAPIError, OperationalError
from sqlalchemy.orm import DeclarativeBase, Session as SASession, scoped_session, sessionmaker

from common import settings

logger = logging.getLogger(__name__)


# ---------------------------------------------------------------------------
# Connection URL builder
# ---------------------------------------------------------------------------

def _build_database_url() -> str:
    """Build a SQLAlchemy connection URL from RAGFlow's existing DATABASE config."""
    cfg = settings.DATABASE.copy()
    db_type = settings.DATABASE_TYPE.lower()

    host = cfg.get("host", "127.0.0.1")
    user = cfg.get("user", "")
    password = cfg.get("password", "")
    name = cfg.get("name", "ragflow")
    default_port = 5432 if db_type in ("postgres", "postgresql") else 3306
    port = cfg.get("port", default_port)

    if db_type in ("mysql", "mariadb"):
        # pymysql is already a transitive dependency via Peewee
        return f"mysql+pymysql://{user}:{password}@{host}:{port}/{name}?charset=utf8mb4"
    if db_type == "oceanbase":
        # sqlalchemy-oceanbase dialect; falls back to mysql+pymysql if unavailable
        try:
            import sqlalchemy_oceanbase  # noqa: F401
            driver = "mysql+oceanbase"
        except ImportError:
            logger.warning(
                "sqlalchemy-oceanbase not installed; falling back to mysql+pymysql for OceanBase. "
                "Install sqlalchemy-oceanbase for full dialect support."
            )
            driver = "mysql+pymysql"
        return f"{driver}://{user}:{password}@{host}:{port}/{name}?charset=utf8mb4"
    if db_type in ("postgres", "postgresql"):
        # psycopg2 is already a transitive dependency via Peewee
        return f"postgresql+psycopg2://{user}:{password}@{host}:{port}/{name}"

    raise ValueError(f"Unsupported DATABASE_TYPE for SQLAlchemy engine: {db_type!r}")


# ---------------------------------------------------------------------------
# Engine
# ---------------------------------------------------------------------------

def _create_sa_engine():
    url = _build_database_url()
    engine = create_engine(
        url,
        # Probes the connection on checkout; transparently recreates invalid
        # connections.  Replaces the checkout-probe part of RetryingPooledMySQLDatabase.
        pool_pre_ping=True,
        pool_size=10,
        max_overflow=20,
        # Recycle connections after 1 hour to avoid "wait_timeout" disconnects.
        pool_recycle=3600,
        # Echo SQL to the logger only in debug mode.
        echo=False,
    )
    _register_disconnect_listener(engine)
    return engine


def _register_disconnect_listener(engine) -> None:
    """Log connection invalidation events for observability."""
    @event.listens_for(engine, "handle_error")
    def _on_error(context):
        if context.connection_invalidated:
            logger.warning(
                "SQLAlchemy invalidated a connection due to a database error: %s",
                context.original_exception,
            )


engine = _create_sa_engine()


# ---------------------------------------------------------------------------
# Session factory
# ---------------------------------------------------------------------------

# scoped_session ensures that all code within the same thread receives the
# same Session instance, eliminating nested-connection conflicts.
_session_factory = sessionmaker(bind=engine, autocommit=False, autoflush=False)
Session = scoped_session(_session_factory)


# ---------------------------------------------------------------------------
# Declarative base for new SQLAlchemy models
# ---------------------------------------------------------------------------

class Base(DeclarativeBase):
    """Declarative base for all SQLAlchemy models introduced during migration.

    Peewee models in db_models.py are NOT subclasses of this Base and will
    continue to operate independently until Phase 3 cleanup.
    """


# ---------------------------------------------------------------------------
# Session lifecycle helpers
# ---------------------------------------------------------------------------

@contextmanager
def get_db_session() -> Generator[SASession, None, None]:
    """Context manager that yields the thread-local scoped session.

    Commits on clean exit, rolls back on exception, and always returns the
    session to the pool.  Suitable for use inside service functions that manage
    their own transaction boundary.

        with get_db_session() as session:
            session.add(obj)
            # auto-commit on exit
    """
    session: Session = Session()
    try:
        yield session
        session.commit()
    except Exception:
        session.rollback()
        raise
    finally:
        Session.remove()


def close_db_session(exception: BaseException | None = None) -> None:
    """Remove the thread-local session at the end of a request.

    Intended to be called from Quart's ``teardown_request`` hook:

        @app.teardown_request
        def _teardown(exc):
            close_db_session(exc)

    If ``exception`` is set the session is rolled back before removal so that
    partial writes from a failed request are never committed.
    """
    if exception is not None:
        try:
            Session.rollback()
        except Exception:
            pass
    Session.remove()


def check_db_health() -> bool:
    """Return True if the engine can reach the database; False otherwise.

    Used by health-check endpoints without raising an exception to the caller.
    """
    try:
        with engine.connect() as conn:
            conn.execute(text("SELECT 1"))
        return True
    except (OperationalError, DBAPIError) as exc:
        logger.error("SQLAlchemy health check failed: %s", exc)
        return False
