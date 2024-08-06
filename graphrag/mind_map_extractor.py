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
import collections
import logging
import re
import logging
import traceback
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
from typing import Any

from graphrag.mind_map_prompt import MIND_MAP_EXTRACTION_PROMPT
from graphrag.utils import ErrorHandlerFn, perform_variable_replacements
from rag.llm.chat_model import Base as CompletionLLM
import markdown_to_json
from functools import reduce
from rag.utils import num_tokens_from_string


@dataclass
class MindMapResult:
    """Unipartite Mind Graph result class definition."""
    output: dict


class MindMapExtractor:

    _llm: CompletionLLM
    _input_text_key: str
    _mind_map_prompt: str
    _on_error: ErrorHandlerFn

    def __init__(
            self,
            llm_invoker: CompletionLLM,
            prompt: str | None = None,
            input_text_key: str | None = None,
            on_error: ErrorHandlerFn | None = None,
    ):
        """Init method definition."""
        # TODO: streamline construction
        self._llm = llm_invoker
        self._input_text_key = input_text_key or "input_text"
        self._mind_map_prompt = prompt or MIND_MAP_EXTRACTION_PROMPT
        self._on_error = on_error or (lambda _e, _s, _d: None)

    def __call__(
            self, sections: list[str], prompt_variables: dict[str, Any] | None = None
    ) -> MindMapResult:
        """Call method definition."""
        if prompt_variables is None:
            prompt_variables = {}

        try:
            exe = ThreadPoolExecutor(max_workers=12)
            threads = []
            token_count = max(self._llm.max_length * 0.8, self._llm.max_length-512)
            texts = []
            res = []
            cnt = 0
            for i in range(len(sections)):
                section_cnt = num_tokens_from_string(sections[i])
                if cnt + section_cnt >= token_count and texts:
                    threads.append(exe.submit(self._process_document, "".join(texts), prompt_variables))
                    texts = []
                    cnt = 0
                texts.append(sections[i])
                cnt += section_cnt
            if texts:
                threads.append(exe.submit(self._process_document, "".join(texts), prompt_variables))

            for i, _ in enumerate(threads):
                res.append(_.result())

            merge_json = reduce(self._merge, res)
            merge_json = self._list_to_kv(merge_json)
        except Exception as e:
            logging.exception("error mind graph")
            self._on_error(
                e,
                traceback.format_exc(), None
            )

        return MindMapResult(output=merge_json)

    def _merge(self, d1, d2):
        for k in d1:
            if k in d2:
                if isinstance(d1[k], dict) and isinstance(d2[k], dict):
                    self._merge(d1[k], d2[k])
                elif isinstance(d1[k], list) and isinstance(d2[k], list):
                    d2[k].extend(d1[k])
                else:
                    d2[k] = d1[k]
            else:
                d2[k] = d1[k]

        return d2

    def _list_to_kv(self, data):
        for key, value in data.items():
            if isinstance(value, dict):
                self._list_to_kv(value)
            elif isinstance(value, list):
                new_value = {}
                for i in range(len(value)):
                    if isinstance(value[i], list):
                        new_value[value[i - 1]] = value[i][0]
                data[key] = new_value
            else:
                continue
        return data

    def _todict(self, layer:collections.OrderedDict):
        to_ret = layer
        if isinstance(layer, collections.OrderedDict):
            to_ret = dict(layer)

        try:
            for key, value in to_ret.items():
                to_ret[key] = self._todict(value)
        except AttributeError:
            pass

        return self._list_to_kv(to_ret)

    def _process_document(
            self, text: str, prompt_variables: dict[str, str]
    ) -> str:
        variables = {
            **prompt_variables,
            self._input_text_key: text,
        }
        text = perform_variable_replacements(self._mind_map_prompt, variables=variables)
        gen_conf = {"temperature": 0.5}
        response = self._llm.chat(text, [], gen_conf)
        response = re.sub(r"```[^\n]*", "", response)
        print(response)
        print("---------------------------------------------------\n", self._todict(markdown_to_json.dictify(response)))
        return self._todict(markdown_to_json.dictify(response))
