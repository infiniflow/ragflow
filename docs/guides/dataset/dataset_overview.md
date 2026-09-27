---
sidebar_position: 1
title: Dataset Overview
sidebar_label: Dataset Overview
slug: /dataset_overview
sidebar_custom_props: {
  categoryIcon: LucideDatabaseZap
}
---

# Dataset Overview

## What Is a Dataset

A dataset is the workspace in RAGFlow that carries knowledge sources and retrieval content. A dataset usually corresponds to a group of business materials, a document collection, or an external data source. In a dataset, users import files, parse files, split them into chunks, maintain metadata, and validate recall. Later, modules such as chats, search, and Agents use this content for retrieval augmentation.

In terms of responsibility, a dataset is more than a "folder". It converts raw documents into retrievable chunks, stores the enabled status of documents and chunks, maintains metadata, and provides foundational data for knowledge artifacts and log tracing.

## Basic Workflow

The following briefly introduces the basic dataset workflow and helps you quickly understand the overall process from creating a dataset to testing retrieval results. The specific operations, configuration items, and feature descriptions involved in each step are described in detail in later sections.

1. Create a dataset and select an embedding model and parsing method.
2. Complete dataset configuration under **Configuration**.
3. Upload or add documents under **Document Management**.
4. Parse documents and generate chunks.
5. Check and adjust chunks and metadata.
6. Use **Retrieval Testing** to test retrieval results.
7. Adjust parsing or retrieval configuration based on the test results.

## Dataset Page Overview

The following briefly introduces the main entries on the dataset detail page and helps you quickly understand the purpose of each page. The specific operations, configuration items, and usage methods for each feature are described in detail in later sections.

- **File list**: The default entry on the dataset detail page. It is used to manage documents, parsing status, enabled status, chunk count, metadata field count, and document-level operations in the dataset.
- **Retrieval Testing**: Used to enter test questions and adjust retrieval parameters to verify the recall effect of the current dataset. Test parameter adjustments are not saved automatically and must be applied separately in **Chat Assistant** or the **Retrieval Agent** component.
- **Artifacts**: Used to view entries for knowledge artifacts related to the current dataset, such as **Wiki**, **Navigation**, and **Graph**. This dataset manual only introduces the entry and viewing method. For generation and updates, see **Knowledge Compilation**.
- **Logs**: Used to view document parsing and dataset-level task records, including document logs and dataset-level logs.
- **Configuration**: Used to maintain the dataset's basic information, embedding model, parsing method, data source associations, and other configurations.
