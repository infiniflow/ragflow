# RAGFlow Database Architecture

## Overview

RAGFlow sử dụng 4 loại database chính để lưu trữ và xử lý dữ liệu:

| Database | Loại | Mục đích chính |
|----------|------|----------------|
| MySQL | Relational | Metadata, user data, configs |
| Elasticsearch/Infinity | Vector + Search | Chunks, embeddings, full-text search |
| Redis | In-memory | Task queue, caching, distributed locks |
| MinIO | Object Storage | Raw files (PDF, DOCX, images) |

---

## Data Flow Diagram

```
┌──────────────────────────────────────────────────────────────────────────┐
│                            USER UPLOAD FILE                               │
└──────────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌──────────────────────────────────────────────────────────────────────────┐
│  STEP 1: MinIO (Object Storage)                                          │
│  ─────────────────────────────────────────────────────────────────────── │
│  Action: Lưu raw file                                                    │
│  Path: bucket={kb_id}, location={filename}                               │
│  Data: Binary content của file gốc                                       │
└──────────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌──────────────────────────────────────────────────────────────────────────┐
│  STEP 2: MySQL (Metadata)                                                │
│  ─────────────────────────────────────────────────────────────────────── │
│  Tables affected:                                                        │
│  • File: {id, parent_id, tenant_id, name, location, size, type}         │
│  • Document: {id, kb_id, name, location, size, parser_id, progress=0}   │
│  • File2Document: {file_id, document_id}                                │
└──────────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌──────────────────────────────────────────────────────────────────────────┐
│  STEP 3: Redis (Task Queue)                                              │
│  ─────────────────────────────────────────────────────────────────────── │
│  Action: Push task message to stream                                     │
│  Queue: "rag_flow_svr_queue"                                            │
│  Message: {                                                              │
│    "id": "task_xxx",                                                    │
│    "doc_id": "doc_xxx",                                                 │
│    "kb_id": "kb_xxx",                                                   │
│    "tenant_id": "tenant_xxx",                                           │
│    "parser_id": "naive|paper|book|...",                                 │
│    "task_type": "parse"                                                 │
│  }                                                                       │
└──────────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌──────────────────────────────────────────────────────────────────────────┐
│  STEP 4: Task Executor (Worker Process)                                  │
│  ─────────────────────────────────────────────────────────────────────── │
│  Actions:                                                                │
│  1. Consume task from Redis queue                                        │
│  2. Fetch raw file from MinIO                                           │
│  3. Parse & chunk document                                              │
│  4. Generate embeddings                                                 │
│  5. Update progress in MySQL                                            │
└──────────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌──────────────────────────────────────────────────────────────────────────┐
│  STEP 5: Elasticsearch/Infinity (Vector Store)                           │
│  ─────────────────────────────────────────────────────────────────────── │
│  Action: Insert chunks with embeddings                                   │
│  Index: {tenant_id}_ragflow                                             │
│  Document: {                                                             │
│    "id": "xxhash(content + doc_id)",                                    │
│    "doc_id": "doc_xxx",                                                 │
│    "kb_id": ["kb_xxx"],                                                 │
│    "content_with_weight": "chunk text...",                              │
│    "q_1024_vec": [0.1, 0.2, ...],                                       │
│    "important_kwd": ["keyword1", "keyword2"],                           │
│    "question_kwd": ["What is...?"],                                     │
│    "page_num_int": [1, 2],                                              │
│    "create_time": "2024-01-01 12:00:00"                                 │
│  }                                                                       │
└──────────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌──────────────────────────────────────────────────────────────────────────┐
│  STEP 6: MySQL (Update Status)                                           │
│  ─────────────────────────────────────────────────────────────────────── │
│  Table: Document                                                         │
│  Update: {                                                               │
│    "chunk_num": 42,                                                     │
│    "token_num": 15000,                                                  │
│    "progress": 1.0,                                                     │
│    "process_duration": 12.5                                             │
│  }                                                                       │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## Database Storage Details

### 1. MySQL Tables

#### User & Tenant Management

| Table | Fields | Description |
|-------|--------|-------------|
| `user` | id, email, password, nickname, avatar, language, timezone, last_login_time, is_superuser | User accounts |
| `tenant` | id, name, llm_id, embd_id, rerank_id, asr_id, img2txt_id, tts_id, parser_ids, credit | Tenant configuration |
| `user_tenant` | id, user_id, tenant_id, role, invited_by | User-Tenant mapping |
| `invitation_code` | id, code, user_id, tenant_id, visit_time | Invitation codes |

#### LLM & Model Configuration

| Table | Fields | Description |
|-------|--------|-------------|
| `llm_factories` | name, logo, tags, rank | LLM provider registry |
| `llm` | llm_name, model_type, fid, max_tokens, tags, is_tools | Model definitions |
| `tenant_llm` | tenant_id, llm_factory, llm_name, api_key, api_base, max_tokens, used_tokens | Tenant API keys |
| `tenant_langfuse` | tenant_id, secret_key, public_key, host | Observability config |

#### Knowledge Base & Documents

| Table | Fields | Description |
|-------|--------|-------------|
| `knowledgebase` | id, tenant_id, name, embd_id, parser_id, doc_num, chunk_num, token_num, similarity_threshold, vector_similarity_weight | KB metadata |
| `document` | id, kb_id, name, location, size, parser_id, parser_config, progress, chunk_num, token_num, meta_fields | Document metadata |
| `file` | id, parent_id, tenant_id, name, location, size, type, source_type | File system structure |
| `file2document` | id, file_id, document_id | File to Document mapping |
| `task` | id, doc_id, from_page, to_page, task_type, priority, progress, retry_count, chunk_ids | Processing tasks |

#### Chat & Conversation

| Table | Fields | Description |
|-------|--------|-------------|
| `dialog` | id, tenant_id, name, kb_ids, llm_id, llm_setting, prompt_config, similarity_threshold, top_n, rerank_id | Chat app config |
| `conversation` | id, dialog_id, user_id, name, message (JSON), reference | Internal chat history |
| `api_4_conversation` | id, dialog_id, user_id, message, reference, tokens, duration, thumb_up, errors | API chat history |
| `api_token` | tenant_id, token, dialog_id, source | API authentication |

#### Agent & Canvas

| Table | Fields | Description |
|-------|--------|-------------|
| `user_canvas` | id, user_id, title, description, canvas_type, canvas_category, dsl (JSON) | Agent workflows |
| `canvas_template` | id, title, description, canvas_type, dsl | Workflow templates |
| `user_canvas_version` | id, user_canvas_id, title, dsl | Version history |
| `mcp_server` | id, tenant_id, name, url, server_type, variables, headers | MCP integrations |

#### Data Connectors

| Table | Fields | Description |
|-------|--------|-------------|
| `connector` | id, tenant_id, name, source, input_type, config, refresh_freq | External data sources |
| `connector2kb` | id, connector_id, kb_id, auto_parse | Connector-KB mapping |
| `sync_logs` | id, connector_id, kb_id, status, new_docs_indexed, error_msg | Sync history |

#### Operations

| Table | Fields | Description |
|-------|--------|-------------|
| `pipeline_operation_log` | id, document_id, pipeline_id, parser_id, progress, dsl | Pipeline logs |
| `search` | id, tenant_id, name, search_config | Search configurations |

---

### 2. Elasticsearch/Infinity Chunk Schema

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Chunk ID = xxhash(content + doc_id) |
| `doc_id` | string | Reference to source document |
| `kb_id` | string[] | Knowledge base IDs (list format) |
| `content_with_weight` | text | Chunk content |
| `content_ltks` | text | Tokenized content (for search) |
| `content_sm_ltks` | text | Fine-grained tokenized content |
| `q_{size}_vec` | float[] | Dense vector embeddings |
| `docnm_kwd` | keyword | Document filename |
| `title_tks` | text | Tokenized title |
| `important_kwd` | keyword[] | Extracted keywords |
| `question_kwd` | keyword[] | Generated questions |
| `tag_fea_kwd` | keyword[] | Content tags |
| `page_num_int` | int[] | Page numbers |
| `top_int` | int[] | Vertical position in page |
| `position_int` | int[] | Position coordinates |
| `image_id` | string | Reference to extracted image |
| `create_time` | string | Creation timestamp |
| `create_timestamp_flt` | float | Unix timestamp |

---

### 3. Redis Data Structures

| Key Pattern | Type | Description |
|-------------|------|-------------|
| `rag_flow_svr_queue` | Stream | Main task queue for document processing |
| `{lock_name}` | String | Distributed locks |
| `{cache_key}` | String/Hash | LLM response cache |
| `{session_id}` | String | User session data |

#### Task Message Schema

```json
{
  "id": "task_xxx",
  "doc_id": "document_id",
  "kb_id": "knowledgebase_id",
  "tenant_id": "tenant_id",
  "parser_id": "naive|paper|book|qa|table|...",
  "parser_config": {},
  "from_page": 0,
  "to_page": 100000,
  "name": "filename.pdf",
  "location": "storage_path",
  "language": "English|Chinese",
  "task_type": "parse|raptor|graphrag"
}
```

---

### 4. MinIO Object Storage

| Bucket | Object Path | Content |
|--------|-------------|---------|
| `{kb_id}` | `{filename}` | Raw document files |
| `{kb_id}` | `{filename}_` | Duplicate files (auto-renamed) |
| `{tenant_id}` | `{chunk_id}` | Extracted images from chunks |

---

## Data Lineage

```
Raw File (MinIO)
    │
    ├── location: "{kb_id}/{filename}"
    │
    ▼
Document (MySQL)
    │
    ├── id: "doc_xxx"
    ├── kb_id: "kb_xxx"
    ├── location: "{filename}"
    │
    ▼
Chunks (Elasticsearch/Infinity)
    │
    ├── doc_id: "doc_xxx"      ← Link back to Document
    ├── kb_id: ["kb_xxx"]      ← Link to Knowledge Base
    └── id: xxhash(content + doc_id)
```

---

## Key Observations

### Current Limitations

1. **No Data Fabric Layer**: Document (`doc_id`) is hard-coded to one Knowledge Base (`kb_id`)
2. **Duplicate Required**: Same file in multiple KBs requires re-upload and re-processing
3. **No Cross-KB Sharing**: Chunks cannot be shared across Knowledge Bases

### Potential Improvements

1. Separate `RawDocument` table from `Document`
2. Allow `Document.kb_id` to be a list or use junction table
3. Enable chunk sharing with multi-KB tagging
