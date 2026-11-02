---
sidebar_position: 1
title: Understand the Core Ingestion Pipeline Components
sidebar_label: Understand the Core Ingestion Pipeline Components
slug: /understand_core_ingestion_pipeline_components
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Understand the Core Ingestion Pipeline Components

The Ingestion Pipeline is composed of the following core components:

- **Parser component**: Reads and understands your files, such as PDFs, images, emails and other file types, and extracts text and structure.
- **Transformer component**: Enriches chunks with summaries, keywords, questions, tags, or metadata. Multiple functions can be enabled in one node.
- **Chunker component**: Splits long text into smaller sections, or chunks, for retrieval.
- **Compiler component**: Organizes chunks into a knowledge tree, mind map, or wiki using a compilation template group and model.
- **Indexer component**: The final step. Sends the processed data to the document engine, supporting hybrid full-text and vector search.

A typical flow is **Begin → Parser → Token Chunker → Transformer → Indexer**. Transformer is optional. The outgoing connection menu also provides **Compiler** after Token Chunker or Transformer for knowledge compilation.

## Configure the Compiler Component

1. Prepare a template group through **Agent > Create agent > Create from blank > Compilation operator**. See [template configuration](../../knowledge_compilation/basic_information_configuration.md). Then add **Compiler** after Token Chunker, Title Chunker, or Transformer; one Compiler is allowed per canvas.
2. Select an accessible **Compilation template group** and chat model. Choose templates for the result you need, such as a knowledge tree or wiki. **Compilation operator** configurations use the same settings.
3. Connect **Compiler → Indexer** to index the original and generated chunks. Save and [validate the pipeline](./test_run.md) with a sample document.

Example: **Begin → Parser → Token Chunker → Compiler → Indexer**. After parsing, check the generated content and retrieval results. If the template group or model is unavailable after import, select an accessible one.
