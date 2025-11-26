# Embedding Generation

## Tong Quan

Embedding generation chuyển đổi text thành dense vectors để thực hiện semantic search.

## File Location
```
/rag/llm/embedding_model.py
```

## Supported Embedding Models

| Provider | Class | Max Tokens | Dimensions |
|----------|-------|-----------|------------|
| OpenAI | `OpenAIEmbed` | 8191 | 1536/3072 |
| Azure OpenAI | `AzureEmbed` | Custom | 1536/3072 |
| Builtin | `BuiltinEmbed` | 8000 | Varies |
| Qwen | `QWenEmbed` | 2048 | 1024 |
| ZHIPU-AI | `ZhipuEmbed` | 512-3072 | 1024 |
| Jina | `JinaEmbed` | 8196 | 768/1024 |
| Mistral | `MistralEmbed` | 8196 | 1024 |
| Voyage AI | `VoyageEmbed` | Custom | 1024 |
| Cohere | `CoHereEmbed` | Custom | 1024 |
| NVIDIA | `NvidiaEmbed` | Custom | 1024 |

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      TEXT INPUT                                  │
│            ["chunk1", "chunk2", "chunk3", ...]                  │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                 TEXT PREPROCESSING                               │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  1. Token counting                                       │   │
│  │  2. Truncation to max_tokens                            │   │
│  │  3. Batch splitting (16 texts/batch)                    │   │
│  └─────────────────────────────────────────────────────────┘   │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                 EMBEDDING MODEL                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  OpenAI / Jina / Cohere / Local Model                   │   │
│  │  → API call with batch                                  │   │
│  │  → Return vectors + token count                         │   │
│  └─────────────────────────────────────────────────────────┘   │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                 OUTPUT VECTORS                                   │
│  np.ndarray (N x D) where N=texts, D=dimensions                 │
│  + total_token_count                                            │
└─────────────────────────────────────────────────────────────────┘
```

## Base Implementation

```python
class Base(ABC):
    def __init__(self, key, model_name, **kwargs):
        """Abstract base for all embedding models"""
        pass

    def encode(self, texts: list) -> tuple[np.ndarray, int]:
        """
        Encode texts to embeddings.

        Args:
            texts: List of strings to embed

        Returns:
            (embeddings, token_count): NumPy array and total tokens used
        """
        raise NotImplementedError()

    def encode_queries(self, text: str) -> tuple[np.ndarray, int]:
        """Encode single query text."""
        raise NotImplementedError()
```

## OpenAI Embedding

```python
class OpenAIEmbed(Base):
    def __init__(self, key, model_name, **kwargs):
        self.client = OpenAI(
            api_key=key,
            base_url=kwargs.get("base_url", "https://api.openai.com/v1")
        )
        self.model_name = model_name

    def encode(self, texts: list):
        batch_size = 16  # OpenAI max
        texts = [truncate(t, 8191) for t in texts]  # Token limit

        ress = []
        total_tokens = 0

        for i in range(0, len(texts), batch_size):
            res = self.client.embeddings.create(
                input=texts[i : i + batch_size],
                model=self.model_name,
                encoding_format="float",
                extra_body={"drop_params": True}
            )
            ress.extend([d.embedding for d in res.data])
            total_tokens += res.usage.total_tokens

        return np.array(ress), total_tokens
```

## Builtin Embedding (HuggingFace TEI)

```python
class BuiltinEmbed(Base):
    _model = None
    _model_name = ""
    _model_lock = threading.Lock()  # Thread-safe initialization

    MAX_TOKENS = {
        "BAAI/bge-large-zh-v1.5": 500,
        "BAAI/bge-m3": 8000,
        "maidalun1020/bce-embedding-base_v1": 500,
        "jina-embeddings-v3": 30000,
    }

    def __init__(self, key, model_name, **kwargs):
        if not BuiltinEmbed._model and "tei-" in os.getenv("COMPOSE_PROFILES", ""):
            with BuiltinEmbed._model_lock:
                # Lazy load HuggingFace TEI model
                BuiltinEmbed._model = HuggingFaceEmbed(
                    embedding_cfg["api_key"],
                    settings.EMBEDDING_MDL,
                    base_url=embedding_cfg["base_url"]
                )
        self._model = BuiltinEmbed._model

    def encode(self, texts: list):
        return self._model.encode(texts)
```

## Qwen Embedding with Retry

```python
class QWenEmbed(Base):
    def encode(self, texts: list):
        import dashscope

        batch_size = 4
        texts = [truncate(t, 2048) for t in texts]
        res = []

        for i in range(0, len(texts), batch_size):
            retry_max = 5
            resp = dashscope.TextEmbedding.call(
                model=self.model_name,
                input=texts[i : i + batch_size],
                api_key=self.key,
                text_type="document"
            )

            # Retry if empty response
            while resp["output"] is None and retry_max > 0:
                time.sleep(10)
                resp = dashscope.TextEmbedding.call(...)
                retry_max -= 1

            if retry_max == 0:
                raise LookupError("Retry_max reached")

            res.extend([d["embedding"] for d in resp["output"]["embeddings"]])

        return np.array(res), total_tokens
```

## Embedding Workflow in RAG

```python
# In task_executor.py - build_chunks()

async def embedding(chunks, embd_mdl, parser_config):
    """Generate embeddings for chunks."""

    # Prepare text for embedding
    texts_to_embed = []
    for chunk in chunks:
        # Option 1: Title + Content weighted embedding
        if parser_config.get("title_weight", 0) > 0:
            text = chunk["title"] + " " + chunk["content"]
        # Option 2: Question-based embedding
        elif parser_config.get("question_kwd"):
            text = chunk["question_kwd"]
        else:
            text = chunk["content_with_weight"]

        texts_to_embed.append(text)

    # Batch embedding
    batch_size = 16
    for i in range(0, len(texts_to_embed), batch_size):
        batch = texts_to_embed[i:i+batch_size]
        embeddings, tokens = embd_mdl.encode(batch)

        # Store vectors in chunks
        for j, emb in enumerate(embeddings):
            chunk_idx = i + j
            chunks[chunk_idx][f"q_{len(emb)}_vec"] = emb.tolist()
```

## Title-Content Weighted Embedding

```python
def weighted_embedding(title_emb, content_emb, title_weight=0.1):
    """
    Combine title and content embeddings.

    Formula:
        weighted_vec = title_weight * title_emb + (1 - title_weight) * content_emb
    """
    return title_weight * title_emb + (1 - title_weight) * content_emb
```

## Vector Storage

```python
# Elasticsearch mapping
{
    "q_1024_vec": {
        "type": "dense_vector",
        "dims": 1024,
        "index": true,
        "similarity": "cosine"
    }
}

# Vector field naming convention
# q_{dimension}_vec
# Examples: q_768_vec, q_1024_vec, q_1536_vec, q_3072_vec
```

## Performance Optimization

### Batch Processing
```python
# OpenAI: 16 texts per batch
# Qwen: 4 texts per batch
# Others: 8-16 texts per batch
```

### Text Truncation
```python
def truncate(text: str, max_tokens: int) -> str:
    """Truncate text to fit within token limit."""
    token_count = num_tokens_from_string(text)
    if token_count <= max_tokens:
        return text

    # Truncate with safety margin
    target_len = int(len(text) * max_tokens / token_count * 0.95)
    return text[:target_len]
```

### Caching
```python
# Model instances are not cached between requests
# But configuration is cached in database
# Thread-safe lazy initialization for builtin models
```

## Configuration

```python
EMBEDDING_CFG = {
    "factory": "OpenAI",
    "api_key": os.getenv("OPENAI_API_KEY"),
    "base_url": "https://api.openai.com/v1",
    "model": "text-embedding-ada-002"
}

# Parser config for embedding behavior
{
    "title_weight": 0.1,  # Weight for title embedding
    "question_kwd": False,  # Use generated questions for embedding
}
```

## Related Files

- `/rag/llm/embedding_model.py` - All embedding implementations
- `/rag/svr/task_executor.py` - Embedding generation in pipeline
- `/rag/utils/es_conn.py` - Vector storage in Elasticsearch
