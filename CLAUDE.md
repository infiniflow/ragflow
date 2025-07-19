# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

RAGFlow is an open-source RAG (Retrieval-Augmented Generation) engine based on deep document understanding. It's built with Python backend and React frontend, providing a complete RAG workflow with document parsing, knowledge base management, and AI-powered chat capabilities.

## Architecture

### Backend (Python)
- **API Layer**: Flask-based REST APIs in `/api/` directory
- **Core Services**: Document processing, RAG operations, LLM integrations
- **Database**: MySQL with Peewee ORM
- **Search**: Elasticsearch/Infinity for vector storage and retrieval
- **Storage**: MinIO for file storage, Redis for caching

### Frontend (React)
- **Framework**: React with TypeScript using Umi.js
- **UI Library**: Ant Design + Radix UI components
- **State Management**: Zustand + React Query
- **Build Tool**: Umi.js with Tailwind CSS

### Key Components
- **Document Parser** (`deepdoc/`): PDF, DOCX, PPTX, etc. parsing with OCR
- **RAG Engine** (`rag/`): Retrieval, embedding, and LLM integration
- **Agent Framework** (`agent/`): Visual workflow builder for AI tasks
- **GraphRAG** (`graphrag/`): Knowledge graph-based RAG
- **Sandbox** (`sandbox/`): Code execution environment

## Development Setup

### Prerequisites
- Python 3.10-3.12
- Node.js 18.20.4+
- Docker & Docker Compose
- MySQL, Redis, Elasticsearch, MinIO (via Docker)

### Backend Development
```bash
# Install dependencies
uv sync --python 3.10 --all-extras
uv run download_deps.py

# Launch infrastructure services
docker compose -f docker/docker-compose-base.yml up -d

# Add to /etc/hosts
127.0.0.1 es01 infinity mysql minio redis sandbox-executor-manager

# Launch backend
source .venv/bin/activate
export PYTHONPATH=$(pwd)
bash docker/launch_backend_service.sh
```

### Frontend Development
```bash
cd web
npm install
npm run dev
```

### Testing
```bash
# Backend tests
uv run pytest test/ -v

# Frontend tests
cd web && npm test

# Specific test markers
uv run pytest test/ -m p1  # high priority
```

## Key Commands

### Build & Deploy
```bash
# Docker build (slim)
docker build --platform linux/amd64 --build-arg LIGHTEN=1 -f Dockerfile -t ragflow:slim .

# Docker build (full)
docker build --platform linux/amd64 -f Dockerfile -t ragflow:full .

# Run with Docker Compose
docker compose -f docker/docker-compose.yml up -d
```

### Code Quality
```bash
# Backend linting
uv run ruff check .

# Frontend linting
cd web && npm run lint

# Type checking
cd web && npx tsc --noEmit
```

## Configuration Files

### Environment Setup
- `docker/.env`: Core environment variables
- `docker/service_conf.yaml.template`: Service configuration template
- `conf/service_conf.yaml`: Runtime configuration (auto-generated)

### Key Configuration Areas
- **LLM Settings**: Model providers, API keys, endpoints
- **Database**: MySQL connection and credentials
- **Storage**: MinIO/S3 configuration
- **Search**: Elasticsearch/Infinity settings
- **Security**: JWT secrets, encryption keys

## Directory Structure

```
ragflow/
├── api/                    # Backend APIs and services
│   ├── apps/              # REST API endpoints
│   ├── db/                # Database models and services
│   └── utils/             # Utility functions
├── agent/                 # Agent framework and components
├── deepdoc/               # Document parsing and OCR
├── graphrag/              # Knowledge graph RAG
├── rag/                   # Core RAG functionality
├── web/                   # React frontend
├── docker/                # Docker configuration
├── sandbox/               # Code execution environment
├── sdk/                   # Python/JS SDKs
└── test/                  # Test suites
```

## Common Development Tasks

### Adding New LLM Provider
1. Add configuration in `conf/llm_factories.json`
2. Implement provider class in `rag/llm/`
3. Update API endpoints in `api/apps/llm_app.py`
4. Add frontend configuration in `web/src/constants/llm.ts`

### Adding New Document Parser
1. Create parser in `deepdoc/parser/`
2. Add parser registration in `deepdoc/parser/__init__.py`
3. Update chunking templates in `rag/app/`

### Database Migrations
1. Update models in `api/db/db_models.py`
2. Create migration script in `scripts/`
3. Run migration: `uv run python scripts/migrate_tenant_data.py`

## Testing Patterns

### Backend Tests
- Located in `test/` directory
- Use pytest with markers (p1, p2, p3)
- Mock external services with pytest-mock
- Test both HTTP APIs and SDK methods

### Frontend Tests
- Jest for unit tests
- React Testing Library for component tests
- Located in `web/src/` alongside components

## Deployment Notes

### Production Deployment
1. Use `docker-compose.yml` for full deployment
2. Set `RAGFLOW_IMAGE` in `.env` to specific version
3. Configure SSL certificates in `nginx/`
4. Set up monitoring and logging

### Development Environment
1. Use `docker-compose-base.yml` for core services
2. Run backend and frontend separately for hot-reload
3. Use `.env.development` for local overrides

## Troubleshooting

### Common Issues
- **Port conflicts**: Check `SVR_HTTP_PORT` in `.env`
- **Database connection**: Verify MySQL container health
- **File upload issues**: Check MinIO configuration
- **Search failures**: Verify Elasticsearch/Infinity status

### Debug Mode
- Backend: Set `DEBUG=1` in environment
- Frontend: Use browser dev tools, React DevTools
- API logs: Check `ragflow-logs/` directory
- Container logs: `docker logs ragflow-server`

## 🏗️ Project Context

**RAGFlow_A** is a fork from the official RAGFlow project, specifically developed for implementing **multitenant functionality**. This is a separate development environment from the original RAGFlow installation.

### Key Differences
- **RAGFlow_A**: Development fork for multitenant features (current workspace)
- **Original RAGFlow**: Production/stable version running in `F:/10_Ragflow/` via Docker
- **Current Branch**: `feature/multitenant` - active development branch

### ⚠️ Development Environment Guidelines

1. **Separate Environments**: Never mix with original RAGFlow Docker containers
2. **Testing**: All multitenant testing should occur in RAGFlow_A workspace
3. **Database**: Use separate database instances to avoid conflicts
4. **Ports**: Ensure different port configurations for development vs production

### Environment Setup Commands

```bash
# Development environment (RAGFlow_A)
cd F:/04_AI/01_Workplace/ragflow_A

# Original environment (separate)
cd F:/10_Ragflow
```

## Multitenant Development Focus

### Current Status
- ✅ Basic tenant models already exist
- ✅ Migration scripts partially implemented
- 🔄 Phase 1: Data model completion in progress
- 📋 Next: API layer tenant filtering

### Testing Strategy
- Use separate Docker services for multitenant testing
- Configure different ports to avoid conflicts
- Test tenant isolation thoroughly before production deployment