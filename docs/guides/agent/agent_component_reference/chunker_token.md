---
sidebar_position: 32
slug: /chunker_token_component
sidebar_custom_props: {
  categoryIcon: LucideBlocks
}
---
# Token chunker component

A component that splits texts into chunks, respecting a maximum token limit and using delimiters to find optimal breakpoints.

---

A **Token chunker** component is a text splitter that creates chunks by respecting a recommended maximum token length, using delimiters to ensure logical chunk breakpoints. It splits long texts into appropriately-sized, semantically related chunks.


## Scenario

A **Token chunker** component is optional, usually placed immediately after **Parser** or **Title chunker**.

## Configurations

### Recommended chunk size

The recommended maximum token limit for each created chunk. The **Token chunker** component creates chunks at specified delimiters. If this token limit is reached before a delimiter, a chunk is created at that point.

### Overlapped percent (%)

This defines the overlap percentage between chunks. An appropriate degree of overlap ensures semantic coherence without creating excessive, redundant tokens for the LLM.

- Default: 0
- Maximum: 30%


### Delimiters

Defaults to `\n`. Click the right-hand **Recycle bin** button to remove it, or click **+ Add** to add a delimiter.


### Output

The global variable name for the output of the **Token chunker** component, which can be referenced by subsequent components in the ingestion pipeline.

- Default: `chunks`
- Type: `Array<Object>`