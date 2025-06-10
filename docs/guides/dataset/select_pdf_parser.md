---
sidebar_position: 1
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
- To use a third-party visual model for parsing PDFs, ensure you have set a default img2txt model under **Set default models** on the **Model providers** page.

## Procedure

1. On your knowledge base's **Configuration** page, select a chunking method, say **General**.

   _The **PDF parser** dropdown menu appears._

2. Select the option that works best with your scenario:

  - DeepDoc: (Default) The default visual model performing OCR, TSR, and DLR tasks on PDFs, which can be time-consuming.
  - Naive: Skip OCR, TSR, and DLR tasks if *all* your PDFs are plain text.
  - A third-party visual model provided by a specific model provider.

:::caution WARNING
Third-party visual models are marked **Experimental**, because we have not fully tested these models for the aforementioned data extraction tasks.
:::

## Frequently asked questions

### When should I select DeepDoc or a third-party visual model as the PDF parser?

Use a visual model to extract data if your PDFs contain formatted or image-based text rather than plain text. DeepDoc is the default visual model but can be time-consuming. You can also choose a lightweight or high-performance img2txt model depending on your needs and hardware capabilities.

### Can I select a visual model to parse my DOCX files?

No, you cannot. This dropdown menu is for PDFs only. To use this feature, convert your DOCX files to PDF first.

