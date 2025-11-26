# Task Service Analysis - Background Job Queue

## Tổng Quan

`task_service.py` (20KB) quản lý background task queue cho document processing, sử dụng Redis làm message broker.

## File Location
```
/api/db/services/task_service.py
```

## Task Model

```python
class Task(DataBaseModel):
    id = CharField(primary_key=True, max_length=32)
    doc_id = CharField(max_length=32, index=True)

    # Page range for chunked processing
    from_page = IntegerField(default=0)
    to_page = IntegerField(default=-1)  # -1 = all pages

    # Task type
    task_type = CharField(max_length=32, default="")
    # Types: "", "graphrag", "raptor", "mindmap", "dataflow"

    # Priority (lower = higher priority)
    priority = IntegerField(default=0)

    # Timing
    begin_at = DateTimeField(null=True)
    process_duration = FloatField(default=0)

    # Progress tracking
    progress = FloatField(default=0)  # 0-1, -1 = failed
    progress_msg = TextField(default="")

    # Retry handling
    retry_count = IntegerField(default=0)

    # Config digest for chunk reuse
    digest = TextField(default="")

    # Result: space-separated chunk IDs
    chunk_ids = LongTextField(default="")
```

## Task Lifecycle

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         TASK LIFECYCLE                                   │
└─────────────────────────────────────────────────────────────────────────┘

[1] TASK CREATION (queue_tasks)
    │
    ├─── Calculate page ranges
    ├─── Create Task records in MySQL
    ├─── Calculate config digest (xxhash)
    ├─── Check for chunk reuse from previous tasks
    └─── Push to Redis queue

         │
         ▼
[2] TASK PENDING
    │
    │  Status: progress=0, retry_count=0
    │  Location: Redis queue
    │
         │
         ▼
[3] TASK PICKED UP (get_task)
    │
    ├─── Consumer pulls from Redis
    ├─── Increment retry_count
    ├─── Load full task context (doc, kb, tenant)
    └─── Return task dict or None if max retries exceeded

         │
         ▼
[4] TASK PROCESSING
    │
    │  Status: progress=0.x, begin_at=now
    │  Worker: task_executor.py
    │
    ├─── Download file from MinIO
    ├─── Parse document
    ├─── Generate chunks
    ├─── Create embeddings
    ├─── Store in Elasticsearch
    └─── Update progress periodically

         │
         ▼
[5] TASK COMPLETED
    │
    │  Status: progress=1.0 (success) or -1 (failed)
    │  Stored: chunk_ids in Task record
    │
    └─── Document.progress updated via _sync_progress()
```

## Core Methods

### queue_tasks() - Task Creation

```python
def queue_tasks(doc: dict, bucket: str, name: str, priority: int = 0):
    """
    Create and queue processing tasks for a document.

    Args:
        doc: Document dict with id, kb_id, parser_config
        bucket: Storage bucket name
        name: File name
        priority: Task priority (0 = normal)

    Process:
        1. Calculate page/row ranges based on file type
        2. Create Task records
        3. Check for chunk reuse
        4. Push to Redis queue
    """

    # Get file from storage
    blob = STORAGE_IMPL.get(bucket, name)

    # Determine splitting strategy based on file type
    if is_pdf(name):
        # PDF: Split by pages
        pages = get_pdf_page_count(blob)
        page_size = doc["parser_config"].get("task_page_size", 12)

        tasks = []
        for start in range(0, pages, page_size):
            end = min(start + page_size, pages)
            tasks.append({
                "id": get_uuid(),
                "doc_id": doc["id"],
                "from_page": start,
                "to_page": end,
                "digest": calculate_digest(doc, start, end)
            })

    elif is_excel(name) and doc["parser_id"] == "table":
        # Excel: Split by rows
        row_count = get_excel_row_count(blob)
        row_size = 3000

        tasks = []
        for start in range(0, row_count, row_size):
            end = min(start + row_size, row_count)
            tasks.append({
                "id": get_uuid(),
                "doc_id": doc["id"],
                "from_page": start,
                "to_page": end,
                "digest": calculate_digest(doc, start, end)
            })

    else:
        # Other files: Single task
        tasks = [{
            "id": get_uuid(),
            "doc_id": doc["id"],
            "from_page": 0,
            "to_page": -1,
            "digest": calculate_digest(doc, 0, -1)
        }]

    # Check for chunk reuse from previous tasks
    prev_tasks = Task.select().where(Task.doc_id == doc["id"])
    for task in tasks:
        reused = reuse_prev_task_chunks(task, prev_tasks, doc["parser_config"])
        if reused:
            task["progress"] = 1.0  # Mark as done

    # Bulk insert tasks
    bulk_insert_into_db(Task, tasks, ignore_conflicts=True)

    # Queue unfinished tasks to Redis
    for task in tasks:
        if task["progress"] < 1.0:
            REDIS_CONN.queue_product(
                get_queue_name(doc["tenant_id"]),
                task
            )
```

### get_task() - Task Retrieval

```python
@classmethod
@DB.connection_context()
def get_task(cls, task_id: str, doc_ids: list = []):
    """
    Retrieve task with full context for processing.

    Args:
        task_id: Task ID
        doc_ids: Optional filter by document IDs

    Returns:
        Task dict with document, KB, and tenant info
        None if task should be abandoned (max retries)
    """

    # Increment retry count
    Task.update(
        retry_count=Task.retry_count + 1
    ).where(Task.id == task_id).execute()

    # Check max retries
    task = Task.get_by_id(task_id)
    if task.retry_count >= 3:
        # Abandon task
        Task.update(
            progress=-1,
            progress_msg="Task abandoned after 3 retries"
        ).where(Task.id == task_id).execute()
        return None

    # Build full context query
    query = (Task
        .select(Task, Document, Knowledgebase, Tenant)
        .join(Document, on=(Task.doc_id == Document.id))
        .join(Knowledgebase, on=(Document.kb_id == Knowledgebase.id))
        .join(Tenant, on=(Knowledgebase.tenant_id == Tenant.id))
        .where(Task.id == task_id))

    if doc_ids:
        query = query.where(Task.doc_id.in_(doc_ids))

    result = list(query.dicts())
    return result[0] if result else None
```

### update_progress() - Safe Progress Update

```python
@classmethod
@DB.connection_context()
def update_progress(cls, id: str, info: dict):
    """
    Safely update task progress.

    Safety rules:
        - progress_msg: Always append (not replace)
        - progress: Only update if new > current (or new == -1 for failure)

    Args:
        id: Task ID
        info: Dict with progress and/or progress_msg
    """

    # Get current state
    task = Task.get_by_id(id)

    # Handle progress_msg (append)
    if "progress_msg" in info:
        # Append new message
        current_msg = task.progress_msg or ""
        new_msg = info["progress_msg"]
        combined = current_msg + new_msg

        # Trim to 3000 lines if needed
        lines = combined.split("\n")
        if len(lines) > 3000:
            combined = "\n".join(lines[-3000:])

        info["progress_msg"] = combined

    # Handle progress (only increase)
    if "progress" in info:
        new_progress = info["progress"]
        current_progress = task.progress

        # Only update if:
        # - Current is not failed (-1) AND
        # - New is failure (-1) OR new > current
        if current_progress == -1:
            del info["progress"]  # Don't update failed tasks
        elif new_progress != -1 and new_progress <= current_progress:
            del info["progress"]  # Don't go backwards

    # Use database lock for thread safety (non-macOS)
    if platform.system() != "Darwin":
        with DB.lock(f"task_progress_{id}", timeout=30):
            Task.update(info).where(Task.id == id).execute()
    else:
        Task.update(info).where(Task.id == id).execute()
```

### reuse_prev_task_chunks() - Optimization

```python
def reuse_prev_task_chunks(
    task: dict,
    prev_tasks: list,
    chunking_config: dict
) -> int:
    """
    Attempt to reuse chunks from previous task runs.

    Reuse conditions:
        - Same from_page
        - Same config digest
        - Previous task completed (progress=1.0)
        - Previous task has chunk_ids

    Returns:
        Number of chunks reused (0 if not reused)
    """

    for prev in prev_tasks:
        if (prev.from_page == task["from_page"] and
            prev.digest == task["digest"] and
            prev.progress == 1.0 and
            prev.chunk_ids):

            # Reuse chunks
            task["chunk_ids"] = prev.chunk_ids
            task["progress"] = 1.0

            # Update document chunk count
            chunk_count = len(prev.chunk_ids.split())
            DocumentService.increment_chunk_num(
                task["doc_id"],
                chunking_config["kb_id"],
                0,  # No new tokens
                chunk_count,
                0   # No duration
            )

            return chunk_count

    return 0
```

### Config Digest Calculation

```python
def calculate_digest(doc: dict, from_page: int, to_page: int) -> str:
    """
    Calculate configuration digest for chunk reuse detection.

    Digest includes:
        - Parser ID
        - Parser config (chunk_token_num, delimiter, etc.)
        - Page range
        - Embedding model ID

    Returns:
        xxhash digest string
    """

    config = {
        "parser_id": doc["parser_id"],
        "parser_config": doc["parser_config"],
        "from_page": from_page,
        "to_page": to_page,
        "embd_id": doc.get("embd_id")
    }

    config_str = json.dumps(config, sort_keys=True)
    return xxhash.xxh64(config_str).hexdigest()
```

## Queue Management

### Redis Queue Structure

```python
# Queue naming
def get_queue_name(tenant_id: str) -> str:
    return f"ragflow:task:{tenant_id}"

# Queue operations
class RedisQueueConnection:
    def queue_product(self, queue_name: str, task: dict):
        """Add task to queue."""
        self.redis.xadd(
            queue_name,
            {"task": json.dumps(task)},
            maxlen=10000
        )

    def queue_consume(self, queue_name: str, consumer_group: str):
        """Consume task from queue."""
        result = self.redis.xreadgroup(
            consumer_group,
            consumer_id,
            {queue_name: ">"},
            count=1,
            block=5000
        )
        return result

    def queue_ack(self, queue_name: str, message_id: str):
        """Acknowledge processed message."""
        self.redis.xack(queue_name, consumer_group, message_id)
```

### Consumer Group Setup

```python
# Create consumer group
REDIS_CONN.redis.xgroup_create(
    queue_name,
    SVR_CONSUMER_GROUP_NAME,
    id="0",
    mkstream=True
)

# Read pending (unacked) messages first
pending = REDIS_CONN.redis.xpending_range(
    queue_name,
    SVR_CONSUMER_GROUP_NAME,
    min="-",
    max="+",
    count=10
)
```

## Progress Synchronization

```python
# In DocumentService._sync_progress()

def _sync_progress(docs: list):
    """
    Synchronize task progress to document level.

    Called periodically to update document status.
    """

    for doc in docs:
        tasks = Task.select().where(Task.doc_id == doc["id"])

        if not tasks:
            continue

        # Calculate aggregate progress
        total_progress = sum(t.progress for t in tasks if t.progress >= 0)
        failed_count = sum(1 for t in tasks if t.progress == -1)

        if failed_count > 0:
            # Any failure = document failed
            doc_progress = -1
            status = "FAIL"
        elif all(t.progress == 1.0 for t in tasks):
            # All complete
            doc_progress = 1.0
            status = "DONE"
        else:
            # In progress
            doc_progress = total_progress / len(tasks)
            status = "RUNNING"

        # Special handling for async tasks
        special_tasks = ["graphrag", "raptor", "mindmap"]
        has_special = any(t.task_type in special_tasks for t in tasks)
        if has_special:
            # Freeze progress while special tasks run
            freeze_progress = True

        # Update document
        DocumentService.update_by_id(doc["id"], {
            "progress": doc_progress,
            "status": status
        })
```

## Task Types

| Type | Purpose | Execution |
|------|---------|-----------|
| `""` (default) | Standard document parsing | Synchronous |
| `graphrag` | Knowledge graph extraction | Async |
| `raptor` | RAPTOR tree building | Async |
| `mindmap` | Mind map generation | Async |
| `dataflow` | Custom pipeline execution | Async |

## Performance Optimizations

### 1. Batch Insert

```python
bulk_insert_into_db(Task, tasks, ignore_conflicts=True)
```

### 2. Page-based Splitting

```python
# PDF split into 12-page chunks
task_page_size = doc["parser_config"].get("task_page_size", 12)
```

### 3. Chunk Reuse

```python
# Skip processing if config unchanged
if prev.digest == task["digest"] and prev.progress == 1.0:
    reuse_chunks(prev, task)
```

### 4. Redis Streams

```python
# Use Redis Streams for reliable queue
# - Message persistence
# - Consumer groups
# - Acknowledgment tracking
```

## Related Files

- `/rag/svr/task_executor.py` - Task execution
- `/api/db/services/document_service.py` - Progress sync
- `/common/connection_utils.py` - Redis connection
