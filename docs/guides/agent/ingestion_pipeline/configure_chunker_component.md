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

After **Parser**, use the outgoing connection point to choose **Token Chunker** or **Title Chunker**. The current Token Chunker menu offers **Indexer**, **Compiler**, and **Transformer** as downstream components; it does not offer Title Chunker.

## Token Chunker

Use token-based chunking for text without reliable section structure.

| Setting | Description |
| --- | --- |
| Delimiters / One | Choose delimiter-based splitting or keep the content as one chunk. |
| Recommended chunk size | Target chunk length in tokens. Defaults to `512` in Delimiters mode. |
| Overlapped percent (%) | Adds overlapping text between adjacent chunks. Defaults to `0`. |
| Image & table context window | Controls the surrounding context associated with images and tables. Defaults to `0`. |
| Delimiters | Defaults to `\n`, `!`, `?`, `;`, `。`, `；`, `！`, and `？`. Add or remove entries as needed. |
| Child chunk are used for retrieval | Enables child chunking for retrieval. Disabled by default. Configure the additional fields shown when enabled. |

The output is `chunks`, an array of objects that downstream components can process.

## Title Chunker

Choose Title Chunker for documents with useful chapter and section structure, such as manuals and papers. Inspect the resulting chunks to check that headings and their content remain together. Do not assume that every source file has a reliable heading hierarchy.


1. Choose **Hierarchy** to retain the heading path in each chunk, or **Group** to split at a selected heading level and group adjacent sections.
2. Select the heading level and configure the matching rules. Use **Add Rule** for another heading pattern and **Add regular expressions** for additional levels within a rule.
3. Set **Chunk token cap**. It defaults to `512`; the slider supports `128`–`8000`. Content exceeding the cap is split further at sentence boundaries.
4. In Hierarchy mode, adjust these options as needed:

| Setting | Description |
| --- | --- |
| Separate parent-heading content | Keeps content directly below a parent heading in a separate chunk. Disabled by default. |
| Set first chunk as global context | Uses the first section as shared context across the heading hierarchy. Disabled by default. |

Use the output `chunks` in downstream components. Test a representative document and check heading matches and chunk boundaries.
