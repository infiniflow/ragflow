---
sidebar_position: 4
title: "Files: Dataset Document Management"
sidebar_label: "Files: Dataset Document Management"
slug: /files_dataset_document_management
sidebar_custom_props:
  categoryIcon: LucideDatabaseZap
---

# Files: Dataset Document Management

## Document Management Overview

Document management is used to upload, add, parse, search, filter, enable, disable, delete, and maintain documents in a dataset. Documents become retrievable content only after parsing generates chunks. On the document management page, you can complete the complete process from importing files to parsing, checking results, maintaining metadata, and maintaining chunks.

## Document List

The document list is used to view the basic status of each document in the current dataset. The main fields in the list include **Name**, **Size**, **Source**, **Enabled**, **Chunks**, **Metadata**, **Parse**, **Status**, and **Action**.

- **Name**: The document name.
- **Size**: The document file size.
- **Source**: The document source, such as local upload, associated data source, or file management.
- **Enabled**: Whether the document participates in dataset retrieval. After a document is disabled, it remains in the dataset but is not used as a retrieval source.
- **Chunks**: The number of chunks generated for the document. A value of `0` usually means the document has not been parsed, parsing failed, or no usable content was produced after parsing.
- **Metadata**: The number of document-level metadata fields. Click this field to view or edit document metadata.
- **Parse**: Displays the current parsing method or provides entries related to adjusting the parsing method and starting parsing.
- **Status**: Displays the document parsing task status, used to determine whether the document is waiting for parsing, being parsed, parsed successfully, or failed.
- **Action**: Provides operations such as renaming the document, viewing information, downloading, and deleting.

## Add Documents

### Upload Local Documents

Click **Add file** and select **Upload file** to upload documents from the local machine to the current dataset. After upload, the documents appear in the document list.

1. Go to the **Files** page of the target dataset.
2. Click **Add file** and select **Upload file**.
3. Select the local documents to add to the dataset.
4. Choose whether to enable **Parse on creation** as needed.
5. After confirming the upload, view the document status in the document list.

### Add Documents from an Associated Data Source

If a dataset has already been associated with an external data source in configuration, documents in the data source can be added to the dataset from the document management page. After addition, the documents enter the dataset document list. They still need to complete parsing before generating chunks, metadata, and retrieval content.

Operation steps:

1. Go to the **Files** page of the target dataset.
2. Click **Add file** and select the associated data source entry.
3. Select the documents to add.
4. Confirm the addition and view the document status in the document list.

### Add Documents from File

Add documents from File Management

Files that have already been uploaded to **File** can be added to a knowledge base by connecting them to the target knowledge base.
Once connected, the files will be processed according to the configuration of the target knowledge base.
For detailed instructions, see [**File > Connect to a knowledge base**](../file/link_dataset.md)

## Parse Documents

Document parsing converts source files into chunks, metadata, and other data available for retrieval. By default, documents use the current dataset configuration's parsing method. For documents that require special handling, you can also change the parsing method of a single document.

### Start Parsing

You can start parsing after documents are uploaded or added.

1. Select one or more documents in the document list.
2. Click **Run** or the corresponding parsing entry.
3. If the document has old chunks or existing auto metadata, handle them according to the interface prompt.
4. After confirming the operation, the system starts the parsing task.

### View Parsing Status

You can view the current parsing status of each document in the document list and view parsing progress and related information through **Logs**. Common statuses include waiting, running, completed, failed, and canceled.

If parsing fails, view the related information in **Logs**, troubleshoot the problem, and run parsing again.

### Parse Again

If parsing configuration changes or parsing results need to be updated, you can parse a document again. Common scenarios that require parsing again include:

- The parsing method or its parameters changed.
- Chunk splitting configuration changed.
- Auto metadata configuration changed.
- Table column role configuration changed.
- The source document was updated or replaced.
- Existing parsing results are incorrect and need to be regenerated.

Parsing again may regenerate chunks and metadata. Before operating, confirm whether old results need to be retained, overwritten, or updated according to the interface prompt.

### Change the Parsing Method of a Single Document

The parsing method in dataset configuration is the default parsing configuration for documents. If a document needs a different parsing method, you can change it separately in the document list or document detail page.

This adjustment only applies to the current document and does not affect the default parsing configuration of the dataset.

## Search and Filter Documents

**Search documents**: You can enter keywords in the document list to search by document name or related content. Search helps quickly locate target documents when there are many documents.

**Filter documents**: The document list supports filtering by parsing status, enabled status, source, and other conditions. The number of documents matching each condition is displayed on the right side of each filter item. You can select one or more conditions as needed.

**Metadata field** is used to further filter documents based on document metadata. The system displays metadata fields available for filtering in the current dataset. For table documents using the **Table** parsing method, columns set to **Metadata** or **Both** in column role configuration can appear here as metadata fields.

You can search field names in the search box under **Metadata field**, or expand a specific field and select corresponding values as filter conditions. After setup, click **Submit** to apply filter conditions. Click **Clear** to clear the current filter conditions.

> Note: The fields actually displayed in **Metadata field** depend on the document metadata in the current dataset, so available filter fields may vary between datasets.

## Batch Operations

After selecting the checkboxes on the left side of the document list, a batch operation bar appears, allowing multiple documents to be processed at the same time.

- **Enabled**: Batch-enable selected documents.
- **Disabled**: Batch-disable selected documents.
- **Run**: Batch-parse selected documents and process old chunks and auto metadata application when needed.
- **Cancel**: Cancel running parsing tasks.
- **Metadata**: Maintain metadata for selected documents in bulk.
- **Delete**: Delete selected documents.

## Single-Document Operations

### View and Manage Documents

Click the document name to enter the document detail page and view document information, parsing results, chunks, and metadata.

- **View parsing results**: Click the document name to enter document details and view chunks and related information generated by parsing.
- **View metadata**: View or edit document-level metadata.
- **Download**: Download the source document.
- **Rename**: Rename the document.

### Enable and Disable Documents

The enabled status determines whether a document participates in dataset retrieval. After a document is disabled, its chunks are no longer used as retrieval sources, but the document and parsing results remain in the dataset.

If you want to temporarily remove a document from retrieval without deleting it, disable the document. If you need to restore retrieval, enable it again.

### Delete Documents

Deleting a document removes it from the current dataset. The corresponding chunks, metadata, and parsing results are also removed and no longer participate in later retrieval or Q&A.

Before deleting, confirm that the document and its parsing results are no longer needed.
