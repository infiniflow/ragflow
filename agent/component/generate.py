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
import re
from functools import partial
import pandas as pd
from api.db import LLMType
from api.db.services.dialog_service import message_fit_in
from api.db.services.llm_service import LLMBundle
from api.settings import retrievaler
from agent.component.base import ComponentBase, ComponentParamBase


class GenerateParam(ComponentParamBase):
    """
    Define the Generate component parameters.
    """

    def __init__(self):
        super().__init__()
        self.llm_id = ""
        self.prompt = ""
        self.max_tokens = 0
        self.temperature = 0
        self.top_p = 0
        self.presence_penalty = 0
        self.frequency_penalty = 0
        self.cite = True
        self.parameters = []

    def check(self):
        self.check_decimal_float(self.temperature, "[Generate] Temperature")
        self.check_decimal_float(self.presence_penalty, "[Generate] Presence penalty")
        self.check_decimal_float(self.frequency_penalty, "[Generate] Frequency penalty")
        self.check_nonnegative_number(self.max_tokens, "[Generate] Max tokens")
        self.check_decimal_float(self.top_p, "[Generate] Top P")
        self.check_empty(self.llm_id, "[Generate] LLM")
        # self.check_defined_type(self.parameters, "Parameters", ["list"])

    def gen_conf(self):
        conf = {}
        if self.max_tokens > 0: conf["max_tokens"] = self.max_tokens
        if self.temperature > 0: conf["temperature"] = self.temperature
        if self.top_p > 0: conf["top_p"] = self.top_p
        if self.presence_penalty > 0: conf["presence_penalty"] = self.presence_penalty
        if self.frequency_penalty > 0: conf["frequency_penalty"] = self.frequency_penalty
        return conf


class Generate(ComponentBase):
    component_name = "Generate"

    def get_dependent_components(self):
        cpnts = [para["component_id"] for para in self._param.parameters if para.get("component_id") and para["component_id"].lower().find("answer") < 0]
        return cpnts

    def set_cite(self, retrieval_res, answer):
        retrieval_res = retrieval_res.dropna(subset=["vector", "content_ltks"]).reset_index(drop=True)
        if "empty_response" in retrieval_res.columns:
            retrieval_res["empty_response"].fillna("", inplace=True)
        answer, idx = retrievaler.insert_citations(answer, [ck["content_ltks"] for _, ck in retrieval_res.iterrows()],
                                                   [ck["vector"] for _, ck in retrieval_res.iterrows()],
                                                   LLMBundle(self._canvas.get_tenant_id(), LLMType.EMBEDDING,
                                                             self._canvas.get_embedding_model()), tkweight=0.7,
                                                   vtweight=0.3)
        doc_ids = set([])
        recall_docs = []
        for i in idx:
            did = retrieval_res.loc[int(i), "doc_id"]
            if did in doc_ids: continue
            doc_ids.add(did)
            recall_docs.append({"doc_id": did, "doc_name": retrieval_res.loc[int(i), "docnm_kwd"]})

        del retrieval_res["vector"]
        del retrieval_res["content_ltks"]

        reference = {
            "chunks": [ck.to_dict() for _, ck in retrieval_res.iterrows()],
            "doc_aggs": recall_docs
        }

        if answer.lower().find("invalid key") >= 0 or answer.lower().find("invalid api") >= 0:
            answer += " Please set LLM API-Key in 'User Setting -> Model Providers -> API-Key'"
        res = {"content": answer, "reference": reference}

        return res

    def _run(self, history, **kwargs):
        chat_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.CHAT, self._param.llm_id)
        prompt = self._param.prompt

        retrieval_res = []
        self._param.inputs = []
        for para in self._param.parameters:
            if not para.get("component_id"): continue
            if para["component_id"].split("@")[0].lower().find("begin") > 0:
                cpn_id, key = para["component_id"].split("@")
                for p in self._canvas.get_component(cpn_id)["obj"]._param.query:
                    if p["key"] == key:
                        kwargs[para["key"]] = p["value"]
                        self._param.inputs.append(
                            {"component_id": para["component_id"], "content": kwargs[para["key"]]})
                        break
                else:
                    assert False, f"Can't find parameter '{key}' for {cpn_id}"
                continue

            cpn = self._canvas.get_component(para["component_id"])["obj"]
            if cpn.component_name.lower() == "answer":
                kwargs[para["key"]] = self._canvas.get_history(1)[0]["content"]
                continue
            _, out = cpn.output(allow_partial=False)
            if "content" not in out.columns:
                kwargs[para["key"]] = "Nothing"
            else:
                if cpn.component_name.lower() == "retrieval":
                    retrieval_res.append(out)
                kwargs[para["key"]] = "  - "+"\n - ".join([o if isinstance(o, str) else str(o) for o in out["content"]])
            self._param.inputs.append({"component_id": para["component_id"], "content": kwargs[para["key"]]})

        if retrieval_res:
            retrieval_res = pd.concat(retrieval_res, ignore_index=True)
        else: retrieval_res = pd.DataFrame([])

        for n, v in kwargs.items():
            prompt = re.sub(r"\{%s\}" % re.escape(n), re.escape(str(v)), prompt)

        if not self._param.inputs and prompt.find("{input}") >= 0:
            retrieval_res = self.get_input()
            input = ("  - " + "\n  - ".join(
                [c for c in retrieval_res["content"] if isinstance(c, str)])) if "content" in retrieval_res else ""
            prompt = re.sub(r"\{input\}", re.escape(input), prompt)

        downstreams = self._canvas.get_component(self._id)["downstream"]
        if kwargs.get("stream") and len(downstreams) == 1 and self._canvas.get_component(downstreams[0])[
            "obj"].component_name.lower() == "answer":
            return partial(self.stream_output, chat_mdl, prompt, retrieval_res)

        if "empty_response" in retrieval_res.columns and not "".join(retrieval_res["content"]):
            res = {"content": "\n- ".join(retrieval_res["empty_response"]) if "\n- ".join(
                retrieval_res["empty_response"]) else "Nothing found in knowledgebase!", "reference": []}
            return pd.DataFrame([res])

        msg = self._canvas.get_history(self._param.message_history_window_size)
        _, msg = message_fit_in([{"role": "system", "content": prompt}, *msg], int(chat_mdl.max_length * 0.97))
        if len(msg) < 2: msg.append({"role": "user", "content": ""})
        ans = chat_mdl.chat(msg[0]["content"], msg[1:], self._param.gen_conf())

        if self._param.cite and "content_ltks" in retrieval_res.columns and "vector" in retrieval_res.columns:
            res = self.set_cite(retrieval_res, ans)
            return pd.DataFrame([res])

        return Generate.be_output(ans)

    def stream_output(self, chat_mdl, prompt, retrieval_res):
        res = None
        if "empty_response" in retrieval_res.columns and not "".join(retrieval_res["content"]):
            res = {"content": "\n- ".join(retrieval_res["empty_response"]) if "\n- ".join(
                retrieval_res["empty_response"]) else "Nothing found in knowledgebase!", "reference": []}
            yield res
            self.set_output(res)
            return

        msg = self._canvas.get_history(self._param.message_history_window_size)
        _, msg = message_fit_in([{"role": "system", "content": prompt}, *msg], int(chat_mdl.max_length * 0.97))
        if len(msg) < 2: msg.append({"role": "user", "content": ""})
        answer = ""
        for ans in chat_mdl.chat_streamly(msg[0]["content"], msg[1:], self._param.gen_conf()):
            res = {"content": ans, "reference": []}
            answer = ans
            yield res

        if self._param.cite and "content_ltks" in retrieval_res.columns and "vector" in retrieval_res.columns:
            res = self.set_cite(retrieval_res, answer)
            yield res

        self.set_output(res)
