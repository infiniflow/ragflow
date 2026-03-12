# RAGFlow Project Instructions for GitHub Copilot

This file provides context, build instructions, and coding standards for the RAGFlow project.
It is structured to follow GitHub Copilot's [customization guidelines](https://docs.github.com/en/copilot/concepts/prompting/response-customization).

## 1. Project Overview
RAGFlow is an open-source RAG (Retrieval-Augmented Generation) engine based on deep document understanding. It is a full-stack application with a Python backend and a React/TypeScript frontend.

- **Version**: 0.24.0
- **Backend**: Python 3.12+ (Quart - async Flask)
- **Frontend**: TypeScript, React, Vite 7.2.7
- **Architecture**: Microservices based on Docker.
  - `api/`: Backend API server (port 9380)
  - `rag/`: Core RAG logic (indexing, retrieval)
  - `deepdoc/`: Document parsing and OCR
  - `agent/`: Agent workflow engine with 22+ components and 19+ tools
  - `web/`: Frontend application
  - `admin/`: Admin service and CLI client (port 9381)
  - `mcp/`: Model Context Protocol server (port 9382)
  - `memory/`: AI Agent memory management service

## 2. Directory Structure

### Core Services
- `api/`: Backend API server (Quart/Flask).
  - `apps/`: API Blueprints (Knowledge Base, Chat, User, etc.)
  - `db/`: Database models and services (Peewee ORM)
  - `utils/`: Utility functions
- `rag/`: Core RAG logic.
  - `llm/`: LLM, Embedding, OCR, Rerank model abstractions
  - `advanced_rag/`: Advanced RAG techniques (RAPTOR, etc.)
  - `graphrag/`: Knowledge Graph RAG
  - `flow/`: RAG pipeline orchestration
  - `nlp/`: NLP utilities
- `deepdoc/`: Document parsing and OCR modules.
  - `parser/`: Document parsers (PDF, DOCX, Excel, PPT, HTML, etc.)
  - `vision/`: Visual processing components

### Agent System
- `agent/`: Agentic reasoning components.
  - `component/`: 22+ components (LLM, retrieval, loop, categorize, switch, etc.)
  - `tools/`: 19+ tools (web search, SQL, code execution, email, etc.)
  - `plugin/`: Plugin system for LLM tools
  - `sandbox/`: Code execution sandbox (gVisor-based)
  - `templates/`: 20+ preset agent templates
  - `canvas.py`: Workflow execution engine

### New Services
- `admin/`: Admin service and CLI client.
  - `server/`: Flask-based admin server (port 9381)
  - `client/`: CLI client for user and service management
- `mcp/`: Model Context Protocol.
  - `server/`: MCP server implementation (port 9382)
  - `client/`: MCP client implementations
- `memory/`: AI Agent memory management.
  - `services/`: Message and query services
  - `utils/`: Connection utilities for ES, Infinity, OceanBase

### Frontend & Deployment
- `web/`: Frontend application (React + Vite + TypeScript).
  - Uses Vite 7.2.7 instead of UmiJS
  - Runs on port 9222 in development
- `docker/`: Docker deployment configurations.
  - `docker-compose-base.yml`: Base services (MySQL, ES, Redis, MinIO, etc.)
  - `docker-compose.yml`: Full stack deployment
- `sdk/`: Python SDK.
- `test/`: Test suites.
  - `unit_test/`: Unit tests
  - `testcases/`: Integration tests (SDK API, HTTP API)
  - `benchmark/`: Performance benchmarks

### Common & Config
- `common/`: Shared utilities and configurations.
  - `doc_store/`: Vector database connections (ES, Infinity, OpenSearch, OceanBase)
  - `settings.py`: Storage factory (MinIO, S3, OSS, Azure, GCS)
- `conf/`: Configuration files.
  - `llm_factories.json`: LLM provider configurations
  - `service_conf.yaml.template`: Service configuration template
  - Mapping files for different vector databases

## 3. Service Ports

| Service | Port | Description |
|---------|------|-------------|
| API Server | 9380 | Main backend API |
| Admin Server | 9381 | Admin management API |
| MCP Server | 9382 | Model Context Protocol |
| Web Dev Server | 9222 | Frontend development server |
| MySQL | 3306 | Metadata database |
| Elasticsearch | 9200 | Vector storage (default) |
| OpenSearch | 9201 | Alternative vector storage |
| Infinity | 23817, 23820, 5432 | Vector database |
| Redis/Valkey | 6379 | Cache and message queue |
| MinIO | 9000, 9001 | Object storage |

## 4. Build Instructions

### Backend (Python)
The project uses **uv** for dependency management.

1. **Setup Environment**:
   ```bash
   uv sync --python 3.12 --all-extras
   uv run download_deps.py
   ```

2. **Download Dependencies**:
   ```bash
   uv run download_deps.py
   # Optional: Use China mirrors
   uv run download_deps.py --china-mirrors
   ```

3. **Run Server**:
   - **Pre-requisite**: Start dependent services (MySQL, ES/Infinity, Redis, MinIO).
     ```bash
     docker compose -f docker/docker-compose-base.yml up -d
     ```
   - **Launch**:
     ```bash
     source .venv/bin/activate
     export PYTHONPATH=$(pwd)
     bash docker/launch_backend_service.sh
     ```
   - **Admin Server** (optional):
     ```bash
     export ENABLE_ADMINSERVER=true
     python admin/server/admin_server.py
     ```
   - **MCP Server** (optional):
     ```bash
     export ENABLE_MCPSERVER=true
     python mcp/server/server.py
     ```

### Frontend (TypeScript/React)
Located in `web/`. Uses Vite 7.2.7 instead of UmiJS.

1. **Install Dependencies**:
   ```bash
   cd web
   npm install
   ```

2. **Run Dev Server**:
   ```bash
   npm run dev
   ```
   Runs on port 9222 by default.

3. **Build for Production**:
   ```bash
   npm run build
   ```

### Docker Deployment
To run the full stack using Docker:
```bash
cd docker
docker compose -f docker-compose.yml up -d
```

### Admin CLI
Install and use the CLI for user and service management:

```bash
# Install
pip install ragflow-cli==0.24.0

# Connect to admin server
ragflow-cli -h 127.0.0.1 -p 9381

# Example commands
LIST SERVICES;
LIST USERS;
CREATE USER <username> <password>;
```

## 5. Testing Instructions

### Backend Tests
The project includes a custom test runner script `run_tests.py`.

- **Run All Tests**:
  ```bash
  python run_tests.py
  ```

- **Run with Coverage**:
  ```bash
  python run_tests.py --coverage
  ```

- **Run in Parallel**:
  ```bash
  python run_tests.py --parallel
  ```

- **Run Specific Test**:
  ```bash
  python run_tests.py --test test/unit_test/common/test_xxx.py
  ```

- **Run with Markers**:
  ```bash
  python run_tests.py --markers "unit"
  ```

### Integration Tests
For integration testing against different backends:

```bash
# Test against Elasticsearch
export HTTP_API_TEST_LEVEL=p2
export HOST_ADDRESS=http://127.0.0.1:9380
pytest -s --tb=short --level=${HTTP_API_TEST_LEVEL} test/testcases/test_sdk_api

# Test against Infinity
DOC_ENGINE=infinity pytest -s --tb=short --level=${HTTP_API_TEST_LEVEL} test/testcases/test_sdk_api
```

### Frontend Tests
- **Run Tests**:
  ```bash
  cd web
  npm run test
  ```

- **Run Storybook**:
  ```bash
  npm run storybook
  ```

## 6. Coding Standards & Guidelines

### Python
- **Formatting**: Use `ruff` for linting and formatting.
  ```bash
  ruff check
  ruff format
  ```

- **Pre-commit**: Ensure pre-commit hooks are installed.
  ```bash
  pre-commit install
  pre-commit run --all-files
  ```

- **Code Style**:
  - Follow PEP 8 guidelines
  - Use type hints where appropriate
  - Add docstrings for public functions and classes
  - Maximum line length: 120 characters

### Frontend (TypeScript/React)
- **Linting**:
  ```bash
  cd web
  npm run lint
  ```

- **Formatting**: Prettier is configured with pre-commit hooks.
  ```bash
  npm run lint
  ```

- **Code Style**:
  - Use functional components with hooks
  - Follow React best practices
  - Use TypeScript for type safety
  - Organize components by feature

### Git Hooks
The project uses Husky for Git hooks:
```bash
# Pre-commit hooks are automatically installed via npm prepare
cd web
npm run prepare
```

## 7. Supported Technologies

### Vector Databases
- Elasticsearch (default)
- OpenSearch
- Infinity
- OceanBase
- SeekDB

### Object Storage
- MinIO (default)
- AWS S3
- Alibaba Cloud OSS
- Azure Blob Storage
- Google Cloud Storage
- OpenDAL

### LLM Providers
Configured in `conf/llm_factories.json`:
- OpenAI (GPT-5, GPT-4, GPT-3.5, o3, etc.)
- Anthropic (Claude)
- Cohere
- Groq
- Ollama
- Google AI (Gemini)
- Azure OpenAI
- DeepSeek
- Qwen (Tongyi Qianwen)
- Baidu Qianfan
- Tencent Cloud
- Volcengine
- Voyage AI
- Replicate
- And many more...

### Embedding Services
- Local TEI (Text Embeddings Inference) server
- Provider APIs (OpenAI, Cohere, etc.)

### Document Parsers
- PDF (with OCR support)
- DOCX/DOC
- Excel/XLSX
- PowerPoint/PPT
- HTML
- Markdown
- JSON
- Images (with OCR)
- Audio (with speech-to-text)

## 8. Development Workflow

### Quick Start
1. Clone the repository
2. Install Python dependencies: `uv sync --python 3.12 --all-extras`
3. Download dependencies: `uv run download_deps.py`
4. Start base services: `docker compose -f docker/docker-compose-base.yml up -d`
5. Start backend: `bash docker/launch_backend_service.sh`
6. Install frontend dependencies: `cd web && npm install`
7. Start frontend: `npm run dev`
8. Access at: http://localhost:9222

### Making Changes
1. Create a feature branch
2. Make your changes following coding standards
3. Run tests: `python run_tests.py`
4. Run linting: `ruff check && ruff format`
5. Test frontend: `cd web && npm run lint`
6. Commit with clear messages

### Troubleshooting
- If `uv` is not installed: `pip install uv`
- If NLTK data is missing: `python -c "import nltk; nltk.download('punkt'); nltk.download('wordnet')"`
- If port conflicts occur: Check `.env` file and modify port configurations
- If Docker services fail: Check `docker logs <service_name>`

## 9. Additional Resources

- **Documentation**: https://ragflow.io/docs
- **GitHub**: https://github.com/infiniflow/ragflow
- **Docker Hub**: https://hub.docker.com/r/infiniflow/ragflow
- **Helm Charts**: `helm/` directory for Kubernetes deployment
- **Example Code**: `example/` directory for SDK and HTTP API examples