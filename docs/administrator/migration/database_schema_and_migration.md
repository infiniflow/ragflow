---
sidebar_position: 1
title: Database Schema and Migration
sidebar_label: Database Schema and Migration
slug: /database_schema_and_migration
sidebar_custom_props: {
  categoryIcon: LucideLocateFixed
}
---

# Database Schema and Migration

Sync schemas and migrate data using official RAGFlow scripts.

---

RAGFlow handles schema updates and migrations automatically at startup. However, for high-volume environments like Kubernetes, massive datasets can cause initialization to exceed 10 minutes, potentially triggering container timeouts or health check failures. To avoid this, you can disable the built-in auto-initialization and manually run these provided scripts to complete database upgrades before launching the service:

- [mysql_migration.py](#mysql_migrationpy): Migrates data between MySQL tables.
- [db_schema_sync.py](#db_schema_syncpy): Syncs database schemas and manages changes using peewee-migrate.

## Mysql_migration.py

The [mysql_migration.py](https://github.com/infiniflow/ragflow/blob/main/tools/scripts/mysql_migration.py) script is a specialized tool for re-organizing RAGFlow’s model-related data. It transitions data from older unified tables into a modern, multi-table structure to support advanced model management.

### Key Functions

- **Sequential migration**: Moves data through three distinct stages—Provider, Instance, and Model—to maintain database integrity and satisfy dependencies.
- **Flexible setup**: Connects to MySQL using either a YAML configuration file or direct command-line arguments.
- **Execution control**: Offers three specific modes: dry-run (preview), table-only (structural setup), and execute (full data move).
- **Automated mapping**: Generates unique IDs and handles complex joins between legacy records and new table structures.
- **Batch logging**: Processes records in sets of 100 and provides a final summary of total duration and row counts.

### When to Use

- **Version upgrades**: Essential when moving to RAGFlow v0.25 or later to ensure your models are correctly categorized in the new schema.
- **Data normalization**: Necessary when consolidating multiple API keys or LLM providers into the updated system format.
- **Kubernetes deployments**: Useful for setting up the database structure independently using the `--create-table-only` flag before main services start.
- **Migration verification**: Used in dry-run mode to identify any legacy records that still need to be moved to the new tables.

## Db_schema_sync.py

The [db_schema_sync.py](https://github.com/infiniflow/ragflow/blob/main/tools/scripts/db_schema_sync.py) script is a synchronization utility that ensures your MySQL database structure matches the Peewee ORM models defined in the RAGFlow source code.

### Key Functions

- **Change detection**: Compares Python model definitions in `api/db/db_models.py` against the live database to identify new tables, added fields, or type mismatches.
- **Migration generation**: Automatically creates Python migration files (containing `migrate()` and `rollback()` logic) in version-specific directories (e.g., `tools/migrate/v0_27_1/`).
- **Schema auditing**: Provides a `--diff` command to view structural discrepancies without applying changes.
- **Execution management**: Applies pending migrations to the database to bring it up to date with the current software version.
- **Safety controls**: Prevents accidental data loss by requiring an explicit `--drop` flag to generate `DROP COLUMN` statements for removed fields.

### When to Use

- **Version upgrades**: When moving to a new version of RAGFlow that introduces structural database changes.
- **Development**: When modifying `db_models.py` and needing to update your local database without manual SQL.
- **CI/CD pipelines**: To automatically prepare or apply database updates during deployment.
- **Troubleshooting**: When the application fails due to "Unknown column" or "Table not found" errors, indicating a desynchronized schema.

## Ingestion task scheduling rollout

The ingestion scheduler adds lease and dispatch-tracking columns and composite
indexes to the hot `ingestion_task` table. In a high-volume installation, do
not rely on an application restart as the schema-change procedure:

1. Back up the database and apply the additive schema changes during a planned
   low-traffic window. Use an approved online-DDL tool such as `gh-ost` or
   `pt-online-schema-change` when the database and change are compatible with
   those tools; otherwise apply the DDL with the provider's online-DDL option
   and monitor lock wait time.
2. Verify the scheduling and claim indexes before starting new ingestors. The
   required definitions are `(status, last_dispatched_at, id)` and
   `(status, claim_expires_at, id)` on `ingestion_task`.
3. Before switching traffic, converge legacy `CREATED` rows to `SCHEDULED`.
   The new ingestor also continuously handles `CREATED` rows during a rolling
   deployment, so a short old-writer/new-reader overlap is recoverable.
4. Roll back only after testing the reverse version's handling of `SCHEDULED`
   rows. Keep the schema additions in place during rollback; removing lease
   columns or indexes while workers are running is not a rollback procedure.

The Go startup migration creates missing scheduling indexes as a fallback, but
it does not replace the online-DDL, backup, lock-monitoring, or rollback
procedure required for a large production table.
