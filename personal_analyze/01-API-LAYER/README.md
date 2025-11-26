# 01-API-LAYER - API Gateway & Request Handling

## Tổng Quan

API Layer là tầng xử lý HTTP requests của RAGFlow, được xây dựng trên **Quart** (async Flask-compatible framework) với kiến trúc **Blueprint-based modular**.

## Kiến Trúc Tổng Quan

```
┌─────────────────────────────────────────────────────────────────────────┐
│                            CLIENT REQUEST                                │
│                    (Web App / SDK / External API)                        │
└────────────────────────────────┬────────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                           NGINX (Reverse Proxy)                          │
│                         Port 80/443 → Port 9380                          │
└────────────────────────────────┬────────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         QUART ASGI SERVER                                │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────────┐│
│  │                        MIDDLEWARE STACK                              ││
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐               ││
│  │  │  CORS   │→ │ Session │→ │  Auth   │→ │  JSON   │               ││
│  │  │ Handler │  │ Manager │  │  Check  │  │ Encoder │               ││
│  │  └─────────┘  └─────────┘  └─────────┘  └─────────┘               ││
│  └─────────────────────────────────────────────────────────────────────┘│
│                                 │                                        │
│  ┌─────────────────────────────┴─────────────────────────────────────┐  │
│  │                      BLUEPRINT ROUTER                              │  │
│  │                                                                    │  │
│  │  /api/v1/kb/*        → kb_app.py                                  │  │
│  │  /api/v1/document/*  → document_app.py                            │  │
│  │  /api/v1/dialog/*    → dialog_app.py (legacy)                     │  │
│  │  /v1/conversation/*  → conversation_app.py                        │  │
│  │  /v1/canvas/*        → canvas_app.py                              │  │
│  │  /api/v1/file/*      → file_app.py                                │  │
│  │  /v1/user/*          → user_app.py                                │  │
│  │  /v1/llm/*           → llm_app.py                                 │  │
│  │  /api/v1/sdk/*       → sdk/*.py                                   │  │
│  │                                                                    │  │
│  └────────────────────────────────────────────────────────────────────┘  │
│                                 │                                        │
└─────────────────────────────────┼────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                          SERVICE LAYER                                   │
│              (DialogService, DocumentService, KBService, ...)            │
└─────────────────────────────────────────────────────────────────────────┘
```

## Cấu Trúc Thư Mục

```
/api/
├── ragflow_server.py          # Entry point - Server initialization
├── apps/
│   ├── __init__.py            # Blueprint registration & middleware
│   ├── document_app.py        # Document upload/management (708 lines)
│   ├── conversation_app.py    # Chat/conversation API (419 lines)
│   ├── canvas_app.py          # Agent workflow API (609 lines)
│   ├── kb_app.py              # Knowledge base management (699 lines)
│   ├── file_app.py            # File operations (454 lines)
│   ├── dialog_app.py          # Legacy dialog API
│   ├── chunk_app.py           # Chunk management
│   ├── search_app.py          # Search operations
│   ├── llm_app.py             # LLM configuration
│   ├── user_app.py            # User management
│   ├── tenant_app.py          # Multi-tenancy
│   ├── system_app.py          # System configuration
│   ├── connector_app.py       # Data source connectors
│   ├── mcp_server_app.py      # MCP integration
│   ├── auth/                  # Authentication modules
│   │   ├── oauth.py           # OAuth base client
│   │   ├── github.py          # GitHub OAuth
│   │   └── oidc.py            # OpenID Connect
│   └── sdk/                   # SDK API endpoints
│       ├── dataset.py
│       ├── doc.py
│       ├── chat.py
│       └── ...
├── db/
│   ├── db_models.py           # Database models
│   └── services/              # Business logic services
└── utils/
    ├── api_utils.py           # API utilities
    └── validation.py          # Request validation
```

## Files Trong Module Này

| File | Mô Tả |
|------|-------|
| [document_app_analysis.md](./document_app_analysis.md) | Phân tích Document Upload/Management API |
| [conversation_app_analysis.md](./conversation_app_analysis.md) | Phân tích Chat/Conversation API với SSE |
| [canvas_app_analysis.md](./canvas_app_analysis.md) | Phân tích Agent Workflow API |
| [authentication_flow.md](./authentication_flow.md) | Chi tiết JWT/OAuth authentication |
| [request_lifecycle.md](./request_lifecycle.md) | Lifecycle của HTTP request |

## Key Concepts

### 1. Application Factory Pattern

```python
# /api/apps/__init__.py
app = Quart(__name__)
app = cors(app, allow_origin="*")
app.url_map.strict_slashes = False
app.json_encoder = CustomJSONEncoder
app.errorhandler(Exception)(server_error_response)

# Session configuration
app.config["SESSION_TYPE"] = "redis"
app.config["SESSION_REDIS"] = settings.decrypt_database_config(name="redis")
app.config["MAX_CONTENT_LENGTH"] = 1024 * 1024 * 1024  # 1GB max upload
```

### 2. Dynamic Blueprint Registration

```python
def register_page(page_path):
    spec = spec_from_file_location(module_name, page_path)
    page = module_from_spec(spec)
    page.app = app
    page.manager = Blueprint(page_name, module_name)
    sys.modules[module_name] = page
    spec.loader.exec_module(page)

    url_prefix = f"/api/{API_VERSION}" if "sdk" in path else f"/{API_VERSION}/{page_name}"
    app.register_blueprint(page.manager, url_prefix=url_prefix)
```

### 3. Standard Response Format

```python
def get_json_result(code: RetCode = RetCode.SUCCESS, message="success", data=None):
    return jsonify({"code": code, "message": message, "data": data})

# Response codes
class RetCode(IntEnum):
    SUCCESS = 0
    EXCEPTION_ERROR = 100
    ARGUMENT_ERROR = 101
    AUTHENTICATION_ERROR = 109
    UNAUTHORIZED = 401
    SERVER_ERROR = 500
```

## Request Flow

```
1. HTTP Request arrives at Nginx
       ↓
2. Nginx forwards to Quart (port 9380)
       ↓
3. CORS middleware applies headers
       ↓
4. Session middleware loads user session from Redis
       ↓
5. _load_user() validates JWT/API token
       ↓
6. Blueprint router matches URL pattern
       ↓
7. @login_required decorator checks authentication
       ↓
8. @validate_request decorator validates parameters
       ↓
9. Route handler executes business logic
       ↓
10. Service layer processes request
       ↓
11. Response formatted via get_json_result()
       ↓
12. Response sent to client
```

## API Endpoint Summary

### Core APIs

| Blueprint | Base URL | Key Endpoints |
|-----------|----------|---------------|
| kb_app | `/api/v1/kb` | create, list, update, delete, detail |
| document_app | `/api/v1/document` | upload, run, list, rm, change_parser |
| conversation_app | `/v1/conversation` | completion (SSE), set, get, list |
| canvas_app | `/v1/canvas` | set, completion (SSE), debug, templates |
| file_app | `/api/v1/file` | upload, list, get, rm, mv |

### SDK APIs

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/dataset` | POST/GET | Dataset CRUD |
| `/api/v1/document` | POST/GET | Document operations |
| `/api/v1/chat` | POST | Chat completions |
| `/api/v1/session` | POST/GET | Session management |
