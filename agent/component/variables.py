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
import re
from abc import ABC
from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from agent.component import GenerateParam, Generate
import json


class VariablesExtractParam(GenerateParam):
    """
    Define the VariablesExtract component parameters.
    """

    def __init__(self):
        super().__init__()
        self.temperature = 0.4
        self.prompt = ""


    def check(self):
        super().check()



    def get_prompt(self, conv:str, params:dict):
        prompt = f"""

You are a data expert extracting information. DON'T generate anything except the information extracted with flat JSON (no nested JSON or Array). 
Get {", ".join([f"'{key}'" for key in params.keys()])} and any field from the conversation below.
{conv}
"""
        logging.info(f"VariablesExtract: get_prompt: {prompt}")
        return prompt


class VariablesExtract(Generate, ABC):
    component_name = "VariablesExtract"

    def _run(self, history, **kwargs):
        args = {}
        logging.info(f"VariablesExtract: _run: {self._param.variables}")
        for para in self._param.variables:
            if para.get("key"):
                if 'begin@' in para["key"]:
                    field = para["key"].split('@')[1]
                    field = field.strip()
                    if field:
                        args[field] = ""

        inputs = self.get_input()
        query = "\n".join(i.strip() for i in inputs["content"] if i.strip())
        hist = self._canvas.get_history(self._param.message_history_window_size)
        initquestion = ""
        conv = ["{}: {}".format("USER", query)]
        for m in hist:
            if m["role"] not in ["user"]:
                continue
            initquestion = m["content"]
            conv.append("{}: {}".format(m["role"].upper(), m["content"]))
        conv = "\n".join(conv)
        chat_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.CHAT, self._param.llm_id)
        ans = chat_mdl.chat(self._param.get_prompt(conv, args),
                            [{"role": "user", "content": "Output:"}], self._param.gen_conf())
        ans = re.sub(r"\s+", " ", ans)
        match = re.search(r"```json\s*(.*?)\s*```", ans, re.DOTALL)
        if match:
            ans = match.group(1)
            logging.info(ans)
        if not ans:
            logging.info(ans)
            return VariablesExtract.be_output(initquestion)

        logging.info(f"ans: {ans}")

        try:
            kwargs = {}
            ans_json = json.loads(ans)
    
            for v in ans_json:
                invalid_values = {"unknown", "none", "invalid", "not found", "not available", "not applicable", "", "null"}
                data = "{}".format(ans_json[v]).strip()
                if data.lower() in invalid_values:
                    continue
                if data:
                    kwargs[v] = data.strip()

                    # Add missing keys to global parameters
                    self._canvas.add_item_global_param(key=v, value=data.strip(), description=f"Extracted variable: {v}")

                    logging.info(f"Extracted variable: {v} = {kwargs[v]}")
            self._canvas.set_global_param(**kwargs)
            logging.info("Begin: query: {}".format(self._canvas.components["begin"]["obj"]._param.query))

            return VariablesExtract.be_output(query)
        except json.JSONDecodeError:
            logging.warning(f"VariablesExtract: LLM returned non-JSON output: {ans}")
            return VariablesExtract.be_output(query)

    def debug(self, **kwargs):
        return self._run([], **kwargs)