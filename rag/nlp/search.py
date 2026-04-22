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
from collections import OrderedDict, defaultdict
from dataclasses import dataclass
from typing import List, Dict, Any, Optional, Union

import numpy as np

from common import settings
from common.constants import PAGERANK_FLD, TAG_FLD
from common.doc_store.doc_store_base import MatchDenseExpr, FusionExpr, OrderByExpr, DocStoreConnection
from common.float_utils import get_float
from common.misc_utils import thread_pool_exec
from common.string_utils import remove_redundant_spaces
from common.tag_feature_utils import parse_tag_features
from rag.nlp import rag_tokenizer, query

logger = logging.getLogger(__name__)

# Global configuration constants
DEFAULT_FUSION_WEIGHTS = "0.05,0.95"
DEFAULT_RERANK_LIMIT = 64
DEFAULT_CITATION_THRESHOLD = 0.63
DEFAULT_SIMILARITY_THRESHOLD = 0.1
RETRY_SIMILARITY_FACTOR = 0.17
RETRY_MIN_MATCH_FACTOR = 0.1
INTERNAL_SMOOTH_S = 1000


def index_name(uid) -> str:
    """Generate index name for tenant id."""
    return f"ragflow_{uid}"


class Dealer:
    """Core retrieval dealer class, handles query routing, hybrid search and reranking."""
    def __init__(self, dataStore: DocStoreConnection):
        self.qryr = query.FulltextQueryer()
        self.dataStore = dataStore

    @dataclass
    class SearchResult:
        """Dataclass for search result return structure."""
        total: int
        ids: List[str]
        query_vector: Optional[List[float]] = None
        field: Optional[Dict[str, Any]] = None
        highlight: Optional[Dict[str, Any]] = None
        aggregation: Optional[Union[List, Dict]] = None
        keywords: Optional[List[str]] = None
        group_docs: Optional[List[List]] = None

    async def get_vector(self, txt: str, emb_mdl, topk: int = 10, similarity: float = 0.1) -> MatchDenseExpr:
        """Encode text to dense vector expression for vector search."""
        qv, _ = await thread_pool_exec(emb_mdl.encode_queries, txt)
        shape = np.array(qv).shape
        if len(shape) > 1:
            raise Exception(f"Vector shape {shape} is invalid, expected 1D array")
        
        embedding_data = [get_float(v) for v in qv]
        vector_column_name = f"q_{len(embedding_data)}_vec"
        return MatchDenseExpr(vector_column_name, embedding_data, 'float', 'cosine', topk, {"similarity": similarity})

    def get_filters(self, req: Dict[str, Any]) -> Dict[str, Any]:
        """Build filter conditions from request parameters."""
        condition = {}
        key_mapping = {"kb_ids": "kb_id", "doc_ids": "doc_id"}
        for key, field in key_mapping.items():
            if req.get(key) is not None:
                condition[field] = req[key]
        
        system_keys = ["knowledge_graph_kwd", "available_int", "entity_kwd", 
                      "from_entity_kwd", "to_entity_kwd", "removed_kwd"]
        for key in system_keys:
            if req.get(key) is not None:
                condition[key] = req[key]
        return condition

    async def search(self, req: Dict[str, Any], idx_names: Union[str, List[str]],
                     kb_ids: List[str], emb_mdl=None,
                     highlight: Optional[Union[bool, List]] = None,
                     rank_feature: Optional[Dict] = None):
        """Execute hybrid search with fulltext & vector retrieval."""
        highlight = highlight or False
        filters = self.get_filters(req)
        orderBy = OrderByExpr()

        pg = max(1, int(req.get("page", 1))) - 1
        topk = int(req.get("topk", 1024))
        ps = int(req.get("size", topk))
        offset, limit = pg * ps, ps

        default_fields = ["docnm_kwd", "content_ltks", "kb_id", "img_id", "title_tks", 
                         "important_kwd", "position_int", "doc_id", "chunk_order_int", 
                         "page_num_int", "top_int", "create_timestamp_flt", 
                         "knowledge_graph_kwd", "question_kwd", "question_tks", 
                         "doc_type_kwd", "available_int", "content_with_weight", 
                         "mom_id", PAGERANK_FLD, TAG_FLD, "row_id()"]
        src = req.get("fields", default_fields)
        kwds = set()
        qst = req.get("question", "")
        q_vec = []
        matchDense = None
        fusionExpr = None

        if not qst:
            if req.get("sort"):
                orderBy.asc("chunk_order_int").asc("page_num_int").asc("top_int").desc("create_timestamp_flt")
            res = await thread_pool_exec(
                self.dataStore.search, src, [], filters, [], orderBy, offset, limit, idx_names, kb_ids
            )
            total = self.dataStore.get_total(res)
            logger.debug("Dealer.search TOTAL (no query): %d", total)
        else:
            highlight_fields = ["content_ltks", "title_tks"] if highlight else (highlight if isinstance(highlight, list) else [])
            matchText, keywords = self.qryr.question(qst, min_match=0.3)

            if emb_mdl is None:
                match_exprs = [matchText]
                res = await thread_pool_exec(
                    self.dataStore.search, src, highlight_fields, filters, match_exprs, orderBy,
                    offset, limit, idx_names, kb_ids, rank_feature=rank_feature
                )
            else:
                matchDense = await self.get_vector(qst, emb_mdl, topk, req.get("similarity", DEFAULT_SIMILARITY_THRESHOLD))
                q_vec = matchDense.embedding_data
                if not settings.DOC_ENGINE_INFINITY:
                    src.append(f"q_{len(q_vec)}_vec")

                fusionExpr = FusionExpr("weighted_sum", topk, {"weights": DEFAULT_FUSION_WEIGHTS})
                match_exprs = [matchText, matchDense, fusionExpr]
                res = await thread_pool_exec(
                    self.dataStore.search, src, highlight_fields, filters, match_exprs, orderBy,
                    offset, limit, idx_names, kb_ids, rank_feature=rank_feature
                )

            total = self.dataStore.get_total(res)
            logger.debug("Dealer.search TOTAL: %d", total)

            if total == 0:
                try:
                    if filters.get("doc_id"):
                        res = await thread_pool_exec(
                            self.dataStore.search, src, [], filters, [], orderBy, offset, limit, idx_names, kb_ids
                        )
                    else:
                        user_sim = req.get("similarity", DEFAULT_SIMILARITY_THRESHOLD)
                        # Fixed: Truly loosen vector similarity threshold, dead min() logic removed
                        retry_sim = max(0.01, user_sim * 0.5)
                        retry_min_match = min(RETRY_MIN_MATCH_FACTOR, 0.3)
                        matchText_retry, _ = self.qryr.question(qst, min_match=retry_min_match)
                        
                        if emb_mdl is None:
                            res = await thread_pool_exec(
                                self.dataStore.search, src, highlight_fields, filters,
                                [matchText_retry], orderBy, offset, limit,
                                idx_names, kb_ids, rank_feature=rank_feature
                            )
                        else:
                            matchDense.extra_options["similarity"] = retry_sim
                            res = await thread_pool_exec(
                                self.dataStore.search, src, highlight_fields, filters,
                                [matchText_retry, matchDense, fusionExpr], orderBy, offset, limit,
                                idx_names, kb_ids, rank_feature=rank_feature
                            )
                    total = self.dataStore.get_total(res)
                    logger.debug("Dealer.search retry TOTAL: %d", total)
                except Exception as e:
                    logger.warning("Search retry failed: %s", str(e), exc_info=True)

            for k in keywords:
                kwds.add(k)
                fine_grams = rag_tokenizer.fine_grained_tokenize(k).split()
                kwds.update([kk for kk in fine_grams if len(kk) >= 2])

        ids = self.dataStore.get_doc_ids(res)
        highlight = self.dataStore.get_highlight(res, list(kwds), "content_with_weight")
        aggs = self.dataStore.get_aggregation(res, "docnm_kwd")

        return self.SearchResult(
            total=total, ids=ids, query_vector=q_vec, aggregation=aggs,
            highlight=highlight, field=self.dataStore.get_fields(res, src + ["_score"]),
            keywords=list(kwds)
        )

    @staticmethod
    def trans2floats(txt: str) -> List[float]:
        """Convert tab-separated string to float list."""
        return [get_float(t) for t in txt.split("\t")]

    def insert_citations(self, answer: str, chunks: List[str], chunk_v: List[List[float]],
                         embd_mdl, tkweight: float = 0.1, vtweight: float = 0.9) -> tuple[str, set]:
        """Insert citation markers with hybrid matching, no in-place modification to input params."""
        if not chunks:
            return answer, set()
        
        local_chunk_v = list(chunk_v)
        pieces = re.split(r"(```)", answer)
        processed_pieces = []
        i = 0
        while i < len(pieces):
            if pieces[i] == "```":
                end_idx = i + 1
                while end_idx < len(pieces) and pieces[end_idx] != "```":
                    end_idx += 1
                processed_pieces.append("".join(pieces[i:end_idx+1]) + "\n")
                i = end_idx + 1
            else:
                processed_pieces.extend(
                    re.split(r"([^\|][；。？!！،؛؟۔\n]|[a-z\u0600-\u06FF][.?;!،؛؟][ \n])", pieces[i])
                )
                i += 1

        for i in range(1, len(processed_pieces)):
            if re.match(r"([^\|][；。？!！،؛؟۔\n]|[a-z\u0600-\u06FF][.?;!،؛؟][ \n])", processed_pieces[i]):
                processed_pieces[i-1] += processed_pieces[i][0]
                processed_pieces[i] = processed_pieces[i][1:]

        valid_indices = [i for i, t in enumerate(processed_pieces) if len(t) >= 5]
        valid_pieces = [processed_pieces[i] for i in valid_indices]
        
        if not valid_pieces:
            return answer, set()

        try:
            ans_v, _ = embd_mdl.encode(valid_pieces)
            target_dim = len(ans_v[0])
            mismatched = 0
            for idx in range(len(local_chunk_v)):
                if len(local_chunk_v[idx]) != target_dim:
                    local_chunk_v[idx] = [0.0] * target_dim
                    mismatched += 1
            if mismatched > 0:
                logger.warning(
                    "Insert citations: %d/%d chunk vectors dimension mismatch (expected %d), replaced with zero vector",
                    mismatched, len(local_chunk_v), target_dim
                )
        except Exception as e:
            logger.error("Citation embedding failed: %s", str(e))
            return answer, set()

        chunks_tks = [rag_tokenizer.tokenize(self.qryr.rmWWW(ck)).split() for ck in chunks]
        citations = {}
        threshold = DEFAULT_CITATION_THRESHOLD

        try:
            sim_matrix = []
            for i, piece in enumerate(valid_pieces):
                sim, _, _ = self.qryr.hybrid_similarity(
                    ans_v[i], local_chunk_v,
                    rag_tokenizer.tokenize(self.qryr.rmWWW(piece)).split(),
                    chunks_tks, tkweight, vtweight
                )
                sim_matrix.append(sim)
        except Exception as e:
            logger.warning("Citation matching failed: %s", str(e))
            return answer, set()

        while threshold > 0.3 and not citations:
            for i, piece in enumerate(valid_pieces):
                sim = sim_matrix[i]
                max_sim = np.max(sim) * 0.99
                if max_sim >= threshold:
                    citations[valid_indices[i]] = [str(ii) for ii in np.argsort(sim)[::-1] if sim[ii] > max_sim][:4]
            threshold *= 0.8

        result = ""
        global_cited = set()
        for i, piece in enumerate(processed_pieces):
            result += piece
            if i not in citations:
                continue
            seen_here = set()
            for cid in citations[i]:
                if int(cid) >= len(local_chunk_v) or cid in seen_here:
                    continue
                result += f" [ID:{cid}]"
                seen_here.add(cid)
                global_cited.add(cid)

        return result, global_cited

    def _rank_feature_scores(self, query_rfea: Optional[Dict], search_res: "Dealer.SearchResult") -> np.ndarray:
        """Calculate tag feature & pagerank composite ranking score."""
        if not search_res.ids or not search_res.field:
            return np.array([])

        pageranks = np.array([search_res.field.get(chunk_id, {}).get(PAGERANK_FLD, 0) for chunk_id in search_res.ids], dtype=float)
        if not query_rfea:
            return pageranks

        q_norm = math.sqrt(sum(s**2 for t, s in query_rfea.items() if t != PAGERANK_FLD))
        if q_norm == 0:
            return pageranks

        rank_scores = []
        for chunk_id in search_res.ids:
            chunk = search_res.field.get(chunk_id, {})
            tag_feas = parse_tag_features(chunk.get(TAG_FLD, ""), allow_json_string=True, allow_python_literal=True)
            if not tag_feas:
                rank_scores.append(0)
                continue

            numerator, denominator = 0, 0
            for tag, score in tag_feas.items():
                if tag in query_rfea:
                    numerator += query_rfea[tag] * score
                denominator += score ** 2

            rank_scores.append(numerator / math.sqrt(denominator) / q_norm if denominator != 0 else 0)
        return np.array(rank_scores) * 10 + pageranks

    def rerank(self, sres: "Dealer.SearchResult", query_txt: str, tkweight: float = 0.3,
               vtweight: float = 0.7, cfield: str = "content_ltks",
               rank_feature: Optional[Dict] = None) -> tuple[np.ndarray, np.ndarray, np.ndarray]:
        """Internal hybrid rerank with token & vector similarity."""
        if not sres.ids or not sres.field or not sres.query_vector:
            return np.array([]), np.array([]), np.array([])

        _, keywords = self.qryr.question(query_txt)
        vec_size = len(sres.query_vector)
        vec_col = f"q_{vec_size}_vec"
        zero_vec = [0.0] * vec_size

        ins_embd = []
        for chunk_id in sres.ids:
            vec = sres.field.get(chunk_id, {}).get(vec_col, zero_vec)
            if isinstance(vec, str):
                vec = self.trans2floats(vec)
            ins_embd.append(vec)
        if not ins_embd:
            return np.array([]), np.array([]), np.array([])

        ins_tw = []
        for chunk_id in sres.ids:
            chunk = sres.field.get(chunk_id, {})
            imp_kwd = chunk.get("important_kwd", [])
            if not isinstance(imp_kwd, list):
                imp_kwd = [imp_kwd]

            content_tks = list(OrderedDict.fromkeys(chunk.get(cfield, "").split()))
            title_tks = chunk.get("title_tks", "").split()
            q_tks = chunk.get("question_tks", "").split()
            tks = content_tks + title_tks*2 + imp_kwd*5 + q_tks*6
            ins_tw.append(tks)

        rank_fea = self._rank_feature_scores(rank_feature, sres)
        sim, tksim, vtsim = self.qryr.hybrid_similarity(
            sres.query_vector, ins_embd, keywords, ins_tw, tkweight, vtweight
        )
        return sim + rank_fea, tksim, vtsim

    def rerank_by_model(self, rerank_mdl, sres: "Dealer.SearchResult", query_txt: str,
                        tkweight: float = 0.3, vtweight: float = 0.7,
                        cfield: str = "content_ltks", rank_feature: Optional[Dict] = None) -> tuple[np.ndarray, np.ndarray, np.ndarray]:
        """External rerank model integration."""
        if not sres.ids or not sres.field:
            return np.array([]), np.array([]), np.array([])

        _, keywords = self.qryr.question(query_txt)
        ins_tw = []
        for chunk_id in sres.ids:
            chunk = sres.field.get(chunk_id, {})
            imp_kwd = chunk.get("important_kwd", [])
            if not isinstance(imp_kwd, list):
                imp_kwd = [imp_kwd]
            tks = chunk.get(cfield, "").split() + chunk.get("title_tks", "").split() + imp_kwd
            ins_tw.append(tks)

        tksim = self.qryr.token_similarity(keywords, ins_tw)
        contents = [remove_redundant_spaces(" ".join(tks)) for tks in ins_tw]
        vtsim, _ = rerank_mdl.similarity(query_txt, contents)
        rank_fea = self._rank_feature_scores(rank_feature, sres)

        return tkweight * np.array(tksim) + vtweight * vtsim + rank_fea, tksim, vtsim

    def hybrid_similarity(self, ans_embd: List[float], ins_embd: List[List[float]], 
                          ans: str, inst: str) -> tuple[np.ndarray, np.ndarray, np.ndarray]:
        """Calculate hybrid token+vector similarity between two texts."""
        return self.qryr.hybrid_similarity(
            ans_embd, ins_embd,
            rag_tokenizer.tokenize(ans).split(),
            rag_tokenizer.tokenize(inst).split()
        )

    async def retrieval(
            self, question: str, embd_mdl, tenant_ids: Union[str, List[str]], kb_ids: List[str],
            page: int, page_size: int, similarity_threshold: float = 0.2,
            vector_similarity_weight: float = 0.3, top: int = 1024,
            doc_ids: Optional[List[str]] = None, aggs: bool = True,
            rerank_mdl=None, highlight: bool = False,
            rank_feature: Optional[Dict] = None
    ):
        """End-to-end retrieval pipeline with dynamic vector dimension support."""
        rank_feature = rank_feature or {PAGERANK_FLD: 10}
        ranks = {"total": 0, "chunks": [], "doc_aggs": []}
        
        if not question:
            return ranks

        page_size = max(1, int(page_size))
        page = max(page, 1)
        if page_size == 1:
            rerank_limit = max(30, DEFAULT_RERANK_LIMIT)
        else:
            rerank_limit = max(30, math.ceil(DEFAULT_RERANK_LIMIT / page_size) * page_size)
        if rerank_mdl and top > 0:
            rerank_limit = min(rerank_limit, top, DEFAULT_RERANK_LIMIT)

        global_offset = (page - 1) * page_size
        req = {
            "kb_ids": kb_ids, "doc_ids": doc_ids,
            "page": global_offset // rerank_limit + 1,
            "size": rerank_limit, "question": question,
            "vector": True, "topk": top,
            "similarity": similarity_threshold, "available_int": 1
        }

        if isinstance(tenant_ids, str):
            tenant_ids = tenant_ids.split(",")

        try:
            sres = await self.search(
                req, [index_name(tid) for tid in tenant_ids], kb_ids, 
                embd_mdl, highlight, rank_feature=rank_feature
            )
        except Exception as e:
            logger.error("Retrieval search failed (tenants=%s): %s", tenant_ids, str(e), exc_info=True)
            return ranks

        try:
            if rerank_mdl and sres.total > 0:
                sim, tsim, vsim = self.rerank_by_model(
                    rerank_mdl, sres, question, 1-vector_similarity_weight,
                    vector_similarity_weight, rank_feature=rank_feature
                )
            else:
                if settings.DOC_ENGINE_INFINITY:
                    sim = [sres.field.get(id, {}).get("_score", 0.0) for id in sres.ids]
                    sim = [s if s is not None else 0.0 for s in sim]
                    tsim = vsim = sim
                else:
                    sim, tsim, vsim = self.rerank(
                        sres, question, 1-vector_similarity_weight,
                        vector_similarity_weight, rank_feature=rank_feature
                    )
        except Exception as e:
            logger.error("Rerank failed: %s", str(e), exc_info=True)
            return ranks

        sim_np = np.array(sim, dtype=np.float64)
        if sim_np.size == 0:
            return ranks

        sorted_idx = np.argsort(sim_np * -1)
        post_threshold = 0.0 if vector_similarity_weight <= 0 or doc_ids else similarity_threshold
        valid_idx = [int(i) for i in sorted_idx if sim_np[i] >= post_threshold]
        ranks["total"] = len(valid_idx)
        if ranks["total"] == 0:
            return ranks

        begin = global_offset % rerank_limit
        page_idx = valid_idx[begin:begin+page_size]
        qv_size = len(sres.query_vector) if sres.query_vector else 0
        vec_col = f"q_{qv_size}_vec" if qv_size > 0 else ""
        zero_vec = [0.0] * qv_size if qv_size > 0 else []

        for i in page_idx:
            if i >= len(sres.ids):
                continue
            chunk_id = sres.ids[i]
            chunk = sres.field.get(chunk_id, {})
            if not chunk:
                continue

            d = {
                "chunk_id": chunk_id,
                "content_ltks": chunk.get("content_ltks", ""),
                "content_with_weight": chunk.get("content_with_weight", ""),
                "doc_id": chunk.get("doc_id", ""),
                "docnm_kwd": chunk.get("docnm_kwd", ""),
                "kb_id": chunk.get("kb_id", ""),
                "important_kwd": chunk.get("important_kwd", []),
                "tag_kwd": chunk.get("tag_kwd", []),
                "image_id": chunk.get("img_id", ""),
                "similarity": float(sim_np[i]),
                "vector_similarity": float(vsim[i] if i < len(vsim) else 0),
                "term_similarity": float(tsim[i] if i < len(tsim) else 0),
                "vector": chunk.get(vec_col, zero_vec),
                "positions": chunk.get("position_int", []),
                "doc_type_kwd": chunk.get("doc_type_kwd", ""),
                "mom_id": chunk.get("mom_id", ""),
                "row_id": chunk.get("row_id()"),
            }

            if highlight and sres.highlight:
                d["highlight"] = remove_redundant_spaces(sres.highlight.get(chunk_id, d["content_with_weight"]))
            ranks["chunks"].append(d)

        if aggs:
            doc_aggs = defaultdict(lambda: {"doc_id": "", "count": 0})
            for i in valid_idx:
                if i >= len(sres.ids):
                    continue
                chunk = sres.field.get(sres.ids[i], {})
                doc_name = chunk.get("docnm_kwd", "")
                doc_aggs[doc_name]["doc_id"] = chunk.get("doc_id", "")
                doc_aggs[doc_name]["count"] += 1

            ranks["doc_aggs"] = [
                {"doc_name": k, "doc_id": v["doc_id"], "count": v["count"]}
                for k, v in sorted(doc_aggs.items(), key=lambda x: x[1]["count"], reverse=True)
            ]

        return ranks

    def sql_retrieval(self, sql: str, fetch_size: int = 128, format: str = "json"):
        """Execute raw SQL query on document store."""
        return self.dataStore.sql(sql, fetch_size, format)

    async def chunk_list(self, doc_id: str, tenant_id: str, kb_ids: List[str], 
                   max_count: int = 1024, offset: int = 0, 
                   fields: List[str] = None, sort_by_position: bool = False):
        """Async fetch chunks with thread pool, no event loop blocking."""
        fields = fields or ["docnm_kwd", "content_with_weight", "img_id"]
        condition = {"doc_id": doc_id}
        fields_set = set(fields)
        
        if sort_by_position:
            fields_set.update(["page_num_int", "position_int", "top_int"])
        
        orderBy = OrderByExpr()
        if sort_by_position:
            orderBy.asc("page_num_int").asc("position_int").asc("top_int")

        res = []
        batch_size = 128
        for p in range(offset, max_count, batch_size):
            limit = min(batch_size, max_count - p)
            if limit <= 0:
                break
            es_res = await thread_pool_exec(
                self.dataStore.search, list(fields_set), [], condition, [], orderBy, p, limit,
                index_name(tenant_id), kb_ids
            )
            chunks = self.dataStore.get_fields(es_res, list(fields_set))
            for id, doc in chunks.items():
                doc["id"] = id
            res.extend(chunks.values())
            if len(chunks) < limit:
                break
        return res

    def all_tags(self, tenant_id: str, kb_ids: List[str]) -> List:
        """Aggregate all tag keywords, unused S parameter fully removed."""
        if not self.dataStore.index_exist(index_name(tenant_id), kb_ids[0]):
            return []
        res = self.dataStore.search([], [], {}, [], OrderByExpr(), 0, 0, 
                                   index_name(tenant_id), kb_ids, ["tag_kwd"])
        return self.dataStore.get_aggregation(res, "tag_kwd")

    def all_tags_in_portion(self, tenant_id: str, kb_ids: List[str]) -> Dict:
        """Calculate normalized tag occurrence ratio with internal smoothing constant."""
        res = self.all_tags(tenant_id, kb_ids)
        total = sum(c for _, c in res)
        return {t: (c+1)/(total+INTERNAL_SMOOTH_S) for t, c in res}

    def tag_content(self, tenant_id: str, kb_ids: List[str], doc: Dict, 
                    all_tags: Dict, topn_tags: int = 3, keywords_topn: int = 30) -> bool:
        """Extract tag features for document chunk."""
        match_txt = self.qryr.paragraph(
            doc.get("title_tks", "") + " " + doc.get("content_ltks", ""),
            doc.get("important_kwd", []), keywords_topn
        )
        res = self.dataStore.search([], [], {}, [match_txt], OrderByExpr(), 0, 0,
                                   index_name(tenant_id), kb_ids, ["tag_kwd"])
        aggs = self.dataStore.get_aggregation(res, "tag_kwd")
        if not aggs:
            return False
        
        total = sum(c for _, c in aggs)
        tag_scores = [(a, round(0.1*(c+1)/(total+INTERNAL_SMOOTH_S)/max(1e-6, all_tags.get(a, 0.0001)))) 
                     for a, c in aggs]
        doc[TAG_FLD] = {a.replace(".", "_"): s for a, s in sorted(tag_scores, key=lambda x:x[1], reverse=True)[:topn_tags] if s>0}
        return True

    def tag_query(self, question: str, tenant_ids: Union[str, List[str]], kb_ids: List[str],
                  all_tags: Dict, topn_tags: int = 3) -> Dict:
        """Extract query-side tag features for ranking."""
        idx_nms = index_name(tenant_ids) if isinstance(tenant_ids, str) else [index_name(tid) for tid in tenant_ids]
        match_txt, _ = self.qryr.question(question, min_match=0.0)
        res = self.dataStore.search([], [], {}, [match_txt], OrderByExpr(), 0, 0, idx_nms, kb_ids, ["tag_kwd"])
        aggs = self.dataStore.get_aggregation(res, "tag_kwd")
        if not aggs:
            return {}
        
        total = sum(c for _, c in aggs)
        tag_scores = [(a, round(0.1*(c+1)/(total+INTERNAL_SMOOTH_S)/max(1e-6, all_tags.get(a, 0.0001)))) 
                     for a, c in aggs]
        return {a.replace(".", "_"): max(1, s) for a, s in sorted(tag_scores, key=lambda x:x[1], reverse=True)[:topn_tags]}

    async def retrieval_by_toc(self, query_txt: str, chunks: List[Dict], 
                               tenant_ids: List[str], chat_mdl, topn: int = 6) -> List[Dict]:
        """TOC retrieval with unified return format for all branches."""
        from rag.prompts.generator import relevant_chunks_with_toc
        cloned_chunks = [chunk.copy() for chunk in chunks] if chunks else []
        
        if not cloned_chunks:
            return sorted(cloned_chunks, key=lambda x: x.get("similarity", 0) * -1)[:topn]

        doc_scores = defaultdict(int)
        doc2kb = {}
        for ck in cloned_chunks:
            doc_scores[ck["doc_id"]] += ck.get("similarity", 0)
            doc2kb[ck["doc_id"]] = ck["kb_id"]
        
        if not doc_scores:
            return sorted(cloned_chunks, key=lambda x: x.get("similarity", 0) * -1)[:topn]
        
        top_doc_id = max(doc_scores.items(), key=lambda x: x[1])[0]
        kb_ids = [doc2kb[top_doc_id]]

        es_res = await thread_pool_exec(
            self.dataStore.search, ["content_with_weight"], [], {"doc_id": top_doc_id, "toc_kwd": "toc"}, [],
            OrderByExpr(), 0, 128, [index_name(tid) for tid in tenant_ids], kb_ids
        )
        toc = []
        for doc in self.dataStore.get_fields(es_res, ["content_with_weight"]).values():
            try:
                toc.extend(json.loads(doc["content_with_weight"]))
            except Exception:
                continue

        if not toc:
            return sorted(cloned_chunks, key=lambda x: x.get("similarity", 0) * -1)[:topn]

        try:
            ids = await relevant_chunks_with_toc(query_txt, toc, chat_mdl, topn*2)
        except Exception as e:
            logger.warning("TOC retrieval failed: %s", str(e))
            return sorted(cloned_chunks, key=lambda x: x.get("similarity", 0) * -1)[:topn]

        if not ids:
            return sorted(cloned_chunks, key=lambda x: x.get("similarity", 0) * -1)[:topn]

        chunk_map = {ck["chunk_id"]: i for i, ck in enumerate(cloned_chunks)}
        qv_size = len(chunks[0].get("vector", [])) if chunks else 0
        zero_vec = [0.0] * qv_size
        
        for cid, sim in ids:
            if cid in chunk_map:
                cloned_chunks[chunk_map[cid]]["similarity"] += sim
                continue
            chunk = await thread_pool_exec(self.dataStore.get, cid, index_name(tenant_ids[0]), kb_ids)
            if not chunk:
                continue
            vec = chunk.get(next((k for k in chunk if k.endswith("_vec")), ""), zero_vec)
            cloned_chunks.append({
                "chunk_id": cid,
                "content_ltks": chunk.get("content_ltks", ""),
                "content_with_weight": chunk.get("content_with_weight", ""),
                "doc_id": top_doc_id,
                "docnm_kwd": chunk.get("docnm_kwd", ""),
                "kb_id": chunk.get("kb_id", ""),
                "important_kwd": chunk.get("important_kwd", []),
                "image_id": chunk.get("img_id", ""),
                "similarity": sim,
                "vector_similarity": sim,
                "term_similarity": sim,
                "vector": vec,
                "positions": chunk.get("position_int", []),
                "doc_type_kwd": chunk.get("doc_type_kwd", "")
            })

        return sorted(cloned_chunks, key=lambda x: x["similarity"] * -1)[:topn]

    def retrieval_by_children(self, chunks: List[Dict], tenant_ids: List[str]) -> List[Dict]:
        """Merge parent chunks: full KeyError guard + important_kwd type pollution defense."""
        if not chunks:
            return []

        mom_chunks = defaultdict(list)
        filtered_chunks = []
        for ck in chunks:
            mom_id = ck.get("mom_id")
            if isinstance(mom_id, str) and mom_id.strip():
                mom_chunks[mom_id].append(ck)
            else:
                filtered_chunks.append(ck)

        if not mom_chunks:
            return sorted(filtered_chunks, key=lambda x: x.get("similarity", 0) * -1)

        qv_size = len(chunks[0].get("vector", [])) if chunks else 0
        zero_vec = [0.0] * qv_size
        
        for mom_id, child_chunks in mom_chunks.items():
            try:
                chunk = self.dataStore.get(mom_id, index_name(tenant_ids[0]), [child_chunks[0]["kb_id"]])
                if not chunk:
                    filtered_chunks.extend(child_chunks)
                    continue

                # Fixed: safe .get() for content_ltks + defensive normalize important_kwd
                content_ltks_merged = " ".join(ck.get("content_ltks", "") for ck in child_chunks)
                merged_important = []
                for ck in child_chunks:
                    kwd_val = ck.get("important_kwd", [])
                    if isinstance(kwd_val, str):
                        kwd_val = [kwd_val]
                    merged_important.extend(kwd_val)

                vec = child_chunks[0].get(next((k for k in child_chunks[0] if k.endswith("_vec")), ""), zero_vec)
                filtered_chunks.append({
                    "chunk_id": mom_id,
                    "content_ltks": content_ltks_merged,
                    "content_with_weight": chunk.get("content_with_weight", ""),
                    "doc_id": chunk.get("doc_id", ""),
                    "docnm_kwd": chunk.get("docnm_kwd", ""),
                    "kb_id": chunk.get("kb_id", ""),
                    "important_kwd": merged_important,
                    "image_id": chunk.get("img_id", ""),
                    "similarity": np.mean([ck.get("similarity", 0) for ck in child_chunks]),
                    "vector_similarity": np.mean([ck.get("similarity", 0) for ck in child_chunks]),
                    "term_similarity": np.mean([ck.get("similarity", 0) for ck in child_chunks]),
                    "vector": vec,
                    "positions": chunk.get("position_int", []),
                    "doc_type_kwd": chunk.get("doc_type_kwd", "")
                })
            except Exception as e:
                logger.warning("Merge child chunks failed: %s", str(e))
                filtered_chunks.extend(child_chunks)

        return sorted(filtered_chunks, key=lambda x: x["similarity"] * -1)
