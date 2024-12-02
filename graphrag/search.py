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
from copy import deepcopy

import pandas as pd
from rag.utils.doc_store_conn import OrderByExpr, FusionExpr

from rag.nlp.search import Dealer


class KGSearch(Dealer):
    def search(self, req, idxnm: str | list[str], kb_ids: list[str], emb_mdl=None, highlight=False):
        def merge_into_first(sres, title="") -> dict[str, str]:
            if not sres:
                return {}
            content_with_weight = ""
            df, texts = [],[]
            for d in sres.values():
                try:
                    df.append(json.loads(d["content_with_weight"]))
                except Exception:
                    texts.append(d["content_with_weight"])
            if df:
                content_with_weight = title + "\n" + pd.DataFrame(df).to_csv()
            else:
                content_with_weight = title + "\n" + "\n".join(texts)
            first_id = ""
            first_source = {}
            for k, v in sres.items():
                first_id = id
                first_source = deepcopy(v)
                break
            first_source["content_with_weight"] = content_with_weight
            first_id = next(iter(sres))
            return {first_id: first_source}

        qst = req.get("question", "")
        matchText, keywords = self.qryr.question(qst, min_match=0.05)
        condition = self.get_filters(req)

        ## Entity retrieval
        condition.update({"knowledge_graph_kwd": ["entity"]})
        assert emb_mdl, "No embedding model selected"
        matchDense = self.get_vector(qst, emb_mdl, 1024, req.get("similarity", 0.1))
        q_vec = matchDense.embedding_data
        src = req.get("fields", ["docnm_kwd", "content_ltks", "kb_id", "img_id", "title_tks", "important_kwd",
                                 "doc_id", f"q_{len(q_vec)}_vec", "position_int", "name_kwd",
                                 "available_int", "content_with_weight",
                                 "weight_int", "weight_flt"
                                 ])

        fusionExpr = FusionExpr("weighted_sum", 32, {"weights": "0.5, 0.5"})

        ent_res = self.dataStore.search(src, list(), condition, [matchText, matchDense, fusionExpr], OrderByExpr(), 0, 32, idxnm, kb_ids)
        ent_res_fields = self.dataStore.getFields(ent_res, src)
        entities = [d.get("name_kwd") for d in ent_res_fields.values() if d.get("name_kwd")]
        ent_ids = self.dataStore.getChunkIds(ent_res)
        ent_content = merge_into_first(ent_res_fields, "-Entities-")
        if ent_content:
            ent_ids = list(ent_content.keys())

        ## Community retrieval
        condition = self.get_filters(req)
        condition.update({"entities_kwd": entities, "knowledge_graph_kwd": ["community_report"]})
        comm_res = self.dataStore.search(src, list(), condition, [matchText, matchDense, fusionExpr], OrderByExpr(), 0, 32, idxnm, kb_ids)
        comm_res_fields = self.dataStore.getFields(comm_res, src)
        comm_ids = self.dataStore.getChunkIds(comm_res)
        comm_content = merge_into_first(comm_res_fields, "-Community Report-")
        if comm_content:
            comm_ids = list(comm_content.keys())

        ## Text content retrieval
        condition = self.get_filters(req)
        condition.update({"knowledge_graph_kwd": ["text"]})
        txt_res = self.dataStore.search(src, list(), condition, [matchText, matchDense, fusionExpr], OrderByExpr(), 0, 6, idxnm, kb_ids)
        txt_res_fields = self.dataStore.getFields(txt_res, src)
        txt_ids = self.dataStore.getChunkIds(txt_res)
        txt_content = merge_into_first(txt_res_fields, "-Original Content-")
        if txt_content:
            txt_ids = list(txt_content.keys())

        return self.SearchResult(
            total=len(ent_ids) + len(comm_ids) + len(txt_ids),
            ids=[*ent_ids, *comm_ids, *txt_ids],
            query_vector=q_vec,
            highlight=None,
            field={**ent_content, **comm_content, **txt_content},
            keywords=[]
        )
