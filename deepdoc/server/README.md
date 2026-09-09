# OSS DeepDoc HTTP API Service

Serves DLA (Document Layout Analysis), OCR (Optical Character Recognition), and
TSR (Table Structure Recognition) models via a unified HTTP API using
[LitServe](https://github.com/Lightning-AI/litserve) and OSS ONNX Runtime models.

## Quick Start

```bash
# Build
docker build -f Dockerfile_deepdoc_oss -t deepdoc_oss:latest .

# Run (CPU only; no GPU required)
docker run -p 9390:9390 deepdoc_oss:latest

# Or via docker compose
docker compose -f docker/docker-compose.yml up -d
```

The service listens on port **9390** by default. Pass `--port` to change it:

```bash
python deepdoc/server/deepdoc_server.py --port 9000 --model-dir /path/to/models
```

## Endpoints

All prediction endpoints accept JPEG images via `multipart/form-data`. The form
field for file uploads is named `request`.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Liveness probe. Returns `ok`. |
| `GET` | `/model` | Model metadata. Returns `{"model":"oss","version":"1.0"}`. |
| `POST` | `/predict/dla` | Document Layout Analysis. |
| `POST` | `/predict/tsr` | Table Structure Recognition. |
| `POST` | `/predict/ocr` | OCR — use form field `operator=det` for detection or `operator=rec` for recognition. |

### `POST /predict/dla`

Analyzes a full page image and returns labelled layout regions.

**Request**

```
curl -X POST http://localhost:9390/predict/dla \
  -F "request=@page.jpg;type=image/jpeg"
```

**Response**

```json
{
  "bboxes": [
    [x0, y0, x1, y1, score, class_id],
    ...
  ]
}
```

| class_id | Label |
|:--------:|-------|
| 0 | title |
| 1 | text |
| 2 | reference |
| 3 | figure |
| 4 | figure caption |
| 5 | table |
| 6 | table caption |
| 8 | equation |

> The OSS model uses 8 unique class IDs. IDs 7 and 9 are reserved for
> compatibility with the SaaS label scheme but are never produced by the
> OSS model.

### `POST /predict/tsr`

Recognizes table structure from a cropped table image.

**Request**

```
curl -X POST http://localhost:9390/predict/tsr \
  -F "request=@table_crop.jpg;type=image/jpeg"
```

**Response**

```json
{
  "bboxes": [
    [x0, y0, x1, y1, score, class_id],
    ...
  ]
}
```

| class_id | Label |
|:--------:|-------|
| 0 | table |
| 1 | table column |
| 2 | table row |
| 3 | table column header |
| 4 | table projected row header |
| 5 | table spanning cell |

### `POST /predict/ocr`

Two modes controlled by the `operator` form field.

#### Detection (`operator=det`)

Returns quadrilateral bounding boxes for detected text regions.

```
curl -X POST "http://localhost:9390/predict/ocr" \
  -F "operator=det" \
  -F "request=@page.jpg;type=image/jpeg"
```

**Response** (5-level nested array):

```json
{
  "output": [
    [
      [
        [
          [[x0,y0],[x1,y1],[x2,y2],[x3,y3]],
          ...
        ]
      ]
    ]
  ]
}
```

#### Recognition (`operator=rec`)

Recognizes text within a cropped region.

```
curl -X POST "http://localhost:9390/predict/ocr" \
  -F "operator=rec" \
  -F "request=@char_crop.jpg;type=image/jpeg"
```

**Response** (4-level nested array):

```json
{
  "output": [
    [
      [
        ["recognized text", 1.0],
        ...
      ]
    ]
  ]
}
```

> Confidence is always `1.0` — the OSS recognition model does not return
> per-character confidence scores.

## Error Responses

| Scenario | HTTP Status |
|----------|:-----------:|
| Missing `operator` field (OCR) | 400 |
| Invalid `operator` value | 400 |
| Empty or corrupt image | 400 |
| Image exceeds 4096×4096 | 400 |
| Internal inference error | 500 |

## Models

All ONNX models are from the [InfiniFlow/deepdoc](https://huggingface.co/InfiniFlow/deepdoc)
HuggingFace repository (Apache 2.0 license):

| File | Size | Purpose |
|------|------|---------|
| `layout.onnx` | 75.7 MB | DLA (YOLOv10) |
| `det.onnx` | 4.7 MB | OCR text detection (PP-OCRv4) |
| `rec.onnx` | 10.8 MB | OCR text recognition (PP-OCRv4) |
| `tsr.onnx` | 12.2 MB | TSR (PaddleDetection) |
| `ocr.res` | 26 KB | OCR character dictionary |

## Architecture

```
deepdoc/server/
├── deepdoc_server.py       # LitServe entry point
├── endpoints/            # LitAPI endpoints (HTTP layer)
│   ├── dla_endpoint.py
│   ├── tsr_endpoint.py
│   └── ocr_endpoint.py
└── adapters/             # Model wrappers (inference + format conversion)
    ├── dla_adapter.py
    ├── tsr_adapter.py
    └── ocr_adapter.py
```

Endpoints → Adapters → `deepdoc/vision/` (reused OSS model classes) → ONNX Runtime.
