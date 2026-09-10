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
"""Spreadsheet parsing must hand the chunker typed, self-contained tables.

Regression guard for "TokenChunker shreds xlsx tables": the spreadsheet parser
used to emit one *text* item per row (or a single raw HTML string under
``output_format=html``), so the TokenChunker cut tables in the middle of
``<td>`` text and dropped the delimiter characters it cut on.

The parser now emits one ``doc_type_kwd="table"`` item per sheet chunk (Go's
``defaultTableChunkRows`` = 256), each carrying its own caption and header row,
and the TokenChunker keeps those items whole.
"""

import importlib
import io
import sys
from types import SimpleNamespace
from unittest.mock import Mock

import pytest
from openpyxl import Workbook

DEFAULT_SETUP = {
    "parse_method": "deepdoc",
    "flatten_media_to_text": False,
    "output_format": "json",
    "suffix": ["xls", "xlsx", "csv"],
}


@pytest.fixture
def flow_modules(monkeypatch):
    """Import the flow modules the way the sibling parser test does.

    The component registry imports ``deepdoc.parser.pdf_parser`` first and
    leaves it half-initialized, so importing ``rag.flow.parser.parser``
    directly raises ``cannot import name 'PlainParser'``. Drop the stale
    entries and re-import ``deepdoc.parser.pdf_parser`` first.
    """
    for module_name in (
        "rag.flow.parser.parser",
        "rag.flow.chunker.token_chunker",
        "deepdoc.parser.pdf_parser",
        "deepdoc.parser",
        "deepdoc",
    ):
        monkeypatch.delitem(sys.modules, module_name, raising=False)
    importlib.invalidate_caches()
    importlib.import_module("deepdoc.parser.pdf_parser")
    return SimpleNamespace(
        parser=importlib.import_module("rag.flow.parser.parser"),
        chunker=importlib.import_module("rag.flow.chunker.token_chunker"),
        delim=importlib.import_module("rag.nlp.delim"),
    )


def build_xlsx(sheets: dict[str, list[list]], pad_rows: int = 0) -> bytes:
    """Build an xlsx with one sheet per entry, padded with ``pad_rows`` data rows."""
    wb = Workbook()
    wb.remove(wb.active)
    for name, rows in sheets.items():
        ws = wb.create_sheet(name)
        for row in rows:
            ws.append(row)
        for i in range(pad_rows):
            ws.append([f"数据{i}", f"部门{i % 3}", f"备注{i}；说明。"])
    buf = io.BytesIO()
    wb.save(buf)
    return buf.getvalue()


def parse(flow_modules, blob: bytes, name: str = "sheet.xlsx", **overrides) -> dict:
    """Run ``Parser._spreadsheet`` against a minimal process double."""
    setup = {**DEFAULT_SETUP, **overrides}
    process = SimpleNamespace(
        _param=SimpleNamespace(setups={"spreadsheet": setup}),
        _canvas=SimpleNamespace(_doc_id=None, _tenant_id="tenant", _language="English"),
        callback=Mock(),
        outputs={},
        set_output=lambda k, v: process.outputs.__setitem__(k, v),
    )
    flow_modules.parser.Parser._spreadsheet(process, name, blob)
    return process.outputs


def test_spreadsheet_default_output_format_is_json(flow_modules):
    # The default must be the structured format; "html" flattens a table into a
    # raw string the chunker can only cut on character boundaries.
    assert flow_modules.parser.ParserParam().setups["spreadsheet"]["output_format"] == "json"


def test_spreadsheet_emits_one_typed_table_item_per_sheet(flow_modules):
    blob = build_xlsx({"SheetA": [["姓名", "部门"], ["张三", "研发"]], "SheetB": [["产品", "数量"], ["A", 1]]})
    items = parse(flow_modules, blob)["json"]

    assert [i["doc_type_kwd"] for i in items] == ["table", "table"]
    # Every sheet is emitted; the old html path silently kept only the first.
    assert [i["positions"][0][0] for i in items] == [0, 1]
    for item in items:
        text = item["text"].strip()
        assert text.startswith("<table>") and text.endswith("</table>")
        assert "<caption>" in text


def test_spreadsheet_table_chunk_is_self_contained(flow_modules):
    # A chunk is retrieved and sent to the LLM on its own, so it must carry the
    # header row that gives the columns their meaning.
    blob = build_xlsx({"Big": [["列1", "列2", "列3"]]}, pad_rows=600)
    items = parse(flow_modules, blob)["json"]

    assert len(items) == 3  # 600 data rows / 256 rows per chunk
    for item in items:
        assert item["text"].count("<th>") == 3
        assert item["doc_type_kwd"] == "table"


def test_spreadsheet_flatten_media_to_text_marks_tables_as_text(flow_modules):
    blob = build_xlsx({"S": [["a", "b"], ["1", "2"]]})
    items = parse(flow_modules, blob, flatten_media_to_text=True)["json"]

    assert [i["doc_type_kwd"] for i in items] == ["text"]


@pytest.mark.parametrize(("suffix", "blob"), [("xlsx", None), ("csv", "姓名,部门\n张三,研发\n".encode())])
def test_spreadsheet_supported_suffixes_produce_tables(flow_modules, suffix, blob):
    if blob is None:
        blob = build_xlsx({"S": [["姓名", "部门"], ["张三", "研发"]]})
    items = parse(flow_modules, blob, name=f"sheet.{suffix}")["json"]

    assert items and items[0]["doc_type_kwd"] == "table"


async def test_token_chunker_keeps_spreadsheet_table_whole(flow_modules):
    """The reported bug: delimiter chunking must not cut inside a table."""
    blob = build_xlsx(
        {
            "Sheet1": [
                ["问题", "回答"],
                # Cells carrying the default delimiter chars and embedded newlines.
                ["碳排放纳入名录是否公布？", "需要公布。根据《管理办法》\n由省级主管部门确定。"],
                ["关停企业如何报告？", "向实际所在地报告；不能独立报告的随上级法人。"],
            ]
        }
    )
    parsed = parse(flow_modules, blob)["json"]

    param = flow_modules.chunker.TokenChunkerParam()  # current upstream default delimiters
    assert param.delimiters == list(flow_modules.delim.DEFAULT_DELIMITER)
    chunker = flow_modules.chunker.TokenChunker.__new__(flow_modules.chunker.TokenChunker)
    chunker._canvas = SimpleNamespace(_doc_id=None, _tenant_id="tenant")
    chunker._param = param
    chunker.callback = Mock()

    await chunker._invoke(name="sheet.xlsx", output_format="json", json_result=parsed)
    chunks = [c["text"] for c in chunker._param.outputs["chunks"]["value"]]

    assert len(chunks) == 1
    text = chunks[0].strip()
    assert text.startswith("<table>") and text.endswith("</table>")
    # No delimiter character may be eaten by the chunker.
    assert "。" in text and "？" in text and "；" in text
