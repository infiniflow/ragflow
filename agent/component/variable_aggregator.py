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

from typing import Any
import os

from common.connection_utils import timeout
from agent.component.base import ComponentBase, ComponentParamBase


class VariableAggregatorParam(ComponentParamBase):
    """
    Parameters for VariableAggregator

    - groups: list of dicts {"group_name": str, "variables": [variable selectors]}
    """

    def __init__(self):
        super().__init__()
        # each group expects: {"group_name": str, "variables": List[str]}
        self.groups = []

    def check(self):
        self.check_empty(self.groups, "[VariableAggregator] groups")
        for g in self.groups:
            if not g.get("group_name"):
                raise ValueError("[VariableAggregator] group_name can not be empty!")
            if not g.get("variables"):
                raise ValueError(
                    f"[VariableAggregator] variables of group `{g.get('group_name')}` can not be empty"
                )
            if not isinstance(g.get("variables"), list):
                raise ValueError(
                    f"[VariableAggregator] variables of group `{g.get('group_name')}` should be a list of strings"
                )

    def get_input_form(self) -> dict[str, dict]:
        return {
            "variables": {
                "name": "Variables",
                "type": "list",
            }
        }


class VariableAggregator(ComponentBase):
    component_name = "VariableAggregator"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 3)))
    def _invoke(self, **kwargs):
        # Group mode: for each group, pick the first available variable
        for group in self._param.groups:
            gname = group.get("group_name")

            # record candidate selectors within this group
            self.set_input_value(f"{gname}.variables", list(group.get("variables", [])))
            for selector in group.get("variables", []):
                val = self._canvas.get_variable_value(selector['value'])
                if val:
                    self.set_output(gname, val)
                    break
            
    @staticmethod
    def _to_object(value: Any) -> Any:
        # Try to convert value to serializable object if it has to_object()
        try:
            return value.to_object()  # type: ignore[attr-defined]
        except Exception:
            return value

    def thoughts(self) -> str:
        return "Aggregating variables from canvas and grouping as configured."
