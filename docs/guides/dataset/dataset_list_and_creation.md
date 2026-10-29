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

Click **Create dataset** on the dataset list page, then complete the following settings:

1. Enter **Name**.
2. Select an available **Embedding model**.
3. In **Parse mode**, choose a built-in template, such as **General**, or an ingestion pipeline previously created in **Agent**. Built-in templates and saved pipelines appear in the same list.
4. Click **Save**, then review the dataset's **Configuration** before uploading documents.

## View Dataset List

After entering **Datasets** from the top navigation bar, the page displays the datasets accessible to the current user as cards, including datasets created by the user and datasets shared by other users. Each dataset card displays basic information such as the dataset avatar or initials, name, document count, and creation time, making it easy to quickly understand the dataset content and status.

Click a dataset card to enter its detail page and view and manage documents, chunks, and related configuration. If there are many datasets, use the pagination feature at the bottom of the page to switch pages and adjust the number of datasets displayed per page.

Click **Create dataset** to create a new dataset. During creation, configure the dataset name, embedding model, document parsing method, and other information.

![Dataset list and creation entry](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/create_dataset_2.jpg)
