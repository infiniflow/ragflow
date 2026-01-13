# Database Layer (api/db/)

This directory contains the database models and connection management for RAGFlow.

## Structure

The database layer has been modularized for better maintainability:

### Core Modules

- **`fields.py`** - Custom Peewee field types
  - `JSONField`, `ListField` - JSON serialization fields
  - `SerializedField`, `JsonSerializedField` - Pickle/JSON serialization
  - `DateTimeTzField` - Timezone-aware datetime field
  - `LongTextField` - Database-specific long text field
  - Helper functions for field manipulation

- **`connection.py`** - Database connection and pooling
  - `RetryingPooledMySQLDatabase` - MySQL connection with retry logic
  - `RetryingPooledPostgresqlDatabase` - PostgreSQL connection with retry logic
  - `DB` - Singleton database connection
  - `DatabaseLock` - Advisory lock enumeration
  - Connection pool management with proper stale connection handling

- **`base.py`** - Base model classes
  - `BaseModel` - Base Peewee model with common functionality
  - `DataBaseModel` - Base model with metadata and timestamps

- **`migrations.py`** - Database migration utilities
  - `alter_db_add_column()` - Add columns to existing tables
  - `alter_db_column_type()` - Modify column types
  - `alter_db_rename_column()` - Rename columns
  - `migrate_db()` - Apply database migrations
  - `init_database_tables()` - Initialize all tables

### Model Modules (`models/`)

Domain-specific model definitions organized by feature:

- **`auth.py`** - User authentication and tenant management
  - `User`, `Tenant`, `UserTenant`, `InvitationCode`, `APIToken`

- **`llm.py`** - LLM and model configurations
  - `LLM`, `LLMFactories`

- **`knowledge.py`** - Knowledge base and document management
  - `Knowledgebase`, `Document`, `File`, `File2Document`, `Task`

- **`dialog.py`** - Chat and conversation management
  - `Dialog`, `Conversation`, `API4Conversation`

- **`canvas.py`** - Canvas templates
  - `CanvasTemplate`

- **`integration.py`** - External service integrations
  - `Connector`, `Connector2Kb`, `MCPServer`

- **`evaluation.py`** - Evaluation datasets and results
  - `EvaluationDataset`, `EvaluationCase`, `EvaluationRun`, `EvaluationResult`

- **`memory.py`** - Memory management
  - `Memory`

- **`system.py`** - System configuration
  - `SystemSettings`

## Backward Compatibility

The original `db_models.py` file has been converted to a compatibility shim that re-exports all symbols from the new modular structure. All existing imports continue to work:

```python
# Legacy import - still works
from api.db.db_models import User, Tenant, DB, JSONField

# New modular imports - also work
from api.db.models import User, Tenant
from api.db.connection import DB
from api.db.fields import JSONField
```

## Connection Pool Fix

The connection handling includes a fix for the PostgreSQL "connection already closed" issue. When connection errors occur, the connection pool is properly cleared before attempting to reconnect, preventing reuse of stale connections.

See `test/unit_test/api_db/test_connection_pool_fix.py` for test coverage.

## Testing

Unit tests are located in `test/unit_test/api_db/`:

- `test_db_imports.py` - Backward compatibility verification
- `test_fields.py` - Custom field type tests
- `test_connection.py` - Connection class tests
- `test_connection_pool_fix.py` - Connection pool handling tests

Run tests:

```bash
pytest test/unit_test/api_db/ -v
```
