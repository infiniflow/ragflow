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
from agent.scenario_planner import ScenarioPlanner


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

    edited = planner.plan(
        title="Edited Draft",
        scenario="Add a notification step after the current flow.",
        existing_dsl=base,
    )

    assert edited["archetype"] == "modify_existing"
    assert edited["mode"] == "modify"
    assert any(op["type"] == "append_notification" for op in edited["operations"])
    components = edited["dsl"]["components"]
    assert any(component["obj"]["component_name"] == "Message" for component in components.values())
    assert any("Notify" in component_id for component_id in components.keys())


def test_plan_can_modify_existing_dsl_with_human_review():
    planner = ScenarioPlanner()
    base = planner.plan(
        title="Research Draft",
        scenario="Research the market, compare sources, and produce a short report.",
    )["dsl"]

    edited = planner.plan(
        title="Research Draft",
        scenario="Insert a human review step before continuing.",
        existing_dsl=base,
    )

    assert edited["archetype"] == "modify_existing"
    assert any(op["type"] == "insert_human_review" for op in edited["operations"])
    components = edited["dsl"]["components"]
    assert any(component["obj"]["component_name"] == "UserFillUp" for component in components.values())


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
