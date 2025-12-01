# OCR Pipeline - PaddleOCR Integration

## Tong Quan

OCR (Optical Character Recognition) pipeline trong RAGFlow su dung PaddleOCR de extract text tu images. He thong duoc toi uu hoa de ho tro ca CPU va GPU, voi kha nang xu ly batch va multi-GPU parallel processing.

## File Location
```
/deepdoc/vision/ocr.py
```

## Architecture

```
                     OCR PIPELINE ARCHITECTURE

                        Input Image
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                      TEXT DETECTOR                               │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Model: det.onnx (DBNet)                                │   │
│  │  - Resize image (max 960px)                             │   │
│  │  - Normalize: mean=[0.485,0.456,0.406]                  │   │
│  │  - Detect text regions → Bounding boxes                 │   │
│  └─────────────────────────────────────────────────────────┘   │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
              ┌────────────────────────┐
              │   Crop Text Regions    │
              │   Sort: top→bottom     │
              │         left→right     │
              └────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                     TEXT RECOGNIZER                              │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Model: rec.onnx (CRNN + CTC)                           │   │
│  │  - Resize to 48x320                                     │   │
│  │  - Batch processing (16 images/batch)                   │   │
│  │  - CTC decode với character dictionary                  │   │
│  └─────────────────────────────────────────────────────────┘   │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
              ┌────────────────────────┐
              │  Filter by confidence  │
              │  (threshold: 0.5)      │
              └────────────────────────┘
                           │
                           ▼
                  Text + Bounding Boxes
```

## Core Components

### 1. OCR Class (Main Entry Point)

```python
class OCR:
    def __init__(self, model_dir=None):
        """
        Initialize OCR with optional model directory.

        Features:
        - Auto-download models from HuggingFace if not found
        - Multi-GPU support via PARALLEL_DEVICES setting
        - Model caching for performance
        """
        if settings.PARALLEL_DEVICES > 0:
            # Create detector/recognizer for each GPU
            self.text_detector = []
            self.text_recognizer = []
            for device_id in range(settings.PARALLEL_DEVICES):
                self.text_detector.append(TextDetector(model_dir, device_id))
                self.text_recognizer.append(TextRecognizer(model_dir, device_id))
        else:
            # Single device (CPU or GPU 0)
            self.text_detector = [TextDetector(model_dir)]
            self.text_recognizer = [TextRecognizer(model_dir)]

        self.drop_score = 0.5  # Confidence threshold

    def __call__(self, img, device_id=0):
        """
        Full OCR pipeline: detect + recognize.

        Returns:
            List of (bounding_box, (text, confidence))
        """
        # 1. Detect text regions
        dt_boxes, det_time = self.text_detector[device_id](img)

        # 2. Sort boxes (top-to-bottom, left-to-right)
        dt_boxes = self.sorted_boxes(dt_boxes)

        # 3. Crop and recognize each region
        img_crop_list = []
        for box in dt_boxes:
            img_crop = self.get_rotate_crop_image(img, box)
            img_crop_list.append(img_crop)

        # 4. Batch recognize
        rec_res, rec_time = self.text_recognizer[device_id](img_crop_list)

        # 5. Filter by confidence
        results = []
        for box, (text, score) in zip(dt_boxes, rec_res):
            if score >= self.drop_score:
                results.append((box.tolist(), (text, score)))

        return results
```

### 2. TextDetector Class

```python
class TextDetector:
    """
    Detect text regions using DBNet model.

    Input: Image (numpy array)
    Output: List of 4-point polygons (bounding boxes)
    """

    def __init__(self, model_dir, device_id=None):
        # Preprocessing pipeline
        self.preprocess_op = [
            DetResizeForTest(limit_side_len=960, limit_type="max"),
            NormalizeImage(
                std=[0.229, 0.224, 0.225],
                mean=[0.485, 0.456, 0.406],
                scale='1./255.'
            ),
            ToCHWImage(),
        ]

        # Postprocessing: DBNet decode
        self.postprocess_op = DBPostProcess(
            thresh=0.3,
            box_thresh=0.5,
            max_candidates=1000,
            unclip_ratio=1.5
        )

        # Load ONNX model
        self.predictor, self.run_options = load_model(model_dir, 'det', device_id)

    def __call__(self, img):
        """
        Detect text regions in image.

        Process:
        1. Preprocess (resize, normalize)
        2. Run inference
        3. Postprocess (decode probability map to polygons)
        4. Filter small boxes
        """
        ori_im = img.copy()

        # Preprocess
        data = transform({'image': img}, self.preprocess_op)
        img_tensor, shape_list = data

        # Inference
        outputs = self.predictor.run(None, {self.input_tensor.name: img_tensor})

        # Postprocess
        post_result = self.postprocess_op({"maps": outputs[0]}, shape_list)
        dt_boxes = post_result[0]['points']

        # Filter small boxes (width or height <= 3)
        dt_boxes = self.filter_tag_det_res(dt_boxes, ori_im.shape)

        return dt_boxes
```

### 3. TextRecognizer Class

```python
class TextRecognizer:
    """
    Recognize text from cropped images using CRNN model.

    Input: List of cropped text region images
    Output: List of (text, confidence) tuples
    """

    def __init__(self, model_dir, device_id=None):
        self.rec_image_shape = [3, 48, 320]  # C, H, W
        self.rec_batch_num = 16

        # CTC decoder with character dictionary
        self.postprocess_op = CTCLabelDecode(
            character_dict_path=os.path.join(model_dir, "ocr.res"),
            use_space_char=True
        )

        # Load ONNX model
        self.predictor, self.run_options = load_model(model_dir, 'rec', device_id)

    def __call__(self, img_list):
        """
        Recognize text from list of images.

        Process:
        1. Sort by width for efficient batching
        2. Resize and normalize each image
        3. Batch inference
        4. CTC decode
        """
        img_num = len(img_list)

        # Sort by aspect ratio (width/height)
        width_list = [img.shape[1] / float(img.shape[0]) for img in img_list]
        indices = np.argsort(np.array(width_list))

        rec_res = [['', 0.0]] * img_num

        # Process in batches
        for beg_idx in range(0, img_num, self.rec_batch_num):
            end_idx = min(img_num, beg_idx + self.rec_batch_num)

            # Prepare batch
            norm_img_batch = []
            max_wh_ratio = self.rec_image_shape[2] / self.rec_image_shape[1]

            for idx in range(beg_idx, end_idx):
                h, w = img_list[indices[idx]].shape[0:2]
                max_wh_ratio = max(max_wh_ratio, w / h)

            for idx in range(beg_idx, end_idx):
                norm_img = self.resize_norm_img(
                    img_list[indices[idx]],
                    max_wh_ratio
                )
                norm_img_batch.append(norm_img[np.newaxis, :])

            norm_img_batch = np.concatenate(norm_img_batch)

            # Inference
            outputs = self.predictor.run(None, {
                self.input_tensor.name: norm_img_batch
            })

            # CTC decode
            preds = outputs[0]
            rec_result = self.postprocess_op(preds)

            # Store results in original order
            for i, result in enumerate(rec_result):
                rec_res[indices[beg_idx + i]] = result

        return rec_res
```

## Model Loading

```python
def load_model(model_dir, nm, device_id=None):
    """
    Load ONNX model with GPU/CPU support.

    Features:
    - Model caching (avoid reloading)
    - Auto GPU detection
    - Configurable GPU memory limit
    """
    model_file_path = os.path.join(model_dir, nm + ".onnx")

    # Check cache
    global loaded_models
    cache_key = model_file_path + str(device_id)
    if cache_key in loaded_models:
        return loaded_models[cache_key]

    # Configure session
    options = ort.SessionOptions()
    options.enable_cpu_mem_arena = False
    options.execution_mode = ort.ExecutionMode.ORT_SEQUENTIAL
    options.intra_op_num_threads = 2
    options.inter_op_num_threads = 2

    # GPU configuration
    if cuda_is_available():
        gpu_mem_limit_mb = int(os.environ.get("OCR_GPU_MEM_LIMIT_MB", "2048"))
        cuda_provider_options = {
            "device_id": device_id or 0,
            "gpu_mem_limit": gpu_mem_limit_mb * 1024 * 1024,
            "arena_extend_strategy": "kNextPowerOfTwo"
        }
        sess = ort.InferenceSession(
            model_file_path,
            options=options,
            providers=['CUDAExecutionProvider'],
            provider_options=[cuda_provider_options]
        )
    else:
        sess = ort.InferenceSession(
            model_file_path,
            options=options,
            providers=['CPUExecutionProvider']
        )

    # Cache and return
    run_options = ort.RunOptions()
    loaded_models[cache_key] = (sess, run_options)
    return loaded_models[cache_key]
```

## Image Processing Utilities

### Rotate Crop Image

```python
def get_rotate_crop_image(self, img, points):
    """
    Crop text region with perspective transform.

    Handles rotated/skewed text by:
    1. Calculate crop dimensions
    2. Apply perspective transform
    3. Auto-rotate if height > width
    """
    assert len(points) == 4, "shape of points must be 4*2"

    # Calculate target dimensions
    img_crop_width = int(max(
        np.linalg.norm(points[0] - points[1]),
        np.linalg.norm(points[2] - points[3])
    ))
    img_crop_height = int(max(
        np.linalg.norm(points[0] - points[3]),
        np.linalg.norm(points[1] - points[2])
    ))

    # Standard rectangle coordinates
    pts_std = np.float32([
        [0, 0],
        [img_crop_width, 0],
        [img_crop_width, img_crop_height],
        [0, img_crop_height]
    ])

    # Perspective transform
    M = cv2.getPerspectiveTransform(points, pts_std)
    dst_img = cv2.warpPerspective(
        img, M, (img_crop_width, img_crop_height),
        borderMode=cv2.BORDER_REPLICATE,
        flags=cv2.INTER_CUBIC
    )

    # Auto-rotate if needed (height/width >= 1.5)
    if dst_img.shape[0] / dst_img.shape[1] >= 1.5:
        # Try different rotations, pick best recognition score
        best_img = self._find_best_rotation(dst_img)
        return best_img

    return dst_img
```

### Box Sorting

```python
def sorted_boxes(self, dt_boxes):
    """
    Sort text boxes: top-to-bottom, left-to-right.

    Algorithm:
    1. Initial sort by (y, x) coordinates
    2. Fine-tune: swap adjacent boxes if on same line
       and right box is to the left
    """
    num_boxes = dt_boxes.shape[0]

    # Sort by top-left corner (y first, then x)
    sorted_boxes = sorted(dt_boxes, key=lambda x: (x[0][1], x[0][0]))
    _boxes = list(sorted_boxes)

    # Fine-tune for same-line boxes
    for i in range(num_boxes - 1):
        for j in range(i, -1, -1):
            # If boxes on same line (y diff < 10) and wrong order
            if abs(_boxes[j + 1][0][1] - _boxes[j][0][1]) < 10 and \
                    _boxes[j + 1][0][0] < _boxes[j][0][0]:
                # Swap
                _boxes[j], _boxes[j + 1] = _boxes[j + 1], _boxes[j]
            else:
                break

    return _boxes
```

## Configuration

```python
# Environment variables
OCR_GPU_MEM_LIMIT_MB = 2048        # GPU memory limit per model
OCR_ARENA_EXTEND_STRATEGY = "kNextPowerOfTwo"  # Memory allocation strategy
PARALLEL_DEVICES = 0               # Number of GPUs (0 = single device)

# Model parameters
DETECTION_PARAMS = {
    "limit_side_len": 960,         # Max image dimension
    "thresh": 0.3,                 # Binary threshold
    "box_thresh": 0.5,             # Box confidence threshold
    "max_candidates": 1000,        # Max detected boxes
    "unclip_ratio": 1.5            # Box expansion ratio
}

RECOGNITION_PARAMS = {
    "image_shape": [3, 48, 320],   # Input shape (C, H, W)
    "batch_num": 16,               # Batch size
    "drop_score": 0.5              # Confidence threshold
}
```

## Models Used

| Model | File | Purpose | Architecture |
|-------|------|---------|--------------|
| Text Detection | det.onnx | Find text regions | DBNet (Differentiable Binarization) |
| Text Recognition | rec.onnx | Read text content | CRNN + CTC |
| Character Dict | ocr.res | Character mapping | CTC vocabulary |

## Integration with PDF Parser

```python
# In pdf_parser.py
def __ocr(self, callback, start_progress, end_progress):
    """
    Run OCR on PDF page images.

    For each page:
    1. Call OCR to get text boxes with positions
    2. Convert coordinates to page coordinate system
    3. Store boxes with page number for later processing
    """
    self.boxes = []

    for page_idx, img in enumerate(self.page_images):
        # Get OCR results
        results = self.ocr(img)

        if not results:
            continue

        # Convert to internal format
        for box, (text, score) in results:
            x0 = min(p[0] for p in box)
            x1 = max(p[0] for p in box)
            y0 = min(p[1] for p in box)
            y1 = max(p[1] for p in box)

            self.boxes.append({
                "x0": x0 / self.ZM,
                "x1": x1 / self.ZM,
                "top": y0 / self.ZM + self.page_cum_height[page_idx],
                "bottom": y1 / self.ZM + self.page_cum_height[page_idx],
                "text": text,
                "page_number": page_idx,
                "score": score
            })

        # Update progress
        if callback:
            progress = start_progress + (end_progress - start_progress) * \
                       (page_idx / len(self.page_images))
            callback(progress, f"OCR page {page_idx + 1}")
```

## Related Files

- `/deepdoc/vision/ocr.py` - Main OCR implementation
- `/deepdoc/vision/operators.py` - Image preprocessing operators
- `/deepdoc/vision/postprocess.py` - DBNet and CTC postprocessing
- `/rag/res/deepdoc/` - Model files (det.onnx, rec.onnx, ocr.res)
