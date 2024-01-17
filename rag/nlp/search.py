# -*- coding: utf-8 -*-
import json
import re
from elasticsearch_dsl import Q, Search, A
from typing import List, Optional, Tuple, Dict, Union
from dataclasses import dataclass

from rag.settings import es_logger
from rag.utils import rmSpace
from rag.nlp import huqie, query
import numpy as np


def index_name(uid): return f"ragflow_{uid}"


class Dealer:
    def __init__(self, es, emb_mdl):
        self.qryr = query.EsQueryer(es)
        self.qryr.flds = [
            "title_tks^10",
            "title_sm_tks^5",
            "content_ltks^2",
            "content_sm_ltks"]
        self.es = es
        self.emb_mdl = emb_mdl

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

    def _vector(self, txt, sim=0.8, topk=10):
        qv = self.emb_mdl.encode_queries(txt)
        return {
            "field": "q_%d_vec"%len(qv),
            "k": topk,
            "similarity": sim,
            "num_candidates": 1000,
            "query_vector": qv
        }

    def search(self, req, idxnm, tks_num=3):
        qst = req.get("question", "")
        bqry, keywords = self.qryr.question(qst)
        if req.get("kb_ids"):
            bqry.filter.append(Q("terms", kb_id=req["kb_ids"]))
        if req.get("doc_ids"):
            bqry.filter.append(Q("terms", doc_id=req["doc_ids"]))
        bqry.boost = 0.05

        s = Search()
        pg = int(req.get("page", 1)) - 1
        ps = int(req.get("size", 1000))
        src = req.get("fields", ["docnm_kwd", "content_ltks", "kb_id","img_id",
                                "image_id", "doc_id", "q_512_vec", "q_768_vec",
                                "q_1024_vec", "q_1536_vec"])

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
            s["knn"] = self._vector(qst, req.get("similarity", 0.4), ps)
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

    def insert_citations(self, ans, top_idx, sres,
                         vfield="q_vec", cfield="content_ltks"):

        ins_embd = [Dealer.trans2floats(
            sres.field[sres.ids[i]][vfield]) for i in top_idx]
        ins_tw = [sres.field[sres.ids[i]][cfield].split(" ") for i in top_idx]
        s = 0
        e = 0
        res = ""

        def citeit():
            nonlocal s, e, ans, res
            if not ins_embd:
                return
            embd = self.emb_mdl.encode(ans[s: e])
            sim = self.qryr.hybrid_similarity(embd,
                                              ins_embd,
                                              huqie.qie(ans[s:e]).split(" "),
                                              ins_tw)
            print(ans[s: e], sim)
            mx = np.max(sim) * 0.99
            if mx < 0.55:
                return
            cita = list(set([top_idx[i]
                        for i in range(len(ins_embd)) if sim[i] > mx]))[:4]
            for i in cita:
                res += f"@?{i}?@"

            return cita

        punct = set("；。？!！")
        if not self.qryr.isChinese(ans):
            punct.add("?")
            punct.add(".")
        while e < len(ans):
            if e - s < 12 or ans[e] not in punct:
                e += 1
                continue
            if ans[e] == "." and e + \
                    1 < len(ans) and re.match(r"[0-9]", ans[e + 1]):
                e += 1
                continue
            if ans[e] == "." and e - 2 >= 0 and ans[e - 2] == "\n":
                e += 1
                continue
            res += ans[s: e]
            citeit()
            res += ans[e]
            e += 1
            s = e

        if s < len(ans):
            res += ans[s:]
            citeit()

        return res

    def rerank(self, sres, query, tkweight=0.3, vtweight=0.7,
               vfield="q_vec", cfield="content_ltks"):
        ins_embd = [
            Dealer.trans2floats(
                sres.field[i]["q_vec"]) for i in sres.ids]
        if not ins_embd:
            return []
        ins_tw = [sres.field[i][cfield].split(" ") for i in sres.ids]
        # return CosineSimilarity([sres.query_vector], ins_embd)[0]
        sim = self.qryr.hybrid_similarity(sres.query_vector,
                                          ins_embd,
                                          huqie.qie(query).split(" "),
                                          ins_tw, tkweight, vtweight)
        return sim



