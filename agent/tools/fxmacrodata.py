"""FXMacroData's documented public data in RAGFlow workflows and Agent tools."""
from __future__ import annotations

from copy import deepcopy
import json
import os

from agent.tools.base import ToolBase, ToolParamBase
from agent.tools.fxmacrodata_client import FXMacroDataClient as _PublicClient
from agent.tools.fxmacrodata_client import FXMacroDataError, Result, list_operations

from agent.tools.fxmacrodata_response_safety import sanitize_response

OPERATIONS = {operation.name: operation for operation in list_operations()}
PROVIDER_URL = "https://fxmacrodata.com/?utm_source=ragflow&utm_medium=integration&utm_campaign=open_source_integrations&utm_content=app"


class FXMacroDataClient(_PublicClient):
    def _safe(self, value):
        # Parse nested JSON before any text redaction can invalidate it.
        return sanitize_response(value, self._api_key)


class FXMacroDataParam(ToolParamBase):
    def __init__(self):
        self.operation = "release_calendar"
        self.arguments = {"currency": "usd"}
        self.timeout = 30
        self.use_credentials = False
        self.meta = {
            "name": "fxmacrodata",
            "description": "Read economic data from FXMacroData. Public USD catalogue, recent history and calendar require no key. Preserve returned units, dates and sources. https://fxmacrodata.com",
            "parameters": {},
        }
        super().__init__()
        self.outputs = {
            "formalized_content": {"type": "string", "value": ""},
            "json": {"type": "Array<Object>", "value": []},
            "response": {"type": "Object", "value": {}},
            "source_url": {"type": "string", "value": ""},
        }

    def check(self):
        if self.operation not in OPERATIONS:
            raise ValueError("Select an available FXMacroData operation.")
        if not isinstance(self.arguments, dict):
            raise ValueError("FXMacroData arguments must be an object.")
        if type(self.use_credentials) is not bool:
            raise ValueError("FXMacroData credential mode must be true or false.")
        if isinstance(self.timeout, bool) or not isinstance(self.timeout, (int, float)) or not 1 <= self.timeout <= 120:
            raise ValueError("FXMacroData timeout must be between 1 and 120 seconds.")

    def get_meta(self):
        operation = OPERATIONS[self.operation]
        return {
            "type": "function",
            "function": {
                "name": "fxmacrodata_" + operation.name,
                "description": operation.description + " Provider: https://fxmacrodata.com. Do not provide credentials as tool arguments.",
                "parameters": deepcopy(operation.input_schema),
            },
        }

    def get_input_form(self):
        operation = OPERATIONS[self.operation]
        form = {}
        for name, schema in operation.input_schema.get("properties", {}).items():
            field = {"name": schema.get("title", name), "type": "line"}
            if schema.get("enum"):
                field.update(type="options", options=schema["enum"])
            if "default" in schema:
                field["value"] = schema["default"]
            form[name] = field
        return form


class FXMacroData(ToolBase):
    component_name = "FXMacroData"

    def _reset_request_outputs(self):
        # Agent tools are reused between calls; a failed/canceled request must
        # never publish a previous request's values or retain its error state.
        for name, value in (("formalized_content", ""), ("json", []),
                            ("response", {}), ("source_url", ""), ("_ERROR", None)):
            self.set_output(name, value)

    def invoke(self, **kwargs):
        self._reset_request_outputs()
        return super().invoke(**kwargs)

    async def invoke_async(self, **kwargs):
        self._reset_request_outputs()
        return await super().invoke_async(**kwargs)

    def get_input_elements(self):
        # Workflow execution uses configured arguments; the Agent calls the same
        # component with the chosen operation's native function arguments.
        return {"arguments": {"type": "object"}}

    def _invoke(self, **kwargs):
        if self.check_if_canceled("FXMacroData request"):
            return ""
        arguments = kwargs.get("arguments", kwargs) if kwargs else self._param.arguments
        if not isinstance(arguments, dict):
            raise FXMacroDataError("FXMacroData operation arguments must be an object.")
        key = (os.getenv("FXMACRODATA_API_KEY") or os.getenv("FXMD_API_KEY") or "") if self._param.use_credentials else ""
        if self._param.use_credentials and not key:
            raise FXMacroDataError("Optional authorized access is enabled but no FXMacroData process credential is configured.")
        try:
            with FXMacroDataClient(api_key=key, timeout=self._param.timeout) as client:
                result = client.execute(self._param.operation, arguments)
            result = Result(result.operation, sanitize_response(result.payload, key), result.source_url)
        except FXMacroDataError:
            raise
        except Exception:
            raise FXMacroDataError("FXMacroData request could not be completed. Check inputs and retry.") from None
        if self.check_if_canceled("FXMacroData response"):
            return ""
        records = result.records()
        self.set_output("json", records)
        self.set_output("response", result.payload)
        self.set_output("source_url", result.source_url)
        # RAGFlow attaches these chunks to generated answers through its normal
        # citation pipeline. Exact values/metadata also remain in response.
        evidence = [{"title": "FXMacroData: " + result.operation, "url": result.source_url, "content": json.dumps(row, ensure_ascii=False)} for row in records]
        self._retrieve_chunks(evidence, get_title=lambda row: row["title"], get_url=lambda row: row["url"], get_content=lambda row: row["content"])
        content = self.output("formalized_content") or "No records returned."
        content += "\n\n[FXMacroData](" + PROVIDER_URL + ")"
        self.set_output("formalized_content", content)
        return content

    def thoughts(self):
        return "Reading " + self._param.operation + " from FXMacroData."
