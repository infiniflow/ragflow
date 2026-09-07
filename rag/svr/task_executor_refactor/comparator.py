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
Comparison Logic Module.

This module provides the [`ContextComparator`](rag/svr/task_executor_refactor/comparator.py:100) class, which compares
intermediate results from two [`RecordingContext`](rag/svr/task_executor_refactor/recording_context.py:54) instances:
one from production execution and one from dry-run execution.

The comparison supports various data types with appropriate strategies:
- Basic types (int, str, bool): Direct equality comparison
- Float numbers: Configurable tolerance range
- Lists: Length comparison + ID set comparison + full content comparison (all chunks)
- Dicts: Key set comparison + recursive value comparison
- None: Equality comparison
"""

import logging
from typing import Any, List, Optional, Set

from rag.svr.task_executor_refactor.recording_context import BaseRecordingContext
from rag.svr.task_executor_refactor.report_generator import (
    ComparisonResult,
    ComparisonReport,
)
from rag.svr.task_executor_refactor.write_operation_interceptor import ALLOWED_METHOD_NAMES


class ContextComparator:
    """Compare two RecordingContext instances for intermediate results.

    This class compares the recorded data from production execution against
    dry-run execution, generating a detailed report of matches and mismatches.

    Usage:
        comparator = ContextComparator()
        report = comparator.compare("task_123", ctx_production, ctx_dry_run)
        print(report.summary())
    """

    # Default tolerance for float comparison
    DEFAULT_FLOAT_TOLERANCE = 1e-6

    # Keys to strip from dict values before comparison (non-deterministic values)
    DICT_KEYS_TO_STRIP = {"seconds", "_created_time", "_elapsed_time"}

    # Keys that represent counts and should be compared as numbers
    COUNT_KEYS = {
        "outline_entry_count",
        "tags_applied_count",
        "final_chunk_count",
        "final_chunk_ids_count",
        "chunk_count",
        "chunk_ids_count",
        "token_count",
        "raptor_token_count",
    }

    # Keys that contain chunk data for comparison
    CHUNK_KEYS = {
        "toc_chunk",
        "raw_chunks",
        "final_chunks",
        "chunks",
        "raptor_chunks",
        "docs_after_prep",
        "dataflow_chunks",
    }

    def __init__(self, float_tolerance: float = None):
        """Initialize the Comparator.

        Args:
            float_tolerance: Tolerance for float comparison.
                Defaults to DEFAULT_FLOAT_TOLERANCE.
        """
        self.float_tolerance = self.DEFAULT_FLOAT_TOLERANCE if float_tolerance is None else float_tolerance

    def _strip_non_deterministic_fields(self, data: dict) -> dict:
        """Remove non-deterministic fields (like 'seconds') from dict values.

        This creates a shallow copy of the data dict with specified keys
        removed from any nested dict values.

        Args:
            data: The input dictionary to process.

        Returns:
            A new dictionary with non-deterministic fields removed.
        """
        import copy
        result = copy.copy(data)
        for key, value in result.items():
            if isinstance(value, dict):
                # Create a new dict without the non-deterministic keys
                cleaned = {
                    k: v for k, v in value.items()
                    if k not in self.DICT_KEYS_TO_STRIP
                }
                result[key] = cleaned
        return result

    @staticmethod
    def _get_key_values_to_compare(prod_data_all:dict):
        prod_data = dict()
        for key, value in prod_data_all.items():
            if key in ALLOWED_METHOD_NAMES:
                continue
            if key.endswith("_time"):
                continue
            if key.startswith("settings.docStoreConn."):
                continue
            prod_data[key] = value
        return prod_data

    def compare(
        self,
        task_id: str,
        ctx_production: BaseRecordingContext,
        ctx_dry_run: BaseRecordingContext,
        comparison_keys: List[str] = None,
    ) -> ComparisonReport:
        """Compare two RecordingContext instances.

        Args:
            task_id: The task identifier.
            ctx_production: RecordingContext from production execution.
            ctx_dry_run: RecordingContext from dry-run execution.
            comparison_keys: Optional list of keys to compare.
                If None, all keys from both contexts will be compared.

        Returns:
            A ComparisonReport with the comparison results.
        """
        report = ComparisonReport(task_id=task_id)

        # Get all keys from both contexts
        prod_data_all = ctx_production.get_all_func_return_values() if ctx_production else {}
        prod_data = self._get_key_values_to_compare(prod_data_all)
        dry_run_data_all = ctx_dry_run.get_all_func_return_values() if ctx_dry_run else {}
        dry_run_data = self._get_key_values_to_compare(dry_run_data_all)

        # Strip non-deterministic fields (like 'seconds') from dict values
        prod_data = self._strip_non_deterministic_fields(prod_data)
        dry_run_data = self._strip_non_deterministic_fields(dry_run_data)

        # Determine keys to compare
        if comparison_keys:
            keys_to_compare = set(comparison_keys)
        else:
            keys_to_compare = set(prod_data.keys()) | set(dry_run_data.keys())

        # Find missing keys
        prod_keys = set(prod_data.keys())
        dry_run_keys = set(dry_run_data.keys())

        report.missing_in_production = sorted(dry_run_keys - prod_keys)
        report.missing_in_dry_run = sorted(prod_keys - dry_run_keys)

        # Compare each key
        for key in sorted(keys_to_compare):
            if key in prod_data and key in dry_run_data:
                result = self.compare_value(key, prod_data[key], dry_run_data[key])
                report.details.append(result)
                if result.match:
                    report.matched_keys += 1
                else:
                    report.mismatched_keys += 1
                    logging.info(f"---prod:{prod_data[key]} diff with dry run:{dry_run_data[key]}")

        report.total_keys = report.matched_keys + report.mismatched_keys
        return report

    def compare_value(
        self,
        key: str,
        prod_value: Any,
        dry_run_value: Any,
    ) -> ComparisonResult:
        """Compare a single value with appropriate strategy.

        Args:
            key: The key being compared.
            prod_value: Value from production context.
            dry_run_value: Value from dry-run context.

        Returns:
            A ComparisonResult with the comparison.
        """
        # Handle None cases
        if prod_value is None and dry_run_value is None:
            return ComparisonResult(key=key, match=True)
        if prod_value is None or dry_run_value is None:
            return ComparisonResult(
                key=key,
                match=False,
                production_value=prod_value,
                dry_run_value=dry_run_value,
                diff_details="One value is None",
            )

        # Handle booleans
        if isinstance(prod_value, bool) and isinstance(dry_run_value, bool):
            match = prod_value == dry_run_value
            return ComparisonResult(
                key=key,
                match=match,
                production_value=prod_value,
                dry_run_value=dry_run_value,
                diff_details=None if match else "Boolean values differ",
            )

        # Handle lists (chunks)
        if isinstance(prod_value, list) and isinstance(dry_run_value, list):
            if key in self.CHUNK_KEYS:
                return self._compare_chunks(key, prod_value, dry_run_value)
            return self._compare_lists(key, prod_value, dry_run_value)

        # Handle dicts
        if isinstance(prod_value, dict) and isinstance(dry_run_value, dict):
            return self._compare_dicts(key, prod_value, dry_run_value)

        # Handle numbers
        if isinstance(prod_value, (int, float)) and isinstance(dry_run_value, (int, float)):
            return self._compare_numbers(key, prod_value, dry_run_value)

        # Handle strings
        if isinstance(prod_value, str) and isinstance(dry_run_value, str):
            match = prod_value == dry_run_value
            return ComparisonResult(
                key=key,
                match=match,
                production_value=prod_value,
                dry_run_value=dry_run_value,
                diff_details=None if match else "String values differ",
            )

        # Default: try direct equality
        match = prod_value == dry_run_value
        return ComparisonResult(
            key=key,
            match=match,
            production_value=prod_value,
            dry_run_value=dry_run_value,
            diff_details=None if match else "Values differ",
        )

    @classmethod
    def _compare_lists(cls, key: str, prod_list: list, dry_run_list: list) -> ComparisonResult:
        """Compare two lists.

        Args:
            key: The key being compared.
            prod_list: List from production context.
            dry_run_list: List from dry-run context.

        Returns:
            A ComparisonResult with the comparison.
        """
        if len(prod_list) != len(dry_run_list):
            return ComparisonResult(
                key=key,
                match=False,
                production_value=len(prod_list),
                dry_run_value=len(dry_run_list),
                diff_details=f"Length differs: {len(prod_list)} vs {len(dry_run_list)}",
            )

        # Try element-wise comparison
        for i, (p, d) in enumerate(zip(prod_list, dry_run_list)):
            if p != d:
                return ComparisonResult(
                    key=key,
                    match=False,
                    production_value=len(prod_list),
                    dry_run_value=len(dry_run_list),
                    diff_details=f"Element {i} differs",
                )

        return ComparisonResult(
            key=key,
            match=True,
            production_value=len(prod_list),
            dry_run_value=len(dry_run_list),
        )

    def _compare_chunks(
        self,
        key: str,
        prod_chunks: list,
        dry_run_chunks: list,
    ) -> ComparisonResult:
        """Compare chunk lists with multi-level strategy.

        Comparison levels:
        1. Length comparison
        2. ID set comparison
        3. Full content comparison (all chunks)

        Args:
            key: The key being compared.
            prod_chunks: Chunks from production context.
            dry_run_chunks: Chunks from dry-run context.

        Returns:
            A ComparisonResult with the comparison.
        """
        # Level 1: Length comparison
        if len(prod_chunks) != len(dry_run_chunks):
            return ComparisonResult(
                key=key,
                match=False,
                production_value=len(prod_chunks),
                dry_run_value=len(dry_run_chunks),
                diff_details=f"Chunk count differs: {len(prod_chunks)} vs {len(dry_run_chunks)}",
            )

        # Level 2: ID set comparison
        prod_ids = self._extract_chunk_ids(prod_chunks)
        dry_run_ids = self._extract_chunk_ids(dry_run_chunks)

        if prod_ids != dry_run_ids:
            missing_ids = prod_ids - dry_run_ids
            extra_ids = dry_run_ids - prod_ids
            details = f"Chunk IDs differ, total prod:{len(prod_ids)}, dry run:{len(dry_run_ids)}"
            if missing_ids:
                details += f", missing in dry-run: {len(missing_ids)}"
            if extra_ids:
                details += f", extra in dry-run: {len(extra_ids)}"
            return ComparisonResult(
                key=key,
                match=False,
                production_value=len(prod_ids),
                dry_run_value=len(dry_run_ids),
                diff_details=details,
            )

        # Level 3: Full content comparison (all chunks)
        content_diffs = self._compare_all_chunks(prod_chunks, dry_run_chunks)
        if content_diffs:
            return ComparisonResult(
                key=key,
                match=False,
                production_value=len(prod_chunks),
                dry_run_value=len(dry_run_chunks),
                diff_details=f"Content differs in samples: {'; '.join(content_diffs[:3])}",
            )

        return ComparisonResult(
            key=key,
            match=True,
            production_value=len(prod_chunks),
            dry_run_value=len(dry_run_chunks),
        )

    def _compare_all_chunks(
        self,
        prod_chunks: list,
        dry_run_chunks: list,
    ) -> List[str]:
        """Compare ALL chunks from both lists.

        Args:
            prod_chunks: Chunks from production context.
            dry_run_chunks: Chunks from dry-run context.

        Returns:
            List of difference descriptions.
        """
        if not prod_chunks or not dry_run_chunks:
            return []

        diffs = []
        n = len(prod_chunks)

        # Check if chunks have valid IDs
        prod_has_id = any(self._get_chunk_id(c) for c in prod_chunks)
        dry_run_has_id = any(self._get_chunk_id(c) for c in dry_run_chunks)
        use_index_matching = not prod_has_id or not dry_run_has_id

        # Build index by chunk ID for matching (only if IDs are available)
        if not use_index_matching:
            dry_run_by_id = {self._get_chunk_id(c): c for c in dry_run_chunks}
        else:
            dry_run_by_id = None

        # Compare ALL chunks
        for idx in range(n):
            prod_chunk = prod_chunks[idx]
            chunk_id = self._get_chunk_id(prod_chunk)

            if use_index_matching:
                # Use index position for matching
                if idx < len(dry_run_chunks):
                    dry_run_chunk = dry_run_chunks[idx]
                else:
                    dry_run_chunk = None
            else:
                # Use ID for matching
                dry_run_chunk = dry_run_by_id.get(chunk_id)

            if dry_run_chunk is None:
                diffs.append(f"Chunk {idx} (id={chunk_id}) not found in dry-run")
                continue

            # Compare content
            content_diff = self._compare_chunk_content(prod_chunk, dry_run_chunk)
            if content_diff:
                diffs.append(f"Chunk {idx} (id={chunk_id}): {content_diff}")

        return diffs

    @classmethod
    def _compare_chunk_content(cls, prod_chunk: dict, dry_run_chunk: dict) -> Optional[str]:
        """Compare content of two chunks.

        Args:
            prod_chunk: Chunk from production context.
            dry_run_chunk: Chunk from dry-run context.

        Returns:
            Difference description or None if matched.
        """
        # Compare key fields
        key_fields = ["content_with_weight", "content_ltks", "doc_id", "kb_id"]
        for fld in key_fields:
            if prod_chunk.get(fld) != dry_run_chunk.get(fld):
                return f"Field '{fld}' differs, prod_chunk:{prod_chunk.get(fld)}, dry_run_chunk:{dry_run_chunk}"

        # Compare vector fields
        prod_vec_keys = {k for k in prod_chunk if k.startswith("q_") and k.endswith("_vec")}
        dry_run_vec_keys = {k for k in dry_run_chunk if k.startswith("q_") and k.endswith("_vec")}

        if prod_vec_keys != dry_run_vec_keys:
            return f"Vector fields differ: {prod_vec_keys} vs {dry_run_vec_keys}"

        for vec_key in prod_vec_keys:
            p_vec = prod_chunk.get(vec_key)
            d_vec = dry_run_chunk.get(vec_key)
            if p_vec != d_vec:
                return f"Vector '{vec_key}' differs"

        return None

    @classmethod
    def _extract_chunk_ids(cls, chunks: list) -> Set[str]:
        """Extract chunk IDs from a list of chunks.

        Args:
            chunks: List of chunk dictionaries.

        Returns:
            Set of chunk IDs.
        """
        ids = set()
        for c in chunks:
            if isinstance(c, dict) and "id" in c:
                ids.add(str(c["id"]))
        return ids

    @classmethod
    def _get_chunk_id(cls, chunk: Any) -> str:
        """Get chunk ID from a chunk dictionary.

        Args:
            chunk: A chunk dictionary.

        Returns:
            Chunk ID as string, or empty string if not found.
        """
        if isinstance(chunk, dict):
            return str(chunk.get("id", ""))
        return ""

    @classmethod
    def _compare_dicts(cls, key: str, prod_dict: dict, dry_run_dict: dict) -> ComparisonResult:
        """Compare two dictionaries.

        Args:
            key: The key being compared.
            prod_dict: Dict from production context.
            dry_run_dict: Dict from dry-run context.

        Returns:
            A ComparisonResult with the comparison.
        """
        prod_keys = set(prod_dict.keys())
        dry_run_keys = set(dry_run_dict.keys())

        if prod_keys != dry_run_keys:
            missing = prod_keys - dry_run_keys
            extra = dry_run_keys - prod_keys
            details = "Keys differ"
            if missing:
                details += f", missing in dry-run: {missing}"
            if extra:
                details += f", extra in dry-run: {extra}"
            return ComparisonResult(
                key=key,
                match=False,
                production_value=sorted(prod_keys),
                dry_run_value=sorted(dry_run_keys),
                diff_details=details,
            )

        # Compare values for each key
        for k in prod_keys:
            p_val = prod_dict[k]
            d_val = dry_run_dict[k]
            if p_val != d_val:
                return ComparisonResult(
                    key=key,
                    match=False,
                    production_value=prod_dict,
                    dry_run_value=dry_run_dict,
                    diff_details=f"Value for key '{k}' differs",
                )

        return ComparisonResult(
            key=key,
            match=True,
            production_value=prod_dict,
            dry_run_value=dry_run_dict,
        )

    def _compare_numbers(
        self,
        key: str,
        prod_value: float,
        dry_run_value: float,
    ) -> ComparisonResult:
        """Compare two numbers with tolerance.

        Args:
            key: The key being compared.
            prod_value: Number from production context.
            dry_run_value: Number from dry-run context.

        Returns:
            A ComparisonResult with the comparison.
        """
        diff = abs(prod_value - dry_run_value)
        if diff <= self.float_tolerance:
            return ComparisonResult(
                key=key,
                match=True,
                production_value=prod_value,
                dry_run_value=dry_run_value,
            )

        return ComparisonResult(
            key=key,
            match=False,
            production_value=prod_value,
            dry_run_value=dry_run_value,
            diff_details=f"Difference {diff} exceeds tolerance {self.float_tolerance}",
        )
