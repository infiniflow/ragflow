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
from contextvars import ContextVar
from enum import Enum

# --- NEW STATE TRACKER ---
class ToolExecutionState(Enum):
    NOT_CALLED = 1
    SUCCESS = 2
    EMPTY_RESULT = 3
    ERROR = 4

# Initialize the context variable with the Enum
_tool_state_tracker = ContextVar("tool_state_tracker", default=ToolExecutionState.NOT_CALLED)

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
    """
    Represents an LLM-driven Agent capable of executing tools, managing conversational context,
    and handling strict validation trapdoors to prevent hallucinated actions.
    """
    component_name = "Agent"

    def __init__(self, canvas, id, param: LLMParam):
        """
        Initialize the Agent component, bind available standard and MCP tools, 
        and set up concurrency-safe tracking callbacks.
        """
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
                
        # --- THE CONCURRENCY FIX & VALIDATION FUNNEL ---
        original_callback = partial(self._canvas.tool_use_callback, id)
        
        # Helper to detect nested empty structures - Unwrapping descriptors)
        def _is_deeply_empty(data):
            if isinstance(data, (bool, int, float)):
                return False
            if not data:
                return True
                
            if isinstance(data, dict):
                # Unwrap common tool descriptors first
                for wrapper_key in ["value", "content", "data", "result", "results"]:
                    if wrapper_key in data:
                        return _is_deeply_empty(data[wrapper_key])
                        
                # If no wrappers, evaluate all values
                return all(_is_deeply_empty(v) for v in data.values())
                
            if isinstance(data, (list, tuple)):
                return all(_is_deeply_empty(v) for v in data)
            if isinstance(data, str):
                return not data.strip()
            return False
        
        def tracking_callback(*args, **kwargs):
            # Ignore internal system calls
            if args and args[0] in ("Multi-turn conversation optimization", "gen_citations"):
                return original_callback(*args, **kwargs)
                
            output = args[2] if len(args) > 2 else kwargs.get("output", None)
            
            # 1. Evaluate THIS specific tool's output state
            this_tool_state = ToolExecutionState.SUCCESS # Default fallback
            
            if output is None:
                this_tool_state = ToolExecutionState.EMPTY_RESULT
            elif isinstance(output, dict):
                if "_ERROR" in output:
                    this_tool_state = ToolExecutionState.ERROR
                elif _is_deeply_empty(output):
                    this_tool_state = ToolExecutionState.EMPTY_RESULT
            elif isinstance(output, list):
                if len(output) == 0 or _is_deeply_empty(output):
                    this_tool_state = ToolExecutionState.EMPTY_RESULT
            elif isinstance(output, str):
                if "**ERROR**" in output or "Unmatched input parameters" in output:
                    this_tool_state = ToolExecutionState.ERROR
                elif not output.strip(): 
                    this_tool_state = ToolExecutionState.EMPTY_RESULT
                    
            # 2. MONOTONIC MERGE: Never let a lower-priority state erase a higher-priority one
            current_state = _tool_state_tracker.get()
            
            if this_tool_state == ToolExecutionState.SUCCESS:
                _tool_state_tracker.set(ToolExecutionState.SUCCESS)
            elif this_tool_state == ToolExecutionState.ERROR and current_state != ToolExecutionState.SUCCESS:
                _tool_state_tracker.set(ToolExecutionState.ERROR)
            elif this_tool_state == ToolExecutionState.EMPTY_RESULT and current_state == ToolExecutionState.NOT_CALLED:
                _tool_state_tracker.set(ToolExecutionState.EMPTY_RESULT)
                        
            return original_callback(*args, **kwargs)
        self.callback = tracking_callback
        self.toolcall_session = LLMToolPluginCallSession(self.tools, self.callback)
        
        if self.tool_meta:
            self.chat_mdl.bind_tools(self.toolcall_session, self.tool_meta)

    def _fit_messages(self, prompt: str, msg: list[dict]) -> list[dict]:
        """
        Truncate or fit messages into the model's maximum context length dynamically.
        """
        _, fitted_messages = message_fit_in(
            [{"role": "system", "content": prompt}, *msg],
            int(self.chat_mdl.max_length * 0.97),
        )
        return fitted_messages

    @staticmethod
    def _append_system_prompt(msg: list[dict], extra_prompt: str) -> None:
        """
        Safely append additional instruction constraints to the active system prompt.
        """
        if extra_prompt and msg and msg[0]["role"] == "system":
            msg[0]["content"] += "\n" + extra_prompt

    @staticmethod
    def _clean_formatted_answer(ans: str) -> str:
        """
        Clean up formatting artifacts, reasoning tags, and markdown code blocks from the raw LLM output.
        """
        ans = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
        ans = re.sub(r"^.*```json", "", ans, flags=re.DOTALL)
        return re.sub(r"```\n*$", "", ans, flags=re.DOTALL)

    def _load_tool_obj(self, cpn: dict) -> object:
        """
        Dynamically load and instantiate a tool component object by its registered name and parameters.
        """
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
        """
        Retrieve the metadata schema for this Agent, including dynamic user prompts.
        """
        self._param.function_name = self._id.split("-->")[-1]
        m = super().get_meta()
        if hasattr(self._param, "user_prompt") and self._param.user_prompt:
            # Keep the JSON schema valid; user_prompt is a string field, not a schema node.
            m["function"]["parameters"]["properties"]["user_prompt"]["default"] = self._param.user_prompt
        return m

    def get_input_form(self) -> dict[str, dict]:
        """
        Generate the input form schema for the Agent component and its associated tools.
        """
        res = {}
        for k, v in self.get_input_elements().items():
            res[k] = {"type": "line", "name": v["name"]}
        for cpn in self._param.tools:
            if not isinstance(cpn, LLM):
                continue
            res.update(cpn.get_input_form())
        return res

    def _get_output_schema(self):
        """
        Extract the structured JSON output schema if one is explicitly defined in the component parameters.
        """
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
        """
        Force the LLM to re-format its previous plain-text response strictly into the requested JSON schema.
        """
        fmt_msgs = [
            {"role": "system", "content": schema_prompt + "\nIMPORTANT: Output ONLY valid JSON. No markdown, no extra text."},
            {"role": "user", "content": text},
        ]
        _, fmt_msgs = message_fit_in(fmt_msgs, int(self.chat_mdl.max_length * 0.97))
        return await self._generate_async(fmt_msgs)

    def _invoke(self, **kwargs):
        """
        Synchronous wrapper for the asynchronous _invoke_async logic.
        """
        return asyncio.run(self._invoke_async(**kwargs))

    
    def _get_tool_execution_state(self) -> ToolExecutionState:
        """
        Safely evaluate the execution state of the invoked tools.
        Relies exclusively on the ContextVar populated by the tracking_callback 
        to ensure we only evaluate current-invocation evidence.
        """
        return _tool_state_tracker.get()

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 20 * 60)))
    async def _invoke_async(self, **kwargs):
        """
        Execute the primary asynchronous agent logic, including prompt formatting, tool invocation,
        and applying state-aware trapdoors to override hallucinated actions.
        """
        if self.check_if_canceled("Agent processing"):
            return

        # STRICT RESET for this async context, retaining token to restore later
        state_token = _tool_state_tracker.set(ToolExecutionState.NOT_CALLED)

        try:
            # Manage prompt scope locally instead of mutating shared instance parameters
            usr_pmt = ""
            if kwargs.get("user_prompt"):
                if kwargs.get("reasoning"):
                    usr_pmt += "\nREASONING:\n{}\n".format(kwargs["reasoning"])
                if kwargs.get("context"):
                    usr_pmt += "\nCONTEXT:\n{}\n".format(kwargs["context"])
                if usr_pmt:
                    usr_pmt += "\nQUERY:\n{}\n".format(str(kwargs["user_prompt"]))
                else:
                    usr_pmt = str(kwargs["user_prompt"])

            if not self.tools:
                if self.check_if_canceled("Agent processing"):
                    return
                
                # Pass the enriched prompt down to the base LLM class locally
                local_kwargs = kwargs.copy()
                if usr_pmt:
                    local_kwargs["user_prompt"] = usr_pmt
                return await LLM._invoke_async(self, **local_kwargs)

            prompt, msg, user_defined_prompt = self._prepare_prompt_variables()
            
            # Inject the locally scoped prompt directly into the message payload
            if usr_pmt:
                msg.append({"role": "user", "content": usr_pmt})

            output_schema = self._get_output_schema()

            component = self._canvas.get_component(self._id)
            downstreams = component["downstream"] if component else []
            ex = self.exception_handler()
            has_message_downstream = any(self._canvas.get_component_obj(cid).component_name.lower() == "message" for cid in downstreams)
            
            if has_message_downstream and not (ex and ex["goto"]) and not output_schema:
                self.set_output("content", partial(self.stream_output_with_tools_async, prompt, deepcopy(msg), user_defined_prompt))
                return

            msg = self._fit_messages(prompt, msg)
            
            # TIGHTENED SAFETY PROMPT
            safety_prompt = (
                "SYSTEM WARNING: You are bound to strict tool validation. "
                "ONLY if you attempt to use a tool and it fails or returns no context, "
                "you MUST NOT invent an answer. You must reply EXACTLY with 'ACTION_NOT_PERFORMED'. "
                "If you are answering a greeting or non-tool query, answer normally."
            )
            self._append_system_prompt(msg, safety_prompt)

            schema_prompt = ""
            if output_schema:
                schema = json.dumps(output_schema, ensure_ascii=False, indent=2)
                schema_prompt = structured_output_prompt(schema)
                self._append_system_prompt(msg, schema_prompt)

            ans = await self._generate_async(msg)
            
            # LAYER 2: ASYNC-SAFE TRAPDOOR
            current_tool_state = self._get_tool_execution_state()
            
            if current_tool_state == ToolExecutionState.EMPTY_RESULT:
                logging.info("Trapdoor triggered: Legitimate empty retrieval, overriding LLM.")
                ans = "ACTION_NOT_PERFORMED"
                
                # 1. ALWAYS broadcast the sentinel to the standard text channel
                # This ensures the business status is available to the system/UI.
                self.set_output("content", ans)
                
                if output_schema:
                    # 2. Send an empty dict to the structured channel 
                    # This maintains the contract for rigid downstream components.
                    self.set_output("structured", {})
                    return {}
                    
                return ans

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
            
        finally:
            # Restore parent context token
            _tool_state_tracker.reset(state_token)

    async def stream_output_with_tools_async(self, prompt, msg, user_defined_prompt={}):
        # STRICT RESET with token retention
        state_token = _tool_state_tracker.set(ToolExecutionState.NOT_CALLED)
        
        try:
            if len(msg) > 3:
                st = timer()
                user_request = await full_question(messages=msg, chat_mdl=self.chat_mdl)
                self.callback("Multi-turn conversation optimization", {}, user_request, elapsed_time=timer() - st)
                msg = [*msg[:-1], {"role": "user", "content": user_request}]

            msg = self._fit_messages(prompt, msg)

            safety_prompt = (
                "SYSTEM WARNING: You are bound to strict tool validation. "
                "ONLY if you attempt to use a tool and it fails or returns no context, "
                "you MUST NOT invent an answer. You must reply EXACTLY with 'ACTION_NOT_PERFORMED'. "
                "If you are answering a greeting or non-tool query, answer normally."
            )
            self._append_system_prompt(msg, safety_prompt)

            need2cite = self._param.cite and self._canvas.get_reference()["chunks"] and self._id.find("-->") < 0
            cited = False
            if need2cite and len(msg) < 7:
                self._append_system_prompt(msg, citation_prompt())
                cited = True

            answer = ""
            trapdoor_fired = False
            
            async for delta in self._generate_streamly(msg):
                if self.check_if_canceled("Agent streaming"):
                    return
                    
                # THE STREAMING TRAPDOOR (In-Loop)
                if self._get_tool_execution_state() == ToolExecutionState.EMPTY_RESULT:
                    if not trapdoor_fired:
                        logging.info("Trapdoor triggered during stream: Empty tool outputs.")
                        yield "ACTION_NOT_PERFORMED"
                        answer = "ACTION_NOT_PERFORMED"
                        self.set_output("content", answer)
                        trapdoor_fired = True
                    return 

                if delta.find("**ERROR**") >= 0:
                    if self.get_exception_default_value():
                        fallback = self.get_exception_default_value()
                        self.set_output("content", fallback)
                        yield fallback
                    else:
                        self.set_output("_ERROR", delta)
                        self.set_output("content", delta)
                        yield delta
                    return
                    
                if not need2cite or cited:
                    yield delta
                answer += delta

            # THE STREAMING TRAPDOOR (Zero-Delta Catch)
            if not trapdoor_fired and self._get_tool_execution_state() == ToolExecutionState.EMPTY_RESULT:
                logging.info("Trapdoor triggered after stream (zero-delta): Empty tool outputs.")
                yield "ACTION_NOT_PERFORMED"
                answer = "ACTION_NOT_PERFORMED"
                self.set_output("content", answer)
                return

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
                    
                # Citation loop protection
                if self._get_tool_execution_state() == ToolExecutionState.EMPTY_RESULT:
                    if not trapdoor_fired:
                        logging.info("Trapdoor triggered during citations stream: Empty tool outputs.")
                        yield "ACTION_NOT_PERFORMED"
                        cited_answer = "ACTION_NOT_PERFORMED"
                        self.set_output("content", cited_answer)
                        trapdoor_fired = True
                    return

                yield delta
                cited_answer += delta

            # Zero-Delta Catch for citations
            if not trapdoor_fired and self._get_tool_execution_state() == ToolExecutionState.EMPTY_RESULT:
                logging.info("Trapdoor triggered after citations stream (zero-delta): Empty tool outputs.")
                yield "ACTION_NOT_PERFORMED"
                cited_answer = "ACTION_NOT_PERFORMED"
                self.set_output("content", cited_answer)
                return
                
            artifact_md = self._collect_tool_artifact_markdown(existing_text=cited_answer)
            if artifact_md:
                yield "\n\n" + artifact_md
                cited_answer += "\n\n" + artifact_md
                
            self.callback("gen_citations", {}, cited_answer, elapsed_time=timer() - st)
            self.set_output("content", cited_answer)
            
        finally:
            # Restore parent context token
            _tool_state_tracker.reset(state_token)

    async def _gen_citations_async(self, text):
        """
        Generate knowledge base citations dynamically by correlating the generated text 
        against the retrieved document chunks.
        """
        retrievals = self._canvas.get_reference()
        retrievals = {"chunks": list(retrievals["chunks"].values()), "doc_aggs": list(retrievals["doc_aggs"].values())}
        formated_refer = kb_prompt(retrievals, self.chat_mdl.max_length, True)
        async for delta_ans in self._generate_streamly([{"role": "system", "content": citation_plus("\n\n".join(formated_refer))}, {"role": "user", "content": text}]):
            yield delta_ans

    def _collect_tool_artifact_markdown(self, existing_text: str = "") -> str:
        """
        Collect any visual or file artifacts (like images or downloadable documents) returned by tools
        and format them cleanly into Markdown links.
        """
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