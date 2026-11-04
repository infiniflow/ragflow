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

After **Parser**, choose **Token Chunker** or **Title Chunker** through the outgoing connection point. After the Chunker, add **Transformer** or **Compiler** to enrich the chunks, or **Indexer** to index them.

The custom pipeline's connection menu offers **Token Chunker** and **Title Chunker**. Built-in previews can display other names, such as **Manual Chunker**; those names are not additional choices in the custom pipeline's add menu. A canvas supports one chunking component, so the menu no longer offers another after one has been added.

## Token Chunker

Use token-based chunking for text without reliable section structure. Click the node to open its panel, then choose **Delimiters** to configure splitting or **One** to keep the content together. The splitting controls below are shown in Delimiters mode; One hides them.

| Setting | Description |
| --- | --- |
| Delimiters / One | Choose delimiter-based splitting or keep the content as one chunk. |
| Recommended chunk size | Target chunk length in tokens. Defaults to `512`; the control supports `1`–`2048`. |
| Overlapped percent (%) | Adds overlapping text between adjacent chunks. Defaults to `0`; the control supports `0`–`30`. |
| Image & table context window | Number of tokens taken before and after an image or table as context. Defaults to `0`; the slider ranges from `0` to `256`. |
| Delimiters | Defaults to `\n`, `!`, `?`, `;`, `。`, `；`, `！`, and `？`. Each entry is a separate delimiter. Edit or remove entries, or click **Add** for another delimiter; check the **Splits at** preview. |
| Child chunk are used for retrieval | Splits parent chunks into smaller retrieval chunks using **Child delimiters**, which appear after enabling the switch. Disabled by default. For line-based child chunks, enable it and add `\n`. |

![Token Chunker delimiters and child chunk settings](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/pipeline_token_chunker_delimiters.jpg)

The output is `chunks`, an array of objects that downstream components can process.

Click **Save draft** after configuring the node. [Publish and validate](./test_run.md) on a sample to check chunk boundaries, overlap, and child chunks.

## Title Chunker

Use Title Chunker for documents with clear headings, such as manuals and papers. Check that the resulting chunks keep each heading with its content.

1. Choose **Hierarchy** to retain the heading path in each chunk, or **Group** to split at a selected heading level and group adjacent sections.
2. Select the heading level and configure the matching rules. Use **Add Rule** for another heading pattern and **Add regular expressions** for additional levels within a rule.
3. Set **Chunk token cap**. It defaults to `512`; the slider supports `128`–`8000`. Content exceeding the cap is split further at sentence boundaries.
4. In Hierarchy mode, adjust these options as needed:

| Setting | Description |
| --- | --- |
| Separate parent-heading content | Keeps content directly below a parent heading in a separate chunk. Disabled by default. |
| Set first chunk as global context | Uses the first section as shared context across the heading hierarchy. Disabled by default. |

![Title Chunker hierarchy, token cap, and heading rules](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/pipeline_title_chunker_hierarchy_rules.jpg)

For Markdown headings, add `^# ` for level 1 and `^## ` for level 2, then select level 2. In Hierarchy mode, check that each subsection retains its parent heading. Use the output `chunks` in downstream components.

## Table and image context for books, papers and manuals

Title Chunker (both Hierarchy and Group) and Manual Chunker accept independent
**Table context window** (`table_context_size`) and **Image context window**
(`image_context_size`) settings. Both default to `0` (disabled); leaving them
unset or explicitly zero preserves the existing output.
Budgets must be non-negative integers; invalid API values fail component
construction instead of being silently rounded or ignored.

A positive value is a token budget **for each side** of the corresponding media
chunk. Text is collected from neighboring source records in document order,
skipping other tables and images. Manual Chunker preserves parser-supplied reading
order when marked; other records are sorted by page, top and left. Collection uses the same whole-sentence
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
