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

from copy import deepcopy
from typing import TypedDict, List
from agent.component.base import ComponentParamBase, ComponentBase


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


class ToolParamBase(ComponentParamBase):
    def __init__(self):
        #self.meta:ToolMeta = None
        super().__init__()
        self._init_inputs()

    def _init_inputs(self):
        self.inputs = {}
        for k,p in self.meta["parameters"].items():
            self.inputs[k] = deepcopy(p)
            self.inputs[k]["ref"] = None

    def get_meta(self):
        return {
            "type": "function",
            "function": {
                "name": self.meta["name"],
                "description": self.meta["description"],
                "parameters": {
                    "type": "object",
                    "properties": {
                        k: {
                            "type": p["type"],
                            "description": p["description"]
                        }
                        for k, p in self.meta["parameters"].items()
                    },
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

    async def invoke(self, **kwargs):
        self._param.debug_inputs = []
        for k,p in self._param.inputs.items():
            if not p.get("ref"):
                continue
            kwargs[k] = self._canvas.get_variable_value(p.get("ref"))
        try:
            await self._invoke(**kwargs)
        except Exception as e:
            raise e

        return self.output()
