# RAGFlow Instructions

Use this file as the local operating guide for the current codebase. Prefer the code and the current CLAUDE.md over any older convention or remembered project shape.

## Upstream and extension policy
- Preserve the ability to merge upstream RAGFlow while retaining local features and data. Keep the standard directory layout and runtime paths; do not reorganize upstream code to enforce a local layering preference.
- Before editing, classify the affected code as an extension, a local change to core, or unchanged upstream. Verify ownership against the accepted upstream commit and history, not directory names alone. Until the provenance registry exists, record the classification and evidence in the change description.
- Apply strict architecture and dead-code rules to owned modules and local integration changes. Unused-in-this-deployment upstream backends and runtimes are not automatically dead code.
- Keep local business logic in its owning module. Put calls to upstream behind narrow adapters and keep core registration/wiring changes small, justified, and tested. Do not create a plugin framework or duplicate the upstream implementation for isolation.
- Do not add mass moves, unrelated formatting, or a package-manager switch to a feature or cleanup task. A necessary core fix is allowed and must have a reason and focused verification.
- Read the [architecture](docs/develop/architecture-and-code-quality-ru.md) for boundaries, the [transition plan](docs/develop/architecture-transition-ru.md) for stages and working instructions, and the [rule catalog](docs/develop/architecture-checks-ru.md) for applicable checks.
- T0 provenance registries and their capture commands are documented in [the inventory report](docs/develop/t0-provenance-report-ru.md). Refresh the main working snapshot with `--write`; after new commits or generated ignored release/runtime resources use `--write --refresh-supporting` to refresh local first-parent and bounded ignored evidence without network fetch. Neither mode certifies behavior or a new upstream base. The [Python T2 tools](docs/develop/t2-python-analysis-ru.md) report imports, imported symbols, parent initialization, direct reverse consumers, unresolved dynamic loading and complexity signals; the report-only runner currently evaluates configured ARC-01/02 static slices, two scoped ARC-03 explicit-cycle checks, isolated domain-import and HTTP-registration probes for Business Documents/OpenMetadata, exact Canvas DSL contracts for the locally owned OpenMetadata component and changed Retrieval tool registries, focused contracts for two Business Documents production adapters, exact locally owned frontend routes/navigation, application bootstrap, provider/router composition and HTTP service factories, exact registration inventories plus lifespan/worker cleanup for ASR, all 15 REST API loaders classified by current T0 as local changes or extensions, the Python API and Flask admin bootstraps, the task-executor registries, CLI handoff, execution modes, unacked replay, special fan-out hydration, non-runnable/error/empty polls, accounting, cleanup/ack, special-task operation logging, heartbeat reporting and stale-worker cleanup, limiter release and shutdown cancellation, the Business Documents worker lifecycle, and the synchronization worker process root plus 37-handler connector dispatch registry. The separate DEAD-01 scanner performs only a static `scan/plan` for explicitly configured owned profiles; `OBSERVED`, zero candidates and `STATIC_CANDIDATE` never confirm deletion, and apply mode does not exist. The [TypeScript observer](docs/develop/t2-typescript-analysis-ru.md) indexes the complete `web/src` TS/TSX graph, browser-entrypoint reachability, eager cycles, reverse consumers and dynamic registration/loading gaps; `OBSERVED` is not an architecture or dead-code PASS, and unresolved computed loading is `INCOMPLETE`. The [Go observer](docs/develop/t2-go-analysis-ru.md) indexes all repository Go sources, profile-specific packages, imports, build constraints, entrypoint reachability and directives without requiring or replacing the Go compiler; its `OBSERVED` is not a build, policy or dead-code PASS. The [runtime-graph classifier](docs/develop/t2-runtime-graph-classification-ru.md) pins production and auxiliary roots, checks every current owned unreachable TypeScript path, separates exact inherited upstream cycle topology from local cycle candidates, validates known dynamic/directive evidence and defines future report-only scopes without creating ignores or a policy PASS. The [Go build-profile planner](docs/develop/t2-go-build-profiles-ru.md) maps every changed `.go` in T0 to an owned `build.sh` build or package-test command; `READY/NOT_EVALUATED` is only a complete plan, while missing Linux/native tools, declared artifacts or structured test evidence is `INCOMPLETE`. The [T3 policy bootstrap](docs/develop/t3-architecture-policy-ru.md) adds an exact changed-path selector and fail-closed aggregate for these existing reports plus an always-triggered workflow named `architecture-policy`; its current `REPORT_ONLY_COMPLETE/NOT_ENABLED` result is not branch protection, and a checker absent from the PR base is explicitly marked as bootstrap/manual review. These tools do not verify historical saved DSL or provide a full architecture/dead-code verdict. Other real queue/DB/Redis paths, unconfigured adapters and policy/dead-code verdicts remain manual. Do not claim they pass, claim that merge is protected before repository settings are verified, or create guessed upstream SHAs or empty baselines.

## Core stance
- Treat obsolete owned code as liability; verify current consumers, dynamic registrations, persisted DSL and external contracts before removal.
- Prefer deletion of obsolete owned paths over shims, deprecated branches, wrapper APIs, and dual-track migration notes. A narrow adapter to a real upstream boundary is not an obsolete wrapper.
- If old and new owned implementations coexist, converge to one path unless an external contract requires both. Do not collapse supported upstream alternatives merely because this deployment selects one.
- Remove dead tests, commented-out code, stale docs, and "move later" notes instead of preserving them.
- Reduce public surface area when a helper can be made private or internal.
- Keep refactors centered on the owning abstraction, not on adjacent compatibility layers.

## Current stack
- Backend: Python 3.13+, Quart-based API server, Peewee ORM, async workers.
- Frontend: React + TypeScript + Vite in `web/`.
- Go: the repository also has a substantial Go module for servers, ingestion, parser/runtime, CLI, and supporting services.
- Runtime services commonly include MySQL/PostgreSQL, Redis, MinIO, and Elasticsearch/Infinity/OpenSearch depending on configuration.

## Code layout to expect
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

## Go-specific rules
- In `internal/ingestion`, `internal/parser`, and `internal/deepdoc`, keep local changes centered on their owner. Collapse confirmed local duplicate paths; preserve the upstream layout and supported profiles.
- Do not add or preserve deprecated Go APIs just to ease migration inside the repo.
- Remove commented-out Go code instead of leaving recovery notes in place.
- Keep package comments and doc comments aligned with the current runtime path, not with migration history.

## Working rules
- Before editing, inspect the nearest code path that actually owns the behavior.
- Keep changes small and local unless the task is explicitly a broader refactor.
- Prefer one implementation path instead of preserving old and new versions side by side.
- Preserve behavior with focused tests when the behavior is still valid; do not keep tests that protect obsolete behavior.
- Remove owned compatibility-only surfaces after checking supported consumers and saved data. Preserve supported external contracts and upstream integration boundaries.
- Comments describe the current path and invariants, not superseded migration history.
- Domain/application code in owned modules must not depend on concrete ORM, network clients, request/session objects or application bootstrap. Check parent-package import side effects before claiming a module is isolated.
- Preserve tenant authorization, transaction boundaries, idempotency and cancellation when extracting a scenario. API and worker must use the same business implementation.
- Select checks by affected behavior and consumers. Missing tools or prerequisites mean incomplete verification, not a pass. Do not run live suites against shared production data.
- During review, use the rule IDs from the catalog. After registry rollout, update the exact affected records; until then, report their information in the change description. Do not widen baseline or ignores to hide new violations.
- For an upstream update, use a separate integration branch/worktree from a complete committed local snapshot. Merge a verified upstream commit; do not rewrite published fork history or use blanket ours/theirs conflict resolution. Test migrations and recovery before release. A new worktree does not include uncommitted work automatically.

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
npm run test:focused -- path/to/test.tsx
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

## Validation preference
- Run the narrowest relevant test, lint, or build command after a change.
- For backend changes, prefer targeted pytest or ruff checks over full-suite runs.
- For frontend changes, prefer the touched-package lint, type-check, or test command.
- For Go changes, prefer package-scoped `bash build.sh --test ...` first.
- Do not default to raw `go test`, `go build`, or IDE Run/Debug for Go in this repo. They often miss the required CGO flags and native static libraries (`office_oxide`, `pdfium-static`, `pdf_oxide`) that `build.sh` wires correctly.
- If Go native builds fail, inspect `build.sh` and `internal/development.md` before changing code. Common environment issues are missing downloaded native deps and missing `lld` on Linux.

## Default review checklist
- Explain every local change to standard core and keep it minimal.
- Remove confirmed obsolete owned code with its registrations, assets and unnecessary dependencies; names such as `legacy` alone are not proof.
- Collapse owned duplicate implementations to one path.
- Drop stale comments and documentation that describe a superseded design.
- Keep exported APIs only when the current code actually needs them.
- Verify applicable architecture, behavior and data rules; include actual commands/results and limits of the evidence.
- Do not reduce coverage scope, disable required checks, or silently extend exceptions to obtain a passing result.

## Continuous cleanup and algorithm simplification
- Follow the [cleanup and simplification protocol](docs/develop/continuous-code-simplification-ru.md) after a coherent change and before completing a development task. For an affected Python owner in the root API/worker profile, run `python tools/quality/inspect_python.py --module <exact-owner-id> --output output/quality/python-analysis.json` after updating its provenance records. For a configured DEAD-01 profile, also run `python tools/quality/check_python_dead_code.py --profile <exact-profile-id> --output output/quality/python-dead-code.json`. Read both reports conservatively: `OBSERVED` is not policy PASS, `STATIC_CANDIDATE` is not a deletion verdict, and exit 2/`INCOMPLETE` requires recording unresolved scope rather than adding ignores. Other profiles and dead-code evidence still need the relevant available tools and explicit limits.
- Inspect changed symbols, their module, callers and dynamic entrypoints for newly dead owned code. Apply a proven, scoped cleanup autonomously within the authorized task; uncertain usage is a finding to investigate, not permission to delete.
- Do not equate an unused binding with a removable computation. Check registration imports, decorators, namespace reflection, resource lifetime, persisted DSL and external contracts before removing code.
- Before applying an automated patch, verify that its source snapshot is still current. On regression, undo only that patch without overwriting other edits; do not reset the shared tree.
- Treat complexity, nesting, repeated scans and I/O amplification as investigation signals. For a significant new signal, simplify the owning scenario or document why the complexity expresses required behavior. Do not split code merely to lower a metric.
- Before simplifying an algorithm, state its outputs, errors, ordering, side effects and invariants. Verify before/after behavior; measure runtime, query counts and memory when traversal, data structures or I/O change. Keep one production implementation.
- CI checks and reports only. Planned cleanup automation does not authorize automatic commits, merges, deploys, product retirement or unrelated upstream refactors.
