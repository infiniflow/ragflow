# Connection Pool Monitoring

This guide explains how to monitor database connection pool health, understand utilization metrics, configure thresholds, and troubleshoot connection exhaustion issues.

## Overview

RAGFlow uses connection pooling to efficiently manage database connections. The `PoolDiagnostics` class provides real-time monitoring and health checks to ensure optimal performance and prevent connection exhaustion.

## Connection Pool Basics

### What is a Connection Pool?

A connection pool maintains a set of database connections that are reused across requests:

- **Benefits**: Reduces overhead of creating new connections for each request
- **Limit**: Maximum connections are constrained by database capacity and system resources
- **Default Size**: 32 connections (configurable)

### Pool States

```
┌─────────────────────────────┐
│   Connection Pool (Max=32)  │
├─────────────────────────────┤
│  ✓ Active Connections (8)   │  In use by application
│  ✓ Idle Connections (20)    │  Available for reuse
└─────────────────────────────┘
```

**Note**: Peewee does not track waiting requests; only active and idle connections are monitored.

## Pool Monitoring

### Automatic Health Monitoring

RAGFlow automatically monitors pool health when the server starts:

```python
from api.db.connection import PoolDiagnostics, DB

# Monitoring starts automatically on server startup
# Health checks run every 60 seconds by default
# Logs warnings when utilization exceeds thresholds
```

### Manual Health Checks

Get pool statistics at any time:

```python
from api.db.connection import PoolDiagnostics, DB

# Get current pool statistics
stats = PoolDiagnostics.get_pool_stats(DB)

print(f"Active connections: {stats['active']}/{stats['max']}")
print(f"Idle connections: {stats['idle']}")
print(f"Utilization: {stats['utilization_percent']:.1f}%")
```

### Pool Statistics

The `get_pool_stats()` function returns:

| Metric | Description |
|--------|-------------|
| `max` | Maximum connections in the pool |
| `active` | Currently in-use connections |
| `idle` | Available connections waiting for use |
| `utilization_percent` | (active/max) * 100 |
| `total_created` | Total connections created (active + idle) |

**Note**: Peewee does not track waiting requests, so no `waiting` field is provided by `get_pool_stats()`.

## Understanding Utilization

### Utilization Thresholds

Three utilization levels trigger different log levels:

| Threshold | Level | Meaning |
|-----------|-------|---------|
| < 80% | DEBUG | Normal operation |
| 80-94% | WARNING | High utilization, monitor closely |
| ≥ 95% | CRITICAL | Danger zone, requests may be blocked |

### Example Scenarios

**Normal Operation (30% utilization)**:

```
Active: 10/32 connections | Idle: 22 | Utilization: 31%
Log: DEBUG - Connection pool: 10/32 active, 22 idle, 31.2% utilization
```

**Warning Level (85% utilization)**:

```
Active: 27/32 connections | Idle: 5 | Utilization: 84%
Log: WARNING - ⚠️ HIGH UTILIZATION: Connection pool: 27/32 active, 5 idle, 84.4% utilization
```

**Critical Level (97% utilization)**:

```
Active: 31/32 connections | Idle: 1 | Utilization: 97%
Log: CRITICAL - ⚠️ CRITICAL: Connection pool: 31/32 active, 1 idle, 96.9% utilization
```

## Configuring Thresholds

### Modifying Thresholds

Edit the thresholds in [api/db/connection.py](../api/db/connection.py):

```python
class PoolDiagnostics:
    HEALTH_CHECK_INTERVAL = 60  # Check every 60 seconds
    WARNING_THRESHOLD = 0.8     # 80% utilization
    CRITICAL_THRESHOLD = 0.95   # 95% utilization
```

To adjust:

```python
# Make it more sensitive (warn at 70%)
PoolDiagnostics.WARNING_THRESHOLD = 0.7

# Less frequent checks (check every 2 minutes)
PoolDiagnostics.HEALTH_CHECK_INTERVAL = 120
```

### Enabling/Disabling Monitoring

Start monitoring:

```python
from api.db.connection import PoolDiagnostics, DB

PoolDiagnostics.start_health_monitoring(DB, interval=60)
```

Stop monitoring:

```python
PoolDiagnostics.stop_health_monitoring()
```

## Troubleshooting Connection Issues

### Symptom: Utilization Stays at 100%

This indicates all connections are in use and requests may be blocked.

**Diagnosis**:

```python
from api.db.connection import PoolDiagnostics, DB

stats = PoolDiagnostics.get_pool_stats(DB)
if stats['utilization_percent'] >= 99:
    print("⚠️ POOL EXHAUSTED - All connections in use!")
    print(f"Active: {stats['active']}/{stats['max']}, Idle: {stats['idle']}")
```

**Solutions**:

1. **Increase pool size** (in server configuration):

   ```bash
   # In docker/.env or service_conf.yaml
   DB_POOL_SIZE=64  # Increase from default 32
   ```

2. **Reduce query duration**: Long-running queries hold connections

   ```python
   # Add timeouts
   DB.execute_sql("SET SESSION max_execution_time=30000")  # MySQL
   # PostgreSQL: statement_timeout = 30000
   ```

3. **Connection leak detection**: Check if connections are properly released

   ```python
   # Monitor active connections over time
   import time
   for i in range(10):
       stats = PoolDiagnostics.get_pool_stats(DB)
       print(f"[{i}] Active: {stats['active']}")
       time.sleep(5)
   ```

4. **Scale horizontally**: Use multiple API instances with load balancer

### Symptom: Frequent WARNING Logs

High utilization is normal during peak load, but persistent warnings indicate capacity issues.

**Analysis**:

```python
# Check peak utilization patterns
# Log messages appear when threshold exceeded
# Look for recurring times when this happens
```

**Solutions**:

1. **Analyze application code**: Find operations that use many connections:

   ```bash
   grep -r "DB\." api/ | grep -v "__pycache__"
   ```

2. **Optimize concurrent requests**: Batch operations when possible

3. **Monitor request rate**: Check if traffic is increasing:

   ```bash
   # In server logs, count requests per minute
   grep "Request:" logs/* | wc -l
   ```

### Symptom: Connection Errors During Load

Errors like "No available database connections" indicate pool exhaustion.

**Example Error**:

```
peewee.OperationalError: (2003, "Can't connect to MySQL server on 'localhost'")
```

**Immediate Actions**:

1. Check pool stats
2. Check server logs for long-running queries
3. Restart API server if hung connections exist

**Root Cause Analysis**:

```python
from api.db.migrations import MigrationHistory

# Check if migrations are hung
hung = MigrationHistory.select().where(
    MigrationHistory.status == "failed"
)

# Check connection creation rate
recent_history = MigrationHistory.select().order_by(
    MigrationHistory.applied_at.desc()
).limit(100)
```

## Performance Tuning

### Connection Pool Size

The optimal pool size depends on:

- **Concurrent users**: More users = larger pool needed
- **Query duration**: Longer queries = larger pool needed
- **System memory**: Each connection uses RAM (typically 1-10MB)

**Sizing Approach**:

1. **Start with baseline**: 20-50 connections for most applications
2. **Load test and monitor**: Measure actual connection utilization under realistic load
3. **Apply adjustment rules**:
   - Increase pool size if utilization consistently >80%
   - Decrease pool size if utilization consistently <30%
4. **Consider key factors**:
   - Peak concurrent requests per second
   - Average query duration
   - Request arrival rate patterns
   - Database server capacity

**Example Derivation**:

```
# Observed metrics during load testing:
# - 50 requests/second peak load
# - 200ms average query latency
# - Baseline pool: 30 connections

stats = PoolDiagnostics.get_pool_stats(DB)
print(f"Current utilization: {stats['utilization_percent']}%")

# If utilization shows 85% during peak:
# → Increase pool size to 40-45 connections
# → Re-test and measure utilization
# → Iterate until 70-80% utilization achieved
```

### Monitoring Pool Efficiency

Track how effectively the pool is being used:

```python
from api.db.connection import PoolDiagnostics, DB
import time

# Monitor pool utilization over time
for minute in range(60):
    stats = PoolDiagnostics.get_pool_stats(DB)
    total_created = stats['total_created']
    max_size = stats['max']
    active = stats['active']
    utilization = stats['utilization_percent']
    
    # Initialization percent: how much of the pool capacity was created
    initialization_percent = (total_created / max_size) * 100 if max_size > 0 else 0
    
    # Active vs created: are we using what we created?
    active_vs_created = (active / total_created) * 100 if total_created > 0 else 0
    
    print(f"Minute {minute}: {utilization}% util, {initialization_percent:.1f}% initialized, {active_vs_created:.1f}% of created in use")
    time.sleep(60)

# Future metrics to consider:
# - Connection reuse rate (requires 'reuse_count' stat)
# - Idle vs active time ratio (requires 'idle_time' tracking)
```

### Connection Reuse

Good connection reuse means:

- Pool size equals max size (all connections created)
- Idle count decreases during load, increases during quiet times
- No connections stuck in "active" when idle

**Bad Pattern**:

```
Active: 1/32, Idle: 0  # Only 1 connection created, others unused
```

**Good Pattern**:

```
Active: 20/32, Idle: 12  # Pool well-utilized, connections available
```

## Database-Specific Considerations

### MySQL Connection Pool

- Default max connections: 32 (configurable in `docker/.env`)
- MySQL server also has a `max_connections` setting (default 151)
- Monitor with: `SHOW PROCESSLIST;`

```sql
-- Check MySQL connection limit
SHOW VARIABLES LIKE 'max_connections';

-- Check current connections
SHOW PROCESSLIST;
SHOW STATUS LIKE 'Threads%';
```

### PostgreSQL Connection Pool

- Default max connections: 32 (configurable)
- PostgreSQL server also has `max_connections` setting (default 100)
- Monitor with: `SELECT * FROM pg_stat_activity;`

```sql
-- Check PostgreSQL connection limit
SHOW max_connections;

-- Check current connections
SELECT state, COUNT(*) FROM pg_stat_activity GROUP BY state;

-- List active connections
SELECT pid, usename, query FROM pg_stat_activity WHERE state = 'active';
```

## Monitoring Best Practices

### ✅ Do

- Monitor pool utilization regularly
- Set up alerts for WARNING and CRITICAL levels
- Track utilization patterns over time
- Test pool behavior under load before production
- Document your pool size decision
- Monitor both application and database

### ❌ Don't

- Ignore WARNING level logs
- Run with pool size = 1 (serializes all queries)
- Set pool size larger than database allows
- Assume default pool size works for all workloads
- Use single-connection pool in production

## Debugging Connection Leaks

If connections seem to be leaking (not being returned to pool):

### 1. Identify Leak Pattern

```python
from api.db.connection import PoolDiagnostics, DB
import time

print("Checking for connection leaks...")
baseline = PoolDiagnostics.get_pool_stats(DB)
print(f"Baseline active: {baseline['active']}")

# Run your application operation here
# app.handle_request()

time.sleep(2)
after = PoolDiagnostics.get_pool_stats(DB)
print(f"After operation: {after['active']}")

if after['active'] > baseline['active']:
    print("⚠️ Possible connection leak detected!")
```

### 2. Find the Culprit

Look for database operations that don't close connections:

```python
from api.db.connection import DB

# Bad Example 1: Starting a transaction without commit/rollback
def bad_transaction():
    DB.begin()  # Transaction started
    Documents.create(name="test")
    # Missing DB.commit() or DB.rollback()!
    # Connection stays open with uncommitted transaction

# Bad Example 2: Manual connection without closing
def bad_manual_connection():
    conn = DB.connection()  # Acquires connection from pool
    cursor = conn.cursor()
    cursor.execute("SELECT * FROM documents")
    # Missing conn.close()!
    # Connection leaked and never returned to pool

# Bad Example 3: Exception mid-transaction without cleanup
def bad_exception_handling():
    DB.begin()
    try:
        Documents.create(name="test")
        raise ValueError("Something went wrong")
        DB.commit()
    except ValueError:
        pass  # Exception caught but DB.rollback() never called!
        # Transaction left open, connection leaked

# Good: Use DB.atomic() for automatic cleanup
def good_operation():
    with DB.atomic():  # Automatic commit on success, rollback on exception
        results = Documents.select()
        # Process results
    # Connection automatically returned to pool

# Good: Proper manual connection handling
def good_manual_connection():
    conn = DB.connection()
    try:
        cursor = conn.cursor()
        cursor.execute("SELECT * FROM documents")
        # Process results
    finally:
        conn.close()  # Always close in finally block

# Good: Proper exception handling around transactions
def good_exception_handling():
    try:
        with DB.atomic():  # Handles commit/rollback automatically
            Documents.create(name="test")
            raise ValueError("Something went wrong")
    except ValueError:
        pass  # Transaction already rolled back by atomic() context manager
```

### 3. Fix and Verify

```python
# Restart server to clear leaked connections
# Fix the code
# Monitor pool to verify fix
stats = PoolDiagnostics.get_pool_stats(DB)
print(f"Pool health after fix: {stats}")
```

## Transaction Monitoring

Pool health is closely related to transaction management:

```python
from api.db.connection import TransactionLogger, DB

# Log transaction state for debugging
TransactionLogger.log_transaction_state(DB, "begin", extra_info="Complex operation")
try:
    with DB.atomic():
        # Do work
        pass
    TransactionLogger.log_transaction_state(DB, "commit")
except Exception as e:
    TransactionLogger.log_transaction_error(DB, e, context="Complex operation")
```

## Metrics and Alerts

### Key Metrics to Monitor

1. **Pool Utilization %**: Should stay below 80% under normal load
2. **Active Connections**: Should vary with traffic patterns
3. **Connection Errors**: Should be zero in normal operation
4. **Query Duration**: Long queries tie up connections
5. **Request Throughput**: Connections per second

### Recommended Alerts

```
WARNING if utilization > 80% for 5+ minutes
CRITICAL if utilization > 95% for any duration
ALERT if error rate > 0.1% (connection errors)
ALERT if max_connections exceeded (requests blocked)
```

## Related Documentation

- [Database Migrations](DATABASE_MIGRATIONS.md) - Manage schema changes
- [Database Compatibility](DATABASE_COMPATIBILITY.md) - Multi-database support
- [AGENTS.md](../AGENTS.md) - Project build and configuration
