---
sidebar_position: 4
slug: /enable_table_of_contents
sidebar_custom_props: {
  categoryIcon: LucideTableOfContents
}
---
# Extract table of contents

Extract page index, namely table of contents, from documents to provide long context RAG and improve retrieval.

---

During indexing, this technique uses LLM to extract and generate chapter information, which is added to each chunk to provide sufficient global context. At the retrieval stage, it first uses the chunks matched by search, then supplements missing chunks based on the page index (table of contents) structure. This addresses issues caused by chunk fragmentation and insufficient context, improving answer quality.

:::danger WARNING
Enabling page index extraction requires significant memory, computational resources, and tokens.
:::

## Prerequisites

The system's default chat model is used to summarize clustered content. Before proceeding, ensure that you have a chat model properly configured:

![Set default models](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/set_default_models.jpg)

## Quickstart

1. Navigate to the **Configuration** page.

2. Enable **Page Index**.

3. To use this technique during retrieval, do either of the following:
   
   - In the **Chat setting** panel of your chat app, switch on the **Page Index** toggle.
   - If you are using an agent, click the **Retrieval** agent component to specify the dataset(s) and switch on the **Page Index** toggle.

## Frequently asked questions

### Will previously parsed files be searched using the directory enhancement feature once I enable `Page Index`?

No. Only files parsed after you enable **Page Index** will be searched using the directory enhancement feature. To apply this feature to files parsed before enabling **Page Index**, you must reparse them.