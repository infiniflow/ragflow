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
import json
import os
import re
from abc import ABC
from enum import Enum
from functools import partial
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
        self.split_ref = ""
        self.delimiters = [","]
        self.outputs = {"result": {"value": "", "type": "string"}}

    def check(self):
        self.check_valid_value(self.method, "Support method", ["split", "merge"])
        self.check_empty(self.delimiters, "delimiters")

    def get_input_form(self) -> dict[str, dict]:
        if self._param.method == "split":
            return {
                "line": {
                    "name": "String",
                    "type": "line"
                }
            }
        return {
            "script": {
                "name": "Script",
                "type": "paragraph"
            }
        }

class StringTransform(ComponentBase, ABC):
    component_name = "StringTransform"

    @timeout(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60))
    def _invoke(self, **kwargs):
        if self._param.method == "split":
            self._split(kwargs.get("line"))
        else:
            self._merge(kwargs.get("script"))

    def _split(self, line:str=None):
        var = self._canvas.get_variable_value(self._param.split_ref) if not line else line
        if not var:
            var = ""
        assert isinstance(var, str), "The input variable is not a string: {}".format(type(var))
        self.set_input_value(self._param.split_ref, var)
        res = []
        for i,s in enumerate(re.split(r"(%s)"%("|".join([re.escape(d) for d in self._param.delimiters])), var, flags=re.DOTALL)):
            if i % 2 == 1:
                continue
            res.append(s)
        self.set_output("result", res)

    def _merge(self, script:str=None):
        s = 0
        all_content = ""
        cache = {}
        script = self._param.script if not script else script
        for r in re.finditer(self.variable_ref_patt, script, flags=re.DOTALL):
            all_content += script[s: r.start()]
            s = r.end()
            exp = r.group(1)
            if exp in cache:
                all_content += cache[exp]
                continue
            v = self._canvas.get_variable_value(exp)
            if isinstance(v, partial):
                cnt = ""
                for t in v():
                    all_content += t
                    cnt += t
                cache[exp] = cnt
            elif isinstance(v, list):
                v = self._param.delimiters[0].join([str(_v) for _v in v])
                all_content += v
                cache[exp] = v
            else:
                if not isinstance(v, str):
                    try:
                        v = json.dumps(v, ensure_ascii=False)
                    except Exception:
                        pass
                all_content += v
                cache[exp] = v

        if s < len(script):
            all_content += script[s: ]

        for k, v in cache.items():
            self.set_input_value(k ,v)

        self.set_output("result", all_content)


