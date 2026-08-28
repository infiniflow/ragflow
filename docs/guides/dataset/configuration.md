---
sidebar_position: 3
title: Configuration
sidebar_label: Configuration
slug: /dataset_configuration
sidebar_custom_props: {
  categoryIcon: LucideDatabaseZap
}
---

# Configuration

## Basic Information

The configuration page is used to maintain the core configuration of a dataset. Basic information includes **Name**, **Language**, **Avatar**, **Description**, **Embedding model**, **PageRank**, and **Tag sets**.

- **Name**: The dataset name. It can still be modified after creation and is used in list cards and the detail page title.
- **Language**: The primary language of the dataset. The language setting affects language assumptions during parsing and model processing.
- **Avatar**: The dataset avatar. Image uploads are supported, and the maximum image size is 4 MB.
- **Description**: The dataset description, used to describe the material scope, business purpose, or maintenance notes.
- **Embedding model**: The model used to vectorize chunks. Changing the embedding model after content has already been parsed usually affects the index and should be handled carefully.
- **PageRank**: Sets the dataset's PageRank score. During retrieval, this score is added to the hybrid similarity score of matched chunks in the dataset, increasing their ranking weight. This is suitable when content from a specific dataset needs higher priority during retrieval across multiple datasets.
- **Tag sets**: Used to associate one or more tag sets with a dataset and add tags to chunks in bulk based on text similarity. During retrieval, queries are also automatically associated with corresponding tags, improving retrieval accuracy with tag information. A tag set must be generated as a sample before use.

## Parsing Method

The parsing method determines how a dataset processes uploaded documents and how documents are converted into chunks. **Parse type** in configuration provides two entries: **Built-in** and **Pipeline**.

### Built-in Parsing

**Built-in** means using RAGFlow's built-in document parsing capabilities. Users can choose a suitable parsing method based on the document type. The system then reads document content, splits it into chunks, and generates the structure required for retrieval according to the selected method.

The following built-in parsing methods are available:

- **General**: A general-purpose parsing method suitable for most conventional documents. It identifies document content and creates chunks according to the configured chunking rules.
- **Q&A**: Designed for data organized as question-answer pairs. Each Q&A pair is treated as an individual chunk.
- **Manual**: Designed for PDFs with a clear hierarchical section structure, such as product manuals and operation guides. Documents are chunked based on their section structure.
- **Table**: Designed for structured tabular data such as XLSX and CSV/TXT files. Each row is typically treated as an individual chunk.
- **Paper**: Designed for PDF papers, research reports, and other academic documents. Documents are chunked based on structural elements such as abstracts, sections, and subsections.
- **Book**: Designed for DOCX, PDF, and TXT books or other long documents with a chapter-based structure.
- **Laws**: Designed for legal documents in DOCX, PDF, and TXT formats. Chunk boundaries are identified based on the structural characteristics of legal documents.
- **Presentation**: Designed for presentations in PDF and PPTX formats. Each page or slide is typically treated as an individual chunk.
- **One**: Treats the entire document as a single chunk. This method is suitable for relatively short documents when the complete context needs to be preserved.
- **Tag**: Used to create a tag set. A dataset using this method provides tags for chunks and queries in other datasets and does not directly participate in the RAG retrieval process.

> **Tip:** On the built-in parsing configuration page, click **Built-in pipeline introduction** on the right to view the supported file formats, detailed chunking rules, and examples for each parsing method.

Selection suggestion: choose the parsing method based on the form of the material itself. Use **General** first for regular documents; use **Table** first for tabular materials; choose the corresponding method for Q&A collections, manuals, papers, images, audio, and emails to reduce later chunk adjustment costs.

### Chunk and Parsing Configuration

After selecting a **Built-in** parsing method, you can further configure document parsing, chunk splitting, and content enhancement parameters. Reasonable configuration helps improve the accuracy and recall of later retrieval.

Different built-in methods display different configuration items. Use the current interface as the source of truth.

#### Document Parsing Configuration

- **PDF parser**: Specifies the parser used when parsing PDF documents. Different parsers differ in text recognition, table structure recognition, layout analysis, processing speed, and other aspects. Choose one based on the content type and parsing requirements of the PDF. The system provides the following PDF parsers by default:
  - **DeepDoc**: The default PDF parser in RAGFlow. It can perform OCR, table structure recognition (TSR), document layout understanding (DLR), and other tasks. It is suitable for PDFs that contain complex layouts, images, scanned content, or tables. Because additional document analysis is required, parsing may take relatively longer.
  - **Naive**: Suitable for PDFs mainly composed of plain text. This method skips OCR, TSR, DLR, and other processing, reducing unnecessary parsing overhead. If a PDF contains scanned pages, complex layouts, or tables, this method is not recommended.
  - **Docling**: An open-source document processing tool that can parse documents such as PDFs and extract structured content.
  - **TCADPParser**: Tencent's open-source document parsing tool, which can be used for PDF content parsing.

In addition to the built-in parsers above, you can also use a vision-language model (VLM) that supports PDF parsing. To use a third-party vision model to parse PDFs, first configure the default VLM under **Model Providers > Set default model**. After configuration, select the corresponding model from the **PDF parser** drop-down list.

Usually, some vision models whose names contain identifiers such as **VL** or **V** support PDF parsing. For documents with complex layouts or mixed text and images, a more capable vision model may produce better parsing results, but it also incurs additional model calls and token consumption. The specific support and parsing effect depend on the model used.

#### Chunk Splitting Configuration

The following configurations are mainly used for built-in methods that need to split text according to specified rules, such as **General**.

- **Recommended chunk size**: Sets the recommended target size when generating chunks. Smaller chunks can provide more fine-grained retrieval results but may lack context. Larger chunks can preserve more context, but retrieval result granularity is relatively coarse.
- **Delimiter for text**: Sets the delimiter used for text splitting. The system splits text based on the delimiter and chunk size. It is recommended to set an appropriate delimiter based on the original paragraph and line-break structure to preserve semantic completeness as much as possible.
- **Overlapped percent (%)**: Sets the content overlap ratio between adjacent chunks. Increasing overlap appropriately can reduce context loss caused by split boundaries, but an overly high overlap ratio increases repeated content.
- **Child chunk are used for retrieval**: Controls whether finer-grained child chunks participate in retrieval. This is suitable for long documents where fine-grained recall needs to be improved.

#### Page and Multimodal Content Configuration

- **Page Index**: Controls whether Page Index-related capabilities are used to process document page information.
- **Image & table context window**: Sets the context range between images or tables and surrounding text. Increasing this value appropriately can preserve more related context when processing images or tables.

#### Content Enhancement Configuration

Some built-in methods provide the following content enhancement configurations, such as **General**, **Manual**, **Paper**, **Book**, and **Laws**.

- **Auto metadata**: Controls whether metadata is generated automatically. After it is enabled, the generation method for metadata can be further configured through **Settings**.
- **Auto-keyword**: Sets the number of keywords automatically generated for chunks to supplement chunk semantic information.
- **Auto-question**: Sets the number of questions automatically generated based on chunk content to supplement query expressions that may match the current chunk.

#### Table Parsing Configuration

When **Table** is selected, the interface provides a dedicated **Column mode** configuration used to control how table columns participate in generating chunk content and metadata.

- **Auto**: The system processes table columns according to the default rules. By default, all columns are included in the chunk text and are also saved as metadata.
- **Manual**: Manually configures the table columns that need to participate in processing. This configuration mainly targets structured table data and differs from chunk splitting configuration for ordinary text-based built-in methods.

#### Other Format Processing Configuration

Some built-in methods may also provide configuration for specific file formats, for example:

- **Excel to HTML**: Controls whether Excel content is converted into HTML for processing, preserving table structure information.

Different built-in methods use different parsing logic, so not all configuration items appear at the same time. After selecting a built-in method, configure only the parameters displayed in the current interface. In general, for ordinary text documents, focus on chunk splitting parameters such as **Recommended chunk size**, **Delimiter for text**, and **Overlapped percent (%)**. For PDFs, papers, books, manuals, or legal documents, focus on **PDF parser** and the corresponding content enhancement configuration. For table data, focus on **Column mode**. When you need to further enrich retrieval information, use content enhancement features such as **Auto metadata**, **Auto-keyword**, and **Auto-question** according to actual requirements.

After configuration is complete, click **Save** to save the settings. When documents are parsed later, the system processes them according to the currently selected built-in method and its configuration.

### Pipeline: Custom Parsing

**Pipeline** means using a custom **Ingestion Pipeline** as the dataset's parsing method. It is suitable for scenarios that require custom document processing logic or complex processing flows.

The pipeline must be created and configured in **Agent > Ingestion Pipeline** in advance. In the dataset, you do not need to configure the processing nodes inside the pipeline again. You only need to select the created pipeline.

Usage:

1. Create and configure an **Ingestion Pipeline** in **Agent > Pipeline**.
2. Go to **Dataset > Configuration** and select **Pipeline** as the parsing method.
3. Select the pipeline to use from the pipeline list.
4. Save the configuration. Afterward, documents in the dataset are processed according to the selected pipeline.

If no pipeline is currently available, you can use the entry provided in the pipeline area to go to **Agent** and create one.

> Note: For pipeline creation, node configuration, and flow orchestration, see [**Ingestion Pipeline**](../../guides/agent/agent_overview.md).

### Auto Metadata: Automatic Metadata Configuration

**Auto Metadata** is used to configure metadata automatically generated during document parsing. The system supports two types of metadata: **Generation** and **Built-in**. You can configure them separately as needed.

> Note: Changes in **Metadata generation settings** only take effect for newly parsed documents later. They do not automatically update documents that have already completed parsing. To apply the new configuration to existing documents, parse the relevant documents again.

#### Generation: Custom Metadata Generation

**Generation** is used to customize metadata fields that need to be generated from document content. Click **Add** to add a field and configure the following items as needed:

- **Field**: The metadata field name.
- **Description**: The field description, used to explain the information to extract or generate from the document.
- **Type**: The field's data type.
- **Values**: The allowed values for the field, used to restrict the generated result's value range.

After saving, the system generates corresponding metadata from document content during subsequent document parsing based on these field definitions.

#### Built-in: Built-in Metadata

**Built-in** provides system-predefined metadata fields. You do not need to manually create or configure field rules. Use the switch on the right side of each corresponding field to choose whether to generate that metadata during document parsing. The following built-in fields are currently supported:

- **update_time**: Records the document update time.
- **file_name**: Records the document file name.

Select the fields to use, enable their switches, and then click **Save** to save the configuration. Generated metadata can be used in document management and retrieval filtering. To view or edit generated metadata, [**Metadata management**](../dataset/metadata_management.md).

### Table Column Role Configuration

When a dataset uses **Table** as the built-in parsing method, you can use **Column mode** to set the purpose of each table column during parsing and retrieval. A column can be included in chunk text for indexing, used only as metadata, or used for both.

#### Auto

When **Auto** is selected, all columns are included in chunk text and are also stored as metadata. This is RAGFlow's default setting. This mode is suitable when you do not need to distinguish the purpose of each column and want all table content to participate in retrieval and also be available as metadata.

#### Manual

When **Manual** is selected, RAGFlow identifies columns in the table and displays them one by one according to the original table column names. For example, **CRIM**, **ZN**, **INDUS**, **CHAS**, and similar names in the interface come from the column names in the current table. They are not predefined RAGFlow fields.

Users can assign one of the following roles to each column through the drop-down menu on the right side of each column name:

- **Indexing**: Includes the column content in chunk text for vector retrieval and full-text retrieval. This is suitable for columns that contain the main retrieval content.
- **Metadata**: Saves the column only as metadata and does not include it in chunk text. This column can be used as a filter field to narrow the retrieval scope.
- **Both**: Uses the column for both indexing and metadata. The column is included in chunk text and participates in vector retrieval and full-text retrieval, and can also be used as metadata for filtering.

For example, if a table contains four columns, **Title**, **Content**, **Category**, and **Year**, you can set them according to actual use:

- **Title -> Both**: The title participates in retrieval and can also be used as metadata.
- **Content -> Indexing**: The body text is mainly used for content retrieval.
- **Category -> Metadata**: Used to filter retrieval results by category.
- **Year -> Metadata**: Used to filter retrieval results by year.

In this way, fields that do not need to participate in content retrieval can be prevented from entering chunk text, while these fields are still preserved as retrieval filter conditions.

> Note: Column role configuration applies to the column structure of a table, not to individual cells. After configuration is modified, the new settings apply to subsequently parsed documents. For documents that have already completed parsing, the documents must be parsed again before the new column roles take effect.

### Associated Data Sources

The **Data source** area is used to associate the current dataset with data sources that have already been added.

Click **Link data source** and select the data source to associate from existing data sources. After association, the current dataset can use the data provided by that data source. This area is only used to establish and manage the association between the dataset and data sources. It does not provide data source creation or connection configuration.

> Tip: To add a new data source, or configure data source connection information, synchronization methods, and other settings, see [**Data source**](../../guides/data_source/overview_and_page_management.md). After adding the data source, return to the dataset configuration page to associate it.
