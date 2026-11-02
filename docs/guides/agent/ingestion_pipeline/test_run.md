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

Save the pipeline, then parse a sample document in a connected knowledge base to check the results.

1. Review **Checklist**, resolve configuration issues, and click **Save**.
2. [Connect the pipeline to a knowledge base](./connect_the_pipeline_to_a_knowledge_base.md). Use a dedicated test knowledge base to avoid affecting existing documents.
3. Upload a small sample file and start parsing from the knowledge base's document controls.
4. Check the document's parsing status and resulting chunks. Confirm that content, chunk boundaries, and enabled enrichment results meet your needs before processing more files.

If parsing fails, use the reported error to check the parser, model access, and component settings.
