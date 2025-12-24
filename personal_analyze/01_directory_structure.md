# RAGFlow - Cấu Trúc Thư Mục

## Tổng Quan

RAGFlow (v0.22.1) là một RAG (Retrieval-Augmented Generation) engine mã nguồn mở dựa trên deep document understanding. Dự án được xây dựng với kiến trúc full-stack bao gồm Python backend và React/TypeScript frontend.

## Cấu Trúc Thư Mục Chi Tiết

```
ragflow/
│
├── api/                          # [BACKEND] Flask API Server
│   ├── ragflow_server.py         # Entry point chính
│   ├── settings.py               # Cấu hình server
│   ├── constants.py              # Hằng số API (API_VERSION = "v1")
│   ├── validation.py             # Request validation
│   │
│   ├── apps/                     # Flask Blueprints - API endpoints
│   │   ├── kb_app.py             # Knowledge Base management
│   │   ├── document_app.py       # Document processing
│   │   ├── dialog_app.py         # Chat/Dialog handling
│   │   ├── canvas_app.py         # Agent workflow canvas
│   │   ├── file_app.py           # File upload/management
│   │   ├── chunk_app.py          # Document chunking
│   │   ├── conversation_app.py   # Conversation management
│   │   ├── search_app.py         # Search functionality
│   │   ├── system_app.py         # System configuration
│   │   ├── llm_app.py            # LLM model management
│   │   ├── connector_app.py      # Data source connectors
│   │   ├── mcp_server_app.py     # MCP server integration
│   │   ├── langfuse_app.py       # Langfuse observability
│   │   ├── api_app.py            # API key management
│   │   ├── plugin_app.py         # Plugin management
│   │   ├── tenant_app.py         # Multi-tenancy
│   │   ├── user_app.py           # User management
│   │   │
│   │   ├── auth/                 # Authentication modules
│   │   │   ├── oauth.py          # OAuth base
│   │   │   ├── github.py         # GitHub OAuth
│   │   │   └── oidc.py           # OpenID Connect
│   │   │
│   │   └── sdk/                  # SDK REST API endpoints
│   │       ├── dataset.py        # Dataset API
│   │       ├── doc.py            # Document API
│   │       ├── chat.py           # Chat API
│   │       ├── session.py        # Session API
│   │       ├── files.py          # File API
│   │       ├── agents.py         # Agent API
│   │       └── dify_retrieval.py # Dify integration
│   │
│   ├── db/                       # Database layer
│   │   ├── db_models.py          # SQLAlchemy/Peewee models (54KB)
│   │   ├── db_utils.py           # Database utilities
│   │   ├── init_data.py          # Initial data seeding
│   │   ├── runtime_config.py     # Runtime configuration
│   │   │
│   │   ├── services/             # Business logic services
│   │   │   ├── user_service.py           # User operations
│   │   │   ├── dialog_service.py         # Dialog logic (37KB)
│   │   │   ├── document_service.py       # Document processing (39KB)
│   │   │   ├── file_service.py           # File handling (22KB)
│   │   │   ├── knowledgebase_service.py  # KB management (21KB)
│   │   │   ├── task_service.py           # Task queue (20KB)
│   │   │   ├── canvas_service.py         # Canvas logic (12KB)
│   │   │   ├── conversation_service.py   # Conversation handling
│   │   │   ├── connector_service.py      # Connector management
│   │   │   ├── llm_service.py            # LLM operations
│   │   │   ├── search_service.py         # Search operations
│   │   │   └── api_service.py            # API token service
│   │   │
│   │   └── joint_services/       # Cross-service operations
│   │
│   └── utils/                    # API utilities
│       ├── api_utils.py          # API helpers
│       ├── file_utils.py         # File utilities
│       ├── crypt.py              # Encryption
│       └── log_utils.py          # Logging
│
├── rag/                          # [CORE] RAG Processing Engine
│   ├── settings.py               # RAG configuration
│   ├── raptor.py                 # RAPTOR algorithm
│   ├── benchmark.py              # Performance testing
│   │
│   ├── llm/                      # LLM Model Abstractions
│   │   ├── chat_model.py         # Chat LLM interface
│   │   ├── embedding_model.py    # Embedding models
│   │   ├── rerank_model.py       # Reranking models
│   │   ├── cv_model.py           # Computer vision
│   │   ├── tts_model.py          # Text-to-speech
│   │   └── sequence2txt_model.py # Sequence to text
│   │
│   ├── flow/                     # RAG Pipeline
│   │   ├── pipeline.py           # Main pipeline
│   │   ├── file.py               # File processing
│   │   │
│   │   ├── parser/               # Document parsing
│   │   │   ├── parser.py
│   │   │   └── schema.py
│   │   │
│   │   ├── extractor/            # Information extraction
│   │   │   ├── extractor.py
│   │   │   └── schema.py
│   │   │
│   │   ├── tokenizer/            # Text tokenization
│   │   │   ├── tokenizer.py
│   │   │   └── schema.py
│   │   │
│   │   ├── splitter/             # Document chunking
│   │   │   ├── splitter.py
│   │   │   └── schema.py
│   │   │
│   │   └── hierarchical_merger/  # Hierarchical merging
│   │       ├── hierarchical_merger.py
│   │       └── schema.py
│   │
│   ├── app/                      # RAG application logic
│   ├── nlp/                      # NLP utilities
│   ├── utils/                    # RAG utilities
│   └── prompts/                  # LLM prompt templates
│
├── deepdoc/                      # [DOCUMENT] Deep Document Understanding
│   ├── parser/                   # Multi-format parsers
│   │   ├── pdf_parser.py         # PDF with layout analysis
│   │   ├── docx_parser.py        # Word documents
│   │   ├── ppt_parser.py         # PowerPoint
│   │   ├── excel_parser.py       # Excel spreadsheets
│   │   ├── html_parser.py        # HTML pages
│   │   ├── markdown_parser.py    # Markdown files
│   │   ├── json_parser.py        # JSON data
│   │   ├── txt_parser.py         # Plain text
│   │   ├── figure_parser.py      # Image/figure extraction
│   │   │
│   │   └── resume/               # Resume parsing
│   │       ├── step_one.py
│   │       └── step_two.py
│   │
│   └── vision/                   # Computer vision modules
│
├── agent/                        # [AGENT] Agentic Workflow System
│   ├── canvas.py                 # Canvas orchestration (25KB)
│   ├── settings.py               # Agent configuration
│   │
│   ├── component/                # Workflow components
│   │   ├── begin.py              # Workflow start
│   │   ├── llm.py                # LLM invocation
│   │   ├── agent_with_tools.py   # Agent with tools
│   │   ├── retrieval.py          # Document retrieval
│   │   ├── categorize.py         # Message categorization
│   │   ├── message.py            # Message handling
│   │   ├── webhook.py            # Webhook triggers
│   │   ├── iteration.py          # Loop iteration
│   │   └── variable_assigner.py  # Variable assignment
│   │
│   ├── tools/                    # External tool integrations
│   │   ├── tavily.py             # Web search
│   │   ├── arxiv.py              # Academic papers
│   │   ├── github.py             # GitHub API
│   │   ├── google.py             # Google Search
│   │   ├── wikipedia.py          # Wikipedia
│   │   ├── email.py              # Email sending
│   │   ├── code_exec.py          # Code execution
│   │   └── yahoofinance.py       # Financial data
│   │
│   └── templates/                # Pre-built workflows
│
├── graphrag/                     # [GRAPH] Knowledge Graph RAG
│   ├── entity_resolution.py      # Entity linking (12KB)
│   ├── search.py                 # Graph search (14KB)
│   ├── utils.py                  # Graph utilities (23KB)
│   ├── general/                  # General graph operations
│   └── light/                    # Lightweight implementations
│
├── web/                          # [FRONTEND] React/TypeScript
│   ├── package.json              # NPM dependencies (172 packages)
│   ├── .umirc.ts                 # UmiJS configuration
│   ├── tailwind.config.js        # Tailwind CSS config
│   │
│   └── src/
│       ├── pages/                # UmiJS page routes
│       │   ├── admin/            # Admin dashboard
│       │   ├── dataset/          # Knowledge base management
│       │   ├── datasets/         # Datasets list
│       │   ├── knowledge/        # Knowledge management
│       │   ├── next-chats/       # Chat interface
│       │   ├── next-searches/    # Search interface
│       │   ├── document-viewer/  # Document preview
│       │   ├── login/            # Authentication
│       │   └── register/         # User registration
│       │
│       ├── components/           # React components
│       │   ├── file-upload-modal/
│       │   ├── pdf-drawer/
│       │   ├── prompt-editor/
│       │   ├── document-preview/
│       │   └── ui/               # Shadcn/UI components
│       │
│       ├── services/             # API client services
│       ├── hooks/                # React hooks
│       ├── interfaces/           # TypeScript interfaces
│       ├── utils/                # Utility functions
│       ├── constants/            # Constants
│       └── locales/              # i18n translations
│
├── common/                       # [SHARED] Common Utilities
│   ├── settings.py               # Main configuration (11KB)
│   ├── config_utils.py           # Config utilities
│   ├── connection_utils.py       # Database connections
│   ├── constants.py              # Global constants
│   ├── exceptions.py             # Exception definitions
│   │
│   ├── Utilities:
│   │   ├── log_utils.py          # Logging setup
│   │   ├── file_utils.py         # File operations
│   │   ├── string_utils.py       # String utilities
│   │   ├── token_utils.py        # Token operations
│   │   └── time_utils.py         # Time utilities
│   │
│   └── data_source/              # Data source connectors
│       ├── confluence_connector.py (81KB)
│       ├── notion_connector.py (25KB)
│       ├── slack_connector.py (22KB)
│       ├── gmail_connector.py
│       ├── discord_connector.py
│       ├── sharepoint_connector.py
│       ├── dropbox_connector.py
│       └── google_drive/
│
├── sdk/                          # [SDK] Python Client Library
│   └── python/
│       └── ragflow_sdk/          # SDK implementation
│
├── mcp/                          # [MCP] Model Context Protocol
│   ├── server/                   # MCP server
│   │   └── server.py
│   └── client/                   # MCP client
│       └── client.py
│
├── admin/                        # [ADMIN] Admin Interface
│   ├── server/                   # Admin backend
│   └── client/                   # Admin frontend
│
├── plugin/                       # [PLUGIN] Plugin System
│   ├── plugin_manager.py         # Plugin management
│   ├── llm_tool_plugin.py        # LLM tool plugins
│   └── embedded_plugins/         # Built-in plugins
│
├── docker/                       # [DEPLOYMENT] Docker Configuration
│   ├── docker-compose.yml        # Main compose file
│   ├── docker-compose-base.yml   # Base services
│   ├── .env                      # Environment variables
│   ├── entrypoint.sh             # Container entry
│   ├── service_conf.yaml.template # Service config
│   ├── nginx/                    # Nginx configuration
│   │   └── nginx.conf
│   └── init.sql                  # Database init
│
├── conf/                         # [CONFIG] Configuration Files
│   ├── llm_factories.json        # LLM providers
│   ├── mapping.json              # Field mappings
│   ├── service_conf.yaml         # Service configuration
│   ├── private.pem               # RSA private key
│   └── public.pem                # RSA public key
│
├── test/                         # [TEST] Testing Suite
│   ├── unit_test/                # Unit tests
│   │   └── common/               # Common utilities tests
│   │
│   └── testcases/                # Integration tests
│       ├── test_http_api/        # HTTP API tests
│       ├── test_sdk_api/         # SDK tests
│       └── test_web_api/         # Web API tests
│
├── example/                      # [EXAMPLES] Usage Examples
│   ├── http/                     # HTTP API examples
│   └── sdk/                      # SDK examples
│
├── intergrations/                # [INTEGRATIONS] Third-party
│   ├── chatgpt-on-wechat/        # WeChat integration
│   ├── extension_chrome/         # Chrome extension
│   └── firecrawl/                # Web scraping
│
├── agentic_reasoning/            # [REASONING] Advanced reasoning
├── sandbox/                      # [SANDBOX] Code execution
├── helm/                         # [K8S] Kubernetes Helm charts
├── docs/                         # [DOCS] Documentation
│
├── pyproject.toml                # Python project config
├── CLAUDE.md                     # Development guidelines
└── README.md                     # Project overview
```

## Mô Tả Chi Tiết Các Thư Mục Chính

### 1. `/api/` - Backend API Server
- **Vai trò**: Xử lý tất cả HTTP requests, authentication, và business logic
- **Framework**: Flask/Quart (async ASGI)
- **Port mặc định**: 9380
- **Entry point**: `ragflow_server.py`

### 2. `/rag/` - RAG Processing Engine
- **Vai trò**: Xử lý pipeline RAG từ document parsing đến retrieval
- **Chức năng chính**:
  - Document parsing và extraction
  - Text tokenization
  - Semantic chunking
  - Embedding generation
  - Reranking

### 3. `/deepdoc/` - Document Understanding
- **Vai trò**: Deep document parsing với layout analysis
- **Hỗ trợ formats**: PDF, Word, PPT, Excel, HTML, Markdown, JSON, TXT
- **Đặc biệt**: OCR và layout analysis cho PDF

### 4. `/agent/` - Agentic Workflow
- **Vai trò**: Hệ thống workflow agent với visual canvas
- **Components**: LLM, Retrieval, Categorize, Webhook, Iteration...
- **Tools**: Tavily, Google, Wikipedia, GitHub, Email...

### 5. `/graphrag/` - Knowledge Graph
- **Vai trò**: Xây dựng và query knowledge graph
- **Chức năng**: Entity resolution, graph search, relationship extraction

### 6. `/web/` - Frontend
- **Framework**: React + TypeScript + UmiJS
- **UI**: Ant Design + Shadcn/UI + Tailwind CSS
- **State**: Zustand
- **Port**: 80/443 (qua Nginx)

### 7. `/common/` - Shared Utilities
- **Vai trò**: Utilities và connectors dùng chung
- **Data sources**: Confluence, Notion, Slack, Gmail, SharePoint...

### 8. `/docker/` - Deployment
- **Services**: MySQL, Elasticsearch/Infinity, Redis, MinIO, Nginx
- **Modes**: CPU/GPU, single/cluster

## Tóm Tắt Thống Kê

| Thư mục | Số files | Mô tả |
|---------|----------|-------|
| api/ | ~100+ | Backend API |
| rag/ | ~50+ | RAG engine |
| deepdoc/ | ~30+ | Document parsers |
| agent/ | ~40+ | Agent system |
| graphrag/ | ~20+ | Knowledge graph |
| web/src/ | ~200+ | Frontend |
| common/ | ~50+ | Shared utilities |
| test/ | ~80+ | Test suite |
