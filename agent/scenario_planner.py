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
"""Scenario planner for drafting and editing RAGFlow canvas DSL from natural language."""

from __future__ import annotations

from copy import deepcopy
from dataclasses import dataclass
from functools import lru_cache
import logging
import json
from pathlib import Path
from typing import Any, Dict, List, Optional


DEFAULT_LLM_ID = "qwen-turbo@Tongyi-Qianwen"
logger = logging.getLogger(__name__)
TEMPLATE_ROOT = Path(__file__).resolve().parent / "templates" / "scenario_planner"


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
        mode = "modify" if existing_dsl is not None else "create"
        logger.info("scenario_planner plan start mode=%s", mode)
        if existing_dsl is not None:
            dsl, operations, warnings = self._modify_existing_dsl(existing_dsl, scenario)
            result = {
                "title": title,
                "canvas_type": canvas_category,
                "mode": mode,
                "archetype": "modify_existing",
                "summary": "Modify an existing workflow draft based on natural-language instructions.",
                "reason": "Detected an existing DSL payload; applying incremental graph edits.",
                "warnings": [
                    "Only a minimal edit set is supported in V1.",
                    "Users should review node wiring before execution.",
                    *warnings,
                ],
                "operations": operations,
                "dsl": dsl,
            }
            logger.info(
                "scenario_planner plan complete mode=%s archetype=%s operations=%s warnings=%s",
                mode,
                result["archetype"],
                len(result.get("operations", []) or []),
                len(result.get("warnings", []) or []),
            )
            return result

        match = self._classify(scenario)
        builder = getattr(self, f"_build_{match.archetype}", None)
        if builder is None:
            logger.error("scenario_planner missing builder for archetype=%s", match.archetype)
            raise ValueError(f"No builder implemented for archetype: {match.archetype}")
        dsl = builder(scenario)
        result = {
            "title": title,
            "canvas_type": canvas_category,
            "mode": mode,
            "archetype": match.archetype,
            "summary": self.ARCHETYPE_DESCRIPTIONS[match.archetype],
            "reason": match.reason,
            "warnings": match.warnings,
            "operations": [{"type": "create_draft", "archetype": match.archetype}],
            "dsl": dsl,
        }
        logger.info(
            "scenario_planner plan complete mode=%s archetype=%s operations=%s warnings=%s",
            mode,
            result["archetype"],
            len(result.get("operations", []) or []),
            len(result.get("warnings", []) or []),
        )
        return result

    def _modify_existing_dsl(self, existing_dsl: Dict[str, Any], instruction: str) -> tuple[Dict[str, Any], List[Dict[str, str]], List[str]]:
        dsl = deepcopy(existing_dsl)
        components = dsl.get("components")
        graph = dsl.get("graph")
        if not isinstance(components, dict) or not isinstance(graph, dict):
            raise ValueError("existing_dsl must be a valid canvas DSL with dict components and graph sections")
        edges = graph.get("edges")
        nodes = graph.get("nodes")
        if not isinstance(edges, list) or not isinstance(nodes, list):
            raise ValueError("existing_dsl must be a valid canvas DSL with graph.edges and graph.nodes as lists")
        if not all(isinstance(component, dict) for component in components.values()):
            raise ValueError("existing_dsl must be a valid canvas DSL with dict component entries")
        if not all(isinstance(node, dict) for node in nodes):
            raise ValueError("existing_dsl must be a valid canvas DSL with dict graph.nodes entries")
        if not all(isinstance(edge, dict) for edge in edges):
            raise ValueError("existing_dsl must be a valid canvas DSL with dict graph.edges entries")
        if "begin" not in components or not any(node.get("id") == "begin" for node in nodes if isinstance(node, dict)):
            raise ValueError("existing_dsl must be a valid canvas DSL with a begin node in both components and graph.nodes")
        for component in components.values():
            downstream = component.get("downstream", [])
            upstream = component.get("upstream", [])
            if not isinstance(downstream, list) or not isinstance(upstream, list):
                raise ValueError("existing_dsl must be a valid canvas DSL with list upstream/downstream component links")
        instruction_l = (instruction or "").strip().lower()
        operations: List[Dict[str, str]] = []
        warnings: List[str] = []
        recognized_any = False
        applied_any = False

        if any(token in instruction_l for token in ("notify", "notification", "alert")):
            recognized_any = True
            tail_id = self._get_tail_component_id(components)
            base_id = (tail_id or "begin").replace(":", "_")
            new_message_id = f"Message:{base_id}Notify"
            if new_message_id not in components:
                applied_any = True
                components[new_message_id] = self._message_component(
                    ["A notification step should be configured here."],
                    [tail_id or "begin"],
                )
                if tail_id and tail_id in components:
                    downstream = components[tail_id].setdefault("downstream", [])
                    if new_message_id not in downstream:
                        downstream.append(new_message_id)
                edges.append(self._edge(tail_id or "begin", new_message_id))
                nodes.append(self._node(new_message_id, "Message", len(nodes)))
                operations.append({"type": "append_notification", "target": new_message_id})
            else:
                operations.append({"type": "already_present", "target": new_message_id})

        if any(token in instruction_l for token in ("review", "approve", "human", "feedback")):
            recognized_any = True
            tail_id = self._get_tail_component_id(components)
            predecessor_ids = self._get_predecessor_ids(components, tail_id)
            base_id = (tail_id or "begin").replace(":", "_")
            fillup_id = f"UserFillUp:{base_id}Review"
            if fillup_id not in components:
                applied_any = True
                review_downstream = [tail_id] if tail_id and tail_id != "begin" else []
                review_upstream = predecessor_ids or ([tail_id] if tail_id and tail_id != "begin" else ["begin"])
                components[fillup_id] = self._user_fillup_component(
                    tips="Review the current output and provide feedback before continuing.",
                    downstream=review_downstream,
                    upstream=review_upstream,
                )
                if tail_id and tail_id != "begin" and predecessor_ids:
                    for predecessor_id in predecessor_ids:
                        downstream = components[predecessor_id].setdefault("downstream", [])
                        downstream[:] = [fillup_id if item == tail_id else item for item in downstream]
                        self._replace_edge_target(edges, predecessor_id, tail_id, fillup_id)
                    components[tail_id]["upstream"] = [fillup_id]
                    edges.append(self._edge(fillup_id, tail_id))
                else:
                    anchor = "begin"
                    if anchor in components:
                        downstream = components[anchor].setdefault("downstream", [])
                        if fillup_id not in downstream:
                            downstream.append(fillup_id)
                    edges.append(self._edge(anchor, fillup_id))
                nodes.append(self._node(fillup_id, "UserFillUp", len(nodes)))
                operations.append({"type": "insert_human_review", "target": fillup_id})
            else:
                operations.append({"type": "already_present", "target": fillup_id})

        if any(token in instruction_l for token in ("add analysis", "analyze", "summarize", "summary")):
            recognized_any = True
            tail_id = self._get_tail_component_id(components)
            base_id = (tail_id or "begin").replace(":", "_")
            new_agent_id = f"Agent:{base_id}Analysis"
            if new_agent_id not in components:
                applied_any = True
                upstream_id = tail_id or "begin"
                tail_output_ref = self._get_output_reference(components, upstream_id)
                components[new_agent_id] = self._agent_component(
                    prompts=[{"role": "user", "content": f"Analyze the following output:\n{tail_output_ref}\n\nOriginal request: {{sys.query}}"}],
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
                operations.append({"type": "append_analysis", "target": new_agent_id})
            else:
                operations.append({"type": "already_present", "target": new_agent_id})

        if recognized_any and not applied_any:
            warnings.append("Requested edit matched a supported operation, but no graph changes were applied.")
            operations.append({"type": "no_op", "target": ""})
        elif not recognized_any:
            warnings.append(
                "No supported edit operation was detected. V1 currently supports notification, review, and analysis-oriented edits."
            )
            operations.append({"type": "no_op", "target": ""})

        return dsl, operations, warnings

    def _get_output_reference(
        self,
        components: Dict[str, Dict[str, Any]],
        component_id: str,
        visited: Optional[set[str]] = None,
    ) -> str:
        if visited is None:
            visited = set()
        if component_id == "begin":
            return "{sys.query}"
        if component_id in visited:
            return "{sys.query}"
        visited.add(component_id)
        component = components.get(component_id, {})
        obj = component.get("obj", {})
        component_name = obj.get("component_name")
        if component_name == "Message":
            upstream = component.get("upstream", []) or []
            if upstream:
                return self._get_output_reference(components, upstream[0], visited)
        params = obj.get("params", {})
        outputs = params.get("outputs")
        if isinstance(outputs, dict) and outputs:
            preferred_field = "content" if "content" in outputs else next(iter(outputs))
            return f"{{{component_id}@{preferred_field}}}"
        upstream = component.get("upstream", []) or []
        if upstream:
            return self._get_output_reference(components, upstream[0], visited)
        return "{sys.query}"

    def _get_tail_component_id(self, components: Dict[str, Dict[str, Any]]) -> Optional[str]:
        """Return a deterministic tail node, preferring the main Message:Output when multiple tails exist."""
        tails = [component_id for component_id, component in components.items() if not component.get("downstream")]
        if not tails:
            return None
        for tail in tails:
            component_name = components.get(tail, {}).get("obj", {}).get("component_name")
            if component_name == "Message" and (tail == "Message:Output" or tail.startswith("Message:Output")):
                return tail
        return tails[-1]

    def _get_predecessor_ids(self, components: Dict[str, Dict[str, Any]], target_id: Optional[str]) -> List[str]:
        if not target_id:
            return []
        predecessors = []
        for component_id, component in components.items():
            downstream = component.get("downstream", []) or []
            if target_id in downstream:
                predecessors.append(component_id)
        return predecessors

    def _replace_edge_target(self, edges: List[Dict[str, Any]], source_id: str, old_target: str, new_target: str) -> None:
        for edge in edges:
            if edge.get("source") == source_id and edge.get("target") == old_target:
                edge["target"] = new_target
                edge["id"] = f"xy-edge__{source_id}{edge.get('sourceHandle', 'start')}-{new_target}{edge.get('targetHandle', 'end')}"

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

    @staticmethod
    @lru_cache(maxsize=8)
    def _load_template_payload(template_name: str) -> Dict[str, Any]:
        template_path = TEMPLATE_ROOT / f"{template_name}.json"
        if not template_path.exists():
            raise ValueError(f"Scenario planner template not found: {template_name}")
        payload = json.loads(template_path.read_text(encoding="utf-8"))
        return payload.get("dsl", payload)

    def _load_template(self, template_name: str) -> Dict[str, Any]:
        return deepcopy(self._load_template_payload(template_name))

    def _edge(self, source: str, target: str, source_handle: str = "start", target_handle: str = "end") -> Dict[str, Any]:
        return {
            "id": f"xy-edge__{source}{source_handle}-{target}{target_handle}",
            "source": source,
            "sourceHandle": source_handle,
            "target": target,
            "targetHandle": target_handle,
            "data": {"isHovered": False},
        }

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

    def _agent_component(
        self,
        prompts: List[Dict[str, str]],
        sys_prompt: str,
        downstream: List[str],
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
                    "mcp": [],
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
                    "tools": [],
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

    def _build_qa_basic(self, scenario: str) -> Dict[str, Any]:
        return self._load_template("qa_basic")

    def _build_interactive_research(self, scenario: str) -> Dict[str, Any]:
        return self._load_template("interactive_research")

    def _build_monitor_notify(self, scenario: str) -> Dict[str, Any]:
        return self._load_template("monitor_notify")

    def _build_batch_review(self, scenario: str) -> Dict[str, Any]:
        return self._load_template("batch_review")
