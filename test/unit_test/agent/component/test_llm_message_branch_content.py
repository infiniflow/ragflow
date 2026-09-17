import asyncio
from functools import partial
from types import SimpleNamespace

import pytest

import agent.component.llm as llm_module
from agent.component.llm import LLM, LLMParam

SECRET_LLM_SYSTEM_PROMPT = "SECRET_LLM_SYSTEM_PROMPT_s3cr3t"
SECRET_USER_QUESTION = "USER_Q_SECRET_XYZ"
PRODUCED_ANSWER = "GLORP_ANSWER_42"


class _FakeCanvas:
    """Minimum canvas surface LLM._invoke_async touches (no model, no DB)."""

    def __init__(self):
        self.vars = {}
        self.components = {}
        self.objs = {}
        self.globals = {}
        self.task_id = "test-task"

    def get_component(self, cid):
        return self.components[cid]

    def get_component_obj(self, cid):
        return self.objs[cid]

    def get_variable_value(self, exp):
        if exp.endswith("@_references"):
            return self.vars.get(exp)
        return self.vars[exp]

    def get_component_name(self, cid):
        obj = self.objs.get(cid)
        if obj is None:
            obj = {k.lower(): v for k, v in self.objs.items()}[cid.lower()]
        return obj.component_name.lower()

    def get_tenant_id(self):
        return "tenant1"

    def get_history(self, n):
        return []

    def get_reference(self):
        return {"chunks": []}

    def is_reff(self, v):
        return False

    def is_canceled(self):
        return False


def _make_param(prompts, sys_prompt=""):
    param = LLMParam()
    param.prompts = prompts
    param.sys_prompt = sys_prompt
    param.debug_inputs = {}
    return param


def _make_llm(canvas, cid, param):
    cpn = LLM.__new__(LLM)
    cpn._canvas = canvas
    cpn._id = cid
    cpn._param = param
    cpn.chat_mdl = SimpleNamespace(max_length=8192)
    cpn.imgs = []
    return cpn


@pytest.fixture
def no_model_resolution(monkeypatch):
    """_prepare_prompt_variables resolves tenant model config through the DB;
    stub it so the test needs no MySQL/credentials/model."""
    monkeypatch.setattr(llm_module, "resolve_model_type", lambda tenant, ref: ["chat"])
    monkeypatch.setattr(llm_module, "resolve_model_config", lambda tenant, mtype, ref: {})
    monkeypatch.setattr(llm_module, "LLMBundle", lambda *a, **k: SimpleNamespace(max_length=8192))


def _run_llm2_with_upstream(upstream_value):
    """A downstream LLM references LLM1@content; return its normalized user input."""
    canvas = _FakeCanvas()
    canvas.vars["LLM1@content"] = upstream_value
    canvas.objs["llm1"] = SimpleNamespace(component_name="LLM")

    param = _make_param([{"role": "user", "content": "Use this: {LLM1@content}"}])
    llm2 = _make_llm(canvas, "llm2", param)
    _, msg, _ = llm2._prepare_prompt_variables()
    return next(m for m in msg if m.get("role") == "user")["content"]


@pytest.mark.p1
def test_no_message_downstream_yields_eager_answer_string(no_model_resolution):
    """Without a direct Message downstream, content is the eager answer string
    (pre-existing contract)."""
    canvas = _FakeCanvas()
    canvas.components["llm1"] = {"downstream": ["llm2"]}
    canvas.objs["llm2"] = SimpleNamespace(component_name="LLM")

    llm1 = _make_llm(canvas, "llm1", _make_param([{"role": "user", "content": "What is glorp?"}], "SYS"))

    async def fake_generate(msg, **kwargs):
        return PRODUCED_ANSWER

    llm1._generate_async = fake_generate

    asyncio.run(llm1._invoke_async())
    content = llm1._param.outputs["content"]["value"]
    assert isinstance(content, str)
    assert content == PRODUCED_ANSWER


@pytest.mark.p1
def test_pure_message_downstream_keeps_deferred_streaming_partial(no_model_resolution):
    """A sole direct Message downstream still gets the deferred streaming partial
    (streaming behavior preserved, no model call made here)."""
    canvas = _FakeCanvas()
    canvas.components["llm1"] = {"downstream": ["msg1"]}
    canvas.objs["msg1"] = SimpleNamespace(component_name="Message")

    llm1 = _make_llm(canvas, "llm1", _make_param([{"role": "user", "content": SECRET_USER_QUESTION}], SECRET_LLM_SYSTEM_PROMPT))

    async def refusing_generate(msg, **kwargs):
        raise AssertionError("Message-only graph must not generate eagerly")

    llm1._generate_async = refusing_generate

    asyncio.run(llm1._invoke_async())
    content = llm1._param.outputs["content"]["value"]
    assert isinstance(content, partial)
    assert content.args[0] == SECRET_LLM_SYSTEM_PROMPT


@pytest.mark.p1
def test_mixed_llm_and_message_downstream_sees_answer_not_partial(no_model_resolution):
    """Regression for the no-tool path of #19556.

    A mixed graph (LLM1 -> LLM2 plus LLM1 -> Message) must keep eager
    execution: LLM2 would otherwise stringify the deferred partial and observe
    the upstream system prompt instead of the semantic answer.
    """
    canvas = _FakeCanvas()
    canvas.components["llm1"] = {"downstream": ["llm2", "msg1"]}
    canvas.objs["llm2"] = SimpleNamespace(component_name="LLM")
    canvas.objs["msg1"] = SimpleNamespace(component_name="Message")

    llm1 = _make_llm(canvas, "llm1", _make_param([{"role": "user", "content": SECRET_USER_QUESTION}], SECRET_LLM_SYSTEM_PROMPT))
    calls = []

    async def fake_generate(msg, **kwargs):
        calls.append(1)
        return PRODUCED_ANSWER

    llm1._generate_async = fake_generate
    asyncio.run(llm1._invoke_async())

    content = llm1._param.outputs["content"]["value"]
    assert content == PRODUCED_ANSWER
    assert not isinstance(content, partial)
    # Both consumers share the single eager result: one generation call.
    assert len(calls) == 1

    normalized = _run_llm2_with_upstream(content)
    assert "functools.partial" not in normalized
    assert SECRET_LLM_SYSTEM_PROMPT not in normalized
    assert PRODUCED_ANSWER in normalized


@pytest.mark.p1
def test_no_tool_agent_delegates_to_llm_mixed_branch(no_model_resolution):
    """A no-tool Agent falls back to LLM._invoke_async (agent_with_tools.py),
    so the mixed-branch contract must hold there too."""
    from agent.component.agent_with_tools import Agent, AgentParam

    canvas = _FakeCanvas()
    canvas.components["agent1"] = {"downstream": ["agent2", "msg1"]}
    canvas.objs["agent2"] = SimpleNamespace(component_name="Agent")
    canvas.objs["msg1"] = SimpleNamespace(component_name="Message")

    param = AgentParam()
    param.prompts = [{"role": "user", "content": SECRET_USER_QUESTION}]
    param.sys_prompt = SECRET_LLM_SYSTEM_PROMPT
    param.debug_inputs = {}

    agent1 = Agent.__new__(Agent)
    agent1._canvas = canvas
    agent1._id = "agent1"
    agent1._param = param
    agent1.chat_mdl = SimpleNamespace(max_length=8192)
    agent1.imgs = []
    agent1.tools = {}
    calls = []

    async def fake_generate(msg, **kwargs):
        calls.append(1)
        return PRODUCED_ANSWER

    agent1._generate_async = fake_generate
    asyncio.run(agent1._invoke_async())

    content = agent1._param.outputs["content"]["value"]
    assert content == PRODUCED_ANSWER
    assert not isinstance(content, partial)
    assert len(calls) == 1
