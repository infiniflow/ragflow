# Hybrid Score Fusion

## Tong Quan

Hybrid Score Fusion kết hợp BM25 lexical scores với vector semantic scores để đạt kết quả tốt nhất.

## Fusion Formula

```
┌─────────────────────────────────────────────────────────────────┐
│                    HYBRID SCORE FUSION                           │
└─────────────────────────────────────────────────────────────────┘

Final_Score = α × BM25_Score + (1 - α) × Vector_Score

where:
    α = text weight (default: 0.05)
    BM25_Score = normalized BM25 ranking score
    Vector_Score = cosine similarity score

RAGFlow Default Weights:
    BM25 Weight: 5% (0.05)
    Vector Weight: 95% (0.95)
```

## Implementation

### Elasticsearch Script Score

```python
# In /rag/utils/es_conn.py

def build_hybrid_query(query_tokens, query_vector, kb_ids, top):
    """
    Build Elasticsearch hybrid query with script scoring.
    """
    return {
        "query": {
            "script_score": {
                "query": {
                    "bool": {
                        "must": bm25_query,
                        "filter": [
                            {"terms": {"kb_id": kb_ids}}
                        ]
                    }
                },
                "script": {
                    "source": """
                        double vector_score = cosineSimilarity(
                            params.query_vector,
                            'q_1024_vec'
                        ) + 1.0;  // Shift to [0, 2]

                        return 0.05 * _score + 0.95 * vector_score;
                    """,
                    "params": {
                        "query_vector": query_vector.tolist()
                    }
                }
            }
        },
        "size": top
    }
```

### Infinity Fusion

```python
# In /rag/utils/infinity_conn.py

# Infinity has built-in fusion support
fusionExpr = FusionExpr(
    method="weighted_sum",
    topk=topk,
    params={"weights": "0.05,0.95"}  # BM25, Vector
)

# Query execution
res = self.infinity_conn.search(
    match_text_expr=matchText,
    match_dense_expr=matchDense,
    fusion_expr=fusionExpr
)
```

## Score Normalization

```python
# Elasticsearch does NOT normalize scores before fusion
# Manual normalization required

def normalize_scores(scores):
    """Min-max normalization to [0, 1]."""
    min_s = min(scores)
    max_s = max(scores)
    if max_s - min_s < 1e-6:
        return [0.5] * len(scores)
    return [(s - min_s) / (max_s - min_s) for s in scores]

# In search.py - rerank()
def rerank(self, sres, question, tkweight=0.3, vtweight=0.7):
    # Get raw scores
    bm25_scores = [sres.field[id].get("term_sim", 0) for id in sres.ids]
    vector_scores = [sres.field[id].get("vector_sim", 0) for id in sres.ids]

    # Normalize
    bm25_norm = normalize_scores(bm25_scores)
    vector_norm = normalize_scores(vector_scores)

    # Combine
    combined = [
        tkweight * bm25_norm[i] + vtweight * vector_norm[i]
        for i in range(len(sres.ids))
    ]

    return combined, bm25_scores, vector_scores
```

## Weight Recommendations

```
┌─────────────────────────────────────────────────────────────────┐
│                    WEIGHT RECOMMENDATIONS                        │
└─────────────────────────────────────────────────────────────────┘

Use Case                      BM25    Vector   Notes
───────────────────────────────────────────────────────────────────
Default/Conversational        5%      95%      Semantic-first
Technical Documentation       30%     70%      Keywords matter
Legal/Compliance              40%     60%      Exact terms important
Code Search                   50%     50%      Balanced
Product Search                20%     80%      Semantic preferred
Academic Papers               30%     70%      Technical terms
```

## Hybrid Similarity Calculation

```python
# In search.py - hybrid_similarity()

def hybrid_similarity(self, avec, bvecs, atks, btkss, tkweight=0.3, vtweight=0.7):
    """
    Calculate hybrid similarity without rerank model.

    Args:
        avec: Query vector
        bvecs: Document vectors
        atks: Query tokens
        btkss: Document token lists
        tkweight: BM25/token weight
        vtweight: Vector weight
    """
    from sklearn.metrics.pairwise import cosine_similarity

    # Vector similarity
    vsim = cosine_similarity([avec], bvecs)[0]

    # Token similarity (Jaccard-like)
    tksim = self.token_similarity(atks, btkss)

    # Weighted combination
    if np.sum(vsim) == 0:
        return np.array(tksim), tksim, vsim

    combined = np.array(vsim) * vtweight + np.array(tksim) * tkweight

    return combined, tksim, vsim

def token_similarity(self, query_tokens, doc_tokens_list):
    """Token overlap similarity."""
    query_set = set(query_tokens)

    sims = []
    for doc_tokens in doc_tokens_list:
        doc_set = set(doc_tokens)
        overlap = len(query_set & doc_set)
        sim = overlap / len(query_set) if query_set else 0
        sims.append(sim)

    return sims
```

## With Reranking Model

```python
def rerank_by_model(self, rerank_mdl, sres, question,
                    tkweight=0.3, vtweight=0.7, rank_feature=None):
    """
    Rerank using cross-encoder model.

    Final score combines:
    1. Token similarity
    2. Vector similarity
    3. Rerank score
    4. Optional rank features (PageRank)
    """
    # Get rerank scores
    contents = [sres.field[id]["content_with_weight"] for id in sres.ids]
    rank_scores, _ = rerank_mdl.similarity(question, contents)

    # Get original similarities
    tksim = [sres.field[id].get("term_sim", 0) for id in sres.ids]
    vsim = [sres.field[id].get("vector_sim", 0) for id in sres.ids]

    # Combine scores
    combined = []
    for i, id in enumerate(sres.ids):
        # Base hybrid score
        score = tkweight * tksim[i] + vtweight * vsim[i]

        # Add rank features (PageRank)
        if rank_feature and id in rank_feature:
            score *= (1 + rank_feature[id])

        # Incorporate rerank score (50-50 blend)
        score = score * 0.5 + rank_scores[i] * 0.5

        combined.append(score)

    return np.array(combined), tksim, vsim
```

## Fusion Strategies

### Weighted Sum (Default)
```
Score = α × BM25 + (1-α) × Vector
Simple, interpretable, adjustable
```

### Reciprocal Rank Fusion (RRF)
```
RRF_score = Σ 1 / (k + rank_i)

where k = 60 (constant)
Less sensitive to score magnitudes
```

### Convex Combination
```
Score = α × BM25 + (1-α) × Vector
where α ∈ [0, 1]
Same as weighted sum with constraint
```

## Configuration

```python
# Search configuration
{
    "vector_similarity_weight": 0.7,   # β weight
    "similarity_threshold": 0.2,       # Minimum score
    "top_k": 1024,                     # Initial candidates
    "top_n": 6,                        # Final results
}

# Elasticsearch script (in query)
{
    "weights": {
        "bm25": 0.05,
        "vector": 0.95
    }
}
```

## Related Files

- `/rag/nlp/search.py` - Fusion implementation
- `/rag/utils/es_conn.py` - Elasticsearch queries
- `/rag/utils/infinity_conn.py` - Infinity queries
