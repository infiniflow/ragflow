# RAGFlow API Examples

This directory contains comprehensive examples for using RAGFlow's API via Python SDK and HTTP requests (cURL).

## 📁 Directory Structure

```
example/
├── sdk/                    # Python SDK examples
│   ├── dataset_example.py  # Dataset CRUD operations
│   ├── chat_example.py     # Chat management and completions
│   ├── document_example.py # Document upload and processing
│   ├── retrieval_example.py # Knowledge base retrieval
│   └── agent_example.py    # Agent CRUD and session management
│
├── http/                   # cURL/HTTP examples
│   ├── dataset_example.sh
│   ├── chat_example.sh
│   ├── document_example.sh
│   ├── retrieval_example.sh
│   └── agent_example.sh
│
└── chat_demo/              # Chat demo application
```

## 🚀 Quick Start

### Prerequisites

1. **Running RAGFlow Instance**
   ```bash
   # Start RAGFlow with Docker
   docker compose up -d
   ```

2. **API Key**
   - Log in to RAGFlow admin panel (default: http://localhost:9380)
   - Go to Settings → API Keys
   - Generate a new API key (format: `ragflow-{token}`)

3. **Python SDK** (for SDK examples)
   ```bash
   pip install ragflow-sdk
   ```

### Configuration

Replace these values in the examples:

| Variable | Location | How to Find |
|----------|----------|-------------|
| `API_KEY` | All examples | RAGFlow admin panel → Settings → API Keys |
| `HOST_ADDRESS` | All examples | Your RAGFlow server URL (default: `http://127.0.0.1`) |
| `AGENT_ID` | Agent examples | Response from create agent or list agents |
| `SESSION_ID` | Agent examples | Response from create session |
| `DATASET_ID` | Dataset examples | Response from create dataset or list datasets |

## 📚 Example Guide

### 1. Dataset Examples

**What it demonstrates:**
- Create, read, update, delete (CRUD) datasets/knowledge bases
- List datasets with pagination
- Include parsing status

**Python SDK:**
```bash
python example/sdk/dataset_example.py
```

**HTTP/cURL:**
```bash
bash example/http/dataset_example.sh
```

---

### 2. Chat Examples

**What it demonstrates:**
- Chat conversation management (create, update, delete)
- List chats with filtering
- Chat completions (streaming and non-streaming)
- Session management

**Python SDK:**
```bash
python example/sdk/chat_example.py
```

**HTTP/cURL:**
```bash
bash example/http/chat_example.sh
```

**Key Features:**
- **Streaming:** Real-time response chunks via Server-Sent Events (SSE)
- **Non-streaming:** Single complete response
- **Session persistence:** Multi-turn conversations with history

---

### 3. Document Examples

**What it demonstrates:**
- Upload documents to knowledge bases
- Parse documents (chunking, embeddings)
- Retrieve document chunks
- Monitor parsing progress
- Delete documents

**Python SDK:**
```bash
python example/sdk/document_example.py
```

**HTTP/cURL:**
```bash
bash example/http/document_example.sh
```

**Supported Formats:**
- PDF, DOCX, TXT, MD, HTML
- Images (with OCR): PNG, JPG, JPEG
- Excel: XLSX, XLS
- PowerPoint: PPTX

---

### 4. Retrieval Examples

**What it demonstrates:**
- Semantic search across knowledge bases
- Similarity-based retrieval
- Multi-dataset search
- Ranking and scoring

**Python SDK:**
```bash
python example/sdk/retrieval_example.py
```

**HTTP/cURL:**
```bash
bash example/http/retrieval_example.sh
```

**Parameters:**
- `question`: Search query
- `datasets`: List of dataset IDs to search
- `document_ids`: (Optional) Specific documents to search
- `offset`/`limit`: Pagination
- `similarity_threshold`: Minimum similarity score (0.0-1.0)
- `vector_similarity_weight`: Balance between vector and keyword search

---

### 5. Agent Examples ⭐ **NEW**

**What it demonstrates:**
- Create and manage AI agents with custom workflows
- Agent session management
- Execute agents (streaming/non-streaming)
- Agent DSL (Domain Specific Language) structure
- Basic agent components (Begin, LLM)

**Python SDK:**
```bash
python example/sdk/agent_example.py
```

**HTTP/cURL:**
```bash
bash example/http/agent_example.sh
```

**Agent Workflow:**
```
1. Create Agent → 2. Create Session → 3. Execute Agent → 4. Delete Session → 5. Delete Agent
```

**DSL Components Available:**
- `Begin` - Entry point (Chat or Webhook)
- `LLM` - Language model interaction
- `Retrieval` - Knowledge base search
- `Agent` - Call sub-agents
- `Code` - Execute Python code
- `Switch` - Conditional branching
- `Categorize` - Text classification
- And many more... (see `/agent/component/` directory)

---

## 🔧 Common Usage Patterns

### Pattern 1: Basic CRUD Operations

All examples follow this pattern:
```python
# 1. Initialize RAGFlow client
ragflow = RAGFlow(api_key=API_KEY, base_url=HOST_ADDRESS)

# 2. Create resource
resource = ragflow.create_xxx(...)

# 3. Update resource
resource.update({...})

# 4. List resources
resources = ragflow.list_xxx(page=1, page_size=10)

# 5. Delete resource
ragflow.delete_xxx(ids=[resource.id])
```

### Pattern 2: Streaming Responses

For chat and agent completions:
```python
# Enable streaming
response = chat.stream_completion(question="...", stream=True)

# Process chunks as they arrive
for chunk in response:
    if chunk.get("event") == "message":
        print(chunk["data"]["content"])
```

### Pattern 3: Error Handling

All examples include error handling:
```python
try:
    # API operations
    result = ragflow.create_xxx(...)
except Exception as e:
    print(f"Error: {str(e)}")
    sys.exit(-1)
```

---

## 🔐 Authentication

All API requests require authentication via Bearer token:

**HTTP Header:**
```
Authorization: Bearer ragflow-{your_api_key}
```

**Python SDK:**
```python
ragflow = RAGFlow(api_key="ragflow-{your_api_key}", base_url="http://127.0.0.1")
```

---

## 🌐 API Endpoints

### Base URL
```
http://{host}:{port}/api/v1
```
Default: `http://localhost:9380/api/v1`

### Endpoint Categories

| Category | Endpoints | Example File |
|----------|-----------|--------------|
| Datasets | `/datasets` | `dataset_example.*` |
| Documents | `/datasets/{id}/documents` | `document_example.*` |
| Chats | `/chats`, `/chats/{id}/completions` | `chat_example.*` |
| Retrieval | `/retrieval` | `retrieval_example.*` |
| Agents | `/agents`, `/agents/{id}/sessions`, `/agents/{id}/completions` | `agent_example.*` |

---

## 🐛 Troubleshooting

### Common Issues

**1. Connection Refused**
```
Error: Connection refused to http://localhost:9380
```
**Solution:** Ensure RAGFlow is running (`docker compose ps`)

**2. Authentication Failed**
```
Error: 401 Unauthorized
```
**Solution:** 
- Check API key is valid
- Verify Bearer token format: `Bearer ragflow-{token}`

**3. Agent Execution Timeout**
```
Error: Agent execution timeout
```
**Solution:**
- Check agent DSL is valid
- Ensure LLM configuration is correct
- Verify knowledge base IDs exist (if using Retrieval component)

**4. Document Parsing Stuck**
```
Document status: "PENDING" for long time
```
**Solution:**
- Check RAGFlow worker logs: `docker compose logs ragflow-worker`
- Verify document format is supported
- Ensure OCR service is running (for images/scanned PDFs)

**5. Module Not Found (Python)**
```
ModuleNotFoundError: No module named 'ragflow_sdk'
```
**Solution:**
```bash
pip install ragflow-sdk
```

---

## 📖 Additional Resources

### Documentation
- [HTTP API Reference](../docs/references/http_api_reference.md)
- [Python SDK Reference](../docs/references/python_api_reference.md)
- [Agent Guide](../docs/guides/agent/agent_introduction.md)
- [Agent Components](../docs/guides/agent/agent_component_reference/)

### GitHub
- [RAGFlow Repository](https://github.com/infiniflow/ragflow)
- [Report Issues](https://github.com/infiniflow/ragflow/issues)
- [Contribute](../CONTRIBUTING.md)

### Community
- [Discord](https://discord.gg/ragflow) - Get help and discuss
- [Discussions](https://github.com/infiniflow/ragflow/discussions) - Q&A and ideas

---

## 💡 Tips

1. **Start Simple:** Begin with dataset and document examples before moving to agents
2. **Use Environment Variables:** Store API keys in `.env` files (never commit them!)
3. **Test with Streaming:** Use streaming for better UX in production applications
4. **Monitor Parsing:** Always check document parsing status before retrieval
5. **Optimize Agents:** Use agent traces (`return_trace: true`) to debug workflow execution
6. **Batch Operations:** Use list operations with pagination for better performance
7. **Resource Cleanup:** Always delete test resources to avoid clutter

---

## 🤝 Contributing

Found an issue or want to add more examples? Contributions are welcome!

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/new-example`)
3. Add your example following the existing pattern
4. Test thoroughly
5. Submit a pull request

---

## 📄 License

All examples are licensed under Apache License 2.0.

---

## 🙏 Acknowledgments

Thanks to all contributors who help improve RAGFlow's documentation and examples!

---

**Last Updated:** April 2025  
**Version:** RAGFlow v1.0+
