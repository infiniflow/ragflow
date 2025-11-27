# Graph Algorithms

## Tong Quan

RAGFlow sử dụng graph algorithms cho knowledge graph construction và GraphRAG retrieval.

## 1. PageRank Algorithm

### File Location
```
/graphrag/entity_resolution.py (line 150)
/graphrag/general/index.py (line 460)
/graphrag/search.py (line 83)
```

### Purpose
Tính importance score cho entities trong knowledge graph.

### Implementation

```python
import networkx as nx

def compute_pagerank(graph):
    """
    Compute PageRank scores for all nodes.
    """
    pagerank = nx.pagerank(graph)
    return pagerank

# Usage in search ranking
def rank_entities(entities, pagerank_scores):
    """
    Rank entities by similarity * pagerank.
    """
    ranked = sorted(
        entities.items(),
        key=lambda x: x[1]["sim"] * x[1]["pagerank"],
        reverse=True
    )
    return ranked
```

### PageRank Formula

```
PageRank Algorithm:

PR(u) = (1-d)/N + d × Σ PR(v)/L(v)
        for all v linking to u

where:
- d = damping factor (typically 0.85)
- N = total number of nodes
- L(v) = number of outbound links from v

Iterative computation until convergence:
PR^(t+1)(u) = (1-d)/N + d × Σ PR^(t)(v)/L(v)
```

### Usage in RAGFlow

```python
# In GraphRAG search
def get_relevant_entities(query, graph):
    # 1. Get candidate entities by similarity
    candidates = vector_search(query)

    # 2. Compute PageRank
    pagerank = nx.pagerank(graph)

    # 3. Combine scores
    for entity in candidates:
        entity["final_score"] = entity["similarity"] * pagerank[entity["id"]]

    return sorted(candidates, key=lambda x: x["final_score"], reverse=True)
```

---

## 2. Leiden Community Detection

### File Location
```
/graphrag/general/leiden.py (lines 72-141)
```

### Purpose
Phát hiện communities trong knowledge graph để tổ chức entities thành groups.

### Implementation

```python
from graspologic.partition import hierarchical_leiden
from graspologic.utils import largest_connected_component

def _compute_leiden_communities(graph, max_cluster_size=12, seed=0xDEADBEEF):
    """
    Compute hierarchical communities using Leiden algorithm.
    """
    # Extract largest connected component
    lcc = largest_connected_component(graph)

    # Run hierarchical Leiden
    community_mapping = hierarchical_leiden(
        lcc,
        max_cluster_size=max_cluster_size,
        random_seed=seed
    )

    # Process results by level
    results = {}
    for level, communities in community_mapping.items():
        for community_id, nodes in communities.items():
            # Calculate community weight
            weight = sum(
                graph.nodes[n].get("rank", 1) *
                graph.nodes[n].get("weight", 1)
                for n in nodes
            )
            results[(level, community_id)] = {
                "nodes": nodes,
                "weight": weight
            }

    return results
```

### Leiden Algorithm

```
Leiden Algorithm (improvement over Louvain):

1. Local Moving Phase:
   - Move nodes between communities to improve modularity
   - Refined node movement to avoid poorly connected communities

2. Refinement Phase:
   - Partition communities into smaller subcommunities
   - Ensures well-connected communities

3. Aggregation Phase:
   - Create aggregate graph with communities as nodes
   - Repeat from step 1 until no improvement

Modularity:
Q = (1/2m) × Σ [Aij - (ki×kj)/(2m)] × δ(ci, cj)

where:
- Aij = edge weight between i and j
- ki = degree of node i
- m = total edge weight
- δ(ci, cj) = 1 if same community, 0 otherwise
```

### Hierarchical Leiden

```
Hierarchical Leiden:
- Recursively applies Leiden to each community
- Creates multi-level community structure
- Controlled by max_cluster_size parameter

Level 0: Root community (all nodes)
Level 1: First-level subcommunities
Level 2: Second-level subcommunities
...
```

---

## 3. Entity Extraction (LLM-based)

### File Location
```
/graphrag/general/extractor.py
/graphrag/light/graph_extractor.py
```

### Purpose
Extract entities và relationships từ text sử dụng LLM.

### Implementation

```python
class GraphExtractor:
    DEFAULT_ENTITY_TYPES = [
        "organization", "person", "geo", "event", "category"
    ]

    async def _process_single_content(self, content, entity_types):
        """
        Extract entities from text using LLM with iterative gleaning.
        """
        # Initial extraction
        prompt = self._build_extraction_prompt(content, entity_types)
        result = await self._llm_chat(prompt)

        entities, relations = self._parse_result(result)

        # Iterative gleaning (up to 2 times)
        for _ in range(2):  # ENTITY_EXTRACTION_MAX_GLEANINGS
            glean_prompt = self._build_glean_prompt(result)
            glean_result = await self._llm_chat(glean_prompt)

            # Check if more entities found
            if "NO" in glean_result.upper():
                break

            new_entities, new_relations = self._parse_result(glean_result)
            entities.extend(new_entities)
            relations.extend(new_relations)

        return entities, relations

    def _parse_result(self, result):
        """
        Parse LLM output into structured entities/relations.

        Format: (entity_type, entity_name, description)
        Format: (source, target, relation_type, description)
        """
        entities = []
        relations = []

        for line in result.split("\n"):
            if line.startswith("(") and line.endswith(")"):
                parts = line[1:-1].split(TUPLE_DELIMITER)
                if len(parts) == 3:  # Entity
                    entities.append({
                        "type": parts[0],
                        "name": parts[1],
                        "description": parts[2]
                    })
                elif len(parts) == 4:  # Relation
                    relations.append({
                        "source": parts[0],
                        "target": parts[1],
                        "type": parts[2],
                        "description": parts[3]
                    })

        return entities, relations
```

### Extraction Pipeline

```
Entity Extraction Pipeline:

1. Initial Extraction
   └── LLM extracts entities using structured prompt

2. Iterative Gleaning (max 2 iterations)
   ├── Ask LLM if more entities exist
   ├── If YES: extract additional entities
   └── If NO: stop gleaning

3. Relation Extraction
   └── Extract relationships between entities

4. Graph Construction
   └── Build NetworkX graph from entities/relations
```

---

## 4. Entity Resolution

### File Location
```
/graphrag/entity_resolution.py
```

### Purpose
Merge duplicate entities trong knowledge graph.

### Implementation

```python
import editdistance
import networkx as nx

class EntityResolution:
    def is_similarity(self, a: str, b: str) -> bool:
        """
        Check if two entity names are similar.
        """
        a, b = a.lower(), b.lower()

        # Chinese: character set intersection
        if self._is_chinese(a):
            a_set, b_set = set(a), set(b)
            max_len = max(len(a_set), len(b_set))
            overlap = len(a_set & b_set)
            return overlap / max_len >= 0.8

        # English: Edit distance
        else:
            threshold = min(len(a), len(b)) // 2
            distance = editdistance.eval(a, b)
            return distance <= threshold

    async def resolve(self, graph):
        """
        Resolve duplicate entities in graph.
        """
        # 1. Find candidate pairs
        nodes = list(graph.nodes())
        candidates = []

        for i, a in enumerate(nodes):
            for b in nodes[i+1:]:
                if self.is_similarity(a, b):
                    candidates.append((a, b))

        # 2. LLM verification (batch)
        confirmed_pairs = []
        for batch in self._batch(candidates, size=100):
            results = await self._llm_verify_batch(batch)
            confirmed_pairs.extend([
                pair for pair, is_same in zip(batch, results)
                if is_same
            ])

        # 3. Merge confirmed pairs
        for a, b in confirmed_pairs:
            self._merge_nodes(graph, a, b)

        # 4. Update PageRank
        pagerank = nx.pagerank(graph)
        for node in graph.nodes():
            graph.nodes[node]["pagerank"] = pagerank[node]

        return graph
```

### Similarity Metrics

```
English Similarity (Edit Distance):
distance(a, b) ≤ min(len(a), len(b)) // 2

Example:
- "microsoft" vs "microsft" → distance=1 ≤ 4 → Similar
- "google" vs "apple" → distance=5 > 2 → Not similar

Chinese Similarity (Character Set):
|a ∩ b| / max(|a|, |b|) ≥ 0.8

Example:
- "北京大学" vs "北京大" → 3/4 = 0.75 → Not similar
- "清华大学" vs "清华" → 2/4 = 0.5 → Not similar
```

---

## 5. Largest Connected Component (LCC)

### File Location
```
/graphrag/general/leiden.py (line 67)
```

### Purpose
Extract largest connected subgraph trước khi community detection.

### Implementation

```python
from graspologic.utils import largest_connected_component

def _stabilize_graph(graph):
    """
    Extract and stabilize the largest connected component.
    """
    # Get LCC
    lcc = largest_connected_component(graph)

    # Sort nodes for reproducibility
    sorted_nodes = sorted(lcc.nodes())
    sorted_graph = lcc.subgraph(sorted_nodes).copy()

    return sorted_graph
```

### LCC Algorithm

```
LCC Algorithm:

1. Find all connected components using BFS/DFS
2. Select component with maximum number of nodes
3. Return subgraph of that component

Complexity: O(V + E)
where V = vertices, E = edges
```

---

## 6. N-hop Path Scoring

### File Location
```
/graphrag/search.py (lines 181-187)
```

### Purpose
Score entities dựa trên path distance trong graph.

### Implementation

```python
def compute_nhop_scores(entity, neighbors, n_hops=2):
    """
    Score entities based on graph distance.
    """
    nhop_scores = {}

    for neighbor in neighbors:
        path = neighbor["path"]
        weights = neighbor["weights"]

        for i in range(len(path) - 1):
            source, target = path[i], path[i + 1]

            # Decay by distance
            score = entity["sim"] / (2 + i)

            if (source, target) in nhop_scores:
                nhop_scores[(source, target)]["sim"] += score
            else:
                nhop_scores[(source, target)] = {"sim": score}

    return nhop_scores
```

### Scoring Formula

```
N-hop Score with Decay:

score(e, path_i) = similarity(e) / (2 + distance_i)

where:
- distance_i = number of hops from source entity
- 2 = base constant to prevent division issues

Total score = Σ score(e, path_i) for all paths
```

---

## Summary

| Algorithm | Purpose | Library |
|-----------|---------|---------|
| PageRank | Entity importance | NetworkX |
| Leiden | Community detection | graspologic |
| Entity Extraction | KG construction | LLM |
| Entity Resolution | Deduplication | editdistance + LLM |
| LCC | Graph preprocessing | graspologic |
| N-hop Scoring | Path-based ranking | Custom |

## Related Files

- `/graphrag/entity_resolution.py` - Entity resolution
- `/graphrag/general/leiden.py` - Community detection
- `/graphrag/general/extractor.py` - Entity extraction
- `/graphrag/search.py` - Graph search
