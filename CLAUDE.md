# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

RAGFlow is an open-source RAG (Retrieval-Augmented Generation) engine based on deep document understanding. It's a full-stack application with:

- Python backend (Flask-based API server)
- React/TypeScript frontend (built with UmiJS)
- Microservices architecture with Docker deployment
- Multiple data stores (MySQL, Elasticsearch/Infinity, Redis, MinIO)

## Architecture

### Backend (`/api/`)

- **Main Server**: `api/ragflow_server.py` - Flask application entry point
- **Apps**: Modular Flask blueprints in `api/apps/` for different functionalities:
  - `kb_app.py` - Knowledge base management
  - `dialog_app.py` - Chat/conversation handling
  - `document_app.py` - Document processing
  - `canvas_app.py` - Agent workflow canvas
  - `file_app.py` - File upload/management
- **Services**: Business logic in `api/db/services/`
- **Models**: Database models in `api/db/db_models.py`

### Core Processing (`/rag/`)

- **Document Processing**: `deepdoc/` - PDF parsing, OCR, layout analysis
- **LLM Integration**: `rag/llm/` - Model abstractions for chat, embedding, reranking
- **RAG Pipeline**: `rag/flow/` - Chunking, parsing, tokenization
- **Graph RAG**: `graphrag/` - Knowledge graph construction and querying

### Agent System (`/agent/`)

- **Components**: Modular workflow components (LLM, retrieval, categorize, etc.)
- **Templates**: Pre-built agent workflows in `agent/templates/`
- **Tools**: External API integrations (Tavily, Wikipedia, SQL execution, etc.)

### Frontend (`/web/`)

- React/TypeScript with UmiJS framework
- Ant Design + shadcn/ui components
- State management with Zustand
- Tailwind CSS for styling

## Common Development Commands

### Backend Development

```bash
# Install Python dependencies
uv sync --python 3.12 --all-extras
uv run scripts/download_deps.py
pre-commit install

# Start dependent services
docker compose -f docker/docker-compose-base.yml up -d

# Run backend (requires services to be running)
source .venv/bin/activate
export PYTHONPATH=$(pwd)
bash docker/launch_backend_service.sh

# Run tests
uv run pytest

# Linting
ruff check
ruff format
```

### Frontend Development

```bash
cd web
npm install
npm run dev        # Development server
npm run build      # Production build
npm run lint       # ESLint
npm run test       # Jest tests
```

### Docker Development

```bash
# Full stack with Docker
cd docker
docker compose -f docker-compose.yml up -d

# Check server status
docker logs -f ragflow-server

# Rebuild images
docker build --platform linux/amd64 -f Dockerfile -t infiniflow/ragflow:nightly .
```

## Key Configuration Files

- `docker/.env` - Environment variables for Docker deployment
- `docker/service_conf.yaml.template` - Backend service configuration
- `pyproject.toml` - Python dependencies and project configuration
- `web/package.json` - Frontend dependencies and scripts

## Testing

- **Python**: pytest with markers (p1/p2/p3 priority levels)
- **Frontend**: Jest with React Testing Library
- **API Tests**: HTTP API and SDK tests in `test/` and `sdk/python/test/`

### Test Architecture

RAGFlow uses a **service-layer testing approach** aligned with [AGENTS.md](AGENTS.md) modularization:

```
test/
├── integration/          # Workflows (dataset lifecycle, chat, etc.)
├── api_contract/        # API validation (response schemas, contracts)
├── unit_test/           # Service & utility unit tests
│   ├── api_db/          # Database layer tests
│   ├── services/        # Service logic tests
│   └── common/          # Utility function tests
└── testcases/           # Legacy - shared fixtures & helpers
```

**Key principle:** Tests validate **business logic** (service layer), not API structure. This makes tests resilient to API refactoring.

#### Writing Integration Tests

Integration tests document user workflows:

```python
# test/integration/test_dataset_lifecycle.py
"""Dataset workflow: create → upload → parse → retrieve → delete"""

class TestDatasetLifecycle:
    def test_complete_workflow(self, api_client):
        # 1. Create dataset
        dataset = api_client.create_dataset({"name": "test"})
        
        # 2. Upload document
        api_client.upload_document(dataset["id"], file)
        
        # 3. Parse document
        api_client.parse_documents(dataset["id"])
        
        # 4. Verify workflow succeeded
        assert api_client.get_dataset(dataset["id"]) is not None
```

**Test file structure:**
- One test file per **workflow** (not per endpoint)
- Each test class represents a related set of workflows
- Use existing fixtures from `test/testcases/conftest.py`
- Import helpers from `test/testcases/test_http_api/common.py`

#### Running Tests

```bash
# All tests
uv run pytest

# Only integration tests
uv run pytest test/integration/

# Only API contracts
uv run pytest test/api_contract/

# With priority markers
uv run pytest -m p1        # High priority tests only
uv run pytest -m "not p3"  # Skip low priority tests

# Specific file
uv run pytest test/integration/test_dataset_lifecycle.py -v
```

**For detailed test architecture**, see [test/TEST_ARCHITECTURE.md](test/TEST_ARCHITECTURE.md).

## Database Engines

RAGFlow supports switching between Elasticsearch (default) and Infinity:

- Set `DOC_ENGINE=infinity` in `docker/.env` to use Infinity
- Requires container restart: `docker compose down -v && docker compose up -d`

## Development Environment Requirements

- Python 3.10-3.12
- Node.js >=18.20.4
- Docker & Docker Compose
- uv package manager
- 16GB+ RAM, 50GB+ disk space

## Optional Dependencies (Docker)

RAGFlow supports optional dependencies that can be installed conditionally at container startup. This follows the modularization principles in [AGENTS.md](AGENTS.md).

### Adding Optional Dependencies

Use the `ensure_pip_dependency()` function in [`docker/entrypoint.sh`](docker/entrypoint.sh):

```bash
# Signature: ensure_pip_dependency <package_name> <package_spec> <env_flag>
ensure_pip_dependency "docling" "docling==2.58.0" "USE_DOCLING"
```

**Features:**

- ✅ **Persistent caching**: Package installed once per volume lifecycle, not on every container start
- ✅ **Cache validation**: Automatically detects and recovers from corrupted caches
- ✅ **Environment-controlled**: Disable with `USE_DOCLING=false`
- ✅ **Backward compatible**: No changes required for existing deployments

**How it works:**

1. Checks environment flag (`USE_DOCLING`). If `false`, skip installation
2. Checks cache marker (`/opt/ragflow/.deps/{package_name}-installed`)
3. If marker exists, verifies package actually imports (auto-recovery)
4. If not cached, installs via pip with persistent cache directory
5. Creates marker only after successful installation

**Adding a new optional dependency:**

```bash
# 1. Add to docker/entrypoint.sh (before service startup)
ensure_pip_dependency "my_package" "my_package==1.0.0" "USE_MY_PACKAGE"

# 2. Add environment variable to docker/.env
USE_MY_PACKAGE=true

# 3. Update docker-compose.yml if needed (for volume persistence)
```
