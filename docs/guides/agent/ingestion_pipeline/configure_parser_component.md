---
sidebar_position: 3
title: Configure the Parser Component
sidebar_label: Configure the Parser Component
slug: /configure_parser_component
sidebar_custom_props: {
  categoryIcon: RagAiAgent
}
---

# Configure the Parser Component

The **Parser** component converts your files into structured text while preserving layout, tables, headers and other formatting.

The initial configuration lists 12 file categories: **PDF**, **Spreadsheet**, **Image**, **Email**, **Markdown**, **Text & Code**, **HTML**, **DOC**, **DOCX**, **PPTX**, **Video**, and **Audio**. Configure the options for each required category.

Key configurations:

For PDF files, select one of the following:

- **DeepDoc** (default): RAGFlow's built-in model. Use for scanned documents or complex layouts with tables.
- **MinerU**: Parses mathematical formulas, tables, and complex page layouts.
- **Naive**: Simple text extraction. Use it for clean, text-based PDFs without complex elements.

![Configure PDF parser method](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/configure_the_parser_component_1.jpg)

### MinerU formula and table controls

When using MinerU for PDF, **Formula recognition** and **Table recognition** are enabled by default. Turn off the corresponding option when it is not needed.

Available controls depend on the connected MinerU service. Some services continue to recognize block formulas even when formula recognition is disabled. If parsing reports that an option is unsupported, ask an administrator to check the service configuration.

### SoMark and OpenDataLoader models

To use **SoMark** or **OpenDataLoader**, first add and enable the OCR model in model settings, then select it in the PDF parser options. The selected model must be available to your account.

If the model is unavailable or its connection settings are incomplete, parsing fails rather than switching to a different model. Check model access and ask an administrator to verify the connection.

### Other file types

For image files:

- **OCR** is enabled by default. To turn it off, first enable **Enable vision enhancement**. Keep at least one of these options enabled so that images can produce searchable text.
- **OCR model** selects local DeepDOC or an available configured PaddleOCR model. The model must be available to the canvas owner.
- **Enable vision enhancement** separately controls image descriptions. Enabling OCR does not enable vision enhancement.
- SVG text is read directly.
- Check parsing warnings if text recognition fails. A configuration with both OCR and vision enhancement disabled is rejected.

For email files:

- Select specific fields to parse, such as `subject` and `body`, for precise extraction.

![Select email fields to parse](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/configure_the_parser_component_2.jpg)

For spreadsheets:

- Preserves table content and row/column structure in the parsed items.

For Word/PPT:

- Preserves document structure, such as headings, paragraphs, and slides, in the parsed items.

For text and markup (HTML/MD):

- Extracts document content as structured items for downstream chunking.


### Output

Select the `json` output to use the parsed content. It contains structured items such as paragraphs and tables. Connect **Token Chunker** or **Title Chunker** after Parser to split this content into `chunks` for Transformer, Compiler, or Indexer.
