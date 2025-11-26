# 06-ALGORITHMS - Core Algorithms & Math

## Tong Quan

Module này chứa các thuật toán core của RAGFlow bao gồm scoring, similarity, chunking, và advanced RAG techniques.

## Kien Truc Algorithms

```
┌─────────────────────────────────────────────────────────────────┐
│                    CORE ALGORITHMS                               │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│  RETRIEVAL ALGORITHMS                                            │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│  │  BM25 Scoring   │  │ Vector Cosine   │  │ Hybrid Fusion   │ │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘ │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│  ADVANCED RAG                                                    │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│  │  RAPTOR         │  │  GraphRAG       │  │  Cross-Encoder  │ │
│  │  (Hierarchical) │  │  (Knowledge G)  │  │  (Reranking)    │ │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘ │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│  TEXT PROCESSING                                                 │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│  │  TF-IDF Weight  │  │  Tokenization   │  │  Query Expand   │ │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

## Files Trong Module Nay

| File | Mo Ta |
|------|-------|
| [bm25_scoring.md](./bm25_scoring.md) | BM25 ranking algorithm |
| [vector_similarity.md](./vector_similarity.md) | Cosine similarity calculations |
| [hybrid_score_fusion.md](./hybrid_score_fusion.md) | Score combination strategies |
| [tfidf_weighting.md](./tfidf_weighting.md) | TF-IDF term weighting |
| [raptor_algorithm.md](./raptor_algorithm.md) | Hierarchical summarization |
| [graphrag_implementation.md](./graphrag_implementation.md) | Knowledge graph RAG |

## Algorithm Formulas

### BM25 Scoring
```
BM25(D, Q) = Σ IDF(qi) × (f(qi, D) × (k1 + 1)) / (f(qi, D) + k1 × (1 - b + b × |D|/avgdl))

where:
    f(qi, D) = term frequency of qi in document D
    |D| = document length
    avgdl = average document length
    k1 = 1.2 (term frequency saturation)
    b = 0.75 (length normalization)
```

### Cosine Similarity
```
cos(θ) = (A · B) / (||A|| × ||B||)

where:
    A, B = embedding vectors
    A · B = dot product
    ||A|| = L2 norm
```

### Hybrid Score Fusion
```
Hybrid_Score = α × BM25_Score + (1-α) × Vector_Score

Default: α = 0.05 (5% BM25, 95% Vector)
```

### TF-IDF Weighting
```
IDF(term) = log10(10 + (N - df(term) + 0.5) / (df(term) + 0.5))
Weight = (0.3 × IDF1 + 0.7 × IDF2) × NER × PoS
```

### Cross-Encoder Reranking
```
Final_Rank = α × Token_Sim + β × Vector_Sim + γ × Rank_Features

where:
    α = 0.3 (token weight)
    β = 0.7 (vector weight)
    γ = variable (PageRank, tag boost)
```

## Algorithm Parameters

| Algorithm | Parameter | Default | Range |
|-----------|-----------|---------|-------|
| **BM25** | k1 | 1.2 | 0-2.0 |
| | b | 0.75 | 0-1.0 |
| **Hybrid** | vector_weight | 0.95 | 0-1.0 |
| | text_weight | 0.05 | 0-1.0 |
| **TF-IDF** | IDF1 weight | 0.3 | - |
| | IDF2 weight | 0.7 | - |
| **Chunking** | chunk_size | 512 | 128-2048 |
| | overlap | 0-10% | 0-100% |
| **RAPTOR** | max_clusters | 10-50 | - |
| | GMM threshold | 0.1 | - |
| **GraphRAG** | entity_topN | 6 | 1-100 |
| | similarity_threshold | 0.3 | 0-1.0 |

## Key Implementation Files

- `/rag/nlp/search.py` - Search algorithms
- `/rag/nlp/term_weight.py` - TF-IDF implementation
- `/rag/nlp/query.py` - Query processing
- `/rag/raptor.py` - RAPTOR algorithm
- `/graphrag/search.py` - GraphRAG search
- `/rag/nlp/__init__.py` - Chunking algorithms

## Performance Metrics

| Metric | Typical Value |
|--------|---------------|
| Vector Search Latency | < 100ms |
| BM25 Search Latency | < 50ms |
| Reranking Latency | 200-500ms |
| Total Retrieval | < 1s |
