# StandardErrorHandler Usage Guide

## Overview

The `StandardErrorHandler` class in `api/db/migrations.py` provides centralized, consistent error handling for database migration operations across MySQL and PostgreSQL databases.

## Quick Start

### Basic Usage

```python
from api.db.migrations import StandardErrorHandler
from common import settings

# In your migration function
try:
    # perform migration operation
    migrate(migrator.add_column("table", "column", CharField()))
except Exception as ex:
    StandardErrorHandler.handle_migration_error(
        ex, 
        table="table",
        column="column",
        operation="add_column",
        db_type=settings.DATABASE_TYPE.lower()
    )
```

## API Reference

### `StandardErrorHandler.categorize_error(exception, db_type="postgres")`

Categorizes an exception into a known error type.

**Parameters:**
- `exception` (Exception): The exception to categorize
- `db_type` (str): Database type - "mysql" or "postgres" (case-insensitive)

**Returns:**
- `tuple`: `(category, is_expected, should_skip)`
  - `category` (str): One of "duplicate_column", "incompatible_type", "missing_column", or "unknown"
  - `is_expected` (bool): Whether this is a known/expected error
  - `should_skip` (bool): Whether the migration should be skipped

**Example:**

```python
try:
    # attempt migration
except Exception as ex:
    category, is_expected, should_skip = StandardErrorHandler.categorize_error(
        ex, db_type="postgres"
    )
    
    if should_skip:
        print(f"Migration already applied: {category}")
    elif is_expected:
        print(f"Expected issue encountered: {category}")
    else:
        print(f"Unexpected error: {category}")
```

### `StandardErrorHandler.handle_migration_error(exception, table, column, operation, db_type="postgres")`

Handles migration error with appropriate logging and returns skip flag.

**Parameters:**
- `exception` (Exception): The exception to handle
- `table` (str): Table name where operation was attempted
- `column` (str): Column name involved in operation
- `operation` (str): Operation name ("add_column", "alter_column_type", "rename_column")
- `db_type` (str): Database type (default: "postgres")

**Returns:**
- `None`: This method does not return a value. It performs error categorization and logging only. For migration flows requiring skip logic, callers should check the `should_skip` flag from `categorize_error()` before handling (see `if should_skip: return` pattern in examples). The method returns early when `should_skip=True` (duplicate/already-applied errors) or `is_expected=True` (expected issues), allowing graceful continuation without raising exceptions.

**Logging Behavior:**
- **DEBUG**: Duplicate column errors (operation already applied)
- **WARNING**: Expected issues (incompatible type, missing column)
- **CRITICAL**: Unexpected/unknown errors

**Example:**

```python
def alter_db_add_column(migrator, table_name, column_name, column_type):
    try:
        from playhouse.migrate import migrate
        migrate(migrator.add_column(table_name, column_name, column_type))
        logging.debug(f"Added column: {table_name}.{column_name}")
    except Exception as ex:
        StandardErrorHandler.handle_migration_error(
            ex, table_name, column_name, "add_column",
            db_type=settings.DATABASE_TYPE.lower()
        )
```

## Error Categories

### Duplicate Column
- **Detection**: MySQL error 1060 or PostgreSQL error 42701
- **Meaning**: Column already exists (likely from previous migration run)
- **Action**: Migration is safely skipped
- **Log Level**: DEBUG

### Incompatible Type
- **Detection**: MySQL errors 1062/1064 or PostgreSQL errors 42804/42P07
- **Meaning**: Column type cannot be changed to requested type
- **Action**: Migration fails but is logged as expected
- **Log Level**: WARNING

### Missing Column
- **Detection**: MySQL error 1054 or PostgreSQL error 42703
- **Meaning**: Column doesn't exist (may be dropped or never created)
- **Action**: Migration fails but is logged as expected
- **Log Level**: WARNING

### Unknown Error
- **Detection**: Any error not matching above patterns
- **Meaning**: Unexpected database error requiring investigation
- **Action**: Migration fails and is logged critically
- **Log Level**: CRITICAL

## Error Detection Methods

The handler uses multiple detection methods for robustness:

### 1. PostgreSQL SQLSTATE Code

```python
if exception.pgcode == "42701":  # Duplicate column
    return ("duplicate_column", True, True)
```

### 2. MySQL Error Code

```python
if exception.args[0] == 1060:  # Duplicate column
    return ("duplicate_column", True, True)
```

### 3. Error Message Pattern

```python
if "duplicate column" in str(exception).lower():
    return ("duplicate_column", True, True)
```

## Common Scenarios

### Scenario 1: Running Migration Twice

```python
# First run: Adds the column successfully
alter_db_add_column(migrator, "users", "email", CharField())
# Logs: "Added column: users.email" (DEBUG)

# Second run: Column already exists
alter_db_add_column(migrator, "users", "email", CharField())
# Logs: "Migration add_column skipped (already applied): POSTGRES.users.email - duplicate_column" (DEBUG)
# Returns gracefully without error
```

### Scenario 2: Type Mismatch

```python
# Attempting to change TEXT to INTEGER fails
alter_db_column_type(migrator, "documents", "content", IntegerField())
# Logs: "Migration alter_column_type encountered expected issue: POSTGRES.documents.content - incompatible_type: ..." (WARNING)
# Returns gracefully, developer can investigate
```

### Scenario 3: Unexpected Error

```python
# Database connection lost during migration
alter_db_add_column(migrator, "users", "profile", JSONField())
# Logs: "Migration add_column failed with unexpected error: POSTGRES.users.profile: Server connection lost" (CRITICAL)
# Developer must investigate and fix the underlying issue
```

## Testing

The handler includes comprehensive unit tests:

```bash
pytest test/unit_test/api_db/test_error_handler.py -v
# Result: 22 passed tests covering all scenarios
```

## Best Practices

1. **Always Pass db_type**

   ```python
   # Good
   StandardErrorHandler.handle_migration_error(ex, table, column, op, db_type=settings.DATABASE_TYPE.lower())
   
   # Okay (uses default)
   StandardErrorHandler.handle_migration_error(ex, table, column, op)
   ```

2. **Log Success**

   ```python
   try:
       perform_migration()
       logging.debug(f"Migration successful: {details}")
   except Exception as ex:
       StandardErrorHandler.handle_migration_error(...)
   ```

3. **Use Consistent Operation Names**
   - "add_column" for adding columns
   - "alter_column_type" for changing column types
   - "rename_column" for renaming columns

4. **Don't Catch After Handling**

   ```python
   # Wrong - error was already handled and logged
   except Exception as ex:
       StandardErrorHandler.handle_migration_error(...)
       raise  # Don't re-raise
   
   # Correct
   except Exception as ex:
       StandardErrorHandler.handle_migration_error(...)
       # Returns normally, no need to raise
   ```

## Extending the Handler

To add support for new error types:

1. **Add error codes to ERROR_CODES**

   ```python
   ERROR_CODES = {
       "new_error_type": {
           "mysql": [error_code],
           "postgres": ["sqlstate_code"]
       }
   }
   ```

2. **Add message patterns to ERROR_MESSAGES**

   ```python
   ERROR_MESSAGES = {
       "new_error_type": ["pattern1", "pattern2"]
   }
   ```

3. **Update categorize_error() if needed**

   ```python
   # Automatic detection in loop, may need custom logic
   ```

4. **Add test cases**

   ```python
   def test_new_error_type_detection(self):
       exc = MockException(...)
       category, is_expected, should_skip = StandardErrorHandler.categorize_error(exc)
       self.assertEqual(category, "new_error_type")
   ```

## Troubleshooting

### "Unknown error" logged but you expected better categorization

1. Check if the error code is in ERROR_CODES
2. Check if the error message contains any ERROR_MESSAGES patterns
3. Add the missing error code or pattern
4. Add a test case to verify detection

### Migration silently succeeds but nothing changed

Check the debug logs:

```bash
# Enable debug logging
export LOG_LEVEL=DEBUG
python api/ragflow_server.py
# Look for "Migration add_column skipped" messages
```

### Different behavior on MySQL vs PostgreSQL

1. Verify error codes are correct for both databases
2. Check that message patterns work for both
3. Test migration on both database types
4. Add cross-database test case if needed

## Related Files

- **Implementation**: `api/db/migrations.py`
- **Tests**: `test/unit_test/api_db/test_error_handler.py`
- **Plan**: `DATABASE_IMPROVEMENTS_PLAN.md`
- **Report**: `PHASE_1_COMPLETION_REPORT.md`
