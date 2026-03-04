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

import networkx as nx
import pytest

from graphrag.utils import (
    GRAPH_FIELD_SEP,
    GraphChange,
    clean_str,
    compute_args_hash,
    dict_has_keys_with_types,
    flat_uniq_list,
    get_from_to,
    graph_merge,
    handle_single_entity_extraction,
    handle_single_relationship_extraction,
    is_continuous_subsequence,
    is_float_regex,
    merge_tuples,
    pack_user_ass_to_openai_messages,
    perform_variable_replacements,
    split_string_by_multi_markers,
    tidy_graph,
)


class TestCleanStr:
    """Tests for clean_str function."""

    def test_basic_string(self):
        assert clean_str("hello world") == "hello world"

    def test_strips_whitespace(self):
        assert clean_str("  hello  ") == "hello"

    def test_removes_html_escapes(self):
        assert clean_str("&amp; &lt; &gt;") == "& < >"

    def test_removes_control_characters(self):
        assert clean_str("hello\x00world") == "helloworld"
        assert clean_str("test\x1f") == "test"
        assert clean_str("\x7fdata") == "data"

    def test_removes_double_quotes(self):
        assert clean_str('"quoted"') == "quoted"

    def test_non_string_passthrough(self):
        assert clean_str(123) == 123
        assert clean_str(None) is None
        assert clean_str([1, 2]) == [1, 2]

    def test_empty_string(self):
        assert clean_str("") == ""

    def test_combined_html_and_control(self):
        assert clean_str("  &amp;\x00test\x1f  ") == "&test"


class TestDictHasKeysWithTypes:
    """Tests for dict_has_keys_with_types function."""

    def test_matching_keys_and_types(self):
        data = {"name": "Alice", "age": 30}
        assert dict_has_keys_with_types(data, [("name", str), ("age", int)]) is True

    def test_missing_key(self):
        data = {"name": "Alice"}
        assert dict_has_keys_with_types(data, [("name", str), ("age", int)]) is False

    def test_wrong_type(self):
        data = {"name": "Alice", "age": "thirty"}
        assert dict_has_keys_with_types(data, [("name", str), ("age", int)]) is False

    def test_empty_expected_fields(self):
        assert dict_has_keys_with_types({"a": 1}, []) is True

    def test_empty_data(self):
        assert dict_has_keys_with_types({}, [("key", str)]) is False

    def test_subclass_type_match(self):
        assert dict_has_keys_with_types({"val": True}, [("val", int)]) is True


class TestPerformVariableReplacements:
    """Tests for perform_variable_replacements function."""

    def test_simple_replacement(self):
        result = perform_variable_replacements("Hello {name}!", variables={"name": "World"})
        assert result == "Hello World!"

    def test_multiple_replacements(self):
        result = perform_variable_replacements(
            "{greeting} {name}!",
            variables={"greeting": "Hi", "name": "Alice"},
        )
        assert result == "Hi Alice!"

    def test_no_variables(self):
        result = perform_variable_replacements("No vars here")
        assert result == "No vars here"

    def test_empty_variables_dict(self):
        result = perform_variable_replacements("{keep}", variables={})
        assert result == "{keep}"

    def test_history_system_message_replacement(self):
        history = [
            {"role": "system", "content": "You are {role}"},
            {"role": "user", "content": "Hello {role}"},
        ]
        perform_variable_replacements("input", history=history, variables={"role": "assistant"})
        assert history[0]["content"] == "You are assistant"
        assert history[1]["content"] == "Hello {role}"

    def test_none_defaults(self):
        result = perform_variable_replacements("text")
        assert result == "text"

    def test_non_string_variable_value(self):
        result = perform_variable_replacements("count: {n}", variables={"n": 42})
        assert result == "count: 42"


class TestGetFromTo:
    """Tests for get_from_to function."""

    def test_ordered_pair(self):
        assert get_from_to("A", "B") == ("A", "B")

    def test_reversed_pair(self):
        assert get_from_to("B", "A") == ("A", "B")

    def test_equal_values(self):
        assert get_from_to("X", "X") == ("X", "X")

    def test_numeric_strings(self):
        assert get_from_to("2", "1") == ("1", "2")


class TestComputeArgsHash:
    """Tests for compute_args_hash function."""

    def test_deterministic(self):
        h1 = compute_args_hash("a", "b", "c")
        h2 = compute_args_hash("a", "b", "c")
        assert h1 == h2

    def test_different_args_different_hash(self):
        h1 = compute_args_hash("a", "b")
        h2 = compute_args_hash("a", "c")
        assert h1 != h2

    def test_returns_hex_string(self):
        result = compute_args_hash("test")
        assert isinstance(result, str)
        assert len(result) == 32
        int(result, 16)

    def test_empty_args(self):
        result = compute_args_hash()
        assert isinstance(result, str)


class TestIsFloatRegex:
    """Tests for is_float_regex function."""

    @pytest.mark.parametrize(
        "value",
        ["1.0", "0.5", "100", "-3.14", "+2.7", ".5", "0"],
    )
    def test_valid_floats(self, value):
        assert is_float_regex(value)

    @pytest.mark.parametrize(
        "value",
        ["abc", "", "1.2.3", "1e10", "inf", "NaN", " 1.0", "1.0 "],
    )
    def test_invalid_floats(self, value):
        assert not is_float_regex(value)


class TestGraphChange:
    """Tests for GraphChange dataclass."""

    def test_default_empty_sets(self):
        change = GraphChange()
        assert change.removed_nodes == set()
        assert change.added_updated_nodes == set()
        assert change.removed_edges == set()
        assert change.added_updated_edges == set()

    def test_mutable_default_independence(self):
        c1 = GraphChange()
        c2 = GraphChange()
        c1.removed_nodes.add("A")
        assert "A" not in c2.removed_nodes


class TestHandleSingleEntityExtraction:
    """Tests for handle_single_entity_extraction function."""

    def test_valid_entity(self):
        attrs = ['"entity"', "Alice", "Person", "A character"]
        result = handle_single_entity_extraction(attrs, "chunk1")
        assert result is not None
        assert result["entity_name"] == "ALICE"
        assert result["entity_type"] == "PERSON"
        assert result["description"] == "A character"
        assert result["source_id"] == "chunk1"

    def test_not_entity_type(self):
        attrs = ['"relationship"', "A", "B", "desc"]
        assert handle_single_entity_extraction(attrs, "c1") is None

    def test_too_few_attributes(self):
        attrs = ['"entity"', "name", "type"]
        assert handle_single_entity_extraction(attrs, "c1") is None

    def test_empty_entity_name(self):
        attrs = ['"entity"', '""', "Type", "Desc"]
        assert handle_single_entity_extraction(attrs, "c1") is None

    def test_entity_name_uppercased(self):
        attrs = ['"entity"', "alice", "person", "desc"]
        result = handle_single_entity_extraction(attrs, "c1")
        assert result["entity_name"] == "ALICE"
        assert result["entity_type"] == "PERSON"


class TestHandleSingleRelationshipExtraction:
    """Tests for handle_single_relationship_extraction function."""

    def test_valid_relationship(self):
        attrs = ['"relationship"', "Alice", "Bob", "friends with", "friendship", "2.0"]
        result = handle_single_relationship_extraction(attrs, "chunk1")
        assert result is not None
        assert result["src_id"] == "ALICE"
        assert result["tgt_id"] == "BOB"
        assert result["weight"] == 2.0
        assert result["description"] == "friends with"
        assert result["keywords"] == "friendship"
        assert result["source_id"] == "chunk1"
        assert "created_at" in result["metadata"]

    def test_not_relationship_type(self):
        attrs = ['"entity"', "A", "B", "desc", "kw"]
        assert handle_single_relationship_extraction(attrs, "c1") is None

    def test_too_few_attributes(self):
        attrs = ['"relationship"', "A", "B", "desc"]
        assert handle_single_relationship_extraction(attrs, "c1") is None

    def test_non_float_weight_defaults_to_one(self):
        attrs = ['"relationship"', "A", "B", "desc", "kw", "not_a_number"]
        result = handle_single_relationship_extraction(attrs, "c1")
        assert result["weight"] == 1.0

    def test_source_target_sorted(self):
        attrs = ['"relationship"', "Zebra", "Apple", "desc", "kw", "1.0"]
        result = handle_single_relationship_extraction(attrs, "c1")
        assert result["src_id"] == "APPLE"
        assert result["tgt_id"] == "ZEBRA"


class TestPackUserAssToOpenaiMessages:
    """Tests for pack_user_ass_to_openai_messages function."""

    def test_single_message(self):
        result = pack_user_ass_to_openai_messages("hello")
        assert result == [{"role": "user", "content": "hello"}]

    def test_alternating_roles(self):
        result = pack_user_ass_to_openai_messages("q1", "a1", "q2")
        assert result == [
            {"role": "user", "content": "q1"},
            {"role": "assistant", "content": "a1"},
            {"role": "user", "content": "q2"},
        ]

    def test_empty(self):
        result = pack_user_ass_to_openai_messages()
        assert result == []


class TestSplitStringByMultiMarkers:
    """Tests for split_string_by_multi_markers function."""

    def test_single_marker(self):
        result = split_string_by_multi_markers("a|b|c", ["|"])
        assert result == ["a", "b", "c"]

    def test_multiple_markers(self):
        result = split_string_by_multi_markers("a|b;c", ["|", ";"])
        assert result == ["a", "b", "c"]

    def test_no_markers(self):
        result = split_string_by_multi_markers("abc", [])
        assert result == ["abc"]

    def test_strips_whitespace(self):
        result = split_string_by_multi_markers("a | b | c", ["|"])
        assert result == ["a", "b", "c"]

    def test_empty_segments_removed(self):
        result = split_string_by_multi_markers("a||b", ["|"])
        assert result == ["a", "b"]

    def test_regex_special_chars_escaped(self):
        result = split_string_by_multi_markers("a.b.c", ["."])
        assert result == ["a", "b", "c"]


class TestGraphMerge:
    """Tests for graph_merge function."""

    def _make_node(self, description="desc", source_id=None):
        return {"description": description, "source_id": source_id or ["s1"]}

    def _make_edge(self, weight=1.0, description="edge", keywords=None, source_id=None):
        return {
            "weight": weight,
            "description": description,
            "keywords": keywords or [],
            "source_id": source_id or ["s1"],
        }

    def test_merge_disjoint_graphs(self):
        g1 = nx.Graph()
        g1.add_node("A", **self._make_node("A desc"))
        g1.graph["source_id"] = ["doc1"]

        g2 = nx.Graph()
        g2.add_node("B", **self._make_node("B desc"))
        g2.graph["source_id"] = ["doc2"]

        change = GraphChange()
        result = graph_merge(g1, g2, change)

        assert result.has_node("A")
        assert result.has_node("B")
        assert "B" in change.added_updated_nodes
        assert result.graph["source_id"] == ["doc1", "doc2"]

    def test_merge_overlapping_nodes(self):
        g1 = nx.Graph()
        g1.add_node("A", description="first", source_id=["s1"])
        g1.graph["source_id"] = ["doc1"]

        g2 = nx.Graph()
        g2.add_node("A", description="second", source_id=["s2"])
        g2.graph["source_id"] = ["doc2"]

        change = GraphChange()
        graph_merge(g1, g2, change)

        assert f"first{GRAPH_FIELD_SEP}second" == g1.nodes["A"]["description"]
        assert g1.nodes["A"]["source_id"] == ["s1", "s2"]

    def test_merge_overlapping_edges(self):
        g1 = nx.Graph()
        g1.add_node("A", **self._make_node())
        g1.add_node("B", **self._make_node())
        g1.add_edge("A", "B", **self._make_edge(weight=1.0, description="e1", keywords=["k1"], source_id=["s1"]))
        g1.graph["source_id"] = ["doc1"]

        g2 = nx.Graph()
        g2.add_node("A", **self._make_node())
        g2.add_node("B", **self._make_node())
        g2.add_edge("A", "B", **self._make_edge(weight=2.0, description="e2", keywords=["k2"], source_id=["s2"]))
        g2.graph["source_id"] = ["doc2"]

        change = GraphChange()
        graph_merge(g1, g2, change)

        edge = g1.get_edge_data("A", "B")
        assert edge["weight"] == 3.0
        assert f"e1{GRAPH_FIELD_SEP}e2" == edge["description"]
        assert edge["keywords"] == ["k1", "k2"]
        assert edge["source_id"] == ["s1", "s2"]

    def test_merge_tracks_changes(self):
        g1 = nx.Graph()
        g1.graph["source_id"] = []

        g2 = nx.Graph()
        g2.add_node("X", **self._make_node())
        g2.add_node("Y", **self._make_node())
        g2.add_edge("X", "Y", **self._make_edge())
        g2.graph["source_id"] = ["doc1"]

        change = GraphChange()
        graph_merge(g1, g2, change)

        assert {"X", "Y"} == change.added_updated_nodes
        assert {("X", "Y")} == change.added_updated_edges

    def test_merge_sets_rank(self):
        g1 = nx.Graph()
        g1.graph["source_id"] = []

        g2 = nx.Graph()
        g2.add_node("A", **self._make_node())
        g2.add_node("B", **self._make_node())
        g2.add_edge("A", "B", **self._make_edge())
        g2.graph["source_id"] = ["doc1"]

        change = GraphChange()
        graph_merge(g1, g2, change)

        assert g1.nodes["A"]["rank"] == 1
        assert g1.nodes["B"]["rank"] == 1


class TestTidyGraph:
    """Tests for tidy_graph function."""

    def test_removes_nodes_missing_attributes(self):
        g = nx.Graph()
        g.add_node("good", description="d", source_id="s")
        g.add_node("bad")
        messages = []
        tidy_graph(g, lambda msg: messages.append(msg))
        assert g.has_node("good")
        assert not g.has_node("bad")
        assert len(messages) == 1

    def test_removes_edges_missing_attributes(self):
        g = nx.Graph()
        g.add_node("A", description="d", source_id="s")
        g.add_node("B", description="d", source_id="s")
        g.add_edge("A", "B")
        messages = []
        tidy_graph(g, lambda msg: messages.append(msg))
        assert not g.has_edge("A", "B")

    def test_adds_keywords_to_edges_without_it(self):
        g = nx.Graph()
        g.add_node("A", description="d", source_id="s")
        g.add_node("B", description="d", source_id="s")
        g.add_edge("A", "B", description="d", source_id="s")
        tidy_graph(g, None)
        assert g.edges["A", "B"]["keywords"] == []

    def test_skip_attribute_check(self):
        g = nx.Graph()
        g.add_node("no_attrs")
        g.add_edge("no_attrs", "no_attrs")
        tidy_graph(g, None, check_attribute=False)
        assert g.has_node("no_attrs")

    def test_none_callback_no_error(self):
        g = nx.Graph()
        g.add_node("bad")
        tidy_graph(g, None)
        assert not g.has_node("bad")


class TestIsContinuousSubsequence:
    """Tests for is_continuous_subsequence function."""

    def test_basic_match(self):
        assert is_continuous_subsequence(("A", "B"), ("A", "B", "C")) is True

    def test_no_match(self):
        assert is_continuous_subsequence(("A", "C"), ("A", "B", "C")) is False

    def test_at_end(self):
        assert is_continuous_subsequence(("B", "C"), ("A", "B", "C")) is True

    def test_single_element_sequence(self):
        assert is_continuous_subsequence(("A", "B"), ("A",)) is False


class TestMergeTuples:
    """Tests for merge_tuples function."""

    def test_basic_merge(self):
        list1 = [("A", "B")]
        list2 = [("B", "C")]
        result = merge_tuples(list1, list2)
        assert ("A", "B", "C") in result

    def test_no_merge_possible(self):
        list1 = [("A", "B")]
        list2 = [("C", "D")]
        result = merge_tuples(list1, list2)
        assert ("A", "B") in result

    def test_self_loop_kept(self):
        list1 = [("A", "B", "A")]
        list2 = []
        result = merge_tuples(list1, list2)
        assert ("A", "B", "A") in result

    def test_empty_lists(self):
        assert merge_tuples([], []) == []


class TestFlatUniqList:
    """Tests for flat_uniq_list function."""

    def test_flat_lists(self):
        arr = [{"k": [1, 2]}, {"k": [2, 3]}]
        result = flat_uniq_list(arr, "k")
        assert set(result) == {1, 2, 3}

    def test_scalar_values(self):
        arr = [{"k": "a"}, {"k": "b"}, {"k": "a"}]
        result = flat_uniq_list(arr, "k")
        assert set(result) == {"a", "b"}

    def test_empty_list(self):
        assert flat_uniq_list([], "k") == []

    def test_mixed_list_and_scalar(self):
        arr = [{"k": [1, 2]}, {"k": 3}]
        result = flat_uniq_list(arr, "k")
        assert set(result) == {1, 2, 3}
