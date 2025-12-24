# Layout Detection - Detectron2 Layout Recognition

## Tong Quan

Layout detection la buoc quan trong trong document processing pipeline, giup phan loai cac vung noi dung trong document (text, title, table, figure, etc.). RAGFlow su dung Detectron2-based models va ho tro nhieu backend khac nhau (ONNX, YOLOv10, Ascend NPU).

## File Location
```
/deepdoc/vision/layout_recognizer.py
```

## Architecture

```
                   LAYOUT DETECTION PIPELINE

                      Page Image
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                    LAYOUT RECOGNIZER                             │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Model Options:                                         │   │
│  │  - ONNX (default): layout.onnx                         │   │
│  │  - YOLOv10: layout_yolov10.onnx                        │   │
│  │  - Ascend NPU: layout.om                               │   │
│  │  - TensorRT DLA: External service                      │   │
│  └─────────────────────────────────────────────────────────┘   │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                    DETECTED LAYOUTS                              │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Layout Types:                                          │   │
│  │  • Text          • Table           • Header            │   │
│  │  • Title         • Table caption   • Footer            │   │
│  │  • Figure        • Figure caption  • Reference         │   │
│  │  • Equation                                            │   │
│  └─────────────────────────────────────────────────────────┘   │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                   TAG OCR BOXES                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  For each OCR box:                                      │   │
│  │  1. Find overlapping layout region                      │   │
│  │  2. Assign layout_type and layoutno                     │   │
│  │  3. Filter garbage (headers, footers, page numbers)     │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## Layout Types

| Type | Description | Xu Ly |
|------|-------------|-------|
| Text | Regular body text | Keep as content |
| Title | Section/document titles | Mark as heading |
| Figure | Images, diagrams, charts | Extract image + caption |
| Figure caption | Descriptions below figures | Associate with figure |
| Table | Data tables | Extract structure (TSR) |
| Table caption | Descriptions for tables | Associate with table |
| Header | Page headers | Filter (garbage) |
| Footer | Page footers | Filter (garbage) |
| Reference | Bibliography section | Filter (optional) |
| Equation | Mathematical formulas | Keep as figure |

## Core Implementation

### LayoutRecognizer Class

```python
class LayoutRecognizer(Recognizer):
    """
    Base layout recognizer using ONNX model.

    Inherits from Recognizer base class for model loading
    and inference.
    """

    labels = [
        "_background_",
        "Text",
        "Title",
        "Figure",
        "Figure caption",
        "Table",
        "Table caption",
        "Header",
        "Footer",
        "Reference",
        "Equation",
    ]

    def __init__(self, domain):
        """
        Initialize with model from HuggingFace or local.

        Args:
            domain: Model domain name (e.g., "layout")
        """
        model_dir = os.path.join(
            get_project_base_directory(),
            "rag/res/deepdoc"
        )
        super().__init__(self.labels, domain, model_dir)

        # Layouts to filter out
        self.garbage_layouts = ["footer", "header", "reference"]

        # Optional TensorRT DLA client
        if os.environ.get("TENSORRT_DLA_SVR"):
            self.client = DLAClient(os.environ["TENSORRT_DLA_SVR"])

    def __call__(self, image_list, ocr_res, scale_factor=3, thr=0.2,
                 batch_size=16, drop=True):
        """
        Detect layouts and tag OCR boxes.

        Args:
            image_list: List of page images
            ocr_res: OCR results per page
            scale_factor: Image zoom factor (default 3)
            thr: Confidence threshold
            batch_size: Inference batch size
            drop: Whether to drop garbage layouts

        Returns:
            - ocr_res: OCR boxes with layout tags
            - page_layout: Layout regions per page
        """
```

### Layout Detection Process

```python
def __call__(self, image_list, ocr_res, scale_factor=3, thr=0.2,
             batch_size=16, drop=True):
    """
    Main layout detection and OCR tagging pipeline.
    """
    # 1. Run layout detection
    if self.client:
        # Use TensorRT DLA service
        layouts = self.client.predict(image_list)
    else:
        # Use local ONNX model
        layouts = super().__call__(image_list, thr, batch_size)

    boxes = []
    garbages = {}
    page_layout = []

    # 2. Process each page
    for pn, lts in enumerate(layouts):
        bxs = ocr_res[pn]

        # Convert layout format
        lts = [{
            "type": b["type"],
            "score": float(b["score"]),
            "x0": b["bbox"][0] / scale_factor,
            "x1": b["bbox"][2] / scale_factor,
            "top": b["bbox"][1] / scale_factor,
            "bottom": b["bbox"][-1] / scale_factor,
            "page_number": pn,
        } for b in lts if float(b["score"]) >= 0.4 or
                         b["type"] not in self.garbage_layouts]

        # Sort layouts by Y position
        lts = self.sort_Y_firstly(lts, np.mean([
            lt["bottom"] - lt["top"] for lt in lts
        ]) / 2)

        # Cleanup overlapping layouts
        lts = self.layouts_cleanup(bxs, lts)
        page_layout.append(lts)

        # 3. Tag OCR boxes with layout types
        for lt_type in ["footer", "header", "reference",
                        "figure caption", "table caption",
                        "title", "table", "text", "figure", "equation"]:
            self._findLayout(lt_type, bxs, lts, pn, image_list,
                           scale_factor, garbages, drop)

        # 4. Add unvisited figures
        for i, lt in enumerate([lt for lt in lts
                               if lt["type"] in ["figure", "equation"]]):
            if lt.get("visited"):
                continue
            lt = deepcopy(lt)
            del lt["type"]
            lt["text"] = ""
            lt["layout_type"] = "figure"
            lt["layoutno"] = f"figure-{i}"
            bxs.append(lt)

        boxes.extend(bxs)

    # 5. Remove duplicate garbage text
    garbag_set = set()
    for k in garbages.keys():
        garbages[k] = Counter(garbages[k])
        for g, c in garbages[k].items():
            if c > 1:  # Appears on multiple pages
                garbag_set.add(g)

    ocr_res = [b for b in boxes if b["text"].strip() not in garbag_set]

    return ocr_res, page_layout
```

### Layout-OCR Box Matching

```python
def _findLayout(self, ty, bxs, lts, pn, image_list, scale_factor,
                garbages, drop):
    """
    Find matching layout for each OCR box.

    Process:
    1. Get all layouts of specified type
    2. For each untagged OCR box:
       - Check if it's garbage (page numbers, etc.)
       - Find overlapping layout region
       - Tag with layout type
       - Filter garbage layouts if drop=True
    """
    lts_of_type = [lt for lt in lts if lt["type"] == ty]

    i = 0
    while i < len(bxs):
        # Skip already tagged boxes
        if bxs[i].get("layout_type"):
            i += 1
            continue

        # Check for garbage patterns
        if self._is_garbage(bxs[i]):
            bxs.pop(i)
            continue

        # Find overlapping layout
        ii = self.find_overlapped_with_threshold(bxs[i], lts_of_type, thr=0.4)

        if ii is None:
            # No matching layout
            bxs[i]["layout_type"] = ""
            i += 1
            continue

        lts_of_type[ii]["visited"] = True

        # Check if should keep garbage layout
        keep_feats = [
            lts_of_type[ii]["type"] == "footer" and
                bxs[i]["bottom"] < image_list[pn].size[1] * 0.9 / scale_factor,
            lts_of_type[ii]["type"] == "header" and
                bxs[i]["top"] > image_list[pn].size[1] * 0.1 / scale_factor,
        ]

        if drop and lts_of_type[ii]["type"] in self.garbage_layouts \
                and not any(keep_feats):
            # Collect garbage for deduplication
            garbages.setdefault(lts_of_type[ii]["type"], []).append(
                bxs[i]["text"]
            )
            bxs.pop(i)
            continue

        # Tag box with layout info
        bxs[i]["layoutno"] = f"{ty}-{ii}"
        bxs[i]["layout_type"] = lts_of_type[ii]["type"] \
            if lts_of_type[ii]["type"] != "equation" else "figure"
        i += 1
```

### Garbage Pattern Detection

```python
def _is_garbage(self, b):
    """
    Detect garbage text patterns.

    Patterns:
    - Bullet points only: "•••"
    - Page numbers: "1 / 10", "3 of 15"
    - URLs: "http://..."
    - Font encoding issues: "(cid:123)"
    """
    patt = [
        r"^•+$",                          # Bullet points
        "^[0-9]{1,2} / ?[0-9]{1,2}$",    # Page X / Y
        r"^[0-9]{1,2} of [0-9]{1,2}$",   # Page X of Y
        "^http://[^ ]{12,}",              # URLs
        r"\(cid *: *[0-9]+ *\)",          # Font encoding
    ]
    return any([re.search(p, b["text"]) for p in patt])
```

## YOLOv10 Variant

```python
class LayoutRecognizer4YOLOv10(LayoutRecognizer):
    """
    YOLOv10-based layout recognizer.

    Differences from base:
    - Different label set
    - Custom preprocessing (LetterBox resize)
    - YOLO-specific postprocessing
    """

    labels = [
        "title", "Text", "Reference", "Figure",
        "Figure caption", "Table", "Table caption",
        "Table caption", "Equation", "Figure caption",
    ]

    def preprocess(self, image_list):
        """
        YOLOv10 preprocessing with letterbox resize.
        """
        inputs = []
        new_shape = self.input_shape

        for img in image_list:
            shape = img.shape[:2]  # H, W

            # Scale ratio
            r = min(new_shape[0] / shape[0], new_shape[1] / shape[1])

            # Compute padding
            new_unpad = int(round(shape[1] * r)), int(round(shape[0] * r))
            dw, dh = new_shape[1] - new_unpad[0], new_shape[0] - new_unpad[1]
            dw /= 2
            dh /= 2

            # Resize
            img = cv2.resize(img, new_unpad, interpolation=cv2.INTER_LINEAR)

            # Pad
            top, bottom = int(round(dh - 0.1)), int(round(dh + 0.1))
            left, right = int(round(dw - 0.1)), int(round(dw + 0.1))
            img = cv2.copyMakeBorder(
                img, top, bottom, left, right,
                cv2.BORDER_CONSTANT, value=(114, 114, 114)
            )

            # Normalize
            img = img / 255.0
            img = img.transpose(2, 0, 1)[np.newaxis, :].astype(np.float32)

            inputs.append({
                self.input_names[0]: img,
                "scale_factor": [shape[1] / new_unpad[0],
                                shape[0] / new_unpad[1], dw, dh]
            })

        return inputs

    def postprocess(self, boxes, inputs, thr):
        """
        YOLO-specific postprocessing with NMS.
        """
        thr = 0.08
        boxes = np.squeeze(boxes)

        # Filter by score
        scores = boxes[:, 4]
        boxes = boxes[scores > thr, :]
        scores = scores[scores > thr]

        if len(boxes) == 0:
            return []

        class_ids = boxes[:, -1].astype(int)
        boxes = boxes[:, :4]

        # Remove padding offset
        boxes[:, 0] -= inputs["scale_factor"][2]
        boxes[:, 2] -= inputs["scale_factor"][2]
        boxes[:, 1] -= inputs["scale_factor"][3]
        boxes[:, 3] -= inputs["scale_factor"][3]

        # Scale to original image
        input_shape = np.array([
            inputs["scale_factor"][0], inputs["scale_factor"][1],
            inputs["scale_factor"][0], inputs["scale_factor"][1]
        ])
        boxes = np.multiply(boxes, input_shape, dtype=np.float32)

        # NMS per class
        indices = []
        for class_id in np.unique(class_ids):
            class_mask = class_ids == class_id
            class_boxes = boxes[class_mask]
            class_scores = scores[class_mask]
            class_keep = nms(class_boxes, class_scores, 0.45)
            indices.extend(np.where(class_mask)[0][class_keep])

        return [{
            "type": self.label_list[class_ids[i]].lower(),
            "bbox": boxes[i].tolist(),
            "score": float(scores[i])
        } for i in indices]
```

## Ascend NPU Support

```python
class AscendLayoutRecognizer(Recognizer):
    """
    Layout recognizer for Huawei Ascend NPU.

    Uses .om (Offline Model) format and ais_bench
    for inference.
    """

    def __init__(self, domain):
        from ais_bench.infer.interface import InferSession

        model_dir = os.path.join(
            get_project_base_directory(),
            "rag/res/deepdoc"
        )
        model_file_path = os.path.join(model_dir, domain + ".om")

        device_id = int(os.getenv("ASCEND_LAYOUT_RECOGNIZER_DEVICE_ID", 0))
        self.session = InferSession(
            device_id=device_id,
            model_path=model_file_path
        )
```

## Layout Cleanup

```python
def layouts_cleanup(self, bxs, lts):
    """
    Clean up overlapping layout regions.

    Process:
    1. Remove layouts that don't overlap with any OCR boxes
    2. Merge overlapping layouts of same type
    3. Adjust boundaries based on OCR boxes
    """
    # Implementation in base Recognizer class
    pass

def find_overlapped_with_threshold(self, box, layouts, thr=0.4):
    """
    Find layout region that overlaps with box.

    Args:
        box: OCR box with x0, x1, top, bottom
        layouts: List of layout regions
        thr: Minimum overlap ratio (IoU)

    Returns:
        Index of best matching layout or None
    """
    best_idx = None
    best_overlap = 0

    for idx, lt in enumerate(layouts):
        # Calculate intersection
        x_overlap = max(0, min(box["x1"], lt["x1"]) - max(box["x0"], lt["x0"]))
        y_overlap = max(0, min(box["bottom"], lt["bottom"]) -
                       max(box["top"], lt["top"]))
        intersection = x_overlap * y_overlap

        # Calculate union
        box_area = (box["x1"] - box["x0"]) * (box["bottom"] - box["top"])
        lt_area = (lt["x1"] - lt["x0"]) * (lt["bottom"] - lt["top"])
        union = box_area + lt_area - intersection

        # IoU
        iou = intersection / union if union > 0 else 0

        if iou > thr and iou > best_overlap:
            best_overlap = iou
            best_idx = idx

    return best_idx
```

## Configuration

```python
# Model selection
LAYOUT_RECOGNIZER_TYPE = "onnx"  # onnx, yolov10, ascend

# Detection parameters
LAYOUT_DETECTION_PARAMS = {
    "threshold": 0.2,            # Confidence threshold
    "batch_size": 16,            # Inference batch size
    "scale_factor": 3,           # Image zoom factor
    "drop_garbage": True,        # Filter headers/footers
}

# TensorRT DLA (optional)
TENSORRT_DLA_SVR = None  # "http://localhost:8080"

# Ascend NPU (optional)
ASCEND_LAYOUT_RECOGNIZER_DEVICE_ID = 0
```

## Integration with PDF Parser

```python
# In pdf_parser.py
def _layouts_rec(self, zoomin):
    """
    Run layout recognition on all pages.

    Process:
    1. Initialize LayoutRecognizer
    2. Run detection on page images
    3. Tag OCR boxes with layout types
    4. Store layout information for later processing
    """
    # Initialize recognizer
    self.layout_recognizer = LayoutRecognizer("layout")

    # Convert PIL images to numpy
    images = [np.array(img) for img in self.page_images]

    # Run layout detection and tagging
    self.boxes, self.page_layout = self.layout_recognizer(
        images,
        [self.boxes],  # OCR results
        scale_factor=zoomin,
        thr=0.2,
        batch_size=16,
        drop=True
    )
```

## Related Files

- `/deepdoc/vision/layout_recognizer.py` - Layout detection
- `/deepdoc/vision/recognizer.py` - Base recognizer class
- `/deepdoc/vision/operators.py` - NMS and preprocessing
- `/rag/res/deepdoc/layout.onnx` - ONNX model
