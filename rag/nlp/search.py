# -*- coding: utf-8 -*-
import json
import re
from elasticsearch_dsl import Q, Search, A
from typing import List, Optional, Dict, Union
from dataclasses import dataclass

from rag.settings import es_logger
from rag.utils import rmSpace
from rag.nlp import huqie, query
import numpy as np


def index_name(uid): return f"ragflow_{uid}"


class Dealer:
    def __init__(self, es):
        self.qryr = query.EsQueryer(es)
        self.qryr.flds = [
            "title_tks^10",
            "title_sm_tks^5",
            "important_kwd^30",
            "important_tks^20",
            "content_ltks^2",
            "content_sm_ltks"]
        self.es = es

    @dataclass
    class SearchResult:
        total: int
        ids: List[str]
        query_vector: List[float] = None
        field: Optional[Dict] = None
        highlight: Optional[Dict] = None
        aggregation: Union[List, Dict, None] = None
        keywords: Optional[List[str]] = None
        group_docs: List[List] = None

    def _vector(self, txt, emb_mdl, sim=0.8, topk=10):
        qv, c = emb_mdl.encode_queries(txt)
        return {
            "field": "q_%d_vec" % len(qv),
            "k": topk,
            "similarity": sim,
            "num_candidates": topk * 2,
            "query_vector": qv
        }

    def search(self, req, idxnm, emb_mdl=None):
        qst = req.get("question", "")
        bqry, keywords = self.qryr.question(qst)
        if req.get("kb_ids"):
            bqry.filter.append(Q("terms", kb_id=req["kb_ids"]))
        if req.get("doc_ids"):
            bqry.filter.append(Q("terms", doc_id=req["doc_ids"]))
        if "available_int" in req:
            if req["available_int"] == 0:
                bqry.filter.append(Q("range", available_int={"lt": 1}))
            else:
                bqry.filter.append(Q("bool", must_not=Q("range", available_int={"lt": 1})))
        bqry.boost = 0.05

        s = Search()
        pg = int(req.get("page", 1)) - 1
        ps = int(req.get("size", 1000))
        src = req.get("fields", ["docnm_kwd", "content_ltks", "kb_id", "img_id",
                                 "image_id", "doc_id", "q_512_vec", "q_768_vec",
                                 "q_1024_vec", "q_1536_vec", "available_int"])

        s = s.query(bqry)[pg * ps:(pg + 1) * ps]
        s = s.highlight("content_ltks")
        s = s.highlight("title_ltks")
        if not qst:
            s = s.sort(
                {"create_time": {"order": "desc", "unmapped_type": "date"}})

        if qst:
            s = s.highlight_options(
                fragment_size=120,
                number_of_fragments=5,
                boundary_scanner_locale="zh-CN",
                boundary_scanner="SENTENCE",
                boundary_chars=",./;:\\!()，。？：！……（）——、"
            )
        s = s.to_dict()
        q_vec = []
        if req.get("vector"):
            assert emb_mdl, "No embedding model selected"
            s["knn"] = self._vector(qst, emb_mdl, req.get("similarity", 0.4), ps)
            s["knn"]["filter"] = bqry.to_dict()
            if "highlight" in s: del s["highlight"]
            q_vec = s["knn"]["query_vector"]
        es_logger.info("【Q】: {}".format(json.dumps(s)))
        res = self.es.search(s, idxnm=idxnm, timeout="600s", src=src)
        es_logger.info("TOTAL: {}".format(self.es.getTotal(res)))
        if self.es.getTotal(res) == 0 and "knn" in s:
            bqry, _ = self.qryr.question(qst, min_match="10%")
            if req.get("kb_ids"):
                bqry.filter.append(Q("terms", kb_id=req["kb_ids"]))
            s["query"] = bqry.to_dict()
            s["knn"]["filter"] = bqry.to_dict()
            s["knn"]["similarity"] = 0.7
            res = self.es.search(s, idxnm=idxnm, timeout="600s", src=src)

        kwds = set([])
        for k in keywords:
            kwds.add(k)
            for kk in huqie.qieqie(k).split(" "):
                if len(kk) < 2:
                    continue
                if kk in kwds:
                    continue
                kwds.add(kk)

        aggs = self.getAggregation(res, "docnm_kwd")

        return self.SearchResult(
            total=self.es.getTotal(res),
            ids=self.es.getDocIds(res),
            query_vector=q_vec,
            aggregation=aggs,
            highlight=self.getHighlight(res),
            field=self.getFields(res, src),
            keywords=list(kwds)
        )

    def getAggregation(self, res, g):
        if not "aggregations" in res or "aggs_" + g not in res["aggregations"]:
            return
        bkts = res["aggregations"]["aggs_" + g]["buckets"]
        return [(b["key"], b["doc_count"]) for b in bkts]

    def getHighlight(self, res):
        def rmspace(line):
            eng = set(list("qwertyuioplkjhgfdsazxcvbnm"))
            r = []
            for t in line.split(" "):
                if not t:
                    continue
                if len(r) > 0 and len(
                        t) > 0 and r[-1][-1] in eng and t[0] in eng:
                    r.append(" ")
                r.append(t)
            r = "".join(r)
            return r

        ans = {}
        for d in res["hits"]["hits"]:
            hlts = d.get("highlight")
            if not hlts:
                continue
            ans[d["_id"]] = "".join([a for a in list(hlts.items())[0][1]])
        return ans

    def getFields(self, sres, flds):
        res = {}
        if not flds:
            return {}
        for d in self.es.getSource(sres):
            m = {n: d.get(n) for n in flds if d.get(n) is not None}
            for n, v in m.items():
                if isinstance(v, type([])):
                    m[n] = "\t".join([str(vv) for vv in v])
                    continue
                if not isinstance(v, type("")):
                    m[n] = str(m[n])
                m[n] = rmSpace(m[n])

            if m:
                res[d["id"]] = m
        return res

    @staticmethod
    def trans2floats(txt):
        return [float(t) for t in txt.split("\t")]

    def insert_citations(self, answer, chunks, chunk_v, embd_mdl, tkweight=0.3, vtweight=0.7):
        pieces = re.split(r"([；。？!！\n]|[a-z][.?;!][ \n])", answer)
        for i in range(1, len(pieces)):
            if re.match(r"[a-z][.?;!][ \n]", pieces[i]):
                pieces[i - 1] += pieces[i][0]
                pieces[i] = pieces[i][1:]
        idx = []
        pieces_ = []
        for i, t in enumerate(pieces):
            if len(t) < 5: continue
            idx.append(i)
            pieces_.append(t)
        if not pieces_: return answer

        ans_v = embd_mdl.encode(pieces_)
        assert len(ans_v[0]) == len(chunk_v[0]), "The dimension of query and chunk do not match: {} vs. {}".format(
            len(ans_v[0]), len(chunk_v[0]))

        chunks_tks = [huqie.qie(ck).split(" ") for ck in chunks]
        cites = {}
        for i,a in enumerate(pieces_):
            sim, tksim, vtsim = self.qryr.hybrid_similarity(ans_v[i],
                                                            chunk_v,
                                                            huqie.qie(pieces_[i]).split(" "),
                                                            chunks_tks,
                                                            tkweight, vtweight)
            mx = np.max(sim) * 0.99
            if mx < 0.55: continue
            cites[idx[i]] = list(set([str(i) for i in range(len(chunk_v)) if sim[i] > mx]))[:4]

        res = ""
        for i,p in enumerate(pieces):
            res += p
            if i not in idx:continue
            if i not in cites:continue
            res += "##%s$$"%"$".join(cites[i])

        return res

    def rerank(self, sres, query, tkweight=0.3, vtweight=0.7, cfield="content_ltks"):
        ins_embd = [
            Dealer.trans2floats(
                sres.field[i]["q_%d_vec" % len(sres.query_vector)]) for i in sres.ids]
        if not ins_embd:
            return []
        ins_tw = [huqie.qie(sres.field[i][cfield]).split(" ") for i in sres.ids]
        sim, tksim, vtsim = self.qryr.hybrid_similarity(sres.query_vector,
                                                        ins_embd,
                                                        huqie.qie(query).split(" "),
                                                        ins_tw, tkweight, vtweight)
        return sim, tksim, vtsim

    def hybrid_similarity(self, ans_embd, ins_embd, ans, inst):
        return self.qryr.hybrid_similarity(ans_embd,
                                           ins_embd,
                                           huqie.qie(ans).split(" "),
                                           huqie.qie(inst).split(" "))

    def retrieval(self, question, embd_mdl, tenant_id, kb_ids, page, page_size, similarity_threshold=0.2,
                  vector_similarity_weight=0.3, top=1024, doc_ids=None, aggs=True):
        req = {"kb_ids": kb_ids, "doc_ids": doc_ids, "size": top,
               "question": question, "vector": True,
               "similarity": similarity_threshold}
        sres = self.search(req, index_name(tenant_id), embd_mdl)

        sim, tsim, vsim = self.rerank(
            sres, question, 1 - vector_similarity_weight, vector_similarity_weight)
        idx = np.argsort(sim * -1)
        ranks = {"total": 0, "chunks": [], "doc_aggs": {}}
        dim = len(sres.query_vector)
        start_idx = (page - 1) * page_size
        for i in idx:
            ranks["total"] += 1
            if sim[i] < similarity_threshold:
                break
            start_idx -= 1
            if start_idx >= 0:
                continue
            if len(ranks["chunks"]) == page_size:
                if aggs:
                    continue
                break
            id = sres.ids[i]
            dnm = sres.field[id]["docnm_kwd"]
            d = {
                "chunk_id": id,
                "content_ltks": sres.field[id]["content_ltks"],
                "doc_id": sres.field[id]["doc_id"],
                "docnm_kwd": dnm,
                "kb_id": sres.field[id]["kb_id"],
                "important_kwd": sres.field[id].get("important_kwd", []),
                "img_id": sres.field[id].get("img_id", ""),
                "similarity": sim[i],
                "vector_similarity": vsim[i],
                "term_similarity": tsim[i],
                "vector": self.trans2floats(sres.field[id].get("q_%d_vec" % dim, "\t".join(["0"] * dim)))
            }
            ranks["chunks"].append(d)
            if dnm not in ranks["doc_aggs"]:
                ranks["doc_aggs"][dnm] = 0
            ranks["doc_aggs"][dnm] += 1

        return ranks
