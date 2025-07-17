# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Backend Development
- **Start backend services from source**: `bash docker/launch_backend_service.sh`
- **Install Python dependencies**: `uv sync --python 3.10 --all-extras`
- **Download required dependencies**: `uv run download_deps.py`
- **Install pre-commit hooks**: `pre-commit install`
- **Python linting**: `uv run ruff check .`
- **Python formatting**: `uv run ruff format .`

### Frontend Development
- **Install frontend dependencies**: `cd web && npm install`
- **Start frontend dev server**: `cd web && npm run dev`
- **Build frontend**: `cd web && npm run build`
- **Frontend linting**: `cd web && npm run lint`
- **Frontend tests**: `cd web && npm run test`

### Docker Development
- **Start with Docker Compose**: `cd docker && docker compose -f docker-compose.yml up -d`
- **Start with GPU support**: `cd docker && docker compose -f docker-compose-gpu.yml up -d`
- **Start base services only**: `docker compose -f docker/docker-compose-base.yml up -d`

### Testing
- **Run Python tests**: `uv run pytest`
- **Run specific test markers**: `uv run pytest -m p1` (high priority tests)
- **Run frontend tests**: `cd web && npm run test`

## System Architecture

RAGFlow is a comprehensive RAG (Retrieval-Augmented Generation) platform with the following key components:

### Core Architecture
- **Backend**: Python-based Flask API server (`api/ragflow_server.py`)
- **Frontend**: React/TypeScript application built with Umi framework (`web/`)
- **Document Processing**: Deep document understanding via `deepdoc/` module
- **Vector Storage**: Supports Elasticsearch (default) and Infinity
- **Task Processing**: Distributed task execution via `rag/svr/task_executor.py`

### Key Modules
- **`api/`**: REST API endpoints and Flask application
- **`rag/`**: Core RAG functionality, LLM integrations, and document processing
- **`agent/`**: Agent-based conversation and workflow management
- **`deepdoc/`**: Document parsing and understanding (PDF, DOCX, etc.)
- **`graphrag/`**: Graph-based RAG implementations
- **`plugin/`**: Plugin system for extensibility
- **`web/`**: React frontend application
- **`docker/`**: Docker configuration and deployment files

### Database Models
- Database models are defined in `api/db/db_models.py`
- Services layer in `api/db/services/` provides business logic
- Supports MySQL, PostgreSQL, and SQLite

### Document Processing Pipeline
1. **File Upload**: Via `api/apps/file_app.py`
2. **Parsing**: Document-specific parsers in `deepdoc/parser/`
3. **Chunking**: Template-based chunking strategies
4. **Embedding**: Vector embeddings via various providers
5. **Storage**: Elasticsearch/Infinity for search and retrieval

### Agent System
- **Components**: Modular agent components in `agent/component/`
- **Templates**: Pre-built agent templates in `agent/templates/`
- **Canvas**: Visual agent workflow builder
- **Execution**: Component-based execution flow

## Development Environment Setup

### Prerequisites
- Python 3.10-3.12
- Node.js >= 18.20.4
- Docker and Docker Compose
- uv package manager

### Local Development
1. **Install dependencies**:
   ```bash
   uv sync --python 3.10 --all-extras
   uv run download_deps.py
   pre-commit install
   ```

2. **Start infrastructure services**:
   ```bash
   docker compose -f docker/docker-compose-base.yml up -d
   ```

3. **Add hosts entry**:
   ```
   127.0.0.1 es01 infinity mysql minio redis sandbox-executor-manager
   ```

4. **Start backend**:
   ```bash
   source .venv/bin/activate
   export PYTHONPATH=$(pwd)
   bash docker/launch_backend_service.sh
   ```

5. **Start frontend**:
   ```bash
   cd web
   npm install
   npm run dev
   ```

### Configuration Files
- **`docker/.env`**: Environment variables for Docker setup
- **`docker/service_conf.yaml.template`**: Backend service configuration
- **`pyproject.toml`**: Python dependencies and tool configuration
- **`web/package.json`**: Frontend dependencies and scripts

## API Structure

### Main Apps
- **`api/apps/api_app.py`**: Core API endpoints
- **`api/apps/dialog_app.py`**: Chat/dialog management
- **`api/apps/document_app.py`**: Document operations
- **`api/apps/kb_app.py`**: Knowledge base management
- **`api/apps/file_app.py`**: File upload/management

### Services Layer
- Services in `api/db/services/` provide business logic
- Clean separation between API endpoints and business logic
- Database operations abstracted through service classes

## Key Development Patterns

### Error Handling
- Consistent error response format across API endpoints
- Proper HTTP status codes
- Structured error logging

### Authentication
- JWT-based authentication
- OAuth integrations in `api/apps/auth/`
- Role-based access control

### Testing Strategy
- Unit tests for core functionality
- Integration tests for API endpoints
- Test data in `test/` and `sdk/python/test/`
- Pytest with custom markers (p1, p2, p3 for priority)

### Code Quality
- Ruff for Python linting and formatting
- Pre-commit hooks for code quality
- TypeScript strict mode for frontend
- ESLint and Prettier for frontend code style

## Deployment

### Docker Images
- **Full image**: ~9GB with embedding models
- **Slim image**: ~2GB without embedding models
- Multi-stage builds for optimization

### Configuration
- Environment-based configuration
- Template-based config files
- Support for multiple deployment environments

### Scaling
- Horizontal scaling via multiple task executors
- Configurable worker count via `WS` environment variable
- Load balancing support through Docker Compose