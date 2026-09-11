---
sidebar_position: 3
title: FAQ
sidebar_label: FAQ
slug: /search_faq
sidebar_custom_props:
  categoryIcon: LucideSearch
---

# FAQ

**What are the main differences between Search and Chat?**

Search is designed for knowledge retrieval. It retrieves and displays relevant content from selected knowledge bases based on the user's query, helping users quickly locate the information they need.

Chat is designed for knowledge-based question answering. In addition to retrieving relevant knowledge, it uses an LLM to understand questions and generate answers. It also supports multi-turn conversations and capabilities such as Agentic Retrieval, making it suitable for scenarios that require information synthesis, reasoning, and natural-language responses.

**Why can't I find content that already exists in the knowledge base?**

The relevant chunks may not meet the **Similarity threshold**, or the query may differ significantly from the wording in the source content. Try lowering the similarity threshold, adjusting the **Vector similarity weight**, or using more specific keywords.

Also make sure that the target knowledge base has been added to the current Search and that the relevant documents have been successfully parsed and indexed.

**Why do my search results contain a lot of irrelevant content?**

Try increasing the **Similarity threshold** to filter out less relevant results.

If your query relies mainly on keywords, proper nouns, or identifiers, you can also decrease the **Vector similarity weight** to give full-text retrieval more weight in hybrid retrieval.

**How should I set the Vector similarity weight?**

This parameter controls the balance between vector retrieval and full-text retrieval.

For semantic search scenarios, you can increase the Vector weight. If you mainly search for product names, identifiers, technical terms, or other content that requires exact matching, you can increase the Full-text weight. In most cases, it is recommended to start with the default value and adjust it based on actual search results.

**Why does Search become slower after I enable the Rerank model?**

When the **Rerank model** is enabled, the system uses the rerank model to further evaluate the relevance of retrieved candidates and reorder them. This introduces additional model calls and processing time.

If result ranking quality is more important, you can enable Rerank. If response speed is more important, you can disable it or reduce **Rerank candidates**.

**Is a higher Similarity threshold always better?**

No. A higher threshold can reduce less relevant results, but it may also filter out useful content, resulting in fewer or even no search results.

Adjust the threshold according to your knowledge base and actual search results to achieve a balance between retrieval coverage and result relevance.
