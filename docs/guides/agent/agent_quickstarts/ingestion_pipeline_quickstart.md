---
sidebar_position: 5
slug: /ingestion_pipeline_quickstart
sidebar_custom_props: {
  categoryIcon: LucideRoute
}
---

# Ingestion pipeline quickstart

RAGFlow's ingestion pipeline is a customizable, step-by-step workflow that prepares your documents for high-quality AI retrieval and answering. You can think of it as building blocks: you connect different processing "components" to create a pipeline tailored to your specific documents and needs.

---

RAGFlow is an open-source RAG platform with strong document processing capabilities. Its built-in module, DeepDoc, uses intelligent parsing to split documents for accurate retrieval. To handle diverse real-world needs—like varied file sources, complex layouts, and richer semantics—RAGFlow now introduces the *ingestion pipeline*.

The ingestion pipeline lets you customize every step of document processing:

- Apply different parsing and splitting rules per scenario
- Add preprocessing like summarization or keyword extraction
- Connect to cloud drives and online data sources
- Use advanced layout-aware models for tables and mixed content

This flexible pipeline adapts to your data, improving answer quality in RAG.

## 1. Understand the core pipeline components

- **Parser** component: Reads and understands your files (PDFs, images, emails, etc.), extracting text and structure.
- **Transformer** component: Enhances text by using AI to add summaries, keywords, or questions to improve search.
- **Chunker** component: Splits long text into optimal-sized segments ("chunks") for better AI retrieval.
- **Indexer** component: The final step. Sends the processed data to the document engine (supports hybrid full-text and vector search).

## 2. Create an ingestion pipeline

1. Go to the **Agent** page.
2. Click **Create agent** and start from a blank canvas or a pre-built template (recommended for beginners).
3. On the canvas, drag and connect components from the right-side panel to design your flow (e.g., Parser → Chunker → Transformer → Indexer).

*Now let's build a typical ingestion pipeline!*

## 3. Configure Parser component

A **Parser** component converts your files into structured text while preserving layout, tables, headers, and other formatting. Its supported files 8 categories, 23+ formats including PDF, Image, Audio, Video, Email, Spreadsheet (Excel), Word, PPT, HTML, and Markdown. The following are some key configurations:

- For PDF files, choose one of the following: 
  - **DeepDoc** (Default): RAGFlow's built-in model. Best for scanned documents or complex layouts with tables.
  - **MinerU**: Industry-leading for complex elements like mathematical formulas and intricate layouts.
  - **Naive**: Simple text extraction. Use for clean, text-based PDFs without complex elements.
- For image files: Default uses OCR. Can also configure Vision Language Models (VLMs) for advanced visual understanding.
- For Email Files: Select specific fields to parse (e.g., "subject", "body") for precise extraction.
- For Spreadsheets: Outputs in HTML format, preserving row/column structure.
- For Word/PPT: Outputs in JSON format, retaining document hierarchy (titles, paragraphs, slides).
- For Text & Markup (HTML/MD): Automatically strips formatting tags, outputting clean text.

![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/parser1.png)
![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/parser2.png)

## 4. Configure Chunker component

The chunker component splits text intelligently. It's goal is to prevent AI context window overflow and improve semantic accuracy in hybrid search. There are two core methods (Can be used sequentially):

- By Tokens (Default):
  - Chunk Size: Default is 512 tokens. Balance between retrieval quality and model compatibility.
  - Overlap: Set **Overlapped percent** to duplicate end of one chunk into start of next. Improves semantic continuity.
  - Separators: Default uses `\n` (newlines) to split at natural paragraph boundaries first, avoiding mid-sentence cuts.
- By Title (Hierarchical):
  - Best for structured documents like manuals, papers, legal contracts.
  - System splits document by chapter/section structure. Each chunk represents a complete structural unit.

:::caution IMPORTANT
In the current design, if using both Token and Title methods, connect the **Token chunker** component first, then **Title chunker** component. Connecting **Title chunker** directly to **Parser** may cause format errors for Email, Image, Spreadsheet, and Text files.
:::

## 5. Configure Transformer component 

A **Transformer** component is designed to bridge the "Semantic Gap". Generally speaking, it uses AI models to add semantic metadata, making your content more discoverable during retrieval. It has four generation types:

- Summary: Create concise overviews.
- Keywords: Extract key terms.
- Questions: Generate questions each text chunk can answer.
- Metadata: Custom metadata extraction.

If you have multiple **Transformers**, ensure that you separate **Transformer** components for each function (e.g., one for Summary, another for Keywords).

The following are some key configurations:

- Model modes: (choose one)
  - Improvise: More creative, good for question generation.
  - Precise: Strictly faithful to text, good for Summary/Keyword extraction.
  - Balance: Middle ground for most scenarios.
- Prompt engineering: System prompts for each generation type are open and customizable.
- Connection: **Transformer** can connect after **Parser** (processes whole document) OR after **Chunker** (processes each chunk).
- Variable referencing: The node doesn't auto-acquire content. In the User prompt, manually reference upstream variables by typing `/` and selecting the specific output (e.g., `/{Parser.output}` or `/{Chunker.output}`).
- Series connection: When chaining **Transformers**, the second **Transformer** component will process the output of the first (e.g., generate Keywords from a Summary) if variables are correctly referenced.

![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/transformer1.png)
![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/transformer2.png)
![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/transformer3.png)

## 6. Configure Indexer component

The **Indexer** component indexes for optimal retrieval. It is the final step writes processed data to the search engine (such as Infinity, Elasticsearch, OpenSearch). The following are some key configurations:

- Search methods:
  - Full-text: Keyword search for exact matches (codes, names).
  - Embedding: Semantic search using vector similarity.
  - Hybrid (Recommended): Both methods combined for best recall.
- Retrieval Strategy:
  - Processed text (Default): Indexes the chunked text.
  - Questions: Indexes generated questions. Often yields higher similarity matching than text-to-text.
  - Augmented context: Indexes summaries instead of raw text. Good for broad topic matching.
- Filename weight: Slider to include document filename as semantic information in retrieval.
- Embedding model: Automatically uses the model set when creating the dataset.

![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/indexer.png)

:::caution IMPORTANT
To search across multiple datasets simultaneously, all selected datasets must use the same embedding model.
:::

## 7. Test run

Click **Run** on your pipeline canvas to upload a sample file and see the step-by-step results.

## 8. Connect pipeline to a dataset

1. When creating or editing a dataset, find the **Ingestion pipeline** section.
2. Click **Choose pipeline** and select your saved pipeline.

![](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/dataset_ingestion_settings.png)

*Now, any files uploaded to this dataset will be processed by your custom pipeline.*

