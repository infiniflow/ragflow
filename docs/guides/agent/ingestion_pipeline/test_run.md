---
sidebar_position: 7
title: Test Run
sidebar_label: Test Run
slug: /test_run
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Validate the Pipeline

The current ingestion pipeline canvas provides **Save**, **Checklist**, and **Management**. It does not have the **Run** button used by conversational Agent workflows.

1. Review **Checklist**, resolve configuration issues, and click **Save**.
2. [Connect the pipeline to a knowledge base](./connect_the_pipeline_to_a_knowledge_base.md). Use a dedicated test knowledge base to avoid affecting existing documents.
3. Upload a small sample file and start parsing from the knowledge base's document controls.
4. Check the document's parsing status and resulting chunks. Confirm that content, chunk boundaries, and enabled enrichment results meet your needs before processing more files.

If parsing fails, check the selected parser, model access, component settings, and reported error. A saved canvas alone does not confirm that the pipeline can successfully process a document.
