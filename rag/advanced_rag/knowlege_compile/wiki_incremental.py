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
from rag.nlp import search


# ----- REFINE concurrency control -----

WIKI_REFINE_MAX_CONCURRENT = 12  # max LLM calls at once for page REFINE


# ----- constants ----

# compile_kwd values
WIKI_PAGE_COMPILE_KWD = "wiki_page"
WIKI_PLAN_GROUP_COMPILE_KWD = "wiki_plan_group"
WIKI_DOC_PAGE_SOURCE_COMPILE_KWD = "wiki_doc_page_source"
WIKI_CANONICAL_ENTITY_COMPILE_KWD = "wiki_canonical_entity"

# Entity matching thresholds (not exposed in YAML)
ENTITY_MERGE_THRESHOLD = 0.90  # auto-merge
ENTITY_AMBIGUOUS_LOW = 0.75  # LLM confirm boundary
ENTITY_PAIRWISE_BATCH_SIZE = 100  # embedding pairwise batch

# Number of concurrent KNN queries for entity matching
ENTITY_MATCH_KNN_CONCURRENT = 20

# Page Router thresholds (kept as code constants — not exposed in YAML)
PAGE_ROUTER_UPDATE_THRESHOLD = 0.80
PAGE_ROUTER_MAYBE_THRESHOLD = 0.50
PAGE_ROUTER_CLUSTER_THRESHOLD = 0.50

# Concept depth threshold (Mode A): a concept needs enough evidence to
# warrant a standalone wiki page.
CONCEPT_MIN_CLAIMS = 3
CONCEPT_MIN_SOURCES = 2

# Re-synthesis triggers (both modes)
RE_SYNTHESIS_MIN_SOURCES = 5
RE_SYNTHESIS_GROWTH_RATIO = 1.5
RE_SYNTHESIS_MIN_CLAIMS = 15
RE_SYNTHESIS_MIN_VERSIONS = 3

# FINALIZE debounce
FINALIZE_DEBOUNCE_SECONDS = 300


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
            entity.get("entity_name") or entity.get("term") or "",
            entity.get("definition_excerpt") or entity.get("description") or entity.get("statement", ""),
        ][:2]
    )


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


# ----- Canonical Entity Index CRUD -----------------------------------------


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
                ["entity_kwd", "entity_type_kwd", "aliases", "aliases_flat_kwd", "source_doc_ids", "mention_count_int"],
                [],
                {"compile_kwd": [WIKI_CANONICAL_ENTITY_COMPILE_KWD]},
                [],
                OrderByExpr(),
                offset,
                page_size,
                index,
                [kb_id],
            )
            field_map = settings.docStoreConn.get_fields(res, ["entity_kwd", "entity_type_kwd", "aliases", "aliases_flat_kwd", "source_doc_ids", "mention_count_int"]) or {}
        except Exception:
            logging.exception("wiki: failed to load canonical entities for kb=%s", kb_id)
            return results
        for row in field_map.values():
            name = row.get("entity_kwd", "")
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
                results[name] = row
        if len(field_map) < page_size:
            break
        offset += page_size
    return results


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
    dim = len(embedding) if embedding else 768
    doc = {
        "entity_kwd": entity_name,
        "entity_type_kwd": entity_type,
        "aliases": json.dumps(list(set(aliases)), ensure_ascii=False),
        "aliases_flat_kwd": "||".join(sorted(set([entity_name] + (aliases or [])))),
        "source_doc_ids": json.dumps(list(set(source_doc_ids)), ensure_ascii=False),
        "mention_count_int": claim_count,
        "compile_kwd": WIKI_CANONICAL_ENTITY_COMPILE_KWD,
        "kb_id": kb_id,
    }
    if embedding is not None:
        vec_col = f"q_{dim}_vec"
        doc[vec_col] = embedding

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


def _extract_raw_entities(map_results: list[dict]) -> list[dict]:
    """Extract raw entity/concept entries from MAP results.

    Returns list of dicts: {name, type, aliases, claim_count, claims, ...}
    Also builds concept entries from concepts[] field.
    """
    raw: dict[str, dict] = {}
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
                    "claims": [],
                    "claim_count": 0,
                    "source_doc_ids": set(),
                    "source_chunk_ids": set(),
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
                    "claims": [],
                    "claim_count": 0,
                    "source_doc_ids": set(),
                    "source_chunk_ids": set(),
                }
            raw[term]["source_doc_ids"].add(doc_id)

        # Process claims and associate with entities/concepts
        for claim in mr.get("claims") or []:
            if isinstance(claim, str):
                claim = json.loads(claim)
            subj = claim.get("entity_name") or claim.get("subject") or claim.get("term", "")
            if not subj or subj not in raw:
                continue
            raw[subj]["claims"].append(claim)
            raw[subj]["claim_count"] += 1
            cid = claim.get("source_chunk_id")
            if cid:
                raw[subj]["source_chunk_ids"].add(cid)

    result = []
    for entry in raw.values():
        entry["source_doc_ids"] = list(entry["source_doc_ids"])
        entry["source_chunk_ids"] = list(entry["source_chunk_ids"])
        result.append(entry)
    return result


async def _wiki_match_entities(
    raw_entities: list[dict],
    existing_canonical: dict[str, dict],
    embd_mdl,
    chat_mdl,
    tenant_id: str,
    kb_id: str,
    incremental: bool,
) -> tuple[dict[str, dict], dict[str, str]]:
    """Entity Matching: raw entities → canonical entities.

    Returns:
        canonical_map: {canonical_name: merged_entry}
        name_resolution: {raw_name: canonical_name}
    """
    # Step 1: Exact match against canonical index
    exact_flat: dict[str, str] = {}  # normalized_name → canonical_name
    for cname, centry in existing_canonical.items():
        flat = centry.get("aliases_flat_kwd", "")
        if not flat:
            continue
        for alias in flat.split("||"):
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

        knn_tasks = [_async_knn(entry, emb) for entry, emb in zip(unmatched, embeddings)]
        knn_results = await asyncio.gather(*knn_tasks)

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
    if not incremental and len(unmatched) > 1 and embd_mdl:
        query_texts = [_entity_to_query_text(e) for e in unmatched]
        embeddings, _ = await thread_pool_exec(embd_mdl.encode, query_texts)

        # Pairwise embedding — adapted from old _embedding_dedup
        merged_into: dict[int, int] = {}  # index → parent index
        maybe_pairs: list[tuple[int, int]] = []

        def _find_parent(x: int) -> int:
            while x in merged_into:
                x = merged_into[x]
            return x

        n = len(unmatched)
        for i in range(n):
            if unmatched[i]["type"] == "concept":
                continue  # concepts don't do LLM pairwise
            for j in range(i + 1, n):
                if unmatched[j]["type"] == "concept":
                    continue
                ei = embeddings[i]
                ej = embeddings[j]
                if hasattr(ei, "tolist"):
                    ei = ei.tolist()
                if hasattr(ej, "tolist"):
                    ej = ej.tolist()
                sim = float(np.dot(ei, ej) / (np.linalg.norm(ei) * np.linalg.norm(ej) + 1e-10))
                if sim >= ENTITY_MERGE_THRESHOLD:
                    pi, pj = _find_parent(i), _find_parent(j)
                    if pi != pj:
                        merged_into[pj] = pi
                elif sim >= ENTITY_AMBIGUOUS_LOW:
                    maybe_pairs.append((i, j))

        # LLM confirm for maybe pairs (first build only)
        if maybe_pairs and chat_mdl:
            llm_candidates = []
            for i, j in maybe_pairs:
                pi, pj = _find_parent(i), _find_parent(j)
                if pi != pj:
                    llm_candidates.append((unmatched[i]["name"], unmatched[j]["name"]))
            if llm_candidates:
                confirmed = await _wiki_confirm_batch(llm_candidates, chat_mdl)
                confirmed_set = set()
                for raw_a, raw_b in confirmed:
                    idx_a = next(k for k, e in enumerate(unmatched) if e["name"] == raw_a)
                    idx_b = next(k for k, e in enumerate(unmatched) if e["name"] == raw_b)
                    pa, pb = _find_parent(idx_a), _find_parent(idx_b)
                    if pa != pb:
                        merged_into[pb] = pa
                        confirmed_set.add(raw_a)
                        confirmed_set.add(raw_b)

        # Apply merges
        merged_indices: dict[int, list[int]] = {}
        for i in range(n):
            pi = _find_parent(i)
            merged_indices.setdefault(pi, []).append(i)

        new_unmatched = []
        for pi, indices in merged_indices.items():
            if len(indices) > 1:
                # Merge into the first entry (most representative)
                master = unmatched[indices[0]]
                for idx in indices[1:]:
                    slave = unmatched[idx]
                    master["claims"].extend(slave["claims"])
                    master["claim_count"] += slave["claim_count"]
                    master["source_doc_ids"] = list(set(master["source_doc_ids"]) | set(slave["source_doc_ids"]))
                    master["aliases"] = list(set(master["aliases"] + slave["aliases"] + [slave["name"]]))
                    # Canonical name: keep the most common one
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
                # Merge raw entity claims into existing
                merged = {
                    "name": cname,
                    "type": existing.get("entity_type_kwd", "entity"),
                    "aliases": existing.get("aliases", []),
                    "claims": [],
                    "claim_count": existing.get("mention_count_int", 0),
                    "source_doc_ids": existing.get("source_doc_ids", []),
                    "source_chunk_ids": [],
                }
                canonical_map[cname] = merged

    # Merge raw entity claims into canonical entries
    for entry in raw_entities:
        raw_name = entry["name"]
        cname = name_resolution.get(raw_name, raw_name)
        if cname in canonical_map:
            # Add claims
            for claim in entry.get("claims", []):
                canonical_map[cname]["claims"].append(claim)
            # Update counts
            canonical_map[cname]["claim_count"] += len(entry.get("claims", []))
            # Merge doc IDs
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
            from rag.advanced_rag.knowlege_compile.structure import chat_mdl_ask

            resp = await chat_mdl_ask(chat_mdl, "You are a KB dedup assistant.", prompt)
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
            slug = row.get("slug_kwd", row.get("page_id", ""))
            if isinstance(slug, str) and slug:
                results[slug] = row
        if len(field_map) < page_size:
            break
        offset += page_size
    return results


def _wiki_decide_concept_pages(all_concepts: list[dict]) -> list[dict]:
    """Filter concepts: only those with enough depth become wiki pages."""
    pages = []
    for concept in all_concepts:
        claims = concept.get("claims", [])
        source_docs = set(c.get("source_doc_id") for c in claims if c.get("source_doc_id"))
        if len(claims) >= CONCEPT_MIN_CLAIMS and len(source_docs) >= CONCEPT_MIN_SOURCES:
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
        return {
            "action": "create",
            "entity_name": entity_name,
            "entity_type": entity_type,
            "additions": new_claims,
            "retained_source_doc_ids": list({c["source_doc_id"] for c in new_claims if c.get("source_doc_id")}),
            "has_delta": bool(new_claims),
        }

    existing_claims = existing_page.get("claims", [])
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
    map_results: list[dict],
    existing_pages: dict[str, dict],
    deleted_doc_ids: set[str],
    canonical_claims: dict[str, list[dict]] | None = None,
    canonical_map: dict[str, dict] | None = None,
    name_resolution: dict[str, str] | None = None,
) -> list[dict]:
    """Parallel per-entity REDUCE over a batch of affected canonical names.

    Uses canonical_claims (from Entity Matching) instead of raw MAP claims.
    """
    name_to_page: dict[str, dict] = {}
    for pid, page in existing_pages.items():
        names = page.get("entity_names_kwd", [])
        if isinstance(names, str):
            names = json.loads(names) if names else []
        for n in names:
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


async def _wiki_mode_a_refine(
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

    if mode == "delete":
        await thread_pool_exec(
            settings.docStoreConn.delete,
            {"compile_kwd": [WIKI_PAGE_COMPILE_KWD], "slug_kwd": [page_id]},
            search.index_name(tenant_id),
            kb_id,
        )
        return None

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
    from rag.advanced_rag.knowlege_compile.structure import chat_mdl_ask

    response = await chat_mdl_ask(
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
    if source_chunks:
        for chunk in source_chunks:
            did = chunk.get("doc_id") or chunk.get("source_doc_id")
            if did and did not in doc_ids:
                doc_ids.append(did)

    # Embed for search
    from common.misc_utils import thread_pool_exec

    embeddings, _ = await thread_pool_exec(embd_mdl.encode, [summary or content[:200]])

    # Derive vector dimension from the embedding shape
    emb_arr = np.asarray(embeddings[0])
    vec_dim = int(emb_arr.shape[0]) if emb_arr.ndim >= 1 and emb_arr.shape[0] else 768

    page = {
        "slug_kwd": page_id,
        "title_kwd": page_title,
        "md_with_weight": content,
        "summary_with_weight": summary or page_title,
        "entity_names_kwd": json.dumps([page_title], ensure_ascii=False),
        "source_doc_ids": json.dumps(doc_ids, ensure_ascii=False),
        "claims": json.dumps(claims, ensure_ascii=False) if claims else "[]",
        "page_version_int": new_version,
        "synthesis_version_int": new_version if mode in ("generate", "re-synthesize") else existing.get("synthesis_version_int", 0),
        "page_type_kwd": page_type_kwd,
        "compile_kwd": WIKI_PAGE_COMPILE_KWD,
        "knowledge_graph_kwd": WIKI_PAGE_COMPILE_KWD,
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

    return page


def _build_mode_a_generate_prompt(
    page_id: str,
    page_title: str,
    claims: list[dict],
    source_chunks: list[dict],
    available_pages: list[str],
    contextual_hints: str,
) -> str:
    chunks_text = "\n\n".join(f"[CHUNK {c.get('id', c.get('chunk_id', i))}]\n{c.get('content_with_weight') or c.get('text', '')}" for i, c in enumerate(source_chunks))
    claims_text = "\n".join(f"- {c.get('statement', c.get('text', ''))}" for c in claims) if claims else "(no claims)"

    return f"""## Concept Page Identity
- Page ID: {page_id}
- Title: {page_title}

## Source Chunks
{chunks_text or "(no source chunks available)"}

## Extracted Claims
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
        chunks_text = "\n\n".join(f"[CHUNK {c.get('id', i)}]\n{c.get('content_with_weight') or c.get('text', '')}" for i, c in enumerate(source_chunks))[: _get_source_budget_chars()]

        return f"""## Page Identity
- Page ID: {page_id}
- Title: {page_title}

## Current Page
{existing_content[:10000] if existing_content else "(empty)"}

## New Claims to Add
{additions_text}

## Claims to Retract
{retractions_text}

## Source Chunks for New Information
{chunks_text}

## Available Pages for [[wikilinks]]
{chr(10).join(f"- {p}" for p in available_pages[:30]) if available_pages else "(none)"}

{contextual_hints}
"""
    else:
        # Full re-synthesis: all claims + all source chunks
        chunks_text = "\n\n".join(f"[CHUNK {c.get('id', i)}]\n{c.get('content_with_weight') or c.get('text', '')}" for i, c in enumerate(source_chunks))[: _get_source_budget_chars(max_budget=120_000)]
        claims_text = "\n".join(f"- {c.get('statement', c.get('text', ''))}" for c in claims)

        return f"""## Page Identity
- Page ID: {page_id}
- Title: {page_title}

## All Source Chunks (for full re-synthesis)
{chunks_text or "(none)"}

## All Claims
{claims_text or "(none)"}

## Available Pages for [[wikilinks]]
{chr(10).join(f"- {p}" for p in available_pages[:50]) if available_pages else "(none)"}

{contextual_hints}
"""


def _get_source_budget_chars(max_budget: int = 60_000) -> int:
    return max_budget


# System prompts for Mode A

_WIKI_MODE_A_GENERATE_SYSTEM = """You are a wiki writer. Generate a new wiki page for the given concept using the provided source chunks and extracted claims.

## RULES
1. CONCEPT PAGE: This is a single-concept wiki page. Organize by THEME, not by entity.
2. CROSS-DOCUMENT SYNTHESIS: Weave information from multiple sources into coherent paragraphs. Compare evidence, explain contradictions.
3. OPENING PARAGRAPH: 2-4 sentences defining the concept. Mention key entities. No heading.
4. SECTIONS: H2 headings, prose first, then sub-points if needed.
5. WIKILINKS: [[page_id]] for related concepts/entities on first mention.
6. DICTIONARY PREVENTION: Do NOT group content by source document. Do NOT create one section per entity. Do NOT write flat bullet lists.

## OUTPUT
Return ONLY the complete markdown page.
First line: SUMMARY: {one-sentence description, 15-40 words}
Then the page content.
"""

_WIKI_MODE_A_MODIFY_SYSTEM = """You are a wiki editor. Update the existing page by integrating new information and removing retracted content.

## RULES
1. CONCEPT PAGE: This is a single-concept wiki page. Organize by THEME, not by entity.
2. CROSS-DOCUMENT SYNTHESIS: Connect new claims to existing content. Weave them into the SAME paragraphs.
3. OPENING PARAGRAPH: Should reflect the FULL updated picture.
4. WIKILINKS: Keep existing and add new [[page_id]] links where appropriate.
5. For FULL RE-SYNTHESIS: Use all source chunks + all claims to rewrite from scratch.
6. For INCREMENTAL MODIFY: Integrate additions, remove retracted content, keep unchanged content.

## DICTIONARY PREVENTION
- Do NOT group content by source document.
- Do NOT simply append new claims at the end.
- Do NOT create one section per entity.

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
        names = existing_page.get("entity_names_kwd", []) if existing_page else []
        if isinstance(names, str):
            names = json.loads(names)
        for name in names:
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
) -> dict[str, list[dict]]:
    """Route affected entities to existing wiki pages via KNN.

    Returns: {page_id: [entity_deltas]}
    - "_new_{page_id}" → new page to create
    - existing page_id → entities assigned to that page
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

    for entity, vec in zip(affected_entities, embeddings):
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
        field_map = settings.docStoreConn.get_fields(res, ["slug_kwd", "title_kwd", "_score"])

        if not field_map:
            orphans.append(entity)
            continue

        for row in field_map.values():
            score = row.get("_score", 0.0)
            page_id = row.get("slug_kwd", "")

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
        orphan_texts = [_entity_to_query_text(e) for e in orphans]
        orphan_embs, _ = await thread_pool_exec(embd_mdl.encode, orphan_texts)
        clusters = _wiki_cluster_entities(orphans, orphan_embs, threshold=PAGE_ROUTER_CLUSTER_THRESHOLD)
        for cluster in clusters:
            names = [e.get("entity_name") or e.get("term", "") for e in cluster]
            # Only create pages for clusters with enough depth
            total_claims = sum(1 for e in cluster for _ in e.get("claims", []))
            if total_claims >= CONCEPT_MIN_CLAIMS:
                page_id = _wiki_derive_page_id(names[0])
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
        ["slug_kwd", "title_kwd", "md_with_weight", "outlinks_kwd", "related_kb_pages_kwd"],
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
    dead_links: dict[str, list[str]] = {}  # pid → [dead links to remove]

    for pid, page in all_pages.items():
        content = page.get("md_with_weight", "")
        original = content

        for match in wikilink_re.finditer(content):
            link = match.group(1).strip()
            if link in valid_ids and link != pid:
                # Valid wikilink → record for cross-reference
                relation_map.setdefault(pid, []).append(
                    {
                        "entity_name": link.split("/")[-1] if "/" in link else link,
                        "relation": "see_also",
                    }
                )
            elif link in canonical_names:
                # Entity reference (Mode A): remove [[]] keep plain text
                content = content.replace(f"[[{link}]]", link, 1)
            else:
                # Dead link: remove [[]] keep plain text
                content = content.replace(f"[[{link}]]", link, 1)
                dead_links.setdefault(pid, []).append(link)

        # Update page content if changed
        update = {}
        if content != original:
            update["md_with_weight"] = content

        relations = relation_map.get(pid, [])
        if relations:
            update["related_kb_pages_kwd"] = json.dumps(relations[:20], ensure_ascii=False)
        elif page.get("related_kb_pages_kwd"):
            # Clear stale related_pages if no relations remain
            update["related_kb_pages_kwd"] = "[]"

        if update:
            index = search.index_name(tenant_id)
            await thread_pool_exec(
                settings.docStoreConn.update,
                {"slug_kwd": pid},
                update,
                index,
                kb_id,
            )


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
        offset = 0
        page_size = 1000
        while True:
            try:
                res = await thread_pool_exec(
                    settings.docStoreConn.search,
                    ["entities", "concepts", "claims", "relations", "topics", "doc_id"],
                    [],
                    {"compile_kwd": ["wiki_map_extract"]},
                    [],
                    OrderByExpr(),
                    offset,
                    page_size,
                    index,
                    [kb_id],
                )
                field_map = settings.docStoreConn.get_fields(res, ["entities", "concepts", "claims", "relations", "topics", "doc_id"]) or {}
            except Exception:
                logging.exception("wiki: failed to load MAP results for kb=%s", kb_id)
                break
            for row in field_map.values():
                map_results.append(row)
            if len(field_map) < page_size:
                break
            offset += page_size

    if not map_results:
        _progress("No MAP results found. Skipping wiki compilation.")
        return summary

    # ----- Phase 2: Entity Matching (跨实体去重) -----
    _progress("Entity Matching: deduplicating entities and concepts ...")

    raw_entities = _extract_raw_entities(map_results)

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

    if not canonical_map:
        _progress("Entity Matching: no canonical entities found. Skipping.")
        return summary

    # Persist new/changed canonical entities
    for cname, centry in canonical_map.items():
        existing = canonical_entities.get(cname)
        if existing:
            old_docs = set(k for k in (existing.get("source_doc_ids") or []))
            new_docs = set(centry.get("source_doc_ids", []))
            if old_docs != new_docs or centry["claim_count"] > existing.get("mention_count_int", 0):
                # Only persist when data changes
                embedding = None  # reuse existing embedding
                await _save_canonical_entity(
                    tenant_id,
                    kb_id,
                    cname,
                    centry["type"],
                    centry.get("aliases", []),
                    centry.get("source_doc_ids", []),
                    centry["claim_count"],
                    embedding=embedding,
                )
        else:
            # New entity: compute embedding
            emb_text = _entity_to_query_text(centry)
            emb_result, _ = await thread_pool_exec(embd_mdl.encode, [emb_text])
            await _save_canonical_entity(
                tenant_id,
                kb_id,
                cname,
                centry["type"],
                centry.get("aliases", []),
                centry.get("source_doc_ids", []),
                centry["claim_count"],
                embedding=emb_result[0].tolist() if hasattr(emb_result[0], "tolist") else emb_result[0],
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
        affected_doc_ids = set()
        for mr in map_results:
            did = mr.get("doc_id")
            if did:
                affected_doc_ids.add(did)
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
        new_doc_ids = affected_doc_ids - (deleted_doc_ids or set())
        if new_doc_ids:
            for mr in map_results:
                did = mr.get("doc_id")
                if did in new_doc_ids:
                    for c in mr.get("claims", []):
                        raw_name = c.get("entity_name") or c.get("subject") or c.get("term")
                        if raw_name:
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
        ["slug_kwd", "title_kwd", "md_with_weight", "claims", "source_doc_ids", "page_version_int", "synthesis_version_int", "entity_names_kwd", "related_kb_pages_kwd", "page_type_kwd"],
    )

    # Build deltas using canonical_map claims
    # Map from canonical name to claims
    canonical_claims: dict[str, list[dict]] = {}
    for cname, centry in canonical_map.items():
        canonical_claims[cname] = centry.get("claims", [])

    deltas = await _wiki_reduce_batch(
        affected_names=affected_names,
        map_results=map_results,
        existing_pages=existing_pages,
        deleted_doc_ids=deleted_doc_ids or set(),
        canonical_claims=canonical_claims,
        canonical_map=canonical_map,
        name_resolution=name_resolution,
    )

    if not deltas:
        _progress("REDUCE: no changes detected.")
        return summary

    # ----- Phase 4: Mode-specific dispatch -----
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
        )
    else:
        # Mode A: only concepts become pages, filter deltas
        concept_deltas = [d for d in deltas if d.get("entity_type") == "concept"]
        summary = await _wiki_mode_a_run(
            deltas=concept_deltas,
            existing_pages=existing_pages,
            chat_mdl=chat_mdl,
            embd_mdl=embd_mdl,
            tenant_id=tenant_id,
            kb_id=kb_id,
            incremental=incremental,
            callback=callback,
            canonical_map=canonical_map,
        )

    # ----- Phase 5: Update doc_page_source with canonical names -----
    # (doc_page_source page_ids is already handled in mode_run)
    _progress("FINALIZE: updating cross-references ...")
    try:
        await _wiki_finalize(tenant_id, kb_id, embd_mdl)
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
    canonical_map: dict[str, dict] | None = None,
) -> dict:
    """Mode A: concept→page. Only entity_type=concept deltas produce pages.

    Args:
        deltas: Already filtered to entity_type="concept" (done in caller)
        canonical_map: Canonical entity index for enriching entity context
    """
    summary = {"pages_created": 0, "pages_modified": 0, "pages_deleted": 0, "errors": []}

    def _progress(msg: str):
        if callback:
            try:
                callback(0.7, f"wiki REFINE A: {msg}")
            except Exception:
                pass

    # Map concept names to existing page IDs
    concept_to_page: dict[str, str] = {}
    for pid, page in existing_pages.items():
        names = page.get("entity_names_kwd", [])
        if isinstance(names, str):
            names = json.loads(names) if names else []
        for n in names:
            concept_to_page[n] = pid

    # Build concept-level deltas. Each concept becomes one page.
    # Non-concept entities are NOT in deltas (filtered by caller), but
    # if canonical_map is provided, their claims enrich concept pages via
    # shared source_chunk_id.
    concept_deltas: dict[str, dict] = {}
    for d in deltas:
        name = d.get("entity_name", "")
        if not name:
            continue
        page_id = concept_to_page.get(name) or _wiki_derive_page_id(name)

        if page_id not in concept_deltas:
            concept_deltas[page_id] = {
                "page_id": page_id,
                "page_title": name,
                "existing_page": existing_pages.get(page_id),
                "additions": [],
                "retractions": [],
                "claims": [],
                "source_chunks": [],
            }
        entry = concept_deltas[page_id]
        entry["additions"].extend(d.get("additions", []))
        entry["retractions"].extend(d.get("retractions", []))
        entry["claims"].extend(d.get("claims", []))

        # Collect source chunks from claims
        for claim in d.get("claims", []):
            cid = claim.get("source_chunk_id")
            if cid:
                entry["source_chunks"].append(
                    {
                        "id": cid,
                        "text": claim.get("statement", claim.get("text", "")),
                    }
                )

        if d.get("action") == "delete":
            entry["action"] = "delete"
        elif entry.get("action") != "delete":
            entry["action"] = d.get("action")

    # Enrich concept pages with entity claims that share source_chunk_ids.
    # Build a chunk_id → first-claim-text index from non-concept canonical entries.
    if canonical_map:
        chunk_claims: dict[str, list[str]] = {}
        for _cname, centry in canonical_map.items():
            if centry.get("type") == "concept":
                continue
            for claim in centry.get("claims", []):
                cid = claim.get("source_chunk_id")
                if cid:
                    chunk_claims.setdefault(cid, []).append(claim.get("statement", claim.get("text", "")))

        for _pid, entry in concept_deltas.items():
            concept_chunk_ids = {c.get("id") for c in entry.get("source_chunks", []) if c.get("id")}
            if not concept_chunk_ids:
                continue
            for cid in concept_chunk_ids:
                texts = chunk_claims.get(cid)
                if texts:
                    # Append one source-chunk entry per chunk id
                    entry["source_chunks"].append({"id": cid, "text": texts[0]})

    # Concept depth check: thin concepts don't create pages
    if not existing_pages:
        # First build: only concepts passing depth threshold
        deep_concepts = _wiki_decide_concept_pages(
            [
                {"term": entry["page_title"], "claims": entry["claims"], "source_doc_ids": list({c.get("source_doc_id") for c in entry["claims"] if c.get("source_doc_id")})}
                for entry in concept_deltas.values()
            ]
        )
        deep_ids = {p["page_id"] for p in deep_concepts}
        concept_deltas = {pid: entry for pid, entry in concept_deltas.items() if pid in deep_ids}
        if not concept_deltas:
            _progress("No concepts pass depth threshold. Skipping.")
            return summary

    all_page_ids = list(existing_pages.keys())
    doc_updates: dict[str, list[str]] = {}
    sem = asyncio.Semaphore(WIKI_REFINE_MAX_CONCURRENT)

    async def _refine_one(pid: str, entry: dict) -> None:
        async with sem:
            try:
                existing = entry["existing_page"]
                if entry.get("action") == "delete":
                    await _wiki_mode_a_refine(
                        mode="delete",
                        page_id=pid,
                        page_title=entry["page_title"],
                        existing_page=existing,
                        page_type_kwd="concept",
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

                result = await _wiki_mode_a_refine(
                    mode=refine_mode,
                    page_id=pid,
                    page_title=entry["page_title"],
                    existing_page=existing,
                    page_type_kwd="concept",
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

    tasks = [_refine_one(pid, entry) for pid, entry in concept_deltas.items()]
    if tasks:
        _progress(f"REFINE A: {len(tasks)} pages (max {WIKI_REFINE_MAX_CONCURRENT} concurrent) ...")
        await asyncio.gather(*tasks)

    for did, pids in doc_updates.items():
        try:
            existing_dps = (await _wiki_load_doc_page_source(tenant_id, kb_id, did)) or {}
            existing_pids = existing_dps.get("page_ids", [])
            for pid in pids:
                if pid not in existing_pids:
                    existing_pids.append(pid)
            # Collect entity names for this doc from canonical_map
            doc_entity_names = []
            if canonical_map:
                for cname, centry in canonical_map.items():
                    if did in centry.get("source_doc_ids", []):
                        doc_entity_names.append(cname)
            await _wiki_update_doc_page_source(
                tenant_id,
                kb_id,
                did,
                existing_pids,
                entity_names=doc_entity_names or existing_dps.get("entity_names"),
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
                cid = c.get("source_chunk_id")
                if cid:
                    chunks.append({"id": cid, "text": c.get("statement", c.get("text", ""))})
        if chunks:
            page_source_chunks[page_key] = chunks

    doc_updates: dict[str, list[str]] = {}  # doc_id → [page_ids]
    sem = asyncio.Semaphore(WIKI_REFINE_MAX_CONCURRENT)

    async def _refine_one(page_id: str, entities: list) -> None:
        async with sem:
            try:
                is_new = page_id.startswith("_new_")
                page_key = page_id[5:] if is_new else page_id
                existing = existing_pages.get(page_key) if not is_new else None

                # Determine page_type for Mode B
                if existing:
                    page_type = existing.get("page_type_kwd", "entity")
                else:
                    # Infer from entity types in the group
                    entity_types = {e.get("entity_type", "entity") for e in entities}
                    if entity_types == {"concept"}:
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
                    await _wiki_mode_a_refine(
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

                result = await _wiki_mode_a_refine(
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
        _progress(f"REFINE B: {len(tasks)} pages (max {WIKI_REFINE_MAX_CONCURRENT} concurrent) ...")
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
                entity_names=existing_dps.get("entity_names"),
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
                await _wiki_mode_a_refine(
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

                await _wiki_mode_a_refine(
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
    "_wiki_mode_a_refine",
    "_wiki_update_doc_page_source",
    "_load_canonical_entities",
    "_save_canonical_entity",
    "_delete_canonical_entity",
    "_extract_raw_entities",
]
