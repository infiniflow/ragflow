---
sidebar_position: 6
slug: /use_tag_sets
---

# Use tag sets

Use tag sets to tag chunks in your datasets.

---

Retrieval accuracy is the touchstone for a production-ready RAG framework. 

v0.16.0 introduces a new concept called tag set

Please note that this feature is dependent solely on vector similarity.

In a retrieval process involving 

## Scenarios

A tag set

## Create a tag set

XLSX, CSV, TXT


:::tip NOTE
A tag knowledge base is *not* involved in indexing or retrieval
:::

## Tag chunks


## Update your tag knowledge base

## Frequently asked questions

### Can I reference more than one tag set?

Yes, you can.

### Difference between a tag set and a standard knowledge base?

A standard knowledge base is a dataset. It will be searched by RAGFlow's document engine and the retrieved chunks will be fed to the LLM. In contrast, a tag set is used solely to attach tags to chunks within your dataset. It does not directly participate in the retrieval process.


### Difference between 