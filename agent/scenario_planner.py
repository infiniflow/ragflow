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
"""Scenario planner for drafting RAGFlow canvas DSL from natural language."""

from __future__ import annotations

from copy import deepcopy
from dataclasses import dataclass
from typing import Any, Dict, List, Optional


DEFAULT_LLM_ID = "qwen-turbo@Tongyi-Qianwen"


@dataclass(frozen=True)
class ScenarioMatch:
    archetype: str
    reason: str
    warnings: List[str]


class ScenarioPlanner:
    """Translate natural-language scenarios into editable canvas skeletons."""

    ARCHETYPE_DESCRIPTIONS = {
        "qa_basic": "Simple question answering or retrieval assistant.",
        "interactive_research": "Two-stage research workflow with planning and human feedback.",
        "monitor_notify": "Fetch/analyze/branch/notify skeleton for monitoring tasks.",
        "batch_review": "Iteration-based batch processing workflow for repeated items.",
    }

    def plan(
        self,
        title: str,
        scenario: str,
        canvas_category: str = "Agent",
        existing_dsl: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        if existing_dsl:
            dsl = self._modify_existing_dsl(existing_dsl, scenario)
            return {
                "title": title,
                "canvas_type": canvas_category,
                "archetype": "modify_existing",
                "summary": "Modify an existing workflow draft based on natural-language instructions.",
                "reason": "Detected an existing DSL payload; applying incremental graph edits.",
                "warnings": [
                    "Only a minimal edit set is supported in V1.",
                    "Users should review node wiring before execution.",
                ],
                "dsl": dsl,
            }

        match = self._classify(scenario)
        builder = getattr(self, f"_build_{match.archetype}")
        dsl = builder(scenario)
        return {
            "title": title,
            "canvas_type": canvas_category,
            "archetype": match.archetype,
            "summary": self.ARCHETYPE_DESCRIPTIONS[match.archetype],
            "reason": match.reason,
            "warnings": match.warnings,
            "dsl": dsl,
        }

    def _modify_existing_dsl(self, existing_dsl: Dict[str, Any], instruction: str) -> Dict[str, Any]:
        dsl = deepcopy(existing_dsl)
        components = dsl.setdefault("components", {})
        graph = dsl.setdefault("graph", {})
        edges = graph.setdefault("edges", [])
        nodes = graph.setdefault("nodes", [])
        instruction_l = (instruction or "").strip().lower()

        last_message_id = None
        last_agent_id = None
        for component_id, component in components.items():
            name = component.get("obj", {}).get("component_name")
            if name == "Message":
                last_message_id = component_id
            elif name == "Agent":
                last_agent_id = component_id

        base_id = (last_message_id or last_agent_id or "begin").replace(":", "_")

        if any(token in instruction_l for token in ("notify", "notification", "alert")):
            new_message_id = f"Message:{base_id}Notify"
            if new_message_id not in components:
                components[new_message_id] = self._message_component(
                    ["A notification step should be configured here."],
                    [last_agent_id or "begin"],
                )
                if last_agent_id and last_agent_id in components:
                    downstream = components[last_agent_id].setdefault("downstream", [])
                    if new_message_id not in downstream:
                        downstream.append(new_message_id)
                edges.append(self._edge(last_agent_id or "begin", new_message_id))
                nodes.append(self._node(new_message_id, "Message", len(nodes)))

        if any(token in instruction_l for token in ("review", "approve", "human", "feedback")):
            fillup_id = f"UserFillUp:{base_id}Review"
            if fillup_id not in components:
                components[fillup_id] = self._user_fillup_component(
                    tips="Review the current output and provide feedback before continuing.",
                    downstream=[],
                    upstream=[last_agent_id or "begin"],
                )
                if last_agent_id and last_agent_id in components:
                    downstream = components[last_agent_id].setdefault("downstream", [])
                    if fillup_id not in downstream:
                        downstream.append(fillup_id)
                edges.append(self._edge(last_agent_id or "begin", fillup_id))
                nodes.append(self._node(fillup_id, "UserFillUp", len(nodes)))

        if any(token in instruction_l for token in ("add analysis", "analyze", "summarize", "summary")):
            new_agent_id = f"Agent:{base_id}Analysis"
            if new_agent_id not in components:
                upstream_id = last_message_id or last_agent_id or "begin"
                components[new_agent_id] = self._agent_component(
                    prompts=[{"role": "user", "content": "{sys.query}"}],
                    sys_prompt="This is an appended analysis step. Users should refine the prompt and attach tools if needed.",
                    downstream=[],
                )
                components[new_agent_id]["upstream"] = [upstream_id]
                if upstream_id in components:
                    upstream_component = components[upstream_id]
                    upstream_downstream = upstream_component.setdefault("downstream", [])
                    if new_agent_id not in upstream_downstream:
                        upstream_downstream.append(new_agent_id)
                edges.append(self._edge(upstream_id, new_agent_id))
                nodes.append(self._node(new_agent_id, "Agent", len(nodes)))

        return dsl

    def _node(self, component_id: str, label: str, index: int) -> Dict[str, Any]:
        return {
            "id": component_id,
            "data": {
                "label": label,
                "name": component_id,
            },
            "position": {
                "x": 120 + index * 260,
                "y": 160,
            },
        }

    def _classify(self, scenario: str) -> ScenarioMatch:
        text = (scenario or "").strip().lower()
        if not text:
            return ScenarioMatch(
                archetype="qa_basic",
                reason="Empty scenario, defaulting to the smallest editable QA flow.",
                warnings=["Scenario text is empty; the generated draft is only a placeholder."],
            )

        if any(token in text for token in ("monitor", "watch", "detect changes", "notify", "notification", "alert")):
            return ScenarioMatch(
                archetype="monitor_notify",
                reason="Detected monitoring/notification intent.",
                warnings=["The monitor skeleton is a draft. Users still need to wire the real fetch/extract tools."],
            )

        if any(token in text for token in ("batch", "each file", "for each", "every file", "multiple resumes", "all resumes")):
            return ScenarioMatch(
                archetype="batch_review",
                reason="Detected repeated-item or batch-processing intent.",
                warnings=[],
            )

        if any(token in text for token in ("research", "investigate", "analyze", "report", "compare", "deep search")):
            return ScenarioMatch(
                archetype="interactive_research",
                reason="Detected multi-step research intent.",
                warnings=[],
            )

        return ScenarioMatch(
            archetype="qa_basic",
            reason="No complex orchestration keyword detected; using a minimal QA skeleton.",
            warnings=[],
        )

    def _base_dsl(self, components: Dict[str, Dict[str, Any]], edges: List[Dict[str, Any]]) -> Dict[str, Any]:
        nodes = []
        for index, (component_id, component) in enumerate(components.items()):
            nodes.append(self._node(component_id, component["obj"]["component_name"], index))

        return {
            "components": components,
            "globals": {
                "sys.conversation_turns": 0,
                "sys.files": [],
                "sys.query": "",
                "sys.user_id": "",
            },
            "graph": {
                "edges": edges,
                "nodes": nodes,
            },
            "history": [],
            "messages": [],
            "path": ["begin"],
            "retrieval": {"chunks": [], "doc_aggs": []},
            "variables": {},
        }

    def _edge(self, source: str, target: str, source_handle: str = "start", target_handle: str = "end") -> Dict[str, Any]:
        return {
            "id": f"xy-edge__{source}{source_handle}-{target}{target_handle}",
            "source": source,
            "sourceHandle": source_handle,
            "target": target,
            "targetHandle": target_handle,
            "data": {"isHovered": False},
        }

    def _begin_component(self, downstream: List[str], prologue: str) -> Dict[str, Any]:
        return {
            "downstream": downstream,
            "obj": {
                "component_name": "Begin",
                "params": {
                    "enablePrologue": True,
                    "inputs": {},
                    "mode": "conversational",
                    "prologue": prologue,
                },
            },
            "upstream": [],
        }

    def _agent_component(
        self,
        prompts: List[Dict[str, str]],
        sys_prompt: str,
        downstream: List[str],
        tools: List[Dict[str, Any]] | None = None,
        mcp: List[Dict[str, Any]] | None = None,
        max_rounds: int = 1,
    ) -> Dict[str, Any]:
        return {
            "downstream": downstream,
            "obj": {
                "component_name": "Agent",
                "params": {
                    "cite": True,
                    "delay_after_error": 1,
                    "description": "",
                    "exception_default_value": "",
                    "exception_goto": [],
                    "exception_method": "",
                    "frequencyPenaltyEnabled": False,
                    "frequency_penalty": 0.7,
                    "llm_id": DEFAULT_LLM_ID,
                    "maxTokensEnabled": False,
                    "max_retries": 3,
                    "max_rounds": max_rounds,
                    "max_tokens": 512,
                    "mcp": mcp or [],
                    "message_history_window_size": 12,
                    "outputs": {
                        "content": {"type": "string", "value": ""},
                        "structured": {},
                    },
                    "presencePenaltyEnabled": False,
                    "presence_penalty": 0.4,
                    "prompts": prompts,
                    "sys_prompt": sys_prompt,
                    "temperature": 0.1,
                    "temperatureEnabled": False,
                    "tools": tools or [],
                    "topPEnabled": False,
                    "top_p": 0.3,
                    "user_prompt": "",
                    "visual_files_var": "",
                },
            },
            "upstream": [],
        }

    def _message_component(self, content: List[str], upstream: List[str]) -> Dict[str, Any]:
        return {
            "downstream": [],
            "obj": {
                "component_name": "Message",
                "params": {"content": content},
            },
            "upstream": upstream,
        }

    def _user_fillup_component(self, tips: str, downstream: List[str], upstream: List[str]) -> Dict[str, Any]:
        io_schema = {
            "instructions": {
                "name": "instructions",
                "optional": False,
                "options": [],
                "type": "paragraph",
            }
        }
        return {
            "downstream": downstream,
            "obj": {
                "component_name": "UserFillUp",
                "params": {
                    "enable_tips": True,
                    "inputs": deepcopy(io_schema),
                    "outputs": deepcopy(io_schema),
                    "tips": tips,
                },
            },
            "upstream": upstream,
        }

    def _switch_component(self, conditions: List[Dict[str, Any]], downstream: List[str], upstream: List[str], end_cpn_ids: List[str]) -> Dict[str, Any]:
        return {
            "downstream": downstream,
            "obj": {
                "component_name": "Switch",
                "params": {
                    "conditions": conditions,
                    "end_cpn_ids": end_cpn_ids,
                },
            },
            "upstream": upstream,
        }

    def _iteration_component(self, items_ref: str, output_ref: str, upstream: List[str]) -> Dict[str, Any]:
        return {
            "downstream": [],
            "obj": {
                "component_name": "Iteration",
                "params": {
                    "items_ref": items_ref,
                    "outputs": {
                        "evaluation": {
                            "ref": output_ref,
                            "type": "Array<string>",
                        }
                    },
                },
            },
            "upstream": upstream,
        }

    def _iteration_item_component(self, parent_id: str, downstream: List[str]) -> Dict[str, Any]:
        return {
            "downstream": downstream,
            "obj": {
                "component_name": "IterationItem",
                "params": {
                    "outputs": {
                        "index": {"type": "integer"},
                        "item": {"type": "unknown"},
                    }
                },
            },
            "parent_id": parent_id,
            "upstream": [],
        }

    def _build_qa_basic(self, scenario: str) -> Dict[str, Any]:
        components = {
            "begin": self._begin_component(["Agent:DraftAnswer"], "Hi! Describe the task you want this workflow to handle."),
            "Agent:DraftAnswer": self._agent_component(
                prompts=[{"role": "user", "content": "{sys.query}"}],
                sys_prompt=(
                    "You are a draft QA agent skeleton. "
                    "Users should replace this prompt, attach tools, and refine citations or retrieval settings as needed."
                ),
                downstream=["Message:Output"],
            ),
            "Message:Output": self._message_component(["{Agent:DraftAnswer@content}"], ["Agent:DraftAnswer"]),
        }
        components["Agent:DraftAnswer"]["upstream"] = ["begin"]
        edges = [
            self._edge("begin", "Agent:DraftAnswer"),
            self._edge("Agent:DraftAnswer", "Message:Output"),
        ]
        return self._base_dsl(components, edges)

    def _build_interactive_research(self, scenario: str) -> Dict[str, Any]:
        components = {
            "begin": self._begin_component(["Agent:Plan"], "Hi! Describe the research task you want to automate."),
            "Agent:Plan": self._agent_component(
                prompts=[{"role": "user", "content": "User query:{sys.query}"}],
                sys_prompt=(
                    "You are the planning agent. Break the scenario into concrete research steps, "
                    "define what evidence is needed, and prepare a short execution plan."
                ),
                downstream=["UserFillUp:ReviewPlan"],
            ),
            "UserFillUp:ReviewPlan": self._user_fillup_component(
                tips="Here is the draft plan:\n{Agent:Plan@content}\nPlease refine or approve it before execution.",
                downstream=["Agent:ExecuteResearch"],
                upstream=["Agent:Plan"],
            ),
            "Agent:ExecuteResearch": self._agent_component(
                prompts=[{"role": "user", "content": "Plan:{Agent:Plan@content}\nUser feedback:{UserFillUp:ReviewPlan@instructions}\nQuery:{sys.query}"}],
                sys_prompt=(
                    "You are the execution agent. Follow the approved plan, collect evidence, "
                    "and draft a concise final answer with clear traceability."
                ),
                downstream=["Message:Output"],
                max_rounds=3,
            ),
            "Message:Output": self._message_component(["{Agent:ExecuteResearch@content}"], ["Agent:ExecuteResearch"]),
        }
        components["Agent:Plan"]["upstream"] = ["begin"]
        components["Agent:ExecuteResearch"]["upstream"] = ["UserFillUp:ReviewPlan"]
        edges = [
            self._edge("begin", "Agent:Plan"),
            self._edge("Agent:Plan", "UserFillUp:ReviewPlan"),
            self._edge("UserFillUp:ReviewPlan", "Agent:ExecuteResearch"),
            self._edge("Agent:ExecuteResearch", "Message:Output"),
        ]
        return self._base_dsl(components, edges)

    def _build_monitor_notify(self, scenario: str) -> Dict[str, Any]:
        components = {
            "begin": self._begin_component(["Agent:FetchState"], "Hi! Describe what should be monitored and how you want to be notified."),
            "Agent:FetchState": self._agent_component(
                prompts=[{"role": "user", "content": "Monitoring target:{sys.query}"}],
                sys_prompt=(
                    "You are the fetch step skeleton. Configure this node to collect the latest state "
                    "from the monitored source before comparison."
                ),
                downstream=["Agent:CompareState"],
            ),
            "Agent:CompareState": self._agent_component(
                prompts=[{"role": "user", "content": "Fetched state:{Agent:FetchState@content}\nOriginal task:{sys.query}"}],
                sys_prompt=(
                    "You are the comparison step skeleton. Decide whether there is a meaningful change. "
                    "Return CHANGED or NO_CHANGE plus a short rationale."
                ),
                downstream=["Switch:Decision"],
            ),
            "Switch:Decision": self._switch_component(
                conditions=[
                    {
                        "items": [
                            {
                                "cpn_id": "Agent:CompareState@content",
                                "operator": "contains",
                                "value": "CHANGED",
                            }
                        ],
                        "logical_operator": "and",
                        "to": ["Agent:Notify"],
                    }
                ],
                downstream=["Agent:Notify", "Message:NoChange"],
                upstream=["Agent:CompareState"],
                end_cpn_ids=["Message:NoChange"],
            ),
            "Agent:Notify": self._agent_component(
                prompts=[{"role": "user", "content": "Comparison result:{Agent:CompareState@content}\nTask:{sys.query}"}],
                sys_prompt=(
                    "You are the notification step skeleton. Prepare the alert payload, summary, "
                    "or follow-up action when a change is detected."
                ),
                downstream=["Message:Output"],
            ),
            "Message:NoChange": self._message_component(["No meaningful change was detected."], ["Switch:Decision"]),
            "Message:Output": self._message_component(["{Agent:Notify@content}"], ["Agent:Notify"]),
        }
        components["Agent:FetchState"]["upstream"] = ["begin"]
        components["Agent:CompareState"]["upstream"] = ["Agent:FetchState"]
        components["Agent:Notify"]["upstream"] = ["Switch:Decision"]
        edges = [
            self._edge("begin", "Agent:FetchState"),
            self._edge("Agent:FetchState", "Agent:CompareState"),
            self._edge("Agent:CompareState", "Switch:Decision"),
            self._edge("Switch:Decision", "Agent:Notify"),
            self._edge("Switch:Decision", "Message:NoChange"),
            self._edge("Agent:Notify", "Message:Output"),
        ]
        return self._base_dsl(components, edges)

    def _build_batch_review(self, scenario: str) -> Dict[str, Any]:
        iteration_id = "Iteration:Items"
        components = {
            "begin": {
                "downstream": [iteration_id],
                "obj": {
                    "component_name": "Begin",
                    "params": {
                        "enablePrologue": True,
                        "inputs": {
                            "task_instructions": {
                                "name": "Task Instructions",
                                "optional": False,
                                "options": [],
                                "type": "line",
                            }
                        },
                        "mode": "conversational",
                        "prologue": "Hi! Upload or provide the items you want to process in batch.",
                    },
                },
                "upstream": [],
            },
            iteration_id: self._iteration_component("sys.files", "Agent:ReviewItem@content", ["begin"]),
            "IterationItem:Current": self._iteration_item_component(iteration_id, ["Agent:ReviewItem"]),
            "Agent:ReviewItem": self._agent_component(
                prompts=[{"role": "user", "content": "Task:{begin@task_instructions}\nCurrent item:{IterationItem:Current@item}\nOriginal scenario:{sys.query}"}],
                sys_prompt=(
                    "You are the per-item batch processor. Review each item independently and "
                    "produce a concise structured result."
                ),
                downstream=["Message:ItemOutput"],
            ),
            "Message:ItemOutput": self._message_component(["{Agent:ReviewItem@content}"], ["Agent:ReviewItem"]),
        }
        components["Agent:ReviewItem"]["parent_id"] = iteration_id
        components["Agent:ReviewItem"]["upstream"] = ["IterationItem:Current"]
        components["Message:ItemOutput"]["parent_id"] = iteration_id

        edges = [
            self._edge("begin", iteration_id),
            self._edge("IterationItem:Current", "Agent:ReviewItem"),
            self._edge("Agent:ReviewItem", "Message:ItemOutput"),
        ]
        dsl = self._base_dsl(components, edges)
        return dsl
