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
from abc import ABC
from enum import Enum
from typing import Optional

import json_repair
from pydantic import BaseModel, Field, field_validator
from agent.component.base import ComponentBase, ComponentParamBase
from api import settings


class Language(str, Enum):
    PYTHON = "python"
    NODEJS = "nodejs"


class CodeExecutionRequest(BaseModel):
    code_b64: str = Field(..., description="Base64 encoded code string")
    language: Language = Field(default=Language.PYTHON, description="Programming language")
    arguments: Optional[dict] = Field(default={}, description="Arguments")

    @field_validator("code_b64")
    @classmethod
    def validate_base64(cls, v: str) -> str:
        try:
            base64.b64decode(v, validate=True)
            return v
        except Exception as e:
            raise ValueError(f"Invalid base64 encoding: {str(e)}")

    @field_validator("language", mode="before")
    @classmethod
    def normalize_language(cls, v) -> str:
        if isinstance(v, str):
            low = v.lower()
            if low in ("python", "python3"):
                return "python"
            elif low in ("javascript", "nodejs"):
                return "nodejs"
        raise ValueError(f"Unsupported language: {v}")


class CodeExecParam(ComponentParamBase):
    """
    Define the code sandbox component parameters.
    """

    def __init__(self):
        super().__init__()
        self.lang = "python"
        self.script = "def main(arg1: str, arg2: str) -> dict: return {\"result\": arg1 + arg2}"
        self.arguments = {"arg1": None, "arg2": None}
        self.outputs = {"result": {"value": "", "type": "string"}}

    def check(self):
        self.check_valid_value(self.lang, "Support languages", ["python", "python3", "nodejs", "javascript"])
        self.check_defined_type(self.enable_network, "Enable network", ["bool"])


class CodeExec(ComponentBase, ABC):
    component_name = "CodeExec"

    async def _invoke(self, **kwargs):
        arguments = {}
        for k, v in self._param.arguments.items():
            arguments[k] = self._canvas.get_variable_value(v) if v else None

        self._execute_code(
            language=self._param.lang,
            code=self._param.script,
            arguments=arguments
        )

    def _execute_code(self, language: str, code: str, arguments: dict):
        import requests

        try:
            code_b64 = self._encode_code(code)
            code_req = CodeExecutionRequest(code_b64=code_b64, language=language, arguments=arguments).model_dump()
        except Exception as e:
            self.set_output("_ERROR", "construct code request error: " + str(e))

        try:
            resp = requests.post(url=f"http://{settings.SANDBOX_HOST}:9385/run", json=code_req, timeout=10)
            body = resp.json()
            if body:
                stderr = body.get("stderr")
                if stderr:
                    self.set_output("_ERROR", stderr)
                    return

                for k,v in json_repair.loads(body.get("stdout")).items():
                    if k not in self._param.outputs:
                        continue
                    self.set_output(k ,v)
            else:
                self.set_output("_ERROR", "There is no response from sanbox")

        except Exception as e:
            self.set_output("_ERROR", "construct code request error: " + str(e))

    def _encode_code(self, code: str) -> str:
        return base64.b64encode(code.encode("utf-8")).decode("utf-8")


