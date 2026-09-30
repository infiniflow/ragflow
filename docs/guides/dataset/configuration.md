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
- **Audio**: An audio parsing method for audio files such as WAV, MP3, AAC, FLAC, and OGG. It uses an ASR model to transcribe audio into text and then generates chunks from the transcription.
- **Email**: An email parsing method for email files such as EML and MSG. It extracts content including the sender, recipients, subject, body, and attachments, and generates chunks from the extracted content.

> **Tip:** On the built-in parsing configuration page, click **View built-in parser details** on the right to view the supported file formats, detailed chunking rules, and examples for each parsing method.

Selection suggestion: choose the parsing method based on the form of the material itself. Use **General** first for regular documents; use **Table** first for tabular materials; choose the corresponding method for Q&A collections, manuals, papers, images, audio, and emails to reduce later chunk adjustment costs.

### Build-in Parsing Configuration

After selecting a built-in template parsing method, you can further configure document parsing, chunking, and content enhancement. Appropriate settings can improve the accuracy and recall of subsequent retrieval.

Different built-in methods display different configuration options. Refer to the options shown in your current interface.

#### Parser Configuration

Parser configuration determines how different types of content are processed. You can configure parsing parameters for each format according to the document structure, layout complexity, and extraction requirements. This helps improve content extraction and the accuracy of subsequent chunk generation.

##### PDF Parsing

Configure how PDF documents are parsed. You can select a parsing method based on the PDF layout and set options such as page ranges and vision models.

- **Multi-column layout recognition:** Recognizes PDFs with complex layouts, such as two-column or multi-column pages. When enabled, the parser identifies text regions and extracts content in a suitable reading order. This is useful for papers, journals, reports, and other multi-column documents.
- **Remove original table of contents:** Removes the PDF's existing table of contents during parsing so that its text does not enter subsequent chunks or search results as body content.
- **Remove headers and footers:** Identifies and removes repeated page headers and footers to reduce irrelevant content in the knowledge base.
- **Parsing method:** Selects a parser for PDF or table content. Parsing methods differ in text recognition, layout analysis, table recognition, and handling of complex documents. Built-in methods include:
  - **DeepDOC:** RAGFlow's default document parser. It performs OCR, table structure recognition, and document layout analysis. It is suitable for PDFs containing complex layouts, images, scanned content, or tables.
  - **Naive:** Suitable for PDFs that primarily contain plain text. It reduces OCR, table recognition, and complex layout analysis, making it faster for simple text-based PDFs.
  - **Docling:** An open-source document parsing tool that can parse PDFs and other documents and extract structured content.
  - **OpenDataLoader:** Extracts text, layout, and structured information from documents. It can serve as an alternative to other built-in parsing methods.
  - **TCADP Parser:** An open-source parser from Tencent that can parse PDF content.
  - **MonkeyOCRv2:** A vision-based parsing method that combines OCR and visual understanding to process scanned documents, complex layouts, and mixed text and images.

  In addition to these built-in methods, you can select an external model service already configured in the system. For example, a vision-language model (VLM) or another model with document understanding capabilities connected through a model provider can be used to parse PDFs or tables.

  External models are often better suited to complex layouts, scanned documents, images, borderless tables, and mixed text and images. However, they incur additional model calls and token usage. The available models depend on the configured model providers and their capabilities.
- **Page range:** Specifies which PDF pages to parse. You can set a start and end page and add multiple page ranges to avoid processing unnecessary content.
- **Disable vision model:** Controls whether a VLM is used during PDF parsing. When this option is enabled, no vision model assists with parsing. When disabled, a vision model can help interpret complex layouts or content combining text and images.
- **Model:** Selects the vision model to use when PDF parsing requires one. A more capable vision model may provide a more complete understanding of complex layouts and visual content. Parsing quality and token usage depend on the selected model.

##### Table Parsing

Configure how tables in documents are parsed. The system can use a built-in parser or a vision model to recognize table structure and extract text and cell information.

- **Parsing method:** Selects a parser or model for tables. Methods differ in table structure recognition, cell content extraction, and handling of complex layouts. Built-in methods include:
  - **DeepDOC:** RAGFlow's built-in document parser. It recognizes table structure and extracts cell content, making it suitable for most standard tables.
  - **TCADP Parser:** An open-source parser from Tencent that can recognize and parse tables in documents.

  You can also select an external model already configured in the system. Available models are grouped by model provider in the drop-down list. For complex tables, borderless tables, merged rows or columns, or tables that require visual interpretation, choose an external model with vision or document understanding capabilities as needed. External models may incur additional model calls and token usage.
- **Disable vision model:** Controls whether a vision model is used for table parsing. Built-in parsers may be sufficient for regular tables with clear boundaries. A vision model can help with complex or borderless tables.
- **Model:** Selects the vision model used for table parsing when vision support is enabled. Models may differ in their understanding of table structure and complex layouts, as well as in token usage.

For ordinary text-based PDFs, consider starting with a built-in parser. For scanned documents, multi-column layouts, complex tables, and mixed text and images, a vision model may improve parsing quality. Because vision models incur additional calls and token usage, enable them according to the document type.

##### Image Parsing

Configure how image files are parsed so that text or visual content can be converted into text for subsequent chunking and retrieval.

- **Parsing method:** Selects how image files are processed.
  - **OCR:** Uses optical character recognition to extract text from images. It is suitable for screenshots, scans, photographs, and other images containing readable text.
  - **External model:** Uses a configured model with visual understanding capabilities. Compared with traditional OCR, such a model can recognize text and interpret layouts, charts, objects, and relationships between text and images. External models are grouped by model provider in the drop-down list. The available models depend on the configured providers and model capabilities.

For images that mainly contain clear text, consider starting with OCR. For charts, complex layouts, interface screenshots, or content that requires visual understanding, select an external vision model. External models may incur additional model calls and token usage.

Parsing quality also depends on image clarity, text size, layout, and overall image quality.

##### Markdown Parsing

Parse Markdown documents while preserving their text and basic structure for subsequent chunking.

- **Remove original table of contents:** Removes an existing table of contents during parsing to avoid repeating title links and body content.
- **Disable vision model:** Controls whether a vision model assists with Markdown parsing. When enabled, no vision model is used. When disabled, the selected vision model can process images or other content in the Markdown document that requires visual understanding.
- **Model:** Selects the vision model when one is needed. Available models depend on the configured model providers and their capabilities. External models may incur additional model calls and token usage.

##### HTML Parsing

Parse HTML documents to extract useful text and structured content while filtering page elements that need not be included in knowledge base retrieval.

- **Remove original table of contents:** Removes an existing table of contents or navigation list to reduce duplicated content.
- **Remove headers and footers:** Attempts to remove repeated headers and footers, reducing the effect of navigation and copyright information on chunking and retrieval.

For HTML files saved or exported from websites, decide whether to enable these options based on the page structure. Keep headers, footers, or tables of contents when they contain information you need.

##### DOC Parsing

Parse Word documents in `.doc` format. You can choose whether to remove the table of contents, headers, and footers, and whether to use an external vision model for complex content.

- **Remove original table of contents:** Removes the existing table of contents to reduce duplication between the contents list and the document body.
- **Remove headers and footers:** Attempts to remove repeated headers and footers so that page numbers, document titles, and copyright information do not repeatedly appear in chunks.
- **Disable vision model:** Controls whether an external vision model assists with DOC parsing. When enabled, no vision model is called. When disabled, you can select a configured external model to further parse images, complex layouts, or other visual content.
- **Model:** Selects an external model to assist with DOC parsing. Only models already configured in the system are available. The options depend on the connected providers and model capabilities. External models may incur additional model calls and token usage.

##### DOCX Parsing

Parse Word documents in `.docx` format. You can choose whether to remove the table of contents, headers, and footers, and whether to use an external vision model for complex content.

- **Remove original table of contents:** Removes the existing table of contents to avoid duplicating headings and body content.
- **Remove headers and footers:** Removes repeated headers and footers to reduce irrelevant information in subsequent chunks and search results.
- **Disable vision model:** Controls whether an external vision model is used for DOCX parsing. When enabled, no vision model is used. When disabled, an external model can help interpret images, complex layouts, and other visual content.
- **Model:** Selects an external model to assist with DOCX parsing. This option only lists configured external models; it does not offer built-in parsing methods such as DeepDOC, Naive, or Docling. The available models depend on the configured model providers.

##### PPTX Parsing

Configure how PPTX files are parsed. Text, tables, images, and other slide content are extracted and converted into text for subsequent chunking and retrieval.

- **Parsing method:** Selects how PPTX files are processed.
  - **DeepDOC:** RAGFlow's built-in document parser. It extracts text, tables, and layout content from slides and is suitable for most standard PPTX files.
  - **TCADP Parser:** An open-source parser from Tencent that can extract text and structured content from PPTX files.
  - **External model:** Uses a configured model with document or visual understanding capabilities. External models are grouped by model provider in the drop-down list. Availability depends on the configured providers and model capabilities.

For PPTX files mainly containing text and standard tables, consider starting with DeepDOC or TCADP Parser. For complex layouts, mixed text and images, many images, or content requiring visual interpretation, choose a capable external model. External models may incur additional model calls and token usage.

Parsing quality depends on slide layout, image quality, content complexity, and the selected method.

##### Video Parsing

Configure how video files are parsed. An external model recognizes and interprets speech, visuals, and other content, then converts the results into text for subsequent chunking and retrieval.

Select a configured external model with video or multimodal understanding capabilities. Video parsing supports only external models already configured in the system. Such a model can analyze video frames, visible text, speech, and contextual information to generate corresponding text.

Available models depend on the configured providers and their capabilities. Long videos, frequent scene changes, and complex content may increase processing time and model usage. External models may incur additional calls and token usage. Parsing quality depends on video quality, audio and visual clarity, content complexity, and the selected model.

##### Audio Parsing

Configure how audio files are parsed. An external model transcribes speech into text for subsequent chunking and retrieval.

Select a configured external model with automatic speech recognition (ASR) capabilities. Audio parsing supports only external models already configured in the system.

Available models depend on the configured providers and their capabilities. Models may differ in language support, accent recognition, handling of multiple speakers or background noise, and long-audio transcription.

External models may incur additional calls and token usage. Transcription quality depends on recording clarity, background noise, speaking speed, language, and accent.

#### Chunker Configuration

##### General Chunker

Configure general chunking of parsed content. The system splits content into chunks based on the target chunk size, overlap ratio, and delimiters. It can also add context to tables and images or use parent-child chunks to improve retrieval precision.

- **Recommended chunk size:** Sets the target size of each chunk in tokens. The default is **512**. The system attempts to combine adjacent content to reach this size, but the value is a target rather than a strict limit. An indivisible content segment that already exceeds the target may remain as one chunk.
- **Overlap ratio (%):** Sets how much content adjacent chunks share. When overlap is enabled, content from the end of one chunk is copied to the start of the next. This reduces context loss at chunk boundaries and improves semantic continuity. Set it to **0** for no overlap.
- **Table context window:** Sets the amount of preceding and following context, in tokens, attached to a table chunk. More context can preserve the relationship between a table and the surrounding text. Set it to **0** to add no extra table context.
- **Image context window:** Sets the amount of preceding and following context, in tokens, attached to an image chunk. This is useful when nearby text is needed to understand an image. Set it to **0** to add no extra image context.
- **Delimiters:** Sets the separators used to split text, such as the newline character `\n`, periods, question marks, exclamation marks, and semicolons.
  - **Split by delimiter:** Text is first split at the configured delimiters. The system then combines adjacent segments according to the recommended chunk size to form chunks of suitable size while preserving meaning where possible.
  - **Split positions:** Displays all configured delimiters so you can see which boundaries are used for chunking.
  - **Add:** Adds a custom delimiter. You can use characters that mark meaningful boundaries in your documents, such as heading or paragraph markers.
- **Use sub-chunks for retrieval:** Enables parent-child chunking. The system creates smaller child chunks from larger parent chunks and uses the child chunks for retrieval. When a relevant child chunk matches, its parent chunk can provide more complete context.

For ordinary documents, start with the default **512-token** chunk size and choose delimiters that match the document structure. Increase overlap when context frequently crosses chunk boundaries. Configure context windows for documents with many tables or images. Enable sub-chunks when you need precise retrieval while retaining broader context.

##### Title Chunker

The Title Chunker identifies document structure from heading levels and organizes content into chunks. It provides two modes: **Hierarchy** and **Group**.

###### Hierarchy

Hierarchy builds a document tree from heading levels and generates independent chunks. Each chunk retains its full parent heading path. For example, a chunk may contain `Part 1 > Chapter 3 > Section 2` followed by the body text. This helps identify the chunk's location even when it is retrieved outside the original document.

Hierarchy is suitable for documents with relatively independent sections and meaningful heading structures, such as legal provisions, regulations, contracts, and technical specifications.

- **Chunk token limit:** Sets the maximum token count for a chunk. Content exceeding the limit is split further to avoid overly large chunks.
- **Heading level:** Selects the heading level used to generate chunks, such as **H3**. The system identifies H1, H2, H3, and other levels according to the configured heading rules, then organizes and splits content at the selected level.
- **Separate parent heading content:** Controls whether body content directly under a parent heading is handled separately. Enabling it prevents the parent section's own content from being mixed with its child sections in one chunk.
- **Set first chunk as global context:** Uses the document's first chunk as document-level background information for subsequent chunks.
- **Rules:** Define how headings are recognized in different documents. Each rule can contain regular expressions for H1 through H5.
  - **Regular expressions for H1–H5:** Identify headings at each level. For example, expressions can match Markdown headings such as `#`, `##`, and `###`; numbered Chinese headings; or English headings such as `PART`, `Chapter`, `Section`, and `Article`.
  - **Add regular expression:** Adds a heading level and its expression to the current rule.
  - **Add rule:** Adds another set of heading recognition rules for a different language, numbering system, or document format.

Hierarchy works well when the location of content within the document matters. Its chunks retain the relevant heading relationships so that retrieval results can include both the body text and its section path.

###### Group

Group creates flatter chunks at a selected heading level and combines adjacent small sections to preserve semantic continuity. Unlike Hierarchy, Group does not add the full parent heading path to every chunk. It emphasizes continuity between nearby content.

Group is suitable for books, user manuals, reports, articles, and other documents whose neighboring paragraphs or sections depend on one another.

- **Chunk token limit:** Sets the maximum token count for a chunk. The system combines adjacent sections based on heading structure while attempting to keep chunks within this limit.
- **Heading level:** Sets the heading level used to group and split content.
  - **Automatic:** Lets the system choose an appropriate level based on the detected document structure.
  - **H1, H2, H3, and other levels:** Select a specific level according to the document structure to control where content is grouped.
- **Rules:** Define how headings are recognized. As with Hierarchy, you can use multiple sets of regular expressions for different formats, languages, and numbering systems.
  - **Regular expressions for H1–H5:** Define matching patterns for each heading level.
  - **Add regular expression:** Adds another heading-level expression to the current rule.
  - **Add rule:** Adds another heading recognition rule for a different document structure.

Group works well when continuity between nearby sections matters. It combines adjacent sections at heading boundaries to keep chunks meaningful without adding the full parent heading path to each one.

#### Extractor Configuration

The Extractor adds information to chunks after chunking. It can use a model to generate keywords, questions, tags, contextual summaries, and metadata. These additions can improve subsequent retrieval and question answering.

##### Model

Select the model used by the Extractor. Automatic keywords, automatic questions, augmented context, and metadata that requires model generation call the selected model. Available models depend on the configured providers and their capabilities.

##### Automatic Keywords

Automatically extract keywords or phrases that represent the main content of each chunk.

- **Number of automatic keywords:** Sets how many keywords are generated for each chunk. Set it to **0** to disable keyword extraction.
- **System prompt:** Sets the instruction sent to the model. The default prompt asks the model to analyze the text and extract the `{{ topn }}` most important keywords or phrases. You can adapt the prompt to specify keyword types, output language, or format.

Generated keywords add semantic information to chunks and can help with retrieval and content matching.

##### Automatic Questions

Automatically generate questions that each chunk can answer. This adds query-like expressions to the content.

- **Number of automatic questions:** Sets how many questions are generated for each chunk. Set it to **0** to disable question generation.
- **System prompt:** Sets the instruction sent to the model. The default prompt asks the model to understand the text and generate `{{ topn }}` questions related to its main content.

Generated questions should cover important information in the chunk without repeating the same meaning. They can make document content easier to match with users' natural-language questions.

##### Tags

Extract or match tags for chunks using a prepared tag file. Tags can classify knowledge content and add semantic information.

- **Number of automatically extracted tags:** Sets how many tags are extracted for each chunk. Set it to **0** to disable automatic tag extraction.
- **Tag file:** Selects the file used for tag extraction. The system matches or generates tags for chunks based on the selected tag collection.

Prepare a tag file suitable for the knowledge base before using this feature. A well-designed tag system can support content classification, retrieval, and filtering.

##### Augmented Context

Generate a concise contextual summary for each chunk and add it as supplementary information. This helps a chunk retain essential background when viewed separately from its source document.

- **Enable augmented context:** Calls the selected model to generate contextual information from the chunk.
- **System prompt:** Sets the instruction sent to the model. The default prompt asks for a concise summary faithful to the source content, without introducing information absent from it.

Augmented context is useful when a chunk may lose important background after splitting. For example, a chunk containing only a short passage can gain a summary of its subject and context, helping retrieval and model understanding.

Enabling this feature increases model calls and token usage.

##### Metadata

Add structured metadata to parsed content. Metadata describes document or chunk attributes and can be used for retrieval, filtering, and content management.

- **Enable metadata:** Extracts or generates metadata according to the configured fields.
- **Settings:** Opens the metadata generation settings, where you configure the fields to extract or generate.

Metadata generation settings provide the following options:

- **Generate:** Define metadata fields to generate. Add fields and configure their descriptions, types, and values. The system generates metadata based on these settings.
- **Built-in parser template:** Use structured information already provided during parsing as a metadata source, without defining the same information again.
- **Add:** Add a metadata field with a name, description, type, and value according to your needs.

For example, you can add fields for document type, department, author, subject, date, or business category. Well-configured metadata lets you narrow subsequent searches with metadata conditions. Metadata generated by a model may incur additional model calls and token usage.

#### Indexer Configuration

Configure how chunks are indexed and written to the retrieval engine. You can use full-text search, vector search, or both, and specify which content field is indexed. The Indexer uses the embedding model configured for the knowledge base; you do not need to select one here.

- **Search methods:** Select one or more ways to index and retrieve chunks.
  - **Embedding:** Creates vector representations and a vector index for semantic retrieval. A relevant chunk may be retrieved even when the query does not use the same words as the source text.
  - **Full-text:** Creates a full-text index for keyword matching. This is useful for product codes, names, technical terms, code, and other content requiring exact matches.
  - **Embedding + Full-text:** Creates both vector and full-text indexes to support semantic and keyword matching.
- **Filename embedding weight:** Sets the filename's weight in the vector representation. The filename contributes semantic information alongside the chunk content. A higher value gives it more influence. Increase the weight when filenames clearly express document topics; decrease it when filenames are random identifiers or have little meaning.
- **Fields:** Specifies the content used for indexing and retrieval. Available fields are **Processed text**, **Questions**, and **Augmented context**.
  - **Processed text:** Indexes the chunk body produced by the Parser, Chunker, and other preceding steps. This is the most direct method and the default selection.
  - **Questions:** Indexes questions generated by the Extractor's Automatic Questions feature. A user's question can then match a pre-generated question, which is useful for question-answering knowledge bases. Configure automatic question extraction before using this field.
  - **Augmented context:** Indexes context generated by the Extractor instead of relying only on the original chunk content. This is useful when chunks need additional background for effective retrieval.

For most knowledge bases, you can enable both **Embedding** and **Full-text** and index **Processed text**. If you have enabled Automatic Questions or Augmented Context, consider indexing **Questions** or **Augmented context** according to the types of queries you expect.

### Custom Ingestion Pipeline

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
