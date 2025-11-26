# RAGFlow Analysis Documentation

Tài liệu phân tích chi tiết về RAGFlow - Open-source RAG Engine.

## Tổng Quan RAGFlow

**RAGFlow** (v0.22.1) là một **Retrieval-Augmented Generation (RAG) engine** mã nguồn mở, được xây dựng dựa trên **deep document understanding**. Đây là một ứng dụng full-stack với:

- **Backend**: Python (Flask/Quart)
- **Frontend**: React/TypeScript (UmiJS)
- **Kiến trúc**: Microservices với Docker
- **Data Stores**: MySQL, Elasticsearch/Infinity, Redis, MinIO

## Danh Sách Tài Liệu

| File | Nội dung |
|------|----------|
| [01_directory_structure.md](./01_directory_structure.md) | Cấu trúc cây thư mục chi tiết |
| [02_system_architecture.md](./02_system_architecture.md) | Kiến trúc hệ thống với diagrams |
| [03_sequence_diagrams.md](./03_sequence_diagrams.md) | Sequence diagrams cho các flows chính |
| [04_modules_analysis.md](./04_modules_analysis.md) | Phân tích chi tiết từng module |
| [05_tech_stack.md](./05_tech_stack.md) | Tech stack và dependencies |
| [06_source_code_analysis.md](./06_source_code_analysis.md) | Phân tích source code chi tiết |

## Tóm Tắt Chức Năng Chính

### 1. Document Processing
- Upload và parse nhiều định dạng (PDF, Word, Excel, PPT, HTML...)
- OCR và layout analysis cho PDF
- Intelligent chunking strategies

### 2. RAG Pipeline
- Hybrid search (Vector + BM25)
- Multiple embedding models support
- Reranking với cross-encoder

### 3. Chat/Dialog
- Streaming responses (SSE)
- Multi-knowledge base retrieval
- Conversation history

### 4. Agent Workflows
- Visual canvas builder
- 15+ built-in components
- 20+ external tool integrations

### 5. Knowledge Graph (GraphRAG)
- Entity extraction và resolution
- Graph-based retrieval
- Relationship visualization

## Kiến Trúc High-Level

```
┌─────────────────────────────────────────────────────────────────┐
│                         CLIENTS                                  │
│        Web App │ Mobile │ Python SDK │ REST API                 │
└────────────────────────────┬────────────────────────────────────┘
                             │
┌────────────────────────────┼────────────────────────────────────┐
│                       NGINX (Gateway)                            │
└────────────────────────────┬────────────────────────────────────┘
                             │
┌────────────────────────────┼────────────────────────────────────┐
│                    APPLICATION LAYER                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │RAGFlow Server│  │ Admin Server │  │  MCP Server  │          │
│  │  (Port 9380) │  │  (Port 9381) │  │  (Port 9382) │          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
└────────────────────────────┬────────────────────────────────────┘
                             │
┌────────────────────────────┼────────────────────────────────────┐
│                     SERVICE LAYER                                │
│  ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐        │
│  │  RAG   │ │DeepDoc │ │ Agent  │ │GraphRAG│ │Services│        │
│  │Pipeline│ │Parsers │ │ Canvas │ │ Engine │ │ Layer  │        │
│  └────────┘ └────────┘ └────────┘ └────────┘ └────────┘        │
└────────────────────────────┬────────────────────────────────────┘
                             │
┌────────────────────────────┼────────────────────────────────────┐
│                      DATA LAYER                                  │
│  ┌────────┐ ┌────────────┐ ┌────────┐ ┌────────┐               │
│  │ MySQL  │ │Elasticsearch│ │ Redis  │ │ MinIO  │               │
│  │(5455)  │ │   (9200)    │ │ (6379) │ │ (9000) │               │
│  └────────┘ └────────────┘ └────────┘ └────────┘               │
└─────────────────────────────────────────────────────────────────┘
```

## Tech Stack Summary

| Layer | Technologies |
|-------|-------------|
| **Frontend** | React 18, TypeScript, UmiJS, Ant Design, Tailwind CSS |
| **Backend** | Python 3.10-3.12, Flask/Quart, Peewee ORM |
| **AI/ML** | OpenAI, Sentence Transformers, Detectron2, PyTorch |
| **Database** | MySQL 8, Elasticsearch 8, Redis 7 |
| **Storage** | MinIO (S3-compatible) |
| **Infrastructure** | Docker, Nginx, Kubernetes/Helm |

## LLM Providers Supported

- OpenAI (GPT-3.5, GPT-4, GPT-4V)
- Anthropic (Claude 3)
- Google (Gemini)
- Alibaba (Qwen)
- Groq, Mistral, Cohere
- Ollama (local models)
- 20+ more providers

## Data Connectors

- Enterprise: Confluence, Notion, SharePoint, Jira
- Communication: Slack, Discord, Gmail, Teams
- Storage: Google Drive, Dropbox, S3, WebDAV

## Quick Stats

| Metric | Value |
|--------|-------|
| Total LOC | ~62,000+ |
| Python Files | ~300+ |
| TS/JS Files | ~400+ |
| Database Models | 25+ |
| API Endpoints | ~50+ |
| LLM Providers | 20+ |
| Data Connectors | 15+ |

## License

RAGFlow is open-source under Apache 2.0 license.

---

*Documentation generated: 2025-11-26*
