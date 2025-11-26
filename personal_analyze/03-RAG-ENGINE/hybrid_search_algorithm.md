# Hybrid Search Algorithm

## Tổng Quan

Hybrid Search kết hợp **Vector Search** (semantic) với **BM25** (lexical) để đạt được kết quả tốt nhất.

## Algorithm Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                     HYBRID SEARCH ALGORITHM                              │
└─────────────────────────────────────────────────────────────────────────┘

                         User Query
                              │
                              ▼
                    ┌─────────────────┐
                    │ Query Processing │
                    │                 │
                    │ 1. Tokenize     │
                    │ 2. TF-IDF       │
                    │ 3. Synonyms     │
                    └────────┬────────┘
                             │
              ┌──────────────┴──────────────┐
              │                             │
              ▼                             ▼
    ┌─────────────────────┐      ┌─────────────────────┐
    │   VECTOR SEARCH     │      │    BM25 SEARCH      │
    │                     │      │                     │
    │ 1. Embed query      │      │ 1. Build query      │
    │ 2. ANN search       │      │ 2. Full-text match  │
    │ 3. Cosine score     │      │ 3. BM25 score       │
    │                     │      │                     │
    │ Weight: 95%         │      │ Weight: 5%          │
    └──────────┬──────────┘      └──────────┬──────────┘
               │                            │
               └────────────┬───────────────┘
                            │
                            ▼
                  ┌─────────────────┐
                  │  SCORE FUSION   │
                  │                 │
                  │ final_score =   │
                  │ 0.05 * bm25 +   │
                  │ 0.95 * vector   │
                  └────────┬────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │  THRESHOLD      │
                  │  FILTERING      │
                  │                 │
                  │ score > 0.2     │
                  └────────┬────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │  TOP-K RESULTS  │
                  │                 │
                  │  Return top 6   │
                  │  chunks         │
                  └─────────────────┘
```

## Code Implementation

### Main Search Function

```python
# /rag/nlp/search.py

class Dealer:
    """Main search handler."""

    def search(self, kb_ids, question, embd_mdl, tenant_ids,
               highlight=True, top=1024, **kwargs):
        """
        Execute hybrid search.

        Args:
            kb_ids: Knowledge base IDs
            question: User query
            embd_mdl: Embedding model
            tenant_ids: Tenant IDs for access control
            highlight: Enable highlighting
            top: Initial candidate count

        Returns:
            Search results with scores
        """

        # 1. Process query
        query_tokens = self.qryr.question(question)

        # 2. Generate query embedding
        query_vector = embd_mdl.encode([question])[0]

        # 3. Build Elasticsearch query
        es_query = self._build_hybrid_query(
            query_tokens,
            query_vector,
            kb_ids,
            tenant_ids,
            top
        )

        # 4. Execute search
        results = self.es.search(
            index=",".join([f"ragflow_{kb_id}" for kb_id in kb_ids]),
            body=es_query
        )

        # 5. Process results
        return self._process_results(results, highlight)
```

### Query Building

```python
def _build_hybrid_query(self, query_tokens, query_vector, kb_ids, tenant_ids, top):
    """
    Build Elasticsearch hybrid query.

    Combines:
    - script_score for vector similarity
    - bool query for BM25
    """

    # Build BM25 query from tokens
    bm25_query = self._build_bm25_query(query_tokens)

    return {
        "query": {
            "script_score": {
                "query": {
                    "bool": {
                        "must": bm25_query,
                        "filter": [
                            {"terms": {"kb_id": kb_ids}},
                            {"terms": {"tenant_id": tenant_ids}}
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
        "size": top,
        "_source": ["content_with_weight", "docnm_kwd", "kb_id", "..."],
        "highlight": {
            "fields": {
                "content_with_weight": {
                    "fragment_size": 120,
                    "number_of_fragments": 5
                }
            }
        }
    }
```

### BM25 Query Construction

```python
def _build_bm25_query(self, query_tokens):
    """
    Build BM25 query with weighted terms.

    Args:
        query_tokens: Dict of token -> weight from qryr.question()

    Returns:
        Elasticsearch bool query
    """

    should_clauses = []

    for token, weight in query_tokens.items():
        # Single term query with boost
        should_clauses.append({
            "match": {
                "content_with_weight": {
                    "query": token,
                    "boost": weight
                }
            }
        })

        # Phrase query for compound terms (higher boost)
        if len(token.split()) > 1:
            should_clauses.append({
                "match_phrase": {
                    "content_with_weight": {
                        "query": token,
                        "boost": weight * 1.5,
                        "slop": 2
                    }
                }
            })

    return {
        "bool": {
            "should": should_clauses,
            "minimum_should_match": "30%"
        }
    }
```

## Query Processing

### TF-IDF Weighting

```python
# /rag/nlp/query.py

class RagQuery:
    """Query processor with TF-IDF weighting."""

    def question(self, query: str) -> dict:
        """
        Process query and return weighted tokens.

        Returns:
            Dict[token, weight] where weight is TF-IDF normalized
        """

        # 1. Tokenize query
        tokens = self.tokenize(query)

        # 2. Calculate term frequencies
        tf = Counter(tokens)

        # 3. Apply TF-IDF weights
        weighted = {}
        for token, freq in tf.items():
            # Log-normalized TF
            tf_score = 1 + math.log(freq)

            # IDF from corpus (pre-computed)
            idf_score = self.idf.get(token, 1.0)

            # Combined weight
            weighted[token] = round(tf_score * idf_score, 2)

        # 4. Normalize weights to [0, 1]
        max_weight = max(weighted.values()) if weighted else 1
        return {k: v / max_weight for k, v in weighted.items()}
```

### Synonym Expansion

```python
def expand_synonyms(self, tokens: dict) -> dict:
    """
    Expand query with synonyms.

    Example:
        "machine learning" → ["machine learning", "ML", "AI"]
    """

    expanded = dict(tokens)

    for token in list(tokens.keys()):
        synonyms = self.synonym_dict.get(token, [])
        for syn in synonyms:
            # Add synonym with reduced weight
            expanded[syn] = tokens[token] * 0.8

    return expanded
```

## Score Fusion Formula

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        SCORE FUSION FORMULA                              │
└─────────────────────────────────────────────────────────────────────────┘

Given:
    - BM25_Score: Full-text relevance score
    - Vector_Score: Cosine similarity (shifted to [0, 2])
    - α: vector_similarity_weight (default: 0.3 in config, 0.95 in ES)

Final Score Calculation:

    In Elasticsearch script:
    ┌───────────────────────────────────────────────────────────────┐
    │  final_score = 0.05 × _score + 0.95 × (cosine + 1.0)         │
    │                                                               │
    │  where:                                                       │
    │    _score = BM25 score from bool query                       │
    │    cosine = cosineSimilarity(query_vec, doc_vec) ∈ [-1, 1]   │
    │    cosine + 1.0 shifts to [0, 2] for positive scores         │
    └───────────────────────────────────────────────────────────────┘

Why 95:5 ratio?
    - Semantic understanding is prioritized
    - BM25 helps with exact keyword matches
    - Prevents semantic drift while maintaining precision

Alternative weights (configurable):
    ┌───────────────────────────────────────────────────────────────┐
    │  70:30 - More emphasis on keywords (technical docs)          │
    │  80:20 - Balanced (general use)                              │
    │  95:5  - Semantic-first (conversational queries)             │
    └───────────────────────────────────────────────────────────────┘
```

## Threshold Filtering

```python
def filter_by_threshold(results, threshold=0.2):
    """
    Filter results by similarity threshold.

    Args:
        results: Search results with scores
        threshold: Minimum score (default: 0.2)

    Returns:
        Filtered results above threshold
    """

    # Normalize scores to [0, 1]
    max_score = max(r["_score"] for r in results) if results else 1
    normalized = [
        {**r, "similarity": r["_score"] / max_score}
        for r in results
    ]

    # Filter by threshold
    return [r for r in normalized if r["similarity"] >= threshold]
```

## Result Processing

```python
def _process_results(self, es_response, highlight=True):
    """
    Process Elasticsearch response into structured results.

    Returns:
        {
            "chunks": [
                {
                    "chunk_id": "...",
                    "content": "...",
                    "similarity": 0.85,
                    "vector_similarity": 0.90,
                    "term_similarity": 0.60,
                    "docnm_kwd": "Document Name",
                    "positions": [[x0, x1, top, bottom]],
                    "highlight": "<em>matched</em> text..."
                }
            ],
            "doc_aggs": [
                {"doc_id": "...", "doc_name": "...", "count": 3}
            ],
            "total": 100
        }
    """

    chunks = []
    doc_counts = Counter()

    for hit in es_response["hits"]["hits"]:
        source = hit["_source"]

        chunk = {
            "chunk_id": hit["_id"],
            "content": source["content_with_weight"],
            "similarity": hit["_score"] / max_score,
            "docnm_kwd": source.get("docnm_kwd", ""),
            "kb_id": source["kb_id"],
            "doc_id": source["doc_id"],
            "positions": self._parse_positions(source.get("position_int", []))
        }

        # Add highlight if available
        if highlight and "highlight" in hit:
            chunk["highlight"] = hit["highlight"]["content_with_weight"][0]

        chunks.append(chunk)
        doc_counts[source["doc_id"]] += 1

    # Build document aggregations
    doc_aggs = [
        {"doc_id": doc_id, "count": count}
        for doc_id, count in doc_counts.most_common()
    ]

    return {
        "chunks": chunks,
        "doc_aggs": doc_aggs,
        "total": es_response["hits"]["total"]["value"]
    }
```

## Elasticsearch Index Mapping

```json
{
    "mappings": {
        "properties": {
            "content_with_weight": {
                "type": "text",
                "analyzer": "ik_max_word",
                "search_analyzer": "ik_smart"
            },
            "q_1024_vec": {
                "type": "dense_vector",
                "dims": 1024,
                "index": true,
                "similarity": "cosine"
            },
            "docnm_kwd": {
                "type": "keyword"
            },
            "kb_id": {
                "type": "keyword"
            },
            "doc_id": {
                "type": "keyword"
            },
            "position_int": {
                "type": "integer"
            }
        }
    },
    "settings": {
        "number_of_shards": 1,
        "number_of_replicas": 0,
        "analysis": {
            "analyzer": {
                "ik_smart": {"type": "ik_smart"},
                "ik_max_word": {"type": "ik_max_word"}
            }
        }
    }
}
```

## Performance Optimization

### 1. Pre-filtering

```python
# Filter by tenant/KB before scoring
"filter": [
    {"terms": {"kb_id": kb_ids}},
    {"terms": {"tenant_id": tenant_ids}}
]
```

### 2. Top-K Limiting

```python
# Get more candidates, then rerank
initial_candidates = 1024  # top
final_results = 6          # top_n after reranking
```

### 3. Approximate Nearest Neighbor (ANN)

```python
# Elasticsearch uses HNSW for fast vector search
"index": true,
"similarity": "cosine"
```

### 4. Result Caching

```python
@cache_result(ttl=300)
def search(self, kb_ids, question, ...):
    # Cache results for 5 minutes
    pass
```

## Configuration

```python
# Default search configuration
SEARCH_CONFIG = {
    "vector_weight": 0.95,        # Vector score weight
    "bm25_weight": 0.05,          # BM25 score weight
    "similarity_threshold": 0.2,   # Minimum similarity
    "top_k": 1024,                # Initial candidates
    "top_n": 6,                   # Final results
    "minimum_should_match": "30%"  # BM25 match requirement
}
```

## Related Files

- `/rag/nlp/search.py` - Main search implementation
- `/rag/nlp/query.py` - Query processing
- `/rag/utils/es_conn.py` - Elasticsearch connection
