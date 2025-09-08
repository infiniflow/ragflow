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
import numbers
import os
from abc import ABC
from typing import Any

from agent.component.base import ComponentBase, ComponentParamBase
from api.utils.api_utils import timeout


class SwitchParam(ComponentParamBase):
    """
    Define the Switch component parameters.
    """

    def __init__(self):
        super().__init__()
        """
        {
            "logical_operator" : "and | or"
            "items" : [
                            {"cpn_id": "categorize:0", "operator": "contains", "value": ""},
                            {"cpn_id": "categorize:0", "operator": "contains", "value": ""},...],
            "to": ""
        }
        """
        self.conditions = []
        self.end_cpn_ids = []
        self.operators = ['contains', 'not contains', 'start with', 'end with', 'empty', 'not empty', '=', '≠', '>',
                          '<', '≥', '≤']

    def check(self):
        self.check_empty(self.conditions, "[Switch] conditions")
        for cond in self.conditions:
            if not cond["to"]:
                raise ValueError("[Switch] 'To' can not be empty!")
        self.check_empty(self.end_cpn_ids, "[Switch] the ELSE/Other destination can not be empty.")

    def get_input_form(self) -> dict[str, dict]:
        return {
            "urls": {
                "name": "URLs",
                "type": "line"
            }
        }

class Switch(ComponentBase, ABC):
    component_name = "Switch"

    @timeout(os.environ.get("COMPONENT_EXEC_TIMEOUT", 3))
    def _invoke(self, **kwargs):
        for cond in self._param.conditions:
            res = []
            for item in cond["items"]:
                if not item["cpn_id"]:
                    continue
                cpn_v = self._canvas.get_variable_value(item["cpn_id"])
                self.set_input_value(item["cpn_id"], cpn_v)
                operatee = item.get("value", "")
                if isinstance(cpn_v, numbers.Number):
                    operatee = float(operatee)
                res.append(self.process_operator(cpn_v, item["operator"], operatee))
                if cond["logical_operator"] != "and" and any(res):
                    self.set_output("next", [self._canvas.get_component_name(cpn_id) for cpn_id in cond["to"]])
                    self.set_output("_next", cond["to"])
                    return

            if all(res):
                self.set_output("next", [self._canvas.get_component_name(cpn_id) for cpn_id in cond["to"]])
                self.set_output("_next", cond["to"])
                return

        self.set_output("next", [self._canvas.get_component_name(cpn_id) for cpn_id in self._param.end_cpn_ids])
        self.set_output("_next", self._param.end_cpn_ids)

    def process_operator(self, input: Any, operator: str, value: Any) -> bool:
        if operator == "contains":
            return True if value.lower() in input.lower() else False
        elif operator == "not contains":
            return True if value.lower() not in input.lower() else False
        elif operator == "start with":
            return True if input.lower().startswith(value.lower()) else False
        elif operator == "end with":
            return True if input.lower().endswith(value.lower()) else False
        elif operator == "empty":
            return True if not input else False
        elif operator == "not empty":
            return True if input else False
        elif operator == "=":
            return True if input == value else False
        elif operator == "≠":
            return True if input != value else False
        elif operator == ">":
            try:
                return True if float(input) > float(value) else False
            except Exception:
                return True if input > value else False
        elif operator == "<":
            try:
                return True if float(input) < float(value) else False
            except Exception:
                return True if input < value else False
        elif operator == "≥":
            try:
                return True if float(input) >= float(value) else False
            except Exception:
                return True if input >= value else False
        elif operator == "≤":
            try:
                return True if float(input) <= float(value) else False
            except Exception:
                return True if input <= value else False

        raise ValueError('Not supported operator' + operator)

    def thoughts(self) -> str:
        return "I’m weighing a few options and will pick the next step shortly."