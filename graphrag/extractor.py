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
from graphrag.utils import get_llm_cache, set_llm_cache
from rag.llm.chat_model import Base as CompletionLLM


class Extractor:
    _llm: CompletionLLM

    def __init__(self, llm_invoker: CompletionLLM):
        self._llm = llm_invoker

    def _chat(self, system, history, gen_conf):
        response = get_llm_cache(self._llm.llm_name, system, history, gen_conf)
        if response:
            return response
        response = self._llm.chat(system, history, gen_conf)
        if response.find("**ERROR**") >= 0:
            raise Exception(response)
        set_llm_cache(self._llm.llm_name, system, response, history, gen_conf)
        return response
