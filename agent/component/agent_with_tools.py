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
from timeit import default_timer as timer
from typing import Any

import json_repair

from agent.component.llm import LLM, LLMParam
from agent.tools.base import LLMToolPluginCallSession, ToolBase, ToolMeta, ToolParamBase
from api.db.joint_services.tenant_model_service import get_model_config_by_type_and_name
from api.db.services.llm_service import LLMBundle
from api.db.services.mcp_server_service import MCPServerService
from api.db.services.tenant_llm_service import TenantLLMService
from common.connection_utils import timeout
from common.mcp_tool_call_conn import MCPToolBinding, MCPToolCallSession, mcp_tool_metadata_to_openai_tool
from rag.prompts.generator import citation_plus, citation_prompt, full_question, kb_prompt, message_fit_in, structured_output_prompt


class AgentParam(LLMParam, ToolParamBase):
    """
    Define the Agent component parameters.
    """

    def __init__(self):
        self.meta: ToolMeta = {
            "name": "agent",
            "description": "This is an agent for a specific task.",
            "parameters": {
                "user_prompt": {"type": "string", "description": "This is the order you need to send to the agent.", "default": "", "required": True},
                "reasoning": {
                    "type": "string",
                    "description": ("Supervisor's reasoning for choosing the this agent. Explain why this agent is being invoked and what is expected of it."),
                    "required": True,
                },
                "context": {
                    "type": "string",
                    "description": (
                        "All relevant background information, prior facts, decisions, and state needed by the agent to solve the current query. Should be as detailed and self-contained as possible."
                    ),
                    "required": True,
                },
            },
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
        chat_model_config = get_model_config_by_type_and_name(self._canvas.get_tenant_id(), TenantLLMService.llm_id2llm_type(self._param.llm_id), self._param.llm_id)
        self.chat_mdl = LLMBundle(
            self._canvas.get_tenant_id(),
            chat_model_config,
            max_retries=self._param.max_retries,
            retry_interval=self._param.delay_after_error,
            max_rounds=self._param.max_rounds,
            verbose_tool_use=False,
        )
        self.tool_meta = []
        for indexed_name, tool_obj in self.tools.items():
            original_meta = tool_obj.get_meta()
            indexed_meta = deepcopy(original_meta)
            indexed_meta["function"]["name"] = indexed_name
            self.tool_meta.append(indexed_meta)

        tool_idx = len(self.tools)
        for mcp in self._param.mcp:
            _, mcp_server = MCPServerService.get_by_id(mcp["mcp_id"])
            custom_header = self._param.custom_header
            tool_call_session = MCPToolCallSession(mcp_server, mcp_server.variables, custom_header)
            for tnm, meta in mcp["tools"].items():
                indexed_name = f"{tnm}_{tool_idx}"
                tool_idx += 1
                self.tool_meta.append(mcp_tool_metadata_to_openai_tool(meta, function_name=indexed_name))
                self.tools[indexed_name] = MCPToolBinding(tool_call_session, tnm)
        self.callback = partial(self._canvas.tool_use_callback, id)
        self.toolcall_session = LLMToolPluginCallSession(self.tools, self.callback)
        if self.tool_meta:
            self.chat_mdl.bind_tools(self.toolcall_session, self.tool_meta)

    def _fit_messages(self, prompt: str, msg: list[dict]) -> list[dict]:
        _, fitted_messages = message_fit_in(
            [{"role": "system", "content": prompt}, *msg],
            int(self.chat_mdl.max_length * 0.97),
        )
        return fitted_messages

    @staticmethod
    def _append_system_prompt(msg: list[dict], extra_prompt: str) -> None:
        if extra_prompt and msg and msg[0]["role"] == "system":
            msg[0]["content"] += "\n" + extra_prompt

    @staticmethod
    def _clean_formatted_answer(ans: str) -> str:
        ans = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
        ans = re.sub(r"^.*```json", "", ans, flags=re.DOTALL)
        return re.sub(r"```\n*$", "", ans, flags=re.DOTALL)

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
        self._param.function_name = self._id.split("-->")[-1]
        m = super().get_meta()
        if hasattr(self._param, "user_prompt") and self._param.user_prompt:
            # Keep the JSON schema valid; user_prompt is a string field, not a schema node.
            m["function"]["parameters"]["properties"]["user_prompt"]["default"] = self._param.user_prompt
        return m

    def get_input_form(self) -> dict[str, dict]:
        res = {}
        for k, v in self.get_input_elements().items():
            res[k] = {"type": "line", "name": v["name"]}
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

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 20 * 60)))
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

        component = self._canvas.get_component(self._id)
        downstreams = component["downstream"] if component else []
        ex = self.exception_handler()
        has_message_downstream = any(self._canvas.get_component_obj(cid).component_name.lower() == "message" for cid in downstreams)
        if has_message_downstream and not (ex and ex["goto"]) and not output_schema:
            self.set_output("content", partial(self.stream_output_with_tools_async, prompt, deepcopy(msg), user_defined_prompt))
            return

        msg = self._fit_messages(prompt, msg)
        self._append_system_prompt(msg, schema_prompt)
        ans = await self._generate_async(msg)

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
                    obj = json_repair.loads(self._clean_formatted_answer(ans))
                    self.set_output("structured", obj)
                    return obj
                except Exception:
                    error = "The answer cannot be parsed as JSON"
                    ans = await self._force_format_to_schema_async(ans, schema_prompt)
                    if ans.find("**ERROR**") >= 0:
                        continue

            self.set_output("_ERROR", error)
            return

        artifact_md = self._collect_tool_artifact_markdown(existing_text=ans)
        if artifact_md:
            ans += "\n\n" + artifact_md
        self.set_output("content", ans)
        return ans

    async def stream_output_with_tools_async(self, prompt, msg, user_defined_prompt={}):
        if len(msg) > 3:
            st = timer()
            user_request = await full_question(messages=msg, chat_mdl=self.chat_mdl)
            self.callback("Multi-turn conversation optimization", {}, user_request, elapsed_time=timer() - st)
            msg = [*msg[:-1], {"role": "user", "content": user_request}]

        msg = self._fit_messages(prompt, msg)

        need2cite = self._param.cite and self._canvas.get_reference()["chunks"] and self._id.find("-->") < 0
        cited = False
        if need2cite and len(msg) < 7:
            self._append_system_prompt(msg, citation_prompt())
            cited = True

        answer = ""
        async for delta in self._generate_streamly(msg):
            if self.check_if_canceled("Agent streaming"):
                return
            if delta.find("**ERROR**") >= 0:
                if self.get_exception_default_value():
                    self.set_output("content", self.get_exception_default_value())
                    yield self.get_exception_default_value()
                else:
                    self.set_output("_ERROR", delta)
                return
            if not need2cite or cited:
                yield delta
            answer += delta

        if not need2cite or cited:
            artifact_md = self._collect_tool_artifact_markdown(existing_text=answer)
            if artifact_md:
                yield "\n\n" + artifact_md
                answer += "\n\n" + artifact_md
            self.set_output("content", answer)
            return

        st = timer()
        cited_answer = ""
        async for delta in self._gen_citations_async(answer):
            if self.check_if_canceled("Agent streaming"):
                return
            yield delta
            cited_answer += delta
        artifact_md = self._collect_tool_artifact_markdown(existing_text=cited_answer)
        if artifact_md:
            yield "\n\n" + artifact_md
            cited_answer += "\n\n" + artifact_md
        self.callback("gen_citations", {}, cited_answer, elapsed_time=timer() - st)
        self.set_output("content", cited_answer)

    async def _gen_citations_async(self, text):
        retrievals = self._canvas.get_reference()
        retrievals = {"chunks": list(retrievals["chunks"].values()), "doc_aggs": list(retrievals["doc_aggs"].values())}
        formated_refer = kb_prompt(retrievals, self.chat_mdl.max_length, True)
        async for delta_ans in self._generate_streamly([{"role": "system", "content": citation_plus("\n\n".join(formated_refer))}, {"role": "user", "content": text}]):
            yield delta_ans

    def _collect_tool_artifact_markdown(self, existing_text: str = "") -> str:
        md_parts = []
        for tool_obj in self.tools.values():
            if not hasattr(tool_obj, "_param") or not hasattr(tool_obj._param, "outputs"):
                continue
            artifacts_meta = tool_obj._param.outputs.get("_ARTIFACTS", {})
            artifacts = artifacts_meta.get("value") if isinstance(artifacts_meta, dict) else None
            if not artifacts:
                continue
            for art in artifacts:
                if not isinstance(art, dict):
                    continue
                url = art.get("url", "")
                if url and (f"![]({url})" in existing_text or f"![{art.get('name', '')}]({url})" in existing_text):
                    continue
                if art.get("mime_type", "").startswith("image/"):
                    md_parts.append(f"![{art['name']}]({url})")
                else:
                    md_parts.append(f"[Download {art['name']}]({url})")
        return "\n\n".join(md_parts)

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
