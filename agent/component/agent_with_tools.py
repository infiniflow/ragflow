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
from concurrent.futures import ThreadPoolExecutor
from copy import deepcopy
from functools import partial
from typing import Any, Tuple

import json_repair
import pandas as pd

from agent.component.llm import LLMParam, LLM
from agent.tools.base import LLMToolPluginCallSession, ToolParamBase, ToolBase, ToolMeta
from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from api.db.services.mcp_server_service import MCPServerService
from api.utils.api_utils import timeout
from rag.llm.chat_model import ReActMode
from rag.prompts import message_fit_in
from rag.prompts.prompts import next_step, COMPLETE_TASK, analyze_task, form_history, \
    citation_prompt, reflect, rank_memories, kb_prompt, citation_plus
from rag.utils import num_tokens_from_string
from rag.utils.mcp_tool_call_conn import MCPToolCallSession, mcp_tool_metadata_to_openai_tool


class AgentParam(LLMParam, ToolParamBase):
    """
    Define the Agent component parameters.
    """

    def __init__(self):
        self.meta:ToolMeta = {
                "name": "agent",
                "description": "This is an agent for a specific task.",
                "parameters": {
                    "user_prompt": {
                        "type": "string",
                        "description": "This is the order you need to send to the agent.",
                        "default": "",
                        "required": True
                    }
                }
            }
        super().__init__()
        self.function_name = "agent"
        self.tools = []
        self.mcp = []
        self.max_rounds = 50
        self.description = ""


class Agent(LLM, ToolBase):
    component_name = "Agent"

    def __init__(self, canvas, id, param: LLMParam):
        LLM.__init__(self, canvas, id, param)
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
            cpn_id = f"{id}-->" + cpn.get("name", "").replace(" ", "_")
            cpn = component_class(cpn["component_name"])(self._canvas, cpn_id, param)
            self.tools[cpn.get_meta()["function"]["name"]] = cpn

        self.chat_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.CHAT, self._param.llm_id,
                                  max_retries=self._param.max_retries,
                                  retry_interval=self._param.delay_after_error,
                                  max_rounds=self._param.max_rounds,
                                  verbose_tool_use=True,
                                  react_mode=ReActMode.REACT
                                  )
        self.tool_meta = [v.get_meta() for _,v in self.tools.items()]

        for mcp in self._param.mcp:
            _, mcp_server = MCPServerService.get_by_id(mcp["mcp_id"])
            tool_call_session = MCPToolCallSession(mcp_server, mcp_server.variables)
            for tnm, meta in mcp["tools"].items():
                self.tool_meta.append(mcp_tool_metadata_to_openai_tool(meta))
                self.tools[tnm] = tool_call_session
        self.callback = partial(self._canvas.tool_use_callback, id.split("-->")[0])
        self.toolcall_session = LLMToolPluginCallSession(self.tools, self.callback)
        #self.chat_mdl.bind_tools(self.toolcall_session, self.tool_metas)

    def get_meta(self) -> dict[str, Any]:
        self._param.function_name= self._id.split("-->")[-1]
        m = super().get_meta()
        if hasattr(self._param, "user_prompt") and self._param.user_prompt:
            m["function"]["parameters"]["properties"]["user_prompt"] = self._param.user_prompt
        return m

    @timeout(os.environ.get("COMPONENT_EXEC_TIMEOUT", 20*60))
    def _invoke(self, **kwargs):
        if kwargs.get("user_prompt"):
            self._param.prompts = [{"role": "user", "content": str(kwargs["user_prompt"])}]

        if not self.tools:
            return LLM._invoke(self, **kwargs)

        prompt, msg = self._prepare_prompt_variables()

        downstreams = self._canvas.get_component(self._id)["downstream"] if self._canvas.get_component(self._id) else []
        if any([self._canvas.get_component_obj(cid).component_name.lower()=="message" for cid in downstreams]) and not self._param.output_structure:
            self.set_output("content", partial(self.stream_output_with_tools, prompt, msg))
            return

        _, msg = message_fit_in([{"role": "system", "content": prompt}, *msg], int(self.chat_mdl.max_length * 0.97))
        use_tools = []
        ans = ""
        for delta_ans, tk in self._react_with_tools_streamly(msg, use_tools):
            ans += delta_ans

        if ans.find("**ERROR**") >= 0:
            logging.error(f"Agent._chat got error. response: {ans}")
            self.set_output("_ERROR", ans)
            return

        self.set_output("content", ans)
        if use_tools:
            self.set_output("use_tools", use_tools)
        return ans

    def stream_output_with_tools(self, prompt, msg):
        _, msg = message_fit_in([{"role": "system", "content": prompt}, *msg], int(self.chat_mdl.max_length * 0.97))
        answer_without_toolcall = ""
        use_tools = []
        for delta_ans,_ in self._react_with_tools_streamly(msg, use_tools):
            answer_without_toolcall += delta_ans
            yield delta_ans

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

    def _gen_citations(self, text):
        retrievals = self._canvas.get_reference()
        retrievals = {"chunks": list(retrievals["chunks"].values()), "doc_aggs": list(retrievals["doc_aggs"].values())}
        formated_refer = kb_prompt(retrievals, self.chat_mdl.max_length, True)
        for delta_ans in self._generate_streamly([{"role": "system", "content": citation_plus(formated_refer)},
                                                  {"role": "user", "content": text}
                                                  ]):
            yield delta_ans, 0

    def _react_with_tools_streamly(self, history: list[dict], use_tools):
        token_count = 0
        tool_metas = self.tool_meta
        hist = deepcopy(history)
        last_calling = ""
        user_request = history[-1]["content"]

        def use_tool(name, args):
            nonlocal hist, use_tools, token_count,last_calling,user_request
            print(f"{last_calling=} == {name=}", )
            # Summarize of function calling
            if all([
                isinstance(self.toolcall_session.get_tool_obj(name), Agent),
                last_calling,
                last_calling != name
            ]):
                self.toolcall_session.get_tool_obj(name).add2system_prompt(f"The chat history with other agents are as following: \n" + self.get_useful_memory(user_request, str(args["user_prompt"])))
            last_calling = name
            tool_response = self.toolcall_session.tool_call(name, args)
            use_tools.append({
                "name": name,
                "arguments": args,
                "results": tool_response
            })
            self.callback("add_memory", {}, "...")
            self.add_memory(hist[-2]["content"], hist[-1]["content"], name, args, str(tool_response))

            return name, tool_response

        self.callback("analyze_task", {}, "...")
        task_desc = analyze_task(self.chat_mdl, user_request, tool_metas)
        for _ in range(self._param.max_rounds + 1):
            response, tk = next_step(self.chat_mdl, hist, tool_metas, task_desc)
            self.callback("next_step", {}, str(response)[:256]+"...")
            token_count += tk
            hist.append({"role": "assistant", "content": response})
            try:
                functions = json_repair.loads(re.sub(r"```.*", "", response))
                if not isinstance(functions, list):
                    raise TypeError(f"List should be returned, but `{functions}`")
                for f in functions:
                    if not isinstance(f, dict):
                        raise TypeError(f"An object type should be returned, but `{f}`")
                with ThreadPoolExecutor(max_workers=5) as executor:
                    thr = []
                    for func in functions:
                        name = func["name"]
                        args = func["arguments"]
                        if name == COMPLETE_TASK:
                            cited = True
                            hist.append({"role": "user", "content": f"Respond with a formal answer. FORGET(DO NOT mention) about `{COMPLETE_TASK}`. The language used in the response MUST be as the same as `{user_request[:32]}`.\n"})
                            if hist[0]["role"] == "system" and self._canvas.get_reference()["chunks"]:
                                if len(hist) < 7:
                                    hist[0]["content"] += citation_prompt()
                                else:
                                    cited = False
                            yield "", token_count
                            entire_txt = ""
                            for delta_ans in self._generate_streamly(hist):
                                if cited:
                                    yield delta_ans, 0
                                entire_txt += delta_ans

                            for delta_ans in self._gen_citations(entire_txt):
                                yield delta_ans, 0

                            yield "", 0
                            return

                        thr.append(executor.submit(use_tool, name, args))

                    reflection = reflect(self.chat_mdl, hist, [th.result() for th in thr])
                    hist.append({"role": "user", "content": reflection})
                    self.callback("reflection", {}, str(reflection)[:256]+"...")

            except Exception as e:
                logging.exception(msg=f"Wrong JSON argument format in LLM ReAct response: {e}")
                hist.append({"role": "user", "content": f"Tool call error, please correct the input parameter of response format and call it again.\n *** Exception ***\n{e}"})

        logging.warning( f"Exceed max rounds: {self._param.max_rounds}")
        if hist[-1]["role"] == "user":
            hist[-1]["content"] += f"\n{user_request}\nBut, Exceed max rounds: {self._param.max_rounds}"
        else:
            hist.append({"role": "user", "content": f"\n{user_request}\nBut, Exceed max rounds: {self._param.max_rounds}"})
        response = self._generate(hist)
        token_count += num_tokens_from_string(response)
        yield response, token_count

    def get_useful_memory(self, goal: str, sub_goal:str, topn=3) -> str:
        self.callback("get_useful_memory", {"topn": 3}, "...")
        mems = self._canvas.get_memory()
        rank = rank_memories(self.chat_mdl, goal, sub_goal, [summ for (user, assist, summ) in mems])
        try:
            rank = json_repair.loads(re.sub(r"```.*", "", rank))[:topn]
            mems = [mems[r] for r in rank]
            return "\n\n".join([f"User: {u}\nAgent: {a}" for u, a,_ in mems])
        except Exception as e:
            logging.exception(e)

        return "Error occurred."
