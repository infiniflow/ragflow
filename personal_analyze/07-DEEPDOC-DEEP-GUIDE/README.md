# DeepDoc Module - Hướng Dẫn Đọc Hiểu Chuyên Sâu

## Mục Lục

1. [Bức Tranh Lớn](#1-bức-tranh-lớn)
2. [Luồng Dữ Liệu](#2-luồng-dữ-liệu)
3. [Phân Tích Chi Tiết Code](#3-phân-tích-chi-tiết-code)
4. [Giải Thích Kỹ Thuật](#4-giải-thích-kỹ-thuật)
5. [Lý Do Thiết Kế](#5-lý-do-thiết-kế)
6. [Thuật Ngữ Khó](#6-thuật-ngữ-khó)
7. [Mở Rộng Từ Code](#7-mở-rộng-từ-code)

---

## 1. Bức Tranh Lớn

### 1.1 DeepDoc Giải Quyết Vấn Đề Gì?

**Vấn đề cốt lõi**: Khi xây dựng hệ thống RAG (Retrieval-Augmented Generation), bạn cần chuyển đổi tài liệu (PDF, Word, Excel...) thành dạng text có cấu trúc để:
- Tìm kiếm semantic (vector search)
- Chia nhỏ (chunking) hợp lý
- Giữ nguyên ngữ cảnh của bảng, hình ảnh

**DeepDoc là gì?**: Một module Python chuyên biệt để:
```
Document Files → Structured Text + Tables + Figures
(PDF, DOCX...)   (Có position, layout type, reading order)
```

### 1.2 Kiến Trúc Tổng Quan

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              DEEPDOC MODULE                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                         PARSER LAYER                                 │   │
│  │  Chuyển đổi các định dạng file thành text có cấu trúc               │   │
│  │                                                                      │   │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐  │   │
│  │  │   PDF    │ │   DOCX   │ │  Excel   │ │   HTML   │ │ Markdown │  │   │
│  │  │  Parser  │ │  Parser  │ │  Parser  │ │  Parser  │ │  Parser  │  │   │
│  │  └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘  │   │
│  │       │            │            │            │            │         │   │
│  └───────┼────────────┼────────────┼────────────┼────────────┼─────────┘   │
│          │            │            │            │            │              │
│          │            └────────────┴────────────┴────────────┘              │
│          │                         │                                        │
│          │              Text-based parsing                                  │
│          │              (pdfplumber, python-docx, openpyxl...)             │
│          │                                                                  │
│          ▼                                                                  │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                         VISION LAYER                                 │   │
│  │  Computer Vision cho PDF phức tạp (scanned, multi-column)           │   │
│  │                                                                      │   │
│  │  ┌──────────────┐  ┌──────────────────┐  ┌────────────────────┐    │   │
│  │  │     OCR      │  │ Layout Recognizer│  │ Table Structure    │    │   │
│  │  │  Detection + │  │    (YOLOv10)     │  │   Recognizer       │    │   │
│  │  │  Recognition │  │                  │  │                    │    │   │
│  │  └──────────────┘  └──────────────────┘  └────────────────────┘    │   │
│  │         │                   │                      │                │   │
│  │         └───────────────────┴──────────────────────┘                │   │
│  │                             │                                        │   │
│  │                    ONNX Runtime Inference                            │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 1.3 Các Thành Phần Chính

| Thành Phần | File | Mục Đích |
|------------|------|----------|
| **PDF Parser** | `parser/pdf_parser.py` | Parser phức tạp nhất - xử lý PDF với OCR + layout |
| **Office Parsers** | `parser/docx_parser.py`, `excel_parser.py`, `ppt_parser.py` | Xử lý file Microsoft Office |
| **Web Parsers** | `parser/html_parser.py`, `markdown_parser.py`, `json_parser.py` | Xử lý file web/markup |
| **OCR Engine** | `vision/ocr.py` | Text detection + recognition |
| **Layout Detector** | `vision/layout_recognizer.py` | Phân loại vùng (text, table, figure...) |
| **Table Detector** | `vision/table_structure_recognizer.py` | Nhận dạng cấu trúc bảng |
| **Operators** | `vision/operators.py` | Image preprocessing pipeline |

### 1.4 Tại Sao Cần DeepDoc?

**Không có DeepDoc** (naive approach):
```python
# Chỉ extract raw text từ PDF
text = pdfplumber.open("doc.pdf").pages[0].extract_text()
# Kết quả: "Header Footer Table content mixed together..."
# ❌ Mất cấu trúc, table thành text xáo trộn
```

**Với DeepDoc**:
```python
parser = RAGFlowPdfParser()
docs, tables = parser("doc.pdf")
# docs: [("Paragraph 1", "page_0_pos_100_200"), ("Paragraph 2", "page_0_pos_300_400")]
# tables: [{"html": "<table>...</table>", "bbox": [...]}]
# ✅ Giữ nguyên cấu trúc, table được parse riêng
```

---

## 2. Luồng Dữ Liệu

### 2.1 Luồng Chính: PDF Processing

```
┌────────────────────────────────────────────────────────────────────────────┐
│                        PDF PROCESSING PIPELINE                              │
└────────────────────────────────────────────────────────────────────────────┘

Input: PDF File (path hoặc bytes)
         │
         ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STEP 1: IMAGE EXTRACTION                                                    │
│  File: pdf_parser.py, __images__() (lines 1042-1159)                        │
│                                                                              │
│  • Convert PDF pages → numpy images (using pdfplumber)                      │
│  • Extract native PDF characters (text layer)                               │
│  • Zoom factor: 3x (default) for OCR accuracy                               │
│                                                                              │
│  Output: page_images[], page_chars[]                                        │
└──────────────────────────────────┬──────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STEP 2: OCR DETECTION & RECOGNITION                                         │
│  File: vision/ocr.py, OCR.__call__() (lines 708-751)                        │
│                                                                              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐                   │
│  │ TextDetector │ →  │   Crop &     │ →  │TextRecognizer│                   │
│  │   (DBNet)    │    │   Rotate     │    │   (CRNN)     │                   │
│  └──────────────┘    └──────────────┘    └──────────────┘                   │
│                                                                              │
│  • Detect text regions → bounding boxes                                     │
│  • Crop each region, auto-rotate if needed                                  │
│  • Recognize text in each region                                            │
│                                                                              │
│  Output: boxes[] with {text, confidence, coordinates}                       │
└──────────────────────────────────┬──────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STEP 3: LAYOUT RECOGNITION                                                  │
│  File: vision/layout_recognizer.py, __call__() (lines 63-157)               │
│                                                                              │
│  • Run YOLOv10 model on page image                                          │
│  • Detect 10 layout types: Text, Title, Table, Figure, etc.                 │
│  • Match OCR boxes to layout regions                                        │
│                                                                              │
│  Output: boxes[] with added {layout_type, layoutno}                         │
└──────────────────────────────────┬──────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STEP 4: COLUMN DETECTION                                                    │
│  File: pdf_parser.py, _assign_column() (lines 355-440)                      │
│                                                                              │
│  • K-Means clustering on X coordinates                                      │
│  • Silhouette score to find optimal k (1-4 columns)                         │
│  • Assign col_id to each text box                                           │
│                                                                              │
│  Output: boxes[] with added {col_id}                                        │
└──────────────────────────────────┬──────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STEP 5: TABLE STRUCTURE RECOGNITION                                         │
│  File: vision/table_structure_recognizer.py, __call__() (lines 67-111)      │
│                                                                              │
│  • Detect rows, columns, headers, spanning cells                            │
│  • Match text boxes to table cells                                          │
│  • Build 2D table matrix                                                    │
│                                                                              │
│  Output: table_components[] with grid structure                             │
└──────────────────────────────────┬──────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STEP 6: TEXT MERGING                                                        │
│  File: pdf_parser.py, _text_merge() (lines 442-478)                         │
│                           _naive_vertical_merge() (lines 480-556)           │
│                                                                              │
│  • Horizontal merge: same line, same column, same layout                    │
│  • Vertical merge: adjacent paragraphs with semantic checks                 │
│  • Respect sentence boundaries (。？！)                                      │
│                                                                              │
│  Output: merged_boxes[] (fewer, larger text blocks)                         │
└──────────────────────────────────┬──────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STEP 7: FILTERING & CLEANUP                                                 │
│  File: pdf_parser.py, _filter_forpages() (lines 685-729)                    │
│                        __filterout_scraps() (lines 971-1029)                │
│                                                                              │
│  • Remove headers/footers (top/bottom 10% of page)                          │
│  • Remove table of contents                                                 │
│  • Filter low-quality OCR results                                           │
│                                                                              │
│  Output: clean_boxes[]                                                      │
└──────────────────────────────────┬──────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  STEP 8: EXTRACT TABLES & FIGURES                                            │
│  File: pdf_parser.py, _extract_table_figure() (lines 757-930)               │
│                                                                              │
│  • Convert table boxes to HTML/descriptive text                             │
│  • Extract figure images with captions                                      │
│  • Handle spanning cells (colspan, rowspan)                                 │
│                                                                              │
│  Output: tables[], figures[]                                                │
└──────────────────────────────────┬──────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│  FINAL OUTPUT                                                                │
│                                                                              │
│  documents: [(text, position_tag), ...]                                     │
│  tables: [{"html": "...", "bbox": [...], "image": ...}, ...]               │
│                                                                              │
│  position_tag format: "page_{page}_x0_{x0}_y0_{y0}_x1_{x1}_y1_{y1}"        │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Luồng OCR Chi Tiết

```
                           Input Image (H, W, 3)
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                        TEXT DETECTION (DBNet)                                │
│  File: vision/ocr.py, TextDetector.__call__() (lines 503-530)               │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
           ┌────────────────────────┼────────────────────────┐
           │                        │                        │
           ▼                        ▼                        ▼
    ┌─────────────┐          ┌─────────────┐          ┌─────────────┐
    │ Preprocess  │          │    ONNX     │          │ Postprocess │
    │             │          │  Inference  │          │             │
    │ • Resize    │    →     │             │    →     │ • Threshold │
    │ • Normalize │          │  DBNet      │          │ • Contours  │
    │ • Transpose │          │  Model      │          │ • Unclip    │
    └─────────────┘          └─────────────┘          └─────────────┘
                                    │
                                    ▼
                         Text Region Polygons
                         [[x0,y0], [x1,y1], [x2,y2], [x3,y3]]
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                      TEXT RECOGNITION (CRNN)                                 │
│  File: vision/ocr.py, TextRecognizer.__call__() (lines 363-408)             │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
           ┌────────────────────────┼────────────────────────┐
           │                        │                        │
           ▼                        ▼                        ▼
    ┌─────────────┐          ┌─────────────┐          ┌─────────────┐
    │    Crop     │          │    ONNX     │          │ CTC Decode  │
    │   Rotate    │          │  Inference  │          │             │
    │             │    →     │             │    →     │ • Argmax    │
    │ Perspective │          │   CRNN      │          │ • Dedup     │
    │ Transform   │          │   Model     │          │ • Remove ε  │
    └─────────────┘          └─────────────┘          └─────────────┘
                                    │
                                    ▼
                    Output: [(box, (text, confidence)), ...]
```

### 2.3 Luồng Layout Recognition

```
                           Input: Page Image + OCR Results
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                     LAYOUT DETECTION (YOLOv10)                               │
│  File: vision/layout_recognizer.py (lines 163-237)                          │
└─────────────────────────────────────────────────────────────────────────────┘
                                        │
           ┌────────────────────────────┼────────────────────────────┐
           │                            │                            │
           ▼                            ▼                            ▼
    ┌─────────────┐              ┌─────────────┐              ┌─────────────┐
    │ Preprocess  │              │    ONNX     │              │ Postprocess │
    │             │              │  Inference  │              │             │
    │ • Resize    │      →       │             │      →       │ • NMS       │
    │   (640x640) │              │  YOLOv10    │              │ • Filter    │
    │ • Pad       │              │   Model     │              │ • Scale     │
    │ • Normalize │              │             │              │   back      │
    └─────────────┘              └─────────────┘              └─────────────┘
                                        │
                                        ▼
                              Layout Detections:
                              [{"type": "Table", "bbox": [...], "score": 0.95}]
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                     OCR-LAYOUT ASSOCIATION                                   │
│  File: vision/layout_recognizer.py (lines 98-147)                           │
│                                                                              │
│  For each OCR box:                                                          │
│    • Find overlapping layout region (threshold: 40%)                        │
│    • Assign layout_type to OCR box                                          │
│    • Filter garbage (headers/footers/page numbers)                          │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
                    Output: OCR boxes with layout_type attribute
                    [{"text": "...", "layout_type": "Text", "layoutno": 1}]
```

### 2.4 Data Flow Summary

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  PDF File   │ →   │   Images    │ →   │ OCR Boxes   │ →   │  Merged     │
│             │     │ + Chars     │     │ + Layout    │     │  Documents  │
└─────────────┘     └─────────────┘     └─────────────┘     └─────────────┘
                                              │
                                              ▼
                                        ┌─────────────┐
                                        │   Tables    │
                                        │  (HTML/Desc)│
                                        └─────────────┘

Input Format:
- File path: str (e.g., "/path/to/doc.pdf")
- Or bytes: bytes (raw PDF content)

Output Format:
- documents: List[Tuple[str, str]]
  - text: Extracted text content
  - position_tag: "page_0_x0_100_y0_200_x1_500_y1_250"

- tables: List[Dict]
  - html: "<table>...</table>"
  - bbox: [x0, y0, x1, y1]
  - image: numpy array (optional)
```

---

## 3. Phân Tích Chi Tiết Code

### 3.1 RAGFlowPdfParser Class

**File**: `/deepdoc/parser/pdf_parser.py`
**Lines**: 52-1479

#### 3.1.1 Constructor (__init__)

```python
# Line 52-104
class RAGFlowPdfParser:
    def __init__(self, **kwargs):
        # Load OCR model
        self.ocr = OCR()  # vision/ocr.py

        # Load Layout Recognizer (YOLOv10)
        self.layout_recognizer = LayoutRecognizer()  # vision/layout_recognizer.py

        # Load Table Structure Recognizer
        self.tsr = TableStructureRecognizer()  # vision/table_structure_recognizer.py

        # Load XGBoost model for text concatenation
        try:
            self.updown_cnt_mdl = xgb.Booster()
            model_path = os.path.join(get_project_base_directory(),
                                      "rag/res/deepdoc/updown_concat_xgb.model")
            self.updown_cnt_mdl.load_model(model_path)
        except Exception as e:
            self.updown_cnt_mdl = None
```

**Giải thích**:
- Constructor khởi tạo 4 models:
  1. **OCR**: Text detection + recognition
  2. **LayoutRecognizer**: Phân loại vùng layout (YOLOv10)
  3. **TableStructureRecognizer**: Nhận dạng cấu trúc bảng
  4. **XGBoost**: Quyết định merge text blocks (31 features)

#### 3.1.2 Main Entry Point (__call__)

```python
# Lines 1160-1168
def __call__(self, fnm, need_image=True, zoomin=3, return_html=False):
    """
    Main entry point for PDF parsing.

    Args:
        fnm: File path or bytes
        need_image: Whether to extract images
        zoomin: Zoom factor for OCR (default 3x)
        return_html: Return HTML tables instead of descriptive text

    Returns:
        (documents, tables) tuple
    """
    self.__images__(fnm, zoomin)           # Step 1: Load images
    self._layouts_rec(zoomin)              # Step 2-3: OCR + Layout
    self._table_transformer_job(zoomin)    # Step 4: Table structure
    self._text_merge(zoomin)               # Step 5: Merge text
    self._filter_forpages()                # Step 6: Filter
    tbls = self._extract_table_figure(...) # Step 7: Extract tables
    return self._final_result(), tbls      # Final output
```

**Tại sao zoomin=3?**
- OCR accuracy tăng đáng kể khi image lớn hơn
- 3x là balance giữa accuracy và memory/speed
- Quá lớn (5x+) → memory issues, quá nhỏ (1x) → OCR errors

#### 3.1.3 Image Loading (__images__)

```python
# Lines 1042-1159
def __images__(self, fnm, zoomin=3, page_from=0, page_to=299, callback=None):
    """
    Load PDF pages as images and extract native characters.
    """
    self.page_images = []
    self.page_chars = []

    # Open PDF with pdfplumber
    with pdfplumber.open(fnm) as pdf:
        for i, page in enumerate(pdf.pages[page_from:page_to]):
            # Convert page to image
            img = page.to_image(resolution=72 * zoomin)
            img = np.array(img.original)
            self.page_images.append(img)

            # Extract native PDF characters
            chars = page.chars
            self.page_chars.append(chars)
```

**Tại sao dùng pdfplumber?**
- Hỗ trợ cả text extraction và image conversion
- Giữ được character-level coordinates
- Xử lý tốt các PDF phức tạp

#### 3.1.4 Column Detection (_assign_column)

```python
# Lines 355-440
def _assign_column(self, boxes, zoomin=3):
    """
    Detect columns using K-Means clustering on X coordinates.
    """
    from sklearn.cluster import KMeans
    from sklearn.metrics import silhouette_score

    # Extract X coordinates
    x_coords = np.array([[b["x0"]] for b in boxes])

    best_k = 1
    best_score = -1

    # Try k from 1 to 4
    for k in range(1, min(5, len(boxes))):
        km = KMeans(n_clusters=k, random_state=42, n_init="auto")
        labels = km.fit_predict(x_coords)

        if k > 1:
            score = silhouette_score(x_coords, labels)
            if score > best_score:
                best_score = score
                best_k = k

    # Final clustering with best k
    km = KMeans(n_clusters=best_k, random_state=42, n_init="auto")
    labels = km.fit_predict(x_coords)

    # Assign column IDs
    for i, box in enumerate(boxes):
        box["col_id"] = labels[i]
```

**Tại sao K-Means?**
- Unsupervised: không cần training data
- Fast: O(n * k * iterations)
- Silhouette score tự động chọn số cột

### 3.2 OCR Class

**File**: `/deepdoc/vision/ocr.py`
**Lines**: 536-752

#### 3.2.1 Text Detection (TextDetector)

```python
# Lines 414-534
class TextDetector:
    def __init__(self, model_dir, device_id=None):
        # Preprocessing pipeline
        self.preprocess_op = [
            DetResizeForTest(limit_side_len=960, limit_type='max'),
            NormalizeImage(mean=[0.485, 0.456, 0.406],
                          std=[0.229, 0.224, 0.225]),
            ToCHWImage(),
            KeepKeys(keep_keys=['image', 'shape'])
        ]

        # Postprocessing
        self.postprocess_op = DBPostProcess(
            thresh=0.3,           # Binary threshold
            box_thresh=0.5,       # Box confidence threshold
            max_candidates=1000,  # Max text regions
            unclip_ratio=1.5      # Box expansion ratio
        )

        # Load ONNX model
        self.ort_sess, self.run_opts = load_model(model_dir, "det", device_id)
```

**DBNet (Differentiable Binarization)**:
- Input: Image → Probability map (text regions)
- Thresholding: prob > 0.3 → foreground
- Unclipping: Expand boxes by 1.5x để capture full text

#### 3.2.2 Text Recognition (TextRecognizer)

```python
# Lines 133-412
class TextRecognizer:
    def __init__(self, model_dir, device_id=None):
        self.rec_image_shape = [3, 48, 320]  # C, H, W
        self.batch_size = 16

        # Load CRNN model
        self.ort_sess, self.run_opts = load_model(model_dir, "rec", device_id)

        # CTC decoder
        self.postprocess_op = CTCLabelDecode(character_dict_path=dict_path)

    def __call__(self, img_list):
        # Sort by aspect ratio for efficient batching
        indices = np.argsort([img.shape[1]/img.shape[0] for img in img_list])

        results = []
        for batch in chunks(indices, self.batch_size):
            # Normalize images
            norm_imgs = [self.resize_norm_img(img_list[i]) for i in batch]

            # Run inference
            preds = self.ort_sess.run(None, {"input": np.stack(norm_imgs)})

            # CTC decode
            texts = self.postprocess_op(preds[0])
            results.extend(texts)

        return results
```

**CRNN + CTC**:
- CNN: Extract visual features
- RNN: Sequence modeling
- CTC: Alignment-free decoding (handles variable-length text)

#### 3.2.3 Rotation Handling

```python
# Lines 584-638
def get_rotate_crop_image(self, img, points):
    """
    Crop text region with auto-rotation detection.
    """
    # Get perspective transform
    rect = self.order_points_clockwise(points)
    M = cv2.getPerspectiveTransform(rect, dst_pts)
    warped = cv2.warpPerspective(img, M, (width, height))

    # Check if text is vertical (height > 1.5 * width)
    if warped.shape[0] / warped.shape[1] >= 1.5:
        # Try 3 orientations
        scores = []
        for angle in [0, 90, -90]:
            rotated = self.rotate(warped, angle)
            _, conf = self.recognizer([rotated])[0]
            scores.append(conf)

        # Use orientation with highest confidence
        best_angle = [0, 90, -90][np.argmax(scores)]
        warped = self.rotate(warped, best_angle)

    return warped
```

**Tại sao cần auto-rotation?**
- PDF có thể chứa text xoay 90°
- OCR model trained on horizontal text
- Auto-detect giúp nhận dạng text dọc chính xác

### 3.3 Layout Recognizer

**File**: `/deepdoc/vision/layout_recognizer.py`
**Lines**: 33-237

#### 3.3.1 YOLOv10 Preprocessing

```python
# Lines 186-209
def preprocess(self, image_list):
    """
    Preprocess images for YOLOv10 inference.
    """
    processed = []
    for img in image_list:
        h, w = img.shape[:2]

        # Calculate scale (preserve aspect ratio)
        r = min(640/h, 640/w)
        new_h, new_w = int(h*r), int(w*r)

        # Resize
        resized = cv2.resize(img, (new_w, new_h))

        # Pad to 640x640 (center padding, gray color)
        padded = np.full((640, 640, 3), 114, dtype=np.uint8)
        pad_top = (640 - new_h) // 2
        pad_left = (640 - new_w) // 2
        padded[pad_top:pad_top+new_h, pad_left:pad_left+new_w] = resized

        # Normalize and transpose
        padded = padded.astype(np.float32) / 255.0
        padded = padded.transpose(2, 0, 1)  # HWC → CHW

        processed.append(padded)

    return np.stack(processed)
```

**Tại sao 640x640?**
- YOLOv10 standard input size
- Balance accuracy vs speed
- 32-stride alignment (640 = 20 * 32)

#### 3.3.2 Layout Types

```python
# Lines 34-46
labels = [
    "_background_",    # 0: Background (ignored)
    "Text",            # 1: Body text paragraphs
    "Title",           # 2: Section/document titles
    "Figure",          # 3: Images, diagrams, charts
    "Figure caption",  # 4: Text describing figures
    "Table",           # 5: Data tables
    "Table caption",   # 6: Text describing tables
    "Header",          # 7: Page headers
    "Footer",          # 8: Page footers
    "Reference",       # 9: Bibliography, citations
    "Equation",        # 10: Mathematical equations
]
```

### 3.4 Table Structure Recognizer

**File**: `/deepdoc/vision/table_structure_recognizer.py`
**Lines**: 30-613

#### 3.4.1 Table Grid Construction

```python
# Lines 172-349
@staticmethod
def construct_table(boxes, is_english=False, html=True, **kwargs):
    """
    Construct 2D table from detected components.
    """
    # Step 1: Sort by row
    boxes = Recognizer.sort_R_firstly(boxes, rowh/2)

    # Step 2: Group into rows
    rows = []
    current_row = [boxes[0]]
    for box in boxes[1:]:
        if box["top"] - current_row[-1]["bottom"] > rowh/2:
            rows.append(current_row)
            current_row = [box]
        else:
            current_row.append(box)
    rows.append(current_row)

    # Step 3: Sort each row by column
    for row in rows:
        row.sort(key=lambda x: x["x0"])

    # Step 4: Build 2D matrix
    n_cols = max(len(row) for row in rows)
    table = [[None] * n_cols for _ in range(len(rows))]

    for i, row in enumerate(rows):
        for j, cell in enumerate(row):
            table[i][j] = cell["text"]

    # Step 5: Generate output
    if html:
        return generate_html_table(table)
    else:
        return generate_descriptive_text(table)
```

#### 3.4.2 Spanning Cell Handling

```python
# Lines 496-575
def __cal_spans(self, boxes):
    """
    Calculate colspan and rowspan for merged cells.
    """
    for box in boxes:
        if "SP" not in box:  # Not a spanning cell
            continue

        # Find which rows this cell spans
        box["rowspan"] = []
        for i, row_box in enumerate(self.rows):
            if self.overlapped_area(box, row_box) > 0.3:
                box["rowspan"].append(i)

        # Find which columns this cell spans
        box["colspan"] = []
        for j, col_box in enumerate(self.cols):
            if self.overlapped_area(box, col_box) > 0.3:
                box["colspan"].append(j)
```

---

## 4. Giải Thích Kỹ Thuật

### 4.1 ONNX Runtime

**ONNX là gì?**
- Open Neural Network Exchange
- Format chuẩn cho deep learning models
- Chạy trên nhiều hardware (CPU, GPU, NPU)

**Tại sao dùng ONNX?**
```python
# Không cần PyTorch/TensorFlow runtime
# Lightweight inference
import onnxruntime as ort

session = ort.InferenceSession("model.onnx")
output = session.run(None, {"input": input_data})
```

**Cấu hình trong DeepDoc**:
```python
# vision/ocr.py, lines 96-127
options = ort.SessionOptions()
options.enable_cpu_mem_arena = False     # Giảm memory fragmentation
options.execution_mode = ort.ExecutionMode.ORT_SEQUENTIAL
options.intra_op_num_threads = 2         # Threads per operator
options.inter_op_num_threads = 2         # Parallel operators

# GPU configuration
if torch.cuda.is_available():
    providers = [
        ('CUDAExecutionProvider', {
            'device_id': device_id,
            'gpu_mem_limit': 2 * 1024 * 1024 * 1024,  # 2GB
        })
    ]
```

### 4.2 CTC Decoding

**CTC (Connectionist Temporal Classification)**:
- Giải quyết alignment problem trong sequence-to-sequence
- Không cần biết vị trí chính xác của từng ký tự

**Ví dụ**:
```
OCR Model Output (time steps):
[a, a, a, -, l, l, -, p, p, h, h, a, -]

CTC Decoding:
1. Merge consecutive duplicates: [a, -, l, -, p, h, a, -]
2. Remove blank tokens (-): [a, l, p, h, a]
3. Result: "alpha"
```

**Implementation**:
```python
# vision/postprocess.py, lines 355-366
def __call__(self, preds, label=None):
    # Get most probable character at each position
    preds_idx = preds.argmax(axis=2)  # Shape: (batch, time)
    preds_prob = preds.max(axis=2)     # Confidence scores

    # Decode with deduplication
    text = self.decode(preds_idx, preds_prob, is_remove_duplicate=True)

    return text
```

### 4.3 Non-Maximum Suppression (NMS)

**NMS là gì?**
- Loại bỏ duplicate detections
- Giữ lại box có confidence cao nhất

**Algorithm**:
```
1. Sort boxes by confidence (descending)
2. Pick box with highest score → add to results
3. Remove boxes with IoU > threshold (e.g., 0.5)
4. Repeat until no boxes remain
```

**Implementation**:
```python
# vision/operators.py, lines 702-725
def nms(bboxes, scores, iou_thresh):
    indices = []
    index = scores.argsort()[::-1]  # Sort descending

    while index.size > 0:
        i = index[0]
        indices.append(i)

        # Compute IoU with remaining boxes
        ious = compute_iou(bboxes[i], bboxes[index[1:]])

        # Keep only boxes with IoU <= threshold
        mask = ious <= iou_thresh
        index = index[1:][mask]

    return indices
```

### 4.4 DBNet (Differentiable Binarization)

**DBNet là gì?**
- Text detection network
- Tạo probability map + threshold map
- Differentiable binarization cho end-to-end training

**Pipeline**:
```
Image → CNN Backbone → Feature Map →
                                    ├→ Probability Map (text regions)
                                    └→ Threshold Map (adaptive threshold)

Final = Probability > Threshold (pixel-wise)
```

**Post-processing**:
```python
# vision/postprocess.py, DBPostProcess
def __call__(self, outs_dict, shape_list):
    pred = outs_dict["maps"]

    # Binary thresholding
    bitmap = pred > self.thresh  # 0.3

    # Find contours
    contours = cv2.findContours(bitmap, cv2.RETR_LIST, cv2.CHAIN_APPROX_SIMPLE)

    # Unclip (expand) boxes
    for contour in contours:
        box = self.unclip(contour, self.unclip_ratio)  # 1.5x expansion
        boxes.append(box)
```

### 4.5 K-Means cho Column Detection

**Tại sao K-Means?**
- Text boxes trong cùng cột có X coordinate tương tự
- K-Means cluster các X values
- Silhouette score chọn số cột tối ưu

**Silhouette Score**:
```
s(i) = (b(i) - a(i)) / max(a(i), b(i))

- a(i): Average distance to same cluster
- b(i): Average distance to nearest other cluster
- Range: [-1, 1], higher = better clustering
```

**Ví dụ**:
```
Page with 2 columns:
Left column boxes: x0 = [50, 52, 48, 51, ...]
Right column boxes: x0 = [400, 398, 402, 399, ...]

K-Means (k=2):
- Cluster 0: x0 ≈ 50 (left column)
- Cluster 1: x0 ≈ 400 (right column)

Silhouette score ≈ 0.95 (high, good separation)
```

---

## 5. Lý Do Thiết Kế

### 5.1 Tại Sao Dùng Multiple Models?

**Vấn đề**: Một model không thể handle tất cả tasks

| Task | Model Type | Lý Do |
|------|------------|-------|
| Text Detection | DBNet | Specialized cho text regions |
| Text Recognition | CRNN | Sequential text với CTC |
| Layout Detection | YOLOv10 | Object detection tốt nhất |
| Table Structure | YOLOv10 variant | Fine-tuned cho table elements |

**Trade-off**:
- Pros: Mỗi model optimized cho task riêng
- Cons: Nhiều models → nhiều memory, complexity

### 5.2 Tại Sao Dùng XGBoost cho Text Merging?

**Vấn đề**: Merge text blocks là decision phức tạp

**Rule-based approach** (naive):
```python
# Simple heuristics
if y_distance < threshold and same_column:
    merge()
# ❌ Không handle edge cases tốt
```

**ML approach** (XGBoost):
```python
# 31 features capturing various signals
features = [
    y_distance / char_height,      # Distance feature
    ends_with_punctuation,          # Text pattern
    same_layout_type,               # Layout feature
    font_size_ratio,                # Typography
    ...
]
# ✅ Learns complex patterns from data
```

**Tại sao XGBoost?**
- Fast inference (tree-based)
- Handles mixed feature types well
- Pre-trained model included

### 5.3 Tại Sao ONNX thay vì PyTorch/TensorFlow?

| Aspect | ONNX Runtime | PyTorch |
|--------|--------------|---------|
| Size | ~50MB | ~500MB+ |
| Memory | Lower | Higher |
| Startup | Fast | Slow (JIT) |
| Dependencies | Minimal | Many |
| Multi-platform | Yes | Limited |

**DeepDoc choice**: ONNX cho production deployment
- Không cần PyTorch runtime
- Lighter memory footprint
- Faster cold start

### 5.4 Tại Sao Zoomin = 3?

**Experiment results**:
```
zoomin=1: OCR accuracy ~70%, fast
zoomin=2: OCR accuracy ~85%, moderate
zoomin=3: OCR accuracy ~95%, acceptable speed ← chosen
zoomin=4: OCR accuracy ~97%, slow
zoomin=5: OCR accuracy ~98%, very slow, memory issues
```

**Balance**: 3x là sweet spot giữa accuracy và resource usage

### 5.5 Tại Sao Hybrid Text Extraction?

**Native PDF text** (pdfplumber):
- Pros: Accurate, fast, preserves fonts
- Cons: Không có cho scanned PDFs

**OCR text**:
- Pros: Works on any image
- Cons: Slower, potential errors

**Hybrid approach**:
```python
# Prefer native text, fallback to OCR
for box in ocr_boxes:
    # Try to match with native characters
    matched_chars = find_overlapping_chars(box, native_chars)

    if matched_chars:
        box["text"] = "".join(matched_chars)  # Use native
    else:
        box["text"] = ocr_result  # Use OCR
```

### 5.6 Pipeline vs End-to-End Model

**End-to-End** (e.g., Donut, Pix2Struct):
- Single model: Image → Structured output
- Pros: Simple, unified
- Cons: Less accurate on specific tasks, hard to debug

**Pipeline** (DeepDoc's choice):
- Multiple specialized models
- Pros:
  - Each model optimized for task
  - Easy to debug/improve individual components
  - Mix and match different models
- Cons:
  - More complexity
  - Potential error accumulation

**DeepDoc's rationale**: Pipeline cho flexibility và accuracy

---

## 6. Thuật Ngữ Khó

### 6.1 Computer Vision Terms

| Term | Definition | Ví Dụ trong DeepDoc |
|------|------------|---------------------|
| **Bounding Box** | Hình chữ nhật bao quanh object | `[x0, y0, x1, y1]` coordinates |
| **IoU** | Intersection over Union - đo overlap | NMS threshold 0.5 |
| **NMS** | Non-Maximum Suppression | Loại duplicate detections |
| **Anchor** | Predefined box sizes | YOLOv10 anchors |
| **Stride** | Downsampling factor | 32 trong YOLOv10 |
| **FPN** | Feature Pyramid Network | Multi-scale detection |

### 6.2 OCR Terms

| Term | Definition | Ví Dụ trong DeepDoc |
|------|------------|---------------------|
| **CTC** | Connectionist Temporal Classification | CRNN output decoding |
| **CRNN** | CNN + RNN | Text recognition model |
| **DBNet** | Differentiable Binarization | Text detection model |
| **Unclip** | Expand polygon boundary | 1.5x expansion ratio |

### 6.3 ML Terms

| Term | Definition | Ví Dụ trong DeepDoc |
|------|------------|---------------------|
| **ONNX** | Open Neural Network Exchange | Model format |
| **Inference** | Running model on input | `session.run()` |
| **Batch** | Multiple inputs processed together | batch_size=16 |
| **Confidence** | Model's certainty score | 0.0 - 1.0 |

### 6.4 Document Processing Terms

| Term | Definition | Ví Dụ trong DeepDoc |
|------|------------|---------------------|
| **Layout** | Document structure | Text, Table, Figure |
| **TSR** | Table Structure Recognition | Row, Column detection |
| **Spanning Cell** | Merged table cell | colspan, rowspan |
| **Reading Order** | Text flow sequence | Top-to-bottom, left-to-right |

---

## 7. Mở Rộng Từ Code

### 7.1 Thêm Parser Mới

**Ví dụ**: Add RTF parser

```python
# deepdoc/parser/rtf_parser.py
from striprtf.striprtf import rtf_to_text

class RAGFlowRtfParser:
    def __call__(self, fnm, binary=None, chunk_token_num=128):
        if binary:
            content = binary.decode('utf-8')
        else:
            with open(fnm, 'r') as f:
                content = f.read()

        text = rtf_to_text(content)

        # Chunk text
        chunks = self._chunk(text, chunk_token_num)

        return [(chunk, f"rtf_chunk_{i}") for i, chunk in enumerate(chunks)]
```

### 7.2 Thêm Layout Type Mới

**Ví dụ**: Add "Code Block" layout

```python
# vision/layout_recognizer.py
labels = [
    "_background_",
    "Text",
    "Title",
    ...
    "Code Block",  # New label (index 11)
]

# Train new YOLOv10 model with "Code Block" annotations
# Update model file
```

### 7.3 Custom Text Merging Logic

```python
# Override default merging behavior
class CustomPdfParser(RAGFlowPdfParser):
    def _should_merge(self, box1, box2):
        """Custom merge logic"""
        # Don't merge code blocks
        if box1.get("layout_type") == "Code Block":
            return False

        # Use default logic otherwise
        return super()._should_merge(box1, box2)
```

### 7.4 Thêm Output Format

```python
# Add Markdown output format
def to_markdown(self, documents, tables):
    md_parts = []

    for text, pos_tag in documents:
        # Detect if title
        if self._is_title(text):
            md_parts.append(f"## {text}\n")
        else:
            md_parts.append(f"{text}\n\n")

    # Convert tables to markdown
    for table in tables:
        md_table = html_to_markdown(table["html"])
        md_parts.append(md_table)

    return "\n".join(md_parts)
```

### 7.5 Optimize Performance

**GPU Batching**:
```python
# Process multiple pages in parallel
def _parallel_ocr(self, images, batch_size=4):
    with ThreadPoolExecutor(max_workers=4) as executor:
        futures = []
        for batch in chunks(images, batch_size):
            future = executor.submit(self.ocr, batch)
            futures.append(future)

        results = [f.result() for f in futures]
    return results
```

**Caching**:
```python
# Cache model instances
_model_cache = {}

def get_ocr_model(model_dir, device_id):
    key = f"{model_dir}_{device_id}"
    if key not in _model_cache:
        _model_cache[key] = OCR(model_dir, device_id)
    return _model_cache[key]
```

### 7.6 Integration với RAG Pipeline

```python
# rag/app/pdf.py (example integration)
from deepdoc.parser import RAGFlowPdfParser

def process_pdf_for_rag(file_path, chunk_size=512):
    parser = RAGFlowPdfParser()

    # Parse PDF
    documents, tables = parser(file_path)

    # Chunk documents
    chunks = []
    for text, pos_tag in documents:
        for chunk in chunk_text(text, chunk_size):
            chunks.append({
                "text": chunk,
                "metadata": {"position": pos_tag}
            })

    # Add tables as separate chunks
    for table in tables:
        chunks.append({
            "text": table["html"],
            "metadata": {"type": "table", "bbox": table["bbox"]}
        })

    return chunks
```

---

## 8. Tổng Kết

### 8.1 Key Takeaways

1. **DeepDoc = Parser Layer + Vision Layer**
   - Parser: Format-specific handling (PDF, DOCX, etc.)
   - Vision: OCR + Layout + Table recognition

2. **Pipeline Architecture**
   - Multiple specialized models
   - Easy to debug and improve

3. **ONNX Runtime**
   - Lightweight inference
   - Cross-platform compatibility

4. **Hybrid Text Extraction**
   - Native PDF text khi available
   - OCR fallback cho scanned documents

### 8.2 Diagram Tổng Hợp

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                            DEEPDOC SUMMARY                                    │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                               │
│  INPUT                   PROCESSING                         OUTPUT            │
│  ─────                   ──────────                         ──────            │
│                                                                               │
│  ┌─────────┐     ┌────────────────────────────┐      ┌─────────────────┐    │
│  │  PDF    │────▶│  1. Image Extraction       │─────▶│  Documents      │    │
│  │  DOCX   │     │  2. OCR (DBNet + CRNN)     │      │  [(text, pos)]  │    │
│  │  Excel  │     │  3. Layout (YOLOv10)       │      │                 │    │
│  │  HTML   │     │  4. Column Detection       │      │  Tables         │    │
│  │  ...    │     │  5. Table Structure        │      │  [html, bbox]   │    │
│  └─────────┘     │  6. Text Merging           │      │                 │    │
│                  │  7. Quality Filtering      │      │  Figures        │    │
│                  └────────────────────────────┘      │  [image, cap]   │    │
│                                                       └─────────────────┘    │
│                                                                               │
│  MODELS USED:                                                                 │
│  ────────────                                                                 │
│  • DBNet (Text Detection)          - ONNX, ~30MB                             │
│  • CRNN (Text Recognition)         - ONNX, ~20MB                             │
│  • YOLOv10 (Layout Detection)      - ONNX, ~50MB                             │
│  • YOLOv10 (Table Structure)       - ONNX, ~50MB                             │
│  • XGBoost (Text Merging)          - Binary, ~5MB                            │
│                                                                               │
│  KEY ALGORITHMS:                                                              │
│  ───────────────                                                              │
│  • CTC Decoding (text recognition)                                           │
│  • NMS (duplicate removal)                                                   │
│  • K-Means (column detection)                                                │
│  • IoU (overlap calculation)                                                 │
│                                                                               │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 8.3 Files Reference

| File | Lines | Description |
|------|-------|-------------|
| `parser/pdf_parser.py` | 1479 | Main PDF parser |
| `vision/ocr.py` | 752 | OCR detection + recognition |
| `vision/layout_recognizer.py` | 457 | Layout detection |
| `vision/table_structure_recognizer.py` | 613 | Table structure |
| `vision/recognizer.py` | 443 | Base recognizer class |
| `vision/operators.py` | 726 | Image preprocessing |
| `vision/postprocess.py` | 371 | Post-processing utilities |

---

*Document created for RAGFlow v0.22.1 analysis*
