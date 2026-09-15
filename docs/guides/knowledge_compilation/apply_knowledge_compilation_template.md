---
sidebar_position: 4
title: Apply a Knowledge Compilation Template
sidebar_label: Apply a Knowledge Compilation Template
slug: /knowledge_compilation/apply_knowledge_compilation_template
sidebar_custom_props: {
  categoryIcon: LucideWandSparkles
}
---

# Apply a Knowledge Compilation Template

## Configure the Ingestion Pipeline

After creating a template, you need to reference the template in an Ingestion Pipeline.

Configuration steps:

1. Create or open an Ingestion Pipeline.
2. Add and connect Parser, Chunker, Compiler, and Indexer.
3. Open CompilerOperator and select the target CompilationTemplate from the template list.
4. Check node connections and required parameters, and then save the Pipeline.

![Configure Ingestion Pipeline with CompilerOperator](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/apply-knowledge-compilation-template-configure-ingestion-pipeline.png)

Note: CompilationTemplate only defines "how to compile". A KnowledgeArtifact is generated only after the template is referenced in CompilerOperator and the document actually executes this Pipeline.

## Apply the Pipeline in Dataset

1. Go to the Dataset page and create a Dataset or open an existing Dataset.
2. Upload the documents to process.
3. In the file list, select Configure Ingestion Pipeline.
4. Select the Ingestion Pipeline that contains CompilerOperator.
5. Start parsing and check in the task logs whether Parser, Chunker, Compiler, and Indexer complete in sequence.

![Create a Dataset and select Pipeline](https://raw.githubusercontent.com/infiniflow/ragflow-docs/78dcfd707366b45934720c7abe480897f31ecbe7/images/connect_the_pipeline_to_a_knowledge_base_1.jpg)

![Configure an Ingestion Pipeline in Dataset](https://raw.githubusercontent.com/infiniflow/ragflow-docs/78dcfd707366b45934720c7abe480897f31ecbe7/images/connect_the_pipeline_to_a_knowledge_base_2.jpg)

If CompilationTemplate is modified, completed documents are not automatically recompiled. You need to reparse or rerun the Pipeline according to the operations currently provided by the product before the new configuration can be applied.

## View Knowledge Artifacts

After knowledge compilation is complete, enter the corresponding knowledge base and select **Artifacts** from the left sidebar. In the upper-right corner of the Artifacts page, select the artifact type you want to view from the drop-down list, such as **Wiki**, **To Skills**, **Tree/Page index**, **Graph**, **Mind map**, or **Timeline**, to view the corresponding generated results.

Knowledge artifacts can be divided by generation scope into document-level and knowledge-base-level artifacts:

- **Document-level knowledge artifacts**: Graph, Tree, PageIndex, MindMap, and Timeline can generate corresponding document-level results. Click a file name in the files list to view its artifacts.

- **Knowledge-base-level knowledge artifacts**: Some knowledge artifacts support further generation of knowledge-base-level results based on documents in the knowledge base. After executing a knowledge-base-level generation task, you can view the generated results in Artifacts. Wiki is generated as a knowledge-base-level artifact. After knowledge compilation for related documents is complete, you need to go to the Artifacts page of the knowledge base and click generate. The system then generates Wiki based on the compilation results in the current knowledge base.

![Generate Wiki from Knowledge Artifacts](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/apply-knowledge-compilation-template-view-knowledge-artifacts.png)


When a knowledge-base-level knowledge artifact generation task is executed, the system generates corresponding knowledge-base-level logs. You can use the logs to view the task execution status and related runtime information. When a generation task fails or the result is abnormal, check it together with the log information.

## Convert Knowledge Artifacts to Skills (To Skills)

After viewing a knowledge artifact, you can use the **To Skills** feature to further organize and convert the artifact into reusable **Skills** that can be used by agents.

**Steps:**

- Go to the **Artifacts** page of the knowledge base and open the knowledge artifact you want to convert.
- Select **To Skills** from the drop-down menu in the upper-right corner of the page.
- The system extracts and organizes relevant content from the current knowledge artifact and generates corresponding **Skills**.
- After generation is complete, the generated Skills are displayed in the **Skills** list. Click a Skill to view its details on the right.

The generated Skill typically includes a name, description, and rules, methods, or instructions extracted and organized from the knowledge artifact, which can be used by agents when performing relevant tasks.

> **Note**
>
> **To Skills** does not modify the original knowledge artifact. Instead, it generates reusable Skills based on the existing knowledge artifact.

## Knowledge Artifact Check

After knowledge artifacts are generated, check the generated results based on the template used and confirm whether the content and structure meet expectations.

For different knowledge artifact types, focus on the following checks:

| Type | Check Focus |
| --- | --- |
| Graph | Whether entities are duplicated; whether relationship directions are correct; whether there are unsupported nodes or edges. |
| Tree | Whether the hierarchy is clear; whether peer nodes are at similar abstraction levels; whether summaries are accurate. |
| PageIndex | Whether chapter hierarchy is preserved; whether facts and conclusions come from the corresponding chapters. |
| MindMap | Whether the central topic is clear; whether branches are duplicated or crossed; whether node names are concise. |
| Timeline | Whether time is accurate; whether event order is correct; whether relative time is misinterpreted. |
| Wiki | Whether page topics are reasonable; whether links between pages are valid; whether facts are consistent with sources. |

## Update Knowledge Artifacts

After knowledge artifacts are generated, the system continuously detects document changes in the knowledge base. When documents are added to or removed from the knowledge base, the corresponding knowledge artifacts are not automatically regenerated. Instead, an update prompt is displayed to remind users to synchronize the latest knowledge base content.

When document changes are detected, an **Update** button appears in the upper-left corner of the knowledge artifact page, and the number of documents to update is displayed next to the button.

![Update Knowledge Artifacts prompt](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/apply-knowledge-compilation-template-update-knowledge-artifacts.png)

Hover over the update prompt area to view the specific document changes:

- **New documents**: Documents uploaded to the knowledge base after the knowledge artifact was generated and not yet included in the current knowledge artifact.
- **Removed documents**: Documents deleted from the knowledge base after the knowledge artifact was generated, but whose related content has not yet been synchronized and removed from the current knowledge artifact.
- **Number indicator**: Indicates the current number of documents detected as pending update.

After confirming that synchronization is needed, click **Update**. The system updates the knowledge artifact based on the current document changes in the knowledge base, keeping it consistent with the latest documents in the knowledge base.

Note: Uploading or deleting knowledge base documents alone does not immediately update existing knowledge artifacts. After the update prompt appears, you need to manually click **Update** to complete synchronization.
