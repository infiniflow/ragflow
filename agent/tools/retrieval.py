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
from functools import partial
import json
import os
import re
from abc import ABC
from agent.tools.base import ToolParamBase, ToolBase, ToolMeta
from common.constants import LLMType
from api.db.services.document_service import DocumentService
from api.db.services.dialog_service import meta_filter
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from common import settings
from common.connection_utils import timeout
from rag.app.tag import label_question
from rag.prompts.generator import cross_languages, kb_prompt, gen_meta_filter


class RetrievalParam(ToolParamBase):
    """
    Define the Retrieval component parameters.
    """

    def __init__(self):
        self.meta:ToolMeta = {
            "name": "search_my_dateset",
            "description": "This tool can be utilized for relevant content searching in the datasets.",
            "parameters": {
                "query": {
                    "type": "string",
                    "description": "The keywords to search the dataset. The keywords should be the most important words/terms(includes synonyms) from the original request.",
                    "default": "",
                    "required": True
                }
            }
        }
        super().__init__()
        self.function_name = "search_my_dateset"
        self.description = "This tool can be utilized for relevant content searching in the datasets."
        self.similarity_threshold = 0.2
        self.keywords_similarity_weight = 0.5
        self.top_n = 8
        self.top_k = 1024
        self.kb_ids = []
        self.kb_vars = []
        self.rerank_id = ""
        self.empty_response = ""
        self.use_kg = False
        self.cross_languages = []
        self.toc_enhance = False
        self.meta_data_filter={}

    def check(self):
        self.check_decimal_float(self.similarity_threshold, "[Retrieval] Similarity threshold")
        self.check_decimal_float(self.keywords_similarity_weight, "[Retrieval] Keyword similarity weight")
        self.check_positive_number(self.top_n, "[Retrieval] Top N")

    def get_input_form(self) -> dict[str, dict]:
        return {
            "query": {
                "name": "Query",
                "type": "line"
            }
        }

class Retrieval(ToolBase, ABC):
    component_name = "Retrieval"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 12)))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("Retrieval processing"):
            return

        if not kwargs.get("query"):
            self.set_output("formalized_content", self._param.empty_response)
            return

        kb_ids: list[str] = []
        for id in self._param.kb_ids:
            if id.find("@") < 0:
                kb_ids.append(id)
                continue
            kb_nm = self._canvas.get_variable_value(id)
            # if kb_nm is a list
            kb_nm_list = kb_nm if isinstance(kb_nm, list) else [kb_nm]
            for nm_or_id in kb_nm_list:
                e, kb = KnowledgebaseService.get_by_name(nm_or_id,
                                                         self._canvas._tenant_id)
                if not e:
                    e, kb = KnowledgebaseService.get_by_id(nm_or_id)
                    if not e:
                        raise Exception(f"Dataset({nm_or_id}) does not exist.")
                kb_ids.append(kb.id)

        filtered_kb_ids: list[str] = list(set([kb_id for kb_id in kb_ids if kb_id]))

        kbs = KnowledgebaseService.get_by_ids(filtered_kb_ids)
        if not kbs:
            raise Exception("No dataset is selected.")

        embd_nms = list(set([kb.embd_id for kb in kbs]))
        assert len(embd_nms) == 1, "Knowledge bases use different embedding models."

        embd_mdl = None
        if embd_nms:
            embd_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.EMBEDDING, embd_nms[0])

        rerank_mdl = None
        if self._param.rerank_id:
            rerank_mdl = LLMBundle(kbs[0].tenant_id, LLMType.RERANK, self._param.rerank_id)

        vars = self.get_input_elements_from_text(kwargs["query"])
        vars = {k:o["value"] for k,o in vars.items()}
        query = self.string_format(kwargs["query"], vars)

        doc_ids=[]
        if self._param.meta_data_filter!={}:
            metas = DocumentService.get_meta_by_kbs(kb_ids)
            if self._param.meta_data_filter.get("method") == "auto":
                chat_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.CHAT)
                filters = gen_meta_filter(chat_mdl, metas, query)
                doc_ids.extend(meta_filter(metas, filters))
                if not doc_ids:
                    doc_ids = None
            elif self._param.meta_data_filter.get("method") == "manual":
                filters=self._param.meta_data_filter["manual"]
                for flt in filters:
                    pat = re.compile(self.variable_ref_patt)
                    s = flt["value"]
                    out_parts = []
                    last = 0

                    for m in pat.finditer(s):
                        out_parts.append(s[last:m.start()])
                        key = m.group(1)
                        v = self._canvas.get_variable_value(key)
                        if v is None:
                            rep = ""
                        elif isinstance(v, partial):
                            buf = []
                            for chunk in v():
                                buf.append(chunk)
                            rep = "".join(buf)
                        elif isinstance(v, str):
                            rep = v
                        else:
                            rep = json.dumps(v, ensure_ascii=False)

                        out_parts.append(rep)
                        last = m.end()

                    out_parts.append(s[last:])
                    flt["value"] = "".join(out_parts)
                doc_ids.extend(meta_filter(metas, filters))
                if not doc_ids:
                    doc_ids = None

        if self._param.cross_languages:
            query = cross_languages(kbs[0].tenant_id, None, query, self._param.cross_languages)

        if kbs:
            query = re.sub(r"^user[:：\s]*", "", query, flags=re.IGNORECASE)
            kbinfos = settings.retriever.retrieval(
                query,
                embd_mdl,
                [kb.tenant_id for kb in kbs],
                filtered_kb_ids,
                1,
                self._param.top_n,
                self._param.similarity_threshold,
                1 - self._param.keywords_similarity_weight,
                doc_ids=doc_ids,
                aggs=False,
                rerank_mdl=rerank_mdl,
                rank_feature=label_question(query, kbs),
            )
            if self.check_if_canceled("Retrieval processing"):
                return

            if self._param.toc_enhance:
                chat_mdl = LLMBundle(self._canvas._tenant_id, LLMType.CHAT)
                cks = settings.retriever.retrieval_by_toc(query, kbinfos["chunks"], [kb.tenant_id for kb in kbs], chat_mdl, self._param.top_n)
                if self.check_if_canceled("Retrieval processing"):
                    return
                if cks:
                    kbinfos["chunks"] = cks
            if self._param.use_kg:
                ck = settings.kg_retriever.retrieval(query,
                                                       [kb.tenant_id for kb in kbs],
                                                       kb_ids,
                                                       embd_mdl,
                                                       LLMBundle(self._canvas.get_tenant_id(), LLMType.CHAT))
                if self.check_if_canceled("Retrieval processing"):
                    return
                if ck["content_with_weight"]:
                    kbinfos["chunks"].insert(0, ck)
        else:
            kbinfos = {"chunks": [], "doc_aggs": []}

        if self._param.use_kg and kbs:
            ck = settings.kg_retriever.retrieval(query, [kb.tenant_id for kb in kbs], filtered_kb_ids, embd_mdl, LLMBundle(kbs[0].tenant_id, LLMType.CHAT))
            if self.check_if_canceled("Retrieval processing"):
                return
            if ck["content_with_weight"]:
                ck["content"] = ck["content_with_weight"]
                del ck["content_with_weight"]
                kbinfos["chunks"].insert(0, ck)

        for ck in kbinfos["chunks"]:
            if "vector" in ck:
                del ck["vector"]
            if "content_ltks" in ck:
                del ck["content_ltks"]

        if not kbinfos["chunks"]:
            self.set_output("formalized_content", self._param.empty_response)
            return

        # Format the chunks for JSON output (similar to how other tools do it)
        json_output = kbinfos["chunks"].copy()

        self._canvas.add_reference(kbinfos["chunks"], kbinfos["doc_aggs"])
        form_cnt = "\n".join(kb_prompt(kbinfos, 200000, True))

        # Set both formalized content and JSON output
        self.set_output("formalized_content", form_cnt)
        self.set_output("json", json_output)

        return form_cnt

    def thoughts(self) -> str:
        return """
Keywords: {}
Looking for the most relevant articles.
        """.format(self.get_input().get("query", "-_-!"))
