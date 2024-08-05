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
from elasticsearch_dsl import Q, Search

from rag.nlp.search import Dealer


class KGSearch(Dealer):
    def search(self, req, idxnm, emb_mdl=None):
        def merge_into_first(sres, title=""):
            df,texts = [],[]
            for d in sres["hits"]["hits"]:
                try:
                    df.append(json.loads(d["_source"]["content_with_weight"]))
                except Exception as e:
                    texts.append(d["_source"]["content_with_weight"])
                    pass
            if not df and not texts: return False
            if df:
                try:
                    sres["hits"]["hits"][0]["_source"]["content_with_weight"] = title + "\n" + pd.DataFrame(df).to_csv()
                except Exception as e:
                    pass
            else:
                sres["hits"]["hits"][0]["_source"]["content_with_weight"] = title + "\n" + "\n".join(texts)
            return True

        src = req.get("fields", ["docnm_kwd", "content_ltks", "kb_id", "img_id", "title_tks", "important_kwd",
                                 "image_id", "doc_id", "q_512_vec", "q_768_vec", "position_int", "name_kwd",
                                 "q_1024_vec", "q_1536_vec", "available_int", "content_with_weight",
                                 "weight_int", "weight_flt", "rank_int"
                                 ])

        qst = req.get("question", "")
        binary_query, keywords = self.qryr.question(qst, min_match="5%")
        binary_query = self._add_filters(binary_query, req)

        ## Entity retrieval
        bqry = deepcopy(binary_query)
        bqry.filter.append(Q("terms", knowledge_graph_kwd=["entity"]))
        s = Search()
        s = s.query(bqry)[0: 32]

        s = s.to_dict()
        q_vec = []
        if req.get("vector"):
            assert emb_mdl, "No embedding model selected"
            s["knn"] = self._vector(
                qst, emb_mdl, req.get(
                    "similarity", 0.1), 1024)
            s["knn"]["filter"] = bqry.to_dict()
            q_vec = s["knn"]["query_vector"]

        ent_res = self.es.search(deepcopy(s), idxnm=idxnm, timeout="600s", src=src)
        entities = [d["name_kwd"] for d in self.es.getSource(ent_res)]
        ent_ids = self.es.getDocIds(ent_res)
        if merge_into_first(ent_res, "-Entities-"):
            ent_ids = ent_ids[0:1]

        ## Community retrieval
        bqry = deepcopy(binary_query)
        bqry.filter.append(Q("terms", entities_kwd=entities))
        bqry.filter.append(Q("terms", knowledge_graph_kwd=["community_report"]))
        s = Search()
        s = s.query(bqry)[0: 32]
        s = s.to_dict()
        comm_res = self.es.search(deepcopy(s), idxnm=idxnm, timeout="600s", src=src)
        comm_ids = self.es.getDocIds(comm_res)
        if merge_into_first(comm_res, "-Community Report-"):
            comm_ids = comm_ids[0:1]

        ## Text content retrieval
        bqry = deepcopy(binary_query)
        bqry.filter.append(Q("terms", knowledge_graph_kwd=["text"]))
        s = Search()
        s = s.query(bqry)[0: 6]
        s = s.to_dict()
        txt_res = self.es.search(deepcopy(s), idxnm=idxnm, timeout="600s", src=src)
        txt_ids = self.es.getDocIds(comm_res)
        if merge_into_first(txt_res, "-Original Content-"):
            txt_ids = comm_ids[0:1]

        return self.SearchResult(
            total=len(ent_ids) + len(comm_ids) + len(txt_ids),
            ids=[*ent_ids, *comm_ids, *txt_ids],
            query_vector=q_vec,
            aggregation=None,
            highlight=None,
            field={**self.getFields(ent_res, src), **self.getFields(comm_res, src), **self.getFields(txt_res, src)},
            keywords=[]
        )

