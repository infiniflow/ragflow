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

After **Parser**, choose **Token Chunker** or **Title Chunker** through the outgoing connection point. After Token Chunker, add **Transformer** or **Compiler** to enrich the chunks, or **Indexer** to index them.

## Token Chunker

Use token-based chunking for text without reliable section structure.

| Setting | Description |
| --- | --- |
| Delimiters / One | Choose delimiter-based splitting or keep the content as one chunk. |
| Recommended chunk size | Target chunk length in tokens. Defaults to `512` in Delimiters mode. |
| Overlapped percent (%) | Adds overlapping text between adjacent chunks. Defaults to `0`. |
| Image & table context window | Number of tokens taken before and after an image or table as context. Defaults to `0`; the slider ranges from `0` to `256`. |
| Delimiters | Defaults to `\n`, `!`, `?`, `;`, `。`, `；`, `！`, and `？`. Add or remove entries as needed. |
| Child chunk are used for retrieval | Splits parent chunks into smaller retrieval chunks using **Child delimiters**. Disabled by default. For line-based child chunks, enable it and add `\n`. |

The output is `chunks`, an array of objects that downstream components can process.

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

For Markdown headings, add `^# ` for level 1 and `^## ` for level 2, then select level 2. In Hierarchy mode, check that each subsection retains its parent heading. Use the output `chunks` in downstream components.
