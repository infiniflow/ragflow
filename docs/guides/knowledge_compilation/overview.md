---
sidebar_position: 1
title: Overview
sidebar_label: Overview
slug: /knowledge_compilation/overview
sidebar_custom_props: {
  categoryIcon: LucideWandSparkles
}
---

# Overview

Knowledge compilation converts unstructured documents into structured knowledge content. The system analyzes information in documents with a large language model and generates different types of knowledge artifacts based on the compilation template selected by the user.

Generated knowledge artifacts can be used for knowledge retrieval, intelligent Q&A, and Agent applications, helping users quickly understand and use key information in documents. The following knowledge artifact types are currently supported:

- **Knowledge graph**: Displays entities in documents and their relationships. It is suitable for content such as personal relationships, organizational structures, and product relationships.
- **Knowledge tree**: Organizes document content by hierarchy. It is suitable for chapter structures, topic classification, and knowledge system organization.
- **Page index**: Preserves the original document structure and enhances chapter positioning. It is suitable for manuals, specifications, reports, and similar materials.
- **Mind map**: Expands content relationships around core topics and helps users quickly understand the overall document structure.
- **Timeline**: Organizes event information in chronological order. It is suitable for historical materials, project records, event tracking, and similar content.
- **Knowledge page**: Generates interconnected knowledge pages. It is suitable for enterprise knowledge, product materials, and domain knowledge management.

Generated knowledge artifacts can be used as auxiliary information for subsequent retrieval and Q&A, improving the efficiency of knowledge queries and content understanding.

## Core Concepts

Before using knowledge compilation, you need to understand the following basic concepts:

- **Compilation template**: Defines how knowledge compilation is generated, including the information types to extract, the organization structure, and generation rules. Users can select different templates based on actual requirements to generate the corresponding knowledge artifacts.
- **Knowledge artifact**: A structured result produced by knowledge compilation, including knowledge graphs, knowledge trees, page indexes, mind maps, timelines, and knowledge pages. Different knowledge artifact types are suitable for different information organization scenarios.
- **Compilation node**: A processing node in the knowledge compilation flow that executes a specified compilation task. When using a compilation node, you need to associate it with the corresponding compilation template to determine the format and structure of the generated content.

## Template Selection Recommendations

Knowledge compilation provides multiple built-in templates. Different templates are suitable for different knowledge organization methods. When creating a knowledge compilation template, select an appropriate template based on the document content and the expected knowledge artifact.

| Template | Applicable Scenario |
| --- | --- |
| Graph | Suitable for extracting entities and relationships between entities in documents, such as people, organizations, products, and their relationships. |
| Tree | Suitable for organizing document content by topic and hierarchy, arranging knowledge into a tree structure. |
| PageIndex | Suitable for preserving the original chapter and page structure of a document and building a hierarchical index for quick content positioning and retrieval. |
| MindMap | Suitable for extracting core topics and branch content from documents and displaying the knowledge structure as a mind map. |
| Timeline | Suitable for documents that contain clear time information and events, organizing and displaying events in chronological order. |
| Wiki | Suitable for documents with substantial content and relationships between topics, organizing the content into interconnected Wiki pages. |

After selecting a template, you can also adjust global rules and template-specific configurations based on actual business requirements to control the content and generation results of knowledge compilation.

## Preparation Before Starting

Before configuration, confirm the following conditions:

- An available LLM has been configured, and the model has strong text understanding, structured output, and reasoning capabilities.
- You have created or plan to create an Ingestion Pipeline that includes Parser, Chunker, Compiler, and Indexer.
- Source documents with clear topics and reliable content are ready.
- The template type to use has been determined based on the target knowledge structure.

Recommendation: When using this feature for the first time, select a small number of representative documents for testing. After confirming the output structure and quality, process data at a larger scale.

## Standard Workflow

1. **Create a knowledge compilation template**: On the Agent page, select **Compilation Operator** when creating a new Agent. Then create a template based on the type of knowledge artifact to generate and complete the related parameter configuration.
2. **Configure the Ingestion Pipeline**: Add Compiler to the Ingestion Pipeline and select the created knowledge compilation template.
3. **Apply the Ingestion Pipeline**: In Dataset, select the documents to process and apply the configured Ingestion Pipeline.
4. **Execute knowledge compilation**: The system parses documents according to the Pipeline configuration and generates the corresponding knowledge artifacts based on the selected template.
5. **View knowledge artifacts**: After compilation is complete, view generated artifacts such as Graph, Tree, PageIndex, MindMap, Timeline, or Wiki.

![Create a knowledge compilation template](https://raw.githubusercontent.com/infiniflow/ragflow-docs/78dcfd707366b45934720c7abe480897f31ecbe7/images/usage-flow-standard-usage-flow.jpg)

![Choose Compilation Operator](https://raw.githubusercontent.com/infiniflow/ragflow-docs/78dcfd707366b45934720c7abe480897f31ecbe7/images/usage-flow-standard-usage-flow-2.jpg)

Note: This section helps users quickly understand the overall workflow of knowledge compilation and only shows the operation interface for "creating a knowledge compilation template". Ingestion Pipeline configuration, document application, knowledge artifact viewing, and other operations are described in detail in the corresponding later chapters with interface screenshots. For specific operations, refer to the relevant chapters.

Typical flow: Parser -> Chunker -> Compiler -> Indexer.

Parser is responsible for parsing, Chunker is responsible for splitting, Compiler is responsible for knowledge compilation, and Indexer is responsible for building the indexes required for subsequent retrieval.
