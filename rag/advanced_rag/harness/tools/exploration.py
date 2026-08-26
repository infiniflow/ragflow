"""Exploration tools: knowledge graph and wiki lookup.

``graph_explore`` lives in :mod:`navigation` (it shares the compiled-structure
machinery) and is re-exported here so the tool registry keeps one import point.

``wiki_query`` runs a hybrid (BM25 + dense) search over the searchable wiki draft
rows written by ``_wiki_persist_draft`` (``compile_kwd="wiki_page_draft"``) and
returns each page's markdown as a chunk. It takes the same ``keywords`` the other
search tools do — the keywords drive the keyword-sentence narrowing. Parameter
names must match the registered ``_search_schema`` (``query`` + ``keywords``),
otherwise every LLM tool call fails with a TypeError.
"""

import json
import logging

# graph_explore is implemented alongside catalog/mindmap navigation because it
# reuses their outline-answering + evidence-pulling helpers.
from rag.advanced_rag.harness.tools.navigation import graph_explore  # noqa: F401

_LOG = logging.getLogger(__name__)

# compile_kwd of the searchable wiki draft rows (see _wiki_persist_draft).
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
