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
    assert all(not _chunk_has_no_data_cells(c[0]) for c in chunks)
    assert chunks[0][1][0] == 0 and chunks[0][1][1] == 2 and chunks[0][1][2] == 13


@pytest.mark.p2
def test_multiple_of_chunk_rows_splits_without_spurious_chunk():
    # 24 data rows with chunk_rows=12 -> exactly 2 data chunks, no trailing header-only chunk.
    chunks = RAGFlowExcelParser().html(_make_xlsx(24), chunk_rows=12)
    assert len(chunks) == 2
    assert all(not _chunk_has_no_data_cells(c[0]) for c in chunks)
    assert chunks[0][1][1] == 2 and chunks[0][1][2] == 13
    assert chunks[1][1][1] == 14 and chunks[1][1][2] == 25


@pytest.mark.p2
def test_html_col_max_uses_widest_row():
    from openpyxl import Workbook

    wb = Workbook()
    ws = wb.active
    ws.append(["H1"])
    ws.append(["a", "b", "c"])
    buf = BytesIO()
    wb.save(buf)
    buf.seek(0)
    chunks = RAGFlowExcelParser().html(buf.read(), chunk_rows=12)
    assert chunks[0][1][3] == 1 and chunks[0][1][4] == 3


@pytest.mark.p2
def test_non_multiple_unchanged():
    # 13 data rows with chunk_rows=12 -> 2 chunks (12 + 1).
    chunks = RAGFlowExcelParser().html(_make_xlsx(13), chunk_rows=12)
    assert len(chunks) == 2
    assert all(not _chunk_has_no_data_cells(c[0]) for c in chunks)


def _make_xlsx_with_values(header, row):
    from openpyxl import Workbook

    wb = Workbook()
    ws = wb.active
    ws.append(header)
    ws.append(row)
    buf = BytesIO()
    wb.save(buf)
    buf.seek(0)
    return buf.read()


@pytest.mark.p2
def test_call_keeps_zero_valued_cells():
    # __call__ produces the text used for indexing. A numeric 0 (and 0.0 / False)
    # is real data, not an empty cell, so it must survive. The header is only
    # emitted alongside a kept value, so a dropped 0 also loses its "stock" label.
    lines = RAGFlowExcelParser()(_make_xlsx_with_values(["name", "stock"], ["widget", 0]))
    joined = " ".join(text for text, _ in lines)
    assert "stock" in joined and "0" in joined, lines
    assert lines[0][1][1] == 2


@pytest.mark.p2
def test_call_skips_truly_empty_cells():
    # None / empty-string cells carry no value and should still be skipped.
    lines = RAGFlowExcelParser()(_make_xlsx_with_values(["name", "note"], ["widget", None]))
    joined = " ".join(text for text, _ in lines)
    assert "note" not in joined, lines


def _make_two_sheet_xlsx():
    from openpyxl import Workbook

    wb = Workbook()
    ws1 = wb.active
    ws1.title = "s1"
    ws1.append(["H"])
    ws1.append(["a"])
    ws2 = wb.create_sheet("s2")
    ws2.append(["H"])
    ws2.append(["b"])
    buf = BytesIO()
    wb.save(buf)
    buf.seek(0)
    return buf.read()


@pytest.mark.p2
def test_call_emits_zero_based_sheet_index():
    # Flow JSON stores this index unchanged; add_positions later does pn+1 so
    # sheet 2 is stored as 2 and the preview selects xs.datas[1].
    lines = RAGFlowExcelParser()(_make_two_sheet_xlsx())
    sheets = {pos[0] for _, pos in lines}
    assert sheets == {0, 1}
    second = next(pos for _, pos in lines if pos[0] == 1)
    assert second == (1, 2, 2, 1, 1)


# Regression for #19185: a spreadsheet whose used range exceeds 10,000 rows
# and contains a blank run in the middle used to lose every row past the
# first 500 of the second data region. ``_get_actual_row_count`` did a
# binary search for the first data window, then capped the forward
# extension at ``last_data_row + 500``, so a 600-row blank run between
# two data regions returned 200 instead of 1000.
#
# The fix removes the 500-row cap; the forward scan now runs all the way
# to ``max_row``. The fast path below 10,000 rows (``if max_row <= 10000:
# return max_row``) is unchanged, so a plain 1000-row sheet still returns
# 1000 with no scan.
def _make_two_region_xlsx(used_max, gap_start, gap_len, last_data_row):
    """Top data block: rows 1..gap_start. Gap: rows gap_start+1..
    gap_start+gap_len. Bottom data block: rows gap_start+gap_len+1..
    last_data_row. ``used_max`` is unused in the data; openpyxl's
    ``max_row`` will reflect ``last_data_row`` because that's the
    highest row we touch.
    """
    from openpyxl import Workbook

    wb = Workbook()
    ws = wb.active
    for r in range(1, gap_start + 1):
        ws.cell(row=r, column=1, value=f"top{r}")
    for r in range(gap_start + gap_len + 1, last_data_row + 1):
        ws.cell(row=r, column=1, value=f"bot{r}")
    buf = BytesIO()
    wb.save(buf)
    buf.seek(0)
    return buf.read(), last_data_row


@pytest.mark.p2
def test_row_number_skips_large_blank_run_in_big_sheet():
    # Reporter's reproducer shape: 200 rows at the top, a 600-row blank
    # run, 200 more rows of data, ``max_row`` in the high thousands. The
    # pre-fix code returned 201 (200 + the binary-search probe row)
    # because the forward extension was capped at 500.
    binary, expected = _make_two_region_xlsx(used_max=None, gap_start=200, gap_len=600, last_data_row=1000)
    assert RAGFlowExcelParser.row_number("fixture.xlsx", binary) == expected


@pytest.mark.p2
def test_row_number_returns_full_data_count_when_gap_under_500():
    # Sanity: a small gap (under the pre-fix 500 cap) was already correct.
    binary, expected = _make_two_region_xlsx(used_max=None, gap_start=200, gap_len=100, last_data_row=600)
    assert RAGFlowExcelParser.row_number("fixture.xlsx", binary) == expected


@pytest.mark.p2
def test_row_number_returns_max_row_directly_for_small_sheets():
    # The fast path (``max_row <= 10000: return max_row``) is unchanged
    # for a plain 400-row sheet with no gap. Build a one-region sheet.
    from openpyxl import Workbook

    wb = Workbook()
    ws = wb.active
    for r in range(1, 401):
        ws.cell(row=r, column=1, value=f"r{r}")
    buf = BytesIO()
    wb.save(buf)
    buf.seek(0)
    assert RAGFlowExcelParser.row_number("fixture.xlsx", buf.read()) == 400
