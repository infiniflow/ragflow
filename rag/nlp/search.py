#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
import json
import logging
import re
import math
import time
from collections import OrderedDict, defaultdict
from dataclasses import dataclass, field
from typing import List, Dict, Any, Optional

from rag.prompts.generator import relevant_chunks_with_toc
from rag.nlp import rag_tokenizer, query
import numpy as np
from rag.utils.doc_store_conn import DocStoreConnection, MatchDenseExpr, FusionExpr, OrderByExpr
from common.string_utils import remove_redundant_spaces
from common.float_utils import get_float
from common.constants import PAGERANK_FLD, TAG_FLD
from common import settings


def index_name(uid): return f"ragflow_{uid}"


@dataclass
class KBRetrievalParams:
    """Per-KB retrieval parameters for independent configuration."""
    kb_id: str
    vector_similarity_weight: float = 0.7  # Weight for vector vs keyword
    similarity_threshold: float = 0.2
    top_k: int = 1024
    rerank_enabled: bool = True


@dataclass
class HierarchicalConfig:
    """Configuration for hierarchical retrieval.
    
    Hierarchical retrieval uses a three-tier approach:
    1. KB Routing: Select relevant knowledge bases based on query
    2. Document Filtering: Filter documents by metadata before vector search
    3. Chunk Refinement: Precise vector search within filtered scope
    """
    
    # Enable hierarchical retrieval
    enabled: bool = False
    
    # Tier 1: KB Routing
    enable_kb_routing: bool = True
    kb_routing_method: str = "auto"  # "auto", "rule_based", "llm_based", "all"
    kb_routing_threshold: float = 0.3  # Min keyword overlap score
    kb_top_k: int = 3  # Max KBs to select
    kb_params: Dict[str, KBRetrievalParams] = field(default_factory=dict)  # Per-KB params
    
    # Tier 2: Document Filtering  
    enable_doc_filtering: bool = True
    doc_top_k: int = 100  # Max documents to pass to tier 3
    metadata_fields: List[str] = field(default_factory=list)  # Fields for filtering
    enable_metadata_similarity: bool = False  # Fuzzy matching for text metadata
    metadata_similarity_threshold: float = 0.7
    use_llm_metadata_filter: bool = False  # LLM-generated filter conditions
    
    # Tier 3: Chunk Refinement
    chunk_top_k: int = 10
    enable_parent_child: bool = False  # Parent-child chunking with summary mapping
    use_summary_mapping: bool = False  # Match via summary vectors first
    
    # Customizable prompts
    keyword_extraction_prompt: Optional[str] = None
    question_generation_prompt: Optional[str] = None
    
    # LLM-based enhancements
    use_llm_question_generation: bool = False  # Generate questions from chunks


@dataclass
class HierarchicalResult:
    """Result metadata from hierarchical retrieval."""
    
    selected_kb_ids: List[str] = field(default_factory=list)
    filtered_doc_ids: List[str] = field(default_factory=list)
    tier1_time_ms: float = 0.0
    tier2_time_ms: float = 0.0
    tier3_time_ms: float = 0.0
    total_time_ms: float = 0.0


class Dealer:
    def __init__(self, dataStore: DocStoreConnection):
        self.qryr = query.FulltextQueryer()
        self.dataStore = dataStore

    @dataclass
    class SearchResult:
        total: int
        ids: list[str]
        query_vector: list[float] | None = None
        field: dict | None = None
        highlight: dict | None = None
        aggregation: list | dict | None = None
        keywords: list[str] | None = None
        group_docs: list[list] | None = None

    def get_vector(self, txt, emb_mdl, topk=10, similarity=0.1):
        qv, _ = emb_mdl.encode_queries(txt)
        shape = np.array(qv).shape
        if len(shape) > 1:
            raise Exception(
                f"Dealer.get_vector returned array's shape {shape} doesn't match expectation(exact one dimension).")
        embedding_data = [get_float(v) for v in qv]
        vector_column_name = f"q_{len(embedding_data)}_vec"
        return MatchDenseExpr(vector_column_name, embedding_data, 'float', 'cosine', topk, {"similarity": similarity})

    def get_filters(self, req):
        condition = dict()
        for key, field in {"kb_ids": "kb_id", "doc_ids": "doc_id"}.items():
            if key in req and req[key] is not None:
                condition[field] = req[key]
        # TODO(yzc): `available_int` is nullable however infinity doesn't support nullable columns.
        for key in ["knowledge_graph_kwd", "available_int", "entity_kwd", "from_entity_kwd", "to_entity_kwd", "removed_kwd"]:
            if key in req and req[key] is not None:
                condition[key] = req[key]
        return condition

    def search(self, req, idx_names: str | list[str],
               kb_ids: list[str],
               emb_mdl=None,
               highlight: bool | list | None = None,
               rank_feature: dict | None = None
               ):
        if highlight is None:
            highlight = False

        filters = self.get_filters(req)
        orderBy = OrderByExpr()

        pg = int(req.get("page", 1)) - 1
        topk = int(req.get("topk", 1024))
        ps = int(req.get("size", topk))
        offset, limit = pg * ps, ps

        src = req.get("fields",
                      ["docnm_kwd", "content_ltks", "kb_id", "img_id", "title_tks", "important_kwd", "position_int",
                       "doc_id", "page_num_int", "top_int", "create_timestamp_flt", "knowledge_graph_kwd",
                       "question_kwd", "question_tks", "doc_type_kwd",
                       "available_int", "content_with_weight", "mom_id", PAGERANK_FLD, TAG_FLD])
        kwds = set([])

        qst = req.get("question", "")
        q_vec = []
        if not qst:
            if req.get("sort"):
                orderBy.asc("page_num_int")
                orderBy.asc("top_int")
                orderBy.desc("create_timestamp_flt")
            res = self.dataStore.search(src, [], filters, [], orderBy, offset, limit, idx_names, kb_ids)
            total = self.dataStore.get_total(res)
            logging.debug("Dealer.search TOTAL: {}".format(total))
        else:
            highlightFields = ["content_ltks", "title_tks"]
            if not highlight:
                highlightFields = []
            elif isinstance(highlight, list):
                highlightFields = highlight
            matchText, keywords = self.qryr.question(qst, min_match=0.3)
            if emb_mdl is None:
                matchExprs = [matchText]
                res = self.dataStore.search(src, highlightFields, filters, matchExprs, orderBy, offset, limit,
                                            idx_names, kb_ids, rank_feature=rank_feature)
                total = self.dataStore.get_total(res)
                logging.debug("Dealer.search TOTAL: {}".format(total))
            else:
                matchDense = self.get_vector(qst, emb_mdl, topk, req.get("similarity", 0.1))
                q_vec = matchDense.embedding_data
                if not settings.DOC_ENGINE_INFINITY:
                    src.append(f"q_{len(q_vec)}_vec")

                fusionExpr = FusionExpr("weighted_sum", topk, {"weights": "0.05,0.95"})
                matchExprs = [matchText, matchDense, fusionExpr]

                res = self.dataStore.search(src, highlightFields, filters, matchExprs, orderBy, offset, limit,
                                            idx_names, kb_ids, rank_feature=rank_feature)
                total = self.dataStore.get_total(res)
                logging.debug("Dealer.search TOTAL: {}".format(total))

                # If result is empty, try again with lower min_match
                if total == 0:
                    if filters.get("doc_id"):
                        res = self.dataStore.search(src, [], filters, [], orderBy, offset, limit, idx_names, kb_ids)
                        total = self.dataStore.get_total(res)
                    else:
                        matchText, _ = self.qryr.question(qst, min_match=0.1)
                        matchDense.extra_options["similarity"] = 0.17
                        res = self.dataStore.search(src, highlightFields, filters, [matchText, matchDense, fusionExpr],
                                                    orderBy, offset, limit, idx_names, kb_ids, rank_feature=rank_feature)
                        total = self.dataStore.get_total(res)
                    logging.debug("Dealer.search 2 TOTAL: {}".format(total))

            for k in keywords:
                kwds.add(k)
                for kk in rag_tokenizer.fine_grained_tokenize(k).split():
                    if len(kk) < 2:
                        continue
                    if kk in kwds:
                        continue
                    kwds.add(kk)

        logging.debug(f"TOTAL: {total}")
        ids = self.dataStore.get_chunk_ids(res)
        keywords = list(kwds)
        highlight = self.dataStore.get_highlight(res, keywords, "content_with_weight")
        aggs = self.dataStore.get_aggregation(res, "docnm_kwd")
        return self.SearchResult(
            total=total,
            ids=ids,
            query_vector=q_vec,
            aggregation=aggs,
            highlight=highlight,
            field=self.dataStore.get_fields(res, src + ["_score"]),
            keywords=keywords
        )

    @staticmethod
    def trans2floats(txt):
        return [get_float(t) for t in txt.split("\t")]

    def insert_citations(self, answer, chunks, chunk_v,
                         embd_mdl, tkweight=0.1, vtweight=0.9):
        assert len(chunks) == len(chunk_v)
        if not chunks:
            return answer, set([])
        pieces = re.split(r"(```)", answer)
        if len(pieces) >= 3:
            i = 0
            pieces_ = []
            while i < len(pieces):
                if pieces[i] == "```":
                    st = i
                    i += 1
                    while i < len(pieces) and pieces[i] != "```":
                        i += 1
                    if i < len(pieces):
                        i += 1
                    pieces_.append("".join(pieces[st: i]) + "\n")
                else:
                    pieces_.extend(
                        re.split(
                            r"([^\|][；。？!！\n]|[a-z][.?;!][ \n])",
                            pieces[i]))
                    i += 1
            pieces = pieces_
        else:
            pieces = re.split(r"([^\|][；。？!！\n]|[a-z][.?;!][ \n])", answer)
        for i in range(1, len(pieces)):
            if re.match(r"([^\|][；。？!！\n]|[a-z][.?;!][ \n])", pieces[i]):
                pieces[i - 1] += pieces[i][0]
                pieces[i] = pieces[i][1:]
        idx = []
        pieces_ = []
        for i, t in enumerate(pieces):
            if len(t) < 5:
                continue
            idx.append(i)
            pieces_.append(t)
        logging.debug("{} => {}".format(answer, pieces_))
        if not pieces_:
            return answer, set([])

        ans_v, _ = embd_mdl.encode(pieces_)
        for i in range(len(chunk_v)):
            if len(ans_v[0]) != len(chunk_v[i]):
                chunk_v[i] = [0.0]*len(ans_v[0])
                logging.warning("The dimension of query and chunk do not match: {} vs. {}".format(len(ans_v[0]), len(chunk_v[i])))

        assert len(ans_v[0]) == len(chunk_v[0]), "The dimension of query and chunk do not match: {} vs. {}".format(
            len(ans_v[0]), len(chunk_v[0]))

        chunks_tks = [rag_tokenizer.tokenize(self.qryr.rmWWW(ck)).split()
                      for ck in chunks]
        cites = {}
        thr = 0.63
        while thr > 0.3 and len(cites.keys()) == 0 and pieces_ and chunks_tks:
            for i, a in enumerate(pieces_):
                sim, tksim, vtsim = self.qryr.hybrid_similarity(ans_v[i],
                                                                chunk_v,
                                                                rag_tokenizer.tokenize(
                                                                    self.qryr.rmWWW(pieces_[i])).split(),
                                                                chunks_tks,
                                                                tkweight, vtweight)
                mx = np.max(sim) * 0.99
                logging.debug("{} SIM: {}".format(pieces_[i], mx))
                if mx < thr:
                    continue
                cites[idx[i]] = list(
                    set([str(ii) for ii in range(len(chunk_v)) if sim[ii] > mx]))[:4]
            thr *= 0.8

        res = ""
        seted = set([])
        for i, p in enumerate(pieces):
            res += p
            if i not in idx:
                continue
            if i not in cites:
                continue
            for c in cites[i]:
                assert int(c) < len(chunk_v)
            for c in cites[i]:
                if c in seted:
                    continue
                res += f" [ID:{c}]"
                seted.add(c)

        return res, seted

    def _rank_feature_scores(self, query_rfea, search_res):
        ## For rank feature(tag_fea) scores.
        rank_fea = []
        pageranks = []
        for chunk_id in search_res.ids:
            pageranks.append(search_res.field[chunk_id].get(PAGERANK_FLD, 0))
        pageranks = np.array(pageranks, dtype=float)

        if not query_rfea:
            return np.array([0 for _ in range(len(search_res.ids))]) + pageranks

        q_denor = np.sqrt(np.sum([s*s for t,s in query_rfea.items() if t != PAGERANK_FLD]))
        for i in search_res.ids:
            nor, denor = 0, 0
            if not search_res.field[i].get(TAG_FLD):
                rank_fea.append(0)
                continue
            for t, sc in eval(search_res.field[i].get(TAG_FLD, "{}")).items():
                if t in query_rfea:
                    nor += query_rfea[t] * sc
                denor += sc * sc
            if denor == 0:
                rank_fea.append(0)
            else:
                rank_fea.append(nor/np.sqrt(denor)/q_denor)
        return np.array(rank_fea)*10. + pageranks

    def rerank(self, sres, query, tkweight=0.3,
               vtweight=0.7, cfield="content_ltks",
               rank_feature: dict | None = None
               ):
        _, keywords = self.qryr.question(query)
        vector_size = len(sres.query_vector)
        vector_column = f"q_{vector_size}_vec"
        zero_vector = [0.0] * vector_size
        ins_embd = []
        for chunk_id in sres.ids:
            vector = sres.field[chunk_id].get(vector_column, zero_vector)
            if isinstance(vector, str):
                vector = [get_float(v) for v in vector.split("\t")]
            ins_embd.append(vector)
        if not ins_embd:
            return [], [], []

        for i in sres.ids:
            if isinstance(sres.field[i].get("important_kwd", []), str):
                sres.field[i]["important_kwd"] = [sres.field[i]["important_kwd"]]
        ins_tw = []
        for i in sres.ids:
            content_ltks = list(OrderedDict.fromkeys(sres.field[i][cfield].split()))
            title_tks = [t for t in sres.field[i].get("title_tks", "").split() if t]
            question_tks = [t for t in sres.field[i].get("question_tks", "").split() if t]
            important_kwd = sres.field[i].get("important_kwd", [])
            tks = content_ltks + title_tks * 2 + important_kwd * 5 + question_tks * 6
            ins_tw.append(tks)

        ## For rank feature(tag_fea) scores.
        rank_fea = self._rank_feature_scores(rank_feature, sres)

        sim, tksim, vtsim = self.qryr.hybrid_similarity(sres.query_vector,
                                                        ins_embd,
                                                        keywords,
                                                        ins_tw, tkweight, vtweight)

        return sim + rank_fea, tksim, vtsim

    def rerank_by_model(self, rerank_mdl, sres, query, tkweight=0.3,
                        vtweight=0.7, cfield="content_ltks",
                        rank_feature: dict | None = None):
        _, keywords = self.qryr.question(query)

        for i in sres.ids:
            if isinstance(sres.field[i].get("important_kwd", []), str):
                sres.field[i]["important_kwd"] = [sres.field[i]["important_kwd"]]
        ins_tw = []
        for i in sres.ids:
            content_ltks = sres.field[i][cfield].split()
            title_tks = [t for t in sres.field[i].get("title_tks", "").split() if t]
            important_kwd = sres.field[i].get("important_kwd", [])
            tks = content_ltks + title_tks + important_kwd
            ins_tw.append(tks)

        tksim = self.qryr.token_similarity(keywords, ins_tw)
        vtsim, _ = rerank_mdl.similarity(query, [remove_redundant_spaces(" ".join(tks)) for tks in ins_tw])
        ## For rank feature(tag_fea) scores.
        rank_fea = self._rank_feature_scores(rank_feature, sres)

        return tkweight * np.array(tksim) + vtweight * vtsim + rank_fea, tksim, vtsim

    def hybrid_similarity(self, ans_embd, ins_embd, ans, inst):
        return self.qryr.hybrid_similarity(ans_embd,
                                           ins_embd,
                                           rag_tokenizer.tokenize(ans).split(),
                                           rag_tokenizer.tokenize(inst).split())

    def retrieval(
        self,
        question,
        embd_mdl,
        tenant_ids,
        kb_ids,
        page,
        page_size,
        similarity_threshold=0.2,
        vector_similarity_weight=0.3,
        top=1024,
        doc_ids=None,
        aggs=True,
        rerank_mdl=None,
        highlight=False,
        rank_feature: dict | None = {PAGERANK_FLD: 10},
    ):
        ranks = {"total": 0, "chunks": [], "doc_aggs": {}}
        if not question:
            return ranks

        # Ensure RERANK_LIMIT is multiple of page_size
        RERANK_LIMIT = math.ceil(64 / page_size) * page_size if page_size > 1 else 1
        req = {
            "kb_ids": kb_ids,
            "doc_ids": doc_ids,
            "page": math.ceil(page_size * page / RERANK_LIMIT),
            "size": RERANK_LIMIT,
            "question": question,
            "vector": True,
            "topk": top,
            "similarity": similarity_threshold,
            "available_int": 1,
        }

        if isinstance(tenant_ids, str):
            tenant_ids = tenant_ids.split(",")

        sres = self.search(req, [index_name(tid) for tid in tenant_ids], kb_ids, embd_mdl, highlight, rank_feature=rank_feature)

        if rerank_mdl and sres.total > 0:
            sim, tsim, vsim = self.rerank_by_model(
                rerank_mdl,
                sres,
                question,
                1 - vector_similarity_weight,
                vector_similarity_weight,
                rank_feature=rank_feature,
            )
        else:
            if settings.DOC_ENGINE_INFINITY:
                # Don't need rerank here since Infinity normalizes each way score before fusion.
                sim = [sres.field[id].get("_score", 0.0) for id in sres.ids]
                sim = [s if s is not None else 0.0 for s in sim]
                tsim = sim
                vsim = sim
            else:
                # ElasticSearch doesn't normalize each way score before fusion.
                sim, tsim, vsim = self.rerank(
                    sres,
                    question,
                    1 - vector_similarity_weight,
                    vector_similarity_weight,
                    rank_feature=rank_feature,
                )

        sim_np = np.array(sim, dtype=np.float64)
        if sim_np.size == 0:
            ranks["doc_aggs"] = []
            return ranks

        sorted_idx = np.argsort(sim_np * -1)

        valid_idx = [int(i) for i in sorted_idx if sim_np[i] >= similarity_threshold]
        filtered_count = len(valid_idx)
        ranks["total"] = int(filtered_count)

        if filtered_count == 0:
            ranks["doc_aggs"] = []
            return ranks

        max_pages = max(RERANK_LIMIT // max(page_size, 1), 1)
        page_index = (page - 1) % max_pages
        begin = page_index * page_size
        end = begin + page_size
        page_idx = valid_idx[begin:end]

        dim = len(sres.query_vector)
        vector_column = f"q_{dim}_vec"
        zero_vector = [0.0] * dim

        for i in page_idx:
            id = sres.ids[i]
            chunk = sres.field[id]
            dnm = chunk.get("docnm_kwd", "")
            did = chunk.get("doc_id", "")

            position_int = chunk.get("position_int", [])
            d = {
                "chunk_id": id,
                "content_ltks": chunk["content_ltks"],
                "content_with_weight": chunk["content_with_weight"],
                "doc_id": did,
                "docnm_kwd": dnm,
                "kb_id": chunk["kb_id"],
                "important_kwd": chunk.get("important_kwd", []),
                "image_id": chunk.get("img_id", ""),
                "similarity": float(sim_np[i]),
                "vector_similarity": float(vsim[i]),
                "term_similarity": float(tsim[i]),
                "vector": chunk.get(vector_column, zero_vector),
                "positions": position_int,
                "doc_type_kwd": chunk.get("doc_type_kwd", ""),
                "mom_id": chunk.get("mom_id", ""),
            }
            if highlight and sres.highlight:
                if id in sres.highlight:
                    d["highlight"] = remove_redundant_spaces(sres.highlight[id])
                else:
                    d["highlight"] = d["content_with_weight"]
            ranks["chunks"].append(d)

        if aggs:
            for i in valid_idx:
                id = sres.ids[i]
                chunk = sres.field[id]
                dnm = chunk.get("docnm_kwd", "")
                did = chunk.get("doc_id", "")
                if dnm not in ranks["doc_aggs"]:
                    ranks["doc_aggs"][dnm] = {"doc_id": did, "count": 0}
                ranks["doc_aggs"][dnm]["count"] += 1

            ranks["doc_aggs"] = [
                {
                    "doc_name": k,
                    "doc_id": v["doc_id"],
                    "count": v["count"],
                }
                for k, v in sorted(
                    ranks["doc_aggs"].items(),
                    key=lambda x: x[1]["count"] * -1,
                )
            ]
        else:
            ranks["doc_aggs"] = []

        return ranks

    def hierarchical_retrieval(
        self,
        question: str,
        embd_mdl,
        tenant_ids,
        kb_ids: List[str],
        kb_infos: Optional[List[Dict[str, Any]]] = None,
        page: int = 1,
        page_size: int = 10,
        similarity_threshold: float = 0.2,
        vector_similarity_weight: float = 0.3,
        top: int = 1024,
        doc_ids: Optional[List[str]] = None,
        aggs: bool = True,
        rerank_mdl=None,
        highlight: bool = False,
        rank_feature: Optional[dict] = None,
        hierarchical_config: Optional[HierarchicalConfig] = None,
        chat_mdl=None,
        doc_metadata: Optional[List[Dict[str, Any]]] = None,
    ) -> Dict[str, Any]:
        """
        Perform hierarchical retrieval with three tiers:
        1. KB Routing: Select relevant knowledge bases
        2. Document Filtering: Filter by document metadata
        3. Chunk Refinement: Precise vector search
        
        Args:
            question: User query
            embd_mdl: Embedding model
            tenant_ids: Tenant IDs
            kb_ids: Knowledge base IDs to search
            kb_infos: Optional KB metadata (name, description) for routing
            page: Page number
            page_size: Results per page
            similarity_threshold: Minimum similarity score
            vector_similarity_weight: Weight for vector vs keyword similarity
            top: Maximum results to consider
            doc_ids: Optional document IDs to filter
            aggs: Whether to include aggregations
            rerank_mdl: Optional reranking model
            highlight: Whether to include highlights
            rank_feature: Ranking features
            hierarchical_config: Hierarchical retrieval configuration
            chat_mdl: Optional chat model for LLM-based features
            doc_metadata: Optional document metadata for filtering
            
        Returns:
            Dict with chunks, doc_aggs, and hierarchical_metadata
        """
        if rank_feature is None:
            rank_feature = {PAGERANK_FLD: 10}
            
        config = hierarchical_config or HierarchicalConfig()
        start_time = time.time()
        
        h_result = HierarchicalResult()
        
        # Tier 1: KB Routing
        tier1_start = time.time()
        selected_kb_ids = self._tier1_kb_routing(
            question, kb_ids, kb_infos, config, chat_mdl
        )
        h_result.selected_kb_ids = selected_kb_ids
        h_result.tier1_time_ms = (time.time() - tier1_start) * 1000
        
        if not selected_kb_ids:
            logging.warning(f"Hierarchical retrieval: No KBs selected for query: {question[:50]}...")
            return {
                "total": 0, 
                "chunks": [], 
                "doc_aggs": [],
                "hierarchical_metadata": h_result
            }
        
        logging.info(f"Tier 1: Selected {len(selected_kb_ids)}/{len(kb_ids)} KBs")
        
        # Get per-KB parameters for selected KBs
        kb_specific_params = {
            kb_id: config.kb_params.get(kb_id)
            for kb_id in selected_kb_ids
            if kb_id in config.kb_params
        }
        if kb_specific_params:
            logging.info(f"Using per-KB params for: {list(kb_specific_params.keys())}")
        
        # Tier 2: Document Filtering
        tier2_start = time.time()
        filtered_doc_ids = self._tier2_document_filtering(
            question, tenant_ids, selected_kb_ids, doc_ids, config, embd_mdl,
            chat_mdl, doc_metadata
        )
        h_result.filtered_doc_ids = filtered_doc_ids
        h_result.tier2_time_ms = (time.time() - tier2_start) * 1000
        
        if filtered_doc_ids:
            logging.info(f"Tier 2: Filtered to {len(filtered_doc_ids)} documents")
        
        # Tier 3: Chunk Refinement
        tier3_start = time.time()
        
        # Use filtered doc_ids if available, otherwise use original
        effective_doc_ids = filtered_doc_ids if filtered_doc_ids else doc_ids
        
        # Parent-child chunking with summary mapping
        if config.enable_parent_child and config.use_summary_mapping:
            ranks = self._tier3_summary_mapping_retrieval(
                question=question,
                embd_mdl=embd_mdl,
                tenant_ids=tenant_ids,
                kb_ids=selected_kb_ids,
                doc_ids=effective_doc_ids,
                page=page,
                page_size=page_size,
                similarity_threshold=similarity_threshold,
                vector_similarity_weight=vector_similarity_weight,
                top=top,
                aggs=aggs,
                rerank_mdl=rerank_mdl,
                highlight=highlight,
                rank_feature=rank_feature,
                config=config,
            )
        else:
            # Standard retrieval
            ranks = self.retrieval(
                question=question,
                embd_mdl=embd_mdl,
                tenant_ids=tenant_ids,
                kb_ids=selected_kb_ids,  # Use selected KBs from Tier 1
                page=page,
                page_size=page_size,
                similarity_threshold=similarity_threshold,
                vector_similarity_weight=vector_similarity_weight,
                top=top,
                doc_ids=effective_doc_ids,  # Use filtered docs from Tier 2
                aggs=aggs,
                rerank_mdl=rerank_mdl,
                highlight=highlight,
                rank_feature=rank_feature,
            )
        
        # Apply customizable prompts for keyword extraction if configured
        if config.keyword_extraction_prompt and ranks.get("chunks"):
            ranks["chunks"] = self._apply_custom_keyword_extraction(
                ranks["chunks"], config.keyword_extraction_prompt, embd_mdl
            )
        
        # Apply LLM-based question generation if configured
        if config.use_llm_question_generation and ranks.get("chunks") and chat_mdl:
            ranks["chunks"] = self._apply_llm_question_generation(
                ranks["chunks"], config.question_generation_prompt, chat_mdl
            )
        
        h_result.tier3_time_ms = (time.time() - tier3_start) * 1000
        h_result.total_time_ms = (time.time() - start_time) * 1000
        
        # Add hierarchical metadata to result
        ranks["hierarchical_metadata"] = {
            "selected_kb_ids": h_result.selected_kb_ids,
            "filtered_doc_ids": h_result.filtered_doc_ids,
            "tier1_time_ms": h_result.tier1_time_ms,
            "tier2_time_ms": h_result.tier2_time_ms,
            "tier3_time_ms": h_result.tier3_time_ms,
            "total_time_ms": h_result.total_time_ms,
            "kb_params_used": {kb_id: True for kb_id in selected_kb_ids if kb_id in config.kb_params},
        }
        
        logging.info(
            f"Hierarchical retrieval: {len(selected_kb_ids)} KBs -> "
            f"{len(filtered_doc_ids) if filtered_doc_ids else 'all'} docs -> "
            f"{len(ranks.get('chunks', []))} chunks in {h_result.total_time_ms:.1f}ms"
        )
        
        return ranks
    
    def _tier1_kb_routing(
        self,
        question: str,
        kb_ids: List[str],
        kb_infos: Optional[List[Dict[str, Any]]],
        config: HierarchicalConfig,
        chat_mdl=None
    ) -> List[str]:
        """
        Tier 1: Route query to relevant knowledge bases.
        
        Supports multiple routing methods:
        - "all": Return all KBs (no filtering)
        - "rule_based": Keyword overlap matching
        - "llm_based": LLM-powered intent analysis
        - "auto": Combines rule-based with LLM fallback
        """
        if not config.enable_kb_routing or not kb_infos:
            return kb_ids
        
        if len(kb_ids) <= config.kb_top_k:
            return kb_ids
        
        method = config.kb_routing_method
        
        if method == "all":
            return kb_ids
        elif method == "llm_based":
            return self._llm_kb_routing(question, kb_ids, kb_infos, config, chat_mdl)
        elif method == "rule_based":
            return self._rule_based_kb_routing(question, kb_ids, kb_infos, config)
        else:  # "auto" - try rule-based first, fall back to LLM if needed
            rule_result = self._rule_based_kb_routing(question, kb_ids, kb_infos, config)
            # If rule-based found good matches, use them
            if rule_result and len(rule_result) < len(kb_ids):
                return rule_result
            # Otherwise try LLM if available
            if chat_mdl:
                return self._llm_kb_routing(question, kb_ids, kb_infos, config, chat_mdl)
            return rule_result
    
    def _rule_based_kb_routing(
        self,
        question: str,
        kb_ids: List[str],
        kb_infos: List[Dict[str, Any]],
        config: HierarchicalConfig
    ) -> List[str]:
        """Rule-based KB routing using keyword overlap."""
        # Tokenize query
        query_tokens = set(rag_tokenizer.tokenize(question.lower()).split())
        if not query_tokens:
            return kb_ids
        
        # Score each KB
        kb_scores = []
        for kb_info in kb_infos:
            kb_id = kb_info.get('id')
            if kb_id not in kb_ids:
                continue
            
            # Combine name and description
            kb_text = ' '.join([
                kb_info.get('name', ''),
                kb_info.get('description', '')
            ]).lower()
            
            kb_tokens = set(rag_tokenizer.tokenize(kb_text).split())
            
            if kb_tokens:
                overlap = len(query_tokens & kb_tokens)
                score = overlap / len(query_tokens)
                kb_scores.append((kb_id, score))
        
        if not kb_scores:
            return kb_ids
        
        # Sort by score descending
        kb_scores.sort(key=lambda x: x[1], reverse=True)
        
        # Filter by threshold
        selected = [
            kb_id for kb_id, score in kb_scores 
            if score >= config.kb_routing_threshold
        ]
        
        # If none pass threshold, take top K
        if not selected:
            selected = [kb_id for kb_id, _ in kb_scores[:config.kb_top_k]]
        
        return selected[:config.kb_top_k] if selected else kb_ids
    
    def _llm_kb_routing(
        self,
        question: str,
        kb_ids: List[str],
        kb_infos: List[Dict[str, Any]],
        config: HierarchicalConfig,
        chat_mdl=None
    ) -> List[str]:
        """LLM-based KB routing using semantic understanding."""
        if not chat_mdl:
            logging.warning("LLM routing requested but no chat model provided, falling back to rule-based")
            return self._rule_based_kb_routing(question, kb_ids, kb_infos, config)
        
        try:
            # Build KB descriptions for LLM
            kb_descriptions = []
            kb_id_map = {}
            for i, kb_info in enumerate(kb_infos):
                kb_id = kb_info.get('id')
                if kb_id not in kb_ids:
                    continue
                name = kb_info.get('name', f'KB_{i}')
                desc = kb_info.get('description', 'No description')
                kb_descriptions.append(f"{i+1}. {name}: {desc}")
                kb_id_map[i+1] = kb_id
            
            if not kb_descriptions:
                return kb_ids
            
            # Create prompt for LLM
            prompt = f"""Given the following user query and available knowledge bases, select the most relevant knowledge bases that would contain information to answer the query.

User Query: {question}

Available Knowledge Bases:
{chr(10).join(kb_descriptions)}

Instructions:
- Return ONLY the numbers of the most relevant knowledge bases (up to {config.kb_top_k})
- Format: comma-separated numbers, e.g., "1, 3, 5"
- If unsure, include more rather than fewer

Selected knowledge bases:"""

            # Call LLM
            response = chat_mdl.chat(prompt, [], {"temperature": 0.1})
            if not response:
                return self._rule_based_kb_routing(question, kb_ids, kb_infos, config)
            
            # Parse response - extract numbers
            import re
            numbers = re.findall(r'\d+', response)
            selected = []
            for num_str in numbers:
                num = int(num_str)
                if num in kb_id_map:
                    selected.append(kb_id_map[num])
            
            if selected:
                logging.info(f"LLM routing selected {len(selected)} KBs: {selected[:3]}...")
                return selected[:config.kb_top_k]
            
            # Fallback to rule-based if LLM response was unparseable
            return self._rule_based_kb_routing(question, kb_ids, kb_infos, config)
            
        except Exception as e:
            logging.error(f"LLM KB routing failed: {e}, falling back to rule-based")
            return self._rule_based_kb_routing(question, kb_ids, kb_infos, config)
    
    def _tier2_document_filtering(
        self,
        question: str,
        tenant_ids,
        kb_ids: List[str],
        doc_ids: Optional[List[str]],
        config: HierarchicalConfig,
        embd_mdl=None,
        chat_mdl=None,
        doc_metadata: Optional[List[Dict[str, Any]]] = None
    ) -> List[str]:
        """
        Tier 2: Filter documents by relevance before chunk search.
        
        Supports multiple filtering strategies:
        - Vector-based: Lightweight search to find relevant documents
        - Metadata filtering: Filter by specified metadata fields
        - Similarity matching: Fuzzy matching on document names/summaries
        - LLM-based: Use LLM to generate filter conditions
        """
        if not config.enable_doc_filtering:
            return doc_ids or []
        
        if doc_ids:
            # Already have doc_ids filter, just limit count
            return doc_ids[:config.doc_top_k]
        
        try:
            if isinstance(tenant_ids, str):
                tenant_ids = tenant_ids.split(",")
            
            filtered_docs = set()
            
            # Strategy 1: LLM-based metadata filtering
            if config.use_llm_metadata_filter and chat_mdl and doc_metadata:
                llm_filtered = self._llm_metadata_filter(
                    question, doc_metadata, config, chat_mdl
                )
                if llm_filtered:
                    filtered_docs.update(llm_filtered)
                    logging.info(f"LLM metadata filter selected {len(llm_filtered)} docs")
            
            # Strategy 2: Metadata similarity matching
            if config.enable_metadata_similarity and doc_metadata:
                similarity_filtered = self._metadata_similarity_filter(
                    question, doc_metadata, config, embd_mdl
                )
                if similarity_filtered:
                    if filtered_docs:
                        filtered_docs.intersection_update(similarity_filtered)
                    else:
                        filtered_docs.update(similarity_filtered)
                    logging.info(f"Metadata similarity filter: {len(similarity_filtered)} docs")
            
            # Strategy 3: Vector-based document search (default)
            if not filtered_docs:
                req = {
                    "kb_ids": kb_ids,
                    "question": question,
                    "size": config.doc_top_k,
                    "topk": config.doc_top_k * 2,
                    "similarity": 0.1,  # Lower threshold for document filtering
                    "available_int": 1,
                }
                
                idx_names = [index_name(tid) for tid in tenant_ids]
                
                # Search with embedding if available
                sres = self.search(req, idx_names, kb_ids, embd_mdl, highlight=False)
                
                if sres and sres.field:
                    for chunk_id in sres.ids:
                        doc_id = sres.field[chunk_id].get("doc_id")
                        if doc_id:
                            filtered_docs.add(doc_id)
            
            return list(filtered_docs)[:config.doc_top_k]
            
        except Exception as e:
            logging.error(f"Tier 2 document filtering error: {e}")
            return []
    
    def _llm_metadata_filter(
        self,
        question: str,
        doc_metadata: List[Dict[str, Any]],
        config: HierarchicalConfig,
        chat_mdl
    ) -> List[str]:
        """Use LLM to generate and apply metadata filter conditions."""
        try:
            # Get available metadata fields
            if not doc_metadata:
                return []
            
            # Sample metadata to show LLM what's available
            sample_fields = set()
            sample_values = {}
            for doc in doc_metadata[:10]:
                for field in config.metadata_fields or doc.keys():
                    if field in doc and field not in ['id', 'doc_id']:
                        sample_fields.add(field)
                        if field not in sample_values:
                            sample_values[field] = []
                        if len(sample_values[field]) < 3:
                            sample_values[field].append(str(doc[field])[:50])
            
            if not sample_fields:
                return []
            
            # Build prompt
            field_examples = "\n".join([
                f"- {field}: examples = {sample_values.get(field, [])}"
                for field in sample_fields
            ])
            
            prompt = f"""Given a user query and available document metadata fields, generate filter conditions to select relevant documents.

User Query: {question}

Available Metadata Fields:
{field_examples}

Instructions:
- Return filter conditions as JSON: {{"field": "value"}} or {{"field": {{"operator": "value"}}}}
- Supported operators: "contains", "equals", "starts_with"
- Only filter on fields that are clearly relevant to the query
- Return empty {{}} if no clear filter applies

Filter conditions (JSON only):"""

            response = chat_mdl.chat(prompt, [], {"temperature": 0.1})
            if not response:
                return []
            
            # Parse JSON response
            try:
                # Extract JSON from response
                json_match = re.search(r'\{[^}]*\}', response)
                if json_match:
                    filters = json.loads(json_match.group())
                else:
                    return []
            except json.JSONDecodeError:
                return []
            
            if not filters:
                return []
            
            # Apply filters to documents
            filtered_ids = []
            for doc in doc_metadata:
                doc_id = doc.get('id') or doc.get('doc_id')
                if not doc_id:
                    continue
                
                matches = True
                for field, condition in filters.items():
                    doc_value = str(doc.get(field, '')).lower()
                    
                    if isinstance(condition, dict):
                        op = list(condition.keys())[0]
                        val = str(list(condition.values())[0]).lower()
                        if op == "contains":
                            matches = val in doc_value
                        elif op == "equals":
                            matches = val == doc_value
                        elif op == "starts_with":
                            matches = doc_value.startswith(val)
                    else:
                        # Simple equality
                        matches = str(condition).lower() in doc_value
                    
                    if not matches:
                        break
                
                if matches:
                    filtered_ids.append(doc_id)
            
            return filtered_ids
            
        except Exception as e:
            logging.error(f"LLM metadata filter error: {e}")
            return []
    
    def _metadata_similarity_filter(
        self,
        question: str,
        doc_metadata: List[Dict[str, Any]],
        config: HierarchicalConfig,
        embd_mdl=None
    ) -> List[str]:
        """Filter documents by metadata text similarity."""
        try:
            if not embd_mdl or not doc_metadata:
                return []
            
            # Get query embedding
            query_vec, _ = embd_mdl.encode_queries(question)
            if not query_vec or len(query_vec) == 0:
                return []
            
            query_vec = np.array(query_vec)
            
            # Score each document by metadata similarity
            doc_scores = []
            for doc in doc_metadata:
                doc_id = doc.get('id') or doc.get('doc_id')
                if not doc_id:
                    continue
                
                # Combine relevant text fields
                text_parts = []
                for field in ['name', 'title', 'summary', 'description', 'docnm_kwd']:
                    if field in doc and doc[field]:
                        text_parts.append(str(doc[field]))
                
                if not text_parts:
                    continue
                
                doc_text = ' '.join(text_parts)
                
                # Get document text embedding
                doc_vec, _ = embd_mdl.encode([doc_text])
                if not doc_vec or len(doc_vec) == 0:
                    continue
                
                doc_vec = np.array(doc_vec[0])
                
                # Cosine similarity
                similarity = np.dot(query_vec, doc_vec) / (
                    np.linalg.norm(query_vec) * np.linalg.norm(doc_vec) + 1e-8
                )
                
                if similarity >= config.metadata_similarity_threshold:
                    doc_scores.append((doc_id, float(similarity)))
            
            # Sort by similarity and return top docs
            doc_scores.sort(key=lambda x: x[1], reverse=True)
            return [doc_id for doc_id, _ in doc_scores[:config.doc_top_k]]
            
        except Exception as e:
            logging.error(f"Metadata similarity filter error: {e}")
            return []
    
    def _tier3_summary_mapping_retrieval(
        self,
        question: str,
        embd_mdl,
        tenant_ids,
        kb_ids: List[str],
        doc_ids: Optional[List[str]],
        page: int,
        page_size: int,
        similarity_threshold: float,
        vector_similarity_weight: float,
        top: int,
        aggs: bool,
        rerank_mdl,
        highlight: bool,
        rank_feature: Optional[dict],
        config: HierarchicalConfig
    ) -> Dict[str, Any]:
        """
        Tier 3 with parent-child chunking and summary mapping.
        
        First matches macro-themes via summary/parent vectors,
        then maps to original child chunks for detailed information.
        """
        try:
            if isinstance(tenant_ids, str):
                tenant_ids = tenant_ids.split(",")
            
            idx_names = [index_name(tid) for tid in tenant_ids]
            
            # Step 1: Search for parent/summary chunks first
            # Look for chunks with mom_id (parent chunks) or summary content
            parent_req = {
                "kb_ids": kb_ids,
                "doc_ids": doc_ids,
                "question": question,
                "size": config.chunk_top_k * 3,  # Get more parents to find children
                "topk": top,
                "similarity": similarity_threshold * 0.8,  # Slightly lower for parents
                "available_int": 1,
            }
            
            parent_sres = self.search(
                parent_req, idx_names, kb_ids, embd_mdl, 
                highlight=False, rank_feature=rank_feature
            )
            
            if not parent_sres or not parent_sres.ids:
                # Fallback to standard retrieval
                return self.retrieval(
                    question=question,
                    embd_mdl=embd_mdl,
                    tenant_ids=tenant_ids,
                    kb_ids=kb_ids,
                    page=page,
                    page_size=page_size,
                    similarity_threshold=similarity_threshold,
                    vector_similarity_weight=vector_similarity_weight,
                    top=top,
                    doc_ids=doc_ids,
                    aggs=aggs,
                    rerank_mdl=rerank_mdl,
                    highlight=highlight,
                    rank_feature=rank_feature,
                )
            
            # Step 2: Find child chunks for matched parents
            parent_ids = set()
            child_doc_ids = set()
            
            for chunk_id in parent_sres.ids:
                chunk = parent_sres.field.get(chunk_id, {})
                mom_id = chunk.get("mom_id")
                doc_id = chunk.get("doc_id")
                
                if mom_id:
                    # This is a child chunk, get its parent
                    parent_ids.add(mom_id)
                else:
                    # This might be a parent, use its ID
                    parent_ids.add(chunk_id)
                
                if doc_id:
                    child_doc_ids.add(doc_id)
            
            # Step 3: Retrieve child chunks for the matched parents
            # Search within the documents that contain matched parents
            child_req = {
                "kb_ids": kb_ids,
                "doc_ids": list(child_doc_ids) if child_doc_ids else doc_ids,
                "question": question,
                "size": page_size * 2,
                "topk": top,
                "similarity": similarity_threshold,
                "available_int": 1,
            }
            
            child_sres = self.search(
                child_req, idx_names, kb_ids, embd_mdl,
                highlight=highlight, rank_feature=rank_feature
            )
            
            if not child_sres or not child_sres.ids:
                # Use parent results if no children found
                child_sres = parent_sres
            
            # Step 4: Rerank and format results
            if rerank_mdl and child_sres.total > 0:
                sim, tsim, vsim = self.rerank_by_model(
                    rerank_mdl, child_sres, question,
                    1 - vector_similarity_weight, vector_similarity_weight,
                    rank_feature=rank_feature,
                )
            else:
                sim, tsim, vsim = self.rerank(
                    child_sres, question,
                    1 - vector_similarity_weight, vector_similarity_weight,
                    rank_feature=rank_feature,
                )
            
            # Format results
            ranks = {"total": 0, "chunks": [], "doc_aggs": {}}
            
            sim_np = np.array(sim, dtype=np.float64)
            if sim_np.size == 0:
                ranks["doc_aggs"] = []
                return ranks
            
            sorted_idx = np.argsort(sim_np * -1)
            valid_idx = [int(i) for i in sorted_idx if sim_np[i] >= similarity_threshold]
            ranks["total"] = len(valid_idx)
            
            if not valid_idx:
                ranks["doc_aggs"] = []
                return ranks
            
            # Get page of results
            begin = (page - 1) * page_size
            end = begin + page_size
            page_idx = valid_idx[begin:end]
            
            dim = len(child_sres.query_vector) if child_sres.query_vector else 0
            vector_column = f"q_{dim}_vec" if dim else ""
            zero_vector = [0.0] * dim if dim else []
            
            for i in page_idx:
                chunk_id = child_sres.ids[i]
                chunk = child_sres.field[chunk_id]
                
                d = {
                    "chunk_id": chunk_id,
                    "content_ltks": chunk.get("content_ltks", ""),
                    "content_with_weight": chunk.get("content_with_weight", ""),
                    "doc_id": chunk.get("doc_id", ""),
                    "docnm_kwd": chunk.get("docnm_kwd", ""),
                    "kb_id": chunk.get("kb_id", ""),
                    "important_kwd": chunk.get("important_kwd", []),
                    "image_id": chunk.get("img_id", ""),
                    "similarity": float(sim_np[i]),
                    "vector_similarity": float(vsim[i]) if i < len(vsim) else 0.0,
                    "term_similarity": float(tsim[i]) if i < len(tsim) else 0.0,
                    "vector": chunk.get(vector_column, zero_vector),
                    "positions": chunk.get("position_int", []),
                    "doc_type_kwd": chunk.get("doc_type_kwd", ""),
                    "mom_id": chunk.get("mom_id", ""),
                    "parent_matched": chunk_id in parent_ids or chunk.get("mom_id") in parent_ids,
                }
                
                if highlight and child_sres.highlight and chunk_id in child_sres.highlight:
                    d["highlight"] = remove_redundant_spaces(child_sres.highlight[chunk_id])
                
                ranks["chunks"].append(d)
            
            # Build doc aggregations
            if aggs:
                for i in valid_idx:
                    chunk = child_sres.field[child_sres.ids[i]]
                    dnm = chunk.get("docnm_kwd", "")
                    did = chunk.get("doc_id", "")
                    if dnm and dnm not in ranks["doc_aggs"]:
                        ranks["doc_aggs"][dnm] = {"doc_id": did, "count": 0}
                    if dnm:
                        ranks["doc_aggs"][dnm]["count"] += 1
                
                ranks["doc_aggs"] = [
                    {"doc_name": k, "doc_id": v["doc_id"], "count": v["count"]}
                    for k, v in sorted(ranks["doc_aggs"].items(), key=lambda x: x[1]["count"] * -1)
                ]
            else:
                ranks["doc_aggs"] = []
            
            logging.info(f"Summary mapping retrieval: {len(parent_ids)} parents -> {len(ranks['chunks'])} chunks")
            return ranks
            
        except Exception as e:
            logging.error(f"Summary mapping retrieval error: {e}")
            # Fallback to standard retrieval
            return self.retrieval(
                question=question,
                embd_mdl=embd_mdl,
                tenant_ids=tenant_ids,
                kb_ids=kb_ids,
                page=page,
                page_size=page_size,
                similarity_threshold=similarity_threshold,
                vector_similarity_weight=vector_similarity_weight,
                top=top,
                doc_ids=doc_ids,
                aggs=aggs,
                rerank_mdl=rerank_mdl,
                highlight=highlight,
                rank_feature=rank_feature,
            )
    
    def _apply_custom_keyword_extraction(
        self,
        chunks: List[Dict[str, Any]],
        prompt_template: str,
        embd_mdl=None
    ) -> List[Dict[str, Any]]:
        """
        Apply customizable prompts for chunk keyword extraction.
        
        Allows users to configure custom prompts for domain-specific
        keyword extraction to better align with their semantics.
        """
        try:
            for chunk in chunks:
                content = chunk.get("content_with_weight", "") or chunk.get("content_ltks", "")
                if not content:
                    continue
                
                # Extract keywords using tokenizer with custom weighting
                tokens = rag_tokenizer.tokenize(content).split()
                
                # Apply custom prompt logic if it contains extraction rules
                if "important" in prompt_template.lower():
                    # Prioritize capitalized words and technical terms
                    important_tokens = [
                        t for t in tokens 
                        if len(t) > 3 and (t[0].isupper() or '_' in t or '-' in t)
                    ]
                    if important_tokens:
                        existing = chunk.get("important_kwd", [])
                        if isinstance(existing, str):
                            existing = [existing]
                        chunk["important_kwd"] = list(set(existing + important_tokens[:10]))
                
                if "question" in prompt_template.lower():
                    # Generate potential questions from content
                    # This is a simplified version - full implementation would use LLM
                    if "?" not in content and len(content) > 50:
                        # Add a generated question hint
                        chunk["question_hint"] = f"What does this section explain about {tokens[0] if tokens else 'the topic'}?"
            
            return chunks
            
        except Exception as e:
            logging.error(f"Custom keyword extraction error: {e}")
            return chunks
    
    def _apply_llm_question_generation(
        self,
        chunks: List[Dict[str, Any]],
        prompt_template: Optional[str],
        chat_mdl
    ) -> List[Dict[str, Any]]:
        """
        Apply LLM-based question generation for chunks.
        
        Generates potential questions that each chunk could answer,
        improving retrieval by enabling question-to-question matching.
        """
        if not chat_mdl:
            return chunks
        
        try:
            default_prompt = """Based on the following content, generate 2-3 concise questions that this content could answer.
Return only the questions, one per line.

Content: {content}

Questions:"""
            
            prompt = prompt_template or default_prompt
            
            for chunk in chunks:
                content = chunk.get("content_with_weight", "") or chunk.get("content_ltks", "")
                if not content or len(content) < 50:
                    continue
                
                # Truncate content if too long
                content_truncated = content[:1500] if len(content) > 1500 else content
                
                try:
                    # Format prompt with content
                    formatted_prompt = prompt.replace("{content}", content_truncated)
                    
                    # Call LLM
                    response = chat_mdl.chat(formatted_prompt, [], {"temperature": 0.3, "max_tokens": 200})
                    
                    if response:
                        # Parse questions from response
                        questions = [
                            q.strip() for q in response.strip().split('\n')
                            if q.strip() and '?' in q
                        ]
                        
                        if questions:
                            chunk["generated_questions"] = questions[:3]
                            logging.debug(f"Generated {len(questions)} questions for chunk")
                
                except Exception as e:
                    logging.warning(f"Question generation failed for chunk: {e}")
                    continue
            
            return chunks
            
        except Exception as e:
            logging.error(f"LLM question generation error: {e}")
            return chunks
    
    def generate_document_metadata(
        self,
        doc_id: str,
        content: str,
        chat_mdl=None,
        embd_mdl=None
    ) -> Dict[str, Any]:
        """
        Generate enhanced metadata for a document.
        
        This is a Data Pipeline enhancement hook that generates:
        - Document summary
        - Key topics/themes
        - Suggested questions
        - Category classification
        
        Can be called during document ingestion to enrich metadata.
        """
        metadata = {
            "doc_id": doc_id,
            "generated": True,
            "summary": "",
            "topics": [],
            "suggested_questions": [],
            "category": "",
        }
        
        if not chat_mdl or not content:
            return metadata
        
        try:
            # Truncate content for LLM
            content_truncated = content[:3000] if len(content) > 3000 else content
            
            prompt = f"""Analyze the following document content and provide:
1. A brief summary (2-3 sentences)
2. Key topics/themes (comma-separated list)
3. 3 questions this document could answer
4. A category classification

Content:
{content_truncated}

Respond in this exact format:
SUMMARY: <summary>
TOPICS: <topic1>, <topic2>, <topic3>
QUESTIONS:
- <question1>
- <question2>
- <question3>
CATEGORY: <category>"""

            response = chat_mdl.chat(prompt, [], {"temperature": 0.2, "max_tokens": 500})
            
            if response:
                # Parse response
                lines = response.strip().split('\n')
                for line in lines:
                    line = line.strip()
                    if line.startswith("SUMMARY:"):
                        metadata["summary"] = line[8:].strip()
                    elif line.startswith("TOPICS:"):
                        topics = line[7:].strip().split(',')
                        metadata["topics"] = [t.strip() for t in topics if t.strip()]
                    elif line.startswith("- ") and "?" in line:
                        metadata["suggested_questions"].append(line[2:].strip())
                    elif line.startswith("CATEGORY:"):
                        metadata["category"] = line[9:].strip()
            
            # Generate summary embedding if embedding model available
            if embd_mdl and metadata["summary"]:
                try:
                    summary_vec, _ = embd_mdl.encode([metadata["summary"]])
                    if summary_vec and len(summary_vec) > 0:
                        metadata["summary_vector"] = summary_vec[0]
                except Exception:
                    pass
            
            logging.info(f"Generated metadata for doc {doc_id}: {len(metadata['topics'])} topics, {len(metadata['suggested_questions'])} questions")
            return metadata
            
        except Exception as e:
            logging.error(f"Document metadata generation error: {e}")
            return metadata

    def sql_retrieval(self, sql, fetch_size=128, format="json"):
        tbl = self.dataStore.sql(sql, fetch_size, format)
        return tbl

    def chunk_list(self, doc_id: str, tenant_id: str,
                   kb_ids: list[str], max_count=1024,
                   offset=0,
                   fields=["docnm_kwd", "content_with_weight", "img_id"],
                   sort_by_position: bool = False):
        condition = {"doc_id": doc_id}

        fields_set = set(fields or [])
        if sort_by_position:
            for need in ("page_num_int", "position_int", "top_int"):
                if need not in fields_set:
                    fields_set.add(need)
        fields = list(fields_set)

        orderBy = OrderByExpr()
        if sort_by_position:
            orderBy.asc("page_num_int")
            orderBy.asc("position_int")
            orderBy.asc("top_int")

        res = []
        bs = 128
        for p in range(offset, max_count, bs):
            es_res = self.dataStore.search(fields, [], condition, [], orderBy, p, bs, index_name(tenant_id),
                                           kb_ids)
            dict_chunks = self.dataStore.get_fields(es_res, fields)
            for id, doc in dict_chunks.items():
                doc["id"] = id
            if dict_chunks:
                res.extend(dict_chunks.values())
            # FIX: Solo terminar si no hay chunks, no si hay menos de bs
            if len(dict_chunks.values()) == 0:
                break
        return res

    def all_tags(self, tenant_id: str, kb_ids: list[str], S=1000):
        if not self.dataStore.indexExist(index_name(tenant_id), kb_ids[0]):
            return []
        res = self.dataStore.search([], [], {}, [], OrderByExpr(), 0, 0, index_name(tenant_id), kb_ids, ["tag_kwd"])
        return self.dataStore.get_aggregation(res, "tag_kwd")

    def all_tags_in_portion(self, tenant_id: str, kb_ids: list[str], S=1000):
        res = self.dataStore.search([], [], {}, [], OrderByExpr(), 0, 0, index_name(tenant_id), kb_ids, ["tag_kwd"])
        res = self.dataStore.get_aggregation(res, "tag_kwd")
        total = np.sum([c for _, c in res])
        return {t: (c + 1) / (total + S) for t, c in res}

    def tag_content(self, tenant_id: str, kb_ids: list[str], doc, all_tags, topn_tags=3, keywords_topn=30, S=1000):
        idx_nm = index_name(tenant_id)
        match_txt = self.qryr.paragraph(doc["title_tks"] + " " + doc["content_ltks"], doc.get("important_kwd", []), keywords_topn)
        res = self.dataStore.search([], [], {}, [match_txt], OrderByExpr(), 0, 0, idx_nm, kb_ids, ["tag_kwd"])
        aggs = self.dataStore.get_aggregation(res, "tag_kwd")
        if not aggs:
            return False
        cnt = np.sum([c for _, c in aggs])
        tag_fea = sorted([(a, round(0.1*(c + 1) / (cnt + S) / max(1e-6, all_tags.get(a, 0.0001)))) for a, c in aggs],
                         key=lambda x: x[1] * -1)[:topn_tags]
        doc[TAG_FLD] = {a.replace(".", "_"): c for a, c in tag_fea if c > 0}
        return True

    def tag_query(self, question: str, tenant_ids: str | list[str], kb_ids: list[str], all_tags, topn_tags=3, S=1000):
        if isinstance(tenant_ids, str):
            idx_nms = index_name(tenant_ids)
        else:
            idx_nms = [index_name(tid) for tid in tenant_ids]
        match_txt, _ = self.qryr.question(question, min_match=0.0)
        res = self.dataStore.search([], [], {}, [match_txt], OrderByExpr(), 0, 0, idx_nms, kb_ids, ["tag_kwd"])
        aggs = self.dataStore.get_aggregation(res, "tag_kwd")
        if not aggs:
            return {}
        cnt = np.sum([c for _, c in aggs])
        tag_fea = sorted([(a, round(0.1*(c + 1) / (cnt + S) / max(1e-6, all_tags.get(a, 0.0001)))) for a, c in aggs],
                         key=lambda x: x[1] * -1)[:topn_tags]
        return {a.replace(".", "_"): max(1, c) for a, c in tag_fea}

    def retrieval_by_toc(self, query:str, chunks:list[dict], tenant_ids:list[str], chat_mdl, topn: int=6):
        if not chunks:
            return []
        idx_nms = [index_name(tid) for tid in tenant_ids]
        ranks, doc_id2kb_id = {}, {}
        for ck in chunks:
            if ck["doc_id"] not in ranks:
                ranks[ck["doc_id"]] = 0
            ranks[ck["doc_id"]] += ck["similarity"]
            doc_id2kb_id[ck["doc_id"]] = ck["kb_id"]
        doc_id = sorted(ranks.items(), key=lambda x: x[1]*-1.)[0][0]
        kb_ids = [doc_id2kb_id[doc_id]]
        es_res = self.dataStore.search(["content_with_weight"], [], {"doc_id": doc_id, "toc_kwd": "toc"}, [], OrderByExpr(), 0, 128, idx_nms,
                                       kb_ids)
        toc = []
        dict_chunks = self.dataStore.get_fields(es_res, ["content_with_weight"])
        for _, doc in dict_chunks.items():
            try:
                toc.extend(json.loads(doc["content_with_weight"]))
            except Exception as e:
                logging.exception(e)
        if not toc:
            return chunks

        ids = relevant_chunks_with_toc(query, toc, chat_mdl, topn*2)
        if not ids:
            return chunks

        vector_size = 1024
        id2idx = {ck["chunk_id"]: i for i, ck in enumerate(chunks)}
        for cid, sim in ids:
            if cid in id2idx:
                chunks[id2idx[cid]]["similarity"] += sim
                continue
            chunk = self.dataStore.get(cid, idx_nms, kb_ids)
            d = {
                "chunk_id": cid,
                "content_ltks": chunk["content_ltks"],
                "content_with_weight": chunk["content_with_weight"],
                "doc_id": doc_id,
                "docnm_kwd": chunk.get("docnm_kwd", ""),
                "kb_id": chunk["kb_id"],
                "important_kwd": chunk.get("important_kwd", []),
                "image_id": chunk.get("img_id", ""),
                "similarity": sim,
                "vector_similarity": sim,
                "term_similarity": sim,
                "vector": [0.0] * vector_size,
                "positions": chunk.get("position_int", []),
                "doc_type_kwd": chunk.get("doc_type_kwd", "")
            }
            for k in chunk.keys():
                if k[-4:] == "_vec":
                    d["vector"] = chunk[k]
                    vector_size = len(chunk[k])
                    break
            chunks.append(d)

        return sorted(chunks, key=lambda x:x["similarity"]*-1)[:topn]

    def retrieval_by_children(self, chunks:list[dict], tenant_ids:list[str]):
        if not chunks:
            return []
        idx_nms = [index_name(tid) for tid in tenant_ids]
        mom_chunks = defaultdict(list)
        i = 0
        while i < len(chunks):
            ck = chunks[i]
            mom_id = ck.get("mom_id")
            if not isinstance(mom_id, str) or not mom_id.strip():
                i += 1
                continue
            mom_chunks[ck["mom_id"]].append(chunks.pop(i))

        if not mom_chunks:
            return chunks

        if not chunks:
            chunks = []

        vector_size = 1024
        for id, cks in mom_chunks.items():
            chunk = self.dataStore.get(id, idx_nms, [ck["kb_id"] for ck in cks])
            d = {
                "chunk_id": id,
                "content_ltks": " ".join([ck["content_ltks"] for ck in cks]),
                "content_with_weight": chunk["content_with_weight"],
                "doc_id": chunk["doc_id"],
                "docnm_kwd": chunk.get("docnm_kwd", ""),
                "kb_id": chunk["kb_id"],
                "important_kwd": [kwd for ck in cks for kwd in ck.get("important_kwd", [])],
                "image_id": chunk.get("img_id", ""),
                "similarity": np.mean([ck["similarity"] for ck in cks]),
                "vector_similarity": np.mean([ck["similarity"] for ck in cks]),
                "term_similarity": np.mean([ck["similarity"] for ck in cks]),
                "vector": [0.0] * vector_size,
                "positions": chunk.get("position_int", []),
                "doc_type_kwd": chunk.get("doc_type_kwd", "")
            }
            for k in cks[0].keys():
                if k[-4:] == "_vec":
                    d["vector"] = cks[0][k]
                    vector_size = len(cks[0][k])
                    break
            chunks.append(d)

        return sorted(chunks, key=lambda x:x["similarity"]*-1)
