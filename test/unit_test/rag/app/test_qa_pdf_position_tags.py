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

"""Drive rag.app.qa.Pdf.__call__ through real position-tag control flow.

OCR, layout, and table detection are mocked. Position tags come from the
loaded RAGFlowPdfParser._line_tag / extract_positions / crop path, including
split adjacent tags and empty tags from invalid geometry.
"""

from __future__ import annotations

import ast
import importlib.util
import inspect
import re
import sys
import types
from pathlib import Path

import pytest
from PIL import Image

from test.unit_test.deepdoc.parser.test_pdf_parser_crop import _load_pdf_parser


def _repo_root():
    return Path(__file__).resolve().parents[4]


def _load_qa(monkeypatch, parser_module):
    repo_root = _repo_root()
    # qa.py imports docx/markdown for other parsers. Pdf.__call__ does not use them,
    # and this unit test must not require those packages to be installed.
    if "docx" not in sys.modules:
        monkeypatch.setitem(sys.modules, "docx", types.SimpleNamespace(Document=object))
    if "markdown" not in sys.modules:
        monkeypatch.setitem(sys.modules, "markdown", types.SimpleNamespace(markdown=lambda text, **_kwargs: text))
    nlp = types.ModuleType("rag.nlp")
    # Bullet/layout inference is not under test. Controlled matches isolate
    # the real QA state machine, tag reader and crop path.
    nlp.has_qbullet = lambda _reg, box, _last_box, last_index, _last_bull, _xs: (re.match(r"^\d+\.\s*", box["text"]), last_index)
    nlp.qbullets_category = lambda _sections: (0, r"^\d+\.\s*")
    nlp.rag_tokenizer = types.SimpleNamespace(tokenize=lambda text: text)
    for name in ("add_positions", "concat_img", "docx_question_level", "is_english", "random_choices", "tokenize_table"):
        setattr(nlp, name, lambda *_args, **_kwargs: None)
    monkeypatch.setitem(sys.modules, "rag.nlp", nlp)
    parser_pkg = types.ModuleType("deepdoc.parser")
    parser_pkg.__path__ = [str(repo_root / "deepdoc" / "parser")]
    parser_pkg.DocxParser = type("DocxParser", (), {})
    parser_pkg.ExcelParser = type("ExcelParser", (), {})
    parser_pkg.PdfParser = parser_module.RAGFlowPdfParser
    monkeypatch.setitem(sys.modules, "deepdoc.parser", parser_pkg)
    monkeypatch.setitem(sys.modules, "deepdoc.parser.utils", types.SimpleNamespace(get_text=lambda *_args, **_kwargs: ""))

    module_name = "test_qa_pdf_position_tags_module"
    module_path = repo_root / "rag" / "app" / "qa.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


def _page(width, height, color):
    return Image.new("RGB", (width, height), color)


def _box(text, page_number, x0, x1, top, bottom, layout_type="title"):
    return {
        "text": text,
        "page_number": page_number,
        "x0": x0,
        "x1": x1,
        "top": top,
        "bottom": bottom,
        "layout_type": layout_type,
    }


def _table(text, page_index, left, right, top, bottom):
    return (
        (None, [text]),
        [[page_index, left, right, top, bottom]],
    )


def _install_pages(parser, pages, page_from, heights):
    parser.page_images = pages
    parser.page_from = page_from
    parser.page_cum_height = [0]
    for height in heights:
        parser.page_cum_height.append(parser.page_cum_height[-1] + height)


class _PreparedPdf:
    def __init__(self, parser_cls, boxes, tables, pages, page_from=0):
        self.parser = parser_cls.__new__(parser_cls)
        self.boxes = boxes
        self.tables = tables
        self.pages = pages
        self.page_from = page_from
        self.calls = []

    def __call__(self, zoomin=1, from_page=None):
        parser = self.parser

        def images(_filename, zoom, page_from, _page_to, _callback):
            self.calls.append(("images", zoom, page_from))
            _install_pages(parser, self.pages, page_from, [page.height / zoom for page in self.pages])

        def layouts(_zoom, drop=False):
            self.calls.append(("layouts", drop))

        def tables(_zoom):
            self.calls.append(("tables",))

        def text_merge():
            self.calls.append(("text_merge",))
            parser.boxes = list(self.boxes)

        def extract_table_figure(*_args):
            self.calls.append(("extract",))
            return list(self.tables)

        parser.__images__ = images
        parser._layouts_rec = layouts
        parser._table_transformer_job = tables
        parser._text_merge = text_merge
        parser._extract_table_figure = extract_table_figure
        original_table_info = parser.get_tbls_info
        table_reads = 0

        def bounded_table_info(items, index):
            nonlocal table_reads
            table_reads += 1
            # Turn a regression to the legacy sentinel loop into a bounded
            # assertion failure rather than hanging the test suite.
            assert table_reads <= len(self.boxes) + len(self.tables) + 20, "table cursor did not terminate"
            return original_table_info(items, index)

        parser.get_tbls_info = bounded_table_info
        return parser(
            "qa.pdf",
            from_page=self.page_from if from_page is None else from_page,
            zoomin=zoomin,
            callback=lambda *_args, **_kwargs: None,
        )


@pytest.fixture
def qa_module(monkeypatch):
    return _load_qa(monkeypatch, _load_pdf_parser(monkeypatch))


def _preamble():
    return _box("cover note", 1, 10, 40, 0, 6, layout_type="text")


def test_loaded_pdf_methods_are_the_file_source(qa_module):
    source = (_repo_root() / "rag" / "app" / "qa.py").read_text(encoding="utf-8")
    tree = ast.parse(source)
    pdf_class = next(node for node in tree.body if isinstance(node, ast.ClassDef) and node.name == "Pdf")
    call = next(node for node in pdf_class.body if isinstance(node, ast.FunctionDef) and node.name == "__call__")
    info = next(node for node in pdf_class.body if isinstance(node, ast.FunctionDef) and node.name == "get_tbls_info")
    assert ast.get_source_segment(source, call) in inspect.getsource(qa_module.Pdf.__call__)
    assert ast.get_source_segment(source, info) in inspect.getsource(qa_module.Pdf.get_tbls_info)


def test_single_page_tags_keep_pairs_and_insert_table_between_them(qa_module):
    pages = [_page(300, 300, (255, 255, 255))]
    boxes = [
        _preamble(),
        _box("1. first question?", 1, 10, 80, 30, 36),
        _box("first answer stays", 1, 10, 80, 44, 50, layout_type="text"),
        _box("2. second question?", 1, 10, 80, 70, 76),
        _box("second answer stays", 1, 10, 80, 84, 90, layout_type="text"),
    ]
    tables = [_table("TABLE-A", 0, 10, 40, 54, 58)]
    prepared = _PreparedPdf(qa_module.Pdf, boxes, tables, pages)

    qai_list, returned_tables = prepared(zoomin=3)

    assert [item[0] for item in qai_list] == ["1. ", "2. "]
    assert qai_list[0][1] == "first question?first answer staysTABLE-A"
    assert qai_list[1][1] == "second question?second answer stays"
    assert "TABLE-A" not in qai_list[1][1]
    assert qai_list[0][3][0][0] == 0
    assert returned_tables == tables
    assert prepared.calls[0] == ("images", 3, 0)


def test_split_adjacent_tags_do_not_raise_and_use_last_fragment_bottom(qa_module):
    # Page 1 is narrower, so get_position clips x1 and _line_tag emits two tags.
    pages = [_page(150, 120, (255, 0, 0)), _page(360, 120, (0, 255, 0))]
    boxes = [
        _preamble(),
        _box("1. cross question?", 1, 10, 40, 30, 36),
        _box("cross answer body", 1, 10, 80, 10, 50, layout_type="text"),
        _box("2. later question?", 2, 10, 40, 50, 56),
    ]
    # Last fragment is page 2 bottom 10, after this table. Pairing first-page
    # pn with that bottom would instead consume the table a second time.
    tables = [_table("EARLY-TABLE", 1, 10, 40, 5, 8)]
    prepared = _PreparedPdf(qa_module.Pdf, boxes, tables, pages)

    qai_list, _tables = prepared(zoomin=3)

    assert [item[0] for item in qai_list] == ["1. ", "2. "]
    assert qai_list[0][1] == "cross question?cross answer body"
    assert "EARLY-TABLE" not in qai_list[0][1]
    assert "EARLY-TABLE" not in qai_list[1][1]
    positions = qai_list[0][3]
    assert [item[0] for item in positions] == [0, 0, 1]
    assert positions[-1][4] == pytest.approx(10)


def test_compressed_multipage_tag_uses_last_page_not_first_page_number(qa_module):
    pages = [_page(300, 120, (10, 0, 0)), _page(300, 120, (0, 10, 0)), _page(300, 120, (0, 0, 10))]
    boxes = [
        _preamble(),
        _box("1. spans pages?", 1, 10, 40, 30, 90),
        _box("2. after span?", 3, 10, 40, 100, 106),
    ]
    # Old get_float('1-3') raised. First-page pn + last bottom would also swallow
    # this middle-page table. Last page of the compressed tag is 3, bottom 10.
    tables = [_table("MID-TABLE", 1, 10, 40, 10, 16)]
    prepared = _PreparedPdf(qa_module.Pdf, boxes, tables, pages)

    qai_list, _tables = prepared(zoomin=3)

    assert qai_list[0][0] == "1. "
    assert "MID-TABLE" not in qai_list[0][1]
    assert "MID-TABLE" not in qai_list[1][1]
    assert [item[0] for item in qai_list[0][3]] == [0, 1, 2]


def test_empty_question_tag_keeps_text_and_does_not_advance_table_cursor(qa_module):
    pages = [_page(300, 240, (255, 255, 255))]
    boxes = [
        _preamble(),
        {
            "text": "1. unpositioned question?",
            "layout_type": "title",
            "x0": float("nan"),
            "x1": 40,
            "top": 30,
            "bottom": 36,
        },
        _box("2. later question?", 1, 10, 40, 50, 56),
    ]
    tables = [_table("SHOULD-STAY", 0, 10, 40, 22, 28)]
    prepared = _PreparedPdf(qa_module.Pdf, boxes, tables, pages)

    qai_list, _tables = prepared(zoomin=3)

    assert [item[0] for item in qai_list] == ["1. ", "2. "]
    assert qai_list[0][1].startswith("unpositioned question?")
    # A later positioned row can attach a table to this unpositioned question.
    assert qai_list[0][2] is not None
    assert "SHOULD-STAY" in qai_list[0][1]
    assert "SHOULD-STAY" not in qai_list[1][1]


def test_empty_answer_tag_keeps_answer_text_without_index_error(qa_module):
    pages = [_page(300, 240, (255, 255, 255))]
    boxes = [
        _preamble(),
        _box("1. question stays?", 1, 10, 40, 30, 36),
        {
            "text": "answer without geometry",
            "layout_type": "text",
            "page_number": 1,
            "x0": 10,
            "x1": 40,
            "top": float("nan"),
            "bottom": 28,
        },
        _box("tail answer", 1, 10, 40, 60, 66, layout_type="text"),
        _box("2. next question?", 1, 10, 40, 80, 86),
    ]
    tables = [_table("KEPT-TABLE", 0, 10, 40, 50, 56)]
    prepared = _PreparedPdf(qa_module.Pdf, boxes, tables, pages)

    qai_list, _tables = prepared(zoomin=3)

    assert qai_list[0][1] == "question stays?answer without geometryKEPT-TABLEtail answer"
    assert qai_list[1][0] == "2. "


def test_from_page_offsets_crop_positions(qa_module):
    pages = [_page(300, 240, (255, 255, 255))]
    boxes = [
        _preamble(),
        _box("1. offset question?", 1, 10, 40, 30, 36),
        _box("offset answer", 1, 10, 40, 44, 50, layout_type="text"),
        _box("2. next offset?", 1, 10, 40, 70, 76),
    ]
    tables = [_table("OFFSET-TABLE", 0, 10, 40, 54, 58)]
    prepared = _PreparedPdf(qa_module.Pdf, boxes, tables, pages, page_from=4)

    qai_list, _tables = prepared(zoomin=3, from_page=4)

    assert prepared.calls[0] == ("images", 3, 4)
    assert qai_list[0][3][0][0] == 4
    assert "OFFSET-TABLE" in qai_list[0][1]
    assert "OFFSET-TABLE" not in qai_list[1][1]


def test_get_tbls_info_reads_table_position_not_text(qa_module):
    parser = qa_module.Pdf.__new__(qa_module.Pdf)
    tables = [_table("body", 6, 1.25, 8.75, 3.5, 9.5)]

    pn, left, right, top, bottom, tag, text = parser.get_tbls_info(tables, 0)

    assert (pn, left, right, top, bottom, text) == (7, 1.25, 8.75, 3.5, 9.5, "body")
    assert tag == "@@7\t1.2\t8.8\t3.5\t9.5##"
    assert parser.get_tbls_info(tables, 1)[0] == 1


def test_empty_question_and_answer_defer_table_until_next_positioned_row(qa_module):
    pages = [_page(300, 300, "white")]
    boxes = [
        _preamble(),
        _box("1. geometry missing?", 0, 10, 40, 20, 28),
        _box("empty answer stays", 0, 10, 40, 32, 38, layout_type="text"),
        _box("positioned continuation", 1, 10, 40, 60, 68, layout_type="text"),
        _box("2. next question?", 1, 10, 40, 80, 88),
    ]
    prepared = _PreparedPdf(qa_module.Pdf, boxes, [_table("DEFERRED", 0, 10, 40, 40, 48)], pages)
    result, _tables = prepared(zoomin=3)
    assert result[0][1] == "geometry missing?empty answer staysDEFERREDpositioned continuation"
    assert "DEFERRED" not in result[1][1]


def test_final_empty_tag_keeps_text_without_guessing_trailing_table_position(qa_module):
    pages = [_page(300, 300, "white")]
    boxes = [_preamble(), _box("1. question?", 1, 10, 40, 20, 28), _box("final unpositioned answer", 0, 10, 40, 35, 40, layout_type="text")]
    tables = [_table("TRAILING", 0, 10, 40, 60, 68)]
    result, returned_tables = _PreparedPdf(qa_module.Pdf, boxes, tables, pages)(zoomin=3)
    assert result[0][1] == "question?final unpositioned answer"
    assert "TRAILING" not in result[0][1]
    assert returned_tables == tables


def test_no_table_sentinel_does_not_loop_before_first_page_content(qa_module):
    pages = [_page(300, 300, "white")]
    boxes = [_preamble(), _box("1. unpositioned?", 0, 10, 40, 20, 28), _box("positioned answer", 1, 10, 40, 40, 48, layout_type="text")]
    result, tables = _PreparedPdf(qa_module.Pdf, boxes, [], pages)(zoomin=3)
    assert result[0][1] == "unpositioned?positioned answer"
    assert tables == []
