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
from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from agent.component import GenerateParam, Generate


class RewriteQuestionParam(GenerateParam):

    """
    Define the QuestionRewrite component parameters.
    """
    def __init__(self):
        super().__init__()
        self.temperature = 0.9
        self.prompt = ""
        self.loop = 1

    def check(self):
        super().check()

    def get_prompt(self):
        self.prompt = """
        You are an expert at query expansion to generate a paraphrasing of a question.
        I can't retrieval relevant information from the knowledge base by using user's question directly.     
        You need to expand or paraphrase user's question by multiple ways such as using synonyms words/phrase, 
        writing the abbreviation in its entirety, adding some extra descriptions or explanations, 
        changing the way of expression, translating the original question into another language (English/Chinese), etc. 
        And return 5 versions of question and one is from translation.
        Just list the question. No other words are needed.
        """
        return self.prompt


class RewriteQuestion(Generate, ABC):
    component_name = "RewriteQuestion"

    def _run(self, history, **kwargs):
        if not hasattr(self, "_loop"):
            setattr(self, "_loop", 0)
        if self._loop >= self._param.loop:
            self._loop = 0
            raise Exception("Maximum loop time exceeds. Can't find relevant information.")
        self._loop += 1
        q = "Question: "
        for r, c in self._canvas.history[::-1]:
            if r == "user":
                q += c
                break

        chat_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.CHAT, self._param.llm_id)
        ans = chat_mdl.chat(self._param.get_prompt(), [{"role": "user", "content": q}],
                            self._param.gen_conf())

        print(ans, ":::::::::::::::::::::::::::::::::")
        return RewriteQuestion.be_output(ans)


