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
import os
import re
from functools import partial
import pandas as pd
import trio

from agent.component.llm import LLMParam, LLM
from agent.tools.base import LLMToolPluginCallSession
from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from api.utils.api_utils import timeout
from rag.prompts import message_fit_in


class AgentParam(LLMParam):
    """
    Define the Agent component parameters.
    """

    def __init__(self):
        super().__init__()
        self.tools = []
        self.max_rounds = 5
        self.description = ""

    def get_meta(self):
        return {
            "type": "function",
            "function": {
                "name": "agent",
                "description": self.description,
                "parameters": {
                    "user_prompt": {
                        "type": "string",
                        "description": "This is the order you need to sent to the agent.",
                        "default": "{sys.query}",
                        "required": True
                    }
                }
            }
        }


class Agent(LLM):
    component_name = "Agent"

    def __init__(self, canvas, id, param: LLMParam):
        super().__init__(canvas, id, param)
        self.tools = {}
        for cpn in self._param.tools:
            from agent.component import component_class
            param = component_class(cpn["component_name"] + "Param")()
            param.update(cpn["params"])
            try:
                param.check()
            except Exception as e:
                self.set_output("_ERROR", cpn["component_name"] + f" configuration error: {e}")
                return
            cpn_id = "_" + cpn["component_name"]
            cpn = component_class(cpn["component_name"])(self._canvas, cpn_id, param)
            self.tools[cpn.get_meta()["function"]["name"]] = cpn

        self.chat_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.CHAT, self._param.llm_id,
                                  max_retries=self._param.max_retries,
                                  retry_interval=self._param.delay_after_error,
                                  max_rounds=self._param.max_rounds
                                  )
        self.chat_mdl.bind_tools(LLMToolPluginCallSession(self.tools), [v.get_meta() for _,v in self.tools.items()])

    @timeout(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60))
    def _invoke(self, **kwargs):
        if not self.tools:
            return super()._invoke(**kwargs)

        prompt, msg = self._prepare_prompt_variables()

        print(prompt, "\n####################################")
        downstreams = self._canvas.get_component(self._id)["downstream"]
        if any([self._canvas.get_component_obj(cid).component_name.lower()=="message" for cid in downstreams]) and not self._param.output_structure:
            self.set_output("content", partial(self.stream_output_with_tools, prompt, msg))
            return

        _, msg = message_fit_in([{"role": "system", "content": prompt}, *msg], int(self.chat_mdl.max_length * 0.97))
        ans = self._generate(msg[0]["content"], msg[1:], conf=self._param.gen_conf())
        msg.pop(0)
        if ans.find("**ERROR**") >= 0:
            logging.error(f"Extractor._chat got error. response: {ans}")
            self.set_output("_ERROR", ans)
            return
        self.set_output("content", ans)

    def stream_output_with_tools(self, prompt, msg):
        _, msg = message_fit_in([{"role": "system", "content": prompt}, *msg], int(self.chat_mdl.max_length * 0.97))
        answer = ""
        for ans in self.chat_mdl.chat_streamly(msg[0]["content"], msg[1:], gen_conf=self._param.gen_conf()):
            yield ans[len(answer):]
            answer = ans
        self.set_output("content", answer)

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
