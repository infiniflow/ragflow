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

import importlib.util
import os
import sys
from io import BytesIO
from unittest import mock

import pytest

# Import RAGFlowExcelParser directly by file path to avoid triggering
# deepdoc/parser/__init__.py and rag.nlp, which pull in heavy dependencies.
for _m in ["pandas", "rag.nlp", "rag.utils", "rag.utils.lazy_image"]:
    if _m not in sys.modules:
        sys.modules[_m] = mock.MagicMock()


def _find_project_root(marker="pyproject.toml"):
    d = os.path.dirname(os.path.abspath(__file__))
    while d != os.path.dirname(d):
        if os.path.exists(os.path.join(d, marker)):
            return d
        d = os.path.dirname(d)
    return None


_PROJECT_ROOT = _find_project_root()
_spec = importlib.util.spec_from_file_location(
    "deepdoc.parser.excel_parser",
    os.path.join(_PROJECT_ROOT, "deepdoc", "parser", "excel_parser.py"),
)
_mod = importlib.util.module_from_spec(_spec)
sys.modules["deepdoc.parser.excel_parser"] = _mod
_spec.loader.exec_module(_mod)

RAGFlowExcelParser = _mod.RAGFlowExcelParser


def _make_xlsx(n_data_rows):
    from openpyxl import Workbook

    wb = Workbook()
    ws = wb.active
    ws.append(["H1", "H2"])
    for i in range(n_data_rows):
        ws.append([f"a{i}", f"b{i}"])
    buf = BytesIO()
    wb.save(buf)
    buf.seek(0)
    return buf.read()


def _chunk_has_no_data_cells(chunk):
    return "<td>" not in chunk and "<td></td>" not in chunk


@pytest.mark.p2
def test_exact_multiple_does_not_emit_header_only_chunk():
    # 12 data rows with chunk_rows=12 (the value rag/app/naive.py uses).
    chunks = RAGFlowExcelParser().html(_make_xlsx(12), chunk_rows=12)
    assert len(chunks) == 1
    assert all(not _chunk_has_no_data_cells(c) for c in chunks)


@pytest.mark.p2
def test_multiple_of_chunk_rows_splits_without_spurious_chunk():
    # 24 data rows with chunk_rows=12 -> exactly 2 data chunks, no trailing header-only chunk.
    chunks = RAGFlowExcelParser().html(_make_xlsx(24), chunk_rows=12)
    assert len(chunks) == 2
    assert all(not _chunk_has_no_data_cells(c) for c in chunks)


@pytest.mark.p2
def test_non_multiple_unchanged():
    # 13 data rows with chunk_rows=12 -> 2 chunks (12 + 1).
    chunks = RAGFlowExcelParser().html(_make_xlsx(13), chunk_rows=12)
    assert len(chunks) == 2
    assert all(not _chunk_has_no_data_cells(c) for c in chunks)


class _FakeDataFrame:
    """Minimal duck-typed stand-in for ``pandas.DataFrame``.

    Only exposes what ``_dataframe_to_workbook`` touches: ``columns``,
    ``values`` and ``apply`` (used by ``_clean_dataframe``).
    """

    def __init__(self, columns, rows):
        self.columns = list(columns)
        self.values = [tuple(r) for r in rows]

    def apply(self, func):
        # _clean_dataframe maps a string cleaner over cells; for these plain
        # numeric/string values it is effectively a no-op.
        return self


@pytest.mark.p2
def test_single_sheet_dict_builds_workbook():
    # pandas.read_excel(sheet_name=None) always returns a dict keyed by sheet
    # name, even when the file has a single sheet. The workbook builder must
    # accept that single-entry dict instead of feeding it to _clean_dataframe.
    df = _FakeDataFrame(["a", "b"], [(1, 2), (3, 4)])
    wb = RAGFlowExcelParser._dataframe_to_workbook({"Sheet1": df})
    assert "Sheet1" in wb.sheetnames
    ws = wb["Sheet1"]
    assert ws.cell(row=1, column=1).value == "a"
    assert ws.cell(row=1, column=2).value == "b"
    assert ws.cell(row=2, column=1).value == 1
    assert ws.cell(row=3, column=2).value == 4


@pytest.mark.p2
def test_multi_sheet_dict_builds_workbook():
    dfs = {
        "S1": _FakeDataFrame(["a"], [(1,)]),
        "S2": _FakeDataFrame(["b"], [(2,)]),
    }
    wb = RAGFlowExcelParser._dataframe_to_workbook(dfs)
    assert wb.sheetnames == ["S1", "S2"]
