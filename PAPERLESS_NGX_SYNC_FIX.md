# Paperless NGX Sync Behavior Fix

## Problem Statement

The Paperless NGX document retrieval was starting on startup/connector creation, which was not desired. The requirement is:

- **Should NOT** start on initial startup/connector creation
- **SHOULD** start when integration settings are changed
- The 30-minute refresh timer (`refresh_freq`) is acceptable
- Should only sync documents that have actually changed in Paperless NGX

## Solution

### Changes Made

#### 1. Skip Initial Reindex on Connector Link (`api/db/services/connector_service.py`)

When a Paperless NGX connector is **first linked** to a knowledge base:
- Skip the automatic `reindex=True` scheduling
- Let the regular polling mechanism handle updates based on `refresh_freq`
- The polling will use time-based filtering to only retrieve changed documents

```python
# New connector being linked
if e and connector_obj.source == FileSource.PAPERLESS_NGX:
    # Skip automatic scheduling for Paperless NGX on initial link
    logging.info(f"Skipping initial reindex for Paperless NGX connector {conn_id} on first link")
    continue
```

#### 2. Trigger Sync on Integration Change (`api/db/services/connector_service.py`)

When a Paperless NGX connector's **integration settings are changed** (re-linking):
- Trigger an incremental sync from the last successful poll end time
- If no previous successful sync exists, trigger a full sync

```python
# Check if this is an existing connector (integration change)
if conn_id in old_conn_ids:
    # For Paperless NGX, trigger sync when integration is changed
    if e and connector_obj.source == FileSource.PAPERLESS_NGX:
        task = SyncLogsService.get_latest_task(conn_id, kb_id)
        if task and task.status == TaskStatus.DONE:
            # Schedule incremental sync from last poll end time
            SyncLogsService.schedule(conn_id, kb_id, task.poll_range_end, total_docs_indexed=task.total_docs_indexed)
        else:
            # No previous successful sync, schedule from beginning
            SyncLogsService.schedule(conn_id, kb_id, reindex=True)
```

#### 3. Trigger Sync on Config Update (`api/apps/connector_app.py`)

When a Paperless NGX connector's **config is updated** via `/set` endpoint:
- Trigger sync for all knowledge bases linked to this connector
- Use incremental sync if there was a previous successful sync
- Use full sync if this is the first sync or previous sync failed

```python
if req.get("id"):
    # Updating existing connector
    ConnectorService.update_by_id(req["id"], conn)
    
    # For Paperless NGX, trigger sync when config is changed
    if e and connector_obj.source == FileSource.PAPERLESS_NGX:
        for c2k in Connector2KbService.query(connector_id=req["id"]):
            task = SyncLogsService.get_latest_task(req["id"], c2k.kb_id)
            if task and task.status == TaskStatus.DONE:
                # Schedule incremental sync
                SyncLogsService.schedule(req["id"], c2k.kb_id, task.poll_range_end, total_docs_indexed=task.total_docs_indexed)
            elif not task or task.status in [TaskStatus.FAIL, TaskStatus.CANCEL]:
                # Schedule full sync
                SyncLogsService.schedule(req["id"], c2k.kb_id, reindex=True)
```

## Behavior Summary

| Event | Previous Behavior | New Behavior |
|-------|------------------|--------------|
| Connector first linked to KB | Immediate full sync (reindex) | No immediate sync, waits for polling schedule |
| Integration settings changed (re-link) | No sync triggered | Incremental sync triggered immediately |
| Connector config updated via `/set` | No sync triggered | Incremental sync triggered for all linked KBs |
| Regular polling (every `refresh_freq` min) | Full or incremental sync | No change - continues as before |

## How Paperless NGX Sync Works

### Polling Mechanism

The sync service (`rag/svr/sync_data_source.py`) uses a polling mechanism:

1. Every `refresh_freq` minutes (default: 30), it checks for scheduled tasks
2. For Paperless NGX, it uses time-based filtering:
   - `modified__gte`: Documents modified after `poll_range_start`
   - `modified__lte`: Documents modified before `poll_range_end`
3. Only documents that changed in that time window are synced

### Incremental vs Full Sync

- **Incremental Sync**: Uses `poll_source(start, end)` to get only changed documents
- **Full Sync**: Uses `load_from_state()` to get all documents from epoch to now
- The connector's time-based filtering ensures only changed documents are retrieved

## Testing Scenarios

### 1. Initial Connector Creation
```bash
# Create Paperless NGX connector
POST /v1/connector/set
{
  "source": "paperless_ngx",
  "name": "My Paperless",
  "config": {"base_url": "http://paperless:8000"},
  "credentials": {"api_token": "xxx"}
}

# Link to knowledge base
POST /v1/kb/update
{
  "kb_id": "xxx",
  "connectors": [{"id": "connector-id"}]
}

# Expected: No immediate sync
# Documents will sync after 30 minutes (or configured refresh_freq)
```

### 2. Integration Settings Change
```bash
# Re-link connector (e.g., change auto_parse setting)
POST /v1/kb/update
{
  "kb_id": "xxx",
  "connectors": [{"id": "connector-id", "auto_parse": "0"}]
}

# Expected: Immediate incremental sync triggered
# Only documents changed since last sync are retrieved
```

### 3. Connector Config Update
```bash
# Update connector config (e.g., change base_url)
POST /v1/connector/set
{
  "id": "connector-id",
  "config": {"base_url": "http://new-paperless:8000"}
}

# Expected: Immediate incremental sync triggered for all linked KBs
# Only documents changed since last sync are retrieved
```

## Files Modified

1. `api/db/services/connector_service.py`
   - Modified `link_connectors()` method to handle Paperless NGX specially
   
2. `api/apps/connector_app.py`
   - Modified `set_connector()` endpoint to trigger sync on config updates

## Backward Compatibility

This change only affects **Paperless NGX** connectors. All other connector types continue to work exactly as before:
- Other connectors still get immediate reindex when first linked
- Other connectors are unaffected by this change

## Notes

- The 30-minute polling frequency is configurable via `refresh_freq` parameter
- The connector uses Paperless NGX's `modified__gte` and `modified__lte` filters to only retrieve changed documents
- This approach minimizes unnecessary data transfer and processing
- Logging has been added to track when syncs are scheduled or skipped
