# Layout & Table Recognition Deep Dive

## Tổng Quan

Sau khi OCR extract được text boxes, DeepDoc cần:
1. **Layout Recognition**: Phân loại vùng (Text, Title, Table, Figure...)
2. **Table Structure Recognition**: Nhận dạng cấu trúc bảng (rows, columns, cells)

## File Structure

```
deepdoc/vision/
├── layout_recognizer.py              # Layout detection (457 lines)
├── table_structure_recognizer.py     # Table structure (613 lines)
└── recognizer.py                     # Base class (443 lines)
```

---

## 1. Layout Recognition (YOLOv10)

### 1.1 Layout Categories

```python
# deepdoc/vision/layout_recognizer.py, lines 34-46

labels = [
    "_background_",     # 0: Background (ignored)
    "Text",             # 1: Body text paragraphs
    "Title",            # 2: Section/document titles
    "Figure",           # 3: Images, diagrams, charts
    "Figure caption",   # 4: Text describing figures
    "Table",            # 5: Data tables
    "Table caption",    # 6: Text describing tables
    "Header",           # 7: Page headers
    "Footer",           # 8: Page footers
    "Reference",        # 9: Bibliography, citations
    "Equation",         # 10: Mathematical equations
]
```

### 1.2 YOLOv10 Architecture

```
YOLOv10 for Document Layout:

Input Image (640, 640, 3)
         │
         ▼
┌─────────────────────────────────────┐
│        CSPDarknet Backbone          │
│  ┌─────┐  ┌─────┐  ┌─────┐  ┌─────┐│
│  │ P1  │→ │ P2  │→ │ P3  │→ │ P4  ││
│  │/2   │  │/4   │  │/8   │  │/16  ││
│  └─────┘  └─────┘  └─────┘  └─────┘│
└─────────────────────────────────────┘
         │      │      │      │
         ▼      ▼      ▼      ▼
┌─────────────────────────────────────┐
│           PANet Neck                 │
│  FPN (top-down) + PAN (bottom-up)   │
│  Multi-scale feature fusion         │
└─────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────┐
│       Detection Heads (3 scales)     │
│  Small (80x80) → tiny objects       │
│  Medium (40x40) → normal objects    │
│  Large (20x20) → big objects        │
└─────────────────────────────────────┘
         │
         ▼
    Raw Predictions:
    [x_center, y_center, width, height, confidence, class_probs...]
```

### 1.3 Preprocessing (LayoutRecognizer4YOLOv10)

```python
# deepdoc/vision/layout_recognizer.py, lines 186-209

def preprocess(self, image_list):
    """
    Preprocess images for YOLOv10.

    Key steps:
    1. Resize maintaining aspect ratio
    2. Pad to 640x640 (gray borders)
    3. Normalize [0,255] → [0,1]
    4. Transpose HWC → CHW
    """
    processed = []
    scale_factors = []

    for img in image_list:
        h, w = img.shape[:2]

        # Calculate scale (preserve aspect ratio)
        r = min(640/h, 640/w)
        new_h, new_w = int(h*r), int(w*r)

        # Resize
        resized = cv2.resize(img, (new_w, new_h))

        # Calculate padding
        pad_top = (640 - new_h) // 2
        pad_left = (640 - new_w) // 2

        # Pad to 640x640 (gray: 114)
        padded = np.full((640, 640, 3), 114, dtype=np.uint8)
        padded[pad_top:pad_top+new_h, pad_left:pad_left+new_w] = resized

        # Normalize and transpose
        padded = padded.astype(np.float32) / 255.0
        padded = padded.transpose(2, 0, 1)  # HWC → CHW

        processed.append(padded)
        scale_factors.append([1/r, 1/r, pad_left, pad_top])

    return np.stack(processed), scale_factors
```

**Visualization**:
```
Original image (1000x800):
┌────────────────────────────────────────┐
│                                        │
│         Document Content               │
│                                        │
└────────────────────────────────────────┘

After resize (scale=0.64) to (640x512):
┌────────────────────────────────────────┐
│                                        │
│         Document Content               │
│                                        │
└────────────────────────────────────────┘

After padding to (640x640):
┌────────────────────────────────────────┐
│░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░│ ← 64px gray padding
├────────────────────────────────────────┤
│                                        │
│         Document Content               │
│                                        │
├────────────────────────────────────────┤
│░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░│ ← 64px gray padding
└────────────────────────────────────────┘
```

### 1.4 NMS Postprocessing

```python
# deepdoc/vision/recognizer.py, lines 330-407

def postprocess(self, boxes, inputs, thr):
    """
    YOLOv10 postprocessing with per-class NMS.
    """
    results = []

    for batch_idx, batch_boxes in enumerate(boxes):
        scale_factor = inputs["scale_factor"][batch_idx]

        # Filter by confidence threshold
        mask = batch_boxes[:, 4] > thr  # confidence > 0.2
        filtered = batch_boxes[mask]

        if len(filtered) == 0:
            results.append([])
            continue

        # Convert xywh → xyxy
        xyxy = self.xywh2xyxy(filtered[:, :4])

        # Remove padding offset
        xyxy[:, [0, 2]] -= scale_factor[2]  # pad_left
        xyxy[:, [1, 3]] -= scale_factor[3]  # pad_top

        # Scale back to original size
        xyxy[:, [0, 2]] *= scale_factor[0]  # scale_x
        xyxy[:, [1, 3]] *= scale_factor[1]  # scale_y

        # Per-class NMS
        class_ids = filtered[:, 5].astype(int)
        scores = filtered[:, 4]

        keep_indices = []
        for cls in np.unique(class_ids):
            cls_mask = class_ids == cls
            cls_boxes = xyxy[cls_mask]
            cls_scores = scores[cls_mask]

            # NMS within class
            keep = self.iou_filter(cls_boxes, cls_scores, iou_thresh=0.45)
            keep_indices.extend(np.where(cls_mask)[0][keep])

        # Build result
        batch_results = []
        for idx in keep_indices:
            batch_results.append({
                "type": self.labels[int(filtered[idx, 5])],
                "bbox": xyxy[idx].tolist(),
                "score": float(filtered[idx, 4])
            })

        results.append(batch_results)

    return results
```

### 1.5 OCR-Layout Association

```python
# deepdoc/vision/layout_recognizer.py, lines 98-147

def __call__(self, image_list, ocr_res, scale_factor=3, thr=0.2, batch_size=16, drop=True):
    """
    Detect layouts and associate with OCR results.
    """
    # Step 1: Run layout detection
    page_layouts = super().__call__(image_list, thr, batch_size)

    # Step 2: Clean up overlapping layouts
    for i, layouts in enumerate(page_layouts):
        page_layouts[i] = self.layouts_cleanup(layouts, thr=0.7)

    # Step 3: Associate OCR boxes with layouts
    for page_idx, (ocr_boxes, layouts) in enumerate(zip(ocr_res, page_layouts)):
        # Sort layouts by priority: Footer → Header → Reference → Caption → Others
        layouts_by_priority = self._sort_by_priority(layouts)

        for ocr_box in ocr_boxes:
            # Find overlapping layout
            matched_layout = self.find_overlapped_with_threshold(
                ocr_box,
                layouts_by_priority,
                thr=0.4  # 40% overlap threshold
            )

            if matched_layout:
                ocr_box["layout_type"] = matched_layout["type"]
                ocr_box["layoutno"] = matched_layout.get("layoutno", 0)
            else:
                ocr_box["layout_type"] = "Text"  # Default to Text

    # Step 4: Filter garbage (headers, footers, page numbers)
    if drop:
        self._filter_garbage(ocr_res, page_layouts)

    return ocr_res, page_layouts
```

### 1.6 Garbage Detection

```python
# deepdoc/vision/layout_recognizer.py, lines 64-66

# Patterns to filter out
garbage_patterns = [
    r"^•+$",                        # Bullet points only
    r"^[0-9]{1,2} / ?[0-9]{1,2}$",  # Page numbers (3/10, 3 / 10)
    r"^[0-9]{1,2} of [0-9]{1,2}$",  # Page numbers (3 of 10)
    r"^http://[^ ]{12,}",           # Long URLs
    r"\(cid *: *[0-9]+ *\)",        # PDF character IDs
]

def is_garbage(text, layout_type, page_position):
    """
    Determine if text should be filtered out.

    Rules:
    - Headers at top 10% of page → keep
    - Footers at bottom 10% of page → keep
    - Headers/footers elsewhere → garbage
    - Page numbers → garbage
    - URLs → garbage
    """
    for pattern in garbage_patterns:
        if re.match(pattern, text):
            return True

    # Position-based filtering
    if layout_type == "Header" and page_position > 0.1:
        return True  # Header not at top
    if layout_type == "Footer" and page_position < 0.9:
        return True  # Footer not at bottom

    return False
```

---

## 2. Table Structure Recognition

### 2.1 Table Components

```python
# deepdoc/vision/table_structure_recognizer.py, lines 31-38

labels = [
    "table",                      # 0: Whole table boundary
    "table column",               # 1: Column separators
    "table row",                  # 2: Row separators
    "table column header",        # 3: Header rows
    "table projected row header", # 4: Row labels
    "table spanning cell",        # 5: Merged cells
]
```

### 2.2 Detection to Grid Construction

```
Detection Output → Table Grid:

┌─────────────────────────────────────────────────────────────────┐
│                        Raw Detections                            │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │ table: [0, 0, 500, 300]                                  │   │
│  │ table row: [0, 0, 500, 50], [0, 50, 500, 100], ...       │   │
│  │ table column: [0, 0, 150, 300], [150, 0, 300, 300], ...  │   │
│  │ table spanning cell: [0, 100, 300, 150]                  │   │
│  └──────────────────────────────────────────────────────────┘   │
│                              │                                   │
│                              ▼                                   │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    Alignment                              │   │
│  │  • Align row boundaries (left/right edges)               │   │
│  │  • Align column boundaries (top/bottom edges)            │   │
│  └──────────────────────────────────────────────────────────┘   │
│                              │                                   │
│                              ▼                                   │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                  Grid Construction                        │   │
│  │                                                           │   │
│  │  ┌──────────┬──────────┬──────────┐                      │   │
│  │  │ Header 1 │ Header 2 │ Header 3 │  ← Row 0 (header)    │   │
│  │  ├──────────┴──────────┼──────────┤                      │   │
│  │  │   Spanning Cell     │  Cell 3  │  ← Row 1             │   │
│  │  ├──────────┬──────────┼──────────┤                      │   │
│  │  │  Cell 4  │  Cell 5  │  Cell 6  │  ← Row 2             │   │
│  │  └──────────┴──────────┴──────────┘                      │   │
│  │                                                           │   │
│  └──────────────────────────────────────────────────────────┘   │
│                              │                                   │
│                              ▼                                   │
│                   HTML or Descriptive Output                     │
└─────────────────────────────────────────────────────────────────┘
```

### 2.3 Alignment Algorithm

```python
# deepdoc/vision/table_structure_recognizer.py, lines 67-111

def __call__(self, images, thr=0.2):
    """
    Detect and align table structure.
    """
    # Run detection
    detections = super().__call__(images, thr)

    for page_dets in detections:
        rows = [d for d in page_dets if d["label"] == "table row"]
        cols = [d for d in page_dets if d["label"] == "table column"]

        if len(rows) > 4:
            # Align row X coordinates (left edges)
            x0_values = [r["x0"] for r in rows]
            mean_x0 = np.mean(x0_values)
            min_x0 = np.min(x0_values)
            aligned_x0 = min(mean_x0, min_x0 + 0.05 * (max(x0_values) - min_x0))

            for r in rows:
                r["x0"] = aligned_x0

            # Align row X coordinates (right edges)
            x1_values = [r["x1"] for r in rows]
            mean_x1 = np.mean(x1_values)
            max_x1 = np.max(x1_values)
            aligned_x1 = max(mean_x1, max_x1 - 0.05 * (max_x1 - min(x1_values)))

            for r in rows:
                r["x1"] = aligned_x1

        if len(cols) > 4:
            # Similar alignment for column Y coordinates
            # ...
```

**Tại sao cần alignment?**

Detection model có thể cho ra boundaries không perfectly aligned:
```
Before alignment:
Row 1: x0=10, x1=490
Row 2: x0=12, x1=488
Row 3: x0=8, x1=492

After alignment:
Row 1: x0=10, x1=490
Row 2: x0=10, x1=490
Row 3: x0=10, x1=490
```

### 2.4 Grid Construction

```python
# deepdoc/vision/table_structure_recognizer.py, lines 172-349

@staticmethod
def construct_table(boxes, is_english=False, html=True, **kwargs):
    """
    Construct 2D table from detected components.

    Args:
        boxes: OCR boxes with R (row), C (column), SP (spanning) attributes
        is_english: Language hint
        html: Output format (HTML or descriptive text)

    Returns:
        HTML table string or descriptive text
    """
    # Step 1: Extract caption
    caption = ""
    for box in boxes[:]:
        if is_caption(box):
            caption = box["text"]
            boxes.remove(box)

    # Step 2: Sort by row position (R attribute)
    rowh = np.median([b["bottom"] - b["top"] for b in boxes])
    boxes = Recognizer.sort_R_firstly(boxes, rowh / 2)

    # Step 3: Group into rows
    rows = []
    current_row = [boxes[0]]

    for box in boxes[1:]:
        # Same row if Y difference < row_height/2
        if abs(box["R"] - current_row[-1]["R"]) < rowh / 2:
            current_row.append(box)
        else:
            rows.append(current_row)
            current_row = [box]
    rows.append(current_row)

    # Step 4: Sort each row by column position (C attribute)
    for row in rows:
        row.sort(key=lambda x: x["C"])

    # Step 5: Build 2D table matrix
    n_rows = len(rows)
    n_cols = max(len(row) for row in rows)

    table = [[None] * n_cols for _ in range(n_rows)]

    for i, row in enumerate(rows):
        for j, cell in enumerate(row):
            table[i][j] = cell

    # Step 6: Handle spanning cells
    table = handle_spanning_cells(table, boxes)

    # Step 7: Generate output
    if html:
        return generate_html_table(table, caption)
    else:
        return generate_descriptive_text(table, caption)
```

### 2.5 Spanning Cell Handling

```python
# deepdoc/vision/table_structure_recognizer.py, lines 496-575

def __cal_spans(self, boxes, rows, cols):
    """
    Calculate colspan and rowspan for merged cells.

    Spanning cell detection:
    - "SP" attribute indicates merged cell
    - Calculate which rows/cols it covers
    """
    for box in boxes:
        if "SP" not in box:
            continue

        # Find rows this cell spans
        box["rowspan"] = []
        for i, row in enumerate(rows):
            overlap = self.overlapped_area(box, row)
            if overlap > 0.3:  # 30% overlap
                box["rowspan"].append(i)

        # Find columns this cell spans
        box["colspan"] = []
        for j, col in enumerate(cols):
            overlap = self.overlapped_area(box, col)
            if overlap > 0.3:
                box["colspan"].append(j)

    return boxes
```

**Example**:
```
Spanning cell detection:

┌──────────┬──────────┬──────────┐
│ Header 1 │ Header 2 │ Header 3 │
├──────────┴──────────┼──────────┤
│   Merged Cell       │  Cell 3  │  ← SP cell spans columns 0-1
│   (colspan=2)       │          │
├──────────┬──────────┼──────────┤
│  Cell 4  │  Cell 5  │  Cell 6  │
└──────────┴──────────┴──────────┘

Detection:
- SP cell bbox: [0, 50, 300, 100]
- Column 0: [0, 0, 150, 200]  → overlap 0.5 ✓
- Column 1: [150, 0, 300, 200] → overlap 0.5 ✓
- Column 2: [300, 0, 450, 200] → overlap 0.0 ✗
→ colspan = [0, 1]
```

### 2.6 HTML Output Generation

```python
# deepdoc/vision/table_structure_recognizer.py, lines 352-393

def __html_table(table, header_rows, caption):
    """
    Generate HTML table from 2D matrix.
    """
    html_parts = ["<table>"]

    # Add caption if exists
    if caption:
        html_parts.append(f"<caption>{caption}</caption>")

    for i, row in enumerate(table):
        html_parts.append("<tr>")

        for j, cell in enumerate(row):
            if cell is None:
                continue  # Skip cells covered by spanning

            # Determine tag (th for header, td for data)
            tag = "th" if i in header_rows else "td"

            # Add colspan/rowspan attributes
            attrs = []
            if cell.get("colspan") and len(cell["colspan"]) > 1:
                attrs.append(f'colspan="{len(cell["colspan"])}"')
            if cell.get("rowspan") and len(cell["rowspan"]) > 1:
                attrs.append(f'rowspan="{len(cell["rowspan"])}"')

            attr_str = " " + " ".join(attrs) if attrs else ""

            # Add cell content
            html_parts.append(f"<{tag}{attr_str}>{cell['text']}</{tag}>")

        html_parts.append("</tr>")

    html_parts.append("</table>")

    return "\n".join(html_parts)
```

**Output Example**:
```html
<table>
  <caption>Table 1: Sales Data</caption>
  <tr>
    <th>Region</th>
    <th>Q1</th>
    <th>Q2</th>
  </tr>
  <tr>
    <td colspan="2">North America</td>
    <td>$150K</td>
  </tr>
  <tr>
    <td>Europe</td>
    <td>$100K</td>
    <td>$120K</td>
  </tr>
</table>
```

### 2.7 Descriptive Text Output

```python
# deepdoc/vision/table_structure_recognizer.py, lines 396-493

def __desc_table(table, header_rows, caption):
    """
    Generate natural language description of table.

    For RAG, sometimes descriptive text is better than HTML.
    """
    descriptions = []

    # Get headers
    headers = [cell["text"] for cell in table[0]] if header_rows else []

    # Process each data row
    for i, row in enumerate(table):
        if i in header_rows:
            continue

        row_desc = []
        for j, cell in enumerate(row):
            if cell is None:
                continue

            if headers and j < len(headers):
                # "Column Name: Value" format
                row_desc.append(f"{headers[j]}: {cell['text']}")
            else:
                row_desc.append(cell['text'])

        if row_desc:
            descriptions.append("; ".join(row_desc))

    # Add source reference
    if caption:
        descriptions.append(f'(from "{caption}")')

    return "\n".join(descriptions)
```

**Output Example**:
```
Region: North America; Q1: $100K; Q2: $150K
Region: Europe; Q1: $80K; Q2: $120K
(from "Table 1: Sales Data")
```

---

## 3. Cell Content Classification

### 3.1 Block Type Detection

```python
# deepdoc/vision/table_structure_recognizer.py, lines 121-149

@staticmethod
def blockType(text):
    """
    Classify cell content type.

    Used for:
    - Header detection (non-numeric cells likely headers)
    - Data validation
    - Smart formatting
    """
    patterns = {
        "Dt": r"(^[0-9]{4}[-/][0-9]{1,2}|[0-9]{1,2}[-/][0-9]{1,2}[-/][0-9]{2,4}|"
              r"[0-9]{1,2}月|[Q][1-4]|[一二三四]季度)",  # Date
        "Nu": r"^[-+]?[0-9.,%%￥$€£¥]+$",  # Number
        "Ca": r"^[A-Z0-9]{4,}$",  # Code
        "En": r"^[a-zA-Z\s]+$",  # English
    }

    for type_name, pattern in patterns.items():
        if re.search(pattern, text):
            return type_name

    # Classify by length
    tokens = text.split()
    if len(tokens) == 1:
        return "Sg"  # Single
    elif len(tokens) <= 3:
        return "Tx"  # Short text
    elif len(tokens) <= 12:
        return "Lx"  # Long text
    else:
        return "Ot"  # Other

# Examples:
# "2023-01-15" → "Dt" (Date)
# "$1,234.56" → "Nu" (Number)
# "ABC123" → "Ca" (Code)
# "Total Revenue" → "En" (English)
# "北京市" → "Tx" (Text)
```

### 3.2 Header Detection

```python
# deepdoc/vision/table_structure_recognizer.py, lines 332-344

def detect_headers(table):
    """
    Detect which rows are headers based on content type.

    Heuristic: If >50% of cells in a row are non-numeric,
    it's likely a header row.
    """
    header_rows = set()

    for i, row in enumerate(table):
        non_numeric = 0
        total = 0

        for cell in row:
            if cell is None:
                continue
            total += 1
            if blockType(cell["text"]) != "Nu":
                non_numeric += 1

        if total > 0 and non_numeric / total > 0.5:
            header_rows.add(i)

    return header_rows
```

---

## 4. Integration với PDF Parser

### 4.1 Table Detection in PDF Pipeline

```python
# deepdoc/parser/pdf_parser.py, lines 196-281

def _table_transformer_job(self, zoomin=3):
    """
    Detect and structure tables using TableStructureRecognizer.
    """
    # Find table layouts
    table_layouts = [
        layout for layout in self.page_layout
        if layout["type"] == "Table"
    ]

    if not table_layouts:
        return

    # Crop table images
    table_images = []
    for layout in table_layouts:
        x0, y0, x1, y1 = layout["bbox"]
        img = self.page_images[layout["page"]][
            int(y0*zoomin):int(y1*zoomin),
            int(x0*zoomin):int(x1*zoomin)
        ]
        table_images.append(img)

    # Run TSR
    table_structures = self.tsr(table_images)

    # Match OCR boxes to table structure
    for layout, structure in zip(table_layouts, table_structures):
        # Get OCR boxes within table region
        table_boxes = [
            box for box in self.boxes
            if self._box_in_region(box, layout["bbox"])
        ]

        # Assign R, C, SP attributes
        for box in table_boxes:
            box["R"] = self._find_row(box, structure["rows"])
            box["C"] = self._find_column(box, structure["columns"])
            if self._is_spanning(box, structure["spanning_cells"]):
                box["SP"] = True

        # Store for later extraction
        self.tb_cpns[layout["id"]] = {
            "boxes": table_boxes,
            "structure": structure
        }
```

### 4.2 Table Extraction

```python
# deepdoc/parser/pdf_parser.py, lines 757-930

def _extract_table_figure(self, need_image, ZM, return_html, need_position):
    """
    Extract tables and figures from detected layouts.
    """
    tables = []

    for layout_id, table_data in self.tb_cpns.items():
        boxes = table_data["boxes"]

        # Construct table (HTML or descriptive)
        if return_html:
            content = TableStructureRecognizer.construct_table(
                boxes, html=True
            )
        else:
            content = TableStructureRecognizer.construct_table(
                boxes, html=False
            )

        table = {
            "content": content,
            "bbox": table_data["bbox"],
        }

        if need_image:
            table["image"] = self._crop_region(table_data["bbox"])

        tables.append(table)

    return tables
```

---

## 5. Performance Considerations

### 5.1 Batch Processing

```python
# deepdoc/vision/recognizer.py, lines 415-437

def __call__(self, image_list, thr=0.7, batch_size=16):
    """
    Batch inference for efficiency.

    Why batch_size=16?
    - GPU memory optimization
    - Balance throughput vs latency
    - Typical document has 10-50 elements
    """
    results = []

    for i in range(0, len(image_list), batch_size):
        batch = image_list[i:i+batch_size]

        # Preprocess
        inputs = self.preprocess(batch)

        # Inference
        outputs = self.ort_sess.run(None, inputs)

        # Postprocess
        batch_results = self.postprocess(outputs, inputs, thr)
        results.extend(batch_results)

    return results
```

### 5.2 Model Caching

```python
# deepdoc/vision/ocr.py, lines 36-73

# Global model cache
loaded_models = {}

def load_model(model_dir, nm, device_id=None):
    """
    Load ONNX model with caching.

    Cache key: model_path + device_id
    """
    model_path = os.path.join(model_dir, f"{nm}.onnx")
    cache_key = f"{model_path}_{device_id}"

    if cache_key in loaded_models:
        return loaded_models[cache_key]

    # Load model...
    session = ort.InferenceSession(model_path, ...)

    loaded_models[cache_key] = (session, run_opts)
    return session, run_opts
```

---

## 6. Troubleshooting

### 6.1 Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| Missing table | Low confidence | Lower threshold (0.1-0.2) |
| Wrong colspan | Misaligned detection | Check row/column alignment |
| Merged cells wrong | Overlap threshold | Adjust SP detection threshold |
| Headers not detected | All numeric | Manual header specification |
| Layout overlap | NMS threshold | Increase NMS IoU threshold |

### 6.2 Debugging

```python
# Visualize layout detection
from deepdoc.vision.seeit import draw_boxes

# Draw layout boxes on image
layout_vis = draw_boxes(
    page_image,
    [(l["bbox"], l["type"]) for l in page_layouts],
    colors={
        "Text": (0, 255, 0),
        "Table": (255, 0, 0),
        "Figure": (0, 0, 255),
    }
)
cv2.imwrite("layout_debug.png", layout_vis)

# Check table structure
for box in table_boxes:
    print(f"Text: {box['text']}")
    print(f"  Row: {box.get('R', 'N/A')}")
    print(f"  Col: {box.get('C', 'N/A')}")
    print(f"  Spanning: {box.get('SP', False)}")
```

---

## 7. References

- YOLOv10 Paper: [YOLOv10: Real-Time End-to-End Object Detection](https://arxiv.org/abs/2405.14458)
- Table Transformer: [PubTables-1M: Towards comprehensive table extraction](https://arxiv.org/abs/2110.00061)
- Document Layout Analysis: [A Survey](https://arxiv.org/abs/2012.15005)
