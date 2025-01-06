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

    def _run(self, history, **kwargs):
        parent = self.get_parent()
        ans = parent.get_input()
        ans = parent._param.delimiter.join(ans["content"]) if "content" in ans else ""
        ans = [a.strip() for a in ans.split(parent._param.delimiter)]
        df = pd.DataFrame([{"content": ans[self._idx]}])
        self._idx += 1
        if self._idx >= len(ans):
            self._idx = -1
        return df

    def end(self):
        return self._idx == -1

