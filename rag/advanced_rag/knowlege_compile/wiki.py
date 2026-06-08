#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
"""Wiki compilation pipeline — MAP phase.

  - Chunks come from ES (or any pre-chunked list passed in by the caller).
    No outline-driven chunking; per-chunk byte offsets are not tracked.
  - The LLM goes through ``rag.prompts.generator.gen_json`` (json_repair-backed).
    Embeddings go through ``LLMBundle.encode`` via ``thread_pool_exec`` (kept
    in the signature for symmetry with the downstream REDUCE / REFINE phases
    even though MAP itself does not embed).
  - Citation anchor is the source chunk id (``source_id`` list per item), not
    a byte position. The LLM is prompted to tag each extracted item with the
    ``[CHUNK_ID …]`` of the chunk it came from.
  - Resume: per-chunk extracts are persisted to ES under
    ``compile_kwd="wiki_map_extract"`` with ``available_int=0`` and no vector
    / token-list fields, so retrievers ignore them but downstream phases can
    fetch them by ``doc_id`` + ``source_id``. Re-running MAP for the same
    ``doc_id`` skips chunks that already have an extract row.

Public entry: ``wiki_map_from_chunks``.
"""
import asyncio
import json
import logging
import re
import string
from typing import Callable, Optional

import xxhash

from common.misc_utils import thread_pool_exec
from common.token_utils import num_tokens_from_string
from rag.nlp import rag_tokenizer
from rag.prompts.generator import INPUT_UTILIZATION, gen_json, message_fit_in, split_chunks


# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

WIKI_MAP_COMPILE_KWD = "wiki_map_extract"
DEFAULT_WIKI_MAP_WORKERS = 6
DEFAULT_WIKI_MAP_TIMEOUT = 600


WIKI_MAP_SYSTEM = (
    "You are a knowledge extraction engine. Extract structured knowledge from the "
    "provided document section. Return ONLY valid JSON matching the schema exactly. "
    "Never include any text outside the JSON object. If a category has no items, use []."
)


WIKI_MAP_USER_TEMPLATE = """\
## Document context
Document id: {doc_id}
Batch contains {chunk_count} packed chunk(s). Each chunk is introduced by a
``[CHUNK_ID <id>]`` line. The chunk_id values to choose from are:
{chunk_id_list}

## Packed chunks
{packed_chunks}

---

Extract all knowledge from every chunk and return a single JSON object with this
exact schema:

{{
  "entities": [
    {{
      "name": "string — entity canonical name as it appears in text",
      "type": "string — one of: person|org|product|regulation|location|system|equipment|other",
      "aliases": ["string"],
      "source_chunk_id": "string — exact value from the chunk_id list above"
    }}
  ],
  "concepts": [
    {{
      "term": "string — concept name OR a thematic section topic (prefer the source's heading wording when coherent)",
      "definition_excerpt": "string — verbatim or near-verbatim defining phrase from the chunk",
      "source_chunk_id": "string — exact value from the chunk_id list above"
    }}
  ],
  "claims": [
    {{
      "statement": "string — a complete factual sentence stated in the source. Any sentence of the form 'X is Y', 'X has Y', 'X does Y', 'X was founded in Y', 'X is located in Y', 'X reported Y', etc. is a claim. Aim for at least 1-3 claims per entity per chunk that mentions it.",
      "subject": "string — entity/concept this claim is about (must match one of the entity/concept names extracted above)",
      "confidence": "explicit",
      "source_chunk_id": "string — exact value from the chunk_id list above"
    }}
  ],
  "relations": [
    {{
      "from": "string — source entity/concept name",
      "to": "string — target entity/concept name",
      "type": "string — e.g. owns|part_of|caused_by|regulates|uses|located_in|other",
      "source_chunk_id": "string — exact value from the chunk_id list above"
    }}
  ],
  "topics": ["string"]
}}

Rules:
- ``source_chunk_id`` MUST be one of the chunk_id values listed above (they
  look like ``C1``, ``C2``, …); do NOT invent new ids. Pick the chunk where
  the item is primarily stated.
- The ``[CHUNK_ID …]`` header lines AND the ``C1``/``C2``/… chunk tags are
  prompt scaffolding — they are NOT part of the document content. Do NOT
  extract them (or any other identifier-looking strings from the headers)
  as entities, concepts, claims, or relations. Entity ``name`` / concept
  ``term`` values must come from the human-readable chunk body only.
- NEVER use bare hexadecimal hashes (such as ``a3f1b2c4d5e6f7a8``),
  UUIDs, database row ids, or any other opaque identifier-looking token
  as an entity ``name`` or concept ``term``. If you cannot find a
  human-readable name for a candidate entity in the chunk body, drop it.
- Concrete examples of values that are ALWAYS WRONG:
    BAD entity: {{"name": "C1", "type": "product", "aliases": ["C1"]}}
    BAD entity: {{"name": "C3", "type": "location"}}
    BAD concept: {{"term": "C2"}}
    BAD entity: {{"name": "d523a888c5b2a167", "type": "location"}}
    BAD entity: {{"name": "41a5271858ca11f1bbb9047c16ec874f", "type": "product"}}
  ``C1`` / ``C2`` / etc. are CHUNK TAGS, not products or locations. The
  hex hashes are DATABASE IDS, not entities. If your candidate ``name``
  matches any of these shapes, do not include the item in the output.
- ``confidence`` is ``"explicit"`` (directly stated) or ``"inferred"`` (implied
  by the text).
- Be exhaustive — include all named entities, defined terms, and factual claims.
- For ``concepts``, extract BOTH (a) named terms with definitions AND (b)
  coherent thematic sub-topics that could become their own wiki page.
- Extract ``claims`` LIBERALLY: every factual sentence about an entity is a
  claim. Definitions, attributes, ownership, locations, dates, actions,
  events, financial figures, regulations cited — all qualify. If you
  extract an entity, you should usually extract one or more claims that
  mention it. An empty ``claims`` array is almost always wrong unless the
  chunks are pure boilerplate.
- ``relations`` only fire when the text states an explicit link between two
  named entities/concepts (``A owns B``, ``A is part of B``, ``A regulates B``).
  Otherwise leave ``relations`` empty.
- Return empty arrays ``[]`` for categories with no findings.
- Return ONLY the JSON object, no markdown fences, no commentary.
"""


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

_EXTRACT_LIST_KEYS = ("entities", "concepts", "claims", "relations")


def _wiki_empty_extract() -> dict:
    return {
        "entities": [],
        "concepts": [],
        "claims": [],
        "relations": [],
        "topics": [],
    }


def _wiki_pick_chunk_text(chunk: dict) -> str:
    text = chunk.get("text") or chunk.get("content_with_weight") or chunk.get("content") or ""
    return text if isinstance(text, str) else ""


# Matches a bare 16-char lowercase-hex token bounded by non-word chars on
# both sides — the shape of an xxh64 hexdigest (chunk_id / row id).
_HEX16_TOKEN_RE = re.compile(r"(?<![0-9a-zA-Z])[0-9a-f]{16}(?![0-9a-zA-Z])")
# Similar for the 32-char doc-id / uuid-without-dashes pattern.
_HEX32_TOKEN_RE = re.compile(r"(?<![0-9a-zA-Z])[0-9a-f]{32}(?![0-9a-zA-Z])")


def _wiki_scrub_known_ids(text: str, ids_to_remove) -> str:
    """Defensive scrub: strip any literal occurrence of known ES ids from
    chunk text before sending to the extraction LLM.

    Some chunkers embed the chunk_id / doc_id into the body (e.g. as a
    header, footer, or breadcrumb). Without this scrub the extraction LLM
    grabs the hash and reports it as an entity name (commonly mis-typed as
    "location"). We belt-and-brace by removing:

      1. Every literal id passed in ``ids_to_remove`` (chunk_ids of the
         batch + the doc_id).
      2. Any standalone 16-hex or 32-hex token still left over after (1).
    """
    if not text:
        return text
    out = text
    for h in ids_to_remove or ():
        if h and isinstance(h, str) and h in out:
            out = out.replace(h, "")
    out = _HEX16_TOKEN_RE.sub("", out)
    out = _HEX32_TOKEN_RE.sub("", out)
    return out


def _wiki_safe_chunk_label(position_in_batch: int) -> str:
    """Per-batch positional chunk tag (``C1``, ``C2``, …).

    We deliberately do NOT use the real ES chunk id as the label here.
    xxhash-based ids look like ``cff1a46bcba3f0e1`` which the extraction
    LLM tends to mistakenly extract as entity names (commonly mis-typed as
    "location"). Short synthetic tags remove that failure mode entirely;
    we map them back to real chunk ids via ``label_to_id`` after extraction.
    """
    return f"C{position_in_batch + 1}"


def _wiki_format_batch_prompt(packed: list[dict]) -> tuple[str, list[str]]:
    """Render the [CHUNK_ID …]-labelled body and return (body_text, label_order)."""
    parts: list[str] = []
    labels: list[str] = []
    for entry in packed:
        labels.append(entry["label"])
        parts.append(f"[CHUNK_ID {entry['label']}]\n{entry['text']}")
    return "\n\n".join(parts), labels


def _wiki_unwrap_extract(res) -> dict:
    """Coerce LLM JSON to the canonical 5-key shape with defaulted lists."""
    out = _wiki_empty_extract()
    if not isinstance(res, dict):
        return out
    for k in _EXTRACT_LIST_KEYS:
        v = res.get(k)
        if isinstance(v, list):
            out[k] = [item for item in v if isinstance(item, dict)]
    topics = res.get("topics")
    if isinstance(topics, list):
        out["topics"] = [t for t in topics if isinstance(t, str) and t.strip()]
    return out


# Matches strings the LLM should NEVER use as an entity / concept / claim name:
#   - chunk tag scaffolding: C1, C2, c0001, …
#   - bare hexadecimal hashes (xxh64 16-char, doc-id 32-char)
#   - UUIDs with or without dashes
# Anything matching is dropped post-extraction as defensive filtering.
_WIKI_IDENTIFIER_LIKE_RE = re.compile(
    r"""^\s*(
        [Cc]\d{1,5}                       # chunk tag like C1, c0001
        | [0-9a-fA-F]{16}                 # xxh64 hexdigest
        | [0-9a-fA-F]{32}                 # md5 / doc_id-shaped
        | [0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}  # UUID
    )\s*$""",
    re.VERBOSE,
)


def _wiki_looks_like_identifier(s) -> bool:
    """True when ``s`` looks like a chunk tag, hash, or UUID rather than a name."""
    if not isinstance(s, str):
        return False
    return bool(_WIKI_IDENTIFIER_LIKE_RE.match(s))


def _wiki_item_has_identifier_name(key: str, item: dict) -> bool:
    """Return True when an extracted item's display name is identifier-shaped.

    Different list keys carry the name field under different keys:
      - entities  → ``name``
      - concepts  → ``term``
      - claims    → ``subject``
      - relations → ``from`` and ``to`` (drop if either is identifier-shaped)
    """
    if key == "entities":
        return _wiki_looks_like_identifier(item.get("name", ""))
    if key == "concepts":
        return _wiki_looks_like_identifier(item.get("term", ""))
    if key == "claims":
        return _wiki_looks_like_identifier(item.get("subject", ""))
    if key == "relations":
        return (
            _wiki_looks_like_identifier(item.get("from", ""))
            or _wiki_looks_like_identifier(item.get("to", ""))
        )
    return False


def _wiki_resolve_chunk_ids(
    extract: dict,
    label_to_id: dict[str, str],
) -> tuple[dict, dict[str, dict]]:
    """Split a batch extract by source chunk id.

    Returns:
        merged: the input extract with ``source_chunk_id`` rewritten to
                ``chunk_ids=[real_id]`` per item, dropping items whose label
                does not match any in ``label_to_id``.
        per_chunk: {real_chunk_id: extract-shaped dict containing only the
                   items attributed to that chunk}. Includes empty extracts
                   for every label in ``label_to_id`` so resume knows the
                   chunk was processed even when nothing was extracted.
    """
    per_chunk: dict[str, dict] = {
        real_id: _wiki_empty_extract() for real_id in label_to_id.values()
    }
    merged = _wiki_empty_extract()
    merged["topics"] = list(extract.get("topics") or [])

    dropped = 0
    dropped_identifier = 0
    for key in _EXTRACT_LIST_KEYS:
        for item in extract.get(key) or []:
            label = item.get("source_chunk_id")
            real = label_to_id.get(label) if isinstance(label, str) else None
            if real is None:
                dropped += 1
                continue
            # Drop items whose display name is identifier-shaped — the LLM
            # occasionally grabs prompt scaffolding (C1, C2, …) or leftover
            # hash tokens and reports them as entities/concepts/claims. The
            # prompt forbids this but post-filtering is the bulletproof guard.
            if _wiki_item_has_identifier_name(key, item):
                dropped_identifier += 1
                continue
            new_item = {k: v for k, v in item.items() if k != "source_chunk_id"}
            new_item["chunk_ids"] = [real]
            merged[key].append(new_item)
            per_chunk[real][key].append(new_item)

    if dropped:
        logging.debug(f"wiki_map: dropped {dropped} item(s) with unrecognized source_chunk_id")
    if dropped_identifier:
        logging.info(
            "wiki_map: dropped %d item(s) whose name looked like a prompt-scaffolding tag or hash",
            dropped_identifier,
        )

    return merged, per_chunk


def _wiki_merge_extracts(extracts: list[dict]) -> dict:
    """Concat the 5 lists across multiple batch extracts (no entity-level
    dedup — that is the REDUCE phase's job)."""
    out = _wiki_empty_extract()
    seen_topics: set[str] = set()
    for ex in extracts:
        if not isinstance(ex, dict):
            continue
        for key in _EXTRACT_LIST_KEYS:
            out[key].extend(ex.get(key) or [])
        for t in ex.get("topics") or []:
            if t not in seen_topics:
                seen_topics.add(t)
                out["topics"].append(t)
    return out


def _wiki_build_resume_doc(
    chunk_id: str,
    doc_id: str,
    per_chunk_extract: dict,
) -> dict:
    """Build the non-searchable ES doc that records a per-chunk MAP extract.

    Intentionally omits ``q_<dim>_vec`` / ``content_ltks`` / ``content_sm_ltks``
    so retrievers cannot surface this row; also sets ``available_int=0`` which
    most ragflow retrievers already filter on.
    """
    content_with_weight = json.dumps(per_chunk_extract, ensure_ascii=False)
    doc_id_str = str(doc_id)
    return {
        "id": xxhash.xxh64(
            (content_with_weight + doc_id_str + str(chunk_id)).encode("utf-8", "surrogatepass")
        ).hexdigest(),
        "doc_id": doc_id_str,
        "compile_kwd": WIKI_MAP_COMPILE_KWD,
        "source_id": [chunk_id],
        "content_with_weight": content_with_weight,
        "available_int": 0,
    }


async def _wiki_load_resume_set(doc_id: str, tenant_id: str, kb_id: str) -> set[str]:
    """Query ES for chunk_ids that already have a wiki_map_extract row for
    this doc. Returns the set of source chunk_ids to skip."""
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr
    from rag.nlp import search as _rag_search

    index = _rag_search.index_name(tenant_id)
    condition = {
        "compile_kwd": [WIKI_MAP_COMPILE_KWD],
        "doc_id": [str(doc_id)],
    }
    select_fields = ["id", "source_id"]
    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            select_fields, [], condition, [], OrderByExpr(),
            0, 10000, index, [kb_id],
        )
        field_map = settings.docStoreConn.get_fields(res, select_fields)
    except Exception:
        logging.exception("wiki_map: failed to query resume set; will re-extract all chunks")
        return set()

    seen: set[str] = set()
    for row in field_map.values():
        src = row.get("source_id") or []
        if isinstance(src, list):
            for cid in src:
                if isinstance(cid, str) and cid:
                    seen.add(cid)
    return seen


async def _wiki_persist_extracts(
    per_chunk: dict[str, dict],
    doc_id: str,
    tenant_id: str,
    kb_id: str,
) -> None:
    """Write one non-searchable ES doc per source chunk_id."""
    if not per_chunk:
        return
    from common import settings
    from rag.nlp import search as _rag_search

    index = _rag_search.index_name(tenant_id)
    docs = [
        _wiki_build_resume_doc(chunk_id, doc_id, extract)
        for chunk_id, extract in per_chunk.items()
        if chunk_id
    ]
    if not docs:
        return
    try:
        await thread_pool_exec(settings.docStoreConn.insert, docs, index, kb_id)
    except Exception:
        logging.exception("wiki_map: failed to persist %d resume docs", len(docs))


# ---------------------------------------------------------------------------
# Per-batch extraction
# ---------------------------------------------------------------------------


async def _wiki_extract_one_batch(
    packed: list[dict],
    doc_id: str,
    chat_mdl,
    language: str,
    timeout: int,
) -> dict:
    """Single LLM call for one packed batch. Returns the raw (label-tagged)
    extract dict."""
    body, labels = _wiki_format_batch_prompt(packed)
    user_prompt = WIKI_MAP_USER_TEMPLATE.format(
        doc_id=doc_id,
        chunk_count=len(packed),
        chunk_id_list="\n".join(f"- {label}" for label in labels),
        packed_chunks=body,
    )
    try:
        res = await asyncio.wait_for(
            gen_json(WIKI_MAP_SYSTEM, user_prompt, chat_mdl, gen_conf={"temperature": 0.1}),
            timeout=timeout,
        )
    except asyncio.TimeoutError:
        logging.warning("wiki_map: batch extraction timed out after %ds (%d chunks)", timeout, len(packed))
        return _wiki_empty_extract()
    except Exception:
        logging.exception("wiki_map: batch extraction failed (%d chunks)", len(packed))
        return _wiki_empty_extract()
    _ = language  # reserved for future localization
    return _wiki_unwrap_extract(res)


async def _wiki_process_batch(
    packed: list[dict],
    batch_idx: int,
    total_batches: int,
    doc_id: str,
    tenant_id: str,
    kb_id: str,
    chat_mdl,
    language: str,
    timeout: int,
    semaphore: Optional[asyncio.Semaphore],
    callback: Optional[Callable],
) -> dict:
    """Run one batch end-to-end: LLM extract → split by source_chunk_id →
    persist resume docs → return the merged batch extract."""
    if not packed:
        return _wiki_empty_extract()

    label_to_id = {entry["label"]: entry["chunk_id"] for entry in packed}

    async def _run() -> dict:
        raw_extract = await _wiki_extract_one_batch(packed, doc_id, chat_mdl, language, timeout)
        merged, per_chunk = _wiki_resolve_chunk_ids(raw_extract, label_to_id)
        await _wiki_persist_extracts(per_chunk, doc_id, tenant_id, kb_id)
        if callback:
            try:
                n_items = sum(len(merged.get(k) or []) for k in _EXTRACT_LIST_KEYS)
                callback(
                    (batch_idx + 1) / max(1, total_batches),
                    f"wiki MAP {batch_idx + 1}/{total_batches}: {n_items} items from {len(packed)} chunks",
                )
            except Exception:
                logging.debug("wiki_map: progress callback failed", exc_info=True)
        return merged

    if semaphore is not None:
        async with semaphore:
            return await _run()
    return await _run()


# ---------------------------------------------------------------------------
# Public entry
# ---------------------------------------------------------------------------


async def wiki_map_from_chunks(
    chunks: list[dict],
    chat_mdl,
    embd_mdl,
    doc_id: str,
    tenant_id: str,
    kb_id: str,
    language: str = "en",
    max_workers: int = DEFAULT_WIKI_MAP_WORKERS,
    timeout: int = DEFAULT_WIKI_MAP_TIMEOUT,
    callback: Optional[Callable] = None,
) -> dict:
    """Phase 1 (MAP) of the wiki compilation pipeline.

    Packs the provided RAGFlow chunks into batches via ``split_chunks``, runs
    one ``gen_json`` extraction call per batch in parallel (bounded by
    ``max_workers``), then splits each batch's output back to per-chunk
    extracts and persists them to ES as non-searchable ``wiki_map_extract``
    rows so subsequent runs can skip chunks already processed.

    Args:
        chunks: list of dicts; each must expose ``id`` and ``text`` (with
            ``content_with_weight`` / ``content`` accepted as fallbacks).
        chat_mdl: LLMBundle for chat (used via ``gen_json``).
        embd_mdl: LLMBundle for embeddings — accepted for downstream symmetry
            with REDUCE/REFINE but **not used in MAP itself**.
        doc_id: source document id; stamped onto every resume doc and on every
            extracted item via ``chunk_ids``.
        tenant_id, kb_id: address the doc-store index for resume reads + writes.
        language: reserved for future prompt localization.
        max_workers: maximum concurrent batches. Defaults to 6.
        timeout: seconds per batch extraction call.
        callback: optional ``(progress: float, msg: str)`` progress callback.

    Returns:
        ``{"entities", "concepts", "claims", "relations", "topics"}`` where
        every item (except ``topics`` strings) carries a
        ``chunk_ids=[<source chunk id>]`` field. No entity-level dedup is
        performed here — that is the REDUCE phase's responsibility.
    """
    _ = embd_mdl  # noqa: F841 — accepted for symmetry with downstream phases

    if not chunks:
        return _wiki_empty_extract()

    # Skip chunks already covered by a prior MAP run for this doc_id.
    resume_set = await _wiki_load_resume_set(doc_id, tenant_id, kb_id)
    if resume_set:
        logging.info("wiki_map: resume — %d chunk(s) already extracted for doc %s", len(resume_set), doc_id)

    # Collect every chunk id up-front (including resume-skipped ones) so we
    # can scrub literal cross-references from the body of the chunks we do
    # process. Always include doc_id in the scrub list too.
    all_known_ids: list[str] = []
    for chunk in chunks:
        cid = chunk.get("id") or chunk.get("chunk_id")
        if isinstance(cid, str) and cid:
            all_known_ids.append(cid)
    if doc_id:
        all_known_ids.append(str(doc_id))

    chunk_ids: list[str] = []
    chunk_texts: list[str] = []
    for chunk in chunks:
        cid = chunk.get("id") or chunk.get("chunk_id")
        if not cid or cid in resume_set:
            continue
        text = _wiki_pick_chunk_text(chunk)
        if not text.strip():
            continue
        # Strip any literal chunk_id / doc_id occurrences and any leftover
        # 16-/32-hex tokens before the LLM sees the body.
        text = _wiki_scrub_known_ids(text, all_known_ids)
        if not text.strip():
            continue
        chunk_ids.append(cid)
        chunk_texts.append(text)

    if not chunk_texts:
        return _wiki_empty_extract()

    # Token budget mirrors compile_structure_from_text. We estimate overhead
    # from the user template alone — chunk_id_list / packed body grow with the
    # batch and are accounted for by split_chunks' per-chunk token counting.
    template_overhead = num_tokens_from_string(WIKI_MAP_SYSTEM + WIKI_MAP_USER_TEMPLATE)
    input_budget = int(chat_mdl.max_length * INPUT_UTILIZATION) - template_overhead
    if input_budget < 1024:
        input_budget = 1024

    raw_batches = split_chunks(chunk_texts, input_budget)
    if not raw_batches:
        return _wiki_empty_extract()

    # Convert split_chunks' [{idx: text}, ...] format into the packed list our
    # batch processor expects: [{label, chunk_id, text}, ...]. Labels are
    # per-batch positional tags (C1, C2, …) — see _wiki_safe_chunk_label for
    # the rationale; mapping back to real chunk ids happens in
    # _wiki_resolve_chunk_ids via label_to_id.
    packed_batches: list[list[dict]] = []
    for batch in raw_batches:
        packed: list[dict] = []
        for position, item in enumerate(batch):
            for idx, text in item.items():
                cid = chunk_ids[idx]
                packed.append({
                    "label": _wiki_safe_chunk_label(position),
                    "chunk_id": cid,
                    "text": text,
                })
        if packed:
            packed_batches.append(packed)

    if not packed_batches:
        return _wiki_empty_extract()

    total_batches = len(packed_batches)
    semaphore = asyncio.Semaphore(max_workers) if max_workers and max_workers > 0 else None

    tasks = [
        asyncio.create_task(
            _wiki_process_batch(
                packed=batch,
                batch_idx=bi,
                total_batches=total_batches,
                doc_id=doc_id,
                tenant_id=tenant_id,
                kb_id=kb_id,
                chat_mdl=chat_mdl,
                language=language,
                timeout=timeout,
                semaphore=semaphore,
                callback=callback,
            )
        )
        for bi, batch in enumerate(packed_batches)
    ]

    try:
        batch_results = await asyncio.gather(*tasks, return_exceptions=False)
    except Exception:
        for t in tasks:
            t.cancel()
        await asyncio.gather(*tasks, return_exceptions=True)
        raise

    merged = _wiki_merge_extracts(batch_results)
    logging.info(
        "wiki_map: doc %s — entities=%d concepts=%d claims=%d relations=%d topics=%d",
        doc_id,
        len(merged["entities"]),
        len(merged["concepts"]),
        len(merged["claims"]),
        len(merged["relations"]),
        len(merged["topics"]),
    )
    return merged


# ---------------------------------------------------------------------------
# REDUCE phase (KB-scoped)
# ---------------------------------------------------------------------------
#
# Migrated from D:/git/arkon/app/ai/mrp/reducer.py, steps 2.1-2.4.
# KB reconciliation (arkon 2.5-2.6) and the planning LLM call (arkon 2.7) are
# deferred to the PLAN phase — they belong with the planner, not the dedup.
#
# Scope difference from arkon: arkon REDUCE runs per source document. Here it
# runs per knowledge base — one set of canonical entities/concepts for the
# entire KB. Inputs come from ES (every wiki_map_extract row in this KB across
# all docs); the result lives in ES under wiki_reduce_result.

WIKI_REDUCE_COMPILE_KWD = "wiki_reduce_result"
DEFAULT_WIKI_REDUCE_MERGE_THRESHOLD = 0.95
DEFAULT_WIKI_REDUCE_AMBIGUOUS_LOW = 0.75
DEFAULT_WIKI_REDUCE_AMBIGUOUS_BATCH = 50
DEFAULT_WIKI_REDUCE_TIMEOUT = 60
_PUNCT_TABLE = str.maketrans("", "", string.punctuation)


WIKI_REDUCE_DISAMBIGUATE_SYSTEM = (
    "You are a named-entity resolution assistant. Return only JSON."
)


# --- helpers ---------------------------------------------------------------


def _wiki_normalize(name: str) -> str:
    if not isinstance(name, str):
        return ""
    return name.lower().strip().translate(_PUNCT_TABLE)


def _wiki_chunk_id_union(*lists) -> list[str]:
    """Order-preserving union of chunk_ids across multiple lists."""
    seen: list[str] = []
    for lst in lists:
        if not lst:
            continue
        for cid in lst:
            if not cid or not isinstance(cid, str):
                continue
            if cid not in seen:
                seen.append(cid)
    return seen


def _wiki_exact_dedup_entities(raw_entities: list[dict]) -> list[dict]:
    """Group entities by (normalized name, type). Canonical name = most common
    spelling. Unions aliases and chunk_ids; sums mention_count."""
    groups: dict[tuple, list[dict]] = {}
    for e in raw_entities:
        if not isinstance(e, dict):
            continue
        key = (_wiki_normalize(e.get("name", "")), e.get("type", "other"))
        if not key[0]:
            continue
        groups.setdefault(key, []).append(e)

    canonical: list[dict] = []
    for (norm_name, etype), group in groups.items():
        name_counts: dict[str, int] = {}
        for e in group:
            n = e.get("name", "")
            if n:
                name_counts[n] = name_counts.get(n, 0) + 1
        best_name = max(name_counts, key=lambda k: name_counts[k]) if name_counts else ""

        aliases: set[str] = set()
        chunk_ids_lists: list[list] = []
        mention_count = 0
        for e in group:
            n = e.get("name", "")
            if n:
                aliases.add(n)
            aliases.update(a for a in (e.get("aliases") or []) if isinstance(a, str))
            chunk_ids_lists.append(e.get("chunk_ids") or [])
            mention_count += 1
        aliases.discard(best_name)

        canonical.append({
            "name": best_name,
            "type": etype,
            "aliases": sorted(aliases),
            "mention_count": mention_count,
            "chunk_ids": _wiki_chunk_id_union(*chunk_ids_lists),
            "_norm": norm_name,
        })
    return canonical


def _wiki_exact_dedup_concepts(raw_concepts: list[dict]) -> list[dict]:
    """Group concepts by normalized term. Keeps the longest definition_excerpt."""
    groups: dict[str, list[dict]] = {}
    for c in raw_concepts:
        if not isinstance(c, dict):
            continue
        key = _wiki_normalize(c.get("term", ""))
        if not key:
            continue
        groups.setdefault(key, []).append(c)

    canonical: list[dict] = []
    for norm_term, group in groups.items():
        term_counts: dict[str, int] = {}
        for c in group:
            t = c.get("term", "")
            if t:
                term_counts[t] = term_counts.get(t, 0) + 1
        best_term = max(term_counts, key=lambda k: term_counts[k]) if term_counts else ""

        best_def = max(
            ((c.get("definition_excerpt") or "") for c in group),
            key=lambda s: len(s) if isinstance(s, str) else 0,
            default="",
        )
        chunk_ids_lists = [c.get("chunk_ids") or [] for c in group]
        canonical.append({
            "term": best_term,
            "definition_excerpt": best_def,
            "mention_count": len(group),
            "chunk_ids": _wiki_chunk_id_union(*chunk_ids_lists),
            "_norm": norm_term,
        })
    return canonical


async def _wiki_embed_names(names: list[str], embd_mdl) -> Optional[list]:
    """Encode a list of strings to vectors via LLMBundle.encode."""
    if not names:
        return []
    try:
        embeddings, _ = await thread_pool_exec(embd_mdl.encode, names)
    except Exception:
        logging.exception("wiki_reduce: embedding batch failed")
        return None
    return list(embeddings)


async def _wiki_embedding_dedup_entities(
    canonical: list[dict],
    embd_mdl,
    merge_threshold: float,
    ambiguous_low: float,
) -> tuple[dict[int, int], list[tuple[int, int]], Optional[list]]:
    """Pairwise cosine over canonical entity names, same-type only.

    Returns (auto_merged_into, ambiguous_pairs, vectors). On embedding failure
    returns ({}, [], None) so the caller can skip dedup gracefully.
    """
    n = len(canonical)
    if n <= 1:
        return {}, [], []

    vectors = await _wiki_embed_names([e.get("name", "") for e in canonical], embd_mdl)
    if vectors is None or len(vectors) != n:
        return {}, [], None

    # Vectorized pairwise cosine. Imported lazily so the module doesn't pay
    # the sklearn import cost when REDUCE isn't called.
    try:
        from sklearn.metrics.pairwise import cosine_similarity
        import numpy as np
        matrix = np.asarray([list(v) for v in vectors], dtype=float)
        sims = cosine_similarity(matrix)
    except Exception:
        logging.exception("wiki_reduce: pairwise cosine failed; skipping embedding dedup")
        return {}, [], vectors

    merged_into: dict[int, int] = {}

    def _root(i: int) -> int:
        while i in merged_into:
            i = merged_into[i]
        return i

    auto_pairs: list[tuple[int, int]] = []
    ambiguous_pairs: list[tuple[int, int]] = []

    for i in range(n):
        # iterate only upper triangle
        for j in range(i + 1, n):
            if canonical[i].get("type") != canonical[j].get("type"):
                continue
            s = float(sims[i, j])
            if s >= merge_threshold:
                auto_pairs.append((i, j))
            elif s >= ambiguous_low:
                ambiguous_pairs.append((i, j))

    # Apply auto-merges into a higher-mention canonical
    for i, j in auto_pairs:
        ri, rj = _root(i), _root(j)
        if ri == rj:
            continue
        if canonical[ri].get("mention_count", 0) >= canonical[rj].get("mention_count", 0):
            merged_into[rj] = ri
        else:
            merged_into[ri] = rj

    # Drop ambiguous pairs whose endpoints are already linked
    still_ambiguous = [(i, j) for i, j in ambiguous_pairs if _root(i) != _root(j)]
    return merged_into, still_ambiguous, vectors


async def _wiki_resolve_ambiguous_entities(
    canonical: list[dict],
    ambiguous_pairs: list[tuple[int, int]],
    chat_mdl,
    merged_into: dict[int, int],
    batch_size: int,
    timeout: int,
) -> dict[int, int]:
    """Batched LLM disambiguation. Each batch asks for a JSON bool array."""
    if not ambiguous_pairs:
        return merged_into

    def _root(i: int) -> int:
        while i in merged_into:
            i = merged_into[i]
        return i

    for batch_start in range(0, len(ambiguous_pairs), batch_size):
        batch = ambiguous_pairs[batch_start:batch_start + batch_size]
        # Drop any pair already linked since last batch
        batch = [(i, j) for i, j in batch if _root(i) != _root(j)]
        if not batch:
            continue

        lines = []
        for k, (i, j) in enumerate(batch):
            lines.append(
                f"{k + 1}. \"{canonical[i].get('name', '')}\" ({canonical[i].get('type', '')}) vs "
                f"\"{canonical[j].get('name', '')}\" ({canonical[j].get('type', '')})"
            )
        user_prompt = (
            f"For each pair below, determine if they refer to the same real-world entity.\n"
            f"Return a JSON array of exactly {len(batch)} booleans "
            "(true = same entity, false = different).\n"
            "Return ONLY the JSON array.\n\n" + "\n".join(lines)
        )
        try:
            res = await asyncio.wait_for(
                gen_json(WIKI_REDUCE_DISAMBIGUATE_SYSTEM, user_prompt, chat_mdl,
                         gen_conf={"temperature": 0.0}),
                timeout=timeout,
            )
        except asyncio.TimeoutError:
            logging.warning("wiki_reduce: disambiguation timed out (%d pairs)", len(batch))
            continue
        except Exception:
            logging.exception("wiki_reduce: disambiguation call failed (%d pairs)", len(batch))
            continue

        # gen_json may return the array directly or wrapped under a key.
        decisions = None
        if isinstance(res, list):
            decisions = res
        elif isinstance(res, dict):
            for v in res.values():
                if isinstance(v, list):
                    decisions = v
                    break
        if not isinstance(decisions, list):
            logging.warning("wiki_reduce: disambiguation returned unexpected shape: %r", type(res))
            continue

        for k, (i, j) in enumerate(batch):
            verdict = decisions[k] if k < len(decisions) else False
            if not verdict:
                continue
            ri, rj = _root(i), _root(j)
            if ri == rj:
                continue
            if canonical[ri].get("mention_count", 0) >= canonical[rj].get("mention_count", 0):
                merged_into[rj] = ri
            else:
                merged_into[ri] = rj

    return merged_into


def _wiki_apply_merges(canonical: list[dict], merged_into: dict[int, int]) -> list[dict]:
    """Union-find collapse: sum mention_count, union aliases + chunk_ids."""
    def _root(i: int) -> int:
        while i in merged_into:
            i = merged_into[i]
        return i

    roots: set[int] = {_root(i) for i in range(len(canonical))}
    out: list[dict] = []
    for ri in roots:
        base = dict(canonical[ri])
        aliases: set[str] = set(base.get("aliases") or [])
        chunk_id_lists: list[list] = [base.get("chunk_ids") or []]
        mention_count = base.get("mention_count", 0)
        for i, e in enumerate(canonical):
            if i == ri or _root(i) != ri:
                continue
            mention_count += e.get("mention_count", 0)
            aliases.update(e.get("aliases") or [])
            n = e.get("name") or e.get("term")
            if isinstance(n, str) and n:
                aliases.add(n)
            chunk_id_lists.append(e.get("chunk_ids") or [])
        aliases.discard(base.get("name") or base.get("term") or "")
        base["aliases"] = sorted(aliases)
        base["mention_count"] = mention_count
        base["chunk_ids"] = _wiki_chunk_id_union(*chunk_id_lists)
        out.append(base)
    return out


# --- ES I/O ----------------------------------------------------------------


async def _wiki_load_all_map_extracts(tenant_id: str, kb_id: str) -> dict:
    """Aggregate every wiki_map_extract row in this KB into one merged dict.

    Pages through ES if the KB has more than the per-call cap. Returns a dict
    in the same shape as wiki_map_from_chunks' return value.
    """
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr
    from rag.nlp import search as _rag_search

    index = _rag_search.index_name(tenant_id)
    condition = {"compile_kwd": [WIKI_MAP_COMPILE_KWD]}
    select_fields = ["id", "content_with_weight"]

    PAGE_SIZE = 1000
    offset = 0
    merged = _wiki_empty_extract()
    seen_topics: set[str] = set()

    while True:
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                select_fields, [], condition, [], OrderByExpr(),
                offset, PAGE_SIZE, index, [kb_id],
            )
            field_map = settings.docStoreConn.get_fields(res, select_fields)
        except Exception:
            logging.exception("wiki_reduce: failed to page wiki_map_extract rows")
            break

        if not field_map:
            break

        for row in field_map.values():
            content = row.get("content_with_weight")
            if not isinstance(content, str) or not content:
                continue
            try:
                payload = json.loads(content)
            except Exception:
                logging.debug("wiki_reduce: skipping unparseable extract row")
                continue
            if not isinstance(payload, dict):
                continue
            for key in _EXTRACT_LIST_KEYS:
                items = payload.get(key)
                if isinstance(items, list):
                    merged[key].extend(item for item in items if isinstance(item, dict))
            topics = payload.get("topics")
            if isinstance(topics, list):
                for t in topics:
                    if isinstance(t, str) and t and t not in seen_topics:
                        seen_topics.add(t)
                        merged["topics"].append(t)

        if len(field_map) < PAGE_SIZE:
            break
        offset += PAGE_SIZE

    return merged


async def _wiki_load_reduce_resume(tenant_id: str, kb_id: str) -> Optional[dict]:
    """Return the cached wiki_reduce_result for this KB, or None."""
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr
    from rag.nlp import search as _rag_search

    index = _rag_search.index_name(tenant_id)
    condition = {"compile_kwd": [WIKI_REDUCE_COMPILE_KWD]}
    select_fields = ["id", "content_with_weight"]
    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            select_fields, [], condition, [], OrderByExpr(),
            0, 1, index, [kb_id],
        )
        field_map = settings.docStoreConn.get_fields(res, select_fields)
    except Exception:
        logging.exception("wiki_reduce: failed to load resume cache")
        return None
    if not field_map:
        return None
    row = next(iter(field_map.values()))
    content = row.get("content_with_weight")
    if not isinstance(content, str) or not content:
        return None
    try:
        cached = json.loads(content)
    except Exception:
        logging.debug("wiki_reduce: cached result unparseable; ignoring")
        return None
    return cached if isinstance(cached, dict) else None


async def _wiki_persist_reduce(reduced: dict, tenant_id: str, kb_id: str) -> None:
    """Upsert the single non-searchable wiki_reduce_result row for this KB."""
    from common import settings
    from rag.nlp import search as _rag_search

    index = _rag_search.index_name(tenant_id)
    kb_id_str = str(kb_id)
    content_with_weight = json.dumps(reduced, ensure_ascii=False)
    # Stable id per KB so a re-run upserts the same row.
    row_id = xxhash.xxh64(
        (WIKI_REDUCE_COMPILE_KWD + ":" + kb_id_str).encode("utf-8", "surrogatepass")
    ).hexdigest()
    doc = {
        "id": row_id,
        "doc_id": kb_id_str,            # sentinel — KB-scoped row, not a real document
        "compile_kwd": WIKI_REDUCE_COMPILE_KWD,
        "source_id": [kb_id_str],
        "content_with_weight": content_with_weight,
        "available_int": 0,
    }
    try:
        # Best-effort delete then insert so re-runs replace cleanly.
        try:
            await thread_pool_exec(
                settings.docStoreConn.delete,
                {"compile_kwd": WIKI_REDUCE_COMPILE_KWD},
                index, kb_id,
            )
        except Exception:
            logging.debug("wiki_reduce: prior result delete failed; will overwrite by id")
        await thread_pool_exec(settings.docStoreConn.insert, [doc], index, kb_id)
    except Exception:
        logging.exception("wiki_reduce: failed to persist result row")


# --- public entry ----------------------------------------------------------


async def wiki_reduce_from_extracts(
    chat_mdl,
    embd_mdl,
    tenant_id: str,
    kb_id: str,
    merge_threshold: float = DEFAULT_WIKI_REDUCE_MERGE_THRESHOLD,
    ambiguous_low: float = DEFAULT_WIKI_REDUCE_AMBIGUOUS_LOW,
    ambiguous_batch_size: int = DEFAULT_WIKI_REDUCE_AMBIGUOUS_BATCH,
    timeout: int = DEFAULT_WIKI_REDUCE_TIMEOUT,
    force_rerun: bool = False,
    callback: Optional[Callable] = None,
) -> dict:
    """Phase 2 (REDUCE/Dedup) — KB-scoped.

    Loads every ``wiki_map_extract`` row in this KB (across all documents) and
    produces a single canonical dict of entities/concepts via:
        1. Exact dedup by ``(normalize(name), type)`` for entities and by
           ``normalize(term)`` for concepts.
        2. Embedding dedup of entity names: vectorized pairwise cosine over
           ``embd_mdl.encode(...)`` output. Pairs of the same type with
           similarity ≥ ``merge_threshold`` auto-merge; pairs in
           ``[ambiguous_low, merge_threshold)`` go to step 3.
        3. LLM disambiguation: batches of ambiguous pairs are sent to
           ``chat_mdl`` via ``gen_json``; true verdicts collapse via union-find.
        4. Apply merges: sum ``mention_count``, union ``aliases`` and
           ``chunk_ids`` per canonical entity.

    The result is persisted to ES as a single non-searchable
    ``wiki_reduce_result`` row per KB. Subsequent calls with
    ``force_rerun=False`` (default) return the cached row immediately; pass
    ``force_rerun=True`` after new ``wiki_map_extract`` rows have been added.

    Args:
        chat_mdl, embd_mdl: ragflow LLMBundle instances.
        tenant_id, kb_id: address the doc-store index.
        merge_threshold: cosine ≥ this auto-merges. Default 0.90.
        ambiguous_low: cosine in [ambiguous_low, merge_threshold) goes to LLM.
        ambiguous_batch_size: max pairs per LLM disambiguation call.
        timeout: seconds per LLM disambiguation batch.
        force_rerun: bypass the cached wiki_reduce_result.
        callback: optional ``(progress: float, msg: str)`` callback.

    Returns the canonical extract dict::

        {
          "entities":  [{"name","type","aliases","mention_count","chunk_ids"}, ...],
          "concepts":  [{"term","definition_excerpt","mention_count","chunk_ids"}, ...],
          "claims":    [...],   # pass-through from MAP
          "relations": [...],   # pass-through from MAP
          "topics":    [...],   # pass-through from MAP
        }
    """
    if not force_rerun:
        cached = await _wiki_load_reduce_resume(tenant_id, kb_id)
        if cached is not None:
            if callback:
                try:
                    callback(1.0, "wiki REDUCE: cache hit")
                except Exception:
                    pass
            return cached

    if callback:
        try:
            callback(0.05, "wiki REDUCE: loading MAP extracts")
        except Exception:
            pass

    raw = await _wiki_load_all_map_extracts(tenant_id, kb_id)
    raw_entities = raw.get("entities") or []
    raw_concepts = raw.get("concepts") or []
    logging.info(
        "wiki_reduce: kb=%s loaded raw entities=%d concepts=%d claims=%d relations=%d",
        kb_id,
        len(raw_entities),
        len(raw_concepts),
        len(raw.get("claims") or []),
        len(raw.get("relations") or []),
    )

    if not raw_entities and not raw_concepts:
        # Nothing to reduce; persist an empty result so resume can short-circuit.
        empty = _wiki_empty_extract()
        await _wiki_persist_reduce(empty, tenant_id, kb_id)
        return empty

    if callback:
        try:
            callback(0.25, "wiki REDUCE: exact dedup")
        except Exception:
            pass

    canonical_entities = _wiki_exact_dedup_entities(raw_entities)
    canonical_concepts = _wiki_exact_dedup_concepts(raw_concepts)
    logging.info(
        "wiki_reduce: after exact dedup entities=%d concepts=%d",
        len(canonical_entities),
        len(canonical_concepts),
    )

    if callback:
        try:
            callback(0.45, "wiki REDUCE: embedding dedup")
        except Exception:
            pass

    if len(canonical_entities) > 1:
        merged_into, ambiguous_pairs, vectors = await _wiki_embedding_dedup_entities( 
            canonical_entities, embd_mdl, merge_threshold, ambiguous_low,
        )
        if vectors is None:
            logging.warning("wiki_reduce: embedding dedup skipped (embedding failure)")
        else:
            if callback:
                try:
                    callback(
                        0.65,
                        f"wiki REDUCE: LLM disambiguation ({len(ambiguous_pairs)} pairs)",
                    )
                except Exception:
                    pass
            merged_into = await _wiki_resolve_ambiguous_entities(
                canonical_entities,
                ambiguous_pairs,
                chat_mdl,
                merged_into,
                batch_size=ambiguous_batch_size,
                timeout=timeout,
            )
            canonical_entities = _wiki_apply_merges(canonical_entities, merged_into)
            logging.info("wiki_reduce: after embedding+LLM dedup entities=%d", len(canonical_entities))

    # Drop the private _norm key before returning / persisting.
    for e in canonical_entities:
        e.pop("_norm", None)
    for c in canonical_concepts:
        c.pop("_norm", None)

    reduced = {
        "entities": canonical_entities,
        "concepts": canonical_concepts,
        "claims": list(raw.get("claims") or []),
        "relations": list(raw.get("relations") or []),
        "topics": list(raw.get("topics") or []),
    }

    if callback:
        try:
            callback(0.9, "wiki REDUCE: persisting result")
        except Exception:
            pass
    await _wiki_persist_reduce(reduced, tenant_id, kb_id)

    logging.info(
        "wiki_reduce: kb=%s done — entities=%d concepts=%d claims=%d relations=%d topics=%d",
        kb_id,
        len(reduced["entities"]),
        len(reduced["concepts"]),
        len(reduced["claims"]),
        len(reduced["relations"]),
        len(reduced["topics"]),
    )

    if callback:
        try:
            callback(1.0, "wiki REDUCE: done")
        except Exception:
            pass

    return reduced


# ---------------------------------------------------------------------------
# PLAN phase (KB-scoped)
# ---------------------------------------------------------------------------
#
# Migrated from D:/git/arkon/app/ai/mrp/reducer.py, steps 2.5-2.7 + 2.8 persist.
# Scope: per KB (one Compilation Plan covering the entire knowledge base),
# matching the REDUCE phase above.
#
# Flow:
#   1. Resume — return cached wiki_compilation_plan ES row when present.
#   2. Load REDUCE output from wiki_reduce_result.
#   3. KB reconciliation — batch-embed entity/concept query texts and run a
#      per-item KNN against existing wiki_page rows in this KB. Classify
#      UPDATE / MAYBE / CREATE by similarity. Batched LLM resolves MAYBE.
#   4. Planning call — one gen_json call producing the Compilation Plan JSON.
#   5. Attach raw items as side context for REFINE (no extra ES round-trips).
#   6. Persist as a single non-searchable wiki_compilation_plan row per KB.
#
# Differences vs arkon: KB-scoped instead of per-source; no `source` pages
# emitted (chunk_ids attribution is enough); plan status defaults to
# "approved" so REFINE can consume immediately (review workflow deferred).

WIKI_PLAN_COMPILE_KWD = "wiki_compilation_plan"
WIKI_PAGE_COMPILE_KWD = "wiki_page"
DEFAULT_WIKI_PLAN_UPDATE_THRESHOLD = 0.85
DEFAULT_WIKI_PLAN_MAYBE_THRESHOLD = 0.60
DEFAULT_WIKI_PLAN_TIMEOUT = 600  # ~10 min — the planning call emits one big
                                 # JSON plan and reasoning models can spend a
                                 # long time thinking before emitting tokens.
                                 # Override via the ``timeout`` arg to
                                 # ``wiki_plan_from_reduction``.
DEFAULT_WIKI_PLAN_RECONCILE_BATCH = 50


WIKI_PLAN_PLANNING_SYSTEM = (
    "You are a wiki compilation planner. Given extracted entities and their "
    "relationship to an existing knowledge base, produce a compilation plan. "
    "Return ONLY valid JSON."
)


WIKI_PLAN_RECONCILE_SYSTEM = (
    "You are a knowledge base assistant. Return only a JSON boolean array."
)


WIKI_PLAN_USER_TEMPLATE = """\
## Knowledge base context
Name: {kb_name}
Description: {kb_description}

## Extracted entities (with mention counts)
{entities_summary}

## Extracted concepts (with mention counts)
{concepts_summary}

## KB reconciliation results
{kb_reconciliation}

Produce a JSON compilation plan:

{{
  "pages": [
    {{
      "action": "CREATE",
      "slug": "concept/example-name",
      "title": "Example Page Title",
      "page_type": "entity | concept | topic",
      "entity_names": ["entity or concept name covered by this page"],
      "related_kb_pages": ["existing-slug-1"],
      "priority": 1
    }}
  ],
  "estimated_page_count": 5,
  "compilation_notes": "any important notes for the compiler"
}}

Rules:
- action must be "CREATE" or "UPDATE".
- For UPDATE, slug MUST be an existing wiki page slug from the KB
  reconciliation list above.
- For CREATE, slug must be new (type-prefixed, lowercase, hyphenated). Use
  English/Latin characters even when the source language differs.
- page_type is one of: entity | concept | topic. Do NOT use "source".
- Group closely related small entities onto the same page (max 3-4 per page).
  BUT if a primary entity is described through several distinct thematic
  sections that appear as concepts above, prefer a separate ``concept`` page
  for EACH such section instead of collapsing them onto the entity page.
- priority 1 = highest importance (process first).
- entity_names must match the names in the entities / concepts lists above.
- Target approximately {target_page_count} total pages (feel free to deviate
  by ±50% if the KB content warrants it).
- Return ONLY the JSON object.
"""


# --- helpers ---------------------------------------------------------------


def _wiki_target_page_count(total_items: int) -> int:
    """Item-count-based heuristic: clamp(8, total // 3, 60)."""
    if total_items <= 0:
        return 8
    return max(8, min(60, total_items // 3))


def _wiki_format_entity_for_plan(entity: dict, reconciliation: dict) -> str:
    aliases = ", ".join((entity.get("aliases") or [])[:3])
    rec = reconciliation.get(entity.get("name", ""), {})
    action = rec.get("action", "CREATE")
    slug = rec.get("page_slug", "")
    kb_info = f"→ {action} {slug}".rstrip()
    line = (
        f"  - {entity.get('name', '')} ({entity.get('type', '')}, "
        f"{entity.get('mention_count', 0)} mentions"
    )
    if aliases:
        line += f", aliases: {aliases}"
    line += f") {kb_info}"
    return line


def _wiki_format_concept_for_plan(concept: dict, reconciliation: dict) -> str:
    rec = reconciliation.get(concept.get("term", ""), {})
    action = rec.get("action", "CREATE")
    slug = rec.get("page_slug", "")
    kb_info = f"→ {action} {slug}".rstrip()
    return (
        f"  - {concept.get('term', '')} ({concept.get('mention_count', 0)} mentions) "
        f"{kb_info}"
    )


async def _wiki_reconcile_with_kb(
    canonical_entities: list[dict],
    canonical_concepts: list[dict],
    embd_mdl,
    tenant_id: str,
    kb_id: str,
    update_threshold: float,
    maybe_threshold: float,
) -> dict[str, dict]:
    """Per-entity / per-concept KNN against compile_kwd=wiki_page rows in this KB.

    Returns ``{name_or_term: {"action", "page_slug", "page_title", "page_id",
    "similarity"}}``. When no wiki pages exist (first run before REFINE), every
    item maps to ``action="CREATE"``.
    """
    from common import settings
    from common.doc_store.doc_store_base import MatchDenseExpr, OrderByExpr
    from rag.nlp import search as _rag_search

    items: list[tuple[str, str, dict]] = []  # (kind, key, source_dict)
    for e in canonical_entities:
        name = e.get("name")
        if isinstance(name, str) and name:
            items.append(("entity", name, e))
    for c in canonical_concepts:
        term = c.get("term")
        if isinstance(term, str) and term:
            items.append(("concept", term, c))

    reconciliation: dict[str, dict] = {}
    if not items:
        return reconciliation

    # Embed all query texts in one batch.
    query_texts: list[str] = []
    for kind, key, src in items:
        if kind == "concept":
            defn = src.get("definition_excerpt") or ""
            text = f"{key}: {defn[:200]}" if defn else key
        else:
            text = key
        query_texts.append(text[:4000])

    try:
        embeddings, _ = await thread_pool_exec(embd_mdl.encode, query_texts)
        vectors = list(embeddings)
    except Exception:
        logging.exception("wiki_plan: reconciliation embedding failed — all items will be CREATE")
        for _, key, _ in items:
            reconciliation[key] = {
                "action": "CREATE", "page_slug": None,
                "page_title": None, "page_id": None, "similarity": 0.0,
            }
        return reconciliation

    if len(vectors) != len(items):
        logging.error(
            "wiki_plan: reconciliation embedding count mismatch (%d vs %d); CREATE all",
            len(vectors), len(items),
        )
        for _, key, _ in items:
            reconciliation[key] = {
                "action": "CREATE", "page_slug": None,
                "page_title": None, "page_id": None, "similarity": 0.0,
            }
        return reconciliation

    index = _rag_search.index_name(tenant_id)
    condition = {"compile_kwd": [WIKI_PAGE_COMPILE_KWD]}
    select_fields = ["id", "wiki_slug_kwd", "wiki_title_kwd", "wiki_page_type_kwd"]

    for (_kind, key, _src), vec in zip(items, vectors):
        vec_list = list(vec) if not hasattr(vec, "tolist") else vec.tolist()
        if not vec_list:
            reconciliation[key] = {
                "action": "CREATE", "page_slug": None,
                "page_title": None, "page_id": None, "similarity": 0.0,
            }
            continue
        match_expr = MatchDenseExpr(
            vector_column_name=f"q_{len(vec_list)}_vec",
            embedding_data=vec_list,
            embedding_data_type="float",
            distance_type="cosine",
            topn=1,
            extra_options={"similarity": maybe_threshold},
        )
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                select_fields, [], condition, [match_expr], OrderByExpr(),
                0, 1, index, [kb_id],
            )
            field_map = settings.docStoreConn.get_fields(res, select_fields)
        except Exception:
            logging.exception("wiki_plan: KNN failed for %r", key)
            reconciliation[key] = {
                "action": "CREATE", "page_slug": None,
                "page_title": None, "page_id": None, "similarity": 0.0,
            }
            continue

        if not field_map:
            reconciliation[key] = {
                "action": "CREATE", "page_slug": None,
                "page_title": None, "page_id": None, "similarity": 0.0,
            }
            continue

        top_id, top_row = next(iter(field_map.items()))
        # Pull similarity from the search result if exposed; fall back to threshold floor.
        sim = 0.0
        try:
            sim_field = getattr(res, "similarity", None) or getattr(res, "scores", None)
            if isinstance(sim_field, dict) and top_id in sim_field:
                sim = float(sim_field[top_id])
        except Exception:
            sim = 0.0
        if sim <= 0.0:
            sim = float(top_row.get("similarity", maybe_threshold))

        slug = top_row.get("wiki_slug_kwd")
        title = top_row.get("wiki_title_kwd")
        if sim >= update_threshold:
            action = "UPDATE"
        else:
            action = "MAYBE"
        reconciliation[key] = {
            "action": action,
            "page_slug": slug,
            "page_title": title,
            "page_id": top_id,
            "similarity": sim,
        }

    return reconciliation


async def _wiki_resolve_maybe_items(
    reconciliation: dict[str, dict],
    chat_mdl,
    batch_size: int,
    timeout: int,
) -> None:
    """Flip MAYBE → UPDATE | CREATE via batched LLM calls. Mutates in place."""
    maybe_items = [(k, v) for k, v in reconciliation.items() if v.get("action") == "MAYBE"]
    if not maybe_items:
        return

    for batch_start in range(0, len(maybe_items), batch_size):
        batch = maybe_items[batch_start:batch_start + batch_size]
        lines = []
        for k, (name, rec) in enumerate(batch):
            title = rec.get("page_title") or rec.get("page_slug") or ""
            slug = rec.get("page_slug") or ""
            sim = rec.get("similarity", 0.0)
            lines.append(
                f"{k + 1}. Entity: \"{name}\" — existing wiki page: \"{title}\" "
                f"(slug: {slug}, similarity: {sim:.2f})"
            )

        user_prompt = (
            "For each pair below, decide whether the entity refers to the same "
            "real-world concept as the existing wiki page (true = UPDATE existing "
            "page, false = CREATE new page).\n"
            f"Return a JSON array of exactly {len(batch)} booleans. "
            "Return ONLY the JSON array.\n\n" + "\n".join(lines)
        )

        try:
            res = await asyncio.wait_for(
                gen_json(WIKI_PLAN_RECONCILE_SYSTEM, user_prompt, chat_mdl,
                         gen_conf={"temperature": 0.0}),
                timeout=timeout,
            )
        except asyncio.TimeoutError:
            logging.warning("wiki_plan: MAYBE resolution timed out (%d pairs); defaulting CREATE", len(batch))
            for name, _ in batch:
                reconciliation[name]["action"] = "CREATE"
            continue
        except Exception:
            logging.exception("wiki_plan: MAYBE resolution failed (%d pairs); defaulting CREATE", len(batch))
            for name, _ in batch:
                reconciliation[name]["action"] = "CREATE"
            continue

        decisions = None
        if isinstance(res, list):
            decisions = res
        elif isinstance(res, dict):
            for v in res.values():
                if isinstance(v, list):
                    decisions = v
                    break

        if not isinstance(decisions, list):
            logging.warning("wiki_plan: MAYBE LLM returned unexpected shape %r; CREATE all", type(res))
            for name, _ in batch:
                reconciliation[name]["action"] = "CREATE"
            continue

        for k, (name, _) in enumerate(batch):
            verdict = decisions[k] if k < len(decisions) else False
            reconciliation[name]["action"] = "UPDATE" if verdict else "CREATE"


async def _wiki_planning_call(
    canonical_entities: list[dict],
    canonical_concepts: list[dict],
    reconciliation: dict[str, dict],
    chat_mdl,
    kb_name: str | None,
    kb_description: str | None,
    target_page_count: int,
    timeout: int,
) -> dict:
    """Single LLM call → Compilation Plan JSON."""
    # Sort by mention count descending so the planner sees the most important
    # items first; cap to keep the prompt size reasonable.
    sorted_entities = sorted(
        canonical_entities, key=lambda x: x.get("mention_count", 0), reverse=True,
    )
    sorted_concepts = sorted(
        canonical_concepts, key=lambda x: x.get("mention_count", 0), reverse=True,
    )

    entities_summary = "\n".join(
        _wiki_format_entity_for_plan(e, reconciliation) for e in sorted_entities[:200]
    ) or "  (none)"
    concepts_summary = "\n".join(
        _wiki_format_concept_for_plan(c, reconciliation) for c in sorted_concepts[:200]
    ) or "  (none)"

    kb_lines: list[str] = []
    for name, rec in reconciliation.items():
        if rec.get("action") == "UPDATE" and rec.get("page_slug"):
            kb_lines.append(
                f"  - UPDATE: {name} → {rec['page_slug']} "
                f"(sim={rec.get('similarity', 0.0):.2f})"
            )
    kb_reconciliation = "\n".join(kb_lines) if kb_lines else "  (all items are new)"

    user_prompt = WIKI_PLAN_USER_TEMPLATE.format(
        kb_name=kb_name or "(unspecified)",
        kb_description=kb_description or "(no description)",
        entities_summary=entities_summary,
        concepts_summary=concepts_summary,
        kb_reconciliation=kb_reconciliation,
        target_page_count=target_page_count,
    )

    try:
        res = await asyncio.wait_for(
            gen_json(WIKI_PLAN_PLANNING_SYSTEM, user_prompt, chat_mdl,
                     gen_conf={"temperature": 0.1}),
            timeout=timeout,
        )
    except asyncio.TimeoutError:
        logging.warning("wiki_plan: planning LLM call timed out after %ds", timeout)
        return {"pages": [], "estimated_page_count": 0, "compilation_notes": "planning timeout"}
    except Exception:
        logging.exception("wiki_plan: planning LLM call failed")
        return {"pages": [], "estimated_page_count": 0, "compilation_notes": "planning failed"}

    if not isinstance(res, dict):
        return {"pages": [], "estimated_page_count": 0, "compilation_notes": "planner returned non-object"}
    if "pages" not in res or not isinstance(res.get("pages"), list):
        res["pages"] = []
    if "estimated_page_count" not in res:
        res["estimated_page_count"] = len(res["pages"])
    res.setdefault("compilation_notes", "")
    return res


# --- ES I/O ---------------------------------------------------------------


async def _wiki_load_reduce_result(tenant_id: str, kb_id: str) -> Optional[dict]:
    """Load the cached REDUCE output for this KB."""
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr
    from rag.nlp import search as _rag_search

    index = _rag_search.index_name(tenant_id)
    condition = {"compile_kwd": [WIKI_REDUCE_COMPILE_KWD]}
    select_fields = ["id", "content_with_weight"]
    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            select_fields, [], condition, [], OrderByExpr(),
            0, 1, index, [kb_id],
        )
        field_map = settings.docStoreConn.get_fields(res, select_fields)
    except Exception:
        logging.exception("wiki_plan: failed to load wiki_reduce_result")
        return None
    if not field_map:
        return None
    row = next(iter(field_map.values()))
    content = row.get("content_with_weight")
    if not isinstance(content, str) or not content:
        return None
    try:
        cached = json.loads(content)
    except Exception:
        logging.debug("wiki_plan: wiki_reduce_result unparseable; ignoring")
        return None
    return cached if isinstance(cached, dict) else None


async def _wiki_load_plan_resume(tenant_id: str, kb_id: str) -> Optional[dict]:
    """Return the cached wiki_compilation_plan for this KB, or None."""
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr
    from rag.nlp import search as _rag_search

    index = _rag_search.index_name(tenant_id)
    condition = {"compile_kwd": [WIKI_PLAN_COMPILE_KWD]}
    select_fields = ["id", "content_with_weight"]
    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            select_fields, [], condition, [], OrderByExpr(),
            0, 1, index, [kb_id],
        )
        field_map = settings.docStoreConn.get_fields(res, select_fields)
    except Exception:
        logging.exception("wiki_plan: failed to load cached plan")
        return None
    if not field_map:
        return None
    row = next(iter(field_map.values()))
    content = row.get("content_with_weight")
    if not isinstance(content, str) or not content:
        return None
    try:
        cached = json.loads(content)
    except Exception:
        logging.debug("wiki_plan: cached plan unparseable; ignoring")
        return None
    return cached if isinstance(cached, dict) else None


async def _wiki_persist_plan(plan: dict, tenant_id: str, kb_id: str) -> None:
    """Upsert the single non-searchable wiki_compilation_plan row for this KB."""
    from common import settings
    from rag.nlp import search as _rag_search

    index = _rag_search.index_name(tenant_id)
    kb_id_str = str(kb_id)
    content_with_weight = json.dumps(plan, ensure_ascii=False)
    row_id = xxhash.xxh64(
        (WIKI_PLAN_COMPILE_KWD + ":" + kb_id_str).encode("utf-8", "surrogatepass")
    ).hexdigest()
    doc = {
        "id": row_id,
        "doc_id": kb_id_str,          # sentinel — KB-scoped row, not a real document
        "compile_kwd": WIKI_PLAN_COMPILE_KWD,
        "source_id": [kb_id_str],
        "content_with_weight": content_with_weight,
        "available_int": 0,
    }
    try:
        try:
            await thread_pool_exec(
                settings.docStoreConn.delete,
                {"compile_kwd": WIKI_PLAN_COMPILE_KWD},
                index, kb_id,
            )
        except Exception:
            logging.debug("wiki_plan: prior plan delete failed; relying on id-based upsert")
        await thread_pool_exec(settings.docStoreConn.insert, [doc], index, kb_id)
    except Exception:
        logging.exception("wiki_plan: failed to persist plan row")


# --- public entry ---------------------------------------------------------


async def wiki_plan_from_reduction(
    chat_mdl,
    embd_mdl,
    tenant_id: str,
    kb_id: str,
    kb_name: Optional[str] = None,
    kb_description: Optional[str] = None,
    update_threshold: float = DEFAULT_WIKI_PLAN_UPDATE_THRESHOLD,
    maybe_threshold: float = DEFAULT_WIKI_PLAN_MAYBE_THRESHOLD,
    reconcile_batch_size: int = DEFAULT_WIKI_PLAN_RECONCILE_BATCH,
    timeout: int = DEFAULT_WIKI_PLAN_TIMEOUT,
    force_rerun: bool = False,
    callback: Optional[Callable] = None,
) -> dict:
    """Phase 3 (PLAN) — KB-scoped.

    Loads the cached ``wiki_reduce_result`` for this KB, reconciles every
    canonical entity/concept against existing ``wiki_page`` rows in the same
    KB (top-1 KNN, with MAYBE matches resolved by a batched LLM call), then
    asks the LLM for one Compilation Plan JSON. The plan is persisted under
    ``compile_kwd="wiki_compilation_plan"`` with ``_status="approved"`` so
    REFINE can consume it immediately.

    Args:
        chat_mdl, embd_mdl: ragflow LLMBundle instances.
        tenant_id, kb_id: address the doc-store index.
        kb_name / kb_description: optional KB-level metadata that biases the
            planner's slug and tone choices.
        update_threshold: cosine ≥ this → UPDATE the existing page outright.
        maybe_threshold: cosine in [maybe_threshold, update_threshold) → ask LLM.
        reconcile_batch_size: max pairs per LLM MAYBE-resolution call.
        timeout: seconds per LLM call (both MAYBE resolution and planning).
        force_rerun: bypass the cached wiki_compilation_plan.
        callback: optional ``(progress: float, msg: str)`` callback.

    Returns the plan dict with this shape (plus underscore-prefixed side
    context fields for REFINE)::

        {
          "pages":               [{action, slug, title, page_type, entity_names, related_kb_pages, priority}, ...],
          "estimated_page_count": int,
          "compilation_notes":   str,
          "_status":             "approved",
          "_entities":           [...],   # canonical entities from REDUCE
          "_concepts":           [...],
          "_claims":             [...],
          "_relations":          [...],
          "_topics":             [...],
          "_reconciliation":     {name: {action, page_slug, page_id, similarity}, ...},
        }
    """
    if not force_rerun:
        cached = await _wiki_load_plan_resume(tenant_id, kb_id)
        if cached is not None:
            if callback:
                try:
                    callback(1.0, "wiki PLAN: cache hit")
                except Exception:
                    pass
            return cached

    if callback:
        try:
            callback(0.05, "wiki PLAN: loading REDUCE result")
        except Exception:
            pass

    reduced = await _wiki_load_reduce_result(tenant_id, kb_id)
    if reduced is None:
        logging.warning("wiki_plan: no wiki_reduce_result found for kb=%s — returning empty plan", kb_id)
        empty = {
            "pages": [], "estimated_page_count": 0,
            "compilation_notes": "no REDUCE result available",
            "_status": "approved",
            "_entities": [], "_concepts": [], "_claims": [], "_relations": [], "_topics": [],
            "_reconciliation": {},
        }
        await _wiki_persist_plan(empty, tenant_id, kb_id)
        return empty

    canonical_entities = reduced.get("entities") or []
    canonical_concepts = reduced.get("concepts") or []
    raw_claims = reduced.get("claims") or []
    raw_relations = reduced.get("relations") or []
    raw_topics = reduced.get("topics") or []

    total_items = len(canonical_entities) + len(canonical_concepts)
    logging.info(
        "wiki_plan: kb=%s reducing-input entities=%d concepts=%d (total=%d)",
        kb_id, len(canonical_entities), len(canonical_concepts), total_items,
    )

    if total_items == 0:
        empty = {
            "pages": [], "estimated_page_count": 0,
            "compilation_notes": "no canonical items",
            "_status": "approved",
            "_entities": canonical_entities, "_concepts": canonical_concepts,
            "_claims": raw_claims, "_relations": raw_relations, "_topics": raw_topics,
            "_reconciliation": {},
        }
        await _wiki_persist_plan(empty, tenant_id, kb_id)
        return empty

    if callback:
        try:
            callback(0.25, "wiki PLAN: KB reconciliation")
        except Exception:
            pass

    reconciliation = await _wiki_reconcile_with_kb(
        canonical_entities=canonical_entities,
        canonical_concepts=canonical_concepts,
        embd_mdl=embd_mdl,
        tenant_id=tenant_id,
        kb_id=kb_id,
        update_threshold=update_threshold,
        maybe_threshold=maybe_threshold,
    )

    if callback:
        n_maybe = sum(1 for v in reconciliation.values() if v.get("action") == "MAYBE")
        try:
            callback(0.55, f"wiki PLAN: resolving {n_maybe} MAYBE items")
        except Exception:
            pass

    await _wiki_resolve_maybe_items(
        reconciliation, chat_mdl,
        batch_size=reconcile_batch_size,
        timeout=timeout,
    )

    if callback:
        try:
            callback(0.75, "wiki PLAN: planning LLM call")
        except Exception:
            pass

    target = _wiki_target_page_count(total_items)
    plan = await _wiki_planning_call(
        canonical_entities=canonical_entities,
        canonical_concepts=canonical_concepts,
        reconciliation=reconciliation,
        chat_mdl=chat_mdl,
        kb_name=kb_name,
        kb_description=kb_description,
        target_page_count=target,
        timeout=timeout,
    )

    plan["_status"] = "approved"
    plan["_entities"] = canonical_entities
    plan["_concepts"] = canonical_concepts
    plan["_claims"] = raw_claims
    plan["_relations"] = raw_relations
    plan["_topics"] = raw_topics
    plan["_reconciliation"] = reconciliation

    if callback:
        try:
            callback(0.9, "wiki PLAN: persisting plan")
        except Exception:
            pass
    await _wiki_persist_plan(plan, tenant_id, kb_id)

    logging.info(
        "wiki_plan: kb=%s done — pages=%d (target=%d) updates=%d creates=%d",
        kb_id,
        len(plan.get("pages") or []),
        target,
        sum(1 for v in reconciliation.values() if v.get("action") == "UPDATE"),
        sum(1 for v in reconciliation.values() if v.get("action") == "CREATE"),
    )

    if callback:
        try:
            callback(1.0, "wiki PLAN: done")
        except Exception:
            pass

    return plan


# ---------------------------------------------------------------------------
# REFINE phase (KB-scoped)
# ---------------------------------------------------------------------------
#
# Migrated from D:/git/arkon/app/ai/mrp/writer.py (simple writer path) and
# merger.py (merge_page_content).
#
# Scope: per KB. Consumes the wiki_compilation_plan row written by PLAN,
# writes one wiki_page per planned page in parallel under a semaphore.
# UPDATE actions LLM-merge new vs existing content with a 70 % shrink-check
# fallback to the new content. Each written page is persisted to ES as a
# searchable wiki_page row (with embedding) so PLAN reconciliation finds it
# on the next REDUCE→PLAN cycle.
#
# Resume: per-slug wiki_page_draft rows act as a cache; a re-entry skips
# slugs already cached unless force_rerun=True.
#
# Differences vs arkon: no full_text — source context is the union of the
# evidence chunks fetched from ES by id. Image-marker handling and the
# complex tool-using writer are deliberately deferred.

WIKI_DRAFT_COMPILE_KWD = "wiki_page_draft"
DEFAULT_WIKI_REFINE_WORKERS = 4
DEFAULT_WIKI_REFINE_TIMEOUT = 300
WIKI_REFINE_SOURCE_BUDGET_CHARS = 60_000
WIKI_MERGE_BODY_SHRINK_THRESHOLD = 0.7
WIKI_MERGE_TIMEOUT = 600


WIKI_REFINE_WRITER_SYSTEM = (
    "You are an enterprise knowledge wiki writer. Your job is to write a single, "
    "high-quality wiki page by reading the SOURCE TEXT provided and using the "
    "evidence checklist as guidance for what to cover.\n\n"
    "# Mindset: COMPILE, do NOT summarize\n"
    "You are not writing an executive summary. You are extracting structured "
    "knowledge and rewriting it into a reusable wiki page. The output should "
    "contain MORE information density than a summary — organized differently, "
    "but not condensed. A summary loses specifics. A wiki page preserves them "
    "in a queryable structure.\n\n"
    "# What to KEEP from the source (do not lose these)\n"
    "- Specific numbers: thresholds, dosages, timeframes, dimensions, percentages.\n"
    "- Named regulations, laws, articles, code references.\n"
    "- Equipment names, model numbers, product specs.\n"
    "- Procedure steps in order, with actual actions.\n"
    "- Worked examples and exceptions.\n"
    "- Named parties, roles, contact paths, escalation chains.\n"
    "- Definitions verbatim or near-verbatim if the source is authoritative.\n"
    "- Cause-effect statements ('X causes Y because Z') — preserve all three parts.\n\n"
    "# What to DROP\n"
    "- Marketing language, mission statements, ceremonial filler.\n"
    "- Source-specific framing: 'This document explains…', 'In Section 3 below…'.\n"
    "- Repeated boilerplate, tables of contents, cover-page metadata.\n"
    "- Prose that just rephrases what was already said.\n\n"
    "# Language\n"
    "Write in the SAME LANGUAGE as the source text. Never translate content.\n\n"
    "# Page structure — CRITICAL\n"
    "Each page must be a proper encyclopedic article, NOT a flat bullet list:\n"
    "1. Opening paragraph (2-4 sentences defining what this is). No heading.\n"
    "2. Sections with H2 headings, each starting with prose before sub-bullets.\n"
    "3. Bold key terms on first use; link them with [[ ]] wikilinks.\n"
    "4. Examples or implications where the source provides them.\n"
    "5. ## See also section at the end with wikilinks to related pages.\n\n"
    "# What NOT to do\n"
    "- Do NOT dump raw bullet points from the source as the entire content.\n"
    "- Do NOT omit the opening prose paragraph.\n"
    "- Do NOT include Citations / Footnotes sections.\n"
    "- Do NOT use [^N] footnote markers.\n"
    "- Do NOT translate the content language.\n\n"
    "# Wikilinks\n"
    "- Use [[slug]] or [[slug|display text]] to cross-link.\n"
    "- CRITICAL: You may ONLY link to slugs from the 'Available pages' list.\n"
    "  Do NOT invent or hallucinate slugs.\n\n"
    "# Minimum depth\n"
    "- concept/topic pages: at least 200 words of actual prose+structure.\n"
    "- entity pages: at least 100 words.\n"
)


WIKI_REFINE_WRITER_USER_TEMPLATE = """\
## Task
{action} the following wiki page.

## Page specification
- Slug: {slug}
- Title: {title}
- Type: {page_type}

## Available pages (ONLY use these slugs for [[wikilinks]])
{all_plan_slugs}

{existing_section}

## Source document text
Read this carefully. Extract all relevant facts for this page's topic.

{source_context}

## Evidence checklist ({evidence_count} items)
The following items were pre-extracted and should be covered in the page.
Use them as a checklist — make sure you don't miss any of these facts.
But also look for additional relevant information in the source text above.

{evidence_blocks}

## Instructions
Write the complete wiki page in markdown based on the source text above.
Cross-link to other pages using [[slug]] or [[slug|display text]] — ONLY
use slugs from the "Available pages" list. Do NOT invent new slugs.
Do NOT include Citations or Footnotes sections.

Return ONLY the markdown content, no other text.
"""


WIKI_REFINE_MERGE_SYSTEM = (
    "You are a wiki page merger. You receive two versions of the same wiki page:\n"
    "- EXISTING: the current version in the knowledge base.\n"
    "- INCOMING: a new version generated from a different source document.\n\n"
    "Your job is to produce a SINGLE unified page that preserves ALL factual "
    "content from BOTH versions. Rules:\n\n"
    "1. KEEP all facts, numbers, procedures, names from both versions.\n"
    "2. REMOVE exact duplicates — if both versions state the same fact, keep it once.\n"
    "3. ORGANIZE coherently — clear H2 sections, opening paragraph, ## See also.\n"
    "4. PRESERVE [[wikilinks]] from both versions.\n"
    "5. Write in the SAME LANGUAGE as the existing content.\n"
    "6. Do NOT summarize or condense — the merged page should be AT LEAST as long "
    "as the longer of the two inputs.\n"
    "7. Do NOT add any facts not present in either version.\n\n"
    "Return ONLY the merged markdown content, no other text."
)


# --- helpers ---------------------------------------------------------------


_REFINE_THINK_PREFIX_RE = re.compile(r"^.*</think>", re.DOTALL)


def _wiki_strip_think(raw: str) -> str:
    """Strip a leading ``...</think>`` block that some LLMs emit."""
    if not isinstance(raw, str):
        return ""
    return _REFINE_THINK_PREFIX_RE.sub("", raw).strip()


def _wiki_assemble_evidence(
    plan_item: dict,
    claims: list[dict],
    entity_by_name: dict[str, dict] | None = None,
    concept_by_term: dict[str, dict] | None = None,
) -> list[dict]:
    """Find claims whose `subject` matches any `entity_name` in the plan item.

    Match is case-insensitive: exact match on the full normalized subject, or
    whole-word substring match for multi-word subjects. Each returned
    evidence item carries chunk_ids[] for downstream source-context loading.

    Fallback: if no claim attributes this page (a common case when the MAP
    LLM extracted entities but no claims for them), synthesize a single
    evidence stub from the canonical entity/concept records — that way
    provenance (chunk_ids / source_doc_ids) and the source-context fetch
    still resolve to the chunks that produced the entity/concept itself.
    Pass ``entity_by_name`` / ``concept_by_term`` (lowercased-key lookups
    over ``plan["_entities"]`` / ``plan["_concepts"]``) to enable the
    fallback.
    """
    raw_names = [
        n.strip() for n in (plan_item.get("entity_names") or [])
        if isinstance(n, str) and n.strip()
    ]
    if not raw_names:
        return []

    names_lower = [n.lower() for n in raw_names]
    patterns = [re.compile(rf"\b{re.escape(n)}\b", re.IGNORECASE) for n in raw_names]

    evidence: list[dict] = []
    for claim in claims:
        if not isinstance(claim, dict):
            continue
        subj_raw = (claim.get("subject") or "").strip()
        if not subj_raw:
            continue
        subj_lower = subj_raw.lower()

        matched = subj_lower in names_lower or any(p.search(subj_raw) for p in patterns)
        if not matched:
            continue

        chunk_ids = claim.get("chunk_ids") or []
        evidence.append({
            "statement": claim.get("statement", ""),
            "subject": claim.get("subject", ""),
            "confidence": claim.get("confidence", "explicit"),
            "chunk_ids": [c for c in chunk_ids if isinstance(c, str) and c],
        })

    if evidence:
        return evidence

    # ---- Fallback: derive evidence from entity/concept chunk_ids. -------
    if not entity_by_name and not concept_by_term:
        return []

    fallback_chunk_ids: list[str] = []
    matched_names: list[str] = []
    for name, name_lc in zip(raw_names, names_lower):
        hit = None
        if entity_by_name:
            hit = entity_by_name.get(name_lc)
        if hit is None and concept_by_term:
            hit = concept_by_term.get(name_lc)
        if not hit:
            continue
        for cid in hit.get("chunk_ids") or []:
            if isinstance(cid, str) and cid and cid not in fallback_chunk_ids:
                fallback_chunk_ids.append(cid)
        matched_names.append(name)

    if not fallback_chunk_ids:
        return []

    # Marker ``_synthetic`` keeps this item out of the writer prompt — it
    # exists only to carry chunk_ids forward for provenance and source-context
    # fetching. _wiki_format_evidence_blocks filters it out.
    return [{
        "statement": "",
        "subject": matched_names[0] if matched_names else raw_names[0],
        "confidence": "inferred",
        "chunk_ids": fallback_chunk_ids,
        "_synthetic": True,
    }]


def _wiki_format_evidence_blocks(evidence: list[dict]) -> str:
    # Filter out synthetic stubs (entity-fallback chunk-id carriers) — they
    # don't represent real claims and shouldn't appear in the writer's
    # evidence checklist.
    real_evidence = [ev for ev in (evidence or []) if not ev.get("_synthetic")]
    if not real_evidence:
        return ("(no pre-extracted evidence — extract facts directly from "
                "the source document text above)")
    lines: list[str] = []
    for i, ev in enumerate(real_evidence, 1):
        confidence = (ev.get("confidence") or "explicit").upper()
        subject = ev.get("subject") or ""
        statement = ev.get("statement") or ""
        lines.append(f"{i}. [{confidence}] {subject}\n   {statement}")
    return "\n\n".join(lines)


def _wiki_collect_evidence_chunk_ids(evidence: list[dict]) -> list[str]:
    seen: list[str] = []
    for ev in evidence:
        for cid in ev.get("chunk_ids") or []:
            if isinstance(cid, str) and cid and cid not in seen:
                seen.append(cid)
    return seen


async def _wiki_load_chunks_by_id(
    chunk_ids: list[str], tenant_id: str, kb_id: str,
) -> dict[str, str]:
    """Batch-fetch chunks from ES by id. Returns {chunk_id: content_with_weight}."""
    if not chunk_ids:
        return {}
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr
    from rag.nlp import search as _rag_search

    index = _rag_search.index_name(tenant_id)
    select_fields = ["id", "content_with_weight"]
    out: dict[str, str] = {}

    BATCH = 500
    for i in range(0, len(chunk_ids), BATCH):
        batch_ids = chunk_ids[i:i + BATCH]
        condition = {"id": batch_ids}
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                select_fields, [], condition, [], OrderByExpr(),
                0, len(batch_ids), index, [kb_id],
            )
            field_map = settings.docStoreConn.get_fields(res, select_fields)
        except Exception:
            logging.exception("wiki_refine: failed to fetch %d chunks", len(batch_ids))
            continue
        for cid, row in field_map.items():
            content = row.get("content_with_weight")
            if isinstance(content, str) and content:
                out[cid] = content
    return out


async def _wiki_build_source_context(
    evidence: list[dict],
    tenant_id: str,
    kb_id: str,
    budget: int = WIKI_REFINE_SOURCE_BUDGET_CHARS,
) -> str:
    """Concatenate evidence chunks into a labelled source-context block.

    Budget is char-based. Evidence chunks come first (preserve their order of
    appearance in the evidence list); if total exceeds budget the tail is
    truncated with a marker.
    """
    chunk_ids = _wiki_collect_evidence_chunk_ids(evidence)
    if not chunk_ids:
        return "(no source chunks available)"

    chunk_map = await _wiki_load_chunks_by_id(chunk_ids, tenant_id, kb_id)
    if not chunk_map:
        return "(source chunks could not be loaded)"

    parts: list[str] = []
    total = 0
    truncated = 0
    for cid in chunk_ids:
        content = chunk_map.get(cid)
        if not content:
            continue
        block = f"[CHUNK {cid}]\n{content}"
        if total + len(block) + 2 > budget:
            remaining = budget - total
            if remaining > 1000:
                parts.append(block[:remaining] + "\n\n[…chunk truncated…]")
                total += remaining
            truncated += 1
            continue
        parts.append(block)
        total += len(block) + 2

    if truncated:
        parts.append(f"\n\n[…{truncated} chunk(s) omitted to fit context budget…]")

    return "\n\n".join(parts)


# --- wikilink rewriting and doc-id collection ------------------------------

_WIKILINK_PIPE_RE = re.compile(r"\[\[([^\[\]\|]+?)\|([^\[\]]+?)\]\]")
_WIKILINK_SIMPLE_RE = re.compile(r"\[\[([^\[\]\|]+?)\]\]")


def _wiki_transform_links(content_md: str, kb_id: str) -> tuple[str, list[str]]:
    """Rewrite ``[[slug]]`` / ``[[slug|display]]`` wikilinks to standard
    markdown links whose href encodes ``(kb_id, slug)`` so a renderer can
    fetch the target page from ES.

    Returns ``(rewritten_md, unique_outlinks)`` — outlinks are slug strings
    in first-seen order. The href format is ``wiki/{kb_id}/{slug}`` which is
    relative; clients are expected to map this to whatever route serves the
    page (e.g. ``/api/v1/wiki/{kb_id}/{slug}``).
    """
    kb_id_str = str(kb_id)
    seen: set[str] = set()
    outlinks: list[str] = []

    def _track(slug: str) -> None:
        s = slug.strip()
        if s and s not in seen:
            seen.add(s)
            outlinks.append(s)

    def _piped(m: re.Match) -> str:
        slug = m.group(1).strip()
        text = m.group(2).strip()
        _track(slug)
        return f"[{text}](wiki/{kb_id_str}/{slug})"

    def _simple(m: re.Match) -> str:
        slug = m.group(1).strip()
        _track(slug)
        return f"[{slug}](wiki/{kb_id_str}/{slug})"

    rewritten = _WIKILINK_PIPE_RE.sub(_piped, content_md or "")
    rewritten = _WIKILINK_SIMPLE_RE.sub(_simple, rewritten)
    return rewritten, outlinks


async def _wiki_collect_doc_ids(
    chunk_ids: list[str], tenant_id: str, kb_id: str,
) -> list[str]:
    """Look up ``doc_id`` for each chunk by id. Returns the unique list in
    first-seen order (subset of the source chunks' parents).

    Defensive: handles both string and list shapes of the ``doc_id`` field
    (different doc-store connectors normalize scalar keyword fields
    differently). Logs when nothing comes back so the empty-source_doc_ids
    failure mode is diagnosable.
    """
    if not chunk_ids:
        return []
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr
    from rag.nlp import search as _rag_search

    index = _rag_search.index_name(tenant_id)
    select_fields = ["id", "doc_id"]
    out: list[str] = []
    seen: set[str] = set()
    total_rows_seen = 0

    def _accept(did) -> None:
        if isinstance(did, str):
            if did and did not in seen:
                seen.add(did)
                out.append(did)
        elif isinstance(did, (list, tuple)):
            for d in did:
                if isinstance(d, str) and d and d not in seen:
                    seen.add(d)
                    out.append(d)

    BATCH = 500
    for i in range(0, len(chunk_ids), BATCH):
        batch_ids = chunk_ids[i:i + BATCH]
        condition = {"id": batch_ids}
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                select_fields, [], condition, [], OrderByExpr(),
                0, len(batch_ids), index, [kb_id],
            )
            field_map = settings.docStoreConn.get_fields(res, select_fields)
        except Exception:
            logging.exception("wiki_refine: failed to fetch doc_ids for %d chunks", len(batch_ids))
            continue
        total_rows_seen += len(field_map)
        for row in field_map.values():
            _accept(row.get("doc_id"))

    if chunk_ids and not out:
        logging.warning(
            "wiki_refine: doc_id resolution returned 0 for %d chunk(s) (rows_found=%d, kb=%s); "
            "first chunk_id=%s",
            len(chunk_ids), total_rows_seen, kb_id, chunk_ids[0],
        )
    return out


async def _wiki_get_existing_page(
    slug: str, tenant_id: str, kb_id: str,
) -> Optional[dict]:
    """Fetch a wiki_page row by slug from this KB. Returns ``{id, content_md,
    content_md_raw, title, page_type}`` or None. ``content_md_raw`` is the
    pre-link-transform markdown — what the merger should consume."""
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr
    from rag.nlp import search as _rag_search

    index = _rag_search.index_name(tenant_id)
    condition = {
        "compile_kwd": [WIKI_PAGE_COMPILE_KWD],
        "wiki_slug_kwd": [slug],
    }
    select_fields = [
        "id", "content_with_weight", "wiki_raw_md_kwd",
        "wiki_title_kwd", "wiki_page_type_kwd",
    ]
    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            select_fields, [], condition, [], OrderByExpr(),
            0, 1, index, [kb_id],
        )
        field_map = settings.docStoreConn.get_fields(res, select_fields)
    except Exception:
        logging.exception("wiki_refine: failed to fetch existing page for slug=%s", slug)
        return None
    if not field_map:
        return None
    row_id, row = next(iter(field_map.items()))
    rendered = row.get("content_with_weight") or ""
    raw = row.get("wiki_raw_md_kwd") or rendered  # fall back to rendered if no raw stored
    return {
        "id": row_id,
        "content_md": rendered,
        "content_md_raw": raw,
        "title": row.get("wiki_title_kwd") or "",
        "page_type": row.get("wiki_page_type_kwd") or "concept",
    }


async def _wiki_chat_text(
    chat_mdl, system_prompt: str, user_prompt: str,
    temperature: float, timeout: int,
) -> str:
    """Single chat call returning the raw text. Trims to chat_mdl.max_length
    via message_fit_in and strips a leading </think> block."""
    msg = [
        {"role": "system", "content": system_prompt},
        {"role": "user", "content": user_prompt},
    ]
    try:
        _, msg = message_fit_in(msg, chat_mdl.max_length)
    except Exception:
        logging.exception("wiki_refine: message_fit_in failed; sending untrimmed")
    try:
        raw = await asyncio.wait_for(
            chat_mdl.async_chat(msg[0]["content"], msg[1:], {"temperature": temperature}),
            timeout=timeout,
        )
    except asyncio.TimeoutError:
        logging.warning("wiki_refine: chat call timed out after %ds", timeout)
        return ""
    except Exception:
        logging.exception("wiki_refine: chat call failed")
        return ""
    if isinstance(raw, tuple):
        raw = raw[0]
    return _wiki_strip_think(raw or "")


async def _wiki_write_page_simple(
    plan_item: dict,
    evidence: list[dict],
    existing_md: Optional[str],
    source_context: str,
    all_plan_slugs: list[str],
    chat_mdl,
    timeout: int,
) -> str:
    """Single LLM call → markdown content."""
    own_slug = plan_item.get("slug") or ""
    available = [s for s in all_plan_slugs if s and s != own_slug]
    slugs_block = (
        "\n".join(f"- [[{s}]]" for s in available) if available
        else "(none — this is the only page)"
    )

    if existing_md:
        existing_section = (
            "## Existing page content (UPDATE — integrate new evidence into this)\n\n"
            f"{existing_md}\n"
        )
    else:
        existing_section = ""

    user_prompt = WIKI_REFINE_WRITER_USER_TEMPLATE.format(
        action=plan_item.get("action", "CREATE"),
        slug=own_slug,
        title=plan_item.get("title", own_slug),
        page_type=plan_item.get("page_type", "concept"),
        all_plan_slugs=slugs_block,
        existing_section=existing_section,
        source_context=source_context,
        evidence_count=len(evidence),
        evidence_blocks=_wiki_format_evidence_blocks(evidence),
    )

    return await _wiki_chat_text(
        chat_mdl, WIKI_REFINE_WRITER_SYSTEM, user_prompt,
        temperature=0.15, timeout=timeout,
    )


async def _wiki_merge_page_content(
    existing_md: str, new_md: str, slug: str, chat_mdl,
    shrink_threshold: float = WIKI_MERGE_BODY_SHRINK_THRESHOLD,
    timeout: int = WIKI_MERGE_TIMEOUT,
) -> str:
    """LLM-merge existing vs new. Falls back to ``new_md`` on shrink-check
    failure or LLM error."""
    if not existing_md or len(existing_md.strip()) < 50:
        return new_md
    if existing_md.strip() == (new_md or "").strip():
        return new_md
    if not new_md:
        return existing_md

    user_prompt = (
        f"Merge these two versions of wiki page `{slug}`:\n\n"
        f"## EXISTING VERSION\n\n{existing_md}\n\n"
        "---\n\n"
        f"## INCOMING VERSION\n\n{new_md}\n\n"
        "---\n\n"
        "Produce the merged page now. Return ONLY the markdown content."
    )
    merged = await _wiki_chat_text(
        chat_mdl, WIKI_REFINE_MERGE_SYSTEM, user_prompt,
        temperature=0.1, timeout=timeout,
    )
    if not merged:
        return new_md

    max_input_len = max(len(existing_md), len(new_md))
    min_acceptable = int(max_input_len * shrink_threshold)
    if len(merged) < min_acceptable:
        logging.warning(
            "wiki_refine: merge rejected for slug=%s (merged=%d chars < %d threshold; "
            "max input=%d). Falling back to new content.",
            slug, len(merged), min_acceptable, max_input_len,
        )
        return new_md
    return merged


def _wiki_extract_summary(content_md: str, max_chars: int = 300) -> str:
    """First non-heading paragraph of the markdown, capped at ``max_chars``."""
    if not isinstance(content_md, str) or not content_md.strip():
        return ""
    buf: list[str] = []
    for line in content_md.splitlines():
        s = line.strip()
        if not s or s.startswith("#"):
            if buf:
                break
            continue
        buf.append(s)
        if len(" ".join(buf)) >= max_chars:
            break
    return " ".join(buf)[:max_chars]


def _wiki_page_row_id(kb_id: str, slug: str) -> str:
    """Stable id for a wiki_page row, scoped to KB + slug."""
    return xxhash.xxh64(
        (WIKI_PAGE_COMPILE_KWD + ":" + str(kb_id) + ":" + str(slug)).encode("utf-8", "surrogatepass")
    ).hexdigest()


def _wiki_draft_row_id(kb_id: str, slug: str) -> str:
    return xxhash.xxh64(
        (WIKI_DRAFT_COMPILE_KWD + ":" + str(kb_id) + ":" + str(slug)).encode("utf-8", "surrogatepass")
    ).hexdigest()


async def _wiki_persist_page(
    page: dict, embd_mdl, tenant_id: str, kb_id: str,
) -> None:
    """Upsert one searchable wiki_page row for ``page``.

    Storage layout (the fields needed to fetch a page by clicking a link plus
    the metadata a UI needs to show provenance):

    - ``content_with_weight`` — rendered markdown with ``[[slug]]`` rewritten
      to clickable links ``[text](wiki/{kb_id}/{slug})``. This is the
      front-end-displayable form.
    - ``wiki_raw_md_kwd`` — pre-transform markdown with the original
      ``[[slug]]`` wikilinks; used by the merger and any LLM-facing re-read.
    - ``wiki_slug_kwd`` / ``wiki_title_kwd`` / ``wiki_page_type_kwd`` —
      lookup + display fields.
    - ``wiki_kb_id_kwd`` — explicit KB identifier (the ``doc_id`` field is a
      sentinel and shouldn't be relied on for filtering).
    - ``wiki_doc_id_kwd`` — list of source document ids that contributed to
      this page (derived from the source chunks).
    - ``wiki_outlinks_kwd`` — slugs this page links to.
    - ``source_id`` — list of source chunk ids.
    - ``q_<dim>_vec`` — embedding of ``title + summary + content``.
    - ``available_int=1`` so retrievers surface the page.
    """
    from common import settings
    from rag.nlp import search as _rag_search

    slug = page.get("slug") or ""
    if not slug:
        return

    index = _rag_search.index_name(tenant_id)
    kb_id_str = str(kb_id)
    title = page.get("title") or slug
    page_type = page.get("page_type") or "concept"
    summary = page.get("summary") or ""

    # Prefer the rendered form if the caller already transformed it (e.g. on
    # resume from draft cache). Otherwise transform now from the raw form.
    content_md_raw = page.get("content_md_raw") or page.get("content_md") or ""
    if page.get("content_md_rendered"):
        content_md_rendered = page["content_md_rendered"]
        outlinks = list(page.get("outlinks") or [])
    else:
        content_md_rendered, outlinks = _wiki_transform_links(content_md_raw, kb_id_str)
        page["content_md_rendered"] = content_md_rendered
        page["outlinks"] = outlinks

    source_chunk_ids = list(page.get("source_chunk_ids") or [])
    source_doc_ids = page.get("source_doc_ids")
    if source_doc_ids is None:
        source_doc_ids = await _wiki_collect_doc_ids(source_chunk_ids, tenant_id, kb_id)
        page["source_doc_ids"] = source_doc_ids

    embed_text = (f"{title}\n\n{summary}\n\n{content_md_rendered}")[:8000]
    try:
        embeddings, _ = await thread_pool_exec(embd_mdl.encode, [embed_text])
    except Exception:
        logging.exception("wiki_refine: embedding failed for slug=%s; skipping persist", slug)
        return
    # ``embeddings`` is typically a numpy.ndarray of shape (1, D); a bare
    # ``if not embeddings`` would trip the array-truth-value error, so check
    # length explicitly.
    try:
        n_embeddings = len(embeddings) if embeddings is not None else 0
    except TypeError:
        n_embeddings = 0
    if n_embeddings == 0:
        logging.warning("wiki_refine: empty embedding for slug=%s; skipping persist", slug)
        return
    vec = embeddings[0]
    vec_list = vec.tolist() if hasattr(vec, "tolist") else list(vec)
    if not vec_list:
        logging.warning("wiki_refine: zero-length embedding for slug=%s; skipping persist", slug)
        return

    content_ltks = rag_tokenizer.tokenize(content_md_rendered) if content_md_rendered else ""
    content_sm_ltks = rag_tokenizer.fine_grained_tokenize(content_ltks) if content_ltks else ""

    row = {
        "id": _wiki_page_row_id(kb_id_str, slug),
        "doc_id": kb_id_str,                     # sentinel; use wiki_kb_id_kwd for filtering
        "compile_kwd": WIKI_PAGE_COMPILE_KWD,
        "wiki_slug_kwd": slug,
        "wiki_title_kwd": title,
        "wiki_page_type_kwd": page_type,
        "wiki_kb_id_kwd": kb_id_str,
        "wiki_doc_id_kwd": list(source_doc_ids),
        "wiki_outlinks_kwd": list(outlinks),
        "wiki_raw_md_kwd": content_md_raw,
        "source_id": source_chunk_ids,
        "content_with_weight": content_md_rendered,
        "content_ltks": content_ltks,
        "content_sm_ltks": content_sm_ltks,
        f"q_{len(vec_list)}_vec": vec_list,
        "available_int": 1,
    }
    try:
        try:
            await thread_pool_exec(
                settings.docStoreConn.delete,
                {"compile_kwd": WIKI_PAGE_COMPILE_KWD, "wiki_slug_kwd": slug},
                index, kb_id,
            )
        except Exception:
            logging.debug("wiki_refine: prior wiki_page delete failed; relying on id upsert")
        await thread_pool_exec(settings.docStoreConn.insert, [row], index, kb_id)
    except Exception:
        logging.exception("wiki_refine: failed to persist wiki_page slug=%s", slug)


async def _wiki_persist_draft(page: dict, tenant_id: str, kb_id: str) -> None:
    """Upsert one non-searchable wiki_page_draft row (resume cache)."""
    from common import settings
    from rag.nlp import search as _rag_search

    slug = page.get("slug") or ""
    if not slug:
        return
    index = _rag_search.index_name(tenant_id)
    content_with_weight = json.dumps(page, ensure_ascii=False)
    row = {
        "id": _wiki_draft_row_id(kb_id, slug),
        "doc_id": str(kb_id),
        "compile_kwd": WIKI_DRAFT_COMPILE_KWD,
        "wiki_slug_kwd": slug,
        "source_id": [str(kb_id)],
        "content_with_weight": content_with_weight,
        "available_int": 0,           # non-searchable
    }
    try:
        try:
            await thread_pool_exec(
                settings.docStoreConn.delete,
                {"compile_kwd": WIKI_DRAFT_COMPILE_KWD, "wiki_slug_kwd": slug},
                index, kb_id,
            )
        except Exception:
            logging.debug("wiki_refine: prior draft delete failed; relying on id upsert")
        await thread_pool_exec(settings.docStoreConn.insert, [row], index, kb_id)
    except Exception:
        logging.exception("wiki_refine: failed to persist draft slug=%s", slug)


async def _wiki_load_refine_resume(tenant_id: str, kb_id: str) -> dict[str, dict]:
    """Load all cached wiki_page_draft rows for this KB. Returns {slug: page}."""
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr
    from rag.nlp import search as _rag_search

    index = _rag_search.index_name(tenant_id)
    condition = {"compile_kwd": [WIKI_DRAFT_COMPILE_KWD]}
    select_fields = ["id", "wiki_slug_kwd", "content_with_weight"]

    PAGE_SIZE = 500
    offset = 0
    out: dict[str, dict] = {}
    while True:
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                select_fields, [], condition, [], OrderByExpr(),
                offset, PAGE_SIZE, index, [kb_id],
            )
            field_map = settings.docStoreConn.get_fields(res, select_fields)
        except Exception:
            logging.exception("wiki_refine: failed to page draft cache")
            break
        if not field_map:
            break
        for row in field_map.values():
            slug = row.get("wiki_slug_kwd")
            content = row.get("content_with_weight")
            if not isinstance(slug, str) or not isinstance(content, str):
                continue
            try:
                cached = json.loads(content)
            except Exception:
                continue
            if isinstance(cached, dict):
                out[slug] = cached
        if len(field_map) < PAGE_SIZE:
            break
        offset += PAGE_SIZE
    return out


# --- public entry ---------------------------------------------------------


async def wiki_refine_from_plan(
    chat_mdl,
    embd_mdl,
    tenant_id: str,
    kb_id: str,
    max_workers: int = DEFAULT_WIKI_REFINE_WORKERS,
    timeout: int = DEFAULT_WIKI_REFINE_TIMEOUT,
    source_budget_chars: int = WIKI_REFINE_SOURCE_BUDGET_CHARS,
    merge_shrink_threshold: float = WIKI_MERGE_BODY_SHRINK_THRESHOLD,
    force_rerun: bool = False,
    callback: Optional[Callable] = None,
) -> list[dict]:
    """Phase 4 (REFINE) — KB-scoped.

    Reads the cached ``wiki_compilation_plan`` for this KB and writes one
    wiki page per planned entry. Writers run in parallel under
    ``asyncio.Semaphore(max_workers)``. UPDATE pages are LLM-merged against
    their existing content (sanity-checked at ``merge_shrink_threshold``).
    Each finished page is persisted as a searchable ``wiki_page`` row in ES,
    plus a non-searchable ``wiki_page_draft`` row for resume.

    Args:
        chat_mdl, embd_mdl: ragflow LLMBundle instances.
        tenant_id, kb_id: address the doc-store index.
        max_workers: max concurrent writers (default 4).
        timeout: seconds per writer LLM call (default 300).
        source_budget_chars: max chars of source-chunk context per writer call.
        merge_shrink_threshold: a merged body shorter than this fraction of
            the longest input falls back to the new content.
        force_rerun: ignore the wiki_page_draft cache and re-write everything.
        callback: optional ``(progress: float, msg: str)`` callback.

    Returns the list of page dicts (one per planned entry). Each page dict
    has ``slug, title, page_type, action, content_md, summary,
    entity_names, related_kb_pages, source_chunk_ids``.
    """
    # ---- Defensive embd_mdl unwrap ---------------------------------------
    # Some callers accidentally pass the result of LLMBundle.encode() — a
    # ``(embeddings, used_tokens)`` tuple — instead of the LLMBundle itself.
    # Earlier phases (REDUCE, PLAN) often hit their resume cache so this
    # surfaces here for the first time. Try a safe unwrap before bailing.
    if not hasattr(embd_mdl, "encode"):
        if isinstance(embd_mdl, tuple) and embd_mdl and hasattr(embd_mdl[0], "encode"):
            logging.warning(
                "wiki_refine: embd_mdl arrived as a %s; unwrapping to first element "
                "(check the call site — was encode()'s return value passed instead "
                "of the LLMBundle?)",
                type(embd_mdl).__name__,
            )
            embd_mdl = embd_mdl[0]
        else:
            logging.error(
                "wiki_refine: embd_mdl has no .encode method (type=%s); aborting REFINE",
                type(embd_mdl).__name__,
            )
            return []
    if not hasattr(chat_mdl, "async_chat"):
        logging.error(
            "wiki_refine: chat_mdl has no .async_chat method (type=%s); aborting REFINE",
            type(chat_mdl).__name__,
        )
        return []

    if callback:
        try:
            callback(0.02, "wiki REFINE: loading plan")
        except Exception:
            pass

    plan = await _wiki_load_plan_resume(tenant_id, kb_id)
    if plan is None or not isinstance(plan, dict):
        logging.warning("wiki_refine: no wiki_compilation_plan found for kb=%s", kb_id)
        return []

    pages_spec = plan.get("pages") or []
    if not pages_spec:
        logging.info("wiki_refine: plan has no pages for kb=%s", kb_id)
        return []
    # Sort by priority then dedupe by slug, keeping the first (highest-priority)
    # entry. The planning LLM sometimes emits the same slug multiple times,
    # which both wastes writer calls and bloats every prompt's "Available
    # pages" list with duplicates.
    sorted_spec = sorted(
        [p for p in pages_spec if isinstance(p, dict) and p.get("slug")],
        key=lambda p: p.get("priority", 99),
    )
    seen_slugs: set[str] = set()
    pages_spec = []
    duplicates_dropped = 0
    for p in sorted_spec:
        s = p.get("slug")
        if not s:
            continue
        if s in seen_slugs:
            duplicates_dropped += 1
            continue
        seen_slugs.add(s)
        pages_spec.append(p)
    if duplicates_dropped:
        logging.info(
            "wiki_refine: dropped %d duplicate slug entr(ies) from plan for kb=%s",
            duplicates_dropped, kb_id,
        )

    all_claims = plan.get("_claims") or []
    # ``all_plan_slugs`` is implicitly deduped now (pages_spec is unique).
    all_plan_slugs = [p["slug"] for p in pages_spec]

    # Build canonical entity/concept lookups for evidence fallback. When MAP
    # produced no claims (a real failure mode we've seen on Chinese / dense
    # technical content), provenance still resolves via the chunk_ids on
    # the entities and concepts themselves. The lookups index every name
    # variant (canonical + aliases) so the planner LLM picking an alias
    # spelling still hits the right canonical record.
    entity_by_name: dict[str, dict] = {}
    for e in plan.get("_entities") or []:
        if not isinstance(e, dict):
            continue
        canon = (e.get("name") or "").strip()
        if canon:
            entity_by_name.setdefault(canon.lower(), e)
        for alias in e.get("aliases") or []:
            if isinstance(alias, str) and alias.strip():
                entity_by_name.setdefault(alias.strip().lower(), e)

    concept_by_term: dict[str, dict] = {}
    for c in plan.get("_concepts") or []:
        if not isinstance(c, dict):
            continue
        term = (c.get("term") or "").strip()
        if term:
            concept_by_term.setdefault(term.lower(), c)
        # Concepts in REDUCE output rarely carry aliases, but accept them if
        # present so a future MAP schema change is forward-compatible.
        for alias in c.get("aliases") or []:
            if isinstance(alias, str) and alias.strip():
                concept_by_term.setdefault(alias.strip().lower(), c)

    # Resume cache
    cached: dict[str, dict] = {}
    if not force_rerun:
        cached = await _wiki_load_refine_resume(tenant_id, kb_id)
        if cached:
            logging.info("wiki_refine: resume — %d cached page(s) for kb=%s", len(cached), kb_id)

    pending = [p for p in pages_spec if p.get("slug") not in cached]
    total = max(1, len(pending))

    if callback:
        try:
            callback(0.1, f"wiki REFINE: writing {len(pending)} page(s) (cached={len(cached)})")
        except Exception:
            pass

    semaphore = asyncio.Semaphore(max_workers) if max_workers and max_workers > 0 else None
    completed = 0
    completed_lock = asyncio.Lock()

    async def _write_one(plan_item: dict) -> Optional[dict]:
        nonlocal completed
        slug = plan_item.get("slug") or ""
        action = (plan_item.get("action") or "CREATE").upper()
        title = plan_item.get("title") or slug
        page_type = plan_item.get("page_type") or "concept"

        async def _run() -> Optional[dict]:
            nonlocal completed
            try:
                evidence = _wiki_assemble_evidence(
                    plan_item, all_claims,
                    entity_by_name=entity_by_name,
                    concept_by_term=concept_by_term,
                )
                source_chunk_ids = _wiki_collect_evidence_chunk_ids(evidence)
                source_context = await _wiki_build_source_context(
                    evidence, tenant_id, kb_id, budget=source_budget_chars,
                )

                # Use the raw [[slug]] form for the writer and merger so the
                # LLM sees a stable, well-known wikilink notation; we render
                # to clickable links once at persist time.
                existing_md_raw: Optional[str] = None
                if action == "UPDATE":
                    existing = await _wiki_get_existing_page(slug, tenant_id, kb_id)
                    if existing:
                        existing_md_raw = existing.get("content_md_raw") or existing.get("content_md")

                content_md_raw = await _wiki_write_page_simple(
                    plan_item, evidence, existing_md_raw, source_context,
                    all_plan_slugs, chat_mdl, timeout,
                )
                if not content_md_raw:
                    content_md_raw = f"# {title}\n\n(Page generation produced no content.)"

                if existing_md_raw:
                    content_md_raw = await _wiki_merge_page_content(
                        existing_md_raw, content_md_raw, slug, chat_mdl,
                        shrink_threshold=merge_shrink_threshold,
                    )

                # Render wikilinks once, here, after all LLM transforms.
                content_md_rendered, outlinks = _wiki_transform_links(content_md_raw, kb_id)
                source_doc_ids = await _wiki_collect_doc_ids(source_chunk_ids, tenant_id, kb_id)
                summary = _wiki_extract_summary(content_md_rendered) or title

                page = {
                    "slug": slug,
                    "title": title,
                    "page_type": page_type,
                    "action": action,
                    # Rendered content (with clickable wiki/{kb_id}/{slug} links) is
                    # what callers and the UI consume; the raw [[slug]] form is
                    # preserved for LLM-facing re-reads and the merger.
                    "content_md": content_md_rendered,
                    "content_md_rendered": content_md_rendered,
                    "content_md_raw": content_md_raw,
                    "outlinks": outlinks,
                    "summary": summary,
                    "entity_names": plan_item.get("entity_names") or [],
                    "related_kb_pages": plan_item.get("related_kb_pages") or [],
                    "source_chunk_ids": source_chunk_ids,
                    "source_doc_ids": source_doc_ids,
                    "kb_id": str(kb_id),
                }
            except Exception:
                logging.exception("wiki_refine: writer failed for slug=%s", slug)
                return None

            try:
                await _wiki_persist_page(page, embd_mdl, tenant_id, kb_id)
            except Exception:
                logging.exception("wiki_refine: persist_page failed for slug=%s", slug)
            try:
                await _wiki_persist_draft(page, tenant_id, kb_id)
            except Exception:
                logging.exception("wiki_refine: persist_draft failed for slug=%s", slug)

            if callback:
                async with completed_lock:
                    completed += 1
                    done = completed
                progress = 0.1 + 0.85 * (done / total)
                try:
                    callback(progress, f"wiki REFINE: {done}/{total} pages written ({slug})")
                except Exception:
                    pass
            return page

        if semaphore is not None:
            async with semaphore:
                return await _run()
        return await _run()

    tasks = [asyncio.create_task(_write_one(p)) for p in pending]
    if tasks:
        try:
            new_pages = await asyncio.gather(*tasks, return_exceptions=False)
        except Exception:
            for t in tasks:
                t.cancel()
            await asyncio.gather(*tasks, return_exceptions=True)
            raise
    else:
        new_pages = []

    results: list[dict] = []
    # Cached pages first (in plan order), then freshly written ones.
    for p in pages_spec:
        slug = p.get("slug")
        if not slug:
            continue
        if slug in cached:
            results.append(cached[slug])
        else:
            # Look up the freshly produced page (None on writer failure).
            for np in new_pages:
                if np and np.get("slug") == slug:
                    results.append(np)
                    break

    logging.info(
        "wiki_refine: kb=%s done — pages written=%d (cached=%d new=%d)",
        kb_id, len(results), len(cached),
        sum(1 for p in new_pages if p),
    )

    if callback:
        try:
            callback(1.0, "wiki REFINE: done")
        except Exception:
            pass

    return results


__all__ = [
    "WIKI_MAP_COMPILE_KWD",
    "WIKI_REDUCE_COMPILE_KWD",
    "WIKI_PLAN_COMPILE_KWD",
    "WIKI_PAGE_COMPILE_KWD",
    "WIKI_DRAFT_COMPILE_KWD",
    "wiki_map_from_chunks",
    "wiki_reduce_from_extracts",
    "wiki_plan_from_reduction",
    "wiki_refine_from_plan",
]
