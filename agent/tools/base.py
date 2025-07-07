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
import time
from copy import deepcopy
from functools import partial
from typing import TypedDict, List, Any
from agent.component.base import ComponentParamBase, ComponentBase
from rag.llm.chat_model import ToolCallSession


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

    def tool_call(self, name: str, arguments: dict[str, Any]) -> Any:
        assert name in self.tools_map, f"LLM tool {name} does not exist"
        self.callback(name, arguments)
        return self.tools_map[name].invoke(**arguments)


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

        desc = self.meta["description"]
        if hasattr(self, "description"):
            desc = self.description

        function_name = self.meta["name"]
        if hasattr(self, "function_name"):
            function_name = self.function_name

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
        self._param.debug_inputs = []
        print(kwargs, "#############################")

        self.set_output("_created_time", time.perf_counter())
        try:
            self._invoke(**kwargs)
        except Exception as e:
            self._param.outputs["_ERROR"] = {"value": str(e)}
            logging.exception(e)

        self.set_output("_elapsed_time", time.perf_counter() - self.output("_created_time"))
        return self.output()
