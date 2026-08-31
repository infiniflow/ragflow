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
- [postgres_migration.py](#postgres_migrationpy): Same model-provider stages for PostgreSQL and GaussDB.
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

## Postgres_migration.py

The [postgres_migration.py](https://github.com/infiniflow/ragflow/blob/main/tools/scripts/postgres_migration.py) script is the PostgreSQL / GaussDB equivalent of `mysql_migration.py`. It shares the same stages, including:

- Creating `tenant_model_provider`, `tenant_model_instance`, and `tenant_model` when they are missing
- Merging `tenant_model.model_type` into an integer bitmask (`model_type_merge`)
- Converting integer `tenant_*_id` columns to `varchar(32)` **and** backfilling them from `llm_id` / `embd_id` by resolving `tenant_model.id` (`tenant_model_id_migration`)

`tools/scripts/run_migrations.sh` selects this script automatically when `DB_TYPE` is `postgres` or `gaussdb`. That pre-startup path is where column-type conversion and data stages run (`model_type_merge` and `tenant_model_id_migration`).

`migrate_db()` is a fallback when the pre-startup script did not run or did not finish:

| Database | `tenant_*_id` fallback in `migrate_db()` | `model_type` merge fallback in `migrate_db()` |
|----------|------------------------------------------|-----------------------------------------------|
| PostgreSQL / GaussDB | Yes (column retype) | Yes (`migrate_postgres_family_model_provider_tables()`, skipped when the version marker is already `v0.27.0`) |
| MySQL / OceanBase | Yes (column retype) | **No** — run `mysql_migration.py` / `run_migrations.sh` manually |

**Version marker:** `mysql_migration.database.version` is written only after every requested stage returns without raising (`mark_database_version_on_success`). A failed or interrupted run does not record `v0.27.0`, so the next script run or `migrate_db()` fallback retries. After a successful mark, later boots skip the fallback. Stages are idempotent (for example `model_type_merge` no-ops when `model_type` is already INT). Do not set this marker by hand after a partial upgrade.

Neither path ALTERs `tenant_model.model_type` to integer first — `tenant_model_seeding` and `model_type_merge` skip when that column is already an integer, which would leave unmerged duplicate rows.

**GaussDB rollout:** Existing GaussDB installations that previously skipped these stages will run `model_type_merge` and `tenant_model_id_migration` the first time `run_migrations.sh` or the `migrate_db()` fallback executes. That is data-affecting (row merge, table swap, `tenant_*_id` backfill); take a backup before upgrading production GaussDB metadata databases.

For GaussDB, connection parameters come from `GAUSSDB_METADATA_*` environment variables.

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
