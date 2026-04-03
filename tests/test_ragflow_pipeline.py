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

Dataset naming convention:
  {Brand}_{Model}_{Year}_{Market}_{Trim}_{YYYYMMDD}_{HHMM}
  Example: Opel_Corsa_2023_UK_All_20260403_1143
  All source types (video, PDF, web, images) share one dataset per analysis run.
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
            "  RAGFLOW_BASE_URL=http://172.18.0.1:9380\n"
            "  RAGFLOW_API_KEY=your-ragflow-key\n"
            "  OPENAI_API_KEY=your-openai-key\n"
        )
    load_dotenv(ENV_FILE)
    return {
        "base_url":        os.getenv("RAGFLOW_BASE_URL", "http://172.18.0.1:9380").rstrip("/"),
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

# To be updated with the full country name → ISO code mapping as needed.
# This allows users to input either format when creating datasets.
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
                         market: str, trim: str, source_type: str = "") -> str:
    """
    Build standardized dataset name:
    {Brand}_{Model}_{Year}_{Market}_{Trim}_{YYYYMMDD}_{HHMM}
    Example: Opel_Corsa_2023_UK_All_20260403_1143
    source_type is no longer in the name — all source types share one dataset.
    source_type param kept for backward compatibility but ignored.
    """
    from datetime import datetime
    now = datetime.now()
    return f"{brand}_{car_model}_{year}_{market}_{trim}_{now.strftime('%Y%m%d')}_{now.strftime('%H%M')}"


def create_analysis_dataset(
    cfg: dict,
    brand: str,
    car_model: str,
    year: str,
    market: str,
    trim: str = "All",
    whisper_backend: str = "youtube-transcript-api",
    whisper_model: str = "base",
    openai_api_key: str = "",
) -> dict:
    """
    MCP tool: create_analysis_dataset
    Create a single RagFlow dataset for one analysis run.
    All source types (video, PDF, web, images) share this dataset.
    Dataset name: {Brand}_{Model}_{Year}_{Market}_{Trim}_{YYYYMMDD}_{HHMM}
    chunk_method is "naive" by default — video documents override per-document
    via parser_id="video" set in the ingest_video endpoint.

    Args:
        cfg             : config dict from load_config()
        brand           : car brand e.g. "Opel", "Peugeot"
        car_model       : car model e.g. "Corsa", "208"
        year            : model year e.g. "2023", "2025"
        market          : target market ISO code or full name
        trim            : car trim level (default: "All")
        whisper_backend : default Whisper backend for video docs in this dataset
        whisper_model   : Whisper model size for local backends
        openai_api_key  : OpenAI key for "openai-api" backend

    Returns:
        dict with dataset metadata including "id" and "name"
    """
    from datetime import date
    market_iso = _normalize_market(market)
    name = _build_dataset_name(brand, car_model, year, market_iso, trim)
    retrieval_date = date.today().isoformat()

    # dataset-level parser_config: only technical Whisper defaults
    # business fields are stored per-document in DocMetadataService
    parser_config = {
        "whisper_backend": whisper_backend,
        "whisper_model":   whisper_model,
    }
    if openai_api_key and whisper_backend == "openai-api":
        parser_config["openai_api_key"] = openai_api_key

    resp = requests.post(
        f"{cfg['base_url']}/api/v1/datasets",
        headers=_headers(cfg),
        json={
            "name":            name,
            "chunk_method":    "naive",
            "embedding_model": "BAAI/bge-small-en-v1.5@Builtin",
            "parser_config":   parser_config,
        },
    )
    data = resp.json()
    if data.get("code") != 0:
        raise RuntimeError(f"create_analysis_dataset failed: {data.get('message')}")
    print(f"✅ Analysis Dataset created: {data['data']['name']}")
    print(f"   id={data['data']['id']}")
    print(f"   brand={brand}, car_model={car_model}, year={year}, market={market_iso}, trim={trim}")
    print(f"   whisper_backend={whisper_backend}, retrieval_date={retrieval_date}")
    return data["data"]


# kept for backward compatibility
def create_video_dataset(cfg, brand, car_model, year, market,
                         whisper_backend="youtube-transcript-api",
                         whisper_model="base", trim="All", openai_api_key=""):
    """Backward-compatible wrapper — use create_analysis_dataset() instead."""
    return create_analysis_dataset(cfg, brand, car_model, year, market,
                                   trim=trim, whisper_backend=whisper_backend,
                                   whisper_model=whisper_model,
                                   openai_api_key=openai_api_key)


def create_pdf_dataset(cfg, brand, car_model, year, market, trim="All"):
    """Backward-compatible wrapper — use create_analysis_dataset() instead."""
    return create_analysis_dataset(cfg, brand, car_model, year, market, trim=trim)


def create_web_dataset(cfg, brand, car_model, year, market, trim="All"):
    """Backward-compatible wrapper — use create_analysis_dataset() instead."""
    return create_analysis_dataset(cfg, brand, car_model, year, market, trim=trim)


def create_image_dataset(cfg, brand, car_model, year, market, trim="All"):
    """Backward-compatible wrapper — use create_analysis_dataset() instead."""
    return create_analysis_dataset(cfg, brand, car_model, year, market, trim=trim)


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
    brand: str = "",
    car_model: str = "",
    year: str = "",
    market: str = "",
    trim: str = "",
    source_type: str = "Video",
    retrieval_date: str = "",
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
    from datetime import date as _date
    _payload = {
        "url":            url,
        "title":          title,
        "brand":          brand,
        "car_model":      car_model,
        "year":           year,
        "market":         market,
        "trim":           trim,
        "source_type":    source_type,
        "retrieval_date": retrieval_date or _date.today().isoformat(),
    }
    resp = requests.post(
        f"{cfg['base_url']}/api/v1/datasets/{dataset_id}/videos",
        headers=_headers(cfg),
        json=_payload,
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
                     Example: "/ragflow/tests/Corsa_test.pdf"
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


def ingest_html(
    cfg: dict,
    dataset_id: str,
    source: str,
) -> dict:
    """
    MCP tool: ingest_html
    Upload an HTML page as a document in a dataset.
    Accepts either a local file path or a URL (auto-downloaded to /tmp).
    The dataset must have been created with chunk_method="naive".

    Args:
        cfg        : config dict from load_config()
        dataset_id : target dataset ID (created with create_web_dataset())
        source     : one of:
                       - absolute/relative local path: "/ragflow/tests/corsa.html"
                       - HTTP/HTTPS URL: "https://www.opel.ie/cars/corsa.html"
                     If a URL is given, the page is fetched and saved to
                     /tmp/<sanitised_filename>.html before upload.

    Returns:
        dict with document metadata including "id" field
    """
    import urllib.request
    import re

    # ── Resolve source to a local file path ──────────────────────────────────
    if source.startswith("http://") or source.startswith("https://"):
        safe_name = re.sub(r"[^\w\-.]", "_", source.split("//", 1)[-1])[:80]
        if not safe_name.endswith(".html"):
            safe_name += ".html"
        local_path = Path(f"/tmp/{safe_name}")
        print(f"🌐 Fetching HTML from URL: {source}")
        urllib.request.urlretrieve(source, local_path)
        print(f"   Saved to: {local_path}")
    else:
        local_path = Path(source)

    if not local_path.exists():
        raise FileNotFoundError(f"HTML file not found: {local_path}")

    # ── Upload ────────────────────────────────────────────────────────────────
    upload_headers = {"Authorization": f"Bearer {cfg['ragflow_api_key']}"}
    with open(local_path, "rb") as f:
        resp = requests.post(
            f"{cfg['base_url']}/api/v1/datasets/{dataset_id}/documents",
            headers=upload_headers,
            files={"file": (local_path.name, f, "text/html")},
        )
    data = resp.json()
    if data.get("code") != 0:
        raise RuntimeError(f"ingest_html failed: {data.get('message')}")
    doc = data["data"][0]
    print(f"✅ HTML uploaded: {local_path.name} (doc_id={doc['id']})")
    return doc


def ingest_image(
    cfg: dict,
    dataset_id: str,
    source: str,
) -> dict:
    """
    MCP tool: ingest_image
    Upload an image file as a document in a dataset.
    Accepts either a local file path or a URL (auto-downloaded to /tmp).
    The dataset must have been created with chunk_method="picture".

    Supported formats: JPEG, PNG, WEBP, BMP, TIFF (anything DeepDoc picture
    parser accepts). DeepDoc will OCR any text present and generate a
    semantic description of the visual content.

    Args:
        cfg        : config dict from load_config()
        dataset_id : target dataset ID (created with create_image_dataset())
        source     : one of:
                       - absolute/relative local path: "/ragflow/tests/corsa_badge.jpg"
                       - HTTP/HTTPS URL: "https://example.com/corsa_spec.jpg"
                     If a URL is given, the image is fetched and saved to
                     /tmp/<sanitised_filename> before upload.

    Returns:
        dict with document metadata including "id" field
    """
    import urllib.request
    import mimetypes
    import re

    # ── MIME type map for upload Content-Type header ──────────────────────────
    _MIME = {
        ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
        ".png": "image/png",  ".webp": "image/webp",
        ".bmp": "image/bmp",  ".tiff": "image/tiff", ".tif": "image/tiff",
    }

    # ── Resolve source to a local file path ──────────────────────────────────
    if source.startswith("http://") or source.startswith("https://"):
        safe_name = re.sub(r"[^\w\-.]", "_", source.split("//", 1)[-1])[:80]
        ext = Path(safe_name).suffix.lower()
        if ext not in _MIME:
            safe_name += ".jpg"
        local_path = Path(f"/tmp/{safe_name}")
        print(f"🌐 Fetching image from URL: {source}")
        urllib.request.urlretrieve(source, local_path)
        print(f"   Saved to: {local_path}")
    else:
        local_path = Path(source)

    if not local_path.exists():
        raise FileNotFoundError(f"Image file not found: {local_path}")

    ext = local_path.suffix.lower()
    mime = _MIME.get(ext, mimetypes.guess_type(str(local_path))[0] or "image/jpeg")

    # ── Upload ────────────────────────────────────────────────────────────────
    upload_headers = {"Authorization": f"Bearer {cfg['ragflow_api_key']}"}
    with open(local_path, "rb") as f:
        resp = requests.post(
            f"{cfg['base_url']}/api/v1/datasets/{dataset_id}/documents",
            headers=upload_headers,
            files={"file": (local_path.name, f, mime)},
        )
    data = resp.json()
    if data.get("code") != 0:
        raise RuntimeError(f"ingest_image failed: {data.get('message')}")
    doc = data["data"][0]
    print(f"✅ Image uploaded: {local_path.name} (doc_id={doc['id']}, mime={mime})")
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
    Query a single dataset and return matching chunks.

    Args:
        cfg                  : config dict from load_config()
        dataset_id           : dataset to query
        question             : natural language query
        top_n                : max number of chunks to return (default: 5)
        similarity_threshold : minimum similarity score 0.0-1.0 (default: 0.1)

    Returns:
        list of chunk dicts. Each chunk contains:
          - content             : transcript/document text
          - similarity          : combined similarity score (0.0-1.0)
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

        # Video-specific fields (timestamp_seconds and transcript_segment now in properties)
        _props = chunk.get("properties", {})
        if _props.get("timestamp_seconds") is not None:
            print(f"  Video       : {chunk.get('docnm_kwd', 'N/A')}")
            print(f"  Timestamp   : {_props.get('timestamp_seconds')}s")
            print(f"  Deep-link   : {_props.get('transcript_segment', 'N/A')}")
        else:
            print(f"  Source      : {chunk.get('document_keyword', 'N/A')}")

        content = chunk.get("content", "")
        if len(content) > max_content_length:
            content = content[:max_content_length] + "..."
        print(f"  Content     : {content}")


# ── Brand-model scoped dataset discovery and retrieval ─────────────────────────

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

    Examples:
        get_datasets_by_brand_model(cfg, "Opel", "Corsa")
          → [Opel_Corsa_2023_UK_All_Video_..., Opel_Corsa_2025_IE_All_Docs_...]

        get_datasets_by_brand_model(cfg, "Opel", "Corsa", year="2025")
          → [Opel_Corsa_2025_IE_All_Docs_...]

        get_datasets_by_brand_model(cfg, "Opel", "Corsa", market="UK", source_type="Video")
          → [Opel_Corsa_2023_UK_All_Video_...]
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
    market, trim and/or source type. Main retrieval entry point for The Brain.

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
        list of chunk dicts with full metadata + source traceability

    Examples:
        # All sources for Opel Corsa (all years, all markets)
        retrieve_by_brand_model(cfg, "Opel", "Corsa", "engine performance")

        # Only 2025 IE Docs
        retrieve_by_brand_model(cfg, "Opel", "Corsa", "engine performance",
                                year="2025", market="IE", source_type="Docs")

        # Only UK Video
        retrieve_by_brand_model(cfg, "Opel", "Corsa", "engine performance",
                                market="UK", source_type="Video")
    """
    datasets = get_datasets_by_brand_model(cfg, brand, model, year, market, trim, source_type)

    if not datasets:
        print(f"⚠️  No datasets found for {brand} {model}")
        return []

    dataset_ids = [d["id"] for d in datasets]

    # ── metadata-driven doc_id filtering ─────────────────────────────────────
    _conditions = [
        {"key": "brand",     "value": brand, "operator": "eq"},
        {"key": "car_model", "value": model, "operator": "eq"},
    ]
    if year:
        years = [year] if isinstance(year, str) else year
        for y in years:
            _conditions.append({"key": "year", "value": y, "operator": "eq"})
    if market:
        _conditions.append({"key": "market", "value": _normalize_market(market), "operator": "eq"})
    if trim:
        _conditions.append({"key": "trim", "value": trim, "operator": "eq"})
    if source_type:
        _conditions.append({"key": "source_type", "value": source_type, "operator": "eq"})

    import json as _json
    _meta_condition = {"conditions": _conditions, "logic": "and"}
    _doc_ids = []
    for _ds_id in dataset_ids:
        _dresp = requests.get(
            f"{cfg['base_url']}/api/v1/datasets/{_ds_id}/documents",
            headers=_headers(cfg),
            params={"metadata_condition": _json.dumps(_meta_condition), "page_size": 1000},
        )
        for _doc in _dresp.json().get("data", {}).get("docs", []):
            _doc_ids.append(_doc["id"])

    if not _doc_ids:
        print(f"⚠️  No documents matched metadata filters for {brand} {model}")
        return []

    resp = requests.post(
        f"{cfg['base_url']}/api/v1/retrieval",
        headers=_headers(cfg),
        json={
            "question":             question,
            "dataset_ids":          dataset_ids,
            "doc_ids":              _doc_ids,
            "similarity_threshold": similarity_threshold,
            "top_n":                top_n,
        },
    )
    data = resp.json()
    if data.get("code") != 0:
        raise RuntimeError(f"retrieve_by_brand_model failed: {data.get('message')}")

    chunks = data.get("data", {}).get("chunks", [])
    print(f"🔍 Query: '{question}' → {len(chunks)} chunks from {len(_doc_ids)} doc(s) in {len(dataset_ids)} dataset(s)")
    return chunks


# ── Full pipeline runners ──────────────────────────────────────────────────────

def run_video_pipeline(
    cfg: dict,
    url: str,
    title: str,
    question: str,
    brand: str,
    car_model: str,
    year: str,
    market: str,
    trim: str = "All",
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
        brand                : car brand e.g. "Opel", "Peugeot"
        car_model            : car model e.g. "Corsa", "208"
        year                 : model year e.g. "2023", "2025"
        market               : target market ISO code or full name e.g. "UK", "FR"
        trim                 : trim level (default: "All")
        whisper_backend      : transcription backend (see module docstring)
        whisper_model        : model size for local Whisper backends
        openai_api_key       : OpenAI key for "openai-api" backend
        top_n                : number of chunks to retrieve
        similarity_threshold : minimum similarity score
        cleanup              : if True, delete dataset after test (default: False)

    Returns:
        dict with keys: dataset_id, doc_id, chunks, elapsed_seconds, whisper_backend
    """
    print(f"\n{'='*60}")
    print(f"  VIDEO PIPELINE TEST")
    print(f"  Backend : {whisper_backend}")
    print(f"  Model   : {whisper_model if whisper_backend not in ('youtube-transcript-api', 'openai-api') else 'N/A'}")
    print(f"  Brand   : {brand} {car_model} {year} {_normalize_market(market)}")
    print(f"  URL     : {url}")
    print(f"{'='*60}")

    start = time.time()

    # Step 1 — Create analysis dataset (all source types share one dataset)
    dataset = create_analysis_dataset(
        cfg, brand, car_model, year, market,
        trim=trim,
        whisper_backend=whisper_backend,
        whisper_model=whisper_model,
        openai_api_key=openai_api_key,
    )
    dataset_id = dataset["id"]

    # Step 2 — Ingest video
    doc = ingest_video(
        cfg, dataset_id, url, title,
        brand=brand, car_model=car_model, year=year,
        market=market, trim=trim, source_type="Video",
    )
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
    brand: str,
    car_model: str,
    year: str,
    market: str,
    trim: str = "All",
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
        file_path            : absolute path to PDF file on disk (inside container).
                               Example: "/ragflow/tests/Corsa_test.pdf"
                               To download a PDF first:
                                 import urllib.request
                                 urllib.request.urlretrieve(pdf_url, "/tmp/doc.pdf")
                                 run_pdf_pipeline(cfg, "/tmp/doc.pdf", question, ...)
        question             : retrieval query to test after ingestion
        brand                : car brand e.g. "Opel", "Peugeot"
        car_model            : car model e.g. "Corsa", "208"
        year                 : model year e.g. "2023", "2025"
        market               : target market ISO code or full name
        trim                 : trim level (default: "All")
        top_n                : number of chunks to retrieve
        similarity_threshold : minimum similarity score
        cleanup              : if True, delete dataset after test (default: False)

    Returns:
        dict with keys: dataset_id, doc_id, chunks, elapsed_seconds
    """
    print(f"\n{'='*60}")
    print(f"  PDF PIPELINE TEST")
    print(f"  File    : {file_path}")
    print(f"  Brand   : {brand} {car_model} {year} {_normalize_market(market)}")
    print(f"{'='*60}")

    start = time.time()

    # Step 1 — Create dataset
    dataset = create_analysis_dataset(cfg, brand, car_model, year, market, trim=trim)
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


def run_web_pipeline(
    cfg: dict,
    source: str,
    question: str,
    brand: str,
    car_model: str,
    year: str,
    market: str,
    trim: str = "All",
    top_n: int = 3,
    similarity_threshold: float = 0.1,
    cleanup: bool = False,
) -> dict:
    """
    MCP tool: run_web_pipeline
    Run the complete HTML/web ingestion pipeline end-to-end:
    create dataset → upload HTML → parse → retrieve → display.

    Accepts a local HTML file path or a live URL. If a URL is given,
    the page is fetched to /tmp before upload (see ingest_html).

    Args:
        cfg                  : config dict from load_config()
        source               : local path or URL of the HTML page.
                               Local example : "/ragflow/tests/corsa_ie.html"
                               URL example   : "https://www.opel.ie/cars/corsa.html"
        question             : retrieval query to test after ingestion
        brand                : car brand e.g. "Opel", "Peugeot"
        car_model            : car model e.g. "Corsa", "208"
        year                 : model year e.g. "2023", "2025"
        market               : target market ISO code or full name
        trim                 : trim level (default: "All")
        top_n                : number of chunks to retrieve
        similarity_threshold : minimum similarity score
        cleanup              : if True, delete dataset after test (default: False)

    Returns:
        dict with keys: dataset_id, doc_id, chunks, elapsed_seconds, source
    """
    print(f"\n{'='*60}")
    print(f"  WEB PIPELINE TEST")
    print(f"  Source  : {source}")
    print(f"  Brand   : {brand} {car_model} {year} {_normalize_market(market)}")
    print(f"{'='*60}")

    start = time.time()

    # Step 1 — Create dataset
    dataset = create_web_dataset(cfg, brand, car_model, year, market, trim)
    dataset_id = dataset["id"]

    # Step 2 — Upload HTML (local or URL)
    doc = ingest_html(cfg, dataset_id, source)
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
        "source":          source,
    }


def run_image_pipeline(
    cfg: dict,
    source: str,
    question: str,
    brand: str,
    car_model: str,
    year: str,
    market: str,
    trim: str = "All",
    top_n: int = 3,
    similarity_threshold: float = 0.1,
    cleanup: bool = False,
) -> dict:
    """
    MCP tool: run_image_pipeline
    Run the complete image ingestion pipeline end-to-end:
    create dataset → upload image → parse (DeepDoc picture) → retrieve → display.

    Accepts a local image file path or a URL. If a URL is given, the image
    is fetched to /tmp before upload (see ingest_image).

    Args:
        cfg                  : config dict from load_config()
        source               : local path or URL of the image file.
                               Local example : "/ragflow/tests/corsa_badge.jpg"
                               URL example   : "https://example.com/corsa_spec.png"
        question             : retrieval query to test after ingestion
        brand                : car brand e.g. "Opel", "Peugeot"
        car_model            : car model e.g. "Corsa", "208"
        year                 : model year e.g. "2023", "2025"
        market               : target market ISO code or full name
        trim                 : trim level (default: "All")
        top_n                : number of chunks to retrieve
        similarity_threshold : minimum similarity score
        cleanup              : if True, delete dataset after test (default: False)

    Returns:
        dict with keys: dataset_id, doc_id, chunks, elapsed_seconds, source
    """
    print(f"\n{'='*60}")
    print(f"  IMAGE PIPELINE TEST")
    print(f"  Source  : {source}")
    print(f"  Brand   : {brand} {car_model} {year} {_normalize_market(market)}")
    print(f"{'='*60}")

    start = time.time()

    # Step 1 — Create dataset
    dataset = create_image_dataset(cfg, brand, car_model, year, market, trim)
    dataset_id = dataset["id"]

    # Step 2 — Upload image (local or URL)
    doc = ingest_image(cfg, dataset_id, source)
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
        "source":          source,
    }


def compare_backends(
    cfg: dict,
    url: str,
    title: str,
    question: str,
    brand: str,
    car_model: str,
    year: str,
    market: str,
    trim: str = "All",
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
        brand         : car brand e.g. "Opel"
        car_model     : car model e.g. "Corsa"
        year          : model year e.g. "2023"
        market        : target market ISO code or full name e.g. "UK"
        trim          : trim level (default: "All")
        backends      : list of backends to test, default:
                          ["youtube-transcript-api", "faster-whisper",
                           "openai-whisper", "openai-api"]
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
                brand=brand,
                car_model=car_model,
                year=year,
                market=market,
                trim=trim,
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

    # ── Test details ─────────────────────────────────────────────────────────────
    VIDEO_URL   = "https://www.youtube.com/watch?v=QFzEVtY_1lQ"
    VIDEO_TITLE = "Opel Corsa 2023 review"
    QUESTION    = "engine performance and fuel economy"

    # ── Choose what to run — uncomment ONE block at a time ───────────────────────

    # Option A: youtube-transcript-api (fast, ~10s, captions only)
    # run_video_pipeline(
    #     cfg=cfg, url=VIDEO_URL, title=VIDEO_TITLE, question=QUESTION,
    #     brand="Opel", car_model="Corsa", year="2023", market="UK", trim="All",
    #     whisper_backend="youtube-transcript-api", cleanup=False,
    # )

    # Option B: faster-whisper (local CPU, ~60s with tiny model)
    # run_video_pipeline(
    #     cfg=cfg, url=VIDEO_URL, title=VIDEO_TITLE, question=QUESTION,
    #     brand="Opel", car_model="Corsa", year="2023", market="UK", trim="All",
    #     whisper_backend="faster-whisper", whisper_model="tiny", cleanup=False,
    # )

    # Option C: openai-whisper (local CPU, ~2-3min with tiny model)
    # run_video_pipeline(
    #     cfg=cfg, url=VIDEO_URL, title=VIDEO_TITLE, question=QUESTION,
    #     brand="Opel", car_model="Corsa", year="2023", market="UK", trim="All",
    #     whisper_backend="openai-whisper", whisper_model="tiny", cleanup=False,
    # )

    # Option D: openai-api (cloud, ~30s, costs ~$0.02/video)
    # run_video_pipeline(
    #     cfg=cfg, url=VIDEO_URL, title=VIDEO_TITLE, question=QUESTION,
    #     brand="Opel", car_model="Corsa", year="2023", market="UK", trim="All",
    #     whisper_backend="openai-api", cleanup=False,
    # )

    # Option E: compare multiple backends side by side
    # compare_backends(
    #     cfg=cfg, url=VIDEO_URL, title=VIDEO_TITLE, question=QUESTION,
    #     brand="Opel", car_model="Corsa", year="2023", market="UK", trim="All",
    #     backends=["youtube-transcript-api", "faster-whisper"],
    #     whisper_model="tiny", top_n=3, cleanup=False,
    # )

    # Option F: Opel Corsa 2025 IE PDF
    # run_pdf_pipeline(
    #     cfg=cfg, file_path="/ragflow/tests/Corsa_test.pdf",
    #     question="engine performance and fuel economy",
    #     brand="Opel", car_model="Corsa", year="2025", market="IE", trim="All",
    #     top_n=3, cleanup=False,
    # )

    # Option G: Peugeot 208 2023 FR PDF
    # run_pdf_pipeline(
    #     cfg=cfg, file_path="/ragflow/tests/208_test.pdf",
    #     question="engine performance and fuel economy",
    #     brand="Peugeot", car_model="208", year="2023", market="FR", trim="All",
    #     top_n=3, cleanup=False,
    # )

    # Option H: brand-scoped retrieval (query existing datasets — no ingestion)
    # get_datasets_by_brand_model(cfg, "Opel", "Corsa")
    # chunks = retrieve_by_brand_model(cfg, "Opel", "Corsa", QUESTION)
    # display_results(chunks)

    # Option H with filters:
    # chunks = retrieve_by_brand_model(cfg, "Opel", "Corsa", QUESTION,
    #                                   year="2025", market="IE", source_type="Docs")
    # display_results(chunks)

    # Option I: HTML / web page ingestion (local file)
    # run_web_pipeline(
    #     cfg=cfg, source="/ragflow/tests/corsa_ie.html",
    #     question="engine performance and fuel economy",
    #     brand="Opel", car_model="Corsa", year="2025", market="IE", trim="All",
    #     top_n=3, cleanup=False,
    # )

    # Option I (URL variant): fetch live page then ingest
    # run_web_pipeline(
    #     cfg=cfg, source="https://www.opel.ie/cars/corsa.html",
    #     question="engine performance and fuel economy",
    #     brand="Opel", car_model="Corsa", year="2025", market="IE", trim="All",
    #     top_n=3, cleanup=False,
    # )

    # Option J: image ingestion (local file — DeepDoc picture parser)
    # run_image_pipeline(
    #     cfg=cfg, source="/ragflow/tests/corsa_badge.jpg",
    #     question="exterior design and colour options",
    #     brand="Opel", car_model="Corsa", year="2025", market="IE", trim="All",
    #     top_n=3, cleanup=False,
    # )

    # Option J (URL variant): fetch image from URL then ingest
    # run_image_pipeline(
    #     cfg=cfg, source="https://example.com/corsa_spec.jpg",
    #     question="exterior design and colour options",
    #     brand="Opel", car_model="Corsa", year="2025", market="IE", trim="All",
    #     top_n=3, cleanup=False,
    # )

    # ── Default: run Option A (fastest smoke test) ────────────────────────────
    run_video_pipeline(
        cfg=cfg, url=VIDEO_URL, title=VIDEO_TITLE, question=QUESTION,
        brand="Opel", car_model="Corsa", year="2023", market="UK", trim="All",
        whisper_backend="youtube-transcript-api", cleanup=False,
    )
