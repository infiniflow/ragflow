# RAGFlow - Phân Tích Chi Tiết Các Module

## 1. Module API (`/api/`)

### 1.1 Tổng Quan

Module API là trung tâm xử lý tất cả HTTP requests của hệ thống. Được xây dựng trên Flask/Quart framework với kiến trúc Blueprint.

### 1.2 Cấu Trúc

```
api/
├── ragflow_server.py      # Entry point - Khởi tạo Flask app
├── settings.py            # Cấu hình server
├── constants.py           # API_VERSION = "v1"
├── validation.py          # Request validation
│
├── apps/                  # API Blueprints
├── db/                    # Database layer
└── utils/                 # Utilities
```

### 1.3 Chi Tiết Các Blueprint (API Apps)

#### 1.3.1 `kb_app.py` - Knowledge Base Management
**Chức năng**: Quản lý Knowledge Base (tạo, xóa, sửa, liệt kê)

**Endpoints chính**:
| Method | Endpoint | Mô tả |
|--------|----------|-------|
| POST | `/api/v1/kb/create` | Tạo KB mới |
| GET | `/api/v1/kb/list` | Liệt kê KBs |
| PUT | `/api/v1/kb/update` | Cập nhật KB |
| DELETE | `/api/v1/kb/delete` | Xóa KB |
| GET | `/api/v1/kb/{id}` | Chi tiết KB |

**Logic chính**:
- Validation tenant permissions
- Tạo Elasticsearch index cho mỗi KB
- Quản lý embedding model settings
- Quản lý parser configurations

#### 1.3.2 `document_app.py` - Document Management
**Chức năng**: Upload, parsing, và quản lý documents

**Endpoints chính**:
| Method | Endpoint | Mô tả |
|--------|----------|-------|
| POST | `/api/v1/document/upload` | Upload file |
| POST | `/api/v1/document/run` | Trigger parsing |
| GET | `/api/v1/document/list` | Liệt kê docs |
| DELETE | `/api/v1/document/delete` | Xóa document |
| GET | `/api/v1/document/{id}/chunks` | Lấy chunks |

**Logic chính**:
- File type validation
- MinIO storage integration
- Background task queuing
- Parsing status tracking

#### 1.3.3 `dialog_app.py` - Chat/Dialog Management
**Chức năng**: Xử lý chat conversations với RAG

**Endpoints chính**:
| Method | Endpoint | Mô tả |
|--------|----------|-------|
| POST | `/api/v1/dialog/create` | Tạo dialog |
| POST | `/api/v1/dialog/chat` | Chat (SSE streaming) |
| POST | `/api/v1/dialog/completion` | Non-streaming chat |
| GET | `/api/v1/dialog/list` | Liệt kê dialogs |

**Logic chính**:
- RAG pipeline orchestration
- Streaming response (SSE)
- Conversation history management
- Multi-KB retrieval

#### 1.3.4 `canvas_app.py` - Agent Workflow
**Chức năng**: Visual workflow builder cho AI agents

**Endpoints chính**:
| Method | Endpoint | Mô tả |
|--------|----------|-------|
| POST | `/api/v1/canvas/create` | Tạo workflow |
| POST | `/api/v1/canvas/run` | Execute workflow |
| PUT | `/api/v1/canvas/update` | Cập nhật |
| GET | `/api/v1/canvas/list` | Liệt kê |

**Logic chính**:
- DSL parsing và validation
- Component orchestration
- Tool integration
- Variable passing between nodes

#### 1.3.5 `file_app.py` - File Management
**Chức năng**: Upload, download, quản lý files

**Endpoints chính**:
| Method | Endpoint | Mô tả |
|--------|----------|-------|
| POST | `/api/v1/file/upload` | Upload file |
| GET | `/api/v1/file/download/{id}` | Download |
| GET | `/api/v1/file/list` | Liệt kê files |
| DELETE | `/api/v1/file/delete` | Xóa file |

#### 1.3.6 `search_app.py` - Search Operations
**Chức năng**: Full-text và semantic search

**Endpoints chính**:
| Method | Endpoint | Mô tả |
|--------|----------|-------|
| POST | `/api/v1/search` | Hybrid search |
| GET | `/api/v1/search/history` | Search history |

### 1.4 Database Services (`/api/db/services/`)

#### `dialog_service.py` (37KB - Service phức tạp nhất)
```python
class DialogService:
    def chat(dialog_id, question, stream=True):
        """
        Main RAG chat function
        1. Load dialog configuration
        2. Get relevant documents (retrieval)
        3. Rerank results
        4. Build prompt with context
        5. Call LLM (streaming)
        6. Save conversation
        """

    def retrieval(dialog, question):
        """
        Hybrid retrieval from Elasticsearch
        - Vector similarity search
        - BM25 full-text search
        - Score combination
        """

    def rerank(chunks, question):
        """
        Cross-encoder reranking
        - Score each chunk against question
        - Return top-k
        """
```

#### `document_service.py` (39KB)
```python
class DocumentService:
    def upload(file, kb_id):
        """Upload file to MinIO, create DB record"""

    def parse(doc_id):
        """Queue document for background parsing"""

    def chunk(doc_id, chunks):
        """Save parsed chunks to ES and DB"""

    def delete(doc_id):
        """Remove doc, chunks, and file"""
```

#### `knowledgebase_service.py` (21KB)
```python
class KnowledgebaseService:
    def create(name, embedding_model, parser_id):
        """Create KB with ES index"""

    def update_parser_config(kb_id, config):
        """Update chunking/parsing settings"""

    def get_statistics(kb_id):
        """Get doc count, chunk count, etc."""
```

### 1.5 Database Models (`/api/db/db_models.py`)

**25+ Models quan trọng**:

```python
# User & Tenant
class User(BaseModel):
    id, email, password, nickname, avatar, status, login_channel

class Tenant(BaseModel):
    id, name, public_key, llm_id, embd_id, parser_id, credit

class UserTenant(BaseModel):
    user_id, tenant_id, role  # owner, admin, member

# Knowledge Management
class Knowledgebase(BaseModel):
    id, tenant_id, name, description, embd_id, parser_id,
    similarity_threshold, vector_similarity_weight, ...

class Document(BaseModel):
    id, kb_id, name, location, size, type, parser_id,
    status, progress, chunk_num, token_num, process_duation

class File(BaseModel):
    id, tenant_id, name, size, location, type, source_type

# Chat & Dialog
class Dialog(BaseModel):
    id, tenant_id, name, description, kb_ids, llm_id,
    prompt_config, similarity_threshold, top_n, top_k

class Conversation(BaseModel):
    id, dialog_id, name, message  # JSON array of messages

# Workflow
class UserCanvas(BaseModel):
    id, tenant_id, name, dsl, avatar  # DSL is workflow definition

class CanvasTemplate(BaseModel):
    id, name, dsl, avatar  # Pre-built templates

# Integration
class APIToken(BaseModel):
    id, tenant_id, token, dialog_id  # For external API access

class MCPServer(BaseModel):
    id, tenant_id, name, host, tools  # MCP server config
```

---

## 2. Module RAG (`/rag/`)

### 2.1 Tổng Quan

Core RAG processing engine - xử lý từ document parsing đến retrieval.

### 2.2 LLM Abstractions (`/rag/llm/`)

#### `chat_model.py` - Chat LLM Interface
```python
class Base:
    """Abstract base for all chat models"""
    def chat(messages, stream=True, **kwargs):
        """Generate chat completion"""

class OpenAIChat(Base):
    """OpenAI GPT models"""

class ClaudeChat(Base):
    """Anthropic Claude models"""

class QwenChat(Base):
    """Alibaba Qwen models"""

class OllamaChat(Base):
    """Local Ollama models"""

# Factory function
def get_chat_model(model_name, api_key, base_url):
    """Return appropriate chat model instance"""
```

**Supported Providers** (20+):
- OpenAI (GPT-3.5, GPT-4, GPT-4V)
- Anthropic (Claude 3)
- Google (Gemini)
- Alibaba (Qwen, Qwen-VL)
- Groq
- Mistral
- Cohere
- DeepSeek
- Zhipu (GLM)
- Moonshot
- Ollama (local)
- NVIDIA
- Bedrock (AWS)
- Azure OpenAI
- Hugging Face
- ...

#### `embedding_model.py` - Embedding Interface
```python
class Base:
    """Abstract base for embeddings"""
    def encode(texts: List[str]) -> List[List[float]]:
        """Generate embeddings for texts"""

class OpenAIEmbed(Base):
    """text-embedding-ada-002, text-embedding-3-*"""

class BGEEmbed(Base):
    """BAAI BGE models"""

class JinaEmbed(Base):
    """Jina AI embeddings"""

# Supported embedding models:
# - OpenAI: ada-002, embedding-3-small, embedding-3-large
# - BGE: bge-base, bge-large, bge-m3
# - Jina: jina-embeddings-v2
# - Cohere: embed-english-v3
# - HuggingFace: sentence-transformers
# - Local: Ollama embeddings
```

#### `rerank_model.py` - Reranking Interface
```python
class Base:
    """Abstract base for rerankers"""
    def rerank(query: str, documents: List[str]) -> List[float]:
        """Score documents against query"""

class CohereRerank(Base):
    """Cohere rerank models"""

class JinaRerank(Base):
    """Jina AI reranker"""

class BGERerank(Base):
    """BAAI BGE reranker"""
```

### 2.3 RAG Pipeline (`/rag/flow/`)

#### Pipeline Architecture
```
Document → Parser → Tokenizer → Splitter → Embedder → Index
```

#### `parser/parser.py`
```python
def parse(file_path, parser_config):
    """
    Parse document based on file type
    Returns: List of text segments with metadata
    """
    # Supported parsers:
    # - naive: Simple text extraction
    # - paper: Academic paper structure
    # - book: Book chapter detection
    # - laws: Legal document parsing
    # - presentation: PPT parsing
    # - qa: Q&A format extraction
    # - table: Table extraction
    # - picture: Image description
    # - one: Single chunk per doc
    # - audio: Audio transcription
    # - email: Email thread parsing
```

#### `splitter/splitter.py`
```python
class Splitter:
    """Document chunking strategies"""

    def split_by_tokens(text, chunk_size=512, overlap=128):
        """Token-based splitting"""

    def split_by_sentences(text, max_sentences=10):
        """Sentence-based splitting"""

    def split_by_delimiter(text, delimiter='\n\n'):
        """Delimiter-based splitting"""

    def split_semantic(text, threshold=0.5):
        """Semantic similarity based splitting"""
```

#### `tokenizer/tokenizer.py`
```python
class Tokenizer:
    """Text tokenization"""

    def tokenize(text):
        """Convert text to tokens"""

    def count_tokens(text):
        """Count tokens in text"""

    # Uses tiktoken for OpenAI models
    # Uses model-specific tokenizers for others
```

### 2.4 RAPTOR (`/rag/raptor.py`)

**RAPTOR** = Recursive Abstractive Processing for Tree-Organized Retrieval

```python
class RAPTOR:
    """
    Hierarchical document representation
    - Clusters similar chunks
    - Creates summaries of clusters
    - Builds tree structure for retrieval
    """

    def build_tree(chunks):
        """Build RAPTOR tree from chunks"""

    def retrieve(query, tree):
        """Retrieve from tree structure"""
```

---

## 3. Module DeepDoc (`/deepdoc/`)

### 3.1 Tổng Quan

Deep document understanding với layout analysis và OCR.

### 3.2 Document Parsers (`/deepdoc/parser/`)

#### `pdf_parser.py` - PDF Processing
```python
class PdfParser:
    """
    Advanced PDF parsing with:
    - OCR for scanned pages
    - Layout analysis (tables, figures, headers)
    - Multi-column detection
    - Image extraction
    """

    def __call__(file_path):
        """Parse PDF file"""
        # 1. Extract text with PyMuPDF
        # 2. Apply OCR if needed (Tesseract)
        # 3. Analyze layout (detectron2/layoutlm)
        # 4. Extract tables (camelot/tabula)
        # 5. Extract images
        # Return structured content
```

#### `docx_parser.py` - Word Documents
```python
class DocxParser:
    """
    Parse .docx files
    - Text extraction
    - Table extraction
    - Image extraction
    - Style preservation
    """
```

#### `excel_parser.py` - Spreadsheets
```python
class ExcelParser:
    """
    Parse .xlsx/.xls files
    - Sheet-by-sheet processing
    - Table structure preservation
    - Formula evaluation
    """
```

#### `html_parser.py` - Web Pages
```python
class HtmlParser:
    """
    Parse HTML content
    - Clean HTML
    - Extract main content
    - Handle tables
    - Remove scripts/styles
    """
```

### 3.3 Vision Module (`/deepdoc/vision/`)

```python
class LayoutAnalyzer:
    """
    Document layout analysis using ML
    - Detectron2 for object detection
    - LayoutLM for document understanding
    """

    def analyze(image):
        """
        Detect document regions:
        - Title
        - Paragraph
        - Table
        - Figure
        - Header/Footer
        - List
        """
```

---

## 4. Module Agent (`/agent/`)

### 4.1 Tổng Quan

Agentic workflow system với visual canvas builder.

### 4.2 Canvas Engine (`/agent/canvas.py`)

```python
class Canvas:
    """
    Main workflow orchestrator
    - Parse DSL definition
    - Execute components in order
    - Handle branching logic
    - Manage variables
    """

    def __init__(self, dsl):
        """Initialize from DSL"""
        self.components = self._parse_dsl(dsl)
        self.graph = self._build_graph()

    def run(self, input_data):
        """Execute workflow"""
        context = {"input": input_data}

        for component in self._topological_sort():
            result = component.execute(context)
            context.update(result)

        return context["output"]
```

### 4.3 Components (`/agent/component/`)

#### `begin.py` - Workflow Start
```python
class BeginComponent:
    """
    Entry point of workflow
    - Initialize variables
    - Receive user input
    """
    def execute(self, context):
        return {"user_input": context["input"]}
```

#### `llm.py` - LLM Component
```python
class LLMComponent:
    """
    Call LLM with configured prompt
    - Template variable substitution
    - Streaming support
    - Output parsing
    """
    def execute(self, context):
        prompt = self.template.format(**context)
        response = self.llm.chat(prompt)
        return {"llm_output": response}
```

#### `retrieval.py` - Retrieval Component
```python
class RetrievalComponent:
    """
    Search knowledge bases
    - Multi-KB search
    - Configurable top_k
    - Score threshold
    """
    def execute(self, context):
        query = context["user_input"]
        results = self.search(query, self.kb_ids)
        return {"retrieved_docs": results}
```

#### `categorize.py` - Conditional Branching
```python
class CategorizeComponent:
    """
    Route to different paths based on conditions
    - LLM-based classification
    - Rule-based matching
    """
    def execute(self, context):
        category = self._classify(context)
        return {"next_node": self.routes[category]}
```

#### `agent_with_tools.py` - Tool-Using Agent
```python
class AgentWithToolsComponent:
    """
    ReAct pattern agent
    - Tool selection
    - Iterative reasoning
    - Observation handling
    """
    def execute(self, context):
        while not done:
            action = self.llm.decide_action(context)
            if action.type == "tool":
                result = self.tools[action.tool].run(action.input)
                context["observation"] = result
            else:
                return {"output": action.response}
```

### 4.4 Tools (`/agent/tools/`)

#### External Tool Integrations

| Tool | File | Chức năng |
|------|------|-----------|
| Tavily | `tavily.py` | Web search API |
| ArXiv | `arxiv.py` | Academic paper search |
| Google | `google.py` | Google search |
| Wikipedia | `wikipedia.py` | Wikipedia lookup |
| GitHub | `github.py` | GitHub API |
| Email | `email.py` | Send emails |
| Code Exec | `code_exec.py` | Execute Python code |
| DeepL | `deepl.py` | Translation |
| Jin10 | `jin10.py` | Financial news |
| TuShare | `tushare.py` | Chinese stock data |
| Yahoo Finance | `yahoofinance.py` | Stock data |
| QWeather | `qweather.py` | Weather data |

```python
class BaseTool:
    """Base class for all tools"""
    name: str
    description: str

    def run(self, input: str) -> str:
        """Execute tool and return result"""

class TavilySearch(BaseTool):
    name = "tavily_search"
    description = "Search the web for current information"

    def run(self, query):
        response = tavily.search(query)
        return format_results(response)
```

---

## 5. Module GraphRAG (`/graphrag/`)

### 5.1 Tổng Quan

Knowledge graph construction và querying.

### 5.2 Entity Resolution (`/graphrag/entity_resolution.py`)

```python
class EntityResolution:
    """
    Entity extraction và linking
    - Extract entities from text
    - Cluster similar entities
    - Resolve duplicates
    """

    def extract_entities(text):
        """Extract named entities using LLM"""
        prompt = f"Extract entities from: {text}"
        return llm.chat(prompt)

    def resolve_entities(entities):
        """Merge duplicate entities"""
        clusters = self._cluster_similar(entities)
        return self._merge_clusters(clusters)
```

### 5.3 Graph Search (`/graphrag/search.py`)

```python
class GraphSearch:
    """
    Query knowledge graph
    - Entity-based search
    - Relationship traversal
    - Subgraph extraction
    """

    def search(query):
        """Find relevant subgraph for query"""
        # 1. Extract query entities
        # 2. Find matching graph entities
        # 3. Traverse relationships
        # 4. Return context subgraph
```

---

## 6. Module Frontend (`/web/`)

### 6.1 Tổng Quan

React/TypeScript SPA với UmiJS framework.

### 6.2 Pages (`/web/src/pages/`)

| Page | Chức năng |
|------|-----------|
| `/dataset` | Knowledge base management |
| `/datasets` | Dataset list view |
| `/next-chats` | Chat interface |
| `/next-searches` | Search interface |
| `/document-viewer` | Document preview |
| `/admin` | Admin dashboard |
| `/login` | Authentication |
| `/register` | User registration |

### 6.3 Components (`/web/src/components/`)

**Core Components**:
- `file-upload-modal/` - File upload UI
- `pdf-drawer/` - PDF preview drawer
- `prompt-editor/` - Prompt template editor
- `document-preview/` - Document viewer
- `llm-setting-items/` - LLM configuration UI
- `ui/` - Shadcn/UI base components

### 6.4 State Management

```typescript
// Using Zustand for state
import { create } from 'zustand';

interface KnowledgebaseStore {
  knowledgebases: Knowledgebase[];
  currentKb: Knowledgebase | null;
  fetchKnowledgebases: () => Promise<void>;
  createKnowledgebase: (data: CreateKbRequest) => Promise<void>;
}

export const useKnowledgebaseStore = create<KnowledgebaseStore>((set) => ({
  knowledgebases: [],
  currentKb: null,
  fetchKnowledgebases: async () => {
    const data = await api.get('/kb/list');
    set({ knowledgebases: data });
  },
  // ...
}));
```

### 6.5 API Services (`/web/src/services/`)

```typescript
// API client using Axios
import { request } from 'umi';

export async function createKnowledgebase(data: CreateKbRequest) {
  return request('/api/v1/kb/create', {
    method: 'POST',
    data,
  });
}

export async function chat(dialogId: string, question: string) {
  return request('/api/v1/dialog/chat', {
    method: 'POST',
    data: { dialog_id: dialogId, question },
    responseType: 'stream',
  });
}
```

---

## 7. Module Common (`/common/`)

### 7.1 Configuration (`/common/settings.py`)

```python
# Main configuration file
class Settings:
    # Database
    MYSQL_HOST = os.getenv('MYSQL_HOST', 'localhost')
    MYSQL_PORT = int(os.getenv('MYSQL_PORT', 5455))
    MYSQL_USER = os.getenv('MYSQL_USER', 'root')
    MYSQL_PASSWORD = os.getenv('MYSQL_PASSWORD', 'infini_rag_flow')
    MYSQL_DATABASE = os.getenv('MYSQL_DATABASE', 'ragflow')

    # Elasticsearch
    ES_HOSTS = os.getenv('ES_HOSTS', 'http://localhost:9200').split(',')

    # Redis
    REDIS_HOST = os.getenv('REDIS_HOST', 'localhost')
    REDIS_PORT = int(os.getenv('REDIS_PORT', 6379))

    # MinIO
    MINIO_HOST = os.getenv('MINIO_HOST', 'localhost:9000')
    MINIO_ACCESS_KEY = os.getenv('MINIO_USER', 'rag_flow')
    MINIO_SECRET_KEY = os.getenv('MINIO_PASSWORD', 'infini_rag_flow')

    # Document Engine
    DOC_ENGINE = os.getenv('DOC_ENGINE', 'elasticsearch')  # or 'infinity'
```

### 7.2 Data Source Connectors (`/common/data_source/`)

**Supported Connectors**:

| Connector | File | Chức năng |
|-----------|------|-----------|
| Confluence | `confluence_connector.py` (81KB) | Atlassian Confluence wiki |
| Notion | `notion_connector.py` (25KB) | Notion databases |
| Slack | `slack_connector.py` (22KB) | Slack messages |
| Gmail | `gmail_connector.py` | Gmail emails |
| Discord | `discord_connector.py` | Discord channels |
| SharePoint | `sharepoint_connector.py` | Microsoft SharePoint |
| Teams | `teams_connector.py` | Microsoft Teams |
| Dropbox | `dropbox_connector.py` | Dropbox files |
| Google Drive | `google_drive/` | Google Drive |
| WebDAV | `webdav_connector.py` | WebDAV servers |
| Moodle | `moodle_connector.py` | Moodle LMS |

```python
class BaseConnector:
    """Abstract base for connectors"""

    def authenticate(credentials):
        """Authenticate with external service"""

    def list_items():
        """List available items"""

    def sync():
        """Sync data to RAGFlow"""

class ConfluenceConnector(BaseConnector):
    """Confluence integration"""

    def __init__(self, url, username, api_token):
        self.client = Confluence(url, username, api_token)

    def sync_space(space_key):
        """Sync all pages from a space"""
        pages = self.client.get_all_pages(space_key)
        for page in pages:
            content = self._convert_to_markdown(page.body)
            yield Document(content=content, metadata=page.metadata)
```

---

## 8. Module SDK (`/sdk/python/`)

### 8.1 Python SDK

```python
from ragflow import RAGFlow

# Initialize client
client = RAGFlow(
    api_key="your-api-key",
    base_url="http://localhost:9380"
)

# Create knowledge base
kb = client.create_knowledgebase(
    name="My KB",
    embedding_model="text-embedding-3-small"
)

# Upload document
doc = kb.upload_document("path/to/document.pdf")

# Wait for parsing
doc.wait_for_ready()

# Create chat
chat = client.create_chat(
    name="My Chat",
    knowledgebase_ids=[kb.id]
)

# Send message
response = chat.send_message("What is this document about?")
print(response.answer)
```

---

## 9. Tóm Tắt Module Dependencies

```
┌─────────────────────────────────────────────────────────────────┐
│                         Frontend (web/)                          │
└─────────────────────────────┬───────────────────────────────────┘
                              │ HTTP/SSE
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                          API (api/)                              │
│   ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐           │
│   │ kb_app  │  │doc_app  │  │dialog_  │  │canvas_  │           │
│   │         │  │         │  │app      │  │app      │           │
│   └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘           │
│        └────────────┴───────────┴────────────┘                  │
│                              │                                   │
│   ┌──────────────────────────┴──────────────────────────┐      │
│   │                    Services Layer                    │      │
│   │  DialogService │ DocumentService │ KBService         │      │
│   └───────────────────────────┬─────────────────────────┘      │
└───────────────────────────────┼─────────────────────────────────┘
                                │
        ┌───────────────────────┼───────────────────────┐
        │                       │                       │
        ▼                       ▼                       ▼
┌───────────────┐    ┌──────────────────┐    ┌──────────────────┐
│   RAG (rag/)  │    │  Agent (agent/)  │    │GraphRAG(graphrag)│
│               │    │                  │    │                  │
│ - LLM Models  │    │ - Canvas Engine  │    │ - Entity Res.    │
│ - Pipeline    │    │ - Components     │    │ - Graph Search   │
│ - Embeddings  │    │ - Tools          │    │                  │
└───────┬───────┘    └────────┬─────────┘    └────────┬─────────┘
        │                     │                       │
        └─────────────────────┼───────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      DeepDoc (deepdoc/)                          │
│                                                                  │
│   PDF Parser │ DOCX Parser │ HTML Parser │ Vision/OCR           │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                       Common (common/)                           │
│                                                                  │
│   Settings │ Utilities │ Data Source Connectors                 │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Data Stores                                 │
│                                                                  │
│   MySQL │ Elasticsearch/Infinity │ Redis │ MinIO                │
└─────────────────────────────────────────────────────────────────┘
```

## 10. Kích Thước Code Ước Tính

| Module | Lines of Code | Complexity |
|--------|--------------|------------|
| api/ | ~15,000 | High |
| rag/ | ~8,000 | High |
| deepdoc/ | ~5,000 | Medium |
| agent/ | ~6,000 | High |
| graphrag/ | ~3,000 | Medium |
| web/src/ | ~20,000 | High |
| common/ | ~5,000 | Medium |
| **Total** | **~62,000** | - |
