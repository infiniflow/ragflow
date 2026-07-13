"""LightGraphSearch — query-time graph retrieval for LightGraph.

Extends ``KGSearch`` with:
- ``compile_kwd="lightgraph"`` filtering on all entity/relation queries.
- Query-time local subgraph construction + entity→chunk resolution.

Returns document chunks that mention the retrieved entities, not entity names.
"""

import asyncio
from copy import deepcopy
import logging
from typing import Dict, List, Tuple

import networkx as nx

from common import settings
from common.doc_store.doc_store_base import OrderByExpr
from common.misc_utils import get_uuid
from common.token_utils import num_tokens_from_string
from rag.graphrag.search import KGSearch
from rag.nlp import search


class LightGraphSearch(KGSearch):
    """Lightweight graph search — filters by ``compile_kwd="lightgraph"``
    and builds a local subgraph at query time. Instead of returning entity
    names, resolves top entities to actual document chunks.
    """

    async def _get_relevant_ents_by_keywords(self, keywords, filters, idxnms, kb_ids, emb_mdl, sim_thr=0.3, N=56):
        """Async wrapper around sync get_relevant_ents_by_keywords to await get_vector."""
        if not keywords:
            return {}
        filters = deepcopy(filters)
        filters["knowledge_graph_kwd"] = "entity"
        matchDense = await self.get_vector(", ".join(keywords), emb_mdl, 1024, sim_thr)
        es_res = self.dataStore.search(["content_with_weight", "entity_kwd", "rank_flt", "n_hop_with_weight"], [], filters, [matchDense], OrderByExpr(), 0, N, idxnms, kb_ids)
        return self._ent_info_from_(es_res, sim_thr)

    async def _get_relevant_relations_by_txt(self, txt, filters, idxnms, kb_ids, emb_mdl, sim_thr=0.3, N=56):
        """Async wrapper around sync get_relevant_relations_by_txt to await get_vector."""
        if not txt:
            return {}
        filters = deepcopy(filters)
        filters["knowledge_graph_kwd"] = "relation"
        matchDense = await self.get_vector(txt, emb_mdl, 1024, sim_thr)
        es_res = self.dataStore.search(["content_with_weight", "_score", "from_entity_kwd", "to_entity_kwd", "weight_int"], [], filters, [matchDense], OrderByExpr(), 0, N, idxnms, kb_ids)
        return self._relation_info_from_(es_res, sim_thr)

    def _entity_name_from_row(self, row) -> str:
        """Extract entity name from a search result row (handles list fields)."""
        name = row.get("entity_kwd") or row.get("from_entity_kwd") or ""
        if isinstance(name, list):
            name = name[0] if name else ""
        return str(name).strip()

    async def _resolve_entities_to_chunks(
        self,
        entity_names: List[str],
        idxnms: List[str],
        kb_ids: List[str],
        emb_mdl,
        max_chunks: int = 20,
        sim_thr: float = 0.0,
    ) -> List[dict]:
        """Resolve entity names to actual document chunks via keyword+vector search.

        1. Full-text keyword search for each entity name in chunk content.
        2. Also vector search using entity name as query.
        3. Deduplicate and rank results.
        """
        if not entity_names:
            return []

        chunk_filters = {
            "kb_id": kb_ids,
            "available_int": 1,
            "must_not": {"exists": "compile_kwd"},
        }
        idxnms_flat = idxnms if isinstance(idxnms, list) else [idxnms]

        all_chunks = {}  # chunk_id -> chunk dict

        # Step 1: Keyword search — find chunks mentioning entity names
        for ent in entity_names[:10]:
            try:
                matchText, _ = self.qryr.question(ent, min_match=0.1)
                res = self.dataStore.search(
                    ["id", "content_with_weight", "docnm_kwd", "kb_id"],
                    [],
                    chunk_filters,
                    [matchText],
                    OrderByExpr(),
                    0,
                    5,
                    idxnms_flat,
                    kb_ids,
                )
                rows = self.dataStore.get_fields(res, ["id", "content_with_weight", "docnm_kwd", "kb_id"])
                for rid, row in rows.items():
                    if rid not in all_chunks:
                        all_chunks[rid] = {
                            "chunk_id": rid,
                            "content_with_weight": row.get("content_with_weight", ""),
                            "docnm_kwd": row.get("docnm_kwd", ""),
                            "kb_id": row.get("kb_id", kb_ids),
                            "similarity": 0.5,
                        }
            except Exception:
                logging.exception(f"LightGraph: keyword lookup failed for entity {ent}")

        # Step 2: Vector search — find semantically related chunks
        if emb_mdl:
            try:
                query_text = ", ".join(entity_names[:5])
                matchDense = await self.get_vector(query_text, emb_mdl, 1024, sim_thr)
                res = self.dataStore.search(
                    ["id", "content_with_weight", "docnm_kwd", "kb_id"],
                    [],
                    chunk_filters,
                    [matchDense],
                    OrderByExpr(),
                    0,
                    max_chunks,
                    idxnms_flat,
                    kb_ids,
                )
                rows = self.dataStore.get_fields(res, ["id", "content_with_weight", "docnm_kwd", "kb_id"])
                for rid, row in rows.items():
                    if rid not in all_chunks:
                        all_chunks[rid] = {
                            "chunk_id": rid,
                            "content_with_weight": row.get("content_with_weight", ""),
                            "docnm_kwd": row.get("docnm_kwd", ""),
                            "kb_id": row.get("kb_id", kb_ids),
                            "similarity": 0.5,
                        }
            except Exception:
                logging.exception("LightGraph: vector search for entity chunks failed")

        # Convert to list, limit
        chunks = list(all_chunks.values())[:max_chunks]
        return chunks

    async def retrieval(
        self,
        question: str,
        tenant_ids: str | List[str],
        kb_ids: List[str],
        emb_mdl,
        llm,
        max_token: int = 8196,
        ent_topn: int = 10,
        rel_topn: int = 10,
        n_hop_depth: int = 2,
        **kwargs,
    ):
        filters = self.get_filters({"kb_ids": kb_ids})
        filters["compile_kwd"] = "lightgraph"
        if isinstance(tenant_ids, str):
            tenant_ids = [tenant_ids]
        idxnms = [search.index_name(tid) for tid in tenant_ids]

        # 1. Query rewrite
        try:
            ty_kwds, ents = await self.query_rewrite(llm, question, idxnms, kb_ids)
        except Exception as e:
            logging.exception(f"LightGraph query_rewrite failed: {e}")
            ty_kwds, ents = [], [question]

        # 2. Entity vector retrieval
        ents_from_query: Dict[str, dict] = {}
        if ents:
            raw = await self._get_relevant_ents_by_keywords(ents, deepcopy(filters), idxnms, kb_ids, emb_mdl, sim_thr=kwargs.get("ent_sim_threshold", 0.3), N=ent_topn * 3)
            ents_from_query.update(raw)

        # 3. Entity type retrieval
        ents_from_types: Dict[str, dict] = {}
        if ty_kwds:
            raw = self.get_relevant_ents_by_types(ty_kwds, deepcopy(filters), idxnms, kb_ids, N=10000)
            ents_from_types.update(raw)

        # 4. Relation vector retrieval
        rels_from_txt: Dict[Tuple, dict] = {}
        rels_from_txt.update(await self._get_relevant_relations_by_txt(question, deepcopy(filters), idxnms, kb_ids, emb_mdl, sim_thr=kwargs.get("rel_sim_threshold", 0.3)))

        # 5. Build local subgraph
        seed_ents = list(ents_from_query.keys())[:ent_topn]
        subgraph = await self._build_local_subgraph(seed_ents, filters, idxnms, kb_ids, n_hop=n_hop_depth)

        ppr = {}
        if subgraph and subgraph.number_of_nodes() > 0:
            personalization = {}
            for n in seed_ents:
                if n in subgraph:
                    personalization[n] = ents_from_query.get(n, {}).get("sim", 0.1)
            if personalization:
                try:
                    ppr = nx.pagerank(subgraph, personalization=personalization, alpha=0.85, max_iter=30)
                except Exception:
                    ppr = {}

        # 6. Score entities
        for name in ents_from_query:
            ents_from_query[name]["pagerank"] = ppr.get(name, 0)
        for name in ents_from_types:
            if name in ents_from_query:
                ents_from_query[name]["sim"] *= 2

        # 7. Rank and select top entities
        ents_sorted = sorted(ents_from_query.items(), key=lambda x: x[1].get("sim", 0) * x[1].get("pagerank", 0), reverse=True)[:ent_topn]
        top_entity_names = [name for name, _ in ents_sorted]

        # 8. Resolve entities → document chunks
        chunks = await self._resolve_entities_to_chunks(top_entity_names, idxnms, kb_ids, emb_mdl, max_chunks=kwargs.get("max_chunks", 30), sim_thr=0.0)

        # 9. Assemble context from actual chunks
        return self._assemble_chunk_context(chunks, max_token)

    def _assemble_chunk_context(self, chunks: List[dict], max_token: int) -> dict:
        """Build context from actual document chunks."""
        if not chunks:
            return {
                "chunk_id": get_uuid(),
                "content_ltks": "",
                "content_with_weight": "",
                "doc_id": "",
                "docnm_kwd": "Knowledge Graph (LightGraph)",
                "kb_id": [],
                "important_kwd": [],
                "image_id": "",
                "similarity": 1.0,
                "vector_similarity": 1.0,
                "term_similarity": 0,
                "vector": [],
                "positions": [],
            }

        result_chunks = []
        token_count = 0
        for ck in chunks:
            content = ck.get("content_with_weight", "") or ""
            if not content:
                continue
            tokens = num_tokens_from_string(content)
            if token_count + tokens > max_token:
                # Truncate to fit
                max_chars = int(len(content) * (max_token - token_count) / tokens)
                content = content[:max_chars]
                result_chunks.append(
                    {
                        "chunk_id": ck.get("chunk_id", get_uuid()),
                        "content_ltks": "",
                        "content_with_weight": content,
                        "doc_id": "",
                        "docnm_kwd": ck.get("docnm_kwd", "Knowledge Graph (LightGraph)"),
                        "kb_id": ck.get("kb_id", []),
                        "important_kwd": [],
                        "image_id": "",
                        "similarity": ck.get("similarity", 1.0),
                        "vector_similarity": ck.get("similarity", 1.0),
                        "term_similarity": 0,
                        "vector": [],
                        "positions": [],
                    }
                )
                break
            result_chunks.append(
                {
                    "chunk_id": ck.get("chunk_id", get_uuid()),
                    "content_ltks": "",
                    "content_with_weight": content,
                    "doc_id": "",
                    "docnm_kwd": ck.get("docnm_kwd", "Knowledge Graph (LightGraph)"),
                    "kb_id": ck.get("kb_id", []),
                    "important_kwd": [],
                    "image_id": "",
                    "similarity": ck.get("similarity", 1.0),
                    "vector_similarity": ck.get("similarity", 1.0),
                    "term_similarity": 0,
                    "vector": [],
                    "positions": [],
                }
            )
            token_count += tokens

        return {
            "chunk_id": get_uuid(),
            "content_ltks": "",
            "content_with_weight": "\n\n------\n\n".join(c["content_with_weight"] for c in result_chunks),
            "doc_id": "",
            "docnm_kwd": "Knowledge Graph (LightGraph)",
            "kb_id": [],
            "important_kwd": [],
            "image_id": "",
            "similarity": 1.0,
            "vector_similarity": 1.0,
            "term_similarity": 0,
            "vector": [],
            "positions": [],
        }

    async def _build_local_subgraph(
        self,
        seed_ents: List[str],
        filters: dict,
        idxnms: List[str],
        kb_ids: List[str],
        n_hop: int = 2,
    ) -> nx.Graph:
        """Build a local subgraph by expanding seed entities via relation lookups."""
        G = nx.Graph()
        current = set(seed_ents)
        visited: set = set()

        for _ in range(n_hop):
            if not current:
                break
            visited.update(current)
            rel_filter = deepcopy(filters)
            rel_filter["knowledge_graph_kwd"] = "relation"
            or_clause = {"$or": [{"from_entity_kwd": list(current)}, {"to_entity_kwd": list(current)}]}
            rel_filter.update(or_clause)

            try:
                res = await asyncio.get_event_loop().run_in_executor(
                    None, lambda: settings.docStoreConn.search(["from_entity_kwd", "to_entity_kwd", "weight_int", "entity_type_kwd"], [], rel_filter, [], OrderByExpr(), 0, 10000, idxnms, kb_ids)
                )
                rows = settings.docStoreConn.get_fields(res, ["from_entity_kwd", "to_entity_kwd", "weight_int", "entity_type_kwd"])
            except Exception:
                logging.exception("LightGraph: local subgraph build failed")
                break

            neighbors = set()
            for rid, row in rows.items():
                f = row.get("from_entity_kwd") or ""
                if isinstance(f, list):
                    f = f[0] if f else ""
                t = row.get("to_entity_kwd") or ""
                if isinstance(t, list):
                    t = t[0] if t else ""
                w = row.get("weight_int", 1)
                if f and t:
                    G.add_edge(f.upper(), t.upper(), weight=int(w))
                    if f.upper() not in visited:
                        neighbors.add(f.upper())
                    if t.upper() not in visited:
                        neighbors.add(t.upper())

            current = neighbors - visited

        return G
