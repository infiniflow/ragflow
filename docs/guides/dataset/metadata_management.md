---
sidebar_position: 6
title: "Metadata: Metadata Management"
sidebar_label: "Metadata: Metadata Management"
slug: /metadata_management
sidebar_custom_props:
  categoryIcon: LucideDatabaseZap
---

# Metadata: Metadata Management

## Metadata Overview

Metadata is structured information associated with documents or chunks. It is used to describe the source, category, time, owner, business attributes, or other supplementary information of content. Metadata can be used for document management, filtering, retrieval range restriction, and result analysis.

Metadata may come from system built-in fields, automatic generation during parsing, table column role configuration, or manual maintenance.

## View and Manage Metadata

In document management or document details, you can view and maintain metadata related to documents. Metadata is displayed in list form, including **Field**, **Type**, **Values**, and related operations.

![Open document metadata](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/delete_chunk.jpg)

![View metadata fields, types, and values](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/view_and_manage_metadata.jpg)

## Add Metadata

You can add metadata manually for documents. When adding metadata, you need to specify the field name, data type, and value. To improve subsequent filtering and retrieval, keep field names, data types, and value formats consistent.

![Add a metadata field](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/add_metadata.jpg)

## Automatically Generate Metadata

For a built-in parsing mode, configure automatic metadata through **Dataset > Configuration > Extractor > Metadata**.

1. Open the target Dataset and select **Configuration**.
2. Open the **Extractor** tab and expand **Metadata**.
3. Turn on the switch beside **Metadata**. Expanding the section alone does not enable generation.
4. Click **Settings** to open **Metadata generation settings**.
5. In **Generation**, click **Add** to define fields and their descriptions, types, and allowed values. In **Built-in**, enable predefined fields such as `update_time` and `file_name` as needed.
6. Click **Save** in the dialog, then **Save** on the Dataset configuration page.

For metadata generated from document content, check that the Extractor's **Model** is available. See [Dataset configuration](./configuration.md) for the detailed settings.

If the Dataset uses a custom ingestion pipeline, configure metadata in that pipeline's [Transformer](../agent/ingestion_pipeline/configure_transformer_component.md), publish the updated version, then open **Dataset > Configuration > Parsing mode**, expand the pipeline, select the updated published version, and click **Save** to apply it.

Changes to auto metadata configuration only affect newly parsed documents. If you need to apply new metadata rules to existing documents, parse the relevant documents again.

## Use Metadata to Filter Documents

On the document list page, you can use **Metadata field** to filter documents. The available fields and values depend on the metadata already present in the current dataset.

For table documents, columns configured as **Metadata** or **Both** can be used as metadata filter fields.

## Metadata and Retrieval

Metadata can also be used to restrict the retrieval range. After metadata conditions are configured, retrieval only searches content that meets the metadata conditions.

When using metadata, keep field names, data types, and value formats unified and accurate. Metadata conditions that are too strict or inaccurate field values may exclude relevant content from the retrieval scope.
