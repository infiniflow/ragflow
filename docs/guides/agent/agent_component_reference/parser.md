---
sidebar_position: 30
slug: /parser_component
sidebar_custom_props: {
  categoryIcon: LucideFilePlay
}
---
# Parser component

A component that sets the parsing rules for your dataset.

---

A **Parser** component is autopopulated on the ingestion pipeline canvas and required in all ingestion pipeline workflows. Just like the **Extract** stage in the traditional ETL process, a **Parser** component in an ingestion pipeline defines how various file types are parsed into structured data. Click the component to display its configuration panel. In this configuration panel, you set the parsing rules for various file types.

## Configurations

Within the configuration panel, you can add multiple parsers and set the corresponding parsing rules or remove unwanted parsers. Please ensure your set of parsers covers all required file types; otherwise, an error would occur when you select this ingestion pipeline on your dataset's **Files** page.

The **Parser** component supports parsing the following file types:

| File type     | File format              |
|---------------|--------------------------|
| PDF           | PDF                      |
| Spreadsheet   | XLSX, XLS, CSV           |
| Image         | PNG, JPG, JPEG, GIF, TIF |
| Email         | EML                      |
| Text & Markup | TXT, MD, MDX, HTML, JSON |
| Word          | DOCX                     |
| PowerPoint    | PPTX, PPT                |
| Audio         | MP3, WAV                 |
| Video         | MP4, AVI, MKV            |

### PDF parser

The output of a PDF parser is `json`. In the PDF parser, you select the parsing method that works best with your PDFs.

- DeepDoc: (Default) The default visual model performing OCR, TSR, and DLR tasks on complex PDFs, but can be time-consuming.
- Naive: Skip OCR, TSR, and DLR tasks if *all* your PDFs are plain text.
- [MinerU](https://github.com/opendatalab/MinerU): (Experimental) An open-source tool that converts PDF into machine-readable formats.
- [Docling](https://github.com/docling-project/docling): (Experimental) An open-source document processing tool for gen AI.
- A third-party visual model from a specific model provider.

:::danger IMPORTANT
Starting from v0.22.0, RAGFlow includes MinerU (&ge; 2.6.3) as an optional PDF parser of multiple backends. Please note that RAGFlow acts only as a *remote client* for MinerU, calling the MinerU API to parse documents and reading the returned files. To use this feature:
:::

1. Prepare a reachable MinerU API service (FastAPI server).
2. In the **.env** file or from the **Model providers** page in the UI, configure RAGFlow as a remote client to MinerU:
   - `MINERU_APISERVER`: The MinerU API endpoint (e.g., `http://mineru-host:8886`).
   - `MINERU_BACKEND`: The MinerU backend:
      - `"pipeline"` (default)
      - `"vlm-http-client"`
      - `"vlm-transformers"`
      - `"vlm-vllm-engine"`
      - `"vlm-mlx-engine"`
      - `"vlm-vllm-async-engine"`
      - `"vlm-lmdeploy-engine"`.
   - `MINERU_SERVER_URL`: (optional) The downstream vLLM HTTP server (e.g., `http://vllm-host:30000`). Applicable when `MINERU_BACKEND` is set to `"vlm-http-client"`. 
   - `MINERU_OUTPUT_DIR`: (optional) The local directory for holding the outputs of the MinerU API service (zip/JSON) before ingestion.
   - `MINERU_DELETE_OUTPUT`: Whether to delete temporary output when a temporary directory is used:
     - `1`: Delete.
     - `0`: Retain.
3. In the web UI, navigate to your dataset's **Configuration** page and find the **Ingestion pipeline** section:  
   - If you decide to use a chunking method from the **Built-in** dropdown, ensure it supports PDF parsing, then select **MinerU** from the **PDF parser** dropdown.
   - If you use a custom ingestion pipeline instead, select **MinerU** in the **PDF parser** section of the **Parser** component.

:::note
All MinerU environment variables are optional. When set, these values are used to auto-provision a MinerU OCR model for the tenant on first use. To avoid auto-provisioning, skip the environment variable settings and only configure MinerU from the **Model providers** page in the UI.
:::

:::caution WARNING
Third-party visual models are marked **Experimental**, because we have not fully tested these models for the aforementioned data extraction tasks.
:::

### Spreadsheet parser

A spreadsheet parser outputs `html`, preserving the original layout and table structure. You may remove this parser if your dataset contains no spreadsheets.

### Image parser

An Image parser uses a native OCR model for text extraction by default. You may select an alternative VLM model, provided that you have properly configured it on the **Model provider** page.

### Email parser

With the Email parser, you select the fields to parse from Emails, such as **subject** and **body**. The parser will then extract text from these specified fields.

### Text&Markup parser

A Text&Markup parser automatically removes all formatting tags (e.g., those from HTML and Markdown files) to output clean, plain text only.

### Word parser

A Word parser outputs `json`, preserving the original document structure information, including titles, paragraphs, tables, headers, and footers.

### PowerPoint (PPT) parser

A PowerPoint parser extracts content from PowerPoint files into `json`, processing each slide individually and distinguishing between its title, body text, and notes.

### Audio parser

An Audio parser transcribes audio files to text. To use this parser, you must first configure an ASR model on the **Model provider** page.

### Video parser

A Video parser transcribes video files to text. To use this parser, you must first configure a VLM model on the **Model provider** page.

## Output

The global variable names for the output of the **Parser** component, which can be referenced by subsequent components in the ingestion pipeline.

| Variable name | Type            |
|---------------|-----------------|
| `markdown`    | `string`        |
| `text`        | `string`        |
| `html`        | `string`        |
| `json`        | `Array<Object>` |
