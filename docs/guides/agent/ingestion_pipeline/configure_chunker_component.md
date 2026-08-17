---
sidebar_position: 4
title: Configure the Chunker Component
sidebar_label: Configure the Chunker Component
slug: /configure_chunker_component
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Configure the Chunker Component

The Chunker component intelligently splits text. Its goal is to prevent overflow of the AI context window and improve semantic accuracy in hybrid search.

There are two core methods, which can be used sequentially:

Token-based chunking (default):

- **Chunk size**: Defaults to 512 tokens, balancing retrieval quality and model compatibility.
- **Overlap**: Set the overlap percentage to copy the end of one chunk to the beginning of the next chunk, improving semantic continuity.
- **Delimiter**: Uses `\n` (line break) by default to split first at natural paragraph boundaries and avoid cutting in the middle of sentences.

Title-based chunking (hierarchical):

- Best suited for structured documents such as manuals, papers and legal contracts.
- The system splits documents by chapter and section structure.
- Each chunk represents a complete structural unit.

:::caution IMPORTANT
In the current design, if both token-based and title-based methods are used, connect the **Token Chunker** component first, and then connect the **Title Chunker** component. Connecting the **Title Chunker** directly to the **Parser** may cause formatting errors for emails, images, spreadsheets and text files.
:::
