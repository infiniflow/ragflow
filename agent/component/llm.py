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
import logging
import re
import json_repair
from copy import deepcopy
from functools import partial
from typing import Any
import pandas as pd
from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from agent.component.base import ComponentBase, ComponentParamBase
from plugin import GlobalPluginManager
from rag.llm.chat_model import ToolCallSession
from rag.prompts import message_fit_in, citation_prompt


class LLMToolPluginCallSession(ToolCallSession):
    def tool_call(self, name: str, arguments: dict[str, Any]) -> str:
        tool = GlobalPluginManager.get_llm_tool_by_name(name)

        if tool is None:
            raise ValueError(f"LLM tool {name} does not exist")

        return tool().invoke(**arguments)


class LLMParam(ComponentParamBase):
    """
    Define the LLM component parameters.
    """

    def __init__(self):
        super().__init__()
        self.llm_id = ""
        self.sys_prompt = ""
        self.prompts = [{"role": "user", "assistant": "begin@sys.query"}]
        self.max_tokens = 0
        self.temperature = 0
        self.top_p = 0
        self.presence_penalty = 0
        self.frequency_penalty = 0
        self.output_structure = None

    def check(self):
        self.check_decimal_float(self.temperature, "[Generate] Temperature")
        self.check_decimal_float(self.presence_penalty, "[Generate] Presence penalty")
        self.check_decimal_float(self.frequency_penalty, "[Generate] Frequency penalty")
        self.check_nonnegative_number(self.max_tokens, "[Generate] Max tokens")
        self.check_decimal_float(self.top_p, "[Generate] Top P")
        self.check_empty(self.llm_id, "[Generate] LLM")

    def gen_conf(self):
        conf = {}
        if self.max_tokens > 0:
            conf["max_tokens"] = self.max_tokens
        if self.temperature > 0:
            conf["temperature"] = self.temperature
        if self.top_p > 0:
            conf["top_p"] = self.top_p
        if self.presence_penalty > 0:
            conf["presence_penalty"] = self.presence_penalty
        if self.frequency_penalty > 0:
            conf["frequency_penalty"] = self.frequency_penalty
        return conf


class LLM(ComponentBase):
    component_name = "LLM"

    def get_input_elements(self):
        res = self.get_input_elements_from_text(self._param.sys_prompt)
        for prompt in self._param.prompts:
            d = self.get_input_elements_from_text(prompt["content"])
            res.update(d)
        return res

    async def _invoke(self, **kwargs):
        def replace_ids(cnt, start_idx, prefix):
            patt = []
            for r in re.finditer(r"ID: %s_([0-9]+)\n", cnt, flags=re.DOTALL):
                idx = int(r.group(1))
                patt.append((f"ID: {prefix}_{idx}", f"ID: {prefix}_{idx+start_idx}"))
            for p, r in patt:
                cnt = re.sub(r, p, cnt, flags=re.DOTALL)
            return cnt

        chat_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.CHAT, self._param.llm_id)
        vars = self.get_input_elements()
        args = {}
        references = {"chunks": [], "doc_aggs": []}
        prompt = self._param.sys_prompt
        for k in vars.keys():
            cpn_id, var_nm = k.split("@")
            cpn = self._canvas.get_component(cpn_id)
            if not cpn:
                raise Exception(f"Can't find variable: '{var_nm}'")
            ref = self._canvas.get_variable_value(f"{cpn_id}@_references")
            if ref:
                prompt = replace_ids(prompt, len(references["chunks"]), cpn_id)
                references["chunks"].extend(ref["chunks"])
                references["doc_aggs"].extend(ref["doc_aggs"])
            args[k] = self._canvas.get_variable_value(k)
            if not isinstance(args[k], str):
                try:
                    args[k] = json.dumps(args[k], ensure_ascii=False)
                except Exception:
                    args[k] = str(args[k])
            self.set_input_value(k, args[k])

        prompt = self._param.sys_prompt
        msg = self._canvas.get_history(self._param.message_history_window_size)
        msg.extend(deepcopy(self._param.prompts))
        prompt = prompt.format(**args)
        for m in msg:
            m["content"] = m["content"].format(**args)
        prompt += citation_prompt() if references["chunks"] else ""

        if self._param.output_structure:
            prompt += "\nThe output MUST follow this JSON format:\n"+json.dumps(self._param.output_structure, ensure_ascii=False, indent=2)
            for _ in range(self._param.retry_times+1):
                _, msg = message_fit_in([{"role": "system", "content": prompt}, *msg], int(chat_mdl.max_length * 0.97))
                ans = chat_mdl.chat(msg[0]["content"], msg[1:], self._param.gen_conf())
                msg.pop(0)
                if ans.find("**ERROR**") >= 0:
                    logging.error(f"Extractor._chat got error. response: {ans}")
                    raise Exception(ans)
                ans = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
                ans = re.sub(r"^.*```json", "", ans, flags=re.DOTALL)
                ans = re.sub(r"```\n*$", "", ans, flags=re.DOTALL)
                try:
                    self._param.outputs["structured_content"] = json_repair.loads(ans)
                    return
                except Exception as e:
                    msg.append({"role": "user", "content": "The answer can't not be parsed as JSON"})

        downstreams = self._canvas.get_component(self._id)["downstream"]
        if kwargs.get("stream") and not downstreams and not self._param.output_structure:
            self._param.outputs["content"]["value"] = partial(self.stream_output, chat_mdl, prompt, msg)

        if self._param.output_structure:
            prompt += "\nThe output MUST follow this JSON format:\n"+json.dumps(self._param.output_structure, ensure_ascii=False, indent=2)

        for _ in range(self._param.retry_times+1):
            _, msg = message_fit_in([{"role": "system", "content": prompt}, *msg], int(chat_mdl.max_length * 0.97))
            ans = chat_mdl.chat(msg[0]["content"], msg[1:], self._param.gen_conf())
            msg.pop(0)
            if ans.find("**ERROR**") >= 0:
                logging.error(f"Extractor._chat got error. response: {ans}")
                raise Exception(ans)
            if self._param.output_structure:
                ans = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
                ans = re.sub(r"^.*```json", "", ans, flags=re.DOTALL)
                ans = re.sub(r"```\n*$", "", ans, flags=re.DOTALL)
                try:
                    self._param.outputs["structured_content"]["value"] = json_repair.loads(ans)
                    return
                except Exception as e:
                    msg.append({"role": "user", "content": "The answer can't not be parsed as JSON"})
            else:
                self._param.outputs["content"]["value"] = ans
                return

    def stream_output(self, chat_mdl, prompt, msg):
        _, msg = message_fit_in([{"role": "system", "content": prompt}, *msg], int(chat_mdl.max_length * 0.97))
        answer = ""
        for ans in chat_mdl.chat_streamly(msg[0]["content"], msg[1:], self._param.gen_conf()):
            res = {"content": ans, "reference": []}
            answer = ans
            yield res

        self._param.outputs["content"]["value"] = answer

    def debug(self, **kwargs):
        chat_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.CHAT, self._param.llm_id)
        prompt = self._param.prompt

        for para in self._param.debug_inputs:
            kwargs[para["key"]] = para.get("value", "")

        for n, v in kwargs.items():
            prompt = re.sub(r"\{%s\}" % re.escape(n), str(v).replace("\\", " "), prompt)

        u = kwargs.get("user")
        ans = chat_mdl.chat(prompt, [{"role": "user", "content": u if u else "Output: "}], self._param.gen_conf())
        return pd.DataFrame([ans])
