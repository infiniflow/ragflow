# Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Exercise the webhook routes with the shared in-memory webhook test doubles."""

import importlib.util
import json
import sys
from pathlib import Path
from types import SimpleNamespace

import pytest


@pytest.fixture
def webhook_stubs():
    path = Path(__file__).resolve().parents[5] / "test/testcases/test_web_api/test_agent_app/test_agents_webhook_unit.py"
    spec = importlib.util.spec_from_file_location("webhook_test_support", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


@pytest.mark.parametrize("execution_mode", ["Immediately", "Deferred"])
@pytest.mark.parametrize(
    "is_test, trace_available",
    [(True, True), (True, False), (False, True)],
    ids=["test-trigger", "trace-unavailable", "production-trigger"],
)
@pytest.mark.parametrize(
    "outcome, expected_status, terminal_event",
    [("success", 201, None), ("recovered", 201, None), ("failed", 500, "error"), ("cancelled", 409, "cancelled")],
)
def test_webhook_reports_canvas_outcome(monkeypatch, webhook_stubs, execution_mode, is_test, trace_available, outcome, expected_status, terminal_event):
    stubs = webhook_stubs
    module = stubs._load_agents_app(monkeypatch)
    redis_client = stubs._FakeRedisClient()
    module.REDIS_CONN.REDIS = redis_client
    key = "webhook-trace-agent-1-logs"
    if not trace_available:
        # A malformed stored payload makes every best-effort append fail.
        redis_client.values[key] = "{"

    class Canvas(stubs._StubCanvas):
        error = ""

        async def run(self, **_kwargs):
            # A partial answer or a node-local error alone does not determine
            # the run outcome: exception defaults/goto may recover that node.
            yield {"event": "message", "data": {"content": "partial answer"}}
            yield {"event": "message_end", "data": {"status": 201}}
            if outcome in {"failed", "recovered"}:
                if outcome == "failed":
                    self.error = "Component execution failed: private-component-id"
                yield {"event": "node_finished", "data": {"error": "private provider detail"}}
            elif outcome == "cancelled":
                self.error = "Task has been canceled"
                yield {"event": "workflow_finished", "data": {"outputs": self.error}}
            if not self.error:
                yield {"event": "workflow_finished", "data": {"outputs": "answer"}}

    monkeypatch.setattr(sys.modules["agent.canvas"], "Canvas", Canvas)
    params = stubs._default_webhook_params(
        security=stubs._anonymous_security(),
        execution_mode=execution_mode,
        response={"status": 202, "body_template": '{"accepted": true}'},
    )
    cvs = stubs._make_webhook_cvs(module, params=params)
    monkeypatch.setattr(module.UserCanvasService, "get_by_id", lambda _id: (True, cvs))
    monkeypatch.setattr(module.UserCanvasService, "query", lambda **_kwargs: [cvs])
    path = "/api/v1/agents/agent-1/webhook" + ("/test" if is_test else "")
    monkeypatch.setattr(module, "request", stubs._DummyRequest(path=path, headers={"Content-Type": "application/json"}, json_body={}))

    tasks = []

    def capture_task(coro):
        tasks.append(coro)
        return SimpleNamespace()

    monkeypatch.setattr(module.asyncio, "create_task", capture_task)
    route = module.webhook_test if is_test else module.webhook
    response = stubs._run(route(agent_id="agent-1"))
    if execution_mode == "Immediately":
        assert response.status_code == 202
        assert json.loads(stubs._run(response.get_data(as_text=True))) == {"accepted": True}
        assert len(tasks) == 1
        stubs._run(tasks.pop())
    else:
        assert response.status_code == expected_status
        payload = json.loads(stubs._run(response.get_data(as_text=True)))
        assert payload["code"] == expected_status
        assert payload["success"] is (terminal_event is None)
        if terminal_event:
            assert payload["message"] == ("Agent run was cancelled." if outcome == "cancelled" else "Agent run failed.")
        else:
            assert payload["message"] == "partial answer"

    if not is_test:
        assert redis_client.values == {}
        return
    if not trace_available:
        assert redis_client.values[key] == "{"
        return
    store = json.loads(redis_client.values[key])
    events = next(iter(store["webhooks"].values()))["events"]
    finished = [event for event in events if event["event"] == "finished"]
    assert len(finished) == 1
    assert events[-1] == finished[0]
    assert finished[0]["success"] is (terminal_event is None)
    terminals = [event for event in events if event["event"] in {"error", "cancelled"}]
    if terminal_event:
        assert len(terminals) == 1
        assert events[-2] == terminals[0]
        assert terminals[0]["event"] == terminal_event
        assert terminals[0]["message"] == ("Agent run was cancelled." if outcome == "cancelled" else "Agent run failed.")
    else:
        assert not terminals
