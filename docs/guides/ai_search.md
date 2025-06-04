---
sidebar_position: 2
slug: /ai_search
---

# Search

Conduct an AI search.

---

An AI search is a single-turn AI conversation using a predefined retrieval strategy (a hybrid search of weighted keyword similarity and weighted vector similarity) and the system's default chat model. It does not involve advanced RAG strategies like knowledge graph, auto-keyword, or auto-question. The related chunks are listed below the chat model's response in descending order based on their similarity scores. 

![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/ai_search.jpg)

:::tip NOTE
When debugging your chat assistant, you can use AI search as a reference to verify your model settings and retrieval strategy.
:::

## Prerequisites

- Ensure that you have configured the system's default models on the **Model providers** page.
- Ensure that the intended knowledge bases are properly configured and the intended documents have finished file parsing.


## Frequently asked questions

### Key difference between an AI search and an AI chat?

A chat is a multi-turn AI conversation where you can define your retrieval strategy (a weighted reranking score can be used to replace the weighted vector similarity in a hybrid search) and choose your chat model. In an AI chat, you can configure advanced RAG strategies, such as knowledge graphs, auto-keyword, and auto-question, for your specific case. Retrieved chunks are not displayed along with the answer.

