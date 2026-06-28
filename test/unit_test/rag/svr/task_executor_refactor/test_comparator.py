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
Unit tests for Comparator module.
"""

from rag.svr.task_executor_refactor.report_generator import (
    ComparisonResult,
    ComparisonReport,
)
from rag.svr.task_executor_refactor.comparator import (
    ContextComparator,
)
from rag.svr.task_executor_refactor.recording_context import RecordingContext


class TestComparisonResult:
    """Tests for ComparisonResult dataclass."""

    def test_init_with_required_fields(self):
        """Test initialization with required fields."""
        result = ComparisonResult(key="test_key", match=True)
        assert result.key == "test_key"
        assert result.match is True
        assert result.production_value is None
        assert result.dry_run_value is None
        assert result.diff_details is None

    def test_init_with_all_fields(self):
        """Test initialization with all fields."""
        result = ComparisonResult(
            key="test_key",
            match=False,
            production_value=100,
            dry_run_value=200,
            diff_details="Values differ"
        )
        assert result.key == "test_key"
        assert result.match is False
        assert result.production_value == 100
        assert result.dry_run_value == 200
        assert result.diff_details == "Values differ"

    def test_to_dict_match(self):
        """Test to_dict for matching result."""
        result = ComparisonResult(key="key", match=True)
        d = result.to_dict()
        assert d == {"key": "key", "match": True, "diff_details": None}

    def test_to_dict_mismatch(self):
        """Test to_dict for mismatching result."""
        result = ComparisonResult(
            key="key",
            match=False,
            diff_details="Difference"
        )
        d = result.to_dict()
        assert d == {"key": "key", "match": False, "diff_details": "Difference"}


class TestComparisonReport:
    """Tests for ComparisonReport dataclass."""

    def test_init_with_required_fields(self):
        """Test initialization with required fields."""
        report = ComparisonReport(task_id="task_123")
        assert report.task_id == "task_123"
        assert report.total_keys == 0
        assert report.matched_keys == 0
        assert report.mismatched_keys == 0
        assert report.missing_in_production == []
        assert report.missing_in_dry_run == []
        assert report.details == []

    def test_summary_no_keys(self):
        """Test summary when no keys to compare."""
        report = ComparisonReport(task_id="task_123")
        assert "No keys to compare" in report.summary()

    def test_summary_with_keys(self):
        """Test summary with keys."""
        report = ComparisonReport(
            task_id="task_123",
            total_keys=10,
            matched_keys=8,
            mismatched_keys=2
        )
        summary = report.summary()
        assert "8/10" in summary
        assert "80.0%" in summary

    def test_to_dict(self):
        """Test to_dict serialization."""
        report = ComparisonReport(
            task_id="task_123",
            total_keys=1,
            matched_keys=1,
            details=[ComparisonResult(key="k", match=True)]
        )
        d = report.to_dict()
        assert d["task_id"] == "task_123"
        assert d["total_keys"] == 1
        assert len(d["details"]) == 1

    def test_to_markdown(self):
        """Test to_markdown serialization."""
        report = ComparisonReport(
            task_id="task_123",
            total_keys=1,
            matched_keys=1,
            mismatched_keys=0,
            missing_in_production=[],
            missing_in_dry_run=[],
            details=[ComparisonResult(key="k", match=True)]
        )
        md = report.to_markdown()
        assert "# Comparison Report: task_123" in md
        assert "## Summary" in md
        assert "## Details" in md

    def test_to_markdown_empty_details(self):
        """Test to_markdown with no details."""
        report = ComparisonReport(task_id="task_123")
        md = report.to_markdown()
        assert "No comparison details" in md


class TestContextComparatorCompareValue:
    """Tests for ContextComparator.compare_value method and initialization."""

    def test_init_default_tolerance(self):
        """Test initialization with default tolerance."""
        assert ContextComparator().float_tolerance == 1e-6

    def test_init_custom_tolerance(self):
        """Test initialization with custom tolerance."""
        assert ContextComparator(float_tolerance=0.01).float_tolerance == 0.01

    def setup_method(self):
        self.comparator = ContextComparator()

    def test_compare_none_values(self):
        """Test comparing None values."""
        result = self.comparator.compare_value("key", None, None)
        assert result.match is True

    def test_compare_one_none(self):
        """Test comparing when one value is None."""
        result = self.comparator.compare_value("key", 1, None)
        assert result.match is False
        assert "None" in result.diff_details

    def test_compare_equal_strings(self):
        """Test comparing equal strings."""
        result = self.comparator.compare_value("key", "hello", "hello")
        assert result.match is True

    def test_compare_different_strings(self):
        """Test comparing different strings."""
        result = self.comparator.compare_value("key", "hello", "world")
        assert result.match is False

    def test_compare_equal_booleans(self):
        """Test comparing equal booleans."""
        result = self.comparator.compare_value("key", True, True)
        assert result.match is True

    def test_compare_different_booleans(self):
        """Test comparing different booleans."""
        result = self.comparator.compare_value("key", True, False)
        assert result.match is False

    def test_compare_equal_integers(self):
        """Test comparing equal integers."""
        result = self.comparator.compare_value("key", 42, 42)
        assert result.match is True

    def test_compare_equal_floats_within_tolerance(self):
        """Test comparing equal floats within tolerance."""
        result = self.comparator.compare_value("key", 1.0000001, 1.0000002)
        assert result.match is True

    def test_compare_different_floats_exceeding_tolerance(self):
        """Test comparing floats exceeding tolerance."""
        result = self.comparator.compare_value("key", 1.0, 2.0)
        assert result.match is False
        assert "exceeds tolerance" in result.diff_details

    def test_compare_equal_lists(self):
        """Test comparing equal lists."""
        result = self.comparator.compare_value("key", [1, 2, 3], [1, 2, 3])
        assert result.match is True

    def test_compare_different_length_lists(self):
        """Test comparing lists with different lengths."""
        result = self.comparator.compare_value("key", [1, 2], [1, 2, 3])
        assert result.match is False
        assert "Length differs" in result.diff_details

    def test_compare_equal_dicts(self):
        """Test comparing equal dicts."""
        result = self.comparator.compare_value("key", {"a": 1}, {"a": 1})
        assert result.match is True

    def test_compare_different_dicts(self):
        """Test comparing different dicts."""
        result = self.comparator.compare_value("key", {"a": 1}, {"a": 2})
        assert result.match is False

    def test_compare_chunks_key_uses_chunk_comparison(self):
        """Test that chunk keys use chunk comparison strategy."""
        result = self.comparator.compare_value(
            "raw_chunks",
            [{"id": "1", "content_with_weight": "a"}],
            [{"id": "1", "content_with_weight": "a"}]
        )
        assert result.match is True


class TestContextComparatorCompareLists:
    """Tests for _compare_lists method."""

    def test_equal_lists(self):
        """Test comparing equal lists."""
        result = ContextComparator._compare_lists("key", [1, 2], [1, 2])
        assert result.match is True

    def test_different_length_lists(self):
        """Test comparing lists with different lengths."""
        result = ContextComparator._compare_lists("key", [1], [1, 2])
        assert result.match is False

    def test_different_elements(self):
        """Test comparing lists with different elements."""
        result = ContextComparator._compare_lists("key", [1, 2], [1, 3])
        assert result.match is False


class TestContextComparatorCompareDicts:
    """Tests for _compare_dicts method."""

    def test_equal_dicts(self):
        """Test comparing equal dicts."""
        result = ContextComparator._compare_dicts("key", {"a": 1}, {"a": 1})
        assert result.match is True

    def test_dicts_different_keys(self):
        """Test comparing dicts with different keys."""
        result = ContextComparator._compare_dicts("key", {"a": 1}, {"b": 1})
        assert result.match is False
        assert "Keys differ" in result.diff_details

    def test_dicts_same_keys_different_values(self):
        """Test comparing dicts with same keys but different values."""
        result = ContextComparator._compare_dicts("key", {"a": 1}, {"a": 2})
        assert result.match is False


class TestContextComparatorCompareNumbers:
    """Tests for _compare_numbers method."""

    def test_equal_numbers(self):
        """Test comparing equal numbers."""
        comparator = ContextComparator()
        result = comparator._compare_numbers("key", 1.0, 1.0)
        assert result.match is True

    def test_numbers_within_tolerance(self):
        """Test comparing numbers within tolerance."""
        comparator = ContextComparator(float_tolerance=0.1)
        result = comparator._compare_numbers("key", 1.0, 1.05)
        assert result.match is True

    def test_numbers_exceeding_tolerance(self):
        """Test comparing numbers exceeding tolerance."""
        comparator = ContextComparator(float_tolerance=0.01)
        result = comparator._compare_numbers("key", 1.0, 1.1)
        assert result.match is False


class TestContextComparatorCompareChunks:
    """Tests for _compare_chunks method."""

    def setup_method(self):
        """Set up test fixtures."""
        self.comparator = ContextComparator()

    def test_equal_chunks(self):
        """Test comparing equal chunk lists."""
        prod = [{"id": "1", "content_with_weight": "a"}]
        dry = [{"id": "1", "content_with_weight": "a"}]
        result = self.comparator._compare_chunks("raw_chunks", prod, dry)
        assert result.match is True

    def test_different_count_chunks(self):
        """Test comparing chunks with different counts."""
        prod = [{"id": "1"}]
        dry = [{"id": "1"}, {"id": "2"}]
        result = self.comparator._compare_chunks("raw_chunks", prod, dry)
        assert result.match is False
        assert "Chunk count differs" in result.diff_details

    def test_different_ids_chunks(self):
        """Test comparing chunks with different IDs."""
        prod = [{"id": "1"}]
        dry = [{"id": "2"}]
        result = self.comparator._compare_chunks("raw_chunks", prod, dry)
        assert result.match is False
        assert "Chunk IDs differ" in result.diff_details

    def test_empty_chunks_lists(self):
        """Test comparing empty chunk lists."""
        result = self.comparator._compare_chunks("raw_chunks", [], [])
        assert result.match is True

    def test_all_chunks_compared_not_sampled(self):
        """Test that ALL chunks are compared, not just samples.
        
        This test creates 10 chunks where only the middle one (index 5) differs.
        With the old sampling strategy, this difference might be missed.
        With full comparison, the difference should always be detected.
        """
        prod = [{"id": str(i), "content_with_weight": f"content_{i}"} for i in range(10)]
        dry = [{"id": str(i), "content_with_weight": f"content_{i}"} for i in range(10)]
        # Only modify chunk at index 5 (which might not be sampled in old strategy)
        dry[5]["content_with_weight"] = "different_content"
        
        result = self.comparator._compare_chunks("raw_chunks", prod, dry)
        assert result.match is False
        assert "Content differs" in result.diff_details

    def test_all_chunks_detect_first_difference(self):
        """Test that first chunk difference is detected."""
        prod = [{"id": "1", "content_with_weight": "a"}, {"id": "2", "content_with_weight": "b"}]
        dry = [{"id": "1", "content_with_weight": "different"}, {"id": "2", "content_with_weight": "b"}]
        
        result = self.comparator._compare_chunks("raw_chunks", prod, dry)
        assert result.match is False

    def test_all_chunks_detect_last_difference(self):
        """Test that last chunk difference is detected."""
        prod = [{"id": "1", "content_with_weight": "a"}, {"id": "2", "content_with_weight": "b"}]
        dry = [{"id": "1", "content_with_weight": "a"}, {"id": "2", "content_with_weight": "different"}]
        
        result = self.comparator._compare_chunks("raw_chunks", prod, dry)
        assert result.match is False

    def test_all_chunks_large_list_all_match(self):
        """Test that large list of chunks all match."""
        prod = [{"id": str(i), "content_with_weight": f"content_{i}"} for i in range(100)]
        dry = [{"id": str(i), "content_with_weight": f"content_{i}"} for i in range(100)]
        
        result = self.comparator._compare_chunks("raw_chunks", prod, dry)
        assert result.match is True

    def test_all_chunks_large_list_one_mismatch(self):
        """Test that a single mismatch in a large list is detected."""
        prod = [{"id": str(i), "content_with_weight": f"content_{i}"} for i in range(100)]
        dry = [{"id": str(i), "content_with_weight": f"content_{i}"} for i in range(100)]
        # Modify only the last chunk
        dry[99]["content_with_weight"] = "different"
        
        result = self.comparator._compare_chunks("raw_chunks", prod, dry)
        assert result.match is False


class TestContextComparatorExtractChunkIds:
    """Tests for _extract_chunk_ids method."""

    def test_extract_ids_from_valid_chunks(self):
        """Test extracting IDs from valid chunks."""
        chunks = [{"id": "1"}, {"id": "2"}, {"id": "3"}]
        ids = ContextComparator._extract_chunk_ids(chunks)
        assert ids == {"1", "2", "3"}

    def test_extract_ids_from_empty_chunks(self):
        """Test extracting IDs from empty list."""
        ids = ContextComparator._extract_chunk_ids([])
        assert ids == set()

    def test_extract_ids_from_chunks_without_id(self):
        """Test extracting IDs from chunks without id field."""
        chunks = [{"content": "a"}, {"id": "1"}]
        ids = ContextComparator._extract_chunk_ids(chunks)
        assert ids == {"1"}


class TestContextComparatorGetChunkId:
    """Tests for _get_chunk_id method."""

    def test_get_id_from_valid_chunk(self):
        """Test getting ID from valid chunk."""
        chunk = {"id": "123"}
        assert ContextComparator._get_chunk_id(chunk) == "123"

    def test_get_id_from_chunk_without_id(self):
        """Test getting ID from chunk without id."""
        chunk = {"content": "a"}
        assert ContextComparator._get_chunk_id(chunk) == ""

    def test_get_id_from_non_dict(self):
        """Test getting ID from non-dict."""
        assert ContextComparator._get_chunk_id("not a dict") == ""


class TestContextComparatorCompare:
    """Tests for compare method."""

    def setup_method(self):
        """Set up test fixtures."""
        self.comparator = ContextComparator()

    def test_compare_empty_contexts(self):
        """Test comparing empty contexts."""
        ctx1 = RecordingContext()
        ctx2 = RecordingContext()
        report = self.comparator.compare("task_1", ctx1, ctx2)
        assert report.total_keys == 0

    def test_compare_matching_values(self):
        """Test comparing contexts with matching values."""
        ctx1 = RecordingContext()
        ctx2 = RecordingContext()
        ctx1.record("key", "value")
        ctx2.record("key", "value")
        report = self.comparator.compare("task_1", ctx1, ctx2)
        assert report.matched_keys == 1
        assert report.mismatched_keys == 0

    def test_compare_mismatching_values(self):
        """Test comparing contexts with mismatching values."""
        ctx1 = RecordingContext()
        ctx2 = RecordingContext()
        ctx1.record("key1", "value1")
        ctx2.record("key1", "value2")
        report = self.comparator.compare("task_1", ctx1, ctx2)
        assert report.mismatched_keys == 1

    def test_compare_missing_key_in_one_context(self):
        """Test comparing when key is missing in one context."""
        ctx1 = RecordingContext()
        ctx2 = RecordingContext()
        ctx1.record("key1", "value1")
        report = self.comparator.compare("task_1", ctx1, ctx2)
        assert "key1" in report.missing_in_dry_run

    def test_compare_with_specific_keys(self):
        """Test comparing with specific keys list."""
        ctx1 = RecordingContext()
        ctx2 = RecordingContext()
        ctx1.record("key1", "value1")
        ctx1.record("key2", "value2")
        ctx2.record("key1", "value1")
        ctx2.record("key2", "value2")
        report = self.comparator.compare("task_1", ctx1, ctx2, comparison_keys=["key1"])
        assert report.total_keys == 1

    def test_compare_filters_out_time_keys(self):
        """Test that _time keys are filtered out."""
        ctx1 = RecordingContext()
        ctx2 = RecordingContext()
        ctx1.record("operation_time", 1.0)
        ctx2.record("operation_time", 1.0)
        report = self.comparator.compare("task_1", ctx1, ctx2)
        assert report.total_keys == 0


class TestContextComparatorStripNonDeterministicFields:
    """Tests for _strip_non_deterministic_fields method."""

    def setup_method(self):
        """Set up test fixtures."""
        self.comparator = ContextComparator()

    def test_strip_seconds_from_dict_value(self):
        """Test that 'seconds' key is removed from dict values."""
        data = {
            "graphrag_result": {"seconds": 45.48, "status": "done"},
            "other_key": "value"
        }
        result = self.comparator._strip_non_deterministic_fields(data)
        assert "seconds" not in result["graphrag_result"]
        assert result["graphrag_result"] == {"status": "done"}
        assert result["other_key"] == "value"

    def test_strip_seconds_from_multiple_dict_values(self):
        """Test that 'seconds' is removed from multiple dict values."""
        data = {
            "result1": {"seconds": 10.0, "count": 5},
            "result2": {"seconds": 20.0, "name": "test"},
            "simple_key": 123
        }
        result = self.comparator._strip_non_deterministic_fields(data)
        assert result["result1"] == {"count": 5}
        assert result["result2"] == {"name": "test"}
        assert result["simple_key"] == 123

    def test_strip_does_not_modify_original_dict(self):
        """Test that the original dict is not modified in place."""
        data = {
            "result": {"seconds": 1.0, "value": "test"}
        }
        _ = data["result"].copy()
        self.comparator._strip_non_deterministic_fields(data)
        # The original nested dict should still have seconds since we only do shallow copy
        assert "seconds" in data["result"]

    def test_strip_with_empty_dict_values(self):
        """Test handling of empty dict values."""
        data = {
            "empty_dict": {},
            "normal_key": "value"
        }
        result = self.comparator._strip_non_deterministic_fields(data)
        assert result["empty_dict"] == {}
        assert result["normal_key"] == "value"

    def test_strip_with_non_dict_values(self):
        """Test that non-dict values are not affected."""
        data = {
            "string_val": "test",
            "int_val": 42,
            "list_val": [1, 2, 3],
            "dict_val": {"seconds": 1.0, "name": "test"}
        }
        result = self.comparator._strip_non_deterministic_fields(data)
        assert result["string_val"] == "test"
        assert result["int_val"] == 42
        assert result["list_val"] == [1, 2, 3]
        assert result["dict_val"] == {"name": "test"}

    def test_strip_seconds_from_graphrag_result(self):
        """Test the specific case from the bug report: graphrag_result with seconds."""
        prod_data = {
            "graphrag_result": {
                "seconds": 45.48,
                "status": "success",
                "entity_count": 100
            }
        }
        dry_run_data = {
            "graphrag_result": {
                "seconds": 0.99,
                "status": "success",
                "entity_count": 100
            }
        }
        prod_stripped = self.comparator._strip_non_deterministic_fields(prod_data)
        dry_run_stripped = self.comparator._strip_non_deterministic_fields(dry_run_data)
        
        # After stripping, both should be equal (except for seconds)
        assert prod_stripped["graphrag_result"] == {"status": "success", "entity_count": 100}
        assert dry_run_stripped["graphrag_result"] == {"status": "success", "entity_count": 100}
        assert prod_stripped["graphrag_result"] == dry_run_stripped["graphrag_result"]

    def test_compare_with_seconds_in_dict_values(self):
        """Test that compare correctly handles dict values with 'seconds' field."""
        ctx1 = RecordingContext()
        ctx2 = RecordingContext()
        ctx1.record("graphrag_result", {"seconds": 45.48, "status": "success"})
        ctx2.record("graphrag_result", {"seconds": 0.99, "status": "success"})
        
        report = self.comparator.compare("task_1", ctx1, ctx2)
        # Should match because seconds is stripped
        assert report.matched_keys == 1
        assert report.mismatched_keys == 0

    def test_compare_with_different_dict_values_excluding_seconds(self):
        """Test that compare correctly detects differences in dict values (excluding seconds)."""
        ctx1 = RecordingContext()
        ctx2 = RecordingContext()
        ctx1.record("graphrag_result", {"seconds": 45.48, "status": "success", "count": 100})
        ctx2.record("graphrag_result", {"seconds": 0.99, "status": "failed", "count": 50})
        
        report = self.comparator.compare("task_1", ctx1, ctx2)
        # Should mismatch because status and count differ
        assert report.mismatched_keys == 1
        assert report.matched_keys == 0
