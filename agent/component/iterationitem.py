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


class IterationItemParam(ComponentParamBase):
    """
    Define the IterationItem component parameters.
    """
    def check(self):
        return True


class IterationItem(ComponentBase, ABC):
    component_name = "IterationItem"

    def __init__(self, canvas, id, param: ComponentParamBase):
        super().__init__(canvas, id, param)
        self._idx = 0

    def _invoke(self, **kwargs):
        parent = self.get_parent()
        arr = self._canvas.get_variable_value(parent._param.items_ref)
        if not isinstance(arr, list):
            self._idx = -1
            raise Exception(parent._param.items_ref + " must be an array, but its type is "+str(type(arr)))

        if self._idx > 0:
            self.output_collation()

        if self._idx >= len(arr):
            self._idx = -1
            return

        self.set_output("item", arr[self._idx])
        self.set_output("index", self._idx)

        self._idx += 1

    def output_collation(self):
        pid = self.get_parent()._id
        for cid in self._canvas.components.keys():
            obj = self._canvas.get_component_obj(cid)
            p = obj.get_parent()
            if not p:
                continue
            if p._id != pid:
                continue

            if p.component_name.lower() in ["categorize", "message", "switch", "userfillup", "interationitem"]:
                continue

            for k, o in p._param.outputs.items():
                if "ref" not in o:
                    continue
                _cid, var = o["ref"].split("@")
                if _cid != cid:
                    continue
                res = p.output(k)
                if not res:
                    res = []
                res.append(obj.output(var))
                p.set_output(k, res)

    def end(self):
        return self._idx == -1

    def thoughts(self) -> str:
        return "Next turn..."