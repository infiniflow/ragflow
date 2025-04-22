---
sidebar_position: 7
slug: /enable_raptor
---

# Enable RAPTOR

A recursive abstractive method used in long-context knowledge retrieval and summarization, balancing broad semantic understanding with fine details.

---

RAPTOR (Recursive Abstractive Processing for Tree Organized Retrieval) is an enhanced document preprocessing technique introduced in a [2024 paper](https://arxiv.org/html/2401.18059v1). Designed to tackle multi-hop question-answering issues, RAPTOR performs recursive clustering and summarization of document chunks to build a hierarchical tree structure. This enables more context-aware retrieval across lengthy documents. RAGFlow v0.6.0 integrates RAPTOR for document clustering as part of its data preprocessing pipeline between data extraction and indexing, as illustrated below.



Our tests with this new approach demonstrate state-of-the-art (SOTA) results on question-answering tasks requiring complex, multi-step reasoning. By combining RAPTOR retrieval with our built-in chunking methods and/or other retrieval-augmented generation (RAG) approaches, you can further improve your question-answering accuracy.

:::danger WARNING
Enabling RAPTOR requires significant memory, computational resources, and tokens.
:::

## Basic principles

After the original documents are divided into chunks, the chunks are clustered by semantic similarity rather than by their original order in the text. Clusters are then summarized into higher-level chunks by your system's default chat model. This process is applied recursively, forming a tree structure with various levels of summarization from the bottom up. As illustrated in the figure below, the initial chunks form the leaf nodes (shown in blue) and are progressively summarized into a root node (shown in orange).

The recursive clustering and summarization capture a broad understanding (by the root node) as well as fine details (by the leaf nodes) necessary for multi-hop question-answering.

## Scenarios

For multi-hop question-answering tasks involving complex, multi-step reasoning, a semantic gap often exists between the question and its answer. As a result, searching with the question often fails to retrieve the relevant chunks that contribute to the correct answer. RAPTOR addresses this challenge by providing the chat model with richer and more context-aware and relevant chunks to summarize, enabling a holistic understanding without losing granular details.

## Prerequisites

The system's default chat model is used to summarize clustered content. Before proceeding, ensure that you have a chat model properly configured:

![Image](https://github.com/user-attachments/assets/6bc34279-68c3-4d99-8d20-b7bd1dafc1c1)

## Configurations

On the **Configuration** page of your knowledge base, the **Use RAPTOR to enhance retrieval** toggle is disabled by default.

### Prompt

```
Please summarize the following paragraphs... Paragraphs as following:
      {cluster_content}
The above is the content you need to summarize.
```

### Max token



### Threshold



### Max cluster


### Random seed

