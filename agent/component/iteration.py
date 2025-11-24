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

"""
class VariableModel(BaseModel):
    data_type: Annotated[Literal["string", "number", "Object", "Boolean", "Array<string>", "Array<number>", "Array<object>", "Array<boolean>"], Field(default="Array<string>")]
    input_mode: Annotated[Literal["constant", "variable"], Field(default="constant")]
    value: Annotated[Any, Field(default=None)]
    model_config = ConfigDict(extra="forbid")
"""

class IterationParam(ComponentParamBase):
    """
    Define the Iteration component parameters.
    """

    def __init__(self):
        super().__init__()
        self.items_ref = ""
        self.variable={}

    def get_input_form(self) -> dict[str, dict]:
        return {
            "items": {
                "type": "json",
                "name": "Items"
            }
        }

    def check(self):
        return True


class Iteration(ComponentBase, ABC):
    component_name = "Iteration"

    def get_start(self):
        for cid in self._canvas.components.keys():
            if self._canvas.get_component(cid)["obj"].component_name.lower() != "iterationitem":
                continue
            if self._canvas.get_component(cid)["parent_id"] == self._id:
                return cid

    def _invoke(self, **kwargs):
        if self.check_if_canceled("Iteration processing"):
            return

        arr = self._canvas.get_variable_value(self._param.items_ref)
        if not isinstance(arr, list):
            self.set_output("_ERROR", self._param.items_ref + " must be an array, but its type is "+str(type(arr)))

    def thoughts(self) -> str:
        return "Need to process {} items.".format(len(self._canvas.get_variable_value(self._param.items_ref)))



