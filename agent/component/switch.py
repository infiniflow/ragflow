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

import pandas as pd
from agent.component.base import ComponentBase, ComponentParamBase


class SwitchParam(ComponentParamBase):

    """
    Define the Switch component parameters.
    """
    def __init__(self):
        super().__init__()
        """
        {
            "cpn_id": "categorize:0",
            "not": False,
            "operator": "gt/gte/lt/lte/eq/in",
            "value": "",
            "to": ""
        }
        """
        self.conditions = []
        self.default = ""

    def check(self):
        self.check_empty(self.conditions, "[Switch] conditions")
        self.check_empty(self.default, "[Switch] Default path")
        for cond in self.conditions:
            if not cond["to"]: raise ValueError(f"[Switch] 'To' can not be empty!")

    def operators(self, field, op, value):
        if op == "gt":
            return float(field) > float(value)
        if op == "gte":
            return float(field) >= float(value)
        if op == "lt":
            return float(field) < float(value)
        if op == "lte":
            return float(field) <= float(value)
        if op == "eq":
            return str(field) == str(value)
        if op == "in":
            return str(field).find(str(value)) >= 0
        return False


class Switch(ComponentBase, ABC):
    component_name = "Switch"

    def _run(self, history, **kwargs):
        for cond in self._param.conditions:
            input = self._canvas.get_component(cond["cpn_id"])["obj"].output()[1]
            if self._param.operators(input.iloc[0, 0], cond["operator"], cond["value"]):
                if not cond["not"]:
                    return pd.DataFrame([{"content": cond["to"]}])

        return pd.DataFrame([{"content": self._param.default}])




