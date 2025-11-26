# RAPTOR Algorithm

## Tong Quan

RAPTOR (Recursive Abstractive Processing for Tree-Organized Retrieval) xây dựng hierarchical summaries để improve retrieval.

## File Location
```
/rag/raptor.py
```

## Algorithm Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                    RAPTOR ALGORITHM                              │
└─────────────────────────────────────────────────────────────────┘

                    Original Chunks
                    [C1, C2, C3, C4, C5, C6, ...]
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  1. EMBEDDING                                                    │
│     Generate embeddings for all chunks                          │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  2. UMAP DIMENSIONALITY REDUCTION                                │
│     Reduce to ~12 dimensions for clustering                     │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  3. GAUSSIAN MIXTURE MODEL (GMM)                                 │
│     Find optimal clusters using BIC                             │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  4. HIERARCHICAL SUMMARIZATION                                   │
│     LLM summarizes each cluster                                 │
│     Summaries become new "chunks"                               │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  5. RECURSIVE ITERATION                                          │
│     Repeat steps 2-4 on summaries                               │
│     Until single summary or max depth                           │
└─────────────────────────────────────────────────────────────────┘

Result: Multi-level tree of summaries
        Level 0: Original chunks
        Level 1: Cluster summaries
        Level 2: Meta-summaries
        ...
```

## Implementation

```python
class RecursiveAbstractiveProcessing4TreeOrganizedRetrieval:
    def __init__(self, max_cluster, llm_model, embd_model, max_token=512):
        self._max_cluster = max_cluster
        self._llm_model = llm_model
        self._embd_model = embd_model
        self._max_token = max_token

        self._prompt = """
        Summarize the following text, focusing on key information:

        {cluster_content}

        Summary:
        """

    async def build_tree(self, chunks: list[str]) -> list[tuple[str, np.ndarray]]:
        """
        Build RAPTOR tree from chunks.

        Args:
            chunks: List of text chunks

        Returns:
            List of (text, embedding) tuples including summaries
        """
        # Generate initial embeddings
        embeddings = await self._embedding_encode(chunks)

        # Start with original chunks
        all_chunks = [(text, emb) for text, emb in zip(chunks, embeddings)]

        # Recursive summarization
        current_chunks = all_chunks.copy()

        while len(current_chunks) > 1:
            # Cluster and summarize
            summaries = await self._cluster_and_summarize(current_chunks)

            if not summaries:
                break

            # Add summaries to tree
            all_chunks.extend(summaries)

            # Use summaries as input for next level
            current_chunks = summaries

        return all_chunks
```

## UMAP Dimensionality Reduction

```python
def _reduce_dimensions(self, embeddings: np.ndarray) -> np.ndarray:
    """
    Reduce embedding dimensions for clustering.

    Uses UMAP with cosine metric.
    """
    import umap

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

## Optimal Cluster Selection (BIC)

```python
def _get_optimal_clusters(self, embeddings: np.ndarray) -> int:
    """
    Find optimal number of clusters using BIC.

    BIC = -2 × log(L) + k × log(n)

    Lower BIC = better model
    """
    from sklearn.mixture import GaussianMixture

    max_clusters = min(self._max_cluster, len(embeddings))
    n_clusters = np.arange(1, max_clusters)

    bics = []
    for n in n_clusters:
        gm = GaussianMixture(
            n_components=n,
            random_state=42,
            covariance_type='full'
        )
        gm.fit(embeddings)
        bics.append(gm.bic(embeddings))

    # Select cluster count with minimum BIC
    optimal_clusters = n_clusters[np.argmin(bics)]

    return optimal_clusters
```

## Clustering with GMM

```python
def _cluster_chunks(self, chunks: list[tuple], embeddings: np.ndarray) -> list[list[int]]:
    """
    Cluster chunks using Gaussian Mixture Model.

    Returns:
        List of cluster assignments (chunk indices per cluster)
    """
    from sklearn.mixture import GaussianMixture

    # Reduce dimensions
    reduced = self._reduce_dimensions(embeddings)

    # Find optimal clusters
    n_clusters = self._get_optimal_clusters(reduced)

    # Fit GMM
    gm = GaussianMixture(
        n_components=n_clusters,
        random_state=42
    )
    gm.fit(reduced)

    # Get probabilities
    probs = gm.predict_proba(reduced)

    # Assign chunks to clusters (threshold = 0.1)
    clusters = [[] for _ in range(n_clusters)]
    for i, prob in enumerate(probs):
        for j, p in enumerate(prob):
            if p > 0.1:  # Threshold
                clusters[j].append(i)

    return [c for c in clusters if c]  # Remove empty
```

## LLM Summarization

```python
async def _summarize_cluster(self, chunk_indices: list[int],
                             chunks: list[tuple]) -> tuple[str, np.ndarray]:
    """
    Summarize a cluster of chunks using LLM.

    Returns:
        (summary_text, summary_embedding)
    """
    # Combine chunk texts
    texts = [chunks[i][0] for i in chunk_indices]
    cluster_content = "\n\n".join(texts)

    # Truncate if too long
    if num_tokens_from_string(cluster_content) > self._max_token * 4:
        cluster_content = cluster_content[:self._max_token * 4]

    # Generate summary
    prompt = self._prompt.format(cluster_content=cluster_content)

    summary = await self._chat(
        "You're a helpful assistant that summarizes text.",
        [{"role": "user", "content": prompt}],
        {"max_tokens": max(self._max_token, 512)}
    )

    # Embed summary
    embedding = await self._embedding_encode([summary])

    return summary, embedding[0]
```

## Main Loop

```python
async def _cluster_and_summarize(self, chunks: list[tuple]) -> list[tuple]:
    """
    One level of clustering and summarization.
    """
    if len(chunks) <= 2:
        return []

    # Extract embeddings
    embeddings = np.array([c[1] for c in chunks])

    # Cluster
    clusters = self._cluster_chunks(chunks, embeddings)

    if len(clusters) <= 1:
        return []

    # Summarize each cluster
    summaries = []
    for cluster_indices in clusters:
        if len(cluster_indices) < 2:
            continue

        summary, emb = await self._summarize_cluster(cluster_indices, chunks)
        summaries.append((summary, emb))

    return summaries
```

## Tree Structure Output

```python
# Final output structure:
tree = [
    # Level 0: Original chunks
    ("Original chunk 1 content...", embedding_1),
    ("Original chunk 2 content...", embedding_2),
    ...
    # Level 1: Cluster summaries
    ("Summary of chunks 1-3...", summary_emb_1),
    ("Summary of chunks 4-6...", summary_emb_2),
    ...
    # Level 2: Meta-summaries
    ("High-level summary of summaries...", meta_emb_1),
    ...
]

# All entries indexed in vector store
# Search retrieves from any level
```

## Configuration

```python
# RAPTOR configuration
{
    "max_cluster": 50,        # Maximum clusters per level
    "max_token": 512,         # Summary max tokens
    "threshold": 0.1,         # GMM probability threshold
}

# In parser_config:
{
    "raptor": {
        "enabled": True,
        "max_cluster": 30,
        "max_depth": 3
    }
}
```

## Benefits

1. **Multi-level Retrieval**: Search across different abstraction levels
2. **Improved Recall**: Summaries capture themes missed by individual chunks
3. **Scalability**: Reduces search space through hierarchy
4. **Context**: Summaries provide broader context for questions

## Related Files

- `/rag/raptor.py` - RAPTOR implementation
- `/rag/svr/task_executor.py` - RAPTOR task handling
- `/api/db/services/task_service.py` - Task types
