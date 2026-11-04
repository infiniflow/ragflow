# RAGFlow Instructions

Use this file as the local operating guide for the current codebase. Prefer the code and the current AGENTS.md over any older convention or remembered project shape.

## Core Stance
- Treat legacy code as liability, not as a compatibility target.
- Prefer deletion over shims, deprecated branches, wrapper APIs, and dual-track migration notes.
- If old and new implementations coexist, converge to one path unless an external contract forces compatibility.
- Remove dead tests, commented-out code, stale docs, and "move later" notes instead of preserving them.
- Reduce public surface area when a helper can be made private or internal.
- Keep refactors centered on the owning abstraction, not on adjacent compatibility layers.

## Current stack
- Backend: Python 3.13+, Quart-based API server, Peewee ORM, async workers.
- Frontend: React + TypeScript + Vite in `web/`. When working under `web/`, read and follow `web/AGENTS.md` — it holds the frontend conventions (dual-backend Go/Python variants, test placement, styling, data fetching).
- Go: the repository also has a substantial Go module for servers, ingestion, parser/runtime, CLI, and supporting services.
- Runtime services commonly include MySQL/PostgreSQL, Redis, MinIO, and Elasticsearch/Infinity/OpenSearch depending on configuration.

## Code Layout to Expect
- `api/`: Python API server entrypoints, blueprints, services, and database code.
- `rag/`: ingestion, retrieval, LLM integration, and graph RAG logic.
- `deepdoc/`: parsing and OCR.
- `agent/`: workflow canvas, components, tools, and templates.
- `cmd/`: Go entrypoints. `ragflow_main` is the main server/admin/ingestor binary surface; `ragflow-cli` is the CLI entrypoint.
- `internal/`: main Go application code. Important subtrees:
- `internal/agent/`: Go agent runtime, canvas execution, components, tool bindings, workflow helpers.
- `internal/cli/`: CLI parsing, HTTP transport, command execution, response formatting.
- `internal/dao/`: Go data-access layer and persistence-facing helpers.
- `internal/deepdoc/`: Go DeepDOC integrations, especially native-backed PDF/DOCX parsing.
- `internal/engine/`: search/index backends such as Elasticsearch and Infinity.
- `internal/entity/`: shared Go entities and model definitions.
- `internal/handler/`: HTTP handlers and route-facing request logic.
- `internal/ingestion/`: Go ingestion pipeline, canvas adapter, components, wiring, service orchestration.
- `internal/ingestion/component/`: stage implementations such as file/parser/chunker/tokenizer/extractor.
- `internal/ingestion/pipeline/`: DSL translation, canvas-driven execution, checkpoints, resume/run logic.
- `internal/parser/`: parser and chunk libraries used by ingestion and other Go paths.
- `internal/parser/parser/`: typed parse-result parsers for markdown/html/pdf/docx/xlsx/text and related families.
- `internal/parser/chunk/`: chunk operator library and DSL/typed execution helpers.
- `internal/service/`: higher-level business services used by handlers and server flows.
- `internal/storage/`: storage backends and in-memory test doubles.
- `internal/router/`: HTTP route registration.
- `internal/server/`: server bootstrap/config wiring.
- `internal/cpp/`: C++ sources used by native-backed Go features.
- `web/`: frontend application.
- `docker/`: local and production compose files.
- `sdk/` and `test/`: SDK and automated tests.

## Go-Specific Rules
- Treat `internal/ingestion`, `internal/parser`, and `internal/deepdoc` as actively refactored code. Prefer collapsing duplicate paths over preserving transitional wrappers.
- Do not add or preserve deprecated Go APIs just to ease migration inside the repo.
- Remove commented-out Go code instead of leaving recovery notes in place.
- Keep package comments and doc comments aligned with the current runtime path, not with migration history.

## Shared database schema (Go + Python)
The Go services and the Python API write to the same MySQL/PostgreSQL schema, and both own parts of it. Neither is authoritative over the whole.

Each ORM derives index names differently:
- Go (`internal/entity/`): GORM derives `idx_<table>_<column>` for indexes and `uni_<table>_<column>` for unique constraints. Tags in this repo name unique indexes explicitly, e.g. `uniqueIndex:idx_commit_file`.
- Python (`api/db/db_models.py`): Peewee derives `<db_table>_<col1>_<col2>` (truncated with an md5 suffix past 64 chars, see `playhouse.migrate.make_index_name`). There is no `idx_` prefix.

Both sides reconcile by **column set, not by name**: Go's `hasUniqueIndex` (`internal/dao/migration.go`) accepts an existing index only when its full column set equals the requested one, and Python's `ensure_model_indexes` (`api/db/db_models.py`) keys `DB.get_indexes()` on the column tuple. Consequences:

- Divergent names covering the same columns are expected, not a defect. Whichever runtime reaches the database first decides the name. Do not "fix" this by renaming inside a migration; it rewrites live databases for no functional gain.
- Add a shared unique index to **both** sides (Go tag + Peewee `Meta.indexes` or `unique=True`), so each runtime recreates it when absent. Do not add a second index over a column set an existing one already covers.
- In Go, declare single-column uniqueness with a named `uniqueIndex:`, not a bare `unique` tag. A bare `unique` becomes a table-level constraint in DDL, which SQLite materializes as an implicit index that cannot be dropped by name, and which GORM's `MigrateColumnUnique` then tries to drop under its own `uni_<table>_<column>` name — MySQL answers 1091 every startup. `namedIndexMigrator` (`internal/dao/database.go`) suppresses exactly that phantom drop; see `TestNamedIndexMigratorSkipsPhantomUniqueDrop`.
- A Python model existing does not mean Python owns that table's constraints. `pipeline_operation_log` has no `run_count` column and no unique index in `db_models.py`; both are added only by `internal/dao/migration.go`.

## Go Test Tiers
Go tests are classified by build tag so the default `go test ./...` run stays self-contained. Tag a test file with `//go:build <tier>` placed before the `package` clause.

| Tier | Build tag | Runs by default? | Needs |
|---|---|---|---|
| Unit | (none) | Yes (`go test ./...`) | Native CGO static libs (wired by `build.sh --test`); no external services — uses in-memory SQLite, miniredis, or `httptest` stubs. |
| Integration | `integration` | No (`-tags integration`) | A real service: MySQL/MinIO/Elasticsearch/Infinity/LLM. Single component, reasonably fast. |
| E2E | `e2e` | No (`-tags e2e`) | Full cross-component pipeline (ingest → index → retrieve) against real services; heavy/slow. |
| Manual | `manual` | No (`-tags manual`) | Very slow/expensive (deepdoc render/parity/snapshot/bench). **Local opt-in ONLY — never run in CI.** |
| Native (orthogonal) | `cgo` / `!cgo` | `cgo` auto-satisfies under CGO_ENABLED=1 | Native static libs (`office_oxide`/`pdfium`/`pdf_oxide`). Combine with tiers, e.g. `//go:build cgo && integration`. |

Run tiers locally via `build.sh`:
```bash
bash build.sh --test                      # unit tier (no tags)
bash build.sh --test-integration ./...    # integration tier
bash build.sh --test-e2e                  # e2e tier
bash build.sh --test-manual               # manual tier (very slow)
bash build.sh --test-all                  # integration + e2e (never includes manual)
```
Rules:
- New tests that touch a real external service MUST carry `integration`/`e2e`/`manual` — do not rely on `t.Skip` + env vars to soft-isolate them in the default unit run. Keep an env guard as a harmless secondary safety net if desired.
- `manual` is never wired into CI or any automated pipeline.
- `unit` (no tag) must stay free of external-service dependencies so `go test ./...` passes without MySQL/MinIO/ES/Infinity/LLM. The native CGO static libraries (`office_oxide`/`pdfium`/`pdf_oxide`) are still required at build time and are wired automatically by `build.sh --test`; that is expected, not an external service.

## Working Rules
- When reviewing documentation or code, inspect the full affected path and report all verifiable findings in one review; do not return after only a few findings and expose further issues in later rounds.
- When handling review comments, independently verify each substantive claim against the current code or tests before accepting, rejecting, or acting on it.
- Before editing, inspect the nearest code path that actually owns the behavior.
- Keep changes small and local unless the task is explicitly a broader refactor.
- Prefer one implementation path instead of preserving old and new versions side by side.
- Preserve behavior with focused tests when the behavior is still valid; do not keep tests that protect obsolete behavior.
- If a surface is only there for compatibility, remove it unless the user asks to keep it.
- Do not add new compatibility wording in comments or docs.
- When a maintainer takes over a community PR, a new commit generated by rewriting history (e.g. `merge`, `rebase -i`) must preserve the original author and add the maintainer as co-author (via a `Co-authored-by:` trailer) instead of overwriting the author with the maintainer alone.

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
uv run ragflow_deps/download_deps.py
bash build.sh --test ./path/to/package/...
bash build.sh --go
# or build specific binaries:
bash build.sh --all
```

## Validation Preference
- Run the narrowest relevant test, lint, or build command after a change.
- For backend changes, prefer targeted pytest or ruff checks over full-suite runs.
- For frontend changes, prefer the touched-package lint, type-check, or test command.
- For Go changes, prefer package-scoped `bash build.sh --test ...` first.
- Do not default to raw `go test`, `go build`, or IDE Run/Debug for Go in this repo. They often miss the required CGO flags and native static libraries (`office_oxide`, `pdfium-static`, `pdf_oxide`) that `build.sh` wires correctly.
- If Go native builds fail, inspect `build.sh` and `internal/development.md` before changing code. Common environment issues are missing downloaded native deps and missing `lld` on Linux.

## Default review checklist
- Remove instead of retaining `deprecated`, `legacy`, or compatibility-only code.
- Collapse duplicate implementations to one path.
- Drop stale comments and documentation that describe a superseded design.
- Keep exported APIs only when the current code actually needs them.
