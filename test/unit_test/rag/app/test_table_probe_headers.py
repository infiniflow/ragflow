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

"""Pre-ingestion schema probe: the column names it reports must be the ones
ingestion produces (rag.app.table.probe_table_headers)."""

from __future__ import annotations

import sys
import warnings
from importlib import import_module, reload
from io import BytesIO
from unittest.mock import MagicMock

warnings.filterwarnings("ignore", message=".*pkg_resources is deprecated.*", category=UserWarning)
import pkg_resources  # noqa: F401 — stabilize xgboost import during collection
import pytest


@pytest.fixture(scope="module")
def table_module():
    """Load rag.app.table with heavy optional dependencies stubbed locally."""
    stub_names = [
        "deepdoc.vision.ocr",
        "deepdoc.parser.figure_parser",
        "rag.app.picture",
    ]
    original_modules = {name: sys.modules.get(name) for name in stub_names}

    try:
        for name in stub_names:
            sys.modules[name] = MagicMock()
        module = import_module("rag.app.table")
        module = reload(module)
        yield module
    finally:
        for name, original in original_modules.items():
            if original is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = original


def test_probe_csv_reports_ingestion_column_names(table_module):
    content = b"name,id,amount,name\nAlice,1,10,Bob\n"
    assert table_module.probe_table_headers(content, "data.csv") == [
        "name",
        "amount",
        "name_2",
    ]


def test_probe_csv_skips_leading_empty_rows_and_names_empty_headers(table_module):
    content = b",,\n,name,amount\n,1,10\n"
    assert table_module.probe_table_headers(content, "data.csv") == [
        "Column_1",
        "name",
        "amount",
    ]


def test_probe_tsv_uses_tab_delimiter(table_module):
    content = b"name\tamount\nAlice\t10\n"
    assert table_module.probe_table_headers(content, "data.tsv") == ["name", "amount"]


def test_probe_csv_strips_utf8_bom(table_module):
    content = b"\xef\xbb\xbfname,amount\nAlice,10\n"
    assert table_module.probe_table_headers(content, "data.csv") == ["name", "amount"]


def test_probe_xlsx_uses_first_sheet_header_row(table_module):
    from openpyxl import Workbook

    wb = Workbook()
    ws = wb.active
    ws.append([None, None])
    ws.append(["id", "amount", "name"])
    ws.append([1, 10, "Alice"])
    buffer = BytesIO()
    wb.save(buffer)

    assert table_module.probe_table_headers(buffer.getvalue(), "data.xlsx") == [
        "amount",
        "name",
    ]


def test_probe_rejects_binary_xls(table_module):
    with pytest.raises(ValueError):
        table_module.probe_table_headers(b"\xd0\xcf\x11\xe0binary", "legacy.xls")


def test_table_column_header_names_matches_the_parser_rules(table_module):
    # Bookkeeping columns dropped, duplicates suffixed, empty header named by
    # position — the same rules the Go parser applies.
    assert table_module.table_column_header_names(["idx", "a", "", "a", "id"]) == [
        "a",
        "Column_3",
        "a_2",
    ]
