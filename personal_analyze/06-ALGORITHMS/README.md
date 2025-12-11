# 06-ALGORITHMS - Core Algorithms & Math

## Tong Quan

Module này chứa TẤT CẢ các thuật toán được sử dụng trong RAGFlow, bao gồm 50+ algorithms chia thành 12 categories.

## Algorithm Categories

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         RAGFLOW ALGORITHM MAP                                │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│  1. CLUSTERING                    │  2. DIMENSIONALITY REDUCTION            │
│  ├── K-Means                      │  ├── UMAP                               │
│  ├── Gaussian Mixture Model (GMM) │  └── Node2Vec Embedding                 │
│  └── Silhouette Score             │                                         │
└───────────────────────────────────┴─────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│  3. GRAPH ALGORITHMS              │  4. NLP/TEXT PROCESSING                 │
│  ├── PageRank                     │  ├── Trie-based Tokenization            │
│  ├── Leiden Community Detection   │  ├── Max-Forward/Backward Algorithm     │
│  ├── Entity Extraction (LLM)      │  ├── DFS with Memoization               │
│  ├── Relation Extraction (LLM)    │  ├── TF-IDF Term Weighting              │
│  ├── Entity Resolution            │  ├── Named Entity Recognition (NER)     │
│  └── Largest Connected Component  │  ├── Part-of-Speech Tagging (POS)       │
│                                   │  ├── Synonym Detection (WordNet)        │
│                                   │  └── Query Expansion                    │
└───────────────────────────────────┴─────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│  5. SIMILARITY/DISTANCE           │  6. INFORMATION RETRIEVAL               │
│  ├── Cosine Similarity            │  ├── BM25 Scoring                       │
│  ├── Edit Distance (Levenshtein)  │  ├── Hybrid Score Fusion                │
│  ├── IoU (Intersection over Union)│  ├── Cross-Encoder Reranking            │
│  ├── Token Similarity             │  └── Weighted Sum Fusion                │
│  └── Hybrid Similarity            │                                         │
└───────────────────────────────────┴─────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│  7. CHUNKING/MERGING              │  8. MACHINE LEARNING MODELS             │
│  ├── Naive Merge (Token-based)    │  ├── XGBoost (Text Concatenation)       │
│  ├── Hierarchical Merge           │  ├── ONNX Models (Vision)               │
│  ├── Tree-based Merge             │  └── Reranking Models                   │
│  └── Binary Search Merge          │                                         │
└───────────────────────────────────┴─────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│  9. VISION/IMAGE PROCESSING       │  10. ADVANCED RAG                       │
│  ├── OCR (ONNX)                   │  ├── RAPTOR (Hierarchical Summarization)│
│  ├── Layout Recognition (YOLOv10) │  ├── GraphRAG                           │
│  ├── Table Structure Recognition  │  └── Community Reports                  │
│  └── Non-Maximum Suppression (NMS)│                                         │
└───────────────────────────────────┴─────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│  11. OPTIMIZATION                 │  12. DATA STRUCTURES                    │
│  ├── BIC (Bayesian Info Criterion)│  ├── Trie Tree                          │
│  └── Silhouette Score             │  ├── Hierarchical Tree                  │
│                                   │  └── NetworkX Graph                     │
└───────────────────────────────────┴─────────────────────────────────────────┘
```

## Files Trong Module Nay

| File | Mo Ta |
|------|-------|
| [bm25_scoring.md](./bm25_scoring.md) | BM25 ranking algorithm |
| [hybrid_score_fusion.md](./hybrid_score_fusion.md) | Score combination |
| [raptor_algorithm.md](./raptor_algorithm.md) | Hierarchical summarization |
| [clustering_algorithms.md](./clustering_algorithms.md) | KMeans, GMM, UMAP |
| [graph_algorithms.md](./graph_algorithms.md) | PageRank, Leiden, Entity Resolution |
| [nlp_algorithms.md](./nlp_algorithms.md) | Tokenization, TF-IDF, NER, POS |
| [vision_algorithms.md](./vision_algorithms.md) | OCR, Layout, NMS |
| [similarity_metrics.md](./similarity_metrics.md) | Cosine, Edit Distance, IoU |

## Complete Algorithm Reference

### 1. CLUSTERING ALGORITHMS

| Algorithm | File | Description |
|-----------|------|-------------|
| K-Means | `/deepdoc/parser/pdf_parser.py:36` | Column detection in PDF layout |
| GMM | `/rag/raptor.py:22` | RAPTOR cluster selection |
| Silhouette Score | `/deepdoc/parser/pdf_parser.py:37` | Cluster validation |

### 2. DIMENSIONALITY REDUCTION

| Algorithm | File | Description |
|-----------|------|-------------|
| UMAP | `/rag/raptor.py:21` | Pre-clustering dimension reduction |
| Node2Vec | `/graphrag/general/entity_embedding.py:24` | Graph node embedding |

### 3. GRAPH ALGORITHMS

| Algorithm | File | Description |
|-----------|------|-------------|
| PageRank | `/graphrag/entity_resolution.py:150` | Entity importance scoring |
| Leiden | `/graphrag/general/leiden.py:72` | Hierarchical community detection |
| Entity Extraction | `/graphrag/general/extractor.py` | LLM-based entity extraction |
| Relation Extraction | `/graphrag/general/extractor.py` | LLM-based relation extraction |
| Entity Resolution | `/graphrag/entity_resolution.py` | Entity deduplication |
| LCC | `/graphrag/general/leiden.py:67` | Largest connected component |

### 4. NLP/TEXT PROCESSING

| Algorithm | File | Description |
|-----------|------|-------------|
| Trie Tokenization | `/rag/nlp/rag_tokenizer.py:72` | Chinese word segmentation |
| Max-Forward | `/rag/nlp/rag_tokenizer.py:250` | Forward tokenization |
| Max-Backward | `/rag/nlp/rag_tokenizer.py:273` | Backward tokenization |
| DFS + Memo | `/rag/nlp/rag_tokenizer.py:120` | Disambiguation |
| TF-IDF | `/rag/nlp/term_weight.py:223` | Term weighting |
| NER | `/rag/nlp/term_weight.py:84` | Named entity recognition |
| POS Tagging | `/rag/nlp/term_weight.py:179` | Part-of-speech tagging |
| Synonym | `/rag/nlp/synonym.py:71` | Synonym lookup |
| Query Expansion | `/rag/nlp/query.py:85` | Query rewriting |
| Porter Stemmer | `/rag/nlp/rag_tokenizer.py:27` | English stemming |
| WordNet Lemmatizer | `/rag/nlp/rag_tokenizer.py:27` | Lemmatization |

### 5. SIMILARITY/DISTANCE METRICS

| Algorithm | File | Formula |
|-----------|------|---------|
| Cosine Similarity | `/rag/nlp/query.py:221` | `cos(θ) = A·B / (‖A‖×‖B‖)` |
| Edit Distance | `/graphrag/entity_resolution.py:28` | Levenshtein distance |
| IoU | `/deepdoc/vision/operators.py:702` | `intersection / union` |
| Token Similarity | `/rag/nlp/query.py:230` | Weighted token overlap |
| Hybrid Similarity | `/rag/nlp/query.py:220` | `α×token + β×vector` |

### 6. INFORMATION RETRIEVAL

| Algorithm | File | Formula |
|-----------|------|---------|
| BM25 | `/rag/nlp/search.py` | ES native BM25 |
| Hybrid Fusion | `/rag/nlp/search.py:126` | `0.05×BM25 + 0.95×Vector` |
| Reranking | `/rag/nlp/search.py:330` | Cross-encoder scoring |
| Argsort Ranking | `/rag/nlp/search.py:429` | Score-based sorting |

### 7. CHUNKING/MERGING

| Algorithm | File | Description |
|-----------|------|-------------|
| Naive Merge | `/rag/nlp/__init__.py:582` | Token-based chunking |
| Naive Merge + Images | `/rag/nlp/__init__.py:645` | With image tracking |
| Hierarchical Merge | `/rag/nlp/__init__.py:487` | Tree-based merging |
| Binary Search | `/rag/nlp/__init__.py:512` | Efficient section lookup |
| DFS Tree Traversal | `/rag/flow/hierarchical_merger/` | Document hierarchy |

### 8. MACHINE LEARNING MODELS

| Model | File | Purpose |
|-------|------|---------|
| XGBoost | `/deepdoc/parser/pdf_parser.py:88` | Text concatenation |
| ONNX OCR | `/deepdoc/vision/ocr.py:32` | Text recognition |
| ONNX Layout | `/deepdoc/vision/layout_recognizer.py` | Layout detection |
| ONNX TSR | `/deepdoc/vision/table_structure_recognizer.py` | Table structure |
| YOLOv10 | `/deepdoc/vision/layout_recognizer.py` | Object detection |

### 9. VISION/IMAGE PROCESSING

| Algorithm | File | Description |
|-----------|------|-------------|
| NMS | `/deepdoc/vision/operators.py:702` | Box filtering |
| IoU Filtering | `/deepdoc/vision/recognizer.py:359` | Overlap detection |
| Bounding Box Overlap | `/deepdoc/vision/layout_recognizer.py:94` | Spatial analysis |

### 10. ADVANCED RAG

| Algorithm | File | Description |
|-----------|------|-------------|
| RAPTOR | `/rag/raptor.py:37` | Hierarchical summarization |
| GraphRAG | `/graphrag/` | Knowledge graph RAG |
| Community Reports | `/graphrag/general/community_reports_extractor.py` | Graph summaries |

### 11. OPTIMIZATION CRITERIA

| Algorithm | File | Formula |
|-----------|------|---------|
| BIC | `/rag/raptor.py:92` | `k×log(n) - 2×log(L)` |
| Silhouette | `/deepdoc/parser/pdf_parser.py:400` | `(b-a) / max(a,b)` |

## Statistics

- **Total Algorithms**: 50+
- **Categories**: 12
- **Key Libraries**: sklearn, UMAP, XGBoost, NetworkX, graspologic, ONNX
