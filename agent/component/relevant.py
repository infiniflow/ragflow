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
from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from agent.component import GenerateParam, Generate
from rag.utils import num_tokens_from_string, encoder


class RelevantParam(GenerateParam):

    """
    Define the Relevant component parameters.
    """
    def __init__(self):
        super().__init__()
        self.prompt = ""
        self.yes = ""
        self.no = ""

    def check(self):
        super().check()
        self.check_empty(self.yes, "[Relevant] 'Yes'")
        self.check_empty(self.no, "[Relevant] 'No'")

    def get_prompt(self):
        self.prompt = """
        You are a grader assessing relevance of a retrieved document to a user question. 
        It does not need to be a stringent test. The goal is to filter out erroneous retrievals.
        If the document contains keyword(s) or semantic meaning related to the user question, grade it as relevant. 
        Give a binary score 'yes' or 'no' score to indicate whether the document is relevant to the question.
        No other words needed except 'yes' or 'no'.
        """
        return self.prompt


class Relevant(Generate, ABC):
    component_name = "Relevant"

    def _run(self, history, **kwargs):
        q = ""
        for r, c in self._canvas.history[::-1]:
            if r == "user":
                q = c
                break
        ans = self.get_input()
        ans = " - ".join(ans["content"]) if "content" in ans else ""
        if not ans:
            return Relevant.be_output(self._param.no)
        ans = "Documents: \n" + ans
        ans = f"Question: {q}\n" + ans
        chat_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.CHAT, self._param.llm_id)

        if num_tokens_from_string(ans) >= chat_mdl.max_length - 4:
            ans = encoder.decode(encoder.encode(ans)[:chat_mdl.max_length - 4])

        ans = chat_mdl.chat(self._param.get_prompt(), [{"role": "user", "content": ans}],
                            self._param.gen_conf())

        logging.debug(ans)
        if ans.lower().find("yes") >= 0:
            return Relevant.be_output(self._param.yes)
        if ans.lower().find("no") >= 0:
            return Relevant.be_output(self._param.no)
        assert False, f"Relevant component got: {ans}"

    def debug(self, **kwargs):
        return self._run([], **kwargs)

