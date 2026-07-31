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

It supports 8 file categories and more than 23 formats, including PDF, images, audio, video, email, spreadsheets (Excel), Word, PPT, HTML and Markdown.

Key configurations:

For PDF files, select one of the following:

- **DeepDoc** (default): RAGFlow's built-in model. Best suited for scanned documents or complex layouts with tables.
- **MinerU**: Industry-leading for complex elements such as mathematical formulas and complex layouts.
- **Naive**: Simple text extraction. Use it for clean, text-based PDFs without complex elements.

For image files:

- OCR is used by default.
- You can also configure a vision language model (VLM) for advanced visual understanding.

For email files:

- Select specific fields to parse, such as `subject` and `body`, for precise extraction.

For spreadsheets:

- Output in HTML format, preserving row and column structure.

For Word/PPT:

- Output in JSON format, preserving document hierarchy, such as headings, paragraphs and slides.

For text and markup (HTML/MD):

- Formatting tags are automatically removed and clean text is output.

![Configure The Parser Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/configure_the_parser_component_1.jpg)

![Configure The Parser Component](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/configure_the_parser_component_2.jpg)
