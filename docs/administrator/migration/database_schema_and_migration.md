---
sidebar_position: 1
slug: /database_schema_and_migration
sidebar_custom_props: {
  categoryIcon: LucideLocateFixed
}
---

# Database schema and migration

Sync schemas and migrate data using official RAGFlow scripts.

---

RAGFlow handles schema updates and migrations automatically at startup. However, for high-volume environments like Kubernetes, massive datasets can cause initialization to exceed 10 minutes, potentially triggering container timeouts or health check failures. To avoid this, you can disable the built-in auto-initialization and manually run these provided scripts to complete database upgrades before launching the service:

- [mysql_migration.py](#mysql_migrationpy): Migrates data between MySQL tables.
- [db_schema_sync.py](#db_schema_syncpy): Syncs database schemas and manages changes using peewee-migrate.

## mysql_migration.py

The [mysql_migration.py](https://github.com/infiniflow/ragflow/blob/main/tools/scripts/mysql_migration.py) script is a specialized tool for re-organizing RAGFlow’s model-related data. It transitions data from older unified tables into a modern, multi-table structure to support advanced model management.

### Key functions

- **Sequential migration**: Moves data through three distinct stages—Provider, Instance, and Model—to maintain database integrity and satisfy dependencies.
- **Flexible setup**: Connects to MySQL using either a YAML configuration file or direct command-line arguments.
- **Execution control**: Offers three specific modes: dry-run (preview), table-only (structural setup), and execute (full data move).
- **Automated mapping**: Generates unique IDs and handles complex joins between legacy records and new table structures.
- **Batch logging**: Processes records in sets of 100 and provides a final summary of total duration and row counts.

### When to use

- **Version upgrades**: Essential when moving to RAGFlow v0.25 or later to ensure your models are correctly categorized in the new schema.
- **Data normalization**: Necessary when consolidating multiple API keys or LLM providers into the updated system format.
- **Kubernetes deployments**: Useful for setting up the database structure independently using the `--create-table-only` flag before main services start.
- **Migration verification**: Used in dry-run mode to identify any legacy records that still need to be moved to the new tables.

## db_schema_sync.py

The [db_schema_sync.py](https://github.com/infiniflow/ragflow/blob/main/tools/scripts/db_schema_sync.py) script is a synchronization utility that ensures your MySQL database structure matches the Peewee ORM models defined in the RAGFlow source code.

### Key functions

- **Change detection**: Compares Python model definitions in `api/db/db_models.py` against the live database to identify new tables, added fields, or type mismatches.
- **Migration generation**: Automatically creates Python migration files (containing `migrate()` and `rollback()` logic) in version-specific directories (e.g., `tools/migrate/v0_25_0/`).
- **Schema auditing**: Provides a `--diff` command to view structural discrepancies without applying changes.
- **Execution management**: Applies pending migrations to the database to bring it up to date with the current software version.
- **Safety controls**: Prevents accidental data loss by requiring an explicit `--drop` flag to generate `DROP COLUMN` statements for removed fields.

### When to use

- **Version upgrades**: When moving to a new version of RAGFlow that introduces structural database changes.
- **Development**: When modifying `db_models.py` and needing to update your local database without manual SQL.
- **CI/CD pipelines**: To automatically prepare or apply database updates during deployment.
- **Troubleshooting**: When the application fails due to "Unknown column" or "Table not found" errors, indicating a desynchronized schema.