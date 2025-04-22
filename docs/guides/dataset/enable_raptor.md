---
sidebar_position: 7
slug: /enable_raptor
---

# Enable RAPTOR

A recursive abstractive method used in long-context knowledge retrieval and summarization, balancing broad semantic understanding with fine details.

---

RAPTOR (Recursive Abstractive Processing for Tree Organized Retrieval) is an enhanced document retrieval technique introduced in a [2024 paper](https://arxiv.org/html/2401.18059v1). RAPTOR performs recursive clustering and summarization of document chunks to build a hierarchical tree structure. This enables more context-aware retrieval across lengthy documents. RAGFlow v0.6.0 introduces RAPTOR as an additional document clustering step between data extraction and indexing, as illustrated below.



Our tests show that this approach achieves state-of-the-art (SOTA) results on question-answering tasks requiring complex, multi-step reasoning. By combining RAPTOR retrieval with our built-in chunking methods and/or other RAG approaches, you can further improve your question-answering accuracy.

:::danger WARNING
Enabling RAPTOR requires significant memory, computational resources, and tokens.
:::

## Basic principles



## Scenarios

For multi-hop question-answering tasks involving complex, multi-step reasoning, a semantic gap often exists between the question and its answer. As a result, searching using the question often fails to retrieve the relevant chunks that contribute to the correct answer. RAPTOR addresses this challenge by providing the chat model with richer and more relevant and context-aware chunks to summarize, enabling a holistic understanding without losing granular details.

:::tip NOTE
Knowledge graphs are also useful for multi-hop question-answering. See [Construct knowledge graph](./construct_knowledge_graph.md) for details. You can use both methods to enhance accuracy in question-answering, but ensure you understand the memory, computational, and token costs that they entail.
:::

## Prerequisites

The system's default chat model is used for content summarization. Before proceeding, ensure that you have a chat model properly configured:

![Image](https://github.com/user-attachments/assets/6bc34279-68c3-4d99-8d20-b7bd1dafc1c1)

## Configurations

### Prompt

```
Please summarize the following paragraphs... Paragraphs as following:
      {cluster_content}
The above is the content to summarize.
```



### Max token



conflicts with the max_token settings 

### Threshold



### Max cluster


### Random seed

