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
from abc import ABC
from agent.component.base import ComponentBase, ComponentParamBase


class LoopParam(ComponentParamBase):
    """
    Define the Loop component parameters.
    """

    def __init__(self):
        super().__init__()
        self.loop_variables = []
        self.loop_termination_condition=[]
        self.maximum_loop_count = 0

    def get_input_form(self) -> dict[str, dict]:
        return {
            "items": {
                "type": "json",
                "name": "Items"
            }
        }

    def check(self):
        return True


class Loop(ComponentBase, ABC):
    component_name = "Loop"

    def get_start(self):
        for cid in self._canvas.components.keys():
            if self._canvas.get_component(cid)["obj"].component_name.lower() != "loopitem":
                continue
            if self._canvas.get_component(cid)["parent_id"] == self._id:
                return cid

    def _invoke(self, **kwargs):
        if self.check_if_canceled("Loop processing"):
            return

        for item in self._param.loop_variables:
            if any([not item.get("variable"), not item.get("input_mode"), not item.get("value"),not item.get("type")]):
                assert "Loop Variable is not complete."
            if item["input_mode"]=="variable":
                self.set_output(item["variable"],self._canvas.get_variable_value(item["value"]))
            elif item["input_mode"]=="constant":
                self.set_output(item["variable"],item["value"])
            else:
                if item["type"] == "number":
                    self.set_output(item["variable"], 0)
                elif item["type"] == "string":
                    self.set_output(item["variable"], "")
                elif item["type"] == "boolean":
                    self.set_output(item["variable"], False)
                elif item["type"].startswith("object"):
                    self.set_output(item["variable"], {})
                elif item["type"].startswith("array"):
                    self.set_output(item["variable"], [])
                else:
                    self.set_output(item["variable"], "")


    def thoughts(self) -> str:
        return "Loop from canvas."