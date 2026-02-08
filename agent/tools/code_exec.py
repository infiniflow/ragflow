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
import ast
import base64
import json
import logging
import os
from abc import ABC
from typing import Optional

from pydantic import BaseModel, Field, field_validator
from strenum import StrEnum

from agent.tools.base import ToolBase, ToolMeta, ToolParamBase
from common import settings
from common.connection_utils import timeout


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
        self.meta: ToolMeta = {
            "name": "execute_code",
            "description": """
This tool has a sandbox that can execute code written in 'Python'/'Javascript'. It receives a piece of code and return a Json string.
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
                "script": {"type": "string", "description": "A piece of code in right format. There MUST be main function.", "required": True},
            },
        }
        super().__init__()
        self.lang = Language.PYTHON.value
        self.script = 'def main(arg1: str, arg2: str) -> dict: return {"result": arg1 + arg2}'
        self.arguments = {}
        self.outputs = {"result": {"value": "", "type": "object"}}

    def check(self):
        self.check_valid_value(self.lang, "Support languages", ["python", "python3", "nodejs", "javascript"])
        self.check_empty(self.script, "Script")

    def get_input_form(self) -> dict[str, dict]:
        res = {}
        for k, v in self.arguments.items():
            res[k] = {"type": "line", "name": k}
        return res


class CodeExec(ToolBase, ABC):
    component_name = "CodeExec"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10 * 60)))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("CodeExec processing"):
            return

        lang = kwargs.get("lang", self._param.lang)
        script = kwargs.get("script", self._param.script)
        arguments = {}
        for k, v in self._param.arguments.items():
            if kwargs.get(k):
                arguments[k] = kwargs[k]
                continue
            arguments[k] = self._canvas.get_variable_value(v) if v else None

        return self._execute_code(language=lang, code=script, arguments=arguments)

    def _execute_code(self, language: str, code: str, arguments: dict):
        import requests

        if self.check_if_canceled("CodeExec execution"):
            return self.output()

        try:
            # Try using the new sandbox provider system first
            try:
                from agent.sandbox.client import execute_code as sandbox_execute_code

                if self.check_if_canceled("CodeExec execution"):
                    return

                # Execute code using the provider system
                result = sandbox_execute_code(
                    code=code,
                    language=language,
                    timeout=int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10 * 60)),
                    arguments=arguments
                )

                if self.check_if_canceled("CodeExec execution"):
                    return

                # Process the result
                if result.stderr:
                    self.set_output("_ERROR", result.stderr)
                    return

                parsed_stdout = self._deserialize_stdout(result.stdout)
                logging.info(f"[CodeExec]: Provider system -> {parsed_stdout}")
                self._populate_outputs(parsed_stdout, result.stdout)
                return

            except (ImportError, RuntimeError) as provider_error:
                # Provider system not available or not configured, fall back to HTTP
                logging.info(f"[CodeExec]: Provider system not available, using HTTP fallback: {provider_error}")

            # Fallback to direct HTTP request
            code_b64 = self._encode_code(code)
            code_req = CodeExecutionRequest(code_b64=code_b64, language=language, arguments=arguments).model_dump()
        except Exception as e:
            if self.check_if_canceled("CodeExec execution"):
                return self.output()

            self.set_output("_ERROR", "construct code request error: " + str(e))
            return self.output()

        try:
            if self.check_if_canceled("CodeExec execution"):
                self.set_output("_ERROR", "Task has been canceled")
                return self.output()

            resp = requests.post(url=f"http://{settings.SANDBOX_HOST}:9385/run", json=code_req, timeout=int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10 * 60)))
            logging.info(f"http://{settings.SANDBOX_HOST}:9385/run,  code_req: {code_req}, resp.status_code {resp.status_code}:")

            if self.check_if_canceled("CodeExec execution"):
                return "Task has been canceled"

            if resp.status_code != 200:
                resp.raise_for_status()
            body = resp.json()
            if body:
                stderr = body.get("stderr")
                if stderr:
                    self.set_output("_ERROR", stderr)
                    return self.output()
                raw_stdout = body.get("stdout", "")
                parsed_stdout = self._deserialize_stdout(raw_stdout)
                logging.info(f"[CodeExec]: http://{settings.SANDBOX_HOST}:9385/run -> {parsed_stdout}")
                self._populate_outputs(parsed_stdout, raw_stdout)
            else:
                self.set_output("_ERROR", "There is no response from sandbox")
                return self.output()

        except Exception as e:
            if self.check_if_canceled("CodeExec execution"):
                return self.output()

            self.set_output("_ERROR", "Exception executing code: " + str(e))

        return self.output()

    def _encode_code(self, code: str) -> str:
        return base64.b64encode(code.encode("utf-8")).decode("utf-8")

    def thoughts(self) -> str:
        return "Running a short script to process data."

    def _deserialize_stdout(self, stdout: str):
        text = str(stdout).strip()
        if not text:
            return ""
        for loader in (json.loads, ast.literal_eval):
            try:
                return loader(text)
            except Exception:
                continue
        return text

    def _coerce_output_value(self, value, expected_type: Optional[str]):
        if expected_type is None:
            return value

        etype = expected_type.strip().lower()
        inner_type = None
        if etype.startswith("array<") and etype.endswith(">"):
            inner_type = etype[6:-1].strip()
            etype = "array"

        try:
            if etype == "string":
                return "" if value is None else str(value)

            if etype == "number":
                if value is None or value == "":
                    return None
                if isinstance(value, (int, float)):
                    return value
                if isinstance(value, str):
                    try:
                        return float(value)
                    except Exception:
                        return value
                return float(value)

            if etype == "boolean":
                if isinstance(value, bool):
                    return value
                if isinstance(value, str):
                    lv = value.lower()
                    if lv in ("true", "1", "yes", "y", "on"):
                        return True
                    if lv in ("false", "0", "no", "n", "off"):
                        return False
                return bool(value)

            if etype == "array":
                candidate = value
                if isinstance(candidate, str):
                    parsed = self._deserialize_stdout(candidate)
                    candidate = parsed
                if isinstance(candidate, tuple):
                    candidate = list(candidate)
                if not isinstance(candidate, list):
                    candidate = [] if candidate is None else [candidate]

                if inner_type == "string":
                    return ["" if v is None else str(v) for v in candidate]
                if inner_type == "number":
                    coerced = []
                    for v in candidate:
                        try:
                            if v is None or v == "":
                                coerced.append(None)
                            elif isinstance(v, (int, float)):
                                coerced.append(v)
                            else:
                                coerced.append(float(v))
                        except Exception:
                            coerced.append(v)
                    return coerced
                return candidate

            if etype == "object":
                if isinstance(value, dict):
                    return value
                if isinstance(value, str):
                    parsed = self._deserialize_stdout(value)
                    if isinstance(parsed, dict):
                        return parsed
                return value
        except Exception:
            return value

        return value

    def _populate_outputs(self, parsed_stdout, raw_stdout: str):
        outputs_items = list(self._param.outputs.items())
        logging.info(f"[CodeExec]: outputs schema keys: {[k for k, _ in outputs_items]}")
        if not outputs_items:
            return

        if isinstance(parsed_stdout, dict):
            for key, meta in outputs_items:
                if key.startswith("_"):
                    continue
                val = self._get_by_path(parsed_stdout, key)
                if val is None and len(outputs_items) == 1:
                    val = parsed_stdout
                coerced = self._coerce_output_value(val, meta.get("type"))
                logging.info(f"[CodeExec]: populate dict key='{key}' raw='{val}' coerced='{coerced}'")
                self.set_output(key, coerced)
            return

        if isinstance(parsed_stdout, (list, tuple)):
            for idx, (key, meta) in enumerate(outputs_items):
                if key.startswith("_"):
                    continue
                val = parsed_stdout[idx] if idx < len(parsed_stdout) else None
                coerced = self._coerce_output_value(val, meta.get("type"))
                logging.info(f"[CodeExec]: populate list key='{key}' raw='{val}' coerced='{coerced}'")
                self.set_output(key, coerced)
            return

        default_val = parsed_stdout if parsed_stdout is not None else raw_stdout
        for idx, (key, meta) in enumerate(outputs_items):
            if key.startswith("_"):
                continue
            val = default_val if idx == 0 else None
            coerced = self._coerce_output_value(val, meta.get("type"))
            logging.info(f"[CodeExec]: populate scalar key='{key}' raw='{val}' coerced='{coerced}'")
            self.set_output(key, coerced)

    def _get_by_path(self, data, path: str):
        if not path:
            return None
        cur = data
        for part in path.split("."):
            part = part.strip()
            if not part:
                return None
            if isinstance(cur, dict):
                cur = cur.get(part)
            elif isinstance(cur, list):
                try:
                    idx = int(part)
                    cur = cur[idx]
                except Exception:
                    return None
            else:
                return None
            if cur is None:
                return None
        logging.info(f"[CodeExec]: resolve path '{path}' -> {cur}")
        return cur
