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
import random
from abc import ABC
from functools import partial
from typing import Tuple, Union

import pandas as pd

from agent.component.base import ComponentBase, ComponentParamBase


class AnswerParam(ComponentParamBase):

    """
    Define the Answer component parameters.
    """
    def __init__(self):
        super().__init__()
        self.post_answers = []

    def check(self):
        return True


class Answer(ComponentBase, ABC):
    component_name = "Answer"

    def _run(self, history, **kwargs):
        if kwargs.get("stream"):
            return partial(self.stream_output)

        ans = self.get_input()
        if self._param.post_answers:
            ans = pd.concat([ans, pd.DataFrame([{"content": random.choice(self._param.post_answers)}])], ignore_index=False)
        return ans

    def stream_output(self):
        res = None
        if hasattr(self, "exception") and self.exception:
            res = {"content": str(self.exception)}
            self.exception = None
            yield res
            self.set_output(res)
            return

        stream = self.get_stream_input()
        if isinstance(stream, pd.DataFrame):
            res = stream
            answer = ""
            for ii, row in stream.iterrows():
                answer += row.to_dict()["content"]
                yield {"content": answer}
        else:
            for st in stream():
                res = st
                yield st
        if self._param.post_answers:
            res["content"] += random.choice(self._param.post_answers)
            yield res

        self.set_output(res)

    def set_exception(self, e):
        self.exception = e

    def output(self, allow_partial=True) -> Tuple[str, Union[pd.DataFrame, partial]]:
        if allow_partial:
            return super.output()

        for r, c in self._canvas.history[::-1]:
            if r == "user":
                return self._param.output_var_name, pd.DataFrame([{"content": c}])

        self._param.output_var_name, pd.DataFrame([])

