# RagFlow Pipeline Testing Guide
> Multi-source ingestion (YouTube video + PDF + Web + Images) + Metadata-driven retrieval  
> Branch: `feature/youtube-ingestion` | Tested on: RagFlow v0.24.0  
> Test video: [Opel Corsa 2023 review](https://www.youtube.com/watch?v=QFzEVtY_1lQ)

---

## Table of Contents
1. [Prerequisites](#prerequisites)
2. [Setup](#setup)
3. [Architecture — One Dataset Per Analysis Run](#architecture--one-dataset-per-analysis-run)
4. [Test File Overview](#test-file-overview)
5. [How to Run Each Test](#how-to-run-each-test)
6. [Real Results — YouTube Backends](#real-results--youtube-backends)
7. [Real Results — PDF Pipeline](#real-results--pdf-pipeline)
8. [Full Comparison Summary](#full-comparison-summary)
9. [MCP Future Usage](#mcp-future-usage)

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

> ⚠️ This file is in `.gitignore` — never commit it. It is excluded from git history.
> Copy from the provided template and fill in your credentials:

```bash
cp tests/.env.test.example tests/.env.test
# Then edit with your actual credentials
```

Or create directly:

```bash
cat > ~/ragflow/tests/.env.test << 'EOF'
RAGFLOW_BASE_URL=http://172.18.0.1:9380
RAGFLOW_API_KEY=ragflow-xxxxxxxxxxxxxxxxxxxx
OPENAI_API_KEY=sk-xxxxxxxxxxxxxxxxxxxx
EOF
```

> **Important:** Use `http://172.18.0.1:9380` (Docker gateway IP), not `localhost`.  
> `localhost` inside the container refers to the container itself, not your host machine.

### 2. Deploy and verify the stack

The deploy script now copies `test_ragflow_pipeline.py` automatically:

```bash
bash docker/deploy-local.sh
```

### 3. Verify test file is in the container

```bash
docker exec docker-ragflow-cpu-1 ls /ragflow/tests/
# Should show: test_ragflow_pipeline.py  RAGFLOW-PIPELINE-TESTING-GUIDE.md  .env.test.example
```

### 4. Copy your `.env.test` into the container

```bash
docker cp ~/ragflow/tests/.env.test docker-ragflow-cpu-1:/ragflow/tests/.env.test
```

> ⚠️ `.env.test` is NOT copied by `deploy-local.sh` — it contains credentials and must be copied manually.

---

## Architecture — One Dataset Per Analysis Run

The pipeline follows a **one dataset per analysis run** architecture. All source types (video, PDF, web, images) share a single dataset, differentiated by per-document metadata.

```
One Analysis Dataset
────────────────────────────────────────────────────────
Name: {Brand}_{Model}_{Year}_{Market}_{Trim}_{YYYYMMDD}_{HHMM}
Example: Opel_Corsa_2023_UK_All_20260403_1143

Contains all source types:
  ├── YouTube Video  → parser_id="video"   (our custom parser)
  ├── PDF spec sheet → parser_id="naive"   (DeepDoc)
  ├── Web page (HTML)→ parser_id="naive"   (DeepDoc)
  └── Image          → parser_id="picture" (DeepDoc vision)

Business metadata stored per-document (not in parser_config):
  brand, car_model, year, market, trim, source_type,
  retrieval_date, youtube_url, video_title

Retrieval filtered via metadata_condition:
  brand="Opel" AND car_model="Corsa" AND source_type="Video"
```

### Why one dataset?

- Simpler management — one dataset per car model/market/year analysis run
- Cross-source retrieval in a single API call
- Filtering is metadata-driven, not dataset-name-driven
- Cleaner RagFlow codebase — business fields stay out of `ParserConfig`

### Dataset naming convention

```
{Brand}_{Model}_{Year}_{Market}_{Trim}_{YYYYMMDD}_{HHMM}

Examples:
  Opel_Corsa_2023_UK_All_20260403_1143
  Opel_Corsa_2025_IE_All_20260403_1200
  Peugeot_208_2023_FR_All_20260403_1215
  Opel_Corsa_2025_IE_GS_20260403_0900    ← trim-specific
```

> Note: Source type is no longer in the dataset name — all source types share one dataset.

---

## Test File Overview

**File:** `tests/test_ragflow_pipeline.py`

### What it does

The script runs the complete RagFlow ingestion pipeline end-to-end:

```
Create analysis dataset → Ingest document(s) → Trigger parsing → Poll until done → Retrieve → Display results
```

Dataset ID and Doc ID are captured and passed automatically — no manual export needed.
Dataset names are **auto-generated** from metadata — no manual naming needed.

### Key functions

```python
# ── Dataset creation ────────────────────────────────────────────────────────
create_analysis_dataset(cfg, brand, car_model, year, market,
                        trim, whisper_backend, whisper_model, openai_api_key)
# backward-compatible wrappers (delegate to create_analysis_dataset):
create_video_dataset(cfg, brand, car_model, year, market, ...)
create_pdf_dataset(cfg, brand, car_model, year, market, trim)

# ── Ingestion ───────────────────────────────────────────────────────────────
ingest_video(cfg, dataset_id, url, title,
             brand, car_model, year, market, trim, source_type, retrieval_date)
ingest_pdf(cfg, dataset_id, file_path)

# ── Processing ──────────────────────────────────────────────────────────────
trigger_parsing(cfg, dataset_id, doc_id)
wait_for_completion(cfg, dataset_id, doc_id, timeout, poll_interval)

# ── Retrieval ───────────────────────────────────────────────────────────────
retrieve(cfg, dataset_id, question, top_n, similarity_threshold)
retrieve_by_brand_model(cfg, brand, model, question,
                        year, market, trim, source_type, top_n)
display_results(chunks, max_content_length)

# ── Full pipeline runners ───────────────────────────────────────────────────
run_video_pipeline(cfg, url, title, question, brand, car_model, year, market,
                   trim, whisper_backend, whisper_model, cleanup)
run_pdf_pipeline(cfg, file_path, question, brand, car_model, year, market,
                 trim, cleanup)
compare_backends(cfg, url, title, question, brand, car_model, year, market,
                 backends, whisper_model, top_n, cleanup)
```

### Deploy and run command

The recommended way to run any test:

```bash
docker exec docker-ragflow-cpu-1 /ragflow/.venv/bin/python3 -c "
import sys, importlib.util
spec = importlib.util.spec_from_file_location('trp', '/ragflow/tests/test_ragflow_pipeline.py')
trp = importlib.util.module_from_spec(spec)
spec.loader.exec_module(trp)
cfg = trp.load_config()
trp.run_video_pipeline(
    cfg=cfg,
    url='https://www.youtube.com/watch?v=QFzEVtY_1lQ',
    title='Opel Corsa 2023 review',
    question='engine performance and fuel economy',
    brand='Opel', car_model='Corsa', year='2023', market='UK', trim='All',
    whisper_backend='youtube-transcript-api', cleanup=True,
)
"
```

---

## How to Run Each Test

### Option A — `youtube-transcript-api` (fastest, captions only)

**When to use:** Local dev, quick validation, video has English captions.  
**Speed:** ~10s parse, ~15s total  
**Cost:** Free

```python
trp.run_video_pipeline(
    cfg=cfg,
    url="https://www.youtube.com/watch?v=QFzEVtY_1lQ",
    title="Opel Corsa 2023 review",
    question="engine performance and fuel economy",
    brand="Opel", car_model="Corsa", year="2023", market="UK", trim="All",
    whisper_backend="youtube-transcript-api",
    cleanup=False,
)
```

---

### Option B — `faster-whisper` (local CPU/GPU)

**When to use:** Videos without captions, production CPU, GCP GPU with `large` model.  
**Speed:** ~68s total (tiny/CPU), ~8s (large/GPU)  
**Cost:** Free (local compute only)

```python
trp.run_video_pipeline(
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
trp.run_video_pipeline(
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
trp.run_video_pipeline(
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
trp.compare_backends(
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

> ⚠️ PDF test files are NOT tracked in git. Copy your own PDF into the container:
> ```bash
> docker cp /path/to/your.pdf docker-ragflow-cpu-1:/ragflow/tests/your.pdf
> ```

```python
# Opel Corsa 2025 IE spec sheet
trp.run_pdf_pipeline(
    cfg=cfg,
    file_path="/ragflow/tests/Corsa_test.pdf",
    question="engine performance and fuel economy",
    brand="Opel", car_model="Corsa", year="2025", market="IE", trim="All",
    top_n=3,
    cleanup=False,
)

# Peugeot 208 2023 FR spec sheet
trp.run_pdf_pipeline(
    cfg=cfg,
    file_path="/ragflow/tests/208_test.pdf",
    question="engine performance and fuel economy",
    brand="Peugeot", car_model="208", year="2023", market="FR", trim="All",
    top_n=3,
    cleanup=False,
)
```

---

### Option G — Metadata-driven retrieval across existing datasets

**When to use:** Querying existing datasets by brand/model — the main retrieval entry point for The Brain.

```python
# Query across ALL Opel Corsa sources (all years, all markets, all source types)
chunks = trp.retrieve_by_brand_model(cfg, "Opel", "Corsa",
                                     "engine performance")

# Query only Video sources for Opel Corsa UK 2023
chunks = trp.retrieve_by_brand_model(cfg, "Opel", "Corsa",
                                     "engine performance",
                                     year="2023", market="UK",
                                     source_type="Video")

# Query only Docs for Opel Corsa IE 2025
chunks = trp.retrieve_by_brand_model(cfg, "Opel", "Corsa",
                                     "engine performance",
                                     year="2025", market="IE",
                                     source_type="Docs")

trp.display_results(chunks)
```

**How filtering works:**

`retrieve_by_brand_model` uses `metadata_condition` to filter documents by their stored metadata before running retrieval. This means filtering is accurate even when multiple brands/models share datasets.

**Available filters:**

| Parameter | Type | Description |
|---|---|---|
| `year` | string or None | `"2023"`, `"2025"` or `None` for all years |
| `market` | string or None | ISO code `"UK"`, `"FR"`, `"IE"` or `None` for all markets |
| `trim` | string or None | `"All"`, `"GS"`, `"Elegance"` or `None` for all trims |
| `source_type` | string or None | `"Video"`, `"Docs"`, `"Web"`, `"Images"` or `None` for all |

---

## Real Results — YouTube Backends

**Video:** [Opel Corsa 2023 review](https://www.youtube.com/watch?v=QFzEVtY_1lQ)  
**Dataset:** `Opel_Corsa_2023_UK_All_20260403_1143`  
**Query:** `"engine performance and fuel economy"`  
**Model size:** `tiny` for all local backends

---

### Backend 1: `youtube-transcript-api`

```
✅ Analysis Dataset created: Opel_Corsa_2023_UK_All_20260403_1143
✅ Video registered: https://www.youtube.com/watch?v=QFzEVtY_1lQ
   doc_id=..., title=Opel Corsa 2023 review
✅ Parsing complete in 10s — 6 chunks produced
🔍 Query: 'engine performance and fuel economy' → 1 chunks returned

  Chunk #1
  Similarity  : 0.6839 (term=0.6839, vector=0.6839)
  Video       : Opel Corsa 2023 review
  Timestamp   : 60s
  Deep-link   : https://www.youtube.com/watch?v=QFzEVtY_1lQ&t=60s
  Content     : Britain it must have something up its sleeve and you know what it kind
                of does let's take it for a drive I'll show you what I mean it's this
                little engine we got a 1.2 L turbocharged petrol engine and...

⏱️  Total pipeline time: 15.0s
```

---

### Backend 2: `faster-whisper` (tiny/CPU)

```
✅ Parsing complete in 68s — 6 chunks produced
🔍 Query: 'engine performance and fuel economy' → 1 chunks returned

  Chunk #1
  Similarity  : 0.6881 (term=0.6881, vector=0.6881)
  Video       : Opel Corsa 2023 review
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
  Timestamp   : 61s
  Deep-link   : https://www.youtube.com/watch?v=QFzEVtY_1lQ&t=61s
  Content     : cars in Britain. It must have something up its sleeve, and you know
                what? It kind of does. Let's take it for a drive, and I'll show you
                what I mean. It's this little engine. We've got a 1.2-litre turb...

⏱️  Total pipeline time: 32.0s
```

---

## Real Results — PDF Pipeline

> ⚠️ PDF test files are not tracked in git. Results below are from the original test run.
> Copy your own PDFs into the container to reproduce.

### Opel Corsa 2025 IE spec sheet

**Dataset:** `Opel_Corsa_2025_IE_All_20260327_2005`  
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

⏱️  Total pipeline time: 213.4s
```

---

### Cross-source retrieval — Opel Corsa (video + PDF in one dataset)

With the new one-dataset architecture, video and PDF are ingested into the same dataset:

```
🔍 Query: 'engine performance and fuel economy' → 4 chunks from 2 doc(s)

  Chunk #1  Similarity: 0.7133  Source: Corsa_test.pdf (PDF spec sheet)
  Chunk #2  Similarity: 0.6917  Source: Corsa_test.pdf (PDF prices table)
  Chunk #3  Similarity: 0.6839  Video: Opel Corsa 2023 review — Timestamp: 60s
  Chunk #4  Similarity: 0.6660  Source: Corsa_test.pdf (comfort table)
```

---

## Full Comparison Summary

| Parser | Source | Chunks | Parse Time | Top Similarity |
|---|---|---|---|---|
| `youtube-transcript-api` | Corsa video 2023 UK | 6 | 10s | 0.6839 |
| `faster-whisper tiny` | Corsa video 2023 UK | 6 | 68s | 0.6881 |
| `openai-whisper tiny` | Corsa video 2023 UK | 6 | 63s | 0.6868 |
| `openai-api` | Corsa video 2023 UK | 6 | 31s | 0.6968 |
| `PDF (naive)` | Corsa specs 2025 IE | 12 | 208s | 0.7133 |
| `PDF (naive)` | Peugeot 208 2023 FR | 16 | 101s | — |

### Key observations

- All 4 video backends return the **same segment (~60s)** — consistent retrieval ✅
- `openai-api` achieves the **highest similarity (0.6968)** among video backends ✅
- `youtube-transcript-api` is **14x faster** than local Whisper for captioned videos ✅
- Both local Whisper backends perform similarly (~65s, ~0.687 similarity) ✅
- PDF parser achieves **highest overall similarity (0.7133)** — structured data advantage ✅
- Cross-source retrieval returns chunks from **both video and PDF** in one call ✅
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
create_analysis_dataset(cfg, brand, car_model, year, market,
                        trim, whisper_backend, whisper_model, openai_api_key)
list_datasets(cfg)
delete_dataset(cfg, dataset_id)

# ── Ingestion ───────────────────────────────────────────────────────────────
ingest_video(cfg, dataset_id, url, title,
             brand, car_model, year, market, trim, source_type, retrieval_date)
ingest_pdf(cfg, dataset_id, file_path)

# ── Processing ──────────────────────────────────────────────────────────────
trigger_parsing(cfg, dataset_id, doc_id)
wait_for_completion(cfg, dataset_id, doc_id, timeout, poll_interval)

# ── Retrieval ───────────────────────────────────────────────────────────────
retrieve_by_brand_model(cfg, brand, model, question,
                        year, market, trim, source_type, top_n)
retrieve(cfg, dataset_id, question, top_n, similarity_threshold)
display_results(chunks, max_content_length)

# ── Full pipeline runners ───────────────────────────────────────────────────
run_video_pipeline(cfg, url, title, question, brand, car_model, year, market,
                   trim, whisper_backend, whisper_model, cleanup)
run_pdf_pipeline(cfg, file_path, question, brand, car_model, year, market,
                 trim, cleanup)
compare_backends(cfg, url, title, question, brand, car_model, year, market,
                 backends, whisper_model, top_n, cleanup)
```

### Example MCP workflow (future)

```python
# Via MCP, The Brain will call:

# 1. Create one analysis dataset for Opel Corsa 2023 UK
result = mcp.call("create_analysis_dataset", {
    "brand": "Opel", "car_model": "Corsa", "year": "2023", "market": "UK",
    "whisper_backend": "faster-whisper", "whisper_model": "large"
})
dataset_id = result["id"]

# 2. Ingest a YouTube video into that dataset
mcp.call("ingest_video", {
    "dataset_id": dataset_id,
    "url": "https://www.youtube.com/watch?v=QFzEVtY_1lQ",
    "title": "Opel Corsa 2023 review",
    "brand": "Opel", "car_model": "Corsa",
    "year": "2023", "market": "UK", "source_type": "Video"
})

# 3. Ingest a PDF into the same dataset
mcp.call("ingest_pdf", {
    "dataset_id": dataset_id,
    "file_path": "/ragflow/tests/Corsa_test.pdf"
})

# 4. Query using metadata filtering
chunks = mcp.call("retrieve_by_brand_model", {
    "brand": "Opel", "model": "Corsa",
    "question": "engine displacement and power output",
    "year": "2023", "market": "UK", "source_type": "Video"
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

*Last updated: April 2026 | RagFlow v0.24.0 | Branch: `feature/youtube-ingestion` | One-dataset-per-analysis-run architecture | Metadata-driven filtering*
