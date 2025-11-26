# 03-RAG-ENGINE - Retrieval-Augmented Generation

## Tổng Quan

RAG Engine là core của RAGFlow, implement các thuật toán retrieval, embedding, reranking và generation.

## Kiến Trúc RAG Engine

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          RAG ENGINE ARCHITECTURE                         │
└─────────────────────────────────────────────────────────────────────────┘

                    ┌─────────────────────────────┐
                    │         User Query          │
                    └──────────────┬──────────────┘
                                   │
                                   ▼
┌───────────────────────────────────────────────────────────────────────┐
│                       QUERY PROCESSING                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                   │
│  │  Tokenize   │→ │  TF-IDF     │→ │  Synonym    │                   │
│  │  Query      │  │  Weight     │  │  Expansion  │                   │
│  └─────────────┘  └─────────────┘  └─────────────┘                   │
└────────────────────────────────────┬──────────────────────────────────┘
                                     │
                    ┌────────────────┴────────────────┐
                    │                                 │
                    ▼                                 ▼
┌───────────────────────────────┐  ┌───────────────────────────────────┐
│      VECTOR SEARCH            │  │        BM25 SEARCH                │
│  ┌─────────────────────────┐  │  │  ┌─────────────────────────────┐  │
│  │  Embedding Model        │  │  │  │  Full-text Query            │  │
│  │  (OpenAI/BGE/Jina)      │  │  │  │  (Elasticsearch)            │  │
│  └───────────┬─────────────┘  │  │  └───────────┬─────────────────┘  │
│              │                │  │              │                    │
│  ┌───────────▼─────────────┐  │  │  ┌───────────▼─────────────────┐  │
│  │  Cosine Similarity      │  │  │  │  BM25 Scoring               │  │
│  │  Score (0-1)            │  │  │  │  Score                      │  │
│  └───────────┬─────────────┘  │  │  └───────────┬─────────────────┘  │
└──────────────┼────────────────┘  └──────────────┼────────────────────┘
               │                                   │
               └───────────────┬───────────────────┘
                               │
                               ▼
┌───────────────────────────────────────────────────────────────────────┐
│                       SCORE FUSION                                     │
│                                                                        │
│   Final = α × Vector_Score + (1-α) × BM25_Score                       │
│   where α = vector_similarity_weight (default: 0.3)                   │
│                                                                        │
└────────────────────────────────┬──────────────────────────────────────┘
                                 │
                                 ▼
┌───────────────────────────────────────────────────────────────────────┐
│                       RERANKING (Optional)                             │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │  Cross-Encoder Model (Jina/Cohere/BGE)                          │  │
│  │  Re-score each chunk against query                              │  │
│  │  Return Top-K after reranking                                   │  │
│  └─────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────┬──────────────────────────────────────┘
                                 │
                                 ▼
┌───────────────────────────────────────────────────────────────────────┐
│                       CONTEXT BUILDING                                 │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │  Format chunks into context string                              │  │
│  │  Add metadata (doc name, page, positions)                       │  │
│  │  Build citation mapping                                         │  │
│  └─────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────┬──────────────────────────────────────┘
                                 │
                                 ▼
┌───────────────────────────────────────────────────────────────────────┐
│                       LLM GENERATION                                   │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │  System Prompt + Context + User Query                           │  │
│  │  Token Fitting (stay within context window)                     │  │
│  │  Streaming Generation                                           │  │
│  │  Citation Insertion                                             │  │
│  └─────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────────┘
```

## Cấu Trúc Thư Mục

```
/rag/
├── llm/                      # LLM Model Abstractions
│   ├── chat_model.py         # Chat LLM interface (30+ providers)
│   ├── embedding_model.py    # Embedding models
│   ├── rerank_model.py       # Reranking models
│   ├── cv_model.py           # Computer vision
│   └── tts_model.py          # Text-to-speech
│
├── nlp/                      # NLP Processing
│   ├── query.py              # Query processing
│   ├── search.py             # Search & retrieval ⭐
│   └── rag_tokenizer.py      # Tokenization
│
├── app/                      # RAG Application
│   └── naive.py              # Naive RAG implementation
│
├── flow/                     # Processing Pipeline
│   ├── pipeline.py           # Pipeline orchestration
│   ├── parser/               # Document parsing
│   ├── tokenizer/            # Tokenization
│   ├── splitter/             # Chunking
│   └── extractor/            # Information extraction
│
├── utils/                    # Utilities
│   ├── es_conn.py            # Elasticsearch connection
│   └── infinity_conn.py      # Infinity connection
│
├── prompts/                  # Prompt Templates
│   ├── generator.py          # Prompt generator
│   ├── citations.md          # Citation prompt
│   ├── keywords.md           # Keyword extraction
│   └── ...                   # Other templates
│
├── raptor.py                 # RAPTOR algorithm
├── settings.py               # Configuration
└── benchmark.py              # Performance testing
```

## Files Trong Module Này

| File | Mô Tả |
|------|-------|
| [hybrid_search_algorithm.md](./hybrid_search_algorithm.md) | Thuật toán Hybrid Search (Vector + BM25) |
| [embedding_generation.md](./embedding_generation.md) | Text embedding và vector generation |
| [rerank_algorithm.md](./rerank_algorithm.md) | Cross-encoder reranking |
| [chunking_strategies.md](./chunking_strategies.md) | Document chunking strategies |
| [prompt_engineering.md](./prompt_engineering.md) | Prompt construction |
| [query_processing.md](./query_processing.md) | Query analysis |

## Core Algorithms

### 1. Hybrid Search

```python
# Score fusion formula
Final_Score = α × Vector_Score + (1-α) × BM25_Score

where:
    α = vector_similarity_weight (default: 0.3)
    Vector_Score = cosine_similarity(query_embedding, chunk_embedding)
    BM25_Score = normalized_bm25(query_tokens, chunk_tokens)
```

### 2. BM25 Scoring

```python
# BM25 formula
BM25(D, Q) = Σ IDF(qi) × (f(qi, D) × (k1 + 1)) / (f(qi, D) + k1 × (1 - b + b × |D|/avgdl))

where:
    f(qi, D) = term frequency of qi in document D
    |D| = document length
    avgdl = average document length
    k1 = 1.2 (term frequency saturation)
    b = 0.75 (length normalization)
```

### 3. Cosine Similarity

```python
# Cosine similarity formula
cos(θ) = (A · B) / (||A|| × ||B||)

where:
    A, B = embedding vectors
    A · B = dot product
    ||A|| = L2 norm
```

### 4. Cross-Encoder Reranking

```python
# Reranking score
Rerank_Score = CrossEncoder(query, document)

# Final ranking
Final_Rank = α × Token_Similarity + β × Vector_Similarity + γ × Rank_Features

where:
    α = 0.3 (token weight)
    β = 0.7 (vector weight)
    γ = variable (PageRank, tag boost)
```

## LLM Provider Support

### Chat Models (30+)

| Provider | Models |
|----------|--------|
| OpenAI | GPT-3.5, GPT-4, GPT-4V |
| Anthropic | Claude 3 (Opus, Sonnet, Haiku) |
| Google | Gemini Pro |
| Alibaba | Qwen, Qwen-VL |
| Groq | LLaMA 3, Mixtral |
| Mistral | Mistral 7B, Mixtral 8x7B |
| Cohere | Command R, Command R+ |
| DeepSeek | DeepSeek Chat |
| Ollama | Local models |
| ... | And many more |

### Embedding Models

| Provider | Models | Dimensions |
|----------|--------|------------|
| OpenAI | text-embedding-3-small | 1536 |
| OpenAI | text-embedding-3-large | 3072 |
| BGE | bge-large-en-v1.5 | 1024 |
| BGE | bge-m3 | 1024 |
| Jina | jina-embeddings-v2 | 768 |
| Cohere | embed-english-v3 | 1024 |

### Reranking Models

| Provider | Models |
|----------|--------|
| Jina | jina-reranker-v2 |
| Cohere | rerank-english-v3 |
| BGE | bge-reranker-large |
| NVIDIA | rerank-qa-mistral-4b |

## Configuration Parameters

### Search Configuration

```python
{
    "similarity_threshold": 0.2,      # Minimum similarity
    "vector_similarity_weight": 0.3,  # α in fusion formula
    "top_n": 6,                       # Final results count
    "top_k": 1024,                    # Initial candidates
    "rerank_model": "jina-reranker-v2"
}
```

### Chunking Configuration

```python
{
    "chunk_token_num": 512,           # Tokens per chunk
    "delimiter": "\n!?。；！？",       # Split delimiters
    "layout_recognize": "DeepDOC",    # Layout detection
    "overlapped_percent": 0           # Chunk overlap
}
```

### Generation Configuration

```python
{
    "temperature": 0.7,
    "max_tokens": 2048,
    "top_p": 1.0,
    "frequency_penalty": 0.0,
    "presence_penalty": 0.0
}
```

## Key Performance Metrics

| Metric | Typical Value | Description |
|--------|---------------|-------------|
| Vector Search Latency | < 100ms | Elasticsearch query time |
| BM25 Search Latency | < 50ms | Full-text search time |
| Reranking Latency | 200-500ms | Cross-encoder inference |
| Embedding Generation | 1-5s/batch | Per batch of 16 texts |
| Total Retrieval | < 1s | End-to-end search |

## Related Files

- `/api/db/services/dialog_service.py` - Uses RAG engine
- `/rag/nlp/search.py` - Core search implementation
- `/rag/utils/es_conn.py` - Elasticsearch queries
