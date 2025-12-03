# Query Decomposition Retrieval

## Overview

The **Query Decomposition Retrieval** component is an advanced retrieval system that automatically decomposes complex queries into simpler sub-questions, performs concurrent retrieval, and intelligently reranks results using LLM-based scoring combined with vector similarity.

This feature addresses a critical limitation in traditional RAG systems: handling complex, multi-faceted queries that require information from multiple sources or aspects.

## Problem Statement

Current approaches to complex query handling have significant limitations:

### 1. Workflow-based Approach
- **High Complexity**: Users must manually assemble multiple components (LLM, loop, retriever) and design complex data flow logic
- **Redundant Overhead**: Each retrieval round requires independent serialization, deserialization, and network calls
- **Poor User Experience**: Requires deep technical expertise to set up

### 2. Agent-based Approach
- **Slow Performance**: Multiple LLM calls for thinking, tool selection, and execution make it inherently slow
- **Unpredictable Behavior**: Agents can be unstable, potentially leading to excessive retrieval rounds or loops
- **Limited Control**: Difficult to ensure deterministic, consistent behavior

## Solution: Native Query Decomposition

The Query Decomposition Retrieval component integrates powerful query decomposition directly into the retrieval pipeline, offering:

- **Simplified User Experience**: One-click enable with customizable prompts - no workflow engineering required
- **Enhanced Performance**: Tight internal integration eliminates overhead and enables global optimization
- **Better Results**: Global chunk deduplication and reranking across sub-queries
- **Deterministic Behavior**: Explicit control over decomposition and scoring logic

## Key Features

### 1. Automatic Query Decomposition
- Uses LLM to intelligently break down complex queries into 2-3 simpler sub-questions
- Each sub-question focuses on one specific aspect
- Configurable decomposition prompt with high-quality defaults

### 2. Concurrent Retrieval
- Retrieves chunks for all sub-queries in parallel
- Significantly faster than sequential processing
- Configurable concurrency control

### 3. Global Deduplication
- Identifies and removes duplicate chunks across all sub-query results
- Tracks which sub-queries retrieved each chunk
- Preserves the best scoring information for each unique chunk

### 4. LLM-based Relevance Scoring
- Uses LLM to judge each chunk's relevance to the original query
- Provides explainable scores with reasoning
- Scores normalized to 0.0-1.0 range

### 5. Score Fusion
- Combines LLM relevance scores with vector similarity scores
- Configurable fusion weight (e.g., 0.7 * LLM_score + 0.3 * vector_score)
- Balances semantic understanding with vector matching

### 6. Global Ranking
- All unique chunks ranked by fused score
- Returns top-N results from global ranking
- Better coverage and relevance than per-sub-query ranking

## How It Works

### Step 1: Query Decomposition

**Input:** "Compare machine learning and deep learning, and explain their applications in computer vision"

**LLM Decomposition:**
1. "What is machine learning and what are its characteristics?"
2. "What is deep learning and what are its characteristics?"
3. "How are machine learning and deep learning used in computer vision?"

### Step 2: Concurrent Retrieval

For each sub-question, perform vector retrieval:
- Sub-query 1 → Retrieves chunks about ML fundamentals
- Sub-query 2 → Retrieves chunks about DL fundamentals  
- Sub-query 3 → Retrieves chunks about CV applications

All retrievals happen in parallel for maximum performance.

### Step 3: Deduplication

If the same chunk appears in multiple sub-query results:
- Keep only one copy
- Track all sub-queries that retrieved it
- Average the vector similarity scores

### Step 4: LLM Scoring

For each unique chunk:
- Call LLM with reranking prompt
- LLM judges: "How useful is this chunk for the original query?"
- Returns score 1-10 with reasoning

### Step 5: Score Fusion

For each chunk:
```
final_score = fusion_weight * (LLM_score / 10) + (1 - fusion_weight) * vector_score
```

Example with fusion_weight=0.7:
```
LLM_score = 8/10 = 0.8
vector_score = 0.75
final_score = 0.7 * 0.8 + 0.3 * 0.75 = 0.56 + 0.225 = 0.785
```

### Step 6: Global Ranking

- Sort all chunks by final_score (descending)
- Return top-N chunks
- Chunks are globally optimal, not just best per sub-query

## Configuration

### Basic Configuration

```python
# In your agent workflow
retrieval = QueryDecompositionRetrieval()

# Enable query decomposition (default: True)
retrieval.enable_decomposition = True

# Maximum number of sub-queries (default: 3)
retrieval.max_decomposition_count = 3

# Number of final results (default: 8)
retrieval.top_n = 8
```

### Advanced Configuration

```python
# Score fusion weight (default: 0.7)
# Higher values trust LLM scores more, lower values trust vector similarity more
retrieval.score_fusion_weight = 0.7

# Enable concurrent retrieval (default: True)
retrieval.enable_concurrency = True

# Similarity threshold (default: 0.2)
retrieval.similarity_threshold = 0.2

# Vector vs keyword weight (default: 0.3)
retrieval.keywords_similarity_weight = 0.3
```

### Custom Prompts

#### Decomposition Prompt

```python
retrieval.decomposition_prompt = """You are a query decomposition expert.

Break down this query into {max_count} sub-questions:
{original_query}

Output as JSON array: ["sub-question 1", "sub-question 2"]
"""
```

#### Reranking Prompt

```python
retrieval.reranking_prompt = """Rate this chunk's relevance (1-10):

Query: {query}
Chunk: {chunk_text}

Output JSON: {{"score": 8, "reason": "Contains key information"}}
"""
```

## Usage Examples

### Example 1: Simple Comparison Query

```python
query = "Compare Python and JavaScript for web development"

# Decomposition produces:
# 1. "What are Python's strengths and use cases in web development?"
# 2. "What are JavaScript's strengths and use cases in web development?"  
# 3. "What are the key differences between Python and JavaScript for web development?"

# Result: Comprehensive answer covering both languages and their comparison
```

### Example 2: Multi-Aspect Research Query

```python
query = "Explain the causes, key events, and consequences of World War II"

# Decomposition produces:
# 1. "What were the main causes that led to World War II?"
# 2. "What were the most significant events during World War II?"
# 3. "What were the major consequences and aftermath of World War II?"

# Result: Well-structured answer covering all three aspects
```

### Example 3: Technical Deep-Dive Query

```python
query = "How does BERT work and what are its applications in NLP?"

# Decomposition produces:
# 1. "What is BERT and how does its architecture work?"
# 2. "What are the main applications of BERT in natural language processing?"

# Result: Technical explanation plus practical applications
```

## Configuration Examples

### For High Precision (Trust LLM More)

```python
retrieval.score_fusion_weight = 0.9  # 90% LLM, 10% vector
retrieval.similarity_threshold = 0.3  # Higher threshold
retrieval.top_n = 5  # Fewer, more precise results
```

### For High Recall (Trust Vector More)

```python
retrieval.score_fusion_weight = 0.3  # 30% LLM, 70% vector
retrieval.similarity_threshold = 0.1  # Lower threshold
retrieval.top_n = 15  # More results for coverage
```

### For Balanced Performance

```python
retrieval.score_fusion_weight = 0.7  # 70% LLM, 30% vector (default)
retrieval.similarity_threshold = 0.2  # Standard threshold (default)
retrieval.top_n = 8  # Standard result count (default)
```

## Performance Comparison

| Approach | Setup Time | Query Time | Result Quality | Determinism |
|----------|------------|------------|----------------|-------------|
| **Manual Workflow** | High (30+ min) | Medium (2-3s) | Depends on design | High |
| **Agent-based** | Medium (10 min) | High (5-10s) | Variable | Low |
| **Query Decomposition** | **Low (1 min)** | **Low (1-2s)** | **High** | **High** |

### Performance Benefits

1. **Concurrent Execution**: Sub-queries retrieved in parallel
2. **Single Deduplication Pass**: No redundant processing
3. **Batch LLM Scoring**: Efficient use of LLM calls
4. **Internal Optimization**: No serialization/network overhead

## Best Practices

### 1. When to Enable Decomposition

✅ **Good for:**
- Complex, multi-faceted queries
- Comparison questions ("Compare A and B")
- Multi-part questions ("Explain X, Y, and Z")
- Research queries requiring comprehensive coverage

❌ **Not needed for:**
- Simple factual queries ("What is X?")
- Single-concept lookups
- Very specific technical questions

### 2. Tuning Score Fusion Weight

- **Start with default (0.7)** for most use cases
- **Increase to 0.8-0.9** if LLM is very good at judging relevance
- **Decrease to 0.5-0.6** if vector similarity is highly reliable
- **Monitor and adjust** based on user feedback

### 3. Prompt Engineering Tips

**Decomposition Prompt:**
- Be explicit about number of sub-questions
- Emphasize non-redundancy
- Require JSON format for reliable parsing
- Keep it concise

**Reranking Prompt:**
- Use clear scoring scale (1-10 is intuitive)
- Request justification for explainability
- Emphasize direct vs indirect relevance
- Require strict JSON format

### 4. Monitoring and Debugging

The component adds metadata to results for debugging:
```python
{
    "chunk_id": "...",
    "content": "...",
    "llm_relevance_score": 0.8,  # LLM score (0-1)
    "vector_similarity_score": 0.75,  # Vector score (0-1)
    "final_fused_score": 0.785,  # Fused score
    "retrieved_by_sub_queries": ["sub-q-1", "sub-q-2"]  # Which sub-queries found it
}
```

## Troubleshooting

### Issue: Decomposition Not Working

**Symptoms:** Always falling back to direct retrieval

**Solutions:**
1. Check `enable_decomposition` is True
2. Verify LLM is properly configured
3. Review decomposition prompt format
4. Check logs for LLM errors

### Issue: Poor Sub-Question Quality

**Symptoms:** Sub-questions are too similar or off-topic

**Solutions:**
1. Refine decomposition prompt
2. Adjust `max_decomposition_count`
3. Consider lowering temperature in LLM config
4. Try different LLM models

### Issue: Slow Performance

**Symptoms:** Queries taking too long

**Solutions:**
1. Ensure `enable_concurrency` is True
2. Reduce `max_decomposition_count`
3. Lower `top_k` to reduce initial retrieval size
4. Consider faster LLM model for scoring

### Issue: Unexpected Rankings

**Symptoms:** Results don't match expectations

**Solutions:**
1. Review `score_fusion_weight` setting
2. Check `similarity_threshold` isn't too restrictive
3. Examine debugging metadata in results
4. Refine reranking prompt for clarity

## API Reference

### Parameters

#### Core Settings

- **enable_decomposition** (bool, default: True)
  - Master toggle for decomposition feature
  
- **max_decomposition_count** (int, default: 3)
  - Maximum number of sub-queries to generate
  - Range: 1-10
  
- **score_fusion_weight** (float, default: 0.7)
  - Weight for LLM score in final ranking
  - Formula: `final = weight * llm + (1-weight) * vector`
  - Range: 0.0-1.0
  
- **enable_concurrency** (bool, default: True)
  - Whether to retrieve sub-queries in parallel

#### Prompts

- **decomposition_prompt** (str)
  - Template for query decomposition
  - Variables: `{original_query}`, `{max_count}`
  
- **reranking_prompt** (str)
  - Template for chunk relevance scoring
  - Variables: `{query}`, `{chunk_text}`

#### Retrieval Settings

- **top_n** (int, default: 8)
  - Number of final results to return
  
- **top_k** (int, default: 1024)
  - Number of initial candidates per sub-query
  
- **similarity_threshold** (float, default: 0.2)
  - Minimum similarity score to include chunk
  
- **keywords_similarity_weight** (float, default: 0.3)
  - Weight of keyword matching vs vector similarity

### Methods

#### _invoke(**kwargs)
Main execution method.

**Args:**
- query (str): User's input query

**Returns:**
- Sets "formalized_content" and "json" outputs

#### thoughts()
Returns description of processing for debugging.

## Integration Examples

### In Agent Workflow

```python
from agent.tools.query_decomposition_retrieval import QueryDecompositionRetrieval

# Create component
retrieval = QueryDecompositionRetrieval()

# Configure
retrieval.enable_decomposition = True
retrieval.score_fusion_weight = 0.7
retrieval.kb_ids = ["kb1", "kb2"]

# Use in workflow
result = retrieval.invoke(query="Complex question here")
```

### With Custom Configuration

```python
# High-precision research mode
research_retrieval = QueryDecompositionRetrieval()
research_retrieval.score_fusion_weight = 0.9  # Trust LLM more
research_retrieval.max_decomposition_count = 4  # More sub-queries
research_retrieval.top_n = 10  # More results

# Fast response mode  
fast_retrieval = QueryDecompositionRetrieval()
fast_retrieval.max_decomposition_count = 2  # Fewer sub-queries
fast_retrieval.enable_concurrency = True  # Parallel processing
fast_retrieval.top_n = 5  # Fewer results
```

## Future Enhancements

Potential improvements for future versions:

1. **Adaptive Decomposition**: Automatically determine optimal number of sub-queries based on query complexity
2. **Hierarchical Decomposition**: Support multi-level query decomposition for extremely complex queries
3. **Cross-Language Decomposition**: Generate sub-queries in multiple languages
4. **Caching**: Cache decomposition results for similar queries
5. **A/B Testing**: Built-in support for comparing different fusion weights
6. **Batch Processing**: Process multiple queries in parallel
7. **Streaming Results**: Return results as they're scored, not all at once

## Support

For issues or questions:
- GitHub Issues: https://github.com/infiniflow/ragflow/issues
- Documentation: https://ragflow.io/docs
- Community: Join our Discord/Slack

## Contributing

We welcome contributions! Areas where you can help:

- Improving default prompts
- Adding support for more languages
- Performance optimizations
- Additional scoring algorithms
- UI enhancements

See [Contributing Guide](../../docs/contribution/README.md) for details.

## License

Copyright 2024 The InfiniFlow Authors. All Rights Reserved.

Licensed under the Apache License, Version 2.0.

