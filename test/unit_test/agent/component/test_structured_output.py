# Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
# SPDX-License-Identifier: Apache-2.0

"""Exercise real components, model routing, and JSON libraries with external dependencies isolated."""

import asyncio
import importlib.util
import json
import sys
from contextlib import nullcontext
from contextvars import ContextVar
from copy import deepcopy
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest

SCHEMA = {
    "type": "object",
    "properties": {
        "decision": {"type": "string", "enum": ["approve", "reject"]},
        "amount": {"type": "number", "minimum": 0, "maximum": 100},
    },
    "required": ["decision", "amount"],
    "additionalProperties": False,
}
VALID = {"decision": "approve", "amount": 50}


@pytest.fixture
def modules(monkeypatch):
    root = Path(__file__).resolve().parents[4]

    def stub(name, **attrs):
        module = ModuleType(name)
        module.__dict__.update(attrs)
        monkeypatch.setitem(sys.modules, name, module)
        return module

    for name in ("agent", "agent.component", "agent.tools", "api", "api.db", "api.db.services", "api.db.joint_services", "common", "rag", "rag.prompts"):
        stub(name).__path__ = [str(root.joinpath(*name.split(".")))]
    stub("agent.settings", PARAM_MAXDEPTH=10, FLOAT_ZERO=1e-6)
    stub("pandas", DataFrame=type("DataFrame", (), {}))
    stub("common.connection_utils", timeout=lambda *a, **k: lambda fn: fn)
    stub("common.misc_utils", thread_pool_exec=asyncio.to_thread)
    stub("common.constants", LLMType=SimpleNamespace(CHAT=SimpleNamespace(value="chat"), VISION=SimpleNamespace(value="vision")))
    stub("langfuse", propagate_attributes=lambda **k: nullcontext())
    stub("api.db.db_models", LLM=object)
    stub("api.db.services.common_service", CommonService=object)
    stub("api.db.services.tenant_llm_service", LLM4Tenant=object)
    stub("common.token_utils", langfuse_run_attrs=None, num_tokens_from_string=len, record_run_token_usage=lambda *a: None, truncate=lambda text, length: text[:length])
    stub("api.db.joint_services.tenant_model_service", resolve_model_config=lambda *a: {}, resolve_model_type=lambda *a: ["chat"])
    stub("api.db.services.dialog_service", _stream_with_think_delta=lambda stream, **k: stream)
    stub("api.db.services.mcp_server_service", MCPServerService=object)
    stub("common.mcp_tool_call_conn", MCPToolBinding=object, MCPToolCallSession=object, mcp_tool_metadata_to_openai_tool=lambda *a, **k: {})
    stub("agent.tools.base", LLMToolPluginCallSession=object, ToolBase=type("ToolBase", (), {}), ToolMeta=dict, ToolParamBase=type("ToolParamBase", (), {}))
    stub(
        "rag.prompts.generator",
        citation_prompt=lambda *a: "",
        message_fit_in=lambda messages, budget: (0, deepcopy(messages)),
        structured_output_prompt=lambda schema: f"\nReturn output that conforms to this JSON Schema: {schema}",
        tool_call_summary=lambda *a: "",
        citation_plus=lambda text: text,
        full_question=lambda *a, **k: "",
        kb_prompt=lambda *a: [],
    )

    def load(name):
        spec = importlib.util.spec_from_file_location(name, root.joinpath(*name.split(".")).with_suffix(".py"))
        module = importlib.util.module_from_spec(spec)
        monkeypatch.setitem(sys.modules, name, module)
        spec.loader.exec_module(module)
        return module

    bundle = load("api.db.services.llm_service")
    base = load("agent.component.base")
    llm = load("agent.component.llm")
    agent = load("agent.component.agent_with_tools")
    return SimpleNamespace(bundle=bundle, base=base, llm=llm, agent=agent, stub=stub, load=load)


class FakeProvider:
    def __init__(self, responses, canvas, tools):
        self.responses = iter(responses)
        self.canvas = canvas
        self.is_tools = tools
        self.calls = []
        self.cancel_after = None

    def respond(self, mode, system, history, kwargs):
        self.calls.append({"mode": mode, "system": system, "history": deepcopy(history), "kwargs": kwargs})
        response = next(self.responses)
        if self.cancel_after == len(self.calls):
            self.canvas.canceled = True
        if isinstance(response, BaseException):
            raise response
        return response, 3

    async def async_chat(self, system, history, gen_conf, **kwargs):
        return self.respond("plain", system, history, kwargs)

    async def async_chat_with_tools(self, system, history, gen_conf, **kwargs):
        return self.respond("tools", system, history, kwargs)


@pytest.fixture(params=["LLM", "Agent", "Agent_without_tools"])
def component(request, modules):
    cls = modules.llm.LLM if request.param == "LLM" else modules.agent.Agent
    obj = cls.__new__(cls)
    obj._id = "test-node"
    obj._param = modules.llm.LLMParam()
    obj._param.outputs = {"structured": deepcopy(SCHEMA)}
    obj._unrunnable = ""
    obj._unrunnable_error = ""
    obj.imgs = []
    obj.tools = {"local-tool": object()} if request.param == "Agent" else {}
    canvas = SimpleNamespace(canceled=False, globals={}, get_component=lambda _: {"downstream": []})
    canvas.is_canceled = lambda: canvas.canceled
    obj._canvas = canvas
    obj._prepare_prompt_variables = lambda: ("System prompt", [{"role": "user", "content": "Review this order"}], {})
    obj._collect_tool_artifact_markdown = lambda **k: ""

    def configure(responses, retries=0, schema=None):
        obj._param.max_retries = retries
        if schema is not None:
            obj._param.outputs["structured"] = deepcopy(schema)
        provider = FakeProvider(responses, canvas, bool(obj.tools))
        bundle = modules.bundle.LLMBundle.__new__(modules.bundle.LLMBundle)
        bundle.is_tools = True
        bundle.mdl = provider
        bundle.langfuse = None
        bundle.verbose_tool_use = False
        bundle.max_length = 8192
        obj.chat_mdl = bundle
        return obj, provider

    return configure


@pytest.mark.parametrize(
    ("answer", "error_field"),
    [
        ({"decision": "approve", "amount": "many"}, "amount"),
        ({"decision": "approve"}, "amount"),
        ({"decision": "maybe", "amount": 50}, "decision"),
        ({"decision": "approve", "amount": 9999}, "amount"),
        ({"decision": "approve", "amount": 50, "extra": 1}, "extra"),
    ],
)
async def test_invalid_output_is_not_published(component, answer, error_field):
    obj, provider = component([json.dumps(answer)])
    obj.set_output("structured", VALID)
    result = await obj.invoke_async()
    assert result["structured"] is None
    assert error_field in obj.error()
    assert len(provider.calls) == 1


@pytest.mark.parametrize("answer", [json.dumps(VALID), "```json\n" + json.dumps(VALID) + "\n```", "{decision: 'approve', amount: 50}"])
async def test_valid_and_repairable_json_is_preserved(component, answer):
    obj, provider = component([answer])
    result = await obj.invoke_async()
    assert result["structured"] == VALID
    assert not obj.error()
    assert len(provider.calls) == 1


async def test_schema_error_is_repaired_with_feedback(component):
    obj, provider = component(['{"decision":"approve","amount":"many"}', json.dumps(VALID)], retries=1)
    await obj.invoke_async()
    assert obj.output("structured") == VALID
    assert not obj.error()
    assert len(provider.calls) == 2
    repair_request = json.dumps(provider.calls[1])
    assert "Fix" in repair_request
    assert "amount" in repair_request
    assert provider.calls[1]["history"] != provider.calls[0]["history"]
    assert provider.calls[1]["mode"] == "plain"
    assert provider.is_tools == bool(obj.tools)


@pytest.mark.parametrize("retries", [0, 1, 2])
async def test_retry_budget_includes_last_attempt_validation(component, retries):
    obj, provider = component(['{"decision":"approve"}'] * (retries + 2), retries=retries)
    await obj.invoke_async()
    assert obj.output("structured") is None
    assert "amount" in obj.error()
    assert len(provider.calls) == retries + 1
    assert sum(call["mode"] == "tools" for call in provider.calls) == int(bool(obj.tools))


async def test_last_allowed_repair_can_succeed(component):
    obj, provider = component(['{"decision":"approve"}', '{"amount":50}', json.dumps(VALID)], retries=2)
    await obj.invoke_async()
    assert obj.output("structured") == VALID
    assert not obj.error()
    assert len(provider.calls) == 3


async def test_missing_root_type_is_normalized_to_object(component):
    schema = {key: value for key, value in SCHEMA.items() if key != "type"}
    obj, provider = component(["null", json.dumps(VALID)], retries=1, schema=schema)
    await obj.invoke_async()
    assert obj.output("structured") == VALID
    assert not obj.error()
    assert len(provider.calls) == 2
    assert '\\"type\\": \\"object\\"' in json.dumps(provider.calls[0])


@pytest.mark.parametrize(
    "schema",
    [
        {**SCHEMA, "required": "amount"},
        {**SCHEMA, "$schema": "https://example.invalid/unknown-schema"},
        {**SCHEMA, "type": "array"},
        {**SCHEMA, "type": ["object", "null"]},
    ],
)
async def test_invalid_schema_fails_before_generation(component, schema):
    obj, provider = component([json.dumps(VALID)], schema=schema)
    await obj.invoke_async()
    assert obj.output("structured") is None
    assert obj.error()
    assert provider.calls == []


async def test_local_schema_reference_is_validated(component):
    schema = {"type": "object", "properties": {"amount": {"$ref": "#/$defs/amount"}}, "$defs": {"amount": {"type": "integer", "minimum": 0}}, "required": ["amount"]}
    obj, provider = component(['{"amount":-1}', '{"amount":5}'], retries=1, schema=schema)
    await obj.invoke_async()
    assert obj.output("structured") == {"amount": 5}
    assert len(provider.calls) == 2


async def test_external_schema_reference_never_fetches_or_retries(component, monkeypatch):
    def forbid_network(*args, **kwargs):
        pytest.fail("Schema validation must not retrieve external resources")

    monkeypatch.setattr("urllib.request.urlopen", forbid_network)
    schema = {"type": "object", "properties": {"amount": {"$ref": "https://example.invalid/amount"}}}
    obj, provider = component(['{"amount":5}'] * 3, retries=2, schema=schema)
    await obj.invoke_async()
    assert obj.output("structured") is None
    assert obj.error()
    assert len(provider.calls) == 1


@pytest.mark.parametrize("cancel_after", [1, 2])
async def test_cancellation_discards_output_and_stops_repairs(component, cancel_after):
    obj, provider = component(['{"amount":5}', json.dumps(VALID)], retries=2)
    provider.cancel_after = cancel_after
    await obj.invoke_async()
    assert obj.output("structured") is None
    assert "canceled" in obj.error()
    assert len(provider.calls) == cancel_after


async def test_cancellation_before_call_has_no_generation(component):
    obj, provider = component([json.dumps(VALID)])
    obj._canvas.canceled = True
    await obj.invoke_async()
    assert provider.calls == []
    assert "canceled" in obj.error()


async def test_asyncio_cancellation_propagates(component):
    obj, _ = component([asyncio.CancelledError()])
    with pytest.raises(asyncio.CancelledError):
        await obj.invoke_async()
    assert obj.output("structured") is None


@pytest.mark.parametrize("failure", [RuntimeError("provider unavailable"), "**ERROR**: provider unavailable"])
async def test_provider_failure_during_repair_preserves_error(component, failure):
    obj, provider = component(['{"amount":5}', failure], retries=1)
    await obj.invoke_async()
    assert obj.output("structured") is None
    assert "provider unavailable" in obj.error()
    assert len(provider.calls) == 2


async def test_plain_output_path_is_unchanged(component):
    obj, provider = component(["normal answer"])
    obj._param.outputs = {}
    await obj.invoke_async()
    assert obj.output("content") == "normal answer"
    assert not obj.error()
    assert len(provider.calls) == 1


async def test_no_tools_option_is_per_call(component):
    obj, provider = component(["first", "repair", "last"])
    bundle = obj.chat_mdl
    bundle.mdl.is_tools = True
    await bundle.async_chat("system", [{"role": "user", "content": "first"}])
    await bundle.async_chat("system", [{"role": "user", "content": "repair"}], use_tools=False)
    await bundle.async_chat("system", [{"role": "user", "content": "last"}])
    assert [call["mode"] for call in provider.calls] == ["tools", "plain", "tools"]
    assert all("use_tools" not in call["kwargs"] for call in provider.calls)


@pytest.mark.parametrize("answer", ["null", "[]", '{"decision":"approve","amount":true}', '{"decision":"approve","amount":NaN}'])
async def test_invalid_root_boolean_number_and_nonfinite_values(component, answer):
    obj, provider = component([answer])
    await obj.invoke_async()
    assert obj.output("structured") is None
    assert obj.error()
    assert len(provider.calls) == 1


async def test_nested_array_error_includes_field_path(component):
    schema = {
        "type": "object",
        "properties": {"orders": {"type": "array", "items": {"type": "object", "properties": {"amount": {"type": "integer"}}, "required": ["amount"]}}},
        "required": ["orders"],
    }
    obj, _ = component(['{"orders":[{"amount":"invalid"}]}'], schema=schema)
    await obj.invoke_async()
    assert obj.output("structured") is None
    assert "$.orders[0].amount" in obj.error()


async def test_explicit_draft7_schema_and_local_ref(component):
    schema = {
        "$schema": "http://json-schema.org/draft-07/schema#",
        "type": "object",
        "properties": {"amount": {"$ref": "#/definitions/amount"}},
        "definitions": {"amount": {"type": "number", "exclusiveMinimum": 0}},
        "required": ["amount"],
    }
    obj, provider = component(['{"amount":0}', '{"amount":1}'], retries=1, schema=schema)
    await obj.invoke_async()
    assert obj.output("structured") == {"amount": 1}
    assert len(provider.calls) == 2


async def test_prompt_fit_failure_stops_before_model_call(component, monkeypatch):
    obj, provider = component([json.dumps(VALID)])
    monkeypatch.setattr(type(obj), "fit_messages", staticmethod(lambda *a: ([], "**ERROR**: context budget exceeded")))
    if obj.tools:
        obj._fit_messages = lambda *a: ([], "**ERROR**: context budget exceeded")
    await obj.invoke_async()
    assert obj.output("structured") is None
    assert "context budget exceeded" in obj.error()
    assert provider.calls == []


@pytest.mark.parametrize("repair_succeeds", [False, True])
async def test_canvas_only_schedules_downstream_after_valid_output(component, modules, monkeypatch, repair_succeeds):
    responses = ["null", json.dumps(VALID) if repair_succeeds else "[]"]
    schema = {key: value for key, value in SCHEMA.items() if key != "type"}
    obj, provider = component(responses, retries=1, schema=schema)
    received = []

    class Param(modules.base.ComponentParamBase):
        def check(self):
            pass

    class Begin(modules.base.ComponentBase):
        component_name = "Begin"

        async def _invoke_async(self, **kwargs):
            pass

        def thoughts(self):
            return ""

    class Sink(Begin):
        component_name = "Sink"

        async def _invoke_async(self, **kwargs):
            received.append(self._canvas.get_component_obj("generator").output("structured"))

    def create_generator(canvas, cid, param):
        obj._canvas, obj._id = canvas, cid
        return obj

    registry = {"Begin": Begin, "Sink": Sink, "Generator": create_generator}
    monkeypatch.setattr(sys.modules["agent.component"], "component_class", lambda name: Param if name.endswith("Param") else registry[name], raising=False)
    monkeypatch.setattr(sys.modules["common.misc_utils"], "get_uuid", lambda: "test-run", raising=False)
    monkeypatch.setattr(sys.modules["common.misc_utils"], "hash_str2int", lambda *a: 1, raising=False)
    monkeypatch.setattr(sys.modules["common.token_utils"], "token_usage_sink", ContextVar("test_usage", default=None), raising=False)
    monkeypatch.setattr(sys.modules["common.token_utils"], "langfuse_run_attrs", ContextVar("test_attrs", default=None))
    monkeypatch.setattr(sys.modules["api.db.joint_services.tenant_model_service"], "get_tenant_default_model_by_type", lambda *a: {}, raising=False)
    monkeypatch.setattr(sys.modules["rag.prompts.generator"], "chunks_format", lambda *a: [], raising=False)
    modules.stub("agent.dsl_migration", normalize_chunker_dsl=lambda dsl: dsl)
    modules.stub("api.db.services.file_service", FileService=object)
    modules.stub("api.db.services.task_service", has_canceled=lambda task_id: False)
    modules.stub("common.llm_request_context", set_llm_request_context=lambda **k: None, reset_llm_request_context=lambda token: None)
    modules.stub("common.exceptions", TaskCanceledException=type("TaskCanceledException", (Exception,), {}))
    modules.stub("rag.utils.redis_conn", REDIS_CONN=object())
    modules.stub("rag.utils.tts_cache", synthesize_with_cache=lambda *a, **k: None)
    canvas_module = modules.load("agent.canvas")
    nodes = [("begin", "Begin", [], ["generator"]), ("generator", "Generator", ["begin"], ["sink"]), ("sink", "Sink", ["generator"], [])]
    dsl = {
        "components": {cid: {"obj": {"component_name": name, "params": {}}, "upstream": upstream, "downstream": downstream} for cid, name, upstream, downstream in nodes},
        "history": [],
        "retrieval": [],
        "path": [],
    }
    canvas = canvas_module.Canvas(json.dumps(dsl), tenant_id="test-tenant", task_id="test-run")
    try:
        events = [event async for event in canvas.run(query="Review this order")]
    finally:
        canvas._thread_pool.shutdown(wait=True)

    finished = next(event["data"] for event in events if event["event"] == "node_finished" and event["data"]["component_id"] == "generator")
    assert len(provider.calls) == 2
    assert sum(call["mode"] == "tools" for call in provider.calls) == int(bool(obj.tools))
    if repair_succeeds:
        assert received == [VALID]
        assert finished["outputs"]["structured"] == VALID
        assert not finished["error"]
        assert any(event["event"] == "workflow_finished" for event in events)
    else:
        assert received == []
        assert finished["outputs"]["structured"] is None
        assert "object" in finished["error"]
        assert canvas.error
        assert not any(event["event"] == "node_started" and event["data"]["component_id"] == "sink" for event in events)
