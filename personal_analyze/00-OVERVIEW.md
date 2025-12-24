# RAGFlow Backend Architecture - Comprehensive Analysis

## Tong Quan

RAGFlow là open-source RAG (Retrieval-Augmented Generation) engine với deep document understanding. Document này tổng hợp phân tích chi tiết kiến trúc backend.

## Version
- RAGFlow v0.22.1
- Analysis Date: 2025-01

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          RAGFlow BACKEND ARCHITECTURE                        │
└─────────────────────────────────────────────────────────────────────────────┘

                    ┌─────────────────────────────────┐
                    │         NGINX / Gateway          │
                    └─────────────────┬───────────────┘
                                      │
                    ┌─────────────────▼───────────────┐
                    │      01-API-LAYER               │
                    │  Flask/Quart Blueprints         │
                    │  - Authentication (JWT/OAuth)   │
                    │  - Request Routing              │
                    │  - SSE Streaming                │
                    └─────────────────┬───────────────┘
                                      │
                    ┌─────────────────▼───────────────┐
                    │      02-SERVICE-LAYER           │
                    │  Business Logic                 │
                    │  - DialogService (Chat)         │
                    │  - DocumentService              │
                    │  - TaskService (Queue)          │
                    │  - LLMBundle (Model Wrapper)    │
                    └─────────────────┬───────────────┘
                                      │
        ┌─────────────────────────────┼─────────────────────────────┐
        │                             │                             │
┌───────▼───────┐         ┌───────────▼───────────┐     ┌───────────▼───────┐
│ 03-RAG-ENGINE │         │  04-AGENT-SYSTEM      │     │05-DOC-PROCESSING │
│               │         │                       │     │                   │
│ - Hybrid      │         │ - Canvas Engine       │     │ - PDF Parser      │
│   Search      │         │ - Components          │     │ - OCR             │
│ - Embedding   │         │ - Tools               │     │ - Layout          │
│ - Reranking   │         │ - ReAct Agent         │     │ - TSR             │
└───────────────┘         └───────────────────────┘     └───────────────────┘
        │                             │                             │
        └─────────────────────────────┼─────────────────────────────┘
                                      │
                    ┌─────────────────▼───────────────┐
                    │      06-ALGORITHMS              │
                    │  - BM25 Scoring                 │
                    │  - Vector Cosine Similarity     │
                    │  - Hybrid Score Fusion          │
                    │  - TF-IDF Weighting             │
                    │  - RAPTOR                       │
                    │  - GraphRAG                     │
                    └─────────────────────────────────┘
                                      │
        ┌─────────────────────────────┼─────────────────────────────┐
        │                             │                             │
┌───────▼───────┐         ┌───────────▼───────────┐     ┌───────────▼───────┐
│    MySQL      │         │   Elasticsearch/      │     │      MinIO        │
│  (Metadata)   │         │   Infinity (Vectors)  │     │   (File Storage)  │
└───────────────┘         └───────────────────────┘     └───────────────────┘
```

## Module Summary

### 01-API-LAYER
API Gateway xử lý authentication, routing, và SSE streaming.

| File | Purpose |
|------|---------|
| `document_app_analysis.md` | Document upload/processing API |
| `conversation_app_analysis.md` | Chat API với SSE |
| `canvas_app_analysis.md` | Agent workflow API |
| `authentication_flow.md` | JWT/OAuth authentication |
| `request_lifecycle.md` | Request processing pipeline |

**Key Technologies:**
- Flask/Quart (async ASGI)
- Blueprint routing
- JWT + API Token authentication
- SSE streaming

### 02-SERVICE-LAYER
Business logic layer với service pattern.

| File | Purpose |
|------|---------|
| `dialog_service_analysis.md` | RAG chat pipeline |
| `task_service_analysis.md` | Background task queue |
| `llm_service_analysis.md` | 60+ LLM provider abstraction |

**Key Technologies:**
- Peewee ORM
- Redis task queue
- LLMBundle wrapper
- Langfuse observability

### 03-RAG-ENGINE
Core RAG implementation với hybrid search.

| File | Purpose |
|------|---------|
| `hybrid_search_algorithm.md` | Vector + BM25 fusion |
| `embedding_generation.md` | 30+ embedding models |
| `rerank_algorithm.md` | Cross-encoder reranking |
| `chunking_strategies.md` | Document chunking |
| `query_processing.md` | TF-IDF query weighting |

**Key Algorithms:**
- Hybrid Score: 95% Vector + 5% BM25
- Cosine similarity
- Cross-encoder reranking
- Token-based chunking

### 04-AGENT-SYSTEM
Agentic workflows với visual canvas.

| File | Purpose |
|------|---------|
| `canvas_execution_engine.md` | Workflow orchestration |
| `component_architecture.md` | Component lifecycle |
| `tool_integration.md` | Tool framework |

**Key Features:**
- DSL-based workflows
- 15+ component types
- 10+ tool integrations
- ReAct agent pattern

### 05-DOCUMENT-PROCESSING
Document parsing pipeline.

| File | Purpose |
|------|---------|
| `task_executor_analysis.md` | Async task processing |
| `pdf_parsing.md` | PDF với OCR + layout |

**Key Technologies:**
- PaddleOCR
- Detectron2 layout detection
- TableTransformer (TSR)
- XGBoost text merging

### 06-ALGORITHMS
Core algorithms và math.

| File | Purpose |
|------|---------|
| `bm25_scoring.md` | BM25 ranking |
| `hybrid_score_fusion.md` | Score combination |
| `raptor_algorithm.md` | Hierarchical summarization |

## Tech Stack Summary

### Backend Framework
```
Python 3.10+
├── Flask/Quart - Web framework
├── Peewee - ORM
├── Trio - Async concurrency
└── Celery-like - Task queue (Redis-based)
```

### Data Stores
```
MySQL - Metadata, users, configs
Elasticsearch/Infinity - Vector search + BM25
Redis - Task queue, caching, sessions
MinIO - Object storage (documents, images)
```

### ML/AI
```
LLM Providers (60+)
├── OpenAI, Azure, Claude, Gemini
├── Qwen, DeepSeek, Groq
├── Ollama (local)
└── LiteLLM (unified interface)

Embedding Models (30+)
├── OpenAI text-embedding-3
├── BGE, Jina, Cohere
└── HuggingFace TEI

Vision Models
├── PaddleOCR
├── Detectron2
└── TableTransformer
```

### Search & Retrieval
```
Hybrid Search
├── BM25 (Elasticsearch native)
├── Vector (cosine similarity)
└── Fusion (weighted sum)

Reranking
├── Jina Reranker
├── Cohere Rerank
└── BGE Reranker
```

## Key Flows

### 1. Document Upload Flow
```
Upload → MinIO → Task Queue → Parser → Chunking → Embedding → Elasticsearch
```

### 2. Chat/Query Flow
```
Query → TF-IDF Weight → Hybrid Search → Rerank → Context Building → LLM → SSE Stream
```

### 3. Agent Workflow Flow
```
User Input → Canvas Engine → Component Execution → Tool Calls → LLM → Output
```

## Performance Metrics

| Operation | Typical Latency |
|-----------|-----------------|
| Vector Search | < 100ms |
| BM25 Search | < 50ms |
| Reranking | 200-500ms |
| Total Retrieval | < 1s |
| Embedding (batch 16) | 1-5s |
| PDF Parsing (10 pages) | 30-60s |

## Configuration Highlights

### Search Config
```python
{
    "vector_similarity_weight": 0.95,   # 95% vector
    "similarity_threshold": 0.2,        # Min similarity
    "top_k": 1024,                       # Initial candidates
    "top_n": 6,                          # Final results
}
```

### Chunking Config
```python
{
    "chunk_token_num": 512,             # Tokens per chunk
    "delimiter": "\n。；！？",          # Split chars
    "overlapped_percent": 0,            # Overlap %
}
```

### Agent Config
```python
{
    "max_rounds": 5,                    # Max tool rounds
    "temperature": 0.7,                 # LLM temperature
    "max_tokens": 2048,                 # Response limit
}
```

## Directory Structure

```
personal_analyze/
├── 00-OVERVIEW.md                  # This file
├── 01-API-LAYER/
│   ├── README.md
│   ├── document_app_analysis.md
│   ├── conversation_app_analysis.md
│   ├── canvas_app_analysis.md
│   ├── authentication_flow.md
│   └── request_lifecycle.md
├── 02-SERVICE-LAYER/
│   ├── README.md
│   ├── dialog_service_analysis.md
│   ├── task_service_analysis.md
│   └── llm_service_analysis.md
├── 03-RAG-ENGINE/
│   ├── README.md
│   ├── hybrid_search_algorithm.md
│   ├── embedding_generation.md
│   ├── rerank_algorithm.md
│   ├── chunking_strategies.md
│   └── query_processing.md
├── 04-AGENT-SYSTEM/
│   ├── README.md
│   ├── canvas_execution_engine.md
│   ├── component_architecture.md
│   └── tool_integration.md
├── 05-DOCUMENT-PROCESSING/
│   ├── README.md
│   ├── task_executor_analysis.md
│   └── pdf_parsing.md
└── 06-ALGORITHMS/
    ├── README.md
    ├── bm25_scoring.md
    ├── hybrid_score_fusion.md
    └── raptor_algorithm.md
```

## Key Source Files

| Module | Key File | Description |
|--------|----------|-------------|
| API | `/api/apps/dialog_app.py` | Chat API endpoints |
| Service | `/api/db/services/dialog_service.py` | RAG chat logic |
| RAG | `/rag/nlp/search.py` | Hybrid search |
| Agent | `/agent/canvas.py` | Workflow engine |
| Parser | `/deepdoc/parser/pdf_parser.py` | PDF parsing |
| Algorithms | `/rag/raptor.py` | RAPTOR algorithm |

## Conclusion

RAGFlow là một comprehensive RAG system với:
- **Multi-provider LLM support** (60+ providers)
- **Advanced document understanding** (OCR, layout, tables)
- **Hybrid search** (Vector + BM25)
- **Agentic workflows** (visual canvas)
- **Production-ready** (multi-tenant, scalable)

Tham khảo các file chi tiết trong từng module để hiểu sâu hơn về implementation.
