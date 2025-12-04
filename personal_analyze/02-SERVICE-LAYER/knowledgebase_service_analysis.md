# Knowledgebase Service Analysis - Dataset Management & Access Control

## Tổng Quan

`knowledgebase_service.py` (566 lines) quản lý **Dataset (Knowledgebase)** - đơn vị tổ chức tài liệu trong RAGFlow, bao gồm CRUD operations, access control, parser configuration, và document association tracking.

## File Location
```
/api/db/services/knowledgebase_service.py
```

## Class Definition

```python
class KnowledgebaseService(CommonService):
    model = Knowledgebase  # Line 49
```

Kế thừa `CommonService` với các method cơ bản: `query()`, `get_by_id()`, `save()`, `update_by_id()`, `delete_by_id()`

---

## Knowledgebase Model Structure

```python
# From db_models.py (Lines 734-753)

class Knowledgebase(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    avatar = TextField(null=True)                    # KB avatar (base64)
    tenant_id = CharField(max_length=32, index=True) # Owner tenant
    name = CharField(max_length=128, index=True)     # KB name
    language = CharField(max_length=32)              # "English"|"Chinese"
    description = TextField(null=True)               # KB description
    embd_id = CharField(max_length=128)              # Embedding model ID
    permission = CharField(max_length=16)            # "me"|"team"
    created_by = CharField(max_length=32)            # Creator user ID

    # Statistics
    doc_num = IntegerField(default=0)                # Document count
    token_num = IntegerField(default=0)              # Total tokens
    chunk_num = IntegerField(default=0)              # Total chunks

    # Search config
    similarity_threshold = FloatField(default=0.2)
    vector_similarity_weight = FloatField(default=0.3)

    # Parser config
    parser_id = CharField(default="naive")           # Default parser
    pipeline_id = CharField(null=True)               # Pipeline workflow ID
    parser_config = JSONField(default={"pages": [[1, 1000000]]})
    pagerank = IntegerField(default=0)
```

---

## Permission Model

### Dual-Level Access Control

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         PERMISSION MODEL                                     │
└─────────────────────────────────────────────────────────────────────────────┘

Level 1: Knowledgebase.permission
    │
    ├─── "me"  ───► Only owner (created_by) can access
    │
    └─── "team" ──► All users in owner's tenant can access

Level 2: UserTenant relationship
    │
    └─── User must belong to KB's tenant to access

Combined Check (get_by_tenant_ids):
┌──────────────────────────────────────────────────────────────────┐
│  ((tenant_id IN joined_tenants) AND (permission == "team"))      │
│                          OR                                       │
│  (tenant_id == user_id)                                          │
└──────────────────────────────────────────────────────────────────┘
```

### Permission Methods

| Method | Lines | Purpose |
|--------|-------|---------|
| `accessible` | 471-486 | Check if user can VIEW KB |
| `accessible4deletion` | 53-83 | Check if user can DELETE KB |
| `get_by_tenant_ids` | 134-197 | Get KBs with permission filter |
| `get_kb_by_id` | 488-500 | Get KB by ID + user permission |

---

## Core Methods

### 1. Create Knowledgebase

**Lines**: 374-429

```python
@classmethod
@DB.connection_context()
def create_with_name(
    cls,
    *,
    name: str,
    tenant_id: str,
    parser_id: str | None = None,
    **kwargs
):
    """
    Create a dataset with validation and defaults.

    Validation Steps:
    1. Name must be string
    2. Name cannot be empty
    3. Name cannot exceed DATASET_NAME_LIMIT bytes (UTF-8)
    4. Deduplicate name within tenant (append _1, _2, etc.)
    5. Verify tenant exists

    Returns:
        (True, payload_dict) on success
        (False, error_result) on failure
    """
```

**Creation Flow**:

```
create_with_name(name, tenant_id, ...)
         │
         ├──► Validate name type
         │       └── Must be string
         │
         ├──► Validate name content
         │       ├── Strip whitespace
         │       ├── Check not empty
         │       └── Check UTF-8 byte length
         │
         ├──► duplicate_name(query, name, tenant_id, status)
         │       └── Returns unique name: "name", "name_1", "name_2"...
         │
         ├──► TenantService.get_by_id(tenant_id)
         │       └── Verify tenant exists
         │
         └──► Build payload dict
                 ├── id: get_uuid()
                 ├── name: deduplicated_name
                 ├── tenant_id: tenant_id
                 ├── created_by: tenant_id
                 ├── parser_id: parser_id or "naive"
                 └── parser_config: get_parser_config(parser_id, config)
```

---

### 2. Get Knowledgebases by Tenant

**Lines**: 134-197

```python
@classmethod
@DB.connection_context()
def get_by_tenant_ids(cls, joined_tenant_ids, user_id,
                      page_number, items_per_page,
                      orderby, desc, keywords,
                      parser_id=None):
    """
    Get knowledge bases accessible to user with pagination.

    Permission Logic:
    - Include team KBs from joined tenants
    - Include private KBs owned by user

    Filters:
    - keywords: Case-insensitive name search
    - parser_id: Filter by parser type

    Joins:
    - User: Get owner nickname and avatar

    Returns:
        (list[dict], total_count)
    """
```

**Query Structure**:

```sql
SELECT
    kb.id, kb.avatar, kb.name, kb.language, kb.description,
    kb.tenant_id, kb.permission, kb.doc_num, kb.token_num,
    kb.chunk_num, kb.parser_id, kb.embd_id,
    user.nickname, user.avatar AS tenant_avatar,
    kb.update_time
FROM knowledgebase kb
JOIN user ON kb.tenant_id = user.id
WHERE
    ((kb.tenant_id IN (?, ?, ...) AND kb.permission = 'team')
     OR kb.tenant_id = ?)
    AND kb.status = '1'
    AND LOWER(kb.name) LIKE '%keyword%'  -- if keywords
    AND kb.parser_id = ?                  -- if parser_id
ORDER BY kb.{orderby} DESC/ASC
LIMIT ? OFFSET ?
```

---

### 3. Get Knowledgebase Detail

**Lines**: 250-292

```python
@classmethod
@DB.connection_context()
def get_detail(cls, kb_id):
    """
    Get comprehensive KB information including pipeline details.

    Joins:
    - UserCanvas (LEFT): Get pipeline name and avatar

    Fields included:
    - Basic: id, avatar, name, language, description
    - Config: parser_id, parser_config, embd_id
    - Stats: doc_num, token_num, chunk_num
    - Pipeline: pipeline_id, pipeline_name, pipeline_avatar
    - GraphRAG: graphrag_task_id, graphrag_task_finish_at
    - RAPTOR: raptor_task_id, raptor_task_finish_at
    - MindMap: mindmap_task_id, mindmap_task_finish_at
    - Timestamps: create_time, update_time

    Returns:
        dict or None if not found
    """
```

---

### 4. Check Parsing Status

**Lines**: 85-117

```python
@classmethod
@DB.connection_context()
def is_parsed_done(cls, kb_id):
    """
    Verify all documents in KB are ready for chat.

    Validation Rules:
    1. KB must exist
    2. No documents in RUNNING/CANCEL/FAIL state
    3. No documents UNSTART with zero chunks

    Returns:
        (True, None) - All parsed
        (False, error_message) - Not ready

    Used by:
        Chat creation validation
    """
```

**Status Check Flow**:

```
is_parsed_done(kb_id)
         │
         ├──► cls.query(id=kb_id)
         │       └── Get KB info
         │
         ├──► DocumentService.get_by_kb_id(kb_id, ...)
         │       └── Get all documents (up to 1000)
         │
         └──► For each document:
                 │
                 ├─── run == RUNNING ───► Return (False, "still being parsed")
                 ├─── run == CANCEL  ───► Return (False, "still being parsed")
                 ├─── run == FAIL    ───► Return (False, "still being parsed")
                 └─── run == UNSTART
                        └── chunk_num == 0 ──► Return (False, "has not been parsed")

         └──► Return (True, None)
```

---

### 5. Parser Configuration Management

**Lines**: 294-345

```python
@classmethod
@DB.connection_context()
def update_parser_config(cls, id, config):
    """
    Deep merge parser configuration.

    Algorithm (dfs_update):
    - For dict values: recursively merge
    - For list values: union (set merge)
    - For scalar values: replace

    Example:
        old = {"pages": [[1, 100]], "ocr": True}
        new = {"pages": [[101, 200]], "language": "en"}
        result = {"pages": [[1, 100], [101, 200]], "ocr": True, "language": "en"}
    """

@classmethod
@DB.connection_context()
def delete_field_map(cls, id):
    """Remove field_map key from parser_config."""

@classmethod
@DB.connection_context()
def get_field_map(cls, ids):
    """
    Aggregate field mappings from multiple KBs.

    Used by: TABLE parser for column mapping
    """
```

**Deep Merge Algorithm**:

```python
def dfs_update(old, new):
    for k, v in new.items():
        if k not in old:
            old[k] = v           # Add new key
        elif isinstance(v, dict):
            dfs_update(old[k], v) # Recursive merge
        elif isinstance(v, list):
            old[k] = list(set(old[k] + v))  # Union lists
        else:
            old[k] = v           # Replace value
```

---

### 6. Document Statistics Management

**Lines**: 516-565

```python
@classmethod
@DB.connection_context()
def atomic_increase_doc_num_by_id(cls, kb_id):
    """
    Atomically increment doc_num by 1.
    Called when: DocumentService.insert()

    SQL: UPDATE knowledgebase SET doc_num = doc_num + 1 WHERE id = ?
    """

@classmethod
@DB.connection_context()
def decrease_document_num_in_delete(cls, kb_id, doc_num_info: dict):
    """
    Decrease statistics when documents are deleted.

    doc_num_info = {
        'doc_num': number of docs deleted,
        'chunk_num': total chunks deleted,
        'token_num': total tokens deleted
    }

    SQL:
        UPDATE knowledgebase SET
            doc_num = doc_num - ?,
            chunk_num = chunk_num - ?,
            token_num = token_num - ?,
            update_time = ?
        WHERE id = ?
    """
```

**Statistics Flow**:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    STATISTICS TRACKING                                       │
└─────────────────────────────────────────────────────────────────────────────┘

[Document Insert]
    │
    └──► KnowledgebaseService.atomic_increase_doc_num_by_id(kb_id)
            └── kb.doc_num += 1

[Chunk Processing]
    │
    └──► DocumentService.increment_chunk_num(doc_id, kb_id, tokens, chunks, ...)
            ├── doc.chunk_num += chunks
            ├── doc.token_num += tokens
            ├── kb.chunk_num += chunks
            └── kb.token_num += tokens

[Document Delete]
    │
    └──► KnowledgebaseService.decrease_document_num_in_delete(kb_id, info)
            ├── kb.doc_num -= info['doc_num']
            ├── kb.chunk_num -= info['chunk_num']
            └── kb.token_num -= info['token_num']
```

---

### 7. Access Control Methods

**Lines**: 471-514

```python
@classmethod
@DB.connection_context()
def accessible(cls, kb_id, user_id):
    """
    Check if user can access (view) KB.

    Logic: User must belong to KB's tenant via UserTenant table.

    SQL:
        SELECT kb.id
        FROM knowledgebase kb
        JOIN user_tenant ON user_tenant.tenant_id = kb.tenant_id
        WHERE kb.id = ? AND user_tenant.user_id = ?
    """

@classmethod
@DB.connection_context()
def accessible4deletion(cls, kb_id, user_id):
    """
    Check if user can delete KB.

    Logic: User must be the CREATOR of the KB.

    SQL:
        SELECT kb.id
        FROM knowledgebase kb
        WHERE kb.id = ? AND kb.created_by = ?
    """
```

**Access Control Diagram**:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    ACCESS CONTROL CHECKS                                     │
└─────────────────────────────────────────────────────────────────────────────┘

VIEW Access (accessible):
┌─────────────────────────────────────────────────────────────────┐
│                        User                                      │
│                          │                                       │
│           ┌──────────────┴──────────────┐                       │
│           ▼                             ▼                       │
│     UserTenant                    Knowledgebase                 │
│     (user_id)                     (tenant_id)                   │
│           │                             │                       │
│           └──────────┬──────────────────┘                       │
│                      ▼                                          │
│              tenant_id MATCH?                                   │
│                      │                                          │
│            ┌────────┴────────┐                                  │
│            Yes               No                                 │
│            │                 │                                  │
│         ALLOWED          DENIED                                 │
└─────────────────────────────────────────────────────────────────┘

DELETE Access (accessible4deletion):
┌─────────────────────────────────────────────────────────────────┐
│                        User                                      │
│                      (user_id)                                   │
│                          │                                       │
│                          ▼                                       │
│                   Knowledgebase                                  │
│                   (created_by)                                   │
│                          │                                       │
│              user_id == created_by?                              │
│                          │                                       │
│            ┌────────────┴────────────┐                          │
│            Yes                        No                         │
│            │                          │                         │
│         ALLOWED                    DENIED                       │
└─────────────────────────────────────────────────────────────────┘
```

---

## Service Interactions

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    SERVICE INTERACTION DIAGRAM                               │
└─────────────────────────────────────────────────────────────────────────────┘

                        KnowledgebaseService
                                │
        ┌───────────────────────┼───────────────────────┐
        │                       │                       │
        ▼                       ▼                       ▼
┌───────────────┐      ┌───────────────┐      ┌───────────────┐
│   Document    │      │    Tenant     │      │     User      │
│   Service     │      │   Service     │      │   Service     │
│               │      │               │      │               │
│ • get_by_kb_id│      │ • get_by_id   │      │ • get profile │
│ • insert      │      │   (validate   │      │   info for    │
│   (→ atomic   │      │    tenant)    │      │   joins       │
│   _increase)  │      │               │      │               │
└───────────────┘      └───────────────┘      └───────────────┘

API Layer Callers:
┌───────────────┐      ┌───────────────┐      ┌───────────────┐
│   kb_app.py   │      │ dialog_app.py │      │  RESTful API  │
│               │      │               │      │               │
│ • create      │      │ • is_parsed   │      │ • create_     │
│ • update      │      │   _done check │      │   with_name   │
│ • delete      │      │   before chat │      │               │
│ • list        │      │               │      │               │
└───────────────┘      └───────────────┘      └───────────────┘
```

---

## Key Method Reference Table

| Category | Method | Lines | Purpose |
|----------|--------|-------|---------|
| **Query** | `get_by_tenant_ids` | 134-197 | Paginated list with permissions |
| **Query** | `get_all_kb_by_tenant_ids` | 199-233 | Get all KBs (batch pagination) |
| **Query** | `get_kb_ids` | 235-248 | Get KB IDs for tenant |
| **Query** | `get_detail` | 250-292 | Comprehensive KB info |
| **Query** | `get_by_name` | 347-363 | Get by name + tenant |
| **Query** | `get_list` | 432-469 | Filtered paginated list |
| **Query** | `get_all_ids` | 365-371 | Get all KB IDs |
| **Create** | `create_with_name` | 374-429 | Validated creation |
| **Access** | `accessible` | 471-486 | View permission check |
| **Access** | `accessible4deletion` | 53-83 | Delete permission check |
| **Access** | `get_kb_by_id` | 488-500 | Get with user permission |
| **Access** | `get_kb_by_name` | 502-514 | Get by name with permission |
| **Config** | `update_parser_config` | 294-321 | Deep merge config |
| **Config** | `delete_field_map` | 323-331 | Remove field map |
| **Config** | `get_field_map` | 333-345 | Get field mappings |
| **Status** | `is_parsed_done` | 85-117 | Check parsing complete |
| **Stats** | `atomic_increase_doc_num_by_id` | 516-524 | Increment doc count |
| **Stats** | `decrease_document_num_in_delete` | 552-565 | Decrease on delete |
| **Stats** | `update_document_number_in_init` | 526-550 | Init doc count |
| **Docs** | `list_documents_by_ids` | 119-132 | Get doc IDs for KBs |

---

## API Endpoints Mapping

| HTTP Method | Endpoint | Service Method |
|-------------|----------|----------------|
| `POST` | `/v1/kb/create` | `create_with_name()` |
| `GET` | `/v1/kb/list` | `get_by_tenant_ids()` |
| `GET` | `/v1/kb/detail` | `get_detail()` |
| `PUT` | `/v1/kb/{kb_id}` | `update_by_id()` (inherited) |
| `DELETE` | `/v1/kb/{kb_id}` | `delete_by_id()` + cleanup |
| `PUT` | `/v1/kb/{kb_id}/config` | `update_parser_config()` |

---

## Parser Configuration Schema

```python
parser_config = {
    # Page range for PDF parsing
    "pages": [[1, 1000000]],  # Default: all pages

    # OCR settings
    "ocr": True,
    "ocr_model": "tesseract",  # or "paddleocr"

    # Layout settings
    "layout_recognize": True,

    # Chunking settings
    "chunk_token_num": 128,
    "delimiter": "\n!?。；！？",

    # For TABLE parser
    "field_map": {
        "column_name": "mapped_field_name"
    },

    # For specific parsers
    "raptor": {"enabled": False},
    "graphrag": {"enabled": False}
}
```

---

## Error Handling

| Location | Error Type | Handling |
|----------|-----------|----------|
| `create_with_name()` | Invalid name | Return `(False, error_result)` |
| `create_with_name()` | Tenant not found | Return `(False, error_result)` |
| `update_parser_config()` | KB not found | Raise `LookupError` |
| `delete_field_map()` | KB not found | Raise `LookupError` |
| `decrease_document_num_in_delete()` | KB not found | Raise `RuntimeError` |
| `update_document_number_in_init()` | ValueError "no data to save" | Pass (ignore) |

---

## Database Patterns

### Atomic Updates

```python
# Atomic increment using SQL expression (Line 522-523)
data["doc_num"] = cls.model.doc_num + 1  # Peewee generates: doc_num + 1
cls.model.update(data).where(cls.model.id == kb_id).execute()
```

### Batch Pagination Pattern

```python
# Avoid deep pagination performance issues (Lines 224-232)
offset, limit = 0, 50
res = []
while True:
    kb_batch = kbs.offset(offset).limit(limit)
    _temp = list(kb_batch.dicts())
    if not _temp:
        break
    res.extend(_temp)
    offset += limit
```

### Selective Field Save

```python
# Save only dirty fields without updating timestamps (Lines 537-545)
dirty_fields = kb.dirty_fields
if cls.model._meta.combined.get("update_time") in dirty_fields:
    dirty_fields.remove(cls.model._meta.combined["update_time"])
kb.save(only=dirty_fields)
```

---

## Key Constants & Imports

```python
# Permission types (from api/db/__init__.py)
class TenantPermission(Enum):
    ME = "me"      # Private to creator
    TEAM = "team"  # Shared with tenant

# Status (from common/constants.py)
class StatusEnum(Enum):
    WASTED = "0"  # Soft deleted
    VALID = "1"   # Active

# Dataset name limit (from api/constants.py)
DATASET_NAME_LIMIT = 128  # bytes (UTF-8)

# Default parser
ParserType.NAIVE = "naive"
```

---

## Performance Considerations

1. **Batch Pagination**: `get_all_kb_by_tenant_ids()` uses offset-limit pagination to avoid memory issues

2. **Selective Joins**: Queries only join necessary tables (User, UserTenant, UserCanvas)

3. **Index Usage**: All filter/sort fields are indexed (`tenant_id`, `name`, `permission`, `status`, `parser_id`)

4. **Atomic Operations**: Statistics updates use SQL expressions for atomicity without explicit transactions

5. **Lazy Loading**: Document details fetched separately from KB list queries
