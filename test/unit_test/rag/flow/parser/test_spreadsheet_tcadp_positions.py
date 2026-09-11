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
import ast
from pathlib import Path
from unittest.mock import Mock

import pytest

from rag.flow.parser.spreadsheet_positions import (
    spreadsheet_positions_from_tcadp_table,
    spreadsheet_positions_from_tcadp_tag,
    tcadp_spreadsheet_json_items,
)

REPO_ROOT = Path(__file__).resolve().parents[5]

_TABLE_HTML = "<table>\n  <tr>\n    <th>Name</th>\n    <th>Qty</th>\n  </tr>\n  <tr>\n    <td>A</td>\n    <td>1</td>\n  </tr>\n  <tr>\n    <td>B</td>\n    <td>2</td>\n  </tr>\n</table>"

_MOCK_SECTIONS = [
    ("Intro text", "@@2\t3\t5\t1\t4##"),
    ("Dummy tagged", "@@1\t0.0\t1000.0\t0.0\t100.0##"),
    ("Untagged", ""),
]
_MOCK_TABLES = [_TABLE_HTML, "<table><tr><td>only</td></tr></table>"]


def _assert_positions(item):
    assert "positions" in item, f"missing positions on {item!r}"
    positions = item["positions"]
    assert isinstance(positions, list) and positions, f"empty positions on {item!r}"
    coord = positions[0]
    assert len(coord) == 5, f"expected 5-tuple positions, got {coord!r}"
    assert all(isinstance(n, int) for n in coord), f"non-int positions {coord!r}"
    assert coord[0] >= 0


@pytest.mark.p1
def test_spreadsheet_positions_from_tcadp_tag():
    assert spreadsheet_positions_from_tcadp_tag("@@2\t3\t5\t1\t4##") == [[1, 3, 5, 1, 4]]
    assert spreadsheet_positions_from_tcadp_tag("@@1\t0.0\t1000.0\t0.0\t100.0##") == [[0, 0, 1000, 0, 100]]
    assert spreadsheet_positions_from_tcadp_tag("@@1-3\t2\t4\t1\t2##") == [[0, 2, 4, 1, 2]]
    assert spreadsheet_positions_from_tcadp_tag("") == [[0, 1, 1, 1, 1]]
    assert spreadsheet_positions_from_tcadp_tag("not-a-tag") == [[0, 1, 1, 1, 1]]


@pytest.mark.p1
def test_spreadsheet_positions_from_tcadp_table_uses_html_span():
    assert spreadsheet_positions_from_tcadp_table(0, _TABLE_HTML) == [[0, 1, 3, 1, 2]]
    assert spreadsheet_positions_from_tcadp_table(1, "not html") == [[1, 1, 1, 1, 1]]


@pytest.mark.p1
def test_tcadp_spreadsheet_json_items_include_positions():
    tcadp = Mock()
    tcadp.parse_pdf.return_value = (_MOCK_SECTIONS, _MOCK_TABLES)
    sections, tables = tcadp.parse_pdf(filepath="book.xlsx", binary=b"xlsx", callback=None, file_type="XLSX")

    items = tcadp_spreadsheet_json_items(sections, tables)

    assert len(items) == 5
    for item in items:
        _assert_positions(item)

    assert items[0]["text"] == "Intro text"
    assert items[0]["doc_type_kwd"] == "text"
    assert items[0]["positions"] == [[1, 3, 5, 1, 4]]

    assert items[1]["positions"] == [[0, 0, 1000, 0, 100]]
    assert items[2]["positions"] == [[0, 1, 1, 1, 1]]

    assert items[3]["doc_type_kwd"] == "table"
    assert items[3]["positions"] == [[0, 1, 3, 1, 2]]
    assert items[4]["positions"] == [[1, 1, 1, 1, 1]]

    # add_positions (task_executor / dataflow_service) stores pn+1.
    sheet, r1, r2, c1, c2 = items[0]["positions"][0]
    assert (sheet + 1, r1, r2, c1, c2) == (2, 3, 5, 1, 4)


@pytest.mark.p1
def test_tcadp_spreadsheet_flatten_media_still_keeps_positions():
    items = tcadp_spreadsheet_json_items(
        [("note", "@@1\t2\t2\t1\t1##")],
        [_TABLE_HTML],
        flatten_media_to_text=True,
    )
    assert [item["doc_type_kwd"] for item in items] == ["text", "text"]
    for item in items:
        _assert_positions(item)
    assert items[1]["positions"] == [[0, 1, 3, 1, 2]]


@pytest.mark.p1
def test_parser_spreadsheet_json_branch_calls_tcadp_spreadsheet_json_items():
    tree = ast.parse((REPO_ROOT / "rag/flow/parser/parser.py").read_text())
    spreadsheet = None
    for node in tree.body:
        if isinstance(node, ast.ClassDef) and node.name == "Parser":
            for item in node.body:
                if isinstance(item, ast.FunctionDef) and item.name == "_spreadsheet":
                    spreadsheet = item
                    break
    assert spreadsheet is not None

    calls = [node for node in ast.walk(spreadsheet) if isinstance(node, ast.Call) and isinstance(node.func, ast.Name) and node.func.id == "tcadp_spreadsheet_json_items"]
    assert len(calls) == 1, "Parser._spreadsheet JSON path must attach positions via tcadp_spreadsheet_json_items"
