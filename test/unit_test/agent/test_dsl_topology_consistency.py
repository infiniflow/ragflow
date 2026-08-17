import copy

import pytest

from agent.dsl_migration import normalize_chunker_dsl


def _component(name, downstream=None, upstream=None):
    component = {"obj": {"component_name": name, "params": {}}}
    if downstream is not None:
        component["downstream"] = downstream
    if upstream is not None:
        component["upstream"] = upstream
    return component


def _node(component_id, label):
    return {"id": component_id, "data": {"label": label, "name": label, "form": {}}}


def test_graph_edges_replace_conflicting_runtime_topology():
    dsl = {
        "components": {
            "begin": _component("Begin", ["load"], []),
            "quality": _component("Quality", [], []),
            "load": _component("Message", [], ["begin"]),
        },
        "graph": {
            "nodes": [_node("begin", "Begin"), _node("quality", "Quality"), _node("load", "Message")],
            "edges": [
                {"source": "begin", "target": "quality"},
                {"source": "quality", "target": "load"},
            ],
        },
    }

    normalized = normalize_chunker_dsl(dsl)

    assert normalized["components"]["begin"]["downstream"] == ["quality"]
    assert normalized["components"]["quality"]["upstream"] == ["begin"]
    assert normalized["components"]["quality"]["downstream"] == ["load"]
    assert normalized["components"]["load"]["upstream"] == ["quality"]
    assert dsl["components"]["begin"]["downstream"] == ["load"]


def test_components_only_dsl_derives_upstream_from_historical_downstream():
    dsl = {
        "components": {
            "begin": _component("Begin", ["quality"]),
            "quality": _component("Quality", ["load"]),
            "load": _component("Message", []),
        }
    }

    normalized = normalize_chunker_dsl(dsl)

    assert normalized["components"]["begin"]["upstream"] == []
    assert normalized["components"]["quality"]["upstream"] == ["begin"]
    assert normalized["components"]["load"]["upstream"] == ["quality"]


def test_graph_projection_preserves_duplicates_branches_cycles_and_isolated_nodes():
    dsl = {
        "components": {
            "begin": _component("Begin"),
            "branch": _component("Switch"),
            "loop": _component("Loop"),
            "iteration": _component("Iteration"),
            "load": _component("Message"),
            "isolated": _component("Message"),
        },
        "graph": {
            "nodes": [
                _node("begin", "Begin"),
                _node("branch", "Switch"),
                _node("loop", "Loop"),
                _node("iteration", "Iteration"),
                _node("load", "Message"),
                _node("isolated", "Message"),
            ],
            "edges": [
                {"source": "begin", "target": "branch"},
                {"source": "branch", "target": "loop", "sourceHandle": "case-a"},
                {"source": "branch", "target": "iteration", "sourceHandle": "case-b"},
                {"source": "loop", "target": "load"},
                {"source": "iteration", "target": "load"},
                {"source": "iteration", "target": "load"},
                {"source": "loop", "target": "loop"},
            ],
        },
    }

    normalized = normalize_chunker_dsl(dsl)["components"]

    assert normalized["branch"]["downstream"] == ["loop", "iteration"]
    assert normalized["load"]["upstream"] == ["loop", "iteration", "iteration"]
    assert normalized["loop"]["upstream"] == ["branch", "loop"]
    assert normalized["isolated"]["upstream"] == []
    assert normalized["isolated"]["downstream"] == []


def test_agent_control_edges_keep_frontend_runtime_projection_rules():
    dsl = {
        "components": {
            "begin": _component("Begin"),
            "agent": _component("Agent"),
            "fallback": _component("Message"),
        },
        "graph": {
            "nodes": [
                _node("begin", "Begin"),
                _node("agent", "Agent"),
                _node("subagent", "Agent"),
                _node("tool", "Tool"),
                _node("fallback", "Message"),
                _node("note", "Note"),
            ],
            "edges": [
                {"source": "begin", "target": "agent"},
                {"source": "agent", "target": "subagent", "targetHandle": "agentTop"},
                {"source": "agent", "target": "tool"},
                {"source": "agent", "target": "fallback", "sourceHandle": "agentException"},
            ],
        },
    }

    normalized = normalize_chunker_dsl(dsl)["components"]

    assert normalized["agent"]["downstream"] == []
    assert normalized["fallback"]["upstream"] == ["agent"]


def test_executable_graph_node_without_component_is_rejected():
    dsl = {
        "components": {"begin": _component("Begin")},
        "graph": {
            "nodes": [_node("begin", "Begin"), _node("quality", "Quality")],
            "edges": [{"source": "begin", "target": "quality"}],
        },
    }

    with pytest.raises(ValueError, match="missing components=.*quality"):
        normalize_chunker_dsl(dsl)


def test_component_without_graph_node_is_rejected():
    dsl = {
        "components": {"begin": _component("Begin"), "hidden": _component("Message")},
        "graph": {"nodes": [_node("begin", "Begin")], "edges": []},
    }

    with pytest.raises(ValueError, match="missing graph nodes=.*hidden"):
        normalize_chunker_dsl(dsl)


def test_legacy_component_rename_happens_before_topology_projection():
    dsl = {
        "components": {
            "begin": _component("Begin", ["Splitter:1"], []),
            "Splitter:1": _component("Splitter", [], ["begin"]),
        },
        "graph": {
            "nodes": [_node("begin", "Begin"), _node("Splitter:1", "Splitter")],
            "edges": [{"source": "begin", "target": "Splitter:1"}],
        },
    }

    normalized = normalize_chunker_dsl(copy.deepcopy(dsl))

    assert normalized["components"]["begin"]["downstream"] == ["TokenChunker:1"]
    assert normalized["components"]["TokenChunker:1"]["upstream"] == ["begin"]


def test_legacy_iteration_with_empty_graph_edges_keeps_runtime_body_links():
    dsl = {
        "components": {
            "iteration": _component("Iteration", ["item"]),
            "item": _component("IterationItem", ["body"]),
            "body": _component("Message", []),
        },
        "graph": {
            "nodes": [
                _node("iteration", "Iteration"),
                _node("item", "IterationItem"),
                _node("body", "Message"),
            ],
            "edges": [],
        },
    }

    normalized = normalize_chunker_dsl(dsl)["components"]

    assert normalized["iteration"]["downstream"] == ["item"]
    assert normalized["item"]["upstream"] == ["iteration"]
    assert normalized["body"]["upstream"] == ["item"]
