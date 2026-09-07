#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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

"""
Unit tests for rag/utils/raptor_utils.py module.
"""

import pytest
from rag.utils.raptor_utils import (
    RAPTOR_TREE_BUILDER,
    PSI_TREE_BUILDER,
    GMM_CLUSTERING_METHOD,
    AHC_CLUSTERING_METHOD,
    get_raptor_tree_builder,
    get_raptor_clustering_method,
    _as_extra_dict,
    _has_raptor_marker,
    _raptor_methods_from_fields,
    collect_raptor_methods,
    collect_raptor_chunk_ids,
    make_raptor_summary_chunk_id,
    is_structured_file_type,
    is_tabular_pdf,
    should_skip_raptor,
    get_skip_reason,
)


class TestGetRaptorTreeBuilder:
    """Tests for get_raptor_tree_builder function."""

    def test_returns_default_raptor_tree_builder(self):
        """Test that default tree builder is 'raptor'."""
        result = get_raptor_tree_builder(None)
        assert result == RAPTOR_TREE_BUILDER

    def test_returns_default_with_empty_config(self):
        """Test that empty config returns default."""
        result = get_raptor_tree_builder({})
        assert result == RAPTOR_TREE_BUILDER

    def test_returns_configured_tree_builder(self):
        """Test that configured tree builder is returned."""
        config = {"tree_builder": PSI_TREE_BUILDER}
        result = get_raptor_tree_builder(config)
        assert result == PSI_TREE_BUILDER

    def test_returns_ext_tree_builder(self):
        """Test that ext.tree_builder takes precedence."""
        config = {"tree_builder": "old", "ext": {"tree_builder": PSI_TREE_BUILDER}}
        result = get_raptor_tree_builder(config)
        assert result == PSI_TREE_BUILDER

    def test_raises_error_for_unsupported_tree_builder(self):
        """Test that unsupported tree builder raises ValueError."""
        config = {"tree_builder": "unknown"}
        with pytest.raises(ValueError, match="Unsupported RAPTOR tree builder"):
            get_raptor_tree_builder(config)


class TestGetRaptorClusteringMethod:
    """Tests for get_raptor_clustering_method function."""

    def test_returns_default_gmm(self):
        """Test that default clustering method is 'gmm'."""
        result = get_raptor_clustering_method(None)
        assert result == GMM_CLUSTERING_METHOD

    def test_returns_configured_clustering_method(self):
        """Test that configured clustering method is returned."""
        config = {"clustering_method": AHC_CLUSTERING_METHOD}
        result = get_raptor_clustering_method(config)
        assert result == AHC_CLUSTERING_METHOD

    def test_returns_ext_clustering_method(self):
        """Test that ext.clustering_method takes precedence."""
        config = {"clustering_method": "old", "ext": {"clustering_method": AHC_CLUSTERING_METHOD}}
        result = get_raptor_clustering_method(config)
        assert result == AHC_CLUSTERING_METHOD

    def test_raises_error_for_unsupported_clustering_method(self):
        """Test that unsupported clustering method raises ValueError."""
        config = {"clustering_method": "unknown"}
        with pytest.raises(ValueError, match="Unsupported RAPTOR clustering method"):
            get_raptor_clustering_method(config)


class TestAsExtraDict:
    """Tests for _as_extra_dict function."""

    def test_returns_dict_as_is(self):
        """Test that dict input is returned as-is."""
        input_dict = {"key": "value"}
        result = _as_extra_dict(input_dict)
        assert result == input_dict

    def test_returns_empty_dict_for_none(self):
        """Test that None input returns empty dict."""
        result = _as_extra_dict(None)
        assert result == {}

    def test_returns_empty_dict_for_empty_string(self):
        """Test that empty string input returns empty dict."""
        result = _as_extra_dict("")
        assert result == {}

    def test_parses_valid_json_string(self):
        """Test that valid JSON string is parsed correctly."""
        input_str = '{"key": "value"}'
        result = _as_extra_dict(input_str)
        assert result == {"key": "value"}

    def test_returns_empty_dict_for_non_dict_json(self):
        """Test that non-dict JSON returns empty dict."""
        input_str = '[1, 2, 3]'
        result = _as_extra_dict(input_str)
        assert result == {}

    def test_parses_python_dict_literal(self):
        """Test that Python dict literal is parsed."""
        input_str = "{'key': 'value'}"
        result = _as_extra_dict(input_str)
        assert result == {"key": "value"}

    def test_returns_empty_dict_for_malformed_string(self):
        """Test that malformed string returns empty dict."""
        input_str = "{invalid json}"
        result = _as_extra_dict(input_str)
        assert result == {}


class TestHasRaptorMarker:
    """Tests for _has_raptor_marker function."""

    def test_returns_true_for_raptor_string(self):
        """Test that 'raptor' string returns True."""
        assert _has_raptor_marker("raptor") is True

    def test_returns_true_for_raptor_in_list(self):
        """Test that 'raptor' in list returns True."""
        assert _has_raptor_marker(["raptor", "other"]) is True

    def test_returns_false_for_other_string(self):
        """Test that other string returns False."""
        assert _has_raptor_marker("other") is False

    def test_returns_false_for_empty_list(self):
        """Test that empty list returns False."""
        assert _has_raptor_marker([]) is False

    def test_returns_false_for_list_without_raptor(self):
        """Test that list without 'raptor' returns False."""
        assert _has_raptor_marker(["psi", "other"]) is False


class TestRaptorMethodsFromFields:
    """Tests for _raptor_methods_from_fields function."""

    def test_returns_default_raptor_method(self):
        """Test that default method is 'raptor'."""
        result = _raptor_methods_from_fields({})
        assert result == {RAPTOR_TREE_BUILDER}

    def test_returns_method_from_extra_dict(self):
        """Test that method is extracted from extra dict."""
        fields = {"extra": {"raptor_method": PSI_TREE_BUILDER}}
        result = _raptor_methods_from_fields(fields)
        assert result == {PSI_TREE_BUILDER}

    def test_returns_method_from_extra_field(self):
        """Test that method is extracted from extra field directly."""
        fields = {"extra": "{'raptor_method': 'psi'}"}
        result = _raptor_methods_from_fields(fields)
        assert result == {PSI_TREE_BUILDER}

    def test_handles_list_method(self):
        """Test that list method is converted to set."""
        fields = {"extra": {"raptor_method": ["raptor", "psi"]}}
        result = _raptor_methods_from_fields(fields)
        assert result == {RAPTOR_TREE_BUILDER, PSI_TREE_BUILDER}

    def test_handles_empty_method(self):
        """Test that empty method returns default."""
        fields = {"extra": {"raptor_method": ""}}
        result = _raptor_methods_from_fields(fields)
        assert result == {RAPTOR_TREE_BUILDER}


class TestCollectRaptorMethods:
    """Tests for collect_raptor_methods function."""

    def test_returns_empty_set_for_empty_map(self):
        """Test that empty field map returns empty set."""
        result = collect_raptor_methods({})
        assert result == set()

    def test_collects_methods_from_raptor_chunks(self):
        """Test that methods are collected from RAPTOR chunks."""
        field_map = {
            "chunk_1": {
                "raptor_kwd": "raptor",
                "extra": {"raptor_method": PSI_TREE_BUILDER}
            }
        }
        result = collect_raptor_methods(field_map)
        assert result == {PSI_TREE_BUILDER}

    def test_skips_non_raptor_chunks(self):
        """Test that non-RAPTOR chunks are skipped."""
        field_map = {
            "chunk_1": {
                "raptor_kwd": "other",
                "extra": {"raptor_method": PSI_TREE_BUILDER}
            }
        }
        result = collect_raptor_methods(field_map)
        assert result == set()

    def test_collects_multiple_methods(self):
        """Test that multiple methods are collected."""
        field_map = {
            "chunk_1": {"raptor_kwd": "raptor", "extra": {"raptor_method": "raptor"}},
            "chunk_2": {"raptor_kwd": "raptor", "extra": {"raptor_method": "psi"}}
        }
        result = collect_raptor_methods(field_map)
        assert result == {RAPTOR_TREE_BUILDER, PSI_TREE_BUILDER}


class TestCollectRaptorChunkIds:
    """Tests for collect_raptor_chunk_ids function."""

    def test_returns_empty_set_for_empty_map(self):
        """Test that empty field map returns empty set."""
        result = collect_raptor_chunk_ids({})
        assert result == set()

    def test_collects_ids_of_raptor_chunks(self):
        """Test that IDs of RAPTOR chunks are collected."""
        field_map = {
            "chunk_1": {"raptor_kwd": "raptor"},
            "chunk_2": {"raptor_kwd": "raptor"}
        }
        result = collect_raptor_chunk_ids(field_map)
        assert result == {"chunk_1", "chunk_2"}

    def test_excludes_specified_methods(self):
        """Test that specified methods are excluded."""
        field_map = {
            "chunk_1": {"raptor_kwd": "raptor", "extra": {"raptor_method": "raptor"}},
            "chunk_2": {"raptor_kwd": "raptor", "extra": {"raptor_method": "psi"}}
        }
        result = collect_raptor_chunk_ids(field_map, exclude_methods={"raptor"})
        assert result == {"chunk_2"}

    def test_skips_non_raptor_chunks(self):
        """Test that non-RAPTOR chunks are skipped."""
        field_map = {
            "chunk_1": {"raptor_kwd": "raptor"},
            "chunk_2": {"raptor_kwd": "other"}
        }
        result = collect_raptor_chunk_ids(field_map)
        assert result == {"chunk_1"}


class TestMakeRaptorSummaryChunkId:
    """Tests for make_raptor_summary_chunk_id function."""

    def test_generates_consistent_id(self):
        """Test that same input generates same ID."""
        id1 = make_raptor_summary_chunk_id("content", "doc_1")
        id2 = make_raptor_summary_chunk_id("content", "doc_1")
        assert id1 == id2

    def test_generates_different_ids_for_different_content(self):
        """Test that different content generates different ID."""
        id1 = make_raptor_summary_chunk_id("content1", "doc_1")
        id2 = make_raptor_summary_chunk_id("content2", "doc_1")
        assert id1 != id2

    def test_generates_different_ids_for_different_doc(self):
        """Test that different doc_id generates different ID."""
        id1 = make_raptor_summary_chunk_id("content", "doc_1")
        id2 = make_raptor_summary_chunk_id("content", "doc_2")
        assert id1 != id2

    def test_returns_string(self):
        """Test that result is a string."""
        result = make_raptor_summary_chunk_id("content", "doc_1")
        assert isinstance(result, str)


class TestIsStructuredFileType:
    """Tests for is_structured_file_type function."""

    def test_returns_true_for_xlsx(self):
        """Test that .xlsx is recognized as structured."""
        assert is_structured_file_type(".xlsx") is True

    def test_returns_true_for_xls(self):
        """Test that .xls is recognized as structured."""
        assert is_structured_file_type(".xls") is True

    def test_returns_true_for_csv(self):
        """Test that .csv is recognized as structured."""
        assert is_structured_file_type(".csv") is True

    def test_returns_true_for_tsv(self):
        """Test that .tsv is recognized as structured."""
        assert is_structured_file_type(".tsv") is True

    def test_returns_false_for_pdf(self):
        """Test that .pdf is not structured."""
        assert is_structured_file_type(".pdf") is False

    def test_returns_false_for_txt(self):
        """Test that .txt is not structured."""
        assert is_structured_file_type(".txt") is False

    def test_returns_false_for_none(self):
        """Test that None is not structured."""
        assert is_structured_file_type(None) is False

    def test_returns_false_for_empty_string(self):
        """Test that empty string is not structured."""
        assert is_structured_file_type("") is False

    def test_handles_case_insensitive(self):
        """Test that case is handled insensitively."""
        assert is_structured_file_type(".XLSX") is True
        assert is_structured_file_type("xlsx") is True

    def test_handles_missing_dot(self):
        """Test that missing dot is handled."""
        assert is_structured_file_type("xlsx") is True


class TestIsTabularPdf:
    """Tests for is_tabular_pdf function."""

    def test_returns_true_for_table_parser(self):
        """Test that table parser returns True."""
        assert is_tabular_pdf("table", {}) is True

    def test_returns_true_for_html4excel(self):
        """Test that html4excel enabled returns True."""
        assert is_tabular_pdf("naive", {"html4excel": True}) is True

    def test_returns_false_for_naive_parser(self):
        """Test that naive parser returns False."""
        assert is_tabular_pdf("naive", {}) is False

    def test_returns_false_for_empty_parser_id(self):
        """Test that empty parser_id returns False."""
        assert is_tabular_pdf("", {}) is False

    def test_returns_false_for_html4excel_false(self):
        """Test that html4excel=False returns False."""
        assert is_tabular_pdf("naive", {"html4excel": False}) is False

    def test_handles_case_insensitive_parser_id(self):
        """Test that parser_id case is handled."""
        assert is_tabular_pdf("TABLE", {}) is True
        assert is_tabular_pdf("Table", {}) is True


class TestShouldSkipRaptor:
    """Tests for should_skip_raptor function."""

    def test_skips_for_xlsx_file(self):
        """Test that .xlsx file skips Raptor."""
        assert should_skip_raptor(file_type=".xlsx") is True

    def test_skips_for_csv_file(self):
        """Test that .csv file skips Raptor."""
        assert should_skip_raptor(file_type=".csv") is True

    def test_skips_for_tabular_pdf(self):
        """Test that tabular PDF skips Raptor."""
        assert should_skip_raptor(file_type=".pdf", parser_id="table") is True

    def test_does_not_skip_for_normal_pdf(self):
        """Test that normal PDF does not skip Raptor."""
        assert should_skip_raptor(file_type=".pdf", parser_id="naive") is False

    def test_does_not_skip_for_txt_file(self):
        """Test that .txt file does not skip Raptor."""
        assert should_skip_raptor(file_type=".txt") is False

    def test_respects_auto_disable_config_false(self):
        """Test that auto_disable_for_structured_data=False disables skipping."""
        assert should_skip_raptor(
            file_type=".xlsx",
            raptor_config={"auto_disable_for_structured_data": False}
        ) is False

    def test_respects_auto_disable_config_true(self):
        """Test that auto_disable_for_structured_data=True enables skipping."""
        assert should_skip_raptor(
            file_type=".xlsx",
            raptor_config={"auto_disable_for_structured_data": True}
        ) is True

    def test_default_auto_disable_is_true(self):
        """Test that default auto_disable is True."""
        assert should_skip_raptor(file_type=".xlsx") is True

    def test_returns_false_for_none_file_type(self):
        """Test that None file_type does not skip."""
        assert should_skip_raptor(file_type=None) is False


class TestGetSkipReason:
    """Tests for get_skip_reason function."""

    def test_returns_reason_for_structured_file(self):
        """Test that reason is returned for structured file."""
        reason = get_skip_reason(file_type=".xlsx")
        assert "Structured data file" in reason
        assert ".xlsx" in reason

    def test_returns_reason_for_tabular_pdf(self):
        """Test that reason is returned for tabular PDF."""
        reason = get_skip_reason(file_type=".pdf", parser_id="table")
        assert "Tabular PDF" in reason
        assert "table" in reason

    def test_returns_empty_for_normal_pdf(self):
        """Test that empty reason is returned for normal PDF."""
        reason = get_skip_reason(file_type=".pdf", parser_id="naive")
        assert reason == ""

    def test_returns_empty_for_txt_file(self):
        """Test that empty reason is returned for .txt file."""
        reason = get_skip_reason(file_type=".txt")
        assert reason == ""

    def test_returns_empty_for_none_file_type(self):
        """Test that empty reason is returned for None file_type."""
        reason = get_skip_reason(file_type=None)
        assert reason == ""
