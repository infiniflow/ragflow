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

"""A .csv file is not always comma separated.

Excel writes the list separator of the machine's locale -- a semicolon across
most of Europe -- and a tab separated export is routinely saved as .csv. Both
`pandas.read_csv` and `csv.reader` default to a comma, and reading such a file
with one does not fail: every row becomes a single column holding the whole
line, separators included.
"""

import importlib.util
import sys
from io import BytesIO
from pathlib import Path
from unittest import mock

import pytest

_REPO_ROOT = Path(__file__).resolve().parents[4]

_MOCK_MODULES = [
    "xgboost",
    "pdfplumber",
    "huggingface_hub",
    "pypdf",
    "sklearn",
    "sklearn.cluster",
    "sklearn.metrics",
    "deepdoc.vision",
]
for _name in _MOCK_MODULES:
    sys.modules.setdefault(_name, mock.MagicMock())


def _load_excel_parser():
    """Load excel_parser.py by path, so deepdoc/parser/__init__.py stays out of it."""
    spec = importlib.util.spec_from_file_location(
        "deepdoc.parser.excel_parser_under_test",
        _REPO_ROOT / "deepdoc" / "parser" / "excel_parser.py",
    )
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


excel_parser = _load_excel_parser()
detect_csv_delimiter = excel_parser.detect_csv_delimiter
RAGFlowExcelParser = excel_parser.RAGFlowExcelParser

ROWS = ["Name;Region;Units", "Widget;EU;12", "Gadget;US;7"]


def _text(delimiter):
    return "\n".join(row.replace(";", delimiter) for row in ROWS) + "\n"


@pytest.mark.p2
@pytest.mark.parametrize("delimiter", [",", ";", "\t", "|"])
def test_detect_csv_delimiter(delimiter):
    assert detect_csv_delimiter(_text(delimiter)) == delimiter


@pytest.mark.p2
@pytest.mark.parametrize(
    "text",
    [
        # A separator inside a quoted field is not a separator.
        'Name,Note\nWidget,"a; b"\nGadget,"c; d"\n',
        # Rows that do not line up leave the default in place.
        "Note\na; b\nc; d\n",
        "name,value\nAlice,1\nBob,2,extra\n",
        "",
    ],
)
def test_detect_csv_delimiter_falls_back_to_a_comma(text):
    assert detect_csv_delimiter(text) == ","


@pytest.mark.p2
@pytest.mark.parametrize("delimiter", [",", ";", "\t", "|"])
def test_a_csv_keeps_its_columns_whatever_the_separator(delimiter):
    """`_read_csv` is the path a .csv upload takes through the Excel parser."""
    workbook = RAGFlowExcelParser._read_csv(BytesIO(_text(delimiter).encode("utf-8")))

    assert list(workbook.columns) == ["Name", "Region", "Units"]
    assert workbook.iloc[0].tolist() == ["Widget", "EU", 12]


@pytest.mark.p2
def test_a_single_column_csv_stays_a_single_column():
    """Guard: a separator that does not line up across the rows is not one."""
    workbook = RAGFlowExcelParser._read_csv(BytesIO(b"Note\na; b\nc; d\n"))

    assert list(workbook.columns) == ["Note"]
    assert workbook["Note"].tolist() == ["a; b", "c; d"]
