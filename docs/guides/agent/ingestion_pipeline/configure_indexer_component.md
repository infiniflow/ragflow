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

**Indexer** prepares chunk data for full-text and embedding search. Connect it after the Chunker or Transformer and use the pipeline with a knowledge base to index documents.

## Search method

The panel displays the configured methods as labels:

- **Embedding**: Vector representations for semantic search.
- **Full-text**: Tokenized text for keyword search.

Using both supports hybrid retrieval. **Hybrid** is not a separate option in this panel.

## Filename embedding weight

Controls the contribution of the document filename to chunk embeddings. Defaults to `0.1`; the UI slider ranges from `0` to `0.5`.

## Field

Select the content to embed:

- **Processed text** (default): Chunk text.
- **Questions**: Questions generated for the chunk.
- **Augmented context**: The generated summary.

For Questions or Augmented context, first enable the corresponding function in Transformer. This setting selects embedding content; it does not replace the configured search methods.

## Embedding model

The model comes from the connected knowledge base, rather than an independent model selector in this panel. Knowledge bases searched together must use the same embedding model.
