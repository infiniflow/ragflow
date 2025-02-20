---
sidebar_position: 10
slug: /run_retrieval_test
---

# Run retrieval test

Conduct a retrieval test on your knowledge base to check whether the intended chunks can be retrieved.

---

After your files are uploaded and parsed, it is recommended that you run a retrieval test before proceeding with the chat assistant configuration. Just like fine-tuning a precision instrument, RAGFlow requires careful tuning to deliver optimal question answering performance. Your knowledge base settings, chat assistant configurations, and the specified large and small models can all significantly impact the final results. Running a retrieval test verifies whether the intended chunks can be recovered, allowing you to quickly identify areas for improvement or pinpoint any issue that needs addressing. For instance, when debugging your question answering system, if you know that the correct chunks can be retrieved, you can focus your efforts elsewhere instead of wasting time on retrieval.

During a retrieval test, chunks created from your specified chunk method are retrieved using a hybrid search that combines weighted keyword similarity with either weighted vector cosine similarity or a weighted reranking score, depending on your settings:

- If **Rerank model** is unselected, weighted keyword similarity will be combined with weighted vector cosine similarity.
- If a rerank model is selected, weighted keyword similarity will be combined with weighted vector reranking score.

In contrast, chunks created from [knowledge graph construction](./construct_knowledge_graph.md) are retrieved solely using vector cosine similarity.

## Configurations

### Similarity threshold

This sets the bar for chunk retrieval: chunks with similarities below the threshold will be filtered out. By default, the threshold is set to 0.2.

### Keyword similarity weight

This sets the weight of keyword similarity in the combined similarity score, whether used with vector cosine similarity or a reranking score. By default, it is set to 0.7, making the weight of the other component 0.3 (1 - 0.7).

### Rerank model

- If left empty, RAGFlow will use a combination of weighted keyword similarity and weighted vector cosine similarity.
- If a rerank model is selected, weighted keyword similarity will be combined with weighted vector reranking score.

:::danger IMPORTANT
Using a rerank model will significantly increase the system's response time.
:::

### Use knowledge graph

Whether to include entity descriptions, entity descriptions, and community reports during the retrieval test. This switch is disabled by default.

It will retrieve descriptions of relevant entities, relationships, and community reports, which will enhance inference of multi-hop and complex question.

### Test text

This field is where you put in your testing query

## Prerequisites

- Your files are uploaded and properly parsed before running a retrieval test.

## Procedure

1. 


## Frequently asked questions

### Is an LLM used when I the Use Knowledge Graph switch is enabled?

Yes, your LLM will be involved to analyze your query and extract the related entities and relationship from the knowledge graph. This also explains why additional tokens and time will be consumed.