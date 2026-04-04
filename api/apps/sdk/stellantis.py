#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

"""
stellantis.py
=============
Stellantis-specific REST API endpoints, mounted automatically by the
sdk/ blueprint loader at /api/v1/stellantis/...

Endpoints
---------
POST /api/v1/stellantis/datasets
    Create one analysis dataset per run (Brand_Model_Year_Market_Trim_YYYYMMDD_HHMM).

POST /api/v1/stellantis/ingest/video
    Register a YouTube video URL as a document with full business metadata.

POST /api/v1/stellantis/ingest/document
    Upload a PDF, HTML or image file as a document with full business metadata.

GET  /api/v1/stellantis/retrieve
    Retrieve chunks by brand/model with optional metadata filters.

All endpoints:
  - require Bearer token auth via @token_required
  - store business metadata (brand, car_model, year, market, trim, source_type)
    via DocMetadataService — never in parser_config
  - follow existing RagFlow response conventions (get_result / get_error_data_result)
  - are async (Quart framework)
"""

import logging
import re
from datetime import date, datetime
from quart import request

from api.db import FileType
from api.db.services.document_service import DocumentService
from api.db.services.doc_metadata_service import DocMetadataService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from api.db.services.tenant_llm_service import TenantLLMService
from api.db.services.user_service import TenantService
from api.db.services.task_service import TaskService, queue_tasks
from api.utils.api_utils import (
    get_error_argument_result,
    get_error_data_result,
    get_error_permission_result,
    get_result,
    remap_dictionary_keys,
    token_required,
    verify_embedding_availability,
)
from common.constants import RetCode, FileSource, LLMType
from common import settings
from rag.nlp import search
from rag.app.tag import label_question

# ── Constants ──────────────────────────────────────────────────────────────────

DEFAULT_EMBEDDING_MODEL = "BAAI/bge-small-en-v1.5@Builtin"
DEFAULT_CHUNK_METHOD    = "naive"

MARKET_ISO_CODES = {
    "United Kingdom": "UK", "Ireland": "IE", "France": "FR",
    "Germany": "DE", "Italy": "IT", "Spain": "ES", "Belgium": "BE",
    "Netherlands": "NL", "Portugal": "PT", "Poland": "PL",
    "Austria": "AT", "Switzerland": "CH", "Sweden": "SE",
    "Norway": "NO", "Denmark": "DK", "Finland": "FI",
}

SUPPORTED_DOCUMENT_MIME = {
    ".pdf":  "application/pdf",
    ".html": "text/html",
    ".htm":  "text/html",
    ".jpg":  "image/jpeg",
    ".jpeg": "image/jpeg",
    ".png":  "image/png",
    ".webp": "image/webp",
    ".bmp":  "image/bmp",
    ".tiff": "image/tiff",
    ".tif":  "image/tiff",
}


# ── Helpers ────────────────────────────────────────────────────────────────────

def _normalize_market(market: str) -> str:
    """Convert full country name to ISO-2 code if needed."""
    if market in MARKET_ISO_CODES.values():
        return market
    return MARKET_ISO_CODES.get(market, market.upper()[:2])


def _build_dataset_name(brand: str, car_model: str, year: str,
                         market: str, trim: str) -> str:
    """
    Build standardized dataset name:
    {Brand}_{Model}_{Year}_{Market}_{Trim}_{YYYYMMDD}_{HHMM}
    Example: Opel_Corsa_2025_IE_All_20260404_1430
    """
    now = datetime.now()
    return (
        f"{brand}_{car_model}_{year}_{market}_{trim}"
        f"_{now.strftime('%Y%m%d')}_{now.strftime('%H%M')}"
    )


def _missing(*fields) -> str | None:
    """Return an error message if any required field is missing, else None."""
    missing = [f for f in fields if not f]
    return f"Missing required fields: {', '.join(missing)}" if missing else None


# ── POST /api/v1/stellantis/datasets ──────────────────────────────────────────

@manager.route("/stellantis/datasets", methods=["POST"])  # noqa: F821
@token_required
async def stellantis_create_dataset(tenant_id):
    """
    Create a single RagFlow dataset for one Stellantis analysis run.
    All source types (Video, PDF, Web, Images) share this dataset.

    Request body (JSON):
      brand           : string, required  — e.g. "Opel"
      car_model       : string, required  — e.g. "Corsa"
      year            : string, required  — e.g. "2025"
      market          : string, required  — ISO code or full name e.g. "IE"
      trim            : string, optional  — default "All"
      whisper_backend : string, optional  — default "youtube-transcript-api"
      whisper_model   : string, optional  — default "base"
      embedding_model : string, optional  — default BAAI/bge-small-en-v1.5@Builtin

    Response:
      Standard RagFlow dataset object with id, name, chunk_method, ...
    """
    req = await request.get_json()
    if not req:
        return get_error_argument_result("Request body must be JSON")

    # ── Required fields ───────────────────────────────────────────────────────
    brand     = (req.get("brand") or "").strip()
    car_model = (req.get("car_model") or "").strip()
    year      = (req.get("year") or "").strip()
    market    = (req.get("market") or "").strip()

    err = _missing(brand, car_model, year, market)
    if err:
        return get_error_argument_result(err)

    # ── Optional fields ───────────────────────────────────────────────────────
    trim            = (req.get("trim") or "All").strip()
    whisper_backend = (req.get("whisper_backend") or "youtube-transcript-api").strip()
    whisper_model   = (req.get("whisper_model") or "base").strip()
    embedding_model = (req.get("embedding_model") or DEFAULT_EMBEDDING_MODEL).strip()

    market_iso = _normalize_market(market)
    name = _build_dataset_name(brand, car_model, year, market_iso, trim)

    # ── Resolve embedding model ───────────────────────────────────────────────
    ok, t = TenantService.get_by_id(tenant_id)
    if not ok:
        return get_error_permission_result(message="Tenant not found")

    embd_id = embedding_model or t.embd_id
    if embd_id != t.embd_id:
        ok, verify_err = verify_embedding_availability(embd_id, tenant_id)
        if not ok:
            return verify_err

    # ── parser_config: Whisper technical defaults only ────────────────────────
    # Business fields (brand, market, …) go to DocMetadataService per-document.
    parser_config = {
        "whisper_backend": whisper_backend,
        "whisper_model":   whisper_model,
    }

    # ── Create dataset ────────────────────────────────────────────────────────
    try:
        from common.misc_utils import get_uuid
        dataset_id = get_uuid()
        dataset = {
            "id":            dataset_id,
            "tenant_id":     tenant_id,
            "name":          name,
            "embd_id":       embd_id,
            "parser_id":     DEFAULT_CHUNK_METHOD,
            "parser_config": parser_config,
        }
        if not KnowledgebaseService.save(**dataset):
            return get_error_data_result(message="Failed to create dataset")

        ok, kb = KnowledgebaseService.get_by_id(dataset_id)
        if not ok:
            return get_error_data_result(message="Dataset created but could not be retrieved")

        response_data = remap_dictionary_keys(kb.to_dict())
        logging.info(
            f"stellantis_create_dataset: created {name} "
            f"(id={dataset_id}, tenant={tenant_id})"
        )
        return get_result(data=response_data)

    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message=f"Database error: {e}")


# ── POST /api/v1/stellantis/ingest/video ──────────────────────────────────────

@manager.route("/stellantis/ingest/video", methods=["POST"])  # noqa: F821
@token_required
async def stellantis_ingest_video(tenant_id):
    """
    Register a YouTube video URL as a document in a Stellantis dataset.
    Business metadata is stored via DocMetadataService (not in parser_config).

    Request body (JSON):
      dataset_id    : string, required  — target dataset ID
      url           : string, required  — YouTube URL
      title         : string, optional  — human-readable title
      brand         : string, required  — e.g. "Opel"
      car_model     : string, required  — e.g. "Corsa"
      year          : string, required  — e.g. "2025"
      market        : string, required  — ISO code or full name
      trim          : string, optional  — default "All"
      source_type   : string, optional  — default "Video"
      retrieval_date: string, optional  — ISO date, default today

    Response:
      { id, name, dataset_id, location, chunk_method, size, ... }
    """
    from common.misc_utils import get_uuid

    req = await request.get_json()
    if not req:
        return get_error_argument_result("Request body must be JSON")

    # ── Required fields ───────────────────────────────────────────────────────
    dataset_id    = (req.get("dataset_id") or "").strip()
    youtube_url   = (req.get("url") or "").strip()
    brand         = (req.get("brand") or "").strip()
    car_model     = (req.get("car_model") or "").strip()
    year          = (req.get("year") or "").strip()
    market        = (req.get("market") or "").strip()

    err = _missing(dataset_id, youtube_url, brand, car_model, year, market)
    if err:
        return get_error_argument_result(err)

    # ── Optional fields ───────────────────────────────────────────────────────
    title          = (req.get("title") or youtube_url).strip()
    trim           = (req.get("trim") or "All").strip()
    source_type    = (req.get("source_type") or "Video").strip()
    retrieval_date = (req.get("retrieval_date") or date.today().isoformat()).strip()
    market_iso     = _normalize_market(market)

    # ── Validate YouTube URL ──────────────────────────────────────────────────
    if not re.search(r"(?:youtube\.com/watch|youtu\.be/)", youtube_url):
        return get_error_argument_result(
            "url must be a YouTube URL (youtube.com/watch or youtu.be)"
        )

    # ── Authorise dataset access ──────────────────────────────────────────────
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return get_result(
            data=False, message="No authorization.",
            code=RetCode.AUTHENTICATION_ERROR
        )

    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return get_error_data_result(message="Invalid dataset ID")

    # ── Build document record ─────────────────────────────────────────────────
    doc_id = get_uuid()
    parser_config = dict(kb.parser_config or {})

    doc = {
        "id":            doc_id,
        "kb_id":         kb.id,
        "parser_id":     "video",
        "pipeline_id":   kb.pipeline_id,
        "parser_config": parser_config,
        "created_by":    tenant_id,
        "type":          FileType.DOC.value,
        "name":          title,
        "source_type":   "local",
        "suffix":        "url",
        "location":      youtube_url,
        "size":          0,
        "thumbnail":     "",
    }

    try:
        DocumentService.insert(doc)
    except Exception as e:
        return get_error_data_result(message=f"Failed to register video document: {e}")

    # ── Store business metadata via DocMetadataService ────────────────────────
    _metadata = {
        "youtube_url":    youtube_url,
        "video_title":    title,
        "brand":          brand,
        "car_model":      car_model,
        "year":           year,
        "market":         market_iso,
        "trim":           trim,
        "source_type":    source_type,
        "retrieval_date": retrieval_date,
    }
    try:
        DocMetadataService.update_document_metadata(doc_id, _metadata)
    except Exception as e:
        logging.warning(
            f"stellantis_ingest_video: metadata update failed "
            f"for doc {doc_id}: {e}"
        )

    logging.info(
        f"stellantis_ingest_video: registered {youtube_url} "
        f"as doc {doc_id} in dataset {dataset_id}"
    )
    return get_result(data=[{
        "id":           doc_id,
        "name":         title,
        "dataset_id":   dataset_id,
        "location":     youtube_url,
        "chunk_method": "video",
        "size":         0,
        "suffix":       "url",
        "type":         FileType.DOC.value,
        "source_type":  "local",
        "run":          "UNSTART",
        "thumbnail":    "",
        "pipeline_id":  kb.pipeline_id,
        "parser_config": parser_config,
        "created_by":   tenant_id,
    }])


# ── POST /api/v1/stellantis/ingest/document ───────────────────────────────────

@manager.route("/stellantis/ingest/document", methods=["POST"])  # noqa: F821
@token_required
async def stellantis_ingest_document(tenant_id):
    """
    Upload a PDF, HTML or image file as a document in a Stellantis dataset.
    Accepts multipart/form-data. Business metadata stored via DocMetadataService.

    Form fields:
      file          : file, required    — the document file
      dataset_id    : string, required  — target dataset ID
      brand         : string, required  — e.g. "Opel"
      car_model     : string, required  — e.g. "Corsa"
      year          : string, required  — e.g. "2025"
      market        : string, required  — ISO code or full name
      trim          : string, optional  — default "All"
      source_type   : string, optional  — "Docs", "Web", or "Images"
                                          auto-detected from file extension if omitted
      retrieval_date: string, optional  — ISO date, default today

    Response:
      { id, name, dataset_id, size, ... }
    """
    import mimetypes
    import pathlib
    import xxhash
    from io import BytesIO
    from common.misc_utils import get_uuid
    from api.db.services.file_service import FileService
    from api.db.services.file2document_service import File2DocumentService

    files = await request.files
    form  = await request.form

    # ── Validate file present ─────────────────────────────────────────────────
    if "file" not in files:
        return get_error_argument_result("Missing required file field: 'file'")

    uploaded_file = files["file"]
    filename      = uploaded_file.filename or "upload"
    file_content  = uploaded_file.read()
    file_size     = len(file_content)

    # ── Required form fields ──────────────────────────────────────────────────
    dataset_id = (form.get("dataset_id") or "").strip()
    brand      = (form.get("brand") or "").strip()
    car_model  = (form.get("car_model") or "").strip()
    year       = (form.get("year") or "").strip()
    market     = (form.get("market") or "").strip()

    err = _missing(dataset_id, brand, car_model, year, market)
    if err:
        return get_error_argument_result(err)

    # ── Optional form fields ──────────────────────────────────────────────────
    trim           = (form.get("trim") or "All").strip()
    retrieval_date = (form.get("retrieval_date") or date.today().isoformat()).strip()
    market_iso     = _normalize_market(market)

    # ── Auto-detect source_type from extension if not provided ────────────────
    ext = pathlib.Path(filename).suffix.lower()
    if form.get("source_type"):
        source_type = form.get("source_type").strip()
    elif ext in (".jpg", ".jpeg", ".png", ".webp", ".bmp", ".tiff", ".tif"):
        source_type = "Images"
    elif ext in (".html", ".htm"):
        source_type = "Web"
    else:
        source_type = "Docs"

    # ── Detect MIME type ──────────────────────────────────────────────────────
    mime = SUPPORTED_DOCUMENT_MIME.get(
        ext,
        mimetypes.guess_type(filename)[0] or "application/octet-stream"
    )

    # ── Authorise dataset access ──────────────────────────────────────────────
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return get_result(
            data=False, message="No authorization.",
            code=RetCode.AUTHENTICATION_ERROR
        )

    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return get_error_data_result(message="Invalid dataset ID")

    # ── Store file in MinIO via FileService ───────────────────────────────────
    # Follows same pattern as doc.py upload() handler
    doc_id    = get_uuid()
    file_hash = xxhash.xxh64(file_content).hexdigest()
    location  = f"{tenant_id}/{doc_id}/{filename}"

    try:
        settings.storage.put(location, BytesIO(file_content))
    except Exception as e:
        return get_error_data_result(message=f"File storage failed: {e}")

    # ── Determine parser_id from extension ────────────────────────────────────
    if ext in (".jpg", ".jpeg", ".png", ".webp", ".bmp", ".tiff", ".tif"):
        parser_id = "picture"
    else:
        parser_id = kb.parser_id or DEFAULT_CHUNK_METHOD

    doc = {
        "id":            doc_id,
        "kb_id":         kb.id,
        "parser_id":     parser_id,
        "pipeline_id":   kb.pipeline_id,
        "parser_config": dict(kb.parser_config or {}),
        "created_by":    tenant_id,
        "type":          FileType.DOC.value,
        "name":          filename,
        "source_type":   FileSource.KNOWLEDGEBASE,
        "suffix":        ext.lstrip("."),
        "location":      location,
        "size":          file_size,
        "thumbnail":     "",
    }

    try:
        DocumentService.insert(doc)
    except Exception as e:
        return get_error_data_result(message=f"Failed to register document: {e}")

    # ── Store business metadata via DocMetadataService ────────────────────────
    _metadata = {
        "brand":          brand,
        "car_model":      car_model,
        "year":           year,
        "market":         market_iso,
        "trim":           trim,
        "source_type":    source_type,
        "retrieval_date": retrieval_date,
    }
    try:
        DocMetadataService.update_document_metadata(doc_id, _metadata)
    except Exception as e:
        logging.warning(
            f"stellantis_ingest_document: metadata update failed "
            f"for doc {doc_id}: {e}"
        )

    logging.info(
        f"stellantis_ingest_document: uploaded {filename} "
        f"({file_size} bytes) as doc {doc_id} in dataset {dataset_id}"
    )
    return get_result(data=[{
        "id":           doc_id,
        "name":         filename,
        "dataset_id":   dataset_id,
        "size":         file_size,
        "suffix":       ext.lstrip("."),
        "location":     location,
        "type":         FileType.DOC.value,
        "source_type":  source_type,
        "chunk_method": parser_id,
        "run":          "UNSTART",
        "thumbnail":    "",
        "created_by":   tenant_id,
    }])


# ── GET /api/v1/stellantis/retrieve ───────────────────────────────────────────

@manager.route("/stellantis/retrieve", methods=["GET"])  # noqa: F821
@token_required
async def stellantis_retrieve(tenant_id):
    """
    Retrieve chunks across all datasets matching brand + model,
    with optional metadata filters (year, market, trim, source_type).

    Query parameters:
      brand                : string, required
      car_model            : string, required
      question             : string, required
      year                 : string, optional
      market               : string, optional  — ISO code or full name
      trim                 : string, optional
      source_type          : string, optional  — "Video", "Docs", "Web", "Images"
      top_n                : integer, optional — default 5
      similarity_threshold : float, optional   — default 0.1

    Response:
      { chunks: [...], total: int, doc_aggs: {...} }
    """
    from common.metadata_utils import meta_filter, convert_conditions

    args = request.args

    # ── Required params ───────────────────────────────────────────────────────
    brand     = (args.get("brand") or "").strip()
    car_model = (args.get("car_model") or "").strip()
    question  = (args.get("question") or "").strip()

    err = _missing(brand, car_model, question)
    if err:
        return get_error_argument_result(err)

    # ── Optional params ───────────────────────────────────────────────────────
    year         = (args.get("year") or "").strip() or None
    market       = (args.get("market") or "").strip() or None
    trim         = (args.get("trim") or "").strip() or None
    source_type  = (args.get("source_type") or "").strip() or None
    top_n        = int(args.get("top_n", 5))
    sim_threshold = float(args.get("similarity_threshold", 0.1))
    market_iso   = _normalize_market(market) if market else None

    # ── Discover matching datasets by name prefix ─────────────────────────────
    prefix = f"{brand}_{car_model}_"
    tenants = TenantService.get_joined_tenants_by_user_id(tenant_id)
    all_kbs, _ = KnowledgebaseService.get_list(
        [m["tenant_id"] for m in tenants],
        tenant_id,
        page_number=1,
        items_per_page=1000,
        orderby="create_time",
        desc=True,
        id=None,
        name=None,
    )

    matched_kbs = [kb for kb in all_kbs if kb.get("name", "").startswith(prefix)]

    if year:
        matched_kbs = [kb for kb in matched_kbs if f"_{year}_" in kb["name"]]
    if market_iso:
        matched_kbs = [kb for kb in matched_kbs if f"_{market_iso}_" in kb["name"]]
    if trim:
        matched_kbs = [kb for kb in matched_kbs if f"_{trim}_" in kb["name"]]

    if not matched_kbs:
        return get_result(data={"total": 0, "chunks": [], "doc_aggs": {}})

    kb_ids = [kb["id"] for kb in matched_kbs]

    # ── Verify all datasets belong to this tenant ─────────────────────────────
    for kb_id in kb_ids:
        if not KnowledgebaseService.accessible(kb_id=kb_id, user_id=tenant_id):
            return get_error_permission_result(
                message=f"No authorization for dataset {kb_id}"
            )

    # ── Build metadata filter for source_type (and other fields) ─────────────
    conditions = [
        {"name": "brand",     "value": brand,     "comparison_operator": "="},
        {"name": "car_model", "value": car_model, "comparison_operator": "="},
    ]
    if year:
        conditions.append({"name": "year",   "value": year,       "comparison_operator": "="})
    if market_iso:
        conditions.append({"name": "market", "value": market_iso, "comparison_operator": "="})
    if trim:
        conditions.append({"name": "trim",   "value": trim,       "comparison_operator": "="})
    if source_type:
        conditions.append({"name": "source_type", "value": source_type, "comparison_operator": "="})

    metadata_condition = {"conditions": conditions, "logic": "and"}

    # ── Resolve doc_ids via metadata filter ───────────────────────────────────
    metas   = DocMetadataService.get_meta_by_kbs(kb_ids)
    doc_ids = meta_filter(metas, convert_conditions(metadata_condition), "and")

    # video docs (size=0) are excluded from the documents API —
    # if source_type is Video or unfiltered, allow doc_ids=None so video
    # chunks are included in results
    if source_type and source_type != "Video" and not doc_ids:
        return get_result(data={"total": 0, "chunks": [], "doc_aggs": {}})

    if source_type == "Video":
        # let retrieval scan everything; video chunks will be present
        doc_ids = None
    elif not doc_ids:
        doc_ids = None

    # ── Load embedding model from first matched dataset ───────────────────────
    try:
        ok, kb = KnowledgebaseService.get_by_id(kb_ids[0])
        if not ok:
            return get_error_data_result(message="Dataset not found")

        kbs_objs = KnowledgebaseService.get_by_ids(kb_ids)
        tenant_ids = list(set(kb_obj.tenant_id for kb_obj in kbs_objs))
        embd_mdl = LLMBundle(kb.tenant_id, LLMType.EMBEDDING, llm_name=kb.embd_id)

        ranks = await settings.retriever.retrieval(
            question,
            embd_mdl,
            tenant_ids,
            kb_ids,
            1,               # page
            top_n,           # page_size / size
            sim_threshold,
            0.3,             # vector_similarity_weight
            1024,            # top_k candidate pool
            doc_ids,
            rerank_mdl=None,
            highlight=False,
            rank_feature=label_question(question, kbs_objs),
        )

        # Strip raw vectors from response
        for c in ranks["chunks"]:
            c.pop("vector", None)

        # Rename keys to match existing /retrieval response shape
        key_mapping = {
            "chunk_id":            "id",
            "content_with_weight": "content",
            "doc_id":              "document_id",
            "important_kwd":       "important_keywords",
            "question_kwd":        "questions",
            "docnm_kwd":           "document_keyword",
            "kb_id":               "dataset_id",
        }
        ranks["chunks"] = [
            {key_mapping.get(k, k): v for k, v in chunk.items()}
            for chunk in ranks["chunks"]
        ]

        logging.info(
            f"stellantis_retrieve: '{question}' → "
            f"{len(ranks['chunks'])} chunks from {len(kb_ids)} dataset(s) "
            f"[brand={brand} model={car_model} source={source_type}]"
        )
        return get_result(data=ranks)

    except Exception as e:
        if "not_found" in str(e):
            return get_result(
                message="No chunks found — check parsing status.",
                code=RetCode.DATA_ERROR,
            )
        logging.exception(e)
        return get_error_data_result(message=f"Retrieval error: {e}")
