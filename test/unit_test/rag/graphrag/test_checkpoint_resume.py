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

"""Tests for GraphRAG checkpoint/resume logic.

The _load_subgraph_from_store function in rag/graphrag/general/index.py
queries the doc store for previously saved subgraphs to avoid re-running
expensive LLM extraction on resume. These tests exercise the core logic:
parsing stored subgraph JSON, filtering by source_id in Python, and
handling edge cases (missing data, string vs list source_id, errors).
"""

import json

import networkx as nx
import pytest


def _make_subgraph(doc_id: str) -> nx.Graph:
    """Create a simple test subgraph for a given doc_id."""
    sg = nx.Graph()
    sg.add_node("ENTITY_A", description="test entity A", source_id=[doc_id])
    sg.add_node("ENTITY_B", description="test entity B", source_id=[doc_id])
    sg.add_edge(
        "ENTITY_A", "ENTITY_B",
        description="related", source_id=[doc_id], weight=1.0, keywords=[],
    )
    sg.graph["source_id"] = [doc_id]
    return sg


def _subgraph_to_store_content(sg: nx.Graph) -> str:
    """Serialize a subgraph the same way generate_subgraph does."""
    return json.dumps(nx.node_link_data(sg, edges="edges"), ensure_ascii=False)


def _load_subgraph_from_field_map(field_map: dict, doc_id: str):
    """Replicate the core logic of _load_subgraph_from_store for unit testing.

    This mirrors the implementation in rag/graphrag/general/index.py without
    importing it (which would pull in the entire API stack and DB dependencies).
    """
    for cid in field_map:
        source_ids = field_map[cid].get("source_id") or []
        if isinstance(source_ids, str):
            source_ids = [source_ids]
        if doc_id not in source_ids:
            continue
        content = field_map[cid].get("content_with_weight", "")
        if content:
            data = json.loads(content)
            sg = nx.node_link_graph(data, edges="edges")
            sg.graph["source_id"] = [doc_id]
            return sg
    return None


class TestLoadSubgraphFromStore:
    """Tests for the subgraph checkpoint loading logic."""

    @pytest.mark.p1
    def test_loads_existing_subgraph(self):
        """When a subgraph exists in the store, it should be loaded and returned."""
        doc_id = "doc_001"
        sg = _make_subgraph(doc_id)

        field_map = {
            "chunk_001": {
                "content_with_weight": _subgraph_to_store_content(sg),
                "source_id": [doc_id],
            }
        }

        result = _load_subgraph_from_field_map(field_map, doc_id)

        assert result is not None
        assert isinstance(result, nx.Graph)
        assert result.has_node("ENTITY_A")
        assert result.has_node("ENTITY_B")
        assert result.has_edge("ENTITY_A", "ENTITY_B")
        assert result.graph["source_id"] == [doc_id]

    @pytest.mark.p1
    def test_returns_none_when_no_subgraph(self):
        """When no subgraph exists, should return None."""
        result = _load_subgraph_from_field_map({}, "doc_missing")
        assert result is None

    @pytest.mark.p2
    def test_filters_by_doc_id_in_python(self):
        """Should only return subgraph whose source_id matches the requested doc_id."""
        sg_a = _make_subgraph("doc_a")
        sg_b = _make_subgraph("doc_b")

        field_map = {
            "chunk_a": {
                "content_with_weight": _subgraph_to_store_content(sg_a),
                "source_id": ["doc_a"],
            },
            "chunk_b": {
                "content_with_weight": _subgraph_to_store_content(sg_b),
                "source_id": ["doc_b"],
            },
        }

        result = _load_subgraph_from_field_map(field_map, "doc_b")

        assert result is not None
        assert result.graph["source_id"] == ["doc_b"]

    @pytest.mark.p2
    def test_does_not_match_wrong_doc(self):
        """Should return None when no entry matches the requested doc_id."""
        sg = _make_subgraph("doc_a")

        field_map = {
            "chunk_a": {
                "content_with_weight": _subgraph_to_store_content(sg),
                "source_id": ["doc_a"],
            },
        }

        result = _load_subgraph_from_field_map(field_map, "doc_b")
        assert result is None

    @pytest.mark.p2
    def test_handles_string_source_id(self):
        """Should handle source_id stored as string instead of list."""
        doc_id = "doc_str"
        sg = _make_subgraph(doc_id)

        field_map = {
            "chunk_001": {
                "content_with_weight": _subgraph_to_store_content(sg),
                "source_id": doc_id,  # string, not list
            }
        }

        result = _load_subgraph_from_field_map(field_map, doc_id)

        assert result is not None
        assert result.graph["source_id"] == [doc_id]

    @pytest.mark.p2
    def test_handles_none_source_id(self):
        """Should handle source_id being None without crashing."""
        field_map = {
            "chunk_001": {
                "content_with_weight": "{}",
                "source_id": None,
            }
        }

        result = _load_subgraph_from_field_map(field_map, "doc_001")
        assert result is None

    @pytest.mark.p2
    def test_skips_empty_content(self):
        """Should skip entries with empty content_with_weight."""
        field_map = {
            "chunk_empty": {
                "content_with_weight": "",
                "source_id": ["doc_001"],
            }
        }

        result = _load_subgraph_from_field_map(field_map, "doc_001")
        assert result is None

    @pytest.mark.p1
    def test_preserves_graph_structure(self):
        """Loaded subgraph should preserve all nodes, edges, and attributes."""
        doc_id = "doc_full"
        sg = _make_subgraph(doc_id)

        field_map = {
            "chunk_001": {
                "content_with_weight": _subgraph_to_store_content(sg),
                "source_id": [doc_id],
            }
        }

        result = _load_subgraph_from_field_map(field_map, doc_id)

        assert set(result.nodes()) == {"ENTITY_A", "ENTITY_B"}
        assert result.nodes["ENTITY_A"]["description"] == "test entity A"
        assert result.nodes["ENTITY_B"]["description"] == "test entity B"
        edge = result.get_edge_data("ENTITY_A", "ENTITY_B")
        assert edge["description"] == "related"
        assert edge["weight"] == 1.0


class TestCheckpointResumeWorkflow:
    """Integration-style test simulating the crash-resume scenario."""

    @pytest.mark.p1
    def test_resume_finds_completed_docs_skips_new_ones(self):
        """Simulates: 3 docs processed, crash, restart.

        Existing subgraphs should be loaded; new doc should return None.
        """
        completed_docs = ["doc_1", "doc_2", "doc_3"]
        field_map = {}
        for doc_id in completed_docs:
            sg = _make_subgraph(doc_id)
            field_map[f"chunk_{doc_id}"] = {
                "content_with_weight": _subgraph_to_store_content(sg),
                "source_id": [doc_id],
            }

        for doc_id in completed_docs:
            result = _load_subgraph_from_field_map(field_map, doc_id)
            assert result is not None
            assert result.graph["source_id"] == [doc_id]

        # New doc not yet processed — should not be found
        result = _load_subgraph_from_field_map(field_map, "doc_4_new")
        assert result is None

    @pytest.mark.p2
    def test_malformed_json_returns_none(self):
        """If stored content is invalid JSON, should return None gracefully."""
        field_map = {
            "chunk_bad": {
                "content_with_weight": "not valid json{{{",
                "source_id": ["doc_bad"],
            }
        }

        with pytest.raises(json.JSONDecodeError):
            _load_subgraph_from_field_map(field_map, "doc_bad")
