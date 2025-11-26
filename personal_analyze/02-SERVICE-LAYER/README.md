# 02-SERVICE-LAYER - Business Logic Layer

## Tổng Quan

Service Layer là tầng business logic của RAGFlow, xử lý tất cả operations phức tạp và orchestrate các components khác nhau.

## Kiến Trúc Service Layer

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          API LAYER (Blueprints)                          │
└────────────────────────────────┬────────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                          SERVICE LAYER                                   │
│                                                                          │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐         │
│  │ DialogService   │  │ DocumentService │  │ KnowledgebaseS. │         │
│  │                 │  │                 │  │                 │         │
│  │ • chat()        │  │ • insert()      │  │ • create()      │         │
│  │ • ask()         │  │ • remove()      │  │ • update()      │         │
│  │ • gen_mindmap() │  │ • get_list()    │  │ • get_by_id()   │         │
│  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘         │
│           │                    │                    │                   │
│  ┌────────┴────────┐  ┌────────┴────────┐  ┌────────┴────────┐         │
│  │ TaskService     │  │ FileService     │  │ LLMService      │         │
│  │                 │  │                 │  │ (LLMBundle)     │         │
│  │ • queue_tasks() │  │ • upload()      │  │                 │         │
│  │ • get_task()    │  │ • download()    │  │ • encode()      │         │
│  │ • update_prog() │  │ • delete()      │  │ • chat()        │         │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘         │
│                                                                          │
└────────────────────────────────┬────────────────────────────────────────┘
                                 │
          ┌──────────────────────┼──────────────────────┐
          │                      │                      │
          ▼                      ▼                      ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│    DATABASE     │    │   VECTOR STORE  │    │     STORAGE     │
│    (MySQL)      │    │ (Elasticsearch) │    │     (MinIO)     │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## Cấu Trúc Thư Mục

```
/api/db/
├── db_models.py              # Peewee ORM models (54KB)
├── db_utils.py               # Database utilities
├── init_data.py              # Initial data seeding
├── runtime_config.py         # Runtime configuration
│
└── services/
    ├── dialog_service.py     # Chat/RAG service (37KB) ⭐
    ├── document_service.py   # Document management (39KB) ⭐
    ├── knowledgebase_service.py  # KB operations (21KB)
    ├── task_service.py       # Task queue (20KB) ⭐
    ├── file_service.py       # File operations (22KB)
    ├── llm_service.py        # LLM abstraction ⭐
    ├── user_service.py       # User management
    ├── conversation_service.py   # Conversation storage
    ├── canvas_service.py     # Canvas/workflow storage
    ├── connector_service.py  # Data source connectors
    ├── api_service.py        # API token management
    ├── search_service.py     # Search operations
    └── common_service.py     # Base service class
```

## Files Trong Module Này

| File | Mô Tả |
|------|-------|
| [dialog_service_analysis.md](./dialog_service_analysis.md) | **Core RAG Chat** - Retrieval, reranking, generation |
| [document_service_analysis.md](./document_service_analysis.md) | Document lifecycle management |
| [task_service_analysis.md](./task_service_analysis.md) | Background task queue system |
| [llm_service_analysis.md](./llm_service_analysis.md) | LLM abstraction và token tracking |
| [knowledgebase_service_analysis.md](./knowledgebase_service_analysis.md) | Knowledge base operations |

## Core Patterns

### 1. CommonService Base Class

```python
class CommonService:
    """Base class for all services with common CRUD operations."""

    model = None  # Override in subclass

    @classmethod
    @DB.connection_context()
    def query(cls, cols=None, reverse=None, order_by=None, **kwargs):
        """
        Flexible query builder.

        Args:
            cols: Columns to select
            reverse: Reverse sort order
            order_by: Sort field
            **kwargs: Filter conditions

        Returns:
            List of matching records
        """
        query = cls.model.select(*cols) if cols else cls.model.select()

        for k, v in kwargs.items():
            query = query.where(getattr(cls.model, k) == v)

        if order_by:
            query = query.order_by(
                getattr(cls.model, order_by).desc() if reverse
                else getattr(cls.model, order_by)
            )

        return list(query)

    @classmethod
    @DB.connection_context()
    def get_by_id(cls, id):
        """Get record by primary key."""
        try:
            record = cls.model.get_by_id(id)
            return True, record
        except DoesNotExist:
            return False, None

    @classmethod
    @DB.connection_context()
    def save(cls, **kwargs):
        """Insert new record."""
        return cls.model.create(**kwargs)

    @classmethod
    @DB.connection_context()
    def update_by_id(cls, id, data):
        """Update record by ID."""
        data["update_time"] = int(time.time() * 1000)
        data["update_date"] = datetime.now()
        return cls.model.update(data).where(cls.model.id == id).execute()
```

### 2. Transaction Handling

```python
# Atomic operations
with DB.atomic():
    for item in items:
        cls.model.update(data).where(...).execute()

# Connection context for query isolation
@DB.connection_context()
def critical_operation():
    # Automatic connection management
    pass

# Database locking for critical sections
with DB.lock("operation_name", timeout=60):
    # Only one process can execute this
    pass
```

### 3. Service-to-Service Communication

```python
# Services call other services
class DialogService:
    @classmethod
    def chat(cls, dialog, messages, **kwargs):
        # Get knowledge bases
        kbs = KnowledgebaseService.get_by_ids(dialog.kb_ids)

        # Get embedding model
        embd_mdl = LLMBundle(tenant_id, LLMType.EMBEDDING, kb.embd_id)

        # Retrieve documents
        kbinfos = retriever.retrieval(query, embd_mdl, ...)

        # Generate response
        for token in chat_mdl.chat_streamly(...):
            yield token
```

## Database Models Overview

### Core Models

```python
# User & Multi-tenancy
User          # id, email, password, access_token
Tenant        # id, name, llm_id, embd_id
UserTenant    # user_id, tenant_id, role

# Knowledge Management
Knowledgebase # id, tenant_id, name, embd_id, parser_config
Document      # id, kb_id, name, status, progress, chunk_num
File          # id, tenant_id, name, location, type
File2Document # file_id, document_id

# Chat & Dialog
Dialog        # id, tenant_id, kb_ids, llm_id, prompt_config
Conversation  # id, dialog_id, message (JSON array)

# Task Queue
Task          # id, doc_id, progress, chunk_ids

# API Integration
APIToken      # id, tenant_id, token, dialog_id
```

### JSON Fields Usage

```python
# Parser configuration
Document.parser_config = {
    "chunk_token_num": 512,
    "delimiter": "\n。；！？",
    "layout_recognize": "DeepDOC"
}

# LLM settings
Dialog.llm_setting = {
    "temperature": 0.7,
    "max_tokens": 2048,
    "top_p": 1.0
}

# Prompt configuration
Dialog.prompt_config = {
    "system": "You are a helpful assistant...",
    "prologue": "Hi! How can I help?",
    "quote": True,
    "reasoning": False
}
```

## Key Service Interactions

```
┌─────────────────────────────────────────────────────────────────────────┐
│                     SERVICE INTERACTION FLOW                             │
└─────────────────────────────────────────────────────────────────────────┘

[Document Upload]
    API
     │
     ├──► FileService.upload_document()
     │       │
     │       ├──► Store file in MinIO
     │       ├──► Create File record
     │       └──► Create Document record
     │
     └──► TaskService.queue_tasks()
             │
             └──► Create Task records
             └──► Push to Redis queue

[Document Processing]
    TaskExecutor (Background)
     │
     ├──► TaskService.get_task()
     ├──► DocumentService.get_by_id()
     ├──► Parse & chunk document
     ├──► LLMBundle.encode() → Generate embeddings
     ├──► Store chunks in Elasticsearch
     ├──► TaskService.update_progress()
     └──► DocumentService.increment_chunk_num()

[Chat/RAG]
    API
     │
     └──► DialogService.chat()
             │
             ├──► KnowledgebaseService.get_by_ids()
             ├──► LLMBundle (embedding) → Query vector
             ├──► retriever.retrieval() → Hybrid search
             ├──► LLMBundle (rerank) → Rerank results
             ├──► LLMBundle (chat) → Generate response
             └──► ConversationService.save()
```

## Performance Patterns

### 1. Batch Operations

```python
def bulk_create_chunks(chunks: List[dict]):
    """Bulk insert for efficiency."""
    with db.atomic():
        for batch in chunked(chunks, 1000):
            Chunk.insert_many(batch).execute()
```

### 2. Connection Pooling

```python
db = PooledMySQLDatabase(
    database,
    max_connections=32,
    stale_timeout=300,
    **connection_params
)
```

### 3. Caching Strategies

```python
# Metadata caching for filtering
@cache_result(ttl=600)
def get_meta_by_kbs(kb_ids):
    """Cache metadata index for 10 minutes."""
    return DocumentService.get_meta_by_kbs(kb_ids)
```

### 4. Token Tracking

```python
class LLMBundle:
    def encode(self, texts):
        embeddings, tokens = self.mdl.encode(texts)
        # Track token usage
        TenantLLMService.increase_usage(
            self.tenant_id,
            LLMType.EMBEDDING,
            tokens
        )
        return embeddings
```

## Error Handling

```python
class ServiceException(Exception):
    """Base exception for service errors."""
    pass

class DocumentNotFoundError(ServiceException):
    pass

class InsufficientQuotaError(ServiceException):
    pass

# Usage
try:
    result = DocumentService.get_by_id(doc_id)
    if not result[0]:
        raise DocumentNotFoundError(f"Document {doc_id} not found")
except DocumentNotFoundError as e:
    return get_json_result(code=404, message=str(e))
```

## Related Files

- `/api/db/db_models.py` - All database models
- `/rag/llm/*.py` - LLM implementations
- `/rag/nlp/search.py` - Search/retrieval logic
