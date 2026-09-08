"""Saved Retrieval Canvas contract using the actual tool registry."""

import json
import os
from pathlib import Path
import subprocess
import sys


def test_retrieval_canvas_loads_through_dynamic_tool_registry(tmp_path):
    fixture = tmp_path / "retrieval-canvas.json"
    fixture.write_text(
        json.dumps(
            {
                "components": {
                    "Retrieval:Contract": {
                        "obj": {
                            "component_name": "Retrieval",
                            "params": {
                                "dataset_ids": [],
                                "kb_ids": ["legacy-dataset-id"],
                                "memory_ids": ["memory-id"],
                                "meta_data_filter": {},
                                "similarity_threshold": 0.2,
                                "keywords_similarity_weight": 0.5,
                                "top_n": 8,
                                "top_k": 1024,
                            },
                        },
                        "downstream": [],
                        "upstream": [],
                    }
                },
                "history": [],
                "path": [],
                "retrieval": {"chunks": [], "doc_aggs": []},
                "globals": {
                    "sys.query": "",
                    "sys.user_id": "isolated-test",
                    "sys.conversation_turns": 0,
                    "sys.files": [],
                },
            }
        ),
        encoding="utf-8",
    )
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

assert component_class("Retrieval").__module__ == "agent.tools.retrieval"
assert component_class("RetrievalParam").__module__ == "agent.tools.retrieval"
with open(sys.argv[1], encoding="utf-8") as source:
    document = json.load(source)
canvas = Canvas(json.dumps(document), tenant_id="isolated-test")
try:
    component = canvas.components["Retrieval:Contract"]["obj"]
    assert isinstance(component, component_class("Retrieval"))
    assert component._dataset_ids == ["legacy-dataset-id"]
    assert component._param.memory_ids == ["memory-id"]
    assert component._param.meta_data_filter == {}
    saved = str(canvas)
finally:
    canvas._thread_pool.shutdown(wait=True)
restored = Canvas(saved, tenant_id="isolated-test")
try:
    component = restored.components["Retrieval:Contract"]["obj"]
    assert component._dataset_ids == ["legacy-dataset-id"]
    assert component._param.memory_ids == ["memory-id"]
finally:
    restored._thread_pool.shutdown(wait=True)
assert not blocked_network, blocked_network
print("DYNAMIC_RETRIEVAL_REGISTRY_OK")
"""
    environment = {**os.environ, "LITELLM_LOCAL_MODEL_COST_MAP": "True", "HF_HUB_OFFLINE": "1", "TRANSFORMERS_OFFLINE": "1"}
    result = subprocess.run(
        [sys.executable, "-c", script, str(fixture)],
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
    assert "DYNAMIC_RETRIEVAL_REGISTRY_OK" in result.stdout
    assert "Exception ignored in:" not in result.stderr, result.stderr[-4000:]
