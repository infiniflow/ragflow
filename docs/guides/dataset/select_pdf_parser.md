---
sidebar_position: -4
slug: /select_pdf_parser
---

# Select PDF parser

Select a visual model for parsing your PDFs.

---

RAGFlow isn't one-size-fits-all. It is built for flexibility and supports deeper customization to accommodate more complex use cases. From v0.17.0 onwards, RAGFlow decouples DeepDoc-specific data extraction tasks from chunking methods **for PDF files**. This separation enables you to autonomously select a visual model for OCR (Optical Character Recognition), TSR (Table Structure Recognition), and DLR (Document Layout Recognition) tasks that balances speed and performance to suit your specific use cases. If your PDFs contain only plain text, you can opt to skip these tasks by selecting the **Naive** option, to reduce the overall parsing time.

![data extraction](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/data_extraction.jpg)

## Prerequisites

- The PDF parser dropdown menu appears only when you select a chunking method compatible with PDFs, including:
    - **General**
    - **Manual**
    - **Paper**
    - **Book**
    - **Laws**
    - **Presentation**
    - **One**
- To use a third-party visual model for parsing PDFs, ensure you have set a default VLM under **Set default models** on the **Model providers** page.

## Quickstart

1. On your dataset's **Configuration** page, select a chunking method, say **General**.

   _The **PDF parser** dropdown menu appears._

2. Select the option that works best with your scenario:

  - DeepDoc: (Default) The default visual model performing OCR, TSR, and DLR tasks on PDFs, but can be time-consuming.
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
4. In the web UI, navigate to the **Configuration** page of your dataset. Click **Built-in** in the **Ingestion pipeline** section, select a chunking method from the **Built-in** dropdown, which supports PDF parsing, and select **MinerU** in **PDF parser**.
5. If you use a custom ingestion pipeline instead, you must also complete the first three steps before selecting **MinerU** in the **Parsing method** section of the **Parser** component.
:::

:::caution WARNING
Third-party visual models are marked **Experimental**, because we have not fully tested these models for the aforementioned data extraction tasks.
:::

## Frequently asked questions

### When should I select DeepDoc or a third-party visual model as the PDF parser?

Use a visual model to extract data if your PDFs contain formatted or image-based text rather than plain text. DeepDoc is the default visual model but can be time-consuming. You can also choose a lightweight or high-performance VLM depending on your needs and hardware capabilities.

### Can I select a visual model to parse my DOCX files?

No, you cannot. This dropdown menu is for PDFs only. To use this feature, convert your DOCX files to PDF first.

