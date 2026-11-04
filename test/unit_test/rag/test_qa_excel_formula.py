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

"""The Q&A Excel chunker indexed formulas instead of the answers.

`rag/app/qa.py` opened the workbook with a bare `load_workbook(...)`, so a cell
holding a formula came back as its source text (`=CONCATENATE("The total is ",42)`)
rather than the value Excel had computed and stored next to it. A maintained FAQ
sheet, where answers are looked up or assembled from other columns, was indexed
as spreadsheet source code.

Every other Excel path in the repo goes through
`RAGFlowExcelParser._load_excel_to_workbook`, which passes `data_only=True`.
"""

import importlib.util
import re
import sys
import types
import zipfile
from io import BytesIO
from pathlib import Path
from unittest import mock

import pytest

_REPO_ROOT = Path(__file__).resolve().parents[3]


class _Dummy:
    def __init__(self, *a, **k):
        pass


def _stub(name, **attrs):
    mod = types.ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    sys.modules[name] = mod
    return mod


def _load(module_name, relative_path):
    spec = importlib.util.spec_from_file_location(module_name, _REPO_ROOT / relative_path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[module_name] = module
    spec.loader.exec_module(module)
    return module


@pytest.fixture(scope="module")
def qa_excel():
    """`rag.app.qa` with the vision / LLM / storage siblings stubbed out."""
    saved = {name: sys.modules.get(name) for name in list(sys.modules)}
    try:
        for name in ("xgboost", "pdfplumber", "huggingface_hub", "pypdf", "sklearn", "sklearn.cluster", "sklearn.metrics", "deepdoc.vision"):
            sys.modules.setdefault(name, mock.MagicMock())

        excel_parser = _load("deepdoc.parser.excel_parser_under_test", "deepdoc/parser/excel_parser.py")

        _stub("deepdoc.parser", ExcelParser=excel_parser.RAGFlowExcelParser, PdfParser=_Dummy, DocxParser=_Dummy)
        _stub("deepdoc.parser.utils", get_text=lambda *a, **k: "")

        yield _load("rag.app.qa_under_test", "rag/app/qa.py").Excel
    finally:
        for name in list(sys.modules):
            if name not in saved:
                del sys.modules[name]
        sys.modules.update(saved)


def _build_xlsx(rows, formula_cell=None, formula=None, cached=None):
    """A workbook, optionally with one formula cell that carries a cached value.

    openpyxl writes `<f>` but never a `<v>`; Excel always writes both, so the
    cached value is injected the way a real file has it.
    """
    from openpyxl import Workbook

    wb = Workbook()
    ws = wb.active
    for row in rows:
        ws.append(list(row))
    if formula_cell is not None:
        ws[formula_cell] = formula

    buf = BytesIO()
    wb.save(buf)
    raw = buf.getvalue()
    if formula_cell is None:
        return raw

    source = zipfile.ZipFile(BytesIO(raw))
    out = BytesIO()
    with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as target:
        for item in source.infolist():
            data = source.read(item.filename)
            if item.filename == "xl/worksheets/sheet1.xml":
                text = data.decode("utf-8")
                text = re.sub(rf'<c r="{formula_cell}"([^>]*)>', rf'<c r="{formula_cell}"\1 t="str">', text, count=1)
                text = text.replace("</f>", f"</f><v>{cached}</v>", 1)
                data = text.encode("utf-8")
            target.writestr(item, data)
    return out.getvalue()


@pytest.mark.p2
def test_computed_answer_is_read_not_its_formula(qa_excel):
    blob = _build_xlsx(
        [("What is the total?", None)],
        formula_cell="B1",
        formula='=CONCATENATE("The total is ",42)',
        cached="The total is 42",
    )

    pairs = qa_excel()("qa.xlsx", blob, callback=lambda *a, **k: None)

    assert pairs == [("What is the total?", "The total is 42")]


@pytest.mark.p2
def test_a_computed_question_is_read_too(qa_excel):
    blob = _build_xlsx(
        [(None, "Use the reset link.")],
        formula_cell="A1",
        formula='=CONCATENATE("How do I ","reset my password?")',
        cached="How do I reset my password?",
    )

    pairs = qa_excel()("qa.xlsx", blob, callback=lambda *a, **k: None)

    assert pairs == [("How do I reset my password?", "Use the reset link.")]


@pytest.mark.p2
def test_an_uncached_formula_reports_the_row_instead_of_answering_with_its_source(qa_excel):
    """A library-written workbook has the formula but no value Excel ever computed.

    openpyxl, xlsxwriter and `pandas.to_excel` write `<f>` without a `<v>`, so
    `data_only=True` yields None and the pair has no answer. Before this change
    the row was kept and the formula's source text was indexed as the answer;
    now the row is reported as a failure by line number, which is the same path
    any other unusable row takes. The alternative, falling back to the source
    text, is the defect this PR removes, so it is not a fallback worth having.
    """
    from openpyxl import Workbook, load_workbook

    wb = Workbook()
    wb.active["A1"] = "What is the total?"
    wb.active["B1"] = '=CONCATENATE("The total is ",42)'
    buf = BytesIO()
    wb.save(buf)
    blob = buf.getvalue()

    # the premise: openpyxl really did write a formula with no cached value
    assert load_workbook(BytesIO(blob), data_only=True).active["B1"].value is None
    assert load_workbook(BytesIO(blob), data_only=False).active["B1"].value == '=CONCATENATE("The total is ",42)'

    seen = []
    pairs = qa_excel()("qa.xlsx", blob, callback=lambda *a, **k: seen.append((a, k)))

    assert pairs == []
    reported = " ".join(str(part) for call in seen for part in call[0] + tuple(call[1].values()))
    assert "1 failure" in reported and "line: 1" in reported


@pytest.mark.p2
def test_plain_pairs_are_unchanged(qa_excel):
    """Guard: a sheet with no formula must come out exactly as before."""
    blob = _build_xlsx([("Question one", "Answer one"), ("Question two", "Answer two")])

    pairs = qa_excel()("qa.xlsx", blob, callback=lambda *a, **k: None)

    assert pairs == [("Question one", "Answer one"), ("Question two", "Answer two")]


@pytest.mark.p2
def test_reads_the_same_workbook_from_a_path(tmp_path, qa_excel):
    blob = _build_xlsx(
        [("What is the total?", None)],
        formula_cell="B1",
        formula='=CONCATENATE("The total is ",42)',
        cached="The total is 42",
    )
    path = tmp_path / "qa.xlsx"
    path.write_bytes(blob)

    pairs = qa_excel()(str(path), callback=lambda *a, **k: None)

    assert pairs == [("What is the total?", "The total is 42")]
