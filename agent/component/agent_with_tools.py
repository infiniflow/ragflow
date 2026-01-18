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
import asyncio
import json
import logging
import os
import re
from copy import deepcopy
from functools import partial
from typing import Any

import json_repair
from timeit import default_timer as timer
from agent.tools.base import LLMToolPluginCallSession, ToolParamBase, ToolBase, ToolMeta
from api.db.services.llm_service import LLMBundle
from api.db.services.tenant_llm_service import TenantLLMService
from api.db.services.mcp_server_service import MCPServerService
from common.connection_utils import timeout
from rag.prompts.generator import next_step_async, COMPLETE_TASK, \
    citation_prompt, kb_prompt, citation_plus, full_question, message_fit_in, structured_output_prompt
from common.mcp_tool_call_conn import MCPToolCallSession, mcp_tool_metadata_to_openai_tool
from agent.component.llm import LLMParam, LLM


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
                    },
                    "reasoning": {
                        "type": "string",
                        "description": (
                            "Supervisor's reasoning for choosing the this agent. "
                            "Explain why this agent is being invoked and what is expected of it."
                        ),
                        "required": True
                    },
                    "context": {
                        "type": "string",
                        "description": (
                                "All relevant background information, prior facts, decisions, "
                                "and state needed by the agent to solve the current query. "
                                "Should be as detailed and self-contained as possible."
                            ),
                        "required": True
                    },
                }
            }
        super().__init__()
        self.function_name = "agent"
        self.tools = []
        self.mcp = []
        self.max_rounds = 5
        self.description = ""
        self.custom_header = {}



class Agent(LLM, ToolBase):
    component_name = "Agent"

    def __init__(self, canvas, id, param: LLMParam):
        LLM.__init__(self, canvas, id, param)
        self.tools = {}
        for idx, cpn in enumerate(self._param.tools):
            cpn = self._load_tool_obj(cpn)
            original_name = cpn.get_meta()["function"]["name"]
            indexed_name = f"{original_name}_{idx}"
            self.tools[indexed_name] = cpn

        self.chat_mdl = LLMBundle(self._canvas.get_tenant_id(), TenantLLMService.llm_id2llm_type(self._param.llm_id), self._param.llm_id,
                                  max_retries=self._param.max_retries,
                                  retry_interval=self._param.delay_after_error,
                                  max_rounds=self._param.max_rounds,
                                  verbose_tool_use=True
                                  )
        self.tool_meta = []
        for indexed_name, tool_obj in self.tools.items():
            original_meta = tool_obj.get_meta()
            indexed_meta = deepcopy(original_meta)
            indexed_meta["function"]["name"] = indexed_name
            self.tool_meta.append(indexed_meta)

        for mcp in self._param.mcp:
            _, mcp_server = MCPServerService.get_by_id(mcp["mcp_id"])
            custom_header = self._param.custom_header
            tool_call_session = MCPToolCallSession(mcp_server, mcp_server.variables, custom_header)
            for tnm, meta in mcp["tools"].items():
                self.tool_meta.append(mcp_tool_metadata_to_openai_tool(meta))
                self.tools[tnm] = tool_call_session
        self.callback = partial(self._canvas.tool_use_callback, id)
        self.toolcall_session = LLMToolPluginCallSession(self.tools, self.callback)
        #self.chat_mdl.bind_tools(self.toolcall_session, self.tool_metas)

    def _load_tool_obj(self, cpn: dict) -> object:
        from agent.component import component_class
        tool_name = cpn["component_name"]
        param = component_class(tool_name + "Param")()
        param.update(cpn["params"])
        try:
            param.check()
        except Exception as e:
            self.set_output("_ERROR", cpn["component_name"] + f" configuration error: {e}")
            raise
        cpn_id = f"{self._id}-->" + cpn.get("name", "").replace(" ", "_")
        return component_class(cpn["component_name"])(self._canvas, cpn_id, param)

    def get_meta(self) -> dict[str, Any]:
        self._param.function_name= self._id.split("-->")[-1]
        m = super().get_meta()
        if hasattr(self._param, "user_prompt") and self._param.user_prompt:
            m["function"]["parameters"]["properties"]["user_prompt"] = self._param.user_prompt
        return m

    def get_input_form(self) -> dict[str, dict]:
        res = {}
        for k, v in self.get_input_elements().items():
            res[k] = {
                "type": "line",
                "name": v["name"]
            }
        for cpn in self._param.tools:
            if not isinstance(cpn, LLM):
                continue
            res.update(cpn.get_input_form())
        return res

    def _get_output_schema(self):
        try:
            cand = self._param.outputs.get("structured")
        except Exception:
            return None

        if isinstance(cand, dict):
            if isinstance(cand.get("properties"), dict) and len(cand["properties"]) > 0:
                return cand
            for k in ("schema", "structured"):
                if isinstance(cand.get(k), dict) and isinstance(cand[k].get("properties"), dict) and len(cand[k]["properties"]) > 0:
                    return cand[k]

        return None

    async def _force_format_to_schema_async(self, text: str, schema_prompt: str) -> str:
        fmt_msgs = [
            {"role": "system", "content": schema_prompt + "\nIMPORTANT: Output ONLY valid JSON. No markdown, no extra text."},
            {"role": "user", "content": text},
        ]
        _, fmt_msgs = message_fit_in(fmt_msgs, int(self.chat_mdl.max_length * 0.97))
        return await self._generate_async(fmt_msgs)

    def _invoke(self, **kwargs):
        return asyncio.run(self._invoke_async(**kwargs))

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 20*60)))
    async def _invoke_async(self, **kwargs):
        if self.check_if_canceled("Agent processing"):
            return

        if kwargs.get("user_prompt"):
            usr_pmt = ""
            if kwargs.get("reasoning"):
                usr_pmt += "\nREASONING:\n{}\n".format(kwargs["reasoning"])
            if kwargs.get("context"):
                usr_pmt += "\nCONTEXT:\n{}\n".format(kwargs["context"])
            if usr_pmt:
                usr_pmt += "\nQUERY:\n{}\n".format(str(kwargs["user_prompt"]))
            else:
                usr_pmt = str(kwargs["user_prompt"])
            self._param.prompts = [{"role": "user", "content": usr_pmt}]

        if not self.tools:
            if self.check_if_canceled("Agent processing"):
                return
            return await LLM._invoke_async(self, **kwargs)

        prompt, msg, user_defined_prompt = self._prepare_prompt_variables()
        output_schema = self._get_output_schema()
        schema_prompt = ""
        if output_schema:
            schema = json.dumps(output_schema, ensure_ascii=False, indent=2)
            schema_prompt = structured_output_prompt(schema)

        downstreams = self._canvas.get_component(self._id)["downstream"] if self._canvas.get_component(self._id) else []
        ex = self.exception_handler()
        if any([self._canvas.get_component_obj(cid).component_name.lower()=="message" for cid in downstreams]) and not (ex and ex["goto"]) and not output_schema:
            self.set_output("content", partial(self.stream_output_with_tools_async, prompt, deepcopy(msg), user_defined_prompt))
            return

        _, msg = message_fit_in([{"role": "system", "content": prompt}, *msg], int(self.chat_mdl.max_length * 0.97))
        use_tools = []
        ans = ""
        async for delta_ans, _tk in self._react_with_tools_streamly_async_simple(prompt, msg, use_tools, user_defined_prompt,schema_prompt=schema_prompt):
            if self.check_if_canceled("Agent processing"):
                return
            ans += delta_ans

        if ans.find("**ERROR**") >= 0:
            logging.error(f"Agent._chat got error. response: {ans}")
            if self.get_exception_default_value():
                self.set_output("content", self.get_exception_default_value())
            else:
                self.set_output("_ERROR", ans)
            return

        if output_schema:
            error = ""
            for _ in range(self._param.max_retries + 1):
                try:
                    def clean_formated_answer(ans: str) -> str:
                        ans = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
                        ans = re.sub(r"^.*```json", "", ans, flags=re.DOTALL)
                        return re.sub(r"```\n*$", "", ans, flags=re.DOTALL)
                    obj = json_repair.loads(clean_formated_answer(ans))
                    self.set_output("structured", obj)
                    if use_tools:
                        self.set_output("use_tools", use_tools)
                    return obj
                except Exception:
                    error = "The answer cannot be parsed as JSON"
                    ans = await self._force_format_to_schema_async(ans, schema_prompt)
                    if ans.find("**ERROR**") >= 0:
                        continue

            self.set_output("_ERROR", error)
            return

        self.set_output("content", ans)
        if use_tools:
            self.set_output("use_tools", use_tools)
        return ans

    async def stream_output_with_tools_async(self, prompt, msg, user_defined_prompt={}):
        _, msg = message_fit_in([{"role": "system", "content": prompt}, *msg], int(self.chat_mdl.max_length * 0.97))
        answer_without_toolcall = ""
        use_tools = []
        async for delta_ans, _ in self._react_with_tools_streamly_async_simple(prompt, msg, use_tools, user_defined_prompt):
            if self.check_if_canceled("Agent streaming"):
                return

            if delta_ans.find("**ERROR**") >= 0:
                if self.get_exception_default_value():
                    self.set_output("content", self.get_exception_default_value())
                    yield self.get_exception_default_value()
                else:
                    self.set_output("_ERROR", delta_ans)
                    return
            answer_without_toolcall += delta_ans
            yield delta_ans

        self.set_output("content", answer_without_toolcall)
        if use_tools:
            self.set_output("use_tools", use_tools)

    async def _react_with_tools_streamly_async_simple(self, prompt, history: list[dict], use_tools, user_defined_prompt={}, schema_prompt: str = ""):
        token_count = 0
        tool_metas = self.tool_meta
        hist = deepcopy(history)
        last_calling = ""
        if len(hist) > 3:
            st = timer()
            user_request = await full_question(messages=history, chat_mdl=self.chat_mdl)
            self.callback("Multi-turn conversation optimization", {}, user_request, elapsed_time=timer()-st)
        else:
            user_request = history[-1]["content"]

        def build_task_desc(prompt: str, user_request: str, user_defined_prompt: dict | None = None) -> str:
            """Build a minimal task_desc by concatenating prompt, query, and tool schemas."""
            user_defined_prompt = user_defined_prompt or {}

            task_desc = (
                "### Agent Prompt\n"
                f"{prompt}\n\n"
                "### User Request\n"
                f"{user_request}\n\n"
            )

            if user_defined_prompt:
                udp_json = json.dumps(user_defined_prompt, ensure_ascii=False, indent=2)
                task_desc += "\n### User Defined Prompts\n" + udp_json + "\n"

            return task_desc


        async def use_tool_async(name, args):
            nonlocal hist, use_tools, last_calling
            logging.info(f"{last_calling=} == {name=}")
            last_calling = name
            tool_response = await self.toolcall_session.tool_call_async(name, args)
            use_tools.append({
                "name": name,
                "arguments": args,
                "results": tool_response
            })
            return name, tool_response

        async def complete():
            nonlocal hist
            need2cite = self._param.cite and self._canvas.get_reference()["chunks"] and self._id.find("-->") < 0
            if schema_prompt:
                need2cite = False
            cited = False
            if hist and hist[0]["role"] == "system":
                if schema_prompt:
                    hist[0]["content"] += "\n" + schema_prompt
                if need2cite and len(hist) < 7:
                    hist[0]["content"] += citation_prompt()
                    cited = True
            yield "", token_count

            _hist = hist
            if len(hist) > 12:
                _hist = [hist[0], hist[1], *hist[-10:]]
            entire_txt = ""
            async for delta_ans in self._generate_streamly(_hist):
                if not need2cite or cited:
                    yield delta_ans, 0
                entire_txt += delta_ans
            if not need2cite or cited:
                return

            st = timer()
            txt = ""
            async for delta_ans in self._gen_citations_async(entire_txt):
                if self.check_if_canceled("Agent streaming"):
                    return
                yield delta_ans, 0
                txt += delta_ans

            self.callback("gen_citations", {}, txt, elapsed_time=timer()-st)

        def build_observation(tool_call_res: list[tuple]) -> str:
            """
            Build a Observation from tool call results.
            No LLM involved.
            """
            if not tool_call_res:
                return ""

            lines = ["Observation:"]
            for name, result in tool_call_res:
                lines.append(f"[{name} result]")
                lines.append(str(result))

            return "\n".join(lines)

        def append_user_content(hist, content):
            if hist[-1]["role"] == "user":
                hist[-1]["content"] += content
            else:
                hist.append({"role": "user", "content": content})

        st = timer()
        task_desc = build_task_desc(prompt, user_request, user_defined_prompt)
        self.callback("analyze_task", {}, task_desc, elapsed_time=timer()-st)
        for _ in range(self._param.max_rounds + 1):
            if self.check_if_canceled("Agent streaming"):
                return
            response, tk = await next_step_async(self.chat_mdl, hist, tool_metas, task_desc, user_defined_prompt)
            # self.callback("next_step", {}, str(response)[:256]+"...")
            token_count += tk or 0
            hist.append({"role": "assistant", "content": response})
            try:
                functions = json_repair.loads(re.sub(r"```.*", "", response))
                if not isinstance(functions, list):
                    raise TypeError(f"List should be returned, but `{functions}`")
                for f in functions:
                    if not isinstance(f, dict):
                        raise TypeError(f"An object type should be returned, but `{f}`")

                tool_tasks = []
                for func in functions:
                    name = func["name"]
                    args = func["arguments"]
                    if name == COMPLETE_TASK:
                        append_user_content(hist, f"Respond with a formal answer. FORGET(DO NOT mention) about `{COMPLETE_TASK}`. The language for the response MUST be as the same as the first user request.\n")
                        async for txt, tkcnt in complete():
                            yield txt, tkcnt
                        return

                    tool_tasks.append(asyncio.create_task(use_tool_async(name, args)))

                results = await asyncio.gather(*tool_tasks) if tool_tasks else []
                st = timer()
                reflection = build_observation(results)
                append_user_content(hist, reflection)
                self.callback("reflection", {}, str(reflection), elapsed_time=timer()-st)

            except Exception as e:
                logging.exception(msg=f"Wrong JSON argument format in LLM ReAct response: {e}")
                e = f"\nTool call error, please correct the input parameter of response format and call it again.\n *** Exception ***\n{e}"
                append_user_content(hist, str(e))

        logging.warning( f"Exceed max rounds: {self._param.max_rounds}")
        final_instruction = f"""
{user_request}
IMPORTANT: You have reached the conversation limit. Based on ALL the information and research you have gathered so far, please provide a DIRECT and COMPREHENSIVE final answer to the original request.
Instructions:
1. SYNTHESIZE all information collected during this conversation
2. Provide a COMPLETE response using existing data - do not suggest additional research
3. Structure your response as a FINAL DELIVERABLE, not a plan
4. If information is incomplete, state what you found and provide the best analysis possible with available data
5. DO NOT mention conversation limits or suggest further steps
6. Focus on delivering VALUE with the information already gathered
Respond immediately with your final comprehensive answer.
        """
        if self.check_if_canceled("Agent final instruction"):
            return
        append_user_content(hist, final_instruction)

        async for txt, tkcnt in complete():
            yield txt, tkcnt

#     async def _react_with_tools_streamly_async(self, prompt, history: list[dict], use_tools, user_defined_prompt={}, schema_prompt: str = ""):
#         token_count = 0
#         tool_metas = self.tool_meta
#         hist = deepcopy(history)
#         last_calling = ""
#         if len(hist) > 3:
#             st = timer()
#             user_request = await full_question(messages=history, chat_mdl=self.chat_mdl)
#             self.callback("Multi-turn conversation optimization", {}, user_request, elapsed_time=timer()-st)
#         else:
#             user_request = history[-1]["content"]

#         async def use_tool_async(name, args):
#             nonlocal hist, use_tools, last_calling
#             logging.info(f"{last_calling=} == {name=}")
#             last_calling = name
#             tool_response = await self.toolcall_session.tool_call_async(name, args)
#             use_tools.append({
#                 "name": name,
#                 "arguments": args,
#                 "results": tool_response
#             })
#             # self.callback("add_memory", {}, "...")
#             #self.add_memory(hist[-2]["content"], hist[-1]["content"], name, args, str(tool_response), user_defined_prompt)

#             return name, tool_response

#         async def complete():
#             nonlocal hist
#             need2cite = self._param.cite and self._canvas.get_reference()["chunks"] and self._id.find("-->") < 0
#             if schema_prompt:
#                 need2cite = False
#             cited = False
#             if hist and hist[0]["role"] == "system":
#                 if schema_prompt:
#                     hist[0]["content"] += "\n" + schema_prompt
#                 if need2cite and len(hist) < 7:
#                     hist[0]["content"] += citation_prompt()
#                     cited = True
#             yield "", token_count

#             _hist = hist
#             if len(hist) > 12:
#                 _hist = [hist[0], hist[1], *hist[-10:]]
#             entire_txt = ""
#             async for delta_ans in self._generate_streamly(_hist):
#                 if not need2cite or cited:
#                     yield delta_ans, 0
#                 entire_txt += delta_ans
#             if not need2cite or cited:
#                 return

#             st = timer()
#             txt = ""
#             async for delta_ans in self._gen_citations_async(entire_txt):
#                 if self.check_if_canceled("Agent streaming"):
#                     return
#                 yield delta_ans, 0
#                 txt += delta_ans

#             self.callback("gen_citations", {}, txt, elapsed_time=timer()-st)

#         def append_user_content(hist, content):
#             if hist[-1]["role"] == "user":
#                 hist[-1]["content"] += content
#             else:
#                 hist.append({"role": "user", "content": content})

#         st = timer()
#         task_desc = await analyze_task_async(self.chat_mdl, prompt, user_request, tool_metas, user_defined_prompt)
#         self.callback("analyze_task", {}, task_desc, elapsed_time=timer()-st)
#         for _ in range(self._param.max_rounds + 1):
#             if self.check_if_canceled("Agent streaming"):
#                 return
#             response, tk = await next_step_async(self.chat_mdl, hist, tool_metas, task_desc, user_defined_prompt)
#             # self.callback("next_step", {}, str(response)[:256]+"...")
#             token_count += tk or 0
#             hist.append({"role": "assistant", "content": response})
#             try:
#                 functions = json_repair.loads(re.sub(r"```.*", "", response))
#                 if not isinstance(functions, list):
#                     raise TypeError(f"List should be returned, but `{functions}`")
#                 for f in functions:
#                     if not isinstance(f, dict):
#                         raise TypeError(f"An object type should be returned, but `{f}`")

#                 tool_tasks = []
#                 for func in functions:
#                     name = func["name"]
#                     args = func["arguments"]
#                     if name == COMPLETE_TASK:
#                         append_user_content(hist, f"Respond with a formal answer. FORGET(DO NOT mention) about `{COMPLETE_TASK}`. The language for the response MUST be as the same as the first user request.\n")
#                         async for txt, tkcnt in complete():
#                             yield txt, tkcnt
#                         return

#                     tool_tasks.append(asyncio.create_task(use_tool_async(name, args)))

#                 results = await asyncio.gather(*tool_tasks) if tool_tasks else []
#                 st = timer()
#                 reflection = await reflect_async(self.chat_mdl, hist, results, user_defined_prompt)
#                 append_user_content(hist, reflection)
#                 self.callback("reflection", {}, str(reflection), elapsed_time=timer()-st)

#             except Exception as e:
#                 logging.exception(msg=f"Wrong JSON argument format in LLM ReAct response: {e}")
#                 e = f"\nTool call error, please correct the input parameter of response format and call it again.\n *** Exception ***\n{e}"
#                 append_user_content(hist, str(e))

#         logging.warning( f"Exceed max rounds: {self._param.max_rounds}")
#         final_instruction = f"""
# {user_request}
# IMPORTANT: You have reached the conversation limit. Based on ALL the information and research you have gathered so far, please provide a DIRECT and COMPREHENSIVE final answer to the original request.
# Instructions:
# 1. SYNTHESIZE all information collected during this conversation
# 2. Provide a COMPLETE response using existing data - do not suggest additional research
# 3. Structure your response as a FINAL DELIVERABLE, not a plan
# 4. If information is incomplete, state what you found and provide the best analysis possible with available data
# 5. DO NOT mention conversation limits or suggest further steps
# 6. Focus on delivering VALUE with the information already gathered
# Respond immediately with your final comprehensive answer.
#         """
#         if self.check_if_canceled("Agent final instruction"):
#             return
#         append_user_content(hist, final_instruction)

#         async for txt, tkcnt in complete():
#             yield txt, tkcnt

    async def _gen_citations_async(self, text):
        retrievals = self._canvas.get_reference()
        retrievals = {"chunks": list(retrievals["chunks"].values()), "doc_aggs": list(retrievals["doc_aggs"].values())}
        formated_refer = kb_prompt(retrievals, self.chat_mdl.max_length, True)
        async for delta_ans in self._generate_streamly([{"role": "system", "content": citation_plus("\n\n".join(formated_refer))},
                                                  {"role": "user", "content": text}
                                                  ]):
            yield delta_ans

    def reset(self, only_output=False):
        """
        Reset all tools if they have a reset method. This avoids errors for tools like MCPToolCallSession.
        """
        for k in self._param.outputs.keys():
            self._param.outputs[k]["value"] = None

        for k, cpn in self.tools.items():
            if hasattr(cpn, "reset") and callable(cpn.reset):
                cpn.reset()
        if only_output:
            return
        for k in self._param.inputs.keys():
            self._param.inputs[k]["value"] = None
        self._param.debug_inputs = {}
