# OceanBase Primary Database Support - Technical Specification

## 1. Overview

This document provides a detailed technical plan for adding OceanBase support as the primary database in RAGFlow, leveraging OceanBase's MySQL compatibility mode. The feature will be enabled via the `DB_TYPE=oceanbase` environment variable.

## 2. Current Architecture Analysis

### 2.1 Database Connection Layer

**Location**: [api/db/db_models.py:242-413](api/db/db_models.py#L242-L413)

The current implementation uses:
- **`RetryingPooledMySQLDatabase`**: MySQL connection pool with retry logic
- **`RetryingPooledPostgresqlDatabase`**: PostgreSQL connection pool with retry logic
- **`PooledDatabase` enum**: Maps database types to connection classes
- **`DatabaseMigrator` enum**: Maps database types to migration handlers
- **`DatabaseLock` enum**: Maps database types to distributed lock implementations

### 2.2 Configuration Flow

**Location**: [common/settings.py:68-69](common/settings.py#L68-L69)

```python
DATABASE_TYPE = os.getenv("DB_TYPE", "mysql")
DATABASE = decrypt_database_config(name=DATABASE_TYPE)
```

### 2.3 Existing OceanBase Configuration

**Location**: [docker/service_conf.yaml.template:34-41](docker/service_conf.yaml.template#L34-L41)

OceanBase is already configured for DOC_ENGINE use, but not as primary database.

---

## 3. Implementation Plan

### 3.1 Create `RetryingPooledOceanBaseDatabase` Class

**Location**: [api/db/db_models.py](api/db/db_models.py) (after line 313, before `PooledDatabase` enum)

**Rationale**: OceanBase uses MySQL protocol in compatibility mode, so we inherit from `PooledMySQLDatabase`. However, we create a separate class to:
1. Allow OceanBase-specific error handling
2. Support OceanBase-specific connection parameters
3. Enable future OceanBase-specific optimizations

**Implementation Details**:

```python
class RetryingPooledOceanBaseDatabase(PooledMySQLDatabase):
    """
    OceanBase database connection pool using MySQL compatibility mode.

    OceanBase supports MySQL protocol, so we inherit from PooledMySQLDatabase.
    This class allows for OceanBase-specific configurations and error handling.
    """
    def __init__(self, *args, **kwargs):
        self.max_retries = kwargs.pop("max_retries", 5)
        self.retry_delay = kwargs.pop("retry_delay", 1)
        # OceanBase-specific: set sql_mode for better compatibility
        kwargs.setdefault('sql_mode', 'STRICT_TRANS_TABLES')
        super().__init__(*args, **kwargs)

    def execute_sql(self, sql, params=None, commit=True):
        for attempt in range(self.max_retries + 1):
            try:
                return super().execute_sql(sql, params, commit)
            except (OperationalError, InterfaceError) as e:
                # MySQL error codes that also apply to OceanBase
                error_codes = [2013, 2006]  # Lost connection, Server gone away
                # OceanBase-specific error codes can be added here
                error_messages = ['', 'Lost connection', 'OceanBase']
                should_retry = (
                    (hasattr(e, 'args') and e.args and e.args[0] in error_codes) or
                    (str(e) in error_messages) or
                    (hasattr(e, '__class__') and e.__class__.__name__ == 'InterfaceError')
                )

                if should_retry and attempt < self.max_retries:
                    logging.warning(
                        f"OceanBase connection issue (attempt {attempt+1}/{self.max_retries}): {e}"
                    )
                    self._handle_connection_loss()
                    time.sleep(self.retry_delay * (2 ** attempt))
                else:
                    logging.error(f"OceanBase execution failure: {e}")
                    raise
        return None

    def _handle_connection_loss(self):
        try:
            self.close()
        except Exception:
            pass
        try:
            self.connect()
        except Exception as e:
            logging.error(f"Failed to reconnect to OceanBase: {e}")
            time.sleep(0.1)
            try:
                self.connect()
            except Exception as e2:
                logging.error(f"Failed to reconnect to OceanBase on second attempt: {e2}")
                raise

    def begin(self):
        for attempt in range(self.max_retries + 1):
            try:
                return super().begin()
            except (OperationalError, InterfaceError) as e:
                error_codes = [2013, 2006]
                error_messages = ['', 'Lost connection']

                should_retry = (
                    (hasattr(e, 'args') and e.args and e.args[0] in error_codes) or
                    (str(e) in error_messages) or
                    (hasattr(e, '__class__') and e.__class__.__name__ == 'InterfaceError')
                )

                if should_retry and attempt < self.max_retries:
                    logging.warning(
                        f"OceanBase connection lost during transaction (attempt {attempt+1}/{self.max_retries})"
                    )
                    self._handle_connection_loss()
                    time.sleep(self.retry_delay * (2 ** attempt))
                else:
                    raise
        return None
```

**Lines of code**: ~50 lines

---

### 3.2 Update `PooledDatabase` Enum

**Location**: [api/db/db_models.py:386-388](api/db/db_models.py#L386-L388)

**Change**:
```python
class PooledDatabase(Enum):
    MYSQL = RetryingPooledMySQLDatabase
    POSTGRES = RetryingPooledPostgresqlDatabase
    OCEANBASE = RetryingPooledOceanBaseDatabase  # Add this line
```

---

### 3.3 Update `DatabaseMigrator` Enum

**Location**: [api/db/db_models.py:391-393](api/db/db_models.py#L391-L393)

**Rationale**: OceanBase uses MySQL-compatible DDL, so we reuse `MySQLMigrator`.

**Change**:
```python
class DatabaseMigrator(Enum):
    MYSQL = MySQLMigrator
    POSTGRES = PostgresqlMigrator
    OCEANBASE = MySQLMigrator  # OceanBase uses MySQL-compatible DDL
```

---

### 3.4 Update `TextFieldType` Enum

**Location**: [api/db/db_models.py:49-51](api/db/db_models.py#L49-L51)

**Rationale**: OceanBase supports `LONGTEXT` like MySQL.

**Change**:
```python
class TextFieldType(Enum):
    MYSQL = "LONGTEXT"
    POSTGRES = "TEXT"
    OCEANBASE = "LONGTEXT"  # OceanBase supports LONGTEXT
```

---

### 3.5 Create `OceanBaseDatabaseLock` Class

**Location**: [api/db/db_models.py](api/db/db_models.py) (after `MysqlDatabaseLock` class, around line 546)

**Rationale**: OceanBase supports MySQL's `GET_LOCK`/`RELEASE_LOCK` functions, so we can reuse the MySQL lock implementation.

**Option A - Reuse MySQL Lock** (Recommended for simplicity):
```python
class OceanBaseDatabaseLock(MysqlDatabaseLock):
    """
    OceanBase distributed lock using MySQL-compatible GET_LOCK/RELEASE_LOCK.
    OceanBase supports MySQL's locking functions in compatibility mode.
    """
    pass
```

**Option B - Full Implementation** (If OceanBase-specific handling needed):
```python
class OceanBaseDatabaseLock:
    def __init__(self, lock_name, timeout=10, db=None):
        self.lock_name = lock_name
        self.timeout = int(timeout)
        self.db = db if db else DB

    @with_retry(max_retries=3, retry_delay=1.0)
    def lock(self):
        cursor = self.db.execute_sql("SELECT GET_LOCK(%s, %s)", (self.lock_name, self.timeout))
        ret = cursor.fetchone()
        if ret[0] == 0:
            raise Exception(f"acquire oceanbase lock {self.lock_name} timeout")
        elif ret[0] == 1:
            return True
        else:
            raise Exception(f"failed to acquire lock {self.lock_name}")

    @with_retry(max_retries=3, retry_delay=1.0)
    def unlock(self):
        cursor = self.db.execute_sql("SELECT RELEASE_LOCK(%s)", (self.lock_name,))
        ret = cursor.fetchone()
        if ret[0] == 0:
            raise Exception(f"oceanbase lock {self.lock_name} was not established by this thread")
        elif ret[0] == 1:
            return True
        else:
            raise Exception(f"oceanbase lock {self.lock_name} does not exist")

    def __enter__(self):
        if isinstance(self.db, (PooledMySQLDatabase, RetryingPooledOceanBaseDatabase)):
            self.lock()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        if isinstance(self.db, (PooledMySQLDatabase, RetryingPooledOceanBaseDatabase)):
            self.unlock()

    def __call__(self, func):
        @wraps(func)
        def magic(*args, **kwargs):
            with self:
                return func(*args, **kwargs)
        return magic
```

---

### 3.6 Update `DatabaseLock` Enum

**Location**: [api/db/db_models.py:549-551](api/db/db_models.py#L549-L551)

**Change**:
```python
class DatabaseLock(Enum):
    MYSQL = MysqlDatabaseLock
    POSTGRES = PostgresDatabaseLock
    OCEANBASE = OceanBaseDatabaseLock  # Add this line
```

---

## 4. Configuration Updates

### 4.1 Update `service_conf.yaml.template`

**Location**: [docker/service_conf.yaml.template](docker/service_conf.yaml.template)

**Add new section for OceanBase as primary database**:

```yaml
# OceanBase as primary database (when DB_TYPE=oceanbase)
# Note: This is different from the 'oceanbase' section used for DOC_ENGINE
oceanbase_db:
  name: '${OCEANBASE_DBNAME:-rag_flow}'
  user: '${OCEANBASE_DB_USER:-root@ragflow}'
  password: '${OCEANBASE_DB_PASSWORD:-infini_rag_flow}'
  host: '${OCEANBASE_DB_HOST:-oceanbase}'
  port: ${OCEANBASE_DB_PORT:-2881}
  max_connections: 900
  stale_timeout: 300
```

**Alternative approach**: Reuse existing `oceanbase` section by adding database name field:
```yaml
oceanbase:
  scheme: 'oceanbase'
  config:
    name: '${OCEANBASE_DBNAME:-rag_flow}'        # Add for primary DB
    db_name: '${OCEANBASE_DOC_DBNAME:-test}'     # Existing for DOC_ENGINE
    user: '${OCEANBASE_USER:-root@ragflow}'
    password: '${OCEANBASE_PASSWORD:-infini_rag_flow}'
    host: '${OCEANBASE_HOST:-oceanbase}'
    port: ${OCEANBASE_PORT:-2881}
    max_connections: 900                          # Add for primary DB
    stale_timeout: 300                            # Add for primary DB
```

### 4.2 Update `docker/.env`

**Location**: [docker/.env](docker/.env)

**Add new environment variable**:
```bash
# Primary database type
# Available options:
# - `mysql` (default)
# - `postgres`
# - `oceanbase`
DB_TYPE=${DB_TYPE:-mysql}

# OceanBase primary database configuration (when DB_TYPE=oceanbase)
# Note: These may overlap with DOC_ENGINE OceanBase settings
OCEANBASE_DBNAME=rag_flow
# OCEANBASE_DB_USER, OCEANBASE_DB_PASSWORD, OCEANBASE_DB_HOST, OCEANBASE_DB_PORT
# can reuse existing OCEANBASE_* variables
```

---

## 5. File Changes Summary

| File | Changes | Lines |
|------|---------|-------|
| [api/db/db_models.py](api/db/db_models.py) | Add `RetryingPooledOceanBaseDatabase` class | ~50 |
| [api/db/db_models.py](api/db/db_models.py) | Update `PooledDatabase` enum | +1 |
| [api/db/db_models.py](api/db/db_models.py) | Update `DatabaseMigrator` enum | +1 |
| [api/db/db_models.py](api/db/db_models.py) | Update `TextFieldType` enum | +1 |
| [api/db/db_models.py](api/db/db_models.py) | Add `OceanBaseDatabaseLock` class | ~5-40 |
| [api/db/db_models.py](api/db/db_models.py) | Update `DatabaseLock` enum | +1 |
| [docker/service_conf.yaml.template](docker/service_conf.yaml.template) | Add OceanBase primary DB config | ~10 |
| [docker/.env](docker/.env) | Add `DB_TYPE` documentation | ~5 |
| [common/config_utils.py](common/config_utils.py) | Handle nested config (optional) | ~10 |

**Total estimated code**: ~50-80 lines (core implementation)

---

## 6. CI Test Plan

### 6.1 Test File Location

Create new test file: `test/test_oceanbase_db.py`

### 6.2 Unit Test Cases

```python
# test/test_oceanbase_db.py

import pytest
from unittest.mock import patch, MagicMock

class TestOceanBaseDatabase:
    """Test OceanBase as primary database"""

    def test_oceanbase_connection_class_exists(self):
        """Verify RetryingPooledOceanBaseDatabase class is defined"""
        from api.db.db_models import RetryingPooledOceanBaseDatabase
        assert RetryingPooledOceanBaseDatabase is not None

    def test_pooled_database_enum_has_oceanbase(self):
        """Verify PooledDatabase enum includes OCEANBASE"""
        from api.db.db_models import PooledDatabase
        assert hasattr(PooledDatabase, 'OCEANBASE')

    def test_database_migrator_enum_has_oceanbase(self):
        """Verify DatabaseMigrator enum includes OCEANBASE"""
        from api.db.db_models import DatabaseMigrator
        assert hasattr(DatabaseMigrator, 'OCEANBASE')

    def test_database_lock_enum_has_oceanbase(self):
        """Verify DatabaseLock enum includes OCEANBASE"""
        from api.db.db_models import DatabaseLock
        assert hasattr(DatabaseLock, 'OCEANBASE')

    def test_text_field_type_has_oceanbase(self):
        """Verify TextFieldType enum includes OCEANBASE"""
        from api.db.db_models import TextFieldType
        assert hasattr(TextFieldType, 'OCEANBASE')
        assert TextFieldType.OCEANBASE.value == "LONGTEXT"

    @patch.dict('os.environ', {'DB_TYPE': 'oceanbase'})
    def test_db_type_env_variable(self):
        """Verify DB_TYPE environment variable is read correctly"""
        import os
        assert os.getenv('DB_TYPE') == 'oceanbase'

    def test_oceanbase_inherits_mysql_pool(self):
        """Verify OceanBase class inherits from MySQL pool"""
        from api.db.db_models import RetryingPooledOceanBaseDatabase
        from playhouse.pool import PooledMySQLDatabase
        assert issubclass(RetryingPooledOceanBaseDatabase, PooledMySQLDatabase)

    def test_oceanbase_retry_logic(self):
        """Test retry logic on connection failure"""
        from api.db.db_models import RetryingPooledOceanBaseDatabase
        # Mock test for retry behavior
        pass  # Implementation details

    def test_oceanbase_lock_mechanism(self):
        """Test OceanBase distributed lock"""
        from api.db.db_models import OceanBaseDatabaseLock
        # Mock test for lock/unlock
        pass  # Implementation details
```

### 6.3 Integration Test Cases

```python
# test/integration/test_oceanbase_integration.py

import pytest
import os

@pytest.mark.skipif(
    os.getenv('DB_TYPE') != 'oceanbase',
    reason="OceanBase integration tests require DB_TYPE=oceanbase"
)
class TestOceanBaseIntegration:
    """Integration tests for OceanBase database"""

    def test_database_connection(self):
        """Test actual connection to OceanBase"""
        from api.db.db_models import DB
        assert DB.is_closed() == False or DB.connect()

    def test_create_table(self):
        """Test table creation in OceanBase"""
        pass

    def test_crud_operations(self):
        """Test basic CRUD operations"""
        pass

    def test_transaction_support(self):
        """Test transaction commit/rollback"""
        pass

    def test_distributed_lock(self):
        """Test GET_LOCK/RELEASE_LOCK functions"""
        pass
```

### 6.4 CI Configuration

Add to `.github/workflows/` or existing CI config:

```yaml
# OceanBase test job
test-oceanbase:
  runs-on: ubuntu-latest
  services:
    oceanbase:
      image: oceanbase/oceanbase-ce:latest
      ports:
        - 2881:2881
      env:
        OB_CLUSTER_NAME: ragflow
        OB_TENANT_NAME: ragflow
  env:
    DB_TYPE: oceanbase
    OCEANBASE_HOST: localhost
    OCEANBASE_PORT: 2881
    OCEANBASE_USER: root@ragflow
    OCEANBASE_PASSWORD: ''
    OCEANBASE_DBNAME: rag_flow
  steps:
    - uses: actions/checkout@v4
    - name: Set up Python
      uses: actions/setup-python@v4
      with:
        python-version: '3.12'
    - name: Install dependencies
      run: |
        pip install uv
        uv sync --python 3.12 --all-extras
    - name: Wait for OceanBase
      run: |
        # Wait for OceanBase to be ready
        sleep 60
    - name: Run OceanBase tests
      run: |
        uv run pytest test/test_oceanbase_db.py -v
```

---

## 7. Migration Guide for Users

### 7.1 Switching from MySQL to OceanBase

1. **Deploy OceanBase cluster** or use OceanBase Cloud

2. **Create database and user**:
   ```sql
   CREATE DATABASE rag_flow;
   CREATE USER 'ragflow'@'%' IDENTIFIED BY 'your_password';
   GRANT ALL PRIVILEGES ON rag_flow.* TO 'ragflow'@'%';
   ```

3. **Update environment variables**:
   ```bash
   export DB_TYPE=oceanbase
   export OCEANBASE_HOST=your_oceanbase_host
   export OCEANBASE_PORT=2881
   export OCEANBASE_USER=ragflow@your_tenant
   export OCEANBASE_PASSWORD=your_password
   export OCEANBASE_DBNAME=rag_flow
   ```

4. **Migrate data** (if needed):
   ```bash
   # Use mysqldump for data migration (OceanBase compatible)
   mysqldump -h mysql_host -u root -p rag_flow > backup.sql
   mysql -h oceanbase_host -P 2881 -u ragflow@tenant -p rag_flow < backup.sql
   ```

5. **Restart RAGFlow**

---

## 8. Compatibility Notes

### 8.1 OceanBase MySQL Compatibility

OceanBase's MySQL mode supports:
- MySQL 5.7/8.0 protocol
- Most MySQL SQL syntax
- `GET_LOCK`/`RELEASE_LOCK` functions
- `LONGTEXT` data type
- Connection pooling via MySQL drivers

### 8.2 Known Limitations

- Some MySQL-specific features may behave differently
- OceanBase has its own transaction isolation semantics
- Large object handling may differ slightly

### 8.3 Recommended OceanBase Version

- OceanBase CE 4.0+ or OceanBase Cloud
- MySQL tenant mode enabled

---

## 9. Rollback Plan

If issues arise:
1. Set `DB_TYPE=mysql` to revert to MySQL
2. No code changes required for rollback
3. Data migration back to MySQL if needed

---

## 10. Acceptance Criteria Checklist

- [ ] `DB_TYPE=oceanbase` environment variable enables OceanBase
- [ ] All existing database operations work with OceanBase
- [ ] Connection pooling functions correctly
- [ ] Retry logic handles OceanBase connection issues
- [ ] Distributed locks work via `GET_LOCK`/`RELEASE_LOCK`
- [ ] Schema migrations execute successfully
- [ ] CI tests pass for OceanBase scenarios
- [ ] Documentation updated

---

## 11. References

- [OceanBase MySQL Compatibility](https://www.oceanbase.com/docs/common-oceanbase-database-cn-1000000001576553)
- [RAGFlow Database Models](api/db/db_models.py)
- [Peewee ORM Documentation](http://docs.peewee-orm.com/)
