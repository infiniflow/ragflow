# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

RAGFlow is an open-source RAG (Retrieval-Augmented Generation) engine based on deep document understanding. It's a full-stack application with:

- Python backend (Quart-based async API server — Quart is the async reimplementation of Flask)
- React/TypeScript frontend (built with vitejs)
- Background task executor workers (separate Python processes, Redis-queue-driven)
- Peewee ORM for database models (not SQLAlchemy)
- Multiple data stores (MySQL/PostgreSQL, Elasticsearch/Infinity/OpenSearch/OceanBase, Redis, MinIO)

## Architecture

### Runtime Architecture

RAGFlow runs as **two separate Python process types**, orchestrated by `docker/launch_backend_service.sh`:

- **API Server** (`api/ragflow_server.py`): Quart-based async HTTP server
- **Task Executors** (`rag/svr/task_executor.py`): Background workers processing documents from Redis streams. Multiple instances run in parallel (controlled by `WS` env var). Each consumes from priority-ordered Redis streams (`te.1.common`, `te.0.common`), using consumer groups for load distribution.

Key consequence: task executors import a different code surface than the API server, so always check which process a module is meant for.

### Backend API (`/api/`)

- **App factory**: `api/apps/__init__.py` — creates the Quart app, configures auth (`login_required` decorator, JWT + API token + session fallback), and dynamically discovers/registers blueprints
- **Two API coexisting patterns**:
  - **RESTful APIs** in `api/apps/restful_apis/` — newer pattern with Pydantic request validation, service layer in `api/apps/services/`, routes registered under `/api/v1`
  - **Legacy APIs** in `api/apps/*_app.py` — older pattern using `@validate_request()`, routes registered under `/v1/<page_name>`
  - **SDK APIs** in `api/apps/sdk/` — registered under `/v1/`
- **Services**: `api/db/services/` — business logic wrapping Peewee model operations. `api/apps/services/` — service layer for the RESTful APIs
- **Models**: `api/db/db_models.py` — Peewee ORM models with pooled MySQL/PostgreSQL connections, custom `JSONField`/`ListField` types, retry logic on connection loss

### Core Processing (`/rag/`)

- **Document ingestion pipeline**: `rag/flow/pipeline.py` — `Pipeline` (extends `agent.canvas.Graph`) orchestrates the ingestion DAG. Components: File (fetches binary from storage), Parser (dispatches to `deepdoc.parser` based on file type), TokenChunker/TitleChunker (splits into chunks), Tokenizer (computes full-text tokens + embedding vectors), Extractor (LLM-based extraction). Data flows via Pydantic `*FromUpstream` schemas.
- **Document parsing**: `deepdoc/` — PDF parsing (vision-based OCR, layout analysis, table structure recognition) and format-specific parsers (DOCX, XLSX, PPT, Markdown, HTML, images). All parsers normalize to a common structure (list of bbox dicts for PDFs, `{text, doc_type_kwd}` for others).
- **LLM Integration**: `rag/llm/` — factory pattern with runtime class discovery. `chat_model.py` (30+ providers via OpenAI SDK and LiteLLM wrappers), `embedding_model.py`, `rerank_model.py`, `cv_model.py` (image-to-text), `sequence2txt_model.py` (ASR), `tts_model.py`. Use `LLMBundle` (from `api.db.services.llm_service`) as the unified interface.
- **Graph RAG**: `rag/graphrag/` — multi-phase pipeline: per-document subgraph extraction (LLM or spaCy NER), Leiden community detection, entity resolution, community summarization. Entities/relations/reports are indexed as chunks alongside regular text chunks, differentiated by `knowledge_graph_kwd`.
- **Search**: `rag/nlp/search.py` — `Dealer` class combines vector similarity + BM25 + re-ranking. `KGSearch` extends it for graph-aware retrieval (entity resolution, n-hop enrichment).

### Agent System (`/agent/`)

- **Execution engine**: `agent/canvas.py` — `Canvas` (extends `Graph`) executes the DAG. Components are run in topological order via `_run_batch`, each receiving upstream outputs as kwargs. Control-flow components (`Categorize`, `Switch`, `Iteration`, `Loop`) dynamically modify the execution path.
- **Component base**: `agent/component/base.py` — `ComponentBase` with `invoke(**kwargs)` / `invoke_async(**kwargs)` lifecycle. Variable references (`{component_id@output_var}` or `{sys.query}`) are resolved from the canvas graph at runtime.
- **Components**: Modular workflow components in `agent/component/` — Begin, LLM, Agent (tool-calling LLM), Categorize, Switch, Iteration, Loop, Message, Invoke (HTTP), and data manipulation nodes. Auto-discovered by `__init__.py`.
- **Templates**: Pre-built agent workflows as JSON DSL files in `agent/templates/`. Each contains a complete `components` DAG, `path`, and `globals`.
- **Tools**: `agent/tools/` — Retrieval, web search (DuckDuckGo, Google, Tavily, SearXNG), academic search (ArXiv, PubMed, Google Scholar, Wikipedia), code execution, SQL execution, email, GitHub, finance data, translation, weather. Tools implement `ToolBase` (extends `ComponentBase`) and produce OpenAI-compatible function descriptors.
- **Plugins**: `agent/plugin/` — plugin system using `pluginlib` for loading external LLM tool plugins from `embedded_plugins/`.

### Frontend (`/web/`)

- React/TypeScript with vitejs framework
- shadcn/ui components (Radix UI primitives + Tailwind CSS)
- `@tanstack/react-query` for server state (cache keys, mutations, invalidation)
- Zustand for local state (primarily agent canvas graph store)
- `react-router` v7 with lazy-loaded pages
- `react-i18next` for i18n (17 languages)
- Axios for HTTP with a layered pattern: endpoint definitions (`utils/api.ts`) → HTTP client (`utils/next-request.ts`) → service layer (`services/`) → query hooks (`hooks/use-*-request.ts`) → components
- `@xyflow/react` for the agent workflow canvas
- `react-hook-form` + `zod` for form validation
- Two API proxy prefixes: `webAPI = '/v1'` (legacy) and `restAPIv1 = '/api/v1'` (RESTful)

## Common Development Commands

### Backend Development

```bash
# Install Python dependencies
uv sync --python 3.13 --all-extras
uv run python3 ragflow_deps/download_deps.py
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

## Database Engines

RAGFlow supports switching between Elasticsearch (default) and Infinity:

- Set `DOC_ENGINE=infinity` in `docker/.env` to use Infinity
- Requires container restart: `docker compose down -v && docker compose up -d`

## Account Password Handling (Critical for Login Flow)

### Password Encryption Pipeline (Browser → Backend → DB Hash)

The login password verification chain is counterintuitive. Understanding this is essential when generating or verifying password hashes.

**Complete flow:**

```
Browser input: "demo"
  → Base64("demo") = "ZGVtbw=="
  → RSA encrypt with conf/public.pem
  → POST to /api/v1/auth/login

Backend DecryptPassword():
  → RSA decrypt with conf/private.pem (passphrase: "Welcome")
  → Returns "ZGVtbw=="  (NOT "demo"!)

VerifyPassword("ZGVtbw==", storedHash)  ← hash is of Base64(password), not raw password
```

**Consequences:**
- The string verified against the hash is **Base64(original password)**, never the raw password
- `DecryptPassword()` handles both RSA-encrypted (browser) and plaintext (curl/API key) inputs: if base64 decode fails, the input is returned as-is for backward compatibility
- Python backend has the same design: `api/utils/crypt.py:decrypt()` RSA-decrypts and returns the Base64-encoded string directly, no further decode

### How to Generate a Valid Password Hash

```bash
# For password "demo" (user input in browser):
# The actual verified string = Base64("demo") = "ZGVtbw=="
# Generate hash with: common.GenerateWerkzeugPasswordHash("ZGVtbw==")
# or use the scrypt template:
# scrypt:32768:8:1$<random-b64-salt>$<hex-hash-of-ZGVtbw==>
```

**To update a user's password in the running database:**
```bash
docker exec docker-mysql-1 mysql -u root -pinfini_rag_flow rag_flow \
  -e "UPDATE user SET password='<hash>' WHERE email='<email>';"
```

### RSA Keys
- `conf/public.pem` — frontend uses this to encrypt Base64(password) before sending
- `conf/private.pem` — backend uses this to decrypt, passphrase `"Welcome"`
- Both referenced in `internal/common/password.go:DecryptPassword()`

### Obtaining an API Token for a Tenant

When testing APIs manually (curl, Go scripts, etc.), you need a valid auth token. The login endpoint returns **two different tokens**:

| Field | Format | Purpose |
|-------|--------|---------|
| `response.body.data.access_token` | Raw UUID | Stored in DB, NOT used for API auth |
| `response.Header["Authorization"]` | itsdangerous-signed token | Used as `Bearer <token>` for all subsequent API requests |

**How to obtain the correct token:**

```bash
# Step 1: Construct the encrypted password
# Raw password → Base64 → RSA encrypt with conf/public.pem
PASSWORD="demo"
PASSWORD_B64=$(echo -n "$PASSWORD" | base64)

# Step 2: POST to login (use RSA encryption — easiest via a Go/Python script)
# Response header contains: Authorization: <itsdangerous-signed-token>

# Step 3: Use the Authorization header value for all API requests
curl -H "Authorization: <itsdangerous-signed-token>" \
  http://127.0.0.1:9222/api/v1/agents
```

**Go snippet (complete login + token extraction):**

```go
// Login
passwordB64 := base64.StdEncoding.EncodeToString([]byte(password))
pubData, _ := os.ReadFile("conf/public.pem")
block, _ := pem.Decode(pubData)
pubKey, _ := x509.ParsePKIXPublicKey(block.Bytes)
ciphertext, _ := rsa.EncryptPKCS1v15(rand.Reader, pubKey.(*rsa.PublicKey), []byte(passwordB64))
encryptedB64 := base64.StdEncoding.EncodeToString(ciphertext)

body, _ := json.Marshal(map[string]string{"email": email, "password": encryptedB64})
resp, _ := http.Post(baseURL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))

// KEY: use the Authorization header, NOT body.access_token
authToken := resp.Header.Get("Authorization")

// Use for API calls
req, _ := http.NewRequest("GET", baseURL+"/api/v1/agents", nil)
req.Header.Set("Authorization", authToken)
```

**The raw `access_token` (UUID) in the response body** is the internal DB token used only by the `itsdangerous` middleware to verify the signed token — it is never passed directly in API Authorization headers.

---

## Agent Run E2E Tests

### Running the Tests

```bash
# Run all agent run e2e tests (in-memory SQLite + miniredis, no Docker needed)
cd /home/zhichyu/github.com/infiniflow/ragflow
go test -count=1 -v -run 'TestRunAgent_RealCanvas|TestRunAgent_RunTracker' ./internal/service/
```

### Test Architecture

All e2e tests live in `internal/service/agent_run_e2e_test.go`. They exercise the full production chain:

```
loadCanvasForUser → versionDAO.GetLatest → decodeCanvasFromDSL →
canvas.Compile → cc.Workflow.Invoke → answer extraction
```

**Test isolation**: Each test stands up its own in-memory SQLite DB (pushed as `dao.DB`), seeds User/Tenant/UserCanvas/UserCanvasVersion rows, and tears down in `t.Cleanup`. Tests use **miniredis** for Redis-backed CheckPointStore + RunTracker — no external services needed.

**Key test helpers:**
- `makeCanvasWithDSL(t, canvasID, userID, tenantID, versionID, dsl)` — seeds all required DB rows
- `drainAgentEvents(t, events)` — drains the `<-chan canvas.RunEvent` channel, buckets results into `messages`, `waiting`, `errors_`, `done`
- `newRunTrackerForTest(t, ttl)` — wires a `canvas.RunTracker` against in-memory miniredis

**Existing e2e tests:**

| Test | What it covers |
|------|---------------|
| `TestRunAgent_RealCanvas_BeginMessage` | Happy path: Begin→Message, verifies `"{{sys.query}}"` resolution |
| `TestRunAgent_RealCanvas_WaitForUserResume` | Resume path: Begin→Message→UserFillUp, two-run cycle |
| `TestRunAgent_RealCanvas_CompileFails` | Error path: unknown component name → sanitized error |
| `TestRunAgent_RealCanvas_InvokeFails` | Error path: unresolvable template ref |
| `TestRunAgent_RunTracker_AttachCheckpoint_CallSequence` | Production boot: Start→AttachCheckpoint→MarkSucceeded with Redis/miniredis |

**Test DSL data files** are in `internal/agent/dsl/testdata/`:
- `agent_msg.json` — Agent+Message with Begin, LLM-powered agent component
- `all.json` — Complex: Begin→UserFillUp→Switch→Loop→Message
- `switch.json`, `resume.json`, `browser.json`, `subagent.json`, etc.

**Handler-level SSE streaming tests** in `internal/handler/agent_test.go` use a `stubChatRunner` that emits pre-configured `canvas.RunEvent` values without a real DB or eino runner, verifying:
- SSE `Content-Type: text/event-stream`
- `data: {...}\n\n` framing
- Trailing `data: [DONE]\n\n` terminator
- OpenAI-compatible non-stream `choices` response shape

**Important**: `_ "ragflow/internal/agent/component"` (blank import in test) is required — it triggers `init()` to register all component factories. Without it, `canvas.Compile` fails to resolve any component type.

---

## Development Environment Requirements

- Python 3.10-3.13
- Node.js >=18.20.4
- Docker & Docker Compose
- uv package manager
- 16GB+ RAM, 50GB+ disk space

1. Think before acting. Read existing files before writing code.
2. Be concise in output but thorough in reasoning.
3. Prefer editing over rewriting whole files.
4. Do not re-read files you have already read.
5. Test your code before declaring done.
6. No sycophantic openers or closing fluff.
7. Keep solutions simple and direct.
8. User instructions always override this file.
