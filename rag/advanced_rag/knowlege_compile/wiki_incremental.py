"""Dual-mode wiki incremental compilation.

Mode A (no-plan, plan=no):
  MAP → REDUCE → REFINE per-concept (generate/modify/re-synthesize) → FINALIZE
  1 concept = 1 page (WeKnora style). Entities enrich concept pages via source chunks.

Mode B (with-plan, plan=yes):
  MAP → REDUCE → PLAN (LLM grouping) → REFINE per-page → FINALIZE
  Incremental: Page Router (KNN) routes entities to existing pages.

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


# ----- REFINE concurrency control -----

WIKI_REFINE_MAX_CONCURRENT = 20  # shared LLM pool size used by the Wiki runner


# ----- constants ----

# compile_kwd values
WIKI_PAGE_COMPILE_KWD = "wiki_page"
WIKI_PLAN_GROUP_COMPILE_KWD = "wiki_plan_group"
WIKI_DOC_PAGE_SOURCE_COMPILE_KWD = "wiki_doc_page_source"
WIKI_CANONICAL_ENTITY_COMPILE_KWD = "wiki_canonical_entity"

# Entity matching thresholds (not exposed in YAML)
ENTITY_MERGE_THRESHOLD = 0.90  # auto-merge
ENTITY_AMBIGUOUS_LOW = 0.75  # LLM confirm boundary
ENTITY_PAIRWISE_BLOCK_SIZE = 1024  # blockwise embedding matrix block size

# Number of concurrent KNN queries for entity matching
ENTITY_MATCH_KNN_CONCURRENT = 20
CANONICAL_PERSIST_CONCURRENT = 20
PAGE_ROUTER_KNN_CONCURRENT = 20

# Thematic topic grouping. No-plan pages have no PLAN step, so pages are grouped
# post-hoc by matching them to the thematic topic labels the MAP phase extracted.
WIKI_TOPIC_MATCH_THRESHOLD = 0.50  # min cosine for a page to attach to a topic
WIKI_TOPIC_MAX_LABELS = 200  # cap on candidate topic labels
WIKI_TOPIC_FALLBACK = "General"  # bucket for pages that match no topic
WIKI_TOPIC_UPDATE_CONCURRENT = 16  # concurrent page topic_kwd updates

# Page Router thresholds (kept as code constants — not exposed in YAML)
PAGE_ROUTER_UPDATE_THRESHOLD = 0.80
PAGE_ROUTER_MAYBE_THRESHOLD = 0.50
PAGE_ROUTER_CLUSTER_THRESHOLD = 0.50

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


def _wiki_derive_page_id(term: str, prefix: str = "concept") -> str:
    """Derive a URL-safe page identifier from a concept/entity name.

    Example: "Smartphone Industry" → "concept/smartphone-industry"
    """
    slug = re.sub(r"[^a-zA-Z0-9\u4e00-\u9fff]+", "-", term).strip("-").lower()
    return f"{prefix}/{slug}"


def _entity_to_query_text(entity: dict) -> str:
    return " ".join(
        [
            entity.get("entity_name") or entity.get("name") or entity.get("term") or "",
            entity.get("definition_excerpt") or entity.get("description") or entity.get("statement", ""),
        ][:2]
    )


def _strip_think(text: str) -> str:
    if not isinstance(text, str):
        return ""
    text = text.strip()
    if text.startswith("</think>"):
        return text.split("</think>", 1)[-1].strip()
    return text


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
    return _strip_think(raw or "")


def _wiki_should_re_synthesize(
    page: dict,
    new_source_doc_ids: set[str],
    next_version: int,
) -> bool:
    existing_sources = set(page.get("source_doc_ids", []))
    total_sources = existing_sources | new_source_doc_ids
    claim_count = len(page.get("claims", []))
    last_synth_ver = page.get("synthesis_version_int", 1)
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
                ["entity_kwd", "entity_type_kwd", "aliases", "source_doc_ids", "mention_count_int"],
                [],
                {"compile_kwd": [WIKI_CANONICAL_ENTITY_COMPILE_KWD]},
                [],
                OrderByExpr(),
                offset,
                page_size,
                index,
                [kb_id],
            )
            field_map = settings.docStoreConn.get_fields(res, ["entity_kwd", "entity_type_kwd", "aliases", "source_doc_ids", "mention_count_int"]) or {}
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
                for fld in ("aliases", "source_doc_ids"):
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
) -> dict:
    """Build a canonical entity row for insert or update."""
    dim = len(embedding) if embedding else 768
    doc = {
        "id": _stable_row_id(WIKI_CANONICAL_ENTITY_COMPILE_KWD, kb_id, entity_name),
        "entity_kwd": entity_name,
        "entity_type_kwd": entity_type,
        "aliases": json.dumps(list(set(aliases)), ensure_ascii=False),
        "source_doc_ids": json.dumps(list(set(source_doc_ids)), ensure_ascii=False),
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
                    source_doc_ids} — NO full claim text, so Entity Matching
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
                }
            raw[name]["source_doc_ids"].add(doc_id)

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
                }
            raw[term]["source_doc_ids"].add(doc_id)

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
                claim_index.setdefault(subj, []).append(claim)

    result = []
    for entry in raw.values():
        entry["source_doc_ids"] = list(entry["source_doc_ids"])
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
                            else:
                                merged_into[ri] = rj

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


async def _load_map_relations(tenant_id: str, kb_id: str) -> list[dict]:
    """Load all extracted (from, to, type) relations from wiki_map_extract rows.

    These are the semantic edges the LLM extracted during MAP. When both
    endpoints correspond to compiled wiki pages they become page-to-page
    connections (outlinks), which is far more reliable than hoping REFINE
    sprinkled [[wikilinks]] into prose.
    """
    from common.doc_store.doc_store_base import OrderByExpr

    index = search.index_name(tenant_id)
    if not settings.docStoreConn.index_exist(index, kb_id):
        return []
    relations: list[dict] = []
    offset, page_size = 0, 1000
    while True:
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                ["content_with_weight"],
                [],
                {"compile_kwd": ["wiki_map_extract"]},
                [],
                OrderByExpr(),
                offset,
                page_size,
                index,
                [kb_id],
            )
            field_map = settings.docStoreConn.get_fields(res, ["content_with_weight"]) or {}
        except Exception:
            logging.exception("wiki: failed to load map relations for kb=%s", kb_id)
            return relations
        for row in field_map.values():
            raw = row.get("content_with_weight")
            if isinstance(raw, str):
                try:
                    raw = json.loads(raw)
                except (json.JSONDecodeError, TypeError):
                    raw = None
            if not isinstance(raw, dict):
                continue
            for r in raw.get("relations") or []:
                if isinstance(r, dict):
                    frm = r.get("from")
                    to = r.get("to")
                    if isinstance(frm, str) and isinstance(to, str):
                        relations.append({"from": frm, "to": to, "type": r.get("type", "related")})
        if len(field_map) < page_size:
            break
        offset += page_size
    return relations


async def _wiki_load_pages_for_graph(tenant_id: str, kb_id: str) -> list[dict]:
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
        link = m.group(1).strip()
        if link and link not in seen:
            seen.add(link)
            outlinks.append(link)
    if kb_id:
        kb_esc = re.escape(str(kb_id))
        for m in re.finditer(rf"\]\(artifact/{kb_esc}/([^)]+)\)", content):
            slug = m.group(1).strip()
            if slug and slug not in seen:
                seen.add(slug)
                outlinks.append(slug)
    return outlinks


def _inside_wikilink(content: str, pos: int) -> bool:
    """Return True iff position ``pos`` falls inside an existing [[...]] span."""
    open_pos = content.rfind("[[", 0, pos)
    if open_pos < 0:
        return False
    # Find the FIRST "]]" that closes the "[[...]]" starting at open_pos
    close_pos = content.find("]]", open_pos + 2)
    if close_pos < 0:
        close_pos = len(content)
    return open_pos + 2 <= pos < close_pos


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
    entity_type: str = "entity",
) -> dict:
    """Per-entity REDUCE: compute additions/retractions vs existing page.

    Returns dict with action (create|update|delete|noop), additions, retractions,
    and entity_type for downstream filtering.
    """
    if existing_page is None:
        if isinstance(entity_type, list):
            entity_type = entity_type[0] if entity_type else "entity"
        entity_type = str(entity_type or "entity").strip()
        # A page must have grounded evidence. MAP can mention an entity as a
        # relation endpoint or metadata-only item without producing a claim;
        # creating a page for that item would persist empty source_doc_ids and
        # source_chunk_ids and make the REFINE prompt generate a placeholder
        # page. Such new entities/concepts are intentionally skipped.
        if not new_claims:
            return {
                "action": "noop",
                "entity_name": entity_name,
                "entity_type": entity_type,
                "additions": [],
                "retractions": [],
                "retained_source_doc_ids": [],
                "has_delta": False,
            }
        return {
            "action": "create",
            "entity_name": entity_name,
            "entity_type": entity_type,
            "additions": new_claims,
            "retained_source_doc_ids": list({c["source_doc_id"] for c in new_claims if c.get("source_doc_id")}),
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

    retractions = [c for c in existing_claims if c.get("source_doc_id") in deleted_set]

    retained_claims = [c for c in existing_claims if c.get("source_doc_id") not in deleted_set]

    retained_texts = {c.get("statement", c.get("text", "")) for c in retained_claims}
    additions = [c for c in new_claims if c.get("statement", c.get("text", "")) not in retained_texts]

    all_doc_ids = {c.get("source_doc_id") for c in retained_claims} | {c.get("source_doc_id") for c in additions}

    if not all_doc_ids:
        return {
            "action": "delete",
            "entity_name": entity_name,
            "entity_type": entity_type,
            "retractions": existing_claims,
            "has_delta": True,
        }
    elif additions or retractions:
        return {
            "action": "update",
            "entity_name": entity_name,
            "entity_type": entity_type,
            "additions": additions,
            "retractions": retractions,
            "retained_source_doc_ids": list(all_doc_ids),
            "has_delta": True,
        }
    return {
        "action": "noop",
        "entity_name": entity_name,
        "entity_type": entity_type,
        "retained_source_doc_ids": list(all_doc_ids),
        "has_delta": False,
    }


async def _wiki_reduce_batch(
    affected_names: set[str],
    existing_pages: dict[str, dict],
    deleted_doc_ids: set[str],
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
        if isinstance(entity_type, list):
            entity_type = entity_type[0] if entity_type else "entity"
        entity_type = str(entity_type or "entity").strip()

        tasks.append(
            _wiki_reduce_entity(
                entity_name=name,
                entity_type=entity_type,
                new_claims=claims,
                existing_page=name_to_page.get(name, existing_pages.get(name)),
                deleted_doc_ids=deleted_doc_ids,
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
            {"doc_id": doc_id},
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
) -> dict | None:
    """Run a single Mode A REFINE action on one concept page.

    Returns updated wiki_page dict, or None if deleted.
    """
    from common.misc_utils import thread_pool_exec

    # Blank page_id would produce `slug_kwd: [""]` queries → Infinity 3052.
    if not page_id or not str(page_id).strip():
        return existing_page

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
            force_full=True,
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
            force_full=False,
        )

    # Call LLM
    response = await _chat_mdl_ask(
        chat_mdl,
        system_prompt,
        user_prompt,
    )

    if not response or not response.strip():
        return existing_page  # keep existing

    # Parse response: expected format starts with "SUMMARY: ..." then content
    content = response.strip()
    summary = ""
    if content.startswith("SUMMARY:"):
        idx = content.find("\n")
        if idx > 0:
            summary = content[8:idx].strip()
            content = content[idx + 1 :].strip()

    # Build the wiki_page dict
    existing = existing_page or {}
    new_version = page_version + 1
    raw_doc_ids = existing.get("source_doc_ids", [])
    doc_ids = json.loads(raw_doc_ids) if isinstance(raw_doc_ids, str) else list(raw_doc_ids)
    source_chunk_ids = set(_as_str_list(existing.get("source_chunk_ids")))
    for claim in claims or []:
        did = claim.get("source_doc_id") if isinstance(claim, dict) else None
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

    # Embed for search
    from common.misc_utils import thread_pool_exec
    from rag.nlp import rag_tokenizer

    embeddings, _ = await thread_pool_exec(embd_mdl.encode, [summary or content[:200]])

    # Derive vector dimension from the embedding shape
    emb_arr = np.asarray(embeddings[0])
    vec_dim = int(emb_arr.shape[0]) if emb_arr.ndim >= 1 and emb_arr.shape[0] else 768
    content_ltks = rag_tokenizer.tokenize(content)

    page = {
        "id": _stable_row_id(WIKI_PAGE_COMPILE_KWD, kb_id, page_id),
        "slug_kwd": page_id,
        "title_kwd": page_title,
        "md_with_weight": content,
        "summary_with_weight": summary or page_title,
        "entity_names_kwd": [page_title],
        "source_chunk_ids": sorted(source_chunk_ids),
        "source_doc_ids": json.dumps(doc_ids, ensure_ascii=False),
        "claims": json.dumps(claims, ensure_ascii=False) if claims else "[]",
        "page_version_int": new_version,
        "synthesis_version_int": new_version if mode in ("generate", "re-synthesize") else existing.get("synthesis_version_int", 0),
        "page_type_kwd": page_type_kwd,
        "compile_kwd": WIKI_PAGE_COMPILE_KWD,
        "knowledge_graph_kwd": WIKI_PAGE_COMPILE_KWD,
        "title_tks": rag_tokenizer.tokenize(page_title),
        "content_ltks": content_ltks,
        "content_sm_ltks": rag_tokenizer.fine_grained_tokenize(content_ltks),
    }
    # Insert vector (adds q_{dim}_vec field)
    vec_col = f"q_{vec_dim}_vec"
    page[vec_col] = embeddings[0].tolist() if hasattr(embeddings[0], "tolist") else embeddings[0]

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
) -> str:
    chunks_text = _build_source_chunks_block(source_chunks)
    claims_text = "\n".join(f"- {c.get('statement', c.get('text', ''))}" for c in claims) if claims else "(no claims)"

    return f"""## Concept Page Identity
- Page ID: {page_id}
- Title: {page_title}

## Source Chunks (verbatim source text — ground every fact in these)
{chunks_text or "(no source chunks available)"}

## Extracted Claims (checklist)
{claims_text}

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
    force_full: bool = False,
) -> str:
    existing_content = existing_page.get("md_with_weight", "") if existing_page else ""

    if not force_full:
        additions_text = "\n".join(f"- {c.get('statement', c.get('text', ''))}" for c in (additions or [])) if additions else "(none)"
        retractions_text = "\n".join(f"- {c.get('statement', c.get('text', ''))}" for c in (retractions or [])) if retractions else "(none)"
        chunks_text = _build_source_chunks_block(source_chunks)

        return f"""## Page Identity
- Page ID: {page_id}
- Title: {page_title}

## Current Page
{existing_content[:10000] if existing_content else "(empty)"}

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

## All Source Chunks (for full re-synthesis — verbatim source text)
{chunks_text or "(none)"}

## All Claims
{claims_text or "(none)"}

## Available Pages for [[wikilinks]]
{chr(10).join(f"- {p}" for p in available_pages[:50]) if available_pages else "(none)"}

{contextual_hints}
"""


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

## SOURCE GROUNDING (COMPILER, not writer)
- The "Source Chunks" section contains VERBATIM source text. Stay close to the source wording — reuse the source's own sentences and facts where possible.
- Every newly added factual claim, entity, or numerical value MUST be directly supported by the provided source chunks. Do NOT invent facts, figures, dates, or relationships not present in the sources.
- Do NOT add rhetorical filler (e.g. "旨在帮助…", "designed to…", "aims to provide…") unless it appears verbatim in a source.
- If the sources disagree, present both views and add a "## Contradictions" section rather than silently picking one.

## OUTPUT
Return ONLY the complete markdown page.
First line: SUMMARY: {one-sentence description, 15-40 words}
Then the page content.
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
Then the updated page content.
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
                related = json.loads(rp)
            elif isinstance(rp, list):
                related = rp
    if not related:
        for name in _as_str_list(existing_page.get("entity_names_kwd") if existing_page else None):
            related.extend(all_relations.get(name, []))
    if not related:
        return ""

    lines = ["## Context: Related Entities & Concepts", "Reference them in the opening paragraph and relevant sections:"]
    for r in related[:10]:
        entity_name = r.get("entity_name") or r.get("name", "")
        relation = r.get("relation") or r.get("type", "related")
        lines.append(f"- [[{entity_name}]] — {relation}")
    return "\n".join(lines)


# ----- Mode B Page Router (KNN entity routing) -----------------------------


async def _wiki_page_router(
    affected_entities: list[dict],
    embd_mdl,
    tenant_id: str,
    kb_id: str,
    existing_page_ids: set[str] | None = None,
) -> dict[str, list[dict]]:
    """Route affected entities to existing wiki pages via KNN.

    Returns: {page_id: [entity_deltas]}
    - "_new_{page_id}" → new page to create
    - existing page_id → entities assigned to that page

    ``existing_page_ids`` is supplied by Mode B from its already-loaded page
    set. An explicitly empty set means this is a first build, so page-index
    KNN routing can be skipped and entities can go straight to clustering.
    """
    from common.misc_utils import thread_pool_exec
    from rag.nlp import search
    from common.doc_store.doc_store_base import OrderByExpr

    query_texts = [_entity_to_query_text(e) for e in affected_entities]
    embeddings, _ = await thread_pool_exec(embd_mdl.encode, query_texts)

    index = search.index_name(tenant_id)
    condition = {"compile_kwd": [WIKI_PAGE_COMPILE_KWD]}

    assignments: dict[str, list[dict]] = {}
    orphans: list[dict] = []
    embedding_by_entity_id = {id(entity): vec for entity, vec in zip(affected_entities, embeddings, strict=False)}

    if existing_page_ids is not None and not existing_page_ids:
        # On a first build there are no pages that can accept a routed entity.
        # Querying the page index once per entity only produces orphans, so go
        # directly to clustering and reuse the embeddings already computed.
        orphans = list(affected_entities)
    else:
        router_sem = asyncio.Semaphore(PAGE_ROUTER_KNN_CONCURRENT)

        async def _search_page(entity: dict, vec) -> tuple[dict, dict]:
            async with router_sem:
                match_expr = MatchDenseExpr(
                    vector_column_name=f"q_{len(vec)}_vec",
                    embedding_data=vec.tolist() if hasattr(vec, "tolist") else vec,
                    embedding_data_type="float",
                    distance_type="cosine",
                    topn=1,
                    extra_options={"similarity": PAGE_ROUTER_MAYBE_THRESHOLD},
                )
                res = await thread_pool_exec(
                    settings.docStoreConn.search,
                    ["slug_kwd", "title_kwd", "_score"],
                    [],
                    condition,
                    [match_expr],
                    OrderByExpr(),
                    0,
                    1,
                    index,
                    [kb_id],
                )
                return entity, settings.docStoreConn.get_fields(res, ["slug_kwd", "title_kwd", "_score"])

        route_results = await asyncio.gather(*(_search_page(entity, vec) for entity, vec in zip(affected_entities, embeddings, strict=False)))
        for entity, field_map in route_results:
            if not field_map:
                orphans.append(entity)
                continue

            for row in field_map.values():
                score = row.get("_score", 0.0)
                page_id = row.get("slug_kwd", "")
                if isinstance(page_id, (list, tuple)):
                    page_id = page_id[0] if page_id else ""
                page_id = str(page_id or "").strip()

                # slug_kwd is a *_kwd field; Infinity may return it empty / mangled
                # on the matched row. A blank page id would route the entity onto a
                # `slug_kwd: [""]` query (Infinity 3052) — treat as orphan instead.
                if not page_id:
                    orphans.append(entity)
                    continue

                if score >= PAGE_ROUTER_UPDATE_THRESHOLD:
                    assignments.setdefault(page_id, []).append(entity)
                elif score >= PAGE_ROUTER_MAYBE_THRESHOLD:
                    assignments.setdefault(f"_maybe_{page_id}", []).append(entity)
                else:
                    orphans.append(entity)
                break

    # Handle maybe candidates (batch LLM confirm optional)
    for key in list(assignments.keys()):
        if key.startswith("_maybe_"):
            page_id = key[7:]
            # Simple heuristic: assign to the page if any claim overlaps
            existing_page_claims = await _load_page_claims(tenant_id, kb_id, page_id)
            confirmed = []
            for entity in assignments[key]:
                entity_claim_texts = {c.get("statement", c.get("text", "")) for c in entity.get("claims", [])}
                existing_claim_texts = {ec.get("statement", ec.get("text", "")) for ec in (existing_page_claims or [])}
                if entity_claim_texts & existing_claim_texts:
                    confirmed.append(entity)
                else:
                    orphans.append(entity)
            if confirmed:
                assignments.setdefault(page_id, []).extend(confirmed)
            del assignments[key]

    # Orphans: cluster by similarity, create grouped pages
    if orphans:
        orphan_embs = [embedding_by_entity_id[id(entity)] for entity in orphans]
        clusters = _wiki_cluster_entities(orphans, orphan_embs, threshold=PAGE_ROUTER_CLUSTER_THRESHOLD)
        for cluster in clusters:
            names = [e.get("entity_name") or e.get("term", "") for e in cluster]
            # Mode B compiles EVERY entity/concept into a page. On a first build
            # there are no existing pages, so every affected entity lands here as
            # an orphan; do NOT gate page creation on a claim count or most pages
            # (esp. claim-light concepts/entities) would never be created.
            if not names:
                continue
            # Pick the page prefix from the cluster's dominant type. The default
            # prefix of _wiki_derive_page_id is "concept"; passing nothing would
            # mislabel every group (incl. people/orgs) as a concept page.
            any_concept = any((e.get("entity_type") or e.get("type")) == "concept" for e in cluster)
            prefix = "concept" if any_concept else "entity"
            page_id = _wiki_derive_page_id(names[0], prefix=prefix)
            if not page_id:
                continue
            assignments[f"_new_{page_id}"] = cluster

    return assignments


async def _load_page_claims(
    tenant_id: str,
    kb_id: str,
    page_id: str,
) -> list[dict]:
    """Load claims for a single wiki page."""
    from rag.nlp import search
    from common.misc_utils import thread_pool_exec
    from common.doc_store.doc_store_base import OrderByExpr

    index = search.index_name(tenant_id)
    condition = {"compile_kwd": [WIKI_PAGE_COMPILE_KWD], "slug_kwd": [page_id]}
    res = await thread_pool_exec(
        settings.docStoreConn.search,
        ["claims", "slug_kwd"],
        [],
        condition,
        [],
        OrderByExpr(),
        0,
        1,
        index,
        [kb_id],
    )
    field_map = settings.docStoreConn.get_fields(res, ["claims", "slug_kwd"])
    for row in field_map.values():
        claims = row.get("claims", "[]")
        if isinstance(claims, str):
            return json.loads(claims)
        return claims
    return []


def _wiki_cluster_entities(
    entities: list[dict],
    embeddings: list,
    threshold: float,
) -> list[list[dict]]:
    """Simple pairwise cosine clustering for orphan entities.

    Returns clusters where intra-cluster cosine >= threshold.
    Each cluster has at least 1 entity.
    """
    if len(entities) <= 1:
        return [entities]

    # Normalize embeddings
    embs = []
    for e in embeddings:
        if hasattr(e, "tolist"):
            e = e.tolist()
        arr = np.asarray(e, dtype=np.float32)
        norm = np.linalg.norm(arr)
        embs.append(arr / norm if norm > 0 else arr)

    n = len(embs)
    assigned = [False] * n
    clusters: list[list[int]] = []

    for i in range(n):
        if assigned[i]:
            continue
        cluster = [i]
        assigned[i] = True
        for j in range(i + 1, n):
            if assigned[j]:
                continue
            similarity = float(np.dot(embs[i], embs[j].T))
            if similarity >= threshold:
                cluster.append(j)
                assigned[j] = True
        clusters.append(cluster)

    result = []
    for cluster in clusters:
        result.append([entities[i] for i in cluster])
    return result


# ----- FINALIZE (shared) ----------------------------------------------------


async def _wiki_finalize(
    tenant_id: str,
    kb_id: str,
    embd_mdl,
    page_ids: list[str] | None = None,
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
    map_relations = await _load_map_relations(tenant_id, kb_id)
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
            if link in valid_ids and link != pid:
                # Valid wikilink → record for cross-reference + outlink
                relation_map.setdefault(pid, []).append(
                    {
                        "entity_name": link.split("/")[-1] if "/" in link else link,
                        "relation": "see_also",
                    }
                )
                outlink_map.setdefault(pid, []).append(link)
            elif link in canonical_names:
                # Entity reference (Mode A): remove [[]] keep plain text
                content = content.replace(f"[[{link}]]", link, 1)
            else:
                # Dead link — try WeKnora-style fuzzy resolution to a similar
                # existing page before giving up. If a close slug exists, retarget
                # the link (cross-link survives); otherwise degrade to plain text.
                resolved = _wiki_resolve_dead_slug(link, valid_ids, name_slug)
                if resolved:
                    content = content.replace(f"[[{link}]]", f"[[{resolved}]]", 1)
                    relation_map.setdefault(pid, []).append({"entity_name": resolved.split("/")[-1] if "/" in resolved else resolved, "relation": "see_also"})
                    if resolved not in outlink_map.setdefault(pid, []):
                        outlink_map[pid].append(resolved)
                else:
                    content = content.replace(f"[[{link}]]", link, 1)
                    dead_links.setdefault(pid, []).append(link)

        # AUTO-LINK: guarantee cross-page connections even when the LLM omits
        # [[...]]. Scan for standalone mentions of other pages' plain names and
        # wrap the FIRST occurrence with [[full_slug]], recording an outlink.
        # existing_links covers both the raw [[slug]] form and the rendered
        # `[text](artifact/{kb_id}/slug)` form (so re-runs stay idempotent even
        # after links were rendered on a previous run).
        existing_links = {m.group(1).strip() for m in wikilink_re.finditer(content)}
        existing_links |= {m.group(1) for m in re.finditer(rf"\]\(artifact/{re.escape(str(kb_id))}/([^)]+)\)", content)}
        for name in ordered_names:
            target = name_slug[name]
            if target == pid:
                continue
            if target in existing_links:
                continue
            idx = content.find(name)
            if idx < 0:
                continue
            # Skip if the occurrence is already inside a [[...]] link span
            if _inside_wikilink(content, idx):
                continue
            content = content[:idx] + f"[[{target}]]" + content[idx + len(name) :]
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

        index = search.index_name(tenant_id)
        await thread_pool_exec(
            settings.docStoreConn.update,
            {"id": page["id"]},
            update,
            index,
            kb_id,
        )


def _wiki_normalize_rows(matrix):
    """L2-normalize the rows of a 2-D float matrix (safe on zero rows)."""
    if matrix.ndim != 2:
        return matrix
    norms = np.linalg.norm(matrix, axis=1, keepdims=True)
    return np.divide(matrix, norms, out=np.zeros_like(matrix), where=norms > 0)


async def _wiki_load_map_topics(index, kb_id) -> list[str]:
    """Collect distinct thematic topic labels from persisted wiki_map_extract rows.

    Lets topic grouping run even when the current invocation carried no fresh MAP
    output (e.g. a no-op re-run over already-built pages). Bounded scan.
    """
    from common.doc_store.doc_store_base import OrderByExpr

    labels: list[str] = []
    seen: set[str] = set()
    offset, page_size, scanned = 0, 500, 0
    while scanned < 5000 and len(labels) < WIKI_TOPIC_MAX_LABELS:
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                ["content_with_weight"],
                [],
                {"compile_kwd": ["wiki_map_extract"]},
                [],
                OrderByExpr(),
                offset,
                page_size,
                index,
                [kb_id],
            )
            rows = settings.docStoreConn.get_fields(res, ["content_with_weight"]) or {}
        except Exception:
            logging.exception("wiki topics: map-topic load failed for kb=%s", kb_id)
            break
        if not rows:
            break
        for row in rows.values():
            raw = row.get("content_with_weight")
            if not isinstance(raw, str) or not raw:
                continue
            try:
                extract = json.loads(raw)
            except Exception:
                continue
            for t in (extract.get("topics") or []) if isinstance(extract, dict) else []:
                if isinstance(t, str):
                    t = t.strip()
                    key = t.lower()
                    if t and key != WIKI_TOPIC_FALLBACK.lower() and key not in seen:
                        seen.add(key)
                        labels.append(t)
                        if len(labels) >= WIKI_TOPIC_MAX_LABELS:
                            break
        scanned += len(rows)
        if len(rows) < page_size:
            break
        offset += page_size
    return labels


async def _wiki_assign_topics(
    embd_mdl,
    tenant_id: str,
    kb_id: str,
    map_topics: list[str] | None = None,
    callback: Callable | None = None,
) -> None:
    """Group concept/entity wiki pages under thematic topics (best-effort).

    No-plan pages have no PLAN grouping step, so pages are grouped post-hoc: each
    page is matched (embedding cosine) to the thematic topic labels the MAP phase
    extracted (accumulated with topics already on record so labels persist across
    runs). The best match above ``WIKI_TOPIC_MATCH_THRESHOLD`` wins, else the page
    lands in the ``WIKI_TOPIC_FALLBACK`` bucket. Every page's ``topic_kwd`` is
    stamped, so ``/artifacts_topics`` (which aggregates concept/entity pages by
    ``topic_kwd``) and the topic-filtered page list resolve. No landing rows are
    written — the topics API falls back to the raw topic name for title/slug.
    Any failure leaves the pages intact (just untopiced) and never raises.
    """
    from common.doc_store.doc_store_base import OrderByExpr

    def _progress(msg: str) -> None:
        if callback:
            try:
                callback(0.97, f"Topics: {msg}")
            except Exception:
                pass

    try:
        index = search.index_name(tenant_id)
        if not settings.docStoreConn.index_exist(index, kb_id):
            return

        # 1. Load all concept/entity pages.
        page_fields = ["slug_kwd", "title_kwd", "summary_with_weight", "source_doc_ids", "topic_kwd"]
        pages: list[dict] = []
        offset, page_size = 0, 1000
        while True:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                page_fields,
                [],
                {"compile_kwd": [WIKI_PAGE_COMPILE_KWD], "page_type_kwd": ["concept", "entity"]},
                [],
                OrderByExpr(),
                offset,
                page_size,
                index,
                [kb_id],
            )
            rows = settings.docStoreConn.get_fields(res, page_fields) or {}
            for row in rows.values():
                # get_fields may return scalar-ish *_kwd fields as a list (e.g.
                # an Infinity/ES aggregation). Normalize slug_kwd to a scalar
                # so it can be used as a dict key later; normalize title_kwd too.
                slug = row.get("slug_kwd")
                if isinstance(slug, (list, tuple)):
                    slug = slug[0] if slug else ""
                    row["slug_kwd"] = slug
                if isinstance(row.get("title_kwd"), (list, tuple)):
                    t = row.get("title_kwd")
                    row["title_kwd"] = t[0] if t else ""
                if slug:
                    pages.append(row)
            if len(rows) < page_size:
                break
            offset += page_size
        if not pages:
            return

        # 2. Candidate labels: this run's MAP topics + topics already stamped on
        #    the pages from earlier runs (so labels accumulate across runs).
        existing_labels: list[str] = []
        for p in pages:
            t = p.get("topic_kwd")
            if isinstance(t, str) and t.strip() and t.strip().lower() != WIKI_TOPIC_FALLBACK.lower():
                existing_labels.append(t.strip())

        labels: list[str] = []
        seen: set[str] = set()
        for t in list(map_topics or []) + existing_labels:
            if not isinstance(t, str):
                continue
            t = t.strip()
            key = t.lower()
            if t and key != WIKI_TOPIC_FALLBACK.lower() and key not in seen:
                seen.add(key)
                labels.append(t)
            if len(labels) >= WIKI_TOPIC_MAX_LABELS:
                break

        # Backfill labels from the persisted MAP extracts when this run carried
        # none (e.g. a no-op re-run over pages built before topic grouping).
        if not labels:
            labels = await _wiki_load_map_topics(index, kb_id)

        # 3. Assign each page to its nearest topic (or the fallback bucket).
        assignments: dict[str, str] = {}
        topic_docs: dict[str, set] = {}

        def _record(slug, topic: str, doc_ids) -> None:
            # slug may come back as a list from some doc-store get_fields
            # implementations — normalize to a scalar before keying.
            if isinstance(slug, (list, tuple)):
                slug = slug[0] if slug else ""
            if not slug:
                return
            assignments[slug] = topic
            bucket = topic_docs.setdefault(topic, set())
            raw = doc_ids
            if isinstance(raw, str):
                try:
                    raw = json.loads(raw)
                except Exception:
                    raw = [raw]
            for d in raw or []:
                if isinstance(d, str) and d:
                    bucket.add(d)

        topic_matrix = None
        if labels:
            tvecs, _ = await thread_pool_exec(embd_mdl.encode, labels)
            topic_matrix = _wiki_normalize_rows(np.asarray(tvecs, dtype=np.float32))
        if topic_matrix is not None and topic_matrix.ndim == 2 and topic_matrix.shape[0] == len(labels):
            page_texts = [f"{p.get('title_kwd') or ''} {p.get('summary_with_weight') or ''}".strip() or (p.get("slug_kwd") or "") for p in pages]
            pvecs, _ = await thread_pool_exec(embd_mdl.encode, page_texts)
            page_matrix = _wiki_normalize_rows(np.asarray(pvecs, dtype=np.float32))
            if page_matrix.ndim == 2 and page_matrix.shape[0] == len(pages):
                sims = page_matrix @ topic_matrix.T
                best = np.argmax(sims, axis=1)
                for i, p in enumerate(pages):
                    score = float(sims[i, best[i]])
                    topic = labels[int(best[i])] if score >= WIKI_TOPIC_MATCH_THRESHOLD else WIKI_TOPIC_FALLBACK
                    _record(p["slug_kwd"], topic, p.get("source_doc_ids"))
        if not assignments:
            # No usable embeddings/labels → single fallback topic keeps nav working.
            for p in pages:
                _record(p["slug_kwd"], WIKI_TOPIC_FALLBACK, p.get("source_doc_ids"))

        by_topic: dict[str, list[str]] = {}
        for slug, topic in assignments.items():
            by_topic.setdefault(topic, []).append(slug)

        # 4. Stamp topic_kwd on each page (bounded concurrency).
        sem = asyncio.Semaphore(WIKI_TOPIC_UPDATE_CONCURRENT)

        async def _stamp(slug: str, topic: str) -> None:
            async with sem:
                try:
                    await thread_pool_exec(
                        settings.docStoreConn.update,
                        {"compile_kwd": [WIKI_PAGE_COMPILE_KWD], "slug_kwd": [slug]},
                        {"topic_kwd": topic},
                        index,
                        kb_id,
                    )
                except Exception:
                    logging.exception("wiki topics: topic_kwd update failed for slug=%s", slug)

        await asyncio.gather(*[_stamp(slug, topic) for slug, topic in assignments.items()])

        # No landing rows are written: list_wiki_topics derives topics from the
        # pages' topic_kwd aggregation and falls back to the raw topic name for
        # title/slug, so page_type="topic" rows would only pollute the page list.
        _ = topic_docs  # provenance retained for a future topic-page feature
        _progress(f"grouped {len(pages)} page(s) into {len(by_topic)} topic(s).")
    except Exception:
        logging.exception("wiki topics: assignment failed for kb=%s", kb_id)


# ----- Main entry point -----------------------------------------------------


async def wiki_compile_incremental(
    *,
    chat_mdl,
    embd_mdl,
    tenant_id: str,
    kb_id: str,
    plan: bool = False,  # True = Mode B, False = Mode A
    incremental: bool = False,  # True = incremental run
    map_results: list[dict] | None = None,  # from MAP phase
    deleted_doc_ids: set[str] | None = None,
    callback: Callable | None = None,
) -> dict:
    """Main entry point for dual-mode wiki compilation.

    Args:
        plan: True=Mode B (with PLAN), False=Mode A (no-plan, WeKnora style)
        incremental: True=incremental update, False=full build
        map_results: MAP outputs. If None, loads from ES.
        deleted_doc_ids: Documents that were removed.
        callback: Progress callback.

    Returns summary dict: {pages_created, pages_modified, pages_deleted}
    """
    from common.misc_utils import thread_pool_exec
    from rag.nlp import search
    from common.doc_store.doc_store_base import OrderByExpr

    summary = {"pages_created": 0, "pages_modified": 0, "pages_deleted": 0, "errors": []}

    def _progress(msg: str):
        if callback:
            try:
                callback(0.5, msg)
            except Exception:
                pass

    # ----- Phase 1: Load MAP results if not provided -----
    if not map_results:
        _progress("Loading MAP results from doc store ...")
        map_results = []
        index = search.index_name(tenant_id)
        # Each wiki_map_extract row stores its per-chunk extract as a JSON blob in
        # ``content_with_weight`` (see _wiki_build_resume_doc) — the entity /
        # concept / claim / relation / topic lists are NOT separate columns, so
        # they must be parsed out of that blob to rebuild the map_result shape
        # that _extract_raw_entities and REDUCE expect.
        select_fields = ["content_with_weight", "doc_id"]
        offset = 0
        page_size = 1000
        while True:
            try:
                res = await thread_pool_exec(
                    settings.docStoreConn.search,
                    select_fields,
                    [],
                    {"compile_kwd": ["wiki_map_extract"]},
                    [],
                    OrderByExpr(),
                    offset,
                    page_size,
                    index,
                    [kb_id],
                )
                field_map = settings.docStoreConn.get_fields(res, select_fields) or {}
            except Exception:
                logging.exception("wiki: failed to load MAP results for kb=%s", kb_id)
                break
            for row in field_map.values():
                raw = row.get("content_with_weight")
                if isinstance(raw, str) and raw:
                    try:
                        extract = json.loads(raw)
                    except Exception:
                        extract = None
                elif isinstance(raw, dict):
                    extract = raw
                else:
                    extract = None
                if not isinstance(extract, dict):
                    continue
                extract["doc_id"] = row.get("doc_id", "")
                map_results.append(extract)
            if len(field_map) < page_size:
                break
            offset += page_size

    if not map_results:
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
    raw_entities, claim_index = _extract_raw_entities(map_results)

    # Collect the thematic topic labels the MAP phase extracted, for the Phase 6
    # topic grouping — done here while map_results is still alive.
    map_topics: list[str] = []
    _seen_topics: set[str] = set()
    for _mr in map_results:
        for _t in _mr.get("topics") or []:
            if isinstance(_t, str):
                _t = _t.strip()
                _k = _t.lower()
                if _t and _k not in _seen_topics:
                    _seen_topics.add(_k)
                    map_topics.append(_t)

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

    # raw_entities (lightweight) no longer needed after matching.
    del raw_entities

    if not canonical_map:
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
            if old_docs != new_docs or centry["claim_count"] > existing.get("mention_count_int", 0):
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
            )
            for cname, centry, _ in new_items
        ]
        await thread_pool_exec(
            settings.docStoreConn.insert,
            new_rows,
            search.index_name(tenant_id),
            kb_id,
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
        # Affected doc ids are the docs contributing to this batch's canonical
        # entities, plus any deleted docs.  Derived from canonical_map (which
        # carries source_doc_ids) — no need to re-read the released map_results.
        affected_doc_ids = set()
        for centry in canonical_map.values():
            affected_doc_ids.update(centry.get("source_doc_ids", []))
        affected_doc_ids = affected_doc_ids | (deleted_doc_ids or set())

        # Map doc_page_source entity_names (raw) through name_resolution -> canonical
        affected_names: set[str] = set()
        if affected_doc_ids:
            dps_tasks = [_wiki_load_doc_page_source(tenant_id, kb_id, did) for did in affected_doc_ids]
            dps_results = await asyncio.gather(*dps_tasks)
            for dps in dps_results:
                if dps:
                    for raw_name in dps.get("entity_names", []):
                        cname = name_resolution.get(raw_name, raw_name)
                        if cname in canonical_names:
                            affected_names.add(cname)
        if not affected_names:
            affected_names = canonical_names
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
            "related_kb_pages_kwd",
            "page_type_kwd",
        ],
    )

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
        canonical_claims=canonical_claims,
        canonical_map=canonical_map,
        name_resolution=name_resolution,
    )

    if not deltas:
        _progress("REDUCE: no changes detected.")
        # Still (re)group existing pages under topics — covers pages that were
        # built before topic grouping existed, or a run where topics changed but
        # no page's claims did.
        await _wiki_assign_topics(embd_mdl, tenant_id, kb_id, map_topics, callback)
        return summary

    # ----- Phase 4: Mode-specific dispatch -----
    # Precompute doc → canonical entity names for doc_page_source tracking,
    # before canonical_map is released.
    doc_to_entities: dict[str, list[str]] = {}
    for cname, centry in canonical_map.items():
        for did in centry.get("source_doc_ids", []):
            doc_to_entities.setdefault(did, []).append(cname)
    del canonical_map

    if plan:
        summary = await _wiki_mode_b_run(
            deltas=deltas,
            existing_pages=existing_pages,
            chat_mdl=chat_mdl,
            embd_mdl=embd_mdl,
            tenant_id=tenant_id,
            kb_id=kb_id,
            incremental=incremental,
            callback=callback,
            doc_to_entities=doc_to_entities,
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
        )
    del deltas
    del canonical_claims
    del name_resolution
    del existing_pages

    # ----- Phase 5: Update doc_page_source with canonical names -----
    # (doc_page_source page_ids is already handled in mode_run)
    _progress("FINALIZE: updating cross-references ...")
    try:
        await _wiki_finalize(tenant_id, kb_id, embd_mdl)
    except Exception:
        logging.exception("wiki: FINALIZE failed for kb=%s", kb_id)
        summary["errors"].append("FAILED_FINALIZE")

    # ----- Phase 6: Thematic topic grouping -----
    _progress("Grouping pages under topics ...")
    await _wiki_assign_topics(embd_mdl, tenant_id, kb_id, map_topics, callback)

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
            }
        entry = page_deltas[page_id]
        entry["additions"].extend(d.get("additions", []))
        entry["retractions"].extend(d.get("retractions", []))
        entry["claims"].extend(d.get("claims", []))

        # Collect source chunks from claims
        for claim in d.get("claims", []):
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
                    return

                next_version = (existing.get("page_version_int", 0) if existing else 0) + 1
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
                )
                if refine_mode == "generate":
                    summary["pages_created"] += 1
                else:
                    summary["pages_modified"] += 1

                if result:
                    for c in entry["additions"]:
                        did = c.get("source_doc_id")
                        if did:
                            doc_updates.setdefault(did, []).append(pid)

            except Exception:
                logging.exception("wiki A: REFINE failed for %s", pid)
                summary["errors"].append(f"REFINE_FAILED:{pid}")

    tasks = [_refine_one(pid, entry) for pid, entry in page_deltas.items()]
    if tasks:
        _progress(f"REFINE A: {len(tasks)} pages (LLM pool max {WIKI_REFINE_MAX_CONCURRENT}) ...")
        await asyncio.gather(*tasks)

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


async def _wiki_mode_b_run(
    *,
    deltas: list[dict],
    existing_pages: dict[str, dict],
    chat_mdl,
    embd_mdl,
    tenant_id: str,
    kb_id: str,
    incremental: bool,
    callback: Callable | None = None,
    doc_to_entities: dict[str, list[str]] | None = None,
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
            "claims": d.get("additions", []) + d.get("claims", []),
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
        embd_mdl=embd_mdl,
        tenant_id=tenant_id,
        kb_id=kb_id,
        existing_page_ids=set(existing_pages),
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
                action = "create" if is_new else "update"
                for ent in entities:
                    additions.extend(ent.get("claims", []))
                    if ent.get("action") == "delete":
                        action = "delete"

                if action == "delete":
                    await _wiki_refine_page(
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
                    summary["pages_deleted"] += 1
                    return

                refine_mode = "generate" if is_new else "modify"
                if existing and _wiki_should_re_synthesize(
                    existing,
                    {c.get("source_doc_id") for c in additions if c.get("source_doc_id")},
                    existing.get("page_version_int", 0) + 1,
                ):
                    refine_mode = "re-synthesize"

                result = await _wiki_refine_page(
                    mode=refine_mode,
                    page_id=page_key,
                    page_title=existing.get("title_kwd", page_key) if existing else entities[0].get("entity_name", page_key),
                    existing_page=existing,
                    page_type_kwd=page_type,
                    additions=additions,
                    retractions=[],
                    source_chunks=page_source_chunks.get(page_key, []),
                    claims=additions,
                    available_pages=all_page_ids,
                    contextual_hints=_wiki_build_contextual_hints(page_key, existing, {}),
                    chat_mdl=chat_mdl,
                    embd_mdl=embd_mdl,
                    tenant_id=tenant_id,
                    kb_id=kb_id,
                    page_version=existing.get("page_version_int", 0) if existing else 0,
                )
                if is_new:
                    summary["pages_created"] += 1
                else:
                    summary["pages_modified"] += 1

                if result:
                    await _wiki_update_plan_group(
                        tenant_id,
                        kb_id,
                        page_key,
                        entity_names=[e.get("entity_name", "") for e in entities],
                        page_version=result.get("page_version_int", 1),
                    )

                    # Collect doc_page_source updates (deferred)
                    for ent in entities:
                        for c in ent.get("claims", []):
                            did = c.get("source_doc_id")
                            if did:
                                doc_updates.setdefault(did, []).append(page_key)

            except Exception:
                logging.exception("wiki B: REFINE failed for %s", page_id)
                summary["errors"].append(f"REFINE_FAILED:{page_id}")

    tasks = [_refine_one(pid, ents) for pid, ents in assignments.items()]
    if tasks:
        _progress(f"REFINE B: {len(tasks)} pages (LLM pool max {WIKI_REFINE_MAX_CONCURRENT}) ...")
        await asyncio.gather(*tasks)

    # Apply doc_page_source updates serially (no race), preserving metadata
    for did, pids in doc_updates.items():
        try:
            existing_dps = (await _wiki_load_doc_page_source(tenant_id, kb_id, did)) or {}
            existing_pids = existing_dps.get("page_ids", [])
            for pid in pids:
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


async def wiki_handle_document_deleted(
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    chat_mdl,
    embd_mdl,
    plan: bool = False,
) -> dict:
    """Clean up wiki pages + canonical entities when a document is deleted.

    Args:
        plan: True=Mode B (update plan_group), False=Mode A

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
                    )

    affected_page_ids = dps.get("page_ids", [])
    if not affected_page_ids:
        return summary

    # Step 2: Update wiki pages
    all_existing_pages = await _search_existing_pages(
        tenant_id,
        kb_id,
        ["slug_kwd", "title_kwd", "md_with_weight", "claims", "source_doc_ids", "page_version_int", "entity_names_kwd", "page_type_kwd"],
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

            page_type = existing.get("page_type_kwd", "concept" if not plan else "entity")

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

            if plan:
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
