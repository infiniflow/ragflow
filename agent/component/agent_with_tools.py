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
import os
import re
from copy import deepcopy
from functools import partial
from typing import Any

import json_repair
import pandas as pd
from agent.component.llm import LLMParam, LLM
from agent.tools.base import LLMToolPluginCallSession, ToolParamBase, ToolBase, ToolMeta
from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from api.utils.api_utils import timeout
from rag.llm.chat_model import ReActMode
from rag.prompts import message_fit_in


class AgentParam(LLMParam, ToolParamBase):
    """
    Define the Agent component parameters.
    """

    def __init__(self):
        self.meta:ToolMeta = {
                "name": "agent",
                "description": "This is an agent for every task.",
                "parameters": {
                    "user_prompt": {
                        "type": "string",
                        "description": "This is the order you need to sent to the agent.",
                        "default": "",
                        "required": True
                    }
                }
            }
        super().__init__()
        self.function_name = "agent"
        self.tools = []
        self.max_rounds = 50
        self.description = ""


class Agent(LLM, ToolBase):
    component_name = "Agent"

    def __init__(self, canvas, id, param: LLMParam):
        LLM.__init__(self, canvas, id, param)
        self.tools = {}
        name = canvas.get_component_name(id)
        for cpn in self._param.tools:
            from agent.component import component_class
            param = component_class(cpn["component_name"] + "Param")()
            param.update(cpn["params"])
            try:
                param.check()
            except Exception as e:
                self.set_output("_ERROR", cpn["component_name"] + f" configuration error: {e}")
                return
            cpn_id = f"{name}-->" + cpn.get("name", "")
            cpn = component_class(cpn["component_name"])(self._canvas, cpn_id, param)
            self.tools[cpn.get_meta()["function"]["name"]] = cpn

        self.chat_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.CHAT, self._param.llm_id,
                                  max_retries=self._param.max_retries,
                                  retry_interval=self._param.delay_after_error,
                                  max_rounds=self._param.max_rounds,
                                  verbose_tool_use=True,
                                  react_mode=ReActMode.REACT
                                  )
        tool_metas = [v.get_meta() for _,v in self.tools.items()]
        self.chat_mdl.bind_tools(LLMToolPluginCallSession(self.tools, partial(self._canvas.tool_use_callback, id)), tool_metas)

    def get_meta(self) -> dict[str, Any]:
        self._param.function_name= self._id
        return super().get_meta()

    def _extract_tool_use(self, ans, use_tools, clean=False):
        patt = r"<tool_call>(.*?)</tool_call>"
        s = 0
        txt = ""
        for r in re.finditer(patt, ans, flags=re.DOTALL):
            try:
                res = json_repair.loads(r.group(1))
                if isinstance(res["result"], dict):
                    use_tools.append(deepcopy(res))
                    res["result"] = "End"
                txt += "<tool_call>{}</tool_call>".format(ans[s: r.start()] + json.dumps(res, ensure_ascii=False, indent=2))
            except:
                txt += "<tool_call>{}</tool_call>".format(r.group(1))

            s = r.end()
        if s < len(ans):
            txt += ans[s:]
        if clean:
            return re.sub(patt, "", txt, flags=re.DOTALL)
        return txt

    @timeout(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60))
    def _invoke(self, **kwargs):
        if kwargs.get("user_prompt"):
            self._param.prompts = [{"role": "user", "content": kwargs["user_prompt"]}]

        if not self.tools:
            return LLM._invoke(self, **kwargs)

        prompt, msg = self._prepare_prompt_variables()

        downstreams = self._canvas.get_component(self._id)["downstream"] if self._canvas.get_component(self._id) else []
        if any([self._canvas.get_component_obj(cid).component_name.lower()=="message" for cid in downstreams]) and not self._param.output_structure:
            self.set_output("content", partial(self.stream_output_with_tools, prompt, msg))
            return

        _, msg = message_fit_in([{"role": "system", "content": prompt}, *msg], int(self.chat_mdl.max_length * 0.97))
        ans = self._generate(msg[0]["content"], msg[1:], conf=self._param.gen_conf())
        msg.pop(0)
        if ans.find("**ERROR**") >= 0:
            logging.error(f"Agent._chat got error. response: {ans}")
            self.set_output("_ERROR", ans)
            return
        use_tools = []
        ans = self._extract_tool_use(ans, use_tools, True)
        self.set_output("content", ans)
        if use_tools:
            self.set_output("use_tools", use_tools)

    def stream_output_with_tools(self, prompt, msg):
        _, msg = message_fit_in([{"role": "system", "content": prompt}, *msg], int(self.chat_mdl.max_length * 0.97))
        answer_without_toolcall = ""
        use_tools = []
        last_idx = 0
        endswith_think = False
        for ans in self.chat_mdl.chat_streamly(msg[0]["content"], msg[1:], gen_conf=self._param.gen_conf()):
            delta_ans = self._extract_tool_use(ans[last_idx:], use_tools)
            if delta_ans.find("<think>") >= 0:
                yield "<think>"
            if delta_ans.endswith("</think>"):
                endswith_think = True
            elif endswith_think:
                endswith_think = False
                yield "</think>"
            yield re.sub(r"(<think>|</think>)", "", delta_ans)

            last_idx = len(ans)
            if ans.endswith("</think>"):
                last_idx -= len("</think>")
            if answer_without_toolcall.endswith("</think>") and delta_ans.endswith("</think>"):
                answer_without_toolcall.rstrip("</think>")
            answer_without_toolcall += delta_ans
        self.set_output("content", answer_without_toolcall)
        if use_tools:
            self.set_output("use_tools", use_tools)

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
