---
sidebar_position: 2
title: Create an Ingestion Pipeline
sidebar_label: Create an Ingestion Pipeline
slug: /create_ingestion_pipeline
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Create an Ingestion Pipeline

1. Open **Agent > Create agent > Create from blank**.
2. Select **Ingestion pipeline**, enter a name, and click **Confirm**.
3. Open the created card. The canvas starts with **Begin → Parser**.
4. Configure Parser, then use its outgoing connection point to add **Token Chunker** or **Title Chunker**.
5. From Token Chunker, add **Transformer** if enrichment is needed, then add **Indexer**. A basic flow is **Begin → Parser → Token Chunker → Transformer → Indexer**.
6. Configure each component, review **Checklist**, and click **Save**.

Use each component's outgoing connection point to add the next step. After Transformer, choose Indexer or Compiler.

See [Validate the pipeline](./test_run.md) before processing a collection of documents.
