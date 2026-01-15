# Database Compatibility Guide

This guide explains RAGFlow's multi-database support strategy, database-specific features, field compatibility matrix, and how to extend support to additional databases.

## Overview

RAGFlow is designed to support multiple database backends, with primary support for PostgreSQL and MySQL. The `DatabaseCompat` class provides a centralized capability matrix and compatibility checks to ensure consistent behavior across databases.

## Supported Databases

### PostgreSQL (Recommended)

**Recommended for**: Most deployments, especially those requiring advanced features

**Key Features**:

- Native JSONB type with operators and functions
- Full text search via TSVECTOR
- ARRAY type support for complex data structures
- Transactional DDL (schema changes inside transactions)
- Rich type casting and conversion functions
- SERIAL/SEQUENCES for auto-increment
- JSON operators: `->`, `->>`, `@>`, `?`, `||`
- Collation behavior depends on database/cluster locale (e.g., `en_US.UTF-8`); see [PostgreSQL Collation Support](https://www.postgresql.org/docs/current/collation.html) for details on case-sensitivity configuration

**Configuration**:

```bash
# docker/.env
DATABASE_TYPE=postgres
POSTGRES_HOST=postgres
POSTGRES_PORT=5432
POSTGRES_USER=ragflow
POSTGRES_PASSWORD=password
POSTGRES_DB=ragflow
```

**Advantages**:

- Better JSON performance (JSONB is binary)
- Unlimited VARCHAR length
- Transaction rollback for schema changes
- Superior text search capabilities

**Considerations**:

- Slightly higher memory usage per connection
- Enum types require explicit `CREATE TYPE` definitions
- ARRAY type requires custom handling in Peewee models

### MySQL (Alternative)

**Recommended for**: Deployments with existing MySQL infrastructure

**Key Features**:

- AUTO_INCREMENT for automatic IDs
- FULLTEXT indexes for text search
- JSON type with functions (JSON_EXTRACT, JSON_SET, etc.)
- Native ENUM type
- Compatible with MariaDB
- VARCHAR row size limit (MySQL's 65,535-byte limit applies to maximum row size - the sum of all variable-length columns plus row overhead, not to a single VARCHAR column; a single VARCHAR can approach this limit only if it's the sole variable-length column, accounting for character set encoding and row overhead)

**Configuration**:

```bash
# docker/.env
DATABASE_TYPE=mysql
MYSQL_HOST=mysql
MYSQL_PORT=3306
MYSQL_USER=root
MYSQL_PASSWORD=password
MYSQL_DB=ragflow
```

**Advantages**:

- Familiar to many developers
- Good performance for simple workloads
- Compatible with MariaDB
- Lower memory per connection

**Considerations**:

- JSON operations less optimized than JSONB
- No transactional DDL
- Type casting more limited
- VARCHAR has practical length limits

## Database Capabilities Matrix

The `DatabaseCompat.CAPABILITIES` dictionary defines what each database supports:

### Feature Support

| Feature | MySQL | PostgreSQL |
|---------|-------|-----------|
| **Full-text search** | ✅ FULLTEXT | ✅ TSVECTOR |
| **JSON functions** | ✅ JSON_* | ✅ JSONB operators |
| **Auto-increment** | ✅ AUTO_INCREMENT | ⚠️ SERIAL/SEQUENCES |
| **Sequence support** | ❌ | ✅ SEQUENCE objects |
| **Type casting** | ⚠️ Limited | ✅ Full |
| **Array type** | ❌ | ✅ ARRAY[] |
| **JSONB type** | ❌ | ✅ JSONB |
| **Transactional DDL** | ❌ | ✅ Yes |
| **Enum type** | ✅ Native ENUM | ⚠️ Requires CREATE TYPE |

### Type Limits

| Type | MySQL | PostgreSQL |
|------|-------|-----------|
| **MAX_VARCHAR** | 65,535 bytes | ~1 GB (practical limit, subject to TOAST/internal storage) |
| **MAX_TEXT** | 64 KB (TEXT), 16 MB (MEDIUMTEXT), 4 GB (LONGTEXT) | ~1 GB (practical limit) |
| **MAX_JSON** | Limited | ~1 GB (practical limit) |
| **Array elements** | N/A | Unlimited |

**Note**: The 4 GB limit applies only to LONGTEXT. Regular TEXT is limited to 64 KB.

## Field Compatibility

### Checking Database Capabilities

Use `DatabaseCompat` to check if a database supports a capability:

```python
from api.db.migrations import DatabaseCompat
from common import settings

db_type = settings.DATABASE_TYPE.lower()

# Check capability
if DatabaseCompat.is_capable(db_type, "jsonb_support"):
    # Use JSONB-specific features
    field = JSONBField()
else:
    # Fall back to JSON
    field = JSONField()

# Check multiple capabilities
capabilities_needed = ["full_text_search", "array_support"]
is_compatible = all(
    DatabaseCompat.is_capable(db_type, cap) 
    for cap in capabilities_needed
)
```

### Field Type Equivalence

The `DatabaseCompat.TYPE_EQUIVALENTS` dictionary maps field types between databases using a nested structure:

#### Type Equivalents Structure

```python
DatabaseCompat.TYPE_EQUIVALENTS = {
    "mysql_to_postgres": {
        "LONGTEXT": "TEXT",
        "MEDIUMTEXT": "TEXT",
        "TINYTEXT": "TEXT",
        "VARCHAR": "VARCHAR",
        "INT": "INTEGER",
        "BIGINT": "BIGINT",
        "FLOAT": "REAL",
        "DOUBLE": "DOUBLE PRECISION",
        "DATETIME": "TIMESTAMP",
        "TIMESTAMP": "TIMESTAMP WITH TIME ZONE",
        "JSON": "JSONB",  # Upgraded to JSONB
        "ENUM": "VARCHAR",  # PostgreSQL ENUM needs CREATE TYPE
    },
    "postgres_to_mysql": {
        "TEXT": "LONGTEXT",
        "VARCHAR": "VARCHAR",
        "INTEGER": "INT",
        "BIGINT": "BIGINT",
        "REAL": "FLOAT",
        "DOUBLE PRECISION": "DOUBLE",
        "TIMESTAMP": "DATETIME",
        "TIMESTAMP WITH TIME ZONE": "TIMESTAMP",
        "JSONB": "JSON",
        "ARRAY": None,  # No MySQL equivalent
    }
}


```python
from api.db.migrations import DatabaseCompat

# If migrating from PostgreSQL to MySQL
mysql_type = DatabaseCompat.get_equivalent_type(
    "TEXT", 
    source_db="postgres", 
    target_db="mysql"
)
# Returns: "LONGTEXT"

# If migrating from MySQL to PostgreSQL
postgres_type = DatabaseCompat.get_equivalent_type(
    "JSON",
    source_db="mysql",
    target_db="postgres"
)
# Returns: "JSONB"
```

## Built-in Field Types and Compatibility

RAGFlow provides custom field types that work across databases:

### LongTextField

Text field with compatibility handling for both databases.

```python
from api.db.fields import LongTextField

class Document(DataBaseModel):
    content = LongTextField()  # LONGTEXT (MySQL) / TEXT (PostgreSQL)
```

**Compatibility**:

- MySQL: `LONGTEXT` (4 GB max)
- PostgreSQL: `TEXT` (unlimited)

### JSONField

JSON field with compatibility handling.

```python
from api.db.fields import JSONField

class Config(DataBaseModel):
    settings = JSONField()  # JSON (MySQL) / JSONB (PostgreSQL)
    
    # Usage
    config = Config.get()
    color = config.settings.get("color")  # Automatic JSON parsing
```

**Compatibility**:

- MySQL: `JSON` text-based
- PostgreSQL: `JSONB` binary-based (preferred)

**Performance Note**: PostgreSQL's JSONB is significantly faster for large JSON objects and complex queries.

### DateTimeTzField

DateTime field with timezone support.

```python
from api.db.fields import DateTimeTzField
from datetime import datetime
import pytz

class Event(DataBaseModel):
    occurred_at = DateTimeTzField()
    
    # Usage with timezone
    event = Event.create(
        occurred_at=datetime.now(pytz.UTC)
    )
```

**Compatibility**:

- MySQL: `TIMESTAMP` (stored in UTC, converted to/from the connection/session `time_zone` on read/write — behavior differs from PostgreSQL's `TIMESTAMP WITH TIME ZONE` which stores the timezone explicitly)
- PostgreSQL: `TIMESTAMP WITH TIME ZONE` (full TZ support with explicit timezone storage)

### SerializedField

Field with automatic serialization/deserialization.

```python
from api.db.fields import SerializedField

class UserPreferences(DataBaseModel):
    filters = SerializedField()  # Serialized to TEXT
    
    # Usage
    prefs = UserPreferences.get()
    active_filters = prefs.filters  # Automatically deserialized
```

**Compatibility**: Works with both databases via custom encoding.

## Field Warnings

Certain fields have known compatibility considerations:

```python
from api.db.migrations import DatabaseCompat

# Check for warnings
warnings = DatabaseCompat.FIELD_WARNINGS.get("postgres", {})
for field_type, warning in warnings.items():
    if warning:
        print(f"{field_type}: {warning}")
```

### MySQL-Specific Warnings

| Field | Warning |
|-------|---------|
| JSONField | JSON is text-based; JSONB-like operations may be slower |
| DateTimeTzField | Timezone handling may differ from PostgreSQL |
| LongTextField | (No warning - fully compatible) |
| SerializedField | (No warning - fully compatible) |

### PostgreSQL-Specific Warnings

| Field | Warning |
|-------|---------|
| JSONField | Consider using JSONB for better performance |
| LongTextField | (No warning - fully compatible) |
| DateTimeTzField | (No warning - fully compatible) |
| SerializedField | (No warning - fully compatible) |

## Validating Field Compatibility

Check if a field is compatible with the current database:

```python
from api.db.migrations import DatabaseCompat
from api.db.fields import JSONField
from common import settings

field = JSONField()
db_type = settings.DATABASE_TYPE.lower()

if DatabaseCompat.validate_field_for_db(field, db_type):
    print(f"Field is compatible with {db_type}")
else:
    print(f"Field may have issues with {db_type}")
```

### Automatic Validation on Startup

Models automatically validate their fields when the database initialization runs (after all models are imported and defined). This happens during the `init_database_tables()` call in the startup sequence:

```python
class MyModel(DataBaseModel):
    json_data = JSONField()
    
    # Validation runs after model definition during init_database_tables()
    @classmethod
    def validate_fields(cls, db_type: Optional[str] = None):
        """Validate all fields are compatible with current DB
        
        Called automatically during database initialization.
        Logs warnings for incompatible fields but allows startup to continue.
        """
        if db_type is None:
            db_type = settings.DATABASE_TYPE.lower()
        
        for field_name, field in cls._meta.fields.items():
            if not DatabaseCompat.validate_field_for_db(field, db_type):
                logging.warning(
                    f"{cls.__name__}.{field_name} may not be fully "
                    f"compatible with {db_type}"
                )

# Validation timing:
# 1. Models are imported (api/db/models/__init__.py)
# 2. init_database_tables() is called during app startup
# 3. validate_fields() runs for each model
# 4. Warnings logged but app continues (validation is non-blocking)
```

## Querying Across Databases

### Text Search (Full-Text)

PostgreSQL (using TSVECTOR):

```python
from documents import Document

# PostgreSQL: Native TSVECTOR support
results = Document.select().where(
    Document.content.match("search term")
)
```

MySQL (using FULLTEXT):

```python
# MySQL: FULLTEXT index
results = Document.select().where(
    Document.content.match("search term")
)
```

### JSON Operations

PostgreSQL (using JSONB):

```python
# PostgreSQL: Optimized JSONB operators
from documents import Document

# Check if key exists
docs = Document.select().where(
    Document.metadata.has_key("author")
)

# Access nested value
docs = Document.select().where(
    Document.metadata["tags"].contains("important")
)
```

MySQL (using JSON functions):

```python
# MySQL: JSON function equivalents
from peewee import fn

# Check if key exists
docs = Document.select().where(
    fn.JSON_CONTAINS_PATH(
        Document.metadata, 'one', '$.author'
    )
)
```

## Migrating Between Databases

### PostgreSQL to MySQL Migration

1. **Field type mapping**:

   ```python
   # Map all field types using equivalents
   from api.db.migrations import DatabaseCompat
   
   source_db = "postgres"
   target_db = "mysql"
   
   type_map = {}
   for pg_type, mysql_type in DatabaseCompat.TYPE_EQUIVALENTS["postgres_to_mysql"].items():
       type_map[pg_type] = mysql_type
   ```

2. **Data export**:

   ```bash
   # Export from PostgreSQL
   pg_dump ragflow > dump.sql
   
   # Review and adjust schema for MySQL compatibility
   # - Change JSONB to JSON
   # - Change TIMESTAMP WITH TIME ZONE to TIMESTAMP
   # - Remove ARRAY types
   ```

3. **Data import**:

   ```bash
   # Import to MySQL
   mysql ragflow < dump.sql
   ```

4. **Testing**:

   ```bash
   # Test with new database
   export DATABASE_TYPE=mysql
   python -m pytest test/ -v
   ```

### MySQL to PostgreSQL Migration

1. **Field type mapping** (use TYPE_EQUIVALENTS):

   ```python
   from api.db.migrations import DatabaseCompat
   
   source_db = "mysql"
   target_db = "postgres"
   
   # Get equivalents for all MySQL types
   type_map = DatabaseCompat.TYPE_EQUIVALENTS["mysql_to_postgres"]
   ```

2. **Data export**:

   ```bash
   # Export from MySQL
   mysqldump ragflow > dump.sql
   
   # Review and adjust schema for PostgreSQL compatibility:
   # - MySQL ENUM types need to be converted to VARCHAR or CREATE TYPE statements
   # - MySQL AUTO_INCREMENT needs to become SERIAL or SEQUENCE
   # - Ensure JSON fields will become JSONB
   ```

3. **Schema migration**:

   ```bash
   # Edit dump.sql to convert MySQL syntax to PostgreSQL:
   # - Remove AUTO_INCREMENT, add SERIAL
   # - Convert JSON to JSONB
   # - Convert ENUM to VARCHAR or custom types
   # - Remove backticks, use double quotes for identifiers
   ```

4. **Data import**:

   ```bash
   # Create database first
   createdb ragflow
   
   # Import data
   psql ragflow < dump.sql
   ```

5. **Post-migration validation**:

   ```bash
   # Verify field compatibility with PostgreSQL
   export DATABASE_TYPE=postgres
   POSTGRES_HOST=localhost POSTGRES_PORT=5432 \
   POSTGRES_USER=ragflow POSTGRES_PASSWORD=password \
   POSTGRES_DB=ragflow \
   python -c "from api.db.models import *; from api.db.migrations import init_database_tables; init_database_tables()"
   ```

6. **Testing**:

   ```bash
   # Comprehensive test with new database
   export DATABASE_TYPE=postgres
   python -m pytest test/ -v
   ```

7. **Upgrade considerations**:

   - JSON fields will function as JSONB (better performance)
   - TIMESTAMP fields gain timezone support (TIMESTAMP WITH TIME ZONE)
   - ARRAY types now available (upgrade text-array fields if needed)
   - ENUM types can now use native PostgreSQL ENUM (optional optimization)

## Database-Specific Best Practices

### PostgreSQL Best Practices

✅ **Do**:

- Use JSONB for all JSON data (faster than JSON)
- Leverage ARRAY types for list fields
- Use TSVECTOR for full-text search
- Rely on transactional DDL in migrations
- Use TIMESTAMP WITH TIME ZONE for timezone-aware dates

❌ **Don't**:

- Use JSON instead of JSONB
- Store arrays as comma-separated text
- Use VARCHAR with very large limits (TEXT is fine)
- Rely on exception handlers alone for DDL errors (always test schema migrations thoroughly)

### MySQL Best Practices

✅ **Do**:

- Use JSON type for JSON data (good performance)
- Use FULLTEXT indexes for text search
- Use ENUM for fixed value lists
- Prefer INT over BIGINT when possible
- Use VARCHAR with reasonable limits

❌ **Don't**:

- Store JSON in TEXT fields
- Use VARCHAR > 1000 without good reason
- Assume type conversion works automatically
- Forget MySQL DDL is not transactional
- MySQL has no native array type — avoid storing lists as comma-separated strings; use JSON columns to represent arrays/structured list data

## Configuration

### Switching Databases

To switch between PostgreSQL and MySQL:

```bash
# 1. Set environment variable
export DATABASE_TYPE=mysql  # or postgres

# 2. Configure connection details in docker/.env
MYSQL_HOST=localhost
MYSQL_PORT=3306
MYSQL_USER=root

# 3. Restart RAGFlow
bash docker/launch_backend_service.sh
```

### Database-Specific Environment Variables

**PostgreSQL** (`docker/.env`):

```bash
DATABASE_TYPE=postgres
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_USER=ragflow
POSTGRES_PASSWORD=password
POSTGRES_DB=ragflow
```

**MySQL** (`docker/.env`):

```bash
DATABASE_TYPE=mysql
MYSQL_HOST=localhost
MYSQL_PORT=3306
MYSQL_USER=root
MYSQL_PASSWORD=password
MYSQL_DB=ragflow
```

## Testing with Multiple Databases

### Running Tests Against Both Databases

```bash
# Test with PostgreSQL
export DATABASE_TYPE=postgres
uv run pytest test/ -v

# Test with MySQL
export DATABASE_TYPE=mysql
uv run pytest test/ -v
```

### Database Compatibility Tests

Tests specifically for database compatibility:

```bash
# Run compatibility tests
uv run pytest test/test_database_compat.py -v

# Check which capabilities are supported
uv run pytest test/test_database_compat.py::TestCapabilityMatrix -v
```

## Extending to New Databases

To add support for a new database (e.g., SQLite):

### 1. Update DatabaseCompat

```python
class DatabaseCompat:
    CAPABILITIES = {
        # ... existing ...
        "sqlite": {
            "full_text_search": False,
            "json_functions": True,
            "auto_increment": True,
            "sequence_support": False,
            "type_casting": "limited",
            # ... etc
        }
    }
    
    TYPE_EQUIVALENTS = {
        "sqlite_to_postgres": { ... },
        "postgres_to_sqlite": { ... },
    }
```

### 2. Update Connection Factory

```python
# In api/db/connection.py
def get_db_connection():
    db_type = settings.DATABASE_TYPE.upper()
    
    if db_type == "POSTGRES":
        return PooledPostgresqlDatabase(...)
    elif db_type == "MYSQL":
        return PooledMySQLDatabase(...)
    elif db_type == "SQLITE":
        return PooledSqliteDatabase(...)
    else:
        raise ValueError(f"Unknown database type: {db_type}")
```

### 3. Migrate Custom Fields

```python
# api/db/fields.py - Add database-specific handling
class LongTextField(TextField):
    def db_value(self, value):
        if settings.DATABASE_TYPE.upper() == "SQLITE":
            # Custom handling for SQLite
            pass
```

### 4. Test Thoroughly

```bash
# Test with new database
export DATABASE_TYPE=sqlite
uv run pytest test/ -v
```

## Related Documentation

- [Database Migrations](DATABASE_MIGRATIONS.md) - Manage schema changes
- [Connection Pool Monitoring](CONNECTION_POOL.md) - Monitor connections
- [AGENTS.md](../AGENTS.md) - Project build and standards
