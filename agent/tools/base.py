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
import time
from copy import deepcopy
import asyncio
from functools import partial
from typing import TypedDict, List, Any
from agent.component.base import ComponentParamBase, ComponentBase
from common.misc_utils import hash_str2int
from rag.prompts.generator import kb_prompt
from common.mcp_tool_call_conn import MCPToolBinding, MCPToolCallSession, ToolCallSession
from timeit import default_timer as timer




from common.misc_utils import thread_pool_exec

class ToolParameter(TypedDict):
    type: str
    description: str
    displayDescription: str
    enum: List[str]
    required: bool


class ToolMeta(TypedDict):
    name: str
    displayName: str
    description: str
    displayDescription: str
    parameters: dict[str, ToolParameter]


class LLMToolPluginCallSession(ToolCallSession):
    def __init__(self, tools_map: dict[str, object], callback: partial):
        self.tools_map = tools_map
        self.callback = callback

    def tool_call(self, name: str, arguments: dict[str, Any], timeout: float | int = 10) -> Any:
        return asyncio.run(self.tool_call_async(name, arguments, timeout))

    async def tool_call_async(self, name: str, arguments: dict[str, Any], timeout: float | int = 10) -> Any:
        assert name in self.tools_map, f"LLM tool {name} does not exist"
        logging.info(f"[ToolCall] invoke name={name} arguments={str(arguments)[:200]}")
        st = timer()
        tool_obj = self.tools_map[name]
        if isinstance(tool_obj, MCPToolBinding):
            resp = await thread_pool_exec(tool_obj.session.tool_call, tool_obj.original_name, arguments, timeout)
        elif isinstance(tool_obj, MCPToolCallSession):
            resp = await thread_pool_exec(tool_obj.tool_call, name, arguments, timeout)
        elif hasattr(tool_obj, "invoke_async") and asyncio.iscoroutinefunction(tool_obj.invoke_async):
            resp = await tool_obj.invoke_async(**arguments)
        else:
            resp = await thread_pool_exec(tool_obj.invoke, **arguments)

        if resp is None and hasattr(tool_obj, "output") and callable(tool_obj.output):
            try:
                fallback_output = tool_obj.output()
                if isinstance(fallback_output, dict) and fallback_output.get("content") not in (None, ""):
                    resp = fallback_output["content"]
                elif fallback_output not in (None, ""):
                    resp = fallback_output
                else:
                    resp = fallback_output
                logging.warning(f"[ToolCall] resp is None, fallback to output name={name} output_keys={list(fallback_output.keys()) if isinstance(fallback_output, dict) else type(fallback_output).__name__}")
            except Exception as e:
                logging.warning(f"[ToolCall] resp is None and output fallback failed name={name} err={e}")

        elapsed = timer() - st
        logging.info(f"[ToolCall] done name={name} elapsed={elapsed:.2f}s result={str(resp)[:200]}")
        self.callback(name, arguments, resp, elapsed_time=elapsed)
        return resp

    def get_tool_obj(self, name):
        return self.tools_map[name]


class ToolParamBase(ComponentParamBase):
    def __init__(self):
        #self.meta:ToolMeta = None
        super().__init__()
        self._init_inputs()
        self._init_attr_by_meta()

    def _init_inputs(self):
        self.inputs = {}
        for k,p in self.meta["parameters"].items():
            self.inputs[k] = deepcopy(p)

    def _init_attr_by_meta(self):
        for k,p in self.meta["parameters"].items():
            if not hasattr(self, k):
                setattr(self, k, p.get("default"))

    def get_meta(self):
        params = {}
        for k, p in self.meta["parameters"].items():
            params[k] = {
                "type": p["type"],
                "description": p["description"]
            }
            if "enum" in p:
                params[k]["enum"] = p["enum"]

        desc = getattr(self, "description", None) or self.meta["description"]
        function_name = getattr(self, "function_name", self.meta["name"])

        return {
            "type": "function",
            "function": {
                "name": function_name,
                "description": desc,
                "parameters": {
                    "type": "object",
                    "properties": params,
                    "required": [k for k, p in self.meta["parameters"].items() if p["required"]]
                }
            }
        }


class ToolBase(ComponentBase):
    def __init__(self, canvas, id, param: ComponentParamBase):
        from agent.canvas import Canvas  # Local import to avoid cyclic dependency

        assert isinstance(canvas, Canvas), "canvas must be an instance of Canvas"
        self._canvas = canvas
        self._id = id
        self._param = param
        self._param.check()

    def get_meta(self) -> dict[str, Any]:
        return self._param.get_meta()

    def invoke(self, **kwargs):
        if self.check_if_canceled("Tool processing"):
            return

        self.set_output("_created_time", time.perf_counter())
        try:
            res = self._invoke(**kwargs)
        except Exception as e:
            self._param.outputs["_ERROR"] = {"value": str(e)}
            logging.exception(e)
            res = str(e)
        self._param.debug_inputs = []

        self.set_output("_elapsed_time", time.perf_counter() - self.output("_created_time"))
        return res

    async def invoke_async(self, **kwargs):
        """
        Async wrapper for tool invocation.
        If `_invoke` is a coroutine, await it directly; otherwise run in a thread to avoid blocking.
        Mirrors the exception handling of `invoke`.
        """
        if self.check_if_canceled("Tool processing"):
            return

        self.set_output("_created_time", time.perf_counter())
        try:
            fn_async = getattr(self, "_invoke_async", None)
            if fn_async and asyncio.iscoroutinefunction(fn_async):
                res = await fn_async(**kwargs)
            elif asyncio.iscoroutinefunction(self._invoke):
                res = await self._invoke(**kwargs)
            else:
                res = await thread_pool_exec(self._invoke, **kwargs)
        except Exception as e:
            self._param.outputs["_ERROR"] = {"value": str(e)}
            logging.exception(e)
            res = str(e)
        self._param.debug_inputs = []

        self.set_output("_elapsed_time", time.perf_counter() - self.output("_created_time"))
        return res

    def _retrieve_chunks(self, res_list: list, get_title, get_url, get_content, get_score=None):
        chunks = []
        aggs = []
        for r in res_list:
            content = get_content(r)
            if not content:
                continue
            content = re.sub(r"!?\[[a-z]+\]\(data:image/png;base64,[ 0-9A-Za-z/_=+-]+\)", "", content)
            content = content[:10000]
            if not content:
                continue
            id = str(hash_str2int(content))
            title = get_title(r)
            url = get_url(r)
            score = get_score(r) if get_score else 1
            chunks.append({
                "chunk_id": id,
                "content": content,
                "doc_id": id,
                "docnm_kwd": title,
                "similarity": score,
                "url": url
            })
            aggs.append({
                "doc_name": title,
                "doc_id": id,
                "count": 1,
                "url": url
            })
        self._canvas.add_reference(chunks, aggs)
        self.set_output("formalized_content", "\n".join(kb_prompt({"chunks": chunks, "doc_aggs": aggs}, 200000, True)))

    def thoughts(self) -> str:
        return self._canvas.get_component_name(self._id) + " is running..."
