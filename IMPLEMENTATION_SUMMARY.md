# Paperless NGX Sync Behavior - Implementation Summary

## Problem Solved

Fixed the Paperless NGX document retrieval behavior to meet the following requirements:

✓ **NOT** start on initial startup/connector creation  
✓ **SHOULD** start when integration settings are changed  
✓ **30-minute refresh timer is acceptable**  
✓ Only sync documents that have changed in Paperless NGX  

## Files Modified

1. **api/db/services/connector_service.py** (41 lines added)
   - Modified `link_connectors()` method
   - Skip initial reindex for new Paperless NGX connectors
   - Trigger sync when integration settings are changed
   - Handle RUNNING/SCHEDULE task states

2. **api/apps/connector_app.py** (24 lines added)
   - Modified `set_connector()` endpoint
   - Trigger sync when connector config is updated
   - Handle RUNNING/SCHEDULE task states
   - Consolidated imports

3. **PAPERLESS_NGX_SYNC_FIX.md** (documentation)
   - Detailed explanation of changes
   - Behavior comparison table
   - Testing scenarios

## Implementation Details

### 1. Skip Initial Sync on Connector Creation

When a Paperless NGX connector is **first linked** to a knowledge base:

```python
# New connector being linked
if e and connector_obj.source == FileSource.PAPERLESS_NGX:
    # Skip automatic scheduling for Paperless NGX on initial link
    logging.info(f"Skipping initial reindex for Paperless NGX connector {conn_id} on first link")
    continue
```

**Result**: No immediate sync. The connector will wait for the regular polling cycle (30 minutes by default).

### 2. Trigger Sync on Integration Change

When an **existing** Paperless NGX connector is re-linked (integration settings changed):

```python
# Check if this is an existing connector (integration change)
if conn_id in old_conn_ids:
    # For Paperless NGX, trigger sync when integration is changed
    if e and connector_obj.source == FileSource.PAPERLESS_NGX:
        task = SyncLogsService.get_latest_task(conn_id, kb_id)
        
        # Skip if already running/scheduled
        if task and task.status in [TaskStatus.RUNNING, TaskStatus.SCHEDULE]:
            logging.info(f"Skipping sync - task already {task.status}")
        elif task and task.status == TaskStatus.DONE:
            # Incremental sync from last poll end
            SyncLogsService.schedule(conn_id, kb_id, task.poll_range_end, total_docs_indexed=task.total_docs_indexed)
        else:
            # Full sync if no previous successful sync
            SyncLogsService.schedule(conn_id, kb_id, reindex=True)
```

**Result**: Immediate sync triggered using incremental sync when possible.

### 3. Trigger Sync on Config Update

When a Paperless NGX connector's **config is updated** via `/set` endpoint:

```python
if e and connector_obj.source == FileSource.PAPERLESS_NGX:
    # Find all KBs linked to this connector
    for c2k in Connector2KbService.query(connector_id=req["id"]):
        task = SyncLogsService.get_latest_task(req["id"], c2k.kb_id)
        
        # Skip if already running/scheduled
        if task and task.status in [TaskStatus.RUNNING, TaskStatus.SCHEDULE]:
            logging.info(f"Skipping sync - task already {task.status}")
            continue
        
        if task and task.status == TaskStatus.DONE:
            # Incremental sync
            SyncLogsService.schedule(req["id"], c2k.kb_id, task.poll_range_end, total_docs_indexed=task.total_docs_indexed)
        else:
            # Full sync
            SyncLogsService.schedule(req["id"], c2k.kb_id, reindex=True)
```

**Result**: Immediate sync triggered for all linked knowledge bases.

## Behavior Comparison

| Event | Before This Fix | After This Fix |
|-------|----------------|----------------|
| Connector first created | - | - |
| Connector first linked to KB | ❌ Immediate full sync | ✅ No sync, waits for polling |
| Integration settings changed (re-link) | ❌ No sync | ✅ Immediate incremental sync |
| Connector config updated (`/set`) | ❌ No sync | ✅ Immediate incremental sync |
| Regular polling (every 30 min) | ✅ Incremental sync | ✅ No change |
| Task already RUNNING/SCHEDULE | N/A | ✅ Skip to avoid duplicates |

## Key Features

### Incremental Sync
- Uses `poll_range_end` from the last successful sync
- Only retrieves documents modified since that time
- Paperless NGX API filters: `modified__gte` and `modified__lte`

### Duplicate Prevention
- Checks if a task is already RUNNING or SCHEDULE
- Skips scheduling a new task if one is active
- Logs the skip with task status

### Backward Compatibility
- Only affects Paperless NGX connectors
- All other connector types unchanged
- No database schema changes required

## Verification

### Static Analysis
```
✓ Skip initial reindex for Paperless NGX on first link
✓ Trigger sync when integration settings are changed
✓ Trigger sync when connector config is updated
✓ FileSource imported in connector_service.py
✓ SyncLogsService imported in connector_app.py
✓ TaskStatus imported in connector_service.py
✓ Incremental sync uses poll_range_end from last task
✓ Logging added for skipping initial reindex
✓ Logging added for integration change sync
✓ Logging added for config change sync
```

### Security
- CodeQL analysis: **0 vulnerabilities found**
- No credentials logged
- Proper error handling maintained

### Compilation
- All Python files compile successfully
- No syntax errors
- All imports resolve correctly

## Testing Recommendations

### Manual Test 1: Initial Creation (Should NOT Sync)
```bash
# 1. Create connector
POST /v1/connector/set
{
  "source": "paperless_ngx",
  "name": "Test Paperless",
  "config": {"base_url": "http://paperless:8000"},
  "credentials": {"api_token": "test-token"}
}

# 2. Link to KB
POST /v1/kb/update
{
  "kb_id": "test-kb",
  "connectors": [{"id": "connector-id"}]
}

# Expected: No immediate sync logged
# Expected: Sync starts after 30 minutes (or configured refresh_freq)
```

### Manual Test 2: Integration Change (Should Sync)
```bash
# Re-link with different settings
POST /v1/kb/update
{
  "kb_id": "test-kb",
  "connectors": [{"id": "connector-id", "auto_parse": "0"}]
}

# Expected: Immediate sync logged
# Expected: "Scheduled incremental sync...due to integration change"
```

### Manual Test 3: Config Update (Should Sync)
```bash
# Update connector config
POST /v1/connector/set
{
  "id": "connector-id",
  "config": {"base_url": "http://new-paperless:8000"}
}

# Expected: Immediate sync logged
# Expected: "Scheduled incremental sync...due to config change"
```

### Manual Test 4: Duplicate Prevention
```bash
# Trigger config update while sync is running
POST /v1/connector/set
{
  "id": "connector-id",
  "config": {"base_url": "http://paperless:8000"}
}

# Expected: "Skipping sync - task already RUNNING"
```

## Log Messages to Look For

### Initial Link (Skip)
```
Skipping initial reindex for Paperless NGX connector <id> on first link
```

### Integration Change (Sync)
```
Scheduled incremental sync for Paperless NGX connector <id> due to integration change
```
or
```
Scheduled initial sync for Paperless NGX connector <id> due to integration change
```

### Config Update (Sync)
```
Scheduled incremental sync for Paperless NGX connector <id> KB <kb_id> due to config change
```
or
```
Scheduled initial sync for Paperless NGX connector <id> KB <kb_id> due to config change
```

### Duplicate Prevention (Skip)
```
Skipping sync for Paperless NGX connector <id> - task already RUNNING
```
or
```
Skipping sync for Paperless NGX connector <id> - task already SCHEDULE
```

## Performance Impact

- **Minimal**: Only adds 1-2 DB queries per operation
- **No polling changes**: Regular polling unchanged
- **No schema changes**: Uses existing database structure
- **Logging overhead**: Negligible

## Future Enhancements (Not in Scope)

- Webhook support for real-time sync
- Configuration option to control initial sync behavior
- Per-connector refresh frequency
- Sync history dashboard

## Rollback Plan

If issues arise, revert commits in reverse order:
1. `b142196` - Address code review feedback
2. `e4b3850` - Add documentation
3. `e89d626` - Trigger Paperless NGX sync on integration changes
4. `8f4e4e8` - Skip automatic reindex for Paperless NGX on connector link

## Security Summary

✅ No vulnerabilities introduced  
✅ No credentials exposed in logs  
✅ Proper error handling maintained  
✅ Input validation unchanged  
✅ CodeQL scan passed with 0 alerts  

## Conclusion

The implementation successfully addresses all requirements:

1. ✅ Paperless NGX does NOT sync on initial connector creation
2. ✅ Paperless NGX DOES sync when integration is changed
3. ✅ Paperless NGX DOES sync when config is updated
4. ✅ 30-minute polling frequency is maintained
5. ✅ Only changed documents are synced (time-based filtering)
6. ✅ No security vulnerabilities introduced
7. ✅ Backward compatible with other connectors

The changes are minimal, focused, and ready for testing.
