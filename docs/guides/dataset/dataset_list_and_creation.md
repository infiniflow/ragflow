---
sidebar_position: 2
title: Dataset List and Creation
sidebar_label: Dataset List and Creation
slug: /dataset_list_and_creation
sidebar_custom_props: {
  categoryIcon: LucideDatabaseZap
}
---

# Dataset List and Creation

## Create Dataset

On the dataset list page, click **Create dataset** to open the creation popup. When creating a dataset, you need to set **Name**, **Embedding model**, and **Parse type**, and then, based on the selected parse type, further select a **Built-in** parsing method or an existing **Pipeline**.

1. Fill in **Name**. The name cannot be empty. It is recommended to use a name that reflects the business or material scope.
2. Select **Embedding model**. The drop-down list displays embedding models that have been added to the system and are currently available. Select the model used to vectorize documents in the dataset.
3. Select **Parse type**. RAGFlow provides the following two types of document parsing:
   - **Built-in**: Uses RAGFlow's built-in document parsing methods. The system provides multiple preset parsing methods. You can choose a suitable method based on the document content and structure, such as general documents, Q&A, tables, papers, books, presentations, and so on. This is suitable when you want to directly use the system's preset parsing rules to process documents.
   - **Pipeline**: Uses a custom pipeline to parse documents. The pipeline must be created and configured in **Agent** in advance. You can customize the document parsing and processing flow according to actual requirements. After creation, you can select the corresponding pipeline when creating a dataset.
4. When **Built-in** is selected, continue selecting the specific built-in parsing method. When **Pipeline** is selected, select the pipeline used to process documents.
5. Click **Save** to complete creation. Creating a dataset is only the first step. After creation, it is recommended to go to the dataset configuration page and further check and configure language, parsing method, auto metadata, data source associations, and other options based on the actual scenario before uploading documents in bulk.

## View Dataset List

After entering **Datasets** from the left menu, the page displays the datasets accessible to the current user as cards, including datasets created by the user and datasets shared by other users. Each dataset card displays basic information such as the dataset avatar or initials, name, document count, and creation time, making it easy to quickly understand the dataset content and status.

Click a dataset card to enter its detail page and view and manage documents, chunks, and related configuration. If there are many datasets, use the pagination feature at the bottom of the page to switch pages and adjust the number of datasets displayed per page.

Click **Create dataset** to create a new dataset. During creation, configure the dataset name, embedding model, document parsing method, and other information.
