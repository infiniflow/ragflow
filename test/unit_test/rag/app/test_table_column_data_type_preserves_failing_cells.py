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
#
"""Regression tests for ``rag.app.table.column_data_type`` retaining the
original cell value when the column-level type conversion fails.

Issue: infiniflow/ragflow#18459.

Pre-fix, ``column_data_type`` silently set ``arr[i] = None`` when
``trans[ty](str(arr[i]))`` raised, and ``trans_bool`` / ``trans_datatime``
also return ``None`` (not raise) for unparseable inputs. The downstream
``chunk()`` then dropped ``None`` cells from both the chunk body and the
typed ES field, so a column of mostly numerics carrying ``N/A``,
``TBD``, ``-``, ``pending`` or a footnote lost exactly those cells with
no user-visible error. The producer also contradicts a comment added
in PR #17946 that says the original value is preserved, and the
consumer in ``chunk()`` has a now-reachable ``isinstance(val, str)``
branch that was previously dead code.

The fix keeps the original value when the conversion raises or returns
``None`` for ``datetime`` / ``bool`` (the two success-returning-None
converters).
"""

import sys
from unittest.mock import MagicMock

import pytest


@pytest.fixture(scope="module")
def column_data_type():
    """Import ``rag.app.table.column_data_type`` with the same
    import-stubbing the issue's repro script uses, so we exercise the
    real function without pulling in Elasticsearch / MinIO / MySQL
    backends that ``rag.app.table`` imports at module load time.
    """
    stub_names = [
        "api.db.services.knowledgebase_service",
        "deepdoc.parser",
        "deepdoc.parser.figure_parser",
        "deepdoc.parser.utils",
        "deepdoc.vision.ocr",
        "rag.app.picture",
    ]
    original_modules = {name: sys.modules.get(name) for name in stub_names}
    try:
        for name in stub_names:
            module = MagicMock()
            module.__path__ = []
            sys.modules[name] = module
        from rag.app import table
    finally:
        for name, original in original_modules.items():
            if original is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = original
    return table.column_data_type


def test_int_column_preserves_text_cell(column_data_type):
    """Regression for #18459: a mostly-numeric column that carries
    ``N/A`` keeps the original string instead of nulling it."""
    out, ty = column_data_type(["1", "2", "3", "N/A"])
    assert ty == "int"
    assert out == [1, 2, 3, "N/A"]


def test_float_column_preserves_text_cell(column_data_type):
    """Mostly-float column that carries ``see note`` keeps the original
    string."""
    out, ty = column_data_type(["1.5", "2.5", "3.5", "see note"])
    assert ty == "float"
    assert out == [1.5, 2.5, 3.5, "see note"]


def test_bool_column_preserves_unknown_value(column_data_type):
    """``trans_bool`` returns ``None`` (not raise) for ``unknown``;
    the fix must keep the original string. The consumer in
    ``chunk()`` checks ``isinstance(val, str)`` to handle this case."""
    out, ty = column_data_type(["yes", "no", "yes", "unknown"])
    assert ty == "bool"
    assert out == ["yes", "no", "yes", "unknown"]


def test_datetime_column_preserves_unparseable_value(column_data_type):
    """``trans_datatime`` returns ``None`` (not raise) for
    ``ongoing``; the fix must keep the original string."""
    out, ty = column_data_type(["2025-01-01", "2025-02-01", "ongoing"])
    assert ty == "datetime"
    assert out == ["2025-01-01 00:00:00", "2025-02-01 00:00:00", "ongoing"]


def test_dash_and_pending_are_preserved(column_data_type):
    """Common Excel placeholders (``-``, ``TBD``, ``pending``) in a
    mostly-numeric column stay in the output as strings. The column
    type inference uses counts; 4/6 ints wins the tie against text
    per the existing ``type_priority`` ordering in ``column_data_type``.
    """
    out, ty = column_data_type(["1", "2", "3", "4", "-", "TBD", "pending"])
    assert ty == "int"
    assert out == [1, 2, 3, 4, "-", "TBD", "pending"]


def test_successful_conversion_still_converts(column_data_type):
    """Regression guard: cells that successfully convert must still
    convert. The fix only changes behavior for failing / None-returning
    converters; pure ints must remain ints, pure datetimes must
    remain datetime strings."""
    out, ty = column_data_type(["1", "2", "3"])
    assert ty == "int"
    assert out == [1, 2, 3]

    out, ty = column_data_type(["2025-01-01", "2025-02-01"])
    assert ty == "datetime"
    assert out == ["2025-01-01 00:00:00", "2025-02-01 00:00:00"]
