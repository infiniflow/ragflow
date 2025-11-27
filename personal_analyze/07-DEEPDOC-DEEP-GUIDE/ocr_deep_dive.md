# OCR Deep Dive

## Tổng Quan

Module OCR trong DeepDoc thực hiện 2 task chính:
1. **Text Detection**: Phát hiện vùng chứa text trong image
2. **Text Recognition**: Nhận dạng text trong các vùng đã phát hiện

## File Structure

```
deepdoc/vision/
├── ocr.py                 # Main OCR class (752 lines)
├── postprocess.py         # CTC decoder, DBNet postprocess (371 lines)
└── operators.py           # Image preprocessing (726 lines)
```

---

## 1. Text Detection (DBNet)

### 1.1 Model Architecture

```
DBNet (Differentiable Binarization Network):

Input Image (H, W, 3)
         │
         ▼
┌─────────────────────────────────────┐
│        ResNet-18 Backbone           │
│  ┌─────┐  ┌─────┐  ┌─────┐  ┌─────┐│
│  │ C1  │→ │ C2  │→ │ C3  │→ │ C4  ││
│  │64ch │  │128ch│  │256ch│  │512ch││
│  └─────┘  └─────┘  └─────┘  └─────┘│
└─────────────────────────────────────┘
         │      │      │      │
         ▼      ▼      ▼      ▼
┌─────────────────────────────────────┐
│        Feature Pyramid Network       │
│  Upsample + Concatenate all levels  │
│  Output: 256 channels               │
└─────────────────────────────────────┘
         │
         ├─────────────────┐
         ▼                 ▼
┌─────────────────┐ ┌─────────────────┐
│  Probability    │ │   Threshold     │
│     Head        │ │     Head        │
│  Conv → Sigmoid │ │  Conv → Sigmoid │
└────────┬────────┘ └────────┬────────┘
         │                   │
         ▼                   ▼
    Prob Map (H, W)    Thresh Map (H, W)
         │                   │
         └─────────┬─────────┘
                   ▼
┌─────────────────────────────────────┐
│    Differentiable Binarization      │
│    B = sigmoid((P - T) * k)         │
│    k = 50 (amplification factor)    │
└─────────────────────────────────────┘
                   │
                   ▼
            Binary Map (H, W)
```

### 1.2 DBNet Post-processing

```python
# deepdoc/vision/postprocess.py, lines 41-259

class DBPostProcess:
    def __init__(self,
                 thresh=0.3,           # Binary threshold
                 box_thresh=0.5,       # Box confidence threshold
                 max_candidates=1000,  # Maximum text regions
                 unclip_ratio=1.5,     # Polygon expansion ratio
                 use_dilation=False,   # Morphological dilation
                 score_mode="fast"):   # fast or slow scoring

    def __call__(self, outs_dict, shape_list):
        """
        Post-process DBNet output.

        Args:
            outs_dict: {"maps": probability_map}
            shape_list: Original image shapes

        Returns:
            List of detected text boxes
        """
        pred = outs_dict["maps"]  # (N, 1, H, W)

        # Step 1: Binary thresholding
        bitmap = pred > self.thresh  # 0.3

        # Step 2: Optional dilation
        if self.use_dilation:
            kernel = np.ones((2, 2))
            bitmap = cv2.dilate(bitmap, kernel)

        # Step 3: Find contours
        contours = cv2.findContours(
            bitmap.astype(np.uint8),
            cv2.RETR_LIST,
            cv2.CHAIN_APPROX_SIMPLE
        )

        # Step 4: Process each contour
        boxes = []
        for contour in contours[:self.max_candidates]:
            # Simplify polygon
            epsilon = 0.002 * cv2.arcLength(contour, True)
            approx = cv2.approxPolyDP(contour, epsilon, True)

            if len(approx) < 4:
                continue

            # Calculate confidence score
            score = self.box_score_fast(pred, approx)
            if score < self.box_thresh:
                continue

            # Unclip (expand) polygon
            box = self.unclip(approx, self.unclip_ratio)
            boxes.append(box)

        return boxes
```

### 1.3 Unclipping Algorithm

**Vấn đề**: DBNet tends to predict tight boundaries → misses edge characters

**Giải pháp**: Expand detected polygon by unclip_ratio

```python
# deepdoc/vision/postprocess.py, lines 163-169

def unclip(self, box, unclip_ratio):
    """
    Expand polygon using Clipper library.

    Công thức:
    distance = Area * unclip_ratio / Perimeter

    Với unclip_ratio = 1.5:
    - Nhỏ polygon → expand nhiều hơn
    - Lớn polygon → expand ít hơn (proportional)
    """
    poly = Polygon(box)
    distance = poly.area * unclip_ratio / poly.length

    offset = pyclipper.PyclipperOffset()
    offset.AddPath(box, pyclipper.JT_ROUND, pyclipper.ET_CLOSEDPOLYGON)

    expanded = offset.Execute(distance)
    return np.array(expanded[0])
```

**Visualization**:
```
Original detection:     After unclip (1.5x):
┌──────────────┐        ┌────────────────────┐
│   Hello      │   →    │      Hello         │
└──────────────┘        └────────────────────┘
                        (expanded boundaries)
```

---

## 2. Text Recognition (CRNN)

### 2.1 Model Architecture

```
CRNN (Convolutional Recurrent Neural Network):

Input: Cropped text image (3, 48, W)
                │
                ▼
┌─────────────────────────────────────┐
│            CNN Backbone              │
│  VGG-style convolutions             │
│  7 conv layers + 4 max pooling      │
│  Output: (512, 1, W/4)              │
└────────────────┬────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────┐
│         Sequence Reshaping          │
│  Collapse height dimension          │
│  Output: (W/4, 512)                 │
└────────────────┬────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────┐
│         Bidirectional LSTM          │
│  2 layers, 256 hidden units         │
│  Output: (W/4, 512)                 │
└────────────────┬────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────┐
│          Classification Head         │
│  Linear(512 → num_classes)          │
│  Output: (W/4, num_classes)         │
└────────────────┬────────────────────┘
                 │
                 ▼
        Probability Matrix (T, C)
        T = time steps, C = characters
```

### 2.2 CTC Decoding

```python
# deepdoc/vision/postprocess.py, lines 347-370

class CTCLabelDecode(BaseRecLabelDecode):
    """
    CTC (Connectionist Temporal Classification) Decoder.

    CTC giải quyết vấn đề:
    - Model output có T time steps
    - Ground truth có N characters
    - T > N (nhiều frame cho 1 ký tự)
    - Không biết alignment chính xác

    CTC thêm special "blank" token (ε):
    - Represents "no output"
    - Allows alignment without explicit segmentation
    """

    def __init__(self, character_dict_path, use_space_char=False):
        super().__init__(character_dict_path, use_space_char)
        # Prepend blank token at index 0
        self.character = ['blank'] + self.character

    def __call__(self, preds, label=None):
        """
        Decode CTC output.

        Args:
            preds: (batch, time, num_classes) probability matrix

        Returns:
            [(text, confidence), ...]
        """
        # Get most probable character at each time step
        preds_idx = preds.argmax(axis=2)   # (batch, time)
        preds_prob = preds.max(axis=2)      # (batch, time)

        # Decode with deduplication
        result = self.decode(preds_idx, preds_prob, is_remove_duplicate=True)

        return result

    def decode(self, text_index, text_prob, is_remove_duplicate=True):
        """
        CTC decoding algorithm.

        Example:
        Raw output:  [a, a, ε, l, l, ε, p, h, a]
        After dedup: [a, ε, l, ε, p, h, a]
        Remove blank: [a, l, p, h, a]
        Final: "alpha"
        """
        result = []

        for batch_idx in range(len(text_index)):
            char_list = []
            conf_list = []

            for idx in range(len(text_index[batch_idx])):
                char_idx = text_index[batch_idx][idx]

                # Skip blank token (index 0)
                if char_idx == 0:
                    continue

                # Skip consecutive duplicates
                if is_remove_duplicate:
                    if idx > 0 and char_idx == text_index[batch_idx][idx-1]:
                        continue

                char_list.append(self.character[char_idx])
                conf_list.append(text_prob[batch_idx][idx])

            text = ''.join(char_list)
            conf = np.mean(conf_list) if conf_list else 0.0

            result.append((text, conf))

        return result
```

### 2.3 Aspect Ratio Handling

```python
# deepdoc/vision/ocr.py, lines 146-170

def resize_norm_img(self, img, max_wh_ratio):
    """
    Resize image maintaining aspect ratio.

    Vấn đề: Text images có width khác nhau
    - "Hi" → narrow
    - "Hello World" → wide

    Giải pháp: Resize theo aspect ratio, pad right side
    """
    imgC, imgH, imgW = self.rec_image_shape  # [3, 48, 320]

    # Calculate target width from aspect ratio
    max_width = int(imgH * max_wh_ratio)
    max_width = min(max_width, imgW)  # Cap at 320

    h, w = img.shape[:2]
    ratio = w / float(h)

    # Resize maintaining aspect ratio
    if ratio * imgH > max_width:
        resized_w = max_width
    else:
        resized_w = int(ratio * imgH)

    resized_img = cv2.resize(img, (resized_w, imgH))

    # Pad right side to max_width
    padded = np.zeros((imgH, max_width, 3), dtype=np.float32)
    padded[:, :resized_w, :] = resized_img

    # Normalize: [0, 255] → [-1, 1]
    padded = (padded / 255.0 - 0.5) / 0.5

    # Transpose: HWC → CHW
    padded = padded.transpose(2, 0, 1)

    return padded
```

**Visualization**:
```
Original images:
┌──────┐  ┌────────────────┐  ┌──────────────────────┐
│ Hi   │  │ Hello          │  │ Hello World          │
└──────┘  └────────────────┘  └──────────────────────┘
 narrow        medium               wide

After resize + pad (to width 320):
┌──────────────────────────────────────────────────────┐
│ Hi   │░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░│
├──────────────────────────────────────────────────────┤
│ Hello          │░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░│
├──────────────────────────────────────────────────────┤
│ Hello World          │░░░░░░░░░░░░░░░░░░░░░░░░░░░░░│
└──────────────────────────────────────────────────────┘
(░ = zero padding)
```

---

## 3. Full OCR Pipeline

### 3.1 OCR Class

```python
# deepdoc/vision/ocr.py, lines 536-752

class OCR:
    """
    End-to-end OCR pipeline.

    Usage:
        ocr = OCR()
        results = ocr(image)
        # results: [(box_points, (text, confidence)), ...]
    """

    def __init__(self, model_dir=None):
        # Auto-download models if not found
        if model_dir is None:
            model_dir = self._get_model_dir()

        # Initialize detector and recognizer
        self.text_detector = TextDetector(model_dir)
        self.text_recognizer = TextRecognizer(model_dir)

    def __call__(self, img, device_id=0, cls=True):
        """
        Full OCR pipeline.

        Args:
            img: numpy array (H, W, 3) in BGR
            device_id: GPU device ID
            cls: Whether to check text orientation

        Returns:
            [(box_4pts, (text, confidence)), ...]
        """
        # Step 1: Detect text regions
        dt_boxes, det_time = self.text_detector(img)

        if dt_boxes is None or len(dt_boxes) == 0:
            return []

        # Step 2: Sort boxes by reading order
        dt_boxes = self.sorted_boxes(dt_boxes)

        # Step 3: Crop and rotate each text region
        img_crop_list = []
        for box in dt_boxes:
            tmp_box = self.get_rotate_crop_image(img, box)
            img_crop_list.append(tmp_box)

        # Step 4: Recognize text
        rec_res, rec_time = self.text_recognizer(img_crop_list)

        # Step 5: Filter by confidence
        results = []
        for box, rec in zip(dt_boxes, rec_res):
            text, score = rec
            if score >= 0.5:  # drop_score threshold
                results.append((box, (text, score)))

        return results
```

### 3.2 Rotation Detection

```python
# deepdoc/vision/ocr.py, lines 584-638

def get_rotate_crop_image(self, img, points):
    """
    Crop text region with automatic rotation detection.

    Vấn đề: Text có thể xoay 90° hoặc 270°
    Giải pháp: Try multiple orientations, pick best CTC score
    """
    # Order points: top-left → top-right → bottom-right → bottom-left
    rect = self.order_points_clockwise(points)

    # Perspective transform to get rectangular crop
    width = int(max(
        np.linalg.norm(rect[0] - rect[1]),
        np.linalg.norm(rect[2] - rect[3])
    ))
    height = int(max(
        np.linalg.norm(rect[0] - rect[3]),
        np.linalg.norm(rect[1] - rect[2])
    ))

    dst = np.array([
        [0, 0],
        [width, 0],
        [width, height],
        [0, height]
    ], dtype=np.float32)

    M = cv2.getPerspectiveTransform(rect, dst)
    warped = cv2.warpPerspective(img, M, (width, height))

    # Check if text is vertical (need rotation)
    if warped.shape[0] / warped.shape[1] >= 1.5:
        # Try 3 orientations
        orientations = [
            (warped, 0),                              # Original
            (cv2.rotate(warped, cv2.ROTATE_90_CLOCKWISE), 90),
            (cv2.rotate(warped, cv2.ROTATE_90_COUNTERCLOCKWISE), -90)
        ]

        best_score = -1
        best_img = warped

        for rot_img, angle in orientations:
            # Quick recognition to get confidence
            _, score = self.text_recognizer([rot_img])[0]
            if score > best_score:
                best_score = score
                best_img = rot_img

        warped = best_img

    return warped
```

### 3.3 Reading Order Sorting

```python
# deepdoc/vision/ocr.py, lines 640-661

def sorted_boxes(self, dt_boxes):
    """
    Sort boxes by reading order (top-to-bottom, left-to-right).

    Algorithm:
    1. Sort by Y coordinate (top of box)
    2. Within same "row" (Y within 10px), sort by X coordinate
    """
    num_boxes = len(dt_boxes)
    sorted_boxes = sorted(dt_boxes, key=lambda x: (x[0][1], x[0][0]))

    # Group into rows and sort each row
    _boxes = list(sorted_boxes)

    for i in range(num_boxes - 1):
        for j in range(i, -1, -1):
            # If boxes are on same row (Y difference < 10)
            if abs(_boxes[j+1][0][1] - _boxes[j][0][1]) < 10:
                # Sort by X coordinate
                if _boxes[j+1][0][0] < _boxes[j][0][0]:
                    _boxes[j], _boxes[j+1] = _boxes[j+1], _boxes[j]
            else:
                break

    return _boxes
```

---

## 4. Performance Optimization

### 4.1 GPU Memory Management

```python
# deepdoc/vision/ocr.py, lines 96-127

def load_model(model_dir, nm, device_id=None):
    """
    Load ONNX model with optimized settings.
    """
    options = ort.SessionOptions()

    # Reduce memory fragmentation
    options.enable_cpu_mem_arena = False

    # Sequential execution (more predictable memory)
    options.execution_mode = ort.ExecutionMode.ORT_SEQUENTIAL

    # Limit thread usage
    options.intra_op_num_threads = 2
    options.inter_op_num_threads = 2

    # GPU configuration
    if torch.cuda.is_available() and device_id is not None:
        providers = [
            ('CUDAExecutionProvider', {
                'device_id': device_id,
                # Limit GPU memory to 2GB
                'gpu_mem_limit': int(os.getenv('OCR_GPU_MEM_LIMIT_MB', 2048)) * 1024 * 1024,
                # Memory allocation strategy
                'arena_extend_strategy': os.getenv('OCR_ARENA_EXTEND_STRATEGY', 'kNextPowerOfTwo'),
            })
        ]
    else:
        providers = ['CPUExecutionProvider']

    session = ort.InferenceSession(model_path, options, providers)

    # Run options for memory cleanup after each run
    run_opts = ort.RunOptions()
    run_opts.add_run_config_entry("memory.enable_memory_arena_shrinkage", "gpu:0")

    return session, run_opts
```

### 4.2 Batch Processing Optimization

```python
# deepdoc/vision/ocr.py, lines 363-408

def __call__(self, img_list):
    """
    Optimized batch recognition.
    """
    # Sort images by aspect ratio for efficient batching
    # Similar widths → less padding waste
    indices = np.argsort([img.shape[1]/img.shape[0] for img in img_list])

    results = [None] * len(img_list)

    for batch_start in range(0, len(indices), self.batch_size):
        batch_indices = indices[batch_start:batch_start + self.batch_size]

        # Get max width in batch for padding
        max_wh_ratio = max(img_list[i].shape[1]/img_list[i].shape[0]
                          for i in batch_indices)

        # Normalize all images to same width
        norm_imgs = []
        for i in batch_indices:
            norm_img = self.resize_norm_img(img_list[i], max_wh_ratio)
            norm_imgs.append(norm_img)

        # Stack into batch
        batch = np.stack(norm_imgs)

        # Run inference
        preds = self.ort_sess.run(None, {"input": batch})

        # Decode results
        texts = self.postprocess_op(preds[0])

        # Map back to original indices
        for j, idx in enumerate(batch_indices):
            results[idx] = texts[j]

    return results
```

### 4.3 Multi-GPU Parallel Processing

```python
# deepdoc/vision/ocr.py, lines 556-579

class OCR:
    def __init__(self, model_dir=None):
        if settings.PARALLEL_DEVICES > 0:
            # Create per-GPU instances
            self.text_detector = [
                TextDetector(model_dir, device_id)
                for device_id in range(settings.PARALLEL_DEVICES)
            ]
            self.text_recognizer = [
                TextRecognizer(model_dir, device_id)
                for device_id in range(settings.PARALLEL_DEVICES)
            ]
        else:
            # Single instance for CPU/single GPU
            self.text_detector = TextDetector(model_dir)
            self.text_recognizer = TextRecognizer(model_dir)
```

---

## 5. Troubleshooting

### 5.1 Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| Low accuracy | Low resolution input | Increase zoomin factor (3-5) |
| Slow inference | Large images | Resize to max 960px |
| Memory error | Too many candidates | Reduce max_candidates |
| Missing text | Tight boundaries | Increase unclip_ratio |
| Wrong orientation | Vertical text | Enable rotation detection |

### 5.2 Debugging Tips

```python
# Enable verbose logging
import logging
logging.basicConfig(level=logging.DEBUG)

# Visualize detections
from deepdoc.vision.seeit import draw_boxes

img_with_boxes = draw_boxes(img, dt_boxes)
cv2.imwrite("debug_detection.png", img_with_boxes)

# Check confidence scores
for box, (text, conf) in results:
    print(f"Text: {text}, Confidence: {conf:.2f}")
    if conf < 0.5:
        print("  ⚠️ Low confidence!")
```

---

## 6. References

- DBNet Paper: [Real-time Scene Text Detection with Differentiable Binarization](https://arxiv.org/abs/1911.08947)
- CRNN Paper: [An End-to-End Trainable Neural Network for Image-based Sequence Recognition](https://arxiv.org/abs/1507.05717)
- CTC Paper: [Connectionist Temporal Classification](https://www.cs.toronto.edu/~graves/icml_2006.pdf)
- PaddleOCR: [GitHub](https://github.com/PaddlePaddle/PaddleOCR)
