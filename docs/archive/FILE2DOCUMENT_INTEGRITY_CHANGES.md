# File2Document Model Integrity Enhancement

## Summary

Enhanced the `File2Document` junction table to enforce referential integrity and prevent orphaned records by making `file_id` and `document_id` required fields with proper validation and database constraints.

## Changes Made

### 1. Model Changes ([api/db/models/knowledge.py](api/db/models/knowledge.py))

#### Updated File2Document Class
- **Made fields non-nullable**: Changed `file_id` and `document_id` from `null=True` to `null=False`
- **Added composite unique constraint**: Prevents duplicate file-document relationships
- **Added model-level validation**: Overrode `save()` method to validate both fields are present
- **Added documentation**: Comprehensive docstrings explaining the constraints and validation

```python
class File2Document(DataBaseModel):
    """
    Junction table linking File and Document entities.
    
    Enforces referential integrity by requiring both file_id and document_id.
    The composite index prevents duplicate relationships and improves query performance.
    """
    id = CharField(max_length=32, primary_key=True)
    file_id = CharField(max_length=32, null=False, help_text="file id", index=True)
    document_id = CharField(max_length=32, null=False, help_text="document id", index=True)

    class Meta:
        db_table = "file2document"
        indexes = (
            # Composite unique constraint to prevent duplicate file-document relationships
            (("file_id", "document_id"), True),  # True = unique constraint
        )
    
    def save(self, *args, **kwargs):
        """
        Override save to validate required fields before persisting.
        
        Raises:
            FieldValueRequiredException: If file_id or document_id is None or empty
        """
        if not self.file_id or not self.document_id:
            raise FieldValueRequiredException(
                "Both file_id and document_id are required for File2Document relationships"
            )
        return super().save(*args, **kwargs)
```

### 2. Exception Class ([common/exceptions.py](common/exceptions.py))

Added new exception for field validation:

```python
class FieldValueRequiredException(Exception):
    """
    Raised when a required field value is missing or empty.
    
    Used to enforce model-level validation for critical relationships
    and prevent orphaned records in the database.
    """
    def __init__(self, msg):
        self.msg = msg
        super().__init__(msg)
```

### 3. Migration Functions ([api/db/migrations.py](api/db/migrations.py))

#### Added Helper Functions

**`alter_db_add_not_null()`**: Makes columns non-nullable
```python
def alter_db_add_not_null(migrator, table_name, column_name):
    """
    Make a column non-nullable with standardized error handling.
    
    This should only be called after ensuring all existing rows have non-null values
    in the target column, otherwise the migration will fail.
    """
```

**`alter_db_add_index()`**: Adds single or composite indexes
```python
def alter_db_add_index(migrator, table_name, columns, unique=False):
    """
    Add an index (composite or single) to a table with standardized error handling.
    
    Args:
        migrator: Database migrator instance
        table_name: Name of the table
        columns: List of column names or single column name
        unique: Whether to create a unique index
    """
```

**`cleanup_file2document_orphans()`**: Removes orphaned records
```python
def cleanup_file2document_orphans():
    """
    Remove File2Document records with NULL file_id or document_id.
    
    This cleanup must run before making these columns non-nullable.
    Orphaned records are invalid and should be removed.
    """
```

#### Added Migrations

Four new migrations in the `migrate_db()` function:

1. **`file2document_cleanup_orphans`**: Removes any existing records with NULL values
2. **`file2document_make_file_id_not_null`**: Makes `file_id` column non-nullable
3. **`file2document_make_document_id_not_null`**: Makes `document_id` column non-nullable
4. **`file2document_add_unique_constraint`**: Adds composite unique index on (file_id, document_id)

### 4. Tests ([test/test_file2document_validation.py](test/test_file2document_validation.py))

Created comprehensive test suite covering:
- Model validation for missing `file_id`
- Model validation for missing `document_id`
- Model validation for empty string values
- Successful save with valid fields
- Migration cleanup logic
- Composite unique constraint definition
- Non-nullable field requirements

All 7 tests pass successfully.

## Benefits

1. **Data Integrity**: Prevents orphaned File2Document records
2. **Referential Integrity**: Ensures all junction records have valid foreign keys
3. **Duplicate Prevention**: Composite unique constraint prevents duplicate relationships
4. **Early Validation**: Model-level validation catches issues before database operations
5. **Better Performance**: Unique index improves query performance
6. **Backward Compatible**: Existing code already provides both fields, so no breaking changes

## Migration Safety

The migrations are designed to be safe:

1. **Cleanup First**: Removes any existing orphaned records before adding constraints
2. **Tracked Execution**: Uses the MigrationHistory system to prevent duplicate runs
3. **Atomic Transaction**: All migrations run in a single transaction with automatic rollback on failure
4. **Standardized Error Handling**: Uses StandardErrorHandler for consistent error management
5. **Database Agnostic**: Works with both MySQL and PostgreSQL

## Verification Steps

1. ✅ Code compiles without errors
2. ✅ All tests pass (7/7)
3. ✅ Linting passes with no issues
4. ✅ Existing usages verified to provide both fields
5. ✅ Migration functions added and tested

## How to Apply

The migrations will automatically run when the application starts:

```bash
# Migrations run automatically during application initialization
docker-compose up -d
```

Or run manually:

```bash
source .venv/bin/activate
export PYTHONPATH=$(pwd)
python -c "from api.db.migrations import migrate_db; migrate_db()"
```

## Rollback (if needed)

If rollback is needed, the migrations can be manually reversed:

1. Drop the unique constraint
2. Alter columns to allow NULL
3. Remove from migration_history table

However, this should only be done in exceptional circumstances, as the changes enforce data integrity.
