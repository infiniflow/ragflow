# BM25 Scoring Algorithm

## Tong Quan

BM25 (Best Matching 25) là probabilistic ranking function sử dụng trong full-text search.

## Mathematical Formula

```
┌─────────────────────────────────────────────────────────────────┐
│                      BM25 FORMULA                                │
└─────────────────────────────────────────────────────────────────┘

BM25(D, Q) = Σ IDF(qi) × (f(qi, D) × (k1 + 1)) / (f(qi, D) + k1 × (1 - b + b × |D|/avgdl))
             i=1..n

where:
    Q = query terms [q1, q2, ..., qn]
    D = document
    f(qi, D) = frequency of term qi in document D
    |D| = length of document D (in words)
    avgdl = average document length in collection
    k1 = term frequency saturation parameter (default: 1.2)
    b = length normalization parameter (default: 0.75)

IDF(qi) = log((N - n(qi) + 0.5) / (n(qi) + 0.5) + 1)

where:
    N = total number of documents
    n(qi) = number of documents containing qi
```

## Implementation in RAGFlow

```python
# RAGFlow uses Elasticsearch's native BM25 through query_string query

# In /rag/nlp/search.py
def search(self, kb_ids, question, embd_mdl, tenant_ids, ...):
    # Build BM25 query
    matchText, keywords = self.qryr.question(qst, min_match=0.3)

    # matchText contains weighted terms
    # Example: "(machine^0.85 ML AI) (learning^0.72) \"machine learning\"^1.7"

    # Execute search
    res = self.dataStore.search(
        src, highlightFields, filters, matchExprs,
        orderBy, offset, limit, idx_names, kb_ids
    )
```

## Query Construction

```python
# In /rag/nlp/query.py

def question(self, txt, tbl="qa", min_match=0.6):
    """
    Build BM25 query with weighted terms.

    Returns:
        MatchTextExpr with query string and field boosts
    """
    # Normalize and tokenize
    tks = rag_tokenizer.tokenize(txt)

    # Get TF-IDF weights
    tks_w = self.tw.weights(tks)

    # Build query string
    q = []
    for (tk, w) in tks_w[:256]:
        # Add term with weight
        syn = self.syn.lookup(tk)  # Get synonyms
        q.append(f"({tk}^{w:.4f} {syn})")

    # Add phrase queries with 2x boost
    for i in range(1, len(tks_w)):
        left, right = tks_w[i-1][0], tks_w[i][0]
        weight = max(tks_w[i-1][1], tks_w[i][1]) * 2
        q.append(f'"{left} {right}"^{weight:.4f}')

    return MatchTextExpr(
        query=" ".join(q),
        fields=self.query_fields,
        min_match=f"{int(min_match * 100)}%"
    )
```

## Field Boosting

```python
# Fields searched with boost factors
query_fields = [
    "title_tks^10",           # Title: 10x boost
    "title_sm_tks^5",         # Title (semantic): 5x
    "important_kwd^30",       # Keywords: 30x
    "important_tks^20",       # Keyword tokens: 20x
    "question_tks^20",        # Question tokens: 20x
    "content_ltks^2",         # Content: 2x
    "content_sm_ltks",        # Content (semantic): 1x
]

# Final BM25 score = sum of field scores with boosts
```

## Elasticsearch Query

```json
{
    "query": {
        "query_string": {
            "query": "(machine^0.85 ML AI) (learning^0.72) \"machine learning\"^1.7",
            "fields": [
                "title_tks^10",
                "important_kwd^30",
                "content_ltks^2"
            ],
            "minimum_should_match": "30%",
            "default_operator": "OR"
        }
    }
}
```

## Parameter Tuning

### k1 Parameter
```
k1 controls term frequency saturation:
- k1 = 0: Binary occurrence (term present or not)
- k1 = 1.2: Moderate saturation (default)
- k1 = 2.0: Less saturation, higher weight for frequent terms

Higher k1 = more weight to term frequency
```

### b Parameter
```
b controls length normalization:
- b = 0: No length normalization
- b = 0.75: Moderate normalization (default)
- b = 1.0: Full normalization

Higher b = more penalty for long documents
```

## BM25 Score Components

```
┌─────────────────────────────────────────────────────────────────┐
│                    BM25 SCORE BREAKDOWN                          │
└─────────────────────────────────────────────────────────────────┘

For query "machine learning" on document D:

1. IDF Component:
   IDF("machine") = log((10000 - 500 + 0.5) / (500 + 0.5) + 1) = 3.0
   IDF("learning") = log((10000 - 300 + 0.5) / (300 + 0.5) + 1) = 3.5

2. TF Component (k1=1.2):
   f("machine", D) = 3 occurrences
   TF_factor = (3 × 2.2) / (3 + 1.2 × (1 - 0.75 + 0.75 × 100/50))
             = 6.6 / (3 + 1.2 × 1.75) = 6.6 / 5.1 = 1.29

3. Final Score:
   BM25 = IDF("machine") × TF("machine") + IDF("learning") × TF("learning")
        = 3.0 × 1.29 + 3.5 × 1.15
        = 3.87 + 4.03 = 7.90
```

## Minimum Match

```python
# minimum_should_match controls required term coverage

min_match = "30%"
# For query with 10 terms:
# At least 3 terms must match

# This prevents:
# - Very sparse matches
# - High recall but low precision
```

## Related Files

- `/rag/nlp/search.py` - BM25 query construction
- `/rag/nlp/query.py` - Query processing
- `/rag/utils/es_conn.py` - Elasticsearch integration
