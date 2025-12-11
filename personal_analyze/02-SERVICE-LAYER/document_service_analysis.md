# Document Service Analysis - Document Lifecycle Management

## Tổng Quan

`document_service.py` (39KB) quản lý toàn bộ **document lifecycle** từ upload đến deletion, bao gồm parsing, chunk management, và progress tracking.

## File Location
```
/api/db/services/document_service.py
```

## Class Definition

```python
class DocumentService(CommonService):
    model = Document  # Line 46
```

Kế thừa `CommonService` với các method cơ bản: `query()`, `get_by_id()`, `save()`, `update_by_id()`, `delete_by_id()`

---

## Document Lifecycle Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        DOCUMENT LIFECYCLE                                    │
└─────────────────────────────────────────────────────────────────────────────┘

[1] UPLOAD PHASE
    │
    ├──► FileService.upload_document()
    │       ├── Store file in MinIO
    │       ├── Create File record
    │       └── Create Document record
    │
    └──► DocumentService.insert(doc)
            ├── Save Document to MySQL
            └── KnowledgebaseService.atomic_increase_doc_num_by_id()
    │
    ▼
[2] QUEUE PHASE
    │
    └──► DocumentService.run(tenant_id, doc)
            │
            ├─(pipeline_id)──► TaskService.queue_dataflow()
            │                   └── Canvas workflow execution
            │
            └─(standard)─────► TaskService.queue_tasks()
                                └── Push to Redis queue
    │
    ▼
[3] PROCESSING PHASE (Background)
    │
    ├──► TaskExecutor picks task from queue
    ├──► Parse document (deepdoc parsers)
    ├──► Generate chunks
    ├──► LLMBundle.encode() → Embeddings
    ├──► Store in Elasticsearch/Infinity
    │
    └──► DocumentService.increment_chunk_num()
            ├── Document.chunk_num += count
            ├── Document.token_num += count
            └── Knowledgebase.chunk_num += count
    │
    ▼
[4] STATUS SYNC
    │
    └──► DocumentService._sync_progress()
            ├── Aggregate task progress
            ├── Update Document.progress
            └── Set run status (DONE/FAIL/RUNNING)
    │
    ▼
[5] QUERY/RETRIEVAL
    │
    ├──► DocumentService.get_by_kb_id()
    └──► docStoreConn.search() → Return chunks for RAG
    │
    ▼
[6] DELETION
    │
    └──► DocumentService.remove_document()
            ├── clear_chunk_num() → Reset KB stats
            ├── TaskService.filter_delete() → Remove tasks
            ├── docStoreConn.delete() → Remove from index
            ├── STORAGE_IMPL.rm() → Delete files
            └── delete_by_id() → Remove DB record
```

---

## Core Methods

### 1. Insert Document

**Lines**: 292-297

```python
@classmethod
@DB.connection_context()
def insert(cls, doc):
    """
    Insert document and increment KB doc count atomically.

    Args:
        doc: Document dict with keys: id, kb_id, name, parser_id, etc.

    Returns:
        Document instance

    Raises:
        RuntimeError: If database operation fails
    """
    if not cls.save(**doc):
        raise RuntimeError("Database error (Document)!")

    # Atomic increment KB document count
    if not KnowledgebaseService.atomic_increase_doc_num_by_id(doc["kb_id"]):
        raise RuntimeError("Database error (Knowledgebase)!")

    return Document(**doc)
```

**Flow**:
1. Save document record to MySQL
2. Atomically increment `Knowledgebase.doc_num`
3. Return Document instance

---

### 2. Remove Document

**Lines**: 301-340

```python
@classmethod
@DB.connection_context()
def remove_document(cls, doc, tenant_id):
    """
    Remove document with full cascade cleanup.

    Cleanup order:
    1. Reset KB statistics (chunk_num, token_num, doc_num)
    2. Delete associated tasks
    3. Retrieve all chunk IDs (paginated)
    4. Delete chunk files from storage (MinIO)
    5. Delete thumbnail if exists
    6. Delete from document store (Elasticsearch)
    7. Clean up knowledge graph references
    8. Delete document record from MySQL
    """
```

**Cascade Cleanup Diagram**:

```
remove_document(doc, tenant_id)
         │
         ├──► clear_chunk_num(doc.id)
         │       └── KB: -chunk_num, -token_num, -doc_num
         │
         ├──► TaskService.filter_delete([Task.doc_id == doc.id])
         │
         ├──► Retrieve chunk IDs (paginated, 1000/page)
         │       for page in range(∞):
         │           chunks = docStoreConn.search(...)
         │           chunk_ids.extend(get_chunk_ids(chunks))
         │           if empty: break
         │
         ├──► Delete chunk files from storage
         │       for cid in chunk_ids:
         │           STORAGE_IMPL.rm(doc.kb_id, cid)
         │
         ├──► Delete thumbnail (if not base64)
         │       STORAGE_IMPL.rm(doc.kb_id, doc.thumbnail)
         │
         ├──► docStoreConn.delete({"doc_id": doc.id}, ...)
         │
         ├──► Clean knowledge graph (if exists)
         │       └── Remove doc.id from graph source_id references
         │
         └──► cls.delete_by_id(doc.id)
```

---

### 3. Run Document Processing

**Lines**: 822-841

```python
@classmethod
def run(cls, tenant_id: str, doc: dict, kb_table_num_map: dict):
    """
    Route document to appropriate processing pipeline.

    Two paths:
    1. Pipeline mode (canvas workflow): queue_dataflow()
    2. Standard mode: queue_tasks()
    """
    from api.db.services.task_service import queue_dataflow, queue_tasks

    doc["tenant_id"] = tenant_id
    doc_parser = doc.get("parser_id", ParserType.NAIVE)

    # Special handling for TABLE parser
    if doc_parser == ParserType.TABLE:
        kb_id = doc.get("kb_id")
        if kb_id not in kb_table_num_map:
            count = DocumentService.count_by_kb_id(kb_id=kb_id, ...)
            kb_table_num_map[kb_id] = count
            if kb_table_num_map[kb_id] <= 0:
                KnowledgebaseService.delete_field_map(kb_id)

    # Route to processing
    if doc.get("pipeline_id", ""):
        queue_dataflow(tenant_id, flow_id=doc["pipeline_id"],
                      task_id=get_uuid(), doc_id=doc["id"])
    else:
        bucket, name = File2DocumentService.get_storage_address(doc_id=doc["id"])
        queue_tasks(doc, bucket, name, 0)
```

**Routing Logic**:

```
doc.run()
    │
    ├─── Has pipeline_id? ────► queue_dataflow()
    │         │                      │
    │         │                      └── Execute canvas workflow
    │         │
    │         No
    │         │
    │         ▼
    │    Get file storage address
    │         │
    │         ▼
    └──────► queue_tasks()
                  │
                  └── Push to Redis queue for TaskExecutor
```

---

### 4. Chunk Number Management

**Lines**: 390-455

```python
# INCREMENT (after parsing completes)
@classmethod
@DB.connection_context()
def increment_chunk_num(cls, doc_id, kb_id, token_num, chunk_num, duration):
    """
    Updates:
    - Document.chunk_num += chunk_num
    - Document.token_num += token_num
    - Document.process_duration += duration
    - Knowledgebase.chunk_num += chunk_num
    - Knowledgebase.token_num += token_num
    """

# DECREMENT (on reprocessing)
@classmethod
@DB.connection_context()
def decrement_chunk_num(cls, doc_id, kb_id, token_num, chunk_num, duration):
    """Reverse of increment_chunk_num"""

# CLEAR (on deletion)
@classmethod
@DB.connection_context()
def clear_chunk_num(cls, doc_id):
    """
    Updates:
    - KB.chunk_num -= doc.chunk_num
    - KB.token_num -= doc.token_num
    - KB.doc_num -= 1
    - Document: reset chunk_num=0, token_num=0
    """

# CLEAR ON RERUN (keeps doc_num)
@classmethod
@DB.connection_context()
def clear_chunk_num_when_rerun(cls, doc_id):
    """Same as clear_chunk_num but KB.doc_num unchanged"""
```

---

### 5. Progress Synchronization

**Lines**: 682-738

```python
@classmethod
def _sync_progress(cls, docs):
    """
    Aggregate task progress → document progress.

    State Machine:
    - ALL tasks done + NO failures → progress=1, status=DONE
    - ALL tasks done + ANY failure → progress=-1, status=FAIL
    - Any task running → progress=avg(task_progress), status=RUNNING
    """
```

**Progress State Machine**:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    PROGRESS STATE MACHINE                                    │
└─────────────────────────────────────────────────────────────────────────────┘

                    ┌─────────────────────┐
                    │   Aggregate Tasks   │
                    │   progress values   │
                    └──────────┬──────────┘
                               │
         ┌─────────────────────┼─────────────────────┐
         │                     │                     │
         ▼                     ▼                     ▼
   ALL DONE (prg=1)      ANY FAILED         IN PROGRESS
   No failures           (any task=-1)      (0 ≤ prg < 1)
         │                     │                     │
         ▼                     ▼                     ▼
   ┌─────────────┐       ┌─────────────┐       ┌─────────────┐
   │ progress=1  │       │ progress=-1 │       │ progress=   │
   │ run=DONE    │       │ run=FAIL    │       │   avg(tasks)│
   │             │       │             │       │ run=RUNNING │
   └─────────────┘       └─────────────┘       └─────────────┘

Progress Calculation:
    prg = sum(task.progress for task if task.progress >= 0) / len(tasks)
```

---

### 6. Get Documents by KB

**Lines**: 125-163

```python
@classmethod
@DB.connection_context()
def get_by_kb_id(cls, kb_id, page_number, items_per_page, orderby, desc,
                 keywords, run_status, types, suffix):
    """
    Advanced query with multiple filters and joins.

    Joins:
    - File2Document → File (for location, size)
    - UserCanvas (LEFT) → for pipeline info
    - User (LEFT) → for creator info

    Filters:
    - kb_id: Required
    - keywords: Search in doc name
    - run_status: [RUNNING, DONE, FAIL, CANCEL]
    - types: Document types
    - suffix: File extensions

    Returns:
        (list[dict], total_count)
    """
```

**Query Structure**:

```sql
SELECT
    document.id, thumbnail, kb_id, parser_id, pipeline_id,
    parser_config, source_type, type, created_by, name,
    location, size, token_num, chunk_num, progress,
    progress_msg, process_begin_at, process_duration,
    meta_fields, suffix, run, status,
    create_time, create_date, update_time, update_date,
    user_canvas.title AS pipeline_name,
    user.nickname
FROM document
JOIN file2document ON document.id = file2document.document_id
JOIN file ON file2document.file_id = file.id
LEFT JOIN user_canvas ON document.pipeline_id = user_canvas.id
LEFT JOIN user ON document.created_by = user.id
WHERE
    document.kb_id = ?
    AND document.status = '1'
    AND (document.name LIKE '%keyword%' OR ...)
    AND document.run IN (?, ?, ...)
    AND document.type IN (?, ?, ...)
    AND file.suffix IN (?, ?, ...)
ORDER BY ? DESC/ASC
LIMIT ? OFFSET ?
```

---

### 7. Full Parse Workflow (`doc_upload_and_parse`)

**Lines**: 889-1030 (module-level function)

```python
def doc_upload_and_parse(conversation_id, file_objs, user_id):
    """
    Complete document upload and parse workflow for chat context.

    Used by: Conversation-based document uploads

    Steps:
    1. Resolve conversation → dialog → KB
    2. Initialize embedding model
    3. Upload files
    4. Parallel parsing (12 workers)
    5. Mind map generation (async)
    6. Embedding (batch 16)
    7. Bulk insert to docStore (batch 64)
    8. Update statistics
    """
```

**Detailed Flow**:

```
doc_upload_and_parse(conversation_id, file_objs, user_id)
         │
         ├──► ConversationService.get_by_id(conversation_id)
         │         └── Get conversation → dialog_id
         │
         ├──► DialogService.get_by_id(dialog_id)
         │         └── Get dialog → kb_ids[0]
         │
         ├──► KnowledgebaseService.get_by_id(kb_id)
         │         └── Get KB → tenant_id, embd_id
         │
         ├──► LLMBundle(tenant_id, EMBEDDING, embd_id)
         │         └── Initialize embedding model
         │
         ├──► FileService.upload_document(kb, file_objs, user_id)
         │         └── Returns: [(doc_dict, file_bytes), ...]
         │
         ├──► ThreadPoolExecutor(max_workers=12)
         │         │
         │         └── for (doc, blob) in files:
         │               executor.submit(parser.chunk, doc["name"], blob, **kwargs)
         │
         ├──► For each parsed document:
         │         │
         │         ├── MindMapExtractor(llm) → Generate mind map
         │         │         └── trio.run(mindmap, chunk_contents)
         │         │
         │         ├── Embedding (batch=16)
         │         │         └── vectors = embedding(doc_id, contents)
         │         │
         │         ├── Add vectors to chunks
         │         │         └── chunk["q_{dim}_vec"] = vector
         │         │
         │         ├── Bulk insert (batch=64)
         │         │         └── docStoreConn.insert(chunks[b:b+64], idxnm, kb_id)
         │         │
         │         └── Update stats
         │               └── increment_chunk_num(doc_id, kb_id, tokens, chunks, 0)
         │
         └──► Return [doc_id, ...]
```

---

## Document Status Fields

```python
# From Document model (db_models.py)

run: CharField(max_length=1)
    # "0" = UNSTART (default)
    # "1" = RUNNING
    # "2" = CANCEL

status: CharField(max_length=1)
    # "0" = WASTED (soft deleted)
    # "1" = VALID (default)

progress: FloatField
    # 0.0 = Not started
    # 0.0-1.0 = In progress
    # 1.0 = Done
    # -1.0 = Failed

progress_msg: TextField
    # Human-readable status message
    # e.g., "Parsing...", "Embedding...", "Done"

process_begin_at: DateTimeField
    # When parsing started

process_duration: FloatField
    # Cumulative processing time (seconds)
```

---

## Service Interactions

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    SERVICE INTERACTION DIAGRAM                               │
└─────────────────────────────────────────────────────────────────────────────┘

                          DocumentService
                                │
        ┌───────────────────────┼───────────────────────┐
        │                       │                       │
        ▼                       ▼                       ▼
┌───────────────┐      ┌───────────────┐      ┌───────────────┐
│ Knowledgebase │      │   Task        │      │ File2Document │
│   Service     │      │   Service     │      │   Service     │
│               │      │               │      │               │
│ • atomic_     │      │ • queue_tasks │      │ • get_storage │
│   increase_   │      │ • queue_      │      │   _address    │
│   doc_num     │      │   dataflow    │      │ • get_by_     │
│ • delete_     │      │ • filter_     │      │   document_id │
│   field_map   │      │   delete      │      │               │
└───────────────┘      └───────────────┘      └───────────────┘
                                │
                                ▼
                       ┌───────────────┐
                       │  FileService  │
                       │               │
                       │ • upload_     │
                       │   document    │
                       └───────────────┘

External Systems:
┌───────────────┐      ┌───────────────┐      ┌───────────────┐
│  docStoreConn │      │ STORAGE_IMPL  │      │  REDIS_CONN   │
│ (Elasticsearch│      │   (MinIO)     │      │  (Queue)      │
│  /Infinity)   │      │               │      │               │
│               │      │               │      │               │
│ • search      │      │ • obj_exist   │      │ • queue_      │
│ • insert      │      │ • rm          │      │   product     │
│ • delete      │      │ • put         │      │ • queue_info  │
│ • createIdx   │      │               │      │               │
└───────────────┘      └───────────────┘      └───────────────┘
```

---

## Key Method Reference Table

| Category | Method | Lines | Purpose |
|----------|--------|-------|---------|
| **Query** | `get_list` | 81-110 | Paginated list with filters |
| **Query** | `get_by_kb_id` | 125-163 | Advanced query with joins |
| **Query** | `get_filter_by_kb_id` | 167-212 | Aggregated filter counts |
| **Query** | `get_chunking_config` | 542-563 | Config for parsing |
| **Insert** | `insert` | 292-297 | Add doc + increment KB |
| **Delete** | `remove_document` | 301-340 | Cascade cleanup |
| **Parse** | `run` | 822-841 | Route to processing |
| **Parse** | `doc_upload_and_parse` | 889-1030 | Full workflow |
| **Status** | `begin2parse` | 627-637 | Set running status |
| **Status** | `_sync_progress` | 682-738 | Aggregate task→doc |
| **Status** | `update_progress` | 665-668 | Batch sync unfinished |
| **Chunks** | `increment_chunk_num` | 390-403 | Add chunks |
| **Chunks** | `decrement_chunk_num` | 407-422 | Remove chunks |
| **Chunks** | `clear_chunk_num` | 426-438 | Reset on delete |
| **Config** | `update_parser_config` | 594-615 | Deep merge config |
| **Access** | `accessible` | 495-505 | User permission check |
| **Access** | `accessible4deletion` | 509-525 | Delete permission |
| **Stats** | `knowledgebase_basic_info` | 767-819 | KB statistics |

---

## Error Handling

| Location | Error Type | Handling |
|----------|-----------|----------|
| `insert()` | RuntimeError | Raised - transaction fails |
| `remove_document()` | Any exception | Caught + pass (silent) |
| `_sync_progress()` | Exception | Logged, continues others |
| `check_doc_health()` | RuntimeError | Raised - upload rejected |
| `update_parser_config()` | LookupError | Raised - update fails |

---

## Performance Patterns

### Batch Operations

| Operation | Batch Size | Purpose |
|-----------|-----------|---------|
| Chunk retrieval | 1000 | Memory efficient deletion |
| Bulk insert | 64 | Batch vector storage |
| Embedding | 16 | LLM batch inference |
| Parallel parsing | 12 workers | Concurrent processing |
| Doc ID retrieval | 100 | Paginated queries |

### Parallel Processing

```python
# ThreadPoolExecutor for parsing
exe = ThreadPoolExecutor(max_workers=12)
for (doc, blob) in files:
    threads.append(exe.submit(parser.chunk, doc["name"], blob, **kwargs))

# Async mind map extraction
trio.run(mindmap_extractor, chunk_contents)
```
