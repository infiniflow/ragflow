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
from functools import partial
import pandas as pd
from agent.component.base import ComponentBase, ComponentParamBase


class BeginParam(ComponentParamBase):

    """
    Define the Begin component parameters.
    """
    def __init__(self):
        super().__init__()
        self.prologue = "Hi! I'm your smart assistant. What can I do for you?"
        self.query = []

    def check(self):
        return True


class Begin(ComponentBase):
    component_name = "Begin"

    def _run(self, history, **kwargs):
        if kwargs.get("stream"):
            return partial(self.stream_output)
        return pd.DataFrame([{"content": self._param.prologue}])

    def stream_output(self):
        res = {"content": self._param.prologue}
        yield res
        self.set_output(self.be_output(res))



