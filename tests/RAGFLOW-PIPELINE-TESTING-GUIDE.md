# RagFlow Pipeline Testing Guide
> YouTube video ingestion (4 backends) + PDF ingestion  
> Branch: `feature/youtube-ingestion` | Tested on: RagFlow v0.24.0  
> Test video: [Vauxhall Corsa 2024 review](https://www.youtube.com/watch?v=QFzEVtY_1lQ)  
> Test PDF: `tests/Corsa_test.pdf`

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

# Copy PDF test file (if testing PDF pipeline)
docker cp tests/Corsa_test.pdf docker-ragflow-cpu-1:/ragflow/tests/Corsa_test.pdf
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

### How to switch between tests

Scroll to the `__main__` block at the bottom of the file. **Uncomment exactly one option at a time**, comment out all others.

```python
# ✅ Active — this will run
run_video_pipeline(
    cfg=cfg, url=VIDEO_URL, title=VIDEO_TITLE, question=QUESTION,
    whisper_backend="youtube-transcript-api", whisper_model="base", cleanup=False,
)

# ❌ Inactive — commented out
# run_video_pipeline(
#     cfg=cfg, url=VIDEO_URL, title=VIDEO_TITLE, question=QUESTION,
#     whisper_backend="faster-whisper", whisper_model="tiny", cleanup=False,
# )
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

Uncomment in `__main__`:
```python
run_video_pipeline(
    cfg=cfg,
    url=VIDEO_URL,
    title=VIDEO_TITLE,
    question=QUESTION,
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

Uncomment in `__main__`:
```python
run_video_pipeline(
    cfg=cfg,
    url=VIDEO_URL,
    title=VIDEO_TITLE,
    question=QUESTION,
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

Uncomment in `__main__`:
```python
run_video_pipeline(
    cfg=cfg,
    url=VIDEO_URL,
    title=VIDEO_TITLE,
    question=QUESTION,
    whisper_backend="openai-whisper",
    whisper_model="tiny",    # tiny | base | small | medium | large
    cleanup=False,
)
```

---

### Option D — `openai-api` (cloud, fastest paid option)

**When to use:** Fastest transcription, no GPU needed, pay-per-use.  
**Speed:** ~31s total  
**Cost:** $0.006/minute of audio (~$0.02 for a 3.5-min video)  
**Requires:** `OPENAI_API_KEY` in `tests/.env.test`

Uncomment in `__main__`:
```python
run_video_pipeline(
    cfg=cfg,
    url=VIDEO_URL,
    title=VIDEO_TITLE,
    question=QUESTION,
    whisper_backend="openai-api",
    whisper_model="base",   # ignored — API always uses whisper-1
    cleanup=False,
)
```

---

### Option E — Compare multiple backends side by side

**When to use:** Benchmarking, choosing the best backend for your use case.

Uncomment in `__main__`:
```python
compare_backends(
    cfg=cfg,
    url=VIDEO_URL,
    title=VIDEO_TITLE,
    question=QUESTION,
    backends=[
        "youtube-transcript-api",
        "faster-whisper",
        # "openai-whisper",   # slow on CPU, uncomment to include
        # "openai-api",       # costs ~$0.02, uncomment to include
    ],
    whisper_model="tiny",   # model size for local backends
    top_n=3,
    cleanup=False,
)
```

---

### Option F — PDF pipeline

**When to use:** Testing document ingestion (spec sheets, reports, manuals).  
**Speed:** ~211s for 6.2MB PDF (12 chunks)

Uncomment in `__main__`:
```python
run_pdf_pipeline(
    cfg=cfg,
    file_path="/ragflow/tests/Corsa_test.pdf",  # path inside container
    question="engine performance and fuel economy",
    top_n=3,
    cleanup=False,
)
```

> To use your own PDF:
> ```bash
> cp /path/to/your.pdf ~/ragflow/tests/your.pdf
> docker cp ~/ragflow/tests/your.pdf docker-ragflow-cpu-1:/ragflow/tests/your.pdf
> ```
> Then set `file_path="/ragflow/tests/your.pdf"` in the script.

---

## Real Results — YouTube Backends

**Video:** [Vauxhall Corsa 2024 review](https://www.youtube.com/watch?v=QFzEVtY_1lQ)  
**Query:** `"engine performance and fuel economy"`  
**Model size:** `tiny` for all local backends

---

### Backend 1: `youtube-transcript-api`

```
✅ Parsing complete in 5s — 6 chunks produced
🔍 Query: 'engine performance and fuel economy' → 1 chunks returned

  Chunk #1
  Similarity  : 0.6839 (term=0.6839, vector=0.6839)
  Video       : Vauxhall Corsa 2024 review
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
  Video       : Vauxhall Corsa 2024 review
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
  Video       : Vauxhall Corsa 2024 review
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
  Video       : Vauxhall Corsa 2024 review
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

**File:** `tests/Corsa_test.pdf` (6.2MB Vauxhall Corsa spec sheet)  
**Query:** `"engine performance and fuel economy"`

```
✅ Parsing complete in 211s — 12 chunks produced
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

> **Note:** RagFlow's PDF parser extracts tables as structured HTML — this preserves
> the table structure for better retrieval accuracy. Chunk #1 hits exactly the right
> table: "ENGINES: PERFORMANCE, FUEL ECONOMY AND EMISSIONS".

---

## Full Comparison Summary

| Parser | Source | Chunks | Parse Time | Top Similarity | Notes |
|---|---|---|---|---|---|
| `youtube-transcript-api` | Corsa video | 6 | 5s | 0.6839 | Fastest, requires captions |
| `faster-whisper tiny` | Corsa video | 6 | 68s | 0.6881 | Local CPU, no API key |
| `openai-whisper tiny` | Corsa video | 6 | 63s | 0.6868 | Local CPU, no API key |
| `openai-api` | Corsa video | 6 | 31s | 0.6968 | Cloud, highest similarity |
| `PDF (naive)` | Corsa_test.pdf | 12 | 211s | 0.7133 | Best similarity, structured tables |

### Key observations

- All 4 video backends return the **same segment (~60s)** — consistent retrieval ✅
- `openai-api` achieves the **highest similarity (0.6968)** among video backends ✅
- `youtube-transcript-api` is **14x faster** than local Whisper for captioned videos ✅
- Both local Whisper backends perform similarly (~65s, ~0.687 similarity) ✅
- PDF parser achieves **highest overall similarity (0.7133)** — structured data advantage ✅
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
# Each function becomes an MCP tool:
create_video_dataset(cfg, name, whisper_backend, whisper_model, openai_api_key)
create_pdf_dataset(cfg, name)
ingest_video(cfg, dataset_id, url, title)
ingest_pdf(cfg, dataset_id, file_path)
trigger_parsing(cfg, dataset_id, doc_id)
wait_for_completion(cfg, dataset_id, doc_id, timeout, poll_interval)
retrieve(cfg, dataset_id, question, top_n, similarity_threshold)
display_results(chunks, max_content_length)
run_video_pipeline(cfg, url, title, question, whisper_backend, ...)
run_pdf_pipeline(cfg, file_path, question, ...)
compare_backends(cfg, url, title, question, backends, ...)
```

### Example MCP workflow (future)

```python
# Via MCP, you'll be able to call:
result = mcp.call("create_video_dataset", {
    "name": "Car_Reviews",
    "whisper_backend": "faster-whisper",
    "whisper_model": "large"
})

mcp.call("ingest_video", {
    "dataset_id": result["id"],
    "url": "https://www.youtube.com/watch?v=QFzEVtY_1lQ",
    "title": "Vauxhall Corsa 2024 review"
})
```

### Parameter reference for each backend

| Backend | `whisper_backend` | `whisper_model` | `openai_api_key` |
|---|---|---|---|
| YouTube captions | `"youtube-transcript-api"` | ignored | not needed |
| Local Whisper (fast) | `"faster-whisper"` | `"tiny"` / `"base"` / `"large"` | not needed |
| Local Whisper (original) | `"openai-whisper"` | `"tiny"` / `"base"` / `"large"` | not needed |
| Cloud Whisper | `"openai-api"` | ignored | required |

---

*Last updated: March 2026 | RagFlow v0.24.0 | Branch: `feature/youtube-ingestion`*
