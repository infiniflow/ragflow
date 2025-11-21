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


class LoopItemParam(ComponentParamBase):
    """
    Define the LoopItem component parameters.
    """
    def check(self):
        return True
    
class LoopItem(ComponentBase, ABC):
    component_name = "LoopItem"

    def __init__(self, canvas, id, param: ComponentParamBase):
        super().__init__(canvas, id, param)
        self._idx = 0
        
        
    def _invoke(self, **kwargs):
        if self.check_if_canceled("LoopItem processing"):
            return
        parent = self.get_parent()
        maximum_loop_count = parent._param.maximum_loop_count
        if self._idx >= maximum_loop_count:
            self._idx = -1
            return
        if self._idx > 0:
            if self.check_if_canceled("LoopItem processing"):
                return
        self._idx += 1

    def evaluate_condition(self,var, operator, value):
        if isinstance(var, str):
            if operator == "contains":
                return value in var
            elif operator == "not contains":
                return value not in var
            elif operator == "start with":
                return var.startswith(value)
            elif operator == "end with":
                return var.endswith(value)
            elif operator == "is":
                return var == value
            elif operator == "is not":
                return var != value
            elif operator == "is empty":
                return var == ""
            elif operator == "is not empty":
                return var != ""

        elif isinstance(var, (int, float)):
            if operator == "=":
                return var == value
            elif operator == "!=":
                return var != value
            elif operator == ">":
                return var > value
            elif operator == "<":
                return var < value
            elif operator == ">=":
                return var >= value
            elif operator == "<=":
                return var <= value
            elif operator == "is empty":
                return var is None
            elif operator == "is not empty":
                return var is not None

        elif isinstance(var, bool):
            if operator == "is":
                return var is value
            elif operator == "is not":
                return var is not value
            elif operator == "is empty":
                return var is None
            elif operator == "is not empty":
                return var is not None

        elif isinstance(var, dict):
            if operator == "is empty":
                return len(var) == 0
            elif operator == "is not empty":
                return len(var) > 0

        elif isinstance(var, list):
            if operator == "contains":
                return value in var
            elif operator == "not contains":
                return value not in var

            elif operator == "is":
                return var == value
            elif operator == "is not":
                return var != value

            elif operator == "is empty":
                return len(var) == 0
            elif operator == "is not empty":
                return len(var) > 0

        raise Exception(f"Invalid operator: {operator}")

    def end(self):
        if self._idx == -1:
            return True
        parent = self.get_parent()

        for item in parent._param.loop_termination_condition:
            if any([not item.get("variable"),not item.get("operator")]):
                assert "Loop Condition is not complete."
            var = self.canvas.get_variable(item["variable"])
            operator = item["operator"]
            input_mode = getattr(item, "input_mode", "Constant")
            if input_mode == "Varibale":
                value = self.canvas.get_variable(getattr(item, "value",""))
            elif input_mode == "Constant":
                value = getattr(item, "value","")
            else:
                assert "Invalid input mode."
            if self.evaluate_condition(var, operator, value):
                return True
        return False

    def next(self):
        if self._idx == -1:
            self._idx = 0
        else:
            self._idx += 1
            if self._idx >= len(self._items):
                self._idx = -1
        return False

    def thoughts(self) -> str:
        return "Next turn..."