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
from functools import partial
import pandas as pd
import trio

from agent.component.llm import LLMParam, LLM
from agent.tools.base import LLMToolPluginCallSession
from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from rag.prompts import message_fit_in


class AgentParam(LLMParam):
    """
    Define the Agent component parameters.
    """

    def __init__(self):
        super().__init__()
        self.llm_enabled_tools = []
        self.max_rounds = 5


class Agent(LLM):
    component_name = "Agent"

    async def _invoke(self, **kwargs):
        tools = {}
        for cpn in self._param.llm_enabled_tools:
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
            tools[cpn.get_meta()["function"]["name"]] = cpn

        if not tools:
            return await super()._invoke(**kwargs)

        prompt, msg = self._prepare_prompt_variables()
        chat_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.CHAT, self._param.llm_id)
        chat_mdl.bind_tools(LLMToolPluginCallSession(tools), [v.get_meta() for _,v in tools.items()])

        print(prompt, "\n####################################")
        downstreams = self._canvas.get_component(self._id)["downstream"]
        if any([self._canvas.get_component_obj(cid).component_name.lower()=="message" for cid in downstreams]) and not self._param.output_structure:
            self.set_output("content", partial(self.stream_output_with_tools, chat_mdl, prompt, msg))
            return

        for _ in range(self._param.retry_times+1):
            _, msg = message_fit_in([{"role": "system", "content": prompt}, *msg], int(chat_mdl.max_length * 0.97))
            error = ""
            async with self.thread_limiter:
                ans = await trio.to_thread.run_sync(lambda : chat_mdl.chat(msg[0]["content"], msg[1:], self._param.gen_conf(), max_rounds=self._param.max_rounds))
            msg.pop(0)
            if ans.find("**ERROR**") >= 0:
                logging.error(f"Extractor._chat got error. response: {ans}")
                error = ans
                continue
            self.set_output("content", ans)
            break

        if error:
            self.set_output("_ERROR", error)

    def stream_output_with_tools(self, chat_mdl, prompt, msg):
        _, msg = message_fit_in([{"role": "system", "content": prompt}, *msg], int(chat_mdl.max_length * 0.97))
        answer = ""
        for ans in chat_mdl.chat_streamly(msg[0]["content"], msg[1:], self._param.gen_conf(), max_rounds=self._param.max_rounds):
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
