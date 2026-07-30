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


# ----- constants -----

# compile_kwd values
WIKI_PAGE_COMPILE_KWD = "wiki_page"
WIKI_PLAN_GROUP_COMPILE_KWD = "wiki_plan_group"
WIKI_DOC_PAGE_SOURCE_COMPILE_KWD = "wiki_doc_page_source"

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
            entity.get("definition_excerpt") or entity.get("description") or "",
        ]
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


async def _search_existing_pages(
    tenant_id: str,
    kb_id: str,
    select_fields: list[str],
) -> dict[str, dict]:
    """Load all wiki_page rows in this KB."""
    from rag.nlp import search
    from common.misc_utils import thread_pool_exec

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
) -> dict:
    """Per-entity REDUCE: compute additions/retractions vs existing page.

    Returns dict with action (create|update|delete|noop), additions, retractions.
    """
    if existing_page is None:
        return {
            "action": "create",
            "entity_name": entity_name,
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
            "retractions": existing_claims,
            "has_delta": True,
        }
    elif additions or retractions:
        return {
            "action": "update",
            "entity_name": entity_name,
            "additions": additions,
            "retractions": retractions,
            "retained_source_doc_ids": list(all_doc_ids),
            "has_delta": True,
        }
    return {
        "action": "noop",
        "entity_name": entity_name,
        "retained_source_doc_ids": list(all_doc_ids),
        "has_delta": False,
    }


async def _wiki_reduce_batch(
    affected_names: set[str],
    map_results: list[dict],
    existing_pages: dict[str, dict],
    deleted_doc_ids: set[str],
) -> list[dict]:
    """Parallel per-entity REDUCE over a batch of affected names."""
    name_to_claims: dict[str, list[dict]] = {}
    for mr in map_results:
        for c in mr.get("claims", []):
            name = c.get("entity_name") or c.get("subject") or c.get("term")
            if name and name in affected_names:
                name_to_claims.setdefault(name, []).append(c)
    for name in affected_names:
        name_to_claims.setdefault(name, [])

    tasks = [
        _wiki_reduce_entity(
            entity_name=name,
            new_claims=claims,
            existing_page=existing_pages.get(name),
            deleted_doc_ids=deleted_doc_ids,
        )
        for name, claims in name_to_claims.items()
    ]
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
    chunk_hashes: dict[str, str] | None = None,
    map_checksum: str | None = None,
) -> None:
    """Record which pages this document contributes to."""
    index = search.index_name(tenant_id)

    condition = {
        "compile_kwd": [WIKI_DOC_PAGE_SOURCE_COMPILE_KWD],
        "doc_id": [doc_id],
    }
    existing = await thread_pool_exec(
        settings.docStoreConn.search,
        ["id", "page_ids", "source_chunk_hashes"],
        [],
        condition,
        [],
        OrderByExpr(),
        0,
        1,
        index,
        [kb_id],
    )
    existing_map = settings.docStoreConn.get_fields(existing, ["id", "page_ids", "source_chunk_hashes"])

    doc = {
        "doc_id": doc_id,
        "kb_id": kb_id,
        "page_ids": json.dumps(page_ids, ensure_ascii=False),
        "source_chunk_hashes": json.dumps(chunk_hashes or {}, ensure_ascii=False),
        "map_checksum": map_checksum or "",
        "compile_kwd": WIKI_DOC_PAGE_SOURCE_COMPILE_KWD,
    }
    if existing_map:
        for row in existing_map.values():
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
        ["page_ids", "source_chunk_hashes", "map_checksum"],
        [],
        condition,
        [],
        OrderByExpr(),
        0,
        1,
        index,
        [kb_id],
    )
    field_map = settings.docStoreConn.get_fields(res, ["page_ids", "source_chunk_hashes", "map_checksum"])
    for row in field_map.values():
        return {
            "page_ids": json.loads(row.get("page_ids", "[]")) if isinstance(row.get("page_ids"), str) else row.get("page_ids", []),
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
    additions: list[dict] | None,
    retractions: list[dict] | None,
    source_chunks: list[dict],
    claims: list[dict],
    available_pages: list[str],
    contextual_hints: str,
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
    doc_ids = existing.get("source_doc_ids", [])
    if source_chunks:
        for chunk in source_chunks:
            did = chunk.get("doc_id") or chunk.get("source_doc_id")
            if did and did not in doc_ids:
                doc_ids.append(did)

    # Embed for search
    from common.misc_utils import thread_pool_exec

    embeddings, _ = await thread_pool_exec(embd_mdl.encode, [summary or content[:200]])

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
        "page_type_kwd": "concept",
        "compile_kwd": WIKI_PAGE_COMPILE_KWD,
        "knowledge_graph_kwd": WIKI_PAGE_COMPILE_KWD,
    }
    # Insert vector (adds q_{dim}_vec field)
    vec_col = f"q_{len(embeddings[0]) if len(np.asarray(embeddings[0])) else 768}_vec"
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

    Runs for both modes. When page_ids is None, processes all wiki_page rows.
    """
    all_pages = await _search_existing_pages(
        tenant_id,
        kb_id,
        ["slug_kwd", "title_kwd", "md_with_weight", "outlinks_kwd", "related_kb_pages_kwd"],
    )
    if not all_pages:
        return

    # Gather all valid page IDs for wikilink validation
    valid_ids = set(all_pages.keys())

    # Build cross-reference map
    relation_map: dict[str, list[dict]] = {}
    for pid, page in all_pages.items():
        content = page.get("md_with_weight", "")
        for vid in valid_ids:
            if vid in content and vid != pid:
                relation_map.setdefault(pid, []).append(
                    {
                        "entity_name": vid.split("/")[-1] if "/" in vid else vid,
                        "relation": "see_also",
                    }
                )

    # Update each page's outlinks and related_pages
    for pid, page in all_pages.items():
        if page_ids is not None and pid not in page_ids:
            continue
        relations = relation_map.get(pid, [])
        # Only update if there's a meaningful change
        update = {}
        if relations:
            update["related_kb_pages_kwd"] = json.dumps(relations[:20], ensure_ascii=False)
        if update:
            from rag.nlp import search
            from common.misc_utils import thread_pool_exec

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

    # ----- Phase 2: REDUCE -----
    _progress("REDUCE: computing per-entity changes ...")

    # Extract all unique entity/concept names from MAP
    all_names: set[str] = set()
    for mr in map_results:
        for c in mr.get("claims", []):
            name = c.get("entity_name") or c.get("subject") or c.get("term")
            if name:
                all_names.add(name)

    if incremental:
        # For incremental, determine affected names from doc_page_source
        affected_doc_ids = set()
        for mr in map_results:
            did = mr.get("doc_id")
            if did:
                affected_doc_ids.add(did)
        affected_doc_ids = affected_doc_ids | (deleted_doc_ids or set())

        affected_names: set[str] = set()
        for did in affected_doc_ids:
            dps = await _wiki_load_doc_page_source(tenant_id, kb_id, did)
            if dps:
                # Find the entities from this doc
                dps_ents = dps.get("entity_names", [])
                for name in dps_ents:
                    if name in all_names:
                        affected_names.add(name)
        if not affected_names and deleted_doc_ids:
            affected_names = all_names
    else:
        affected_names = all_names

    # Load existing pages
    existing_pages = await _search_existing_pages(
        tenant_id,
        kb_id,
        ["slug_kwd", "title_kwd", "md_with_weight", "claims", "source_doc_ids", "page_version_int", "synthesis_version_int", "entity_names_kwd", "related_kb_pages_kwd", "page_type_kwd"],
    )

    deltas = await _wiki_reduce_batch(
        affected_names=affected_names,
        map_results=map_results,
        existing_pages=existing_pages,
        deleted_doc_ids=deleted_doc_ids or set(),
    )

    if not deltas:
        _progress("REDUCE: no changes detected.")
        return summary

    # ----- Phase 3: Mode-specific dispatch -----
    if plan:
        # Mode B: Page Router + per-page REFINE
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
        # Mode A: per-concept REFINE
        summary = await _wiki_mode_a_run(
            deltas=deltas,
            existing_pages=existing_pages,
            chat_mdl=chat_mdl,
            embd_mdl=embd_mdl,
            tenant_id=tenant_id,
            kb_id=kb_id,
            incremental=incremental,
            callback=callback,
        )

    # ----- Phase 4: FINALIZE -----
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
) -> dict:
    """Mode A: per-concept generate/modify/re-synthesize."""

    summary = {"pages_created": 0, "pages_modified": 0, "pages_deleted": 0, "errors": []}

    def _progress(msg: str):
        if callback:
            try:
                callback(0.7, f"wiki REFINE A: {msg}")
            except Exception:
                pass

    # Group deltas by concept page
    # For Mode A, each concept maps to exactly one page.
    # We need to find which concepts are affected and their pages.
    concept_deltas: dict[str, dict] = {}
    for d in deltas:
        name = d.get("entity_name", "")
        if not name:
            continue
        # Check if there's an existing page for this concept
        page_id = None
        for pid, page in existing_pages.items():
            names = page.get("entity_names_kwd", [])
            if isinstance(names, str):
                names = json.loads(names) if names else []
            if name in names:
                page_id = pid
                break
        if not page_id:
            page_id = _wiki_derive_page_id(name)

        concept_deltas.setdefault(
            page_id,
            {
                "page_id": page_id,
                "page_title": name,
                "existing_page": existing_pages.get(page_id),
                "additions": [],
                "retractions": [],
                "claims": [],
            },
        )
        entry = concept_deltas[page_id]
        entry["additions"].extend(d.get("additions", []))
        entry["retractions"].extend(d.get("retractions", []))

        if d.get("action") == "delete":
            entry["action"] = "delete"
        elif entry.get("action") != "delete":
            entry["action"] = d.get("action")

    # Load available pages for wikilinks
    all_page_ids = list(existing_pages.keys())

    total = len(concept_deltas)
    done = 0

    # Determine refine mode for each page
    for pid, entry in concept_deltas.items():
        try:
            existing = entry["existing_page"]
            next_version = (existing.get("page_version_int", 0) if existing else 0) + 1
            new_doc_ids = {c.get("source_doc_id") for c in entry["additions"] if c.get("source_doc_id")}

            if entry.get("action") == "delete":
                refine_mode = "delete"
            elif existing and _wiki_should_re_synthesize(existing, new_doc_ids, next_version):
                refine_mode = "re-synthesize"
            elif existing:
                refine_mode = "modify"
            else:
                refine_mode = "generate"

            _progress(f"({done + 1}/{total}) {refine_mode} {pid}")

            if refine_mode in ("generate", "modify", "re-synthesize"):
                # Load source chunks for this concept
                source_chunks = []
                all_claims = []
                for d in deltas:
                    src_chunks = d.get("additions", [])
                    for claim in src_chunks:
                        chunk_id = claim.get("source_chunk_id")
                        if chunk_id:
                            source_chunks.append({"id": chunk_id, "text": claim.get("statement", claim.get("text", ""))})
                    for claim in d.get("claims", []) if d.get("has_delta") else []:
                        all_claims.append(claim)

                result = await _wiki_mode_a_refine(
                    mode=refine_mode,
                    page_id=pid,
                    page_title=entry["page_title"],
                    existing_page=existing,
                    additions=entry["additions"],
                    retractions=entry["retractions"],
                    source_chunks=source_chunks,
                    claims=all_claims or entry["claims"],
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
                elif refine_mode == "delete":
                    summary["pages_deleted"] += 1
                else:
                    summary["pages_modified"] += 1

                # Update doc_page_source (only for Mode A: concept directly tracks page_id)
                if result:
                    affected_doc_ids = set(c.get("source_doc_id") for c in entry["additions"] if c.get("source_doc_id"))
                    for did in affected_doc_ids:
                        existing_dps = await _wiki_load_doc_page_source(tenant_id, kb_id, did) or {}
                        existing_pids = existing_dps.get("page_ids", [])
                        if pid not in existing_pids:
                            existing_pids.append(pid)
                        await _wiki_update_doc_page_source(
                            tenant_id,
                            kb_id,
                            did,
                            existing_pids,
                        )

            done += 1
        except Exception:
            logging.exception("wiki A: REFINE failed for %s", pid)
            summary["errors"].append(f"REFINE_FAILED:{pid}")
            done += 1

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

    total = len(assignments)
    done = 0

    for page_id, entities in assignments.items():
        try:
            is_new = page_id.startswith("_new_")
            page_key = page_id[5:] if is_new else page_id
            existing = existing_pages.get(page_key) if not is_new else None

            # Build claims from entities
            additions = []
            retractions = []
            action = "create" if is_new else "update"

            for entity in entities:
                additions.extend(entity.get("claims", []))
                if entity.get("action") == "delete":
                    action = "delete"

            _progress(f"({done + 1}/{total}) {action} {page_key}")

            if action == "delete":
                await _wiki_mode_a_refine(
                    mode="delete",
                    page_id=page_key,
                    page_title=existing.get("title_kwd", page_key) if existing else page_key,
                    existing_page=existing,
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
                done += 1
                continue

            # Determine refine mode
            if is_new:
                refine_mode = "generate"
            elif existing and _wiki_should_re_synthesize(
                existing,
                {c.get("source_doc_id") for c in additions if c.get("source_doc_id")},
                existing.get("page_version_int", 0) + 1,
            ):
                refine_mode = "re-synthesize"
            else:
                refine_mode = "modify"

            result = await _wiki_mode_a_refine(
                mode=refine_mode,
                page_id=page_key,
                page_title=existing.get("title_kwd", page_key) if existing else entities[0].get("entity_name", page_key),
                existing_page=existing,
                additions=additions,
                retractions=retractions,
                source_chunks=[],
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

            # Update plan_group (Mode B)
            if result:
                await _wiki_update_plan_group(
                    tenant_id,
                    kb_id,
                    page_key,
                    entity_names=[e.get("entity_name", "") for e in entities],
                    page_version=result.get("page_version_int", 1),
                )

            # Update doc_page_source
            affected_doc_ids = set(c.get("source_doc_id") for c in additions if c.get("source_doc_id"))
            for did in affected_doc_ids:
                existing_dps = await _wiki_load_doc_page_source(tenant_id, kb_id, did) or {}
                existing_pids = existing_dps.get("page_ids", [])
                if page_key not in existing_pids:
                    existing_pids.append(page_key)
                await _wiki_update_doc_page_source(
                    tenant_id,
                    kb_id,
                    did,
                    existing_pids,
                )

            done += 1
        except Exception:
            logging.exception("wiki B: REFINE failed for %s", page_id)
            summary["errors"].append(f"REFINE_FAILED:{page_id}")
            done += 1

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
    """Clean up wiki pages when a document is deleted.

    Args:
        plan: True=Mode B (update plan_group), False=Mode A

    Returns: {pages_modified, pages_deleted, errors}
    """
    summary = {"pages_modified": 0, "pages_deleted": 0, "errors": []}

    dps = await _wiki_load_doc_page_source(tenant_id, kb_id, doc_id)
    if not dps:
        return summary

    affected_page_ids = dps.get("page_ids", [])
    if not affected_page_ids:
        return summary

    for page_id in affected_page_ids:
        try:
            existing_pages = await _search_existing_pages(
                tenant_id,
                kb_id,
                ["slug_kwd", "title_kwd", "md_with_weight", "claims", "source_doc_ids", "page_version_int", "entity_names_kwd"],
            )
            existing = existing_pages.get(page_id)

            if not existing:
                continue

            source_doc_ids = existing.get("source_doc_ids", [])
            if isinstance(source_doc_ids, str):
                source_doc_ids = json.loads(source_doc_ids) if source_doc_ids else []

            # Remove the deleted doc from source list
            if doc_id in source_doc_ids:
                source_doc_ids.remove(doc_id)

            if not source_doc_ids:
                # No more sources → delete page
                await _wiki_mode_a_refine(
                    mode="delete",
                    page_id=page_id,
                    page_title=existing.get("title_kwd", page_id),
                    existing_page=existing,
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
                # Modify: remove content from deleted doc
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
                    additions=[],
                    retractions=retractions,
                    source_chunks=[],
                    claims=retained,
                    available_pages=list(existing_pages.keys()),
                    contextual_hints=_wiki_build_contextual_hints(page_id, existing, {}),
                    chat_mdl=chat_mdl,
                    embd_mdl=embd_mdl,
                    tenant_id=tenant_id,
                    kb_id=kb_id,
                    page_version=existing.get("page_version_int", 0),
                )
                summary["pages_modified"] += 1

            if plan:
                # Update plan_group: remove entity if doc was its only source
                plan_condition = {
                    "compile_kwd": [WIKI_PLAN_GROUP_COMPILE_KWD],
                    "page_id": [page_id],
                }
                index = search.index_name(tenant_id)
                from common.misc_utils import thread_pool_exec
                from common.doc_store.doc_store_base import OrderByExpr

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

    # Remove doc_page_source
    await _wiki_delete_doc_page_source(tenant_id, kb_id, doc_id)

    # Run FINALIZE
    try:
        await _wiki_finalize(tenant_id, kb_id, embd_mdl)
    except Exception:
        logging.exception("wiki: FINALIZE after deletion failed")

    return summary


__all__ = [
    "WIKI_PAGE_COMPILE_KWD",
    "WIKI_PLAN_GROUP_COMPILE_KWD",
    "WIKI_DOC_PAGE_SOURCE_COMPILE_KWD",
    "wiki_compile_incremental",
    "wiki_handle_document_deleted",
    "_wiki_reduce_entity",
    "_wiki_reduce_batch",
    "_wiki_page_router",
    "_wiki_finalize",
    "_wiki_mode_a_refine",
    "_wiki_update_doc_page_source",
]
