---
sidebar_position: 1
title: Understand the Core Ingestion Pipeline Components
sidebar_label: Understand the Core Ingestion Pipeline Components
slug: /understand_core_ingestion_pipeline_components
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Understand the Core Ingestion Pipeline Components

The Ingestion Pipeline is composed of the following core components:

- **Parser component**: Reads and understands your files, such as PDFs, images, emails and other file types, and extracts text and structure.
- **Transformer component**: Enhances text by using AI to add summaries, keywords or questions, improving search.
- **Chunker component**: Splits long text into optimally sized fragments, or chunks, to improve AI retrieval.
- **Indexer component**: The final step. Sends the processed data to the document engine, supporting hybrid full-text and vector search.
