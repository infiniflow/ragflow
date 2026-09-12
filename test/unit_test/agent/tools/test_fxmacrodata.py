"""Native Canvas/ToolBase integration with synthetic HTTP transport fixtures."""
import asyncio
from copy import deepcopy
import json
import logging

import pytest

from agent.canvas import Canvas
import agent.canvas as canvas_module
from agent.component.base import ComponentParamBase
from agent.tools.base import LLMToolPluginCallSession, ToolBase
from agent.tools import fxmacrodata as provider
from agent.tools.fxmacrodata_client import list_operations
from agent.tools.fxmacrodata import FXMacroDataClient


class Response:
    def __init__(self, payload, status=200, stream=False):
        self.status_code = status
        self.headers = {"Content-Type": "text/event-stream" if stream else "application/json"}
        self.content = (("data: " + json.dumps(payload) + "\n\n") if stream else json.dumps(payload)).encode()
        self.closed = False

    def iter_content(self, chunk_size):
        yield self.content

    def close(self):
        self.closed = True


class Session:
    def __init__(self, payload, status=200):
        self.payload = payload
        self.status = status
        self.calls = []
        self.responses = []

    def request(self, method, url, **kwargs):
        self.calls.append((method, url, deepcopy(kwargs)))
        body = kwargs.get("json") or {}
        if body.get("method") == "initialize":
            result = Response({"jsonrpc": "2.0", "id": body["id"], "result": {"protocolVersion": "2025-03-26"}})
        elif body.get("method") == "notifications/initialized":
            result = Response(None, 202)
        elif body.get("method") == "tools/call":
            result = Response({"jsonrpc": "2.0", "id": body["id"], "result": self.payload}, self.status)
        else:
            result = Response(self.payload, self.status, kwargs.get("headers", {}).get("Accept") == "text/event-stream")
        self.responses.append(result)
        return result


def arguments_for(operation):
    values = {"currency": "USD", "base": "EUR", "quote": "USD", "indicator": "inflation", "factor": "monetary_stance", "positions_json": "[]"}
    result = {}
    for name in operation.input_schema.get("required", []):
        schema = operation.input_schema["properties"][name]
        if name in values:
            result[name] = values[name]
        elif "default" in schema:
            result[name] = deepcopy(schema["default"])
        elif "enum" in schema:
            result[name] = schema["enum"][0]
        else:
            result[name] = {"integer": 1, "number": 1, "boolean": False, "array": [], "object": {}}.get(schema.get("type"), "fixture")
    return result


@pytest.fixture
def make_canvas(monkeypatch):
    instances = []
    monkeypatch.setattr(canvas_module, "has_canceled", lambda task_id: False)

    def make(operation="release_calendar", arguments=None, **settings):
        params = {"operation": operation, "arguments": {"currency": "USD"} if arguments is None else arguments, **settings}
        dsl = {"components": {"fxmd": {"obj": {"component_name": "FXMacroData", "params": params}, "upstream": [], "downstream": []}}, "history": [], "path": [], "retrieval": []}
        canvas = Canvas(json.dumps(dsl), tenant_id="synthetic-test-tenant")
        instances.append(canvas)
        return canvas, canvas.get_component_obj("fxmd")

    yield make
    for canvas in instances:
        canvas.close()
        canvas._thread_pool.shutdown(wait=True)


def transport(monkeypatch, payload, status=200):
    session = Session(payload, status)
    monkeypatch.setattr(provider, "FXMacroDataClient", lambda **kwargs: FXMacroDataClient(session=session, **kwargs))
    return session


@pytest.mark.parametrize("operation", list_operations(), ids=lambda operation: operation.name)
def test_all_operations_load_from_native_dsl_and_execute_canvas_inputs(operation, make_canvas, monkeypatch):
    payload = {"data": [{"fixture": "synthetic-native-transport", "value": None, "available": False}], "units": "fixture-only"}
    session = transport(monkeypatch, payload)
    canvas, tool = make_canvas(operation.name, arguments_for(operation))
    assert isinstance(tool, ToolBase) and isinstance(tool._param, ComponentParamBase)
    assert tool.get_input() == {"arguments": arguments_for(operation)}
    content = tool.invoke(**tool.get_input())
    assert not tool.output("_ERROR")
    assert session.calls and all(response.closed for response in session.responses)
    if operation.name == "stream_events":
        assert tool.output("response")["events"][0]["data"] == payload
    else:
        assert tool.output("response") == payload
    assert tool.output("json")
    assert "https://fxmacrodata.com" in content
    assert canvas.get_reference()["chunks"] and canvas.get_reference()["doc_aggs"]
    assert canvas._build_message_end(tool)["reference"] == canvas.get_reference()
    assert all(chunk["url"] == tool.output("source_url") for chunk in canvas.get_reference()["chunks"].values())
    metadata = tool.get_meta()["function"]
    assert metadata["name"] == "fxmacrodata_" + operation.name
    assert metadata["parameters"] == operation.input_schema
    assert all("api_key" not in call[2]["params"] for call in session.calls)


def test_complete_unique_metadata_and_public_schemas():
    assert len(list_operations()) == 72
    names = set()
    for operation in list_operations():
        parameter = provider.FXMacroDataParam()
        parameter.update({"operation": operation.name, "arguments": arguments_for(operation)})
        parameter.check()
        metadata = parameter.get_meta()["function"]
        names.add(metadata["name"])
        assert set(parameter.get_input_form()) == set(operation.input_schema.get("properties", {}))
        assert "api_key" not in metadata["parameters"].get("properties", {})
    assert len(names) == 72


def test_native_agent_session_calls_async_tool_and_callback(make_canvas, monkeypatch):
    session = transport(monkeypatch, {"data": [{"fixture": "agent-test"}]})
    canvas, tool = make_canvas()
    callbacks = []
    name = tool.get_meta()["function"]["name"]
    caller = LLMToolPluginCallSession({name: tool}, lambda *args, **kwargs: callbacks.append((args, kwargs)))
    result = asyncio.run(caller.tool_call_async(name, {"currency": "USD"}))
    assert "agent-test" in result and callbacks and session.calls
    assert canvas.get_reference()["chunks"]


def test_environment_key_is_invocation_only_and_not_serialized(make_canvas, monkeypatch, caplog):
    secret = "synthetic-ragflow-credential-sentinel"
    monkeypatch.setenv("FXMACRODATA_API_KEY", secret)
    session = transport(monkeypatch, {"data": [{"fixture": "authorized-test"}]})
    canvas, tool = make_canvas(use_credentials=True)
    assert secret not in str(canvas)
    with caplog.at_level(logging.DEBUG):
        tool.invoke(**tool.get_input())
    assert any(call[2]["params"].get("api_key") == secret for call in session.calls)
    assert secret not in str(canvas) + caplog.text + json.dumps(tool.get_meta())


def test_public_mode_ignores_inherited_credentials(make_canvas, monkeypatch):
    monkeypatch.setenv("FXMACRODATA_API_KEY", "synthetic-key-must-not-be-used")
    session = transport(monkeypatch, {"data": []})
    _, tool = make_canvas()
    result = tool.invoke(**tool.get_input())
    assert "No records returned" in result
    assert tool.output("json") == []
    assert all("api_key" not in call[2]["params"] for call in session.calls)


@pytest.mark.parametrize("status", [401, 403, 429, 500])
def test_errors_and_native_traceback_do_not_expose_key(make_canvas, monkeypatch, caplog, status):
    secret = "synthetic-error-credential-sentinel"
    monkeypatch.setenv("FXMACRODATA_API_KEY", secret)
    transport(monkeypatch, {"error": secret}, status)
    canvas, tool = make_canvas(use_credentials=True)
    with caplog.at_level(logging.DEBUG):
        result = tool.invoke(**tool.get_input())
    assert tool.output("_ERROR")
    assert secret not in str(canvas) + result + caplog.text
    assert tool.output("json") == [] and not canvas.get_reference()["chunks"]


def test_missing_credential_fails_without_request(make_canvas, monkeypatch):
    monkeypatch.delenv("FXMACRODATA_API_KEY", raising=False)
    monkeypatch.delenv("FXMD_API_KEY", raising=False)
    session = transport(monkeypatch, {})
    _, tool = make_canvas(use_credentials=True)
    assert "no FXMacroData process credential" in tool.invoke(**tool.get_input())
    assert not session.calls


@pytest.mark.parametrize("when", ["before", "after"])
def test_native_cancellation_prevents_outputs(make_canvas, monkeypatch, when):
    session = transport(monkeypatch, {"data": [{"fixture": "cancel-test"}]})
    canvas, tool = make_canvas()
    monkeypatch.setattr(canvas_module, "has_canceled", lambda task_id: when == "before" or bool(session.calls))
    tool.invoke(**tool.get_input())
    assert bool(session.calls) == (when == "after")
    assert tool.output("json") == [] and not canvas.get_reference()["chunks"]
    assert tool.output("_ERROR") == "Task has been canceled"


@pytest.mark.parametrize("settings", [{"operation": "invalid"}, {"arguments": []}, {"timeout": float("nan")}, {"timeout": True}, {"timeout": 121}, {"use_credentials": "false"}])
def test_invalid_native_parameters_rejected_before_execution(make_canvas, settings):
    with pytest.raises(ValueError):
        make_canvas(**settings)


def test_pagination_selectors_and_original_unavailable_metadata_survive(make_canvas, monkeypatch):
    payload = {"data": [], "available": False, "next_cursor": "fixture-cursor", "source_pending": True}
    session = transport(monkeypatch, payload)
    _, tool = make_canvas("indicator_history", {"currency": "USD", "indicator": "inflation", "limit": 2, "offset": 4})
    tool.invoke(**tool.get_input())
    assert not tool.output("_ERROR")
    assert session.calls[0][2]["params"]["limit"] == 2
    assert session.calls[0][2]["params"]["offset"] == 4
    assert tool.output("response") == payload and tool.output("json") == []


def test_visual_mcp_artifacts_remain_original_outputs(make_canvas, monkeypatch):
    operation = next(operation for operation in list_operations() if operation.method == "MCP" and "visual" in operation.name)
    payload = {"content": [{"type": "resource_link", "uri": "https://fxmacrodata.com/fixture-only", "mimeType": "text/html"}], "structuredContent": {"fixture": "original-artifact-metadata"}}
    session = transport(monkeypatch, payload)
    _, tool = make_canvas(operation.name, arguments_for(operation))
    tool.invoke(**tool.get_input())
    assert not tool.output("_ERROR") and tool.output("response") == payload
    assert len(session.calls) == 3


def test_native_error_blocks_endpoint_or_credential_arguments(make_canvas, monkeypatch):
    session = transport(monkeypatch, {})
    _, tool = make_canvas(arguments={"currency": "USD", "base_url": "https://example.invalid"})
    assert "endpoint overrides" in tool.invoke(**tool.get_input())
    assert not session.calls


@pytest.mark.parametrize("async_mode", [False, True])
def test_reused_native_tool_clears_previous_data_and_error(make_canvas, monkeypatch, async_mode):
    session = transport(monkeypatch, {"data": [{"fixture": "first-success"}]})
    _, tool = make_canvas()
    def invoke():
        if async_mode:
            return asyncio.run(tool.invoke_async(**tool.get_input()))
        return tool.invoke(**tool.get_input())
    invoke()
    assert tool.output("json")
    session.status = 503
    invoke()
    assert tool.output("_ERROR")
    assert tool.output("json") == []
    assert tool.output("response") == {}
    assert tool.output("source_url") == ""
    assert tool.output("formalized_content") == ""
    session.status = 200
    invoke()
    assert not tool.output("_ERROR")
    assert tool.output("json")


@pytest.mark.parametrize("async_mode", [False, True])
def test_reused_native_tool_clears_data_before_early_cancellation(make_canvas, monkeypatch, async_mode):
    transport(monkeypatch, {"data": [{"fixture": "previous-query"}]})
    _, tool = make_canvas()
    tool.invoke(**tool.get_input())
    monkeypatch.setattr(canvas_module, "has_canceled", lambda task_id: True)
    if async_mode:
        asyncio.run(tool.invoke_async(**tool.get_input()))
    else:
        tool.invoke(**tool.get_input())
    assert tool.output("_ERROR") == "Task has been canceled"
    assert tool.output("json") == []
    assert tool.output("response") == {}


@pytest.mark.parametrize("secret_field_value", [True, False, None, 17])
def test_escaped_json_credential_is_redacted_before_records_citations_and_exports(make_canvas, monkeypatch, secret_field_value):
    secret = "synthetic-ragflow-escaped-key"
    encoded = "".join("\\u%04x" % ord(char) for char in secret)
    nested = '{"note":"' + encoded + '","apiKey":' + json.dumps(secret_field_value) + ',"public_value":1,"available":false}'
    monkeypatch.setenv("FXMACRODATA_API_KEY", secret)
    transport(monkeypatch, {"content": [{"type": "text", "text": nested}], "isError": False})
    canvas, tool = make_canvas("mcp_ping", {}, use_credentials=True)
    result = tool.invoke(**tool.get_input())
    assert not tool.output("_ERROR")
    assert tool.output("json")[0]["public_value"] == 1
    assert tool.output("json")[0]["available"] is False
    assert tool.output("response")["isError"] is False
    assert secret not in result + str(canvas) + json.dumps(tool.output())
    assert json.loads(tool.output("response")["content"][0]["text"])["note"] == "[redacted]"
    assert tool.output("json")[0]["apiKey"] == "[redacted]"


def test_invalid_structured_form_argument_blocks_http_and_correction_recovers(make_canvas, monkeypatch):
    operation = provider.OPERATIONS["mcp_plot_visual_artifact"]
    arguments = arguments_for(operation)
    arguments["series"] = '[{"source":'
    session = transport(monkeypatch, {"data": [{"fixture": "corrected-query"}]})
    _, tool = make_canvas(operation.name, arguments)
    tool.invoke(**tool.get_input())
    assert tool.output("_ERROR") and not session.calls
    tool._param.arguments["series"] = []
    tool.invoke(**tool.get_input())
    assert not tool.output("_ERROR")
    assert session.calls and tool.output("json")


@pytest.mark.parametrize("encoding", ["json", "unicode_lower", "unicode_upper"])
def test_configured_credential_escapes_in_plain_prose_are_redacted(make_canvas, monkeypatch, encoding):
    secret = 'synthetic-ragflow-"key"\\é😀'
    raw = secret.encode("utf-16-be")
    escaped = json.dumps(secret, ensure_ascii=True)[1:-1] if encoding == "json" else "".join(
        "\\u" + (raw[index:index + 2].hex().upper() if encoding == "unicode_upper" else raw[index:index + 2].hex())
        for index in range(0, len(raw), 2)
    )
    monkeypatch.setenv("FXMACRODATA_API_KEY", secret)
    transport(monkeypatch, {"data": [{"note": "Echo " + escaped + ".", "available": False}]})
    canvas, tool = make_canvas("ping", {}, use_credentials=True)
    result = tool.invoke(**tool.get_input())
    assert not tool.output("_ERROR")
    assert tool.output("json")[0] == {"note": "Echo [redacted].", "available": False}
    assert escaped not in result + str(canvas)
    assert tool.output("response")["data"][0]["note"] == "Echo [redacted]."
