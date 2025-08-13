# MonkeyOCR Integration Workflow Diagrams

## 🏗️ **Component Architecture Overview**

This diagram shows how files in `/api/db`, `/deepdoc`, and `/rag` directories work together for MonkeyOCR integration:

```mermaid
graph TD
    subgraph "📁 User Request"
        USER["User uploads document<br/>via Python SDK"]
    end
    
    subgraph "📁 /api/db - Database Layer"
        DBINIT["/api/db/__init__.py<br/>ParserType.MONKEY_OCR"]
        DBMODELS["/api/db/db_models.py<br/>Document & File models"]
        INITDATA["/api/db/init_data.py<br/>parser_ids registration"]
        
        subgraph "📁 /api/db/services"
            FILESVC["/api/db/services/file_service.py<br/>get_parser() method"]
            DOCSVC["/api/db/services/document_service.py<br/>Document operations"]
            KBSVC["/api/db/services/knowledgebase_service.py<br/>KB operations"]
        end
    end
    
    subgraph "📁 /rag - Processing Layer"
        RAGSETTINGS["/rag/settings.py<br/>Configuration"]
        
        subgraph "📁 /rag/app"
            RAGFACTORY["/rag/app/__init__.py<br/>FACTORY dictionary"]
            MONKEYPARSER["/rag/app/monkey_ocr_parser.py<br/>chunk() function"]
        end
        
        subgraph "📁 /rag/nlp"
            TOKENIZER["/rag/nlp/<br/>tokenize functions"]
        end
    end
    
    subgraph "📁 /deepdoc - Vision Layer"
        DEEPINIT["/deepdoc/__init__.py<br/>Module initialization"]
        
        subgraph "📁 /deepdoc/vision"
            VISIONINIT["/deepdoc/vision/__init__.py<br/>Export MonkeyOCR class"]
            MONKEYOCR["/deepdoc/vision/monkey_ocr.py<br/>MonkeyOCR class<br/>load→process→unload"]
        end
        
        subgraph "📁 /deepdoc/parser"
            PARSERS["/deepdoc/parser/<br/>Other document parsers"]
        end
    end
    
    subgraph "📦 MonkeyOCR Source"
        MONKEYSRC["/monkeyocr/<br/>Local MonkeyOCR source<br/>UNIPipe, OCRPipe"]
    end
    
    subgraph "💾 Storage"
        DB[(Database<br/>Documents & Chunks)]
        ES[(Elasticsearch<br/>Search Index)]
        STORAGE[(File Storage<br/>Binary files)]
    end

    %% Main workflow
    USER --> FILESVC
    FILESVC --> DBINIT
    FILESVC --> DBMODELS
    DBINIT --> INITDATA
    FILESVC --> RAGFACTORY
    
    RAGFACTORY --> MONKEYPARSER
    MONKEYPARSER --> VISIONINIT
    VISIONINIT --> MONKEYOCR
    MONKEYOCR --> MONKEYSRC
    
    MONKEYPARSER --> TOKENIZER
    MONKEYPARSER --> DOCSVC
    DOCSVC --> DBMODELS
    DOCSVC --> DB
    
    MONKEYPARSER --> ES
    FILESVC --> STORAGE
    
    KBSVC --> DB
    RAGSETTINGS --> MONKEYPARSER

    %% Data flow labels
    USER -.->|"1. upload file"| FILESVC
    FILESVC -.->|"2. determine parser"| DBINIT
    RAGFACTORY -.->|"3. route to parser"| MONKEYPARSER
    MONKEYPARSER -.->|"4. load MonkeyOCR"| MONKEYOCR
    MONKEYOCR -.->|"5. process document"| MONKEYSRC
    MONKEYPARSER -.->|"6. tokenize content"| TOKENIZER
    MONKEYPARSER -.->|"7. store chunks"| DB
    MONKEYPARSER -.->|"8. index for search"| ES

    %% Styling
    classDef userLayer fill:#e3f2fd,stroke:#1976d2,stroke-width:2px
    classDef dbLayer fill:#fff3e0,stroke:#f57c00,stroke-width:2px
    classDef ragLayer fill:#e8f5e8,stroke:#388e3c,stroke-width:2px
    classDef deepdocLayer fill:#fce4ec,stroke:#c2185b,stroke-width:2px
    classDef sourceLayer fill:#f3e5f5,stroke:#7b1fa2,stroke-width:2px
    classDef storageLayer fill:#fffde7,stroke:#689f38,stroke-width:2px

    class USER userLayer
    class DBINIT,DBMODELS,INITDATA,FILESVC,DOCSVC,KBSVC dbLayer
    class RAGSETTINGS,RAGFACTORY,MONKEYPARSER,TOKENIZER ragLayer
    class DEEPINIT,VISIONINIT,MONKEYOCR,PARSERS deepdocLayer
    class MONKEYSRC sourceLayer
    class DB,ES,STORAGE storageLayer
```

---

## 🔄 **Detailed Processing Sequence**

This sequence diagram shows the step-by-step interaction between specific files during MonkeyOCR document processing:

```mermaid
sequenceDiagram
    participant USER as User/SDK
    participant FILESVC as /api/db/services/<br/>file_service.py
    participant DBINIT as /api/db/__init__.py<br/>(ParserType)
    participant FACTORY as /rag/app/__init__.py<br/>(FACTORY)
    participant PARSER as /rag/app/<br/>monkey_ocr_parser.py
    participant VISION as /deepdoc/vision/<br/>monkey_ocr.py
    participant SOURCE as /monkeyocr/<br/>UNIPipe
    participant TOKENIZER as /rag/nlp/<br/>tokenize
    participant DOCSVC as /api/db/services/<br/>document_service.py
    participant DB as Database
    participant ES as Elasticsearch

    Note over USER,ES: MonkeyOCR Document Processing Workflow

    USER->>+FILESVC: upload_documents(["doc.pdf"])
    FILESVC->>FILESVC: store file in storage
    FILESVC->>+DBINIT: check ParserType.MONKEY_OCR
    DBINIT-->>-FILESVC: parser type available
    FILESVC->>FILESVC: get_parser(doc_type, "doc.pdf", "monkey_ocr")
    FILESVC-->>-USER: file uploaded, ready for parsing

    USER->>+FACTORY: parse_documents(doc_ids)
    FACTORY->>FACTORY: lookup parser for "monkey_ocr"
    FACTORY->>+PARSER: chunk(filename, binary, **kwargs)
    
    PARSER->>PARSER: callback(0.1, "Initializing MonkeyOCR...")
    PARSER->>+VISION: MonkeyOCR()
    VISION->>VISION: __init__(model_dir=None)
    VISION-->>-PARSER: MonkeyOCR instance
    
    PARSER->>+VISION: get_markdown_result(file_path, config)
    VISION->>VISION: load_model()
    VISION->>+SOURCE: UNIPipe(pdf_res_path, model_type)
    SOURCE-->>-VISION: model loaded
    VISION->>+SOURCE: pipe_analyze(file_path)
    SOURCE->>SOURCE: process document<br/>- structure recognition<br/>- content extraction<br/>- relationship detection
    SOURCE-->>-VISION: {'markdown': content, 'structure': info}
    VISION->>VISION: unload_model() + gc.collect()
    VISION-->>-PARSER: structured result

    PARSER->>PARSER: callback(0.5, "Converting to RAGFlow chunks...")
    PARSER->>PARSER: convert_to_ragflow_chunks(result, filename, lang)
    
    loop For each structure element
        PARSER->>+TOKENIZER: tokenize(doc, element_text, eng)
        TOKENIZER->>TOKENIZER: process content<br/>- create content_ltks<br/>- extract keywords<br/>- generate tokens
        TOKENIZER-->>-PARSER: tokenized content
        
        PARSER->>PARSER: create chunk with MonkeyOCR metadata<br/>img_id = JSON(structure_type, bbox, confidence)
    end
    
    PARSER->>PARSER: callback(0.8, "Storing chunks...")
    PARSER->>+DOCSVC: update document status
    DOCSVC->>+DB: UPDATE documents SET chunk_num, status, progress
    DB-->>-DOCSVC: updated
    DOCSVC-->>-PARSER: document updated
    
    loop For each chunk
        PARSER->>+DB: INSERT INTO chunks
        DB-->>-PARSER: chunk stored
        PARSER->>+ES: index chunk for search
        ES-->>-PARSER: indexed
    end
    
    PARSER->>PARSER: callback(1.0, "Processing complete")
    PARSER-->>-FACTORY: [chunk1, chunk2, ..., chunkN]
    FACTORY-->>-USER: processing completed

    Note over USER,ES: Chunks now available for RAG queries
```

---

## 📋 **File Interaction Summary**

### **1. 📁 `/api/db` Directory - Database Layer**

| File | Role | MonkeyOCR Integration |
|------|------|---------------------|
| `__init__.py` | Define parser types | ✏️ Add `MONKEY_OCR = "monkey_ocr"` |
| `init_data.py` | Register parsers | ✏️ Add `monkey_ocr:MonkeyOCR` to parser_ids |
| `db_models.py` | Database models | 📖 Reference Document/File models |
| `services/file_service.py` | File operations | ✏️ Add PDF/image detection in `get_parser()` |
| `services/document_service.py` | Document operations | 📖 Used to update document status |
| `services/knowledgebase_service.py` | KB operations | 📖 Used for knowledge base integration |

### **2. 📁 `/rag` Directory - Processing Layer**

| File | Role | MonkeyOCR Integration |
|------|------|---------------------|
| `app/__init__.py` | Parser factory | ✏️ Add MonkeyOCR to FACTORY dictionary |
| `app/monkey_ocr_parser.py` | Main parser logic | 🆕 Create new parser implementation |
| `nlp/` | Text processing | 📖 Used for tokenizing MonkeyOCR output |
| `settings.py` | Configuration | 📖 Reference for settings |

### **3. 📁 `/deepdoc` Directory - Vision Layer**

| File | Role | MonkeyOCR Integration |
|------|------|---------------------|
| `vision/__init__.py` | Vision module exports | ✏️ Export MonkeyOCR class |
| `vision/monkey_ocr.py` | MonkeyOCR wrapper | 🆕 Create new vision component |
| `parser/` | Document parsers | 📖 Reference other parser patterns |

### **4. 📦 **MonkeyOCR Source**

| Location | Role | Integration |
|----------|------|-------------|
| `/monkeyocr/` | Local source code | 🆕 Clone MonkeyOCR repository |
| `/monkeyocr/magic_pdf/pipe/` | Core processing | 📖 UNIPipe, OCRPipe classes |

---

## 🔄 **Data Flow Explanation**

### **Step 1: File Upload & Parser Selection**
```
User SDK → file_service.py → __init__.py (ParserType) → init_data.py (registration)
```

### **Step 2: Parser Routing**
```
FACTORY dictionary → monkey_ocr_parser.py → deepdoc/vision/monkey_ocr.py
```

### **Step 3: MonkeyOCR Processing**
```
MonkeyOCR class → load_model() → UNIPipe → pipe_analyze() → unload_model()
```

### **Step 4: Content Processing**
```
Structure extraction → tokenization → chunk creation → metadata embedding
```

### **Step 5: Storage**
```
Database (structured data) + Elasticsearch (search index) + File storage (binaries)
```

---

## 🎯 **Key Integration Points**

1. **Parser Registration**: `/api/db/__init__.py` + `/api/db/init_data.py`
2. **File Routing**: `/api/db/services/file_service.py`
3. **Processing Pipeline**: `/rag/app/__init__.py` → `/rag/app/monkey_ocr_parser.py`
4. **Vision Processing**: `/deepdoc/vision/monkey_ocr.py`
5. **Model Integration**: `/monkeyocr/` local source
6. **Data Storage**: Existing RAGFlow database schema

This workflow ensures MonkeyOCR integrates seamlessly with RAGFlow's existing architecture while maintaining the load→process→unload memory management strategy! 🚀 