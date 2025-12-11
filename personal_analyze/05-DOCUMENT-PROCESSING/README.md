# 05-DOCUMENT-PROCESSING - Document Parsing Pipeline

## Tong Quan

Document Processing pipeline chuyển đổi raw documents thành searchable chunks với layout analysis, OCR, và intelligent chunking.

## Kien Truc Document Processing

```
┌─────────────────────────────────────────────────────────────────┐
│                 DOCUMENT PROCESSING PIPELINE                     │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   File Upload   │────▶│  Task Creation  │────▶│  Task Queue     │
│   (MinIO)       │     │   (MySQL)       │     │   (Redis)       │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                                                        │
                                                        ▼
┌─────────────────────────────────────────────────────────────────┐
│                     TASK EXECUTOR                                │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  1. Download file from MinIO                             │   │
│  │  2. Select parser based on file type                     │   │
│  │  3. Execute parsing pipeline                             │   │
│  │  4. Generate embeddings                                  │   │
│  │  5. Index in Elasticsearch                               │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        │                     │                     │
        ▼                     ▼                     ▼
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│   PDF Parser    │  │  Office Parser  │  │  Text Parser    │
│                 │  │                 │  │                 │
│ - Layout detect │  │ - DOCX/XLSX    │  │ - TXT/MD/CSV   │
│ - OCR          │  │ - Table extract │  │ - Direct chunk  │
│ - Table struct  │  │ - Image embed   │  │                 │
└─────────────────┘  └─────────────────┘  └─────────────────┘
```

## Cau Truc Thu Muc

```
/rag/
├── svr/
│   └── task_executor.py    # Main task executor ⭐
├── app/
│   ├── naive.py           # Document parsing logic
│   ├── paper.py           # Academic paper parser
│   ├── qa.py              # Q&A document parser
│   └── table.py           # Structured table parser
├── flow/
│   ├── parser/            # Document parsers
│   ├── splitter/          # Chunking logic
│   └── tokenizer/         # Tokenization
└── nlp/
    └── __init__.py        # naive_merge() chunking

/deepdoc/
├── parser/
│   └── pdf_parser.py      # RAGFlow PDF parser ⭐
├── vision/
│   ├── ocr.py            # PaddleOCR integration
│   ├── layout_recognizer.py  # Detectron2 layout
│   └── table_structure_recognizer.py  # TSR
└── images/
    └── ...               # Image processing
```

## Files Trong Module Nay

| File | Mo Ta |
|------|-------|
| [task_executor_analysis.md](./task_executor_analysis.md) | Task execution pipeline |
| [pdf_parsing.md](./pdf_parsing.md) | PDF parsing với layout analysis |
| [ocr_pipeline.md](./ocr_pipeline.md) | OCR với PaddleOCR |
| [layout_detection.md](./layout_detection.md) | Detectron2 layout recognition |
| [table_extraction.md](./table_extraction.md) | Table structure recognition |
| [file_type_handlers.md](./file_type_handlers.md) | Handler cho từng file type |

## Processing Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                    PDF PROCESSING PIPELINE                       │
└─────────────────────────────────────────────────────────────────┘

                    PDF Binary Input
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│  1. IMAGE EXTRACTION (0-40%)                                     │
│     pdfplumber → PIL Images (3x zoom)                           │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  2. OCR DETECTION (40-63%)                                       │
│     PaddleOCR → Bounding boxes + Text                           │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  3. LAYOUT RECOGNITION (63-83%)                                  │
│     Detectron2 → Layout types (Text, Title, Table, Figure)      │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  4. TABLE STRUCTURE (TSR)                                        │
│     TableTransformer → Rows, Columns, Cells                     │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  5. TEXT MERGING                                                 │
│     ML-based vertical merge (XGBoost)                           │
│     Column detection (KMeans)                                   │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  6. CHUNKING                                                     │
│     naive_merge() → Token-based chunks                          │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  7. EMBEDDING + INDEXING                                         │
│     Vector generation → Elasticsearch                           │
└─────────────────────────────────────────────────────────────────┘
```

## Supported File Types

| Category | Extensions | Parser |
|----------|------------|--------|
| **PDF** | .pdf | RAGFlowPdfParser, PlainParser, VisionParser |
| **Office** | .docx, .xlsx, .pptx | python-docx, openpyxl |
| **Text** | .txt, .md, .csv | Direct reading |
| **Images** | .jpg, .png, .tiff | Vision LLM |
| **Email** | .eml | Email parser |
| **Web** | .html | Beautiful Soup |

## Layout Types Detected

| Type | Description |
|------|-------------|
| Text | Regular body text |
| Title | Section/document titles |
| Figure | Images and diagrams |
| Figure caption | Figure descriptions |
| Table | Data tables |
| Table caption | Table descriptions |
| Header | Page headers |
| Footer | Page footers |
| Reference | Bibliography |
| Equation | Mathematical equations |

## Key Algorithms

### Text Merging (XGBoost)
```
Features:
- Y-distance normalized by char height
- Same layout number
- Ending punctuation patterns
- Beginning character patterns
- Chinese numbering patterns

Output: Merge probability → threshold decision
```

### Column Detection (KMeans)
```
Input: X-coordinates of text boxes
Output: Optimal column assignments

Algorithm:
1. For k = 1 to max_columns:
   - Fit KMeans(k)
   - Calculate silhouette_score
2. Select k with best score
3. Assign boxes to columns
```

## Configuration

```python
parser_config = {
    "chunk_token_num": 512,           # Tokens per chunk
    "delimiter": "\n。；！？",         # Chunk boundaries
    "layout_recognize": "DeepDOC",    # Layout method
    "task_page_size": 12,             # Pages per task
}

# Task executor config
MAX_CONCURRENT_TASKS = 5
EMBEDDING_BATCH_SIZE = 16
DOC_BULK_SIZE = 64
```

## Related Files

- `/rag/svr/task_executor.py` - Main executor
- `/deepdoc/parser/pdf_parser.py` - PDF parsing
- `/deepdoc/vision/ocr.py` - OCR engine
- `/rag/nlp/__init__.py` - Chunking algorithms
