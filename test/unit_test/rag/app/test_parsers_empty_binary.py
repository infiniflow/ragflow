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

"""A 0-byte upload reaches the chunkers as b"", not None.

`rag/svr/task_executor.py` guards only `binary is None`, then calls
`chunker.chunk(task["name"], binary=binary, ...)`. `task["name"]` is a display
name, so any branch that falls back to opening `filename` looks for a file that
does not exist. Every site below must therefore branch on `binary is None`, not
on its truthiness.

Follows #17904 (`html_parser.py`), #18712 (`email.py`, `presentation.py`) and
#18826 (`naive.py`).
"""

from __future__ import annotations

import importlib
from zipfile import BadZipFile

import pytest

# Display names, not paths on disk.
PDF_NAME = "report.pdf"
DOCX_NAME = "report.docx"
XLSX_NAME = "report.xlsx"

PDF_MODULES = ["book", "laws", "manual", "one", "paper", "qa"]
PDF_CHUNK_MODULES = ["paper", "qa"]
DOCX_MODULES = ["laws", "manual", "qa"]


class _Reached(Exception):
    """Raised from the stubbed OCR entry point to stop the parser right there."""


def _load(module_name: str):
    """Import a `rag.app` module lazily, inside the test that needs it.

    Keeps this file independent of whether any other test module already
    imported `rag.app.*` at collection time, so it behaves the same standalone
    as it does in a directory run.
    """
    return importlib.import_module(f"rag.app.{module_name}")


def _noop_callback(*args, **kwargs):
    return None


@pytest.fixture(autouse=True)
def _stub_rag_tokenizer(monkeypatch):
    def fake_tokenize(text):
        return str(text)

    monkeypatch.setattr("rag.nlp.rag_tokenizer.tokenize", fake_tokenize)
    monkeypatch.setattr("rag.nlp.rag_tokenizer.fine_grained_tokenize", fake_tokenize)


@pytest.fixture
def captured_ocr_source(monkeypatch):
    """Capture what `Pdf.__call__` hands to `__images__`, then abort the parse.

    `__images__` is the first thing every `Pdf.__call__` does with the chosen
    source, so it is exactly where the branch decision becomes observable.
    `PdfParser.__init__` is stubbed out too: it loads the OCR and layout models
    from the Hub, which this test neither needs nor should depend on.
    """
    from deepdoc.parser import PdfParser

    captured: list = []

    def capture(self, fnm, *args, **kwargs):
        captured.append(fnm)
        raise _Reached

    monkeypatch.setattr(PdfParser, "__init__", lambda self, *args, **kwargs: None)
    monkeypatch.setattr(PdfParser, "__images__", capture)
    return captured


@pytest.mark.p2
@pytest.mark.parametrize("module_name", PDF_MODULES)
def test_pdf_empty_binary_reaches_ocr_with_bytes(module_name, captured_ocr_source):
    """A 0-byte PDF must reach OCR as b"", not as the display name."""
    module = _load(module_name)

    with pytest.raises(_Reached):
        module.Pdf()(PDF_NAME, b"", callback=_noop_callback)

    assert captured_ocr_source == [b""]


@pytest.mark.p2
@pytest.mark.parametrize("module_name", PDF_CHUNK_MODULES)
def test_pdf_chunk_empty_binary_reaches_ocr_with_bytes(module_name, captured_ocr_source):
    """Same guarantee through `chunk()`, which selects the source itself before
    handing it to `Pdf.__call__`."""
    module = _load(module_name)

    with pytest.raises(_Reached):
        module.chunk(PDF_NAME, binary=b"", callback=_noop_callback)

    assert captured_ocr_source == [b""]


@pytest.mark.p2
@pytest.mark.parametrize("module_name", DOCX_MODULES)
def test_docx_empty_binary_does_not_open_display_name(module_name):
    """python-docx must receive the empty bytes, so it reports a bad zip
    instead of a package missing under the display name."""
    module = _load(module_name)

    with pytest.raises(BadZipFile, match="File is not a zip file"):
        module.Docx()(DOCX_NAME, b"")


@pytest.mark.p2
def test_qa_excel_empty_binary_loads_from_bytes():
    """openpyxl must receive the empty bytes, so the failure describes the
    payload rather than a file missing under the display name."""
    qa = _load("qa")

    with pytest.raises(BadZipFile, match="File is not a zip file"):
        qa.Excel()(XLSX_NAME, b"", callback=_noop_callback)


@pytest.mark.p2
def test_table_excel_empty_binary_loads_from_bytes():
    """`_load_excel_to_workbook` sniffs the header and falls back to its CSV
    path for anything that is not a zip/OLE container. Reaching that failure
    proves it was handed the bytes and not the display name."""
    table = _load("table")

    with pytest.raises(Exception, match="Failed to parse CSV and convert to Excel Workbook"):
        table.Excel()(XLSX_NAME, b"", callback=_noop_callback)
