---
sidebar_position: 7
title: Validate the Pipeline
sidebar_label: Validate the Pipeline
slug: /test_run
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Validate the Pipeline

Publish a pipeline version, then parse a sample document in a connected knowledge base to check the results. The pipeline canvas has no **Run** button; document processing is started from the knowledge base.

1. Click **Save draft**. If **Checklist** appears, resolve its configuration issues. Click **Publish** and confirm to create the version you want to test.
2. [Connect the pipeline to a knowledge base](./connect_the_pipeline_to_a_knowledge_base.md), expand it in **Configuration > Parsing mode**, select a published version within the dropdown, and click **Save**. Use a dedicated test knowledge base to avoid affecting existing documents.
3. Upload a small sample file and start parsing from the knowledge base's document controls.
4. Check the document's parsing status and resulting chunks. Confirm that content, chunk boundaries, and enabled enrichment results meet your needs before processing more files.

Check the functions you actually enabled: PDF reading order and page ranges, OCR or image descriptions, chunk size and heading structure, Transformer enrichment, and Compiler output. Check retrieval after indexing, especially when Indexer uses Questions or Augmented context.

If parsing fails, use the reported error to check the parser, model access, and component settings. After changing the pipeline, save and publish a new version, select that version in the test Dataset, and reparse the sample. A saved draft or a newly published version does not update a Dataset's existing selection or document chunks automatically.
