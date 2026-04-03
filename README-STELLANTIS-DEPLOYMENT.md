# RagFlow v0.24.0 — Stellantis Deployment Guide
> Configured for local development (Docker Desktop + WSL2) and GCP production deployment.  
> Vector backend: **Infinity** (not Elasticsearch) | Embedding: **TEI local** (CPU mode)

---

## Table of Contents
1. [Prerequisites](#prerequisites)
2. [Quick Start — Local WSL2](#quick-start--local-wsl2)
3. [Configuration Overview](#configuration-overview)
4. [Known Issues & Fixes](#known-issues--fixes)
5. [Health Checks](#health-checks)
6. [Corporate Network (Kaspersky SSL)](#corporate-network-kaspersky-ssl)
7. [API Usage](#api-usage)
8. [Automotive Intelligence Pipeline](#automotive-intelligence-pipeline)
9. [GCP Production Deployment](#gcp-production-deployment)
10. [Development Workflow](#development-workflow)
11. [Upgrading](#upgrading)

---

## Prerequisites

| Requirement | Version | Notes |
|---|---|---|
| Docker Desktop | ≥ 24.0.0 | WSL2 backend enabled |
| Docker Compose | ≥ v2.26.1 | Included with Docker Desktop |
| WSL2 | Ubuntu 22.04 / 24.04 | Windows only |
| RAM | ≥ 16GB allocated to WSL2 | 20GB recommended |
| Disk | ≥ 60GB free | Images + models + data |

---

## Quick Start — Local WSL2

### 1. Configure WSL2 memory (Windows only)

Create or edit `%USERPROFILE%\.wslconfig`:

```ini
[wsl2]
memory=20GB
processors=8
swap=4GB
kernelCommandLine="sysctl.vm.max_map_count=262144"
```

Then restart WSL2:
```powershell
wsl --shutdown
```

### 2. Clone this repo into WSL2

> ⚠️ Always clone inside the WSL2 filesystem (`~/`), never under `/mnt/c/`

```bash
cd ~
git clone https://github.com/R-Oussama-INFOMINEO/ragflow.git
cd ragflow
git checkout feature/youtube-ingestion
```

### 3. Pull images and start the stack

```bash
cd docker
docker compose -f docker-compose.yml pull
docker compose -f docker-compose.yml up -d
```

> The stack uses the custom image `ragflow-stellantis:v0.24.0` defined in `docker/.env`.
> If this image is not available locally, build it first:
> ```bash
> cd ~/ragflow && docker build -f Dockerfile.custom -t ragflow-stellantis:v0.24.0 .
> ```

### 4. Apply the embedding model fix (run once after first start)

```bash
bash docker/fix-tenant-embedding.sh
```

> This fixes a v0.24.0 bug where the local TEI embedding model is registered
> without the `@Builtin` provider suffix, causing all document parsing to fail.

### 5. Watch startup logs

```bash
docker logs -f docker-ragflow-cpu-1
```

Wait for the RAGFlow ASCII banner and `Running on all addresses (0.0.0.0)`.
First startup takes **5–15 minutes** — DeepDoc models download from HuggingFace (~2GB).

### 6. Access the UI

Open your browser at: `http://localhost`

Register a new account on first login.

---

## Configuration Overview

All configuration lives in `docker/.env`. Key settings for this deployment:

| Setting | Value | Notes |
|---|---|---|
| `DOC_ENGINE` | `infinity` | Vector DB backend (not Elasticsearch) |
| `RAGFLOW_IMAGE` | `ragflow-stellantis:v0.24.0` | Custom image with video ingestion baked in |
| `COMPOSE_PROFILES` | includes `tei-cpu` | Enables local CPU embedding |
| `TEI_MODEL` | `BAAI/bge-small-en-v1.5` | CPU-friendly, ~1.2GB RAM |
| `DOC_BULK_SIZE` | `4` | Chunk commit batch size |
| `EMBEDDING_BATCH_SIZE` | `8` | Embedding batch size for CPU |
| `TZ` | `Africa/Casablanca` | Change to your local timezone |

### Services and ports

| Service | Container | Port | Role |
|---|---|---|---|
| RagFlow UI + API | `docker-ragflow-cpu-1` | 80, 9380 | Main application |
| Infinity | `docker-infinity-1` | 23820 | Vector database |
| MySQL | `docker-mysql-1` | 5455 | Metadata storage |
| MinIO | `docker-minio-1` | 9000, 9001 | File storage |
| Redis/Valkey | `docker-redis-1` | 6379 | Task queue |
| TEI | `docker-tei-cpu-1` | 6380 | Embedding service |

---

## Known Issues & Fixes

### Fix 1 — Embedding model `@None` not authorized

**Symptom:** Document parsing fails with:
```
[ERROR]Fail to bind embedding model: Model(BAAI/bge-small-en-v1.5@None) not authorized
```

**Cause:** v0.24.0 bug — tenant table stores embedding model ID without `@Builtin` suffix.

**Fix:** Run once after fresh deployment:
```bash
bash docker/fix-tenant-embedding.sh
```

Or manually:
```bash
docker exec docker-mysql-1 mysql -u root -pinfini_rag_flow rag_flow \
  -e "UPDATE tenant SET embd_id='BAAI/bge-small-en-v1.5@Builtin' \
      WHERE embd_id='BAAI/bge-small-en-v1.5';" 2>/dev/null
```

**Verify:**
```bash
docker exec docker-mysql-1 mysql -u root -pinfini_rag_flow rag_flow \
  -e "SELECT name, embd_id FROM tenant;" 2>/dev/null
```
Both rows must show `BAAI/bge-small-en-v1.5@Builtin`.

---

### Fix 2 — vm.max_map_count too low

**Symptom:** Infinity container crashes or fails health check.

**Fix:** Already handled by `.wslconfig` `kernelCommandLine` setting.

**Verify:**
```bash
cat /proc/sys/vm/max_map_count
# Must print: 262144
```

---

### Fix 3 — Port 80 already in use

**Symptom:** Stack fails to start, port binding error.

**Fix:** Edit `docker/docker-compose.yml` and change `80:80` to `8080:80`,
then access the UI at `http://localhost:8080`.

---

### Fix 4 — Custom image not found

**Symptom:** Stack fails to start with `pull access denied for ragflow-stellantis`.

**Cause:** The custom image hasn't been built locally yet.

**Fix:**
```bash
cd ~/ragflow && docker build -f Dockerfile.custom -t ragflow-stellantis:v0.24.0 .
```

This takes 8–10 minutes and layers on top of `infiniflow/ragflow:v0.24.0`.
It installs ffmpeg, all Whisper backends, and pre-downloads the `tiny` and `base` models.

---

### Fix 5 — Corporate SSL proxy (HuggingFace / OpenAI SDK)

**Symptom:** Whisper model download or OpenAI API calls fail with:
```
SSL: CERTIFICATE_VERIFY_FAILED — self-signed certificate in certificate chain
```

**Cause:** Corporate SSL inspection (e.g. Kaspersky) intercepts HTTPS connections.

**Fix:** Already baked into `Dockerfile.custom` via three ENV variables:
```dockerfile
ENV HF_HUB_DISABLE_SSL_VERIFICATION=1   # HuggingFace model downloads
ENV CURL_CA_BUNDLE=""                    # curl/requests based libraries
ENV REQUESTS_CA_BUNDLE=""               # OpenAI SDK (httpx)
```

No manual action needed — these are set automatically in the custom image.

> ⚠️ For LLM/chat calls (Gemini API), a separate Kaspersky certificate injection
> is still required. See the [Corporate Network](#corporate-network-kaspersky-ssl) section.

---

## Health Checks

Run after stack startup to verify all services:

```bash
# All containers status
docker compose -f docker/docker-compose.yml ps

# MySQL
docker exec docker-mysql-1 mysqladmin -u root -pinfini_rag_flow ping 2>/dev/null

# MinIO
curl -s -o /dev/null -w "%{http_code}" http://localhost:9000/minio/health/live

# TEI embedding service
docker logs --tail=5 docker-tei-cpu-1 | grep -E "Ready|Error"

# Infinity
docker inspect docker-infinity-1 --format='{{.State.Health.Status}}'

# Video parser registration
docker exec docker-ragflow-cpu-1 /ragflow/.venv/bin/python3 -c "
from common.constants import ParserType
from rag.svr.task_executor import FACTORY
assert ParserType.VIDEO.value in FACTORY
print('Video parser: OK')
" 2>&1 | grep "Video parser"

# Whisper backends availability
docker exec docker-ragflow-cpu-1 /ragflow/.venv/bin/python3 -c "
import subprocess
for pkg in ['faster_whisper', 'yt_dlp', 'whisper']:
    try:
        __import__(pkg)
        print(f'✅ {pkg} importable')
    except ImportError:
        print(f'❌ {pkg} missing')
r = subprocess.run(['ffmpeg', '-version'], capture_output=True)
print('✅ ffmpeg available' if r.returncode == 0 else '❌ ffmpeg missing')
from faster_whisper import WhisperModel
for size in ['tiny', 'base']:
    try:
        WhisperModel(size, device='cpu', compute_type='int8')
        print(f'✅ faster-whisper {size} model cached')
    except Exception as e:
        print(f'❌ faster-whisper {size} failed: {e}')
"
```

---

## Corporate Network (Kaspersky SSL)

If your machine uses Kaspersky Endpoint Security with SSL inspection
(common in corporate environments), the RagFlow container cannot reach
external APIs (Gemini, HuggingFace) without trusting the Kaspersky CA certificate.

> ⚠️ The SSL fix is only needed for **LLM/chat calls** (Gemini API).
> Whisper model downloads and OpenAI API calls are handled automatically
> via ENV variables baked into the image (see Fix 5 above).

### Export the certificate (Windows PowerShell)

```powershell
$cert = Get-ChildItem -Path Cert:\LocalMachine\Root |
  Where-Object { $_.Subject -like "*Kaspersky*" } |
  Select-Object -First 1

$b64 = [Convert]::ToBase64String($cert.RawData, 'InsertLineBreaks')
"-----BEGIN CERTIFICATE-----`n$b64`n-----END CERTIFICATE-----" |
  Out-File -FilePath "$env:USERPROFILE\kaspersky-ca.pem" -Encoding ASCII

# Copy to WSL2
Copy-Item "$env:USERPROFILE\kaspersky-ca.pem" "\\wsl$\Ubuntu\home\$env:USERNAME\kaspersky-ca.pem"
```

### Inject into RagFlow container

```bash
# Copy cert into container
docker cp ~/kaspersky-ca.pem docker-ragflow-cpu-1:/tmp/kaspersky-ca.pem

# Add to system CA store
docker exec -u root docker-ragflow-cpu-1 bash -c \
  'cp /tmp/kaspersky-ca.pem /usr/local/share/ca-certificates/kaspersky-ca.crt && \
   update-ca-certificates'

# Add to Python certifi stores (required for LLM API calls)
docker exec -u root docker-ragflow-cpu-1 bash -c \
  'find / -name "cacert.pem" 2>/dev/null | grep -i certifi | \
   while read f; do cat /tmp/kaspersky-ca.pem >> "$f"; done'
```

> ⚠️ This fix does not persist across container restarts.
> Re-run after any `docker compose down && up` cycle.
> Not needed when running outside the corporate network (e.g. GCP, home).

---

## API Usage

### Get your API key

UI → Avatar (top-right) → API KEY → Create new key

### List datasets

```bash
API_KEY="ragflow-xxxxxxxxxxxx"

curl -s -X GET "http://localhost:9380/api/v1/datasets" \
  -H "Authorization: Bearer ${API_KEY}" | python3 -m json.tool
```

### Upload and parse a document

```bash
DATASET_ID="your_dataset_id"

curl -s -X POST "http://localhost:9380/api/v1/datasets/${DATASET_ID}/documents" \
  -H "Authorization: Bearer ${API_KEY}" \
  -F "file=@/path/to/your/document.pdf"
```

### Retrieval query

```bash
curl -s -X POST "http://localhost:9380/api/v1/retrieval" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${API_KEY}" \
  -d '{
    "question": "Your question here",
    "dataset_ids": ["'"${DATASET_ID}"'"],
    "similarity_threshold": 0.1,
    "keywords_similarity_weight": 0.7,
    "top_n": 3
  }' | python3 -m json.tool
```

---

## Automotive Intelligence Pipeline

This deployment includes a custom multi-source ingestion pipeline built on top of RagFlow v0.24.0, designed for the Stellantis automotive intelligence system.

### Architecture Overview

```
One Analysis Dataset per Car Model / Market / Year
─────────────────────────────────────────────────
Name: {Brand}_{Model}_{Year}_{Market}_{Trim}_{YYYYMMDD}_{HHMM}
Example: Opel_Corsa_2023_UK_All_20260403_1143

All source types share one dataset:
  ├── YouTube Videos   → parser_id="video"  (our custom parser)
  ├── PDFs             → parser_id="naive"  (DeepDoc)
  ├── Web pages (HTML) → parser_id="naive"  (DeepDoc)
  └── Images           → parser_id="picture" (DeepDoc vision)

Business metadata stored per-document in DocMetadataService:
  brand, car_model, year, market, trim, source_type,
  retrieval_date, youtube_url, video_title

Retrieval filtered via metadata_condition (not dataset name):
  brand="Opel" AND car_model="Corsa" AND source_type="Video"
```

### Data Flow

```
YouTube URL / PDF / HTML / Image
    │
    ▼
POST /api/v1/datasets                      ← create one analysis dataset
    │  chunk_method="naive" (default)
    │  whisper_backend, whisper_model       ← only technical fields in parser_config
    ▼
POST /api/v1/datasets/{id}/videos          ← register YouTube URL (sets parser_id="video")
POST /api/v1/datasets/{id}/documents       ← upload PDF/HTML/Image
    │
    │  metadata stored via DocMetadataService:
    │  brand, car_model, year, market, trim,
    │  source_type, retrieval_date, youtube_url, video_title
    ▼
task_executor.py
    │  reads parser_id from document (not dataset)
    │  video: fetches youtube_url from DocMetadataService
    │  bypasses MinIO for video — no file upload
    ▼
rag/app/video.py → _fetch_transcript()     ← video pipeline
    ├── youtube-transcript-api  → fetch captions directly (fast, default)
    ├── faster-whisper          → download audio + local transcription (CPU/GPU)
    ├── openai-whisper          → download audio + local transcription (CPU/GPU)
    └── openai-api              → download audio + cloud transcription (fastest)

DeepDoc Engine                             ← docs/web/image pipeline
    ├── PDF parser (naive)
    ├── HTML parser (naive)
    └── Image parser (picture/vision)
    │
    ▼
TEI embedding (BAAI/bge-small-en-v1.5@Builtin)
    ▼
Infinity vector store
    │  chunk properties: timestamp_seconds, transcript_segment
    │  document metadata: brand, car_model, year, market, trim,
    │                     source_type, youtube_url, video_title
    ▼
POST /api/v1/retrieval
    │  doc_ids filtered by metadata_condition before retrieval
    ▼
chunks with timestamp deep-links + full source traceability
```

---

### Dataset Naming Convention

All datasets follow this standardized naming format:

```
{Brand}_{Model}_{Year}_{Market}_{Trim}_{YYYYMMDD}_{HHMM}
```

**Examples:**
```
Opel_Corsa_2023_UK_All_20260403_1143
Opel_Corsa_2025_IE_All_20260403_1200
Peugeot_208_2023_FR_All_20260403_1215
Opel_Corsa_2025_IE_GS_20260403_0900    ← trim-specific
```

> Note: Source type is no longer in the dataset name — all source types (video, PDF, web, images) share one dataset. Source type is tracked per-document via metadata.

**Market ISO codes:**

| Country | Code |
|---|---|
| United Kingdom | `UK` |
| Ireland | `IE` |
| France | `FR` |
| Germany | `DE` |
| Italy | `IT` |
| Spain | `ES` |
| Belgium | `BE` |
| Netherlands | `NL` |

> Date and time are **auto-generated** at ingestion time — no manual input needed.
> Full country names are automatically normalized to ISO codes.

---

### Metadata Architecture

Business metadata is stored **per-document** in `DocMetadataService` (not in `parser_config`). This keeps RagFlow's upstream `ParserConfig` schema clean and enables metadata-based filtering.

**Document metadata fields:**

| Field | Stored in | Description |
|---|---|---|
| `brand` | DocMetadataService | Car manufacturer e.g. `"Opel"`, `"Peugeot"` |
| `car_model` | DocMetadataService | Car model e.g. `"Corsa"`, `"208"` |
| `year` | DocMetadataService | Model year e.g. `"2023"`, `"2025"` |
| `market` | DocMetadataService | Target market ISO code e.g. `"UK"`, `"FR"` |
| `trim` | DocMetadataService | Trim level e.g. `"All"`, `"GS"`, `"Elegance"` |
| `source_type` | DocMetadataService | `"Video"`, `"Docs"`, `"Web"`, `"Images"` |
| `retrieval_date` | DocMetadataService | Auto-generated ingestion date `"YYYY-MM-DD"` |
| `youtube_url` | DocMetadataService | Full YouTube URL (video only) |
| `video_title` | DocMetadataService | Human-readable title (video only) |

**Parser config fields (technical only):**

| Field | Description |
|---|---|
| `whisper_backend` | Transcription backend: `youtube-transcript-api` \| `faster-whisper` \| `openai-whisper` \| `openai-api` |
| `whisper_model` | Model size: `tiny`, `base`, `small`, `medium`, `large` |
| `openai_api_key` | Required only for `openai-api` backend |
| `chunk_by` | Chunking strategy: `segment` (default, ~60s windows) \| `seconds` (fixed window) |

---

### Transcript Backends

| Backend | Speed (5-min video) | Requires | Best for |
|---|---|---|---|
| `youtube-transcript-api` | ~1 sec | Nothing | Local dev, videos with captions |
| `faster-whisper` | ~30 sec (tiny/CPU), ~8 sec (large/GPU) | yt-dlp + ffmpeg (baked in) | Production CPU/GPU |
| `openai-whisper` | ~2 min (tiny/CPU) | yt-dlp + ffmpeg (baked in) | Alternative local option |
| `openai-api` | ~10 sec | OpenAI API key | Cloud, fastest, $0.006/min |

> **Model size guidance:**
> - `tiny` — fastest, lower accuracy (~29 sec on CPU for 3.5-min video)
> - `base` — good balance of speed and accuracy (~60 sec on CPU)
> - `large` — best accuracy, requires GPU (~8 sec on GCP GPU)

---

### Step-by-step ingestion workflow

**Step 1 — Create one analysis dataset for all source types**

```bash
API_KEY="your_api_key"

curl -s -X POST "http://localhost:9380/api/v1/datasets" \
  -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Opel_Corsa_2023_UK_All_20260403_1143",
    "chunk_method": "naive",
    "embedding_model": "BAAI/bge-small-en-v1.5@Builtin",
    "parser_config": {
      "whisper_backend": "youtube-transcript-api",
      "whisper_model": "base"
    }
  }' | python3 -m json.tool
```

Save the returned `id` as `DATASET_ID`.

**Step 2 — Ingest a YouTube video**

```bash
curl -s -X POST "http://localhost:9380/api/v1/datasets/${DATASET_ID}/videos" \
  -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://www.youtube.com/watch?v=QFzEVtY_1lQ",
    "title": "Opel Corsa 2023 review",
    "brand": "Opel",
    "car_model": "Corsa",
    "year": "2023",
    "market": "UK",
    "trim": "All",
    "source_type": "Video"
  }' | python3 -m json.tool
```

**Step 3 — Ingest a PDF into the same dataset**

```bash
curl -s -X POST "http://localhost:9380/api/v1/datasets/${DATASET_ID}/documents" \
  -H "Authorization: Bearer ${API_KEY}" \
  -F "file=@/path/to/corsa_spec.pdf"
```

> After uploading, update the document metadata:
```bash
DOC_ID="returned_doc_id"

curl -s -X POST "http://localhost:9380/api/v1/datasets/${DATASET_ID}/metadata/update" \
  -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "selector": {"document_ids": ["'"${DOC_ID}"'"]},
    "updates": [
      {"key": "brand", "value": "Opel"},
      {"key": "car_model", "value": "Corsa"},
      {"key": "year", "value": "2023"},
      {"key": "market", "value": "UK"},
      {"key": "source_type", "value": "Docs"}
    ]
  }'
```

**Step 4 — Trigger parsing for all documents**

```bash
curl -s -X POST "http://localhost:9380/api/v1/datasets/${DATASET_ID}/chunks" \
  -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d "{\"document_ids\": [\"${DOC_ID}\"]}" | python3 -m json.tool
```

**Step 5 — Metadata-filtered retrieval**

```bash
curl -s -X POST "http://localhost:9380/api/v1/retrieval" \
  -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "question": "engine performance and fuel economy",
    "dataset_ids": ["'"${DATASET_ID}"'"],
    "similarity_threshold": 0.1,
    "top_n": 5
  }' | python3 -m json.tool
```

---

### Python ingestion helper

```python
import requests
from datetime import datetime, date

BASE_URL = "http://localhost:9380"
API_KEY  = "your_api_key"
HEADERS  = {"Authorization": f"Bearer {API_KEY}", "Content-Type": "application/json"}

MARKET_ISO_CODES = {
    "United Kingdom": "UK", "Ireland": "IE", "France": "FR",
    "Germany": "DE", "Italy": "IT", "Spain": "ES",
    "Belgium": "BE", "Netherlands": "NL",
}

def _normalize_market(market: str) -> str:
    if market in MARKET_ISO_CODES.values():
        return market
    return MARKET_ISO_CODES.get(market, market.upper()[:2])

def _build_name(brand, car_model, year, market, trim) -> str:
    now = datetime.now()
    return f"{brand}_{car_model}_{year}_{market}_{trim}_{now.strftime('%Y%m%d')}_{now.strftime('%H%M')}"

def create_analysis_dataset(
    brand: str, car_model: str, year: str, market: str,
    trim: str = "All",
    whisper_backend: str = "youtube-transcript-api",
    whisper_model: str = "base",
    openai_api_key: str = "",
) -> str:
    """
    Create one analysis dataset for all source types (video, PDF, web, images).
    Dataset name is auto-generated: {Brand}_{Model}_{Year}_{Market}_{Trim}_{YYYYMMDD}_{HHMM}
    """
    market_iso = _normalize_market(market)
    name = _build_name(brand, car_model, year, market_iso, trim)
    parser_config = {
        "whisper_backend": whisper_backend,
        "whisper_model": whisper_model,
    }
    if openai_api_key and whisper_backend == "openai-api":
        parser_config["openai_api_key"] = openai_api_key
    resp = requests.post(f"{BASE_URL}/api/v1/datasets", headers=HEADERS, json={
        "name": name, "chunk_method": "naive",
        "embedding_model": "BAAI/bge-small-en-v1.5@Builtin",
        "parser_config": parser_config,
    })
    return resp.json()["data"]["id"]

def ingest_video(
    dataset_id: str, url: str, title: str,
    brand: str, car_model: str, year: str, market: str,
    trim: str = "All", source_type: str = "Video",
) -> str:
    """Register a YouTube video. Metadata stored in DocMetadataService."""
    market_iso = _normalize_market(market)
    resp = requests.post(
        f"{BASE_URL}/api/v1/datasets/{dataset_id}/videos",
        headers=HEADERS,
        json={
            "url": url, "title": title,
            "brand": brand, "car_model": car_model, "year": year,
            "market": market_iso, "trim": trim, "source_type": source_type,
            "retrieval_date": date.today().isoformat(),
        },
    )
    return resp.json()["data"][0]["id"]

def retrieve_by_metadata(
    dataset_ids: list, question: str,
    brand: str, car_model: str,
    year: str | None = None,
    market: str | None = None,
    source_type: str | None = None,
    top_n: int = 5,
) -> list:
    """
    Query datasets filtered by document metadata.
    Filters are applied per-document via metadata_condition.
    """
    import json as _json
    conditions = [
        {"key": "brand",     "value": brand,      "operator": "eq"},
        {"key": "car_model", "value": car_model,   "operator": "eq"},
    ]
    if year:
        conditions.append({"key": "year",        "value": year,        "operator": "eq"})
    if market:
        conditions.append({"key": "market",      "value": _normalize_market(market), "operator": "eq"})
    if source_type:
        conditions.append({"key": "source_type", "value": source_type, "operator": "eq"})

    meta_condition = {"conditions": conditions, "logic": "and"}
    doc_ids = []
    for ds_id in dataset_ids:
        resp = requests.get(
            f"{BASE_URL}/api/v1/datasets/{ds_id}/documents",
            headers=HEADERS,
            params={"metadata_condition": _json.dumps(meta_condition), "page_size": 1000},
        )
        for doc in resp.json().get("data", {}).get("docs", []):
            doc_ids.append(doc["id"])

    if not doc_ids:
        return []

    resp = requests.post(f"{BASE_URL}/api/v1/retrieval", headers=HEADERS, json={
        "question": question, "dataset_ids": dataset_ids,
        "doc_ids": doc_ids,
        "similarity_threshold": 0.1, "top_n": top_n,
    })
    return resp.json()["data"]["chunks"]

# ── Usage examples ────────────────────────────────────────────────────────────

# Create one analysis dataset for Opel Corsa 2023 UK
ds_id = create_analysis_dataset("Opel", "Corsa", "2023", "UK",
                                 whisper_backend="youtube-transcript-api")

# Ingest a YouTube video into that dataset
ingest_video(ds_id,
    url="https://www.youtube.com/watch?v=QFzEVtY_1lQ",
    title="Opel Corsa 2023 review",
    brand="Opel", car_model="Corsa", year="2023", market="UK")

# Query all Opel Corsa sources in this dataset
chunks = retrieve_by_metadata([ds_id], "engine performance",
                               brand="Opel", car_model="Corsa")

# Query only video sources
chunks = retrieve_by_metadata([ds_id], "engine performance",
                               brand="Opel", car_model="Corsa",
                               source_type="Video")

# Print results with deeplinks
for c in chunks:
    props = c.get("properties", {})
    if props.get("timestamp_seconds") is not None:
        print(f"[{props['timestamp_seconds']}s] {c['content'][:100]}")
        print(f"  Watch: {props.get('transcript_segment', '')}")
    else:
        print(f"[Doc] {c['content'][:100]}")
```

---

### Files modified / created

| File | Status | Change |
|---|---|---|
| `common/constants.py` | Modified | Added `ParserType.VIDEO = "video"` |
| `rag/app/video.py` | **Added** | Multi-backend transcript dispatcher + 4 backend implementations |
| `rag/svr/task_executor.py` | Modified | Fetch `youtube_url` from `DocMetadataService`; bypass MinIO for video |
| `rag/nlp/search.py` | Modified | Removed stale video root fields from chunk fetch list |
| `api/apps/sdk/dataset.py` | Modified | `POST /datasets/{id}/videos` endpoint; stores metadata in DocMetadataService |
| `api/apps/sdk/doc.py` | Modified | Generic `properties` dict on `Chunk` model (replaces video root fields) |
| `api/utils/validation_utils.py` | Modified | Business fields removed from `ParserConfig`; only technical Whisper fields remain |
| `api/utils/api_utils.py` | Modified | Added `"video": None` to `get_parser_config` map |
| `api/db/init_data.py` | Modified | Added `video:Video` to tenant `parser_ids` |
| `conf/infinity_mapping.json` | Modified | Added video chunk columns to Infinity schema |
| `Dockerfile.custom` | **Added** | ffmpeg, faster-whisper, yt-dlp, openai-whisper; SSL bypasses; pre-cached models |
| `tests/test_ragflow_pipeline.py` | **Added** | MCP-ready pipeline test utility with metadata-driven retrieval |
| `docker/deploy-local.sh` | **Added** | Local deploy script |

### RagFlow codebase isolation

All Stellantis business logic is isolated from RagFlow's upstream codebase:

| Layer | Owner | What lives there |
|---|---|---|
| `video.py` | **Ours** (added, not upstream) | All YouTube/Whisper logic |
| `DocMetadataService` | RagFlow (called only, not modified) | Business metadata storage |
| `ParserConfig` | RagFlow upstream | Technical parser fields only |
| `task["name"]` | RagFlow upstream | Human-readable title only |
| `Chunk` model | RagFlow upstream | Generic fields only |

---

### Requirements and constraints

- `youtube-transcript-api` backend: video must have English captions (manual or auto-generated)
- `faster-whisper` / `openai-whisper` / `openai-api` backends: works on any video with audio, no captions needed
- All Whisper dependencies (ffmpeg, yt-dlp, faster-whisper, openai-whisper) are baked into `ragflow-stellantis:v0.24.0` — no manual install needed
- `faster-whisper` tiny and base models are pre-cached in the image — no download on first use
- Datasets use `chunk_method="naive"` as default — video documents automatically use `parser_id="video"` set by the `/videos` endpoint
- Dataset names are auto-generated — do not set them manually
- The UI Files tab shows "No data available" for video documents — this is expected (URL-based, no MinIO upload); use the Retrieval Testing tab instead
- On GCP with `bge-m3`, re-index from scratch — no code changes needed

---

## GCP Production Deployment

> 🚧 This section will be updated after GCP deployment is completed.

### Planned changes for GCP

| Setting | Local value | GCP value |
|---|---|---|
| `RAGFLOW_IMAGE` | `ragflow-stellantis:v0.24.0` | rebuild from `Dockerfile.custom` on GCP |
| `TEI_MODEL` | `BAAI/bge-small-en-v1.5` | `BAAI/bge-m3` |
| `COMPOSE_PROFILES` | `tei-cpu` | `tei-gpu` |
| `DOC_BULK_SIZE` | `4` | `16` (higher throughput) |
| `EMBEDDING_BATCH_SIZE` | `8` | `32` (GPU handles larger batches) |
| `whisper_model` | `tiny` / `base` (CPU) | `large` (GPU, ~8 sec/video) |
| LLM provider | Gemini via OpenAI-compatible | Gemini via OpenAI-compatible |

### Important: re-indexing required on GCP

`BAAI/bge-small-en-v1.5` produces **512-dimension** vectors.
`BAAI/bge-m3` produces **1024-dimension** vectors.

These are **incompatible**. All datasets must be re-created and re-parsed
from scratch on the GCP instance. Do not migrate data volumes from local to GCP.

### GCP prerequisites (to be documented)

- [ ] GCP project with required APIs enabled
- [ ] Terraform service account with appropriate IAM roles
- [ ] Gemini API key (restricted to Generative Language API)
- [ ] VM instance with GPU support (for bge-m3 embedding + faster-whisper large)
- [ ] Docker and Docker Compose installed on GCP VM
- [ ] Firewall rules for ports 80, 443, 9380
- [ ] Build `ragflow-stellantis:v0.24.0` image on the GCP VM

---

## Development Workflow

### Making code changes

When modifying Python source files, the container must be restarted to pick up changes. Use the helper script:

```bash
bash docker/deploy-local.sh
```

This restarts the container, waits 45 seconds for startup, and re-copies all modified source files including `tests/test_ragflow_pipeline.py`.

### Files tracked by deploy-local.sh

```
common/constants.py
rag/app/video.py
rag/svr/task_executor.py
rag/nlp/search.py
api/apps/sdk/dataset.py
api/apps/sdk/doc.py
api/utils/validation_utils.py
api/utils/api_utils.py
api/db/init_data.py
conf/infinity_mapping.json
tests/test_ragflow_pipeline.py
```

### Rebuilding the custom Docker image

After significant changes that should be permanent (not just for a session):

```bash
cd ~/ragflow
docker build -f Dockerfile.custom -t ragflow-stellantis:v0.24.0 .
```

Then force-recreate the container from the new image:
```bash
cd docker && docker compose up -d --no-deps --force-recreate ragflow-cpu
```

> ⚠️ `docker compose down && up` restarts the existing container from the old image.
> Always use `--force-recreate` after rebuilding the image.

---

## Upgrading

To pull future RagFlow releases while keeping your customizations:

```bash
cd ~/ragflow

# Fetch latest from official repo
git fetch upstream

# Check available tags
git tag | grep v0

# Create a new branch for the new version
git checkout -b local/v0.XX.0-infinity upstream/v0.XX.0

# Cherry-pick our video ingestion commits onto the new base
git cherry-pick <first-stellantis-commit>..<last-stellantis-commit>

# Re-apply your .env changes and rebuild custom image
# Push to your fork
git push origin local/v0.XX.0-infinity
```

> **Merge conflict guidance for future upgrades:**
> - `api/apps/sdk/dataset.py` — our `ingest_video` endpoint should be migrated to `api/apps/restful_apis/dataset_api.py` when upgrading past v0.24.0 (upstream refactored this file)
> - `api/apps/sdk/doc.py` — minimal diff, check `Chunk` model and chunk serializers
> - `api/utils/validation_utils.py` — check `ParserConfig` for new upstream fields to preserve
> - `rag/svr/task_executor.py` — check the `build_chunks` function for structural changes
> - All other Stellantis files (`video.py`, `deploy-local.sh`, `test_ragflow_pipeline.py`) have zero upstream conflict risk

---

## Stack management commands

```bash
# Stop without deleting data
docker compose -f docker/docker-compose.yml stop

# Full restart
docker compose -f docker/docker-compose.yml down
docker compose -f docker/docker-compose.yml up -d

# Force-recreate container from new image (after docker build)
cd docker && docker compose up -d --no-deps --force-recreate ragflow-cpu

# DANGER: delete all data and start fresh
docker compose -f docker/docker-compose.yml down -v

# View logs
docker logs -f docker-ragflow-cpu-1   # Main app
docker logs -f docker-tei-cpu-1       # Embedding service
docker logs -f docker-infinity-1      # Vector DB

# Check disk usage
docker system df

# Re-deploy source files without full rebuild (dev only)
bash docker/deploy-local.sh
```

---

*Last updated: April 2026 | RagFlow v0.24.0 | Branch: `feature/youtube-ingestion` | One-dataset-per-analysis-run architecture | Metadata-driven filtering | 4-backend YouTube transcription*
