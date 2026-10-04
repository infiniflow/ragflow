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

## Basic Information

The configuration page allows you to manage the core settings of a knowledge base. Basic information includes the name, language, avatar, description, permissions, embedding model, PageRank, and tag sets.

- **Name**: The name of the knowledge base. It can be changed after creation and is displayed on knowledge base cards and detail page headers.
- **Language**: The primary language of the knowledge base. This setting affects the language assumptions used during parsing and model processing.
- **Avatar**: The avatar of the knowledge base. Image uploads are supported, with a maximum file size of 4 MB.
- **Description**: A description of the knowledge base, used to explain its data scope, business purpose, or maintenance information.
- **Permissions**: Controls access to the knowledge base and the scope of allowed operations.
- **Embedding model**: The model used to vectorize chunks. Changing the embedding model after content has already been parsed usually affects existing indexes and should be done with caution.
- **PageRank**: Sets the PageRank score for the knowledge base. During retrieval, this score is added to the hybrid similarity score of matching chunks from the knowledge base, increasing their ranking weight. This is useful when searching across multiple knowledge bases and you want to prioritize content from a specific knowledge base.

## Dataset Page Overview

The following briefly introduces the main entries on the dataset detail page and helps you quickly understand the purpose of each page. The specific operations, configuration items, and usage methods for each feature are described in detail in later sections.

- **File list**: The default entry on the dataset detail page. It is used to manage documents, parsing status, enabled status, chunk count, metadata field count, and document-level operations in the dataset.
- **Retrieval Testing**: Used to enter test questions and adjust retrieval parameters to verify the recall effect of the current dataset. Test parameter adjustments are not saved automatically and must be applied separately in **Chat Assistant** or the **Retrieval Agent** component.
- **Artifacts**: Used to view entries for knowledge artifacts related to the current dataset, such as **Wiki**, **Navigation**, and **Graph**. This dataset manual only introduces the entry and viewing method. For generation and updates, see **Knowledge Compilation**.
- **Logs**: Used to view document parsing and dataset-level task records, including document logs and dataset-level logs.
- **Configuration**: Used to maintain the dataset's basic information, embedding model, parsing method, data source associations, and other configurations.
