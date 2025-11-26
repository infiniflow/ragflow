# Rerank Algorithm

## Tong Quan

Reranking sử dụng cross-encoder models để re-score và sắp xếp lại search results dựa trên query-document relevance.

## File Location
```
/rag/llm/rerank_model.py
/rag/nlp/search.py (rerank_by_model method)
```

## Reranking Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                    INITIAL SEARCH RESULTS                        │
│  Top 1024 candidates from hybrid search                          │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                    CROSS-ENCODER RERANKING                       │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  For each (query, document) pair:                        │   │
│  │    score = CrossEncoder(query, document)                 │   │
│  └─────────────────────────────────────────────────────────┘   │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                    SCORE FUSION                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  final_score = α × token_sim + β × vector_sim + γ × rank │   │
│  │  where α=0.3, β=0.7, γ=variable                         │   │
│  └─────────────────────────────────────────────────────────┘   │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                    TOP-N RESULTS                                 │
│  Return top 6 (default) highest scoring documents               │
└─────────────────────────────────────────────────────────────────┘
```

## Supported Rerank Models

| Provider | Class | Notes |
|----------|-------|-------|
| Jina | `JinaRerank` | multilingual |
| Cohere | `CoHereRerank` | Native SDK |
| NVIDIA | `NvidiaRerank` | Model-specific URLs |
| Voyage AI | `VoyageRerank` | Token counting |
| Qwen | `QWenRerank` | Dashscope |
| BGE | `HuggingfaceRerank` | TEI HTTP |
| LocalAI | `LocalAIRerank` | Custom normalization |
| SILICONFLOW | `SILICONFLOWRerank` | Chunk config |

## Base Implementation

```python
class Base(ABC):
    def similarity(self, query: str, texts: list) -> tuple[np.ndarray, int]:
        """
        Calculate relevance scores for query-document pairs.

        Args:
            query: Search query
            texts: List of document texts

        Returns:
            (scores, token_count): Array of relevance scores and tokens used
        """
        raise NotImplementedError()
```

## Jina Rerank

```python
class JinaRerank(Base):
    def __init__(self, key, model_name, base_url=None):
        self.headers = {
            "Content-Type": "application/json",
            "Authorization": f"Bearer {key}"
        }
        self.base_url = base_url or "https://api.jina.ai/v1/rerank"
        self.model_name = model_name

    def similarity(self, query: str, texts: list):
        texts = [truncate(t, 8196) for t in texts]

        data = {
            "model": self.model_name,
            "query": query,
            "documents": texts,
            "top_n": len(texts)
        }

        res = requests.post(self.base_url, headers=self.headers, json=data).json()
        rank = np.zeros(len(texts), dtype=float)

        for d in res["results"]:
            rank[d["index"]] = d["relevance_score"]

        return rank, total_token_count_from_response(res)
```

## LocalAI Rerank with Normalization

```python
class LocalAIRerank(Base):
    def similarity(self, query: str, texts: list):
        # ... API call ...

        # Normalize scores to [0, 1] range
        min_rank = np.min(rank)
        max_rank = np.max(rank)

        if not np.isclose(min_rank, max_rank, atol=1e-3):
            rank = (rank - min_rank) / (max_rank - min_rank)
        else:
            rank = np.zeros_like(rank)

        return rank, token_count
```

## Rerank Integration in Search

```python
# In search.py - rerank_by_model()

def rerank_by_model(self, rerank_mdl, sres, question,
                    tkweight=0.3, vtweight=0.7, rank_feature=None):
    """
    Rerank search results using cross-encoder model.

    Args:
        rerank_mdl: Reranking model instance
        sres: Search results with content
        question: Original query
        tkweight: Token similarity weight (default 0.3)
        vtweight: Vector similarity weight (default 0.7)
        rank_feature: Optional PageRank scores

    Returns:
        (combined_sim, token_sim, vector_sim): Score arrays
    """

    # Extract content for reranking
    contents = [sres.field[id]["content_with_weight"] for id in sres.ids]

    # Call rerank model
    rank_scores, token_count = rerank_mdl.similarity(question, contents)

    # Get original similarities
    tksim = [sres.field[id].get("term_sim", 0) for id in sres.ids]
    vsim = [sres.field[id].get("vector_sim", 0) for id in sres.ids]

    # Weighted combination
    combined = []
    for i, id in enumerate(sres.ids):
        score = tkweight * tksim[i] + vtweight * vsim[i]

        # Add rank feature (PageRank) if available
        if rank_feature and id in rank_feature:
            score *= (1 + rank_feature[id])

        # Incorporate rerank score
        score = score * 0.5 + rank_scores[i] * 0.5

        combined.append(score)

    return np.array(combined), tksim, vsim
```

## Hybrid Similarity (Without Rerank Model)

```python
def hybrid_similarity(self, avec, bvecs, atks, btkss, tkweight=0.3, vtweight=0.7):
    """
    Calculate hybrid similarity without rerank model.

    Uses:
    - Cosine similarity for vectors
    - Token overlap for text matching
    """
    from sklearn.metrics.pairwise import cosine_similarity

    # Vector similarity
    vsim = cosine_similarity([avec], bvecs)[0]

    # Token similarity
    tksim = self.token_similarity(atks, btkss)

    # Weighted combination
    combined = np.array(vsim) * vtweight + np.array(tksim) * tkweight

    return combined, tksim, vsim

def token_similarity(self, query_tokens, doc_tokens_list):
    """
    Calculate token overlap similarity.

    Formula:
        sim = |query ∩ doc| / |query|
    """
    query_set = set(query_tokens)

    sims = []
    for doc_tokens in doc_tokens_list:
        doc_set = set(doc_tokens)
        overlap = len(query_set & doc_set)
        sim = overlap / len(query_set) if query_set else 0
        sims.append(sim)

    return sims
```

## Final Ranking Formula

```python
# Complete reranking formula
Final_Rank = α × Token_Similarity + β × Vector_Similarity + γ × Rank_Features

# Where:
#   α = 0.3 (token weight, configurable)
#   β = 0.7 (vector weight, configurable)
#   γ = variable (PageRank, tag boost)

# With rerank model:
Final_Score = 0.5 × Hybrid_Score + 0.5 × Rerank_Score
```

## Configuration

```python
RERANK_CFG = {
    "factory": "Jina",
    "api_key": os.getenv("JINA_API_KEY"),
    "base_url": "https://api.jina.ai/v1/rerank",
    "model": "jina-reranker-v2-base-multilingual"
}

# Search configuration
{
    "rerank_model": "jina-reranker-v2",  # Rerank model to use
    "vector_similarity_weight": 0.7,      # β weight
    "top_n": 6,                           # Final results
    "top_k": 1024,                        # Initial candidates
}
```

## Performance Considerations

### Latency
- Reranking adds 200-500ms latency
- Typically processes 50-100 candidates

### Batch Size
- Most models support batch processing
- Trade-off: larger batch = more memory, faster total time

### When to Use Reranking
- High-stakes queries requiring precision
- When initial retrieval quality is insufficient
- Cross-lingual retrieval scenarios

## Related Files

- `/rag/llm/rerank_model.py` - Rerank model implementations
- `/rag/nlp/search.py` - Reranking integration
- `/api/db/services/dialog_service.py` - Rerank model selection
