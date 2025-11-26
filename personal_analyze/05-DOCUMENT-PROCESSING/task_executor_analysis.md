# Task Executor Analysis

## Tong Quan

Task executor là main orchestration engine xử lý documents asynchronously với queue-based processing.

## File Location
```
/rag/svr/task_executor.py
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    TASK EXECUTOR ARCHITECTURE                    │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                      Main Event Loop (trio)                      │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  report_status() - Heartbeat (30s interval)              │   │
│  │  - Update server status                                  │   │
│  │  - Cleanup stale tasks                                   │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Task Manager Loop                                       │   │
│  │  ├── collect() - Get task from Redis queue              │   │
│  │  ├── do_handle_task() - Process with semaphore          │   │
│  │  │   ├── build_chunks()                                 │   │
│  │  │   ├── embedding()                                    │   │
│  │  │   └── insert_es()                                    │   │
│  │  └── handle_task() - ACK and error handling             │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## Main Entry Point

```python
async def main():
    """Main entry point for task executor."""

    # Initialize connections
    init_db_connection()
    init_es_connection()
    init_minio_connection()

    # Start concurrent tasks
    async with trio.open_nursery() as nursery:
        # Heartbeat reporter
        nursery.start_soon(report_status)

        # Task processing loop
        nursery.start_soon(task_loop)

async def task_loop():
    """Main task processing loop."""
    while True:
        try:
            # Get task from queue
            task = await collect()

            if task:
                # Process with concurrency limit
                async with semaphore:
                    await do_handle_task(task)
        except Exception as e:
            logging.exception(e)
            await trio.sleep(1)
```

## Task Collection

```python
async def collect():
    """
    Collect task from Redis queue.

    Returns:
        Task dict or None if no tasks available
    """
    # Try to get from queue
    result = REDIS_CONN.queue_consume(
        queue_name=get_queue_name(),
        consumer_group=SVR_CONSUMER_GROUP_NAME,
        block=5000  # 5 second timeout
    )

    if not result:
        return None

    # Parse task
    message_id, task_data = result
    task = json.loads(task_data["task"])

    # Get full task context
    task_info = TaskService.get_task(task["id"])

    if not task_info:
        # Task canceled or max retries exceeded
        REDIS_CONN.queue_ack(queue_name, message_id)
        return None

    task_info["message_id"] = message_id
    return task_info
```

## Task Handling

```python
async def do_handle_task(task):
    """
    Main task processing logic.

    Steps:
    1. Download file from MinIO
    2. Build chunks (parse + chunk + enrich)
    3. Generate embeddings
    4. Index in Elasticsearch
    """

    doc_id = task["doc_id"]
    task_id = task["id"]

    try:
        # Update progress: Starting
        TaskService.update_progress(task_id, {
            "progress": 0.1,
            "progress_msg": "Starting document processing..."
        })

        # 1. Download file
        file_blob = await download_from_minio(task)

        # 2. Build chunks
        chunks = await build_chunks(task, file_blob)

        if not chunks:
            TaskService.update_progress(task_id, {
                "progress": -1,
                "progress_msg": "No content extracted"
            })
            return

        # 3. Generate embeddings
        chunks = await embedding(chunks, task)

        # 4. Index in Elasticsearch
        await insert_es(chunks, task)

        # 5. Update success
        TaskService.update_progress(task_id, {
            "progress": 1.0,
            "progress_msg": f"Completed. {len(chunks)} chunks created.",
            "chunk_ids": " ".join([c["id"] for c in chunks])
        })

    except Exception as e:
        logging.exception(e)
        TaskService.update_progress(task_id, {
            "progress": -1,
            "progress_msg": str(e)
        })

async def handle_task(task, result):
    """
    Post-processing: ACK queue and cleanup.
    """
    REDIS_CONN.queue_ack(
        get_queue_name(),
        task["message_id"]
    )
```

## Chunk Building

```python
async def build_chunks(task, file_blob):
    """
    Build chunks from document.

    Process:
    1. Select parser based on file type
    2. Parse document
    3. Chunk content
    4. Enrich chunks (keywords, questions)
    """

    file_name = task["name"]
    parser_id = task["parser_id"]
    parser_config = task["parser_config"]

    # Select parser
    if file_name.endswith(".pdf"):
        if parser_config.get("layout_recognize") == "DeepDOC":
            parser = RAGFlowPdfParser()
        elif parser_config.get("layout_recognize") == "Plain":
            parser = PlainParser()
        else:
            parser = VisionParser()

    elif file_name.endswith(".docx"):
        parser = DocxParser()

    elif file_name.endswith(".xlsx"):
        parser = ExcelParser()

    else:
        parser = TextParser()

    # Parse document
    sections = parser.parse(
        file_blob,
        from_page=task.get("from_page", 0),
        to_page=task.get("to_page", -1),
        callback=lambda p, m: TaskService.update_progress(task["id"], {
            "progress": p,
            "progress_msg": m
        })
    )

    # Chunk content
    chunks = naive_merge(
        sections,
        chunk_token_num=parser_config.get("chunk_token_num", 512),
        delimiter=parser_config.get("delimiter", "\n。；！？"),
        overlapped_percent=parser_config.get("overlapped_percent", 0)
    )

    # Build chunk records
    chunk_records = []
    for i, (content, positions) in enumerate(chunks):
        chunk_id = xxhash.xxh64(content + task["doc_id"]).hexdigest()

        chunk_records.append({
            "id": chunk_id,
            "doc_id": task["doc_id"],
            "kb_id": task["kb_id"],
            "content_with_weight": content,
            "docnm_kwd": task["name"],
            "page_num_int": extract_page_nums(positions),
            "position_int": encode_positions(positions),
            "create_time": datetime.now().isoformat(),
        })

    # Enrich chunks
    if parser_config.get("auto_keywords"):
        await add_keywords(chunk_records, task)

    if parser_config.get("auto_questions"):
        await add_questions(chunk_records, task)

    return chunk_records
```

## Embedding Generation

```python
async def embedding(chunks, task):
    """
    Generate embeddings for chunks.
    """
    embd_mdl = LLMBundle(
        task["tenant_id"],
        LLMType.EMBEDDING,
        task.get("embd_id")
    )

    batch_size = 16
    total_tokens = 0

    for i in range(0, len(chunks), batch_size):
        batch = chunks[i:i+batch_size]

        # Prepare texts
        texts = [c["content_with_weight"] for c in batch]

        # Generate embeddings
        embeddings, tokens = embd_mdl.encode(texts)
        total_tokens += tokens

        # Store vectors
        for j, emb in enumerate(embeddings):
            chunk_idx = i + j
            vec_field = f"q_{len(emb)}_vec"
            chunks[chunk_idx][vec_field] = emb.tolist()

        # Update progress
        progress = 0.7 + 0.2 * (i / len(chunks))
        TaskService.update_progress(task["id"], {
            "progress": progress,
            "progress_msg": f"Embedding {i+len(batch)}/{len(chunks)} chunks"
        })

    return chunks
```

## Elasticsearch Indexing

```python
async def insert_es(chunks, task):
    """
    Bulk insert chunks to Elasticsearch.
    """
    es = get_es_connection()
    index_name = f"ragflow_{task['kb_id']}"

    # Ensure index exists
    if not es.indices.exists(index=index_name):
        es.indices.create(index=index_name, body=ES_MAPPING)

    # Bulk insert
    bulk_size = 64
    for i in range(0, len(chunks), bulk_size):
        batch = chunks[i:i+bulk_size]

        actions = []
        for chunk in batch:
            actions.append({
                "_index": index_name,
                "_id": chunk["id"],
                "_source": chunk
            })

        helpers.bulk(es, actions)

        # Update progress
        progress = 0.9 + 0.1 * (i / len(chunks))
        TaskService.update_progress(task["id"], {
            "progress": progress,
            "progress_msg": f"Indexing {i+len(batch)}/{len(chunks)} chunks"
        })
```

## Concurrency Control

```python
# Global semaphores
task_semaphore = trio.Semaphore(MAX_CONCURRENT_TASKS)  # 5
chunk_semaphore = trio.Semaphore(MAX_CONCURRENT_CHUNK_BUILDERS)  # 1
minio_semaphore = trio.Semaphore(MAX_CONCURRENT_MINIO)  # 10

async def do_handle_task(task):
    async with task_semaphore:
        # ... processing

async def build_chunks(task, blob):
    async with chunk_semaphore:
        # ... chunk building

async def download_from_minio(task):
    async with minio_semaphore:
        # ... download
```

## Progress Tracking

```python
# Progress stages:
# 0.0 - 0.1: Starting
# 0.1 - 0.4: Image extraction (PDF)
# 0.4 - 0.6: OCR
# 0.6 - 0.7: Layout + text merge
# 0.7 - 0.9: Embedding
# 0.9 - 1.0: Indexing

def update_progress(task_id, info):
    """
    Thread-safe progress update.

    Rules:
    - progress_msg: Always append
    - progress: Only update if new > current (or -1 for failure)
    """
    # ... implementation
```

## Task Types

```python
TASK_TYPES = {
    "": "standard",          # Standard document parsing
    "graphrag": "graphrag",  # Knowledge graph extraction
    "raptor": "raptor",      # RAPTOR tree building
    "mindmap": "mindmap",    # Mind map generation
    "dataflow": "dataflow",  # Custom pipeline
}

async def do_handle_task(task):
    task_type = task.get("task_type", "")

    if task_type == "graphrag":
        await handle_graphrag_task(task)
    elif task_type == "raptor":
        await handle_raptor_task(task)
    else:
        await handle_standard_task(task)
```

## Configuration

```python
# Environment variables
MAX_CONCURRENT_TASKS = int(os.environ.get("MAX_CONCURRENT_TASKS", 5))
MAX_CONCURRENT_CHUNK_BUILDERS = int(os.environ.get("MAX_CONCURRENT_CHUNK_BUILDERS", 1))
MAX_CONCURRENT_MINIO = int(os.environ.get("MAX_CONCURRENT_MINIO", 10))

DOC_MAXIMUM_SIZE = 100 * 1024 * 1024  # 100MB
DOC_BULK_SIZE = 64
EMBEDDING_BATCH_SIZE = 16
```

## Related Files

- `/rag/svr/task_executor.py` - Main executor
- `/api/db/services/task_service.py` - Task management
- `/rag/app/naive.py` - Document parsing
- `/rag/nlp/__init__.py` - Chunking
