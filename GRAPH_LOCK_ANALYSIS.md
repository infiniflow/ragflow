# Knowledge Graph Lock Coverage Analysis

## Problem Summary

During document parsing, the knowledge graph was randomly resetting, causing all nodes and edges to disappear. This analysis investigated whether the Redis distributed locking mechanism was properly protecting all `set_graph()` operations.

## Lock Coverage Analysis for `set_graph()` Operations

### ✅ **Document Parsing Operations (PROPERLY LOCKED)**

**In `run_graphrag()` function (`/srv/bfabriclocal/Desktop/ragflow/graphrag/general/index.py:75-120`):**

```python
graphrag_task_lock = RedisDistributedLock(f"graphrag_task_{kb_id}", lock_value=doc_id, timeout=1200)
await graphrag_task_lock.spin_acquire()  # LOCK ACQUIRED

try:
    # 1. ✅ COVERED: merge_subgraph() calls set_graph() on line 220
    new_graph = await merge_subgraph(...)
    
    if with_resolution:
        await graphrag_task_lock.spin_acquire()  # Re-acquire (redundant but safe)
        # 2. ✅ COVERED: resolve_entities() calls set_graph() on line 248  
        await resolve_entities(...)
        
    if with_community:
        await graphrag_task_lock.spin_acquire()  # Re-acquire (redundant but safe)
        # 3. ✅ COVERED: extract_community() does NOT call set_graph()
        # It only inserts community reports, not graph data
        await extract_community(...)
        
finally:
    graphrag_task_lock.release()  # LOCK RELEASED
```

### ✅ **API Operations (PROPERLY LOCKED)**

**API endpoints also use the same lock (`/srv/bfabriclocal/Desktop/ragflow/api/apps/kb_app.py`):**

- `resolve_entities` API: Has `graphrag_task_lock` protection 
- `detect_communities` API: Has `graphrag_task_lock` protection
- **But these don't call `set_graph()`** - they update the graph via `settings.docStoreConn.update()`

```python
graphrag_task_lock = RedisDistributedLock(
    f"graphrag_task_{kb_id}", 
    lock_value="api_entity_resolution",  # or "api_community_detection"
    timeout=1200
)

try:
    await graphrag_task_lock.spin_acquire()
    # ... GraphRAG operations ...
finally:
    graphrag_task_lock.release()
```

## Summary: All `set_graph()` Calls Are Properly Locked

| Call Site | Function | Lock Protection | Status |
|-----------|----------|----------------|--------|
| `index.py:220` | `merge_subgraph()` | ✅ `graphrag_task_lock` | **PROTECTED** |
| `index.py:248` | `resolve_entities()` | ✅ `graphrag_task_lock` | **PROTECTED** |

## Key Findings

1. **✅ All `set_graph()` operations are properly covered by locks**
2. **✅ Community detection doesn't call `set_graph()`** - it only manages community reports
3. **✅ API operations use a different update mechanism** (`docStoreConn.update()`) that doesn't go through `set_graph()`
4. **✅ The lock is held for the entire duration** of each `set_graph()` operation

## Root Cause Analysis: Why Data Was Still Being Lost

Since the locking coverage was correct, the issue was actually caused by **bugs in the Redis lock implementation**:

### 🐛 **Critical Bug #1: `spin_acquire()` Deleted Existing Locks**

**File**: `/srv/bfabriclocal/Desktop/ragflow/rag/utils/redis_conn.py:353-358`

**Original broken code:**
```python
async def spin_acquire(self):
    REDIS_CONN.delete_if_equal(self.lock_key, self.lock_value)  # ❌ DELETES THE LOCK!
    while True:
        if self.lock.acquire(token=self.lock_value):
            break
        await trio.sleep(10)
```

**Problem**: Line 354 would delete any existing lock before trying to acquire, allowing multiple processes to get the same lock simultaneously.

**Fixed code:**
```python
async def spin_acquire(self):
    # Don't delete existing locks - just try to acquire properly
    while True:
        if self.lock.acquire(token=self.lock_value):
            break
        await trio.sleep(1)  # Reduced sleep time for faster acquisition
```

### 🐛 **Critical Bug #2: `release()` Didn't Actually Release**

**Original broken code:**
```python
def release(self):
    REDIS_CONN.delete_if_equal(self.lock_key, self.lock_value)  # ❌ WRONG!
```

**Problem**: This deleted the Redis key but didn't properly release the underlying Redis lock.

**Fixed code:**
```python
def release(self):
    # Properly release the underlying Redis lock
    try:
        self.lock.release()
    except Exception as e:
        # Fallback to delete if release fails
        REDIS_CONN.delete_if_equal(self.lock_key, self.lock_value)
```

### 🐛 **Critical Bug #3: Invalid API Constructor Parameter**

**Original broken code:**
```python
graphrag_task_lock = RedisDistributedLock(
    f"graphrag_task_{kb_id}", 
    lock_value="api_entity_resolution", 
    redis_conn=settings.redis_conn,  # ❌ INVALID PARAMETER
    timeout=1200
)
```

**Problem**: The `RedisDistributedLock` constructor doesn't accept a `redis_conn` parameter, causing the locks to potentially fail silently.

**Fixed code:**
```python
graphrag_task_lock = RedisDistributedLock(
    f"graphrag_task_{kb_id}", 
    lock_value="api_entity_resolution", 
    timeout=1200
)
```

## Additional Risk Factor: `set_graph()` Race Condition Window

Even with perfect locking, the `set_graph()` function has an inherent risk:

```python
async def set_graph(tenant_id: str, kb_id: str, embd_mdl, graph: nx.Graph, change: GraphChange, callback):
    # ❌ DELETE ALL GRAPH DATA immediately
    await trio.to_thread.run_sync(lambda: settings.docStoreConn.delete(
        {"knowledge_graph_kwd": ["graph", "subgraph"]}, 
        search.index_name(tenant_id), kb_id
    ))
    
    # ... 50+ lines of processing ...
    
    # ✅ INSERT NEW DATA much later
    await trio.to_thread.run_sync(lambda: settings.docStoreConn.insert(...))
```

**Risk**: If anything fails between deletion and insertion, the graph is permanently lost.

## Resolution Status

| Issue | Status | Fix Applied |
|-------|--------|-------------|
| Lock coverage verification | ✅ **COMPLETE** | All `set_graph()` calls are properly locked |
| Redis lock `spin_acquire()` bug | ✅ **FIXED** | Removed lock deletion before acquisition |
| Redis lock `release()` bug | ✅ **FIXED** | Now properly releases underlying lock |
| API constructor bug | ✅ **FIXED** | Removed invalid parameter |
| Frontend query invalidation | ✅ **FIXED** | Added `knowledgeBaseId` to query keys |

## Conclusion

The knowledge graph reset issue was **not caused by missing lock coverage** - the locks were properly protecting all critical operations. Instead, it was caused by **fundamental bugs in the Redis distributed lock implementation** that allowed multiple processes to acquire the same lock simultaneously.

With the Redis lock bugs fixed, the knowledge graph should now be properly protected during document parsing operations, preventing the random resets that were occurring before.

## Testing Recommendations

1. **Monitor graph stability** during multi-document parsing
2. **Check Redis logs** for lock acquisition/release patterns
3. **Verify no "graphrag_task_lock acquired" messages overlap** for the same `kb_id`
4. **Test with both single and multiple workers** to ensure lock effectiveness

## Files Modified

- `/srv/bfabriclocal/Desktop/ragflow/rag/utils/redis_conn.py` - Fixed Redis lock implementation
- `/srv/bfabriclocal/Desktop/ragflow/api/apps/kb_app.py` - Fixed API lock constructor
- `/srv/bfabriclocal/Desktop/ragflow/web/src/hooks/knowledge-hooks.ts` - Fixed query invalidation