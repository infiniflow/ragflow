# RAGFlow - Tech Stack Analysis

## 1. Tổng Quan Tech Stack

RAGFlow sử dụng một tech stack hiện đại, được thiết kế để xử lý các workload AI/ML nặng với khả năng scale tốt.

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           TECH STACK OVERVIEW                            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │                        FRONTEND                                     │ │
│  │  React 18 │ TypeScript │ UmiJS │ Ant Design │ Tailwind CSS         │ │
│  │  Zustand │ TanStack Query │ XYFlow │ Monaco Editor                 │ │
│  └────────────────────────────────────────────────────────────────────┘ │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │                        BACKEND                                      │ │
│  │  Python 3.10-3.12 │ Flask/Quart │ Peewee ORM │ Celery              │ │
│  │  AsyncIO │ JWT │ SSE Streaming                                      │ │
│  └────────────────────────────────────────────────────────────────────┘ │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │                        AI/ML                                        │ │
│  │  LangChain │ OpenAI │ Sentence Transformers │ Hugging Face         │ │
│  │  PyTorch │ Detectron2 │ Tesseract OCR                              │ │
│  └────────────────────────────────────────────────────────────────────┘ │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │                        DATA LAYER                                   │ │
│  │  MySQL 8 │ Elasticsearch 8 │ Redis │ MinIO │ Infinity              │ │
│  └────────────────────────────────────────────────────────────────────┘ │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │                        INFRASTRUCTURE                               │ │
│  │  Docker │ Docker Compose │ Kubernetes │ Nginx │ Helm               │ │
│  └────────────────────────────────────────────────────────────────────┘ │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Frontend Technologies

### 2.1 Core Framework

| Technology | Version | Mục đích |
|------------|---------|----------|
| **React** | 18.x | UI library chính |
| **TypeScript** | 5.x | Type-safe JavaScript |
| **UmiJS** | 4.x | React framework (Ant Design ecosystem) |
| **Vite** | 5.x | Build tool (nhanh hơn Webpack) |

### 2.2 UI Libraries

| Library | Version | Mục đích |
|---------|---------|----------|
| **Ant Design** | 5.x | Primary UI component library |
| **Shadcn/UI** | Latest | Modern, customizable components |
| **Radix UI** | Latest | Headless UI primitives |
| **Tailwind CSS** | 3.x | Utility-first CSS framework |
| **LESS** | 4.x | CSS preprocessor (legacy) |

### 2.3 State Management & Data Fetching

| Library | Mục đích |
|---------|----------|
| **Zustand** | Lightweight state management |
| **TanStack React Query** | Server state & caching |
| **Axios** | HTTP client |

### 2.4 Specialized Libraries

| Library | Mục đích |
|---------|----------|
| **XYFlow (React Flow)** | Workflow/canvas visualization |
| **Monaco Editor** | Code editor (VS Code core) |
| **AntV G2/G6** | Data visualization & graphs |
| **Recharts** | Charts and analytics |
| **Lexical** | Rich text editor (Facebook) |
| **React Markdown** | Markdown rendering |
| **i18next** | Internationalization |
| **React Hook Form** | Form handling |
| **Zod** | Schema validation |

### 2.5 Package.json Dependencies (172 packages)

```json
{
  "dependencies": {
    "react": "^18.2.0",
    "react-dom": "^18.2.0",
    "umi": "^4.0.0",
    "antd": "^5.0.0",
    "@tanstack/react-query": "^5.0.0",
    "zustand": "^4.0.0",
    "axios": "^1.0.0",
    "tailwindcss": "^3.0.0",
    "@xyflow/react": "^12.0.0",
    "@monaco-editor/react": "^4.0.0",
    "lexical": "^0.12.0",
    "react-markdown": "^9.0.0",
    "i18next": "^23.0.0",
    "react-hook-form": "^7.0.0",
    "zod": "^3.0.0",
    "@radix-ui/react-*": "latest",
    "@ant-design/icons": "^5.0.0",
    "@antv/g2": "^5.0.0",
    "@antv/g6": "^5.0.0"
  }
}
```

---

## 3. Backend Technologies

### 3.1 Core Framework

| Technology | Version | Mục đích |
|------------|---------|----------|
| **Python** | 3.10-3.12 | Programming language |
| **Flask** | 3.x | Web framework |
| **Quart** | 0.19.x | Async Flask (ASGI) |
| **Hypercorn** | Latest | ASGI server |

### 3.2 Database & ORM

| Technology | Mục đích |
|------------|----------|
| **Peewee** | Lightweight ORM (primary) |
| **SQLAlchemy** | Advanced ORM operations |
| **PyMySQL** | MySQL driver |

### 3.3 Authentication & Security

| Library | Mục đích |
|---------|----------|
| **PyJWT** | JWT token handling |
| **bcrypt** | Password hashing |
| **python-jose** | JOSE implementation |
| **Authlib** | OAuth integration |

### 3.4 Async & Background Tasks

| Library | Mục đích |
|---------|----------|
| **asyncio** | Async I/O |
| **aiohttp** | Async HTTP client |
| **Redis/Valkey** | Task queue & caching |
| **APScheduler** | Job scheduling |

### 3.5 API & Documentation

| Library | Mục đích |
|---------|----------|
| **Flasgger** | Swagger/OpenAPI docs |
| **Flask-CORS** | CORS handling |
| **Werkzeug** | WSGI utilities |

### 3.6 pyproject.toml Dependencies (150+ packages)

```toml
[project]
name = "ragflow"
version = "0.22.1"
requires-python = ">=3.10,<3.13"

dependencies = [
    # Web Framework
    "flask>=3.0.0",
    "quart>=0.19.0",
    "hypercorn>=0.17.0",
    "flask-cors>=4.0.0",
    "flasgger>=0.9.0",

    # Database
    "peewee>=3.17.0",
    "pymysql>=1.1.0",

    # Authentication
    "pyjwt>=2.8.0",
    "bcrypt>=4.1.0",

    # Async
    "aiohttp>=3.9.0",
    "httpx>=0.27.0",

    # Data Processing
    "pandas>=2.0.0",
    "numpy>=1.26.0",

    # AI/ML (see section 4)
    ...
]
```

---

## 4. AI/ML Technologies

### 4.1 LLM Integration

| Provider | Library | Models Supported |
|----------|---------|-----------------|
| **OpenAI** | `openai>=1.0` | GPT-3.5, GPT-4, GPT-4V |
| **Anthropic** | `anthropic>=0.20` | Claude 3 family |
| **Google** | `google-generativeai` | Gemini Pro |
| **Cohere** | `cohere>=5.0` | Command, Embed, Rerank |
| **Groq** | `groq>=0.4` | LLaMA, Mixtral |
| **Mistral** | `mistralai>=0.1` | Mistral 7B, Mixtral |
| **Ollama** | `ollama>=0.1` | Local models |
| **HuggingFace** | `huggingface_hub` | Open source models |

### 4.2 Embedding Models

| Library | Models |
|---------|--------|
| **Sentence Transformers** | all-MiniLM, all-mpnet, etc. |
| **OpenAI Embeddings** | text-embedding-3-small/large |
| **BGE** | bge-base, bge-large, bge-m3 |
| **Jina** | jina-embeddings-v2 |
| **Cohere** | embed-english-v3 |

```python
# Embedding configuration
EMBEDDING_MODELS = {
    "openai": {
        "text-embedding-3-small": {"dim": 1536, "max_tokens": 8191},
        "text-embedding-3-large": {"dim": 3072, "max_tokens": 8191},
    },
    "bge": {
        "bge-base-en-v1.5": {"dim": 768, "max_tokens": 512},
        "bge-large-en-v1.5": {"dim": 1024, "max_tokens": 512},
        "bge-m3": {"dim": 1024, "max_tokens": 8192},
    },
    "sentence-transformers": {
        "all-MiniLM-L6-v2": {"dim": 384, "max_tokens": 256},
        "all-mpnet-base-v2": {"dim": 768, "max_tokens": 384},
    }
}
```

### 4.3 Document Processing

| Library | Mục đích |
|---------|----------|
| **PyMuPDF (fitz)** | PDF text extraction |
| **pdf2image** | PDF to image conversion |
| **Tesseract (pytesseract)** | OCR |
| **python-docx** | Word document parsing |
| **openpyxl** | Excel parsing |
| **python-pptx** | PowerPoint parsing |
| **BeautifulSoup4** | HTML parsing |
| **markdown** | Markdown processing |
| **camelot-py** | Table extraction from PDF |
| **tabula-py** | Alternative table extraction |

### 4.4 Computer Vision

| Library | Mục đích |
|---------|----------|
| **Detectron2** | Layout analysis |
| **LayoutLM** | Document understanding |
| **OpenCV** | Image processing |
| **Pillow** | Image manipulation |
| **YOLO** | Object detection |

### 4.5 NLP & Text Processing

| Library | Mục đích |
|---------|----------|
| **tiktoken** | OpenAI tokenization |
| **nltk** | Natural language toolkit |
| **spaCy** | NLP pipeline |
| **regex** | Advanced regex |
| **chardet** | Character encoding detection |

### 4.6 Vector Operations

| Library | Mục đích |
|---------|----------|
| **NumPy** | Numerical operations |
| **SciPy** | Scientific computing |
| **scikit-learn** | ML utilities, clustering |
| **faiss-cpu/gpu** | Vector similarity search |

---

## 5. Data Storage Technologies

### 5.1 Relational Database

| Technology | Mục đích | Configuration |
|------------|----------|---------------|
| **MySQL 8.0** | Primary database | Port 5455 |
| **PostgreSQL** | Alternative (supported) | - |

**MySQL Schema Design**:
- InnoDB engine
- UTF8MB4 character set
- JSON columns for flexible data
- Foreign keys for integrity

### 5.2 Vector/Search Database

| Technology | Mục đích | Configuration |
|------------|----------|---------------|
| **Elasticsearch 8.12** | Default vector store | Port 9200 |
| **Infinity** | Alternative (in-house) | Port 23817 |
| **OpenSearch** | Alternative | Port 9200 |
| **OceanBase** | Alternative (distributed) | - |

**Elasticsearch Configuration**:
```json
{
  "settings": {
    "number_of_shards": 1,
    "number_of_replicas": 0,
    "analysis": {
      "analyzer": {
        "ik_smart": { "type": "ik_smart" },
        "ik_max_word": { "type": "ik_max_word" }
      }
    }
  },
  "mappings": {
    "properties": {
      "content": { "type": "text", "analyzer": "ik_smart" },
      "embedding": {
        "type": "dense_vector",
        "dims": 1536,
        "index": true,
        "similarity": "cosine"
      }
    }
  }
}
```

### 5.3 Cache & Message Queue

| Technology | Mục đích | Configuration |
|------------|----------|---------------|
| **Redis 7.x** | Cache, sessions, queue | Port 6379 |
| **Valkey** | Redis alternative | Port 6379 |

**Redis Usage**:
- Session storage
- Rate limiting
- Task queue (custom implementation)
- Cache layer

### 5.4 Object Storage

| Technology | Mục đích | Configuration |
|------------|----------|---------------|
| **MinIO** | S3-compatible storage | Port 9000/9001 |
| **AWS S3** | Cloud storage option | - |
| **Azure Blob** | Cloud storage option | - |

**MinIO Structure**:
```
ragflow/                    # Bucket
├── {tenant_id}/
│   ├── {kb_id}/
│   │   ├── {file_id}      # Original files
│   │   └── chunks/        # Processed chunks
│   └── temp/              # Temporary files
└── system/                # System files
```

---

## 6. Infrastructure Technologies

### 6.1 Containerization

| Technology | Mục đích |
|------------|----------|
| **Docker** | Container runtime |
| **Docker Compose** | Multi-container orchestration |
| **BuildKit** | Efficient image building |

**Docker Images**:
```yaml
services:
  ragflow-server:
    image: infiniflow/ragflow:latest
    # or: ragflow:nightly for development

  mysql:
    image: mysql:8.0

  elasticsearch:
    image: docker.elastic.co/elasticsearch/elasticsearch:8.12.0

  redis:
    image: redis:7-alpine

  minio:
    image: minio/minio:latest
```

### 6.2 Web Server & Proxy

| Technology | Mục đích | Configuration |
|------------|----------|---------------|
| **Nginx** | Reverse proxy, static files | Port 80/443 |
| **Hypercorn** | ASGI server | Port 9380 |

**Nginx Configuration**:
```nginx
upstream ragflow {
    server ragflow-server:9380;
}

server {
    listen 80;

    location /api/ {
        proxy_pass http://ragflow;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
    }

    location / {
        root /usr/share/nginx/html;
        try_files $uri $uri/ /index.html;
    }
}
```

### 6.3 Kubernetes Deployment

| Technology | Mục đích |
|------------|----------|
| **Kubernetes** | Container orchestration |
| **Helm** | K8s package manager |

**Helm Chart Structure**:
```
helm/
├── Chart.yaml
├── values.yaml
├── templates/
│   ├── deployment.yaml
│   ├── service.yaml
│   ├── configmap.yaml
│   └── ingress.yaml
```

---

## 7. Development Tools

### 7.1 Python Development

| Tool | Mục đích |
|------|----------|
| **uv** | Package manager (fast) |
| **pip** | Traditional package manager |
| **pre-commit** | Git hooks |
| **ruff** | Linter & formatter |
| **pytest** | Testing framework |
| **mypy** | Type checking |

### 7.2 Frontend Development

| Tool | Mục đích |
|------|----------|
| **npm/pnpm** | Package manager |
| **ESLint** | Linting |
| **Prettier** | Code formatting |
| **Jest** | Testing |
| **Storybook** | Component development |
| **Husky** | Git hooks |

### 7.3 Version Control & CI/CD

| Tool | Mục đích |
|------|----------|
| **Git** | Version control |
| **GitHub Actions** | CI/CD |
| **Docker Hub** | Image registry |

---

## 8. Monitoring & Observability

### 8.1 Logging

| Library | Mục đích |
|---------|----------|
| **Python logging** | Standard logging |
| **structlog** | Structured logging |

### 8.2 Tracing

| Integration | Mục đích |
|-------------|----------|
| **Langfuse** | LLM observability |
| **OpenTelemetry** | Distributed tracing |

### 8.3 Metrics

| Tool | Mục đích |
|------|----------|
| **Prometheus** | Metrics collection |
| **Grafana** | Visualization |

---

## 9. Third-party Integrations

### 9.1 LLM Providers

```
┌─────────────────────────────────────────────────────────────┐
│                    LLM Provider Support                      │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Commercial APIs:                                            │
│  ┌───────┐ ┌───────┐ ┌───────┐ ┌───────┐ ┌───────┐        │
│  │OpenAI │ │Claude │ │Gemini │ │Cohere │ │ Groq  │        │
│  └───────┘ └───────┘ └───────┘ └───────┘ └───────┘        │
│                                                              │
│  China Providers:                                            │
│  ┌───────┐ ┌───────┐ ┌───────┐ ┌───────┐ ┌───────┐        │
│  │ Qwen  │ │Zhipu  │ │Baichuan│ │Spark  │ │ERNIE  │        │
│  └───────┘ └───────┘ └───────┘ └───────┘ └───────┘        │
│                                                              │
│  Self-hosted:                                                │
│  ┌───────┐ ┌───────┐ ┌───────┐                             │
│  │Ollama │ │ vLLM  │ │LocalAI│                             │
│  └───────┘ └───────┘ └───────┘                             │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### 9.2 Data Source Connectors

| Category | Services |
|----------|----------|
| **Enterprise Wiki** | Confluence, Notion, SharePoint |
| **Communication** | Slack, Discord, Gmail, Teams |
| **Cloud Storage** | Google Drive, Dropbox, S3, WebDAV |
| **Development** | GitHub, Jira |
| **Education** | Moodle |
| **Finance** | TuShare, AkShare, Yahoo Finance |

### 9.3 Search APIs

| Service | Mục đích |
|---------|----------|
| **Tavily** | AI-optimized web search |
| **Google Search** | Web search |
| **Google Scholar** | Academic search |
| **SearXNG** | Meta search |
| **ArXiv** | Academic papers |
| **Wikipedia** | Knowledge lookup |

---

## 10. System Requirements

### 10.1 Minimum Requirements

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| **CPU** | 4 cores | 8+ cores |
| **RAM** | 16 GB | 32+ GB |
| **Disk** | 50 GB | 200+ GB SSD |
| **GPU** | - | NVIDIA 8GB+ VRAM |

### 10.2 Software Requirements

| Software | Version |
|----------|---------|
| **Docker** | 20.10+ |
| **Docker Compose** | 2.0+ |
| **Python** | 3.10-3.12 |
| **Node.js** | 18.20.4+ |

### 10.3 Port Requirements

| Port | Service |
|------|---------|
| 80/443 | Nginx (HTTP/HTTPS) |
| 9380 | RAGFlow API |
| 9381 | Admin Server |
| 9200 | Elasticsearch |
| 5455 | MySQL |
| 6379 | Redis |
| 9000/9001 | MinIO |

---

## 11. Tóm Tắt Tech Stack

### Production Stack

```
Frontend:     React 18 + TypeScript + UmiJS + Ant Design + Tailwind
Backend:      Python 3.11 + Flask/Quart + Peewee
AI/ML:        OpenAI + Sentence Transformers + Detectron2
Database:     MySQL 8 + Elasticsearch 8
Cache:        Redis 7
Storage:      MinIO
Proxy:        Nginx
Container:    Docker + Docker Compose
Orchestration: Kubernetes + Helm
```

### Development Stack

```
Package Mgmt: uv (Python), npm (Node.js)
Linting:      ruff (Python), ESLint (JS/TS)
Testing:      pytest (Python), Jest (JS/TS)
CI/CD:        GitHub Actions
Version Ctrl: Git
```

### Key Architectural Choices

1. **Async-first**: Quart ASGI cho high concurrency
2. **Hybrid Search**: Vector + BM25 trong Elasticsearch
3. **Multi-tenant**: Data isolation per tenant
4. **Pluggable LLMs**: Abstract interface cho nhiều providers
5. **Containerized**: Full Docker deployment
6. **Event-driven**: Background processing với Redis queue
