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

## Add Metadata

You can add metadata manually for documents. When adding metadata, you need to specify the field name, data type, and value. To improve subsequent filtering and retrieval, keep field names, data types, and value formats consistent.

## Automatically Generate Metadata

After **Auto Metadata** is enabled in dataset configuration, the system can automatically generate metadata during document parsing according to the configured generation rules. Automatically generated metadata usually comes from document content, source information, or built-in parsing information.

Changes to auto metadata configuration only affect newly parsed documents. If you need to apply new metadata rules to existing documents, parse the relevant documents again.

## Use Metadata to Filter Documents

On the document list page, you can use **Metadata field** to filter documents. The available fields and values depend on the metadata already present in the current dataset.

For table documents, columns configured as **Metadata** or **Both** can be used as metadata filter fields.

## Metadata and Retrieval

Metadata can also be used to restrict the retrieval range. After metadata conditions are configured, retrieval only searches content that meets the metadata conditions.

When using metadata, keep field names, data types, and value formats unified and accurate. Metadata conditions that are too strict or inaccurate field values may exclude relevant content from the retrieval scope.
