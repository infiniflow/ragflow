# Clustering Algorithms

## Tong Quan

RAGFlow sử dụng clustering algorithms cho PDF layout analysis và RAPTOR hierarchical summarization.

## 1. K-Means Clustering

### File Location
```
/deepdoc/parser/pdf_parser.py (lines 36, 394, 425, 1047-1055)
```

### Purpose
Phát hiện cột (columns) trong PDF layout bằng cách clustering text boxes theo X-coordinate.

### Implementation

```python
from sklearn.cluster import KMeans

def _assign_column(self):
    """
    Detect columns using KMeans clustering on X coordinates.
    """
    # Get X coordinates of text boxes
    x_coords = np.array([[b["x0"]] for b in self.bxs])

    best_k = 1
    best_score = -1

    # Find optimal number of columns (1-5)
    for k in range(1, min(5, len(self.bxs))):
        if k >= len(self.bxs):
            break

        km = KMeans(n_clusters=k, random_state=42, n_init="auto")
        labels = km.fit_predict(x_coords)

        if k > 1:
            score = silhouette_score(x_coords, labels)
            if score > best_score:
                best_score = score
                best_k = k

    # Assign columns with optimal k
    km = KMeans(n_clusters=best_k, random_state=42, n_init="auto")
    labels = km.fit_predict(x_coords)

    for i, bx in enumerate(self.bxs):
        bx["col_id"] = labels[i]
```

### Algorithm

```
K-Means Algorithm:
1. Initialize k centroids randomly
2. Repeat until convergence:
   a. Assign each point to nearest centroid
   b. Update centroids as mean of assigned points
3. Return cluster assignments

Objective: minimize Σ ||xi - μci||²
where μci is centroid of cluster containing xi
```

### Parameters

| Parameter | Value | Description |
|-----------|-------|-------------|
| n_clusters | 1-5 | Number of columns to detect |
| n_init | "auto" | Initialization runs |
| random_state | 42 | Reproducibility |

---

## 2. Gaussian Mixture Model (GMM)

### File Location
```
/rag/raptor.py (lines 22, 102-106, 195-199)
```

### Purpose
RAPTOR algorithm sử dụng GMM để cluster document chunks trước khi summarization.

### Implementation

```python
from sklearn.mixture import GaussianMixture

def _get_optimal_clusters(self, embeddings: np.ndarray, random_state: int):
    """
    Find optimal number of clusters using BIC criterion.
    """
    max_clusters = min(self._max_cluster, len(embeddings))
    n_clusters = np.arange(1, max_clusters)

    bics = []
    for n in n_clusters:
        gm = GaussianMixture(
            n_components=n,
            random_state=random_state,
            covariance_type='full'
        )
        gm.fit(embeddings)
        bics.append(gm.bic(embeddings))

    # Select cluster count with minimum BIC
    optimal_clusters = n_clusters[np.argmin(bics)]
    return optimal_clusters

def _cluster_chunks(self, chunks, embeddings):
    """
    Cluster chunks using GMM with soft assignments.
    """
    # Reduce dimensions first
    reduced = self._reduce_dimensions(embeddings)

    # Find optimal k
    n_clusters = self._get_optimal_clusters(reduced, random_state=42)

    # Fit GMM
    gm = GaussianMixture(n_components=n_clusters, random_state=42)
    gm.fit(reduced)

    # Get soft assignments (probabilities)
    probs = gm.predict_proba(reduced)

    # Assign chunks to clusters with probability > threshold
    clusters = [[] for _ in range(n_clusters)]
    for i, prob in enumerate(probs):
        for j, p in enumerate(prob):
            if p > 0.1:  # Threshold
                clusters[j].append(i)

    return clusters
```

### GMM Formula

```
GMM Probability Density:
p(x) = Σ πk × N(x | μk, Σk)

where:
- πk = mixture weight for component k
- N(x | μk, Σk) = Gaussian distribution with mean μk and covariance Σk

BIC (Bayesian Information Criterion):
BIC = k × log(n) - 2 × log(L̂)

where:
- k = number of parameters
- n = number of samples
- L̂ = maximum likelihood
```

### Soft Assignment

GMM cho phép soft assignment (một chunk có thể thuộc nhiều clusters):

```
Chunk i belongs to Cluster j if P(j|xi) > threshold (0.1)
```

---

## 3. UMAP (Dimensionality Reduction)

### File Location
```
/rag/raptor.py (lines 21, 186-190)
```

### Purpose
Giảm số chiều của embeddings trước khi clustering để improve cluster quality.

### Implementation

```python
import umap

def _reduce_dimensions(self, embeddings: np.ndarray) -> np.ndarray:
    """
    Reduce embedding dimensions using UMAP.
    """
    n_samples = len(embeddings)

    # Calculate neighbors based on sample size
    n_neighbors = int((n_samples - 1) ** 0.8)

    # Target dimensions
    n_components = min(12, n_samples - 2)

    reducer = umap.UMAP(
        n_neighbors=max(2, n_neighbors),
        n_components=n_components,
        metric="cosine",
        random_state=42
    )

    return reducer.fit_transform(embeddings)
```

### UMAP Algorithm

```
UMAP (Uniform Manifold Approximation and Projection):

1. Build high-dimensional graph:
   - Compute k-nearest neighbors
   - Create weighted edges based on distance

2. Build low-dimensional representation:
   - Initialize randomly
   - Optimize layout using cross-entropy loss
   - Preserve local structure (neighbors stay neighbors)

Key idea: Preserve topological structure, not absolute distances
```

### Parameters

| Parameter | Value | Description |
|-----------|-------|-------------|
| n_neighbors | (n-1)^0.8 | Local neighborhood size |
| n_components | min(12, n-2) | Output dimensions |
| metric | cosine | Distance metric |

---

## 4. Silhouette Score

### File Location
```
/deepdoc/parser/pdf_parser.py (lines 37, 400, 1052)
```

### Purpose
Đánh giá cluster quality để chọn optimal k cho K-Means.

### Formula

```
Silhouette Score:
s(i) = (b(i) - a(i)) / max(a(i), b(i))

where:
- a(i) = average distance to points in same cluster
- b(i) = average distance to points in nearest other cluster

Range: [-1, 1]
- s ≈ 1: Point well-clustered
- s ≈ 0: Point on boundary
- s < 0: Point may be misclassified

Overall score = mean(s(i)) for all points
```

### Usage

```python
from sklearn.metrics import silhouette_score

# Find optimal k
best_k = 1
best_score = -1

for k in range(2, max_clusters):
    km = KMeans(n_clusters=k)
    labels = km.fit_predict(data)

    score = silhouette_score(data, labels)

    if score > best_score:
        best_score = score
        best_k = k
```

---

## 5. Node2Vec (Graph Embedding)

### File Location
```
/graphrag/general/entity_embedding.py (lines 24-44)
```

### Purpose
Generate embeddings cho graph nodes trong knowledge graph.

### Implementation

```python
from graspologic.embed import node2vec_embed

def embed_node2vec(graph, dimensions=1536, num_walks=10,
                   walk_length=40, window_size=2, iterations=3):
    """
    Generate node embeddings using Node2Vec algorithm.
    """
    lcc_tensors, embedding = node2vec_embed(
        graph=graph,
        dimensions=dimensions,
        num_walks=num_walks,
        walk_length=walk_length,
        window_size=window_size,
        iterations=iterations,
        random_seed=86
    )

    return embedding
```

### Node2Vec Algorithm

```
Node2Vec Algorithm:

1. Random Walk Generation:
   - For each node, perform biased random walks
   - Walk strategy controlled by p (return) and q (in-out)

2. Skip-gram Training:
   - Treat walks as sentences
   - Train Word2Vec Skip-gram model
   - Node → Embedding vector

Walk probabilities:
- p: Return parameter (go back to previous node)
- q: In-out parameter (explore vs exploit)

Low p, high q → BFS-like (local structure)
High p, low q → DFS-like (global structure)
```

### Parameters

| Parameter | Value | Description |
|-----------|-------|-------------|
| dimensions | 1536 | Embedding size |
| num_walks | 10 | Walks per node |
| walk_length | 40 | Steps per walk |
| window_size | 2 | Skip-gram window |
| iterations | 3 | Training iterations |

---

## Summary

| Algorithm | Purpose | Library |
|-----------|---------|---------|
| K-Means | PDF column detection | sklearn |
| GMM | RAPTOR clustering | sklearn |
| UMAP | Dimension reduction | umap-learn |
| Silhouette | Cluster validation | sklearn |
| Node2Vec | Graph embedding | graspologic |

## Related Files

- `/deepdoc/parser/pdf_parser.py` - K-Means, Silhouette
- `/rag/raptor.py` - GMM, UMAP
- `/graphrag/general/entity_embedding.py` - Node2Vec
