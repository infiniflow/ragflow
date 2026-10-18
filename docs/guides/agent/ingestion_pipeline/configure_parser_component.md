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

![Select parser file format](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/configure_the_parser_component_2.jpg)

Key configurations:

For PDF files, select one of the following:

- **DeepDoc** (default): RAGFlow's built-in model. Best suited for scanned documents or complex layouts with tables.
- **MinerU**: Industry-leading for complex elements such as mathematical formulas and complex layouts.
- **Naive**: Simple text extraction. Use it for clean, text-based PDFs without complex elements.

![Configure PDF parser method](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/configure_the_parser_component_1.jpg)

### MinerU formula and table controls

When a MinerU model is selected for PDF, **Formula recognition** and **Table
recognition** are saved on that Parser component's PDF setup as
`mineru_formula_enable` and `mineru_table_enable`. Both default to enabled when
omitted; an explicit `false` requests disabling the corresponding control,
subject to the API and backend capabilities below.

- **`/file_parse`, pipeline and VLM backends:** RAGFlow forwards both controls
  as the `formula_enable` and `table_enable` multipart fields.
- **`/file_parse`, hybrid backends:** Table recognition follows the table
  control. The formula control applies to **inline formulas only**; MinerU
  still recognizes block formulas.
- **V1 upload/job API:** This protocol has no formula/table switches. Omitted
  or enabled controls keep the existing job request. Disabling either control
  fails with an explicit error before a file is uploaded or a job is created;
  use a `/file_parse` server when these controls are required.

For example, the component-scoped PDF configuration below disables formulas
while retaining table recognition:

```json
{
  "pdf": {
    "parse_method": "<tenant MinerU model ID>",
    "mineru_formula_enable": false,
    "mineru_table_enable": true
  }
}
```

### Other file types

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
