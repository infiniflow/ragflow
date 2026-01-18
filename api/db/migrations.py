#
# Migration helpers extracted from db_models.py
#
from __future__ import annotations

import inspect
import logging
import sys
import time
from datetime import datetime

from peewee import BooleanField, CharField, DateTimeField, IntegerField, TextField, AutoField

from api.db.base import DataBaseModel
from api.db.connection import DB
from api.db.error_handlers import StandardErrorHandler
from api.db.fields import JSONField, LongTextField
from api.db.pool import DatabaseMigrator
from common import settings

# Import all model classes at module level
# This makes them available for inspection in init_database_tables()
from api.db.models import *  # noqa: F401, F403


class MigrationHistory(DataBaseModel):
    """
    Track migration execution history to prevent duplicate runs and enable rollback.

    This table stores the status of each migration operation, allowing the system to:
    - Skip migrations that have already been successfully applied
    - Track failed migrations for debugging
    - Record execution duration for performance monitoring
    - Support future rollback capabilities
    """

    id = AutoField(primary_key=True)  # Auto-increment primary key
    migration_name = CharField(max_length=255, unique=True, null=False, index=True, help_text="Unique migration identifier")
    applied_at = DateTimeField(null=False, index=True, help_text="When the migration was executed")
    status = CharField(max_length=16, null=False, default="success", index=True, help_text="success|failed|skipped")
    error_message = TextField(null=True, help_text="Error details if migration failed")
    duration_ms = IntegerField(null=True, help_text="Execution time in milliseconds")
    db_type = CharField(max_length=16, null=False, index=True, help_text="mysql|postgres")

    class Meta:
        db_table = "migration_history"
        indexes = (
            (("migration_name", "status"), False),  # Composite index for queries
        )


class MigrationTracker:
    """
    Utility class to track and manage database migrations.

    Provides methods to:
    - Initialize tracking table
    - Check if migrations have been applied
    - Record migration execution results
    - Query migration history
    """

    @staticmethod
    def init_tracking_table():
        """
        Create migration_history table if it doesn't exist.

        This should be called before any migrations are run to ensure
        the tracking infrastructure is in place.
        """
        try:
            if not MigrationHistory.table_exists():
                MigrationHistory.create_table(safe=True)
                logging.info("Created migration_history tracking table")
            else:
                logging.debug("migration_history table already exists")
        except Exception as ex:
            logging.error(f"Failed to create migration_history table: {ex}")
            # Don't raise - allow migrations to proceed even if tracking fails

    @staticmethod
    def has_migration_run(migration_name: str) -> bool:
        """
        Check if a migration has been successfully applied.

        Args:
            migration_name: Unique identifier for the migration

        Returns:
            bool: True if migration was successfully applied, False otherwise
        """
        try:
            result = MigrationHistory.select().where((MigrationHistory.migration_name == migration_name) & (MigrationHistory.status == "success")).first()
            return result is not None
        except Exception as ex:
            logging.warning(f"Failed to check migration status for {migration_name}: {ex}")
            # If we can't check, assume it hasn't run (fail-safe approach)
            return False

    @staticmethod
    def record_migration(migration_name: str, status: str, error: str | None = None, duration_ms: int | None = None):
        """
        Record the result of a migration execution.

        Args:
            migration_name: Unique identifier for the migration
            status: Execution status (success|failed|skipped)
            error: Error message if migration failed (optional)
            duration_ms: Execution time in milliseconds (optional)
        """
        try:
            MigrationHistory.insert(
                migration_name=migration_name, applied_at=datetime.now(), status=status, error_message=error, duration_ms=duration_ms, db_type=settings.DATABASE_TYPE.lower()
            ).on_conflict(
                conflict_target=[MigrationHistory.migration_name],
                update={MigrationHistory.applied_at: datetime.now(), MigrationHistory.status: status, MigrationHistory.error_message: error, MigrationHistory.duration_ms: duration_ms},
            ).execute()
            logging.debug(f"Recorded migration: {migration_name} ({status})")
        except Exception as ex:
            # Don't fail the migration if we can't record it
            logging.warning(f"Failed to record migration {migration_name}: {ex}")

    @staticmethod
    def get_migration_history(limit: int = 100):
        """
        Retrieve migration history.

        Args:
            limit: Maximum number of records to return (default: 100)

        Returns:
            list: Migration records ordered by applied_at descending
        """
        try:
            return list(MigrationHistory.select().order_by(MigrationHistory.applied_at.desc()).limit(limit))
        except Exception as ex:
            logging.error(f"Failed to retrieve migration history: {ex}")
            return []

    @staticmethod
    def get_failed_migrations():
        """
        Retrieve all failed migrations for debugging.

        Returns:
            list: Failed migration records ordered by applied_at descending
        """
        try:
            return list(MigrationHistory.select().where(MigrationHistory.status == "failed").order_by(MigrationHistory.applied_at.desc()))
        except Exception as ex:
            logging.error(f"Failed to retrieve failed migrations: {ex}")
            return []


def alter_db_add_column(migrator, table_name, column_name, column_type):
    """
    Add a column to a table with standardized error handling.

    Handles expected errors (duplicate columns) gracefully and logs
    appropriate messages based on error severity.
    """
    try:
        with DB.atomic():
            from playhouse.migrate import migrate

            migrate(migrator.add_column(table_name, column_name, column_type))
            logging.debug(f"Added column: {table_name}.{column_name}")
    except Exception as ex:  # noqa: BLE001
        StandardErrorHandler.handle_migration_error(ex, table_name, column_name, "add_column", db_type=settings.DATABASE_TYPE.lower())
        raise


def alter_db_column_type(migrator, table_name, column_name, new_column_type):
    """
    Alter a column's type with standardized error handling.

    Handles expected type incompatibility errors and logs appropriately.
    """
    try:
        with DB.atomic():
            from playhouse.migrate import migrate

            migrate(migrator.alter_column_type(table_name, column_name, new_column_type))
            logging.debug(f"Altered column type: {table_name}.{column_name}")
    except Exception as ex:  # noqa: BLE001
        StandardErrorHandler.handle_migration_error(ex, table_name, column_name, "alter_column_type", db_type=settings.DATABASE_TYPE.lower())


def alter_db_rename_column(migrator, table_name, old_column_name, new_column_name):
    """
    Rename a column with standardized error handling.

    Handles cases where the column doesn't exist or has already been renamed.
    """
    try:
        with DB.atomic():
            from playhouse.migrate import migrate

            migrate(migrator.rename_column(table_name, old_column_name, new_column_name))
            logging.debug(f"Renamed column: {table_name}.{old_column_name} -> {new_column_name}")
    except Exception as ex:  # noqa: BLE001
        StandardErrorHandler.handle_migration_error(ex, table_name, old_column_name, "rename_column", db_type=settings.DATABASE_TYPE.lower())


def alter_db_add_not_null(migrator, table_name, column_name):
    """
    Make a column non-nullable with standardized error handling.

    This should only be called after ensuring all existing rows have non-null values
    in the target column, otherwise the migration will fail.
    """
    try:
        with DB.atomic():
            from playhouse.migrate import migrate

            migrate(migrator.add_not_null(table_name, column_name))
            logging.debug(f"Made column non-nullable: {table_name}.{column_name}")
    except Exception as ex:  # noqa: BLE001
        StandardErrorHandler.handle_migration_error(ex, table_name, column_name, "add_not_null", db_type=settings.DATABASE_TYPE.lower())


def alter_db_add_index(migrator, table_name, columns, unique=False):
    """
    Add an index (composite or single) to a table with standardized error handling.

    Args:
        migrator: Database migrator instance
        table_name: Name of the table
        columns: List of column names or single column name
        unique: Whether to create a unique index
    """
    try:
        with DB.atomic():
            from playhouse.migrate import migrate

            if isinstance(columns, str):
                columns = [columns]

            migrate(migrator.add_index(table_name, columns, unique))
            index_type = "unique index" if unique else "index"
            logging.debug(f"Added {index_type} on {table_name}({', '.join(columns)})")
    except Exception as ex:  # noqa: BLE001
        StandardErrorHandler.handle_migration_error(ex, table_name, str(columns), "add_index", db_type=settings.DATABASE_TYPE.lower())


def cleanup_file2document_orphans():
    """
    Remove File2Document records with NULL file_id or document_id.

    This cleanup must run before making these columns non-nullable.
    Orphaned records are invalid and should be removed.
    """
    try:
        with DB.atomic():
            from api.db.models.knowledge import File2Document

            # Delete records with NULL file_id or document_id
            deleted = File2Document.delete().where((File2Document.file_id.is_null()) | (File2Document.document_id.is_null())).execute()

            if deleted > 0:
                logging.info(f"Cleaned up {deleted} orphaned File2Document records with NULL foreign keys")
            else:
                logging.debug("No orphaned File2Document records found")
    except Exception as ex:  # noqa: BLE001
        StandardErrorHandler.handle_migration_error(ex, "file2document", "orphan_cleanup", "data_cleanup", db_type=settings.DATABASE_TYPE.lower())


def check_evaluation_fk_integrity():
    """
    Verify there are no orphaned foreign-key values across evaluation tables.

    Checks for:
      - evaluation_cases.dataset_id referencing missing evaluation_datasets.id
      - evaluation_runs.dataset_id referencing missing evaluation_datasets.id
      - evaluation_runs.dialog_id referencing missing dialog.id
      - evaluation_results.run_id referencing missing evaluation_runs.id
      - evaluation_results.case_id referencing missing evaluation_cases.id

    Raises an Exception if any violations are found to prevent adding FK constraints
    that would fail. Logs detailed counts for each violation type.
    """
    violations = {}

    # Cases -> Datasets
    q_cases = "SELECT COUNT(*) AS cnt FROM evaluation_cases ec LEFT JOIN evaluation_datasets ed ON ec.dataset_id = ed.id WHERE ed.id IS NULL"
    cases_orphans = list(DB.execute_sql(q_cases))[0][0]
    violations["evaluation_cases.dataset_id"] = cases_orphans

    # Runs -> Datasets
    q_runs_ds = "SELECT COUNT(*) AS cnt FROM evaluation_runs er LEFT JOIN evaluation_datasets ed ON er.dataset_id = ed.id WHERE ed.id IS NULL"
    runs_ds_orphans = list(DB.execute_sql(q_runs_ds))[0][0]
    violations["evaluation_runs.dataset_id"] = runs_ds_orphans

    # Runs -> Dialog
    q_runs_dialog = "SELECT COUNT(*) AS cnt FROM evaluation_runs er LEFT JOIN dialog d ON er.dialog_id = d.id WHERE d.id IS NULL"
    runs_dialog_orphans = list(DB.execute_sql(q_runs_dialog))[0][0]
    violations["evaluation_runs.dialog_id"] = runs_dialog_orphans

    # Results -> Runs
    q_results_run = "SELECT COUNT(*) AS cnt FROM evaluation_results r LEFT JOIN evaluation_runs er ON r.run_id = er.id WHERE er.id IS NULL"
    results_run_orphans = list(DB.execute_sql(q_results_run))[0][0]
    violations["evaluation_results.run_id"] = results_run_orphans

    # Results -> Cases
    q_results_case = "SELECT COUNT(*) AS cnt FROM evaluation_results r LEFT JOIN evaluation_cases ec ON r.case_id = ec.id WHERE ec.id IS NULL"
    results_case_orphans = list(DB.execute_sql(q_results_case))[0][0]
    violations["evaluation_results.case_id"] = results_case_orphans

    total = sum(violations.values())
    if total > 0:
        logging.error(f"Foreign key pre-check failed: {violations}")
        raise Exception("Foreign key integrity violations detected. Please fix orphans before applying constraints: " + ", ".join([f"{k}={v}" for k, v in violations.items() if v > 0]))
    else:
        logging.info("Foreign key pre-check passed: no orphaned records found across evaluation tables")


def add_memory_permissions_constraint(migrator):
    """
    Add CHECK constraint to memory.permissions to enforce only 'me' or 'team' values.

    This constraint ensures that only valid permission values are persisted in the database,
    preventing invalid data at the database layer.
    """
    try:
        with DB.atomic():
            # The constraint name should be unique and descriptive
            constraint_name = "chk_memory_permissions_values"
            constraint_sql = f"ALTER TABLE memory ADD CONSTRAINT {constraint_name} CHECK (permissions IN ('me','team'))"

            # Execute the raw SQL to add the constraint
            DB.execute_sql(constraint_sql)
            logging.debug(f"Added CHECK constraint to memory.permissions: {constraint_name}")
    except Exception as ex:  # noqa: BLE001
        # If constraint already exists or other errors, log but don't fail
        if "already exists" in str(ex) or "Duplicate" in str(ex):
            logging.debug(f"Memory permissions constraint already exists, skipping: {ex}")
        else:
            StandardErrorHandler.handle_migration_error(ex, "memory", "permissions", "add_check_constraint", db_type=settings.DATABASE_TYPE.lower())


def add_memory_forgetting_policy_constraint(migrator):
    """
    Add CHECK constraint to memory.forgetting_policy to enforce only 'LRU' or 'FIFO' values.

    This constraint ensures that only valid forgetting policy values are persisted in the database,
    preventing invalid data at the database layer.
    """
    try:
        with DB.atomic():
            # The constraint name should be unique and descriptive
            constraint_name = "chk_memory_forgetting_policy_values"
            constraint_sql = f"ALTER TABLE memory ADD CONSTRAINT {constraint_name} CHECK (forgetting_policy IN ('LRU','FIFO'))"

            # Execute the raw SQL to add the constraint
            DB.execute_sql(constraint_sql)
            logging.debug(f"Added CHECK constraint to memory.forgetting_policy: {constraint_name}")
    except Exception as ex:  # noqa: BLE001
        # If constraint already exists or other errors, log but don't fail
        if "already exists" in str(ex) or "Duplicate" in str(ex):
            logging.debug(f"Memory forgetting_policy constraint already exists, skipping: {ex}")
        else:
            StandardErrorHandler.handle_migration_error(ex, "memory", "forgetting_policy", "add_check_constraint", db_type=settings.DATABASE_TYPE.lower())


def add_fk_constraint_if_not_exists(constraint_name: str, sql: str):
    """
    Add a foreign key constraint idempotently.

    Executes the ALTER TABLE ADD CONSTRAINT SQL and handles cases where
    the constraint already exists gracefully. This allows migrations to be
    re-run without failing on duplicate constraints.

    Args:
        constraint_name: Name of the constraint being added (for logging)
        sql: The complete ALTER TABLE ADD CONSTRAINT SQL statement
    """
    try:
        with DB.atomic():
            DB.execute_sql(sql)
            logging.debug(f"Added foreign key constraint: {constraint_name}")
    except Exception as ex:  # noqa: BLE001
        # Check if the error is due to constraint already existing
        error_str = str(ex).lower()
        if "already exists" in error_str or "duplicate" in error_str:
            logging.debug(f"Foreign key constraint {constraint_name} already exists, skipping")
        else:
            # Unexpected error, log and re-raise
            logging.error(f"Failed to add foreign key constraint {constraint_name}: {ex}")
            raise


def migrate_db():
    """
    Apply all database migrations atomically with tracking.

    Migrations are tracked in the migration_history table to prevent duplicate
    execution. Each migration is timed and its status is recorded.

    All migrations are wrapped in a single atomic transaction to ensure
    all-or-nothing execution. If any migration fails, all changes in the
    current transaction block are rolled back.
    """
    from api.db.connection import TransactionLogger

    # Initialize tracking table first (before disabling logging)
    MigrationTracker.init_tracking_table()

    # Temporarily suppress verbose logs from peewee/playhouse during migrations
    # Save original log levels to restore them after migration completes
    peewee_logger = logging.getLogger("peewee")
    playhouse_logger = logging.getLogger("playhouse")
    original_peewee_level = peewee_logger.level
    original_playhouse_level = playhouse_logger.level

    peewee_logger.setLevel(logging.CRITICAL)
    playhouse_logger.setLevel(logging.CRITICAL)
    migrator = DatabaseMigrator[settings.DATABASE_TYPE.upper()].value(DB)

    # Define all migrations with unique names
    migrations = [
        (
            "add_file_source_type",
            lambda: alter_db_add_column(migrator, "file", "source_type", CharField(max_length=128, null=False, default="", help_text="where dose this document come from", index=True)),
        ),
        (
            "add_tenant_rerank_id",
            lambda: alter_db_add_column(migrator, "tenant", "rerank_id", CharField(max_length=128, null=False, default="BAAI/bge-reranker-v2-m3", help_text="default rerank model ID")),
        ),
        ("add_dialog_rerank_id", lambda: alter_db_add_column(migrator, "dialog", "rerank_id", CharField(max_length=128, null=False, default="", help_text="default rerank model ID"))),
        ("alter_dialog_top_k", lambda: alter_db_column_type(migrator, "dialog", "top_k", IntegerField(default=1024))),
        ("add_tenant_llm_api_key", lambda: alter_db_add_column(migrator, "tenant_llm", "api_key", CharField(max_length=2048, null=True, help_text="API KEY", index=True))),
        ("add_api_token_source", lambda: alter_db_add_column(migrator, "api_token", "source", CharField(max_length=16, null=True, help_text="none|agent|dialog", index=True))),
        ("add_tenant_tts_id", lambda: alter_db_add_column(migrator, "tenant", "tts_id", CharField(max_length=256, null=True, help_text="default tts model ID", index=True))),
        ("add_api_4_conversation_source", lambda: alter_db_add_column(migrator, "api_4_conversation", "source", CharField(max_length=16, null=True, help_text="none|agent|dialog", index=True))),
        ("add_task_retry_count", lambda: alter_db_add_column(migrator, "task", "retry_count", IntegerField(default=0))),
        ("alter_api_token_dialog_id", lambda: alter_db_column_type(migrator, "api_token", "dialog_id", CharField(max_length=32, null=True, index=True))),
        ("add_tenant_llm_max_tokens", lambda: alter_db_add_column(migrator, "tenant_llm", "max_tokens", IntegerField(default=8192, index=True))),
        ("add_api_4_conversation_dsl", lambda: alter_db_add_column(migrator, "api_4_conversation", "dsl", JSONField(null=True, default=dict))),
        ("add_knowledgebase_pagerank", lambda: alter_db_add_column(migrator, "knowledgebase", "pagerank", IntegerField(default=0, index=False))),
        ("add_api_token_beta", lambda: alter_db_add_column(migrator, "api_token", "beta", CharField(max_length=255, null=True, index=True))),
        ("add_task_digest", lambda: alter_db_add_column(migrator, "task", "digest", TextField(null=True, help_text="task digest", default=""))),
        ("add_task_chunk_ids", lambda: alter_db_add_column(migrator, "task", "chunk_ids", LongTextField(null=True, help_text="chunk ids", default=""))),
        ("add_conversation_user_id", lambda: alter_db_add_column(migrator, "conversation", "user_id", CharField(max_length=255, null=True, help_text="user_id", index=True))),
        ("add_document_meta_fields", lambda: alter_db_add_column(migrator, "document", "meta_fields", JSONField(null=True, default=dict))),
        ("add_task_task_type", lambda: alter_db_add_column(migrator, "task", "task_type", CharField(max_length=32, null=False, default=""))),
        ("add_task_priority", lambda: alter_db_add_column(migrator, "task", "priority", IntegerField(default=0))),
        ("add_user_canvas_permission", lambda: alter_db_add_column(migrator, "user_canvas", "permission", CharField(max_length=16, null=False, help_text="me|team", default="me", index=True))),
        ("add_llm_is_tools", lambda: alter_db_add_column(migrator, "llm", "is_tools", BooleanField(null=False, help_text="support tools", default=False))),
        ("add_mcp_server_variables", lambda: alter_db_add_column(migrator, "mcp_server", "variables", JSONField(null=True, help_text="MCP Server variables", default=dict))),
        ("rename_task_process_duation", lambda: alter_db_rename_column(migrator, "task", "process_duation", "process_duration")),
        ("rename_document_process_duation", lambda: alter_db_rename_column(migrator, "document", "process_duation", "process_duration")),
        ("add_document_suffix", lambda: alter_db_add_column(migrator, "document", "suffix", CharField(max_length=32, null=False, default="", help_text="The real file extension suffix", index=True))),
        ("add_api_4_conversation_errors", lambda: alter_db_add_column(migrator, "api_4_conversation", "errors", TextField(null=True, help_text="errors"))),
        ("add_dialog_meta_data_filter", lambda: alter_db_add_column(migrator, "dialog", "meta_data_filter", JSONField(null=True, default=dict))),
        ("alter_canvas_template_title", lambda: alter_db_column_type(migrator, "canvas_template", "title", JSONField(null=True, default=dict, help_text="Canvas title"))),
        ("alter_canvas_template_description", lambda: alter_db_column_type(migrator, "canvas_template", "description", JSONField(null=True, default=dict, help_text="Canvas description"))),
        (
            "add_user_canvas_category",
            lambda: alter_db_add_column(migrator, "user_canvas", "canvas_category", CharField(max_length=32, null=False, default="agent_canvas", help_text="agent_canvas|dataflow_canvas", index=True)),
        ),
        (
            "add_canvas_template_category",
            lambda: alter_db_add_column(
                migrator, "canvas_template", "canvas_category", CharField(max_length=32, null=False, default="agent_canvas", help_text="agent_canvas|dataflow_canvas", index=True)
            ),
        ),
        ("add_knowledgebase_pipeline_id", lambda: alter_db_add_column(migrator, "knowledgebase", "pipeline_id", CharField(max_length=32, null=True, help_text="Pipeline ID", index=True))),
        ("add_document_pipeline_id", lambda: alter_db_add_column(migrator, "document", "pipeline_id", CharField(max_length=32, null=True, help_text="Pipeline ID", index=True))),
        (
            "add_knowledgebase_graphrag_task_id",
            lambda: alter_db_add_column(migrator, "knowledgebase", "graphrag_task_id", CharField(max_length=32, null=True, help_text="Graph RAG task ID", index=True)),
        ),
        ("add_knowledgebase_raptor_task_id", lambda: alter_db_add_column(migrator, "knowledgebase", "raptor_task_id", CharField(max_length=32, null=True, help_text="RAPTOR task ID", index=True))),
        ("add_knowledgebase_graphrag_finish_at", lambda: alter_db_add_column(migrator, "knowledgebase", "graphrag_task_finish_at", DateTimeField(null=True))),
        ("add_knowledgebase_raptor_finish_at", lambda: alter_db_add_column(migrator, "knowledgebase", "raptor_task_finish_at", DateTimeField(null=True))),
        ("add_knowledgebase_mindmap_task_id", lambda: alter_db_add_column(migrator, "knowledgebase", "mindmap_task_id", CharField(max_length=32, null=True, help_text="Mindmap task ID", index=True))),
        ("add_knowledgebase_mindmap_finish_at", lambda: alter_db_add_column(migrator, "knowledgebase", "mindmap_task_finish_at", DateTimeField(null=True))),
        ("alter_tenant_llm_api_key_text", lambda: alter_db_column_type(migrator, "tenant_llm", "api_key", TextField(null=True, help_text="API KEY"))),
        (
            "add_tenant_llm_status",
            lambda: alter_db_add_column(migrator, "tenant_llm", "status", CharField(max_length=1, null=False, help_text="is it validate(0: wasted, 1: validate)", default="1", index=True)),
        ),
        ("alter_user_login_channel_max_length", lambda: alter_db_column_type(migrator, "user", "login_channel", CharField(max_length=64, null=True, help_text="from which user login"))),
        ("add_connector2kb_auto_parse", lambda: alter_db_add_column(migrator, "connector2kb", "auto_parse", CharField(max_length=1, null=False, default="1", index=False))),
        ("add_llm_factories_rank", lambda: alter_db_add_column(migrator, "llm_factories", "rank", IntegerField(default=0, index=False))),
        # File2Document junction table integrity enforcement
        ("file2document_cleanup_orphans", cleanup_file2document_orphans),  # Must run first to remove NULL values
        ("file2document_make_file_id_not_null", lambda: alter_db_add_not_null(migrator, "file2document", "file_id")),
        ("file2document_make_document_id_not_null", lambda: alter_db_add_not_null(migrator, "file2document", "document_id")),
        ("file2document_add_unique_constraint", lambda: alter_db_add_index(migrator, "file2document", ["file_id", "document_id"], unique=True)),
        # Evaluation results integrity: one result per (run_id, case_id)
        ("evaluation_results_add_unique_run_case", lambda: alter_db_add_index(migrator, "evaluation_results", ["run_id", "case_id"], unique=True)),
        # Pre-check FK integrity before adding constraints
        ("evaluation_fk_precheck", check_evaluation_fk_integrity),
        # Evaluation foreign key constraints for referential integrity
        (
            "evaluation_cases_fk_dataset",
            lambda: add_fk_constraint_if_not_exists(
                "fk_evaluation_cases_dataset_id",
                "ALTER TABLE evaluation_cases ADD CONSTRAINT fk_evaluation_cases_dataset_id FOREIGN KEY (dataset_id) REFERENCES evaluation_datasets(id) ON DELETE CASCADE ON UPDATE CASCADE",
            ),
        ),
        (
            "evaluation_runs_fk_dataset",
            lambda: add_fk_constraint_if_not_exists(
                "fk_evaluation_runs_dataset_id",
                "ALTER TABLE evaluation_runs ADD CONSTRAINT fk_evaluation_runs_dataset_id FOREIGN KEY (dataset_id) REFERENCES evaluation_datasets(id) ON DELETE CASCADE ON UPDATE CASCADE",
            ),
        ),
        (
            "evaluation_runs_fk_dialog",
            lambda: add_fk_constraint_if_not_exists(
                "fk_evaluation_runs_dialog_id",
                "ALTER TABLE evaluation_runs ADD CONSTRAINT fk_evaluation_runs_dialog_id FOREIGN KEY (dialog_id) REFERENCES dialog(id) ON DELETE CASCADE ON UPDATE CASCADE",
            ),
        ),
        (
            "evaluation_results_fk_run",
            lambda: add_fk_constraint_if_not_exists(
                "fk_evaluation_results_run_id",
                "ALTER TABLE evaluation_results ADD CONSTRAINT fk_evaluation_results_run_id FOREIGN KEY (run_id) REFERENCES evaluation_runs(id) ON DELETE CASCADE ON UPDATE CASCADE",
            ),
        ),
        (
            "evaluation_results_fk_case",
            lambda: add_fk_constraint_if_not_exists(
                "fk_evaluation_results_case_id",
                "ALTER TABLE evaluation_results ADD CONSTRAINT fk_evaluation_results_case_id FOREIGN KEY (case_id) REFERENCES evaluation_cases(id) ON DELETE CASCADE ON UPDATE CASCADE",
            ),
        ),
        # Memory model constraints
        ("memory_add_permissions_constraint", lambda: add_memory_permissions_constraint(migrator)),
        ("memory_add_forgetting_policy_constraint", lambda: add_memory_forgetting_policy_constraint(migrator)),
    ]

    # Wrap all migrations in a single atomic transaction
    # This ensures all-or-nothing execution with automatic rollback on failure
    migration_start = time.time()
    successful_migrations = []
    failed_migration = None
    failed_error = None
    failed_duration = None

    try:
        with DB.atomic():
            TransactionLogger.log_transaction_state(DB, "begin", "migration batch")

            # Execute migrations with tracking
            for migration_name, migration_fn in migrations:
                # Skip if already applied
                if MigrationTracker.has_migration_run(migration_name):
                    logging.debug(f"Skipping already-applied migration: {migration_name}")
                    continue

                # Execute migration with timing
                start_time = time.time()
                try:
                    migration_fn()
                    duration_ms = int((time.time() - start_time) * 1000)
                    MigrationTracker.record_migration(migration_name, "success", duration_ms=duration_ms)
                    successful_migrations.append(migration_name)
                except Exception as ex:
                    # Capture failure details before transaction rollback
                    duration_ms = int((time.time() - start_time) * 1000)
                    failed_migration = migration_name
                    failed_error = str(ex)
                    failed_duration = duration_ms
                    TransactionLogger.log_transaction_error(DB, ex, f"migration '{migration_name}'")
                    # Re-raise to trigger rollback of entire transaction
                    raise

            TransactionLogger.log_transaction_state(DB, "commit", f"{len(successful_migrations)} migrations applied")

    except Exception as ex:
        # Transaction automatically rolled back by context manager
        total_duration = int((time.time() - migration_start) * 1000)
        TransactionLogger.log_transaction_state(DB, "rollback", f"migration '{failed_migration}' failed after {total_duration}ms, rolled back {len(successful_migrations)} successful migrations")
        logging.error(f"Migration batch rolled back due to failure in '{failed_migration}': {ex}. All {len(successful_migrations)} successful migrations in this batch were rolled back.")

        # Record the failure outside the rolled-back transaction
        # This ensures the failure record persists even though the migration was rolled back
        if failed_migration is not None:
            try:
                MigrationTracker.record_migration(failed_migration, "failed", error=failed_error, duration_ms=failed_duration)
            except Exception as record_ex:  # noqa: BLE001
                # If we can't record the failure, log but don't fail the rollback
                logging.warning(f"Failed to record migration failure for {failed_migration}: {record_ex}")

        # Re-raise to notify caller of failure
        raise
    finally:
        # Restore original logger levels
        peewee_logger.setLevel(original_peewee_level)
        playhouse_logger.setLevel(original_playhouse_level)


@DB.connection_context()
@DB.lock("init_database_tables", 60)
def init_database_tables(alter_fields=None):
    if alter_fields is None:
        alter_fields = []

    # Models are already imported at module level via 'from api.db.models import *'
    # Scan all loaded modules for DataBaseModel subclasses
    table_objs = []
    create_failed_list = []

    # Get all classes from all loaded modules
    for module_name, module in sys.modules.items():
        if module_name.startswith("api.db.models"):
            members = inspect.getmembers(module, inspect.isclass)
            for class_name, obj in members:
                try:
                    # Check if it's a DataBaseModel subclass (but not DataBaseModel itself)
                    if obj != DataBaseModel and issubclass(obj, DataBaseModel):
                        # Avoid duplicates (same class imported in multiple modules)
                        if obj not in table_objs:
                            table_objs.append(obj)
                            logging.debug(f"Found model class: {class_name} from {module_name}")
                except TypeError:
                    # issubclass() raises TypeError if obj is not a class
                    continue

    # Validate that we found models
    if not table_objs:
        raise Exception("FATAL: No DataBaseModel classes found! Check api/db/models/ imports")

    logging.info(f"Found {len(table_objs)} model classes to initialize")

    # Create tables
    for obj in table_objs:
        if not obj.table_exists():
            logging.info(f"Creating table: {obj._meta.table_name} (model: {obj.__name__})")
            try:
                obj.create_table(safe=True)
                logging.info(f"✓ Created table: {obj._meta.table_name}")
            except Exception as ex:  # noqa: BLE001
                logging.error(f"✗ Failed to create table {obj._meta.table_name}: {ex}")
                logging.exception(ex)
                create_failed_list.append(obj.__name__)
        else:
            logging.debug(f"Table {obj._meta.table_name} already exists, skipping")

    if create_failed_list:
        logging.error(f"create tables failed: {create_failed_list}")
        raise Exception(f"create tables failed: {create_failed_list}")
    migrate_db()


def fill_db_model_object(model_object, human_model_dict):
    for k, v in human_model_dict.items():
        attr_name = f"{k}"
        if hasattr(model_object.__class__, attr_name):
            setattr(model_object, attr_name, v)
    return model_object
