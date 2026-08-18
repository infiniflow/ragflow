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

"""#18459: cells the column-level type conversion cannot represent must be
preserved, not silently dropped.

The column type is a majority vote, and any cell the winning converter
cannot represent (``N/A`` in a numeric column, ``unknown`` in a bool
column, ``ongoing`` in a datetime column) was replaced with ``None`` —
missing from the chunk body (chunk() skips None cells) AND from the stored
field. The fix makes the conversion keep the original value, which is
exactly the shape chunk()'s ES branch for strings in non-text columns was
written against (that branch was unreachable dead code before)."""

import sys
from unittest.mock import MagicMock

# Same shape as test_table_chunk_column_roles.py: keep optional
# storage/parser backends out of the import path.
for _name in (
    "api.db.services.knowledgebase_service",
    "deepdoc.parser",
    "deepdoc.parser.figure_parser",
    "deepdoc.parser.utils",
    "deepdoc.vision.ocr",
    "rag.app.picture",
    "common.settings",
):
    _m = MagicMock()
    _m.__path__ = []
    sys.modules[_name] = _m
sys.modules["common.settings"].DOC_ENGINE_INFINITY = False

import pytest

from rag.app.table import column_data_type


@pytest.mark.p1
def test_int_column_preserves_unconvertible_cell():
    out, ty = column_data_type(["1", "2", "3", "N/A"])
    assert ty == "int"
    assert out == [1, 2, 3, "N/A"]


@pytest.mark.p1
def test_float_column_preserves_unconvertible_cell():
    out, ty = column_data_type(["1.5", "2.5", "3.5", "see note"])
    assert ty == "float"
    assert out == [1.5, 2.5, 3.5, "see note"]


@pytest.mark.p1
def test_bool_column_preserves_unknown_cell():
    # trans_bool returns None for unrecognized strings (it does not raise),
    # so the old code nulled these through the SUCCESS path.
    out, ty = column_data_type(["yes", "no", "yes", "unknown"])
    assert ty == "bool"
    # trans_bool returns the strings "yes"/"no", not Python bools.
    assert out == ["yes", "no", "yes", "unknown"]


@pytest.mark.p1
def test_datetime_column_preserves_unparseable_cell():
    # trans_datatime also returns None instead of raising.
    out, ty = column_data_type(["2025-01-01", "2025-02-01", "ongoing"])
    assert ty == "datetime"
    # trans_datatime returns normalized datetime STRINGS.
    assert out[0] == "2025-01-01 00:00:00"
    assert out[1] == "2025-02-01 00:00:00"
    assert out[2] == "ongoing"


@pytest.mark.p1
def test_fully_convertible_columns_are_unchanged():
    out, ty = column_data_type(["10", "20", "30"])
    assert ty == "int"
    assert out == [10, 20, 30]

    out, ty = column_data_type(["2025-01-01", "2025-02-01"])
    assert ty == "datetime"
    assert all(str(v).startswith("2025") for v in out)


@pytest.mark.p1
def test_nan_and_nat_still_normalize_to_none():
    import math

    out, ty = column_data_type(["1", "2", "3", None])
    assert ty == "int"
    assert out == [1, 2, 3, None]
