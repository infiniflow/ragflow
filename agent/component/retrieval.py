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
import logging
from abc import ABC

import pandas as pd

from api.db import LLMType
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from api import settings
from agent.component.base import ComponentBase, ComponentParamBase
from rag.app.tag import label_question
from rag.utils.tavily_conn import Tavily


class RetrievalParam(ComponentParamBase):

    """
    Define the Retrieval component parameters.
    """
    def __init__(self):
        super().__init__()
        self.similarity_threshold = 0.2
        self.keywords_similarity_weight = 0.5
        self.top_n = 8
        self.top_k = 1024
        self.kb_ids = []
        self.rerank_id = ""
        self.empty_response = ""
        self.tavily_api_key = ""
        self.use_kg = False

    def check(self):
        self.check_decimal_float(self.similarity_threshold, "[Retrieval] Similarity threshold")
        self.check_decimal_float(self.keywords_similarity_weight, "[Retrieval] Keyword similarity weight")
        self.check_positive_number(self.top_n, "[Retrieval] Top N")


class Retrieval(ComponentBase, ABC):
    component_name = "Retrieval"

    def _run(self, history, **kwargs):
        query = self.get_input()
        query = str(query["content"][0]) if "content" in query else ""
        lines = query.split('\n')
        query = lines[-1] if lines else ""
        kbs = KnowledgebaseService.get_by_ids(self._param.kb_ids)
        if not kbs:
            return Retrieval.be_output("")

        embd_nms = list(set([kb.embd_id for kb in kbs]))
        assert len(embd_nms) == 1, "Knowledge bases use different embedding models."

        embd_mdl = None
        if embd_nms:
            embd_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.EMBEDDING, embd_nms[0])
            self._canvas.set_embedding_model(embd_nms[0])

        rerank_mdl = None
        if self._param.rerank_id:
            rerank_mdl = LLMBundle(kbs[0].tenant_id, LLMType.RERANK, self._param.rerank_id)

        if kbs:
            kbinfos = settings.retrievaler.retrieval(query, embd_mdl, kbs[0].tenant_id, self._param.kb_ids,
                                        1, self._param.top_n,
                                        self._param.similarity_threshold, 1 - self._param.keywords_similarity_weight,
                                        aggs=False, rerank_mdl=rerank_mdl,
                                        rank_feature=label_question(query, kbs))
        else:
            kbinfos = {"chunks": [], "doc_aggs": []}

        if self._param.use_kg and kbs:
            ck = settings.kg_retrievaler.retrieval(query,
                                                   [kbs[0].tenant_id],
                                                   self._param.kb_ids,
                                                   embd_mdl,
                                                   LLMBundle(kbs[0].tenant_id, LLMType.CHAT))
            if ck["content_with_weight"]:
                kbinfos["chunks"].insert(0, ck)

        if self._param.tavily_api_key:
            tav = Tavily(self._param.tavily_api_key)
            tav_res = tav.retrieve_chunks(query)
            kbinfos["chunks"].extend(tav_res["chunks"])
            kbinfos["doc_aggs"].extend(tav_res["doc_aggs"])

        if not kbinfos["chunks"]:
            df = Retrieval.be_output("")
            if self._param.empty_response and self._param.empty_response.strip():
                df["empty_response"] = self._param.empty_response
            return df

        df = pd.DataFrame(kbinfos["chunks"])
        df["content"] = df["content_with_weight"]
        del df["content_with_weight"]
        logging.debug("{} {}".format(query, df))
        return df.dropna()


