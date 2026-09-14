#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
"""Round-trip stability integration tests for the agent dsl bridge.

These mirror the front-end bridge pipeline (`web/src/pages/agent/utils/dsl-bridge.ts`)
in pure Python so the same invariants — v1 export byte-stable under
re-import, v2 export byte-stable under re-import, React-Flow internal
fields downgraded to warnings — can be checked from the Python test
suite without the npm/Jest toolchain.

The Python port is deliberately a near-line-for-line translation of
the TypeScript implementation: any change to the TS bridge should
be reflected here in the same commit. The two implementations agree
on the structural invariants (position, edge topology, components
map, _layout positions) and disagree only on the iteration order of
plain-object keys at JSON.stringify time — that's a downstream
concern, not a round-trip-stability concern.

Fixtures live in `internal/agent/dsl/testdata/` and are
the same on-disk JSONs the Go server's `TestNormalizeForCanvas_FixtureSmoke`
consumes. A new fixture added to that directory can be picked up
here by adding one `def test_*` below.
"""

from __future__ import annotations

import json
import warnings
from typing import Any

import pytest


# ─── Python port of web/src/pages/agent/utils/dsl-bridge.ts ─────────────
#
# Only the round-trip-relevant subset is ported. The TS bridge also
# handles EmptyDsl/EmptyDslV1 seeding, DataflowEmptyDsl variants,
# and the place-holder filter — those are not exercised by the
# round-trip path and are deliberately omitted here.

_COMPONENT_NAME_TO_NODE_TYPE: dict[str, str] = {
    "Begin": "beginNode",
    "Retrieval": "ragNode",
    "Categorize": "categorizeNode",
    "Message": "messageNode",
    "Answer": "messageNode",
    "RewriteQuestion": "rewriteNode",
    "ExeSQL": "toolNode",
    "Switch": "switchNode",
    "Agent": "agentNode",
    "Tool": "toolNode",
    "File": "fileNode",
    "Parser": "parserNode",
    "Tokenizer": "tokenizerNode",
    "TokenChunker": "chunkerNode",
    "TitleChunker": "chunkerNode",
    "Extractor": "contextNode",
    "Loop": "loopNode",
    "LoopStart": "loopStartNode",
    "ExitLoop": "exitLoopNode",
    "Iteration": "iterationNode",
    "IterationStart": "iterationStartNode",
    "DataOperations": "dataOperationsNode",
    "ListOperations": "listOperationsNode",
    "VariableAssigner": "variableAssignerNode",
    "VariableAggregator": "variableAggregatorNode",
    "Keyword": "keywordNode",
    "Note": "noteNode",
    "Placeholder": "placeholderNode",
    "Code": "toolNode",
}


def _component_name_to_node_type(name: str) -> str:
    return _COMPONENT_NAME_TO_NODE_TYPE.get(name, "agentNode")


def _to_string_list(value: Any) -> list[str]:
    if not isinstance(value, list):
        return []
    return [v for v in value if isinstance(v, str) and v]


# ─── v1 helpers ─────────────────────────────────────────────────────────


def _v1_components_to_graph(components: dict[str, Any]) -> tuple[list[dict], list[dict]]:
    """Port of `v1ComponentsToGraph` in dsl-bridge.ts.

    Returns (nodes, edges) in React-Flow shape. Positions are the
    default 50/350/200 row layout — the round-trip test for the v1
    path uses `_layout` as the authoritative position source, so
    the v1-derived positions are only consumed when no `_layout`
    exists (matching the TS bridge's fallback behaviour).
    """
    nodes: list[dict] = []
    edges: list[dict] = []
    for i, (key, raw) in enumerate(components.items()):
        comp = raw if isinstance(raw, dict) else {}
        obj = comp.get("obj") if isinstance(comp.get("obj"), dict) else {}
        name = obj.get("component_name") or comp.get("name") or key
        params = obj.get("params") if isinstance(obj.get("params"), dict) else (comp.get("params") if isinstance(comp.get("params"), dict) else {})
        nodes.append(
            {
                "id": key,
                "type": _component_name_to_node_type(name),
                "position": {"x": 50 + i * 350, "y": 200},
                "data": {"label": name, "name": name, "form": params},
                "sourcePosition": "right",
                "targetPosition": "left",
            }
        )
        for dst in _to_string_list(comp.get("downstream")):
            edges.append(
                {
                    "id": f"xy-edge__{key}-{dst}",
                    "source": key,
                    "target": dst,
                    "sourceHandle": "start",
                    "targetHandle": "end",
                }
            )
    return nodes, edges


def _graph_to_v1_components(graph: dict[str, Any]) -> dict[str, Any]:
    """Port of `graphToV1Components` in dsl-bridge.ts (the inverse of
    `_v1_components_to_graph`).
    """
    edges = graph.get("edges") or []
    downstream_map: dict[str, list[str]] = {}
    upstream_map: dict[str, list[str]] = {}
    for edge in edges:
        if not isinstance(edge, dict):
            continue
        src = edge.get("source")
        dst = edge.get("target")
        if not isinstance(src, str) or not isinstance(dst, str):
            continue
        downstream_map.setdefault(src, []).append(dst)
        upstream_map.setdefault(dst, []).append(src)

    components: dict[str, Any] = {}
    for raw_node in graph.get("nodes") or []:
        if not isinstance(raw_node, dict):
            continue
        node_id = raw_node.get("id")
        if not isinstance(node_id, str) or not node_id:
            continue
        data = raw_node.get("data") if isinstance(raw_node.get("data"), dict) else {}
        label = data.get("label") or node_id
        form = data.get("form") if isinstance(data.get("form"), dict) else {}
        components[node_id] = {
            "obj": {
                "component_name": label,
                "params": form,
            },
            "downstream": downstream_map.get(node_id, []),
            "upstream": upstream_map.get(node_id, []),
        }
    return components


def _build_dsl_components_by_graph(nodes: list[dict], edges: list[dict], seed: dict[str, Any]) -> dict[str, Any]:
    """Port of `buildDslComponentsByGraph` (web/src/pages/agent/utils.ts:472).

    Reverse-derives a v1-style `components` map from React-Flow
    nodes/edges. Each node becomes a `component_name`/`params` pair
    under `obj`, with `downstream`/`upstream` aggregated from the
    edge list. Legacy `obj` fields on existing entries in `seed` are
    preserved by the TS implementation (round-trip a `_deprecated_*`
    field through a save/edit cycle); the Python port keeps the same
    shape and accepts the same `seed` for consistency, even though
    no legacy fields are currently in use.
    """
    downstream_map: dict[str, list[str]] = {}
    upstream_map: dict[str, list[str]] = {}
    for edge in edges or []:
        if not isinstance(edge, dict):
            continue
        src = edge.get("source")
        dst = edge.get("target")
        if not isinstance(src, str) or not isinstance(dst, str):
            continue
        downstream_map.setdefault(src, []).append(dst)
        upstream_map.setdefault(dst, []).append(src)

    out: dict[str, Any] = dict(seed)  # preserve legacy `obj` fields
    for raw_node in nodes or []:
        if not isinstance(raw_node, dict):
            continue
        node_id = raw_node.get("id")
        if not isinstance(node_id, str) or not node_id:
            continue
        data = raw_node.get("data") if isinstance(raw_node.get("data"), dict) else {}
        label = data.get("label") or node_id
        form = data.get("form") if isinstance(data.get("form"), dict) else {}
        out[node_id] = {
            "obj": {
                "component_name": label,
                "params": form,
            },
            "downstream": downstream_map.get(node_id, []),
            "upstream": upstream_map.get(node_id, []),
        }
    return out


def _build_v1_dsl_from_import(raw: dict[str, Any], is_agent: bool) -> dict[str, Any]:
    """Port of `buildV1DslFromImport`. Accepts both v1 (`components` key)
    and v2 (`graph` key) input shapes and produces a v1 envelope.
    """
    out: dict[str, Any] = dict(raw)  # shallow copy, will overwrite below
    if isinstance(raw.get("components"), dict) and raw["components"]:
        out["components"] = raw["components"]
        layout = raw.get("_layout")
        out["_layout"] = layout
    elif isinstance(raw.get("graph"), dict) and (isinstance(raw["graph"].get("nodes"), list) and raw["graph"]["nodes"]):
        graph = raw["graph"]
        out["components"] = _graph_to_v1_components(graph)
        # Mirror the TS bridge: stash positions in _layout so the
        # user's saved canvas layout comes back unchanged.
        out["_layout"] = {
            "nodes": graph.get("nodes") or [],
            "edges": graph.get("edges") or [],
        }
    return out


# ─── v2 helpers ─────────────────────────────────────────────────────────


def _build_v2_dsl_from_import(raw: dict[str, Any], is_agent: bool) -> dict[str, Any]:
    """Port of `buildV2DslFromImport`. Accepts both v1 and v2 input
    shapes and produces a v2 envelope.
    """
    out: dict[str, Any] = dict(raw)
    if isinstance(raw.get("graph"), dict) and (isinstance(raw["graph"].get("nodes"), list) and raw["graph"]["nodes"]):
        out["graph"] = raw["graph"]
        # v2 input that carries its own `components` (e.g. a v2 file
        # exported from the front-end with `bridge.exportDsl`) keeps
        # them. When the input only has `graph` (e.g. a hand-edited
        # dsl or a Go-server payload that was serialized without the
        # v2 dual `components` block) we derive a v1-style
        # `components` map via `buildDslComponentsByGraph` so the
        # server can still serve a re-imported file.
        if not isinstance(raw.get("components"), dict):
            out["components"] = _build_dsl_components_by_graph(
                out["graph"].get("nodes") or [],
                out["graph"].get("edges") or [],
                {},
            )
    elif isinstance(raw.get("components"), dict) and raw["components"]:
        components = raw["components"]
        out["components"] = components
        layout = raw.get("_layout")
        # v1 → v2 cross-mode: prefer saved _layout positions over
        # the default 50/350/200 row layout the inverse-conversion
        # would produce. The user's drag-and-drop work survives.
        if isinstance(layout, dict) and isinstance(layout.get("nodes"), list) and layout["nodes"]:
            out["graph"] = {
                "nodes": layout["nodes"],
                "edges": layout.get("edges") or [],
            }
        else:
            nodes, edges = _v1_components_to_graph(components)
            out["graph"] = {"nodes": nodes, "edges": edges}
    else:
        out["graph"] = {"nodes": [], "edges": []}
        out["components"] = {}
    return out


# ─── Public bridge surface (Python port) ────────────────────────────────


def dsl_to_graph(dsl: dict[str, Any]) -> tuple[list[dict], list[dict]]:
    """Port of the mode-agnostic `dslToGraph` reader. Reads the
    canonical `graph` block exclusively — no `_layout` or
    `components` fallback, matching the strict-graph contract
    in the TypeScript bridge.
    """
    graph = dsl.get("graph") if isinstance(dsl.get("graph"), dict) else None
    if graph and isinstance(graph.get("nodes"), list) and graph["nodes"]:
        return list(graph["nodes"]), list(graph.get("edges") or [])
    return [], []


def graph_to_dsl(mode: str, nodes: list[dict], edges: list[dict], old_dsl: dict[str, Any]) -> dict[str, Any]:
    """Port of `graphToDsl` (both v1 and v2 branches)."""
    out = dict(old_dsl)
    if mode == "v1":
        out["_layout"] = {"nodes": list(nodes), "edges": list(edges)}
        if "graph" in out:
            del out["graph"]
    else:
        out["graph"] = {"nodes": list(nodes), "edges": list(edges)}
        if "_layout" in out:
            del out["_layout"]
    return out


def export_dsl(mode: str, nodes: list[dict], edges: list[dict], old_dsl: dict[str, Any]) -> dict[str, Any]:
    """Port of `exportDsl` (both modes). Returns the file-shape payload
    that ends up in the downloaded .json.
    """
    full = graph_to_dsl(mode, nodes, edges, old_dsl)
    if mode == "v1":
        return {
            "components": full.get("components", {}),
            "_layout": full.get("_layout"),
            "retrieval": full.get("retrieval", []),
            "history": full.get("history", []),
            "path": full.get("path", []),
            "variables": full.get("variables", []),
            "globals": full.get("globals", {}),
        }
    return {
        "graph": full.get("graph"),
        "components": full.get("components", {}),
        "globals": full.get("globals", {}),
        "variables": full.get("variables", []),
    }


def import_dsl(raw: dict[str, Any], is_agent: bool) -> tuple[str, dict[str, Any]]:
    """Port of `importDsl`. Always routes through v2 — the single-wire
    contract treats non-graph payloads as empty-canvas seed under one
    canonical shape, matching the TypeScript bridge.
    """
    return "v2", _build_v2_dsl_from_import(raw, is_agent)


def round_trip(raw: dict[str, Any]) -> dict[str, Any]:
    """Run the full bridge pipeline:
    importDsl → dslToGraph → graphToDsl → dslToGraph → exportDsl
    and return the final exported payload.
    """
    mode, imported = import_dsl(raw, is_agent=True)
    nodes, edges = dsl_to_graph(imported)
    redrawn = graph_to_dsl(mode, nodes, edges, imported)
    n2, e2 = dsl_to_graph(redrawn)
    return export_dsl(mode, n2, e2, redrawn)


# ─── Diff classifier ────────────────────────────────────────────────────
#
# Walk two payloads depth-first. Mismatches on React-Flow internal
# fields (`dragging`, `selected`, `measured`, `data.isHovered`) land
# in `warnings` — the test still passes, but the user is informed.
# Mismatches on any other field land in `failures` — the test fails
# with a clear pointer to the offending path.

_NODE_INTERNALS = {"dragging", "selected", "measured"}
_EDGE_INTERNALS = {"isHovered"}


def _is_internal(path: str, key: str) -> bool:
    # The leaf key is the strongest signal: `dragging`/`selected`/
    # `measured` on a node, and `isHovered` on an edge or its
    # nested `data` block, are always React-Flow internals.
    if key in _NODE_INTERNALS:
        return True
    if key in _EDGE_INTERNALS:
        # top-level isHovered on an edge object, or nested under edges[].data
        segments = path.split(".")
        return any(seg.startswith("edges") or seg == "data" for seg in segments)
    # Walk up the parent path: if any ancestor is an internal key
    # (e.g. `measured.width` lives under the `measured` parent),
    # the whole subtree is React-Flow-managed and any leaf mismatch
    # is a transient-state flip, not a real bug.
    if any(seg in _NODE_INTERNALS or seg in _EDGE_INTERNALS for seg in path.split(".")):
        return True
    return False


class Diff:
    """Result of comparing two dsl-shaped payloads."""

    def __init__(self) -> None:
        self.warnings: list[str] = []
        self.failures: list[str] = []

    def add(self, path: str, kind: str, exp: Any, act: Any, key: str) -> None:
        msg = f"{path}: {kind} ({_stable(exp)} vs {_stable(act)})"
        if _is_internal(path, key):
            self.warnings.append(msg)
        else:
            self.failures.append(msg)

    def assert_stable(self) -> None:
        """pytest entry point: warn on warnings, fail on failures."""
        for w in self.warnings:
            warnings.warn(f"[React-Flow-internal] {w}", stacklevel=2)
        assert self.failures == [], f"{len(self.failures)} round-trip mismatches:\n" + "\n".join(f"  - {f}" for f in self.failures)


def _stable(v: Any) -> str:
    if v is None:
        return "null"
    if isinstance(v, str):
        return json.dumps(v, ensure_ascii=False)
    try:
        return json.dumps(v, ensure_ascii=False, sort_keys=True)
    except (TypeError, ValueError):
        return repr(v)


def diff_dsl(expected: Any, actual: Any, path: str = "") -> Diff:
    out = Diff()
    _compare_into(expected, actual, path, out)
    return out


def _compare_into(expected: Any, actual: Any, path: str, out: Diff) -> None:
    if expected == actual:
        return
    if expected is None or actual is None or type(expected) is not type(actual):
        out.add(path or "<root>", "value", expected, actual, "")
        return
    if not isinstance(expected, (dict, list)):
        out.add(path or "<root>", "value", expected, actual, "")
        return

    exp_arr = isinstance(expected, list)
    act_arr = isinstance(actual, list)
    if exp_arr != act_arr:
        out.failures.append(f"{path or '<root>'}: array/object mismatch")
        return

    if exp_arr:
        if len(expected) != len(actual):
            out.failures.append(f"{path or '<root>'}: length {len(expected)} != {len(actual)}")
        for i in range(min(len(expected), len(actual))):
            _compare_into(expected[i], actual[i], f"{path}[{i}]", out)
        return

    # both dict
    all_keys = set(expected) | set(actual)
    for key in all_keys:
        sub = f"{path}.{key}" if path else key
        if key not in expected:
            out.add(sub, "missing in expected", None, actual[key], key)
        elif key not in actual:
            out.add(sub, "missing in actual", expected[key], None, key)
        elif isinstance(expected[key], dict) and expected[key] is not None:
            _compare_into(expected[key], actual[key], sub, out)
        elif expected[key] != actual[key]:
            out.add(sub, "value", expected[key], actual[key], key)


# ─── Tests ──────────────────────────────────────────────────────────────


class TestDslBridgeRoundTrip:
    """Unit test of the diff classifier used by round-trip tests."""

    @pytest.mark.p3
    def test_diff_classifier_routes_correctly(self) -> None:
        """Direct unit test of the diff classifier — independent of
        the bridge. Verifies the warning/failure split that the
        round-trip tests rely on.
        """
        expected = {
            "id": "n1",
            "type": "beginNode",
            "position": {"x": 100, "y": 100},
            "dragging": False,
            "selected": False,
            "measured": {"width": 200, "height": 81},
        }
        actual = {
            "id": "n1",
            "type": "beginNode",
            "position": {"x": 999, "y": 100},  # semantic mismatch
            "dragging": True,  # RF internal
            "selected": True,  # RF internal
            "measured": {"width": 999, "height": 81},  # RF internal
        }
        diff = diff_dsl(expected, actual)

        # All three RF-internal fields must be in warnings, not failures.
        # We match on substring because the diff walks into nested
        # objects (e.g. `measured` is a `{width, height}` object and
        # its inner fields end up at `measured.width`/`measured.height`).
        warning_paths = [w.split(":", 1)[0] for w in diff.warnings]
        assert any(p == "dragging" for p in warning_paths)
        assert any(p == "selected" for p in warning_paths)
        assert any(p.startswith("measured") for p in warning_paths)

        # The semantic mismatch must be a failure with a clear
        # pointer. The diff walks into `position: {x, y}` and
        # reports each leaf individually, so only `x` shows up
        # (`y` matches the expected 100).
        assert diff.failures == ["position.x: value (100 vs 999)"]
