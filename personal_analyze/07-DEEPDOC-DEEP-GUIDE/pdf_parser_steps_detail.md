# Chi Tiết Các Bước Xử Lý Khi Gọi RAGFlowPdfParser

## Mục Lục

1. [Tổng Quan Pipeline](#1-tổng-quan-pipeline)
2. [Step 1: Khởi Tạo Parser](#step-1-khởi-tạo-parser-__init__)
3. [Step 2: Load Images & OCR](#step-2-load-images--ocr-__images__)
4. [Step 3: Layout Recognition](#step-3-layout-recognition-_layouts_rec)
5. [Step 4: Table Structure Detection](#step-4-table-structure-detection-_table_transformer_job)
6. [Step 5: Column Detection](#step-5-column-detection-_assign_column)
7. [Step 6: Text Merge (Horizontal)](#step-6-text-merge-horizontal-_text_merge)
8. [Step 7: Text Merge (Vertical)](#step-7-text-merge-vertical-_naive_vertical_merge)
9. [Step 8: Filter & Cleanup](#step-8-filter--cleanup)
10. [Step 9: Extract Tables & Figures](#step-9-extract-tables--figures-_extract_table_figure)
11. [Step 10: Final Output](#step-10-final-output-__filterout_scraps)

---

## 1. Tổng Quan Pipeline

### 1.1 Entry Points

Có 2 entry points chính:

```python
# Entry Point 1: Simple call (Line 1160-1168)
def __call__(self, fnm, need_image=True, zoomin=3, return_html=False):
    self.__images__(fnm, zoomin)           # Step 2
    self._layouts_rec(zoomin)              # Step 3
    self._table_transformer_job(zoomin)    # Step 4
    self._text_merge()                     # Step 6
    self._concat_downward()                # Step 7 (disabled)
    self._filter_forpages()                # Step 8
    tbls = self._extract_table_figure(...) # Step 9
    return self.__filterout_scraps(...), tbls  # Step 10

# Entry Point 2: Detailed parsing (Line 1170-1252)
def parse_into_bboxes(self, fnm, callback=None, zoomin=3):
    # Same steps but with callbacks và more detailed output
```

### 1.2 Pipeline Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         PDF PARSING PIPELINE                                 │
└─────────────────────────────────────────────────────────────────────────────┘

PDF File (path/bytes)
         │
         ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STEP 1: __init__()                                                          │
│  • Load OCR model (DBNet + CRNN)                                            │
│  • Load LayoutRecognizer (YOLOv10)                                          │
│  • Load TableStructureRecognizer (YOLOv10)                                  │
│  • Load XGBoost model (text concatenation)                                  │
└─────────────────────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STEP 2: __images__()                                                        │
│  • Convert PDF pages to images (pdfplumber)                                 │
│  • Extract native PDF characters                                            │
│  • Run OCR detection + recognition                                          │
│  • Merge native chars with OCR boxes                                        │
│  Output: self.boxes[], self.page_images[], self.page_chars[]               │
└─────────────────────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STEP 3: _layouts_rec()                                                      │
│  • Run YOLOv10 on page images                                               │
│  • Detect 10 layout types (Text, Title, Table, Figure...)                   │
│  • Associate OCR boxes with layouts                                         │
│  • Filter garbage (headers, footers, page numbers)                          │
│  Output: boxes[] with layout_type, layoutno attributes                      │
└─────────────────────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STEP 4: _table_transformer_job()                                            │
│  • Crop table regions from images                                           │
│  • Run TableStructureRecognizer                                             │
│  • Detect rows, columns, headers, spanning cells                            │
│  • Tag boxes with R (row), C (column), H (header), SP (spanning)           │
│  Output: self.tb_cpns[], boxes[] with table attributes                      │
└─────────────────────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STEP 5: _assign_column() (called in _text_merge)                            │
│  • K-Means clustering on X coordinates                                      │
│  • Silhouette score to find optimal k (1-4 columns)                         │
│  • Assign col_id to each text box                                           │
│  Output: boxes[] with col_id attribute                                      │
└─────────────────────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STEP 6: _text_merge()                                                       │
│  • Horizontal merge: same line, same column, same layout                    │
│  Output: Fewer, wider text boxes                                            │
└─────────────────────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STEP 7: _naive_vertical_merge() / _concat_downward()                        │
│  • Vertical merge: adjacent paragraphs                                      │
│  • Semantic checks (punctuation, distance, overlap)                         │
│  Output: Merged paragraphs                                                  │
└─────────────────────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STEP 8: _filter_forpages()                                                  │
│  • Remove table of contents                                                 │
│  • Remove dirty pages (repetitive patterns)                                 │
│  Output: Cleaned boxes[]                                                    │
└─────────────────────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STEP 9: _extract_table_figure()                                             │
│  • Extract table boxes → construct HTML/descriptive                         │
│  • Extract figure boxes → crop images                                       │
│  • Associate captions with tables/figures                                   │
│  Output: tables[], figures[]                                                │
└─────────────────────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STEP 10: __filterout_scraps()                                               │
│  • Filter low-quality text blocks                                           │
│  • Add position tags                                                        │
│  • Format final output                                                      │
│  Output: (documents, tables)                                                │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Step 1: Khởi Tạo Parser (`__init__`)

**File**: `pdf_parser.py`
**Lines**: 52-105

### Code Analysis

```python
class RAGFlowPdfParser:
    def __init__(self, **kwargs):
        # ═══════════════════════════════════════════════════════════════════
        # 1. LOAD OCR MODEL
        # ═══════════════════════════════════════════════════════════════════
        self.ocr = OCR()  # Line 66
        # OCR class chứa:
        # - TextDetector (DBNet): Phát hiện vùng text
        # - TextRecognizer (CRNN): Nhận dạng text trong vùng

        # ═══════════════════════════════════════════════════════════════════
        # 2. SETUP PARALLEL PROCESSING
        # ═══════════════════════════════════════════════════════════════════
        self.parallel_limiter = None
        if settings.PARALLEL_DEVICES > 1:
            # Tạo capacity limiter cho mỗi GPU
            self.parallel_limiter = [
                trio.CapacityLimiter(1)  # 1 task per device
                for _ in range(settings.PARALLEL_DEVICES)
            ]

        # ═══════════════════════════════════════════════════════════════════
        # 3. LOAD LAYOUT RECOGNIZER
        # ═══════════════════════════════════════════════════════════════════
        layout_recognizer_type = os.getenv("LAYOUT_RECOGNIZER_TYPE", "onnx")

        if layout_recognizer_type == "ascend":
            self.layouter = AscendLayoutRecognizer(recognizer_domain)  # Huawei NPU
        else:
            self.layouter = LayoutRecognizer(recognizer_domain)  # ONNX (default)

        # ═══════════════════════════════════════════════════════════════════
        # 4. LOAD TABLE STRUCTURE RECOGNIZER
        # ═══════════════════════════════════════════════════════════════════
        self.tbl_det = TableStructureRecognizer()  # Line 86

        # ═══════════════════════════════════════════════════════════════════
        # 5. LOAD XGBOOST MODEL (Text Concatenation)
        # ═══════════════════════════════════════════════════════════════════
        self.updown_cnt_mdl = xgb.Booster()  # Line 88

        # Try GPU first
        try:
            import torch.cuda
            if torch.cuda.is_available():
                self.updown_cnt_mdl.set_param({"device": "cuda"})
        except:
            pass

        # Load model weights
        model_dir = os.path.join(get_project_base_directory(), "rag/res/deepdoc")
        self.updown_cnt_mdl.load_model(
            os.path.join(model_dir, "updown_concat_xgb.model")
        )

        # ═══════════════════════════════════════════════════════════════════
        # 6. INITIALIZE STATE
        # ═══════════════════════════════════════════════════════════════════
        self.page_from = 0
        self.column_num = 1
```

### Models Loaded

| Model | Type | Purpose | Size |
|-------|------|---------|------|
| OCR (DBNet) | ONNX | Text detection | ~30MB |
| OCR (CRNN) | ONNX | Text recognition | ~20MB |
| LayoutRecognizer | ONNX (YOLOv10) | Layout detection | ~50MB |
| TableStructureRecognizer | ONNX (YOLOv10) | Table structure | ~50MB |
| XGBoost | Binary | Text concatenation | ~5MB |

---

## Step 2: Load Images & OCR (`__images__`)

**File**: `pdf_parser.py`
**Lines**: 1042-1159

### 2.1 PDF to Images Conversion

```python
def __images__(self, fnm, zoomin=3, page_from=0, page_to=299, callback=None):
    # ═══════════════════════════════════════════════════════════════════
    # INITIALIZE STATE VARIABLES
    # ═══════════════════════════════════════════════════════════════════
    self.lefted_chars = []       # Characters không match với OCR box
    self.mean_height = []        # Average character height per page
    self.mean_width = []         # Average character width per page
    self.boxes = []              # OCR results
    self.garbages = {}           # Garbage patterns found
    self.page_cum_height = [0]   # Cumulative page heights
    self.page_layout = []        # Layout detection results
    self.page_from = page_from

    # ═══════════════════════════════════════════════════════════════════
    # CONVERT PDF PAGES TO IMAGES (Lines 1052-1067)
    # ═══════════════════════════════════════════════════════════════════
    with pdfplumber.open(fnm) as pdf:
        self.pdf = pdf

        # Convert each page to image
        # resolution = 72 * zoomin (default: 72 * 3 = 216 DPI)
        self.page_images = [
            p.to_image(resolution=72 * zoomin, antialias=True).annotated
            for p in pdf.pages[page_from:page_to]
        ]

        # ═══════════════════════════════════════════════════════════════
        # EXTRACT NATIVE PDF CHARACTERS (Lines 1058-1062)
        # ═══════════════════════════════════════════════════════════════
        # Extract character-level info from PDF text layer
        self.page_chars = [
            [c for c in page.dedupe_chars().chars if self._has_color(c)]
            for page in pdf.pages[page_from:page_to]
        ]

        self.total_page = len(pdf.pages)
```

### 2.2 Language Detection

```python
    # ═══════════════════════════════════════════════════════════════════
    # DETECT DOCUMENT LANGUAGE (Lines 1093-1100)
    # ═══════════════════════════════════════════════════════════════════
    # Sample random characters, check if English
    self.is_english = [
        re.search(r"[ a-zA-Z0-9,/¸;:'\[\]\(\)!@#$%^&*\"?<>._-]{30,}",
                  "".join(random.choices([c["text"] for c in self.page_chars[i]],
                                        k=min(100, len(self.page_chars[i])))))
        for i in range(len(self.page_chars))
    ]

    # If >50% pages are English, mark document as English
    if sum([1 if e else 0 for e in self.is_english]) > len(self.page_images) / 2:
        self.is_english = True
    else:
        self.is_english = False
```

### 2.3 OCR Processing (Parallel)

```python
    # ═══════════════════════════════════════════════════════════════════
    # ASYNC OCR PROCESSING (Lines 1102-1145)
    # ═══════════════════════════════════════════════════════════════════
    async def __img_ocr(i, device_id, img, chars, limiter):
        # Add spaces between characters if needed
        j = 0
        while j + 1 < len(chars):
            if (chars[j]["text"] and chars[j + 1]["text"]
                and re.match(r"[0-9a-zA-Z,.:;!%]+",
                            chars[j]["text"] + chars[j + 1]["text"])
                and chars[j + 1]["x0"] - chars[j]["x1"] >=
                    min(chars[j + 1]["width"], chars[j]["width"]) / 2):
                chars[j]["text"] += " "
            j += 1

        # Run OCR with rate limiting for parallel execution
        if limiter:
            async with limiter:
                await trio.to_thread.run_sync(
                    lambda: self.__ocr(i + 1, img, chars, zoomin, device_id)
                )
        else:
            self.__ocr(i + 1, img, chars, zoomin, device_id)

    # Launch OCR tasks
    async def __img_ocr_launcher():
        if self.parallel_limiter:
            # Parallel processing across multiple GPUs
            async with trio.open_nursery() as nursery:
                for i, img in enumerate(self.page_images):
                    chars = preprocess(i)
                    nursery.start_soon(
                        __img_ocr, i,
                        i % settings.PARALLEL_DEVICES,  # Round-robin GPU
                        img, chars,
                        self.parallel_limiter[i % settings.PARALLEL_DEVICES]
                    )
        else:
            # Sequential processing
            for i, img in enumerate(self.page_images):
                chars = preprocess(i)
                await __img_ocr(i, 0, img, chars, None)

    trio.run(__img_ocr_launcher)
```

### 2.4 OCR Core Function (`__ocr`)

```python
def __ocr(self, pagenum, img, chars, ZM=3, device_id=None):
    """
    Core OCR function for a single page.

    Lines: 282-345
    """
    # ═══════════════════════════════════════════════════════════════════
    # STEP 2.4.1: TEXT DETECTION
    # ═══════════════════════════════════════════════════════════════════
    bxs = self.ocr.detect(np.array(img), device_id)  # Line 284
    # Returns: [(box_points, (text_hint, confidence)), ...]

    if not bxs:
        self.boxes.append([])
        return

    # ═══════════════════════════════════════════════════════════════════
    # STEP 2.4.2: CONVERT TO BOX FORMAT
    # ═══════════════════════════════════════════════════════════════════
    bxs = [(line[0], line[1][0]) for line in bxs]
    bxs = Recognizer.sort_Y_firstly([
        {
            "x0": b[0][0] / ZM,
            "x1": b[1][0] / ZM,
            "top": b[0][1] / ZM,
            "bottom": b[-1][1] / ZM,
            "text": "",
            "txt": t,
            "chars": [],
            "page_number": pagenum
        }
        for b, t in bxs
        if b[0][0] <= b[1][0] and b[0][1] <= b[-1][1]
    ], self.mean_height[pagenum - 1] / 3)

    # ═══════════════════════════════════════════════════════════════════
    # STEP 2.4.3: MERGE NATIVE PDF CHARS WITH OCR BOXES
    # ═══════════════════════════════════════════════════════════════════
    for c in chars:
        # Find overlapping OCR box
        ii = Recognizer.find_overlapped(c, bxs)
        if ii is None:
            self.lefted_chars.append(c)
            continue

        # Check height compatibility (within 70% tolerance)
        ch = c["bottom"] - c["top"]
        bh = bxs[ii]["bottom"] - bxs[ii]["top"]
        if abs(ch - bh) / max(ch, bh) >= 0.7 and c["text"] != " ":
            self.lefted_chars.append(c)
            continue

        # Add character to box
        bxs[ii]["chars"].append(c)

    # ═══════════════════════════════════════════════════════════════════
    # STEP 2.4.4: RECONSTRUCT TEXT FROM CHARS
    # ═══════════════════════════════════════════════════════════════════
    for b in bxs:
        if not b["chars"]:
            del b["chars"]
            continue

        # Sort chars by Y position, then concatenate
        m_ht = np.mean([c["height"] for c in b["chars"]])
        for c in Recognizer.sort_Y_firstly(b["chars"], m_ht):
            if c["text"] == " " and b["text"]:
                if re.match(r"[0-9a-zA-Zа-яА-Я,.?;:!%%]", b["text"][-1]):
                    b["text"] += " "
            else:
                b["text"] += c["text"]
        del b["chars"]

    # ═══════════════════════════════════════════════════════════════════
    # STEP 2.4.5: OCR RECOGNITION FOR BOXES WITHOUT NATIVE TEXT
    # ═══════════════════════════════════════════════════════════════════
    boxes_to_reg = []
    img_np = np.array(img)
    for b in bxs:
        if not b["text"]:
            # Crop region for OCR
            left, right = b["x0"] * ZM, b["x1"] * ZM
            top, bott = b["top"] * ZM, b["bottom"] * ZM
            b["box_image"] = self.ocr.get_rotate_crop_image(
                img_np,
                np.array([[left, top], [right, top],
                         [right, bott], [left, bott]], dtype=np.float32)
            )
            boxes_to_reg.append(b)
        del b["txt"]

    # Batch recognition
    texts = self.ocr.recognize_batch(
        [b["box_image"] for b in boxes_to_reg],
        device_id
    )
    for i, b in enumerate(boxes_to_reg):
        b["text"] = texts[i]
        del b["box_image"]

    # Filter empty boxes
    bxs = [b for b in bxs if b["text"]]
    self.boxes.append(bxs)
```

### 2.5 Data Flow Diagram

```
PDF File
    │
    ├──────────────────────────────────────────────────────────────┐
    │                                                              │
    ▼                                                              ▼
┌─────────────────────────┐                          ┌─────────────────────────┐
│    pdfplumber.open()    │                          │     pdf.pages[i]        │
│                         │                          │    .to_image()          │
│  Extract text layer     │                          │                         │
│  (native characters)    │                          │  Resolution: 216 DPI    │
└───────────┬─────────────┘                          └───────────┬─────────────┘
            │                                                    │
            ▼                                                    ▼
    page_chars[]                                          page_images[]
    (Native PDF text)                                     (PIL Images)
            │                                                    │
            │                                                    │
            │                     ┌──────────────────────────────┘
            │                     │
            │                     ▼
            │         ┌─────────────────────────┐
            │         │   OCR Detection         │
            │         │   (DBNet)               │
            │         │                         │
            │         │   Input: page_image     │
            │         │   Output: bounding boxes│
            │         └───────────┬─────────────┘
            │                     │
            │                     ▼
            │         ┌─────────────────────────┐
            │         │   Box-Char Matching     │
            │         │                         │
            └────────▶│   Match native chars    │
                      │   to OCR boxes          │
                      │   (overlap detection)   │
                      └───────────┬─────────────┘
                                  │
                    ┌─────────────┴─────────────┐
                    │                           │
                    ▼                           ▼
            Boxes with text              Boxes without text
            (from native)                (need OCR recognition)
                    │                           │
                    │                           ▼
                    │               ┌─────────────────────────┐
                    │               │   OCR Recognition       │
                    │               │   (CRNN)                │
                    │               │                         │
                    │               │   Crop → Recognize      │
                    │               └───────────┬─────────────┘
                    │                           │
                    └─────────────┬─────────────┘
                                  │
                                  ▼
                          self.boxes[]
                          [{"x0", "x1", "top", "bottom", "text", "page_number"}, ...]
```

---

## Step 3: Layout Recognition (`_layouts_rec`)

**File**: `pdf_parser.py`
**Lines**: 347-353

### Code Analysis

```python
def _layouts_rec(self, ZM, drop=True):
    """
    Run layout recognition on all pages.

    Args:
        ZM: Zoom factor (default 3)
        drop: Whether to filter garbage layouts (headers, footers)
    """
    assert len(self.page_images) == len(self.boxes)

    # ═══════════════════════════════════════════════════════════════════
    # CALL LAYOUT RECOGNIZER
    # ═══════════════════════════════════════════════════════════════════
    # LayoutRecognizer.__call__() internally:
    # 1. Runs YOLOv10 on each page image
    # 2. Detects 10 layout types
    # 3. Associates OCR boxes with layouts
    # 4. Filters garbage if drop=True
    self.boxes, self.page_layout = self.layouter(
        self.page_images,  # List of page images
        self.boxes,        # List of OCR boxes per page (flattened after this)
        ZM,                # Zoom factor
        drop=drop          # Filter garbage
    )

    # ═══════════════════════════════════════════════════════════════════
    # ADD CUMULATIVE Y COORDINATES
    # ═══════════════════════════════════════════════════════════════════
    # After layouter, self.boxes is flattened (not per-page anymore)
    for i in range(len(self.boxes)):
        self.boxes[i]["top"] += self.page_cum_height[self.boxes[i]["page_number"] - 1]
        self.boxes[i]["bottom"] += self.page_cum_height[self.boxes[i]["page_number"] - 1]
```

### Layout Types

```python
# From layout_recognizer.py, lines 34-46
labels = [
    "_background_",     # 0: Ignored
    "Text",             # 1: Body text paragraphs
    "Title",            # 2: Section titles
    "Figure",           # 3: Images, charts, diagrams
    "Figure caption",   # 4: Text describing figures
    "Table",            # 5: Data tables
    "Table caption",    # 6: Text describing tables
    "Header",           # 7: Page headers
    "Footer",           # 8: Page footers
    "Reference",        # 9: Bibliography
    "Equation",         # 10: Mathematical formulas
]
```

### Box Attributes After Layout Recognition

```python
# Each box in self.boxes now has:
{
    "x0": float,           # Left edge
    "x1": float,           # Right edge
    "top": float,          # Top edge (cumulative)
    "bottom": float,       # Bottom edge (cumulative)
    "text": str,           # Recognized text
    "page_number": int,    # 1-indexed page number
    "layout_type": str,    # "text", "title", "table", "figure", etc.
    "layoutno": int,       # Layout region ID
}
```

---

## Step 4: Table Structure Detection (`_table_transformer_job`)

**File**: `pdf_parser.py`
**Lines**: 196-281

### Code Analysis

```python
def _table_transformer_job(self, ZM):
    """
    Detect table structure and tag boxes with R/C/H/SP attributes.
    """
    logging.debug("Table processing...")

    # ═══════════════════════════════════════════════════════════════════
    # STEP 4.1: EXTRACT TABLE REGIONS
    # ═══════════════════════════════════════════════════════════════════
    imgs, pos = [], []
    tbcnt = [0]
    MARGIN = 10
    self.tb_cpns = []

    for p, tbls in enumerate(self.page_layout):
        # Filter only table layouts
        tbls = [f for f in tbls if f["type"] == "table"]
        tbcnt.append(len(tbls))

        if not tbls:
            continue

        for tb in tbls:
            # Crop table region with margin
            left = tb["x0"] - MARGIN
            top = tb["top"] - MARGIN
            right = tb["x1"] + MARGIN
            bott = tb["bottom"] + MARGIN

            # Scale by zoom factor
            pos.append((left * ZM, top * ZM))
            imgs.append(self.page_images[p].crop((
                left * ZM, top * ZM,
                right * ZM, bott * ZM
            )))

    if not imgs:
        return

    # ═══════════════════════════════════════════════════════════════════
    # STEP 4.2: RUN TABLE STRUCTURE RECOGNIZER
    # ═══════════════════════════════════════════════════════════════════
    recos = self.tbl_det(imgs)  # Line 220
    # Returns per table: [{"label": "table row|column|header|spanning", "x0", "top", ...}, ...]

    # ═══════════════════════════════════════════════════════════════════
    # STEP 4.3: MAP COORDINATES BACK TO FULL PAGE
    # ═══════════════════════════════════════════════════════════════════
    tbcnt = np.cumsum(tbcnt)
    for i in range(len(tbcnt) - 1):  # For each page
        pg = []
        for j, tb_items in enumerate(recos[tbcnt[i]:tbcnt[i + 1]]):
            poss = pos[tbcnt[i]:tbcnt[i + 1]]
            for it in tb_items:
                # Add offset back
                it["x0"] += poss[j][0]
                it["x1"] += poss[j][0]
                it["top"] += poss[j][1]
                it["bottom"] += poss[j][1]

                # Scale back from zoom
                for n in ["x0", "x1", "top", "bottom"]:
                    it[n] /= ZM

                # Add cumulative height
                it["top"] += self.page_cum_height[i]
                it["bottom"] += self.page_cum_height[i]
                it["pn"] = i
                it["layoutno"] = j
                pg.append(it)
        self.tb_cpns.extend(pg)

    # ═══════════════════════════════════════════════════════════════════
    # STEP 4.4: GATHER COMPONENTS BY TYPE
    # ═══════════════════════════════════════════════════════════════════
    def gather(kwd, fzy=10, ption=0.6):
        eles = Recognizer.sort_Y_firstly(
            [r for r in self.tb_cpns if re.match(kwd, r["label"])],
            fzy
        )
        eles = Recognizer.layouts_cleanup(self.boxes, eles, 5, ption)
        return Recognizer.sort_Y_firstly(eles, 0)

    headers = gather(r".*header$")
    rows = gather(r".* (row|header)")
    spans = gather(r".*spanning")
    clmns = sorted(
        [r for r in self.tb_cpns if re.match(r"table column$", r["label"])],
        key=lambda x: (x["pn"], x["layoutno"], x["x0"])
    )
    clmns = Recognizer.layouts_cleanup(self.boxes, clmns, 5, 0.5)

    # ═══════════════════════════════════════════════════════════════════
    # STEP 4.5: TAG BOXES WITH TABLE ATTRIBUTES
    # ═══════════════════════════════════════════════════════════════════
    for b in self.boxes:
        if b.get("layout_type", "") != "table":
            continue

        # Find row (R)
        ii = Recognizer.find_overlapped_with_threshold(b, rows, thr=0.3)
        if ii is not None:
            b["R"] = ii
            b["R_top"] = rows[ii]["top"]
            b["R_bott"] = rows[ii]["bottom"]

        # Find header (H)
        ii = Recognizer.find_overlapped_with_threshold(b, headers, thr=0.3)
        if ii is not None:
            b["H"] = ii
            b["H_top"] = headers[ii]["top"]
            b["H_bott"] = headers[ii]["bottom"]
            b["H_left"] = headers[ii]["x0"]
            b["H_right"] = headers[ii]["x1"]

        # Find column (C)
        ii = Recognizer.find_horizontally_tightest_fit(b, clmns)
        if ii is not None:
            b["C"] = ii
            b["C_left"] = clmns[ii]["x0"]
            b["C_right"] = clmns[ii]["x1"]

        # Find spanning cell (SP)
        ii = Recognizer.find_overlapped_with_threshold(b, spans, thr=0.3)
        if ii is not None:
            b["SP"] = ii
            b["H_top"] = spans[ii]["top"]
            b["H_bott"] = spans[ii]["bottom"]
            b["H_left"] = spans[ii]["x0"]
            b["H_right"] = spans[ii]["x1"]
```

### Data Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    TABLE STRUCTURE DETECTION                                 │
└─────────────────────────────────────────────────────────────────────────────┘

page_layout[]  ───────────────────────────┐
(Table regions)                           │
                                          ▼
                              ┌─────────────────────────┐
                              │   Crop Table Regions    │
                              │   + MARGIN (10px)       │
                              └───────────┬─────────────┘
                                          │
                                          ▼
                              ┌─────────────────────────┐
                              │  TableStructureRec()    │
                              │  (YOLOv10)              │
                              │                         │
                              │  Detects:               │
                              │  • table row            │
                              │  • table column         │
                              │  • table column header  │
                              │  • table spanning cell  │
                              └───────────┬─────────────┘
                                          │
                                          ▼
                              ┌─────────────────────────┐
                              │  Tag OCR Boxes          │
                              │                         │
                              │  • R (row index)        │
                              │  • C (column index)     │
                              │  • H (header index)     │
                              │  • SP (spanning cell)   │
                              └─────────────────────────┘

After this step, table boxes have:
{
    "R": 0,           # Row index
    "R_top": 100,     # Row top boundary
    "R_bott": 150,    # Row bottom boundary
    "C": 1,           # Column index
    "C_left": 50,     # Column left boundary
    "C_right": 200,   # Column right boundary
    "H": 0,           # Header row index (if header)
    "SP": 2,          # Spanning cell index (if spanning)
}
```

---

## Step 5: Column Detection (`_assign_column`)

**File**: `pdf_parser.py`
**Lines**: 355-440

### Algorithm Overview

```
K-Means Column Detection:

1. Group boxes by page
2. For each page:
   a. Extract X0 coordinates
   b. Normalize indented text (within 12% page width)
   c. Try K from 1 to 4
   d. Select K with highest silhouette score
3. Use majority voting for global column count
4. Final clustering with selected K
5. Remap cluster IDs to left-to-right order
```

### Code Analysis

```python
def _assign_column(self, boxes, zoomin=3):
    """
    Detect number of columns using K-Means clustering.
    """
    if not boxes:
        return boxes
    if all("col_id" in b for b in boxes):
        return boxes

    # ═══════════════════════════════════════════════════════════════════
    # GROUP BOXES BY PAGE
    # ═══════════════════════════════════════════════════════════════════
    by_page = defaultdict(list)
    for b in boxes:
        by_page[b["page_number"]].append(b)

    page_cols = {}

    # ═══════════════════════════════════════════════════════════════════
    # FOR EACH PAGE: FIND OPTIMAL K
    # ═══════════════════════════════════════════════════════════════════
    for pg, bxs in by_page.items():
        if not bxs:
            page_cols[pg] = 1
            continue

        x0s_raw = np.array([b["x0"] for b in bxs], dtype=float)

        # Calculate page width
        min_x0 = np.min(x0s_raw)
        max_x1 = np.max([b["x1"] for b in bxs])
        width = max_x1 - min_x0

        # ═══════════════════════════════════════════════════════════════
        # INDENT TOLERANCE: Normalize near-left-edge text
        # ═══════════════════════════════════════════════════════════════
        INDENT_TOL = width * 0.12  # 12% of page width
        x0s = []
        for x in x0s_raw:
            if abs(x - min_x0) < INDENT_TOL:
                x0s.append([min_x0])  # Snap to left edge
            else:
                x0s.append([x])
        x0s = np.array(x0s, dtype=float)

        # ═══════════════════════════════════════════════════════════════
        # TRY K FROM 1 TO 4
        # ═══════════════════════════════════════════════════════════════
        max_try = min(4, len(bxs))
        if max_try < 2:
            max_try = 1

        best_k = 1
        best_score = -1

        for k in range(1, max_try + 1):
            km = KMeans(n_clusters=k, n_init="auto")
            labels = km.fit_predict(x0s)

            centers = np.sort(km.cluster_centers_.flatten())
            if len(centers) > 1:
                try:
                    score = silhouette_score(x0s, labels)
                except ValueError:
                    continue
            else:
                score = 0

            if score > best_score:
                best_score = score
                best_k = k

        page_cols[pg] = best_k
        logging.info(f"[Page {pg}] best_score={best_score:.2f}, best_k={best_k}")

    # ═══════════════════════════════════════════════════════════════════
    # MAJORITY VOTING FOR GLOBAL COLUMN COUNT
    # ═══════════════════════════════════════════════════════════════════
    global_cols = Counter(page_cols.values()).most_common(1)[0][0]
    logging.info(f"Global column_num by majority: {global_cols}")

    # ═══════════════════════════════════════════════════════════════════
    # FINAL CLUSTERING WITH SELECTED K
    # ═══════════════════════════════════════════════════════════════════
    for pg, bxs in by_page.items():
        if not bxs:
            continue

        k = page_cols[pg]
        if len(bxs) < k:
            k = 1

        x0s = np.array([[b["x0"]] for b in bxs], dtype=float)
        km = KMeans(n_clusters=k, n_init="auto")
        labels = km.fit_predict(x0s)

        # ═══════════════════════════════════════════════════════════════
        # REMAP CLUSTER IDS: Left-to-right order
        # ═══════════════════════════════════════════════════════════════
        centers = km.cluster_centers_.flatten()
        order = np.argsort(centers)
        remap = {orig: new for new, orig in enumerate(order)}

        for b, lb in zip(bxs, labels):
            b["col_id"] = remap[lb]

    return boxes
```

### Visualization

```
Single column (k=1):                    Two columns (k=2):
┌────────────────────────────┐          ┌─────────────┬─────────────┐
│ Text text text text        │          │ Col 0       │ Col 1       │
│ text text text text        │          │ Text text   │ Text text   │
│ text text text text        │          │ text text   │ text text   │
│ text text text text        │          │ text text   │ text text   │
└────────────────────────────┘          └─────────────┴─────────────┘
       col_id = 0                        col_id = 0    col_id = 1

X coordinates:                          X coordinates:
[50, 52, 48, 51, ...]                   [50, 52, 300, 302, 49, 301, ...]
     ↓ K-Means                               ↓ K-Means
  k=1, all → 0                           k=2, cluster 0 → 0, cluster 1 → 1
```

---

## Step 6: Text Merge (Horizontal) (`_text_merge`)

**File**: `pdf_parser.py`
**Lines**: 442-478

### Algorithm

```
Horizontal Merge Conditions:
1. Same page
2. Same column (col_id)
3. Same layout (layoutno)
4. Not table/figure/equation
5. Y distance < mean_height / 3
```

### Code Analysis

```python
def _text_merge(self, zoomin=3):
    """
    Merge horizontally adjacent boxes with same layout.
    """
    bxs = self._assign_column(self.boxes, zoomin)  # Ensure col_id assigned

    # Helper functions
    def end_with(b, txt):
        txt = txt.strip()
        tt = b.get("text", "").strip()
        return tt and tt.find(txt) == len(tt) - len(txt)

    def start_with(b, txts):
        tt = b.get("text", "").strip()
        return tt and any([tt.find(t.strip()) == 0 for t in txts])

    # ═══════════════════════════════════════════════════════════════════
    # HORIZONTAL MERGE LOOP
    # ═══════════════════════════════════════════════════════════════════
    i = 0
    while i < len(bxs) - 1:
        b = bxs[i]
        b_ = bxs[i + 1]

        # Skip if different page or column
        if b["page_number"] != b_["page_number"]:
            i += 1
            continue
        if b.get("col_id") != b_.get("col_id"):
            i += 1
            continue

        # Skip if different layout or special type
        if b.get("layoutno", "0") != b_.get("layoutno", "1"):
            i += 1
            continue
        if b.get("layout_type", "") in ["table", "figure", "equation"]:
            i += 1
            continue

        # Check Y distance
        y_dis = abs(self._y_dis(b, b_))
        threshold = self.mean_height[bxs[i]["page_number"] - 1] / 3

        if y_dis < threshold:
            # ═══════════════════════════════════════════════════════════
            # MERGE: Expand box to include next
            # ═══════════════════════════════════════════════════════════
            bxs[i]["x1"] = b_["x1"]                    # Extend right edge
            bxs[i]["top"] = (b["top"] + b_["top"]) / 2      # Average top
            bxs[i]["bottom"] = (b["bottom"] + b_["bottom"]) / 2  # Average bottom
            bxs[i]["text"] += b_["text"]              # Concatenate text
            bxs.pop(i + 1)                             # Remove merged box
            continue  # Check if can merge more

        i += 1

    self.boxes = bxs
```

### Visualization

```
Before horizontal merge:
┌──────┐ ┌──────┐ ┌──────┐
│Hello │ │World │ │!     │  (same line, same layout)
└──────┘ └──────┘ └──────┘

After horizontal merge:
┌────────────────────────┐
│Hello World!            │
└────────────────────────┘
```

---

## Step 7: Text Merge (Vertical) (`_naive_vertical_merge`)

**File**: `pdf_parser.py`
**Lines**: 480-556

### Algorithm

```
Vertical Merge Conditions:
1. Same page and column
2. Same layout (layoutno)
3. Y distance < 1.5 * mean_height
4. Horizontal overlap > 30%
5. Semantic checks (punctuation, text patterns)
```

### Code Analysis

```python
def _naive_vertical_merge(self, zoomin=3):
    """
    Merge vertically adjacent boxes within same layout.
    """
    bxs = self._assign_column(self.boxes, zoomin)

    # ═══════════════════════════════════════════════════════════════════
    # GROUP BY PAGE AND COLUMN
    # ═══════════════════════════════════════════════════════════════════
    grouped = defaultdict(list)
    for b in bxs:
        grouped[(b["page_number"], b.get("col_id", 0))].append(b)

    merged_boxes = []

    for (pg, col), bxs in grouped.items():
        # Sort by top-to-bottom, left-to-right
        bxs = sorted(bxs, key=lambda x: (x["top"], x["x0"]))
        if not bxs:
            continue

        mh = self.mean_height[pg - 1] if self.mean_height else 10

        i = 0
        while i + 1 < len(bxs):
            b = bxs[i]
            b_ = bxs[i + 1]

            # ═══════════════════════════════════════════════════════════
            # SKIP CONDITIONS
            # ═══════════════════════════════════════════════════════════

            # Remove page numbers at page boundaries
            if b["page_number"] < b_["page_number"]:
                if re.match(r"[0-9  •一—-]+$", b["text"]):
                    bxs.pop(i)
                    continue

            # Skip empty text
            if not b["text"].strip():
                bxs.pop(i)
                continue

            # Skip different layouts
            if b.get("layoutno") != b_.get("layoutno"):
                i += 1
                continue

            # Skip if too far apart vertically
            if b_["top"] - b["bottom"] > mh * 1.5:
                i += 1
                continue

            # ═══════════════════════════════════════════════════════════
            # CHECK HORIZONTAL OVERLAP
            # ═══════════════════════════════════════════════════════════
            overlap = max(0, min(b["x1"], b_["x1"]) - max(b["x0"], b_["x0"]))
            min_width = min(b["x1"] - b["x0"], b_["x1"] - b_["x0"])
            if overlap / max(1, min_width) < 0.3:
                i += 1
                continue

            # ═══════════════════════════════════════════════════════════
            # SEMANTIC ANALYSIS
            # ═══════════════════════════════════════════════════════════
            # Features favoring concatenation
            concatting_feats = [
                b["text"].strip()[-1] in ",;:'\"，、'"；：-",       # Ends with continuation punct
                len(b["text"].strip()) > 1 and
                    b["text"].strip()[-2] in ",;:'\"，'"、；：",
                b_["text"].strip() and
                    b_["text"].strip()[0] in "。；？！?"）),，、：", # Starts with ending punct
            ]

            # Features preventing concatenation
            feats = [
                b.get("layoutno", 0) != b_.get("layoutno", 0),    # Different layout
                b["text"].strip()[-1] in "。？！?",               # Sentence end
                self.is_english and b["text"].strip()[-1] in ".!?",
                b["page_number"] == b_["page_number"] and
                    b_["top"] - b["bottom"] > mh * 1.5,           # Too far
                b["page_number"] < b_["page_number"] and
                    abs(b["x0"] - b_["x0"]) > self.mean_width[b["page_number"] - 1] * 4,
            ]

            # Features for definite split
            detach_feats = [
                b["x1"] < b_["x0"],  # No horizontal overlap at all
                b["x0"] > b_["x1"],
            ]

            # ═══════════════════════════════════════════════════════════
            # DECISION
            # ═══════════════════════════════════════════════════════════
            if (any(feats) and not any(concatting_feats)) or any(detach_feats):
                i += 1
                continue

            # ═══════════════════════════════════════════════════════════
            # MERGE
            # ═══════════════════════════════════════════════════════════
            b["text"] = (b["text"].rstrip() + " " + b_["text"].lstrip()).strip()
            b["bottom"] = b_["bottom"]
            b["x0"] = min(b["x0"], b_["x0"])
            b["x1"] = max(b["x1"], b_["x1"])
            bxs.pop(i + 1)

        merged_boxes.extend(bxs)

    self.boxes = sorted(merged_boxes, key=lambda x: (x["page_number"], x.get("col_id", 0), x["top"]))
```

### Visualization

```
Before vertical merge:
┌────────────────────────┐
│This is paragraph one   │
└────────────────────────┘
┌────────────────────────┐
│that continues here and │
└────────────────────────┘
┌────────────────────────┐
│ends with this line.    │
└────────────────────────┘

After vertical merge:
┌────────────────────────┐
│This is paragraph one   │
│that continues here and │
│ends with this line.    │
└────────────────────────┘
```

---

## Step 8: Filter & Cleanup

### 8.1 `_filter_forpages`

**Lines**: 685-729

```python
def _filter_forpages(self):
    """
    Remove table of contents and dirty pages.
    """
    if not self.boxes:
        return

    # ═══════════════════════════════════════════════════════════════════
    # DETECT AND REMOVE TABLE OF CONTENTS
    # ═══════════════════════════════════════════════════════════════════
    findit = False
    i = 0
    while i < len(self.boxes):
        # Check for TOC headers
        text_lower = re.sub(r"( | |\u3000)+", "", self.boxes[i]["text"].lower())
        if not re.match(r"(contents|目录|目次|table of contents|致谢|acknowledge)$", text_lower):
            i += 1
            continue

        findit = True
        eng = re.match(r"[0-9a-zA-Z :'.-]{5,}", self.boxes[i]["text"].strip())
        self.boxes.pop(i)  # Remove TOC header

        if i >= len(self.boxes):
            break

        # Get prefix of first TOC entry
        prefix = self.boxes[i]["text"].strip()[:3] if not eng else \
                 " ".join(self.boxes[i]["text"].strip().split()[:2])

        # Remove empty entries
        while not prefix:
            self.boxes.pop(i)
            if i >= len(self.boxes):
                break
            prefix = self.boxes[i]["text"].strip()[:3] if not eng else \
                     " ".join(self.boxes[i]["text"].strip().split()[:2])

        self.boxes.pop(i)
        if i >= len(self.boxes) or not prefix:
            break

        # Remove entries matching TOC pattern
        for j in range(i, min(i + 128, len(self.boxes))):
            if not re.match(prefix, self.boxes[j]["text"]):
                continue
            for k in range(i, j):
                self.boxes.pop(i)
            break

    if findit:
        return

    # ═══════════════════════════════════════════════════════════════════
    # DETECT AND REMOVE DIRTY PAGES
    # ═══════════════════════════════════════════════════════════════════
    page_dirty = [0] * len(self.page_images)
    for b in self.boxes:
        # Count repetitive patterns (common in scanned TOC)
        if re.search(r"(··|··|··)", b["text"]):
            page_dirty[b["page_number"] - 1] += 1

    # Pages with >3 repetitive patterns are dirty
    page_dirty = set([i + 1 for i, t in enumerate(page_dirty) if t > 3])

    if not page_dirty:
        return

    # Remove all boxes from dirty pages
    i = 0
    while i < len(self.boxes):
        if self.boxes[i]["page_number"] in page_dirty:
            self.boxes.pop(i)
            continue
        i += 1
```

---

## Step 9: Extract Tables & Figures (`_extract_table_figure`)

**File**: `pdf_parser.py`
**Lines**: 757-930

### Code Analysis

```python
def _extract_table_figure(self, need_image, ZM, return_html, need_position,
                          separate_tables_figures=False):
    """
    Extract tables and figures from detected layouts.
    """
    tables = {}
    figures = {}

    # ═══════════════════════════════════════════════════════════════════
    # STEP 9.1: SEPARATE TABLE AND FIGURE BOXES
    # ═══════════════════════════════════════════════════════════════════
    i = 0
    lst_lout_no = ""
    nomerge_lout_no = []

    while i < len(self.boxes):
        if "layoutno" not in self.boxes[i]:
            i += 1
            continue

        lout_no = f"{self.boxes[i]['page_number']}-{self.boxes[i]['layoutno']}"

        # Mark captions as non-mergeable
        if (TableStructureRecognizer.is_caption(self.boxes[i]) or
            self.boxes[i]["layout_type"] in ["table caption", "title",
                                             "figure caption", "reference"]):
            nomerge_lout_no.append(lst_lout_no)

        # ═══════════════════════════════════════════════════════════════
        # EXTRACT TABLE BOXES
        # ═══════════════════════════════════════════════════════════════
        if self.boxes[i]["layout_type"] == "table":
            # Skip source citations
            if re.match(r"(数据|资料|图表)*来源[:： ]", self.boxes[i]["text"]):
                self.boxes.pop(i)
                continue

            if lout_no not in tables:
                tables[lout_no] = []
            tables[lout_no].append(self.boxes[i])
            self.boxes.pop(i)
            lst_lout_no = lout_no
            continue

        # ═══════════════════════════════════════════════════════════════
        # EXTRACT FIGURE BOXES
        # ═══════════════════════════════════════════════════════════════
        if need_image and self.boxes[i]["layout_type"] == "figure":
            if re.match(r"(数据|资料|图表)*来源[:： ]", self.boxes[i]["text"]):
                self.boxes.pop(i)
                continue

            if lout_no not in figures:
                figures[lout_no] = []
            figures[lout_no].append(self.boxes[i])
            self.boxes.pop(i)
            lst_lout_no = lout_no
            continue

        i += 1

    # ═══════════════════════════════════════════════════════════════════
    # STEP 9.2: MERGE CROSS-PAGE TABLES
    # ═══════════════════════════════════════════════════════════════════
    nomerge_lout_no = set(nomerge_lout_no)
    tbls = sorted([(k, bxs) for k, bxs in tables.items()],
                  key=lambda x: (x[1][0]["top"], x[1][0]["x0"]))

    i = len(tbls) - 1
    while i - 1 >= 0:
        k0, bxs0 = tbls[i - 1]
        k, bxs = tbls[i]
        i -= 1

        if k0 in nomerge_lout_no:
            continue
        if bxs[0]["page_number"] == bxs0[0]["page_number"]:
            continue
        if bxs[0]["page_number"] - bxs0[0]["page_number"] > 1:
            continue

        mh = self.mean_height[bxs[0]["page_number"] - 1]
        if self._y_dis(bxs0[-1], bxs[0]) > mh * 23:
            continue

        # Merge tables
        tables[k0].extend(tables[k])
        del tables[k]

    # ═══════════════════════════════════════════════════════════════════
    # STEP 9.3: ASSOCIATE CAPTIONS WITH TABLES/FIGURES
    # ═══════════════════════════════════════════════════════════════════
    i = 0
    while i < len(self.boxes):
        c = self.boxes[i]
        if not TableStructureRecognizer.is_caption(c):
            i += 1
            continue

        # Find nearest table/figure
        def nearest(tbls):
            mink, minv = "", float('inf')
            for k, bxs in tbls.items():
                for b in bxs:
                    if b.get("layout_type", "").find("caption") >= 0:
                        continue
                    y_dis = self._y_dis(c, b)
                    x_dis = self._x_dis(c, b) if not x_overlapped(c, b) else 0
                    dis = y_dis**2 + x_dis**2
                    if dis < minv:
                        mink, minv = k, dis
            return mink, minv

        tk, tv = nearest(tables)
        fk, fv = nearest(figures)

        if tv < fv and tk:
            tables[tk].insert(0, c)
        elif fk:
            figures[fk].insert(0, c)
        self.boxes.pop(i)

    # ═══════════════════════════════════════════════════════════════════
    # STEP 9.4: CONSTRUCT TABLE OUTPUT
    # ═══════════════════════════════════════════════════════════════════
    res = []
    for k, bxs in tables.items():
        if not bxs:
            continue

        bxs = Recognizer.sort_Y_firstly(bxs, np.mean([(b["bottom"] - b["top"]) / 2 for b in bxs]))
        poss = []

        # Crop table image
        img = cropout(bxs, "table", poss)

        # Construct table content (HTML or descriptive)
        content = self.tbl_det.construct_table(
            bxs,
            html=return_html,
            is_english=self.is_english
        )

        res.append((img, content))

    return res
```

### Table Construction Flow

```
Table boxes with R/C/H/SP attributes
                │
                ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  TableStructureRecognizer.construct_table()                                  │
│                                                                              │
│  1. Sort by row (R attribute)                                               │
│  2. Group into rows                                                         │
│  3. Sort each row by column (C attribute)                                   │
│  4. Build 2D table matrix                                                   │
│  5. Handle spanning cells (SP attribute)                                    │
│  6. Generate output format                                                  │
└─────────────────────────────────────────────────────────────────────────────┘
                │
        ┌───────┴───────┐
        │               │
        ▼               ▼
    HTML Output     Descriptive Output

HTML:
<table>
  <caption>Table 1: Data</caption>
  <tr><th>Name</th><th>Value</th></tr>
  <tr><td>Item 1</td><td>100</td></tr>
</table>

Descriptive:
Name: Item 1; Value: 100
Name: Item 2; Value: 200
(from "Table 1: Data")
```

---

## Step 10: Final Output (`__filterout_scraps`)

**File**: `pdf_parser.py`
**Lines**: 971-1029

### Code Analysis

```python
def __filterout_scraps(self, boxes, ZM):
    """
    Filter low-quality text blocks and format final output.
    """
    def width(b):
        return b["x1"] - b["x0"]

    def height(b):
        return b["bottom"] - b["top"]

    def usefull(b):
        """Check if box is useful."""
        if b.get("layout_type"):
            return True
        # Width > 1/3 page width
        if width(b) > self.page_images[b["page_number"] - 1].size[0] / ZM / 3:
            return True
        # Height > mean character height
        if height(b) > self.mean_height[b["page_number"] - 1]:
            return True
        return False

    res = []

    while boxes:
        lines = []
        widths = []
        pw = self.page_images[boxes[0]["page_number"] - 1].size[0] / ZM
        mh = self.mean_height[boxes[0]["page_number"] - 1]
        mj = self.proj_match(boxes[0]["text"]) or \
             boxes[0].get("layout_type", "") == "title"

        # ═══════════════════════════════════════════════════════════════
        # DFS TO FIND CONNECTED LINES
        # ═══════════════════════════════════════════════════════════════
        def dfs(line, st):
            nonlocal mh, pw, lines, widths
            lines.append(line)
            widths.append(width(line))
            mmj = self.proj_match(line["text"]) or \
                  line.get("layout_type", "") == "title"

            for i in range(st + 1, min(st + 20, len(boxes))):
                # Stop at page boundary
                if boxes[i]["page_number"] - line["page_number"] > 0:
                    break

                # Stop if too far vertically
                if not mmj and self._y_dis(line, boxes[i]) >= 3 * mh and \
                   height(line) < 1.5 * mh:
                    break

                if not usefull(boxes[i]):
                    continue

                # Check horizontal proximity
                if mmj or (self._x_dis(boxes[i], line) < pw / 10):
                    dfs(boxes[i], i)
                    boxes.pop(i)
                    break

        try:
            if usefull(boxes[0]):
                dfs(boxes[0], 0)
            else:
                logging.debug("WASTE: " + boxes[0]["text"])
        except:
            pass

        boxes.pop(0)

        # ═══════════════════════════════════════════════════════════════
        # FILTER AND FORMAT OUTPUT
        # ═══════════════════════════════════════════════════════════════
        mw = np.mean(widths)
        if mj or mw / pw >= 0.35 or mw > 200:
            # Add position tags to each line
            result = "\n".join([
                c["text"] + self._line_tag(c, ZM)
                for c in lines
            ])
            res.append(result)
        else:
            logging.debug("REMOVED: " + "<<".join([c["text"] for c in lines]))

    return "\n\n".join(res)
```

### Position Tag Format

```python
def _line_tag(self, bx, ZM):
    """
    Generate position tag for a text box.

    Format: @@{page_numbers}\t{x0}\t{x1}\t{top}\t{bottom}##

    Example: @@1-2\t50.0\t450.0\t100.0\t120.0##
    (Text spans pages 1-2, coordinates in original scale)
    """
    pn = [bx["page_number"]]
    top = bx["top"] - self.page_cum_height[pn[0] - 1]
    bott = bx["bottom"] - self.page_cum_height[pn[0] - 1]

    # Handle multi-page spanning
    while bott * ZM > self.page_images[pn[-1] - 1].size[1]:
        bott -= self.page_images[pn[-1] - 1].size[1] / ZM
        pn.append(pn[-1] + 1)

    return "@@{}\t{:.1f}\t{:.1f}\t{:.1f}\t{:.1f}##".format(
        "-".join([str(p) for p in pn]),
        bx["x0"], bx["x1"], top, bott
    )
```

### Final Output Format

```python
# Return value of __call__:
(
    # documents: str (paragraphs separated by \n\n)
    "Paragraph 1 text@@1\t50.0\t450.0\t100.0\t150.0##\n\n"
    "Paragraph 2 text@@1\t50.0\t450.0\t200.0\t250.0##\n\n"
    "...",

    # tables: List[Tuple[PIL.Image, str|List[str]]]
    [
        (table_image_1, "<table>...</table>"),
        (table_image_2, ["desc line 1", "desc line 2"]),
    ]
)
```

---

## Tổng Kết

### Complete Pipeline Summary

| Step | Method | Lines | Input | Output |
|------|--------|-------|-------|--------|
| 1 | `__init__` | 52-105 | - | Models loaded |
| 2 | `__images__` | 1042-1159 | PDF file | boxes[], page_images[] |
| 3 | `_layouts_rec` | 347-353 | page_images, boxes | boxes with layout_type |
| 4 | `_table_transformer_job` | 196-281 | page_images, boxes | boxes with R/C/H/SP |
| 5 | `_assign_column` | 355-440 | boxes | boxes with col_id |
| 6 | `_text_merge` | 442-478 | boxes | merged boxes (horizontal) |
| 7 | `_naive_vertical_merge` | 480-556 | boxes | merged boxes (vertical) |
| 8 | `_filter_forpages` | 685-729 | boxes | cleaned boxes |
| 9 | `_extract_table_figure` | 757-930 | boxes | tables[], figures[] |
| 10 | `__filterout_scraps` | 971-1029 | boxes | formatted text |

### Key Data Structures

```python
# Box structure throughout pipeline
{
    # Basic (from OCR)
    "x0": float,           # Left edge
    "x1": float,           # Right edge
    "top": float,          # Top edge (cumulative Y)
    "bottom": float,       # Bottom edge (cumulative Y)
    "text": str,           # Recognized text
    "page_number": int,    # 1-indexed page

    # From layout recognition (Step 3)
    "layout_type": str,    # "text", "title", "table", "figure"...
    "layoutno": int,       # Layout region ID

    # From table detection (Step 4)
    "R": int,              # Row index
    "C": int,              # Column index
    "H": int,              # Header row index
    "SP": int,             # Spanning cell index

    # From column detection (Step 5)
    "col_id": int,         # Column ID (0-based)
}
```
