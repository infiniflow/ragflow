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
from abc import ABC
from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from agent.component import GenerateParam, Generate
from agent.settings import DEBUG


class KeywordExtractParam(GenerateParam):
    """
    Define the KeywordExtract component parameters.
    """

    def __init__(self):
        super().__init__()
        self.top_n = 1

    def check(self):
        super().check()
        self.check_positive_integer(self.top_n, "Top N")

    def get_prompt(self):
        self.prompt = """
- Role: You're a question analyzer. 
- Requirements: 
  - Summarize user's question, and give top %s important keyword/phrase.
  - Use comma as a delimiter to separate keywords/phrases.
- Answer format: (in language of user's question)
  - keyword: 
""" % self.top_n
        return self.prompt


class KeywordExtract(Generate, ABC):
    component_name = "KeywordExtract"

    def _run(self, history, **kwargs):
        q = ""
        for r, c in self._canvas.history[::-1]:
            if r == "user":
                q += c
                break

        chat_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.CHAT, self._param.llm_id)
        ans = chat_mdl.chat(self._param.get_prompt(), [{"role": "user", "content": q}],
                            self._param.gen_conf())

        ans = re.sub(r".*keyword:", "", ans).strip()
        if DEBUG: print(ans, ":::::::::::::::::::::::::::::::::")
        return KeywordExtract.be_output(ans)
