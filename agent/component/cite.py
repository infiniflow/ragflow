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
from abc import ABC

import pandas as pd

from api.db import LLMType
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from api.settings import retrievaler
from agent.component.base import ComponentBase, ComponentParamBase


class CiteParam(ComponentParamBase):

    """
    Define the Retrieval component parameters.
    """
    def __init__(self):
        super().__init__()
        self.cite_sources = []

    def check(self):
        self.check_empty(self.cite_source, "Please specify where you want to cite from.")


class Cite(ComponentBase, ABC):
    component_name = "Cite"

    def _run(self, history, **kwargs):
        input = "\n- ".join(self.get_input()["content"])
        sources = [self._canvas.get_component(cpn_id).output()[1] for cpn_id in self._param.cite_source]
        query = []
        for role, cnt in history[::-1][:self._param.message_history_window_size]:
            if role != "user":continue
            query.append(cnt)
        query = "\n".join(query)

        kbs = KnowledgebaseService.get_by_ids(self._param.kb_ids)
        if not kbs:
            raise ValueError("Can't find knowledgebases by {}".format(self._param.kb_ids))
        embd_nms = list(set([kb.embd_id for kb in kbs]))
        assert len(embd_nms) == 1, "Knowledge bases use different embedding models."

        embd_mdl = LLMBundle(kbs[0].tenant_id, LLMType.EMBEDDING, embd_nms[0])

        rerank_mdl = None
        if self._param.rerank_id:
            rerank_mdl = LLMBundle(kbs[0].tenant_id, LLMType.RERANK, self._param.rerank_id)

        kbinfos = retrievaler.retrieval(query, embd_mdl, kbs[0].tenant_id, self._param.kb_ids,
                                        1, self._param.top_n,
                                        self._param.similarity_threshold, 1 - self._param.keywords_similarity_weight,
                                        aggs=False, rerank_mdl=rerank_mdl)

        if not kbinfos["chunks"]: return pd.DataFrame()
        df = pd.DataFrame(kbinfos["chunks"])
        df["content"] = df["content_with_weight"]
        del df["content_with_weight"]
        return df


