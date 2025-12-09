---
sidebar_position: 30
slug: /parser_component
---

# Parser component

A component that sets the parsing rules for your dataset.

---

A **Parser** component is autopopulated on the ingestion pipeline canvas and required in all ingestion pipeline workflows. Just like the **Extract** stage in the traditional ETL process, a **Parser** component in an ingestion pipeline defines how various file types are parsed into structured data. Click the component to display its configuration panel. In this configuration panel, you set the parsing rules for various file types.

## Configurations

Within the configuration panel, you can add multiple parsers and set the corresponding parsing rules or remove unwanted parsers. Please ensure your set of parsers covers all required file types; otherwise, an error would occur when you select this ingestion pipeline on your dataset's **Files** page.

The **Parser** component supports parsing the following file types:

| File type     | File format              |
| ------------- | ------------------------ |
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
MinerU PDF document parsing is available starting from v0.22.0. RAGFlow supports MinerU (>= 2.6.3) as an optional PDF parser with multiple backends. RAGFlow acts only as a client for MinerU, calling it to parse documents, reading the output files, and ingesting the parsed content. To use this feature, follow these steps:

1. Prepare MinerU:

   - **If you deploy RAGFlow from source**, install MinerU into an isolated virtual environment (recommended path: `$HOME/uv_tools`):

   ```bash
   mkdir -p "$HOME/uv_tools"
   cd "$HOME/uv_tools"
   uv venv .venv
   source .venv/bin/activate
   uv pip install -U "mineru[core]" -i https://mirrors.aliyun.com/pypi/simple
   # or
   # uv pip install -U "mineru[all]" -i https://mirrors.aliyun.com/pypi/simple
   ```

   - **If you deploy RAGFlow with Docker**, you usually only need to turn on MinerU support in `docker/.env`:

   ```bash
   # docker/.env
   ...
   USE_MINERU=true
   ...
   ```

   Enabling `USE_MINERU=true` will internally perform the same setup as the manual configuration (including setting the MinerU executable path and related environment variables). You only need the manual installation above if you are running from source or want full control over the MinerU installation.

2. Start RAGFlow with MinerU enabled:

   - **Source deployment** – in the RAGFlow repo, export the key MinerU-related variables and start the backend service:

   ```bash
   # in RAGFlow repo
   export MINERU_EXECUTABLE="$HOME/uv_tools/.venv/bin/mineru"
   export MINERU_DELETE_OUTPUT=0   # keep output directory
   export MINERU_BACKEND=pipeline  # or another backend you prefer

   source .venv/bin/activate
   export PYTHONPATH=$(pwd)
   bash docker/launch_backend_service.sh
   ```

   - **Docker deployment** – after setting `USE_MINERU=true`, restart the containers so that the new settings take effect:

   ```bash
   # in RAGFlow repo
   docker compose -f docker/docker-compose.yml restart
   ```

3. Restart the ragflow-server.
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

| Variable name | Type                     |
| ------------- | ------------------------ |
| `markdown`    | `string`                 |
| `text`        | `string`                 |
| `html`        | `string`                 |
| `json`        | `Array<Object>`          |
