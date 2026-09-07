"""Dual-mode wiki incremental compilation.

Entity mode:
  MAP → REDUCE → REFINE per-concept (generate/modify/re-synthesize) → FINALIZE
  1 concept = 1 page (WeKnora style). Entities enrich concept pages via source chunks.

Topic mode:
  MAP → REDUCE → PLAN (LLM grouping) → REFINE per-page → FINALIZE
  Incremental: embeddings retrieve page candidates; the LLM makes final routes.

Both modes share MAP + REDUCE + FINALIZE.
"""

from __future__ import annotations

import asyncio
import json
import logging
import re
from typing import Callable

import numpy as np

from common import settings
from common.doc_store.doc_store_base import MatchDenseExpr, OrderByExpr
from common.misc_utils import thread_pool_exec
from rag.prompts.generator import message_fit_in
from rag.nlp import search

from ._common import (
    knowledge_compile_gen_conf as _knowledge_compile_gen_conf,
    stable_row_id as _stable_row_id,
)


# ----- REFINE progress -----

WIKI_REFINE_PROGRESS_UPDATES = 20


async def _wiki_run_refine_tasks(tasks: list, progress: Callable[[str], None]) -> None:
    """Run REFINE page tasks and emit at most roughly 20 progress updates."""
    total = len(tasks)
    if not total:
        return
    report_every = max(1, (total + WIKI_REFINE_PROGRESS_UPDATES - 1) // WIKI_REFINE_PROGRESS_UPDATES)
    scheduled = [asyncio.create_task(task) for task in tasks]
    try:
        for done, task in enumerate(asyncio.as_completed(scheduled), start=1):
            await task
            if done % report_every == 0 or done == total:
                progress(f"{done}/{total} pages completed.")
    except BaseException:
        for task in scheduled:
            if not task.done():
                task.cancel()
        await asyncio.gather(*scheduled, return_exceptions=True)
        raise


# ----- constants ----

# compile_kwd values
WIKI_PAGE_COMPILE_KWD = "wiki_page"
WIKI_PLAN_GROUP_COMPILE_KWD = "wiki_plan_group"
WIKI_DOC_PAGE_SOURCE_COMPILE_KWD = "wiki_doc_page_source"
WIKI_CANONICAL_ENTITY_COMPILE_KWD = "wiki_canonical_entity"
WIKI_REFINE_FAILURE_COMPILE_KWD = "wiki_refine_failure"

# Entity matching thresholds (not exposed in YAML)
ENTITY_MERGE_THRESHOLD = 0.90  # auto-merge
ENTITY_AMBIGUOUS_LOW = 0.75  # LLM confirm boundary
ENTITY_PAIRWISE_BLOCK_SIZE = 1024  # blockwise embedding matrix block size

# Number of concurrent KNN queries for entity matching
ENTITY_MATCH_KNN_CONCURRENT = 20
CANONICAL_PERSIST_CONCURRENT = 20
PAGE_ROUTER_KNN_CONCURRENT = 20
WIKI_GROUP_LLM_MAX_CONCURRENT = 8
WIKI_GROUP_LLM_CANDIDATE_SIZE = 24
WIKI_ROUTE_LLM_BATCH_SIZE = 12

WIKI_TOPIC_FALLBACK = "General"  # bucket for pages that match no topic
WIKI_PAGE_TOPIC_CANDIDATE_LIMIT = 50

# Page Router thresholds (kept as code constants — not exposed in YAML)
PAGE_ROUTER_MAYBE_THRESHOLD = 0.50
PAGE_ROUTER_TOP_K = 5
PAGE_ROUTER_MAX_CANDIDATES = 12
PAGE_CLUSTER_MIN_PAGES = 8
PAGE_CLUSTER_MAX_PAGES = 60
PAGE_CLUSTER_ITEMS_PER_PAGE = 3
PAGE_CLUSTER_HARD_MAX_SIZE = 8
PAGE_CLUSTER_MAX_ITERATIONS = 20
PAGE_CLUSTER_CONVERGENCE_EPSILON = 1e-4

# Re-synthesis triggers (both modes)
RE_SYNTHESIS_MIN_SOURCES = 5
RE_SYNTHESIS_GROWTH_RATIO = 1.5
RE_SYNTHESIS_MIN_CLAIMS = 15
RE_SYNTHESIS_MIN_VERSIONS = 3

# Evidence quality (WeKnora-style verbatim chunk sourcing).
# The page-writer sees the ACTUAL source-chunk text (not just the condensed
# claim statement) so pages stay fact-grounded and information-dense.
WIKI_SOURCE_BUDGET_CHARS = 32_768  # cap on verbatim chunk text fed to the writer
WIKI_SOURCE_BUDGET_RUNES = 12_000  # per-chunk-batch budget (rune-based, mirrors WeKnora)


# ----- helpers ---------------------------------------------------------------


def _wiki_log_stats(stage: str, event: str, **fields) -> None:
    """Emit machine-readable compilation statistics for one pipeline stage."""
    logging.info("wiki stats %s", json.dumps({"stage": stage, "event": event, **fields}, ensure_ascii=False, sort_keys=True))


def _wiki_derive_page_id(term: str, prefix: str = "concept") -> str:
    """Derive a URL-safe page identifier from a concept/entity name.

    Example: "Smartphone Industry" → "concept/smartphone-industry"
    """
    slug = re.sub(r"[^a-zA-Z0-9\u4e00-\u9fff]+", "-", term).strip("-").lower()
    return f"{prefix}/{slug}"


def _entity_to_query_text(entity: dict) -> str:
    parts = [entity.get("entity_name") or entity.get("name") or entity.get("term") or ""]
    aliases = entity.get("aliases") or []
    if isinstance(aliases, str):
        aliases = [aliases]
    parts.extend(str(alias) for alias in aliases[:5] if alias)
    description = entity.get("definition_excerpt") or entity.get("description") or entity.get("statement", "")
    if description:
        parts.append(str(description))
    for claim in (entity.get("claims") or [])[:3]:
        if not isinstance(claim, dict):
            continue
        statement = claim.get("statement") or claim.get("text")
        if statement:
            parts.append(str(statement))
    return " ".join(parts)


def _strip_think(text: str) -> str:
    if not isinstance(text, str):
        return ""
    text = text.strip()
    if text.startswith("</think>"):
        return text.split("</think>", 1)[-1].strip()
    return text


def _wiki_parse_json_array(text: str) -> list | None:
    """Extract one JSON array from an LLM response."""
    if not isinstance(text, str):
        return None
    start = text.find("[")
    end = text.rfind("]")
    if start < 0 or end < start:
        return None
    try:
        value = json.loads(text[start : end + 1])
    except (json.JSONDecodeError, TypeError):
        return None
    return value if isinstance(value, list) else None


async def _chat_mdl_ask(chat_mdl, system_prompt: str, user_prompt: str, temperature: float = 0.0) -> str:
    msg = [
        {"role": "system", "content": system_prompt},
        {"role": "user", "content": user_prompt},
    ]
    try:
        _, msg = message_fit_in(msg, chat_mdl.max_length)
    except Exception:
        logging.exception("wiki incremental: message_fit_in failed; sending untrimmed")
    request_conf = _knowledge_compile_gen_conf(chat_mdl, {"temperature": temperature})
    try:
        raw = await chat_mdl.async_chat(msg[0]["content"], msg[1:], request_conf)
    except Exception:
        raise
    if isinstance(raw, tuple):
        raw = raw[0]
    response = _strip_think(raw or "")
    if any(line.lstrip().startswith("**ERROR**") for line in response.splitlines()):
        raise RuntimeError(f"Wiki LLM call failed: {response}")
    return response


def _wiki_should_re_synthesize(
    page: dict,
    new_source_doc_ids: set[str],
    next_version: int,
) -> bool:
    existing_sources = set(page.get("source_doc_ids", []))
    total_sources = existing_sources | new_source_doc_ids
    claim_count = len(page.get("claims", []))
    last_synth_ver = _as_int(page.get("synthesis_version_int"), 1)
    versions_since = next_version - last_synth_ver

    return (
        len(total_sources) >= RE_SYNTHESIS_MIN_SOURCES
        and claim_count >= RE_SYNTHESIS_MIN_CLAIMS
        and versions_since >= RE_SYNTHESIS_MIN_VERSIONS
        and len(total_sources) >= len(existing_sources) * RE_SYNTHESIS_GROWTH_RATIO
    )


async def _wiki_load_chunk_texts(tenant_id: str, kb_id: str, chunk_ids: list[str]) -> dict[str, str]:
    """Fetch verbatim chunk text from the doc store by chunk id.

    Returns ``{chunk_id: content_with_weight}``. Used to ground page writing in
    the ACTUAL source text (WeKnora-style evidence) rather than the condensed
    claim statements. Mirrors ``wiki._wiki_load_chunks_by_id`` which lives in
    the old-mode module and is intentionally NOT imported here.
    """
    if not chunk_ids:
        return {}
    from common.doc_store.doc_store_base import OrderByExpr

    index = search.index_name(tenant_id)
    unique = [c for c in dict.fromkeys(chunk_ids) if isinstance(c, str) and c]
    if not unique:
        return {}
    out: dict[str, str] = {}
    BATCH = 500
    for i in range(0, len(unique), BATCH):
        batch = unique[i : i + BATCH]
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                ["id", "content_with_weight"],
                [],
                {"id": batch},
                [],
                OrderByExpr(),
                0,
                len(batch),
                index,
                [kb_id],
            )
            field_map = settings.docStoreConn.get_fields(res, ["id", "content_with_weight"]) or {}
        except Exception:
            logging.exception("wiki: batch chunk fetch failed (%d ids)", len(batch))
            field_map = {}
        for cid, row in field_map.items():
            content = row.get("content_with_weight")
            if isinstance(content, str) and content:
                out[cid] = content
    # Honor the rune budget: do not accumulate verbatim text we won't feed the
    # writer (mirrors WeKnora maxRunesPerCitationBatch=12000).
    total_runes = sum(len(v) for v in out.values())
    if total_runes > WIKI_SOURCE_BUDGET_RUNES:
        trimmed: dict[str, str] = {}
        budget = 0
        for cid, content in out.items():
            budget += len(content)
            if budget > WIKI_SOURCE_BUDGET_RUNES:
                break
            trimmed[cid] = content
        out = trimmed
    return out


def _wiki_enrich_source_chunks(source_chunks: list[dict], chunk_texts: dict[str, str]) -> list[dict]:
    """Merge verbatim chunk text into source_chunks.

    Each source_chunk entry is ``{"id": <chunk_id>, "text": <claim-original>}``.
    When the verbatim chunk content is available it REPLACES ``text`` (the
    writer should read the source, not the condensed claim); otherwise the
    claim text is kept as a fallback. Preserves original order and dedups by id.
    """
    enriched: list[dict] = []
    seen: set[str] = set()
    for sc in source_chunks:
        cid = sc.get("id") or sc.get("chunk_id")
        if not cid:
            continue
        cid = str(cid)
        if cid in seen:
            continue
        seen.add(cid)
        verbatim = chunk_texts.get(cid)
        enriched.append(
            {
                "id": cid,
                "text": verbatim if verbatim else sc.get("text", sc.get("content_with_weight", "")),
                "_verbatim": bool(verbatim),
            }
        )
    return enriched


# ----- Canonical Entity Index CRUD -----------------------------------------


async def _wiki_has_any_pages(tenant_id: str, kb_id: str) -> bool:
    """Return True if the KB has at least one compiled wiki_page row.

    Used to detect whether a build has ever succeeded — if no page exists but
    MAP rows do, the previous build was interrupted and should be restarted
    as a full build rather than treated as incremental.
    """
    index = search.index_name(tenant_id)
    try:
        if not settings.docStoreConn.index_exist(index, kb_id):
            return False
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            ["slug_kwd"],
            [],
            {"compile_kwd": [WIKI_PAGE_COMPILE_KWD]},
            [],
            OrderByExpr(),
            0,
            1,
            index,
            [kb_id],
        )
        return bool(settings.docStoreConn.get_fields(res, ["slug_kwd"]))
    except Exception:
        logging.exception("wiki: _wiki_has_any_pages failed for kb=%s", kb_id)
        return False


async def _wiki_load_refine_failures(tenant_id: str, kb_id: str) -> dict[str, dict]:
    """Load persisted REFINE failures keyed by page id."""
    index = search.index_name(tenant_id)
    if not settings.docStoreConn.index_exist(index, kb_id):
        return {}
    fields = ["slug_kwd", "content_with_weight"]
    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            fields,
            [],
            {"compile_kwd": [WIKI_REFINE_FAILURE_COMPILE_KWD]},
            [],
            OrderByExpr(),
            0,
            1000,
            index,
            [kb_id],
        )
        rows = settings.docStoreConn.get_fields(res, fields) or {}
    except Exception:
        logging.exception("wiki: failed to load REFINE failures for kb=%s", kb_id)
        return {}

    failures: dict[str, dict] = {}
    for row in rows.values():
        page_id = row.get("slug_kwd")
        payload = row.get("content_with_weight")
        if isinstance(page_id, list):
            page_id = page_id[0] if page_id else ""
        if not isinstance(page_id, str) or not page_id.strip():
            continue
        if isinstance(payload, str):
            try:
                payload = json.loads(payload) if payload else {}
            except (json.JSONDecodeError, TypeError):
                payload = {}
        failures[page_id.strip()] = payload if isinstance(payload, dict) else {}
    return failures


async def _wiki_record_refine_failure(
    tenant_id: str,
    kb_id: str,
    page_id: str,
    entity_names: list[str] | None = None,
    error: str = "",
) -> None:
    """Persist one failed page so a later Update can retry it."""
    page_id = str(page_id or "").strip()
    if not page_id:
        return
    index = search.index_name(tenant_id)
    payload = json.dumps(
        {
            "page_id": page_id,
            "entity_names": [str(name).strip() for name in entity_names or [] if str(name).strip()],
            "error": str(error or "")[:1000],
        },
        ensure_ascii=False,
    )
    row = {
        "id": _stable_row_id(WIKI_REFINE_FAILURE_COMPILE_KWD, kb_id, page_id),
        "doc_id": str(kb_id),
        "compile_kwd": WIKI_REFINE_FAILURE_COMPILE_KWD,
        "slug_kwd": page_id,
        "content_with_weight": payload,
        "available_int": 0,
    }
    try:
        await thread_pool_exec(
            settings.docStoreConn.delete,
            {"compile_kwd": [WIKI_REFINE_FAILURE_COMPILE_KWD], "slug_kwd": [page_id]},
            index,
            kb_id,
        )
        await thread_pool_exec(settings.docStoreConn.insert, [row], index, kb_id)
    except Exception:
        logging.exception("wiki: failed to persist REFINE failure page=%s", page_id)


async def _wiki_clear_refine_failure(tenant_id: str, kb_id: str, page_id: str) -> None:
    """Remove the retry marker after a page succeeds or is deleted."""
    page_id = str(page_id or "").strip()
    if not page_id:
        return
    try:
        await thread_pool_exec(
            settings.docStoreConn.delete,
            {"compile_kwd": [WIKI_REFINE_FAILURE_COMPILE_KWD], "slug_kwd": [page_id]},
            search.index_name(tenant_id),
            kb_id,
        )
    except Exception:
        logging.exception("wiki: failed to clear REFINE failure page=%s", page_id)


async def _load_canonical_entities(
    tenant_id: str,
    kb_id: str,
) -> dict[str, dict]:
    """Load all canonical entity rows. Returns {entity_name: row}."""
    index = search.index_name(tenant_id)
    if not settings.docStoreConn.index_exist(index, kb_id):
        return {}
    results: dict[str, dict] = {}
    offset = 0
    page_size = 1000
    while True:
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                ["entity_kwd", "entity_type_kwd", "aliases", "source_doc_ids", "source_chunk_ids", "mention_count_int"],
                [],
                {"compile_kwd": [WIKI_CANONICAL_ENTITY_COMPILE_KWD]},
                [],
                OrderByExpr(),
                offset,
                page_size,
                index,
                [kb_id],
            )
            field_map = (
                settings.docStoreConn.get_fields(
                    res,
                    ["entity_kwd", "entity_type_kwd", "aliases", "source_doc_ids", "source_chunk_ids", "mention_count_int"],
                )
                or {}
            )
        except Exception:
            logging.exception("wiki: failed to load canonical entities for kb=%s", kb_id)
            return results
        for row in field_map.values():
            name = row.get("entity_kwd", "")
            if isinstance(name, list):
                # entity_kwd is analyzed by whitespace-# (shared schema field),
                # so Infinity may return it as a token list. Rejoin with spaces
                # to reconstruct the canonical entity name.
                name = " ".join(str(t) for t in name if t)
            name = str(name or "").strip()
            if name:
                # Deserialize JSON fields
                for fld in ("aliases", "source_doc_ids", "source_chunk_ids"):
                    val = row.get(fld)
                    if isinstance(val, str):
                        try:
                            row[fld] = json.loads(val) if val else []
                        except (json.JSONDecodeError, TypeError):
                            row[fld] = []
                mc = row.get("mention_count_int", 0)
                if isinstance(mc, str):
                    mc = int(mc) if mc.isdigit() else 0
                row["mention_count_int"] = mc
                # entity_type_kwd is a *_kwd field → Infinity returns it as a
                # list (e.g. ['concept']). Normalize to a scalar so downstream
                # checks like `entity_type == "concept"` work correctly.
                et = row.get("entity_type_kwd")
                if isinstance(et, list):
                    et = et[0] if et else ""
                row["entity_type_kwd"] = str(et or "entity").strip()
                results[name] = row
        if len(field_map) < page_size:
            break
        offset += page_size
    return results


def _build_canonical_entity_doc(
    tenant_id: str,
    kb_id: str,
    entity_name: str,
    entity_type: str,
    aliases: list[str],
    source_doc_ids: list[str],
    claim_count: int,
    embedding: list[float] | None = None,
    source_chunk_ids: list[str] | None = None,
) -> dict:
    """Build a canonical entity row for insert or update."""
    dim = len(embedding) if embedding else 768
    doc = {
        "id": _stable_row_id(WIKI_CANONICAL_ENTITY_COMPILE_KWD, kb_id, entity_name),
        "entity_kwd": entity_name,
        "entity_type_kwd": entity_type,
        "aliases": json.dumps(list(set(aliases)), ensure_ascii=False),
        "source_doc_ids": sorted(set(source_doc_ids)),
        "source_chunk_ids": sorted(set(source_chunk_ids or [])),
        "mention_count_int": claim_count,
        "compile_kwd": WIKI_CANONICAL_ENTITY_COMPILE_KWD,
        "kb_id": kb_id,
    }
    if embedding is not None:
        vec_col = f"q_{dim}_vec"
        doc[vec_col] = embedding
    return doc


async def _save_canonical_entity(
    tenant_id: str,
    kb_id: str,
    entity_name: str,
    entity_type: str,
    aliases: list[str],
    source_doc_ids: list[str],
    claim_count: int,
    embedding: list[float] | None = None,
    source_chunk_ids: list[str] | None = None,
) -> None:
    """Insert or update a canonical entity row."""
    index = search.index_name(tenant_id)
    doc = _build_canonical_entity_doc(
        tenant_id,
        kb_id,
        entity_name,
        entity_type,
        aliases,
        source_doc_ids,
        claim_count,
        embedding,
        source_chunk_ids,
    )

    condition = {"compile_kwd": [WIKI_CANONICAL_ENTITY_COMPILE_KWD], "entity_kwd": [entity_name]}
    existing = await thread_pool_exec(
        settings.docStoreConn.search,
        ["entity_kwd"],
        [],
        condition,
        [],
        OrderByExpr(),
        0,
        1,
        index,
        [kb_id],
    )
    if settings.docStoreConn.get_fields(existing, ["entity_kwd"]):
        await thread_pool_exec(
            settings.docStoreConn.update,
            {"compile_kwd": [WIKI_CANONICAL_ENTITY_COMPILE_KWD], "entity_kwd": entity_name},
            doc,
            index,
            kb_id,
        )
    else:
        await thread_pool_exec(settings.docStoreConn.insert, [doc], index, kb_id)


async def _update_canonical_entity(
    tenant_id: str,
    kb_id: str,
    entity_name: str,
    entity_type: str,
    aliases: list[str],
    source_doc_ids: list[str],
    claim_count: int,
    source_chunk_ids: list[str] | None = None,
) -> None:
    """Update a known canonical row without an existence query."""
    index = search.index_name(tenant_id)
    doc = _build_canonical_entity_doc(
        tenant_id,
        kb_id,
        entity_name,
        entity_type,
        aliases,
        source_doc_ids,
        claim_count,
        source_chunk_ids=source_chunk_ids,
    )
    await thread_pool_exec(
        settings.docStoreConn.update,
        {"compile_kwd": [WIKI_CANONICAL_ENTITY_COMPILE_KWD], "entity_kwd": entity_name},
        doc,
        index,
        kb_id,
    )


async def _delete_canonical_entity(
    tenant_id: str,
    kb_id: str,
    entity_name: str,
) -> None:
    """Delete a canonical entity row."""
    index = search.index_name(tenant_id)
    await thread_pool_exec(
        settings.docStoreConn.delete,
        {"compile_kwd": [WIKI_CANONICAL_ENTITY_COMPILE_KWD], "entity_kwd": [entity_name]},
        index,
        kb_id,
    )


async def _knn_search_canonical(
    tenant_id: str,
    kb_id: str,
    embedding: list[float],
    threshold: float = ENTITY_MERGE_THRESHOLD,
) -> tuple[str, float] | None:
    """KNN search canonical entity index. Returns (entity_name, score) or None."""
    index = search.index_name(tenant_id)
    dim = len(embedding)
    match_expr = MatchDenseExpr(
        vector_column_name=f"q_{dim}_vec",
        embedding_data=embedding,
        embedding_data_type="float",
        distance_type="cosine",
        topn=1,
        extra_options={"similarity": threshold},
    )
    res = await thread_pool_exec(
        settings.docStoreConn.search,
        ["entity_kwd", "_score"],
        [],
        {"compile_kwd": [WIKI_CANONICAL_ENTITY_COMPILE_KWD]},
        [match_expr],
        OrderByExpr(),
        0,
        1,
        index,
        [kb_id],
    )
    field_map = settings.docStoreConn.get_fields(res, ["entity_kwd", "_score"])
    for row in field_map.values():
        name = row.get("entity_kwd", "")
        if isinstance(name, list):
            name = " ".join(str(t) for t in name if t)
        name = str(name or "").strip()
        score = row.get("_score", 0.0)
        if name and score >= threshold:
            return name, score
    return None


def _normalize_key(name: str) -> str:
    """Lowercase + strip whitespace + strip ASCII punctuation."""
    if not isinstance(name, str):
        return ""
    return re.sub(r"[^\w\s]", "", name.lower()).strip()


# ----- Entity Matching -----------------------------------------------------


def _extract_raw_entities(map_results: list[dict]) -> tuple[list[dict], dict[str, list[dict]]]:
    """Extract lightweight entity/concept metadata + a claim index from MAP.

    Returns a tuple:
      (entities, claim_index)
        entities:   list of LIGHTWEIGHT dicts {name, type, aliases, claim_count,
                    source_doc_ids, source_chunk_ids} — NO full claim text, so Entity Matching
                    operates on small metadata (mirrors old-mode dedup).
        claim_index: {name: [claim_dict, ...]} — full claim text kept separately,
                    loaded on-demand only for affected entities after matching.
    """
    raw: dict[str, dict] = {}
    claim_index: dict[str, list[dict]] = {}
    for mr in map_results:
        doc_id = mr.get("doc_id", "")

        # Process entities[]
        for ent in mr.get("entities") or []:
            if isinstance(ent, str):
                ent = json.loads(ent)
            name = ent.get("name", "")
            if not name:
                continue
            if name not in raw:
                raw[name] = {
                    "name": name,
                    "type": ent.get("type", "entity"),
                    "aliases": ent.get("aliases") or [],
                    "claim_count": 0,
                    "source_doc_ids": set(),
                    "source_chunk_ids": set(),
                }
            raw[name]["source_doc_ids"].add(doc_id)
            raw[name]["source_chunk_ids"].update(_wiki_claim_chunk_ids(ent))

        # Process concepts[]
        for concept in mr.get("concepts") or []:
            if isinstance(concept, str):
                concept = json.loads(concept)
            term = concept.get("term", "")
            if not term:
                continue
            if term not in raw:
                raw[term] = {
                    "name": term,
                    "type": "concept",
                    "aliases": [term],
                    "claim_count": 0,
                    "source_doc_ids": set(),
                    "source_chunk_ids": set(),
                }
            raw[term]["source_doc_ids"].add(doc_id)
            raw[term]["source_chunk_ids"].update(_wiki_claim_chunk_ids(concept))

        # Process claims, tracking count (metadata) but storing full text only
        # in claim_index (kept separate, loadable on demand).
        for claim in mr.get("claims") or []:
            if isinstance(claim, str):
                claim = json.loads(claim)
            subj = claim.get("entity_name") or claim.get("subject") or claim.get("term", "")
            if not subj:
                continue
            if subj in raw:
                raw[subj]["claim_count"] += 1
                raw[subj]["source_chunk_ids"].update(_wiki_claim_chunk_ids(claim))
                claim_index.setdefault(subj, []).append(claim)

        # A relation is grounded evidence for both endpoints even when MAP did
        # not emit a dedicated claim for either one.
        for relation in mr.get("relations") or []:
            if isinstance(relation, str):
                relation = json.loads(relation)
            relation_chunks = _wiki_claim_chunk_ids(relation)
            for endpoint in (relation.get("from"), relation.get("to")):
                if endpoint in raw:
                    raw[endpoint]["source_chunk_ids"].update(relation_chunks)

    result = []
    for entry in raw.values():
        entry["source_doc_ids"] = list(entry["source_doc_ids"])
        entry["source_chunk_ids"] = list(entry["source_chunk_ids"])
        result.append(entry)
    return result, claim_index


async def _wiki_match_entities(
    raw_entities: list[dict],
    existing_canonical: dict[str, dict],
    embd_mdl,
    chat_mdl,
    tenant_id: str,
    kb_id: str,
    incremental: bool,
    progress: Callable[[str], None] | None = None,
) -> tuple[dict[str, dict], dict[str, str]]:
    """Entity Matching: raw entities → canonical entities.

    Returns:
        canonical_map: {canonical_name: merged_entry}
        name_resolution: {raw_name: canonical_name}entries still need semantic matching
    """

    def _progress(msg: str) -> None:
        logging.info("wiki entity matching: %s", msg)
        if progress:
            try:
                progress(f"Entity Matching: {msg}")
            except Exception:
                logging.exception("wiki: entity matching progress callback failed")

    def _progress_interval(total: int) -> int:
        if total <= 20:
            return max(total, 1)
        return max(10, min(200, total // 10))

    # Step 1: Exact match against canonical index
    _progress(f"exact matching {len(raw_entities)} raw entries against {len(existing_canonical)} canonical entries ...")
    exact_flat: dict[str, str] = {}  # normalized_name → canonical_name
    for cname, centry in existing_canonical.items():
        # Use the non-analyzed `aliases` JSON field (deserialized in
        # _load_canonical_entities) plus the entity name itself. Do NOT rely on
        # `aliases_flat_kwd`: it is an analyzed varchar (whitespace-#) that
        # Infinity returns as a token list, losing the "||" structure.
        aliases = centry.get("aliases")
        if not isinstance(aliases, list):
            continue
        for alias in [cname] + [a for a in aliases if isinstance(a, str)]:
            exact_flat[_normalize_key(alias)] = cname

    name_resolution: dict[str, str] = {}  # raw_name → canonical_name
    llm_merge_pairs: list[dict[str, str]] = []
    unmatched: list[dict] = []  # entities not matched by exact
    for entry in raw_entities:
        raw_name = entry["name"]
        norm = _normalize_key(raw_name)
        if norm in exact_flat:
            name_resolution[raw_name] = exact_flat[norm]
        else:
            unmatched.append(entry)
    _progress(f"exact matched {len(name_resolution)}; {len(unmatched)} entries still need semantic matching.")

    # Step 2: KNN match for unmatched entities
    # Single search at ENTITY_AMBIGUOUS_LOW (0.75), classify by score:
    #   0.90+  → auto-merge (direct_match)
    #   0.75-0.90 → LLM confirm for entity types only
    #   <0.75  → no match
    if unmatched and embd_mdl and existing_canonical:
        query_texts = [_entity_to_query_text(e) for e in unmatched]
        embeddings, _ = await thread_pool_exec(embd_mdl.encode, query_texts)

        sem = asyncio.Semaphore(ENTITY_MATCH_KNN_CONCURRENT)

        async def _knn_one(entry: dict, vec) -> tuple[dict, str | None, float]:
            if hasattr(vec, "tolist"):
                vec = vec.tolist()
            result = await _knn_search_canonical(tenant_id, kb_id, vec, ENTITY_AMBIGUOUS_LOW)
            if result:
                return entry, result[0], result[1]
            return entry, None, 0.0

        async def _async_knn(entry: dict, vec):
            async with sem:
                return await _knn_one(entry, vec)

        _progress(f"KNN unmatched entities {len(query_texts)} ...")
        knn_tasks = [_async_knn(entry, emb) for entry, emb in zip(unmatched, embeddings)]
        knn_results = await asyncio.gather(*knn_tasks)
        _progress("KNN unmatched entities done.")

        still_unmatched: list[dict] = []
        maybe_pairs: list[tuple[dict, str]] = []
        for entry, cname, score in knn_results:
            if cname and score >= ENTITY_MERGE_THRESHOLD:
                # Direct merge
                name_resolution[entry["name"]] = cname
            elif cname and score >= ENTITY_AMBIGUOUS_LOW:
                # Ambiguous: LLM confirm for entity types only
                if entry["type"] == "concept":
                    name_resolution[entry["name"]] = cname
                else:
                    maybe_pairs.append((entry, cname))
            else:
                still_unmatched.append(entry)

        # LLM confirm for maybe pairs (entity only, 0.75-0.90)
        if maybe_pairs and chat_mdl:
            confirmed = await _wiki_confirm_batch(
                [(e["name"], cname) for e, cname in maybe_pairs],
                chat_mdl,
            )
            confirmed_set = set()
            for raw_name, cname in confirmed:
                name_resolution[raw_name] = cname
                confirmed_set.add(raw_name)
                llm_merge_pairs.append({"from": raw_name, "into": cname, "scope": "existing_canonical"})
            for e, cname in maybe_pairs:
                if e["name"] not in confirmed_set:
                    still_unmatched.append(e)

        unmatched = still_unmatched

    # Step 3: Intra-build pairwise (only on first build, i.e., non-incremental)
    # Blockwise matrix multiplication — mirrors old _embedding_dedup in
    # _common.py.  Avoids O(N²) Python loops and materializing an N×N matrix
    # (OOM risk); peak memory is O(block²).
    if not incremental and len(unmatched) > 1 and embd_mdl:
        query_texts = [_entity_to_query_text(e) for e in unmatched]
        embeddings, _ = await thread_pool_exec(embd_mdl.encode, query_texts)
        try:
            matrix = np.asarray([list(v) for v in embeddings], dtype=np.float32)
            if matrix.ndim != 2 or matrix.shape[0] != len(unmatched):
                raise ValueError("invalid embedding matrix shape")
            norms = np.linalg.norm(matrix, axis=1, keepdims=True)
            matrix = np.divide(matrix, norms, out=np.zeros_like(matrix), where=norms > 0)
        except Exception:
            logging.exception("wiki: pairwise embedding failed; skipping semantic merge")
            matrix = None

        if matrix is not None:
            merged_into: dict[int, int] = {}
            maybe_pairs: list[tuple[int, int]] = []

            def _root(i: int) -> int:
                while i in merged_into:
                    i = merged_into[i]
                return i

            n = len(unmatched)
            block_size = ENTITY_PAIRWISE_BLOCK_SIZE
            # Group by type: only entities of the SAME type are pairwise candidates
            # (entity-vs-entity, concept-vs-concept).  This mirrors old behavior
            # of passing type_key so cross-type pairs are never merged.
            groups: dict[str, list[int]] = {}
            for idx, entry in enumerate(unmatched):
                groups.setdefault(entry.get("type", "entity"), []).append(idx)

            auto_pairs: list[tuple[int, int]] = []
            ambiguous_pairs: list[tuple[int, int]] = []
            for group_indices in groups.values():
                for left_start in range(0, len(group_indices), block_size):
                    left_indices = group_indices[left_start : left_start + block_size]
                    left_vectors = matrix[left_indices]
                    for right_start in range(left_start, len(group_indices), block_size):
                        right_indices = group_indices[right_start : right_start + block_size]
                        sims = left_vectors @ matrix[right_indices].T  # [B, B] BLAS
                        if right_start == left_start:
                            candidate_mask = np.triu(sims >= ENTITY_AMBIGUOUS_LOW, k=1)
                        else:
                            candidate_mask = sims >= ENTITY_AMBIGUOUS_LOW
                        rows, cols = np.nonzero(candidate_mask)
                        for row, col in zip(rows.tolist(), cols.tolist(), strict=True):
                            score = float(sims[row, col])
                            if score >= ENTITY_MERGE_THRESHOLD:
                                auto_pairs.append((left_indices[row], right_indices[col]))
                            else:
                                ambiguous_pairs.append((left_indices[row], right_indices[col]))

            # Apply auto-merges with union-find (higher evidence wins)
            for i, j in auto_pairs:
                ri, rj = _root(i), _root(j)
                if ri == rj:
                    continue
                if unmatched[ri].get("claim_count", 0) >= unmatched[rj].get("claim_count", 0):
                    merged_into[rj] = ri
                else:
                    merged_into[ri] = rj

            # Keep only ambiguous pairs still in separate groups
            still_ambiguous = [(i, j) for i, j in ambiguous_pairs if _root(i) != _root(j)]

            # LLM confirm for ambiguous pairs (first build only)
            if still_ambiguous and chat_mdl:
                llm_candidates = [(unmatched[i]["name"], unmatched[j]["name"]) for i, j in still_ambiguous]
                confirmed = await _wiki_confirm_batch(llm_candidates, chat_mdl)
                confirmed_map = {frozenset((a, b)) for a, b in confirmed}
                for i, j in still_ambiguous:
                    pair = frozenset((unmatched[i]["name"], unmatched[j]["name"]))
                    if pair in confirmed_map:
                        ri, rj = _root(i), _root(j)
                        if ri != rj:
                            if unmatched[ri].get("claim_count", 0) >= unmatched[rj].get("claim_count", 0):
                                merged_into[rj] = ri
                                llm_merge_pairs.append({"from": unmatched[rj]["name"], "into": unmatched[ri]["name"], "scope": "intra_build"})
                            else:
                                merged_into[ri] = rj
                                llm_merge_pairs.append({"from": unmatched[ri]["name"], "into": unmatched[rj]["name"], "scope": "intra_build"})

            # Apply merges
            merged_indices: dict[int, list[int]] = {}
            for i in range(n):
                pi = _root(i)
                merged_indices.setdefault(pi, []).append(i)

            new_unmatched = []
            for pi, indices in merged_indices.items():
                if len(indices) > 1:
                    master = unmatched[indices[0]]
                    for idx in indices[1:]:
                        slave = unmatched[idx]
                        # Lightweight entries have no 'claims' field; only
                        # metadata is aggregated (claim text lives in claim_index
                        # and is aggregated later via name_resolution).
                        master["claim_count"] += slave["claim_count"]
                        master["source_doc_ids"] = list(set(master["source_doc_ids"]) | set(slave["source_doc_ids"]))
                        master["source_chunk_ids"] = list(set(master.get("source_chunk_ids", [])) | set(slave.get("source_chunk_ids", [])))
                        master["aliases"] = list(set(master["aliases"] + slave["aliases"] + [slave["name"]]))
                        name_resolution[slave["name"]] = master["name"]
                    new_unmatched.append(master)
                else:
                    new_unmatched.append(unmatched[indices[0]])
            unmatched = new_unmatched

    # Step 4: Build canonical map
    canonical_map: dict[str, dict] = {}
    for entry in unmatched:
        cname = entry["name"]
        canonical_map[cname] = entry
        name_resolution.setdefault(cname, cname)

    # Also load existing canonical entries that match
    for raw_name, cname in name_resolution.items():
        if cname not in canonical_map:
            existing = existing_canonical.get(cname)
            if existing:
                # Lightweight entry — claims loaded separately on demand
                merged = {
                    "name": cname,
                    "type": existing.get("entity_type_kwd", "entity"),
                    "aliases": existing.get("aliases", []),
                    "claim_count": existing.get("mention_count_int", 0),
                    "source_doc_ids": existing.get("source_doc_ids", []),
                    "source_chunk_ids": existing.get("source_chunk_ids", []),
                }
                canonical_map[cname] = merged

    # Aggregate lightweight metadata (claim_count, source_doc_ids) across all
    # raw entities that resolved to the same canonical name.
    for entry in raw_entities:
        raw_name = entry["name"]
        cname = name_resolution.get(raw_name, raw_name)
        if cname in canonical_map:
            canonical_map[cname]["claim_count"] += entry.get("claim_count", 0)
            existing_docs = set(canonical_map[cname].get("source_doc_ids", []))
            existing_docs.update(entry.get("source_doc_ids", []))
            canonical_map[cname]["source_doc_ids"] = list(existing_docs)
            existing_chunks = set(canonical_map[cname].get("source_chunk_ids", []))
            existing_chunks.update(entry.get("source_chunk_ids", []))
            canonical_map[cname]["source_chunk_ids"] = list(existing_chunks)
            aliases = set(canonical_map[cname].get("aliases", []))
            aliases.update(alias for alias in entry.get("aliases", []) if isinstance(alias, str) and alias)
            if raw_name != cname:
                aliases.add(raw_name)
            aliases.discard(cname)
            canonical_map[cname]["aliases"] = sorted(aliases)

    for merge in llm_merge_pairs:
        _wiki_log_stats("MATCH", "llm_merge", kb_id=kb_id, incremental=incremental, **merge)
    _wiki_log_stats("MATCH", "llm_merge_summary", kb_id=kb_id, incremental=incremental, before=len(raw_entities), after=len(canonical_map), llm_merge_count=len(llm_merge_pairs))

    return canonical_map, name_resolution


async def _wiki_confirm_batch(
    candidates: list[tuple[str, str]],
    chat_mdl,
) -> list[tuple[str, str]]:
    """Batch LLM confirm — adapted from old _common.py pattern.

    Takes [(name_a, name_b), ...], returns confirmed [(name_a, name_b), ...].
    """
    if not candidates:
        return []
    # Split into batches of 50
    batch_size = 50
    confirmed = []
    for i in range(0, len(candidates), batch_size):
        batch = candidates[i : i + batch_size]
        prompt_lines = []
        for j, (a, b) in enumerate(batch):
            prompt_lines.append(f'{j + 1}. "{a}" vs "{b}"')
        prompt = (
            "You are a KB dedup assistant. For each pair, determine if they "
            "refer to the SAME real-world entity.\n"
            "Respond with a JSON array of booleans in the same order:\n"
            "  [true, false, true, ...]\n"
            "where true = SAME entity, false = DIFFERENT.\n\n" + "\n".join(prompt_lines)
        )
        try:
            resp = await _chat_mdl_ask(chat_mdl, "You are a KB dedup assistant.", prompt)
            if resp:
                resp = resp.strip()
                # Extract JSON array
                arr_match = re.search(r"\[.*?\]", resp, re.DOTALL)
                if arr_match:
                    booleans = json.loads(arr_match.group(0))
                    for j, is_same in enumerate(booleans):
                        if is_same and j < len(batch):
                            confirmed.append(batch[j])
        except Exception:
            logging.exception("wiki: LLM confirm batch failed")
    return confirmed


async def _search_existing_pages(
    tenant_id: str,
    kb_id: str,
    select_fields: list[str],
) -> dict[str, dict]:
    """Load all wiki_page rows in this KB."""
    index = search.index_name(tenant_id)
    if not settings.docStoreConn.index_exist(index, kb_id):
        return {}

    results: dict[str, dict] = {}
    offset = 0
    page_size = 1000
    while True:
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                select_fields,
                [],
                {"compile_kwd": [WIKI_PAGE_COMPILE_KWD]},
                [],
                OrderByExpr(),
                offset,
                page_size,
                index,
                [kb_id],
            )
            field_map = settings.docStoreConn.get_fields(res, select_fields) or {}
        except Exception:
            logging.exception("wiki: failed to load existing pages for kb=%s", kb_id)
            return results
        for row_id, row in field_map.items():
            # Keep the storage document id.  FINALIZE must update by id so
            # markdown values go through the document update fast path and
            # retain their newlines.
            row["id"] = row_id
            slug = row.get("slug_kwd", row.get("page_id", ""))
            if isinstance(slug, list):
                slug = slug[0] if slug else ""
            slug = str(slug or "").strip()
            if slug:
                results[slug] = row
        if len(field_map) < page_size:
            break
        offset += page_size
    return results


async def _load_map_relations(
    tenant_id: str,
    kb_id: str,
    excluded_doc_ids: set[str] | None = None,
    chunk_state: dict[str, dict] | None = None,
) -> list[dict]:
    """Load all extracted (from, to, type) relations from wiki_map_extract rows.

    These are the semantic edges the LLM extracted during MAP. When both
    endpoints correspond to compiled wiki pages they become page-to-page
    connections (outlinks), which is far more reliable than hoping REFINE
    sprinkled [[wikilinks]] into prose.
    """
    if chunk_state is None:
        from rag.advanced_rag.knowlege_compile.wiki import _wiki_load_active_map_state

        chunk_state = await _wiki_load_active_map_state(tenant_id, kb_id)
    from rag.advanced_rag.knowlege_compile.wiki import _wiki_load_map_extracts_for_state

    extracts = await _wiki_load_map_extracts_for_state(tenant_id, kb_id, chunk_state)
    relations: list[dict] = []
    for extract in extracts:
        if excluded_doc_ids and str(extract.get("doc_id") or "") in excluded_doc_ids:
            continue
        for relation in extract.get("relations") or []:
            if not isinstance(relation, dict):
                continue
            source = relation.get("from")
            target = relation.get("to")
            if isinstance(source, str) and isinstance(target, str):
                relations.append({"from": source, "to": target, "type": relation.get("type", "related")})
    return relations


async def _wiki_load_pages_for_graph(
    tenant_id: str,
    kb_id: str,
    excluded_doc_ids: set[str] | None = None,
    chunk_state: dict[str, dict] | None = None,
) -> list[dict]:
    """Reload compiled wiki_page rows and project them onto the canvas-graph
    shape expected by ``dataset_wiki_generator.build_wiki_page_graph``.

    Each returned page has: {slug, title, summary, entity_names,
    source_chunk_ids, source_doc_ids, outlinks, page_type}. Used by the
    incremental entry point to materialize the artifact canvas graph after
    ``wiki_compile_incremental`` persists pages (which it does internally
    without returning the page list).
    """
    from common.doc_store.doc_store_base import OrderByExpr

    select_fields = [
        "slug_kwd",
        "title_kwd",
        "page_type_kwd",
        "summary_with_weight",
        "md_with_weight",
        "entity_names_kwd",
        "outlinks_kwd",
        "source_chunk_ids",
        "source_doc_ids",
    ]
    pages: list[dict] = []
    offset, page_size = 0, 1000
    while True:
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                select_fields,
                [],
                {"compile_kwd": [WIKI_PAGE_COMPILE_KWD]},
                [],
                OrderByExpr(),
                offset,
                page_size,
                search.index_name(tenant_id),
                [kb_id],
            )
            field_map = settings.docStoreConn.get_fields(res, select_fields) or {}
        except Exception:
            logging.exception("wiki: failed to load pages for graph kb=%s", kb_id)
            return pages
        for row in field_map.values():
            slug = row.get("slug_kwd")
            if isinstance(slug, (list, tuple)):
                slug = slug[0] if slug else ""
            slug = str(slug or "").strip()
            if not slug:
                continue
            # outlinks_kwd is a *_kwd field analyzed by whitespace-#: Infinity
            # shreds the stored JSON list and get_fields reads it back as [] (or
            # a mangled token list). The reliable source of edges is the
            # [[wikilinks]] actually present in the page body (auto-link +
            # relation-based injection both write them). Prefer content-derived
            # outlinks; fall back to the kwd field only if content has none.
            outlinks = _wiki_extract_outlinks_from_content(str(row.get("md_with_weight") or ""), kb_id)
            if not outlinks:
                outlinks = _as_str_list(row.get("outlinks_kwd"))
            title = row.get("title_kwd")
            if isinstance(title, (list, tuple)):
                title = title[0] if title else ""
            page_type = row.get("page_type_kwd")
            if isinstance(page_type, (list, tuple)):
                page_type = page_type[0] if page_type else ""
            pages.append(
                {
                    "slug": slug,
                    "title": str(title or slug),
                    "summary": str(row.get("summary_with_weight") or ""),
                    "page_type": str(page_type or "concept"),
                    "entity_names": _as_str_list(row.get("entity_names_kwd")),
                    "outlinks": outlinks,
                    "source_chunk_ids": _as_str_list(row.get("source_chunk_ids")),
                    "source_doc_ids": _as_str_list(row.get("source_doc_ids")),
                }
            )
        if len(field_map) < page_size:
            break
        offset += page_size
    # A page may contain no generated wikilink even though MAP extracted a
    # semantic relation.  Rebuild the graph from those grounded MAP relations
    # as a fallback; otherwise wiki_entity rows exist but wiki_relation stays
    # empty.  Page slugs remain the graph identities, while member names,
    # titles, and slug suffixes are accepted as relation endpoints.
    if pages:
        name_to_slug: dict[str, str] = {}
        for page in pages:
            slug = page["slug"]
            names = [slug.rsplit("/", 1)[-1], page.get("title", ""), *page.get("entity_names", [])]
            for name in names:
                if isinstance(name, str) and name.strip():
                    name_to_slug.setdefault(name.strip(), slug)
        try:
            if excluded_doc_ids is None:
                from api.db.services.document_service import DocumentService

                excluded_doc_ids = await thread_pool_exec(DocumentService.get_disabled_doc_ids_by_kb_id, kb_id)
            map_relations = await _load_map_relations(
                tenant_id,
                kb_id,
                excluded_doc_ids=excluded_doc_ids,
                chunk_state=chunk_state,
            )
        except Exception:
            logging.exception("wiki: failed to load MAP relations for graph fallback kb=%s", kb_id)
            map_relations = []
        pages_by_slug = {page["slug"]: page for page in pages}
        for relation in map_relations:
            source = name_to_slug.get(str(relation.get("from") or "").strip())
            target = name_to_slug.get(str(relation.get("to") or "").strip())
            if not source or not target or source == target:
                continue
            outlinks = pages_by_slug[source].setdefault("outlinks", [])
            if target not in outlinks:
                outlinks.append(target)
    return pages


_WIKILINK_RE = re.compile(r"\[\[([^\]]+)\]\]")


def _wiki_extract_outlinks_from_content(content: str, kb_id: str = "") -> list[str]:
    """Extract unique internal link targets from page markdown, in order.

    Accepts both the raw ``[[slug]]`` form and the rendered Markdown form
    ``[text](artifact/{kb_id}/{slug})``. Used to backfill graph edges from
    page bodies (outlinks_kwd is a *_kwd field Infinity shreds on read-back).
    """
    if not content:
        return []
    seen: set[str] = set()
    outlinks: list[str] = []
    for m in _WIKILINK_RE.finditer(content):
        # ``[[page_slug|display text]]`` stores the page identity before the
        # pipe.  The display text is only presentation data and must never be
        # used as the graph target.
        link = m.group(1).split("|", 1)[0].strip()
        if link and link not in seen:
            seen.add(link)
            outlinks.append(link)
    if kb_id:
        kb_esc = re.escape(str(kb_id))
        for m in re.finditer(rf"\]\(artifact/{kb_esc}/([^)]+)\)", content):
            slug = m.group(1).split("|", 1)[0].strip()
            if slug and slug not in seen:
                seen.add(slug)
                outlinks.append(slug)
    return outlinks


_WIKI_PROTECTED_LINK_RE = re.compile(r"\[\[[^\]\n]+\]\]|\[[^\]\n]*\]\([^)\n]+\)")


def _wiki_find_unlinked_mention(content: str, name: str) -> int:
    """Find the first occurrence of ``name`` outside existing link markup."""
    if not content or not name:
        return -1
    protected_spans = [(match.start(), match.end()) for match in _WIKI_PROTECTED_LINK_RE.finditer(content)]
    raw_open = content.rfind("[[")
    if raw_open >= 0 and content.find("]]", raw_open + 2) < 0:
        protected_spans.append((raw_open, len(content)))
    start = 0
    while True:
        idx = content.find(name, start)
        if idx < 0:
            return -1
        end = idx + len(name)
        if not any(idx < protected_end and end > protected_start for protected_start, protected_end in protected_spans):
            return idx
        start = idx + 1


_WIKI_PIPE_LINK_RE = re.compile(r"\[\[([^\[\]\|]+?)\|([^\[\]]+?)\]\]")
_WIKI_SIMPLE_LINK_RE = re.compile(r"\[\[([^\[\]\|]+?)\]\]")


def _wiki_render_links(content: str, kb_id: str, valid_slugs: set[str]) -> str:
    """Render internal ``[[slug]]`` / ``[[slug|text]]`` wikilinks into
    navigable Markdown links ``[text](artifact/{kb_id}/{slug})``.

    This is the format the frontend wiki viewer can deep-link (plain ``[[...]]``
    renders as text and cannot navigate). Targets not in ``valid_slugs`` are
    left as plain text (their label only). Mirrors the old-mode behaviour.
    """
    if not content:
        return content
    kb = str(kb_id)

    def _simple(m: re.Match) -> str:
        slug = m.group(1).strip()
        if slug not in valid_slugs:
            return slug
        label = slug.rsplit("/", 1)[-1] if "/" in slug else slug
        return f"[{label}](artifact/{kb}/{slug})"

    def _piped(m: re.Match) -> str:
        slug = m.group(1).strip()
        text = m.group(2).strip()
        if slug not in valid_slugs:
            return text
        return f"[{text}](artifact/{kb}/{slug})"

    rendered = _WIKI_PIPE_LINK_RE.sub(_piped, content)
    rendered = _WIKI_SIMPLE_LINK_RE.sub(_simple, rendered)
    return rendered


def _wiki_resolve_dead_slug(link: str, valid_ids: set[str], name_slug: dict[str, str]) -> str | None:
    """WeKnora-style fuzzy resolution of a dead wikilink to a live page.

    A ``[[dead_slug]]`` may fail to match a valid page because the target page
    was renamed / its slug changed, or because the writer used an alias. Try,
    in order: exact match, normalized-slug match, display-text reverse lookup
    (via ``name_slug``), then bigram-token similarity over the plain names.
    Returns a valid target slug or ``None`` (caller then degrades to text).
    """
    if not link:
        return None

    def _norm(s: str) -> str:
        return re.sub(r"[-_]+", "-", s.strip().lower())

    plain = link.rsplit("/", 1)[-1] if "/" in link else link
    l_norm = _norm(link)
    p_norm = _norm(plain)

    # 1. Exact / normalized target.
    if link in valid_ids:
        return link
    if l_norm in valid_ids:
        return l_norm

    # 2. Display-text reverse lookup via name_slug (plain names + titles + aliases).
    if plain in name_slug:
        return name_slug[plain]
    if p_norm in {_norm(k) for k in name_slug}:
        for k, v in name_slug.items():
            if _norm(k) == p_norm:
                return v

    # 3. Bigram-token similarity over plain names (longest tokens, then best match).
    def _bigrams(text: str) -> set[str]:
        return {text[i : i + 2] for i in range(max(0, len(text) - 1))}

    p_tokens = _norm(plain).split("-")
    p_bigrams = _bigrams(p_norm)
    best: tuple[float, str] | None = None
    for cand_name, cand_slug in name_slug.items():
        c_norm = _norm(cand_name)
        c_tokens = c_norm.split("-")
        # Require at least one shared token (or a shared prefix token) to avoid
        # linking unrelated pages.
        if not (set(p_tokens) & set(c_tokens)):
            continue
        cb = _bigrams(c_norm)
        denom = len(p_bigrams | cb)
        if denom == 0:
            continue
        score = len(p_bigrams & cb) / denom
        if best is None or score > best[0]:
            best = (score, cand_slug)
    if best and best[0] >= 0.5:
        return best[1]
    return None


def _as_str_list(raw) -> list[str]:
    """Coerce a stored field (JSON string / list / None) into a list of str."""
    if raw is None:
        return []
    if isinstance(raw, str):
        if not raw:
            return []
        try:
            val = json.loads(raw)
        except (json.JSONDecodeError, TypeError):
            return [raw]
        return _as_str_list(val)
    if isinstance(raw, (list, tuple)):
        return [str(v) for v in raw if v is not None]
    return []


def _as_int(raw, default: int = 0) -> int:
    """Coerce numeric doc-store fields, which some backends return as strings."""
    try:
        return int(raw)
    except (TypeError, ValueError):
        return default


def _wiki_claim_chunk_ids(claim: dict) -> list[str]:
    """Return the source chunk id(s) a MAP claim is attributed to.

    MAP (``wiki._wiki_resolve_chunk_ids``) rewrites every item's
    ``source_chunk_id`` into ``chunk_ids=[real_id]`` (an array). Later stages
    must read the array, falling back to the singular ``source_chunk_id`` for
    robustness.
    """
    if not isinstance(claim, dict):
        return []
    ids = claim.get("chunk_ids")
    if isinstance(ids, str):
        ids = [ids]
    if isinstance(ids, (list, tuple)):
        return [str(c) for c in ids if c]
    s = claim.get("source_chunk_id")
    return [str(s)] if s else []


def _wiki_dedupe_claims(claims: list[dict]) -> list[dict]:
    result: list[dict] = []
    seen: set[tuple[str, str, tuple[str, ...]]] = set()
    for claim in claims:
        if not isinstance(claim, dict):
            continue
        key = (
            str(claim.get("statement") or claim.get("text") or ""),
            str(claim.get("source_doc_id") or ""),
            tuple(sorted(_wiki_claim_chunk_ids(claim))),
        )
        if key in seen:
            continue
        seen.add(key)
        result.append(claim)
    return result


def _wiki_topics_for_docs(
    doc_ids: list[str] | set[str],
    doc_topics: dict[str, list[str]] | None,
    topic_pool: dict[str, str] | None = None,
) -> list[str]:
    topics: list[str] = []
    seen: set[str] = set()
    for doc_id in doc_ids:
        for topic in (doc_topics or {}).get(doc_id, []):
            if not isinstance(topic, str):
                continue
            topic = topic.strip()
            key = _normalize_key(topic)
            if not topic or key == _normalize_key(WIKI_TOPIC_FALLBACK) or key in seen:
                continue
            seen.add(key)
            topics.append(topic)
    for topic in (topic_pool or {}).values():
        key = _normalize_key(topic)
        if topic and key not in seen:
            seen.add(key)
            topics.append(topic)
    return topics


async def _wiki_prepare_topic_embeddings(
    doc_topics: dict[str, list[str]],
    embd_mdl,
    extra_topics: list[str] | None = None,
) -> dict[str, object]:
    topics = sorted(
        {
            topic
            for values in list(doc_topics.values()) + [extra_topics or []]
            for topic in values
            if isinstance(topic, str) and topic.strip() and _normalize_key(topic) != _normalize_key(WIKI_TOPIC_FALLBACK)
        },
        key=lambda value: (value.casefold(), value),
    )
    if not topics or embd_mdl is None:
        return {}
    embeddings, _ = await thread_pool_exec(embd_mdl.encode, topics)
    return {topic: vector for topic, vector in zip(topics, embeddings, strict=True)}


def _wiki_topic_query_text(
    page_title: str,
    claims: list[dict] | None,
    source_chunks: list[dict] | None,
    existing_page: dict | None = None,
) -> str:
    parts = [f"title={page_title}"] if page_title else []
    if existing_page:
        summary = existing_page.get("summary_with_weight") or ""
        if summary:
            parts.append(f"summary={summary}")
    evidence = []
    for claim in (claims or [])[:8]:
        if isinstance(claim, dict):
            text = claim.get("statement") or claim.get("text")
            if text:
                evidence.append(str(text))
    if evidence:
        parts.append(f"evidence={' | '.join(evidence)}")
    chunk_text = []
    for chunk in (source_chunks or [])[:4]:
        if isinstance(chunk, dict):
            text = chunk.get("text") or chunk.get("content_with_weight")
            if text:
                chunk_text.append(str(text)[:500])
    if chunk_text:
        parts.append(f"source={' | '.join(chunk_text)}")
    return "; ".join(parts)


async def _wiki_rank_topic_candidates(
    page_title: str,
    claims: list[dict] | None,
    source_chunks: list[dict] | None,
    existing_page: dict | None,
    topic_candidates: list[str] | None,
    topic_embeddings: dict[str, object] | None,
    embd_mdl,
) -> list[str]:
    """Use embedding only to recall topic candidates; LLM remains the selector."""
    candidates = []
    seen: set[str] = set()
    for topic in topic_candidates or []:
        if not isinstance(topic, str):
            continue
        topic = topic.strip()
        key = _normalize_key(topic)
        if topic and key not in seen:
            seen.add(key)
            candidates.append(topic)
    if len(candidates) <= 1 or embd_mdl is None:
        return candidates[:WIKI_PAGE_TOPIC_CANDIDATE_LIMIT]

    query_text = _wiki_topic_query_text(page_title, claims, source_chunks, existing_page)
    query_embedding, _ = await thread_pool_exec(embd_mdl.encode, [query_text])
    query = np.asarray(query_embedding[0], dtype=np.float32)
    query_norm = np.linalg.norm(query)
    if query_norm <= 0:
        return candidates[:WIKI_PAGE_TOPIC_CANDIDATE_LIMIT]
    query = query / query_norm

    local_topic_embeddings = dict(topic_embeddings or {})
    missing_topics = [topic for topic in candidates if topic not in local_topic_embeddings]
    if missing_topics:
        encoded, _ = await thread_pool_exec(embd_mdl.encode, missing_topics)
        local_topic_embeddings.update({topic: vector for topic, vector in zip(missing_topics, encoded, strict=True)})
        if topic_embeddings is not None:
            topic_embeddings.update({topic: vector for topic, vector in zip(missing_topics, encoded, strict=True)})

    ranked = []
    for topic in candidates:
        vector = np.asarray(local_topic_embeddings.get(topic), dtype=np.float32) if topic in local_topic_embeddings else None
        if vector is None or vector.size == 0:
            continue
        norm = np.linalg.norm(vector)
        score = float(np.dot(query, vector / norm)) if norm > 0 else -1.0
        ranked.append((score, topic))
    ranked.sort(key=lambda item: (-item[0], item[1]))
    return [topic for _, topic in ranked[:WIKI_PAGE_TOPIC_CANDIDATE_LIMIT]]


def _wiki_decide_concept_pages(all_concepts: list[dict]) -> list[dict]:
    """Return every concept as a wiki page.

    Mode A compiles EVERY entity AND concept into its own page, so no depth
    filter is applied here. A concept that was extracted by MAP (it exists in
    ``concepts[]``) is compiled even if it has no dedicated claim rows — its
    page is enriched from the source chunks of the document(s) where it
    appears.
    """
    pages = []
    for concept in all_concepts:
        claims = concept.get("claims", [])
        source_docs = set(c.get("source_doc_id") for c in claims if c.get("source_doc_id"))
        pages.append(
            {
                "page_id": _wiki_derive_page_id(concept["term"]),
                "page_title": concept["term"],
                "concept": concept,
                "claims": claims,
                "source_doc_ids": list(source_docs),
            }
        )
    return pages


# ----- REDUCE (shared, per-entity) ------------------------------------------


async def _wiki_reduce_entity(
    entity_name: str,
    new_claims: list[dict],
    existing_page: dict | None,
    deleted_doc_ids: set[str],
    invalidated_chunk_ids: set[str] | None = None,
    entity_type: str = "entity",
    aliases: list[str] | None = None,
    source_doc_ids: list[str] | None = None,
    source_chunk_ids: list[str] | None = None,
) -> dict:
    """Per-entity REDUCE: compute additions/retractions vs existing page.

    Returns dict with action (create|update|delete|noop), additions, retractions,
    and entity_type for downstream filtering.
    """
    if existing_page is None:
        if isinstance(entity_type, list):
            entity_type = entity_type[0] if entity_type else "entity"
        entity_type = str(entity_type or "entity").strip()
        # Claims are not the only evidence: entity/concept rows and relations
        # carry their own source chunk attribution from MAP.
        if not new_claims and not source_chunk_ids:
            return {
                "action": "noop",
                "entity_name": entity_name,
                "entity_type": entity_type,
                "aliases": aliases or [],
                "additions": [],
                "retractions": [],
                "retained_source_doc_ids": [],
                "has_delta": False,
            }
        return {
            "action": "create",
            "entity_name": entity_name,
            "entity_type": entity_type,
            "aliases": aliases or [],
            "additions": new_claims,
            "source_chunk_ids": sorted(set(source_chunk_ids or [])),
            "retained_source_doc_ids": sorted(set(source_doc_ids or []) | {c["source_doc_id"] for c in new_claims if c.get("source_doc_id")}),
            "has_delta": True,
        }

    existing_claims = existing_page.get("claims", [])
    # The stored `claims` column is a JSON string (json.dumps in _wiki_refine_page).
    # Normalize to a list of dicts defensively — iterating a raw string yields
    # characters and breaks every `c.get(...)` below.
    if isinstance(existing_claims, str):
        try:
            existing_claims = json.loads(existing_claims) if existing_claims else []
        except (json.JSONDecodeError, TypeError):
            existing_claims = []
    if isinstance(existing_claims, (list, tuple)):
        existing_claims = [c for c in existing_claims if isinstance(c, dict)]
    else:
        existing_claims = []
    deleted_set = deleted_doc_ids or set()
    invalidated_set = invalidated_chunk_ids or set()
    existing_chunk_ids = set(_as_str_list(existing_page.get("source_chunk_ids")))
    all_page_evidence_invalidated = bool(existing_chunk_ids) and existing_chunk_ids <= invalidated_set

    retractions = [
        c for c in existing_claims if c.get("source_doc_id") in deleted_set or bool(set(_wiki_claim_chunk_ids(c)) & invalidated_set) or (all_page_evidence_invalidated and not _wiki_claim_chunk_ids(c))
    ]

    retained_claims = [c for c in existing_claims if c not in retractions]

    retained_texts = {c.get("statement", c.get("text", "")) for c in retained_claims}
    additions = [c for c in new_claims if c.get("statement", c.get("text", "")) not in retained_texts]

    all_doc_ids = (
        {c.get("source_doc_id") for c in retained_claims if c.get("source_doc_id")} | {c.get("source_doc_id") for c in additions if c.get("source_doc_id")} | (set(source_doc_ids or []) - deleted_set)
    )
    current_chunk_ids = sorted(set(source_chunk_ids or []) if source_chunk_ids else existing_chunk_ids - invalidated_set)
    evidence_changed = set(current_chunk_ids) != existing_chunk_ids

    if not all_doc_ids:
        return {
            "action": "delete",
            "entity_name": entity_name,
            "entity_type": entity_type,
            "aliases": aliases or [],
            "retractions": existing_claims,
            "source_chunk_ids": current_chunk_ids,
            "has_delta": True,
        }
    elif additions or retractions or evidence_changed:
        return {
            "action": "update",
            "entity_name": entity_name,
            "entity_type": entity_type,
            "aliases": aliases or [],
            "additions": additions,
            "retractions": retractions,
            "source_chunk_ids": current_chunk_ids,
            "retained_source_doc_ids": list(all_doc_ids),
            "has_delta": True,
        }
    return {
        "action": "noop",
        "entity_name": entity_name,
        "entity_type": entity_type,
        "aliases": aliases or [],
        "source_chunk_ids": current_chunk_ids,
        "retained_source_doc_ids": list(all_doc_ids),
        "has_delta": False,
    }


async def _wiki_reduce_batch(
    affected_names: set[str],
    existing_pages: dict[str, dict],
    deleted_doc_ids: set[str],
    invalidated_chunk_ids: set[str] | None = None,
    canonical_claims: dict[str, list[dict]] | None = None,
    canonical_map: dict[str, dict] | None = None,
    name_resolution: dict[str, str] | None = None,
    map_results: list[dict] | None = None,
) -> list[dict]:
    """Parallel per-entity REDUCE over a batch of affected canonical names.

    Uses canonical_claims (from Entity Matching) instead of raw MAP claims.
    """
    name_to_page: dict[str, dict] = {}
    for pid, page in existing_pages.items():
        for n in _as_str_list(page.get("entity_names_kwd")):
            name_to_page[n] = page
        slug = pid.split("/")[-1] if "/" in pid else pid
        name_to_page.setdefault(slug, page)

    # Use canonical claims if provided (post-entity-matching)
    if canonical_claims is not None:
        claims_source = canonical_claims
    else:
        # Fallback: aggregate from raw MAP results
        claims_source = {}
        for mr in map_results:
            for c in mr.get("claims", []):
                name = c.get("entity_name") or c.get("subject") or c.get("term")
                if name:
                    raw_name = name
                    if name_resolution:
                        raw_name = name_resolution.get(name, name)
                    claims_source.setdefault(raw_name, []).append(c)

    for name in affected_names:
        claims_source.setdefault(name, [])

    tasks = []
    for name in affected_names:
        claims = claims_source.get(name, [])
        # Determine entity_type from canonical_map
        entity_type = "entity"
        if canonical_map and name in canonical_map:
            entity_type = canonical_map[name].get("type", "entity")
        aliases = canonical_map[name].get("aliases", []) if canonical_map and name in canonical_map else []
        source_doc_ids = canonical_map[name].get("source_doc_ids", []) if canonical_map and name in canonical_map else []
        source_chunk_ids = canonical_map[name].get("source_chunk_ids", []) if canonical_map and name in canonical_map else []
        if isinstance(entity_type, list):
            entity_type = entity_type[0] if entity_type else "entity"
        entity_type = str(entity_type or "entity").strip()

        tasks.append(
            _wiki_reduce_entity(
                entity_name=name,
                entity_type=entity_type,
                aliases=aliases,
                source_doc_ids=source_doc_ids,
                source_chunk_ids=source_chunk_ids,
                new_claims=claims,
                existing_page=name_to_page.get(name, existing_pages.get(name)),
                deleted_doc_ids=deleted_doc_ids,
                invalidated_chunk_ids=invalidated_chunk_ids,
            )
        )
    if not tasks:
        return []
    results = await asyncio.gather(*tasks)
    return [r for r in results if r.get("has_delta")]


# ----- doc_page_source tracking ---------------------------------------------


async def _wiki_update_doc_page_source(
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    page_ids: list[str],
    entity_names: list[str] | None = None,
    chunk_hashes: dict[str, str] | None = None,
    map_checksum: str | None = None,
) -> None:
    """Record which pages and entities this document contributes to."""
    index = search.index_name(tenant_id)

    condition = {
        "compile_kwd": [WIKI_DOC_PAGE_SOURCE_COMPILE_KWD],
        "doc_id": [doc_id],
    }
    existing = await thread_pool_exec(
        settings.docStoreConn.search,
        ["id", "page_ids", "entity_names", "source_chunk_hashes", "map_checksum"],
        [],
        condition,
        [],
        OrderByExpr(),
        0,
        1,
        index,
        [kb_id],
    )
    existing_map = settings.docStoreConn.get_fields(existing, ["id", "page_ids", "entity_names", "source_chunk_hashes", "map_checksum"])

    if existing_map:
        for row in existing_map.values():
            if chunk_hashes is None:
                saved = row.get("source_chunk_hashes", "{}")
                chunk_hashes = json.loads(saved) if isinstance(saved, str) else saved
            if map_checksum is None:
                val = row.get("map_checksum", "") or ""
                if val:
                    map_checksum = val
            if entity_names is None:
                saved_names = row.get("entity_names", "[]")
                entity_names = json.loads(saved_names) if isinstance(saved_names, str) else saved_names
            break

    doc = {
        "id": _stable_row_id(WIKI_DOC_PAGE_SOURCE_COMPILE_KWD, kb_id, doc_id),
        "doc_id": doc_id,
        "kb_id": kb_id,
        "page_ids": json.dumps(page_ids, ensure_ascii=False),
        "entity_names": json.dumps(entity_names or [], ensure_ascii=False),
        "source_chunk_hashes": json.dumps(chunk_hashes or {}, ensure_ascii=False),
        "map_checksum": map_checksum or "",
        "compile_kwd": WIKI_DOC_PAGE_SOURCE_COMPILE_KWD,
    }
    if existing_map:
        await thread_pool_exec(
            settings.docStoreConn.update,
            # ``doc_id`` is shared by source chunks, MAP resume rows, and this
            # tracking row. Updating by doc_id rewrites every one of them into
            # ``wiki_doc_page_source``. The tracking row has a stable unique
            # id, so updates must always use that identity.
            {"id": doc["id"]},
            doc,
            index,
            kb_id,
        )
    else:
        await thread_pool_exec(
            settings.docStoreConn.insert,
            [doc],
            index,
            kb_id,
        )


async def _wiki_load_doc_page_source(
    tenant_id: str,
    kb_id: str,
    doc_id: str,
) -> dict | None:
    """Load doc_page_source record for a document."""
    index = search.index_name(tenant_id)
    condition = {
        "compile_kwd": [WIKI_DOC_PAGE_SOURCE_COMPILE_KWD],
        "doc_id": [doc_id],
    }
    res = await thread_pool_exec(
        settings.docStoreConn.search,
        ["page_ids", "entity_names", "source_chunk_hashes", "map_checksum"],
        [],
        condition,
        [],
        OrderByExpr(),
        0,
        1,
        index,
        [kb_id],
    )
    field_map = settings.docStoreConn.get_fields(res, ["page_ids", "entity_names", "source_chunk_hashes", "map_checksum"])
    for row in field_map.values():
        return {
            "page_ids": json.loads(row.get("page_ids", "[]")) if isinstance(row.get("page_ids"), str) else row.get("page_ids", []),
            "entity_names": json.loads(row.get("entity_names", "[]")) if isinstance(row.get("entity_names"), str) else row.get("entity_names", []),
            "source_chunk_hashes": json.loads(row.get("source_chunk_hashes", "{}")) if isinstance(row.get("source_chunk_hashes"), str) else row.get("source_chunk_hashes", {}),
            "map_checksum": row.get("map_checksum", ""),
        }
    return None


async def _wiki_delete_doc_page_source(
    tenant_id: str,
    kb_id: str,
    doc_id: str,
) -> None:
    """Delete doc_page_source record when a document is removed."""
    index = search.index_name(tenant_id)
    await thread_pool_exec(
        settings.docStoreConn.delete,
        {
            "compile_kwd": [WIKI_DOC_PAGE_SOURCE_COMPILE_KWD],
            "doc_id": [doc_id],
        },
        index,
        kb_id,
    )


# ----- Mode A: no-plan REFINE -----------------------------------------------


async def _wiki_refine_page(
    *,
    mode: str,  # "generate" | "modify" | "re-synthesize" | "delete"
    page_id: str,
    page_title: str,
    existing_page: dict | None,
    page_type_kwd: str = "concept",
    additions: list[dict] | None = None,
    retractions: list[dict] | None = None,
    source_chunks: list[dict] | None = None,
    claims: list[dict] | None = None,
    available_pages: list[str] | None = None,
    contextual_hints: str = "",
    chat_mdl,
    embd_mdl,
    tenant_id: str,
    kb_id: str,
    page_version: int,
    entity_names: list[str] | None = None,
    page_embedding=None,
    embed_routing_context: bool = False,
    source_doc_ids: list[str] | None = None,
    topic_candidates: list[str] | None = None,
    topic_selection_stats: dict[str, int] | None = None,
    topic_embeddings: dict[str, object] | None = None,
    topic_pool: dict[str, str] | None = None,
    topic_pool_lock: asyncio.Lock | None = None,
    member_evidence: list[dict] | None = None,
) -> dict | None:
    """Run a single Mode A REFINE action on one concept page.

    Returns updated wiki_page dict, or None if deleted.
    """
    from common.misc_utils import thread_pool_exec

    # Blank page_id would produce `slug_kwd: [""]` queries → Infinity 3052.
    if not page_id or not str(page_id).strip():
        return existing_page
    page_version = _as_int(page_version)

    topic_candidates = await _wiki_rank_topic_candidates(
        page_title,
        claims,
        source_chunks,
        existing_page,
        topic_candidates,
        topic_embeddings,
        embd_mdl,
    )

    if mode == "delete":
        deleted_count = await thread_pool_exec(
            settings.docStoreConn.delete,
            {"compile_kwd": [WIKI_PAGE_COMPILE_KWD], "slug_kwd": [page_id]},
            search.index_name(tenant_id),
            kb_id,
        )
        if not isinstance(deleted_count, int) or deleted_count <= 0:
            logging.warning("wiki: page deletion did not remove page=%s", page_id)
            return existing_page
        from api.db.services.file_commit_service import FileCommitService

        commit_slug = page_id if page_id.startswith(f"{page_type_kwd}/") else f"{page_type_kwd}/{page_id}"
        FileCommitService.delete_page_history(kb_id, page_type_kwd, commit_slug)
        return None

    # WeKnora-style verbatim evidence: load the ACTUAL source-chunk text for
    # every referenced chunk id so the writer is grounded in the source, not
    # just in the condensed claim statements. This runs per-page (incremental
    # friendly) and is bounded by WIKI_SOURCE_BUDGET_CHARS.
    if source_chunks:
        chunk_ids = [sc.get("id") or sc.get("chunk_id") for sc in source_chunks if (sc.get("id") or sc.get("chunk_id"))]
        if chunk_ids:
            try:
                chunk_texts = await _wiki_load_chunk_texts(tenant_id, kb_id, [str(c) for c in chunk_ids])
                if chunk_texts:
                    source_chunks = _wiki_enrich_source_chunks(source_chunks, chunk_texts)
            except Exception:
                logging.exception("wiki: verbatim chunk enrichment failed for page %s", page_id)

    # Build the prompt based on mode
    if mode == "generate":
        system_prompt = _WIKI_MODE_A_GENERATE_SYSTEM
        user_prompt = _build_mode_a_generate_prompt(
            page_id,
            page_title,
            claims,
            source_chunks,
            available_pages,
            contextual_hints,
            topic_candidates,
            member_evidence,
        )
    elif mode == "re-synthesize":
        system_prompt = _WIKI_MODE_A_MODIFY_SYSTEM
        user_prompt = _build_mode_a_modify_prompt(
            page_id,
            page_title,
            existing_page,
            additions,
            retractions,
            claims,
            source_chunks,
            available_pages,
            contextual_hints,
            topic_candidates,
            force_full=True,
            member_evidence=member_evidence,
        )
    else:  # modify
        system_prompt = _WIKI_MODE_A_MODIFY_SYSTEM
        user_prompt = _build_mode_a_modify_prompt(
            page_id,
            page_title,
            existing_page,
            additions,
            retractions,
            claims,
            source_chunks,
            available_pages,
            contextual_hints,
            topic_candidates,
            force_full=False,
            member_evidence=member_evidence,
        )

    # Call LLM
    response = await _chat_mdl_ask(
        chat_mdl,
        system_prompt,
        user_prompt,
    )

    if not response or not response.strip():
        return existing_page  # keep existing

    # Parse response metadata. Topic is selected semantically by the same LLM
    # that planned/wrote the page; it must not be overwritten later by a
    # knowledge-base-wide embedding nearest-neighbour pass.
    content_lines = response.strip().splitlines()
    summary = ""
    title = ""
    topic = ""
    while content_lines:
        line = content_lines[0].strip()
        if not line and (summary or title or topic):
            content_lines.pop(0)
            continue
        if line.upper().startswith("SUMMARY:") and not summary:
            summary = line.split(":", 1)[1].strip()
            content_lines.pop(0)
            continue
        if line.upper().startswith("TITLE:") and not title:
            title = line.split(":", 1)[1].strip()
            content_lines.pop(0)
            continue
        if line.upper().startswith("TOPIC:") and not topic:
            topic = line.split(":", 1)[1].strip()
            content_lines.pop(0)
            continue
        break
    content = "\n".join(content_lines).strip()
    if not content:
        return existing_page

    # Build the wiki_page dict. Single-member pages keep their original title;
    # only grouped pages may receive a synthesized title from the LLM.
    existing = existing_page or {}
    member_names = {str(name).strip() for name in (entity_names or []) if str(name).strip()}
    if len(member_names) <= 1:
        title = str(existing.get("title_kwd") or page_title).strip()
    else:
        title = title or str(existing.get("title_kwd") or page_title).strip()
    if not topic:
        existing_topic = existing.get("topic_kwd")
        if isinstance(existing_topic, (list, tuple)):
            existing_topic = existing_topic[0] if existing_topic else ""
        topic = str(existing_topic or WIKI_TOPIC_FALLBACK).strip()
    topic_key = _normalize_key(topic)
    candidate_keys = {_normalize_key(candidate) for candidate in topic_candidates or [] if candidate}
    is_new_topic = bool(topic and topic_key not in candidate_keys)
    added_to_candidates = False
    if is_new_topic and topic_pool is not None:
        added_to_candidates = topic_key not in topic_pool
        if topic_pool_lock is not None:
            async with topic_pool_lock:
                added_to_candidates = topic_key not in topic_pool
                topic_pool.setdefault(topic_key, topic)
        else:
            topic_pool.setdefault(topic_key, topic)
        if topic_embeddings is not None and topic not in topic_embeddings:
            encoded, _ = await thread_pool_exec(embd_mdl.encode, [topic])
            topic_embeddings[topic] = encoded[0]
        if added_to_candidates and topic_selection_stats is not None:
            topic_selection_stats["new_added"] = topic_selection_stats.get("new_added", 0) + 1
    normalized_topic_candidates = {_normalize_key(candidate) for candidate in topic_candidates or [] if candidate}
    topic_in_candidates = _normalize_key(topic) in normalized_topic_candidates
    _wiki_log_stats(
        "TOPIC",
        "page_selection",
        page_id=page_id,
        candidate_count=len(normalized_topic_candidates),
        candidates=list((topic_candidates or [])[:WIKI_PAGE_TOPIC_CANDIDATE_LIMIT]),
        selected=topic,
        is_new=not topic_in_candidates,
        added_to_candidates=added_to_candidates,
    )
    if topic_selection_stats is not None:
        topic_selection_stats["selected"] = topic_selection_stats.get("selected", 0) + 1
        if not topic_in_candidates:
            topic_selection_stats["new"] = topic_selection_stats.get("new", 0) + 1
    new_version = page_version + 1
    raw_existing_claims = existing.get("claims", [])
    if isinstance(raw_existing_claims, str):
        try:
            raw_existing_claims = json.loads(raw_existing_claims) if raw_existing_claims else []
        except (json.JSONDecodeError, TypeError):
            raw_existing_claims = []
    existing_claims = [claim for claim in raw_existing_claims if isinstance(claim, dict)] if isinstance(raw_existing_claims, list) else []

    def _claim_key(claim: dict) -> tuple[str, str, tuple[str, ...]]:
        return (
            str(claim.get("statement") or claim.get("text") or ""),
            str(claim.get("source_doc_id") or ""),
            tuple(sorted(_wiki_claim_chunk_ids(claim))),
        )

    retraction_keys = {_claim_key(claim) for claim in (retractions or []) if isinstance(claim, dict)}
    effective_claims = [] if mode == "generate" else [claim for claim in existing_claims if _claim_key(claim) not in retraction_keys]
    seen_claims = {_claim_key(claim) for claim in effective_claims}
    for claim in list(claims or []) + list(additions or []):
        if not isinstance(claim, dict):
            continue
        key = _claim_key(claim)
        if key not in seen_claims:
            seen_claims.add(key)
            effective_claims.append(claim)
    # Claims are the authoritative provenance after applying retractions.  Do
    # not seed these fields from the old page: doing so keeps deleted or moved
    # documents attached to the page forever.
    doc_ids: list[str] = []
    source_chunk_ids: set[str] = set()
    for claim in effective_claims:
        did = claim.get("source_doc_id") if isinstance(claim, dict) else None
        if did and did not in doc_ids:
            doc_ids.append(did)
        source_chunk_ids.update(_wiki_claim_chunk_ids(claim))
    for did in source_doc_ids or []:
        if did and did not in doc_ids:
            doc_ids.append(did)
    if source_chunks:
        for chunk in source_chunks:
            cid = chunk.get("id") or chunk.get("chunk_id")
            if cid:
                source_chunk_ids.add(str(cid))
            did = chunk.get("doc_id") or chunk.get("source_doc_id")
            if did and did not in doc_ids:
                doc_ids.append(did)

    # Mode B embeds the generated page subject for subsequent routing. A
    # centroid of member vectors favors lexical similarity and loses thematic
    # relations (for example a company and a technology it develops).
    from rag.nlp import rag_tokenizer

    if page_embedding is None:
        embedding_text = summary or content[:200]
        if embed_routing_context:
            embedding_text = "; ".join(
                part
                for part in (
                    f"title={page_title}" if page_title else "",
                    f"summary={summary}" if summary else "",
                    f"members={', '.join(sorted(set(entity_names or [])))}" if entity_names else "",
                    f"content={content[:500]}" if content else "",
                )
                if part
            )
        embeddings, _ = await thread_pool_exec(embd_mdl.encode, [embedding_text])
        page_embedding = embeddings[0]

    # Derive vector dimension from the embedding shape
    emb_arr = np.asarray(page_embedding)
    vec_dim = int(emb_arr.shape[0]) if emb_arr.ndim >= 1 and emb_arr.shape[0] else 768
    content_ltks = rag_tokenizer.tokenize(content)

    page = {
        "id": _stable_row_id(WIKI_PAGE_COMPILE_KWD, kb_id, page_id),
        "slug_kwd": page_id,
        "title_kwd": title,
        "md_with_weight": content,
        "summary_with_weight": summary or title,
        "entity_names_kwd": sorted(set(entity_names or [page_title])),
        "source_chunk_ids": sorted(source_chunk_ids),
        "source_doc_ids": doc_ids,
        "claims": json.dumps(effective_claims, ensure_ascii=False) if effective_claims else "[]",
        "page_version_int": new_version,
        "synthesis_version_int": new_version if mode in ("generate", "re-synthesize") else existing.get("synthesis_version_int", 0),
        "page_type_kwd": page_type_kwd,
        "topic_kwd": topic,
        "compile_kwd": WIKI_PAGE_COMPILE_KWD,
        "knowledge_graph_kwd": WIKI_PAGE_COMPILE_KWD,
        "title_tks": rag_tokenizer.tokenize(title),
        "content_ltks": content_ltks,
        "content_sm_ltks": rag_tokenizer.fine_grained_tokenize(content_ltks),
    }
    # Insert vector (adds q_{dim}_vec field)
    vec_col = f"q_{vec_dim}_vec"
    page[vec_col] = page_embedding.tolist() if hasattr(page_embedding, "tolist") else page_embedding

    # Persist
    index = search.index_name(tenant_id)

    existing_entry = await thread_pool_exec(
        settings.docStoreConn.search,
        ["slug_kwd"],
        [],
        {"compile_kwd": [WIKI_PAGE_COMPILE_KWD], "slug_kwd": [page_id]},
        [],
        OrderByExpr(),
        0,
        1,
        index,
        [kb_id],
    )
    if settings.docStoreConn.get_fields(existing_entry, ["slug_kwd"]):
        await thread_pool_exec(
            settings.docStoreConn.update,
            {"slug_kwd": page_id},
            page,
            index,
            kb_id,
        )
    else:
        await thread_pool_exec(
            settings.docStoreConn.insert,
            [page],
            index,
            kb_id,
        )

    # Keep generated pages in the same version history as manual edits.  The
    # incremental compiler is the normal Wiki path, so without this record the
    # first generated page (and subsequent generated revisions) are invisible
    # to the commits API.
    from api.db.services.file_commit_service import FileCommitService

    content_before = ""
    if existing_page:
        content_before = existing_page.get("md_with_weight") or existing_page.get("content_with_weight") or ""
    commit_slug = page_id if page_id.startswith(f"{page_type_kwd}/") else f"{page_type_kwd}/{page_id}"
    try:
        FileCommitService.record_page_edit(
            tenant_id=tenant_id,
            kb_id=kb_id,
            page_type=page_type_kwd,
            slug=commit_slug,
            content_before=content_before,
            content_after=content,
            title="Regenerated by artifact compilation",
            comments=f"Auto-update via incremental wiki compilation (action={mode.upper()})",
            user_id=None,
        )
    except Exception:
        logging.exception("wiki: generated page version record failed for page=%s", page_id)

    return page


def _build_source_chunks_block(source_chunks: list[dict], max_budget: int = WIKI_SOURCE_BUDGET_CHARS) -> str:
    """Render the verbatim source-chunk block for the writer prompt.

    Chunks carrying verbatim text (``_verbatim=True``) are labelled clearly so
    the writer treats them as ground truth; the whole block is capped at
    ``max_budget`` characters. Missing/empty chunks are dropped.
    """
    if not source_chunks:
        return ""
    parts: list[str] = []
    total = 0
    for c in source_chunks:
        cid = c.get("id") or c.get("chunk_id")
        text = c.get("content_with_weight") or c.get("text") or ""
        if not text or not cid:
            continue
        if c.get("_verbatim"):
            block = f"[SOURCE {cid}]\n{text}"
        else:
            block = f"[CHUNK {cid}]\n{text}"
        if total + len(block) + 2 > max_budget:
            break
        parts.append(block)
        total += len(block) + 2
    if not parts:
        return ""
    if total >= max_budget:
        parts.append("[…further source chunks omitted to fit context budget…]")
    return "\n\n".join(parts)


def _build_mode_a_generate_prompt(
    page_id: str,
    page_title: str,
    claims: list[dict],
    source_chunks: list[dict],
    available_pages: list[str],
    contextual_hints: str,
    topic_candidates: list[str] | None = None,
    member_evidence: list[dict] | None = None,
) -> str:
    chunks_text = _build_source_chunks_block(source_chunks)
    claims_text = "\n".join(f"- {c.get('statement', c.get('text', ''))}" for c in claims) if claims else "(no claims)"

    member_text = _build_member_evidence_block(member_evidence)

    return f"""## Concept Page Identity
- Page ID: {page_id}
- Title: {page_title}

## Required Page Members
{member_text or "(single member page)"}

## Source Chunks (verbatim source text — ground every fact in these)
{chunks_text or "(no source chunks available)"}

## Extracted Claims (checklist)
{claims_text}

## Candidate Topics
{chr(10).join(f"- {topic}" for topic in (topic_candidates or [])[:WIKI_PAGE_TOPIC_CANDIDATE_LIMIT]) or "(none; create a short canonical topic from the page evidence)"}

## Available Pages for [[wikilinks]]
{chr(10).join(f"- {p}" for p in available_pages[:50]) if available_pages else "(none)"}

{contextual_hints}
"""


def _build_mode_a_modify_prompt(
    page_id: str,
    page_title: str,
    existing_page: dict | None,
    additions: list[dict] | None,
    retractions: list[dict] | None,
    claims: list[dict],
    source_chunks: list[dict],
    available_pages: list[str],
    contextual_hints: str,
    topic_candidates: list[str] | None = None,
    force_full: bool = False,
    member_evidence: list[dict] | None = None,
) -> str:
    existing_content = existing_page.get("md_with_weight", "") if existing_page else ""
    existing_topic = existing_page.get("topic_kwd", "") if existing_page else ""
    if isinstance(existing_topic, (list, tuple)):
        existing_topic = existing_topic[0] if existing_topic else ""
    topic_block = chr(10).join(f"- {topic}" for topic in (topic_candidates or [])[:WIKI_PAGE_TOPIC_CANDIDATE_LIMIT])
    member_text = _build_member_evidence_block(member_evidence)

    if not force_full:
        additions_text = "\n".join(f"- {c.get('statement', c.get('text', ''))}" for c in (additions or [])) if additions else "(none)"
        retractions_text = "\n".join(f"- {c.get('statement', c.get('text', ''))}" for c in (retractions or [])) if retractions else "(none)"
        chunks_text = _build_source_chunks_block(source_chunks)

        return f"""## Page Identity
- Page ID: {page_id}
- Title: {page_title}

## Required Page Members
{member_text or "(single member page)"}

## Current Page
{existing_content[:10000] if existing_content else "(empty)"}

## Current Topic
{existing_topic or "(none)"}

## Candidate Topics
{topic_block or "(none; retain the current topic when it still fits, otherwise create a short canonical topic from the page evidence)"}

## New Claims to Add
{additions_text}

## Claims to Retract
{retractions_text}

## Source Chunks for New Information (verbatim source text — ground every fact in these)
{chunks_text}

## Available Pages for [[wikilinks]]
{chr(10).join(f"- {p}" for p in available_pages[:30]) if available_pages else "(none)"}

{contextual_hints}
"""
    else:
        # Full re-synthesis: all claims + all source chunks (larger budget)
        chunks_text = _build_source_chunks_block(source_chunks, max_budget=120_000)
        claims_text = "\n".join(f"- {c.get('statement', c.get('text', ''))}" for c in claims)

        return f"""## Page Identity
- Page ID: {page_id}
- Title: {page_title}

## Required Page Members
{member_text or "(single member page)"}

## All Source Chunks (for full re-synthesis — verbatim source text)
{chunks_text or "(none)"}

## All Claims
{claims_text or "(none)"}

## Current Topic
{existing_topic or "(none)"}

## Candidate Topics
{topic_block or "(none; retain the current topic when it still fits, otherwise create a short canonical topic from the page evidence)"}

## Available Pages for [[wikilinks]]
{chr(10).join(f"- {p}" for p in available_pages[:50]) if available_pages else "(none)"}

{contextual_hints}
"""


def _build_member_evidence_block(member_evidence: list[dict] | None) -> str:
    """Render per-member evidence so grouped pages cannot silently omit members."""
    if not member_evidence:
        return ""
    blocks: list[str] = []
    for member in member_evidence:
        name = str(member.get("name") or "").strip()
        if not name:
            continue
        claims = member.get("claims") or []
        claims_text = (
            "\n".join(f"- {c.get('statement', c.get('text', ''))}" for c in claims if isinstance(c, dict) and c.get("statement", c.get("text", "")))
            or "(no extracted claims; use the member's source evidence)"
        )
        chunk_ids = ", ".join(str(cid) for cid in member.get("source_chunk_ids") or [] if cid)
        blocks.append(f"### Member: {name}\nClaims:\n{claims_text}\nSource chunk IDs: {chunk_ids or '(none)'}")
    return "\n\n".join(blocks)


# System prompts for Mode A

_WIKI_MODE_A_GENERATE_SYSTEM = """You are a wiki COMPILER. Generate a new wiki page for the given concept using the provided source chunks and extracted claims.

## LANGUAGE
Write the ENTIRE page in the SAME LANGUAGE as the source chunks. If the source chunks are written in Chinese, write the page in Chinese. Do not switch to English, and do not translate entity names (keep them verbatim: e.g. keep "张伟", do not write "Zhang Wei").

## RULES
1. CONCEPT PAGE: This is a single-concept wiki page. Organize by THEME, not by entity.
2. CROSS-DOCUMENT SYNTHESIS: Weave information from multiple sources into coherent paragraphs. Compare evidence, explain contradictions.
3. OPENING PARAGRAPH: 2-4 sentences defining the concept. Mention key entities. No heading.
4. SECTIONS: H2 headings, prose first, then sub-points if needed.
   Markdown formatting is mandatory: put every heading on its own line and separate every paragraph with a blank line.
5. WIKILINKS: Use ONLY the exact page IDs listed in "Available Pages for [[wikilinks]]" (they already carry the entity/ or concept/ prefix). Insert [[EXACT_PAGE_ID]] on first mention of a related concept/entity. NEVER invent a link target, NEVER drop the prefix, NEVER write English names.
6. DICTIONARY PREVENTION: Do NOT group content by source document. Do NOT create one section per entity. Do NOT write flat bullet lists.
7. MEMBER COVERAGE: If "Required Page Members" lists multiple members, the page MUST contain grounded factual content about EVERY listed member. Do not silently omit or replace any member. If the members are unrelated, keep them in clearly separated subsections while preserving all supported facts.

## SOURCE GROUNDING (COMPILER, not writer)
- The "Source Chunks" section contains VERBATIM source text. Stay close to the source wording — reuse the source's own sentences and facts where possible.
- Every newly added factual claim, entity, or numerical value MUST be directly supported by the provided source chunks. Do NOT invent facts, figures, dates, or relationships not present in the sources.
- Do NOT add rhetorical filler (e.g. "旨在帮助…", "designed to…", "aims to provide…") unless it appears verbatim in a source.
- If the sources disagree, present both views and add a "## Contradictions" section rather than silently picking one.

## OUTPUT
Return ONLY the complete markdown page.
First line: SUMMARY: {one-sentence description, 15-40 words}
Second line: TITLE: {a concise title covering all required page members}
Third line: TOPIC: {the best short canonical topic for this page}
Then the page content.

TITLE is the human-readable page title, not the page ID. When multiple
members are merged, synthesize a title covering the combined subject. For a
single-member page, keep the supplied title unchanged.

Choose TOPIC by understanding the page subject and evidence. Prefer a fitting
item from Candidate Topics. If none fits, create a concise topic in the source
language. Do not choose by superficial character or word overlap.
"""

_WIKI_MODE_A_MODIFY_SYSTEM = """You are a wiki editor. Update the existing page by integrating new information and removing retracted content.

## LANGUAGE
Write the ENTIRE page in the SAME LANGUAGE as the source chunks. If the source chunks are written in Chinese, write the page in Chinese. Do not switch to English, and do not translate entity names (keep them verbatim: e.g. keep "张伟", do not write "Zhang Wei").

## RULES
1. CONCEPT PAGE: This is a single-concept wiki page. Organize by THEME, not by entity.
2. CROSS-DOCUMENT SYNTHESIS: Connect new claims to existing content. Weave them into the SAME paragraphs.
3. OPENING PARAGRAPH: Should reflect the FULL updated picture.
4. WIKILINKS: Keep existing and add new [[page_id]] links where appropriate. Use ONLY the exact page IDs listed in "Available Pages for [[wikilinks]]" (they already carry the entity/ or concept/ prefix). NEVER invent a link target, NEVER drop the prefix, NEVER write English names.
5. For FULL RE-SYNTHESIS: Use all source chunks + all claims to rewrite from scratch.
6. For INCREMENTAL MODIFY: Integrate additions, remove retracted content, keep unchanged content.
7. MARKDOWN FORMATTING: Put every heading on its own line and separate every paragraph with a blank line. Do not return the whole page as one line.
8. MEMBER COVERAGE: If "Required Page Members" lists multiple members, the updated page MUST retain grounded factual content about EVERY listed member. Do not silently omit any member.

## DICTIONARY PREVENTION
- Do NOT group content by source document.
- Do NOT simply append new claims at the end.
- Do NOT create one section per entity.

## SOURCE GROUNDING (COMPILER, not writer)
- The "Source Chunks" section contains VERBATIM source text. Stay close to the source wording — reuse the source's own sentences and facts where possible.
- Every newly added factual claim, entity, or numerical value MUST be directly supported by the provided source chunks. Do NOT invent facts, figures, dates, or relationships not present in the sources.
- Do NOT add rhetorical filler (e.g. "旨在帮助…", "designed to…", "aims to provide…") unless it appears verbatim in a source.
- If new sources contradict existing page content, present both views and add a "## Contradictions / Updates" section rather than silently overwriting.

## OUTPUT
Return ONLY the complete updated markdown page.
First line: SUMMARY: {one-sentence description of what changed, 15-40 words}
Second line: TITLE: {a concise title covering all required page members}
Third line: TOPIC: {the best short canonical topic for the complete updated page}
Then the updated page content.

TITLE is the human-readable page title, not the page ID. When multiple
members are merged, rewrite the title to cover the complete updated subject.
For a single-member page, keep the supplied title unchanged.

Choose TOPIC by understanding the complete page subject and evidence. Prefer a
fitting item from Candidate Topics; retain Current Topic when it remains the
best fit. If neither fits, create a concise topic in the source language. Do
not choose by superficial character or word overlap.
"""


def _wiki_build_contextual_hints(
    page_id: str,
    existing_page: dict | None,
    all_relations: dict[str, list[dict]],
) -> str:
    """Build contextual hints prompt block from related_pages."""
    related = []
    if existing_page:
        rp = existing_page.get("related_kb_pages_kwd")
        if rp:
            if isinstance(rp, str):
                try:
                    parsed = json.loads(rp)
                    related = parsed if isinstance(parsed, list) else [rp]
                except (json.JSONDecodeError, TypeError):
                    related = [rp]
            elif isinstance(rp, list):
                related = rp
    if not related:
        for name in _as_str_list(existing_page.get("entity_names_kwd") if existing_page else None):
            related.extend(all_relations.get(name, []))
    if not related:
        return ""

    lines = ["## Context: Related Entities & Concepts", "Reference them in the opening paragraph and relevant sections:"]
    for r in related[:10]:
        if isinstance(r, dict):
            entity_name = r.get("entity_name") or r.get("name") or r.get("slug", "")
            relation = r.get("relation") or r.get("type", "related")
        else:
            entity_name = str(r or "").strip()
            relation = "related"
        if not entity_name:
            continue
        lines.append(f"- [[{entity_name}]] — {relation}")
    return "\n".join(lines)


# ----- Mode B Page Router (embedding candidates + LLM decision) ------------


def _wiki_entity_planning_text(entity: dict, *, max_claims: int = 3) -> str:
    name = str(entity.get("entity_name") or entity.get("name") or entity.get("term") or "").strip()
    aliases = ", ".join(_as_str_list(entity.get("aliases"))[:5])
    description = str(entity.get("definition_excerpt") or entity.get("description") or "").strip()
    claims = []
    for claim in (entity.get("claims") or [])[:max_claims]:
        if isinstance(claim, dict):
            statement = claim.get("statement") or claim.get("text")
            if statement:
                claims.append(str(statement))
    parts = [f"name={name}"]
    if aliases:
        parts.append(f"aliases={aliases}")
    if description:
        parts.append(f"description={description}")
    if claims:
        parts.append(f"evidence={' | '.join(claims)}")
    relations = []
    for relation in (entity.get("relations") or [])[:8]:
        if not isinstance(relation, dict):
            continue
        counterpart = relation.get("entity") or relation.get("counterpart")
        relation_type = relation.get("type") or "related"
        if counterpart:
            relations.append(f"{relation_type}: {counterpart}")
    if relations:
        parts.append(f"relations={' | '.join(relations)}")
    return "; ".join(parts)


async def _wiki_llm_partition_candidate(
    entities: list[dict],
    chat_mdl,
) -> list[list[dict]] | None:
    """Ask the LLM to partition one embedding-generated candidate community."""
    if len(entities) <= 1:
        return [entities]
    numbered = "\n".join(f"{idx}: {_wiki_entity_planning_text(entity)}" for idx, entity in enumerate(entities))
    prompt = f"""Group the following knowledge-base entities into coherent encyclopedia pages.
Each page must have one clear subject. Group entities only when a reader would naturally expect them to be explained on the same page. Do not use entity types as grouping rules because types are user-defined.

Return ONLY a JSON array of arrays of integer IDs, for example [[0, 2], [1]].
Every ID from 0 through {len(entities) - 1} must appear exactly once. A group may contain at most {PAGE_CLUSTER_HARD_MAX_SIZE} IDs.

Entities:
{numbered}"""
    response = await _chat_mdl_ask(chat_mdl, "You plan concise, semantically coherent encyclopedia pages.", prompt)
    raw_groups = _wiki_parse_json_array(response)
    if raw_groups is None:
        return None

    seen: set[int] = set()
    groups: list[list[dict]] = []
    for raw_group in raw_groups:
        if not isinstance(raw_group, list) or not raw_group or len(raw_group) > PAGE_CLUSTER_HARD_MAX_SIZE:
            return None
        indices: list[int] = []
        for raw_idx in raw_group:
            if isinstance(raw_idx, bool) or not isinstance(raw_idx, int) or raw_idx < 0 or raw_idx >= len(entities) or raw_idx in seen:
                return None
            seen.add(raw_idx)
            indices.append(raw_idx)
        groups.append([entities[idx] for idx in indices])
    if seen != set(range(len(entities))):
        return None
    return groups


async def _wiki_llm_group_entities(
    entities: list[dict],
    embeddings: list,
    chat_mdl,
    semaphore: asyncio.Semaphore | None = None,
    kb_id: str = "",
) -> list[list[dict]]:
    """Use embeddings for candidate communities and the LLM for final groups."""
    if len(entities) <= 1:
        _wiki_log_stats("PLAN", "group_summary", kb_id=kb_id, before=len(entities), after=len(entities), reduction_count=0, merged_group_count=0)
        return [entities]
    candidate_count = max(1, int(np.ceil(len(entities) / WIKI_GROUP_LLM_CANDIDATE_SIZE)))
    candidates = _wiki_cluster_entities(entities, embeddings, target_count=candidate_count)
    semaphore = semaphore or asyncio.Semaphore(WIKI_GROUP_LLM_MAX_CONCURRENT)

    async def _partition(candidate: list[dict]) -> list[list[dict]]:
        groups = None
        for attempt in range(2):
            async with semaphore:
                try:
                    groups = await _wiki_llm_partition_candidate(candidate, chat_mdl)
                except Exception:
                    logging.exception("wiki: LLM page grouping failed (attempt %s)", attempt + 1)
                    groups = None
            if groups is not None:
                break
        if groups is not None:
            merged_groups = [[str(entity.get("entity_name") or entity.get("term") or "") for entity in group] for group in groups if len(group) > 1]
            for members in merged_groups:
                _wiki_log_stats("PLAN", "llm_page_group", kb_id=kb_id, member_count=len(members), members=members)
            _wiki_log_stats(
                "PLAN", "llm_group_candidate", kb_id=kb_id, before=len(candidate), after=len(groups), reduction_count=sum(len(group) - 1 for group in groups), merged_group_count=len(merged_groups)
            )
            return groups
        # Embeddings only form the retrieval community. They must not decide
        # the final page boundary when the LLM is unavailable or invalid.
        _wiki_log_stats("PLAN", "llm_group_unresolved", kb_id=kb_id, before=len(candidate), after=len(candidate), retry_count=2)
        return [[entity] for entity in candidate]

    grouped = await asyncio.gather(*(_partition(candidate) for candidate in candidates))
    groups = [group for candidate_groups in grouped for group in candidate_groups]
    _wiki_log_stats(
        "PLAN",
        "group_summary",
        kb_id=kb_id,
        before=len(entities),
        after=len(groups),
        reduction_count=sum(len(group) - 1 for group in groups),
        merged_group_count=sum(1 for group in groups if len(group) > 1),
    )
    return groups


async def _wiki_llm_route_batches(
    route_items: list[tuple[dict, list[dict]]],
    chat_mdl,
) -> dict[int, str]:
    """Choose an existing page or NEW for each entity in bounded batches."""
    if not route_items:
        return {}
    semaphore = asyncio.Semaphore(WIKI_GROUP_LLM_MAX_CONCURRENT)

    async def _route_batch(batch: list[tuple[int, dict, list[dict]]]) -> dict[int, str]:
        lines = []
        allowed: dict[int, set[str]] = {}
        for item_id, entity, candidates in batch:
            options = []
            allowed[item_id] = {"NEW"}
            for candidate in candidates:
                page_id = candidate["page_id"]
                allowed[item_id].add(page_id)
                options.append(
                    {
                        "page": page_id,
                        "title": candidate.get("title", ""),
                        "summary": candidate.get("summary", ""),
                        "members": candidate.get("members", []),
                        "similarity": round(candidate.get("score", 0.0), 4),
                        "signals": candidate.get("signals", []),
                        "cooccurrence_count": candidate.get("cooccurrence_count", 0),
                    }
                )
            lines.append(json.dumps({"id": item_id, "entity": _wiki_entity_planning_text(entity), "options": options}, ensure_ascii=False))
        prompt = """Route each entity to the single existing encyclopedia page whose subject truly covers it, or choose NEW when none does. Similarity is candidate retrieval evidence, not proof. Prefer an existing page only when the semantic fit is clear.

Return ONLY a JSON array like [{\"id\": 0, \"page\": \"entity/example\"}, {\"id\": 1, \"page\": \"NEW\"}].

Items:
""" + "\n".join(lines)
        try:
            async with semaphore:
                response = await _chat_mdl_ask(chat_mdl, "You route entities to semantically appropriate encyclopedia pages.", prompt)
        except Exception:
            logging.exception("wiki: LLM page routing batch failed")
            return {}
        decisions = _wiki_parse_json_array(response)
        if decisions is None:
            return {}
        result: dict[int, str] = {}
        for decision in decisions:
            if not isinstance(decision, dict):
                continue
            item_id = decision.get("id")
            page_id = decision.get("page")
            if isinstance(item_id, int) and item_id in allowed and isinstance(page_id, str) and page_id in allowed[item_id]:
                result[item_id] = page_id
        return result

    indexed_items = [(item_id, entity, candidates) for item_id, (entity, candidates) in enumerate(route_items)]

    async def _run(items: list[tuple[int, dict, list[dict]]]) -> dict[int, str]:
        batches = [items[i : i + WIKI_ROUTE_LLM_BATCH_SIZE] for i in range(0, len(items), WIKI_ROUTE_LLM_BATCH_SIZE)]
        results = await asyncio.gather(*(_route_batch(batch) for batch in batches))
        return {item_id: page_id for result in results for item_id, page_id in result.items()}

    decisions = await _run(indexed_items)
    missing = [item for item in indexed_items if item[0] not in decisions]
    if missing:
        # Retry only missing/invalid items. A malformed item in one batch must
        # not make correctly routed entities pay for a full-batch retry.
        decisions.update(await _run(missing))
    return decisions


def _wiki_route_page_candidate(page_id: str, page: dict, *, score: float = 0.0) -> dict:
    title = page.get("title_kwd", "")
    if isinstance(title, (list, tuple)):
        title = title[0] if title else ""
    return {
        "score": float(score or 0.0),
        "page_id": page_id,
        "title": str(title or ""),
        "summary": str(page.get("summary_with_weight") or ""),
        "members": _as_str_list(page.get("entity_names_kwd"))[:12],
        "signals": [],
        "cooccurrence_count": 0,
    }


def _wiki_expand_route_candidates(
    entity: dict,
    dense_candidates: list[dict],
    existing_pages: dict[str, dict],
    entity_pages: dict[str, set[str]],
    chunk_pages: dict[str, set[str]],
    *,
    include_candidate_neighbors: bool = False,
) -> list[dict]:
    """Merge semantic retrieval with authoritative ownership and graph evidence."""
    candidates = {candidate["page_id"]: dict(candidate) for candidate in dense_candidates if candidate.get("page_id") in existing_pages}

    def _add(page_id: str, signal: str, *, cooccurrence_count: int = 0) -> None:
        page = existing_pages.get(page_id)
        if not page:
            return
        candidate = candidates.setdefault(page_id, _wiki_route_page_candidate(page_id, page))
        signals = set(candidate.get("signals") or [])
        signals.add(signal)
        candidate["signals"] = sorted(signals)
        candidate["cooccurrence_count"] = max(int(candidate.get("cooccurrence_count") or 0), cooccurrence_count)

    entity_name = str(entity.get("entity_name") or entity.get("term") or "").strip()
    for page_id in entity_pages.get(_normalize_key(entity_name), set()):
        _add(page_id, "current_owner")

    for relation in entity.get("relations") or []:
        if not isinstance(relation, dict):
            continue
        counterpart = str(relation.get("entity") or relation.get("counterpart") or "").strip()
        for page_id in entity_pages.get(_normalize_key(counterpart), set()):
            _add(page_id, "relation")

    cooccurrence: dict[str, int] = {}
    for chunk_id in _as_str_list(entity.get("source_chunk_ids")):
        for page_id in chunk_pages.get(chunk_id, set()):
            cooccurrence[page_id] = cooccurrence.get(page_id, 0) + 1
    for page_id, count in cooccurrence.items():
        _add(page_id, "cooccurrence", cooccurrence_count=count)

    if include_candidate_neighbors:
        initial_page_ids = list(candidates)
        for page_id in initial_page_ids:
            page = existing_pages.get(page_id, {})
            for neighbor_ref in _as_str_list(page.get("outlinks_kwd")) + _as_str_list(page.get("related_kb_pages_kwd")):
                if neighbor_ref in existing_pages:
                    _add(neighbor_ref, "candidate_neighbor")
                    continue
                for neighbor_id in entity_pages.get(_normalize_key(neighbor_ref), set()):
                    _add(neighbor_id, "candidate_neighbor")

    priority = {"current_owner": 0, "relation": 1, "cooccurrence": 2, "embedding": 3, "candidate_neighbor": 4}

    def _rank(candidate: dict) -> tuple:
        signal_rank = min((priority.get(signal, 4) for signal in candidate.get("signals") or []), default=4)
        return (signal_rank, -int(candidate.get("cooccurrence_count") or 0), -float(candidate.get("score") or 0.0), candidate["page_id"])

    return sorted(candidates.values(), key=_rank)[:PAGE_ROUTER_MAX_CANDIDATES]


async def _wiki_page_router(
    affected_entities: list[dict],
    chat_mdl,
    embd_mdl,
    tenant_id: str,
    kb_id: str,
    existing_pages: dict[str, dict] | None = None,
) -> dict[str, list[dict]]:
    """Route entities using KNN candidates followed by an LLM decision.

    Returns: {page_id: [entity_deltas]}
    - "_new_{page_id}" → new page to create
    - existing page_id → entities assigned to that page

    ``existing_pages`` is supplied by Mode B from its already-loaded page set.
    An explicitly empty dict means this is a first build, so page-index
    candidate retrieval can be skipped and entities can go straight to grouping.
    """
    from common.misc_utils import thread_pool_exec
    from rag.nlp import search
    from common.doc_store.doc_store_base import OrderByExpr

    query_texts = [_entity_to_query_text(e) for e in affected_entities]
    embeddings, _ = await thread_pool_exec(embd_mdl.encode, query_texts)
    for entity, vec in zip(affected_entities, embeddings, strict=False):
        entity["_embedding"] = vec

    index = search.index_name(tenant_id)
    condition = {"compile_kwd": [WIKI_PAGE_COMPILE_KWD]}

    assignments: dict[str, list[dict]] = {}
    orphans: list[dict] = []
    embedding_by_entity_id = {id(entity): vec for entity, vec in zip(affected_entities, embeddings, strict=False)}

    existing_pages = existing_pages or {}
    entity_pages: dict[str, set[str]] = {}
    chunk_pages: dict[str, set[str]] = {}
    for page_id, page in existing_pages.items():
        for entity_name in _as_str_list(page.get("entity_names_kwd")):
            entity_pages.setdefault(_normalize_key(entity_name), set()).add(page_id)
        for chunk_id in _as_str_list(page.get("source_chunk_ids")):
            chunk_pages.setdefault(chunk_id, set()).add(page_id)

    if not existing_pages:
        # On a first build there are no pages that can accept a routed entity.
        # Querying the page index once per entity only produces orphans, so go
        # directly to clustering and reuse the embeddings already computed.
        orphans = list(affected_entities)
        _wiki_log_stats("ROUTE", "summary", affected=len(affected_entities), llm_existing=0, llm_new=0, llm_missing=0, new_confirmed_existing=0, final_new=len(orphans))
    else:
        router_sem = asyncio.Semaphore(PAGE_ROUTER_KNN_CONCURRENT)

        async def _search_page(entity: dict, vec) -> tuple[dict, dict]:
            async with router_sem:
                match_expr = MatchDenseExpr(
                    vector_column_name=f"q_{len(vec)}_vec",
                    embedding_data=vec.tolist() if hasattr(vec, "tolist") else vec,
                    embedding_data_type="float",
                    distance_type="cosine",
                    topn=PAGE_ROUTER_TOP_K,
                    extra_options={"similarity": PAGE_ROUTER_MAYBE_THRESHOLD},
                )
                res = await thread_pool_exec(
                    settings.docStoreConn.search,
                    ["slug_kwd", "title_kwd", "summary_with_weight", "entity_names_kwd", "_score"],
                    [],
                    condition,
                    [match_expr],
                    OrderByExpr(),
                    0,
                    PAGE_ROUTER_TOP_K,
                    index,
                    [kb_id],
                )
                return entity, settings.docStoreConn.get_fields(res, ["slug_kwd", "title_kwd", "summary_with_weight", "entity_names_kwd", "_score"])

        route_results = await asyncio.gather(*(_search_page(entity, vec) for entity, vec in zip(affected_entities, embeddings, strict=False)))
        route_items: list[tuple[dict, list[dict]]] = []
        for entity, field_map in route_results:
            if entity.get("action") == "delete":
                assignments.setdefault("_deleted", []).append(entity)
                continue
            candidates = []
            for row in (field_map or {}).values():
                score = float(row.get("_score", 0.0) or 0.0)
                page_id = row.get("slug_kwd", "")
                if isinstance(page_id, (list, tuple)):
                    page_id = page_id[0] if page_id else ""
                page_id = str(page_id or "").strip()
                if page_id:
                    title = row.get("title_kwd", "")
                    if isinstance(title, (list, tuple)):
                        title = title[0] if title else ""
                    candidate = _wiki_route_page_candidate(page_id, existing_pages.get(page_id, row), score=score)
                    candidate["signals"] = ["embedding"]
                    candidates.append(candidate)
            candidates = _wiki_expand_route_candidates(entity, candidates, existing_pages, entity_pages, chunk_pages)
            if not candidates:
                orphans.append(entity)
                continue
            route_items.append((entity, candidates))

        try:
            decisions = await _wiki_llm_route_batches(route_items, chat_mdl)
        except Exception:
            logging.exception("wiki: LLM page routing failed")
            decisions = {}
        first_new_count = sum(1 for page_id in decisions.values() if page_id == "NEW")
        first_existing_count = sum(1 for page_id in decisions.values() if page_id != "NEW")
        missing_count = len(route_items) - len(decisions)
        second_pass_items: list[tuple[int, dict, list[dict]]] = []
        confirmed_existing_count = 0
        for item_id, (entity, candidates) in enumerate(route_items):
            page_id = decisions.get(item_id)
            if page_id and page_id != "NEW":
                assignments.setdefault(page_id, []).append(entity)
                continue
            if page_id == "NEW":
                expanded = _wiki_expand_route_candidates(
                    entity,
                    candidates,
                    existing_pages,
                    entity_pages,
                    chunk_pages,
                    include_candidate_neighbors=True,
                )
                original_ids = {candidate["page_id"] for candidate in candidates}
                added_ids = [candidate["page_id"] for candidate in expanded if candidate["page_id"] not in original_ids]
                _wiki_log_stats(
                    "ROUTE",
                    "new_confirmation_candidates",
                    entity=str(entity.get("entity_name") or entity.get("term") or ""),
                    initial_candidate_count=len(candidates),
                    added_candidate_count=len(added_ids),
                    added_to_candidates=bool(added_ids),
                    added_page_ids=added_ids,
                    confirmation_candidate_count=len(expanded),
                )
                second_pass_items.append((item_id, entity, expanded))
                continue
            owner = next((candidate for candidate in candidates if "current_owner" in candidate.get("signals", [])), None)
            if owner:
                assignments.setdefault(owner["page_id"], []).append(entity)
            else:
                orphans.append(entity)

        if second_pass_items:
            confirmation_items = [(entity, candidates) for _, entity, candidates in second_pass_items]
            confirmations = await _wiki_llm_route_batches(confirmation_items, chat_mdl)
            for confirmation_id, (_, entity, candidates) in enumerate(second_pass_items):
                page_id = confirmations.get(confirmation_id)
                if page_id and page_id != "NEW":
                    assignments.setdefault(page_id, []).append(entity)
                    confirmed_existing_count += 1
                    continue
                owner = next((candidate for candidate in candidates if "current_owner" in candidate.get("signals", [])), None)
                if owner and page_id is None:
                    assignments.setdefault(owner["page_id"], []).append(entity)
                else:
                    orphans.append(entity)
        _wiki_log_stats(
            "ROUTE",
            "summary",
            affected=len(affected_entities),
            llm_existing=first_existing_count,
            llm_new=first_new_count,
            llm_missing=missing_count,
            new_confirmed_existing=confirmed_existing_count,
            final_new=len(orphans),
        )

    # Orphans: cluster by similarity, create grouped pages
    # A deletion that cannot be routed to an existing page must not create a
    # new page merely so the downstream delete action can remove it again.
    orphans = [entity for entity in orphans if entity.get("action") != "delete"]
    if orphans:
        orphan_embs = [embedding_by_entity_id[id(entity)] for entity in orphans]
        clusters = await _wiki_llm_group_entities(orphans, orphan_embs, chat_mdl, kb_id=kb_id)
        used_page_ids = set(existing_pages) | {key[5:] for key in assignments if key.startswith("_new_")}
        for cluster in clusters:
            representative = min(
                cluster,
                key=lambda entity: (-len(entity.get("claims") or []), str(entity.get("entity_name") or entity.get("term", "")).casefold(), str(entity.get("entity_name") or entity.get("term", ""))),
            )
            cluster = [representative] + [entity for entity in cluster if entity is not representative]
            names = [e.get("entity_name") or e.get("term", "") for e in cluster]
            # Mode B compiles EVERY entity/concept into a page. On a first build
            # there are no existing pages, so every affected entity lands here as
            # an orphan; do NOT gate page creation on a claim count or most pages
            # (esp. claim-light concepts/entities) would never be created.
            if not names:
                continue
            # Mode B pages are semantic groups, not projections of a user-defined
            # entity type. Keep one neutral page namespace for every cluster.
            base_page_id = _wiki_derive_page_id(names[0], prefix="entity")
            if not base_page_id:
                continue
            page_id = base_page_id
            suffix = 2
            while page_id in used_page_ids:
                page_id = f"{base_page_id}-{suffix}"
                suffix += 1
            used_page_ids.add(page_id)
            assignments[f"_new_{page_id}"] = cluster

    return assignments


def _wiki_cluster_entities(
    entities: list[dict],
    embeddings: list,
    target_count: int | None = None,
) -> list[list[dict]]:
    """Deterministic capacity-constrained spherical k-means.

    Absolute cosine thresholds intentionally do not decide the number of pages:
    their score distributions vary too much between embedding models.
    ``target_count`` is a soft page-count target and defaults to roughly one
    page per three entities, bounded to 8..60 for larger sets.
    """
    if len(entities) <= 1:
        return [entities]

    matrix = np.asarray([np.asarray(e, dtype=np.float32) for e in embeddings], dtype=np.float32)
    if matrix.ndim != 2 or matrix.shape[0] != len(entities):
        raise ValueError("entity embeddings must be a two-dimensional matrix")
    matrix = _wiki_normalize_rows(matrix)
    n = len(entities)
    if target_count is None:
        if n <= PAGE_CLUSTER_MIN_PAGES:
            target_count = n
        else:
            target_count = max(PAGE_CLUSTER_MIN_PAGES, min(PAGE_CLUSTER_MAX_PAGES, round(n / PAGE_CLUSTER_ITEMS_PER_PAGE)))
    target_count = max(1, min(int(target_count), n))

    names = [str(entity.get("entity_name") or entity.get("name") or entity.get("term") or "") for entity in entities]
    evidence = [len(entity.get("claims") or []) for entity in entities]
    stable_order = sorted(range(n), key=lambda idx: (names[idx].casefold(), names[idx], idx))

    # Deterministic farthest-first initialization. The most grounded entity is
    # the first center; every later center is the point least represented by
    # the centers already chosen.
    first = min(range(n), key=lambda idx: (-evidence[idx], names[idx].casefold(), names[idx], idx))
    center_indices = [first]
    selected = {first}
    while len(center_indices) < target_count:
        similarities = matrix @ matrix[center_indices].T
        nearest = np.max(similarities, axis=1)
        candidate = min(
            (idx for idx in stable_order if idx not in selected),
            key=lambda idx: (float(nearest[idx]), names[idx].casefold(), names[idx], idx),
        )
        center_indices.append(candidate)
        selected.add(candidate)

    centroids = matrix[center_indices].copy()
    previous_assignments: list[int] | None = None
    assignments = [0] * n
    hard_capacity = max(PAGE_CLUSTER_HARD_MAX_SIZE, int(np.ceil(n / target_count)))

    for _ in range(PAGE_CLUSTER_MAX_ITERATIONS):
        scores = matrix @ centroids.T
        sizes = [0] * target_count
        assignments = [-1] * n
        # Place entities with a strong preference first, so capacity pressure
        # moves ambiguous entities rather than a cluster's clearest members.
        ranked_entities = sorted(
            stable_order,
            key=lambda idx: (
                -float(np.max(scores[idx]) - np.partition(scores[idx], -2)[-2]) if target_count > 1 else -float(scores[idx, 0]),
                names[idx].casefold(),
                names[idx],
                idx,
            ),
        )
        for idx in ranked_entities:
            ranked_clusters = sorted(range(target_count), key=lambda cid: (-float(scores[idx, cid]), cid))
            chosen = next((cid for cid in ranked_clusters if sizes[cid] < hard_capacity), ranked_clusters[0])
            assignments[idx] = chosen
            sizes[chosen] += 1

        # Empty clusters are repaired by moving the least well represented
        # member from a cluster that can spare one.
        for empty_cid in (cid for cid, size in enumerate(sizes) if size == 0):
            movable = [idx for idx in stable_order if sizes[assignments[idx]] > 1]
            if not movable:
                break
            moved = min(movable, key=lambda idx: (float(scores[idx, assignments[idx]]), names[idx].casefold(), names[idx], idx))
            sizes[assignments[moved]] -= 1
            assignments[moved] = empty_cid
            sizes[empty_cid] = 1

        new_centroids = []
        for cid in range(target_count):
            member_indices = [idx for idx, assigned in enumerate(assignments) if assigned == cid]
            centroid = np.mean(matrix[member_indices], axis=0)
            norm = np.linalg.norm(centroid)
            new_centroids.append(centroid / norm if norm > 0 else centroids[cid])
        new_centroids = np.asarray(new_centroids, dtype=np.float32)
        movement = float(np.max(np.linalg.norm(new_centroids - centroids, axis=1)))
        centroids = new_centroids
        if assignments == previous_assignments or movement < PAGE_CLUSTER_CONVERGENCE_EPSILON:
            break
        previous_assignments = list(assignments)

    clusters = []
    for cid in range(target_count):
        member_indices = [idx for idx in stable_order if assignments[idx] == cid]
        if member_indices:
            clusters.append([entities[idx] for idx in member_indices])
    clusters.sort(key=lambda cluster: (str(cluster[0].get("entity_name") or "").casefold(), str(cluster[0].get("entity_name") or "")))
    return clusters


# ----- FINALIZE (shared) ----------------------------------------------------


async def _wiki_finalize(
    tenant_id: str,
    kb_id: str,
    embd_mdl,
    page_ids: list[str] | None = None,
    chunk_state: dict[str, dict] | None = None,
) -> None:
    """Post-REFINE cleanup: dead wikilinks + cross-reference update.

    Always scans ALL wiki_page rows (page_ids is ignored — full scan).
    Three wikilink types:
    1. Valid page → keep [[]], update related_kb_pages_kwd
    2. Entity reference (in canonical index) → remove [[]], keep plain text
    3. Dead link → remove [[]], keep plain text
    """
    all_pages = await _search_existing_pages(
        tenant_id,
        kb_id,
        [
            "slug_kwd",
            "id",
            "title_kwd",
            "md_with_weight",
            "outlinks_kwd",
            "related_kb_pages_kwd",
            "entity_names_kwd",
        ],
    )
    if not all_pages:
        return

    valid_ids = set(all_pages.keys())

    # Load canonical entity names for entity reference detection (Mode A)
    canonical_names = set()
    canonical_index = await _load_canonical_entities(tenant_id, kb_id)
    for cname in canonical_index:
        canonical_names.add(cname)
        # Also add aliases
        for alias in canonical_index[cname].get("aliases", []):
            canonical_names.add(alias)

    wikilink_re = re.compile(r"\[\[([^\]]+)\]\]")
    relation_map: dict[str, list[dict]] = {}
    outlink_map: dict[str, list[str]] = {}  # pid → [valid target slugs]
    dead_links: dict[str, list[str]] = {}  # pid → [dead links to remove]
    index = search.index_name(tenant_id)

    # name → page slug map for AUTO-LINKING. Built from every page's plain name
    # (slug suffix) + title, longest names first so multi-word / multi-char
    # mentions are matched greedily before shorter substrings.
    name_slug: dict[str, str] = {}
    for pid in all_pages:
        plain = pid.split("/")[-1] if "/" in pid else pid
        if plain:
            name_slug[plain] = pid
        title = all_pages[pid].get("title_kwd")
        if isinstance(title, (list, tuple)):
            title = title[0] if title else ""
        if isinstance(title, str) and title and title != plain:
            name_slug[title] = pid
        # Map every entity the page actually contains (incl. ones merged into a
        # group page by Mode B's plan/grouping) to this page. Otherwise relation
        # endpoints like "梁大伟" that were merged into another page won't match
        # and most wiki pages stay unlinked.
        for en in _as_str_list(all_pages[pid].get("entity_names_kwd")):
            if en and en != plain:
                name_slug[en] = pid
    ordered_names = sorted(name_slug.keys(), key=lambda n: (-len(n), n))

    # RELATION-BASED LINKING: use the semantic (from, to) edges extracted during
    # MAP to connect pages. A page-to-page edge is created whenever both
    # endpoints resolve to compiled wiki pages. This is the primary source of
    # graph connections — far more reliable than prose [[wikilinks]].
    from api.db.services.document_service import DocumentService

    disabled_doc_ids = await thread_pool_exec(DocumentService.get_disabled_doc_ids_by_kb_id, kb_id)
    map_relations = await _load_map_relations(
        tenant_id,
        kb_id,
        excluded_doc_ids=disabled_doc_ids,
        chunk_state=chunk_state,
    )
    relation_edges: dict[str, set[str]] = {}  # pid → {target slug}
    if map_relations:
        for rel in map_relations:
            from_pg = name_slug.get(rel["from"])
            to_pg = name_slug.get(rel["to"])
            if from_pg and to_pg and from_pg != to_pg:
                relation_edges.setdefault(from_pg, set()).add(to_pg)
                relation_edges.setdefault(to_pg, set()).add(from_pg)

    for pid, page in all_pages.items():
        content = page.get("md_with_weight", "")
        original = content

        for match in wikilink_re.finditer(content):
            link = match.group(1).strip()
            # Keep an explicit display label when a page contains several
            # merged entities.  The page slug identifies the destination;
            # it must not replace the entity name shown in the prose.
            target, separator, display_text = link.partition("|")
            target = target.strip()
            display_text = display_text.strip() if separator else ""
            if target in valid_ids and target != pid:
                # Valid wikilink → record for cross-reference + outlink
                relation_map.setdefault(pid, []).append(
                    {
                        "entity_name": display_text or (target.split("/")[-1] if "/" in target else target),
                        "relation": "see_also",
                    }
                )
                outlink_map.setdefault(pid, []).append(target)
            elif target in canonical_names:
                # Entity reference (Mode A): remove [[]] keep plain text
                replacement = display_text or target
                content = content.replace(match.group(0), replacement, 1)
            else:
                # Dead link — try WeKnora-style fuzzy resolution to a similar
                # existing page before giving up. If a close slug exists, retarget
                # the link (cross-link survives); otherwise degrade to plain text.
                resolved = _wiki_resolve_dead_slug(target, valid_ids, name_slug)
                if resolved:
                    resolved_link = f"[[{resolved}|{display_text}]]" if display_text else f"[[{resolved}]]"
                    content = content.replace(match.group(0), resolved_link, 1)
                    relation_map.setdefault(pid, []).append({"entity_name": display_text or (resolved.split("/")[-1] if "/" in resolved else resolved), "relation": "see_also"})
                    if resolved not in outlink_map.setdefault(pid, []):
                        outlink_map[pid].append(resolved)
                else:
                    content = content.replace(match.group(0), display_text or target, 1)
                    dead_links.setdefault(pid, []).append(target)

        # AUTO-LINK: guarantee cross-page connections even when the LLM omits
        # [[...]]. Scan for standalone mentions of other pages' plain names and
        # wrap the FIRST occurrence with [[full_slug]], recording an outlink.
        # existing_links covers both the raw [[slug]] form and the rendered
        # `[text](artifact/{kb_id}/slug)` form (so re-runs stay idempotent even
        # after links were rendered on a previous run).
        existing_links = {m.group(1).split("|", 1)[0].strip() for m in wikilink_re.finditer(content)}
        existing_links |= {m.group(1) for m in re.finditer(rf"\]\(artifact/{re.escape(str(kb_id))}/([^)]+)\)", content)}
        for name in ordered_names:
            target = name_slug[name]
            if target == pid:
                continue
            if target in existing_links:
                continue
            idx = _wiki_find_unlinked_mention(content, name)
            if idx < 0:
                continue
            # Preserve the matched prose as the link label.  This matters when
            # several entities share one page: e.g. the page slug may be
            # ``entity/五色棒`` while the text mentions
            # ``治世之能臣，乱世之奸雄``.
            content = content[:idx] + f"[[{target}|{name}]]" + content[idx + len(name) :]
            existing_links.add(target)
            if target not in outlink_map.setdefault(pid, []):
                outlink_map[pid].append(target)
            relation_map.setdefault(pid, []).append({"entity_name": name, "relation": "see_also"})

        # Merge semantic relation edges (from MAP extraction) into the outlinks.
        # These connect pages even when prose never mentions the counterpart.
        # We ALSO inject a "相关页面 / Related" [[wikilink]] section so the
        # cross-page link is visible in the body and survives the shredded
        # *_kwd field (outlinks_kwd is analyzed by whitespace-# → reads back []).
        rel_targets = []
        for target in relation_edges.get(pid, ()):
            if target == pid:
                continue
            if target not in outlink_map.setdefault(pid, []):
                outlink_map[pid].append(target)
                target_name = target.split("/")[-1] if "/" in target else target
                relation_map.setdefault(pid, []).append({"entity_name": target_name, "relation": "related"})
            if target not in existing_links:
                rel_targets.append(target)
                existing_links.add(target)
        if rel_targets:
            if not content.rstrip().endswith("## 相关页面"):
                content = content.rstrip() + "\n\n## 相关页面\n"
            content += "\n".join(f"- [[{t}]]" for t in rel_targets) + "\n"

        # Render internal [[slug]] wikilinks into clickable Markdown links
        # `[text](artifact/{kb_id}/{slug})` — the format the frontend's wiki
        # viewer can navigate (the raw [[...]] form renders as plain text and
        # cannot be deep-linked). Only slugs that resolve to a compiled page
        # become links; anything else is left as plain text.
        rendered_content = content
        if rendered_content:
            # valid_slugs must be the set of LINK TARGETS (outlink values), not
            # the source page keys. Build it from every outlink_map value plus
            # every page's own slug so self-references resolve too.
            link_targets = set(outlink_map.keys())
            for _, targets in outlink_map.items():
                link_targets.update(targets)
            rendered_content = _wiki_render_links(rendered_content, kb_id, link_targets)

        # Update page content if changed (store the RENDERED markdown so the
        # frontend gets navigable artifact links).
        update = {}
        if rendered_content != original:
            update["md_with_weight"] = rendered_content

        relations = relation_map.get(pid, [])
        if relations:
            # NOTE: *_kwd fields are analyzed by whitespace-#. Storing a
            # json.dumps string makes Infinity shred it so get_fields reads it
            # back as [] (breaking every consumer of related_kb_pages_kwd).
            # Mirror the old-mode format: a native list of STRINGS (Infinity
            # stores it and get_fields/_as_str_list restore it as a list). The
            # old-mode related_kb_pages is exactly a list of page slugs/names,
            # so we collapse each {entity_name, relation} entry to its string.
            update["related_kb_pages_kwd"] = [r.get("entity_name") or r.get("slug") or str(r) for r in relations[:20]]
        elif page.get("related_kb_pages_kwd"):
            # Clear stale related_pages if no relations remain
            update["related_kb_pages_kwd"] = []

        # Always refresh outlinks: unique valid targets (graph + list ordering
        # depend on outlinks_int). Preserves ordering for a stable canvas.
        # Store a native list (NOT json.dumps) so the *_kwd field reads back
        # correctly instead of being shredded into [] by whitespace-#.
        outlinks = outlink_map.get(pid) or []
        update["outlinks_kwd"] = list(outlinks)
        update["outlinks_int"] = len(outlinks)

        await thread_pool_exec(
            settings.docStoreConn.update,
            {"id": page["id"]},
            update,
            index,
            kb_id,
        )

    # FINALIZE updates every page without forcing a refresh per write. Make the
    # complete batch searchable once here, before the caller reloads these rows
    # to materialize wiki_entity/wiki_relation. Without this barrier, the page
    # API can expose the new outlinks while the graph is built from the previous
    # search snapshot (for example, a linked page still gets weight=0/no edges).
    refresh_idx = getattr(settings.docStoreConn, "refresh_idx", None)
    if callable(refresh_idx):
        await thread_pool_exec(refresh_idx, index)


def _wiki_normalize_rows(matrix):
    """L2-normalize the rows of a 2-D float matrix (safe on zero rows)."""
    if matrix.ndim != 2:
        return matrix
    norms = np.linalg.norm(matrix, axis=1, keepdims=True)
    return np.divide(matrix, norms, out=np.zeros_like(matrix), where=norms > 0)


# ----- Main entry point -----------------------------------------------------


async def wiki_compile_incremental(
    *,
    chat_mdl,
    embd_mdl,
    tenant_id: str,
    kb_id: str,
    mode: str,
    chunk_delta: dict[str, set[str]],
    previous_chunk_state: dict[str, dict],
    current_chunk_state: dict[str, dict],
    incremental: bool = False,  # True = incremental run
    deleted_doc_ids: set[str] | None = None,
    callback: Callable | None = None,
) -> dict:
    """Main entry point for dual-mode wiki compilation.

    Args:
        mode: ``entity`` for Mode A, or ``topic`` for Mode B.
        incremental: True=incremental update, False=full build
        deleted_doc_ids: Documents that were removed.
        callback: Progress callback.

    Returns summary dict: {pages_created, pages_modified, pages_deleted}
    """
    from common.misc_utils import thread_pool_exec
    from rag.nlp import search

    summary = {"pages_created": 0, "pages_modified": 0, "pages_deleted": 0, "errors": []}

    def _progress(msg: str):
        if callback:
            try:
                callback(0.5, msg)
            except Exception:
                pass

    invalidated_chunk_ids = set(chunk_delta.get("changed_chunk_ids") or set()) | set(chunk_delta.get("deleted_chunk_ids") or set())
    delta_current_chunk_ids = set(chunk_delta.get("new_chunk_ids") or set()) | set(chunk_delta.get("changed_chunk_ids") or set())
    delta_before_results: list[dict] = []
    delta_after_results: list[dict] = []

    # Versioned MAP storage contains historical rows.  Incremental compilation
    # must select exactly the versions referenced by the candidate current
    # state, never every historical row in the index.
    from rag.advanced_rag.knowlege_compile.wiki import _wiki_load_map_extracts_for_state

    map_results = await _wiki_load_map_extracts_for_state(tenant_id, kb_id, current_chunk_state)
    if delta_current_chunk_ids:
        delta_after_results = await _wiki_load_map_extracts_for_state(
            tenant_id,
            kb_id,
            current_chunk_state,
            delta_current_chunk_ids,
        )
    if invalidated_chunk_ids:
        delta_before_results = await _wiki_load_map_extracts_for_state(
            tenant_id,
            kb_id,
            previous_chunk_state,
            invalidated_chunk_ids,
        )

    if not map_results and not delta_before_results:
        _progress("No MAP results found. Skipping wiki compilation.")
        return summary

    # ----- Correct incremental flag for interrupted first builds -----
    # If the caller believes this is incremental but the KB has NO wiki_page
    # rows, it means a previous full build was cancelled mid-way, leaving only
    # half-written wiki_map_extract rows.  Incremental logic relies on existing
    # pages + doc_page_source to route changes; without any compiled page it
    # would try to diff against nothing and produce empty output.  In that case
    # fall back to a full build.
    if incremental:
        try:
            has_pages = await _wiki_has_any_pages(tenant_id, kb_id)
        except Exception:
            logging.exception("wiki: failed to check existing pages; assuming first build")
            has_pages = False
        if not has_pages:
            _progress("No compiled wiki pages found; treating as first build (previous build was interrupted).")
            incremental = False

    # ----- Phase 2: Entity Matching -----
    _progress("Entity Matching: deduplicating entities and concepts ...")

    # Lightweight metadata for matching + separate claim index for on-demand
    # claim loading.  Keeps Entity Matching operating on small metadata only
    # (mirrors old-mode dedup); full claim text is loaded per-affected-name
    # after matching, so peak memory stays bounded.
    map_results = map_results or []
    raw_entities, claim_index = _extract_raw_entities(map_results)
    before_raw_entities, _before_claim_index = _extract_raw_entities(delta_before_results)
    before_raw_names = {entry.get("name") for entry in before_raw_entities if entry.get("name")}
    after_raw_entities, _after_claim_index = _extract_raw_entities(delta_after_results)
    after_raw_names = {entry.get("name") for entry in after_raw_entities if entry.get("name")}

    # Preserve MAP topic provenance so each page's writer chooses among topics
    # extracted from that page's own source documents, rather than from an
    # unrelated knowledge-base-wide label pool.
    doc_topics: dict[str, list[str]] = {}
    raw_topic_count = 0
    raw_relations: list[dict] = []
    for _mr in map_results:
        _doc_id = str(_mr.get("doc_id") or "").strip()
        if not _doc_id:
            continue
        _seen_topics: set[str] = set()
        for _t in _mr.get("topics") or []:
            if isinstance(_t, str):
                raw_topic_count += 1
                _t = _t.strip()
                _k = _t.casefold()
                if _t and _k not in _seen_topics:
                    _seen_topics.add(_k)
                    doc_topics.setdefault(_doc_id, []).append(_t)
        for _relation in _mr.get("relations") or []:
            if isinstance(_relation, str):
                try:
                    _relation = json.loads(_relation)
                except (json.JSONDecodeError, TypeError):
                    continue
            if not isinstance(_relation, dict):
                continue
            _from = _relation.get("from")
            _to = _relation.get("to")
            if isinstance(_from, str) and isinstance(_to, str) and _from and _to:
                raw_relations.append({"from": _from, "to": _to, "type": _relation.get("type") or "related"})

    unique_topics = sorted({_t for _topics in doc_topics.values() for _t in _topics}, key=lambda value: (value.casefold(), value))
    _wiki_log_stats(
        "TOPIC",
        "map_summary",
        document_count=len(doc_topics),
        raw_count=raw_topic_count,
        unique_count=len(unique_topics),
        topics=unique_topics,
    )

    # Release the heavy raw MAP payload as early as possible.  All metadata is
    # now in raw_entities and full claim text in claim_index; keeping
    # map_results alive would pin the raw MAP structures in memory.
    del map_results

    canonical_entities = await _load_canonical_entities(tenant_id, kb_id)

    canonical_map, name_resolution = await _wiki_match_entities(
        raw_entities=raw_entities,
        existing_canonical=canonical_entities,
        embd_mdl=embd_mdl,
        chat_mdl=chat_mdl,
        tenant_id=tenant_id,
        kb_id=kb_id,
        incremental=incremental,
    )
    canonical_resolution = dict(name_resolution)
    for canonical_name, canonical_entry in canonical_entities.items():
        canonical_resolution.setdefault(canonical_name, canonical_name)
        for alias in canonical_entry.get("aliases") or []:
            if isinstance(alias, str) and alias:
                canonical_resolution.setdefault(alias, canonical_name)
    entity_relations: dict[str, list[dict]] = {}
    seen_relations: set[tuple[str, str, str]] = set()
    for relation in raw_relations:
        source = canonical_resolution.get(relation["from"], relation["from"])
        target = canonical_resolution.get(relation["to"], relation["to"])
        relation_type = str(relation.get("type") or "related")
        if not source or not target or source == target:
            continue
        for owner, counterpart in ((source, target), (target, source)):
            key = (owner, counterpart, relation_type)
            if key in seen_relations:
                continue
            seen_relations.add(key)
            entity_relations.setdefault(owner, []).append({"entity": counterpart, "type": relation_type})
    del raw_relations
    del canonical_resolution

    # ``map_results`` is the complete current snapshot when chunk deltas are
    # supplied.  Replace provenance accumulated in the canonical cache with
    # that snapshot so changed/deleted chunk ids do not survive indefinitely.
    current_evidence: dict[str, dict[str, set[str] | int]] = {}
    for entry in raw_entities:
        cname = name_resolution.get(entry["name"], entry["name"])
        evidence = current_evidence.setdefault(cname, {"docs": set(), "chunks": set(), "claims": 0})
        evidence["docs"].update(entry.get("source_doc_ids") or [])
        evidence["chunks"].update(entry.get("source_chunk_ids") or [])
        evidence["claims"] += int(entry.get("claim_count") or 0)
    for cname, centry in canonical_map.items():
        evidence = current_evidence.get(cname)
        if evidence is None:
            continue
        centry["source_doc_ids"] = sorted(evidence["docs"])
        centry["source_chunk_ids"] = sorted(evidence["chunks"])
        centry["claim_count"] = evidence["claims"]

    # raw_entities (lightweight) no longer needed after matching.
    del raw_entities

    if not canonical_map and not before_raw_names:
        _progress("Entity Matching: no canonical entities found. Skipping.")
        return summary

    _progress("Entity Matching: %d" % len(name_resolution.keys()))
    # Persist new/changed canonical entities.
    # Compute embeddings for ALL new entities in ONE batch call (the embedding
    # API is invoked per text by the underlying driver — batching 95 entities as
    # one call is ~95x faster than a per-entity call, each of which only carries
    # a handful of tokens and spends ~0.3s in round-trip latency).
    changed_items: list[tuple[str, dict]] = []
    new_items: list[tuple[str, dict, str]] = []  # (cname, centry, emb_text)
    for cname, centry in canonical_map.items():
        existing = canonical_entities.get(cname)
        if existing:
            old_docs = set(k for k in (existing.get("source_doc_ids") or []))
            new_docs = set(centry.get("source_doc_ids", []))
            old_chunks = set(existing.get("source_chunk_ids") or [])
            new_chunks = set(centry.get("source_chunk_ids", []))
            old_aliases = set(existing.get("aliases") or [])
            new_aliases = set(centry.get("aliases") or [])
            if old_docs != new_docs or old_chunks != new_chunks or old_aliases != new_aliases or centry["claim_count"] > existing.get("mention_count_int", 0):
                # Only persist when data changes; reuse the existing embedding.
                changed_items.append((cname, centry))
        else:
            new_items.append((cname, centry, _entity_to_query_text(centry)))

    if changed_items:
        persist_sem = asyncio.Semaphore(CANONICAL_PERSIST_CONCURRENT)

        async def _update_changed(item: tuple[str, dict]) -> None:
            cname, centry = item
            async with persist_sem:
                await _update_canonical_entity(
                    tenant_id,
                    kb_id,
                    cname,
                    centry["type"],
                    centry.get("aliases", []),
                    centry.get("source_doc_ids", []),
                    centry["claim_count"],
                    source_chunk_ids=centry.get("source_chunk_ids", []),
                )

        await asyncio.gather(*(_update_changed(item) for item in changed_items))

    if new_items and embd_mdl:
        batch_texts = [t for _, _, t in new_items]
        batch_embs, _ = await thread_pool_exec(embd_mdl.encode, batch_texts)
        new_rows = [
            _build_canonical_entity_doc(
                tenant_id,
                kb_id,
                cname,
                centry["type"],
                centry.get("aliases", []),
                centry.get("source_doc_ids", []),
                centry["claim_count"],
                embedding=emb.tolist() if hasattr(emb, "tolist") else emb,
                source_chunk_ids=centry.get("source_chunk_ids", []),
            )
            for (cname, centry, _), emb in zip(new_items, batch_embs, strict=False)
        ]
        await thread_pool_exec(
            settings.docStoreConn.insert,
            new_rows,
            search.index_name(tenant_id),
            kb_id,
        )
    elif new_items:
        new_rows = [
            _build_canonical_entity_doc(
                tenant_id,
                kb_id,
                cname,
                centry["type"],
                centry.get("aliases", []),
                centry.get("source_doc_ids", []),
                centry["claim_count"],
                source_chunk_ids=centry.get("source_chunk_ids", []),
            )
            for cname, centry, _ in new_items
        ]
        await thread_pool_exec(
            settings.docStoreConn.insert,
            new_rows,
            search.index_name(tenant_id),
            kb_id,
        )

    if invalidated_chunk_ids:
        for cname, existing in canonical_entities.items():
            if cname in canonical_map:
                continue
            old_chunks = set(existing.get("source_chunk_ids") or [])
            if not old_chunks & invalidated_chunk_ids:
                continue
            remaining_chunks = old_chunks - invalidated_chunk_ids
            if not remaining_chunks:
                await _delete_canonical_entity(tenant_id, kb_id, cname)
            else:
                await _update_canonical_entity(
                    tenant_id,
                    kb_id,
                    cname,
                    existing.get("entity_type_kwd", "entity"),
                    existing.get("aliases", []),
                    existing.get("source_doc_ids", []),
                    existing.get("mention_count_int", 0),
                    source_chunk_ids=sorted(remaining_chunks),
                )

    # Clean up deleted canonical entities (from doc deletion)
    if deleted_doc_ids:
        for cname, centry in list(canonical_map.items()):
            centry["source_doc_ids"] = [d for d in centry.get("source_doc_ids", []) if d not in (deleted_doc_ids or set())]
            if not centry["source_doc_ids"] and centry["claim_count"] <= 0:
                await _delete_canonical_entity(tenant_id, kb_id, cname)
                del canonical_map[cname]

    # ----- Phase 3: REDUCE -----
    _progress("REDUCE: computing per-entity changes ...")

    # Use canonical names (from Entity Matching) instead of raw MAP names
    canonical_names: set[str] = set(canonical_map.keys())

    if incremental:
        # Resolve names from both sides of the chunk delta.  Deleted names may
        # no longer exist in the current canonical map, but their old pages
        # still need retraction/deletion.
        existing_aliases: dict[str, str] = {}
        for cname, centry in canonical_entities.items():
            existing_aliases[_normalize_key(cname)] = cname
            for alias in centry.get("aliases") or []:
                if isinstance(alias, str) and alias:
                    existing_aliases[_normalize_key(alias)] = cname
        affected_names = {name_resolution.get(raw_name) or existing_aliases.get(_normalize_key(raw_name)) or raw_name for raw_name in before_raw_names | after_raw_names}
        affected_names.discard("")
    else:
        affected_names = canonical_names

    # Load existing pages
    existing_pages = await _search_existing_pages(
        tenant_id,
        kb_id,
        [
            "slug_kwd",
            "title_kwd",
            "md_with_weight",
            "claims",
            "source_chunk_ids",
            "source_doc_ids",
            "page_version_int",
            "synthesis_version_int",
            "entity_names_kwd",
            "outlinks_kwd",
            "related_kb_pages_kwd",
            "page_type_kwd",
            "topic_kwd",
        ],
    )
    topic_pool = {
        _normalize_key(topic): topic for page in existing_pages.values() for topic in _as_str_list(page.get("topic_kwd")) if topic and _normalize_key(topic) != _normalize_key(WIKI_TOPIC_FALLBACK)
    }
    if mode == "topic" and existing_pages:
        plan_members = await _wiki_load_plan_group_members(tenant_id, kb_id)
        for page_id, names in plan_members.items():
            if page_id in existing_pages and names:
                existing_pages[page_id]["entity_names_kwd"] = names

    # A failed REFINE page must be reconsidered together with the current
    # document delta.  The current REDUCE snapshot remains authoritative, so
    # deleted entities can still produce a deletion rather than being blindly
    # retried from stale failure metadata.
    refine_failures = await _wiki_load_refine_failures(tenant_id, kb_id)
    retry_names = {str(name).strip() for failure in refine_failures.values() for name in failure.get("entity_names", []) if str(name).strip()}
    if incremental:
        affected_names.update(retry_names)

    # Build canonical claims ON-DEMAND only for affected names, then release
    # the full claim_index.  claim_index is keyed by RAW entity name (from MAP),
    # so claims must be aggregated through name_resolution onto the canonical
    # name.  Otherwise a canonical name that differs from every raw name
    # (e.g. "Apple Computer" → "Apple Inc.") would resolve to no claims and
    # every affected entity would be a no-op → empty output.
    canonical_claims: dict[str, list[dict]] = {}
    for raw_name, claims in claim_index.items():
        cname = name_resolution.get(raw_name, raw_name)
        if cname in affected_names:
            canonical_claims.setdefault(cname, []).extend(claims)
    # Ensure every affected name has an entry (possibly empty)
    for name in affected_names:
        canonical_claims.setdefault(name, [])
    del claim_index

    deltas = await _wiki_reduce_batch(
        affected_names=affected_names,
        existing_pages=existing_pages,
        deleted_doc_ids=deleted_doc_ids or set(),
        invalidated_chunk_ids=invalidated_chunk_ids,
        canonical_claims=canonical_claims,
        canonical_map=canonical_map,
        name_resolution=name_resolution,
    )

    if not deltas:
        _progress("REDUCE: no changes detected.")
        return summary

    # ----- Phase 4: Mode-specific dispatch -----
    # Precompute doc → canonical entity names for doc_page_source tracking,
    # before canonical_map is released.
    doc_to_entities: dict[str, list[str]] = {}
    entity_evidence: dict[str, dict[str, list[str]]] = {}
    for cname, centry in canonical_map.items():
        entity_evidence[cname] = {
            "source_doc_ids": list(centry.get("source_doc_ids", [])),
            "source_chunk_ids": list(centry.get("source_chunk_ids", [])),
        }
        for did in centry.get("source_doc_ids", []):
            doc_to_entities.setdefault(did, []).append(cname)
    del canonical_map

    topic_embeddings = await _wiki_prepare_topic_embeddings(doc_topics, embd_mdl, list(topic_pool.values()))
    topic_pool_lock = asyncio.Lock()
    if mode == "topic":
        summary = await _wiki_mode_b_run(
            deltas=deltas,
            existing_pages=existing_pages,
            chat_mdl=chat_mdl,
            embd_mdl=embd_mdl,
            tenant_id=tenant_id,
            kb_id=kb_id,
            callback=callback,
            doc_to_entities=doc_to_entities,
            entity_evidence=entity_evidence,
            entity_relations=entity_relations,
            doc_topics=doc_topics,
            topic_embeddings=topic_embeddings,
            topic_pool=topic_pool,
            topic_pool_lock=topic_pool_lock,
        )
    else:
        # Mode A: every entity AND concept becomes a page (no PLAN grouping).
        # Each canonical entity/concept compiles to its own wiki page.
        summary = await _wiki_mode_a_run(
            deltas=deltas,
            existing_pages=existing_pages,
            chat_mdl=chat_mdl,
            embd_mdl=embd_mdl,
            tenant_id=tenant_id,
            kb_id=kb_id,
            incremental=incremental,
            callback=callback,
            canonical_claims=canonical_claims,
            doc_to_entities=doc_to_entities,
            doc_topics=doc_topics,
            topic_embeddings=topic_embeddings,
            topic_pool=topic_pool,
            topic_pool_lock=topic_pool_lock,
        )
    del deltas
    del canonical_claims
    del name_resolution
    del existing_pages

    # ----- Phase 5: Update doc_page_source with canonical names -----
    # (doc_page_source page_ids is already handled in mode_run)
    _progress("FINALIZE: updating cross-references ...")
    try:
        await _wiki_finalize(tenant_id, kb_id, embd_mdl, chunk_state=current_chunk_state)
    except Exception:
        logging.exception("wiki: FINALIZE failed for kb=%s", kb_id)
        summary["errors"].append("FAILED_FINALIZE")

    return summary


async def _wiki_mode_a_run(
    *,
    deltas: list[dict],
    existing_pages: dict[str, dict],
    chat_mdl,
    embd_mdl,
    tenant_id: str,
    kb_id: str,
    incremental: bool,
    callback: Callable | None = None,
    canonical_claims: dict[str, list[dict]] | None = None,
    doc_to_entities: dict[str, list[str]] | None = None,
    doc_topics: dict[str, list[str]] | None = None,
    topic_embeddings: dict[str, object] | None = None,
    topic_pool: dict[str, str] | None = None,
    topic_pool_lock: asyncio.Lock | None = None,
) -> dict:
    """Mode A: every grounded entity and concept compiles to its own page.

    No PLAN grouping — each canonical entity/concept is a single page.
    Args:
        deltas: All entity/concept deltas (entity_type retained in each).
        canonical_claims: Canonical name → claims, for enriching page evidence.
        doc_to_entities: doc_id → [canonical names] for doc_page_source.
    """
    summary = {"pages_created": 0, "pages_modified": 0, "pages_deleted": 0, "errors": []}

    def _progress(msg: str):
        if callback:
            try:
                callback(0.7, f"wiki REFINE A: {msg}")
            except Exception:
                pass

    # Map names to existing page IDs
    name_to_page: dict[str, str] = {}
    for pid, page in existing_pages.items():
        for n in _as_str_list(page.get("entity_names_kwd")):
            name_to_page[n] = pid

    # Build page-level deltas. Each grounded entity/concept becomes one page;
    # REDUCE has already filtered metadata-only entities without claims.
    page_deltas: dict[str, dict] = {}
    for d in deltas:
        name = d.get("entity_name", "")
        if not name:
            continue
        entity_type = d.get("entity_type", "entity")
        if isinstance(entity_type, list):
            entity_type = entity_type[0] if entity_type else "entity"
        entity_type = str(entity_type or "entity").strip()
        # Derive page_id with type-appropriate prefix (concept/ vs entity/)
        prefix = "concept" if entity_type == "concept" else "entity"
        page_id = name_to_page.get(name) or _wiki_derive_page_id(name, prefix=prefix)

        if page_id not in page_deltas:
            page_deltas[page_id] = {
                "page_id": page_id,
                "page_title": name,
                "existing_page": existing_pages.get(page_id),
                "additions": [],
                "retractions": [],
                "claims": [],
                "source_chunks": [],
                "source_doc_ids": set(),
            }
        entry = page_deltas[page_id]
        entry["additions"].extend(d.get("additions", []))
        entry["retractions"].extend(d.get("retractions", []))
        delta_claims = _wiki_dedupe_claims(list(d.get("claims", [])) + list(d.get("additions", [])))
        entry["claims"].extend(delta_claims)
        entry["source_doc_ids"].update(d.get("retained_source_doc_ids", []))

        # Entity/concept and relation extraction rows carry source chunks even
        # when no dedicated claim exists.
        for cid in d.get("source_chunk_ids", []):
            entry["source_chunks"].append({"id": cid, "text": ""})

        # Collect source chunks from both complete claims and REDUCE additions.
        for claim in delta_claims:
            for cid in _wiki_claim_chunk_ids(claim):
                entry["source_chunks"].append(
                    {
                        "id": cid,
                        "text": claim.get("statement", claim.get("text", "")),
                        "source_doc_id": claim.get("source_doc_id"),
                    }
                )

        if d.get("action") == "delete":
            entry["action"] = "delete"
        elif entry.get("action") != "delete":
            entry["action"] = d.get("action")

    # Enrich pages with related claims that share source_chunk_ids.
    # Build a chunk_id → first-claim-text index from canonical_claims
    # (claims loaded on-demand after matching).
    if canonical_claims:
        chunk_claims: dict[str, list[str]] = {}
        for _cname, claims in canonical_claims.items():
            for claim in claims:
                for cid in _wiki_claim_chunk_ids(claim):
                    chunk_claims.setdefault(cid, []).append(claim.get("statement", claim.get("text", "")))

        for _pid, entry in page_deltas.items():
            page_chunk_ids = {c.get("id") for c in entry.get("source_chunks", []) if c.get("id")}
            if not page_chunk_ids:
                continue
            for cid in page_chunk_ids:
                texts = chunk_claims.get(cid)
                if texts:
                    # Append one source-chunk entry per chunk id
                    entry["source_chunks"].append({"id": cid, "text": texts[0]})

    # Concept depth check: thin CONCEPTS don't create pages (avoids dictionary
    # entries). Pages without grounded claims have already been filtered by
    # REDUCE above.
    # ONLY on the very first full build (non-incremental), when concept claims
    # are complete.  During incremental builds the deltas carry only the
    # changed claims, so a threshold check would wrongly reject every new
    # concept — new concepts in incremental mode are created regardless.
    if not incremental and not existing_pages:
        concept_pages = [entry for entry in page_deltas.values() if entry.get("page_id", "").startswith("concept/")]
        if concept_pages:
            deep_concepts = _wiki_decide_concept_pages(
                [
                    {"term": entry["page_title"], "claims": entry["claims"], "source_doc_ids": list({c.get("source_doc_id") for c in entry["claims"] if c.get("source_doc_id")})}
                    for entry in concept_pages
                ]
            )
            deep_ids = {p["page_id"] for p in deep_concepts}
            # Keep all entity pages + only deep concepts
            page_deltas = {pid: entry for pid, entry in page_deltas.items() if not pid.startswith("concept/") or pid in deep_ids}
        if not page_deltas:
            _progress("No pages to compile. Skipping.")
            return summary

    all_page_ids = list(existing_pages.keys())
    doc_updates: dict[str, list[str]] = {}
    topic_selection_stats = {"selected": 0, "new": 0, "new_added": 0}
    # Do not use a 20-slot semaphore around the whole page worker.  The worker
    # also performs source loading, embedding, and persistence after the LLM
    # returns; limiting that whole region would artificially starve the LLM
    # pool.  LLMCallPool is the only limit for actual chat calls.
    sem = asyncio.Semaphore(max(1, len(page_deltas)))

    async def _refine_one(pid: str, entry: dict) -> None:
        async with sem:
            try:
                existing = entry["existing_page"]
                page_type = "concept" if pid.startswith("concept/") else "entity"
                if entry.get("action") == "delete":
                    await _wiki_refine_page(
                        mode="delete",
                        page_id=pid,
                        page_title=entry["page_title"],
                        existing_page=existing,
                        page_type_kwd=page_type,
                        additions=None,
                        retractions=None,
                        source_chunks=[],
                        claims=[],
                        available_pages=all_page_ids,
                        contextual_hints="",
                        chat_mdl=chat_mdl,
                        embd_mdl=embd_mdl,
                        tenant_id=tenant_id,
                        kb_id=kb_id,
                        page_version=existing.get("page_version_int", 0) if existing else 0,
                    )
                    summary["pages_deleted"] += 1
                    await _wiki_clear_refine_failure(tenant_id, kb_id, pid)
                    return

                next_version = _as_int(existing.get("page_version_int")) + 1 if existing else 1
                new_doc_ids = {c.get("source_doc_id") for c in entry["additions"] if c.get("source_doc_id")}
                if existing and _wiki_should_re_synthesize(existing, new_doc_ids, next_version):
                    refine_mode = "re-synthesize"
                elif existing:
                    refine_mode = "modify"
                else:
                    refine_mode = "generate"

                result = await _wiki_refine_page(
                    mode=refine_mode,
                    page_id=pid,
                    page_title=entry["page_title"],
                    existing_page=existing,
                    page_type_kwd=page_type,
                    additions=entry["additions"],
                    retractions=entry["retractions"],
                    source_chunks=entry["source_chunks"],
                    claims=entry["claims"],
                    available_pages=all_page_ids,
                    contextual_hints="",
                    chat_mdl=chat_mdl,
                    embd_mdl=embd_mdl,
                    tenant_id=tenant_id,
                    kb_id=kb_id,
                    page_version=existing.get("page_version_int", 0) if existing else 0,
                    source_doc_ids=sorted(entry["source_doc_ids"]),
                    topic_candidates=_wiki_topics_for_docs(entry["source_doc_ids"], doc_topics, topic_pool),
                    topic_selection_stats=topic_selection_stats,
                    topic_embeddings=topic_embeddings,
                    topic_pool=topic_pool,
                    topic_pool_lock=topic_pool_lock,
                )
                if result is None:
                    raise RuntimeError(f"REFINE returned no page content for {pid}")
                await _wiki_clear_refine_failure(tenant_id, kb_id, pid)
                if refine_mode == "generate":
                    summary["pages_created"] += 1
                else:
                    summary["pages_modified"] += 1

                if result:
                    for did in entry["source_doc_ids"]:
                        doc_updates.setdefault(did, []).append(pid)

            except Exception as exc:
                logging.exception("wiki A: REFINE failed for %s", pid)
                await _wiki_record_refine_failure(
                    tenant_id,
                    kb_id,
                    pid,
                    entry.get("existing_page", {}).get("entity_names_kwd") or [entry.get("page_title", pid)],
                    error=str(exc),
                )
                summary["errors"].append(f"REFINE_FAILED:{pid}")

    tasks = [_refine_one(pid, entry) for pid, entry in page_deltas.items()]
    if tasks:
        _progress(f"{len(tasks)} pages ...")
        await _wiki_run_refine_tasks(tasks, _progress)
    _wiki_log_stats("TOPIC", "selection_summary", mode="A", **topic_selection_stats)

    for did, pids in doc_updates.items():
        try:
            existing_dps = (await _wiki_load_doc_page_source(tenant_id, kb_id, did)) or {}
            existing_pids = existing_dps.get("page_ids", [])
            for pid in pids:
                if pid not in existing_pids:
                    existing_pids.append(pid)
            # Collect entity names for this doc from precomputed doc_to_entities
            doc_entity_names = (doc_to_entities or {}).get(did, []) or existing_dps.get("entity_names")
            await _wiki_update_doc_page_source(
                tenant_id,
                kb_id,
                did,
                existing_pids,
                entity_names=doc_entity_names,
                chunk_hashes=existing_dps.get("source_chunk_hashes"),
                map_checksum=existing_dps.get("map_checksum"),
            )
        except Exception:
            logging.exception("wiki A: doc_page_source update failed for doc %s", did)

    _progress(f"done: +{summary['pages_created']} ~{summary['pages_modified']} -{summary['pages_deleted']}")
    return summary


def _wiki_claims_for_entity(page: dict, entity_name: str) -> list[dict]:
    """Return claims owned by one page member without guessing from prose."""
    claims = _wiki_parse_claims(page.get("claims"))
    member_names = _as_str_list(page.get("entity_names_kwd"))
    if len(member_names) == 1 and _normalize_key(member_names[0]) == _normalize_key(entity_name):
        return claims

    normalized_name = _normalize_key(entity_name)
    return [claim for claim in claims if _normalize_key(claim.get("entity_name") or claim.get("subject") or claim.get("term")) == normalized_name]


def _wiki_reconcile_page_moves(
    assignments: dict[str, list[dict]],
    existing_pages: dict[str, dict],
) -> dict[str, list[dict]]:
    """Route deletions to their owner and remove moved members from old pages."""
    previous_pages: dict[str, list[tuple[str, str]]] = {}
    for page_id, page in existing_pages.items():
        for name in _as_str_list(page.get("entity_names_kwd")):
            previous_pages.setdefault(_normalize_key(name), []).append((page_id, name))

    result: dict[str, list[dict]] = {}
    for target_id, entities in assignments.items():
        target_key = target_id[5:] if target_id.startswith("_new_") else target_id
        for entity in entities:
            name = entity.get("entity_name", "")
            old_memberships = previous_pages.get(_normalize_key(name), [])
            action = entity.get("action")

            # A deletion has no semantic destination: it belongs only to every
            # page that currently records this entity as a member.
            if action == "delete":
                for old_page_id, stored_name in old_memberships:
                    removal = dict(entity)
                    removal["entity_name"] = stored_name
                    removal["claims"] = []
                    removal["retractions"] = list(entity.get("retractions", [])) + _wiki_claims_for_entity(existing_pages[old_page_id], stored_name)
                    result.setdefault(old_page_id, []).append(removal)
                continue

            result.setdefault(target_id, []).append(entity)
            for old_page_id, stored_name in old_memberships:
                if old_page_id == target_key:
                    continue
                removal = {
                    "entity_name": stored_name,
                    "entity_type": entity.get("entity_type", "entity"),
                    "aliases": entity.get("aliases", []),
                    "claims": [],
                    "retractions": _wiki_claims_for_entity(existing_pages[old_page_id], stored_name),
                    "action": "delete",
                }
                result.setdefault(old_page_id, []).append(removal)

    return {page_id: entities for page_id, entities in result.items() if entities}


async def _wiki_mode_b_run(
    *,
    deltas: list[dict],
    existing_pages: dict[str, dict],
    chat_mdl,
    embd_mdl,
    tenant_id: str,
    kb_id: str,
    callback: Callable | None = None,
    doc_to_entities: dict[str, list[str]] | None = None,
    entity_evidence: dict[str, dict[str, list[str]]] | None = None,
    entity_relations: dict[str, list[dict]] | None = None,
    doc_topics: dict[str, list[str]] | None = None,
    topic_embeddings: dict[str, object] | None = None,
    topic_pool: dict[str, str] | None = None,
    topic_pool_lock: asyncio.Lock | None = None,
) -> dict:
    """Mode B: Page Router + per-page REFINE."""

    summary = {"pages_created": 0, "pages_modified": 0, "pages_deleted": 0, "errors": []}

    def _progress(msg: str):
        if callback:
            try:
                callback(0.7, f"wiki REFINE B: {msg}")
            except Exception:
                pass

    # Convert deltas to entity dicts that Page Router can process
    affected_entities = [
        {
            "entity_name": d.get("entity_name", ""),
            "entity_type": d.get("entity_type", "entity"),
            "aliases": d.get("aliases", []),
            "claims": _wiki_dedupe_claims(d.get("additions", []) + d.get("claims", [])),
            "retractions": d.get("retractions", []),
            "source_chunk_ids": d.get("source_chunk_ids", []),
            "source_doc_ids": d.get("retained_source_doc_ids", []),
            "relations": (entity_relations or {}).get(d.get("entity_name", ""), []),
            "action": d.get("action", ""),
        }
        for d in deltas
        if d.get("entity_name")
    ]

    if not affected_entities:
        _progress("No affected entities. Skipping.")
        return summary

    # Run Page Router
    _progress(f"Page Router: routing {len(affected_entities)} entities ...")
    assignments = await _wiki_page_router(
        affected_entities=affected_entities,
        chat_mdl=chat_mdl,
        embd_mdl=embd_mdl,
        tenant_id=tenant_id,
        kb_id=kb_id,
        existing_pages=existing_pages,
    )

    assignments = _wiki_reconcile_page_moves(assignments, existing_pages)
    assignments = await _wiki_split_unstable_page_assignments(
        assignments=assignments,
        existing_pages=existing_pages,
        chat_mdl=chat_mdl,
        embd_mdl=embd_mdl,
        kb_id=kb_id,
    )

    if not assignments:
        _progress("Page Router: no assignments. Skipping.")
        return summary

    # Load available pages for wikilinks
    all_page_ids = list(existing_pages.keys())

    # Collect source chunks per assignment for the REFINE prompt
    page_source_chunks: dict[str, list[dict]] = {}
    for pid, entities in assignments.items():
        page_key = pid[5:] if pid.startswith("_new_") else pid
        chunks: list[dict] = []
        for ent in entities:
            for cid in ent.get("source_chunk_ids", []):
                chunks.append({"id": cid, "text": ""})
            for c in ent.get("claims", []):
                for cid in _wiki_claim_chunk_ids(c):
                    chunks.append(
                        {
                            "id": cid,
                            "text": c.get("statement", c.get("text", "")),
                            "source_doc_id": c.get("source_doc_id"),
                        }
                    )
        if chunks:
            page_source_chunks[page_key] = chunks

    doc_updates: dict[str, list[str]] = {}  # doc_id → [page_ids]
    doc_removals: dict[str, list[str]] = {}  # doc_id → [page_ids]
    topic_selection_stats = {"selected": 0, "new": 0, "new_added": 0}
    # The shared LLMCallPool limits chat calls.  This semaphore must not cap
    # the complete worker because embedding and page persistence happen after
    # the chat call and should not consume an LLM concurrency slot.
    sem = asyncio.Semaphore(max(1, len(assignments)))

    async def _refine_one(page_id: str, entities: list) -> None:
        async with sem:
            try:
                is_new = page_id.startswith("_new_")
                page_key = page_id[5:] if is_new else page_id
                page_key = str(page_key or "").strip()
                if not page_key:
                    # Blank assignment (e.g. a mangled slug from the router) —
                    # nothing to generate for an empty page id.
                    return
                existing = existing_pages.get(page_key) if not is_new else None

                # Determine page_type for Mode B. The slug prefix is the source
                # of truth: a page whose slug starts with "concept/" is a concept
                # page (the router derives it via _wiki_derive_page_id). Inferring
                # from `entities[].entity_type` is unreliable (many entities carry
                # a concrete type like "person"/"org", or none at all), which
                # previously mislabelled concept pages as entity.
                if existing:
                    page_type = existing.get("page_type_kwd", "entity")
                    if isinstance(page_type, (list, tuple)):
                        page_type = page_type[0] if page_type else "entity"
                else:
                    if page_key.startswith("concept/"):
                        page_type = "concept"
                    else:
                        page_type = "entity"

                additions = []
                retractions = []
                page_source_doc_ids: set[str] = set()
                member_evidence: list[dict] = []
                action = "create" if is_new else "update"
                for ent in entities:
                    ent_claims = list(ent.get("claims", []))
                    additions.extend(ent_claims)
                    retractions.extend(ent.get("retractions", []))
                    page_source_doc_ids.update(ent.get("source_doc_ids", []))
                    member_evidence.append(
                        {
                            "name": ent.get("entity_name", ""),
                            "claims": ent_claims,
                            "source_chunk_ids": ent.get("source_chunk_ids", []),
                        }
                    )

                existing_names = _as_str_list(existing.get("entity_names_kwd")) if existing else []
                added_names = [ent.get("entity_name", "") for ent in entities if ent.get("action") != "delete" and ent.get("entity_name")]
                deleted_names = {ent.get("entity_name", "") for ent in entities if ent.get("action") == "delete"}
                member_names = sorted((set(existing_names) | set(added_names)) - deleted_names)
                if not member_names:
                    action = "delete"

                member_source_chunks = list(page_source_chunks.get(page_key, []))
                for member_name in member_names:
                    evidence = (entity_evidence or {}).get(member_name, {})
                    page_source_doc_ids.update(evidence.get("source_doc_ids", []))
                    member_source_chunks.extend({"id": cid, "text": ""} for cid in evidence.get("source_chunk_ids", []))
                    if not any(item.get("name") == member_name for item in member_evidence):
                        member_evidence.append(
                            {
                                "name": member_name,
                                "claims": _wiki_claims_for_entity(existing, member_name) if existing else [],
                                "source_chunk_ids": evidence.get("source_chunk_ids", []),
                            }
                        )

                if action == "delete":
                    deleted_page = await _wiki_refine_page(
                        mode="delete",
                        page_id=page_key,
                        page_title=existing.get("title_kwd", page_key) if existing else page_key,
                        existing_page=existing,
                        page_type_kwd=page_type,
                        additions=None,
                        retractions=None,
                        source_chunks=[],
                        claims=[],
                        available_pages=all_page_ids,
                        contextual_hints="",
                        chat_mdl=chat_mdl,
                        embd_mdl=embd_mdl,
                        tenant_id=tenant_id,
                        kb_id=kb_id,
                        page_version=existing.get("page_version_int", 0) if existing else 0,
                    )
                    if deleted_page is None:
                        await _wiki_clear_refine_failure(tenant_id, kb_id, page_key)
                        await _wiki_delete_plan_group(tenant_id, kb_id, page_key)
                        for did in _as_str_list(existing.get("source_doc_ids")) if existing else []:
                            doc_removals.setdefault(did, []).append(page_key)
                        summary["pages_deleted"] += 1
                    return

                refine_mode = "generate" if is_new else "modify"
                if existing and _wiki_should_re_synthesize(
                    existing,
                    {c.get("source_doc_id") for c in additions if c.get("source_doc_id")},
                    _as_int(existing.get("page_version_int")) + 1,
                ):
                    refine_mode = "re-synthesize"

                result = await _wiki_refine_page(
                    mode=refine_mode,
                    page_id=page_key,
                    page_title=existing.get("title_kwd", page_key) if existing else entities[0].get("entity_name", page_key),
                    existing_page=existing,
                    page_type_kwd=page_type,
                    additions=additions,
                    retractions=retractions,
                    source_chunks=member_source_chunks,
                    claims=additions,
                    available_pages=all_page_ids,
                    contextual_hints=_wiki_build_contextual_hints(page_key, existing, {}),
                    chat_mdl=chat_mdl,
                    embd_mdl=embd_mdl,
                    tenant_id=tenant_id,
                    kb_id=kb_id,
                    page_version=existing.get("page_version_int", 0) if existing else 0,
                    entity_names=member_names,
                    embed_routing_context=True,
                    source_doc_ids=sorted(page_source_doc_ids),
                    topic_candidates=_wiki_topics_for_docs(page_source_doc_ids, doc_topics, topic_pool),
                    topic_selection_stats=topic_selection_stats,
                    topic_embeddings=topic_embeddings,
                    topic_pool=topic_pool,
                    topic_pool_lock=topic_pool_lock,
                    member_evidence=member_evidence,
                )
                if result is None:
                    raise RuntimeError(f"REFINE returned no page content for {page_key}")
                if result:
                    await _wiki_clear_refine_failure(tenant_id, kb_id, page_key)
                    if is_new:
                        summary["pages_created"] += 1
                    else:
                        summary["pages_modified"] += 1
                    await _wiki_update_plan_group(
                        tenant_id,
                        kb_id,
                        page_key,
                        entity_names=member_names,
                        page_version=result.get("page_version_int", 1),
                    )

                    # Collect doc_page_source updates (deferred)
                    for ent in entities:
                        for c in ent.get("claims", []):
                            did = c.get("source_doc_id")
                            if did:
                                doc_updates.setdefault(did, []).append(page_key)
                    for did in page_source_doc_ids:
                        doc_updates.setdefault(did, []).append(page_key)
                    old_doc_ids = set(_as_str_list(existing.get("source_doc_ids"))) if existing else set()
                    new_doc_ids = set(_as_str_list(result.get("source_doc_ids")))
                    for did in old_doc_ids - new_doc_ids:
                        doc_removals.setdefault(did, []).append(page_key)

            except Exception as exc:
                logging.exception("wiki B: REFINE failed for %s", page_id)
                failed_page_id = page_id[5:] if page_id.startswith("_new_") else page_id
                await _wiki_record_refine_failure(
                    tenant_id,
                    kb_id,
                    failed_page_id,
                    [ent.get("entity_name", "") for ent in entities if isinstance(ent, dict)],
                    error=str(exc),
                )
                summary["errors"].append(f"REFINE_FAILED:{page_id}")

    tasks = [_refine_one(pid, ents) for pid, ents in assignments.items()]
    if tasks:
        _progress(f"{len(tasks)} pages ...")
        await _wiki_run_refine_tasks(tasks, _progress)
    _wiki_log_stats("TOPIC", "selection_summary", mode="B", **topic_selection_stats)

    # Apply doc_page_source updates serially (no race), preserving metadata
    for did in set(doc_updates) | set(doc_removals):
        try:
            existing_dps = (await _wiki_load_doc_page_source(tenant_id, kb_id, did)) or {}
            existing_pids = existing_dps.get("page_ids", [])
            removed_pids = set(doc_removals.get(did, []))
            existing_pids = [pid for pid in existing_pids if pid not in removed_pids]
            for pid in doc_updates.get(did, []):
                if pid not in existing_pids:
                    existing_pids.append(pid)
            await _wiki_update_doc_page_source(
                tenant_id,
                kb_id,
                did,
                existing_pids,
                entity_names=(doc_to_entities or {}).get(did, []) or existing_dps.get("entity_names"),
                chunk_hashes=existing_dps.get("source_chunk_hashes"),
                map_checksum=existing_dps.get("map_checksum"),
            )
        except Exception:
            logging.exception("wiki B: doc_page_source update failed for doc %s", did)

    _progress(f"done: +{summary['pages_created']} ~{summary['pages_modified']} -{summary['pages_deleted']}")
    return summary


def _wiki_parse_claims(raw_claims) -> list[dict]:
    if isinstance(raw_claims, str):
        try:
            raw_claims = json.loads(raw_claims) if raw_claims else []
        except (json.JSONDecodeError, TypeError):
            raw_claims = []
    return [claim for claim in raw_claims or [] if isinstance(claim, dict)] if isinstance(raw_claims, (list, tuple)) else []


def _wiki_embedding_cohesion(matrix: np.ndarray) -> float:
    if matrix.ndim != 2 or matrix.shape[0] <= 1:
        return 1.0
    centroid = np.mean(matrix, axis=0)
    norm = np.linalg.norm(centroid)
    if norm <= 0:
        return 0.0
    return float(np.mean(matrix @ (centroid / norm)))


async def _wiki_split_unstable_page_assignments(
    *,
    assignments: dict[str, list[dict]],
    existing_pages: dict[str, dict],
    chat_mdl,
    embd_mdl,
    kb_id: str = "",
) -> dict[str, list[dict]]:
    """Let the LLM reconsider affected pages whose embedding cohesion degrades."""
    if not assignments:
        return assignments

    candidates: dict[str, dict] = {}
    all_members: list[dict] = []
    for page_id, incoming in assignments.items():
        existing = existing_pages.get(page_id) if not page_id.startswith("_new_") else None
        if not existing:
            continue
        old_names = _as_str_list(existing.get("entity_names_kwd"))
        deleted_names = {entity.get("entity_name", "") for entity in incoming if entity.get("action") == "delete"}
        incoming_by_name = {entity.get("entity_name", ""): entity for entity in incoming if entity.get("entity_name")}
        member_names = sorted((set(old_names) | set(incoming_by_name)) - deleted_names)
        if len(member_names) <= 1:
            continue
        members = []
        for name in member_names:
            incoming_entity = incoming_by_name.get(name, {})
            members.append(
                {
                    "entity_name": name,
                    "entity_type": incoming_entity.get("entity_type", "entity"),
                    "aliases": incoming_entity.get("aliases", []),
                    "claims": _wiki_claims_for_entity(existing, name) + incoming_entity.get("claims", []),
                    "retractions": incoming_entity.get("retractions", []),
                    "source_chunk_ids": incoming_entity.get("source_chunk_ids", []),
                    "source_doc_ids": incoming_entity.get("source_doc_ids", []),
                    "action": incoming_entity.get("action", "update"),
                }
            )
        start = len(all_members)
        all_members.extend(members)
        candidates[page_id] = {
            "existing": existing,
            "old_names": old_names,
            "members": members,
            "removed_retractions": [claim for entity in incoming if entity.get("action") == "delete" for claim in entity.get("retractions", [])],
            "vector_slice": slice(start, len(all_members)),
        }

    if all_members:
        vectors, _ = await thread_pool_exec(embd_mdl.encode, [_entity_to_query_text(member) for member in all_members])
        matrix = _wiki_normalize_rows(np.asarray(vectors, dtype=np.float32))
    else:
        matrix = np.empty((0, 0), dtype=np.float32)

    group_semaphore = asyncio.Semaphore(WIKI_GROUP_LLM_MAX_CONCURRENT)

    async def _reconsider(record: dict) -> list[list[dict]] | None:
        member_matrix = matrix[record["vector_slice"]]
        old_name_set = set(record["old_names"])
        old_member_indices = [idx for idx, member in enumerate(record["members"]) if member["entity_name"] in old_name_set]
        combined_cohesion = _wiki_embedding_cohesion(member_matrix)
        old_cohesion = _wiki_embedding_cohesion(member_matrix[old_member_indices]) if old_member_indices else 1.0
        over_capacity = len(record["members"]) > PAGE_CLUSTER_HARD_MAX_SIZE
        degraded = len(old_member_indices) >= 2 and combined_cohesion < old_cohesion - 0.05
        if not over_capacity and not degraded:
            return None
        return await _wiki_llm_group_entities(record["members"], member_matrix, chat_mdl, semaphore=group_semaphore, kb_id=kb_id)

    reconsidered = await asyncio.gather(*(_reconsider(record) for record in candidates.values()))
    for record, clusters in zip(candidates.values(), reconsidered, strict=True):
        record["clusters"] = clusters

    result: dict[str, list[dict]] = {}
    used_page_ids = set(existing_pages) | {key[5:] for key in assignments if key.startswith("_new_")}
    for page_id, incoming in assignments.items():
        record = candidates.get(page_id)
        if not record or not record.get("clusters") or len(record["clusters"]) <= 1:
            result[page_id] = incoming
            continue
        existing = record["existing"]
        clusters = record["clusters"]
        removed_retractions = record["removed_retractions"]

        page_title = existing.get("title_kwd", "")
        if isinstance(page_title, (list, tuple)):
            page_title = page_title[0] if page_title else ""
        retained_idx = next(
            (idx for idx, cluster in enumerate(clusters) if page_title and any(member["entity_name"] == page_title for member in cluster)),
            max(range(len(clusters)), key=lambda idx: (len(clusters[idx]), -idx)),
        )
        moved_claims = [claim for idx, cluster in enumerate(clusters) if idx != retained_idx for member in cluster for claim in member.get("claims", [])]
        retained_cluster = clusters[retained_idx]
        if retained_cluster and (moved_claims or removed_retractions):
            retained_cluster[0]["retractions"] = retained_cluster[0].get("retractions", []) + moved_claims + removed_retractions
        result[page_id] = retained_cluster

        for idx, cluster in enumerate(clusters):
            if idx == retained_idx:
                continue
            representative = min(
                cluster,
                key=lambda entity: (-len(entity.get("claims") or []), str(entity.get("entity_name", "")).casefold(), str(entity.get("entity_name", ""))),
            )
            cluster = [representative] + [entity for entity in cluster if entity is not representative]
            prefix = page_id.split("/", 1)[0] if "/" in page_id else "entity"
            base_id = _wiki_derive_page_id(representative.get("entity_name", ""), prefix=prefix)
            candidate_id = base_id
            suffix = 2
            while candidate_id in used_page_ids:
                candidate_id = f"{base_id}-{suffix}"
                suffix += 1
            used_page_ids.add(candidate_id)
            for entity in cluster:
                entity["action"] = "create"
                entity["retractions"] = []
            result[f"_new_{candidate_id}"] = cluster

    return result


async def _wiki_update_plan_group(
    tenant_id: str,
    kb_id: str,
    page_id: str,
    entity_names: list[str],
    page_version: int,
) -> None:
    """Update or create plan_group row (Mode B)."""
    from rag.nlp import search
    from common.misc_utils import thread_pool_exec
    from common.doc_store.doc_store_base import OrderByExpr

    index = search.index_name(tenant_id)
    condition = {
        "compile_kwd": [WIKI_PLAN_GROUP_COMPILE_KWD],
        "page_id": [page_id],
    }

    doc = {
        "id": _stable_row_id(WIKI_PLAN_GROUP_COMPILE_KWD, kb_id, page_id),
        "kb_id": kb_id,
        "page_id": page_id,
        "entity_names": json.dumps(entity_names, ensure_ascii=False),
        "page_version_int": page_version,
        "compile_kwd": WIKI_PLAN_GROUP_COMPILE_KWD,
    }

    existing = await thread_pool_exec(
        settings.docStoreConn.search,
        ["page_id"],
        [],
        condition,
        [],
        OrderByExpr(),
        0,
        1,
        index,
        [kb_id],
    )
    if settings.docStoreConn.get_fields(existing, ["page_id"]):
        await thread_pool_exec(
            settings.docStoreConn.update,
            {"page_id": page_id},
            doc,
            index,
            kb_id,
        )
    else:
        await thread_pool_exec(
            settings.docStoreConn.insert,
            [doc],
            index,
            kb_id,
        )


async def _wiki_delete_plan_group(tenant_id: str, kb_id: str, page_id: str) -> None:
    await thread_pool_exec(
        settings.docStoreConn.delete,
        {"compile_kwd": [WIKI_PLAN_GROUP_COMPILE_KWD], "page_id": [page_id]},
        search.index_name(tenant_id),
        kb_id,
    )


async def _wiki_load_plan_group_members(tenant_id: str, kb_id: str) -> dict[str, list[str]]:
    """Load the authoritative Mode B page membership map."""
    index = search.index_name(tenant_id)
    fields = ["page_id", "entity_names"]
    result: dict[str, list[str]] = {}
    offset = 0
    page_size = 1000
    while True:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            fields,
            [],
            {"compile_kwd": [WIKI_PLAN_GROUP_COMPILE_KWD]},
            [],
            OrderByExpr(),
            offset,
            page_size,
            index,
            [kb_id],
        )
        rows = settings.docStoreConn.get_fields(res, fields) or {}
        for row in rows.values():
            page_id = row.get("page_id", "")
            if isinstance(page_id, (list, tuple)):
                page_id = page_id[0] if page_id else ""
            raw_names = row.get("entity_names", [])
            if isinstance(raw_names, str):
                try:
                    raw_names = json.loads(raw_names) if raw_names else []
                except (json.JSONDecodeError, TypeError):
                    raw_names = []
            names = sorted({str(name) for name in raw_names or [] if name})
            if page_id and names:
                result[str(page_id)] = names
        if len(rows) < page_size:
            break
        offset += page_size
    return result


async def wiki_handle_document_deleted(
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    chat_mdl,
    embd_mdl,
    mode: str,
) -> dict:
    """Clean up wiki pages + canonical entities when a document is deleted.

    Args:
        mode: ``topic`` updates plan groups; ``entity`` does not.

    Returns: {pages_modified, pages_deleted, errors}
    """
    summary = {"pages_modified": 0, "pages_deleted": 0, "errors": []}

    # Step 1: Update canonical entity index (decrement claim_count)
    dps = await _wiki_load_doc_page_source(tenant_id, kb_id, doc_id)
    if not dps:
        return summary

    entity_names = dps.get("entity_names", [])
    if entity_names:
        canonical_index = await _load_canonical_entities(tenant_id, kb_id)
        for ename in entity_names:
            centry = canonical_index.get(ename)
            if centry:
                src_ids = centry.get("source_doc_ids", [])
                if isinstance(src_ids, str):
                    try:
                        src_ids = json.loads(src_ids) if src_ids else []
                    except (json.JSONDecodeError, TypeError):
                        src_ids = []
                if doc_id in src_ids:
                    src_ids.remove(doc_id)

                if not src_ids:
                    await _delete_canonical_entity(tenant_id, kb_id, ename)
                else:
                    # Keep existing mention_count_int; the next REFINE phase
                    # will recalculate claims precisely from wiki pages.
                    # Removing the doc_id from source_doc_ids prevents future
                    # incremental runs from re-tracking this deletion.
                    await _save_canonical_entity(
                        tenant_id,
                        kb_id,
                        ename,
                        centry.get("entity_type_kwd", "entity"),
                        centry.get("aliases", []),
                        src_ids,
                        centry.get("mention_count_int", len(src_ids)),
                        source_chunk_ids=centry.get("source_chunk_ids", []),
                    )

    affected_page_ids = dps.get("page_ids", [])
    if not affected_page_ids:
        return summary

    # Step 2: Update wiki pages
    all_existing_pages = await _search_existing_pages(
        tenant_id,
        kb_id,
        ["slug_kwd", "title_kwd", "md_with_weight", "claims", "source_doc_ids", "page_version_int", "entity_names_kwd", "page_type_kwd", "topic_kwd"],
    )

    for page_id in affected_page_ids:
        try:
            existing = all_existing_pages.get(page_id)
            if not existing:
                continue

            source_doc_ids = existing.get("source_doc_ids", [])
            if isinstance(source_doc_ids, str):
                source_doc_ids = json.loads(source_doc_ids) if source_doc_ids else []

            if doc_id in source_doc_ids:
                source_doc_ids.remove(doc_id)

            page_type = existing.get("page_type_kwd", "concept" if mode == "entity" else "entity")

            if not source_doc_ids:
                await _wiki_refine_page(
                    mode="delete",
                    page_id=page_id,
                    page_title=existing.get("title_kwd", page_id),
                    existing_page=existing,
                    page_type_kwd=page_type,
                    additions=None,
                    retractions=None,
                    source_chunks=[],
                    claims=[],
                    available_pages=[],
                    contextual_hints="",
                    chat_mdl=chat_mdl,
                    embd_mdl=embd_mdl,
                    tenant_id=tenant_id,
                    kb_id=kb_id,
                    page_version=existing.get("page_version_int", 0),
                    source_doc_ids=source_doc_ids,
                )
                summary["pages_deleted"] += 1
            else:
                existing_claims = existing.get("claims", [])
                if isinstance(existing_claims, str):
                    existing_claims = json.loads(existing_claims) if existing_claims else []

                retractions = [c for c in existing_claims if c.get("source_doc_id") == doc_id]
                retained = [c for c in existing_claims if c.get("source_doc_id") != doc_id]

                await _wiki_refine_page(
                    mode="modify",
                    page_id=page_id,
                    page_title=existing.get("title_kwd", page_id),
                    existing_page=existing,
                    page_type_kwd=page_type,
                    additions=[],
                    retractions=retractions,
                    source_chunks=[],
                    claims=retained,
                    available_pages=list(all_existing_pages.keys()),
                    contextual_hints=_wiki_build_contextual_hints(page_id, existing, {}),
                    chat_mdl=chat_mdl,
                    embd_mdl=embd_mdl,
                    tenant_id=tenant_id,
                    kb_id=kb_id,
                    page_version=existing.get("page_version_int", 0),
                )
                summary["pages_modified"] += 1

            if mode == "topic":
                plan_condition = {
                    "compile_kwd": [WIKI_PLAN_GROUP_COMPILE_KWD],
                    "page_id": [page_id],
                }
                index = search.index_name(tenant_id)
                res_pg = await thread_pool_exec(
                    settings.docStoreConn.search,
                    ["entity_names", "source_doc_ids"],
                    [],
                    plan_condition,
                    [],
                    OrderByExpr(),
                    0,
                    1,
                    index,
                    [kb_id],
                )
                pg_map = settings.docStoreConn.get_fields(res_pg, ["entity_names", "source_doc_ids"])
                for pg_row in pg_map.values():
                    pg_src_ids = pg_row.get("source_doc_ids", [])
                    if isinstance(pg_src_ids, str):
                        pg_src_ids = json.loads(pg_src_ids)
                    if doc_id in pg_src_ids:
                        pg_src_ids.remove(doc_id)
                    await _wiki_update_plan_group(
                        tenant_id,
                        kb_id,
                        page_id,
                        entity_names=json.loads(pg_row.get("entity_names", "[]")) if isinstance(pg_row.get("entity_names"), str) else pg_row.get("entity_names", []),
                        page_version=existing.get("page_version_int", 0),
                    )
                    break

        except Exception:
            logging.exception("wiki: document deletion cleanup failed for page=%s doc=%s", page_id, doc_id)
            summary["errors"].append(f"CLEANUP_FAILED:{page_id}")

    await _wiki_delete_doc_page_source(tenant_id, kb_id, doc_id)

    try:
        await _wiki_finalize(tenant_id, kb_id, embd_mdl)
    except Exception:
        logging.exception("wiki: FINALIZE after deletion failed")

    return summary


__all__ = [
    "WIKI_PAGE_COMPILE_KWD",
    "WIKI_PLAN_GROUP_COMPILE_KWD",
    "WIKI_DOC_PAGE_SOURCE_COMPILE_KWD",
    "WIKI_CANONICAL_ENTITY_COMPILE_KWD",
    "wiki_compile_incremental",
    "wiki_handle_document_deleted",
    "_wiki_reduce_entity",
    "_wiki_reduce_batch",
    "_wiki_match_entities",
    "_wiki_page_router",
    "_wiki_finalize",
    "_wiki_refine_page",
    "_wiki_update_doc_page_source",
    "_load_canonical_entities",
    "_save_canonical_entity",
    "_delete_canonical_entity",
    "_extract_raw_entities",
]
