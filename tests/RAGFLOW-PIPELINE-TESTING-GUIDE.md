# RagFlow Pipeline Testing Guide
> YouTube video ingestion (4 backends) + PDF ingestion + Brand-scoped retrieval  
> Branch: `feature/youtube-ingestion` | Tested on: RagFlow v0.24.0  
> Test video: [Opel Corsa 2023 review](https://www.youtube.com/watch?v=QFzEVtY_1lQ)  
> Test PDFs: `tests/Corsa_test.pdf` (Opel Corsa 2025 IE) | `tests/208_test.pdf` (Peugeot 208 2023 FR)

---

## Table of Contents
1. [Prerequisites](#prerequisites)
2. [Setup](#setup)
3. [Test File Overview](#test-file-overview)
4. [How to Run Each Test](#how-to-run-each-test)
5. [Real Results — YouTube Backends](#real-results--youtube-backends)
6. [Real Results — PDF Pipeline](#real-results--pdf-pipeline)
7. [Full Comparison Summary](#full-comparison-summary)
8. [MCP Future Usage](#mcp-future-usage)

---

## Prerequisites

- RagFlow stack running: `bash docker/deploy-local.sh`
- Custom image built: `ragflow-stellantis:v0.24.0`
- `python-dotenv` installed in container venv:

```bash
docker exec docker-ragflow-cpu-1 /ragflow/.venv/bin/python3 -m pip install python-dotenv --quiet
```

---

## Setup

### 1. Create `tests/.env.test`

> ⚠️ This file is in `.gitignore` — never commit it. Create it manually each time.

```bash
cat > tests/.env.test << 'EOF'
RAGFLOW_BASE_URL=http://172.18.0.1:9380
RAGFLOW_API_KEY=ragflow-xxxxxxxxxxxxxxxxxxxx
OPENAI_API_KEY=sk-xxxxxxxxxxxxxxxxxxxx
EOF
```

> **Important:** Use `http://172.18.0.1:9380` (Docker gateway IP), not `localhost`.  
> `localhost` inside the container refers to the container itself, not your host machine.

### 2. Copy files into the container

```bash
# Create tests/ folder in container (first time only)
docker exec docker-ragflow-cpu-1 mkdir -p /ragflow/tests

# Copy test script and config
docker cp tests/.env.test docker-ragflow-cpu-1:/ragflow/tests/.env.test
docker cp tests/test_ragflow_pipeline.py docker-ragflow-cpu-1:/ragflow/tests/test_ragflow_pipeline.py

# Copy PDF test files
docker cp tests/Corsa_test.pdf docker-ragflow-cpu-1:/ragflow/tests/Corsa_test.pdf
docker cp tests/208_test.pdf docker-ragflow-cpu-1:/ragflow/tests/208_test.pdf
```

---

## Test File Overview

**File:** `tests/test_ragflow_pipeline.py`

### What it does

The script runs the complete RagFlow ingestion pipeline end-to-end:

```
Create dataset → Register document → Trigger parsing → Poll until done → Retrieve → Display results
```

Dataset ID and Doc ID are captured and passed automatically — no manual export needed.
Dataset names are **auto-generated** from metadata — no manual naming needed.

### Key functions

```python
# ── Dataset creation ────────────────────────────────────────────────────────
create_video_dataset(cfg, brand, car_model, year, market,
                     whisper_backend, whisper_model, trim, openai_api_key)
create_pdf_dataset(cfg, brand, car_model, year, market, trim)

# ── Ingestion ───────────────────────────────────────────────────────────────
ingest_video(cfg, dataset_id, url, title)
ingest_pdf(cfg, dataset_id, file_path)

# ── Processing ──────────────────────────────────────────────────────────────
trigger_parsing(cfg, dataset_id, doc_id)
wait_for_completion(cfg, dataset_id, doc_id, timeout, poll_interval)

# ── Retrieval ───────────────────────────────────────────────────────────────
retrieve(cfg, dataset_id, question, top_n, similarity_threshold)
get_datasets_by_brand_model(cfg, brand, model, year, market, trim, source_type)
retrieve_by_brand_model(cfg, brand, model, question, year, market, trim, source_type, top_n)
display_results(chunks, max_content_length)

# ── Full pipeline runners ───────────────────────────────────────────────────
run_video_pipeline(cfg, url, title, question, whisper_backend, whisper_model,
                   brand, car_model, year, market, trim, cleanup)
run_pdf_pipeline(cfg, file_path, question, brand, car_model, year, market, trim, cleanup)
compare_backends(cfg, url, title, question, brand, car_model, year, market,
                 backends, whisper_model, top_n, cleanup)
```

### Dataset naming convention

Datasets are auto-named using this format:
```
{Brand}_{Model}_{Year}_{Market}_{Trim}_{SourceType}_{YYYYMMDD}_{HHMM}

Examples:
  Opel_Corsa_2023_UK_All_Video_20260327_2005
  Opel_Corsa_2025_IE_All_Docs_20260327_2005
  Peugeot_208_2023_FR_All_Docs_20260327_2009
```

### How to switch between tests

Scroll to the `__main__` block at the bottom of the file. **Uncomment exactly one option at a time**, comment out all others.

```python
# ✅ Active — this will run
run_video_pipeline(
    cfg=cfg,
    url=VIDEO_URL, title=VIDEO_TITLE, question=QUESTION,
    brand="Opel", car_model="Corsa", year="2023", market="UK",
    whisper_backend="youtube-transcript-api", cleanup=False,
)

# ❌ Inactive — commented out
# run_video_pipeline(...)
```

### Deploy and run command

After editing and saving the file, always run:

```bash
docker cp tests/test_ragflow_pipeline.py \
  docker-ragflow-cpu-1:/ragflow/tests/test_ragflow_pipeline.py && \
docker exec docker-ragflow-cpu-1 /ragflow/.venv/bin/python3 \
  /ragflow/tests/test_ragflow_pipeline.py 2>&1
```

---

## How to Run Each Test

### Option A — `youtube-transcript-api` (fastest, captions only)

**When to use:** Local dev, quick validation, video has English captions.  
**Speed:** ~5s parse, ~43s total  
**Cost:** Free

```python
run_video_pipeline(
    cfg=cfg,
    url="https://www.youtube.com/watch?v=QFzEVtY_1lQ",
    title="Opel Corsa 2023 review",
    question="engine performance and fuel economy",
    brand="Opel", car_model="Corsa", year="2023", market="UK", trim="All",
    whisper_backend="youtube-transcript-api",
    whisper_model="base",   # ignored for this backend
    cleanup=False,
)
```

---

### Option B — `faster-whisper` (local CPU/GPU)

**When to use:** Videos without captions, production CPU, GCP GPU with `large` model.  
**Speed:** ~68s total (tiny/CPU), ~8s (large/GPU)  
**Cost:** Free (local compute only)

```python
run_video_pipeline(
    cfg=cfg,
    url="https://www.youtube.com/watch?v=QFzEVtY_1lQ",
    title="Opel Corsa 2023 review",
    question="engine performance and fuel economy",
    brand="Opel", car_model="Corsa", year="2023", market="UK", trim="All",
    whisper_backend="faster-whisper",
    whisper_model="tiny",    # tiny=fastest | base=balanced | large=best (GPU)
    cleanup=False,
)
```

> **Model size guide:**
> - `tiny`  — ~68s on CPU, lower accuracy
> - `base`  — ~120s on CPU, good accuracy (recommended local)
> - `large` — ~8s on GCP GPU, best accuracy (recommended production)

---

### Option C — `openai-whisper` (local CPU/GPU, original library)

**When to use:** Alternative to faster-whisper, same accuracy profile.  
**Speed:** ~63s total (tiny/CPU)  
**Cost:** Free (local compute only)

```python
run_video_pipeline(
    cfg=cfg,
    url="https://www.youtube.com/watch?v=QFzEVtY_1lQ",
    title="Opel Corsa 2023 review",
    question="engine performance and fuel economy",
    brand="Opel", car_model="Corsa", year="2023", market="UK", trim="All",
    whisper_backend="openai-whisper",
    whisper_model="tiny",
    cleanup=False,
)
```

---

### Option D — `openai-api` (cloud, fastest paid option)

**When to use:** Fastest transcription, no GPU needed, pay-per-use.  
**Speed:** ~31s total  
**Cost:** $0.006/minute of audio (~$0.02 for a 3.5-min video)  
**Requires:** `OPENAI_API_KEY` in `tests/.env.test`

```python
run_video_pipeline(
    cfg=cfg,
    url="https://www.youtube.com/watch?v=QFzEVtY_1lQ",
    title="Opel Corsa 2023 review",
    question="engine performance and fuel economy",
    brand="Opel", car_model="Corsa", year="2023", market="UK", trim="All",
    whisper_backend="openai-api",
    cleanup=False,
)
```

---

### Option E — Compare multiple backends side by side

**When to use:** Benchmarking, choosing the best backend for your use case.

```python
compare_backends(
    cfg=cfg,
    url="https://www.youtube.com/watch?v=QFzEVtY_1lQ",
    title="Opel Corsa 2023 review",
    question="engine performance and fuel economy",
    brand="Opel", car_model="Corsa", year="2023", market="UK", trim="All",
    backends=[
        "youtube-transcript-api",
        "faster-whisper",
        # "openai-whisper",   # slow on CPU, uncomment to include
        # "openai-api",       # costs ~$0.02, uncomment to include
    ],
    whisper_model="tiny",
    top_n=3,
    cleanup=False,
)
```

---

### Option F — PDF pipeline

**When to use:** Testing document ingestion (spec sheets, reports, manuals).  
**Speed:** ~211s for 6.2MB PDF (12 chunks), ~101s for 3MB PDF (16 chunks)

```python
# Opel Corsa 2025 IE spec sheet
run_pdf_pipeline(
    cfg=cfg,
    file_path="/ragflow/tests/Corsa_test.pdf",
    question="engine performance and fuel economy",
    brand="Opel", car_model="Corsa", year="2025", market="IE", trim="All",
    top_n=3,
    cleanup=False,
)

# Peugeot 208 2023 FR spec sheet
run_pdf_pipeline(
    cfg=cfg,
    file_path="/ragflow/tests/208_test.pdf",
    question="engine performance and fuel economy",
    brand="Peugeot", car_model="208", year="2023", market="FR", trim="All",
    top_n=3,
    cleanup=False,
)
```

> To use your own PDF:
> ```bash
> cp /path/to/your.pdf ~/ragflow/tests/your.pdf
> docker cp ~/ragflow/tests/your.pdf docker-ragflow-cpu-1:/ragflow/tests/your.pdf
> ```
> Then set `file_path="/ragflow/tests/your.pdf"` and the correct metadata.

---

### Option G — Brand-scoped retrieval

**When to use:** Querying existing datasets by brand/model — the main retrieval entry point for The Brain.

```python
import sys
sys.path.insert(0, '/ragflow/tests')
from test_ragflow_pipeline import load_config, get_datasets_by_brand_model, retrieve_by_brand_model, display_results

cfg = load_config()

# Discover all datasets for a brand+model
get_datasets_by_brand_model(cfg, "Opel", "Corsa")
# → Opel_Corsa_2023_UK_All_Video_... + Opel_Corsa_2025_IE_All_Docs_...

# Query across ALL Opel Corsa sources (all years, all markets)
chunks = retrieve_by_brand_model(cfg, "Opel", "Corsa", "engine performance")

# Query only 2025 IE Docs
chunks = retrieve_by_brand_model(cfg, "Opel", "Corsa", "engine performance",
                                  year="2025", market="IE", source_type="Docs")

# Query only UK Video
chunks = retrieve_by_brand_model(cfg, "Opel", "Corsa", "engine performance",
                                  market="UK", source_type="Video")

display_results(chunks)
```

**Available filters:**

| Parameter | Type | Description |
|---|---|---|
| `year` | string or None | `"2023"`, `"2025"` or `None` for all years |
| `market` | string or None | ISO code `"UK"`, `"FR"`, `"IE"` or `None` for all markets |
| `trim` | string or None | `"All"`, `"GS"`, `"Elegance"` or `None` for all trims |
| `source_type` | string or None | `"Video"`, `"Docs"` or `None` for all types |

---

## Real Results — YouTube Backends

**Video:** [Opel Corsa 2023 review](https://www.youtube.com/watch?v=QFzEVtY_1lQ)  
**Dataset:** `Opel_Corsa_2023_UK_All_Video_20260327_2005`  
**Query:** `"engine performance and fuel economy"`  
**Model size:** `tiny` for all local backends

---

### Backend 1: `youtube-transcript-api`

```
✅ Parsing complete in 5s — 6 chunks produced
🔍 Query: 'engine performance and fuel economy' → 1 chunks returned

  Chunk #1
  Similarity  : 0.6839 (term=0.6839, vector=0.6839)
  Video       : Opel Corsa 2023 review
  YouTube URL : https://www.youtube.com/watch?v=QFzEVtY_1lQ
  Timestamp   : 60s
  Deep-link   : https://www.youtube.com/watch?v=QFzEVtY_1lQ&t=60s
  Content     : Britain it must have something up its sleeve and you know what it kind
                of does let's take it for a drive I'll show you what I mean it's this
                little engine we got a 1.2 L turbocharged petrol engine and...

⏱️  Total pipeline time: 42.9s
```

---

### Backend 2: `faster-whisper` (tiny/CPU)

```
✅ Parsing complete in 68s — 6 chunks produced
🔍 Query: 'engine performance and fuel economy' → 1 chunks returned

  Chunk #1
  Similarity  : 0.6881 (term=0.6881, vector=0.6881)
  Video       : Opel Corsa 2023 review
  YouTube URL : https://www.youtube.com/watch?v=QFzEVtY_1lQ
  Timestamp   : 61s
  Deep-link   : https://www.youtube.com/watch?v=QFzEVtY_1lQ&t=61s
  Content     : for it to be one of the most popular cars in Britain. It must have
                something up its sleeve, and you know what, it kind of does. Let's
                take it for a drive and I'll show you what I mean. It's this littl...

⏱️  Total pipeline time: 68.5s
```

---

### Backend 3: `openai-whisper` (tiny/CPU)

```
✅ Parsing complete in 63s — 6 chunks produced
🔍 Query: 'engine performance and fuel economy' → 1 chunks returned

  Chunk #1
  Similarity  : 0.6868 (term=0.6868, vector=0.6868)
  Video       : Opel Corsa 2023 review
  YouTube URL : https://www.youtube.com/watch?v=QFzEVtY_1lQ
  Timestamp   : 61s
  Deep-link   : https://www.youtube.com/watch?v=QFzEVtY_1lQ&t=61s
  Content     : for it to be one of the most popular cars in Britain. It must have
                something up its sleeve, and you know what? It kind of does. Let's
                take it for a drive and I'll show you what I mean. It's this littl...

⏱️  Total pipeline time: 63.6s
```

---

### Backend 4: `openai-api` (cloud)

```
✅ Parsing complete in 31s — 6 chunks produced
🔍 Query: 'engine performance and fuel economy' → 1 chunks returned

  Chunk #1
  Similarity  : 0.6968 (term=0.6968, vector=0.6968)
  Video       : Opel Corsa 2023 review
  YouTube URL : https://www.youtube.com/watch?v=QFzEVtY_1lQ
  Timestamp   : 61s
  Deep-link   : https://www.youtube.com/watch?v=QFzEVtY_1lQ&t=61s
  Content     : cars in Britain. It must have something up its sleeve, and you know
                what? It kind of does. Let's take it for a drive, and I'll show you
                what I mean. It's this little engine. We've got a 1.2-litre turb...

⏱️  Total pipeline time: 32.0s
```

---

## Real Results — PDF Pipeline

### Opel Corsa 2025 IE — `Corsa_test.pdf`

**File:** `tests/Corsa_test.pdf` (6.2MB Opel Corsa 2025 IE spec sheet)  
**Dataset:** `Opel_Corsa_2025_IE_All_Docs_20260327_2005`  
**Query:** `"engine performance and fuel economy"`

```
✅ Parsing complete in 208s — 12 chunks produced
🔍 Query: 'engine performance and fuel economy' → 3 chunks returned

  Chunk #1
  Similarity  : 0.7133 (term=0.7133, vector=0.7133)
  Source      : Corsa_test.pdf
  Content     : <table><caption> CHOOSE YOUR WHEELS AND TYRES ENGINES: PERFORMANCE,
                FUEL ECONOMY AND EMISSIONS PETROL ENGINES E & OE July 2025.
                *Fuel consumption figures are determined according to the WLTP...

  Chunk #2
  Similarity  : 0.6914 (term=0.6914, vector=0.6914)
  Source      : Corsa_test.pdf
  Content     : <table><caption> ALLNEWCORSARANGEPRICES*</caption>
                Trim | Fuel Type | Transmission | CO2 (WLTP) | Tax Band...

  Chunk #3
  Similarity  : 0.6659 (term=0.6659, vector=0.6659)
  Source      : Corsa_test.pdf
  Content     : <table><caption> YOUR NEW CORSA IN DETAIL: COMFORT & CONVENIENCE
                COMFORT & CONVENIENCE FEATURES | SC | ELEGANCE | GS...

⏱️  Total pipeline time: 213.4s
```

---

### Peugeot 208 2023 FR — `208_test.pdf`

**File:** `tests/208_test.pdf` (3MB Peugeot 208 2023 FR spec sheet)  
**Dataset:** `Peugeot_208_2023_FR_All_Docs_20260327_2009`  
**Query:** `"engine performance and fuel economy"`

```
✅ Parsing complete in 101s — 16 chunks produced
⏱️  Total pipeline time: ~105s
```

> **Note:** RagFlow's PDF parser extracts tables as structured HTML — this preserves
> the table structure for better retrieval accuracy.

---

### Brand-scoped retrieval — Opel Corsa (all sources)

**Query across `Opel_Corsa_2023_UK_All_Video` + `Opel_Corsa_2025_IE_All_Docs`:**

```
🔎 Found 2 dataset(s) for Opel Corsa
   → Opel_Corsa_2025_IE_All_Docs_20260327_2005 (chunks=12)
   → Opel_Corsa_2023_UK_All_Video_20260327_2005 (chunks=6)
🔍 Query: 'engine performance and fuel economy' → 4 chunks from 2 dataset(s)

  Chunk #1  Similarity: 0.7133  Source: Corsa_test.pdf (PDF spec sheet)
  Chunk #2  Similarity: 0.6917  Source: Corsa_test.pdf (PDF prices table)
  Chunk #3  Similarity: 0.6839  Video: Opel Corsa 2023 review @ 60s → &t=60s
  Chunk #4  Similarity: 0.6660  Source: Corsa_test.pdf (comfort table)
```

---

## Full Comparison Summary

| Parser | Source | Dataset | Chunks | Parse Time | Top Similarity |
|---|---|---|---|---|---|
| `youtube-transcript-api` | Corsa video 2023 UK | `..._Video_...` | 6 | 5s | 0.6839 |
| `faster-whisper tiny` | Corsa video 2023 UK | `..._Video_...` | 6 | 68s | 0.6881 |
| `openai-whisper tiny` | Corsa video 2023 UK | `..._Video_...` | 6 | 63s | 0.6868 |
| `openai-api` | Corsa video 2023 UK | `..._Video_...` | 6 | 31s | 0.6968 |
| `PDF (naive)` | Corsa specs 2025 IE | `..._Docs_...` | 12 | 208s | 0.7133 |
| `PDF (naive)` | Peugeot 208 2023 FR | `..._Docs_...` | 16 | 101s | — |

### Key observations

- All 4 video backends return the **same segment (~60s)** — consistent retrieval ✅
- `openai-api` achieves the **highest similarity (0.6968)** among video backends ✅
- `youtube-transcript-api` is **14x faster** than local Whisper for captioned videos ✅
- Both local Whisper backends perform similarly (~65s, ~0.687 similarity) ✅
- PDF parser achieves **highest overall similarity (0.7133)** — structured data advantage ✅
- Brand-scoped retrieval returns chunks from **both video and PDF** in one call ✅
- For GCP GPU deployment, switch `faster-whisper` to `large` model for ~8s parse time ✅

### Recommended backend by use case

| Use Case | Recommended Backend | Model |
|---|---|---|
| Local dev / quick test | `youtube-transcript-api` | N/A |
| Production CPU | `faster-whisper` | `base` |
| Production GCP GPU | `faster-whisper` | `large` |
| No GPU, best accuracy | `openai-api` | N/A |
| Videos without captions | `faster-whisper` or `openai-api` | `base` / N/A |

---

## MCP Future Usage

All functions in `test_ragflow_pipeline.py` are structured as MCP-ready tools.
Each function has typed parameters and a docstring that maps directly to an MCP tool description.

### How functions map to MCP tools

```python
# ── Dataset management ──────────────────────────────────────────────────────
create_video_dataset(cfg, brand, car_model, year, market,
                     whisper_backend, whisper_model, trim, openai_api_key)
create_pdf_dataset(cfg, brand, car_model, year, market, trim)
list_datasets(cfg)
delete_dataset(cfg, dataset_id)

# ── Ingestion ───────────────────────────────────────────────────────────────
ingest_video(cfg, dataset_id, url, title)
ingest_pdf(cfg, dataset_id, file_path)

# ── Processing ──────────────────────────────────────────────────────────────
trigger_parsing(cfg, dataset_id, doc_id)
wait_for_completion(cfg, dataset_id, doc_id, timeout, poll_interval)

# ── Retrieval ───────────────────────────────────────────────────────────────
get_datasets_by_brand_model(cfg, brand, model, year, market, trim, source_type)
retrieve_by_brand_model(cfg, brand, model, question, year, market, trim, source_type, top_n)
retrieve(cfg, dataset_id, question, top_n, similarity_threshold)
display_results(chunks, max_content_length)

# ── Full pipeline runners ───────────────────────────────────────────────────
run_video_pipeline(cfg, url, title, question, brand, car_model, year, market,
                   trim, whisper_backend, whisper_model, cleanup)
run_pdf_pipeline(cfg, file_path, question, brand, car_model, year, market, trim, cleanup)
compare_backends(cfg, url, title, question, brand, car_model, year, market,
                 backends, whisper_model, top_n, cleanup)
```

### Example MCP workflow (future)

```python
# Via MCP, DeerFlow will call:

# 1. Ingest a new video source
result = mcp.call("create_video_dataset", {
    "brand": "Opel", "car_model": "Corsa", "year": "2023", "market": "UK",
    "whisper_backend": "faster-whisper", "whisper_model": "large"
})
mcp.call("ingest_video", {
    "dataset_id": result["id"],
    "url": "https://www.youtube.com/watch?v=QFzEVtY_1lQ",
    "title": "Opel Corsa 2023 review"
})

# 2. Query for a specific parameter (e.g. engine specs)
chunks = mcp.call("retrieve_by_brand_model", {
    "brand": "Opel", "model": "Corsa",
    "question": "engine displacement and power output",
    "year": "2025", "market": "IE", "source_type": "Docs"
})
```

### Parameter reference for each backend

| Backend | `whisper_backend` | `whisper_model` | `openai_api_key` |
|---|---|---|---|
| YouTube captions | `"youtube-transcript-api"` | ignored | not needed |
| Local Whisper (fast) | `"faster-whisper"` | `"tiny"` / `"base"` / `"large"` | not needed |
| Local Whisper (original) | `"openai-whisper"` | `"tiny"` / `"base"` / `"large"` | not needed |
| Cloud Whisper | `"openai-api"` | ignored | required |

### Market ISO codes reference

| Country | Code | Country | Code |
|---|---|---|---|
| United Kingdom | `UK` | Germany | `DE` |
| Ireland | `IE` | Italy | `IT` |
| France | `FR` | Spain | `ES` |
| Belgium | `BE` | Netherlands | `NL` |

---

*Last updated: March 2026 | RagFlow v0.24.0 | Branch: `feature/youtube-ingestion` | Brand/model/year/market/trim metadata structure added*
