---
sidebar_position: 2
slug: /ai_search
---

# Search

Conduct an AI search.

---

An AI search is a single-turn AI conversation using a predefined retrieval strategy (a hybrid search of weighted keyword similarity and weighted vector similarity) and the system's default chat model. It does not involve advanced RAG strategies like knowledge graph, auto-keyword, or auto-question. Retrieved chunks will be listed below the chat model's response.

![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/ai_search.jpg)

:::tip NOTE
When debugging your chat assistant, you can use AI search as a reference to verify your model settings and retrieval strategy.
:::


:::info NOTE
A chat is a multi-turn AI conversation where you can define your retrieval strategy (a weighted reranking score can be used to replace the weighted vector similarity in a hybrid search) and choose your chat model. In an AI chat, you can configure advanced RAG strategies, such as knowledge graphs, auto-keyword, and auto-question, for your specific case. Retrieved chunks are not displayed along with the answer.
:::

