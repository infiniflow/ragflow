"""Exploration tools: knowledge-graph walks and wiki page drill-downs.

Two ways of reaching evidence that is *not* a bag of similar chunks:

1. **``graph_explore``** — seeds entities by dense similarity to the question,
   expands BFS over compiled ``relation`` rows for a bounded number of hops, then
   asks the model whether the resulting subgraph answers the question outright;
   when it does not, the subgraph is returned as evidence passages so the caller
   can keep researching.

   Reads the compiled knowledge graph only (``knowledge_graph_kwd``
   entity/relation rows whose ``compile_kwd`` resolves to ``hypergraph``).
   Datasets without a compiled graph short-circuit — every caller treats an empty
   subgraph as "no graph here", not as an error.

2. **``wiki_query``** — asks a question of one dataset's compiled *wiki* pages,
   which are already synthesised narratives rather than raw chunks.

``_kg_scopes`` is the one piece the rest of the harness needs from here:
compiled-product expansion uses it to resolve the (kb_id, tenant_id) pairs to
scan.
"""

import json
import logging

from rag.advanced_rag.harness.structure_qa import _ask_structure

# Everything else (settings / doc-store exprs / thread pool / index name) is
# imported lazily inside each helper, matching the original module's style.

_LOG = logging.getLogger(__name__)

# ── Knowledge-graph exploration ─────────────────────────────────────────────
#
# Unlike catalog/mindmap (which read the merged "graph" JSON of one doc), the KG
# store keeps one searchable row per entity/relation, so graph_explore *searches*
# its way to a small subgraph: seed entities by the question, hop out over
# relations to the 2nd-degree neighbours, then answer from that subgraph.

# Scope of the compiled KG rows we search. Without a doc_scope we read the
# dataset-merged rows (one graph per dataset); with a doc_scope we read the
# per-document rows and filter by doc_id. Kept local rather than imported from
# the task-executor layer so the harness has no dependency on it.
_SCOPE_KWD_DATASET = "dataset"
_SCOPE_KWD_DOC = "doc"

_KG_SEEDS = 2  # top-N entities matched directly to the question
_KG_SEED_POOL = 64  # KNN candidate pool before the mention_count_int re-sort
_KG_SEED_SIM = 0.8  # dense-similarity floor for seed entities
_KG_HOPS = 2  # relation hops out from the seeds (1 => "2nd degree")
_KG_NEIGHBORS = 128  # cap on neighbour entity rows resolved per hop
_KG_REL_LIMIT = 32  # relations fetched per endpoint filter


async def _kg_scopes(tools, doc_scope: list[str] | None = None):
    """Resolve the (kb_id, tenant_id, doc_ids|None) groups to search.

    With a ``doc_scope`` the graph is limited to those docs (grouped by their
    KB); otherwise the whole bound KB graph is explored.
    """
    if hasattr(tools, "scoped_doc_ids"):
        doc_scope = tools.scoped_doc_ids(doc_scope)
    if doc_scope:
        by_kb: dict[tuple, list[str]] = {}
        for doc_id in doc_scope:
            # peewee MySQL lookup — call directly to reuse the pool's connection.
            resolved = tools._resolve_doc_tenant(doc_id)
            if resolved:
                by_kb.setdefault(resolved, []).append(doc_id)
        return [(kb, tenant, docs) for (kb, tenant), docs in by_kb.items()]
    return [(kb.id, kb.tenant_id, None) for kb in getattr(tools, "kbs", []) or []]


async def _kg_search(
    tools,
    kb_id: str,
    tenant_id: str,
    doc_ids,
    kind: str,
    text: str = "",
    top_n: int = 8,
    extra: dict | None = None,
    scope_kwd: str | None = None,
    order_desc: str | None = None,
    pool: int | None = None,
    similarity: float = 0.6,
) -> list[dict]:
    """Search the compiled KG rows of one KB and return the raw field maps.

    ``scope_kwd`` narrows to dataset-merged (``"dataset"``) or per-doc (``"doc"``)
    rows. ``order_desc`` sorts the hits by that field descending; when combined
    with a dense ``text`` match, ``pool`` sets the KNN candidate count so the
    re-sort ranks a wider pool than the ``top_n`` finally kept. ``similarity`` is
    the dense-match floor.
    """
    from common import settings
    from common.doc_store.doc_store_base import MatchTextExpr, OrderByExpr
    from common.misc_utils import thread_pool_exec
    from rag.nlp import search

    condition: dict = {"knowledge_graph_kwd": [kind]}
    if scope_kwd:
        condition["scope_kwd"] = [scope_kwd]
    if doc_ids:
        condition["doc_id"] = list(doc_ids)
    if extra:
        condition.update(extra)

    fields = ["content_with_weight", "source_chunk_ids", "doc_id", "docnm_kwd", "name_kwd", "mention_count_int", "from_entity_kwd", "to_entity_kwd"]
    exprs = []
    if text:
        knn_topn = pool or top_n
        if getattr(tools, "embed_mdl", None):
            try:
                exprs.append(await settings.retriever.get_vector(text, tools.embed_mdl, knn_topn, similarity))
            except Exception:
                _LOG.exception("[Graph exploration] vector build failed; using keyword match")
        if not exprs:
            exprs.append(MatchTextExpr(["content_ltks", "content_sm_ltks"], text, knn_topn))

    order_by = OrderByExpr()
    if order_desc:
        try:
            order_by.desc(order_desc)
        except Exception:
            order_by = OrderByExpr()

    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            fields,
            [],
            condition,
            exprs,
            order_by,
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
        "description": (payload.get("description") or payload.get("description") or ""),
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


def _endpoint_terms(names) -> list[str]:
    """Case variants for matching relation endpoints.

    ``dataset_structure_merger`` lowercases ``from_entity_kwd``/``to_entity_kwd``
    on merged rows while entity names keep their original case, so hop queries
    must try both forms. Accepts a single name or an iterable and returns the
    sorted union of each name's original and lowercased form.
    """
    if isinstance(names, str):
        names = [names]
    terms: set[str] = set()
    for n in names or []:
        n = (n or "").strip()
        if n:
            terms.add(n)
            terms.add(n.lower())
    return sorted(terms)


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

    Seeds the top-``_KG_SEEDS`` entities for the question (dense match above
    ``_KG_SEED_SIM``, ranked by ``mention_count_int``), hops ``_KG_HOPS`` out over
    their relations, then asks the chat model whether that subgraph answers the
    question. When it does, the answer is returned directly; when it doesn't, the
    source passages behind the entities/relations the model found relevant are
    returned as evidence (narrowed by ``keywords``) so the caller can continue.

    Without ``doc_scope`` the dataset-merged graph is searched
    (``scope_kwd="dataset"``); with it, the per-document rows of those docs
    (``scope_kwd="doc"``, filtered by ``doc_id``).

    :returns: ``{"answer": str, "chunks": [...], "doc_aggs": [...]}`` — exactly one
        of ``answer`` / ``chunks`` is populated.
    """
    from rag.advanced_rag.harness.tools.search import _narrow_by_keywords

    _empty = {"answer": "", "chunks": [], "doc_aggs": []}
    if hasattr(tools, "scoped_doc_ids"):
        doc_scope = tools.scoped_doc_ids(doc_scope)
    _LOG.info(f'[Graph exploration] Exploring the knowledge graph for "{query}" (keywords: {keywords})')

    scopes = await _kg_scopes(tools, doc_scope)
    if not scopes:
        _LOG.info("[Graph exploration] No knowledge base in scope.")
        return _empty

    scope_kwd = _SCOPE_KWD_DOC if doc_scope else _SCOPE_KWD_DATASET
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
        # (1) Seeds: condition C — dense match (>= _KG_SEED_SIM) over the scoped
        # entity rows, ranked by mention_count_int desc, top _KG_SEEDS.
        seed_rows = await _kg_search(
            tools,
            kb_id,
            tenant_id,
            doc_ids,
            "entity",
            text=text,
            top_n=_KG_SEEDS,
            scope_kwd=scope_kwd,
            order_desc="mention_count_int",
            pool=_KG_SEED_POOL,
            similarity=_KG_SEED_SIM,
        )
        seeds = [e for e in (_kg_parse_entity(r) for r in seed_rows.values()) if e]
        frontier = _add_entities(seeds, kb_id)
        _LOG.info("[Graph exploration] Seeded %d entity(ies): %s", len(frontier), ", ".join(frontier) or "none")

        # (2) Expand _KG_HOPS out, collecting relations and neighbour entities.
        for _hop in range(_KG_HOPS):
            if not frontier:
                break
            terms = _endpoint_terms(frontier)  # case variants — merged rows lowercase endpoints
            rel_rows: dict = {}
            rel_rows.update(await _kg_search(tools, kb_id, tenant_id, doc_ids, "relation", top_n=_KG_REL_LIMIT, scope_kwd=scope_kwd, extra={"from_entity_kwd": terms}))
            rel_rows.update(await _kg_search(tools, kb_id, tenant_id, doc_ids, "relation", top_n=_KG_REL_LIMIT, scope_kwd=scope_kwd, extra={"to_entity_kwd": terms}))
            hop_relations = [r for r in (_kg_parse_relation(x) for x in rel_rows.values()) if r]
            relations.extend(hop_relations)

            seen_lower = {k.split(":", 1)[1] for k in ent_names if k.startswith(f"{kb_id}:")}
            neigh_names = {n.strip() for r in hop_relations for n in (r["from"], r["to"]) if n and n.strip()}
            neigh_lower_set = {n.lower() for n in neigh_names} - seen_lower
            if not neigh_lower_set:
                break
            neigh_filtered = {n for n in neigh_names if n.lower() in neigh_lower_set}
            neigh_rows = await _kg_search(
                tools,
                kb_id,
                tenant_id,
                doc_ids,
                "entity",
                top_n=min(max(len(neigh_filtered), 1), _KG_NEIGHBORS),
                scope_kwd=scope_kwd,
                extra={"name_kwd": _endpoint_terms(neigh_filtered)},
            )
            neighbours = [e for e in (_kg_parse_entity(r) for r in neigh_rows.values()) if e]
            frontier = _add_entities(neighbours, kb_id)
            _LOG.info("[Graph exploration] Hop %d reached %d neighbour entity(ies).", _hop + 1, len(frontier))

    if not entities and not relations:
        _LOG.info("[Graph exploration] No compiled knowledge graph in scope.")
        return _empty

    _LOG.info("[Graph exploration] Built a subgraph of %d entity(ies) and %d relation(s).", len(entities), len(relations))

    # (3) Does the subgraph answer the question?
    answer, relevant = await _ask_structure(tools, query, entities, relations, "knowledge graph", "Graph exploration")

    # (4a) Sufficient — return the answer, no chunks.
    if answer:
        _LOG.info("[Graph exploration] The subgraph answered the question directly.")
        return {"answer": answer, "chunks": [], "doc_aggs": []}

    # (4b) Insufficient — return the source passages behind the relevant nodes.
    # Imported lazily: both live in ``navigation``, which imports this module.
    from rag.advanced_rag.harness.tools.navigation import _doc_aggs, _load_chunks_by_ids

    evidence = _collect_evidence_ids(entities, relations, relevant)
    chunks: list[dict] = []
    for doc_id, ids in evidence.items():
        if doc_id and ids:
            chunks.extend(await _load_chunks_by_ids(tools, doc_id, ids))

    before = len(chunks)
    chunks = _narrow_by_keywords(chunks, keywords)
    _LOG.info("[Graph exploration] Insufficient; returning %d evidence passage(s) (%d before keyword filtering).", len(chunks), before)

    return {"answer": "", "chunks": chunks, "doc_aggs": _doc_aggs(chunks)}


# ── Wiki page drill-down ──────────────────────────────────────────────────
#
# Compiled wiki pages are already synthesised narratives rather than raw chunks,
# so a question can often be answered straight from them.

_WIKI_DRAFT_COMPILE_KWD = "wiki_page_draft"
_WIKI_QUERY_TOP_N = 12


async def wiki_query(tools, query: str, keywords: str = "") -> dict:
    """Search the compiled wiki.

    Hybrid (BM25 over ``title_tks`` / ``content_ltks`` / ``content_sm_ltks`` +
    dense over ``q_<dim>_vec``) search across each bound KB's ``wiki_page_draft``
    rows. The page markdown is parsed out of each row's ``content_with_weight``
    (which stays the page JSON) and returned as chunks, narrowed by ``keywords``.

    :returns: ``{"answer": "", "chunks": [...], "doc_aggs": [...]}``
    """
    from common import settings
    from common.doc_store.doc_store_base import FusionExpr, OrderByExpr
    from common.misc_utils import thread_pool_exec
    from rag.nlp import search as _rag_search
    from rag.advanced_rag.harness.tools.search import _narrow_by_keywords

    _LOG.info(f'[Wiki lookup] Searching the compiled wiki for "{query}" (keywords: {keywords})')

    kbs = getattr(tools, "kbs", []) or []
    text = f"{query} {keywords}".strip()
    if not kbs or not text:
        return {"answer": "", "chunks": [], "doc_aggs": []}

    fields = ["content_with_weight", "docnm_kwd", "title_kwd", "wiki_slug_kwd", "source_doc_ids", "doc_id"]
    qryr = settings.retriever.qryr
    chunks: list[dict] = []

    for kb in kbs:
        kb_id = kb.id
        tenant_id = kb.tenant_id
        index = _rag_search.index_name(tenant_id)
        try:
            # BM25 over the standard tokenized fields, fused with dense when an
            # embedder is available — mirrors the retriever's own hybrid search.
            match_text, _ = qryr.question(text, min_match=0.3)
            exprs = [match_text]
            if getattr(tools, "embed_mdl", None):
                try:
                    match_dense = await settings.retriever.get_vector(text, tools.embed_mdl, _WIKI_QUERY_TOP_N, 0.1)
                    exprs = [match_text, match_dense, FusionExpr("weighted_sum", _WIKI_QUERY_TOP_N, {"weights": "0.001, 1"})]
                except Exception:
                    _LOG.exception("[Wiki lookup] dense expr build failed; BM25 only")
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                fields,
                [],
                {"compile_kwd": [_WIKI_DRAFT_COMPILE_KWD]},
                exprs,
                OrderByExpr(),
                0,
                _WIKI_QUERY_TOP_N,
                index,
                [kb_id],
            )
            rows = settings.docStoreConn.get_fields(res, fields) or {}
        except Exception:
            _LOG.exception("[Wiki lookup] search failed for kb=%s", kb_id)
            continue

        for cid, row in rows.items():
            try:
                page = json.loads(row.get("content_with_weight") or "{}")
            except Exception:
                page = {}
            if not isinstance(page, dict):
                page = {}
            content = page.get("content_md_rendered") or page.get("content_md") or page.get("content_md_raw") or ""
            if not content:
                continue
            title = row.get("docnm_kwd") or page.get("title") or row.get("title_kwd") or ""
            slug = row.get("wiki_slug_kwd") or page.get("slug") or ""
            chunks.append(
                {
                    "chunk_id": cid,
                    "content_with_weight": content,
                    "docnm_kwd": title,
                    "doc_id": slug or row.get("doc_id") or kb_id,
                    "wiki_slug_kwd": slug,
                }
            )

    before = len(chunks)
    chunks = _narrow_by_keywords(chunks, keywords)
    _LOG.info("[Wiki lookup] Found %d wiki page(s), kept %d after keyword filtering.", before, len(chunks))

    doc_aggs: list[dict] = []
    seen: set = set()
    for c in chunks:
        did = c.get("doc_id")
        if did and did not in seen:
            seen.add(did)
            doc_aggs.append({"doc_id": did, "doc_name": c.get("docnm_kwd") or ""})

    return {"answer": "", "chunks": chunks, "doc_aggs": doc_aggs}
