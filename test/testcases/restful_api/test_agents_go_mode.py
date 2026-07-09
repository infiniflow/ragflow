import os
import uuid
import pytest

pytestmark = pytest.mark.skipif(
    os.environ.get("API_PROXY_SCHEME", "python") != "go",
    reason="Go mode only (set API_PROXY_SCHEME=go + bin/ragflow_server on 9384)",
)

V2_DSL = {
    "graph": {
        "nodes": [
            {"id": "begin", "type": "beginNode", "position": {"x": 50, "y": 200}, "data": {"label": "Begin", "name": "begin"}},
            {"id": "answer:0", "type": "messageNode", "position": {"x": 400, "y": 200}, "data": {"label": "Answer", "name": "answer"}},
        ],
        "edges": [
            {"id": "xy-edge__begin-answer:0", "source": "begin", "target": "answer:0", "sourceHandle": "end", "targetHandle": "start"},
        ],
    },
    "components": {
        "begin": {"obj": {"component_name": "Begin", "params": {}}, "downstream": ["answer:0"], "upstream": []},
        "answer:0": {"obj": {"component_name": "Answer", "params": {}}, "downstream": [], "upstream": ["begin"]},
    },
    "retrieval": [],
    "history": [],
    "path": [],
    "variables": [],
    "globals": {"sys.query": ""},
}


@pytest.mark.p2
def test_v2_dsl_round_trip_position_preserved(rest_client):
    # Unique title per run so the test is idempotent (Go returns 102 on duplicate).
    title = f"go_v2_e2e_{uuid.uuid4().hex[:8]}"

    # 1. POST v2 DSL
    r = rest_client.post("/agents", json={"title": title, "dsl": V2_DSL})
    assert r.status_code == 200 and r.json()["code"] == 0, r.text
    agent_id = r.json()["data"]["id"]

    # 2. GET — verify v2 shape (graph + components, NO _layout)
    dsl = rest_client.get(f"/agents/{agent_id}").json()["data"]["dsl"]
    assert "graph" in dsl, f"v2 must have 'graph', got keys={sorted(dsl.keys())}"
    assert "_layout" not in dsl, f"v2 must NOT emit '_layout', got keys={sorted(dsl.keys())}"
    assert "components" in dsl
    assert len(dsl["graph"]["nodes"]) == 2
    assert len(dsl["graph"]["edges"]) == 1

    # 3. PUT — move begin node to (777, 888)
    new_dsl = {**V2_DSL, "graph": {**V2_DSL["graph"]}}
    new_dsl["graph"]["nodes"] = [{**n, "position": {"x": 777, "y": 888}} if n["id"] == "begin" else n for n in V2_DSL["graph"]["nodes"]]
    r = rest_client.put(f"/agents/{agent_id}", json={"title": title, "dsl": new_dsl})
    assert r.status_code == 200 and r.json()["code"] == 0, r.text

    # 4. Re-GET — position preserved
    pos = next(n["position"] for n in rest_client.get(f"/agents/{agent_id}").json()["data"]["dsl"]["graph"]["nodes"] if n["id"] == "begin")
    assert pos == {"x": 777, "y": 888}, f"position lost: {pos}"
