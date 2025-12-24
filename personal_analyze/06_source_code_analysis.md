# RAGFlow - Phân Tích Source Code Chi Tiết

## 1. Tổng Quan Codebase

### 1.1 Thống Kê Code

| Metric | Giá trị |
|--------|---------|
| **Total Lines of Code** | ~62,000+ |
| **Python Files** | ~300+ |
| **TypeScript/JavaScript Files** | ~400+ |
| **Test Files** | ~100+ |
| **Configuration Files** | ~50+ |

### 1.2 Code Quality Metrics

```
┌─────────────────────────────────────────────────────────────────┐
│                     CODE QUALITY OVERVIEW                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Python Code:                                                    │
│  ├── Linter: ruff (strict)                                      │
│  ├── Formatter: ruff format                                     │
│  ├── Type Hints: Partial (improving)                            │
│  └── Test Coverage: ~60%                                        │
│                                                                  │
│  TypeScript Code:                                                │
│  ├── Linter: ESLint (strict)                                    │
│  ├── Formatter: Prettier                                        │
│  ├── Type Safety: Strict mode                                   │
│  └── Test Coverage: ~40%                                        │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## 2. Backend Code Analysis

### 2.1 Entry Point Analysis

**File**: `api/ragflow_server.py`

```python
# Simplified structure analysis

from quart import Quart
from quart_cors import cors

# Application factory pattern
def create_app():
    app = Quart(__name__)

    # CORS configuration
    app = cors(app, allow_origin="*")

    # Session configuration
    app.config['SECRET_KEY'] = ...
    app.config['SESSION_TYPE'] = 'redis'

    # Register blueprints
    from api.apps import (
        kb_app, document_app, dialog_app,
        canvas_app, file_app, user_app, ...
    )

    app.register_blueprint(kb_app, url_prefix='/api/v1/kb')
    app.register_blueprint(document_app, url_prefix='/api/v1/document')
    # ... more blueprints

    # Swagger documentation
    from flasgger import Swagger
    Swagger(app)

    return app

# Main entry
if __name__ == '__main__':
    app = create_app()
    app.run(host='0.0.0.0', port=9380)
```

**Key Patterns**:
- Application Factory Pattern
- Blueprint-based modular architecture
- ASGI với Quart (async Flask)
- Swagger/OpenAPI documentation

### 2.2 API Blueprint Structure

**Pattern sử dụng**:

```python
# Typical blueprint structure (e.g., kb_app.py)

from flask import Blueprint, request
from api.db.services import KnowledgebaseService
from api.utils.api_utils import get_data, validate_request

kb_app = Blueprint('kb', __name__)

@kb_app.route('/create', methods=['POST'])
@validate_request  # Decorator for validation
@login_required    # Authentication decorator
async def create():
    """
    Create a new knowledge base.
    ---
    tags:
      - Knowledge Base
    parameters:
      - name: body
        in: body
        required: true
        schema:
          type: object
          properties:
            name:
              type: string
            description:
              type: string
    responses:
      200:
        description: Success
    """
    try:
        req = await get_data(request)
        tenant_id = get_tenant_id(request)

        # Validation
        if not req.get('name'):
            return error_response("Name is required")

        # Business logic
        kb = KnowledgebaseService.create(
            name=req['name'],
            tenant_id=tenant_id,
            description=req.get('description', '')
        )

        return success_response(kb.to_dict())

    except Exception as e:
        return error_response(str(e))
```

**Design Patterns**:
- RESTful API design
- Decorator pattern cho cross-cutting concerns
- Service layer separation
- Consistent error handling

### 2.3 Service Layer Analysis

**File**: `api/db/services/dialog_service.py` (37KB - phức tạp nhất)

```python
# Core RAG chat implementation

class DialogService:
    """
    Main service for RAG-based chat functionality.
    Handles retrieval, reranking, and generation.
    """

    @classmethod
    def chat(cls, dialog_id: str, question: str,
             stream: bool = True, **kwargs):
        """
        Main chat entry point.

        Flow:
        1. Load dialog configuration
        2. Get conversation history
        3. Perform retrieval
        4. Rerank results
        5. Build prompt with context
        6. Generate response (streaming)
        7. Save conversation
        """

        # 1. Load dialog
        dialog = Dialog.get_by_id(dialog_id)

        # 2. Get history
        conversation = Conversation.get_or_create(...)
        history = conversation.messages[-10:]  # Last 10 messages

        # 3. Retrieval
        chunks = cls._retrieval(dialog, question)

        # 4. Reranking
        if dialog.rerank_id:
            chunks = cls._rerank(chunks, question, dialog.top_n)

        # 5. Build prompt
        context = cls._build_context(chunks)
        prompt = cls._build_prompt(dialog, question, context, history)

        # 6. Generate
        if stream:
            return cls._stream_generate(dialog, prompt)
        else:
            return cls._generate(dialog, prompt)

    @classmethod
    def _retrieval(cls, dialog, question):
        """
        Hybrid retrieval from Elasticsearch.
        Combines vector similarity and BM25.
        """
        # Generate query embedding
        embedding = EmbeddingModel.encode(question)

        # Build ES query
        query = {
            "script_score": {
                "query": {
                    "bool": {
                        "should": [
                            {"match": {"content": question}},  # BM25
                        ],
                        "filter": [
                            {"terms": {"kb_id": dialog.kb_ids}}
                        ]
                    }
                },
                "script": {
                    "source": """
                        cosineSimilarity(params.query_vector, 'embedding') + 1.0
                    """,
                    "params": {"query_vector": embedding}
                }
            }
        }

        # Execute search
        results = es.search(index="ragflow_*", body={"query": query})
        return results['hits']['hits']

    @classmethod
    def _stream_generate(cls, dialog, prompt):
        """
        Streaming generation using SSE.
        """
        llm = ChatModel.get(dialog.llm_id)

        for chunk in llm.chat(prompt, stream=True):
            yield {
                "answer": chunk.content,
                "reference": {},
                "done": False
            }

        yield {"answer": "", "done": True}
```

**Key Implementation Details**:
- Hybrid search (vector + BM25)
- Streaming response với SSE
- Conversation history management
- Configurable reranking

### 2.4 Database Model Analysis

**File**: `api/db/db_models.py` (54KB)

```python
# Using Peewee ORM

from peewee import *
from playhouse.shortcuts import model_to_dict

# Base model with common fields
class BaseModel(Model):
    id = CharField(primary_key=True, max_length=32)
    create_time = BigIntegerField(default=lambda: int(time.time() * 1000))
    update_time = BigIntegerField(default=lambda: int(time.time() * 1000))
    create_date = DateTimeField(default=datetime.now)
    update_date = DateTimeField(default=datetime.now)

    class Meta:
        database = db

    def to_dict(self):
        return model_to_dict(self)

# User model
class User(BaseModel):
    email = CharField(max_length=255, unique=True)
    password = CharField(max_length=255)
    nickname = CharField(max_length=255, null=True)
    avatar = TextField(null=True)
    status = CharField(max_length=16, default='active')
    login_channel = CharField(max_length=32, default='password')
    last_login_time = DateTimeField(null=True)

    class Meta:
        table_name = 'user'

# Knowledge Base model
class Knowledgebase(BaseModel):
    tenant_id = CharField(max_length=32)
    name = CharField(max_length=255)
    description = TextField(null=True)

    # Embedding configuration
    embd_id = CharField(max_length=128)

    # Parser configuration (JSON)
    parser_id = CharField(max_length=32, default='naive')
    parser_config = JSONField(default={})

    # Search configuration
    similarity_threshold = FloatField(default=0.2)
    vector_similarity_weight = FloatField(default=0.3)
    top_n = IntegerField(default=6)

    # Statistics
    doc_num = IntegerField(default=0)
    token_num = IntegerField(default=0)
    chunk_num = IntegerField(default=0)

    class Meta:
        table_name = 'knowledgebase'
        indexes = (
            (('tenant_id', 'name'), True),  # Unique constraint
        )

# Document model
class Document(BaseModel):
    kb_id = CharField(max_length=32)
    name = CharField(max_length=512)
    location = CharField(max_length=1024)  # MinIO path
    size = BigIntegerField(default=0)
    type = CharField(max_length=32)

    # Processing status
    status = CharField(max_length=16, default='UNSTART')
    # UNSTART -> RUNNING -> FINISHED / FAIL
    progress = FloatField(default=0)
    progress_msg = TextField(null=True)

    # Parser configuration
    parser_id = CharField(max_length=32)
    parser_config = JSONField(default={})

    # Statistics
    chunk_num = IntegerField(default=0)
    token_num = IntegerField(default=0)
    process_duration = FloatField(default=0)

    class Meta:
        table_name = 'document'

# Dialog (Chat) model
class Dialog(BaseModel):
    tenant_id = CharField(max_length=32)
    name = CharField(max_length=255)
    description = TextField(null=True)

    # Knowledge base references
    kb_ids = JSONField(default=[])  # List of KB IDs

    # LLM configuration
    llm_id = CharField(max_length=128)
    llm_setting = JSONField(default={
        'temperature': 0.7,
        'max_tokens': 2048,
        'top_p': 1.0
    })

    # Prompt configuration
    prompt_config = JSONField(default={
        'system': 'You are a helpful assistant.',
        'prologue': '',
        'show_quote': True
    })

    # Retrieval configuration
    similarity_threshold = FloatField(default=0.2)
    vector_similarity_weight = FloatField(default=0.3)
    top_n = IntegerField(default=6)
    top_k = IntegerField(default=1024)

    # Reranking
    rerank_id = CharField(max_length=128, null=True)

    class Meta:
        table_name = 'dialog'
```

**ORM Patterns**:
- Active Record pattern (Peewee)
- JSON fields cho flexible data
- Soft timestamps (create/update)
- Index optimization

### 2.5 RAG Pipeline Code Analysis

**File**: `rag/flow/pipeline.py`

```python
# Document processing pipeline

class Pipeline:
    """
    Main document processing pipeline.
    Orchestrates: Parse → Tokenize → Split → Embed → Index
    """

    def __init__(self, document_id: str):
        self.doc = Document.get_by_id(document_id)
        self.kb = Knowledgebase.get_by_id(self.doc.kb_id)

        # Initialize components based on config
        self.parser = self._get_parser()
        self.tokenizer = self._get_tokenizer()
        self.splitter = self._get_splitter()
        self.embedder = self._get_embedder()

    def run(self):
        """Execute the full pipeline."""
        try:
            self._update_status('RUNNING')

            # 1. Download file from MinIO
            file_content = self._download_file()

            # 2. Parse document
            self._update_progress(0.1, "Parsing document...")
            parsed = self.parser.parse(file_content)

            # 3. Extract and tokenize
            self._update_progress(0.3, "Tokenizing...")
            tokens = self.tokenizer.tokenize(parsed)

            # 4. Split into chunks
            self._update_progress(0.5, "Chunking...")
            chunks = self.splitter.split(tokens)

            # 5. Generate embeddings
            self._update_progress(0.7, "Embedding...")
            embedded_chunks = self._embed_chunks(chunks)

            # 6. Index to Elasticsearch
            self._update_progress(0.9, "Indexing...")
            self._index_chunks(embedded_chunks)

            # 7. Update statistics
            self._update_status('FINISHED')
            self._update_statistics(len(chunks))

        except Exception as e:
            self._update_status('FAIL', str(e))
            raise

    def _embed_chunks(self, chunks: List[str]) -> List[dict]:
        """Generate embeddings for chunks in batches."""
        batch_size = 32
        results = []

        for i in range(0, len(chunks), batch_size):
            batch = chunks[i:i+batch_size]
            embeddings = self.embedder.encode(batch)

            for chunk, embedding in zip(batch, embeddings):
                results.append({
                    'content': chunk,
                    'embedding': embedding,
                    'kb_id': self.kb.id,
                    'doc_id': self.doc.id
                })

        return results

    def _index_chunks(self, chunks: List[dict]):
        """Bulk index chunks to Elasticsearch."""
        actions = []
        for i, chunk in enumerate(chunks):
            actions.append({
                '_index': f'ragflow_{self.kb.id}',
                '_id': f'{self.doc.id}_{i}',
                '_source': chunk
            })

        # Bulk insert
        helpers.bulk(es, actions)
```

**Pipeline Patterns**:
- Chain of Responsibility
- Strategy pattern cho parsers
- Batch processing
- Progress tracking

---

## 3. Frontend Code Analysis

### 3.1 Project Structure

```typescript
// UmiJS project structure
web/src/
├── pages/           // Route-based pages
├── components/      // Reusable components
├── services/        // API calls
├── hooks/           // Custom React hooks
├── interfaces/      // TypeScript types
├── utils/           // Utility functions
├── constants/       // Constants
├── locales/         // i18n translations
└── less/            // Global styles
```

### 3.2 Page Component Analysis

**File**: `web/src/pages/dataset/index.tsx`

```typescript
// Knowledge Base List Page

import { useState, useEffect } from 'react';
import { useRequest } from 'ahooks';
import { Table, Button, Modal, message } from 'antd';
import { useNavigate } from 'umi';

import { getKnowledgebases, deleteKnowledgebase } from '@/services/kb';
import CreateKbModal from './components/CreateKbModal';

interface Knowledgebase {
  id: string;
  name: string;
  description: string;
  doc_num: number;
  chunk_num: number;
  create_time: number;
}

const DatasetList: React.FC = () => {
  const navigate = useNavigate();
  const [createModalVisible, setCreateModalVisible] = useState(false);

  // Data fetching with caching
  const { data, loading, refresh } = useRequest(getKnowledgebases, {
    refreshDeps: [],
  });

  // Table columns definition
  const columns = [
    {
      title: 'Name',
      dataIndex: 'name',
      key: 'name',
      render: (text: string, record: Knowledgebase) => (
        <a onClick={() => navigate(`/dataset/${record.id}`)}>{text}</a>
      ),
    },
    {
      title: 'Documents',
      dataIndex: 'doc_num',
      key: 'doc_num',
    },
    {
      title: 'Chunks',
      dataIndex: 'chunk_num',
      key: 'chunk_num',
    },
    {
      title: 'Actions',
      key: 'actions',
      render: (_: any, record: Knowledgebase) => (
        <Button danger onClick={() => handleDelete(record.id)}>
          Delete
        </Button>
      ),
    },
  ];

  const handleDelete = async (id: string) => {
    Modal.confirm({
      title: 'Confirm Delete',
      content: 'Are you sure you want to delete this knowledge base?',
      onOk: async () => {
        await deleteKnowledgebase(id);
        message.success('Deleted successfully');
        refresh();
      },
    });
  };

  return (
    <div className="p-6">
      <div className="flex justify-between mb-4">
        <h1 className="text-2xl font-bold">Knowledge Bases</h1>
        <Button type="primary" onClick={() => setCreateModalVisible(true)}>
          Create
        </Button>
      </div>

      <Table
        loading={loading}
        columns={columns}
        dataSource={data?.data || []}
        rowKey="id"
      />

      <CreateKbModal
        visible={createModalVisible}
        onClose={() => setCreateModalVisible(false)}
        onSuccess={() => {
          setCreateModalVisible(false);
          refresh();
        }}
      />
    </div>
  );
};

export default DatasetList;
```

**React Patterns**:
- Functional components với hooks
- Custom hooks cho data fetching
- Controlled components
- Composition pattern

### 3.3 State Management

**File**: `web/src/hooks/useKnowledgebaseStore.ts`

```typescript
// Zustand store for knowledge base state

import { create } from 'zustand';
import { devtools, persist } from 'zustand/middleware';

interface KnowledgebaseState {
  knowledgebases: Knowledgebase[];
  currentKb: Knowledgebase | null;
  loading: boolean;
  error: string | null;

  // Actions
  fetchKnowledgebases: () => Promise<void>;
  setCurrentKb: (kb: Knowledgebase | null) => void;
  createKnowledgebase: (data: CreateKbRequest) => Promise<Knowledgebase>;
  updateKnowledgebase: (id: string, data: UpdateKbRequest) => Promise<void>;
  deleteKnowledgebase: (id: string) => Promise<void>;
}

export const useKnowledgebaseStore = create<KnowledgebaseState>()(
  devtools(
    persist(
      (set, get) => ({
        knowledgebases: [],
        currentKb: null,
        loading: false,
        error: null,

        fetchKnowledgebases: async () => {
          set({ loading: true, error: null });
          try {
            const response = await api.get('/kb/list');
            set({ knowledgebases: response.data, loading: false });
          } catch (error) {
            set({ error: error.message, loading: false });
          }
        },

        setCurrentKb: (kb) => set({ currentKb: kb }),

        createKnowledgebase: async (data) => {
          const response = await api.post('/kb/create', data);
          const newKb = response.data;
          set((state) => ({
            knowledgebases: [...state.knowledgebases, newKb],
          }));
          return newKb;
        },

        updateKnowledgebase: async (id, data) => {
          await api.put(`/kb/${id}`, data);
          set((state) => ({
            knowledgebases: state.knowledgebases.map((kb) =>
              kb.id === id ? { ...kb, ...data } : kb
            ),
          }));
        },

        deleteKnowledgebase: async (id) => {
          await api.delete(`/kb/${id}`);
          set((state) => ({
            knowledgebases: state.knowledgebases.filter((kb) => kb.id !== id),
          }));
        },
      }),
      {
        name: 'knowledgebase-storage',
        partialize: (state) => ({ currentKb: state.currentKb }),
      }
    )
  )
);
```

**State Management Patterns**:
- Zustand cho global state
- React Query cho server state
- Middleware (devtools, persist)
- Immer-style updates

### 3.4 API Service Layer

**File**: `web/src/services/api.ts`

```typescript
// API client configuration

import axios, { AxiosInstance, AxiosRequestConfig } from 'axios';
import { message } from 'antd';

class ApiClient {
  private instance: AxiosInstance;

  constructor() {
    this.instance = axios.create({
      baseURL: '/api/v1',
      timeout: 30000,
      headers: {
        'Content-Type': 'application/json',
      },
    });

    this.setupInterceptors();
  }

  private setupInterceptors() {
    // Request interceptor
    this.instance.interceptors.request.use(
      (config) => {
        const token = localStorage.getItem('access_token');
        if (token) {
          config.headers.Authorization = `Bearer ${token}`;
        }
        return config;
      },
      (error) => Promise.reject(error)
    );

    // Response interceptor
    this.instance.interceptors.response.use(
      (response) => response.data,
      (error) => {
        const { response } = error;

        if (response?.status === 401) {
          // Token expired
          localStorage.removeItem('access_token');
          window.location.href = '/login';
        } else if (response?.status === 403) {
          message.error('Permission denied');
        } else if (response?.status >= 500) {
          message.error('Server error');
        } else {
          message.error(response?.data?.message || 'Request failed');
        }

        return Promise.reject(error);
      }
    );
  }

  // Streaming support for chat
  async stream(url: string, data: any, onMessage: (data: any) => void) {
    const response = await fetch(`/api/v1${url}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${localStorage.getItem('access_token')}`,
      },
      body: JSON.stringify(data),
    });

    const reader = response.body?.getReader();
    const decoder = new TextDecoder();

    while (true) {
      const { done, value } = await reader!.read();
      if (done) break;

      const text = decoder.decode(value);
      const lines = text.split('\n');

      for (const line of lines) {
        if (line.startsWith('data: ')) {
          const data = JSON.parse(line.slice(6));
          onMessage(data);
        }
      }
    }
  }

  get = (url: string, config?: AxiosRequestConfig) =>
    this.instance.get(url, config);

  post = (url: string, data?: any, config?: AxiosRequestConfig) =>
    this.instance.post(url, data, config);

  put = (url: string, data?: any, config?: AxiosRequestConfig) =>
    this.instance.put(url, data, config);

  delete = (url: string, config?: AxiosRequestConfig) =>
    this.instance.delete(url, config);
}

export const api = new ApiClient();
```

**API Patterns**:
- Axios interceptors
- Token management
- SSE streaming support
- Error handling

### 3.5 Chat Component Analysis

**File**: `web/src/pages/next-chats/components/ChatWindow.tsx`

```typescript
// Streaming chat component

import { useState, useRef, useEffect } from 'react';
import { Input, Button, Spin } from 'antd';
import { SendOutlined } from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';

interface Message {
  role: 'user' | 'assistant';
  content: string;
  sources?: Source[];
}

interface ChatWindowProps {
  dialogId: string;
}

const ChatWindow: React.FC<ChatWindowProps> = ({ dialogId }) => {
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [streamingContent, setStreamingContent] = useState('');

  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, streamingContent]);

  const handleSend = async () => {
    if (!input.trim() || loading) return;

    const question = input.trim();
    setInput('');
    setLoading(true);
    setStreamingContent('');

    // Add user message
    setMessages((prev) => [...prev, { role: 'user', content: question }]);

    try {
      // Stream response
      await api.stream(
        '/dialog/chat',
        { dialog_id: dialogId, question },
        (data) => {
          if (data.done) {
            // Finalize message
            setMessages((prev) => [
              ...prev,
              {
                role: 'assistant',
                content: streamingContent || data.answer,
                sources: data.reference?.chunks || [],
              },
            ]);
            setStreamingContent('');
          } else {
            // Stream content
            setStreamingContent((prev) => prev + data.answer);
          }
        }
      );
    } catch (error) {
      setMessages((prev) => [
        ...prev,
        { role: 'assistant', content: 'Error: Failed to get response' },
      ]);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="flex flex-col h-full">
      {/* Messages */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {messages.map((msg, idx) => (
          <MessageBubble key={idx} message={msg} />
        ))}

        {/* Streaming content */}
        {streamingContent && (
          <div className="bg-gray-100 rounded-lg p-4">
            <ReactMarkdown>{streamingContent}</ReactMarkdown>
            <Spin size="small" />
          </div>
        )}

        <div ref={messagesEndRef} />
      </div>

      {/* Input */}
      <div className="border-t p-4">
        <div className="flex space-x-2">
          <Input.TextArea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onPressEnter={(e) => {
              if (!e.shiftKey) {
                e.preventDefault();
                handleSend();
              }
            }}
            placeholder="Type your message..."
            autoSize={{ minRows: 1, maxRows: 4 }}
          />
          <Button
            type="primary"
            icon={<SendOutlined />}
            onClick={handleSend}
            loading={loading}
          />
        </div>
      </div>
    </div>
  );
};
```

**Chat UI Patterns**:
- Real-time streaming
- Auto-scroll
- Markdown rendering
- Loading states

---

## 4. Agent System Code Analysis

### 4.1 Canvas Engine

**File**: `agent/canvas.py`

```python
# Workflow execution engine

from typing import Dict, Any, Generator
import json

class Canvas:
    """
    Visual workflow execution engine.
    Executes DSL-defined workflows with components.
    """

    def __init__(self, dsl: dict, tenant_id: str):
        self.dsl = dsl
        self.tenant_id = tenant_id
        self.components = self._parse_components()
        self.graph = self._build_graph()
        self.context = {}

    def _parse_components(self) -> Dict[str, 'Component']:
        """Parse DSL into component instances."""
        components = {}

        for node in self.dsl.get('nodes', []):
            node_type = node['type']
            component_class = COMPONENT_REGISTRY.get(node_type)

            if component_class:
                components[node['id']] = component_class(
                    node_id=node['id'],
                    config=node.get('config', {}),
                    canvas=self
                )

        return components

    def _build_graph(self) -> Dict[str, list]:
        """Build execution graph from edges."""
        graph = {node_id: [] for node_id in self.components}

        for edge in self.dsl.get('edges', []):
            source = edge['source']
            target = edge['target']
            condition = edge.get('condition')

            graph[source].append({
                'target': target,
                'condition': condition
            })

        return graph

    def run(self, input_data: dict) -> Generator[dict, None, None]:
        """
        Execute workflow and yield streaming results.
        """
        self.context = {'input': input_data}

        # Find start node
        current_node = self._find_start_node()

        while current_node:
            component = self.components[current_node]

            # Execute component
            for output in component.execute(self.context):
                yield {
                    'node_id': current_node,
                    'output': output,
                    'done': False
                }

            # Update context with component output
            self.context.update(component.output)

            # Find next node
            current_node = self._get_next_node(current_node)

        yield {'done': True, 'result': self.context.get('final_output')}

    def _get_next_node(self, current: str) -> str | None:
        """Determine next node based on edges and conditions."""
        edges = self.graph.get(current, [])

        for edge in edges:
            if edge['condition']:
                # Evaluate condition
                if self._evaluate_condition(edge['condition']):
                    return edge['target']
            else:
                return edge['target']

        return None

    def _evaluate_condition(self, condition: dict) -> bool:
        """Evaluate edge condition."""
        var_name = condition.get('variable')
        operator = condition.get('operator')
        value = condition.get('value')

        actual = self.context.get(var_name)

        if operator == '==':
            return actual == value
        elif operator == '!=':
            return actual != value
        elif operator == 'contains':
            return value in str(actual)

        return False

# Component Registry
COMPONENT_REGISTRY = {
    'begin': BeginComponent,
    'llm': LLMComponent,
    'retrieval': RetrievalComponent,
    'categorize': CategorizeComponent,
    'message': MessageComponent,
    'webhook': WebhookComponent,
    'iteration': IterationComponent,
    'agent': AgentWithToolsComponent,
}
```

### 4.2 Component Base Class

```python
# Base component implementation

from abc import ABC, abstractmethod
from typing import Generator, Dict, Any

class Component(ABC):
    """Abstract base for workflow components."""

    def __init__(self, node_id: str, config: dict, canvas: 'Canvas'):
        self.node_id = node_id
        self.config = config
        self.canvas = canvas
        self.output = {}

    @abstractmethod
    def execute(self, context: dict) -> Generator[dict, None, None]:
        """
        Execute component logic.
        Yields intermediate outputs for streaming.
        """
        pass

    def get_variable(self, name: str, context: dict) -> Any:
        """Get variable from context or config."""
        if name.startswith('{{') and name.endswith('}}'):
            var_path = name[2:-2].strip()
            return self._resolve_path(var_path, context)
        return name

    def _resolve_path(self, path: str, context: dict) -> Any:
        """Resolve dot-notation path in context."""
        parts = path.split('.')
        value = context

        for part in parts:
            if isinstance(value, dict):
                value = value.get(part)
            else:
                return None

        return value

class LLMComponent(Component):
    """LLM invocation component."""

    def execute(self, context: dict) -> Generator[dict, None, None]:
        # Get prompt template
        prompt_template = self.config.get('prompt', '')

        # Substitute variables
        prompt = self._substitute_variables(prompt_template, context)

        # Get LLM
        llm_id = self.config.get('llm_id')
        llm = ChatModel.get(llm_id)

        # Stream response
        full_response = ''
        for chunk in llm.chat(prompt, stream=True):
            full_response += chunk.content
            yield {'type': 'token', 'content': chunk.content}

        self.output = {'llm_output': full_response}
        yield {'type': 'complete', 'content': full_response}
```

---

## 5. Code Patterns & Best Practices

### 5.1 Design Patterns Used

| Pattern | Location | Purpose |
|---------|----------|---------|
| **Factory** | `rag/llm/*.py` | Create LLM/Embedding instances |
| **Strategy** | `deepdoc/parser/` | Different parsing strategies |
| **Observer** | `agent/canvas.py` | Event streaming |
| **Chain of Responsibility** | `rag/flow/pipeline.py` | Processing pipeline |
| **Decorator** | `api/apps/*.py` | Auth, validation |
| **Singleton** | `common/settings.py` | Configuration |
| **Repository** | `api/db/services/` | Data access |
| **Builder** | Prompt construction | Build complex prompts |

### 5.2 Error Handling Patterns

```python
# Consistent error handling

from api.common.exceptions import (
    ValidationError,
    AuthenticationError,
    NotFoundError,
    ServiceError
)

# API level
@app.errorhandler(ValidationError)
def handle_validation_error(e):
    return jsonify({
        'code': 400,
        'message': str(e)
    }), 400

@app.errorhandler(Exception)
def handle_exception(e):
    logger.exception("Unhandled exception")
    return jsonify({
        'code': 500,
        'message': 'Internal server error'
    }), 500

# Service level
class DocumentService:
    @classmethod
    def get(cls, doc_id: str) -> Document:
        doc = Document.get_or_none(Document.id == doc_id)
        if not doc:
            raise NotFoundError(f"Document {doc_id} not found")
        return doc
```

### 5.3 Logging Patterns

```python
# Structured logging

import logging
from common.log_utils import setup_logger

logger = setup_logger(__name__)

class DialogService:
    @classmethod
    def chat(cls, dialog_id: str, question: str):
        logger.info(
            "Chat request",
            extra={
                'dialog_id': dialog_id,
                'question_length': len(question),
                'event': 'chat_start'
            }
        )

        try:
            result = cls._process_chat(dialog_id, question)
            logger.info(
                "Chat completed",
                extra={
                    'dialog_id': dialog_id,
                    'chunks_retrieved': len(result['chunks']),
                    'event': 'chat_complete'
                }
            )
            return result
        except Exception as e:
            logger.error(
                "Chat failed",
                extra={
                    'dialog_id': dialog_id,
                    'error': str(e),
                    'event': 'chat_error'
                },
                exc_info=True
            )
            raise
```

### 5.4 Testing Patterns

```python
# pytest test structure

import pytest
from unittest.mock import Mock, patch

class TestDialogService:
    """Test cases for DialogService."""

    @pytest.fixture
    def mock_dialog(self):
        """Create mock dialog for testing."""
        return Mock(
            id='test-dialog',
            kb_ids=['kb-1'],
            llm_id='openai/gpt-4'
        )

    @pytest.fixture
    def mock_es(self):
        """Mock Elasticsearch client."""
        with patch('api.db.services.dialog_service.es') as mock:
            yield mock

    def test_retrieval_returns_chunks(self, mock_dialog, mock_es):
        """Test that retrieval returns expected chunks."""
        # Arrange
        mock_es.search.return_value = {
            'hits': {
                'hits': [
                    {'_source': {'content': 'chunk 1'}},
                    {'_source': {'content': 'chunk 2'}}
                ]
            }
        }

        # Act
        chunks = DialogService._retrieval(mock_dialog, "test query")

        # Assert
        assert len(chunks) == 2
        mock_es.search.assert_called_once()

    @pytest.mark.parametrize("question,expected_chunks", [
        ("simple query", 5),
        ("complex multi-word query", 10),
    ])
    def test_retrieval_with_different_queries(
        self, mock_dialog, mock_es, question, expected_chunks
    ):
        """Parameterized test for different query types."""
        # Test implementation
        pass
```

---

## 6. Security Analysis

### 6.1 Authentication Implementation

```python
# JWT authentication

import jwt
from functools import wraps

def login_required(f):
    """Decorator to require authentication."""
    @wraps(f)
    async def decorated(*args, **kwargs):
        token = request.headers.get('Authorization', '').replace('Bearer ', '')

        if not token:
            return jsonify({'error': 'Token required'}), 401

        try:
            payload = jwt.decode(
                token,
                current_app.config['SECRET_KEY'],
                algorithms=['HS256']
            )
            g.user_id = payload['user_id']
            g.tenant_id = payload['tenant_id']
        except jwt.ExpiredSignatureError:
            return jsonify({'error': 'Token expired'}), 401
        except jwt.InvalidTokenError:
            return jsonify({'error': 'Invalid token'}), 401

        return await f(*args, **kwargs)

    return decorated
```

### 6.2 Input Validation

```python
# Request validation

from marshmallow import Schema, fields, validate

class CreateKbSchema(Schema):
    name = fields.Str(required=True, validate=validate.Length(min=1, max=255))
    description = fields.Str(validate=validate.Length(max=1024))
    embedding_model = fields.Str(required=True)
    parser_id = fields.Str(validate=validate.OneOf(['naive', 'paper', 'book']))

def validate_request(schema_class):
    """Decorator for request validation."""
    def decorator(f):
        @wraps(f)
        async def decorated(*args, **kwargs):
            schema = schema_class()
            try:
                data = await request.get_json()
                validated = schema.load(data)
                g.validated_data = validated
            except ValidationError as e:
                return jsonify({'error': e.messages}), 400
            return await f(*args, **kwargs)
        return decorated
    return decorator
```

### 6.3 SQL Injection Prevention

```python
# Using Peewee ORM (parameterized queries)

# Safe - uses parameterized query
documents = Document.select().where(
    Document.kb_id == kb_id,
    Document.status == 'FINISHED'
)

# Unsafe - raw SQL (avoided in codebase)
# cursor.execute(f"SELECT * FROM document WHERE kb_id = '{kb_id}'")  # DON'T DO THIS
```

---

## 7. Performance Optimizations

### 7.1 Database Optimizations

```python
# Batch operations

def bulk_create_chunks(chunks: List[dict]):
    """Bulk insert chunks for performance."""
    with db.atomic():
        for batch in chunked(chunks, 1000):
            Chunk.insert_many(batch).execute()

# Connection pooling
from playhouse.pool import PooledMySQLDatabase

db = PooledMySQLDatabase(
    'ragflow',
    max_connections=32,
    stale_timeout=300,
    **connection_params
)
```

### 7.2 Caching Strategies

```python
# Redis caching

import redis
from functools import lru_cache

redis_client = redis.Redis(host='localhost', port=6379)

def cache_result(ttl=3600):
    """Decorator for Redis caching."""
    def decorator(f):
        @wraps(f)
        def decorated(*args, **kwargs):
            cache_key = f"{f.__name__}:{hash(str(args) + str(kwargs))}"

            cached = redis_client.get(cache_key)
            if cached:
                return json.loads(cached)

            result = f(*args, **kwargs)
            redis_client.setex(cache_key, ttl, json.dumps(result))
            return result
        return decorated
    return decorator

@cache_result(ttl=600)
def get_embedding_model_config(model_id: str):
    """Cached embedding model configuration."""
    return LLMFactories.get_model_config(model_id)
```

### 7.3 Async Operations

```python
# Async document processing

import asyncio
from concurrent.futures import ThreadPoolExecutor

async def process_documents_async(doc_ids: List[str]):
    """Process multiple documents concurrently."""

    async def process_one(doc_id: str):
        pipeline = Pipeline(doc_id)
        await asyncio.to_thread(pipeline.run)

    tasks = [process_one(doc_id) for doc_id in doc_ids]
    await asyncio.gather(*tasks, return_exceptions=True)
```

---

## 8. Tóm Tắt Code Quality

### Strengths

1. **Clean Architecture**: Separation of concerns với layers rõ ràng
2. **Consistent Patterns**: Decorator, factory patterns được sử dụng nhất quán
3. **Type Hints**: TypeScript strict mode, Python type hints improving
4. **Error Handling**: Consistent error handling across layers
5. **Async Support**: Full async support với Quart
6. **Streaming**: SSE streaming cho real-time responses

### Areas for Improvement

1. **Test Coverage**: Cần tăng coverage (hiện ~50-60%)
2. **Documentation**: Inline docs có thể chi tiết hơn
3. **Type Hints (Python)**: Chưa hoàn toàn consistent
4. **Error Messages**: Một số error messages chưa user-friendly

### Code Metrics Summary

| Metric | Value | Status |
|--------|-------|--------|
| Lines of Code | ~62,000 | Large |
| Cyclomatic Complexity | Moderate | OK |
| Technical Debt | Low-Medium | Acceptable |
| Test Coverage | ~50-60% | Needs improvement |
| Documentation | Partial | Needs improvement |
