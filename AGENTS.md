# RAGFlow Instructions

Use this file as the local operating guide for the current codebase. Prefer the code and the current CLAUDE.md over any older convention or remembered project shape.

## Core Stance
- Treat legacy code as liability, not as a compatibility target.
- Prefer deletion over shims, deprecated branches, wrapper APIs, and dual-track migration notes.
- If old and new implementations coexist, converge to one path unless an external contract forces compatibility.
- Remove dead tests, commented-out code, stale docs, and "move later" notes instead of preserving them.
- Reduce public surface area when a helper can be made private or internal.
- Keep refactors centered on the owning abstraction, not on adjacent compatibility layers.

## Current stack
- Backend: Go implements the API, admin, ingestor, and syncer modes in `cmd/ragflow_server.go`; the CLI runs from `cmd/ragflow-cli.go`. The Python API, admin, and workers are still present and `docker/entrypoint.sh` still defaults to Python when `API_PROXY_SCHEME` is unset. Go-only operation is the migration target, not the current repository state.
- Frontend: React + TypeScript + Vite in `web/`. When working under `web/`, read and follow `web/AGENTS.md` for frontend conventions.
- The Go module contains the server, ingestion, parsing, agent runtime, CLI, and supporting services.
- Runtime services commonly include MySQL/PostgreSQL, Redis, MinIO, and Elasticsearch/Infinity/OpenSearch depending on configuration.

## Go-only target
- Remove all Python code and Python-specific dependencies, scripts, containers, CI jobs, documentation, SDKs, and tests from the repository. Do not add new Python code while this removal is in progress. Replace Python-based preparation of Go native libraries, DeepDoc models, and the BPE table before removing the existing download scripts.
- Move any still-required behavior and test coverage into the Go backend or the frontend before deleting the old implementation. Do not preserve a Python server, compatibility layer, fallback, or dual-backend path.
- Before deleting `rag/`, move the Go DeepDoc `.ort` models and `ocr.res` out of `rag/res/deepdoc/`, then update model discovery, dependency preparation, Docker packaging, and tests together.
- Remove the frontend's Go/Python backend variants and update `web/CLAUDE.md` when the frontend no longer needs them. Until then, treat its dual-backend instructions as migration-only guidance; new frontend work should target the Go API.
- Update Docker's default entrypoint, dependency image, and CI workflows when removing their Python paths. Until then, select Go explicitly with `API_PROXY_SCHEME=go` where that variable controls startup.
- Keep this guide aligned with the checked-in tree. Once the Python directories are deleted, remove their migration inventory below.

## Code Layout to Expect
- `cmd/ragflow_server.go`: Go entrypoint for the API, admin, ingestor, and syncer modes (`--api`, `--admin`, `--ingestor`, `--syncer`).
- `cmd/ragflow-cli.go`: Go CLI entrypoint.
- `api/`, `rag/`, `deepdoc/`, and `agent/`: legacy Python implementation pending deletion. Do not add features or extend dependencies there.
- `internal/`: main Go application code. Important subtrees:
- `internal/agent/`: Go agent runtime, canvas execution, components, tool bindings, workflow helpers.
- `internal/admin/`: Go admin routes, handlers, and services.
- `internal/binding/cpp/`: C++ tokenizer binding built by `build.sh` for the Go server.
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
- `internal/syncer/`: Go file synchronization service and connectors.
- `internal/rag/`: Go retrieval and RAG logic.
- `internal/harness/`: Go runtime harness and supporting execution code.
- `web/`: frontend application.
- `docker/`: local and production compose files.
- `sdk/python/`: legacy Python client SDK pending deletion.
- `ragflow_deps/download_go_deps.py`: current Python helper for Go native libraries and model files; replace it before deleting it. `build.sh` currently reads its ONNX Runtime version pin.
- `test/`: contains Python tests, including tests that import the legacy Python implementation directly. Migrate needed coverage to Go tests or frontend tests, then remove the Python tests; keep only tests for supported code.

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
Go tests are classified by build tag. Tag a test file with `//go:build <tier>` placed before the `package` clause. Run them through `build.sh` so the required CGO configuration and native libraries are available.

| Tier | Build tag | Runs by default? | Needs |
|---|---|---|---|
| Unit | (none) | Yes (`bash build.sh --test`) | Native CGO static libs; no external services — uses in-memory SQLite, miniredis, or `httptest` stubs. |
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
- `unit` (no tag) must stay free of external-service dependencies so `bash build.sh --test` runs without MySQL/MinIO/ES/Infinity/LLM. The native CGO static libraries (`office_oxide`/`pdfium`/`pdf_oxide`) are still required at build time and are wired by `build.sh --test`; that is expected, not an external service.

## Working Rules
- When reviewing documentation or code, inspect the full affected path and report all verifiable findings in one review; do not return after only a few findings and expose further issues in later rounds.
- When handling review comments, independently verify each substantive claim against the current code or tests before accepting, rejecting, or acting on it.
- Before editing, inspect the nearest code path that actually owns the behavior.
- For server changes, work in the Go implementation. If a Python behavior is still needed, port it to Go and test the Go path before removing the Python code.
- Keep changes small and local unless the task is explicitly a broader refactor.
- Prefer one implementation path instead of preserving old and new versions side by side.
- Preserve behavior with focused tests when the behavior is still valid; do not keep tests that protect obsolete behavior.
- If a surface is only there for compatibility, remove it unless the user asks to keep it.
- Do not add new compatibility wording in comments or docs.
- When a maintainer takes over a community PR, a new commit generated by rewriting history (e.g. `merge`, `rebase -i`) must preserve the original author and add the maintainer as co-author (via a `Co-authored-by:` trailer) instead of overwriting the author with the maintainer alone.

## Commands
### Go backend
```bash
bash build.sh --all                         # build native code and Go server
bash build.sh --go                          # build Go server with native library already built
bash build.sh --test ./internal/handler/... # targeted unit tests
bash build.sh --test                        # all unit tests
bash build.sh --test-integration ./...      # integration tests; requires real services
bash build.sh --test-e2e                    # full-pipeline tests; requires real services
bash build.sh --test-manual                 # local opt-in only
bash build.sh --test-all                    # integration + e2e; excludes manual
```

Native libraries and Go DeepDoc models currently come from `ragflow_deps/download_go_deps.py`; see `README_zh.md` for its isolated setup. The Go tokenizer also needs `ragflow_deps/cl100k_base.tiktoken`, which the current Python dependency pipeline provides. These are build/runtime resource preparation steps, not a Python server requirement.

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

## Validation Preference
- Run the narrowest relevant test, lint, or build command after a change.
- For backend changes, first run package-scoped `bash build.sh --test ./path/to/package/...`. Run the integration or e2e tier when the changed behavior needs real services or a full pipeline.
- For frontend changes, prefer the touched-package lint, type-check, or test command.
- Do not default to raw `go test`, `go build`, or IDE Run/Debug for Go in this repo. They often miss the required CGO flags and native static libraries (`office_oxide`, `pdfium-static`, `pdf_oxide`) that `build.sh` wires correctly.
- After a Go build, execute the resulting binary's help command for each changed server mode; a successful link alone does not prove the binary starts. On this host, the original `build.sh` selected default LLD 18 and produced binaries that crashed before Go `main`; linking with LLD 20 started successfully. Verify the linker actually selected on Linux before trusting a new binary.
- If Go native builds fail, inspect `build.sh` and `internal/development.md` before changing code. Common environment issues are missing downloaded native dependencies or an incompatible `ld.lld` on Linux.

## Default review checklist
- Remove instead of retaining `deprecated`, `legacy`, or compatibility-only code.
- Collapse duplicate implementations to one path.
- Drop stale comments and documentation that describe a superseded design.
- Keep exported APIs only when the current code actually needs them.
