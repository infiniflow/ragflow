"""Navigation tools: document catalog and concept mindmap.

Both answer from a document's *compiled structure* rather than going straight to
retrieval: they read the compiled entities+relations for the documents in scope,
let the chat model answer from that outline, and pull the underlying chunks back
via each relevant entity's ``source_chunk_ids``.

Knowledge compilation writes every template kind into the same graph rows
(``{"entities": [...], "relations": [...]}``), so the two tools share one
implementation and differ only in which *kinds* they read:

* ``catalog_navigate`` — the document's layout: tree / TOC, page index, RAPTOR.
* ``mindmap_navigate`` — the concept mindmap.

Both take the same ``keywords`` the search tools do — keywords drive query
expansion and the keyword-sentence narrowing; without them navigation would hand
back full, un-narrowed chunks.
"""

import json
import logging
import re

import json_repair

_LOG = logging.getLogger(__name__)

# Compiled-structure kinds that describe a document's *layout*: a tree/outline
# or a page index. ``_compilation_template_kind`` in the API folds page_index
# and knowledge_graph into "timeline"; RAPTOR is its own bucket and is
# inherently tree-like, so it counts too.
_CATALOG_KINDS = {"tree", "timeline", "raptor", "page_index", "pageindex"}

# Compiled-structure kinds that describe the document's *concepts*.
_MINDMAP_KINDS = {"mindmap", "mind_map"}

# Cap how much compiled structure we render into the prompt.
_MAX_ENTITIES = 300
_MAX_RELATIONS = 300
_MAX_EVIDENCE_CHUNKS = 24

_NAV_SYSTEM = """You are given {noun} of one or more documents — an outline of entities and their relations — and a question.

Decide whether that outline alone already answers the question.

Rules:
1. Answer ONLY from the outline below. Do not invent facts.
2. Set "is_sufficient" to true only when the outline genuinely answers the question; otherwise false with an empty answer.
3. Always fill "relevant_entities" with the exact `name` values of the entities most related to the question (up to 10), even when the outline is not sufficient — they are used to pull the underlying source text.

Output ONLY JSON, no prose, no code fences:
{{"is_sufficient": true/false, "answer": "<answer, or empty>", "relevant_entities": ["<entity name>", ...]}}"""


def _normalize_kind(kind) -> str:
    """Mirror the API's kind normalization (page_index/knowledge_graph -> timeline)."""
    if not isinstance(kind, str):
        return ""
    normalized = kind.strip().lower().replace("-", "_")
    if normalized in {"pageindex", "page_index", "knowledge_graph"}:
        return "timeline"
    return normalized


async def _load_compiled_structure(tools, doc_id: str, kinds: set) -> dict:
    """Read a document's compiled graphs for the given template ``kinds``.

    Mirrors the fetch in ``GET /datasets/<id>/documents/<doc_id>/structure/graph``:
    one query for the template-authored graph rows, a second for the RAPTOR row
    (which carries ``compile_kwd`` but no ``knowledge_graph_kwd``).

    Returns ``{"entities": [...], "relations": [...]}`` for the matching buckets.
    """
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr
    from common.misc_utils import thread_pool_exec
    from rag.nlp import search

    resolved = await thread_pool_exec(tools._resolve_doc_tenant, doc_id)
    if not resolved:
        return {"entities": [], "relations": []}
    kb_id, tenant_id = resolved

    index_name = search.index_name(tenant_id)
    fields = [
        "content_with_weight",
        "compile_kwd",
        "compilation_template_ids",
        "compilation_template_kind_kwd",
    ]

    async def _query(condition: dict, limit: int) -> dict:
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                fields,
                [],
                condition,
                [],
                OrderByExpr(),
                0,
                limit,
                index_name,
                [kb_id],
            )
            return settings.docStoreConn.get_fields(res, fields) or {}
        except Exception:
            _LOG.exception("catalog_navigate: failed reading compiled structure for doc=%s", doc_id)
            return {}

    rows = dict(await _query({"doc_id": [doc_id], "knowledge_graph_kwd": ["graph"]}, 1000))
    rows.update(await _query({"doc_id": [doc_id], "compile_kwd": ["raptor_graph"]}, 16))

    entities: list[dict] = []
    relations: list[dict] = []
    for row in rows.values():
        try:
            graph = json.loads(row.get("content_with_weight") or "{}")
        except Exception:
            continue
        if not isinstance(graph, dict):
            continue

        compile_kwd = row.get("compile_kwd") or ""
        kind = _normalize_kind(row.get("compilation_template_kind_kwd") or compile_kwd)
        if compile_kwd == "raptor_graph":
            kind = "raptor"
        if kind not in kinds:
            continue

        entities.extend(graph.get("entities") or [])
        relations.extend(graph.get("relations") or [])

    return {"entities": entities, "relations": relations}


def _render_structure(entities: list[dict], relations: list[dict]) -> str:
    """Render the compiled structure as a compact outline for the prompt."""
    lines: list[str] = []
    if entities:
        lines.append("Entities:")
        for e in entities[:_MAX_ENTITIES]:
            name = (e.get("name") or "").strip()
            if not name:
                continue
            typ = (e.get("type") or "other").strip()
            desc = " ".join((e.get("discription") or "").split())
            lines.append(f"- {name} ({typ})" + (f": {desc}" if desc else ""))
    if relations:
        lines.append("\nRelations:")
        for r in relations[:_MAX_RELATIONS]:
            src, tgt = (r.get("from") or "").strip(), (r.get("to") or "").strip()
            if not src or not tgt:
                continue
            lines.append(f"- {src} -[{(r.get('type') or 'related').strip()}]-> {tgt}")
    return "\n".join(lines)


def _entity_chunk_ids(entities: list[dict], wanted_names: list[str]) -> list[str]:
    """Union the source_chunk_ids of the entities the model called relevant.

    Matches on ``name`` or any alias, case-insensitively. Relations carry no
    source_chunk_ids, so entities are the only evidence anchor.
    """
    wanted = {n.strip().lower() for n in wanted_names if isinstance(n, str) and n.strip()}
    ids: list[str] = []
    seen: set[str] = set()
    for e in entities:
        names = {(e.get("name") or "").strip().lower()}
        names.update((a or "").strip().lower() for a in (e.get("aliases") or []))
        if not (names & wanted):
            continue
        for cid in e.get("source_chunk_ids") or []:
            if isinstance(cid, str) and cid and cid not in seen:
                seen.add(cid)
                ids.append(cid)
    return ids


async def _load_chunks_by_ids(tools, doc_id: str, chunk_ids: list[str]) -> list[dict]:
    """Fetch chunks by their ids from the doc store."""
    if not chunk_ids:
        return []
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr
    from common.misc_utils import thread_pool_exec
    from rag.nlp import search

    resolved = await thread_pool_exec(tools._resolve_doc_tenant, doc_id)
    if not resolved:
        return []
    kb_id, tenant_id = resolved

    fields = ["content_with_weight", "docnm_kwd", "doc_id"]
    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            fields,
            [],
            {"id": chunk_ids[:_MAX_EVIDENCE_CHUNKS]},
            [],
            OrderByExpr(),
            0,
            _MAX_EVIDENCE_CHUNKS,
            search.index_name(tenant_id),
            [kb_id],
        )
        rows = settings.docStoreConn.get_fields(res, fields) or {}
    except Exception:
        _LOG.exception("catalog_navigate: failed loading evidence chunks for doc=%s", doc_id)
        return []

    chunks = []
    for cid, row in rows.items():
        chunks.append(
            {
                "chunk_id": cid,
                "content_with_weight": row.get("content_with_weight") or "",
                "docnm_kwd": row.get("docnm_kwd") or "",
                "doc_id": row.get("doc_id") or doc_id,
            }
        )
    return chunks


def _doc_aggs(chunks: list[dict]) -> list[dict]:
    aggs, seen = [], set()
    for c in chunks:
        did = c.get("doc_id")
        if did and did not in seen:
            seen.add(did)
            aggs.append({"doc_id": did, "doc_name": c.get("docnm_kwd") or ""})
    return aggs


async def _ask_structure(tools, topic: str, entities: list[dict], relations: list[dict], noun: str, label: str) -> tuple[str, list[str]]:
    """Ask the chat model to answer ``topic`` from the rendered outline.

    Returns ``(answer, relevant_entity_names)`` — ``answer`` is empty unless the
    model judged the outline sufficient; the names are always returned so the
    caller can pull the underlying source chunks.
    """
    verdict = {}
    try:
        from rag.prompts.generator import form_message, message_fit_in

        user = f"Question:\n{topic}\n\n{noun.capitalize()}:\n{_render_structure(entities, relations)}\n\nOutput JSON:"
        _, msg = message_fit_in(form_message(_NAV_SYSTEM.format(noun=f"the {noun}"), user), tools.chat_mdl.max_length)
        ans = await tools.chat_mdl.async_chat(msg[0]["content"], msg[1:], {"temperature": 0.2})
        if isinstance(ans, tuple):
            ans = ans[0]
        cleaned = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
        cleaned = re.sub(r"```(?:json)?\s*|\s*```", "", cleaned).strip()
        verdict = json_repair.loads(cleaned) or {}
        if not isinstance(verdict, dict):
            verdict = {}
    except Exception:
        _LOG.exception(f"[{label}] Could not read the outline with the model.")

    is_sufficient = bool(verdict.get("is_sufficient"))
    answer = (verdict.get("answer") or "").strip() if is_sufficient else ""
    relevant = [n for n in (verdict.get("relevant_entities") or []) if isinstance(n, str)]
    _LOG.info(
        "[%s] The %s %s the question; %d relevant entity(ies): %s",
        label,
        noun,
        "answers" if is_sufficient else "does not fully answer",
        len(relevant),
        ", ".join(relevant[:10]) or "none",
    )
    return answer, relevant


async def _navigate_compiled(
    tools,
    topic: str,
    keywords: str,
    doc_scope: list[str] | None,
    kinds: set,
    label: str,
    noun: str,
) -> dict:
    """Answer ``topic`` from the compiled structure of the docs in ``doc_scope``.

    Shared by :func:`catalog_navigate` and :func:`mindmap_navigate` — compilation
    writes every template kind into the same graph rows, so the tools differ only
    in which ``kinds`` they read and how they describe themselves.

    Falls back to a plain hybrid search when there is no doc scope, no compiled
    structure of these kinds, or no source chunks behind the relevant entities.

    :returns: ``{"answer": str, "chunks": [...], "doc_aggs": [...]}``
    """
    from rag.advanced_rag.harness.tools.search import _narrow_by_keywords, hybrid_search

    if not doc_scope:
        _LOG.info(f"[{label}] No document scope given — falling back to a normal search.")
        return await hybrid_search(tools, query=topic, keywords=keywords)

    entities: list[dict] = []
    relations: list[dict] = []
    per_doc: list[tuple[str, list[dict]]] = []
    for doc_id in doc_scope:
        structure = await _load_compiled_structure(tools, doc_id, kinds)
        if structure["entities"] or structure["relations"]:
            per_doc.append((doc_id, structure["entities"]))
            entities.extend(structure["entities"])
            relations.extend(structure["relations"])

    if not entities and not relations:
        _LOG.info(f"[{label}] These documents have no {noun} — falling back to a normal search.")
        return await hybrid_search(tools, query=topic, keywords=keywords, doc_scope=doc_scope)

    _LOG.info("[%s] Read an outline of %d entity(ies) and %d relation(s).", label, len(entities), len(relations))

    answer, relevant = await _ask_structure(tools, topic, entities, relations, noun, label)

    # Pull the source text behind the relevant entities, per originating doc.
    chunks: list[dict] = []
    for doc_id, doc_entities in per_doc:
        ids = _entity_chunk_ids(doc_entities, relevant)
        if ids:
            chunks.extend(await _load_chunks_by_ids(tools, doc_id, ids))

    if not chunks:
        _LOG.info(f"[{label}] No source text behind those entities — falling back to a normal search.")
        return await hybrid_search(tools, query=topic, keywords=keywords, doc_scope=doc_scope)

    before = len(chunks)
    narrowed = _narrow_by_keywords(chunks, keywords)
    if narrowed:
        chunks = narrowed
        _LOG.info("[%s] Pulled %d source passage(s) behind those entities, kept %d after keyword filtering.", label, before, len(chunks))
    else:
        _LOG.info("[%s] Keyword filtering removed all %d passage(s); restoring unfiltered set.", label, before)

    return {"answer": answer, "chunks": chunks, "doc_aggs": _doc_aggs(chunks)}


async def catalog_navigate(tools, topic: str, keywords: str = "", doc_scope: list[str] | None = None) -> dict:
    """Answer from the documents' compiled catalog (tree / TOC, page index, RAPTOR).

    :returns: ``{"answer": str, "chunks": [...], "doc_aggs": [...]}``
    """
    _LOG.info(f'[Catalog navigation] Looking through the document catalog for "{topic}" (keywords: {keywords})')
    return await _navigate_compiled(
        tools,
        topic,
        keywords,
        doc_scope,
        kinds=_CATALOG_KINDS,
        label="Catalog navigation",
        noun="document catalog",
    )


async def mindmap_navigate(tools, topic: str, keywords: str = "", doc_scope: list[str] | None = None) -> dict:
    """Answer from the documents' compiled concept mindmap.

    ``topic`` (not ``concept``) — the parameter name must match the registered
    ``_navigate_schema``, otherwise every LLM call raises a TypeError.

    :returns: ``{"answer": str, "chunks": [...], "doc_aggs": [...]}``
    """
    _LOG.info(f'[Mindmap navigation] Following the concept mindmap for "{topic}" (keywords: {keywords})')
    return await _navigate_compiled(
        tools,
        topic,
        keywords,
        doc_scope,
        kinds=_MINDMAP_KINDS,
        label="Mindmap navigation",
        noun="concept mindmap",
    )


# ── Dataset navigation (document router) ────────────────────────────────────

_NAV_MAX_DOCS = 8  # documents the nav tree routes a query to
_NAV_MAX_HITS_PER_KB = 8


async def dataset_navigate(tools, topic: str, keywords: str = "", doc_scope: list[str] | None = None) -> dict:
    """Route the query to the most relevant documents via the dataset nav tree,
    then search within only those documents.

    The nav tree is a KB-level RAPTOR-style summary of every document; searching
    it narrows the corpus to the handful of documents worth reading before the
    real chunk retrieval runs (coarse-to-fine). Falls back to a plain hybrid
    search when the KB has no nav tree or nothing routes.

    :returns: ``{"answer": "", "chunks": [...], "doc_aggs": [...]}``
    """
    from rag.advanced_rag.harness.tools.search import hybrid_search

    query = topic
    _LOG.info(f'[Dataset navigation] Finding the most relevant documents for "{query}" (keywords: {keywords})')

    # Caller already narrowed the corpus — just retrieve within it.
    if doc_scope:
        return await hybrid_search(tools, query=query, keywords=keywords, doc_scope=doc_scope)

    from rag.advanced_rag.knowlege_compile.dataset_nav import search_dataset_nav

    candidates: list[tuple[float, str]] = []
    nav_entities: list[dict] = []
    seen: set[str] = set()
    seen_nodes: set[str] = set()
    for kb in getattr(tools, "kbs", []) or []:
        try:
            hits = await search_dataset_nav(
                kb.tenant_id,
                kb.id,
                query,
                embd_mdl=getattr(tools, "embed_mdl", None),
                top_k=_NAV_MAX_HITS_PER_KB,
            )
        except Exception:
            _LOG.exception("[Dataset navigation] nav-tree search failed for kb=%s", kb.id)
            continue
        for h in hits:
            score = float(h.get("score") or 0.0)
            node_name = str(h.get("name") or "")
            if node_name and node_name not in seen_nodes:
                seen_nodes.add(node_name)
                nav_entities.append(
                    {
                        "name": node_name,
                        "type": h.get("type") or "dataset_nav",
                        "discription": h.get("description") or "",
                    }
                )
            for did in h.get("doc_ids") or []:
                if did and did not in seen:
                    seen.add(did)
                    candidates.append((score, did))

    candidates.sort(key=lambda item: item[0], reverse=True)
    routed = [did for _, did in candidates[:_NAV_MAX_DOCS]]
    if not routed:
        _LOG.info("[Dataset navigation] No dataset map here — falling back to a normal search.")
        return await hybrid_search(tools, query=query, keywords=keywords)

    _LOG.info("[Dataset navigation] Routed to %d document(s); searching within them.", len(routed))
    answer = ""
    if nav_entities:
        answer, _ = await _ask_structure(tools, query, nav_entities, [], "dataset map", "Dataset navigation")
    result = await hybrid_search(tools, query=query, keywords=keywords, doc_scope=routed)
    result["answer"] = answer
    return result


# ── Knowledge-graph exploration ─────────────────────────────────────────────
#
# Unlike catalog/mindmap (which read the merged "graph" JSON of one doc), the KG
# store keeps one searchable row per entity/relation, so graph_explore *searches*
# its way to a small subgraph: seed entities by the question, hop out over
# relations to the 2nd-degree neighbours, then answer from that subgraph.

_KG_SEEDS = 3  # entities matched directly to the question
_KG_HOPS = 1  # relation hops out from the seeds (1 => "2nd degree")
_KG_NEIGHBORS = 10  # neighbour entity rows resolved per hop
_KG_REL_LIMIT = 32  # relations fetched per endpoint filter


async def _kg_scopes(tools, doc_scope: list[str] | None):
    """Resolve the (kb_id, tenant_id, doc_ids|None) groups to search.

    With a ``doc_scope`` the graph is limited to those docs (grouped by their
    KB); otherwise the whole bound KB graph is explored.
    """
    from common.misc_utils import thread_pool_exec

    if doc_scope:
        by_kb: dict[tuple, list[str]] = {}
        for doc_id in doc_scope:
            resolved = await thread_pool_exec(tools._resolve_doc_tenant, doc_id)
            if resolved:
                by_kb.setdefault(resolved, []).append(doc_id)
        return [(kb, tenant, docs) for (kb, tenant), docs in by_kb.items()]
    return [(kb.id, kb.tenant_id, None) for kb in getattr(tools, "kbs", []) or []]


async def _kg_search(tools, kb_id: str, tenant_id: str, doc_ids, kind: str, text: str = "", top_n: int = 8, extra: dict | None = None) -> list[dict]:
    """Search the compiled KG rows of one KB and return the raw field maps."""
    from common import settings
    from common.doc_store.doc_store_base import MatchTextExpr, OrderByExpr
    from common.misc_utils import thread_pool_exec
    from rag.nlp import search

    condition: dict = {"knowledge_graph_kwd": [kind]}
    if doc_ids:
        condition["doc_id"] = list(doc_ids)
    if extra:
        condition.update(extra)

    fields = ["content_with_weight", "source_chunk_ids", "doc_id", "docnm_kwd", "from_entity_kwd", "to_entity_kwd"]
    exprs = []
    if text:
        if getattr(tools, "embed_mdl", None):
            try:
                exprs.append(await settings.retriever.get_vector(text, tools.embed_mdl, top_n, 0.1))
            except Exception:
                _LOG.exception("[Graph exploration] vector build failed; using keyword match")
        if not exprs:
            exprs.append(MatchTextExpr(["content_ltks", "content_sm_ltks"], text, top_n))

    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            fields,
            [],
            condition,
            exprs,
            OrderByExpr(),
            0,
            top_n,
            search.index_name(tenant_id),
            [kb_id],
        )
        rows = settings.docStoreConn.get_fields(res, fields) or {}
    except Exception:
        _LOG.exception("[Graph exploration] KG search failed (kind=%s)", kind)
        return {}
    return rows


def _kg_parse_entity(row: dict) -> dict | None:
    try:
        payload = json.loads(row.get("content_with_weight") or "{}")
    except Exception:
        payload = {}
    name = (payload.get("name") or payload.get("term") or payload.get("title") or "").strip()
    if not name:
        return None
    aliases = [str(a).strip() for a in (payload.get("aliases") or []) if str(a).strip()]
    return {
        "name": name,
        "type": (payload.get("type") or "other"),
        "discription": (payload.get("discription") or payload.get("description") or ""),
        "aliases": aliases,
        "source_chunk_ids": list(row.get("source_chunk_ids") or []),
        "doc_id": row.get("doc_id") or "",
        "docnm_kwd": row.get("docnm_kwd") or "",
    }


def _kg_parse_relation(row: dict) -> dict | None:
    src = (row.get("from_entity_kwd") or "").strip()
    tgt = (row.get("to_entity_kwd") or "").strip()
    if not src or not tgt:
        return None
    typ = "related"
    try:
        payload = json.loads(row.get("content_with_weight") or "{}")
        typ = payload.get("type") or payload.get("relation") or "related"
    except Exception:
        pass
    return {
        "from": src,
        "to": tgt,
        "type": typ,
        "source_chunk_ids": list(row.get("source_chunk_ids") or []),
        "doc_id": row.get("doc_id") or "",
    }


def _collect_evidence_ids(entities: list[dict], relations: list[dict], relevant_names: list[str]) -> dict:
    """Group the source_chunk_ids of the relevant entities AND relations by doc.

    An entity is relevant when its name/alias was named by the model; a relation
    is relevant when either endpoint is.
    """
    wanted = {n.strip().lower() for n in relevant_names if isinstance(n, str) and n.strip()}
    by_doc: dict[str, list[str]] = {}
    seen: set[tuple[str, str]] = set()

    def _add(doc_id: str, ids):
        for cid in ids or []:
            if not (isinstance(cid, str) and cid):
                continue
            key = (doc_id, cid)
            if key in seen:
                continue
            seen.add(key)
            by_doc.setdefault(doc_id, []).append(cid)

    for e in entities:
        names = {(e.get("name") or "").strip().lower(), *[(a or "").strip().lower() for a in (e.get("aliases") or [])]}
        if names & wanted:
            _add(e.get("doc_id") or "", e.get("source_chunk_ids"))
    for r in relations:
        if {(r.get("from") or "").strip().lower(), (r.get("to") or "").strip().lower()} & wanted:
            _add(r.get("doc_id") or "", r.get("source_chunk_ids"))
    return by_doc


async def graph_explore(tools, query: str, keywords: str = "", doc_scope: list[str] | None = None) -> dict:
    """Explore the compiled knowledge graph to answer ``query``.

    Searches seed entities for the question, hops out over their relations to the
    2nd-degree neighbours, asks the chat model to answer from that subgraph, then
    pulls the source passages behind the entities/relations it found relevant and
    narrows them with ``keywords``. Falls back to a plain hybrid search when the
    KB has no compiled graph or no source text is behind the relevant nodes.

    :returns: ``{"answer": str, "chunks": [...], "doc_aggs": [...]}``
    """
    from rag.advanced_rag.harness.tools.search import _narrow_by_keywords, hybrid_search

    _LOG.info(f'[Graph exploration] Exploring the knowledge graph for "{query}" (keywords: {keywords})')

    scopes = await _kg_scopes(tools, doc_scope)
    if not scopes:
        _LOG.info("[Graph exploration] No knowledge base in scope — falling back to a normal search.")
        return await hybrid_search(tools, query=query, keywords=keywords, doc_scope=doc_scope)

    text = f"{query} {keywords}".strip()
    entities: list[dict] = []
    relations: list[dict] = []
    ent_names: set[str] = set()

    def _add_entities(new: list[dict], scope_key: str = "") -> list[str]:
        added = []
        for e in new:
            key = f"{scope_key}:{e['name'].lower()}"
            if key in ent_names:
                continue
            ent_names.add(key)
            entities.append(e)
            added.append(e["name"])
        return added

    for kb_id, tenant_id, doc_ids in scopes:
        seed_rows = await _kg_search(tools, kb_id, tenant_id, doc_ids, "entity", text=text, top_n=_KG_SEEDS)
        seeds = [e for e in (_kg_parse_entity(r) for r in seed_rows.values()) if e]
        frontier = _add_entities(seeds, kb_id)
        _LOG.info("[Graph exploration] Seeded %d entity(ies): %s", len(frontier), ", ".join(frontier) or "none")

        for _hop in range(_KG_HOPS):
            if not frontier:
                break
            # Relations touching the current frontier (as source OR target).
            rel_rows: dict = {}
            rel_rows.update(await _kg_search(tools, kb_id, tenant_id, doc_ids, "relation", top_n=_KG_REL_LIMIT, extra={"from_entity_kwd": frontier}))
            rel_rows.update(await _kg_search(tools, kb_id, tenant_id, doc_ids, "relation", top_n=_KG_REL_LIMIT, extra={"to_entity_kwd": frontier}))
            hop_relations = [r for r in (_kg_parse_relation(x) for x in rel_rows.values()) if r]
            relations.extend(hop_relations)

            neighbour_names = {n for r in hop_relations for n in (r["from"], r["to"]) if n.lower() not in ent_names}
            if not neighbour_names:
                break
            neigh_rows = await _kg_search(tools, kb_id, tenant_id, doc_ids, "entity", text=" ".join(neighbour_names), top_n=_KG_NEIGHBORS)
            wanted = {n.lower() for n in neighbour_names}
            neighbours = [e for e in (_kg_parse_entity(r) for r in neigh_rows.values()) if e and e["name"].lower() in wanted]
            frontier = _add_entities(neighbours, kb_id)
            _LOG.info("[Graph exploration] Hop %d reached %d neighbour entity(ies).", _hop + 1, len(frontier))

    if not entities and not relations:
        _LOG.info("[Graph exploration] No compiled knowledge graph here — falling back to a normal search.")
        return await hybrid_search(tools, query=query, keywords=keywords, doc_scope=doc_scope)

    _LOG.info("[Graph exploration] Built a subgraph of %d entity(ies) and %d relation(s).", len(entities), len(relations))

    answer, relevant = await _ask_structure(tools, query, entities, relations, "knowledge graph", "Graph exploration")

    evidence = _collect_evidence_ids(entities, relations, relevant)
    chunks: list[dict] = []
    for doc_id, ids in evidence.items():
        if doc_id and ids:
            chunks.extend(await _load_chunks_by_ids(tools, doc_id, ids))

    if not chunks:
        _LOG.info("[Graph exploration] No source text behind those nodes — falling back to a normal search.")
        fallback = await hybrid_search(tools, query=query, keywords=keywords, doc_scope=doc_scope)
        fallback["answer"] = answer
        return fallback

    before = len(chunks)
    chunks = _narrow_by_keywords(chunks, keywords)
    _LOG.info("[Graph exploration] Pulled %d source passage(s) behind those nodes, kept %d after keyword filtering.", before, len(chunks))

    return {"answer": answer, "chunks": chunks, "doc_aggs": _doc_aggs(chunks)}
