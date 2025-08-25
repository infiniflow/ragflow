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
import logging
import os
from abc import ABC
from strenum import StrEnum
from typing import Optional
from pydantic import BaseModel, Field, field_validator
from agent.tools.base import ToolParamBase, ToolBase, ToolMeta
from api import settings
from api.utils.api_utils import timeout


class Language(StrEnum):
    PYTHON = "python"
    NODEJS = "nodejs"


class CodeExecutionRequest(BaseModel):
    code_b64: str = Field(..., description="Base64 encoded code string")
    language: str = Field(default=Language.PYTHON.value, description="Programming language")
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


class CodeExecParam(ToolParamBase):
    """
    Define the code sandbox component parameters.
    """

    def __init__(self):
        self.meta:ToolMeta = {
            "name": "execute_code",
            "description": """
This tool has a sandbox that can execute code written in 'Python'/'Javascript'. It recieves a piece of code and return a Json string.
Here's a code example for Python(`main` function MUST be included):
def main() -> dict:
    \"\"\"
    Generate Fibonacci numbers within 100.
    \"\"\"
    def fibonacci_recursive(n):
        if n <= 1:
            return n
        else:
            return fibonacci_recursive(n-1) + fibonacci_recursive(n-2)
    return {
        "result": fibonacci_recursive(100),
    }

Here's a code example for Javascript(`main` function MUST be included and exported):
const axios = require('axios');
async function main(args) {
  try {
    const response = await axios.get('https://github.com/infiniflow/ragflow');
    console.log('Body:', response.data);
  } catch (error) {
    console.error('Error:', error.message);
  }
}
module.exports = { main };
            """,
            "parameters": {
                "lang": {
                    "type": "string",
                    "description": "The programming language of this piece of code.",
                    "enum": ["python", "javascript"],
                    "required": True,
                },
                "script": {
                    "type": "string",
                    "description": "A piece of code in right format. There MUST be main function.",
                    "required": True
                }
            }
        }
        super().__init__()
        self.lang = Language.PYTHON.value
        self.script = "def main(arg1: str, arg2: str) -> dict: return {\"result\": arg1 + arg2}"
        self.arguments = {}
        self.outputs = {"result": {"value": "", "type": "string"}}

    def check(self):
        self.check_valid_value(self.lang, "Support languages", ["python", "python3", "nodejs", "javascript"])
        self.check_empty(self.script, "Script")

    def get_input_form(self) -> dict[str, dict]:
        res = {}
        for k, v in self.arguments.items():
            res[k] = {
                "type": "line",
                "name": k
            }
        return res


class CodeExec(ToolBase, ABC):
    component_name = "CodeExec"

    @timeout(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60))
    def _invoke(self, **kwargs):
        lang = kwargs.get("lang", self._param.lang)
        script = kwargs.get("script", self._param.script)
        arguments = {}
        for k, v in self._param.arguments.items():
            if kwargs.get(k):
                arguments[k] = kwargs[k]
                continue
            arguments[k] = self._canvas.get_variable_value(v) if v else None

        self._execute_code(
            language=lang,
            code=script,
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
            resp = requests.post(url=f"http://{settings.SANDBOX_HOST}:9385/run", json=code_req, timeout=os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60))
            logging.info(f"http://{settings.SANDBOX_HOST}:9385/run", code_req, resp.status_code)
            if resp.status_code != 200:
                resp.raise_for_status()
            body = resp.json()
            if body:
                stderr = body.get("stderr")
                if stderr:
                    self.set_output("_ERROR", stderr)
                    return
                try:
                    rt = eval(body.get("stdout", ""))
                except Exception:
                    rt = body.get("stdout", "")
                logging.info(f"http://{settings.SANDBOX_HOST}:9385/run -> {rt}")
                if isinstance(rt, tuple):
                    for i, (k, o) in enumerate(self._param.outputs.items()):
                        if k.find("_") == 0:
                            continue
                        o["value"] = rt[i]
                elif isinstance(rt, dict):
                    for i, (k, o) in enumerate(self._param.outputs.items()):
                        if k not in rt or k.find("_") == 0:
                            continue
                        o["value"] = rt[k]
                else:
                    for i, (k, o) in enumerate(self._param.outputs.items()):
                        if k.find("_") == 0:
                            continue
                        o["value"] = rt
            else:
                self.set_output("_ERROR", "There is no response from sandbox")

        except Exception as e:
            self.set_output("_ERROR", "Exception executing code: " + str(e))

        return self.output()

    def _encode_code(self, code: str) -> str:
        return base64.b64encode(code.encode("utf-8")).decode("utf-8")

    def thoughts(self) -> str:
        return "Running a short script to process data."
