# Database Migration Guide

This guide explains how database migrations work in RAGFlow, how to add new migrations, and how to troubleshoot migration issues.

## Overview

RAGFlow uses an automated migration system that tracks database schema changes and prevents duplicate migration execution. The system supports both MySQL and PostgreSQL with database-agnostic migration logic.

### Key Features

- **Duplicate Prevention**: Migrations are only applied once per database instance
- **Execution Tracking**: All migrations are recorded with timestamp, status, and duration
- **Error Handling**: Expected errors (like duplicate columns) are distinguished from critical failures
- **Atomic Operations**: Migrations are wrapped in transactions for data consistency (transactional DDL on PostgreSQL; best-effort atomicity on MySQL due to implicit commits)
- **Cross-Database Support**: Same migrations work on MySQL and PostgreSQL

## Understanding Migrations

### Migration History Table

Each database maintains a `migration_history` table that tracks all applied migrations:

```sql
-- PostgreSQL syntax
CREATE TABLE migration_history (
    id SERIAL PRIMARY KEY,
    migration_name VARCHAR(255) UNIQUE NOT NULL,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(16) DEFAULT 'success',  -- 'success' | 'failed' | 'skipped'
    error_message TEXT,
    duration_ms INTEGER,
    db_type VARCHAR(16)  -- 'mysql' | 'postgres'
);

-- MySQL syntax
CREATE TABLE migration_history (
    id INT AUTO_INCREMENT PRIMARY KEY,
    migration_name VARCHAR(255) UNIQUE NOT NULL,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(16) DEFAULT 'success',  -- 'success' | 'failed' | 'skipped'
    error_message TEXT,
    duration_ms INTEGER,
    db_type VARCHAR(16)  -- 'mysql' | 'postgres'
);
```

**Note**: PostgreSQL uses `SERIAL` for auto-incrementing primary keys, while MySQL uses `INT AUTO_INCREMENT`.

### Migration Lifecycle

When migrations run:

1. **Initialization**: `MigrationTracker.init_tracking_table()` creates the tracking table if needed
2. **Check**: For each migration, check if it has already been successfully applied
3. **Execute**: If not yet applied, execute the migration operation
4. **Record**: Log the result (success, failed, or skipped) with metadata
5. **Atomicity**: 
   - **PostgreSQL**: Full transactional atomicity — all migrations in a run are wrapped in a single transaction and can be rolled back as a unit
   - **MySQL**: No transactional DDL support — each DDL statement (ALTER TABLE, CREATE INDEX, etc.) causes an implicit commit, so rollbacks cannot undo DDL changes. Migrations are not fully atomic on MySQL.
   - **Recommendation for MySQL users**: Test migrations on a backup first, use smaller reversible steps, or maintain database backups before running migrations

### Migration Status Values

- **success**: Migration completed without errors
- **failed**: Migration encountered an unexpected error
- **skipped**: Migration was already applied and was skipped

## Running Migrations

### Automatic Migrations on Startup

Migrations automatically run when the RAGFlow API server starts:

```bash
export PYTHONPATH=$(pwd)
bash docker/launch_backend_service.sh
```

The server will:

1. Initialize the migration tracking table
2. Check which migrations have already been applied
3. Apply any new migrations in sequence
4. Log results to the server logs

### Manual Migration Execution

To manually trigger migrations from Python:

```python
from api.db.migrations import migrate_db

# Run all pending migrations
migrate_db()
```

To check migration status:

```python
from api.db.migrations import MigrationTracker

# Get all applied migrations
history = MigrationTracker.get_migration_history()
for record in history:
    print(f"{record.migration_name}: {record.status} "
          f"({record.duration_ms}ms on {record.applied_at})")

# Check if specific migration has run
has_run = MigrationTracker.has_migration_run("add_file_source_type")
print(f"Migration 'add_file_source_type': {has_run}")
```

## Adding New Migrations

### Step 1: Define the Migration Function

In [api/db/migrations.py](../api/db/migrations.py), add a new migration function:

```python
def add_my_new_column(migrator):
    """
    Add a new column to track feature X.
    
    Handles duplicate column errors gracefully for
    idempotent execution.
    """
    alter_db_add_column(
        migrator, 
        "documents",          # table name
        "feature_x_score",    # column name
        FloatField(null=True) # field type
    )
```

**Important**: Use the `alter_db_add_column()` helper function which includes:

- Consistent error handling
- Appropriate logging levels
- Cross-database compatibility

### Step 2: Register the Migration

In the `migrate_db()` function, add your migration to the migrations list:

```python
def migrate_db():
    """Apply migrations with tracking"""
    MigrationTracker.init_tracking_table()
    logging.disable(logging.ERROR)
    migrator = DatabaseMigrator[settings.DATABASE_TYPE.upper()].value(DB)
    
    migrations = [
        ("add_file_source_type", lambda: alter_db_add_column(...)),
        ("add_tenant_rerank_id", lambda: alter_db_add_column(...)),
        # ... existing migrations ...
        ("add_my_new_column", lambda: add_my_new_column(migrator)),  # NEW
    ]
    
    # ... rest of function ...
```

**Key Rules**:

- Use a unique, descriptive migration name (e.g., `"add_column_name"`)
- Migration names should be descriptive and concise
- Order matters: list migrations in the order they should be applied
- Don't reuse migration names or change existing ones

### Step 3: Test the Migration

Test your migration with both databases:

```bash
# Test with PostgreSQL (default)
docker compose -f docker/docker-compose-base.yml up -d
uv run pytest test/test_db_migrations.py -k "migration" -v

# Test with MySQL
export DATABASE_TYPE=mysql
uv run pytest test/test_db_migrations.py -k "migration" -v
```

## Error Handling

### Expected Errors

Common expected errors are automatically handled:

- **Duplicate Column**: Column already exists (logged as DEBUG)
- **Incompatible Type**: Type mismatch or casting issue (logged as WARNING)
- **Missing Column**: Column doesn't exist (logged as WARNING)

Example log output:

```
DEBUG: Migration add_column skipped (already applied): POSTGRES.documents.new_field
WARNING: Migration alter_column encountered expected issue: MYSQL.documents.field - incompatible_type
```

### Unexpected Errors

Errors that don't match known patterns are logged as CRITICAL:

```
CRITICAL: Migration add_column failed with unexpected error: POSTGRES.documents.field: ...
```

### Troubleshooting

If a migration fails:

1. **Check the logs**: Look for the error message and category
2. **Review the query**: See what the migration was trying to do
3. **Check the database**: Verify the current schema state
4. **Check migration history**: Query the tracking table:

```python
from api.db.migrations import MigrationHistory
from peewee import fn

# Get failed migrations
failed = MigrationHistory.select().where(
    MigrationHistory.status == "failed"
)
for record in failed:
    print(f"{record.migration_name}: {record.error_message}")

# Get most recent migrations
recent = MigrationHistory.select().order_by(
    MigrationHistory.applied_at.desc()
).limit(10)
```

5. **Manual remediation**: If needed, manually fix the schema and mark the migration as skipped:

```python
from datetime import datetime
from api.db.migrations import MigrationHistory
from common import settings

# After manually fixing the schema issue (e.g., running ALTER TABLE directly)
# Record the migration as completed to prevent re-execution
MigrationHistory.create(
    migration_name="20250115_problematic_migration",  # Use the actual migration name
    status="success",  # or "skipped" if you chose to skip it
    duration_ms=0,
    db_type=settings.DATABASE_TYPE.lower(),
    applied_at=datetime.now(),
    error_message="Manually remediated after direct schema fix"
)
```

This marks the migration as completed in the history, preventing future attempts to re-run it.


## Database Compatibility

### MySQL-Specific Considerations

- VARCHAR max length: MySQL's 65,535 byte limit applies to the **entire row** (minus row overhead), not a single VARCHAR column. A single VARCHAR can theoretically hold up to 65,535 characters, but is constrained by:
  - Total row size limit (all columns combined)
  - Character set byte usage (e.g., `utf8mb4` uses up to 4 bytes per character)
  - Length prefix overhead (1-2 bytes depending on max length)
  - **Recommendation**: Use `TEXT` for very large column data or when content may exceed row limits
- JSON type is text-based, not optimized like PostgreSQL's JSONB
- DDL operations are not transactional
- Type casting is more limited

### PostgreSQL-Specific Considerations

- VARCHAR has no practical length limit
- Use JSONB for better performance with JSON data
- DDL is fully transactional
- Rich type casting support available

### Cross-Database Migrations

The `DatabaseCompat` class defines equivalence mappings:

```python
from api.db.migrations import DatabaseCompat

# Check database capabilities
if DatabaseCompat.is_capable(settings.DATABASE_TYPE.lower(), "jsonb_support"):
    # Use JSONB-specific features
    pass

# Get type equivalent for target database
mysql_type = DatabaseCompat.get_equivalent_type(
    "TEXT", 
    source_db="postgres", 
    target_db="mysql"
)
# Returns: "LONGTEXT"
```

## Migration Best Practices

### ✅ Do

- Keep migrations small and focused on one schema change
- Use descriptive migration names
- Add docstrings explaining why the change was needed
- Test with both MySQL and PostgreSQL
- Handle expected errors gracefully
- Reference the issue/feature number in the docstring

### ❌ Don't

- Reuse migration names (each migration must be unique)
- Perform complex data transformations in migrations (use separate scripts for long-running or complex data operations)
- Lock large tables for extended periods
- Omit error handling
- Skip database compatibility testing

## Performance Considerations

### Large Table Migrations

For migrations on large tables:

1. Add the column first (nullable)
2. Update data in batches separately
3. Add constraints in a follow-up migration

```python
def add_indexed_column_safe(migrator):
    """Add column to large table safely"""
    # Add nullable column first (fast)
    alter_db_add_column(migrator, "large_table", "new_field", CharField(null=True))
    
    # Update in batches (done separately, not in migration)
    # Add constraint in follow-up migration
```

### Monitoring Migration Duration

Check the migration history to monitor performance:

```python
from api.db.migrations import MigrationHistory

# Get slow migrations
slow = MigrationHistory.select().where(
    MigrationHistory.duration_ms > 1000  # > 1 second
)
```

## Reverting Migrations

Currently, the system doesn't support automatic rollback. If a migration causes issues:

1. **Identify the problem**: Check migration history and server logs
2. **Manual fix**: Apply the reverse schema change manually
3. **Mark as reverted**: Record the reversion:

```python
from datetime import datetime
from api.db.migrations import MigrationHistory
MigrationHistory.create(
    migration_name="revert_problematic_migration",
    status="success",
    duration_ms=0,
    db_type=settings.DATABASE_TYPE.lower(),
    applied_at=datetime.now()
)
```

4. **Future feature**: Full rollback support is planned for a future release

## Advanced Topics

### Custom Migration Logic

For complex migrations, create a custom function:

```python
def migrate_complex_change(migrator):
    """
    Complex migration with custom logic.
    
    This demonstrates how to do more than simple column operations.
    """
    # Phase 1: Add new column
    alter_db_add_column(migrator, "documents", "status_v2", CharField())
    
    # Phase 2: Simple data migration (if needed)
    # Note: Only trivial, atomic schema-aligned data tweaks are allowed.
    # Complex or long-running transformations must use separate scripts.
    
    # Phase 3: Drop old column (if needed)
    # This would be a separate migration
```

### Conditional Migrations

Apply migrations only for specific databases:

```python
from common import settings

if settings.DATABASE_TYPE.upper() == "POSTGRES":
    migrations.append(("postgres_specific_feature", lambda: add_postgres_feature(migrator)))
elif settings.DATABASE_TYPE.upper() == "MYSQL":
    migrations.append(("mysql_specific_feature", lambda: add_mysql_feature(migrator)))
```

## Getting Help

- **Server Logs**: Check RAGFlow logs for detailed migration messages
- **Migration History**: Query `migration_history` table for status
- **Error Messages**: Look for categorized error types (duplicate_column, incompatible_type, etc.)
- **Tests**: Run migration tests to validate changes
- **GitHub Issues**: Report migration issues with database logs attached

## Related Documentation

- [Connection Pool Monitoring](CONNECTION_POOL.md) - Monitor database connections
- [Database Compatibility](DATABASE_COMPATIBILITY.md) - Multi-database support details
- [AGENTS.md](../AGENTS.md) - Project build and coding standards
