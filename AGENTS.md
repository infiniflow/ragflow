# RAGFlow Instructions

Use this file as the local operating guide for the current codebase. Prefer the code and the current CLAUDE.md over any older convention or remembered project shape.

## Core stance
- Treat legacy code as liability, not as a compatibility target.
- Prefer deletion over shims, deprecated branches, wrapper APIs, and dual-track migration notes.
- If old and new implementations coexist, converge to one path unless an external contract forces compatibility.
- Remove dead tests, commented-out code, stale docs, and "move later" notes instead of preserving them.
- Reduce public surface area when a helper can be made private or internal.
- Keep refactors centered on the owning abstraction, not on adjacent compatibility layers.

## Current stack
- Backend: Python 3.13+, Quart-based API server, Peewee ORM, async workers.
- Frontend: React + TypeScript + Vite in `web/`.
- Go: the repository also has a Go module for CLI and service code.
- Runtime services commonly include MySQL/PostgreSQL, Redis, MinIO, and Elasticsearch/Infinity/OpenSearch depending on configuration.

## Code layout to expect
- `api/`: Python API server entrypoints, blueprints, services, and database code.
- `rag/`: ingestion, retrieval, LLM integration, and graph RAG logic.
- `deepdoc/`: parsing and OCR.
- `agent/`: workflow canvas, components, tools, and templates.
- `internal/` and `cmd/`: Go services, parsers, and CLIs.
- `web/`: frontend application.
- `docker/`: local and production compose files.
- `sdk/` and `test/`: SDK and automated tests.

## Working rules
- Before editing, inspect the nearest code path that actually owns the behavior.
- Keep changes small and local unless the task is explicitly a broader refactor.
- Prefer one implementation path instead of preserving old and new versions side by side.
- Preserve behavior with focused tests when the behavior is still valid; do not keep tests that protect obsolete behavior.
- If a surface is only there for compatibility, remove it unless the user asks to keep it.
- Do not add new compatibility wording in comments or docs.

## Commands
### Backend
```bash
uv sync --python 3.13 --all-extras
uv run python3 ragflow_deps/download_deps.py
docker compose -f docker/docker-compose-base.yml up -d
source .venv/bin/activate
export PYTHONPATH=$(pwd)
bash docker/launch_backend_service.sh
uv run pytest
ruff check
ruff format
```

### Frontend
```bash
cd web
npm install
npm run dev
npm run build
npm run lint
npm run test
npm run type-check
```

### Go
```bash
go test ./...
go build ./cmd/...
```

## Validation preference
- Run the narrowest relevant test, lint, or build command after a change.
- For backend changes, prefer targeted pytest or ruff checks over full-suite runs.
- For frontend changes, prefer the touched-package lint, type-check, or test command.
- For Go changes, prefer package-scoped `go test` or `go build` first.

## Default review checklist
- Remove instead of retaining `deprecated`, `legacy`, or compatibility-only code.
- Collapse duplicate implementations to one path.
- Drop stale comments and documentation that describe a superseded design.
- Keep exported APIs only when the current code actually needs them.
