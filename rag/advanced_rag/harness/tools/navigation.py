"""Navigation tools: document catalog and concept mindmap.

Both answer from a document's *compiled structure* rather than going straight to
retrieval: they read the compiled entities+relations for the documents in scope,
let the chat model answer from that outline, and pull the underlying chunks back
via each relevant entity's ``source_chunk_ids``.

Knowledge compilation writes every template kind into the same graph rows
(``{"entities": [...], "relations": [...]}``), so the two tools share one
implementation and differ only in which *kinds* they read:

* ``ontology_navigate`` — the document's layout: tree / TOC, page index, RAPTOR.
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
            _LOG.exception("ontology_navigate: failed reading compiled structure for doc=%s", doc_id)
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
            desc = " ".join((e.get("description") or "").split())
            lines.append(f"- {name} ({typ})" + (f": {desc}" if desc else ""))
    if relations:
        lines.append("\nRelations:")
        for r in relations[:_MAX_RELATIONS]:
            src, tgt = (r.get("from") or "").strip(), (r.get("to") or "").strip()
            if not src or not tgt:
                continue
            lines.append(f"- {src} -[{(r.get('type') or 'related').strip()}]-> {tgt}")
    return "\n".join(lines)


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
        _LOG.exception("ontology_navigate: failed loading evidence chunks for doc=%s", doc_id)
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


async def _navigate_within_doc(
    tools,
    topic: str,
    keywords: str,
    doc_scope: list[str] | None,
    kinds: set,
) -> list[dict]:
    """Return the chunk texts behind the entities the model finds relevant.

    Reads the compiled ``kinds`` structure of every doc in ``doc_scope`` — the
    tree / page-index catalog, or the concept mindmap — asks the chat model
    which of those entities are relevant to the question, and returns the chunks
    attached to the selected entities' ``source_chunk_ids``.

    Routing only — no fallback search. Returns ``[]`` when there is no doc scope,
    no question/keywords, no compiled structure of these kinds, no relevant
    entity, or no source chunks behind the selected entities.
    """
    if not doc_scope:
        return []
    query = " ".join(part for part in ((topic or "").strip(), (keywords or "").strip()) if part).strip()
    if not query:
        return []

    # 1. List the compiled structure entities of each doc, tagged with their
    #    originating doc_id so their chunks load from the right document.
    entities: list[dict] = []
    for doc_id in doc_scope:
        structure = await _load_compiled_structure(tools, doc_id, kinds)
        for e in structure.get("entities") or []:
            if isinstance(e, dict) and (e.get("name") or "").strip():
                entities.append({**e, "_doc_id": doc_id})
    if not entities:
        return []

    # 2. Ask the model which entities are relevant to the question.
    selected = await _ask_nav_select(tools, query, entities, "entities", _MAX_ENTITIES)
    if not selected:
        return []

    # 3. Load the chunks attached to the selected entities (grouped by doc).
    ids_by_doc: dict[str, list[str]] = {}
    seen: set[tuple[str, str]] = set()
    for e in selected:
        doc_id = e.get("_doc_id")
        if not doc_id:
            continue
        for cid in e.get("source_chunk_ids") or []:
            if isinstance(cid, str) and cid and (doc_id, cid) not in seen:
                seen.add((doc_id, cid))
                ids_by_doc.setdefault(doc_id, []).append(cid)

    chunks: list[dict] = []
    for doc_id, ids in ids_by_doc.items():
        chunks.extend(await _load_chunks_by_ids(tools, doc_id, ids))
    _LOG.info("[Navigation] Selected %d entity(ies) → %d source chunk(s).", len(selected), len(chunks))
    return chunks


async def ontology_navigate(tools, topic: str, keywords: str = "", doc_scope: list[str] | None = None) -> dict:
    """Answer from the documents' compiled catalog (tree / page index).

    :returns: ``{"answer": "", "chunks": [...], "doc_aggs": [...]}``
    """
    if hasattr(tools, "scoped_doc_ids"):
        doc_scope = tools.scoped_doc_ids(doc_scope)
    if not doc_scope:
        doc_scope = []
    _LOG.info(f'[Ontology navigation] Looking through the document catalog for "{topic}" (keywords: {keywords}) in doc: {len(doc_scope)}')
    if not doc_scope:
        _LOG.info(f'[Ontology navigation] No doc scope provided: "{topic}" (keywords: {keywords})')
        return {"answer": "", "chunks": [], "doc_aggs": []}
    chunks = await _navigate_within_doc(tools, topic, keywords, doc_scope, _CATALOG_KINDS)
    return {"answer": "", "chunks": chunks, "doc_aggs": _doc_aggs(chunks)}


async def mindmap_navigate(tools, topic: str, keywords: str = "", doc_scope: list[str] | None = None) -> dict:
    """Answer from the documents' compiled concept mindmap.

    ``topic`` (not ``concept``) — the parameter name must match the registered
    ``_navigate_schema``, otherwise every LLM call raises a TypeError.

    :returns: ``{"answer": "", "chunks": [...], "doc_aggs": [...]}``
    """
    if hasattr(tools, "scoped_doc_ids"):
        doc_scope = tools.scoped_doc_ids(doc_scope)
    if not doc_scope:
        doc_scope = []
    _LOG.info(f'[Mindmap navigation] Following the concept mindmap for "{topic}" (keywords: {keywords}) in doc: {len(doc_scope)}')
    chunks = await _navigate_within_doc(tools, topic, keywords, doc_scope, _MINDMAP_KINDS)
    return {"answer": "", "chunks": chunks, "doc_aggs": _doc_aggs(chunks)}


# ── Dataset navigation (document router) ────────────────────────────────────

_NAV_MAX_DOCS = 8  # documents the nav tree routes a query to
_NAV_MAX_HITS_PER_KB = 8

# Tree-walk router tunables.
_NAV_MAX_CLUSTERS = 500  # top-level clusters listed / rendered to the LLM
_NAV_CHILDREN_PAGE_SIZE = 1000  # children fetched per node
_NAV_TREE_MAX_DEPTH = 6  # BFS depth cap when descending sub-clusters to leaves
_NAV_TREE_MAX_LEAVES = 300  # document leaves rendered to the doc-select LLM

_NAV_SELECT_SYSTEM = """You are routing a question through a dataset's navigation tree.

You are given a QUESTION and a numbered list of {noun}, each with a name and a short description.
Choose the {noun} most likely to contain information relevant to answering the question.

Rules:
1. Judge only from the names and descriptions shown.
2. Be selective — include an item only if it is plausibly relevant. Include several when several are equally plausible.
3. If none are clearly relevant, return an empty list.
4. Return the bracketed index numbers of the chosen {noun}.

Output ONLY JSON, no prose, no code fences:
{{"relevant": [<index>, ...]}}"""


async def _ask_nav_select(tools, query: str, items: list[dict], noun: str, max_items: int) -> list[dict]:
    """Ask the chat model which of ``items`` are relevant to ``query``.

    Items are rendered as a numbered list of ``name`` (+ optional ``doc_count``)
    and ``description``; the model returns the chosen indices. Index-based (not
    id-based) so the model never has to reproduce opaque doc ids or exact names.
    Returns the selected item dicts (a subset of ``items``), or ``[]`` when the
    model declines or the call fails.
    """
    if not items:
        return []
    from rag.prompts.generator import form_message, message_fit_in

    capped = items[:max_items]
    lines = []
    for i, it in enumerate(capped):
        name = str(it.get("name") or "").strip() or f"item-{i}"
        desc = str(it.get("description") or "").strip().replace("\n", " ")
        extra = f" [{it['doc_count']} docs]" if it.get("doc_count") else ""
        lines.append(f"[{i}] {name}{extra}: {desc[:300]}")

    system = _NAV_SELECT_SYSTEM.format(noun=noun)
    user = f"Question:\n{query}\n\n{noun.capitalize()} (numbered):\n" + "\n".join(lines) + "\n\nOutput JSON:"
    try:
        _, msg = message_fit_in(form_message(system, user), tools.chat_mdl.max_length)
        ans = await tools.chat_mdl.async_chat(msg[0]["content"], msg[1:], {"temperature": 0.2})
        if isinstance(ans, tuple):
            ans = ans[0]
        cleaned = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
        cleaned = re.sub(r"```(?:json)?\s*|\s*```", "", cleaned).strip()
        verdict = json_repair.loads(cleaned) or {}
    except Exception:
        _LOG.exception("[Dataset navigation] LLM %s selection failed", noun)
        return []
    if not isinstance(verdict, dict):
        return []
    raw = verdict.get("relevant")
    if not isinstance(raw, list):
        return []

    out: list[dict] = []
    seen_idx: set[int] = set()
    for r in raw:
        try:
            idx = int(r)
        except (TypeError, ValueError):
            continue
        if 0 <= idx < len(capped) and idx not in seen_idx:
            seen_idx.add(idx)
            out.append(capped[idx])
    return out


async def _collect_nav_leaves(dataset_api_service, clusters: list[dict], doc_scope: list[str] | None = None) -> list[dict]:
    """BFS from the selected clusters down to their document leaves.

    Each cluster carries ``name`` + ``kb``. A node's children are either document
    leaves (``type == "doc"`` — collected, deduped by ``doc_id``) or sub-clusters
    (descended, depth- and count-capped).
    """
    leaves: list[dict] = []
    seen_docs: set[str] = set()
    seen_nodes: set[tuple] = set()
    frontier: list[tuple] = [(c["kb"], c["name"], 0) for c in clusters if c.get("name")]
    allowed_docs = set(doc_scope or [])

    while frontier and len(leaves) < _NAV_TREE_MAX_LEAVES:
        kb, name, depth = frontier.pop(0)
        node_key = (kb.id, name)
        if node_key in seen_nodes:
            continue
        seen_nodes.add(node_key)
        try:
            ok, data = await dataset_api_service.list_nav_children(kb.id, kb.tenant_id, name, page=1, page_size=_NAV_CHILDREN_PAGE_SIZE)
        except Exception:
            _LOG.exception("[Dataset navigation] list_nav_children failed for kb=%s node=%s", kb.id, name)
            continue
        if not ok or not isinstance(data, dict):
            continue
        for item in data.get("items") or []:
            if item.get("type") == "doc":
                did = str(item.get("doc_id") or "").strip()
                if did and (not allowed_docs or did in allowed_docs) and did not in seen_docs:
                    seen_docs.add(did)
                    leaves.append({**item, "kb": kb})
                    if len(leaves) >= _NAV_TREE_MAX_LEAVES:
                        break
            elif item.get("type") == "cluster" and item.get("name") and depth + 1 < _NAV_TREE_MAX_DEPTH:
                frontier.append((kb, item["name"], depth + 1))
    return leaves


def _nav_cluster_names(clusters: list[dict]) -> str:
    names = [str(c.get("name") or "").strip() for c in clusters]
    return ", ".join(n for n in names if n) or "none"


async def dataset_navigation_by_tree(tools, topic: str, keywords: str = "", doc_scope: list[str] | None = None) -> list[str]:
    """Return the ``doc_id``s most relevant to the question / keywords by walking
    the dataset nav tree with the chat model.

    The nav tree is a KB-level RAPTOR-style summary of every document. Two LLM
    passes narrow the corpus coarse-to-fine:

      1. List the top-level clusters (``list_nav_clusters``).
      2. Ask the model which clusters are relevant to the question.
      3. Descend those clusters to their document leaves (``list_nav_children``,
         recursing sub-clusters).
      4. Ask the model which documents are worth reading.

    Returns the routed ``doc_id`` list (capped at ``_NAV_MAX_DOCS``), or ``[]``
    when no question/keywords are given, there is no cluster tree, or the model
    finds nothing relevant. This function only routes — it does not retrieve.
    """
    query = " ".join(part for part in ((topic or "").strip(), (keywords or "").strip()) if part).strip()
    if not query:
        return []
    if hasattr(tools, "scoped_doc_ids"):
        doc_scope = tools.scoped_doc_ids(doc_scope)

    _LOG.info('[Dataset navigation] Walking the dataset tree for "%s"', query)

    from api.apps.services import dataset_api_service

    kbs = getattr(tools, "kbs", []) or []

    # 1. List every top-level cluster across the bound KBs (tagged with its KB).
    clusters: list[dict] = []
    for kb in kbs:
        try:
            ok, data = await dataset_api_service.list_nav_clusters(kb.id, kb.tenant_id, page=1, page_size=_NAV_MAX_CLUSTERS)
        except Exception:
            _LOG.exception("[Dataset navigation] list_nav_clusters failed for kb=%s", kb.id)
            continue
        if not ok or not isinstance(data, dict):
            continue
        for item in data.get("items") or []:
            if item.get("type") == "cluster" and item.get("name"):
                clusters.append({**item, "kb": kb})

    if not clusters:
        _LOG.info("[Dataset navigation] no cluster there.")
        return []

    # 2. Ask the model which clusters are relevant. Nothing relevant → [].
    selected_clusters = await _ask_nav_select(tools, query, clusters, "clusters", _NAV_MAX_CLUSTERS)
    if not selected_clusters:
        _LOG.info("[Dataset navigation] no cluster found.")
        return []
    _LOG.info("[Dataset navigation] %d/%d cluster(s) selected.", len(selected_clusters), len(clusters))

    # 3. Descend the selected clusters to their document leaves.
    leaves = await _collect_nav_leaves(dataset_api_service, selected_clusters, doc_scope)
    if not leaves:
        _LOG.info("[Dataset navigation] no leaf under selected cluster %s.", _nav_cluster_names(selected_clusters))
        return []

    # 4. Ask the model which documents to look into. Nothing relevant → [].
    selected_docs = await _ask_nav_select(tools, query, leaves, "documents", _NAV_TREE_MAX_LEAVES)
    if not selected_docs:
        _LOG.info("[Dataset navigation] no doc selected under cluster %s.", _nav_cluster_names(selected_clusters))
        return []

    routed: list[str] = []
    seen_docs: set[str] = set()
    for d in selected_docs:
        did = str(d.get("doc_id") or "").strip()
        if did and did not in seen_docs:
            seen_docs.add(did)
            routed.append(did)
    _LOG.info("[Dataset navigation] Routed to %d document(s).", len(routed))
    return routed[:_NAV_MAX_DOCS]


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
    from common.misc_utils import thread_pool_exec

    if hasattr(tools, "scoped_doc_ids"):
        doc_scope = tools.scoped_doc_ids(doc_scope)
    if doc_scope:
        by_kb: dict[tuple, list[str]] = {}
        for doc_id in doc_scope:
            resolved = await thread_pool_exec(tools._resolve_doc_tenant, doc_id)
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
    evidence = _collect_evidence_ids(entities, relations, relevant)
    chunks: list[dict] = []
    for doc_id, ids in evidence.items():
        if doc_id and ids:
            chunks.extend(await _load_chunks_by_ids(tools, doc_id, ids))

    before = len(chunks)
    chunks = _narrow_by_keywords(chunks, keywords)
    _LOG.info("[Graph exploration] Insufficient; returning %d evidence passage(s) (%d before keyword filtering).", len(chunks), before)

    return {"answer": "", "chunks": chunks, "doc_aggs": _doc_aggs(chunks)}
