# PDF Parsing Pipeline

## Tong Quan

RAGFlow PDF parser kết hợp OCR, layout detection, và table structure recognition để extract structured content từ PDFs.

## File Location
```
/deepdoc/parser/pdf_parser.py
```

## Processing Pipeline

```
┌─────────────────────────────────────────────────────────────────┐
│                    PDF PARSING PIPELINE                          │
└─────────────────────────────────────────────────────────────────┘

PDF Binary
    │
    ▼
┌─────────────────────────────────────────────────────────────────┐
│  1. __images__() [0-40%]                                         │
│     ┌─────────────────────────────────────────────────────┐     │
│     │  pdfplumber.open(pdf_binary)                        │     │
│     │  for page in pdf.pages:                             │     │
│     │      img = page.to_image(resolution=72*ZM)          │     │
│     │      images.append(img.original)  # PIL Image       │     │
│     └─────────────────────────────────────────────────────┘     │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  2. __ocr() [40-63%]                                             │
│     ┌─────────────────────────────────────────────────────┐     │
│     │  For each page image:                               │     │
│     │  - PaddleOCR.detect() → text regions                │     │
│     │  - PaddleOCR.recognize() → text content             │     │
│     │  Output: bxs = [{x0, x1, top, bottom, text}, ...]  │     │
│     └─────────────────────────────────────────────────────┘     │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  3. _layouts_rec() [63-83%]                                      │
│     ┌─────────────────────────────────────────────────────┐     │
│     │  Detectron2 layout detection:                       │     │
│     │  - Text, Title, Table, Figure, Header, Footer, etc. │     │
│     │  Tag OCR boxes with layout_type                     │     │
│     └─────────────────────────────────────────────────────┘     │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  4. _table_transformer_job() [Table TSR]                         │
│     ┌─────────────────────────────────────────────────────┐     │
│     │  For tables detected:                               │     │
│     │  - Crop table region                                │     │
│     │  - Run TableStructureRecognizer                     │     │
│     │  - Detect rows, columns, cells                      │     │
│     └─────────────────────────────────────────────────────┘     │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  5. Text Merging Pipeline                                        │
│     ┌─────────────────────────────────────────────────────┐     │
│     │  _text_merge() → Horizontal merge                   │     │
│     │  _assign_column() → KMeans column detection         │     │
│     │  _naive_vertical_merge() → XGBoost vertical merge   │     │
│     │  _final_reading_order_merge() → Reading order       │     │
│     └─────────────────────────────────────────────────────┘     │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  6. _extract_table_figure() [83-100%]                            │
│     ┌─────────────────────────────────────────────────────┐     │
│     │  - Separate tables/figures from text                │     │
│     │  - Find and associate captions                      │     │
│     │  - Crop images for tables/figures                   │     │
│     │  - Convert table structure to natural language      │     │
│     └─────────────────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────────────────┘
```

## RAGFlowPdfParser Class

```python
class RAGFlowPdfParser:
    ZM = 3  # Zoom factor for image extraction

    def __init__(self):
        self.ocr = OCR()
        self.layout_recognizer = LayoutRecognizer()
        self.tsr = TableStructureRecognizer()

    def parse_into_bboxes(self, filename, callback=None):
        """
        Main parsing method.

        Returns:
            List of text boxes with layout information
        """
        # 1. Extract images
        self.__images__(filename, callback, 0, 0.4)

        # 2. OCR detection
        self.__ocr(callback, 0.4, 0.63)

        # 3. Layout recognition
        self._layouts_rec(callback, 0.63, 0.83)

        # 4. Table structure recognition
        self._table_transformer_job()

        # 5. Text merging
        self._text_merge()
        self._assign_column()
        self._naive_vertical_merge()
        self._final_reading_order_merge()

        # 6. Extract tables/figures
        return self._extract_table_figure(callback, 0.83, 1.0)
```

## Image Extraction

```python
def __images__(self, filename, callback, start_progress, end_progress):
    """
    Extract page images from PDF.
    """
    self.pdf = pdfplumber.open(filename)
    self.page_images = []
    self.page_cum_heights = [0]

    total = len(self.pdf.pages)

    for i, page in enumerate(self.pdf.pages):
        # Convert to image with 3x zoom
        img = page.to_image(resolution=72 * self.ZM)
        self.page_images.append(img.original)

        # Track cumulative heights for coordinate mapping
        self.page_cum_heights.append(
            self.page_cum_heights[-1] + page.height * self.ZM
        )

        # Progress callback
        if callback:
            progress = start_progress + (end_progress - start_progress) * (i / total)
            callback(progress, f"Extracting page {i+1}/{total}")
```

## OCR Processing

```python
def __ocr(self, callback, start_progress, end_progress):
    """
    Run OCR on all pages.
    """
    self.bxs = []  # All text boxes

    for page_idx, img in enumerate(self.page_images):
        # Detect text regions
        detections = self.ocr.detect(img)

        if not detections:
            continue

        # Recognize text in regions
        for det in detections:
            x0, y0, x1, y1 = det["box"]
            confidence = det["confidence"]

            # Crop region
            region_img = img.crop((x0, y0, x1, y1))

            # Recognize
            text = self.ocr.recognize(region_img)

            if text.strip():
                self.bxs.append({
                    "x0": x0,
                    "x1": x1,
                    "top": y0 + self.page_cum_heights[page_idx],
                    "bottom": y1 + self.page_cum_heights[page_idx],
                    "text": text,
                    "page_num": page_idx,
                    "confidence": confidence
                })

        # Progress
        if callback:
            progress = start_progress + (end_progress - start_progress) * (page_idx / len(self.page_images))
            callback(progress, f"OCR page {page_idx+1}")
```

## Layout Recognition

```python
def _layouts_rec(self, callback, start_progress, end_progress):
    """
    Detect layout types for text boxes.
    """
    for page_idx, img in enumerate(self.page_images):
        # Run layout detection
        layouts = self.layout_recognizer.detect(img)

        # Tag OCR boxes with layout type
        for layout in layouts:
            lx0, ly0, lx1, ly1 = layout["box"]
            layout_type = layout["type"]  # Text, Title, Table, etc.
            layout_num = layout["num"]

            # Find overlapping OCR boxes
            for bx in self.bxs:
                if bx["page_num"] != page_idx:
                    continue

                # Check overlap
                if self._overlaps(bx, (lx0, ly0, lx1, ly1)):
                    bx["layout_type"] = layout_type
                    bx["layout_num"] = layout_num

        # Progress
        if callback:
            progress = start_progress + (end_progress - start_progress) * (page_idx / len(self.page_images))
            callback(progress, f"Layout detection page {page_idx+1}")
```

## Text Merging

```python
def _text_merge(self):
    """
    Horizontal merge of adjacent boxes with same layout.
    """
    # Sort by position
    self.bxs.sort(key=lambda b: (b["page_num"], b["top"], b["x0"]))

    merged = []
    current = None

    for bx in self.bxs:
        if current is None:
            current = bx
            continue

        # Check if should merge
        if self._should_merge_horizontal(current, bx):
            # Merge
            current["x1"] = bx["x1"]
            current["text"] += " " + bx["text"]
        else:
            merged.append(current)
            current = bx

    if current:
        merged.append(current)

    self.bxs = merged

def _assign_column(self):
    """
    Detect columns using KMeans clustering.
    """
    from sklearn.cluster import KMeans
    from sklearn.metrics import silhouette_score

    # Get X coordinates
    x_coords = np.array([[b["x0"]] for b in self.bxs])

    best_k = 1
    best_score = -1

    # Find optimal number of columns
    for k in range(1, min(5, len(self.bxs))):
        if k >= len(self.bxs):
            break

        km = KMeans(n_clusters=k, random_state=42)
        labels = km.fit_predict(x_coords)

        if k > 1:
            score = silhouette_score(x_coords, labels)
            if score > best_score:
                best_score = score
                best_k = k

    # Assign columns
    km = KMeans(n_clusters=best_k, random_state=42)
    labels = km.fit_predict(x_coords)

    for i, bx in enumerate(self.bxs):
        bx["col_id"] = labels[i]

def _naive_vertical_merge(self):
    """
    Vertical merge using XGBoost model.
    """
    model = load_model("updown_concat_xgb.model")

    merged = []
    current = None

    for bx in self.bxs:
        if current is None:
            current = bx
            continue

        # Extract features
        features = self._extract_merge_features(current, bx)

        # Predict
        prob = model.predict_proba([features])[0][1]

        if prob > 0.5:
            # Merge
            current["bottom"] = bx["bottom"]
            current["text"] += "\n" + bx["text"]
        else:
            merged.append(current)
            current = bx

    if current:
        merged.append(current)

    self.bxs = merged
```

## Merge Features

```python
def _extract_merge_features(self, top_box, bottom_box):
    """
    Extract features for vertical merge decision.

    Returns 36+ features including:
    - Y-distance normalized
    - Same layout number
    - Ending punctuation patterns
    - Beginning character patterns
    - Chinese numbering patterns
    """
    features = []

    # Distance features
    y_dist = bottom_box["top"] - top_box["bottom"]
    char_height = top_box["bottom"] - top_box["top"]
    features.append(y_dist / char_height if char_height > 0 else 0)

    # Layout features
    features.append(1 if top_box.get("layout_num") == bottom_box.get("layout_num") else 0)

    # Text pattern features
    top_text = top_box["text"]
    bottom_text = bottom_box["text"]

    # Ending punctuation
    features.append(1 if top_text.endswith((".", "。", "!", "?", "！", "？")) else 0)
    features.append(1 if top_text.endswith((",", "，", ";", "；")) else 0)

    # Beginning patterns
    features.append(1 if bottom_text[0:1].isupper() else 0)
    features.append(1 if re.match(r"^[一二三四五六七八九十]+、", bottom_text) else 0)
    features.append(1 if re.match(r"^第[一二三四五六七八九十]+章", bottom_text) else 0)

    # ... more features

    return features
```

## Table Extraction

```python
def _extract_table_figure(self, callback, start_progress, end_progress):
    """
    Extract tables and figures with captions.
    """
    results = []

    for bx in self.bxs:
        layout_type = bx.get("layout_type", "text")

        if layout_type == "table":
            # Get table content from TSR
            table_content = self._get_table_content(bx)

            # Find caption
            caption = self._find_caption(bx, "table")

            results.append({
                "type": "table",
                "content": table_content,
                "caption": caption,
                "positions": [(bx["page_num"], bx["x0"], bx["x1"], bx["top"], bx["bottom"])]
            })

        elif layout_type == "figure":
            # Crop figure image
            fig_img = self._crop_region(bx)

            # Find caption
            caption = self._find_caption(bx, "figure")

            results.append({
                "type": "figure",
                "image": fig_img,
                "caption": caption,
                "positions": [(bx["page_num"], bx["x0"], bx["x1"], bx["top"], bx["bottom"])]
            })

        else:
            # Regular text
            results.append({
                "type": "text",
                "content": bx["text"],
                "positions": [(bx["page_num"], bx["x0"], bx["x1"], bx["top"], bx["bottom"])]
            })

    return results

def _get_table_content(self, table_box):
    """
    Convert table structure to natural language.

    Example output:
        "Row 1, Column Name: Value
         Row 2, Column Name: Value"
    """
    # Get TSR results for this table
    tsr_result = self.table_structures.get(table_box["layout_num"])

    if not tsr_result:
        return table_box["text"]

    # Build natural language representation
    lines = []
    for row_idx, row in enumerate(tsr_result["rows"]):
        for col_idx, cell in enumerate(row["cells"]):
            col_name = tsr_result["headers"][col_idx] if col_idx < len(tsr_result["headers"]) else f"Column {col_idx+1}"
            lines.append(f"Row {row_idx+1}, {col_name}: {cell['text']}")

    return "\n".join(lines)
```

## Configuration

```python
# PDF parser configuration
{
    "layout_recognize": "DeepDOC",  # DeepDOC, Plain, Vision
    "ocr_timeout": 60,              # OCR timeout seconds
    "max_page_size": 4096,          # Max image dimension
    "zoom_factor": 3,               # Image zoom for OCR
}
```

## Related Files

- `/deepdoc/parser/pdf_parser.py` - Main parser
- `/deepdoc/vision/ocr.py` - OCR engine
- `/deepdoc/vision/layout_recognizer.py` - Layout detection
- `/deepdoc/vision/table_structure_recognizer.py` - TSR
