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

### Tenant SoMark and OpenDataLoader models

The Go Parser can use an enrolled **SoMark** or **OpenDataLoader** OCR model
by tenant-model ID or by `model@instance@provider` reference, for example:

```json
{
  "pdf": {
    "parse_method": "SoMark-model@production@SoMark",
    "output_format": "json"
  }
}
```

`layout_recognizer` also accepts a reference. Selection follows the existing PDF
model rule: a model reference in `parse_method` takes precedence; otherwise the
reference in `layout_recognizer` is used.

The selected model and instance must be active, OCR-capable and owned by the
current tenant/provider. The Parser resolves the instance endpoint and key on
each execution, so credential changes apply without resaving the pipeline.
SoMark supports ordinary instance key/base-URL storage; OpenDataLoader also
supports its provider-card JSON configuration, including nested `api_key` maps.

For an explicit reference, a missing endpoint, inaccessible/disabled model or
invalid instance configuration is an error. The Parser never substitutes another
instance, a global environment credential or a generic VLM. An empty optional
key remains empty. Transport SSRF protections still apply.

SoMark's non-secret per-model format/feature settings supply defaults; explicit
pipeline options override them. An explicit `false` is retained. Resolved
credentials stay on the per-run parser object, not in saved pipeline settings.

Bare engine names (`somark`, `opendataloader`) continue to use their existing
parser-specific setup/environment configuration.

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
