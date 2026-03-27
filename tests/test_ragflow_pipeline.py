"""
test_ragflow_pipeline.py
========================
RagFlow pipeline testing utility — YouTube video ingestion + PDF ingestion.

All functions are structured as MCP-ready tools:
  - typed parameters with defaults
  - docstrings describing each parameter and return value
  - returns dict (JSON-serializable) for easy MCP wrapping

Usage:
  1. Fill in tests/.env.test with your API keys
  2. Run a full pipeline:
       python tests/test_ragflow_pipeline.py
  3. Or import and call individual functions:
       from tests.test_ragflow_pipeline import run_video_pipeline
       run_video_pipeline(...)

Backends reference:
  youtube-transcript-api  : fast (~1s), no download, requires video captions
  faster-whisper          : local CPU/GPU (~30s tiny / ~60s base on CPU)
  openai-whisper          : local CPU/GPU (~2min tiny on CPU)
  openai-api              : cloud (~10s), costs $0.006/min, requires OPENAI_API_KEY

Model sizes (faster-whisper / openai-whisper):
  tiny   : fastest, lower accuracy  — good for quick local tests
  base   : balanced speed/accuracy  — recommended for local CPU
  small  : better accuracy          — ~2x slower than base
  medium : high accuracy            — requires ~5GB RAM
  large  : best accuracy            — recommended for GCP GPU (~8s/video)
"""

import os
import sys
import time
import json
from pathlib import Path
from typing import Optional

import requests
from dotenv import load_dotenv

# ── Load .env.test ─────────────────────────────────────────────────────────────

ENV_FILE = Path(__file__).parent / ".env.test"


def load_config() -> dict:
    """
    MCP tool: load_config
    Load API keys and base URL from tests/.env.test.

    Returns:
        dict with keys: base_url, ragflow_api_key, openai_api_key
    """
    if not ENV_FILE.exists():
        raise FileNotFoundError(
            f"Missing config file: {ENV_FILE}\n"
            "Create it with:\n"
            "  RAGFLOW_BASE_URL=http://localhost:9380\n"
            "  RAGFLOW_API_KEY=your-ragflow-key\n"
            "  OPENAI_API_KEY=your-openai-key\n"
        )
    load_dotenv(ENV_FILE)
    return {
        "base_url":        os.getenv("RAGFLOW_BASE_URL", "http://localhost:9380").rstrip("/"),
        "ragflow_api_key": os.getenv("RAGFLOW_API_KEY", ""),
        "openai_api_key":  os.getenv("OPENAI_API_KEY", ""),
    }


def _headers(cfg: dict) -> dict:
    return {
        "Authorization": f"Bearer {cfg['ragflow_api_key']}",
        "Content-Type": "application/json",
    }


# ── Dataset management ─────────────────────────────────────────────────────────

def list_datasets(cfg: dict) -> list:
    """
    MCP tool: list_datasets
    List all datasets in RagFlow.

    Args:
        cfg: config dict from load_config()

    Returns:
        list of dataset dicts (id, name, chunk_method, chunk_count, ...)
    """
    resp = requests.get(
        f"{cfg['base_url']}/api/v1/datasets",
        headers=_headers(cfg),
    )
    data = resp.json()
    if data.get("code") != 0:
        raise RuntimeError(f"list_datasets failed: {data.get('message')}")
    return data["data"]


MARKET_ISO_CODES = {
    "United Kingdom": "UK", "Ireland": "IE", "France": "FR",
    "Germany": "DE", "Italy": "IT", "Spain": "ES", "Belgium": "BE",
    "Netherlands": "NL", "Portugal": "PT", "Poland": "PL",
    "Austria": "AT", "Switzerland": "CH", "Sweden": "SE",
    "Norway": "NO", "Denmark": "DK", "Finland": "FI",
}

def _normalize_market(market: str) -> str:
    """Convert full country name to ISO code if needed."""
    if market in MARKET_ISO_CODES.values():
        return market  # already ISO code
    return MARKET_ISO_CODES.get(market, market.upper()[:2])

def _build_dataset_name(brand: str, car_model: str, year: str,
                         market: str, trim: str, source_type: str) -> str:
    """
    Build standardized dataset name:
    {Brand}_{Model}_{Year}_{Market}_{Trim}_{SourceType}_{YYYYMMDD}_{HHMM}
    Example: Opel_Corsa_2023_UK_All_Video_20260327_1445
    """
    from datetime import datetime
    now = datetime.now()
    return f"{brand}_{car_model}_{year}_{market}_{trim}_{source_type}_{now.strftime('%Y%m%d')}_{now.strftime('%H%M')}"

def create_video_dataset(
    cfg: dict,
    brand: str,
    car_model: str,
    year: str,
    market: str,
    whisper_backend: str = "youtube-transcript-api",
    whisper_model: str = "base",
    trim: str = "All",
    openai_api_key: str = "",
) -> dict:
    """
    MCP tool: create_video_dataset
    Create a RagFlow dataset for YouTube video ingestion.
    Dataset name is auto-generated: {Brand}_{Model}_{Year}_{Market}_{Trim}_Video_{YYYYMMDD}_{HHMM}

    Args:
        cfg             : config dict from load_config()
        brand           : car brand e.g. "Opel", "Peugeot", "Fiat"
        car_model       : car model e.g. "Corsa", "208", "500"
        year            : model year e.g. "2023", "2024", "2025"
        market          : target market — ISO code or full name:
                            "UK" or "United Kingdom"
                            "IE" or "Ireland"
                            "FR" or "France"
                            "DE" or "Germany"
        whisper_backend : transcription backend:
                            "youtube-transcript-api"  — fast, captions only (default)
                            "faster-whisper"          — local CPU/GPU
                            "openai-whisper"          — local CPU/GPU (original)
                            "openai-api"              — cloud (needs key)
        whisper_model   : "tiny" | "base" | "small" | "medium" | "large"
        trim            : car trim level e.g. "GS", "Elegance", "All" (default: "All")
        openai_api_key  : only for "openai-api" backend

    Returns:
        dict with dataset metadata including "id" and "name"
    """
    from datetime import date
    if not openai_api_key:
        openai_api_key = cfg.get("openai_api_key", "")

    market_iso = _normalize_market(market)
    name = _build_dataset_name(brand, car_model, year, market_iso, trim, "Video")
    retrieval_date = date.today().isoformat()

    parser_config = {
        "whisper_backend":  whisper_backend,
        "whisper_model":    whisper_model,
        "brand":            brand,
        "car_model":        car_model,
        "year":             year,
        "market":           market_iso,
        "trim":             trim,
        "source_type":      "Video",
        "retrieval_date":   retrieval_date,
    }
    if openai_api_key and whisper_backend == "openai-api":
        parser_config["openai_api_key"] = openai_api_key

    resp = requests.post(
        f"{cfg['base_url']}/api/v1/datasets",
        headers=_headers(cfg),
        json={
            "name":            name,
            "chunk_method":    "video",
            "embedding_model": "BAAI/bge-small-en-v1.5@Builtin",
            "parser_config":   parser_config,
        },
    )
    data = resp.json()
    if data.get("code") != 0:
        raise RuntimeError(f"create_video_dataset failed: {data.get('message')}")
    print(f"✅ Video Dataset created: {data['data']['name']}")
    print(f"   id={data['data']['id']}")
    print(f"   brand={brand}, car_model={car_model}, year={year}, market={market_iso}, trim={trim}")
    print(f"   whisper_backend={whisper_backend}, retrieval_date={retrieval_date}")
    return data["data"]


def create_pdf_dataset(
    cfg: dict,
    brand: str,
    car_model: str,
    year: str,
    market: str,
    trim: str = "All",
) -> dict:
    """
    MCP tool: create_pdf_dataset
    Create a RagFlow dataset for PDF/HTML/Image/Excel ingestion (DeepDoc parser).
    Dataset name is auto-generated: {Brand}_{Model}_{Year}_{Market}_{Trim}_Docs_{YYYYMMDD}_{HHMM}

    Args:
        cfg       : config dict from load_config()
        brand     : car brand e.g. "Opel", "Peugeot"
        car_model : car model e.g. "Corsa", "208"
        year      : model year e.g. "2023", "2025"
        market    : target market — ISO code or full name
        trim      : car trim level (default: "All")

    Returns:
        dict with dataset metadata including "id" and "name"
    """
    from datetime import date
    market_iso = _normalize_market(market)
    name = _build_dataset_name(brand, car_model, year, market_iso, trim, "Docs")
    retrieval_date = date.today().isoformat()

    resp = requests.post(
        f"{cfg['base_url']}/api/v1/datasets",
        headers=_headers(cfg),
        json={
            "name":            name,
            "chunk_method":    "naive",
            "embedding_model": "BAAI/bge-small-en-v1.5@Builtin",
            "parser_config": {
                "brand":          brand,
                "car_model":      car_model,
                "year":           year,
                "market":         market_iso,
                "trim":           trim,
                "source_type":    "Docs",
                "retrieval_date": retrieval_date,
            },
        },
    )
    data = resp.json()
    if data.get("code") != 0:
        raise RuntimeError(f"create_pdf_dataset failed: {data.get('message')}")
    print(f"✅ Docs Dataset created: {data['data']['name']}")
    print(f"   id={data['data']['id']}")
    print(f"   brand={brand}, car_model={car_model}, year={year}, market={market_iso}, trim={trim}")
    print(f"   retrieval_date={retrieval_date}")
    return data["data"]


def delete_dataset(cfg: dict, dataset_id: str) -> bool:
    """
    MCP tool: delete_dataset
    Delete a dataset and all its documents and chunks.

    Args:
        cfg        : config dict from load_config()
        dataset_id : dataset ID to delete

    Returns:
        True if deleted successfully
    """
    resp = requests.delete(
        f"{cfg['base_url']}/api/v1/datasets",
        headers=_headers(cfg),
        json={"ids": [dataset_id]},
    )
    data = resp.json()
    if data.get("code") != 0:
        raise RuntimeError(f"delete_dataset failed: {data.get('message')}")
    print(f"✅ Dataset {dataset_id} deleted")
    return True


# ── Ingestion ──────────────────────────────────────────────────────────────────

def ingest_video(
    cfg: dict,
    dataset_id: str,
    url: str,
    title: str = "",
) -> dict:
    """
    MCP tool: ingest_video
    Register a YouTube video URL as a document in a dataset.
    The dataset must have been created with chunk_method="video".

    Args:
        cfg        : config dict from load_config()
        dataset_id : target dataset ID
        url        : YouTube URL (youtube.com/watch or youtu.be format)
        title      : human-readable title stored with each chunk (optional)

    Returns:
        dict with document metadata including "id" field
    """
    resp = requests.post(
        f"{cfg['base_url']}/api/v1/datasets/{dataset_id}/videos",
        headers=_headers(cfg),
        json={"url": url, "title": title},
    )
    data = resp.json()
    if data.get("code") != 0:
        raise RuntimeError(f"ingest_video failed: {data.get('message')}")
    doc = data["data"][0]
    print(f"✅ Video registered: {url}")
    print(f"   doc_id={doc['id']}, title={title or '(none)'}")
    return doc


def ingest_pdf(
    cfg: dict,
    dataset_id: str,
    file_path: str,
) -> dict:
    """
    MCP tool: ingest_pdf
    Upload a local PDF file as a document in a dataset.
    The dataset must have been created with chunk_method="naive" (or any non-video method).

    Args:
        cfg        : config dict from load_config()
        dataset_id : target dataset ID
        file_path  : absolute or relative path to the PDF file on disk.
                     Example: "/home/user/docs/report.pdf"
                     To download a PDF first, use:
                       import urllib.request
                       urllib.request.urlretrieve(pdf_url, "/tmp/report.pdf")
                       ingest_pdf(cfg, dataset_id, "/tmp/report.pdf")

    Returns:
        dict with document metadata including "id" field
    """
    path = Path(file_path)
    if not path.exists():
        raise FileNotFoundError(f"PDF not found: {file_path}")

    headers = {"Authorization": f"Bearer {cfg['ragflow_api_key']}"}
    with open(path, "rb") as f:
        resp = requests.post(
            f"{cfg['base_url']}/api/v1/datasets/{dataset_id}/documents",
            headers=headers,
            files={"file": (path.name, f, "application/pdf")},
        )
    data = resp.json()
    if data.get("code") != 0:
        raise RuntimeError(f"ingest_pdf failed: {data.get('message')}")
    doc = data["data"][0]
    print(f"✅ PDF uploaded: {path.name} (doc_id={doc['id']})")
    return doc


# ── Processing ─────────────────────────────────────────────────────────────────

def trigger_parsing(
    cfg: dict,
    dataset_id: str,
    doc_id: str,
) -> bool:
    """
    MCP tool: trigger_parsing
    Trigger chunking + embedding for a document.

    Args:
        cfg        : config dict from load_config()
        dataset_id : dataset containing the document
        doc_id     : document ID to parse

    Returns:
        True if task queued successfully
    """
    resp = requests.post(
        f"{cfg['base_url']}/api/v1/datasets/{dataset_id}/chunks",
        headers=_headers(cfg),
        json={"document_ids": [doc_id]},
    )
    data = resp.json()
    if data.get("code") != 0:
        raise RuntimeError(f"trigger_parsing failed: {data.get('message')}")
    print(f"✅ Parsing triggered for doc_id={doc_id}")
    return True


def wait_for_completion(
    cfg: dict,
    dataset_id: str,
    doc_id: str,
    timeout: int = 300,
    poll_interval: int = 5,
) -> dict:
    """
    MCP tool: wait_for_completion
    Poll document status until parsing is done or failed.
    Uses container logs monitoring since the /documents API does not
    return video URL-based documents in v0.24.0.
    Prints progress to terminal while waiting.

    Args:
        cfg           : config dict from load_config()
        dataset_id    : dataset containing the document
        doc_id        : document ID to monitor
        timeout       : max seconds to wait before giving up (default: 300)
        poll_interval : seconds between status checks (default: 5)

    Returns:
        dict with doc_id and chunk_count
    """
    print(f"⏳ Waiting for parsing to complete (timeout={timeout}s)...")
    start = time.time()
    last_count = -1

    while time.time() - start < timeout:
        elapsed = int(time.time() - start)

        # Poll dataset chunk_count — increases from 0 as parsing completes
        resp = requests.get(
            f"{cfg['base_url']}/api/v1/datasets",
            headers=_headers(cfg),
            params={"id": dataset_id},
        )
        data = resp.json()
        datasets = data.get("data", [])
        if datasets:
            chunk_count = datasets[0].get("chunk_count", 0)
            doc_count   = datasets[0].get("document_count", 0)

            if chunk_count != last_count:
                print(f"  [{elapsed}s] chunk_count={chunk_count}, document_count={doc_count}")
                last_count = chunk_count

            # Parsing done when chunk_count > 0
            if chunk_count > 0:
                print(f"✅ Parsing complete in {elapsed}s — {chunk_count} chunks produced")
                return {"doc_id": doc_id, "chunk_count": chunk_count}

        time.sleep(poll_interval)

    raise TimeoutError(f"⏰ Parsing did not complete within {timeout}s")


# ── Retrieval ──────────────────────────────────────────────────────────────────

def retrieve(
    cfg: dict,
    dataset_id: str,
    question: str,
    top_n: int = 5,
    similarity_threshold: float = 0.1,
) -> list:
    """
    MCP tool: retrieve
    Query a dataset and return matching chunks.

    Args:
        cfg                  : config dict from load_config()
        dataset_id           : dataset to query
        question             : natural language query
        top_n                : max number of chunks to return (default: 5)
        similarity_threshold : minimum similarity score 0.0–1.0 (default: 0.1)

    Returns:
        list of chunk dicts. Each chunk contains:
          - content             : transcript/document text
          - similarity          : combined similarity score (0.0–1.0)
          - term_similarity     : keyword match score
          - vector_similarity   : semantic similarity score
          For video chunks additionally:
          - youtube_url         : original YouTube URL
          - video_id            : 11-char YouTube video ID
          - video_title         : title provided at ingestion
          - timestamp_seconds   : start time of segment in video
          - transcript_segment  : deep-link URL (&t=Xs) to exact moment
    """
    resp = requests.post(
        f"{cfg['base_url']}/api/v1/retrieval",
        headers=_headers(cfg),
        json={
            "question":            question,
            "dataset_ids":         [dataset_id],
            "similarity_threshold": similarity_threshold,
            "top_n":               top_n,
        },
    )
    data = resp.json()
    if data.get("code") != 0:
        raise RuntimeError(f"retrieve failed: {data.get('message')}")
    chunks = data.get("data", {}).get("chunks", [])
    print(f"🔍 Query: '{question}' → {len(chunks)} chunks returned")
    return chunks


def display_results(chunks: list, max_content_length: int = 200) -> None:
    """
    MCP tool: display_results
    Pretty-print retrieval results to terminal.

    Args:
        chunks             : list of chunk dicts from retrieve()
        max_content_length : truncate content preview to this length (default: 200)
    """
    if not chunks:
        print("  (no results)")
        return

    for i, chunk in enumerate(chunks, 1):
        print(f"\n{'─'*60}")
        print(f"  Chunk #{i}")
        print(f"{'─'*60}")
        print(f"  Similarity  : {chunk.get('similarity', 0):.4f} "
              f"(term={chunk.get('term_similarity', 0):.4f}, "
              f"vector={chunk.get('vector_similarity', 0):.4f})")

        # Video-specific fields
        if chunk.get("youtube_url"):
            print(f"  Video       : {chunk.get('video_title', 'N/A')}")
            print(f"  YouTube URL : {chunk.get('youtube_url')}")
            print(f"  Timestamp   : {chunk.get('timestamp_seconds')}s")
            print(f"  Deep-link   : {chunk.get('transcript_segment')}")
        else:
            print(f"  Source      : {chunk.get('document_keyword', 'N/A')}")

        content = chunk.get("content", "")
        if len(content) > max_content_length:
            content = content[:max_content_length] + "..."
        print(f"  Content     : {content}")

# ── Brand-model scoped dataset discovery and retrieval ──────────────────────────────────────────────────────────────────

def get_datasets_by_brand_model(
    cfg: dict,
    brand: str,
    model: str,
    year: str | list | None = None,
    market: str | None = None,
    trim: str | None = None,
    source_type: str | None = None,
) -> list:
    """
    MCP tool: get_datasets_by_brand_model
    Find all datasets matching brand + model, optionally filtered by
    year, market, trim and/or source type.

    Args:
        cfg         : config dict from load_config()
        brand       : car brand e.g. "Opel"
        model       : car model e.g. "Corsa"
        year        : optional — None=all, "2023"=exact, ["2023","2025"]=multiple
        market      : optional — ISO code e.g. "UK", "FR", "IE"
        trim        : optional — e.g. "All", "GS", "Elegance"
        source_type : optional — "Video", "Docs", "Web", "Images"

    Returns:
        list of matching dataset dicts
    """
    all_datasets = list_datasets(cfg)
    prefix = f"{brand}_{model}_"
    matched = [d for d in all_datasets if d["name"].startswith(prefix)]

    if year is not None:
        years = [year] if isinstance(year, str) else year
        matched = [d for d in matched if any(f"_{y}_" in d["name"] for y in years)]

    if market is not None:
        market_iso = _normalize_market(market)
        matched = [d for d in matched if f"_{market_iso}_" in d["name"]]

    if trim is not None:
        matched = [d for d in matched if f"_{trim}_" in d["name"]]

    if source_type is not None:
        matched = [d for d in matched if f"_{source_type}_" in d["name"]]

    print(f"🔎 Found {len(matched)} dataset(s) for {brand} {model}"
          + (f" {year}" if year else "")
          + (f" {market}" if market else "")
          + (f" {trim}" if trim else "")
          + (f" [{source_type}]" if source_type else ""))
    for d in matched:
        print(f"   → {d['name']} (chunks={d['chunk_count']})")
    return matched


def retrieve_by_brand_model(
    cfg: dict,
    brand: str,
    model: str,
    question: str,
    year: str | list | None = None,
    market: str | None = None,
    trim: str | None = None,
    source_type: str | None = None,
    top_n: int = 5,
    similarity_threshold: float = 0.1,
) -> list:
    """
    MCP tool: retrieve_by_brand_model
    Query all datasets for a brand+model, optionally scoped by year,
    market, trim and/or source type.

    Args:
        cfg                  : config dict from load_config()
        brand                : car brand e.g. "Opel"
        model                : car model e.g. "Corsa"
        question             : natural language query
        year                 : optional year filter
        market               : optional market filter e.g. "UK", "FR"
        trim                 : optional trim filter e.g. "All", "GS"
        source_type          : optional source type filter e.g. "Video", "Docs"
        top_n                : max chunks to return (default: 5)
        similarity_threshold : min similarity 0.0-1.0 (default: 0.1)

    Returns:
        list of chunk dicts with full metadata

    Examples:
        # All sources for Opel Corsa
        retrieve_by_brand_model(cfg, "Opel", "Corsa", "engine performance")

        # UK market only
        retrieve_by_brand_model(cfg, "Opel", "Corsa", "engine performance", market="UK")

        # 2025 IE Docs only
        retrieve_by_brand_model(cfg, "Opel", "Corsa", "engine performance",
                                year="2025", market="IE", source_type="Docs")
    """
    datasets = get_datasets_by_brand_model(cfg, brand, model, year, market, trim, source_type)

    if not datasets:
        print(f"⚠️  No datasets found for {brand} {model}")
        return []

    dataset_ids = [d["id"] for d in datasets]

    resp = requests.post(
        f"{cfg['base_url']}/api/v1/retrieval",
        headers=_headers(cfg),
        json={
            "question":             question,
            "dataset_ids":          dataset_ids,
            "similarity_threshold": similarity_threshold,
            "top_n":                top_n,
        },
    )
    data = resp.json()
    if data.get("code") != 0:
        raise RuntimeError(f"retrieve_by_brand_model failed: {data.get('message')}")

    chunks = data.get("data", {}).get("chunks", [])
    print(f"🔍 Query: '{question}' → {len(chunks)} chunks from {len(dataset_ids)} dataset(s)")
    return chunks

# ── Full pipeline runners ──────────────────────────────────────────────────────

def run_video_pipeline(
    cfg: dict,
    url: str,
    title: str,
    question: str,
    whisper_backend: str = "youtube-transcript-api",
    whisper_model: str = "base",
    openai_api_key: str = "",
    top_n: int = 3,
    similarity_threshold: float = 0.1,
    cleanup: bool = False,
) -> dict:
    """
    MCP tool: run_video_pipeline
    Run the complete video ingestion pipeline end-to-end:
    create dataset → ingest video → parse → retrieve → display.

    Args:
        cfg                  : config dict from load_config()
        url                  : YouTube URL to ingest
        title                : human-readable video title
        question             : retrieval query to test after ingestion
        whisper_backend      : transcription backend (see module docstring)
        whisper_model        : model size for local Whisper backends
        openai_api_key       : OpenAI key for "openai-api" backend (uses .env if empty)
        top_n                : number of chunks to retrieve
        similarity_threshold : minimum similarity score
        cleanup              : if True, delete dataset after test (default: False)

    Returns:
        dict with keys: dataset_id, doc_id, chunks, elapsed_seconds
    """
    import re
    # Generate unique dataset name from backend + timestamp
    ts = int(time.time())
    safe_backend = whisper_backend.replace("-", "_")
    dataset_name = f"test_{safe_backend}_{ts}"

    print(f"\n{'='*60}")
    print(f"  VIDEO PIPELINE TEST")
    print(f"  Backend : {whisper_backend}")
    print(f"  Model   : {whisper_model if whisper_backend not in ('youtube-transcript-api', 'openai-api') else 'N/A'}")
    print(f"  URL     : {url}")
    print(f"{'='*60}")

    start = time.time()

    # Step 1 — Create dataset
    dataset = create_video_dataset(
        cfg, dataset_name, whisper_backend, whisper_model, openai_api_key
    )
    dataset_id = dataset["id"]

    # Step 2 — Ingest video
    doc = ingest_video(cfg, dataset_id, url, title)
    doc_id = doc["id"]

    # Step 3 — Trigger parsing
    trigger_parsing(cfg, dataset_id, doc_id)

    # Step 4 — Wait for completion
    wait_for_completion(cfg, dataset_id, doc_id)

    # Step 5 — Retrieve
    chunks = retrieve(cfg, dataset_id, question, top_n, similarity_threshold)

    # Step 6 — Display
    display_results(chunks)

    elapsed = round(time.time() - start, 1)
    print(f"\n⏱️  Total pipeline time: {elapsed}s")

    if cleanup:
        delete_dataset(cfg, dataset_id)

    return {
        "dataset_id":      dataset_id,
        "doc_id":          doc_id,
        "chunks":          chunks,
        "elapsed_seconds": elapsed,
        "whisper_backend": whisper_backend,
        "whisper_model":   whisper_model,
    }


def run_pdf_pipeline(
    cfg: dict,
    file_path: str,
    question: str,
    top_n: int = 3,
    similarity_threshold: float = 0.1,
    cleanup: bool = False,
) -> dict:
    """
    MCP tool: run_pdf_pipeline
    Run the complete PDF ingestion pipeline end-to-end:
    create dataset → upload PDF → parse → retrieve → display.

    Args:
        cfg                  : config dict from load_config()
        file_path            : absolute path to PDF file on disk.
                               To download a PDF first:
                                 import urllib.request
                                 urllib.request.urlretrieve(pdf_url, "/tmp/doc.pdf")
                                 run_pdf_pipeline(cfg, "/tmp/doc.pdf", question)
        question             : retrieval query to test after ingestion
        top_n                : number of chunks to retrieve
        similarity_threshold : minimum similarity score
        cleanup              : if True, delete dataset after test (default: False)

    Returns:
        dict with keys: dataset_id, doc_id, chunks, elapsed_seconds
    """
    ts = int(time.time())
    dataset_name = f"test_pdf_{ts}"

    print(f"\n{'='*60}")
    print(f"  PDF PIPELINE TEST")
    print(f"  File    : {file_path}")
    print(f"{'='*60}")

    start = time.time()

    # Step 1 — Create dataset
    dataset = create_pdf_dataset(cfg, dataset_name)
    dataset_id = dataset["id"]

    # Step 2 — Upload PDF
    doc = ingest_pdf(cfg, dataset_id, file_path)
    doc_id = doc["id"]

    # Step 3 — Trigger parsing
    trigger_parsing(cfg, dataset_id, doc_id)

    # Step 4 — Wait for completion
    wait_for_completion(cfg, dataset_id, doc_id)

    # Step 5 — Retrieve
    chunks = retrieve(cfg, dataset_id, question, top_n, similarity_threshold)

    # Step 6 — Display
    display_results(chunks)

    elapsed = round(time.time() - start, 1)
    print(f"\n⏱️  Total pipeline time: {elapsed}s")

    if cleanup:
        delete_dataset(cfg, dataset_id)

    return {
        "dataset_id":      dataset_id,
        "doc_id":          doc_id,
        "chunks":          chunks,
        "elapsed_seconds": elapsed,
    }


def compare_backends(
    cfg: dict,
    url: str,
    title: str,
    question: str,
    backends: Optional[list] = None,
    whisper_model: str = "tiny",
    top_n: int = 3,
    cleanup: bool = False,
) -> dict:
    """
    MCP tool: compare_backends
    Run the same video through multiple backends and compare results side by side.

    Args:
        cfg           : config dict from load_config()
        url           : YouTube URL to test
        title         : human-readable video title
        question      : retrieval query used for all backends
        backends      : list of backends to test, default:
                          ["youtube-transcript-api", "faster-whisper",
                           "openai-whisper", "openai-api"]
                        Remove any backend you don't want to test.
                        Remove "openai-api" if you don't have an OpenAI key.
        whisper_model : model size for faster-whisper and openai-whisper (default: "tiny")
        top_n         : chunks to retrieve per backend
        cleanup       : delete all test datasets after comparison

    Returns:
        dict mapping backend_name -> pipeline result dict
    """
    if backends is None:
        backends = [
            "youtube-transcript-api",
            "faster-whisper",
            "openai-whisper",
            "openai-api",
        ]

    results = {}
    summary = []

    for backend in backends:
        try:
            result = run_video_pipeline(
                cfg=cfg,
                url=url,
                title=title,
                question=question,
                whisper_backend=backend,
                whisper_model=whisper_model,
                top_n=top_n,
                cleanup=cleanup,
            )
            results[backend] = result
            top_similarity = result["chunks"][0].get("similarity", 0) if result["chunks"] else 0
            summary.append({
                "backend":        backend,
                "chunks":         len(result["chunks"]),
                "elapsed":        result["elapsed_seconds"],
                "top_similarity": round(top_similarity, 4),
                "status":         "✅ OK",
            })
        except Exception as e:
            results[backend] = {"error": str(e)}
            summary.append({
                "backend": backend,
                "status":  f"❌ FAILED: {e}",
            })

    # Print comparison table
    print(f"\n{'='*60}")
    print("  BACKEND COMPARISON SUMMARY")
    print(f"{'='*60}")
    print(f"  {'Backend':<30} {'Status':<12} {'Elapsed':>8} {'Chunks':>7} {'TopSim':>8}")
    print(f"  {'-'*30} {'-'*12} {'-'*8} {'-'*7} {'-'*8}")
    for s in summary:
        if "elapsed" in s:
            print(f"  {s['backend']:<30} {s['status']:<12} {s['elapsed']:>7}s "
                  f"{s['chunks']:>7} {s['top_similarity']:>8.4f}")
        else:
            print(f"  {s['backend']:<30} {s['status']}")
    print(f"{'='*60}\n")

    return results


# ── Main — run all tests ───────────────────────────────────────────────────────
if __name__ == "__main__":
    # ── Config ──────────────────────────────────────────────────────────────────
    cfg = load_config()

    # ── Test video details ───────────────────────────────────────────────────────
    VIDEO_URL   = "https://www.youtube.com/watch?v=QFzEVtY_1lQ"
    VIDEO_TITLE = "Vauxhall Corsa 2024 review"
    QUESTION    = "engine performance and fuel economy"

    # ── Choose what to run — uncomment ONE block at a time ───────────────────────

    # Option A: youtube-transcript-api (fast, ~10s, captions only)
    # run_video_pipeline(
    #     cfg=cfg, url=VIDEO_URL, title=VIDEO_TITLE, question=QUESTION,
    #     whisper_backend="youtube-transcript-api", whisper_model="base", cleanup=False,
    # )

    # Option B: faster-whisper (local CPU, ~60s with tiny model)
    # run_video_pipeline(
    #     cfg=cfg, url=VIDEO_URL, title=VIDEO_TITLE, question=QUESTION,
    #     whisper_backend="faster-whisper", whisper_model="tiny", cleanup=False,
    # )

    # Option C: openai-whisper (local CPU, ~2-3min with tiny model)
    # run_video_pipeline(
    #     cfg=cfg, url=VIDEO_URL, title=VIDEO_TITLE, question=QUESTION,
    #     whisper_backend="openai-whisper", whisper_model="tiny", cleanup=False,
    # )

    # Option D: openai-api (cloud, ~30s, costs ~$0.02/video)
    # run_video_pipeline(
    #     cfg=cfg, url=VIDEO_URL, title=VIDEO_TITLE, question=QUESTION,
    #     whisper_backend="openai-api", cleanup=False,
    # )

    # Option E: compare multiple backends side by side
    # compare_backends(
    #     cfg=cfg, url=VIDEO_URL, title=VIDEO_TITLE, question=QUESTION,
    #     backends=["youtube-transcript-api", "faster-whisper"],
    #     whisper_model="tiny", top_n=3, cleanup=False,
    # )

    # Option F: PDF pipeline test
    run_pdf_pipeline(
        cfg=cfg, file_path="/ragflow/tests/Corsa_test.pdf",
        question="engine performance and fuel economy", top_n=3, cleanup=False,
    )
