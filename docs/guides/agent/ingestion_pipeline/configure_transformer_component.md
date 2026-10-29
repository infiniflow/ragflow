---
sidebar_position: 5
title: Configure the Transformer Component
sidebar_label: Configure the Transformer Component
slug: /configure_transformer_component
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Configure the Transformer Component

**Transformer** uses a chat model to generate summaries, keywords, questions, tags, or metadata for document chunks. These additions provide more ways to find relevant content during retrieval.

Enable one or more of the following options:

- **Summary**: Creates a concise overview.
- **Auto keywords**: Extracts key terms.
- **Auto-question**: Generates questions that each text chunk can answer.
- **Tags**: Assigns relevant tags from the selected **Tag file**. Set the maximum number of tags to extract.
- **Metadata**: Custom metadata extraction.

![Select Transformer generation type](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/transformer2.png)

A single **Transformer** can enable multiple options; a separate component is not required for each function.

Key configurations:

**Model**: Select a configured chat model.

Prompt engineering:

- Customize **System prompt** where the selected option provides it.

Connection and input:

- Connect **Transformer** to an upstream node that provides document chunks. It enriches those chunks with the enabled extraction results.
- After **Transformer**, add **Indexer** for indexing or **Compiler** for knowledge compilation. Generated summaries are stored alongside the original chunk text.
