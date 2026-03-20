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
import uuid
from abc import ABC
from typing import Optional

from pydantic import BaseModel, Field, field_validator
from strenum import StrEnum

from agent.tools.base import ToolBase, ToolMeta, ToolParamBase
from api.db.services.file_service import FileService
from common import settings
from common.connection_utils import timeout
from common.constants import SANDBOX_ARTIFACT_BUCKET, SANDBOX_ARTIFACT_EXPIRE_DAYS


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

To generate charts or files (images, PDFs, CSVs, etc.), save them to the `artifacts/` directory (relative to the working directory). The sandbox will automatically collect these files and return them. Example:
def main() -> dict:
    import matplotlib
    matplotlib.use("Agg")
    import matplotlib.pyplot as plt
    import pandas as pd

    df = pd.DataFrame({"x": [1, 2, 3, 4], "y": [10, 20, 25, 30]})
    fig, ax = plt.subplots()
    ax.plot(df["x"], df["y"])
    ax.set_title("Sample Chart")
    fig.savefig("artifacts/chart.png", dpi=150, bbox_inches="tight")
    plt.close(fig)
    return {"summary": "Chart saved to artifacts/chart.png"}

Available Python packages: pandas, numpy, matplotlib, requests.
Supported artifact file types: .png, .jpg, .jpeg, .svg, .pdf, .csv, .json, .html

Collected artifacts are also parsed automatically and appended to the stable text output `content`. The content includes sections like `attachment1 (image): ...`, `attachment2 (pdf): ...`, so downstream nodes can consume a single text output without depending on unstable attachment-specific variables.

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
    _lifecycle_configured = False

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

        timeout_seconds = int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10 * 60))

        try:
            # Try using the new sandbox provider system first
            try:
                from agent.sandbox.client import execute_code as sandbox_execute_code

                if self.check_if_canceled("CodeExec execution"):
                    return

                # Execute code using the provider system
                result = sandbox_execute_code(code=code, language=language, timeout=timeout_seconds, arguments=arguments)

                if self.check_if_canceled("CodeExec execution"):
                    return

                artifacts = result.metadata.get("artifacts", []) if result.metadata else []
                return self._process_execution_result(result.stdout, result.stderr, "Provider system", artifacts)

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

            resp = requests.post(url=f"http://{settings.SANDBOX_HOST}:9385/run", json=code_req, timeout=timeout_seconds)
            logging.info(f"http://{settings.SANDBOX_HOST}:9385/run,  code_req: {code_req}, resp.status_code {resp.status_code}:")

            if self.check_if_canceled("CodeExec execution"):
                return "Task has been canceled"

            if resp.status_code != 200:
                resp.raise_for_status()
            body = resp.json()
            if body:
                return self._process_execution_result(
                    body.get("stdout", ""),
                    body.get("stderr"),
                    f"http://{settings.SANDBOX_HOST}:9385/run",
                    body.get("artifacts", []),
                )
            else:
                self.set_output("_ERROR", "There is no response from sandbox")
                return self.output()

        except Exception as e:
            if self.check_if_canceled("CodeExec execution"):
                return self.output()

            self.set_output("_ERROR", "Exception executing code: " + str(e))

        return self.output()

    def _process_execution_result(self, stdout: str, stderr: str | None, source: str, artifacts: list | None = None):
        if stderr and not stdout and not artifacts:
            self.set_output("_ERROR", stderr)
            return self.output()

        # Clear any stale error from previous runs or base class initialization
        self.set_output("_ERROR", "")

        if stderr:
            logging.warning(f"[CodeExec]: stderr (non-fatal): {stderr[:500]}")

        parsed_stdout = self._deserialize_stdout(stdout)
        logging.info(f"[CodeExec]: {source} -> {parsed_stdout}")
        self._populate_outputs(parsed_stdout, stdout)
        content_parts = []
        base_content = self._build_content_text(parsed_stdout, raw_stdout=stdout)
        if base_content:
            content_parts.append(base_content)

        if artifacts:
            artifact_urls = self._upload_artifacts(artifacts)
            if artifact_urls:
                self.set_output("_ARTIFACTS", artifact_urls)
            attachment_text = self._build_attachment_content(artifacts, artifact_urls)
            self.set_output("_ATTACHMENT_CONTENT", attachment_text)
            if attachment_text:
                content_parts.append(attachment_text)
        else:
            self.set_output("_ATTACHMENT_CONTENT", "")

        self.set_output("content", "\n\n".join([part for part in content_parts if part]).strip())

        return self.output()

    @classmethod
    def _ensure_bucket_lifecycle(cls):
        if cls._lifecycle_configured:
            return
        try:
            storage = settings.STORAGE_IMPL
            # Only MinIO/S3 backends expose .conn for lifecycle config
            if not hasattr(storage, "conn") or storage.conn is None:
                cls._lifecycle_configured = True
                return
            if not storage.conn.bucket_exists(SANDBOX_ARTIFACT_BUCKET):
                storage.conn.make_bucket(SANDBOX_ARTIFACT_BUCKET)
            from minio.commonconfig import Filter
            from minio.lifecycleconfig import Expiration, LifecycleConfig, Rule

            rule = Rule(
                rule_id="auto-expire",
                status="Enabled",
                rule_filter=Filter(prefix=""),
                expiration=Expiration(days=SANDBOX_ARTIFACT_EXPIRE_DAYS),
            )
            storage.conn.set_bucket_lifecycle(SANDBOX_ARTIFACT_BUCKET, LifecycleConfig([rule]))
            logging.info(f"[CodeExec]: Set {SANDBOX_ARTIFACT_EXPIRE_DAYS}-day lifecycle on bucket '{SANDBOX_ARTIFACT_BUCKET}'")
            cls._lifecycle_configured = True
        except Exception as e:
            # Do NOT set _lifecycle_configured so we retry next time
            logging.warning(f"[CodeExec]: Failed to set bucket lifecycle: {e}")

    def _upload_artifacts(self, artifacts: list) -> list[dict]:
        self._ensure_bucket_lifecycle()
        uploaded = []
        for art in artifacts:
            try:
                name = art.get("name", "") if isinstance(art, dict) else getattr(art, "name", "")
                content_b64 = art.get("content_b64", "") if isinstance(art, dict) else getattr(art, "content_b64", "")
                mime_type = art.get("mime_type", "") if isinstance(art, dict) else getattr(art, "mime_type", "")
                size = art.get("size", 0) if isinstance(art, dict) else getattr(art, "size", 0)
                if not content_b64 or not name:
                    continue

                ext = os.path.splitext(name)[1].lower()
                storage_name = f"{uuid.uuid4().hex}{ext}"
                binary = base64.b64decode(content_b64)

                settings.STORAGE_IMPL.put(SANDBOX_ARTIFACT_BUCKET, storage_name, binary)

                url = f"/v1/document/artifact/{storage_name}"
                uploaded.append(
                    {
                        "name": name,
                        "url": url,
                        "mime_type": mime_type,
                        "size": size,
                    }
                )
                logging.info(f"[CodeExec]: Uploaded artifact {name} -> {url}")
            except Exception as e:
                logging.warning(f"[CodeExec]: Failed to upload artifact: {e}")
        return uploaded

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

    def _build_attachment_content(self, artifacts: list, artifact_urls: list[dict] | None = None) -> str:
        sections = []
        artifact_urls = artifact_urls or []

        for idx, art in enumerate(artifacts, start=1):
            key = f"attachment{idx}"
            try:
                name = art.get("name", "") if isinstance(art, dict) else getattr(art, "name", "")
                content_b64 = art.get("content_b64", "") if isinstance(art, dict) else getattr(art, "content_b64", "")
                mime_type = art.get("mime_type", "") if isinstance(art, dict) else getattr(art, "mime_type", "")
                if not name or not content_b64:
                    continue

                blob = base64.b64decode(content_b64)
                parsed = FileService.parse(
                    name,
                    blob,
                    False,
                    tenant_id=self._canvas.get_tenant_id(),
                )
                attachment_type = self._normalize_attachment_type(name, mime_type)
                section = self._format_attachment_section(key, attachment_type, name, parsed)
                sections.append(section)
                logging.info(f"[CodeExec]: parse attachment section key='{key}' from artifact='{name}'")
            except Exception as e:
                logging.warning(f"[CodeExec]: Failed to parse artifact for content section '{key}': {e}")
                fallback_type = self._normalize_attachment_type(
                    art.get("name", "") if isinstance(art, dict) else getattr(art, "name", ""),
                    art.get("mime_type", "") if isinstance(art, dict) else getattr(art, "mime_type", ""),
                )
                fallback_name = art.get("name", "") if isinstance(art, dict) else getattr(art, "name", "")
                fallback_url = ""
                if idx - 1 < len(artifact_urls):
                    fallback_url = artifact_urls[idx - 1].get("url", "")
                fallback_text = "Artifact generated but parse failed."
                if fallback_url:
                    fallback_text += f" Download: {fallback_url}"
                sections.append(self._format_attachment_section(key, fallback_type, fallback_name, fallback_text))

        if sections:
            return f"attachment_count: {len(sections)}\n\n" + "\n\n".join(sections)
        return "attachment_count: 0"

    def _normalize_attachment_type(self, name: str, mime_type: str) -> str:
        mime_type = str(mime_type or "").strip().lower()
        if mime_type.startswith("image/"):
            return "image"
        if mime_type == "application/pdf":
            return "pdf"
        if mime_type == "text/csv":
            return "csv"
        if mime_type == "application/json":
            return "json"
        if mime_type == "text/html":
            return "html"

        ext = os.path.splitext(name or "")[1].lower().lstrip(".")
        return ext or "file"

    def _format_attachment_section(self, key: str, attachment_type: str, name: str, parsed: str) -> str:
        title = f"{key} ({attachment_type})"
        if name:
            title += f": {name}"
        body = parsed if isinstance(parsed, str) else json.dumps(parsed, ensure_ascii=False)
        return f"{title}\n{body}".strip()

    def _build_content_text(self, parsed_stdout, raw_stdout: str = "") -> str:
        if isinstance(parsed_stdout, str):
            return parsed_stdout.strip()
        if isinstance(parsed_stdout, (dict, list, tuple)):
            try:
                return json.dumps(parsed_stdout, ensure_ascii=False, indent=2).strip()
            except Exception:
                return str(parsed_stdout).strip()
        if parsed_stdout is None:
            return str(raw_stdout or "").strip()
        return str(parsed_stdout).strip()

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
