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

### Tenant SoMark and OpenDataLoader models

The Go Parser can use an enrolled **SoMark** or **OpenDataLoader** OCR model
by tenant-model ID or by `model@instance@provider` reference, for example:

```json
{
  "pdf": {
    "parse_method": "SoMark-model@production@SoMark"
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

### Other file types

For image files:

- **OCR** is on by default. **OCR model** starts with **DeepDoc** for local
  OCR. Select an added **PaddleOCR** or **PaddleOCR.local** model to use
  that tenant model's instance instead; select **DeepDoc** again to return to
  local OCR.
- If the model list cannot be loaded, the field displays an error and preserves
  the saved selection. A successful empty list is shown separately; **DeepDoc**
  remains available in either case.
- Turning **OCR** off skips both local and selected-model OCR without clearing
  the saved selection. **Enable vision enhancement** independently controls VLM
  descriptions: selecting an OCR model never enables vision enhancement.
- SVG files have their document text read directly; they are not submitted to
  PaddleOCR or a VLM.
- Image attachments and extracted text continue through the picture pipeline's
  OneChunker and Tokenizer. An unavailable, disabled, inaccessible, or failing
  selected model is reported in Parser warnings, without falling back to another
  OCR instance. An image with no OCR text or vision description has no searchable
  text and is discarded by the Tokenizer.

For Go Parser configuration, the image family stores `ocr_enabled` and optional
`ocr_model_id` (an enrolled tenant model ID, not a provider name). An omitted or
empty `ocr_model_id` keeps local OCR. The selected model must be an active OCR
model on an active PaddleOCR instance belonging to the pipeline's tenant.
Self-hosted instances need a configured base URL. A cloud instance without one
uses the public catalog endpoint and logs a warning with tenant, model and
instance IDs, not credentials. Server-wide endpoint environment variables do not
replace an explicitly selected instance.

For email files:

- Select specific fields to parse, such as `subject` and `body`, for precise extraction.

For spreadsheets:

- Output in HTML format, preserving row and column structure.

For Word/PPT:

- Output in JSON format, preserving document hierarchy, such as headings, paragraphs and slides.

For text and markup (HTML/MD):

- Formatting tags are automatically removed and clean text is output.
