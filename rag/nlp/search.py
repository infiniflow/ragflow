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

from rag.nlp import rag_tokenizer, query
import numpy as np
from common.doc_store.doc_store_base import MatchDenseExpr, FusionExpr, OrderByExpr, DocStoreConnection
from common.string_utils import remove_redundant_spaces
from common.float_utils import get_float
from common.constants import PAGERANK_FLD, TAG_FLD
from common.tag_feature_utils import parse_tag_features
from common import settings

from common.misc_utils import thread_pool_exec

RETRIEVAL_DEBUG_TRACE_ENABLED = False

def index_name(uid): return f"ragflow_{uid}"


@dataclass
class ChunkDebugInfo:
    chunk_id: str
    doc_id: str
    doc_name: str
    kb_id: str
    initial_score: float = 0.0
    term_similarity: float = 0.0
    vector_similarity: float = 0.0
    rerank_score: float = 0.0
    filter_reason: str | None = None
    final_position: int | None = None
    content_preview: str = ""
    is_pruned: bool = False
    rank_feature_score: float = 0.0

    def to_dict(self) -> dict:
        return {
            "chunk_id": self.chunk_id,
            "doc_id": self.doc_id,
            "doc_name": self.doc_name,
            "kb_id": self.kb_id,
            "initial_score": self.initial_score,
            "term_similarity": self.term_similarity,
            "vector_similarity": self.vector_similarity,
            "rerank_score": self.rerank_score,
            "rank_feature_score": self.rank_feature_score,
            "filter_reason": self.filter_reason,
            "final_position": self.final_position,
            "content_preview": self.content_preview[:100] if self.content_preview else "",
            "is_pruned": self.is_pruned,
        }


@dataclass
class RetrievalDebugTrace:
    query: str
    tenant_ids: list[str]
    kb_ids: list[str]
    top_k: int
    top_n: int
    similarity_threshold: float
    vector_similarity_weight: float

    initial_search_count: int = 0
    pruned_count: int = 0
    rerank_used: bool = False
    rerank_model: str | None = None
    filtered_by_threshold_count: int = 0
    filtered_by_pagination_count: int = 0
    final_chunks_count: int = 0
    doc_engine_score_used: bool = False

    all_chunks: list[ChunkDebugInfo] | None = None
    final_chunks: list[ChunkDebugInfo] | None = None

    def enable_detail(self):
        self.all_chunks = []
        self.final_chunks = []

    def to_dict(self) -> dict:
        result = {
            "query": self.query,
            "tenant_ids": self.tenant_ids,
            "kb_ids": self.kb_ids,
            "top_k": self.top_k,
            "top_n": self.top_n,
            "similarity_threshold": self.similarity_threshold,
            "vector_similarity_weight": self.vector_similarity_weight,
            "initial_search_count": self.initial_search_count,
            "pruned_count": self.pruned_count,
            "rerank_used": self.rerank_used,
            "rerank_model": self.rerank_model,
            "filtered_by_threshold_count": self.filtered_by_threshold_count,
            "filtered_by_pagination_count": self.filtered_by_pagination_count,
            "final_chunks_count": self.final_chunks_count,
            "doc_engine_score_used": self.doc_engine_score_used,
            "summary": {
                "selected": self.final_chunks_count,
                "pruned_deleted_docs": self.pruned_count,
                "filtered_by_threshold": self.filtered_by_threshold_count,
                "filtered_by_pagination": self.filtered_by_pagination_count,
            }
        }
        if self.all_chunks is not None:
            result["all_chunks"] = [c.to_dict() for c in self.all_chunks]
        if self.final_chunks is not None:
            result["final_chunks"] = [c.to_dict() for c in self.final_chunks]
        return result

    def log_summary(self):
        summary_lines = [
            "=" * 80,
            "RETRIEVAL DEBUG TRACE SUMMARY",
            "=" * 80,
            f"Query: {self.query}",
            f"Tenants: {self.tenant_ids}, KBs: {self.kb_ids}",
            f"Params: top_k={self.top_k}, top_n={self.top_n}, threshold={self.similarity_threshold}, vs_weight={self.vector_similarity_weight}",
            "-" * 80,
            f"Initial search results: {self.initial_search_count} chunks",
            f"Pruned (deleted docs): {self.pruned_count} chunks",
            f"Rerank used: {self.rerank_used} (model: {self.rerank_model})",
            f"Doc engine score used: {self.doc_engine_score_used}",
            f"Filtered by threshold: {self.filtered_by_threshold_count} chunks",
            f"Filtered by pagination: {self.filtered_by_pagination_count} chunks",
            f"Final selected: {self.final_chunks_count} chunks",
            "=" * 80,
        ]
        logging.info("\n".join(summary_lines))

        if self.final_chunks:
            logging.info("FINAL CHUNKS DETAIL:")
            for i, chunk in enumerate(self.final_chunks):
                logging.info(
                    f"  [{i}] ID={chunk.chunk_id}, "
                    f"doc={chunk.doc_name}, "
                    f"term_sim={chunk.term_similarity:.4f}, "
                    f"vec_sim={chunk.vector_similarity:.4f}, "
                    f"rerank={chunk.rerank_score:.4f}"
                )

        if self.all_chunks:
            filtered = [c for c in self.all_chunks if c.filter_reason]
            if filtered:
                logging.info("FILTERED CHUNKS:")
                for chunk in filtered[:20]:
                    logging.info(
                        f"  ID={chunk.chunk_id}, "
                        f"doc={chunk.doc_name}, "
                        f"reason={chunk.filter_reason}, "
                        f"scores: term={chunk.term_similarity:.4f}, vec={chunk.vector_similarity:.4f}, rerank={chunk.rerank_score:.4f}"
                    )
                if len(filtered) > 20:
                    logging.info(f"  ... and {len(filtered) - 20} more filtered chunks")


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

    async def get_vector(self, txt, emb_mdl, topk=10, similarity=0.1):
        qv, _ = await thread_pool_exec(emb_mdl.encode_queries, txt)
        shape = np.array(qv).shape
        if len(shape) > 1:
            raise Exception(
                f"Dealer.get_vector returned array's shape {shape} doesn't match expectation(exact one dimension).")
        embedding_data = [get_float(v) for v in qv]
        vector_column_name = f"q_{len(embedding_data)}_vec"
        return MatchDenseExpr(vector_column_name, embedding_data, 'float', 'cosine', topk, {"similarity": similarity})

    async def _existing_doc_ids(self, doc_ids: list[str]) -> set[str]:
        if not doc_ids:
            return set()

        unique_doc_ids = list(dict.fromkeys(doc_ids))

        def _load():
            from api.db.services.document_service import DocumentService

            return {row["id"] for row in DocumentService.get_by_ids(unique_doc_ids).dicts()}

        return await thread_pool_exec(_load)

    async def _prune_deleted_chunks(self, sres: SearchResult) -> SearchResult:
        # Temporary safety net:
        # Some delete paths can leave stale chunks in the doc store if the DB row
        # is removed but the vector record is not fully cleaned up. We filter those
        # chunks here so chat/retrieval does not surface content from deleted docs.
        # Keep this as a fallback, not as the primary delete mechanism.
        chunk_doc_ids = [chunk.get("doc_id") for chunk in sres.field.values() if chunk and chunk.get("doc_id")]
        if not chunk_doc_ids:
            return sres

        existing_doc_ids = await self._existing_doc_ids(chunk_doc_ids)
        if len(existing_doc_ids) == len(set(chunk_doc_ids)):
            return sres

        filtered_ids = []
        filtered_field = {}
        filtered_highlight = {} if sres.highlight else sres.highlight
        removed = 0

        for chunk_id in sres.ids:
            chunk = sres.field.get(chunk_id)
            if not chunk or chunk.get("doc_id") not in existing_doc_ids:
                removed += 1
                continue

            filtered_ids.append(chunk_id)
            filtered_field[chunk_id] = chunk
            if sres.highlight and chunk_id in sres.highlight:
                filtered_highlight[chunk_id] = sres.highlight[chunk_id]

        if removed:
            logging.warning("Pruned %s stale chunks whose documents no longer exist.", removed)

        return self.SearchResult(
            total=len(filtered_ids),
            ids=filtered_ids,
            query_vector=sres.query_vector,
            field=filtered_field,
            highlight=filtered_highlight,
            aggregation=sres.aggregation,
            keywords=sres.keywords,
            group_docs=sres.group_docs,
        )

    def get_filters(self, req):
        condition = dict()
        for key, field in {"kb_ids": "kb_id", "doc_ids": "doc_id"}.items():
            if key in req and req[key] is not None:
                condition[field] = req[key]
        # TODO(yzc): `available_int` is nullable however infinity doesn't support nullable columns.
        for key in ["knowledge_graph_kwd", "available_int", "entity_kwd", "from_entity_kwd", "to_entity_kwd",
                    "removed_kwd"]:
            if key in req and req[key] is not None:
                condition[key] = req[key]
        return condition

    async def search(self, req, idx_names: str | list[str],
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
                       "doc_id", "chunk_order_int", "page_num_int", "top_int", "create_timestamp_flt", "knowledge_graph_kwd",
                       "question_kwd", "question_tks", "doc_type_kwd",
                       "available_int", "content_with_weight", "mom_id", PAGERANK_FLD, TAG_FLD, "row_id()"])
        kwds = set([])

        qst = req.get("question", "")
        q_vec = []
        if not qst:
            if req.get("sort"):
                orderBy.asc("chunk_order_int")
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
                res = await thread_pool_exec(self.dataStore.search, src, highlightFields, filters, matchExprs, orderBy, offset, limit,
                                            idx_names, kb_ids, rank_feature=rank_feature)
                total = self.dataStore.get_total(res)
                logging.debug("Dealer.search TOTAL: {}".format(total))
            else:
                matchDense = await self.get_vector(qst, emb_mdl, topk, req.get("similarity", 0.1))
                q_vec = matchDense.embedding_data
                if not settings.DOC_ENGINE_INFINITY:
                    src.append(f"q_{len(q_vec)}_vec")

                fusionExpr = FusionExpr("weighted_sum", topk, {"weights": "0.05,0.95"})
                matchExprs = [matchText, matchDense, fusionExpr]

                res = await thread_pool_exec(self.dataStore.search, src, highlightFields, filters, matchExprs, orderBy, offset, limit,
                                            idx_names, kb_ids, rank_feature=rank_feature)
                total = self.dataStore.get_total(res)
                logging.debug("Dealer.search TOTAL: {}".format(total))

                # If result is empty, try again with lower min_match
                if total == 0:
                    if filters.get("doc_id"):
                        res = await thread_pool_exec(self.dataStore.search, src, [], filters, [], orderBy, offset, limit, idx_names, kb_ids)
                        total = self.dataStore.get_total(res)
                    else:
                        matchText, _ = self.qryr.question(qst, min_match=0.1)
                        matchDense.extra_options["similarity"] = 0.17
                        res = await thread_pool_exec(self.dataStore.search, src, highlightFields, filters, [matchText, matchDense, fusionExpr],
                                                    orderBy, offset, limit, idx_names, kb_ids,
                                                    rank_feature=rank_feature)
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
        ids = self.dataStore.get_doc_ids(res)
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
                    # Sentence boundary regex includes Arabic punctuation (، ؛ ؟ ۔)
                    pieces_.extend(
                        re.split(
                            r"([^\|][；。？!！،؛؟۔\n]|[a-z\u0600-\u06FF][.?;!،؛؟][ \n])",
                            pieces[i]))
                    i += 1
            pieces = pieces_
        else:
            # Sentence boundary regex includes Arabic punctuation (، ؛ ؟ ۔)
            pieces = re.split(r"([^\|][；。？!！،؛؟۔\n]|[a-z\u0600-\u06FF][.?;!،؛؟][ \n])", answer)
        for i in range(1, len(pieces)):
            if re.match(r"([^\|][；。？!！،؛؟۔\n]|[a-z\u0600-\u06FF][.?;!،؛؟][ \n])", pieces[i]):
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
                chunk_v[i] = [0.0] * len(ans_v[0])
                logging.warning(
                    "The dimension of query and chunk do not match: {} vs. {}".format(len(ans_v[0]), len(chunk_v[i])))

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

        q_denor = np.sqrt(np.sum([s * s for t, s in query_rfea.items() if t != PAGERANK_FLD]))
        if q_denor == 0:
            return np.array([0 for _ in range(len(search_res.ids))]) + pageranks
        for i in search_res.ids:
            nor, denor = 0, 0
            if not search_res.field[i].get(TAG_FLD):
                rank_fea.append(0)
                continue
            tag_feas = parse_tag_features(search_res.field[i].get(TAG_FLD), allow_json_string=True, allow_python_literal=True)
            if not tag_feas:
                rank_fea.append(0)
                continue
            for t, sc in tag_feas.items():
                if t in query_rfea:
                    nor += query_rfea[t] * sc
                denor += sc * sc
            if denor == 0:
                rank_fea.append(0)
            else:
                rank_fea.append(nor / np.sqrt(denor) / q_denor)
        return np.array(rank_fea) * 10. + pageranks

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
        print(f"[DEBUG rerank_by_model] query={query}, tkweight={tkweight}, vtweight={vtweight}")
        _, keywords = self.qryr.question(query)
        print(f"[DEBUG rerank_by_model] keywords={keywords}")

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
            print(f"[DEBUG rerank_by_model] chunk id={i}, content_ltks={len(content_ltks)}, title_tks={len(title_tks)}, important_kwd={len(important_kwd)}")
            doc_text = remove_redundant_spaces(" ".join(tks))
            if len(doc_text) > 100:
                print(f"[DEBUG rerank_by_model] chunk id={i}, doc_text (first 100)={doc_text[:100]}...")
            else:
                print(f"[DEBUG rerank_by_model] chunk id={i}, doc_text={doc_text}")

        docs = [remove_redundant_spaces(" ".join(tks)) for tks in ins_tw]
        print(f"[DEBUG rerank_by_model] docs sent to reranker: {len(docs)} docs")
        for idx, doc in enumerate(docs[:2]):  # Print first 2
            print(f"[DEBUG rerank_by_model] doc[{idx}] len={len(doc)}, full={doc}")
            if len(doc) > 100:
                print(f"[DEBUG rerank_by_model] doc[{idx}] (first 100)={doc[:100]}...")
            else:
                print(f"[DEBUG rerank_by_model] doc[{idx}]={doc}")

        tksim = self.qryr.token_similarity(keywords, ins_tw)
        print(f"[DEBUG rerank_by_model] tksim={tksim}")
        vtsim, _ = rerank_mdl.similarity(query, docs)
        print(f"[DEBUG rerank_by_model] vtsim from reranker={vtsim}")
        ## For rank feature(tag_fea) scores.
        rank_fea = self._rank_feature_scores(rank_feature, sres)
        print(f"[DEBUG rerank_by_model] rank_fea={rank_fea}")

        return tkweight * np.array(tksim) + vtweight * vtsim + rank_fea, tksim, vtsim

    def hybrid_similarity(self, ans_embd, ins_embd, ans, inst):
        return self.qryr.hybrid_similarity(ans_embd,
                                           ins_embd,
                                           rag_tokenizer.tokenize(ans).split(),
                                           rag_tokenizer.tokenize(inst).split())

    async def retrieval(
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
            debug: bool = False,
    ):
        ranks = {"total": 0, "chunks": [], "doc_aggs": {}}
        if not question:
            return ranks

        debug_trace = None
        if debug or RETRIEVAL_DEBUG_TRACE_ENABLED:
            debug_trace = RetrievalDebugTrace(
                query=question,
                tenant_ids=list(tenant_ids) if isinstance(tenant_ids, (list, str)) else [],
                kb_ids=list(kb_ids) if isinstance(kb_ids, list) else [],
                top_k=top,
                top_n=page_size,
                similarity_threshold=similarity_threshold,
                vector_similarity_weight=vector_similarity_weight,
            )
            if debug:
                debug_trace.enable_detail()

        # Keep the historical windowing strategy by default, but when an external
        # reranker is enabled cap candidate count by both top_k and provider-safe 64.
        RERANK_LIMIT = math.ceil(64 / page_size) * page_size if page_size > 1 else 1
        RERANK_LIMIT = max(30, RERANK_LIMIT)
        if rerank_mdl and top > 0:
            RERANK_LIMIT = min(RERANK_LIMIT, top, 64)
        page = max(page, 1)
        global_offset = (page - 1) * page_size
        req = {
            "kb_ids": kb_ids,
            "doc_ids": doc_ids,
            "page": global_offset // RERANK_LIMIT + 1,
            "size": RERANK_LIMIT,
            "question": question,
            "vector": True,
            "topk": top,
            "similarity": similarity_threshold,
            "available_int": 1,
        }
        logging.debug(f"[Search] global_offset={global_offset}, rerank_limit={RERANK_LIMIT}, page_size={page_size}, page={page}")

        if isinstance(tenant_ids, str):
            tenant_ids = tenant_ids.split(",")

        sres = await self.search(req, [index_name(tid) for tid in tenant_ids], kb_ids, embd_mdl, highlight,
                           rank_feature=rank_feature)
        
        if debug_trace:
            debug_trace.initial_search_count = len(sres.ids) if sres.ids else 0
        
        # Temporary retrieval-side guard: prune chunks whose parent document no
        # longer exists before reranking and returning results.
        original_count = sres.total
        sres = await self._prune_deleted_chunks(sres)
        
        if debug_trace and original_count > sres.total:
            debug_trace.pruned_count = original_count - sres.total
        
        if sres.total == 0:
            ranks["doc_aggs"] = []
            if debug_trace and debug:
                if RETRIEVAL_DEBUG_TRACE_ENABLED or debug:
                    debug_trace.log_summary()
                ranks["debug_trace"] = debug_trace.to_dict()
            return ranks

        if rerank_mdl and sres.total > 0:
            if debug_trace:
                debug_trace.rerank_used = True
                debug_trace.rerank_model = getattr(rerank_mdl, "llm_name", str(rerank_mdl))
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
                if debug_trace:
                    debug_trace.doc_engine_score_used = True
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
            if debug_trace and debug:
                if RETRIEVAL_DEBUG_TRACE_ENABLED or debug:
                    debug_trace.log_summary()
                ranks["debug_trace"] = debug_trace.to_dict()
            return ranks

        sorted_idx = np.argsort(sim_np * -1)

        # When vector_similarity_weight is 0, similarity_threshold is not meaningful for term-only scores.
        post_threshold = 0.0 if vector_similarity_weight <= 0 else similarity_threshold

        # When doc_ids is explicitly provided (metadata or document filtering), bypass threshold
        # User wants those specific documents regardless of their relevance score
        if doc_ids:
            post_threshold = 0.0

        valid_idx = [int(i) for i in sorted_idx if sim_np[i] >= post_threshold]
        filtered_count = len(valid_idx)
        ranks["total"] = int(filtered_count)

        if debug_trace:
            debug_trace.filtered_by_threshold_count = len(sorted_idx) - len(valid_idx)

        if filtered_count == 0:
            ranks["doc_aggs"] = []
            if debug_trace and debug:
                if RETRIEVAL_DEBUG_TRACE_ENABLED or debug:
                    debug_trace.log_summary()
                ranks["debug_trace"] = debug_trace.to_dict()
            return ranks

        begin = global_offset % RERANK_LIMIT
        end = begin + page_size
        page_idx = valid_idx[begin:end]

        if debug_trace:
            debug_trace.filtered_by_pagination_count = len(valid_idx) - len(page_idx)
            debug_trace.final_chunks_count = len(page_idx)

        dim = len(sres.query_vector)
        vector_column = f"q_{dim}_vec"
        zero_vector = [0.0] * dim

        for pos_in_page, i in enumerate(page_idx):
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
                "tag_kwd": chunk.get("tag_kwd", []),
                "image_id": chunk.get("img_id", ""),
                "similarity": float(sim_np[i]),
                "vector_similarity": float(vsim[i]),
                "term_similarity": float(tsim[i]),
                "vector": chunk.get(vector_column, zero_vector),
                "positions": position_int,
                "doc_type_kwd": chunk.get("doc_type_kwd", ""),
                "mom_id": chunk.get("mom_id", ""),
                "row_id": chunk.get("row_id()"),
            }
            if highlight and sres.highlight:
                if id in sres.highlight:
                    d["highlight"] = remove_redundant_spaces(sres.highlight[id])
                else:
                    d["highlight"] = d["content_with_weight"]
            ranks["chunks"].append(d)

            if debug_trace and debug_trace.final_chunks is not None:
                chunk_debug = ChunkDebugInfo(
                    chunk_id=id,
                    doc_id=did,
                    doc_name=dnm,
                    kb_id=chunk.get("kb_id", ""),
                    initial_score=float(chunk.get("_score", 0.0)),
                    term_similarity=float(tsim[i]),
                    vector_similarity=float(vsim[i]),
                    rerank_score=float(sim_np[i]),
                    final_position=pos_in_page,
                    content_preview=chunk.get("content_ltks", "")[:200],
                    is_pruned=False,
                )
                debug_trace.final_chunks.append(chunk_debug)

        if debug_trace and debug_trace.all_chunks is not None:
            for sorted_pos, i in enumerate(sorted_idx):
                id = sres.ids[i]
                chunk = sres.field[id]
                dnm = chunk.get("docnm_kwd", "")
                did = chunk.get("doc_id", "")

                filter_reason = None
                final_pos = None

                if i not in valid_idx:
                    filter_reason = "threshold"
                elif sorted_pos < begin or sorted_pos >= begin + page_size:
                    filter_reason = "pagination"
                else:
                    final_pos = sorted_pos - begin

                chunk_debug = ChunkDebugInfo(
                    chunk_id=id,
                    doc_id=did,
                    doc_name=dnm,
                    kb_id=chunk.get("kb_id", ""),
                    initial_score=float(chunk.get("_score", 0.0)),
                    term_similarity=float(tsim[i]),
                    vector_similarity=float(vsim[i]),
                    rerank_score=float(sim_np[i]),
                    filter_reason=filter_reason,
                    final_position=final_pos,
                    content_preview=chunk.get("content_ltks", "")[:200],
                    is_pruned=False,
                )
                debug_trace.all_chunks.append(chunk_debug)

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

        if debug_trace:
            if RETRIEVAL_DEBUG_TRACE_ENABLED or debug:
                debug_trace.log_summary()
            if debug:
                ranks["debug_trace"] = debug_trace.to_dict()

        return ranks

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
            limit = min(bs, max_count - p)
            if limit <= 0:
                break
            es_res = self.dataStore.search(fields, [], condition, [], orderBy, p, limit, index_name(tenant_id),
                                           kb_ids)
            dict_chunks = self.dataStore.get_fields(es_res, fields)
            for id, doc in dict_chunks.items():
                doc["id"] = id
            if dict_chunks:
                res.extend(dict_chunks.values())
            chunk_count = len(dict_chunks)
            if chunk_count == 0 or chunk_count < limit:
                break
        return res

    def all_tags(self, tenant_id: str, kb_ids: list[str], S=1000):
        if not self.dataStore.index_exist(index_name(tenant_id), kb_ids[0]):
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
        match_txt = self.qryr.paragraph(doc["title_tks"] + " " + doc["content_ltks"], doc.get("important_kwd", []),
                                        keywords_topn)
        res = self.dataStore.search([], [], {}, [match_txt], OrderByExpr(), 0, 0, idx_nm, kb_ids, ["tag_kwd"])
        aggs = self.dataStore.get_aggregation(res, "tag_kwd")
        if not aggs:
            return False
        cnt = np.sum([c for _, c in aggs])
        tag_fea = sorted([(a, round(0.1 * (c + 1) / (cnt + S) / max(1e-6, all_tags.get(a, 0.0001)))) for a, c in aggs],
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
        tag_fea = sorted([(a, round(0.1 * (c + 1) / (cnt + S) / max(1e-6, all_tags.get(a, 0.0001)))) for a, c in aggs],
                         key=lambda x: x[1] * -1)[:topn_tags]
        return {a.replace(".", "_"): max(1, c) for a, c in tag_fea}

    async def retrieval_by_toc(self, query: str, chunks: list[dict], tenant_ids: list[str], chat_mdl, topn: int = 6):
        from rag.prompts.generator import relevant_chunks_with_toc # moved from the top of the file to avoid circular import
        if not chunks:
            return []
        idx_nms = [index_name(tid) for tid in tenant_ids]
        ranks, doc_id2kb_id = {}, {}
        for ck in chunks:
            if ck["doc_id"] not in ranks:
                ranks[ck["doc_id"]] = 0
            ranks[ck["doc_id"]] += ck["similarity"]
            doc_id2kb_id[ck["doc_id"]] = ck["kb_id"]
        doc_id = sorted(ranks.items(), key=lambda x: x[1] * -1.)[0][0]
        kb_ids = [doc_id2kb_id[doc_id]]
        es_res = self.dataStore.search(["content_with_weight"], [], {"doc_id": doc_id, "toc_kwd": "toc"}, [],
                                       OrderByExpr(), 0, 128, idx_nms,
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

        ids = await relevant_chunks_with_toc(query, toc, chat_mdl, topn * 2)
        if not ids:
            return chunks

        vector_size = 1024
        id2idx = {ck["chunk_id"]: i for i, ck in enumerate(chunks)}
        for cid, sim in ids:
            if cid in id2idx:
                chunks[id2idx[cid]]["similarity"] += sim
                continue
            chunk = self.dataStore.get(cid, idx_nms[0], kb_ids)
            if not chunk:
                continue
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

        return sorted(chunks, key=lambda x: x["similarity"] * -1)[:topn]

    def retrieval_by_children(self, chunks: list[dict], tenant_ids: list[str]):
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
            chunk = self.dataStore.get(id, idx_nms[0], [ck["kb_id"] for ck in cks])
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

        return sorted(chunks, key=lambda x: x["similarity"] * -1)
