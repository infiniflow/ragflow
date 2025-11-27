# Vision Algorithms

## Tong Quan

RAGFlow sử dụng computer vision algorithms cho document understanding, OCR, và layout analysis.

## 1. OCR (Optical Character Recognition)

### File Location
```
/deepdoc/vision/ocr.py (lines 30-120)
```

### Purpose
Text detection và recognition từ document images.

### Implementation

```python
import onnxruntime as ort

class OCR:
    def __init__(self):
        # Load ONNX models
        self.det_model = ort.InferenceSession("ocr_det.onnx")
        self.rec_model = ort.InferenceSession("ocr_rec.onnx")

    def detect(self, image, device_id=0):
        """
        Detect text regions in image.

        Returns:
            List of bounding boxes with confidence scores
        """
        # Preprocess
        img = self._preprocess_det(image)

        # Run detection
        outputs = self.det_model.run(None, {"input": img})

        # Post-process to get boxes
        boxes = self._postprocess_det(outputs[0])

        return boxes

    def recognize(self, image, boxes):
        """
        Recognize text in detected regions.

        Returns:
            List of (text, confidence) tuples
        """
        results = []

        for box in boxes:
            # Crop region
            crop = self._crop_region(image, box)

            # Preprocess
            img = self._preprocess_rec(crop)

            # Run recognition
            outputs = self.rec_model.run(None, {"input": img})

            # Decode to text
            text, conf = self._decode_ctc(outputs[0])
            results.append((text, conf))

        return results
```

### OCR Pipeline

```
OCR Pipeline:
┌─────────────────────────────────────────────────────────────────┐
│  Input Image                                                     │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  Detection Model (ONNX)                                          │
│  - DB (Differentiable Binarization) based                       │
│  - Output: Text region polygons                                 │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  Post-processing                                                 │
│  - Polygon to bounding box                                      │
│  - Filter by confidence                                         │
│  - NMS for overlapping boxes                                    │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  Recognition Model (ONNX)                                        │
│  - CRNN (CNN + RNN) based                                       │
│  - CTC decoding                                                 │
│  - Output: Character sequence                                   │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│  Output: [(text, confidence, box), ...]                         │
└─────────────────────────────────────────────────────────────────┘
```

### CTC Decoding

```
CTC (Connectionist Temporal Classification):

Input: Probability matrix P (T × C)
       T = time steps, C = character classes

Algorithm:
1. For each time step, get most probable character
2. Merge consecutive duplicates
3. Remove blank tokens

Example:
Raw output: [a, a, -, b, b, b, -, c]
After merge: [a, -, b, -, c]
After blank removal: [a, b, c]
Final: "abc"
```

---

## 2. Layout Recognition (YOLOv10)

### File Location
```
/deepdoc/vision/layout_recognizer.py (lines 33-100)
```

### Purpose
Detect document layout elements (text, title, table, figure, etc.).

### Implementation

```python
class LayoutRecognizer:
    LABELS = [
        "text", "title", "figure", "figure caption",
        "table", "table caption", "header", "footer",
        "reference", "equation"
    ]

    def __init__(self):
        self.model = ort.InferenceSession("layout_yolov10.onnx")

    def detect(self, image):
        """
        Detect layout elements in document image.
        """
        # Preprocess (resize, normalize)
        img = self._preprocess(image)

        # Run inference
        outputs = self.model.run(None, {"images": img})

        # Post-process
        boxes, labels, scores = self._postprocess(outputs[0])

        # Filter by confidence
        results = []
        for box, label, score in zip(boxes, labels, scores):
            if score > 0.4:  # Confidence threshold
                results.append({
                    "box": box,
                    "type": self.LABELS[label],
                    "confidence": score
                })

        return results
```

### Layout Types

```
Document Layout Categories:
┌──────────────────┬────────────────────────────────────┐
│ Type             │ Description                        │
├──────────────────┼────────────────────────────────────┤
│ text             │ Body text paragraphs               │
│ title            │ Section/document titles            │
│ figure           │ Images, diagrams, charts           │
│ figure caption   │ Text describing figures            │
│ table            │ Data tables                        │
│ table caption    │ Text describing tables             │
│ header           │ Page headers                       │
│ footer           │ Page footers                       │
│ reference        │ Bibliography, citations            │
│ equation         │ Mathematical equations             │
└──────────────────┴────────────────────────────────────┘
```

### YOLO Detection

```
YOLOv10 Detection:

1. Backbone: Feature extraction (CSPDarknet)
2. Neck: Feature pyramid (PANet)
3. Head: Prediction heads for different scales

Output format:
[x_center, y_center, width, height, confidence, class_probs...]

Post-processing:
1. Apply sigmoid to confidence
2. Multiply conf × class_prob for class scores
3. Filter by score threshold
4. Apply NMS
```

---

## 3. Table Structure Recognition (TSR)

### File Location
```
/deepdoc/vision/table_structure_recognizer.py (lines 30-100)
```

### Purpose
Detect table structure (rows, columns, cells, headers).

### Implementation

```python
class TableStructureRecognizer:
    LABELS = [
        "table", "table column", "table row",
        "table column header", "projected row header",
        "spanning cell"
    ]

    def __init__(self):
        self.model = ort.InferenceSession("table_structure.onnx")

    def recognize(self, table_image):
        """
        Recognize structure of a table image.
        """
        # Preprocess
        img = self._preprocess(table_image)

        # Run inference
        outputs = self.model.run(None, {"input": img})

        # Parse structure
        structure = self._parse_structure(outputs)

        return structure

    def _parse_structure(self, outputs):
        """
        Parse model output into table structure.
        """
        rows = []
        columns = []
        cells = []

        for detection in outputs:
            label = self.LABELS[detection["class"]]

            if label == "table row":
                rows.append(detection["box"])
            elif label == "table column":
                columns.append(detection["box"])
            elif label == "spanning cell":
                cells.append({
                    "box": detection["box"],
                    "colspan": self._estimate_colspan(detection, columns),
                    "rowspan": self._estimate_rowspan(detection, rows)
                })

        return {
            "rows": sorted(rows, key=lambda x: x[1]),  # Sort by Y
            "columns": sorted(columns, key=lambda x: x[0]),  # Sort by X
            "cells": cells
        }
```

### TSR Output

```
Table Structure Output:

{
    "rows": [
        {"y": 10, "height": 30},   # Row 1
        {"y": 40, "height": 30},   # Row 2
        ...
    ],
    "columns": [
        {"x": 0, "width": 100},    # Col 1
        {"x": 100, "width": 150},  # Col 2
        ...
    ],
    "cells": [
        {"row": 0, "col": 0, "text": "Header 1"},
        {"row": 0, "col": 1, "text": "Header 2"},
        {"row": 1, "col": 0, "text": "Data 1", "colspan": 2},
        ...
    ]
}
```

---

## 4. Non-Maximum Suppression (NMS)

### File Location
```
/deepdoc/vision/operators.py (lines 702-725)
```

### Purpose
Filter overlapping bounding boxes trong object detection.

### Implementation

```python
def nms(boxes, scores, iou_threshold=0.5):
    """
    Non-Maximum Suppression algorithm.

    Args:
        boxes: List of [x1, y1, x2, y2]
        scores: Confidence scores
        iou_threshold: IoU threshold for suppression

    Returns:
        Indices of kept boxes
    """
    # Sort by score (descending)
    indices = np.argsort(scores)[::-1]

    keep = []
    while len(indices) > 0:
        # Keep highest scoring box
        current = indices[0]
        keep.append(current)

        if len(indices) == 1:
            break

        # Compute IoU with remaining boxes
        remaining = indices[1:]
        ious = compute_iou(boxes[current], boxes[remaining])

        # Keep boxes with IoU below threshold
        indices = remaining[ious < iou_threshold]

    return keep
```

### NMS Algorithm

```
NMS (Non-Maximum Suppression):

Input: Boxes B, Scores S, Threshold θ
Output: Filtered boxes

Algorithm:
1. Sort boxes by score (descending)
2. Select box with highest score → add to results
3. Remove boxes with IoU > θ with selected box
4. Repeat until no boxes remain

Example:
Boxes: [A(0.9), B(0.8), C(0.7)]
IoU(A,B) = 0.7 > 0.5 → Remove B
IoU(A,C) = 0.3 < 0.5 → Keep C
Result: [A, C]
```

---

## 5. Intersection over Union (IoU)

### File Location
```
/deepdoc/vision/operators.py (lines 702-725)
/deepdoc/vision/recognizer.py (lines 339-357)
```

### Purpose
Measure overlap between bounding boxes.

### Implementation

```python
def compute_iou(box1, box2):
    """
    Compute Intersection over Union.

    Args:
        box1, box2: [x1, y1, x2, y2] format

    Returns:
        IoU value in [0, 1]
    """
    # Intersection coordinates
    x1 = max(box1[0], box2[0])
    y1 = max(box1[1], box2[1])
    x2 = min(box1[2], box2[2])
    y2 = min(box1[3], box2[3])

    # Intersection area
    intersection = max(0, x2 - x1) * max(0, y2 - y1)

    # Union area
    area1 = (box1[2] - box1[0]) * (box1[3] - box1[1])
    area2 = (box2[2] - box2[0]) * (box2[3] - box2[1])
    union = area1 + area2 - intersection

    # IoU
    if union == 0:
        return 0

    return intersection / union
```

### IoU Formula

```
IoU (Intersection over Union):

IoU = Area(A ∩ B) / Area(A ∪ B)

     = Area(A ∩ B) / (Area(A) + Area(B) - Area(A ∩ B))

Range: [0, 1]
- IoU = 0: No overlap
- IoU = 1: Perfect overlap

Threshold Usage:
- Detection: IoU > 0.5 → Same object
- NMS: IoU > 0.5 → Suppress duplicate
```

---

## 6. Image Preprocessing

### File Location
```
/deepdoc/vision/operators.py
```

### Purpose
Prepare images for neural network input.

### Implementation

```python
class StandardizeImage:
    """Normalize image to [0, 1] range."""

    def __call__(self, image):
        return image.astype(np.float32) / 255.0

class NormalizeImage:
    """Apply mean/std normalization."""

    def __init__(self, mean=[0.485, 0.456, 0.406],
                 std=[0.229, 0.224, 0.225]):
        self.mean = np.array(mean)
        self.std = np.array(std)

    def __call__(self, image):
        return (image - self.mean) / self.std

class ToCHWImage:
    """Convert HWC to CHW format."""

    def __call__(self, image):
        return image.transpose((2, 0, 1))

class LinearResize:
    """Resize image maintaining aspect ratio."""

    def __init__(self, target_size):
        self.target = target_size

    def __call__(self, image):
        h, w = image.shape[:2]
        scale = self.target / max(h, w)
        new_h, new_w = int(h * scale), int(w * scale)
        return cv2.resize(image, (new_w, new_h),
                         interpolation=cv2.INTER_CUBIC)
```

### Preprocessing Pipeline

```
Image Preprocessing Pipeline:

1. Resize (maintain aspect ratio)
   - Target: 640 or 1280 depending on model

2. Standardize (0-255 → 0-1)
   - image = image / 255.0

3. Normalize (ImageNet stats)
   - image = (image - mean) / std
   - mean = [0.485, 0.456, 0.406]
   - std = [0.229, 0.224, 0.225]

4. Transpose (HWC → CHW)
   - PyTorch format: (C, H, W)

5. Pad (to square)
   - Pad with zeros to square shape
```

---

## 7. XGBoost Text Concatenation

### File Location
```
/deepdoc/parser/pdf_parser.py (lines 88-101, 131-170)
```

### Purpose
Predict whether adjacent text boxes should be merged.

### Implementation

```python
import xgboost as xgb

class PDFParser:
    def __init__(self):
        # Load pre-trained XGBoost model
        self.concat_model = xgb.Booster()
        self.concat_model.load_model("updown_concat_xgb.model")

    def should_concat(self, box1, box2):
        """
        Predict if two text boxes should be concatenated.
        """
        # Extract features
        features = self._extract_concat_features(box1, box2)

        # Create DMatrix
        dmatrix = xgb.DMatrix([features])

        # Predict probability
        prob = self.concat_model.predict(dmatrix)[0]

        return prob > 0.5

    def _extract_concat_features(self, box1, box2):
        """
        Extract 20+ features for concatenation decision.
        """
        features = []

        # Distance features
        y_dist = box2["top"] - box1["bottom"]
        char_height = box1["bottom"] - box1["top"]
        features.append(y_dist / max(char_height, 1))

        # Alignment features
        x_overlap = min(box1["x1"], box2["x1"]) - max(box1["x0"], box2["x0"])
        features.append(x_overlap / max(box1["x1"] - box1["x0"], 1))

        # Text pattern features
        text1, text2 = box1["text"], box2["text"]
        features.append(1 if text1.endswith((".", "。", "!", "?")) else 0)
        features.append(1 if text2[0].isupper() else 0)

        # Layout features
        features.append(1 if box1.get("layout_num") == box2.get("layout_num") else 0)

        # ... more features

        return features
```

### Feature List

```
XGBoost Concatenation Features:

1. Spatial Features:
   - Y-distance / char_height
   - X-alignment overlap ratio
   - Same page flag

2. Text Pattern Features:
   - Ends with sentence punctuation
   - Ends with continuation punctuation
   - Next starts with uppercase
   - Next starts with number
   - Chinese numbering pattern

3. Layout Features:
   - Same layout_type
   - Same layout_num
   - Same column

4. Tokenization Features:
   - Token count ratio
   - Last/first token match

Total: 20+ features
```

---

## Summary

| Algorithm | Purpose | Model Type |
|-----------|---------|------------|
| OCR | Text detection + recognition | ONNX (DB + CRNN) |
| Layout Recognition | Element detection | ONNX (YOLOv10) |
| TSR | Table structure | ONNX |
| NMS | Box filtering | Classical |
| IoU | Overlap measure | Classical |
| XGBoost | Text concatenation | Gradient Boosting |

## Related Files

- `/deepdoc/vision/ocr.py` - OCR models
- `/deepdoc/vision/layout_recognizer.py` - Layout detection
- `/deepdoc/vision/table_structure_recognizer.py` - TSR
- `/deepdoc/vision/operators.py` - Image processing
- `/deepdoc/parser/pdf_parser.py` - XGBoost integration
