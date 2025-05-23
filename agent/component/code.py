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


class CodeParam(ComponentParamBase):
    """
    Define the code sandbox component parameters.
    """

    def __init__(self):
        super().__init__()
        self.lang = "python"
        self.script = ""
        self.arguments = []
        self.address = f"http://{settings.SANDBOX_HOST}:9385/run"
        self.enable_network = True

    def check(self):
        self.check_valid_value(self.lang, "Support languages", ["python", "python3", "nodejs", "javascript"])
        self.check_defined_type(self.enable_network, "Enable network", ["bool"])


class Code(ComponentBase, ABC):
    component_name = "Code"

    def _run(self, history, **kwargs):
        arguments = {}
        for input in self._param.arguments:
            if "@" in input["component_id"]:
                component_id = input["component_id"].split("@")[0]
                refered_component_key = input["component_id"].split("@")[1]
                refered_component = self._canvas.get_component(component_id)["obj"]

                for param in refered_component._param.query:
                    if param["key"] == refered_component_key:
                        if "value" in param:
                            arguments[input["name"]] = param["value"]
            else:
                cpn = self._canvas.get_component(input["component_id"])["obj"]
                if cpn.component_name.lower() == "answer":
                    arguments[input["name"]] = self._canvas.get_history(1)[0]["content"]
                    continue
                _, out = cpn.output(allow_partial=False)
                if not out.empty:
                    arguments[input["name"]] = "\n".join(out["content"])

        return self._execute_code(
            language=self._param.lang,
            code=self._param.script,
            arguments=arguments,
            address=self._param.address,
            enable_network=self._param.enable_network,
        )

    def _execute_code(self, language: str, code: str, arguments: dict, address: str, enable_network: bool):
        import requests

        try:
            code_b64 = self._encode_code(code)
            code_req = CodeExecutionRequest(code_b64=code_b64, language=language, arguments=arguments).model_dump()
        except Exception as e:
            return Code.be_output("**Error**: construct code request error: " + str(e))

        try:
            resp = requests.post(url=address, json=code_req, timeout=10)
            body = resp.json()
            if body:
                stdout = body.get("stdout")
                stderr = body.get("stderr")
                return Code.be_output(stdout or stderr)
            else:
                return Code.be_output("**Error**: There is no response from sanbox")

        except Exception as e:
            return Code.be_output("**Error**: Internal error in sanbox: " + str(e))

    def _encode_code(self, code: str) -> str:
        return base64.b64encode(code.encode("utf-8")).decode("utf-8")

    def get_input_elements(self):
        elements = []
        for input in self._param.arguments:
            cpn_id = input["component_id"]
            elements.append({"key": cpn_id, "name": input["name"]})
        return elements
