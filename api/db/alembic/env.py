"""Alembic environment configuration for RAGFlow.

Connects to the database using the same engine as the application so that
credentials never need to be duplicated in alembic.ini.

Autogenerate compares against ``api.db.sqlalchemy_engine.Base.metadata``.
During Phase 2 of the Peewee → SQLAlchemy migration, only models that have
been ported to SQLAlchemy will appear here; Peewee models are not visible to
Alembic until they are rewritten as SQLAlchemy declarative models.
"""

import logging
from logging.config import fileConfig

from alembic import context

# Read alembic.ini logging configuration.
config = context.config
if config.config_file_name is not None:
    fileConfig(config.config_file_name)

logger = logging.getLogger("alembic.env")

# Import Base so autogenerate can compare against its metadata.
# Import all SQLAlchemy model modules here as they are added during Phase 2
# so Alembic can detect their tables.
from api.db.sqlalchemy_engine import Base, engine  # noqa: E402

target_metadata = Base.metadata


def run_migrations_offline() -> None:
    """Run migrations in 'offline' mode (no live DB connection required).

    Emits the SQL statements to stdout for review before applying.
    """
    url = str(engine.url)
    context.configure(
        url=url,
        target_metadata=target_metadata,
        literal_binds=True,
        dialect_opts={"paramstyle": "named"},
        compare_type=True,
    )
    with context.begin_transaction():
        context.run_migrations()


def run_migrations_online() -> None:
    """Run migrations in 'online' mode against a live database connection."""
    with engine.connect() as connection:
        context.configure(
            connection=connection,
            target_metadata=target_metadata,
            compare_type=True,
            # Render item-level BATCH mode for SQLite compatibility (no-op on MySQL/PG).
            render_as_batch=False,
        )
        with context.begin_transaction():
            context.run_migrations()


if context.is_offline_mode():
    run_migrations_offline()
else:
    run_migrations_online()
