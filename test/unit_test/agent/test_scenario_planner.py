#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
from copy import deepcopy

import pytest

from agent.scenario_planner import ScenarioPlanner


pytestmark = pytest.mark.p2


def _edge_pairs(dsl):
    return {(edge["source"], edge["target"]) for edge in dsl["graph"]["edges"]}


def test_plan_defaults_to_qa_basic():
    planner = ScenarioPlanner()
    draft = planner.plan(
        title="QA Draft",
        scenario="Answer questions about an internal handbook.",
    )

    assert draft["archetype"] == "qa_basic"
    assert draft["mode"] == "create"
    assert draft["operations"] == [{"type": "create_draft", "archetype": "qa_basic"}]
    assert "dsl" in draft
    assert draft["dsl"]["path"] == ["begin"]
    assert "Agent:DraftAnswer" in draft["dsl"]["components"]


def test_plan_selects_interactive_research():
    planner = ScenarioPlanner()
    draft = planner.plan(
        title="Research Draft",
        scenario="Research the market, compare sources, and produce a short report.",
    )

    assert draft["archetype"] == "interactive_research"
    components = draft["dsl"]["components"]
    assert "Agent:Plan" in components
    assert "UserFillUp:ReviewPlan" in components
    assert "Agent:ExecuteResearch" in components


def test_plan_selects_monitor_notify():
    planner = ScenarioPlanner()
    draft = planner.plan(
        title="Monitor Draft",
        scenario="Monitor a website, detect changes, and notify me if anything changes.",
    )

    assert draft["archetype"] == "monitor_notify"
    assert draft["warnings"]
    components = draft["dsl"]["components"]
    assert "Switch:Decision" in components
    assert "Agent:Notify" in components


def test_plan_selects_batch_review():
    planner = ScenarioPlanner()
    draft = planner.plan(
        title="Batch Draft",
        scenario="For each uploaded resume, review it and summarize the fit.",
    )

    assert draft["archetype"] == "batch_review"
    components = draft["dsl"]["components"]
    assert "Iteration:Items" in components
    assert "IterationItem:Current" in components
    assert components["Agent:ReviewItem"]["parent_id"] == "Iteration:Items"


def test_plan_can_modify_existing_dsl_with_notification():
    planner = ScenarioPlanner()
    base = planner.plan(
        title="Base Draft",
        scenario="Answer questions about an internal handbook.",
    )["dsl"]
    base_snapshot = deepcopy(base)
    base_components = base["components"]
    base_nodes = base["graph"]["nodes"]
    base_edges = base["graph"]["edges"]

    edited = planner.plan(
        title="Edited Draft",
        scenario="Add a notification step after the current flow.",
        existing_dsl=base,
    )

    assert edited["archetype"] == "modify_existing"
    assert edited["mode"] == "modify"
    op = next(op for op in edited["operations"] if op["type"] == "append_notification")
    target_id = op["target"]
    components = edited["dsl"]["components"]
    graph = edited["dsl"]["graph"]

    assert base == base_snapshot
    assert target_id not in base_components
    assert len(components) == len(base_components) + 1
    assert len(graph["nodes"]) == len(base_nodes) + 1
    assert len(graph["edges"]) == len(base_edges) + 1
    assert components[target_id]["obj"]["component_name"] == "Message"
    assert components[target_id]["upstream"] == ["Message:Output"]
    assert target_id in components["Message:Output"]["downstream"]
    assert ("Message:Output", target_id) in _edge_pairs(edited["dsl"])
    assert target_id in {node["id"] for node in graph["nodes"]}


def test_plan_notification_prefers_message_output_tail_for_multi_terminal_graph():
    planner = ScenarioPlanner()
    base = planner.plan(
        title="Monitor Draft",
        scenario="Monitor a website, detect changes, and notify me if anything changes.",
    )["dsl"]

    edited = planner.plan(
        title="Edited Monitor Draft",
        scenario="Add a notification step after the current flow.",
        existing_dsl=base,
    )

    op = next(op for op in edited["operations"] if op["type"] == "append_notification")
    target_id = op["target"]
    components = edited["dsl"]["components"]

    assert components[target_id]["upstream"] == ["Message:Output"]
    assert target_id in components["Message:Output"]["downstream"]
    assert target_id not in components["Message:NoChange"]["downstream"]
    assert ("Message:Output", target_id) in _edge_pairs(edited["dsl"])
    assert ("Message:NoChange", target_id) not in _edge_pairs(edited["dsl"])


def test_plan_can_modify_existing_dsl_with_human_review():
    planner = ScenarioPlanner()
    base = planner.plan(
        title="Research Draft",
        scenario="Research the market, compare sources, and produce a short report.",
    )["dsl"]
    base_snapshot = deepcopy(base)
    base_components = base["components"]
    base_nodes = base["graph"]["nodes"]
    base_edges = base["graph"]["edges"]
    tail_id = "Message:Output"
    predecessor_ids = list(base_components[tail_id]["upstream"])

    edited = planner.plan(
        title="Research Draft",
        scenario="Insert a human review step before continuing.",
        existing_dsl=base,
    )

    assert edited["archetype"] == "modify_existing"
    op = next(op for op in edited["operations"] if op["type"] == "insert_human_review")
    target_id = op["target"]
    components = edited["dsl"]["components"]
    graph = edited["dsl"]["graph"]

    assert base == base_snapshot
    assert target_id not in base_components
    assert len(components) == len(base_components) + 1
    assert len(graph["nodes"]) == len(base_nodes) + 1
    assert len(graph["edges"]) == len(base_edges) + 1
    assert components[target_id]["obj"]["component_name"] == "UserFillUp"
    assert components[target_id]["upstream"] == predecessor_ids
    assert components[target_id]["downstream"] == [tail_id]
    assert components[tail_id]["upstream"] == [target_id]
    for predecessor_id in predecessor_ids:
        assert target_id in components[predecessor_id]["downstream"]
        assert tail_id not in components[predecessor_id]["downstream"]
        assert (predecessor_id, target_id) in _edge_pairs(edited["dsl"])
        assert (predecessor_id, tail_id) not in _edge_pairs(edited["dsl"])
    assert (target_id, tail_id) in _edge_pairs(edited["dsl"])
    assert target_id in {node["id"] for node in graph["nodes"]}


def test_plan_reports_noop_for_unsupported_edit():
    planner = ScenarioPlanner()
    base = planner.plan(
        title="Base Draft",
        scenario="Answer questions about an internal handbook.",
    )["dsl"]

    edited = planner.plan(
        title="Edited Draft",
        scenario="Change the whole graph into a fully autonomous planner.",
        existing_dsl=base,
    )

    assert edited["mode"] == "modify"
    assert edited["operations"] == [{"type": "no_op", "target": ""}]
    assert edited["warnings"]


def test_plan_can_modify_existing_dsl_with_analysis():
    planner = ScenarioPlanner()
    base = planner.plan(
        title="Base Draft",
        scenario="Answer questions about an internal handbook.",
    )["dsl"]
    base_snapshot = deepcopy(base)
    base_components = base["components"]
    base_nodes = base["graph"]["nodes"]
    base_edges = base["graph"]["edges"]

    edited = planner.plan(
        title="Edited Draft",
        scenario="Add analysis after the current flow.",
        existing_dsl=base,
    )

    assert edited["mode"] == "modify"
    op = next(op for op in edited["operations"] if op["type"] == "append_analysis")
    target_id = op["target"]
    components = edited["dsl"]["components"]
    graph = edited["dsl"]["graph"]
    prompts = components[target_id]["obj"]["params"]["prompts"]

    assert base == base_snapshot
    assert target_id not in base_components
    assert len(components) == len(base_components) + 1
    assert len(graph["nodes"]) == len(base_nodes) + 1
    assert len(graph["edges"]) == len(base_edges) + 1
    assert components[target_id]["upstream"] == ["Message:Output"]
    assert target_id in components["Message:Output"]["downstream"]
    assert ("Message:Output", target_id) in _edge_pairs(edited["dsl"])
    assert target_id in {node["id"] for node in graph["nodes"]}
    assert "Original request: {sys.query}" in prompts[0]["content"]
    assert "{Agent:DraftAnswer@content}" in prompts[0]["content"]
    assert "{Message:Output@content}" not in prompts[0]["content"]


def test_output_reference_falls_back_to_upstream_or_query():
    planner = ScenarioPlanner()
    components = {
        "Agent:WithOutput": planner._agent_component(
            prompts=[{"role": "user", "content": "{sys.query}"}],
            sys_prompt="Draft an answer.",
            downstream=["Switch:Decision"],
        ),
        "Switch:Decision": {
            "downstream": [],
            "obj": {
                "component_name": "Switch",
                "params": {},
            },
            "upstream": ["Agent:WithOutput"],
        },
        "Switch:NoUpstream": {
            "downstream": [],
            "obj": {
                "component_name": "Switch",
                "params": {},
            },
            "upstream": [],
        },
    }

    assert planner._get_output_reference(components, "Switch:Decision") == "{Agent:WithOutput@content}"
    assert planner._get_output_reference(components, "Switch:NoUpstream") == "{sys.query}"


def test_plan_analysis_uses_user_fillup_output_reference():
    planner = ScenarioPlanner()
    existing = {
        "components": {
            "begin": {
                "downstream": ["UserFillUp:Review"],
                "obj": {"component_name": "Begin", "params": {}},
                "upstream": [],
            },
            "UserFillUp:Review": planner._user_fillup_component(
                tips="Review the draft.",
                downstream=[],
                upstream=["begin"],
            ),
        },
        "graph": {
            "nodes": [
                {"id": "begin"},
                {"id": "UserFillUp:Review"},
            ],
            "edges": [
                {"source": "begin", "target": "UserFillUp:Review"},
            ],
        },
    }

    edited = planner.plan(
        title="Edited Draft",
        scenario="Add analysis after the current flow.",
        existing_dsl=existing,
    )

    op = next(op for op in edited["operations"] if op["type"] == "append_analysis")
    target_id = op["target"]
    prompt = edited["dsl"]["components"][target_id]["obj"]["params"]["prompts"][0]["content"]

    assert "{UserFillUp:Review@instructions}" in prompt
    assert "{UserFillUp:Review@content}" not in prompt


def test_plan_analysis_avoids_result_placeholder_for_outputless_tail():
    planner = ScenarioPlanner()
    existing = {
        "components": {
            "begin": {
                "downstream": ["Switch:Decision"],
                "obj": {"component_name": "Begin", "params": {}},
                "upstream": [],
            },
            "Switch:Decision": {
                "downstream": [],
                "obj": {"component_name": "Switch", "params": {}},
                "upstream": ["begin"],
            },
        },
        "graph": {
            "nodes": [
                {"id": "begin"},
                {"id": "Switch:Decision"},
            ],
            "edges": [
                planner._edge("begin", "Switch:Decision"),
            ],
        },
    }

    edited = planner.plan(
        title="Edited Draft",
        scenario="Add analysis after the current flow.",
        existing_dsl=existing,
    )

    op = next(op for op in edited["operations"] if op["type"] == "append_analysis")
    target_id = op["target"]
    prompt = edited["dsl"]["components"][target_id]["obj"]["params"]["prompts"][0]["content"]

    assert "@result" not in prompt
    assert "Analyze the following output:\n{sys.query}" in prompt


def test_plan_raises_clear_error_for_missing_builder(monkeypatch):
    planner = ScenarioPlanner()

    monkeypatch.setattr(
        planner,
        "_classify",
        lambda scenario: type(
            "Match",
            (),
            {"archetype": "missing_builder", "reason": "test", "warnings": []},
        )(),
    )

    with pytest.raises(ValueError, match="No builder implemented for archetype: missing_builder"):
        planner.plan(title="Broken", scenario="test")


@pytest.mark.parametrize(
    ("invalid_dsl", "message"),
    [
        ({}, "dict components and graph sections"),
        ({"components": {}, "graph": {"edges": [], "nodes": []}}, "begin node"),
        ({"components": {"begin": {}}, "graph": {"edges": [], "nodes": [{"id": "Agent:DraftAnswer"}]}}, "begin node"),
        ({"components": {"begin": {}}, "graph": {"edges": {}, "nodes": []}}, "graph.edges and graph.nodes as lists"),
        ({"components": {"begin": {}}, "graph": {"edges": [], "nodes": {}}}, "graph.edges and graph.nodes as lists"),
    ],
)
def test_plan_rejects_invalid_existing_dsl_structure(invalid_dsl, message):
    planner = ScenarioPlanner()

    with pytest.raises(ValueError, match=message):
        planner.plan(
            title="Broken",
            scenario="Add a notification step",
            existing_dsl=invalid_dsl,
        )
