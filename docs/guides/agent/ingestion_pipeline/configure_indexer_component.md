---
sidebar_position: 6
title: Configure the Indexer Component
sidebar_label: Configure the Indexer Component
slug: /configure_indexer_component
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Configure the Indexer Component

The **Indexer** component indexes data for optimal retrieval. It is the final step, writing processed data into search engines such as Infinity, Elasticsearch and OpenSearch.

Key configurations:

Search method:

- **Full-text**: Keyword search for exact matches, such as code and names.
- **Embedding**: Semantic search using vector similarity.
- **Hybrid** (recommended): Combines both methods for the best recall.

Retrieval strategy:

- **Processed text** (default): Indexes chunked text.
- **Questions**: Indexes generated questions. This usually produces higher similarity matches than text-to-text matching.
- **Enhanced context**: Indexes summaries instead of raw text. Suitable for broad topic matching.

Filename weight:

- A slider for including the document filename as semantic information in retrieval.

Embedding model:

- Automatically uses the model set when creating the knowledge base.

:::caution IMPORTANT
To search across multiple knowledge bases at the same time, all selected knowledge bases must use the same embedding model.
:::

![Configure The Indexer Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/configure_the_indexer_component.jpg)
