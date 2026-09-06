"""Saved Canvas contract using the actual registry in an isolated interpreter."""

import json
import os
from pathlib import Path
import subprocess
import sys

from common.openmetadata_agents import OPENMETADATA_AGENT_ROLES, build_openmetadata_agent_dsl


def test_saved_openmetadata_canvas_loads_through_dynamic_registry(tmp_path):
    fixtures = tmp_path / "canvases.json"
    fixtures.write_text(json.dumps([build_openmetadata_agent_dsl(role) for role in OPENMETADATA_AGENT_ROLES]), encoding="utf-8")
    script = r"""
import json
import socket
import sys

blocked_network = []
socketpair_code = getattr(socket.socketpair, "__code__", None)

def deny_network(event, args):
    if event not in {"socket.connect", "socket.getaddrinfo", "socket.sendto"}:
        return
    # Windows asyncio creates a private socketpair via a temporary TCP listener.
    # Allow only that stdlib frame connecting to its own just-created listener.
    caller = sys._getframe(1)
    if event == "socket.connect" and caller.f_code is socketpair_code:
        listener = caller.f_locals.get("lsock")
        if listener is not None and args[1] == listener.getsockname()[:2]:
            return
    blocked_network.append(event)
    raise RuntimeError("Dynamic-registry test forbids external network access")

sys.addaudithook(deny_network)

from agent.component import component_class
from agent.canvas import Canvas

assert component_class("OpenMetadata").__module__ == "agent.component.openmetadata"
assert component_class("OpenMetadataParam").__module__ == "agent.component.openmetadata"
with open(sys.argv[1], encoding="utf-8") as source:
    documents = json.load(source)
for dsl in documents:
    role = dsl["meta"]["role_id"]
    component_id = f"OpenMetadata:{role.replace('_', '')}"
    canvas = Canvas(json.dumps(dsl), tenant_id="isolated-test")
    try:
        component = canvas.components[component_id]["obj"]
        assert isinstance(component, component_class("OpenMetadata"))
        assert component._param.role == role
        assert set(component._param.outputs) >= {"content", "result", "entity_ids"}
        saved = str(canvas)
    finally:
        canvas._thread_pool.shutdown(wait=True)
    restored = Canvas(saved, tenant_id="isolated-test")
    try:
        assert restored.components[component_id]["obj"]._param.role == role
        assert restored.components[component_id]["downstream"] == [f"Message:{role}"]
    finally:
        restored._thread_pool.shutdown(wait=True)
    invalid = json.loads(saved)
    invalid["components"][component_id]["obj"]["params"]["role"] = "unknown-persisted-role"
    try:
        Canvas(json.dumps(invalid), tenant_id="isolated-test")
    except ValueError as error:
        assert "Role" in str(error)
    else:
        raise AssertionError("Invalid persisted role must fail parameter validation")
assert not blocked_network, blocked_network
print(f"DYNAMIC_REGISTRY_OK:{len(documents)}")
"""
    environment = {**os.environ, "LITELLM_LOCAL_MODEL_COST_MAP": "True", "HF_HUB_OFFLINE": "1", "TRANSFORMERS_OFFLINE": "1"}
    result = subprocess.run(
        [sys.executable, "-c", script, str(fixtures)],
        cwd=Path(__file__).resolve().parents[3],
        env=environment,
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
        timeout=120,
        check=False,
    )
    assert result.returncode == 0, result.stdout[-4000:] + result.stderr[-4000:]
    assert f"DYNAMIC_REGISTRY_OK:{len(OPENMETADATA_AGENT_ROLES)}" in result.stdout
    assert "Exception ignored in:" not in result.stderr, result.stderr[-4000:]
