import asyncio
from functools import partial
from types import SimpleNamespace

import pytest

from agent.component.agent_with_tools import Agent

SECRET_AGENT_SYSTEM_PROMPT = "SECRET_AGENT1_SYSTEM_PROMPT_s3cr3t"
SECRET_USER_QUESTION = "USER_Q_SECRET_XYZ"
PRODUCED_ANSWER = "GLORP_ANSWER_42"


class _FakeCanvas:
    """Minimum canvas surface the exercised methods touch (no model, no DB)."""

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
    return SimpleNamespace(
        inputs={},
        outputs={},
        prompts=prompts,
        sys_prompt=sys_prompt,
        cite=False,
        visual_files_var=None,
        message_history_window_size=13,
        llm_id="test-llm",
        max_retries=0,
        delay_after_error=0,
        exception_method=None,
        exception_default_value=None,
        exception_goto=None,
        debug_inputs={},
    )


def _make_agent(canvas, cid, param, tools):
    agent = Agent.__new__(Agent)
    agent._canvas = canvas
    agent._id = cid
    agent._param = param
    agent.tools = tools
    agent.chat_mdl = SimpleNamespace(max_length=8192)
    agent.imgs = []
    return agent


def _run_agent2_with_upstream(upstream_value):
    """Agent2 references Agent1@content; returns its normalized user input."""
    canvas = _FakeCanvas()
    canvas.vars["Agent1@content"] = upstream_value
    canvas.objs["agent1"] = SimpleNamespace(component_name="Agent")

    param = _make_param(
        [{"role": "user", "content": "Use this: {Agent1@content}"}],
        sys_prompt="AGENT2_SYS",
    )
    agent2 = _make_agent(canvas, "agent2", param, {"retrieval_0": object()})
    _, msg, _ = agent2._prepare_prompt_variables()
    return next(m for m in msg if m.get("role") == "user")["content"]


@pytest.mark.p1
def test_no_message_downstream_yields_eager_answer_string():
    """Without a direct Message downstream, Agent content is the eager
    answer string (pre-existing contract)."""
    canvas = _FakeCanvas()
    canvas.components["agent1"] = {"downstream": ["agent2"]}
    canvas.objs["agent2"] = SimpleNamespace(component_name="Agent")

    param = _make_param([{"role": "user", "content": "What is glorp?"}], sys_prompt="SYS")
    agent1 = _make_agent(canvas, "agent1", param, {"retrieval_0": object()})
    agent1._fit_messages = lambda prompt, msg: (msg, None)

    async def fake_generate(msg):
        return PRODUCED_ANSWER

    agent1._generate_async = fake_generate

    result = asyncio.run(agent1._invoke_async())
    content = agent1._param.outputs["content"]["value"]
    assert result == PRODUCED_ANSWER
    assert isinstance(content, str)
    assert content == PRODUCED_ANSWER


@pytest.mark.p1
def test_pure_message_downstream_keeps_deferred_streaming_partial():
    """A sole direct Message downstream still gets the deferred streaming
    partial (streaming behavior preserved, no model call made here)."""
    canvas = _FakeCanvas()
    canvas.components["agent1"] = {"downstream": ["msg1"]}
    canvas.objs["msg1"] = SimpleNamespace(component_name="Message")

    param = _make_param(
        [{"role": "user", "content": SECRET_USER_QUESTION}],
        sys_prompt=SECRET_AGENT_SYSTEM_PROMPT,
    )
    agent1 = _make_agent(canvas, "agent1", param, {"retrieval_0": object()})

    asyncio.run(agent1._invoke_async())
    content = agent1._param.outputs["content"]["value"]
    assert isinstance(content, partial)
    assert content.args[0] == SECRET_AGENT_SYSTEM_PROMPT


@pytest.mark.p1
def test_mixed_agent_and_message_downstream_sees_answer_not_partial():
    """Regression for https://github.com/infiniflow/ragflow/issues/19556.

    With a mixed graph (Agent1 -> Agent2 plus Agent1 -> Message), Agent1's
    semantic content must be the eager answer string -- never a partial --
    so Agent2's input cannot leak bound system prompt/message state.
    """
    canvas = _FakeCanvas()
    canvas.components["agent1"] = {"downstream": ["agent2", "msg1"]}
    canvas.objs["agent2"] = SimpleNamespace(component_name="Agent")
    canvas.objs["msg1"] = SimpleNamespace(component_name="Message")

    param = _make_param(
        [{"role": "user", "content": SECRET_USER_QUESTION}],
        sys_prompt=SECRET_AGENT_SYSTEM_PROMPT,
    )
    agent1 = _make_agent(canvas, "agent1", param, {"retrieval_0": object()})
    agent1._fit_messages = lambda prompt, msg: (msg, None)
    calls = []

    async def fake_generate(msg):
        calls.append(1)
        return PRODUCED_ANSWER

    agent1._generate_async = fake_generate
    asyncio.run(agent1._invoke_async())

    content = agent1._param.outputs["content"]["value"]
    assert content == PRODUCED_ANSWER
    assert not isinstance(content, partial)
    # Both consumers share the single eager result: one generation call.
    assert len(calls) == 1

    normalized = _run_agent2_with_upstream(content)
    assert "functools.partial" not in normalized
    assert SECRET_AGENT_SYSTEM_PROMPT not in normalized
    assert PRODUCED_ANSWER in normalized
