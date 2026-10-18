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

## Table and image context for books, papers and manuals

Title Chunker (both Hierarchy and Group) and Manual Chunker accept independent
**Table context window** (`table_context_size`) and **Image context window**
(`image_context_size`) settings. Both default to `0` (disabled); leaving them
unset or explicitly zero preserves the existing output.
Budgets must be non-negative integers; invalid API values fail component
construction instead of being silently rounded or ignored.

A positive value is a token budget **for each side** of the corresponding media
chunk. Text is collected from neighboring source records in document order,
skipping other tables and images. Manual Chunker uses its physical reading order
after sorting by page, top and left. Collection uses the same whole-sentence
rules as General Chunker: `。!?？；！`, newline and period followed by a space.
The last selected sentence is kept whole even if it exceeds the remaining
budget; a paragraph without a sentence boundary may therefore exceed it too.

Windows can cross headings and pages within the source document, but not
spreadsheet sheet boundaries. Each invocation processes a single source file,
with no context retained between documents. Context is collected before
heading grouping and token-cap splitting, so generated heading copies are not
used as neighbors. Text chunks keep their existing token-cap behavior; a media
chunk remains atomic and its context is not trimmed by `chunk_token_cap`.

Selected text is separated from the table/image body with newlines and becomes
part of that chunk's final text, token count and identifier before image
cropping/upload. Original media payloads, references, positions and chunk order
are preserved. Context text remains in its original text chunk as well.
For a captionless image whose body consists only of context, the identifier also
includes its original image payload/reference identity, so distinct images with
the same neighbors cannot overwrite each other.

For example, component-scoped configuration for a book pipeline can enable
table context while leaving image context off:

```json
{
  "TitleChunker:GrumpyGarlicsBake": {
    "table_context_size": 64,
    "image_context_size": 0
  }
}
```

Use the actual component ID from the selected pipeline. The same parameters
are accepted on the built-in book, paper and manual chunkers; no flat
`parser_config` migration is required.

:::caution IMPORTANT
In the current design, if both token-based and title-based methods are used, connect the **Token Chunker** component first, and then connect the **Title Chunker** component. Connecting the **Title Chunker** directly to the **Parser** may cause formatting errors for emails, images, spreadsheets and text files.
:::
