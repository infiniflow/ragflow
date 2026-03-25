# RagFlow v0.24.0 — Deployment Guide
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
8. [YouTube Video Ingestion](#youtube-video-ingestion)
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

This takes 2–3 minutes and layers on top of `infiniflow/ragflow:v0.24.0`.

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
```

---

## Corporate Network (Kaspersky SSL)

If your machine uses Kaspersky Endpoint Security with SSL inspection
(common in corporate environments), the RagFlow container cannot reach
external APIs (Gemini, HuggingFace) without trusting the Kaspersky CA certificate.

> ⚠️ The SSL fix is only needed for **LLM/chat calls** (Gemini API).
> YouTube transcript ingestion does NOT require the SSL fix.

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

### Extract page numbers from retrieval response

```python
import json, requests

response = requests.post(
    "http://localhost:9380/api/v1/retrieval",
    headers={"Authorization": f"Bearer {API_KEY}"},
    json={"question": "...", "dataset_ids": [DATASET_ID], "top_n": 3}
)

for chunk in response.json()["data"]["chunks"]:
    pages = sorted(set([p[0] for p in chunk.get("positions", []) if p]))
    print(f"chunk_id : {chunk['id']}")
    print(f"source   : {chunk['document_keyword']}")
    print(f"pages    : {pages}")
    print(f"content  : {chunk['content'][:200]}")
```

---

## YouTube Video Ingestion

This deployment includes a custom YouTube transcript ingestion pipeline built on top of RagFlow v0.24.0.

### Architecture

```
YouTube URL
    │
    ▼
POST /api/v1/datasets/{id}/videos          ← new endpoint
    │
    ▼
task_executor.py (parser_id="video")
    │  bypasses MinIO — no file upload
    ▼
rag/app/video.py
    │  youtube-transcript-api → 315 raw cues
    │  merge into 60-second overlapping segments
    │  tokenize via rag_tokenizer
    ▼
TEI embedding (BAAI/bge-small-en-v1.5@Builtin)
    ▼
Infinity vector store
    │  stores: youtube_url, video_id, video_title,
    │          timestamp_seconds, transcript_segment
    ▼
POST /api/v1/retrieval
    │  returns all video metadata fields per chunk
    ▼
chunk with timestamp deep-link (&t=60s)
```

### Files modified / created

| File | Change |
|---|---|
| `common/constants.py` | Added `ParserType.VIDEO = "video"` |
| `rag/app/video.py` | New parser — transcript download, segmentation, tokenization |
| `rag/svr/task_executor.py` | Registered video parser in FACTORY; bypass MinIO for video tasks |
| `rag/nlp/search.py` | Added video fields to Infinity retrieval field list and response dict |
| `api/apps/sdk/dataset.py` | New `POST /datasets/{id}/videos` endpoint |
| `api/apps/sdk/doc.py` | Extended `Chunk` model and both serializers with video fields |
| `api/utils/validation_utils.py` | Added `"video"` to `chunk_method` validator |
| `api/utils/api_utils.py` | Added `"video": None` to `get_parser_config` map |
| `api/db/init_data.py` | Added `video:Video` to tenant `parser_ids` |
| `conf/infinity_mapping.json` | Added 5 video columns to Infinity schema |
| `Dockerfile.custom` | Custom image layer — bakes in all changes + youtube-transcript-api |

### Step-by-step ingestion workflow

**Step 1 — Create a video dataset**

```bash
API_KEY="your_api_key"

curl -s -X POST "http://localhost:9380/api/v1/datasets" \
  -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Car_Reviews",
    "chunk_method": "video",
    "embedding_model": "BAAI/bge-small-en-v1.5@Builtin"
  }' | python3 -m json.tool
```

Save the returned `id` as `DATASET_ID`.

**Step 2 — Ingest a YouTube video**

```bash
curl -s -X POST "http://localhost:9380/api/v1/datasets/${DATASET_ID}/videos" \
  -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://www.youtube.com/watch?v=VIDEO_ID",
    "title": "Human-readable title for this video"
  }' | python3 -m json.tool
```

Save the returned `id` as `DOC_ID`.

**Step 3 — Trigger processing**

```bash
curl -s -X POST "http://localhost:9380/api/v1/datasets/${DATASET_ID}/chunks" \
  -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d "{\"document_ids\": [\"${DOC_ID}\"]}" | python3 -m json.tool
```

Processing takes 10–30 seconds. Monitor with:
```bash
docker logs docker-ragflow-cpu-1 --tail=5 -f 2>&1 | grep -i "done\|fail\|video"
```

**Step 4 — Query with timestamp deep-links**

```bash
curl -s -X POST "http://localhost:9380/api/v1/retrieval" \
  -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d "{
    \"question\": \"engine performance and fuel economy\",
    \"dataset_ids\": [\"${DATASET_ID}\"],
    \"similarity_threshold\": 0.1,
    \"top_n\": 3
  }" | python3 -m json.tool
```

### Retrieval response fields

Each chunk in the retrieval response includes these video-specific fields:

| Field | Type | Description |
|---|---|---|
| `youtube_url` | string | Original YouTube URL |
| `video_id` | string | 11-character YouTube video ID |
| `video_title` | string | Title provided at ingestion time |
| `timestamp_seconds` | integer | Start time of this segment in the video |
| `transcript_segment` | string | Deep-link URL (`&t=Xs`) to jump to exact moment |

### Example retrieval response (video chunk)

```json
{
  "content": "it rides on the stellantis CMP platform which is shared with the Peugeot 208...",
  "document_keyword": "https://www.youtube.com/watch?v=QFzEVtY_1lQ",
  "youtube_url": "https://www.youtube.com/watch?v=QFzEVtY_1lQ",
  "video_id": "QFzEVtY_1lQ",
  "video_title": "Vauxhall Corsa 2024 review",
  "timestamp_seconds": 121,
  "transcript_segment": "https://www.youtube.com/watch?v=QFzEVtY_1lQ&t=121s",
  "similarity": 0.699,
  "positions": []
}
```

### Python ingestion helper

```python
import requests

BASE_URL = "http://localhost:9380"
API_KEY = "your_api_key"
HEADERS = {"Authorization": f"Bearer {API_KEY}", "Content-Type": "application/json"}

def create_video_dataset(name: str) -> str:
    resp = requests.post(f"{BASE_URL}/api/v1/datasets", headers=HEADERS, json={
        "name": name,
        "chunk_method": "video",
        "embedding_model": "BAAI/bge-small-en-v1.5@Builtin",
    })
    return resp.json()["data"]["id"]

def ingest_video(dataset_id: str, url: str, title: str) -> str:
    resp = requests.post(
        f"{BASE_URL}/api/v1/datasets/{dataset_id}/videos",
        headers=HEADERS,
        json={"url": url, "title": title},
    )
    doc_id = resp.json()["data"][0]["id"]
    # trigger processing
    requests.post(
        f"{BASE_URL}/api/v1/datasets/{dataset_id}/chunks",
        headers=HEADERS,
        json={"document_ids": [doc_id]},
    )
    return doc_id

def retrieve(dataset_id: str, question: str, top_n: int = 5) -> list:
    resp = requests.post(f"{BASE_URL}/api/v1/retrieval", headers=HEADERS, json={
        "question": question,
        "dataset_ids": [dataset_id],
        "similarity_threshold": 0.1,
        "top_n": top_n,
    })
    return resp.json()["data"]["chunks"]

# Usage
ds_id = create_video_dataset("Car_Reviews")
ingest_video(ds_id, "https://www.youtube.com/watch?v=QFzEVtY_1lQ", "Vauxhall Corsa 2024 review")

# Wait for processing, then query
chunks = retrieve(ds_id, "engine performance")
for c in chunks:
    print(f"[{c['timestamp_seconds']}s] {c['content'][:100]}")
    print(f"  Watch: {c['transcript_segment']}")
```

### Requirements and constraints

- Videos must have English captions (manual or auto-generated)
- `youtube-transcript-api` is baked into `ragflow-stellantis:v0.24.0` — no manual install needed
- The dataset must be created with `chunk_method: "video"` — the UI dropdown does not show `video` (UI is hardcoded); always use the API
- On GCP with `bge-m3`, re-index from scratch — `parser_id` stays `"video"`, no code changes needed

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
| LLM provider | Gemini via OpenAI-compatible | Gemini via OpenAI-compatible |

### Important: re-indexing required on GCP

`BAAI/bge-small-en-v1.5` produces **512-dimension** vectors.
`BAAI/bge-m3` produces **1024-dimension** vectors.

These are **incompatible**. All datasets must be re-created and re-parsed
from scratch on the GCP instance. Do not migrate data volumes from local to GCP.

### GCP prerequisites (to be documented)

- [ ] GCP project with required APIs enabled
- [ ] Terraform service account with appropriate IAM roles (`stellantis-terraform-sa@stellantis-490509.iam.gserviceaccount.com`)
- [ ] Gemini API key (restricted to Generative Language API)
- [ ] GCS bucket for Terraform state (optional)
- [ ] VM instance with GPU support (for bge-m3 embedding)
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

This restarts the container and re-copies all modified source files in one command.

### Rebuilding the custom Docker image

After significant changes that should be permanent (not just for a session):

```bash
cd ~/ragflow
docker build -f Dockerfile.custom -t ragflow-stellantis:v0.24.0 .
```

Then restart the stack:
```bash
cd docker && docker compose -f docker-compose.yml down && docker compose -f docker-compose.yml up -d
```

### Files tracked by deploy-local.sh

The script re-copies these files on every deploy:

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
```

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
git cherry-pick af81ef57a..e2802acea

# Re-apply your .env changes
cp docker/.env.backup docker/.env
# Update RAGFLOW_IMAGE to new version, rebuild custom image

# Push to your fork
git push origin local/v0.XX.0-infinity
```

> The video ingestion commits are isolated and additive — they should cherry-pick cleanly onto any future v0.2X release with minimal conflicts.

---

## Stack management commands

```bash
# Stop without deleting data
docker compose -f docker/docker-compose.yml stop

# Full restart
docker compose -f docker/docker-compose.yml down
docker compose -f docker/docker-compose.yml up -d

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

*Last updated: March 2026 | RagFlow v0.24.0 | Branch: `feature/youtube-ingestion`*
