#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
import base64
import os
import re
from abc import ABC
from enum import Enum
from typing import Optional

import json_repair
from jinja2 import StrictUndefined
from jinja2.sandbox import SandboxedEnvironment
from pydantic import BaseModel, Field, field_validator
from agent.component.base import ComponentBase, ComponentParamBase
from api import settings
from api.utils.api_utils import timeout


class StringTransformParam(ComponentParamBase):
    """
    Define the code sandbox component parameters.
    """

    def __init__(self):
        super().__init__()
        self.method = "split"
        self.script = ""
        self.delimiters = [","]
        self.outputs = {"result": {"value": "", "type": "string"}}

    def check(self):
        self.check_valid_value(self.method, "Support method", ["split", "merge"])
        self.check_empty(self.delimiters, "delimiters")


class StringTransform(ComponentBase, ABC):
    component_name = "StringTransform"

    @timeout(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60))
    def _invoke(self, **kwargs):
        if self._param.method == "split":
            self._split()
        else:
            self._transform()

    def _split(self):
        _, obj = self._param.inputs.items()[0]
        if obj.get("value"):
            self.set_output("result", re.split(r"(%s)"%("|".join([re.escape(d) for d in self._param.delimiters])), obj["value"], flags=re.DOTALL))
            return

        var = self._canvas.get_variable_value(obj["ref"])
        assert isinstance(var, str), "The input variable is not a string: {}".format(type(var))

    def _transform(self):
        param = {}
        for k, obj in self._param.inputs.items():
            if obj.get("value"):
                param[k] = obj["value"]
                continue
            param[k] = self._canvas.get_variable_value(obj["ref"])
            if isinstance(param[k], list):
                param[k] = self._param.delimiters[0].join([str(s) for s in param[k]])

        env = SandboxedEnvironment(
            autoescape=True,
            undefined=StrictUndefined,
        )

        template = env.from_string(self._param.script)
        try:
            content = template.render(param)
            self.set_output("result", content)
        except Exception as e:
            self.set_output("_ERROR", str(e))


