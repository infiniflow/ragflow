# Database Layer Improvements - Implementation Plan

**Status**: Planning Phase  
**Created**: 2026-01-09  
**Target Branch**: `fix-postgres-connection-pool`

---

## Executive Summary

Implement 5 interconnected database improvements to enhance reliability, observability, and maintainability:

1. **Error Handling Standardization** - Consistent error suppression for expected failures
2. **Connection Pool Diagnostics** - Real-time pool health monitoring
3. **Migration Tracking System** - Prevent duplicate migrations, enable rollback capability
4. **Database Compatibility Layer** - Future-proof multi-database support
5. **Transaction Safety** - Atomic migrations with rollback capability

**Total Estimated Effort**: 12-14 hours  
**Implementation Order**: Sequential phases with incremental testing

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│ api/db/migrations.py (Refactored)                       │
├─────────────────────────────────────────────────────────┤
│ ✓ StandardErrorHandler (new)                            │
│ ✓ MigrationTracker (new)                                │
│ ✓ DatabaseCompat (new)                                  │
│ ✓ Transaction-wrapped migrate_db()                      │
└──────────┬──────────────────────────────────────────────┘
           │
           ├──→ api/db/connection.py (Enhanced)
           │    ├─ PoolDiagnostics (new)
           │    └─ Retry instrumentation (new)
           │
           ├──→ api/db/fields.py (No changes)
           │
           └──→ Database (PostgreSQL/MySQL)
                ├─ migration_history table (new)
                └─ Existing tables (unchanged)
```

---

## Phase 1: Error Handling Standardization

**Duration**: 2-3 hours  
**Files**: `api/db/migrations.py`

### Objectives

1. Extract common error handling logic into reusable class
2. Support PostgreSQL and MySQL error codes
3. Distinguish expected vs. unexpected failures
4. Enable logging at appropriate levels

### Implementation Steps

#### Step 1a: Create StandardErrorHandler class

```python
class StandardErrorHandler:
    """Centralized error handling for database operations"""
    
    # Define error categories for both databases
    ERROR_CODES = {
        "duplicate_column": {
            "mysql": [1060],
            "postgres": ["42701"]
        },
        "incompatible_type": {
            "mysql": [1062, 1064],
            "postgres": ["42804", "42P07"]
        },
        "missing_column": {
            "mysql": [1054],
            "postgres": ["42703"]
        }
    }
    
    ERROR_MESSAGES = {
        "duplicate_column": ["duplicate column", "already exists"],
        "incompatible_type": ["cannot be cast", "incompatible", "type mismatch"],
        "missing_column": ["does not exist", "no such column"]
    }
    
    @staticmethod
    def categorize_error(exception, db_type="postgres"):
        """Categorize exception into expected/unexpected"""
        # Implementation returns: (category, is_expected, should_skip)
        pass
    
    @staticmethod
    def handle_migration_error(exception, table, column, operation, db_type):
        """Handle migration error with appropriate logging level"""
        # Returns: should_continue (bool)
        pass
```

#### Step 1b: Refactor alter_db_add_column

```python
def alter_db_add_column(migrator, table_name, column_name, column_type):
    try:
        from playhouse.migrate import migrate
        migrate(migrator.add_column(table_name, column_name, column_type))
        logging.debug(f"Added column: {table_name}.{column_name}")
    except Exception as ex:
        category, is_expected, should_skip = StandardErrorHandler.categorize_error(
            ex, settings.DATABASE_TYPE.lower()
        )
        
        if should_skip:
            logging.debug(
                f"Skipped (expected): {table_name}.{column_name} - {category}: {ex}"
            )
            return
        
        if is_expected:
            logging.warning(
                f"Migration issue: {table_name}.{column_name} - {category}: {ex}"
            )
            return
        
        logging.critical(
            f"Failed to add {settings.DATABASE_TYPE.upper()}.{table_name} "
            f"column {column_name}: {ex}"
        )
```

#### Step 1c: Apply same pattern to alter_db_column_type and alter_db_rename_column

- Consistent error handling
- Appropriate logging levels
- Clear distinction: expected (warning) vs unexpected (critical)

### Testing

- Unit tests for StandardErrorHandler.categorize_error()
- Test with real PostgreSQL "column already exists" error
- Test with real PostgreSQL "incompatible type" error
- Verify logging levels are appropriate

---

## Phase 2: Connection Pool Diagnostics

**Duration**: 2-3 hours  
**Files**: `api/db/connection.py`

### Objectives

1. Add real-time connection pool metrics
2. Log warnings when pool utilization exceeds thresholds
3. Track connection creation/closure
4. Enable operational visibility

### Implementation Steps

#### Step 2a: Create PoolDiagnostics class

```python
class PoolDiagnostics:
    """Monitor and report connection pool health"""
    
    HEALTH_CHECK_INTERVAL = 60  # seconds
    WARNING_THRESHOLD = 0.8      # 80% utilization
    CRITICAL_THRESHOLD = 0.95    # 95% utilization
    
    @staticmethod
    def get_pool_stats(db):
        """Get current pool statistics"""
        # Returns: {
        #   "max": int,
        #   "active": int,
        #   "idle": int,
        #   "waiting": int,
        #   "utilization_percent": float
        # }
        pass
    
    @staticmethod
    def log_pool_health(db):
        """Log pool health at appropriate level"""
        pass
    
    @staticmethod
    def start_health_monitoring(db, interval=HEALTH_CHECK_INTERVAL):
        """Background thread to monitor pool health"""
        pass
```

#### Step 2b: Add instrumentation to connection retry logic

```python
def get_db_connection():
    attempt = 1
    while attempt <= MAX_RETRIES:
        try:
            # ... existing connection logic ...
            logging.debug(f"Database connection established")
            PoolDiagnostics.start_health_monitoring(db)
            return db
        except Exception as ex:
            backoff = exponential_backoff(attempt)
            logging.warning(
                f"Connection attempt {attempt}/{MAX_RETRIES} failed, "
                f"backing off {backoff}s: {ex}"
            )
            attempt += 1
```

#### Step 2c: Add pool stats to log startup

```python
def log_connection_config(db):
    """Log pool configuration at startup"""
    stats = PoolDiagnostics.get_pool_stats(db)
    logging.info(f"Connection pool initialized: {stats}")
```

### Testing

- Verify pool stats are retrieved correctly
- Test with pool under load (simulate concurrent connections)
- Verify warning/critical thresholds trigger at right utilization levels
- Check background monitoring thread doesn't cause issues

---

## Phase 3: Migration Tracking System

**Duration**: 3-4 hours  
**Files**: `api/db/migrations.py`, database schema

### Objectives

1. Track applied migrations in database
2. Prevent duplicate migration execution
3. Enable selective migration execution
4. Support future rollback capability

### Implementation Steps

#### Step 3a: Create migration_history table

```sql
CREATE TABLE IF NOT EXISTS migration_history (
    id SERIAL PRIMARY KEY,
    migration_name VARCHAR(255) UNIQUE NOT NULL,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(16) DEFAULT 'success',  -- 'success' | 'failed' | 'skipped'
    error_message TEXT,
    duration_ms INTEGER,
    db_type VARCHAR(16)  -- 'mysql' | 'postgres'
);

CREATE INDEX idx_migration_status ON migration_history(migration_name, status);
```

#### Step 3b: Create MigrationTracker class

```python
class MigrationTracker:
    """Track migration execution state"""
    
    @staticmethod
    def init_tracking_table():
        """Create migration_history table if not exists"""
        pass
    
    @staticmethod
    def has_migration_run(migration_name):
        """Check if migration has been successfully applied"""
        pass
    
    @staticmethod
    def record_migration(migration_name, status, error=None, duration_ms=None):
        """Record migration execution"""
        pass
    
    @staticmethod
    def get_migration_history():
        """Return all applied migrations"""
        pass
    
    @staticmethod
    def skip_if_run(migration_name):
        """Decorator to skip migrations already applied"""
        pass
```

#### Step 3c: Integrate tracking into migrate_db()

```python
def migrate_db():
    """Apply migrations with tracking"""
    MigrationTracker.init_tracking_table()
    logging.disable(logging.ERROR)
    migrator = DatabaseMigrator[settings.DATABASE_TYPE.upper()].value(DB)
    
    migrations = [
        ("add_file_source_type", lambda: alter_db_add_column(...)),
        ("add_tenant_rerank_id", lambda: alter_db_add_column(...)),
        # ... more migrations
    ]
    
    for migration_name, migration_fn in migrations:
        if MigrationTracker.has_migration_run(migration_name):
            logging.debug(f"Skipping already-applied: {migration_name}")
            continue
        
        start_time = time.time()
        try:
            migration_fn()
            duration_ms = (time.time() - start_time) * 1000
            MigrationTracker.record_migration(
                migration_name, "success", duration_ms=duration_ms
            )
        except Exception as ex:
            duration_ms = (time.time() - start_time) * 1000
            MigrationTracker.record_migration(
                migration_name, "failed", error=str(ex), duration_ms=duration_ms
            )
            logging.error(f"Migration {migration_name} failed: {ex}")
    
    logging.disable(logging.NOTSET)
```

### Testing

- Verify migration_history table is created
- Run migrations twice, verify second run skips applied migrations
- Check migration_history records are accurate
- Test with failed migration (record error_message)
- Verify duration_ms is reasonable

---

## Phase 4: Database Compatibility Layer

**Duration**: 2-3 hours  
**Files**: `api/db/migrations.py`, `api/db/base.py`

### Objectives

1. Centralize database-specific logic
2. Validate field compatibility
3. Support future database additions
4. Document capabilities per database

### Implementation Steps

#### Step 4a: Create DatabaseCompat class

```python
class DatabaseCompat:
    """Database capability matrix and compatibility checks"""
    
    CAPABILITIES = {
        "mysql": {
            "full_text_search": True,
            "json_functions": True,
            "auto_increment": True,
            "sequence_support": False,
            "type_casting": "limited",
            "max_varchar": 65535,
        },
        "postgres": {
            "full_text_search": True,  # TSVECTOR
            "json_functions": True,    # JSONB
            "auto_increment": False,   # Uses SERIAL/SEQUENCES
            "sequence_support": True,
            "type_casting": "full",
            "max_varchar": None,  # Unlimited
        }
    }
    
    @staticmethod
    def validate_field_for_db(field, db_type):
        """Validate field is compatible with database"""
        pass
    
    @staticmethod
    def get_equivalent_type(field_type, source_db, target_db):
        """Get equivalent field type for target database"""
        pass
    
    @staticmethod
    def is_capable(db_type, capability):
        """Check if database supports capability"""
        pass
```

#### Step 4b: Add field validation to DataBaseModel

```python
class DataBaseModel(Model):
    """Add compatibility checking"""
    
    @classmethod
    def validate_fields(cls):
        """Validate all fields are compatible with current DB"""
        db_type = settings.DATABASE_TYPE.lower()
        for field_name, field in cls._meta.fields.items():
            if not DatabaseCompat.validate_field_for_db(field, db_type):
                logging.warning(
                    f"{cls.__name__}.{field_name} may not be fully compatible "
                    f"with {db_type}"
                )
```

#### Step 4c: Document capabilities in code

```python
# Add to each migration or field definition:
@DatabaseCompat.requires("mysql", "auto_increment")
def add_auto_id_column(migrator):
    """MySQL-specific migration"""
    pass
```

### Testing

- Verify capability matrix matches actual database features
- Test field validation for each database type
- Ensure warnings are logged for incompatible fields
- Document any fields that differ between databases

---

## Phase 5: Transaction Safety

**Duration**: 2 hours  
**Files**: `api/db/migrations.py`, `api/db/connection.py`

### Objectives

1. Make migrations atomic (all-or-nothing)
2. Enable automatic rollback on failure
3. Prevent partial migrations
4. Log transaction state

### Implementation Steps

#### Step 5a: Wrap migrate_db in transaction

```python
def migrate_db():
    """Apply all migrations atomically"""
    MigrationTracker.init_tracking_table()
    logging.disable(logging.ERROR)
    migrator = DatabaseMigrator[settings.DATABASE_TYPE.upper()].value(DB)
    
    try:
        with DB.atomic():
            # All migrations here
            for migration_name, migration_fn in migrations:
                if MigrationTracker.has_migration_run(migration_name):
                    continue
                migration_fn()
                MigrationTracker.record_migration(migration_name, "success")
    except Exception as ex:
        logging.error(f"Migration block rolled back due to: {ex}")
        # All recorded migrations in this block are rolled back
        raise
    finally:
        logging.disable(logging.NOTSET)
```

#### Step 5b: Add connection transaction logging

```python
class TransactionLogger:
    """Log transaction state for debugging"""
    
    @staticmethod
    def log_transaction_state(db, operation="begin"):
        """Log transaction state changes"""
        if operation == "begin":
            logging.debug(f"Transaction started, isolation level: {db.isolation_level}")
        elif operation == "commit":
            logging.debug("Transaction committed")
        elif operation == "rollback":
            logging.debug("Transaction rolled back")
```

### Testing

- Create failing migration, verify all changes rolled back
- Test with multiple migrations, verify atomicity
- Check transaction isolation level is appropriate
- Verify rollback logs are clear

---

## Phase 6: Integration Testing

**Duration**: 2-3 hours  
**Test Files**: `test/test_db_migrations.py` (new)

### Test Categories

#### 6a: Error Handling Tests

```python
class TestErrorHandling(TestCase):
    def test_duplicate_column_mysql(self):
        """Verify duplicate column error is handled correctly"""
        pass
    
    def test_duplicate_column_postgres(self):
        """Verify PostgreSQL 42701 error is handled correctly"""
        pass
    
    def test_incompatible_type_cast(self):
        """Verify type incompatibility is logged as warning"""
        pass
    
    def test_unexpected_error_is_critical(self):
        """Verify non-expected errors are logged as critical"""
        pass
```

#### 6b: Pool Diagnostics Tests

```python
class TestPoolDiagnostics(TestCase):
    def test_pool_stats_retrieved(self):
        """Verify pool statistics are accurate"""
        pass
    
    def test_warning_on_high_utilization(self):
        """Verify warning logged at 80% utilization"""
        pass
    
    def test_critical_on_very_high_utilization(self):
        """Verify critical logged at 95% utilization"""
        pass
```

#### 6c: Migration Tracking Tests

```python
class TestMigrationTracking(TestCase):
    def test_migration_history_table_created(self):
        """Verify migration_history table exists"""
        pass
    
    def test_migration_not_run_twice(self):
        """Verify second run skips already-applied migrations"""
        pass
    
    def test_failed_migration_recorded(self):
        """Verify failed migration status is recorded"""
        pass
    
    def test_migration_duration_tracked(self):
        """Verify duration_ms is recorded"""
        pass
```

#### 6d: Transaction Safety Tests

```python
class TestTransactionSafety(TestCase):
    def test_migration_rollback_on_error(self):
        """Verify partial migrations are rolled back"""
        pass
    
    def test_atomic_all_or_nothing(self):
        """Verify all migrations succeed or all roll back"""
        pass
```

#### 6e: Multi-Database Tests

```python
class TestDatabaseCompat(TestCase):
    def test_capability_matrix_mysql(self):
        """Verify MySQL capabilities are correct"""
        pass
    
    def test_capability_matrix_postgres(self):
        """Verify PostgreSQL capabilities are correct"""
        pass
```

---

## Phase 7: Documentation

**Duration**: 1 hour  
**Files**: New docs + code comments

### Documentation Items

1. **Database Migration Guide** (`docs/DATABASE_MIGRATIONS.md`)
   - How migrations work
   - How to add new migrations
   - How to check migration status
   - How to troubleshoot migration issues

2. **Connection Pool Monitoring** (`docs/CONNECTION_POOL.md`)
   - How to monitor pool health
   - Understanding utilization metrics
   - Configuring thresholds
   - Troubleshooting connection exhaustion

3. **Multi-Database Support** (`docs/DATABASE_COMPATIBILITY.md`)
   - Supported databases (MySQL, PostgreSQL)
   - Database-specific features
   - Field compatibility matrix
   - Future database additions

4. **Code Comments**
   - Docstrings for StandardErrorHandler
   - Docstrings for PoolDiagnostics
   - Docstrings for MigrationTracker
   - Docstrings for DatabaseCompat

---

## Implementation Schedule

### Week 1 (Estimated)

- **Day 1**: Phase 1 (Error Handling) - Complete & test
- **Day 2**: Phase 2 (Pool Diagnostics) - Complete & test
- **Day 3**: Phase 3 (Migration Tracking) - Complete & test
- **Day 4**: Phase 4 (Database Compat) - Complete & test
- **Day 5**: Phase 5 (Transactions) - Complete & test

### Week 2 (Estimated)

- **Day 1**: Phase 6 (Integration Testing) - Complete all test categories
- **Day 2**: Phase 7 (Documentation) - Complete all docs
- **Day 3**: Code review & refinement
- **Day 4**: Final testing & integration
- **Day 5**: Merge to main, release notes

---

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|-----------|
| Migration tracking DB unavailable | Low | Medium | Graceful degradation if tracking table fails |
| Connection pool diagnostics overhead | Low | Low | Optional background thread |
| Transaction rollback on PostgreSQL | Low | High | Comprehensive testing of atomic blocks |
| Backward compatibility break | Very Low | High | Maintain existing APIs, extend only |
| Test coverage gaps | Medium | Medium | Comprehensive test suite in Phase 6 |

---

## Success Criteria

✅ All error handling is consistent and follows logging levels  
✅ Connection pool health is visible via logs  
✅ Migrations never run twice  
✅ Failed migrations are tracked and can be investigated  
✅ Database compatibility is documented and validated  
✅ All migrations are atomic (all-or-nothing)  
✅ 100% of new code is tested  
✅ Documentation is comprehensive and up-to-date  
✅ No breaking changes to existing APIs  
✅ Performance impact is negligible (<5% overhead)

---

## Dependencies & Blockers

**None identified** - All improvements are additive and don't require external dependencies.

---

## Rollback Plan

If critical issues emerge during any phase:

1. Revert to `fix-postgres-connection-pool` branch
2. Disable tracking table creation if it causes issues
3. Disable pool diagnostics background thread if it impacts performance
4. Phase-by-phase rollback is possible due to modular design

---

## Next Steps

1. ✅ Review this plan
2. → Start Phase 1 implementation
3. → Integrate phases incrementally
4. → Test thoroughly after each phase
5. → Merge to main with comprehensive PR
