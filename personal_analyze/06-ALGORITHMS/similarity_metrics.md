# Similarity & Distance Metrics

## Tong Quan

RAGFlow sử dụng multiple similarity metrics cho search, ranking, và entity resolution.

## 1. Cosine Similarity

### File Location
```
/rag/nlp/query.py (line 221)
/rag/raptor.py (line 189)
/rag/nlp/search.py (line 60)
```

### Purpose
Đo độ tương đồng giữa hai vectors (embeddings).

### Formula

```
Cosine Similarity:

cos(θ) = (A · B) / (||A|| × ||B||)

       = Σ(Ai × Bi) / (√Σ(Ai²) × √Σ(Bi²))

Range: [-1, 1]
- cos = 1: Identical direction
- cos = 0: Orthogonal
- cos = -1: Opposite direction

For normalized vectors:
cos(θ) = A · B  (dot product only)
```

### Implementation

```python
from sklearn.metrics.pairwise import cosine_similarity
import numpy as np

def compute_cosine_similarity(vec1, vec2):
    """
    Compute cosine similarity between two vectors.
    """
    # Using sklearn
    sim = cosine_similarity([vec1], [vec2])[0][0]
    return sim

def compute_batch_similarity(query_vec, doc_vecs):
    """
    Compute similarity between query and multiple documents.
    """
    # Returns array of similarities
    sims = cosine_similarity([query_vec], doc_vecs)[0]
    return sims

# Manual implementation
def cosine_sim_manual(a, b):
    dot_product = np.dot(a, b)
    norm_a = np.linalg.norm(a)
    norm_b = np.linalg.norm(b)
    return dot_product / (norm_a * norm_b)
```

### Usage in RAGFlow

```python
# Vector search scoring
def hybrid_similarity(self, query_vec, doc_vecs, tkweight=0.3, vtweight=0.7):
    # Cosine similarity for vectors
    vsim = cosine_similarity([query_vec], doc_vecs)[0]

    # Token similarity
    tksim = self.token_similarity(query_tokens, doc_tokens)

    # Weighted combination
    combined = vsim * vtweight + tksim * tkweight

    return combined
```

---

## 2. Edit Distance (Levenshtein)

### File Location
```
/graphrag/entity_resolution.py (line 28, 246)
```

### Purpose
Measure string similarity cho entity resolution.

### Formula

```
Edit Distance (Levenshtein):

d(a, b) = minimum number of single-character edits
          (insertions, deletions, substitutions)

Dynamic Programming:
d[i][j] = min(
    d[i-1][j] + 1,      # deletion
    d[i][j-1] + 1,      # insertion
    d[i-1][j-1] + c     # substitution (c=0 if same, 1 if different)
)

Base cases:
d[i][0] = i
d[0][j] = j
```

### Implementation

```python
import editdistance

def is_similar_by_edit_distance(a: str, b: str) -> bool:
    """
    Check if two strings are similar using edit distance.

    Threshold: distance ≤ min(len(a), len(b)) // 2
    """
    a, b = a.lower(), b.lower()
    threshold = min(len(a), len(b)) // 2
    distance = editdistance.eval(a, b)
    return distance <= threshold

# Examples:
# "microsoft" vs "microsft" → distance=1, threshold=4 → Similar
# "google" vs "apple" → distance=5, threshold=2 → Not similar
```

### Similarity Threshold

```
Edit Distance Threshold Strategy:

threshold = min(len(a), len(b)) // 2

Rationale:
- Allows ~50% character differences
- Handles typos and minor variations
- Stricter for short strings

Examples:
| String A    | String B    | Distance | Threshold | Similar? |
|-------------|-------------|----------|-----------|----------|
| microsoft   | microsft    | 1        | 4         | Yes      |
| google      | googl       | 1        | 2         | Yes      |
| amazon      | apple       | 5        | 2         | No       |
| ibm         | ibm         | 0        | 1         | Yes      |
```

---

## 3. Chinese Character Similarity

### File Location
```
/graphrag/entity_resolution.py (lines 250-255)
```

### Purpose
Similarity measure cho Chinese entity names.

### Formula

```
Chinese Character Similarity:

sim(a, b) = |set(a) ∩ set(b)| / max(|set(a)|, |set(b)|)

Threshold: sim ≥ 0.8

Example:
a = "北京大学" → set = {北, 京, 大, 学}
b = "北京大" → set = {北, 京, 大}
intersection = {北, 京, 大}
sim = 3 / max(4, 3) = 3/4 = 0.75 < 0.8 → Not similar
```

### Implementation

```python
def is_similar_chinese(a: str, b: str) -> bool:
    """
    Check if two Chinese strings are similar.
    Uses character set intersection.
    """
    a_set = set(a)
    b_set = set(b)

    max_len = max(len(a_set), len(b_set))
    intersection = len(a_set & b_set)

    similarity = intersection / max_len

    return similarity >= 0.8

# Examples:
# "清华大学" vs "清华" → 2/4 = 0.5 → Not similar
# "人工智能" vs "人工智慧" → 3/4 = 0.75 → Not similar
# "机器学习" vs "机器学习研究" → 4/6 = 0.67 → Not similar
```

---

## 4. Token Similarity (Weighted)

### File Location
```
/rag/nlp/query.py (lines 230-242)
```

### Purpose
Measure similarity based on token overlap với weights.

### Formula

```
Token Similarity:

sim(query, doc) = Σ weight(t) for t ∈ (query ∩ doc)
                  ────────────────────────────────────
                  Σ weight(t) for t ∈ query

where weight(t) = TF-IDF weight of token t

Range: [0, 1]
- 0: No token overlap
- 1: All query tokens in document
```

### Implementation

```python
def token_similarity(self, query_tokens_weighted, doc_tokens):
    """
    Compute weighted token similarity.

    Args:
        query_tokens_weighted: [(token, weight), ...]
        doc_tokens: set of document tokens

    Returns:
        Similarity score in [0, 1]
    """
    doc_set = set(doc_tokens)

    matched_weight = 0
    total_weight = 0

    for token, weight in query_tokens_weighted:
        total_weight += weight
        if token in doc_set:
            matched_weight += weight

    if total_weight == 0:
        return 0

    return matched_weight / total_weight

# Example:
# query = [("machine", 0.4), ("learning", 0.35), ("tutorial", 0.25)]
# doc = {"machine", "learning", "introduction"}
# matched = 0.4 + 0.35 = 0.75
# total = 1.0
# similarity = 0.75
```

---

## 5. Hybrid Similarity

### File Location
```
/rag/nlp/query.py (lines 220-228)
```

### Purpose
Combine token và vector similarity.

### Formula

```
Hybrid Similarity:

hybrid = α × token_sim + β × vector_sim

where:
- α = text weight (default: 0.3)
- β = vector weight (default: 0.7)
- α + β = 1.0

Alternative with rank features:
hybrid = (α × token_sim + β × vector_sim) × (1 + γ × pagerank)
```

### Implementation

```python
def hybrid_similarity(self, query_vec, doc_vecs,
                      query_tokens, doc_tokens_list,
                      tkweight=0.3, vtweight=0.7):
    """
    Compute hybrid similarity combining token and vector similarity.
    """
    from sklearn.metrics.pairwise import cosine_similarity

    # Vector similarity (cosine)
    vsim = cosine_similarity([query_vec], doc_vecs)[0]

    # Token similarity
    tksim = []
    for doc_tokens in doc_tokens_list:
        sim = self.token_similarity(query_tokens, doc_tokens)
        tksim.append(sim)

    tksim = np.array(tksim)

    # Handle edge case
    if np.sum(vsim) == 0:
        return tksim, tksim, vsim

    # Weighted combination
    combined = vsim * vtweight + tksim * tkweight

    return combined, tksim, vsim
```

### Weight Recommendations

```
Hybrid Weights by Use Case:
┌─────────────────────────┬────────┬────────┐
│ Use Case                │ Token  │ Vector │
├─────────────────────────┼────────┼────────┤
│ Conversational/Semantic │ 0.05   │ 0.95   │
│ Technical Documentation │ 0.30   │ 0.70   │
│ Legal/Exact Match       │ 0.40   │ 0.60   │
│ Code Search             │ 0.50   │ 0.50   │
│ Default                 │ 0.30   │ 0.70   │
└─────────────────────────┴────────┴────────┘
```

---

## 6. IoU (Intersection over Union)

### File Location
```
/deepdoc/vision/operators.py (lines 702-725)
```

### Purpose
Measure bounding box overlap.

### Formula

```
IoU = Area(A ∩ B) / Area(A ∪ B)

    = Area(intersection) / (Area(A) + Area(B) - Area(intersection))

Range: [0, 1]
- IoU = 0: No overlap
- IoU = 1: Perfect overlap
```

### Implementation

```python
def compute_iou(box1, box2):
    """
    Compute IoU between two boxes [x1, y1, x2, y2].
    """
    # Intersection
    x1 = max(box1[0], box2[0])
    y1 = max(box1[1], box2[1])
    x2 = min(box1[2], box2[2])
    y2 = min(box1[3], box2[3])

    intersection = max(0, x2 - x1) * max(0, y2 - y1)

    # Union
    area1 = (box1[2] - box1[0]) * (box1[3] - box1[1])
    area2 = (box2[2] - box2[0]) * (box2[3] - box2[1])
    union = area1 + area2 - intersection

    return intersection / union if union > 0 else 0
```

---

## 7. N-gram Similarity

### File Location
```
/graphrag/entity_resolution.py (2-gram analysis)
```

### Purpose
Check digit differences trong entity names.

### Implementation

```python
def check_2gram_digit_difference(a: str, b: str) -> bool:
    """
    Check if strings differ only in digit 2-grams.
    """
    def get_2grams(s):
        return [s[i:i+2] for i in range(len(s)-1)]

    a_grams = get_2grams(a)
    b_grams = get_2grams(b)

    # Find different 2-grams
    diff_grams = set(a_grams) ^ set(b_grams)

    # Check if all differences are digit-only
    for gram in diff_grams:
        if not gram.isdigit():
            return False

    return True

# Example:
# "product2023" vs "product2024" → True (only digit diff)
# "productA" vs "productB" → False (letter diff)
```

---

## Summary Table

| Metric | Formula | Range | Use Case |
|--------|---------|-------|----------|
| Cosine | A·B / (‖A‖×‖B‖) | [-1, 1] | Vector search |
| Edit Distance | min edits | [0, ∞) | String matching |
| Chinese Char | \|A∩B\| / max(\|A\|,\|B\|) | [0, 1] | Chinese entities |
| Token | Σw(matched) / Σw(all) | [0, 1] | Keyword matching |
| Hybrid | α×token + β×vector | [0, 1] | Combined search |
| IoU | intersection / union | [0, 1] | Box overlap |

## Related Files

- `/rag/nlp/query.py` - Similarity calculations
- `/rag/nlp/search.py` - Search ranking
- `/graphrag/entity_resolution.py` - Entity matching
- `/deepdoc/vision/operators.py` - Box metrics
